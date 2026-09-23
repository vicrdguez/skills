package ledger

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// SubmissionObservation is a narrow, normalized read of one exact owned PR.
// Only the forge adapter interprets provider fields; no public prose or
// mergeability is a completion fact.
type SubmissionObservation struct {
	State       string // open, merged, or closed (without merge)
	SourceHead  string
	MergeCommit string
}

type CompletionForge interface {
	ObserveSubmission(context.Context, ForgeAttachment, IntegrationTarget, string) (SubmissionObservation, error)
}

type CompletionResult struct {
	Item        string           `json:"item"`
	State       string           `json:"state"`
	Already     bool             `json:"already_recorded,omitempty"`
	Commit      string           `json:"ledger_commit,omitempty"`
	Replication *PublicationNote `json:"replication,omitempty"`
}

// ObserveCompletion reads one selected record, observes only its attached PR
// outside the lock, then revalidates that record and its phase evidence before
// recording a terminal fact. An unrelated ledger commit cannot stale the read.
func ObserveCompletion(ctx context.Context, s *Store, repository github.RepositoryID, item string, forge CompletionForge) (*CompletionResult, error) {
	state, directory, head, err := s.deliveryState(repository, item)
	if err != nil {
		return nil, err
	}
	result := &CompletionResult{Item: item, State: state.State}
	if state.State == Merged || state.State == Superseded {
		return result, nil // committed facts remain usable without the forge
	}
	identity := repository.Owner + "/" + repository.Name
	if state.Submission == nil {
		return result, refuse("no exact owned Submission is attached to "+item, "retain the stored state until ordinary publication attaches a Submission")
	}
	if state.Submission.Number <= 0 || !strings.EqualFold(state.Submission.Repository, identity) || state.Target == nil || !strings.EqualFold(state.Target.Repository, identity) || state.Target.Branch == "" {
		return result, refuse("the selected Submission or Integration Target attachment is incomplete or inconsistent", "inspect the selected ledger attachments; neither a forge search nor an assumed destination can repair identity")
	}
	// Pin report presence and bytes as well as the selected state. Reports can
	// change independently of state.json during another selected-item write.
	reports := make(map[string]string)
	for _, phase := range []string{ImplementPhase, WatchdogPhase} {
		path := directory + "/" + phase + "-report.md"
		if gitOK(s.Root, "cat-file", "-e", head+":"+path) {
			reports[path], err = showPath(s, head, path)
			if err != nil {
				return result, err
			}
		}
	}
	if forge == nil {
		return result, refuse("forge observation is unavailable", "retry status when the owned Submission can be read; no terminal fact was recorded")
	}
	observation, err := forge.ObserveSubmission(ctx, *state.Submission, *state.Target, state.Branch)
	if err != nil {
		return result, err
	}
	if observation.State == "open" {
		return result, nil
	}
	if observation.State != Merged && observation.State != "closed" {
		return result, refuse("owned Submission returned no confirmed terminal outcome", "inspect the exact PR and retry when merge or unmerged closure can be confirmed")
	}
	terminal := Superseded
	if observation.State == Merged {
		terminal = Merged
	}
	s.observeDeliveryUpstream(ctx)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var committed string
	pending := PublicationNote{Status: PushPending, Detail: "terminal ledger commit has not been replicated"}
	err = s.withMutation(func() error {
		if err := s.requireReconciled(); err != nil {
			return err
		}
		current, currentDirectory, currentHead, err := s.deliveryState(repository, item)
		if err != nil {
			return err
		}
		if currentDirectory != directory || !reflect.DeepEqual(current, state) {
			return refuse("selected Work Item changed during completion observation", "inspect the current Claim, attachments, lifecycle and reports, then observe again")
		}
		for _, phase := range []string{ImplementPhase, WatchdogPhase} {
			path := directory + "/" + phase + "-report.md"
			before, existed := reports[path]
			present := gitOK(s.Root, "cat-file", "-e", currentHead+":"+path)
			if present != existed {
				return refuse("selected phase results changed during completion observation", "inspect the current selected evidence and observe again")
			}
			if present {
				currentReport, err := showPath(s, currentHead, path)
				if err != nil || currentReport != before {
					return refuse("selected phase results changed during completion observation", "inspect the current selected evidence and observe again")
				}
			}
		}
		path := directory + "/state.json"
		if err := s.requireCleanPaths(path); err != nil {
			return err
		}
		current.State = terminal
		current.Completion = &TerminalEvidence{Submission: *state.Submission, Target: *state.Target, SourceHead: observation.SourceHead, MergeCommit: observation.MergeCommit}
		if current.Publication == nil {
			current.Publication = &PublicationState{}
		}
		current.Publication.Push = &pending
		if err := writeJSON(filepath.Join(s.Root, path), current); err != nil {
			return err
		}
		if err := s.commit("observe completion "+repository.Name+"/"+item, path); err != nil {
			return err
		}
		committed, err = s.head()
		return err
	})
	if err != nil {
		return result, err
	}
	result.State, result.Commit = terminal, committed
	note, _ := s.push(committed)
	result.Replication = &note
	// Publication bookkeeping is a separate selected-record mutation. It may
	// not roll back or edit an intervening worker's newer record.
	if note.Status == PushPushed {
		_ = s.withMutation(func() error {
			current, _, _, err := s.deliveryState(repository, item)
			if err != nil || current.State != terminal || !reflect.DeepEqual(current.Completion, &TerminalEvidence{Submission: *state.Submission, Target: *state.Target, SourceHead: observation.SourceHead, MergeCommit: observation.MergeCommit}) || current.Publication == nil || current.Publication.Push == nil || *current.Publication.Push != pending {
				return err
			}
			path := directory + "/state.json"
			if err := s.requireCleanPaths(path); err != nil {
				return err
			}
			current.Publication.Push = nil
			if err := writeJSON(filepath.Join(s.Root, path), current); err != nil {
				return err
			}
			return s.commit("record completion replication "+repository.Name+"/"+item, path)
		})
	} else {
		// The original terminal commit is locally authoritative even when the
		// remote cannot accept it; expose the concrete replication failure.
		result.Replication = &note
	}
	return result, nil
}

