package ledger

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// ContractDocument is one accepted document with its exact ledger
// reference: the full commit at which the path is valid and the path
// itself.
type ContractDocument struct {
	Path     string `json:"path"`
	Commit   string `json:"commit"`
	Contents string `json:"contents"`
}

// Readback is the complete readback of one accepted slice: its recorded
// facts and the exact bytes of every accepted document.
type Readback struct {
	Project       string             `json:"project"`
	Repository    string             `json:"repository"`
	Proposal      string             `json:"proposal"`
	Slice         string             `json:"slice"`
	Item          string             `json:"item"`
	State         string             `json:"state"`
	Title         string             `json:"title"`
	Branch        string             `json:"branch"`
	Dependencies  []DependencyState  `json:"dependencies"`
	Issue         *ForgeAttachment   `json:"issue,omitempty"`
	ParentIssue   *ForgeAttachment   `json:"parent_issue,omitempty"`
	ParentPending *PublicationNote   `json:"parent_pending_publication,omitempty"`
	Pending       *PublicationState  `json:"pending_publication,omitempty"`
	Documents     []ContractDocument `json:"documents"`
}

// DependencyState is one recorded dependency with the blocker's currently
// recorded state, when its record is readable locally.
type DependencyState struct {
	Item  string `json:"item"`
	State string `json:"state,omitempty"`
}

// ShowItem returns the current accepted record of one slice from local
// ledger records. It reads the committed head, needs no forge access, no
// source markers, and no history search.
func ShowItem(store *Store, repository github.RepositoryID, item string) (*Readback, error) {
	proposal, slice, found := strings.Cut(item, "/")
	if !found || !ValidRecordName(proposal) || !ValidRecordName(slice) {
		return nil, refuse(
			"Work Item reference "+item+" is not a proposal/slice identity",
			"use the identity and readback command the acceptance reported, such as add-order-cancellation/foundation",
		)
	}
	head, err := store.head()
	if err != nil {
		return nil, err
	}
	projectName := repository.Name
	identity := repository.Owner + "/" + repository.Name
	var project ProjectIdentity
	if err := readJSONAt(store, head, filepath.Join(projectsRoot, projectName, "project.json"), &project); err != nil {
		return nil, refuse(
			"no committed Project record for "+identity+" at ledger head "+head,
			"check the configured ledger and source repository; skl reads no uncommitted or forge substitute",
		)
	}
	if project.Repository != identity {
		return nil, refuse(
			"project "+projectName+" belongs to "+project.Repository+", which is a different repository from "+identity,
			"choose the correct source repository or repair the ledger Project with human direction",
		)
	}
	proposalDirectory := store.proposalDirectoryAt(head, projectName, proposal)
	directory := filepath.Join(proposalDirectory, slice)
	var state SliceState
	if err := readJSONAt(store, head, filepath.Join(directory, "state.json"), &state); err != nil {
		return nil, refuse(
			"no accepted record for "+item+" in project "+projectName+" at committed ledger head "+head,
			"check the identity with the acceptance output, or supply an exact --commit and --path reference",
		)
	}
	var meta ProposalMeta
	if err := readJSONAt(store, head, filepath.Join(proposalDirectory, "proposal.json"), &meta); err != nil {
		return nil, refuse(
			"committed record "+proposalDirectory+" has no readable proposal.json at ledger head "+head,
			"repair or restore the damaged record with human direction",
		)
	}
	readback := &Readback{
		Project: projectName, Repository: project.Repository,
		Proposal: proposal, Slice: slice, Item: item,
		State: state.State, Title: state.Title, Branch: state.Branch,
		Issue: state.Issue, ParentIssue: meta.ParentIssue, ParentPending: meta.ParentPublication, Pending: state.Publication,
	}
	for _, dependency := range state.Dependencies {
		dependencyState := DependencyState{Item: dependency}
		if blocker, err := store.readStateByReferenceAt(head, projectName, dependency); err == nil {
			dependencyState.State = blocker
		}
		readback.Dependencies = append(readback.Dependencies, dependencyState)
	}
	names, err := acceptedFileNamesAt(store, head, directory)
	if err != nil {
		return nil, refuse(
			"committed record of "+item+" is unreadable at ledger head "+head+": "+err.Error(),
			"repair or restore the complete accepted record with human direction",
		)
	}
	for _, required := range []string{"behavior.md", "intent.md"} {
		if !containsString(names, required) {
			return nil, refuse(
				"committed record of "+item+" is incomplete at ledger head "+head+": "+required+" is missing",
				"repair or restore the complete accepted record with human direction; skl never omits accepted obligations silently",
			)
		}
	}
	for _, name := range names {
		path := filepath.ToSlash(filepath.Join(directory, name))
		contents, err := showPath(store, head, path)
		if err != nil {
			return nil, refuse(
				"committed contract "+filepath.ToSlash(path)+" is unavailable at ledger head "+head+": "+err.Error(),
				"repair or restore the complete accepted record with human direction; skl substitutes no working-tree or forge content",
			)
		}
		readback.Documents = append(readback.Documents, ContractDocument{
			Path: filepath.ToSlash(path), Commit: head, Contents: contents,
		})
	}
	if len(readback.Documents) == 0 {
		return nil, refuse(
			"committed record of "+item+" holds no contract files at ledger head "+head,
			"repair or restore the complete accepted record with human direction",
		)
	}
	return readback, nil
}

