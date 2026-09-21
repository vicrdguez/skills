package ledger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vicrdguez/skills/github"
)

// ForgeIssue is one open descriptive forge issue observed during safe
// resolution of an uncertain publication.
type ForgeIssue struct {
	Number int
	Title  string
	Body   string
}

// Forge publishes descriptive human-facing issues through the existing
// forge adapter. It is a transport, never an authority: skl does not author
// prose, mine labels or comments, or read state from it.
type Forge interface {
	// CreateIssue publishes one descriptive issue and returns its number.
	CreateIssue(ctx context.Context, title, body string) (int, error)
	// ListOpenIssues lists open non-merge-request issues for safe resolution.
	ListOpenIssues(ctx context.Context) ([]ForgeIssue, error)
	// ListChildren lists the issue numbers grouped under one parent issue.
	ListChildren(ctx context.Context, parent int) ([]int, error)
	// AttachChild groups one child issue under its parent issue.
	AttachChild(ctx context.Context, parent, child int) error
}

// SliceAcceptance is the acceptance outcome of one slice.
type SliceAcceptance struct {
	Name         string           `json:"name"`
	Title        string           `json:"title"`
	Branch       string           `json:"branch"`
	Dependencies []string         `json:"dependencies,omitempty"`
	State        string           `json:"state"`
	Issue        *ForgeAttachment `json:"issue,omitempty"`
	IssueStatus  *PublicationNote `json:"issue_status,omitempty"`
	PushStatus   *PublicationNote `json:"push_status,omitempty"`
}

// Acceptance is the complete outcome of one acceptance invocation.
type Acceptance struct {
	// Status is "accepted" for a new local acceptance and "existing" when
	// the unchanged proposal was already accepted.
	Status      string           `json:"status"`
	Project     string           `json:"project"`
	Repository  string           `json:"repository"`
	Proposal    string           `json:"proposal"`
	ParentTitle string           `json:"parent_title,omitempty"`
	ParentIssue *ForgeAttachment `json:"parent_issue,omitempty"`
	ParentNote  *PublicationNote `json:"parent_note,omitempty"`
	// Commit is the full ledger commit that carries the accepted records
	// after this invocation's local writes.
	Commit  string            `json:"commit"`
	Slices  []SliceAcceptance `json:"slices"`
	HeadRef string            `json:"head_ref"`
}

// Accept validates and freezes one proposal into the local ledger, then
// attempts initial publication. The local commit is authoritative; push and
// issue publication are best-effort surfaces whose failures stay visibly
// pending without undoing acceptance.
func Accept(ctx context.Context, store *Store, repository github.RepositoryID, declaration *ProposalDeclaration, forge Forge, now func() time.Time) (*Acceptance, error) {
	// One whole-declaration gate before any write: existing Work Item
	// dependencies must resolve against the project's current records.
	project, err := store.resolveProject(repository)
	if err != nil {
		return nil, err
	}
	if err := store.resolveExternalDependencies(project.Name, declaration); err != nil {
		return nil, err
	}
	if err := store.requireCleanTree(); err != nil {
		return nil, err
	}

	identity := repository.Owner + "/" + repository.Name
	proposalDirectory := filepath.Join(store.Root, projectsRoot, project.Name, "proposals", declaration.Proposal)
	meta, recorded, err := store.readProposalMeta(project.Name, declaration.Proposal)
	if err != nil {
		return nil, err
	}
	outcome := &Acceptance{
		Project: project.Name, Repository: identity, Proposal: declaration.Proposal,
		ParentTitle: strings.TrimSpace(declaration.ParentTitle),
	}
	if _, err := os.Stat(proposalDirectory); err == nil {
		if !recorded {
			return nil, refuse(
				"record proposals/"+declaration.Proposal+" exists without a readable proposal.json",
				"repair or remove the damaged record with human direction, then retry",
			)
		}
		if err := store.compareAccepted(project.Name, declaration, meta); err != nil {
			return nil, err
		}
		outcome.Status = "existing"
		outcome.ParentIssue = meta.ParentIssue
		if err := store.loadAcceptedStates(project.Name, declaration, outcome); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	} else {
		if err := store.freeze(project, declaration, now()); err != nil {
			return nil, err
		}
		outcome.Status = "accepted"
		if err := store.loadAcceptedStates(project.Name, declaration, outcome); err != nil {
			return nil, err
		}
	}

	if err := Publish(ctx, store, project.Name, declaration, outcome, forge); err != nil {
		return nil, err
	}
	head, err := store.head()
	if err != nil {
		return nil, err
	}
	outcome.Commit = head
	outcome.HeadRef = fmt.Sprintf("%s/%s", projectsRoot, project.Name)
	return outcome, nil
}

