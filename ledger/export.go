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
	if err := store.requireExportDestination(destination); err != nil {
		return nil, err
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		return nil, err
	}
	declaration, err := snapshot.exportDeclaration(repository, proposalName)
	if err != nil {
		return nil, err
	}
	if err := writeIntake(destination, declaration); err != nil {
		return nil, err
	}
	return &Export{
		Project: repository.Name, Repository: repository.Owner + "/" + repository.Name, Proposal: proposalName,
		Commit: snapshot.Revision, Destination: destination, Slices: declaration.SliceNames(),
	}, nil
}

// writeIntake writes a declaration in the intake layout LoadDeclaration
// reads: proposal.json, proposal.md and one directory of Contract files per
// Slice.
func writeIntake(destination string, declaration *ProposalDeclaration) error {
	contents, err := json.MarshalIndent(declaration, "", "  ")
	if err != nil {
		return err
	}
	files := map[string][]byte{"proposal.json": append(contents, '\n'), "proposal.md": declaration.Description}
	for _, slice := range declaration.Slices {
		for name, contents := range slice.contract {
			files[filepath.Join(slice.Name, name)] = contents
		}
	}
	for name, contents := range files {
		path := filepath.Join(destination, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// requireExportDestination accepts an absent or empty directory outside the
// ledger clone.
func (s *Store) requireExportDestination(destination string) error {
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
// Proposal from its records at this revision, with its description and
// frozen Contract bytes.
func (v *Snapshot) exportDeclaration(repository github.RepositoryID, name string) (*ProposalDeclaration, error) {
	identity := repository.Owner + "/" + repository.Name
	project := v.projects[repository.Name]
	if project == nil {
		return nil, refuse(
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
		return nil, refuse(
			"Proposal "+name+" of project "+project.name+" is archived at ledger revision "+v.Revision,
			"read its records with `skl browse proposal --project "+project.name+" --proposal "+name+" --archived`; archived Proposals are not exported",
		)
	}
	if active == nil {
		return nil, refuse(
			"no active Proposal "+name+" in project "+project.name+" at ledger revision "+v.Revision,
			"select a Proposal listed by `skl browse project --project "+project.name+"`",
		)
	}
	read, err := v.readOne(project, active)
	if err != nil {
		return nil, err
	}
	if read.repository != identity {
		return nil, refuse(
			"project "+project.name+" is recorded for "+read.repository+", not "+identity,
			"choose the correct source repository or repair the ledger Project with human direction",
		)
	}
	proposal := read.proposals[0]
	if len(proposal.diagnostics) > 0 {
		return nil, damagedExport(name, v.Revision, proposal.diagnostics[0].Problem)
	}
	directory := v.proposalPath(project.name, active) + "/"
	paths := []string{directory + "proposal.md"}
	for _, slice := range active.slices {
		for file := range slice.files {
			if contractFiles[file] {
				paths = append(paths, directory+slice.name+"/"+file)
			}
		}
	}
	blobs, err := v.blobs(paths)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if _, present := blobs[path]; !present {
			return nil, damagedExport(name, v.Revision, strings.TrimPrefix(path, directory)+" is not a committed file")
		}
	}
	declaration := &ProposalDeclaration{Proposal: name, Branch: proposal.meta.Branch, ParentTitle: proposal.meta.ParentTitle, Description: blobs[directory+"proposal.md"]}
	for _, slice := range proposal.slices {
		if !slice.readable {
			return nil, damagedExport(name, v.Revision, "the state of Slice "+slice.tree.name+" is unreadable")
		}
		contract := map[string][]byte{}
		for file := range slice.tree.files {
			if contractFiles[file] {
				contract[file] = blobs[directory+slice.tree.name+"/"+file]
			}
		}
		for _, required := range []string{"intent.md", "behavior.md"} {
			if contract[required] == nil {
				return nil, damagedExport(name, v.Revision, "Slice "+slice.tree.name+" misses "+required)
			}
		}
		declaration.Slices = append(declaration.Slices, SliceDeclaration{
			Name: slice.tree.name, Title: slice.state.Title, Branch: slice.state.Branch,
			Depends: slice.state.Dependencies, Superseded: slice.state.State == Superseded, contract: contract,
		})
	}
	return declaration, nil
}

func damagedExport(proposal, revision, problem string) error {
	return refuse(
		"the record of Proposal "+proposal+" at ledger revision "+revision+" is incomplete: "+problem,
		"repair or restore the accepted record with human direction, then retry",
	)
}
