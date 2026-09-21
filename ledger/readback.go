package ledger

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	Project      string             `json:"project"`
	Repository   string             `json:"repository"`
	Proposal     string             `json:"proposal"`
	Slice        string             `json:"slice"`
	Item         string             `json:"item"`
	State        string             `json:"state"`
	Title        string             `json:"title"`
	Branch       string             `json:"branch"`
	Dependencies []DependencyState  `json:"dependencies"`
	Issue        *ForgeAttachment   `json:"issue,omitempty"`
	ParentIssue  *ForgeAttachment   `json:"parent_issue,omitempty"`
	Pending      *PublicationState  `json:"pending_publication,omitempty"`
	Documents    []ContractDocument `json:"documents"`
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
	project, err := store.resolveProject(repository)
	if err != nil {
		return nil, err
	}
	proposal, slice, found := strings.Cut(item, "/")
	if !found || !ValidRecordName(proposal) || !ValidRecordName(slice) {
		return nil, refuse(
			"Work Item reference "+item+" is not a proposal/slice identity",
			"use the identity and readback command the acceptance reported, such as add-order-cancellation/foundation",
		)
	}
	state, found, err := store.readSliceState(project.Name, proposal, slice)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, refuse(
			"no accepted record for "+item+" in project "+project.Name,
			"check the identity with the acceptance output, or supply an exact --commit and --path reference",
		)
	}
	meta, recorded, err := store.readProposalMeta(project.Name, proposal)
	if err != nil {
		return nil, err
	}
	if !recorded {
		return nil, refuse(
			"record proposals/"+proposal+" has no readable proposal.json",
			"repair or remove the damaged record with human direction",
		)
	}
	head, err := store.head()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(projectsRoot, project.Name, "proposals", proposal, slice)
	readback := &Readback{
		Project: project.Name, Repository: project.Repository,
		Proposal: proposal, Slice: slice, Item: item,
		State: state.State, Title: state.Title, Branch: state.Branch,
		Issue: state.Issue, ParentIssue: meta.ParentIssue, Pending: state.Publication,
	}
	for _, dependency := range state.Dependencies {
		dependencyState := DependencyState{Item: dependency}
		if blocker, _, err := store.readStateByReference(project.Name, dependency); err == nil {
			dependencyState.State = blocker
		}
		readback.Dependencies = append(readback.Dependencies, dependencyState)
	}
	for _, name := range acceptedFileNames(store, directory) {
		contents, err := showPath(store, head, filepath.Join(directory, name))
		if err != nil {
			return nil, err
		}
		readback.Documents = append(readback.Documents, ContractDocument{
			Path: filepath.ToSlash(filepath.Join(directory, name)), Commit: head, Contents: contents,
		})
	}
	if len(readback.Documents) == 0 {
		return nil, refuse(
			"record of "+item+" holds no contract files",
			"repair or remove the damaged record with human direction",
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

// acceptedFileNames lists the contract files present in one slice record.
func acceptedFileNames(store *Store, directory string) []string {
	entries, err := os.ReadDir(filepath.Join(store.Root, directory))
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && contractFiles[entry.Name()] {
			names = append(names, entry.Name())
		}
	}
	return names
}

// readStateByReference reads the current state of a dependency reference.
func (s *Store) readStateByReference(project, reference string) (string, bool, error) {
	trimmed := strings.TrimPrefix(reference, "proposals/")
	proposal, slice, found := strings.Cut(trimmed, "/")
	if !found {
		return "", false, fmt.Errorf("reference %s is not a Work Item reference", reference)
	}
	state, found, err := s.readSliceState(project, proposal, slice)
	return state.State, found, err
}