// freeze writes the complete accepted record set and commits it as one
// local ledger mutation.
func (s *Store) freeze(project Project, declaration *ProposalDeclaration, accepted time.Time) error {
	if project.Created {
		if err := s.writeProject(project); err != nil {
			return err
		}
	}
	if err := s.writeProposalMeta(project.Name, declaration.Proposal, ProposalMeta{
		Accepted: accepted.UTC().Format(time.RFC3339), ParentTitle: strings.TrimSpace(declaration.ParentTitle),
	}); err != nil {
		return err
	}
	if err := s.writeProposalDescription(project.Name, declaration.Proposal, declaration.Description); err != nil {
		return err
	}
	for index := range declaration.Slices {
		slice := declaration.Slices[index]
		if err := s.writeContract(project.Name, declaration.Proposal, slice); err != nil {
			return err
		}
		if err := s.writeSliceState(project.Name, declaration.Proposal, slice.Name, SliceState{
			State: ReadyForImplementation, Title: slice.Title, Branch: slice.Branch,
			Dependencies: declaration.CanonicalDependencies(slice),
		}); err != nil {
			return err
		}
	}
	return s.commit("accept "+project.Name+"/"+declaration.Proposal, filepath.Join(projectsRoot, project.Name))
}

// loadAcceptedStates reads the just-written or existing slice states into
// the acceptance outcome.
func (s *Store) loadAcceptedStates(project string, declaration *ProposalDeclaration, outcome *Acceptance) error {
	for index := range declaration.Slices {
		slice := declaration.Slices[index]
		state, found, err := s.readSliceState(project, declaration.Proposal, slice.Name)
		if err != nil || !found {
			if err == nil {
				return fmt.Errorf("accepted record of slice %s is missing its state.json", slice.Name)
			}
			return err
		}
		acceptance := SliceAcceptance{
			Name: slice.Name, Title: state.Title, Branch: state.Branch,
			Dependencies: state.Dependencies, State: state.State, Issue: state.Issue,
		}
		if state.Publication != nil {
			if state.Publication.Issue != nil {
				acceptance.IssueStatus = state.Publication.Issue
			}
			if state.Publication.Push != nil {
				acceptance.PushStatus = state.Publication.Push
			}
		}
		outcome.Slices = append(outcome.Slices, acceptance)
	}
	sort.Slice(outcome.Slices, func(i, j int) bool { return outcome.Slices[i].Name < outcome.Slices[j].Name })
	return nil
}

// slice returns the acceptance outcome of one named slice.
func (a *Acceptance) slice(name string) *SliceAcceptance {
	for index := range a.Slices {
		if a.Slices[index].Name == name {
			return &a.Slices[index]
		}
	}
	return nil
}

// commit stages exactly the given record paths and records them as one
// brief local mutation. Staging explicit paths keeps unrelated work out of
// the acceptance commit.
func (s *Store) commit(message string, paths ...string) error {
	if len(paths) > 0 {
		if _, err := git(s.Root, append([]string{"add", "--"}, paths...)...); err != nil {
			return fmt.Errorf("stage the ledger records at %s: %v", s.Root, gitError(s.Root, []string{"add"}, err))
		}
	}
	if _, err := git(s.Root, "commit", "-m", message); err != nil {
		return fmt.Errorf("commit the ledger at %s: %v; configure user.name and user.email in the ledger clone and retry", s.Root, gitError(s.Root, []string{"commit"}, err))
	}
	return nil
}

// requireCleanTree refuses acceptance while the ledger holds uncommitted
// edits or leftovers of an interrupted write, so no acceptance absorbs
// unrelated work or claims success over an unsafe state.
func (s *Store) requireCleanTree() error {
	dirty, err := git(s.Root, "status", "--porcelain")
	if err != nil {
		return err
	}
	if dirty == "" {
		return nil
	}
	return refuse(
		"the ledger clone at "+s.Root+" has uncommitted changes",
		"commit or restore them, or remove leftovers of an interrupted write, and keep unrelated work out of the acceptance commit; then retry",
	)
}

// resolveExternalDependencies verifies every dependency that is not a
// declared sibling names a recorded Work Item of this project.
func (s *Store) resolveExternalDependencies(project string, declaration *ProposalDeclaration) error {
	declared := make(map[string]bool, len(declaration.Slices))
	for index := range declaration.Slices {
		declared[declaration.Slices[index].Name] = true
	}
	for index := range declaration.Slices {
		slice := declaration.Slices[index]
		for _, dependency := range slice.Depends {
			dependency = declaration.normalizeDependency(dependency)
			if declared[dependency] {
				continue
			}
			if _, _, valid := workItemReference(dependency); !valid {
				return refuse(
					"dependency "+dependency+" of slice "+slice.Name+" does not resolve",
					"reference a declared sibling slice or an existing ledger Work Item as proposals/<proposal>/<slice>",
				)
			}
			if _, err := os.Stat(filepath.Join(s.Root, projectsRoot, project, dependency, "state.json")); err != nil {
				return refuse(
					"dependency "+dependency+" of slice "+slice.Name+" is not recorded in project "+project,
					"accept that Work Item first or correct the reference to a recorded one",
				)
			}
		}
	}
	return nil
}
