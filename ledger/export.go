package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// Export is one written intake directory: the ledger commit it was read
// from, the destination, and the exported Slices in declaration order.
type Export struct {
	Project     string   `json:"project"`
	Repository  string   `json:"repository"`
	Proposal    string   `json:"proposal"`
	Commit      string   `json:"commit"`
	Destination string   `json:"destination"`
	Slices      []string `json:"slices"`
}

// ExportProposal writes one active accepted Proposal of the repository's
// Project to destination in the intake layout LoadDeclaration reads, so that
// accepting the export on the same ledger reports it unchanged. It reads one
// committed revision, takes no mutation lock and writes nothing to the
// ledger. Every refusal precedes the first write to destination.
func ExportProposal(store *Store, repository github.RepositoryID, proposalName, destination string) (*Export, error) {
	destination, err := filepath.Abs(destination)
	if err != nil {
		return nil, err
	}
	if err := store.refuseExportDestination(destination); err != nil {
		return nil, err
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		return nil, err
	}
	declaration, files, err := snapshot.exportDeclaration(repository, proposalName)
	if err != nil {
		return nil, err
	}
	contents, err := json.MarshalIndent(declaration, "", "  ")
	if err != nil {
		return nil, err
	}
	files["proposal.json"] = append(contents, '\n')
	for name, contents := range files {
		path := filepath.Join(destination, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			return nil, err
		}
	}
	return &Export{
		Project: repository.Name, Repository: repository.Owner + "/" + repository.Name, Proposal: proposalName,
		Commit: snapshot.Revision, Destination: destination, Slices: declaration.SliceNames(),
	}, nil
}

// refuseExportDestination accepts an absent or empty directory outside the
// ledger clone.
func (s *Store) refuseExportDestination(destination string) error {
	root, err := filepath.EvalSymlinks(s.Root)
	if err != nil {
		return err
	}
	resolved := destination
	for parent := destination; ; parent = filepath.Dir(parent) {
		if real, err := filepath.EvalSymlinks(parent); err == nil {
			resolved = filepath.Join(real, strings.TrimPrefix(destination, parent))
			break
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	if relative, err := filepath.Rel(root, resolved); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return refuse(
			"export destination "+destination+" is inside the ledger clone "+s.Root,
			"choose a destination outside the ledger clone",
		)
	}
	entries, err := os.ReadDir(destination)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return refuse(
			"export destination "+destination+" is not a readable directory: "+err.Error(),
			"choose an absent or empty directory",
		)
	case len(entries) > 0:
		return refuse(
			"export destination "+destination+" is not empty",
			"choose an absent or empty directory; export never overwrites or removes files",
		)
	}
	return nil
}

// exportDeclaration synthesizes the intake declaration of one active
// Proposal from its records at this revision, and returns the description
// and frozen Contract bytes keyed by their intake path.
func (v *Snapshot) exportDeclaration(repository github.RepositoryID, name string) (*ProposalDeclaration, map[string][]byte, error) {
	identity := repository.Owner + "/" + repository.Name
	project := v.projects[repository.Name]
	if project == nil {
		return nil, nil, refuse(
			"no Project for "+identity+" at ledger revision "+v.Revision,
			"accept a Proposal for this repository first, or check --repo and --remote",
		)
	}
	var active, archived *proposalTree
	for _, candidate := range project.proposals {
		if candidate.name == name && candidate.archived {
			archived = candidate
		} else if candidate.name == name {
			active = candidate
		}
	}
	if active == nil && archived != nil {
		return nil, nil, refuse(
			"Proposal "+name+" of project "+project.name+" is archived at ledger revision "+v.Revision,
			"read its records with `skl browse proposal --project "+project.name+" --proposal "+name+" --archived`; archived Proposals are not exported",
		)
	}
	if active == nil {
		return nil, nil, refuse(
			"no active Proposal "+name+" in project "+project.name+" at ledger revision "+v.Revision,
			"select a Proposal listed by `skl browse project --project "+project.name+"`",
		)
	}
	read, err := v.readOne(project, active)
	if err != nil {
		return nil, nil, err
	}
	if read.repository != identity {
		return nil, nil, refuse(
			"project "+project.name+" is recorded for "+read.repository+", not "+identity,
			"choose the correct source repository or repair the ledger Project with human direction",
		)
	}
	proposal := read.proposals[0]
	if len(proposal.diagnostics) > 0 {
		return nil, nil, damagedExport(name, v.Revision, proposal.diagnostics[0].Problem)
	}
	directory := v.proposalPath(project.name, active)
	paths := []string{directory + "/proposal.md"}
	for _, slice := range active.slices {
		for file := range slice.files {
			if contractFiles[file] {
				paths = append(paths, directory+"/"+slice.name+"/"+file)
			}
		}
	}
	blobs, err := v.blobs(paths)
	if err != nil {
		return nil, nil, err
	}
	files := map[string][]byte{}
	for _, path := range paths {
		contents, present := blobs[path]
		if !present {
			return nil, nil, damagedExport(name, v.Revision, strings.TrimPrefix(path, directory+"/")+" is not a committed file")
		}
		files[strings.TrimPrefix(path, directory+"/")] = contents
	}
	declaration := &ProposalDeclaration{Proposal: name, Branch: proposal.meta.Branch, ParentTitle: proposal.meta.ParentTitle}
	for _, slice := range proposal.slices {
		if !slice.readable {
			return nil, nil, damagedExport(name, v.Revision, "the state of Slice "+slice.tree.name+" is unreadable")
		}
		for _, required := range []string{"intent.md", "behavior.md"} {
			if !slice.tree.files[required] {
				return nil, nil, damagedExport(name, v.Revision, "Slice "+slice.tree.name+" misses "+required)
			}
		}
		declaration.Slices = append(declaration.Slices, SliceDeclaration{
			Name: slice.tree.name, Title: slice.state.Title, Branch: slice.state.Branch,
			Depends: slice.state.Dependencies, Superseded: slice.state.State == Superseded,
		})
	}
	return declaration, files, nil
}

func damagedExport(proposal, revision, problem string) error {
	return refuse(
		"the record of Proposal "+proposal+" at ledger revision "+revision+" is incomplete: "+problem,
		"repair or restore the accepted record with human direction, then retry",
	)
}