// StatusItem exposes selected committed facts, never public labels or prose.
type StatusItem struct {
	Item         string             `json:"item"`
	State        string             `json:"state"`
	Claimed      bool               `json:"claimed"`
	Submission   *ForgeAttachment   `json:"submission,omitempty"`
	Target       *IntegrationTarget `json:"integration_target,omitempty"`
	Completion   *TerminalEvidence  `json:"completion,omitempty"`
	Dependencies []DependencyState  `json:"dependencies,omitempty"`
	Pending      *PublicationState  `json:"pending_publication,omitempty"`
	Observation  string             `json:"observation,omitempty"`
}

type ProposalStatus struct {
	Proposal       string `json:"proposal"`
	FullyDelivered bool   `json:"fully_delivered"`
	Retireable     bool   `json:"retireable"`
}

// StatusItems enumerates only the selected project's current records, or one
// explicitly selected Work Item. Fixed-item reads never hydrate other slices
// except its named blockers.
func StatusItems(s *Store, repository github.RepositoryID, item string) ([]string, error) {
	if item != "" {
		_, _, _, err := s.deliveryState(repository, item)
		if err != nil {
			return nil, err
		}
		return []string{item}, nil
	}
	if _, err := s.projectIdentity(repository); err != nil {
		return nil, err
	}
	head, err := s.head()
	if err != nil {
		return nil, err
	}
	paths, err := git(s.Root, "ls-tree", "-r", "--name-only", head, "--", filepath.Join(projectsRoot, repository.Name, "proposals"))
	if err != nil {
		return nil, err
	}
	var items []string
	for _, path := range strings.Split(paths, "\n") {
		if strings.HasSuffix(path, "/state.json") {
			items = append(items, strings.TrimSuffix(strings.TrimPrefix(path, projectsRoot+"/"+repository.Name+"/proposals/"), "/state.json"))
		}
	}
	return items, nil
}

func (s *Store) projectIdentity(repository github.RepositoryID) (ProjectIdentity, error) {
	head, err := s.head()
	if err != nil {
		return ProjectIdentity{}, err
	}
	var project ProjectIdentity
	if err := readJSONAt(s, head, filepath.Join(projectsRoot, repository.Name, "project.json"), &project); err != nil || project.Repository != repository.Owner+"/"+repository.Name {
		return ProjectIdentity{}, refuse("no matching committed Project for "+repository.Owner+"/"+repository.Name, "select the correct source repository and configured ledger")
	}
	return project, nil
}

// CompletionStatus returns committed state after observation, including
// dependency and parent accounting. The parent is only computed for a project
// summary; a fixed-item request needs no unrelated child contents.
func CompletionStatus(s *Store, repository github.RepositoryID, items []string, projectSummary bool, problems map[string]string) ([]StatusItem, []ProposalStatus, error) {
	head, err := s.head()
	if err != nil {
		return nil, nil, err
	}
	var results []StatusItem
	parents := make(map[string][]StatusItem)
	for _, item := range items {
		directory, err := itemDirectory(repository.Name, item)
		if err != nil {
			return nil, nil, err
		}
		var state SliceState
		if err := readJSONAt(s, head, directory+"/state.json", &state); err != nil {
			return nil, nil, refuse("selected Work Item "+item+" has no readable committed state", "repair the selected record without substituting forge contents")
		}
		result := StatusItem{Item: item, State: state.State, Claimed: state.Claim != nil, Submission: state.Submission, Target: state.Target, Completion: state.Completion, Pending: state.Publication, Observation: problems[item]}
		for _, dependency := range state.Dependencies {
			entry := DependencyState{Item: dependency}
			entry.State, err = s.readStateByReferenceAt(head, repository.Name, dependency)
			if err != nil {
				entry.State = "unknown"
			}
			result.Dependencies = append(result.Dependencies, entry)
		}
		results = append(results, result)
		if projectSummary {
			proposal, _, _ := strings.Cut(item, "/")
			parents[proposal] = append(parents[proposal], result)
		}
	}
	var summaries []ProposalStatus
	for proposal, children := range parents {
		full, terminal, unclaimed := len(children) > 0, len(children) > 0, true
		for _, child := range children {
			full = full && child.State == Merged
			terminal = terminal && (child.State == Merged || child.State == Superseded)
			unclaimed = unclaimed && !child.Claimed
		}
		summaries = append(summaries, ProposalStatus{Proposal: proposal, FullyDelivered: full, Retireable: terminal && unclaimed})
	}
	// Project summaries use stable ordering independent of Go map iteration.
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Proposal < summaries[j].Proposal })
	return results, summaries, nil
}