// ShowReference returns the exact content at one explicit ledger reference.
// A missing commit or path is diagnosed, never substituted.
func ShowReference(store *Store, commit, path string) (ContractDocument, error) {
	if len(commit) != 40 || strings.ContainsAny(commit, "ghijklmnopqrstuvwxyz") {
		return ContractDocument{}, refuse(
			"reference commit "+commit+" is not a full commit SHA",
			"supply the full 40-character commit SHA the acceptance or readback reported",
		)
	}
	resolved, err := git(store.Root, "rev-parse", "--verify", "--end-of-options", commit+"^{commit}")
	if err != nil || resolved != commit {
		return ContractDocument{}, refuse(
			"reference commit "+commit+" is unavailable in the ledger clone",
			"fetch that exact commit or correct the reference; skl substitutes no other revision",
		)
	}
	path = strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+path)), "/")
	if path == "" || strings.HasPrefix(path, ".") || strings.Contains(path, "../") {
		return ContractDocument{}, refuse(
			"reference path "+path+" is not a ledger record path",
			"supply a path inside the ledger, such as projects/<project>/proposals/<proposal>/<slice>/intent.md",
		)
	}
	contents, err := showPath(store, commit, path)
	if err != nil {
		return ContractDocument{}, refuse(
			"path "+path+" is unavailable at commit "+commit+": "+err.Error(),
			"correct the path or choose the revision that holds it; skl substitutes no other revision, forge body, or source artifact",
		)
	}
	return ContractDocument{Path: path, Commit: commit, Contents: contents}, nil
}

// showPath reads one path at one exact revision, preserving bytes and
// reporting the concrete Git cause of any miss.
func showPath(store *Store, commit, path string) (string, error) {
	raw, err := exec.Command("git", "-C", store.Root, "show", commit+":"+path).Output()
	if err != nil {
		return "", gitError(store.Root, []string{"show", commit + ":" + path}, err)
	}
	return string(raw), nil
}

func readJSONAt(store *Store, commit, path string, destination any) error {
	contents, err := showPath(store, commit, filepath.ToSlash(path))
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(contents), destination); err != nil {
		return fmt.Errorf("decode %s at %s: %w", filepath.ToSlash(path), commit, err)
	}
	return nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// acceptedFileNamesAt lists the contract files committed in one slice tree.
func acceptedFileNamesAt(store *Store, commit, directory string) ([]string, error) {
	contents, err := git(store.Root, "ls-tree", "--name-only", commit+":"+filepath.ToSlash(directory))
	if err != nil {
		return nil, gitError(store.Root, []string{"ls-tree", "--name-only", commit + ":" + filepath.ToSlash(directory)}, err)
	}
	var names []string
	for _, name := range strings.Fields(contents) {
		if contractFiles[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// readStateByReferenceAt reads a dependency state from the same committed
// ledger revision as the surrounding readback. An archived blocker keeps its
// identity and recorded lifecycle.
func (s *Store) readStateByReferenceAt(commit, project, reference string) (string, error) {
	trimmed := strings.TrimPrefix(reference, "proposals/")
	proposal, slice, found := strings.Cut(trimmed, "/")
	if !found {
		return "", fmt.Errorf("reference %s is not a Work Item reference", reference)
	}
	var state SliceState
	if err := readJSONAt(s, commit, filepath.Join(s.proposalDirectoryAt(commit, project, proposal), slice, "state.json"), &state); err != nil {
		return "", err
	}
	return state.State, nil
}
