package ledger

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sameDependencies compares two canonical dependency lists ignoring order.
func sameDependencies(recorded, declared []string) bool {
	if len(recorded) != len(declared) {
		return false
	}
	seen := make(map[string]int, len(recorded))
	for _, reference := range recorded {
		seen[reference]++
	}
	for _, reference := range declared {
		seen[reference]--
		if seen[reference] < 0 {
			return false
		}
	}
	return true
}

// ForgeAttachment is one recorded forge attachment: the repository and issue
// number of a published descriptive issue.
type ForgeAttachment struct {
	Repository string `json:"repository"`
	Number     int    `json:"number"`
}

// PublicationNote is one durable pending-publication fact. Successful push
// needs no record; the clone's own refs show it.
type PublicationNote struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// PublicationState collects the pending-publication facts applicable to one
// slice. An absent field means no pending fact is known for it.
type PublicationState struct {
	Push  *PublicationNote `json:"push,omitempty"`
	Issue *PublicationNote `json:"issue,omitempty"`
}

// SliceState is the persisted state.json of one slice. It owns the current
// lifecycle, dependencies, planned source attachment, forge attachments,
// and pending-publication information; Contract bytes stay frozen.
type SliceState struct {
	State        string            `json:"state"`
	Title        string            `json:"title"`
	Branch       string            `json:"branch"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Issue        *ForgeAttachment  `json:"issue,omitempty"`
	Publication  *PublicationState `json:"publication,omitempty"`
}

// ProposalMeta is the persisted proposal.json: proposal metadata only. The
// directory membership defines the slices; no child inventory is stored.
type ProposalMeta struct {
	Accepted    string           `json:"accepted"`
	ParentTitle string           `json:"parent_title,omitempty"`
	ParentIssue *ForgeAttachment `json:"parent_issue,omitempty"`
}

// contractFileNames returns the frozen contract file names of one slice in a
// stable order.
func contractFileNames(contract map[string][]byte) []string {
	names := make([]string, 0, len(contract))
	for name := range contract {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// readSliceState reads the current state.json of one slice record.
func (s *Store) readSliceState(project, proposal, slice string) (SliceState, bool, error) {
	path := filepath.Join(s.Root, projectsRoot, project, "proposals", proposal, slice, "state.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SliceState{}, false, nil
		}
		return SliceState{}, false, err
	}
	var state SliceState
	if err := json.Unmarshal(contents, &state); err != nil {
		return SliceState{}, true, refuse(
			"state.json of "+ItemPath(proposal, slice)+" is unreadable",
			"repair or remove the damaged record with human direction, then retry",
		)
	}
	return state, true, nil
}

// readProposalMeta reads the current proposal.json of one proposal record.
func (s *Store) readProposalMeta(project, proposal string) (ProposalMeta, bool, error) {
	path := filepath.Join(s.Root, projectsRoot, project, "proposals", proposal, "proposal.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProposalMeta{}, false, nil
		}
		return ProposalMeta{}, false, err
	}
	var meta ProposalMeta
	if err := json.Unmarshal(contents, &meta); err != nil {
		return ProposalMeta{}, true, refuse(
			"proposal.json of proposals/"+proposal+" is unreadable",
			"repair or remove the damaged record with human direction, then retry",
		)
	}
	return meta, true, nil
}

// writeSliceState writes one slice's state.json.
func (s *Store) writeSliceState(project, proposal, slice string, state SliceState) error {
	path := filepath.Join(s.Root, projectsRoot, project, "proposals", proposal, slice)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(path, "state.json"), state)
}

// writeProposalMeta writes one proposal's proposal.json.
func (s *Store) writeProposalMeta(project, proposal string, meta ProposalMeta) error {
	path := filepath.Join(s.Root, projectsRoot, project, "proposals", proposal)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(path, "proposal.json"), meta)
}

// writeProposalDescription writes the durable proposal description.
func (s *Store) writeProposalDescription(project, proposal string, description []byte) error {
	path := filepath.Join(s.Root, projectsRoot, project, "proposals", proposal)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "proposal.md"), description, 0o644)
}

// writeContract freezes one slice's accepted contract bytes under its record.
func (s *Store) writeContract(project, proposal string, slice SliceDeclaration) error {
	path := filepath.Join(s.Root, projectsRoot, project, "proposals", proposal, slice.Name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	for name, contents := range slice.contract {
		if err := os.WriteFile(filepath.Join(path, name), contents, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, value any) error {
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(contents, '\n'), 0o644)
}

// compareAccepted reports whether an existing accepted record set matches
// the incoming declaration exactly: the same contract bytes and declared
// relationships. Any difference names the first mismatching fact.
func (s *Store) compareAccepted(project string, declaration *ProposalDeclaration, meta ProposalMeta) error {
	directory := filepath.Join(s.Root, projectsRoot, project, "proposals", declaration.Proposal)
	description, err := os.ReadFile(filepath.Join(directory, "proposal.md"))
	if err != nil {
		return refuse(
			"existing record proposals/"+declaration.Proposal+" is incomplete: its proposal.md is unavailable",
			"restore the accepted record with human direction, or use a new proposal name",
		)
	}
	if !bytes.Equal(description, declaration.Description) {
		return refuse(
			"proposal "+declaration.Proposal+" is already accepted with a different proposal.md",
			"a changed Contract requires a renewed Proposal rather than in-place replacement",
		)
	}
	if strings.TrimSpace(meta.ParentTitle) != strings.TrimSpace(declaration.ParentTitle) {
		return refuse(
			"proposal "+declaration.Proposal+" is already accepted with a different parent grouping",
			"a changed declared relationship requires a renewed Proposal rather than in-place replacement",
		)
	}
	for index := range declaration.Slices {
		slice := declaration.Slices[index]
		state, found, err := s.readSliceState(project, declaration.Proposal, slice.Name)
		if err != nil {
			return err
		}
		if !found {
			return refuse(
				"proposal "+declaration.Proposal+" is already accepted without slice "+slice.Name,
				"a changed slice membership requires a renewed Proposal rather than in-place replacement",
			)
		}
		if state.Title != slice.Title || state.Branch != slice.Branch {
			return refuse(
				"slice "+slice.Name+" of proposal "+declaration.Proposal+" is already accepted with a different title or planned branch",
				"a changed declared relationship requires a renewed Proposal rather than in-place replacement",
			)
		}
		if !sameDependencies(state.Dependencies, declaration.CanonicalDependencies(slice)) {
			return refuse(
				"slice "+slice.Name+" of proposal "+declaration.Proposal+" is already accepted with different dependencies",
				"a changed declared relationship requires a renewed Proposal rather than in-place replacement",
			)
		}
		for name, contents := range slice.contract {
			accepted, err := os.ReadFile(filepath.Join(directory, slice.Name, name))
			if err != nil {
				return refuse(
					"existing record of slice "+slice.Name+" is incomplete: "+name+" is unavailable",
					"restore the accepted record with human direction, or use a new proposal name",
				)
			}
			if !bytes.Equal(accepted, contents) {
				return refuse(
					"contract file "+name+" of slice "+slice.Name+" is already accepted with different content",
					"a changed Contract requires a renewed Proposal rather than in-place replacement",
				)
			}
		}
		for _, name := range []string{"intent.md", "behavior.md", "plan.md", "tasks.md"} {
			if _, wanted := slice.contract[name]; wanted {
				continue
			}
			if _, err := os.Stat(filepath.Join(directory, slice.Name, name)); err == nil {
				return refuse(
					"slice "+slice.Name+" is already accepted with a "+name+" the renewed declaration omits",
					"a changed Contract requires a renewed Proposal rather than in-place replacement",
				)
			}
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	expected := make(map[string]bool, len(declaration.Slices)+2)
	expected["proposal.json"] = true
	expected["proposal.md"] = true
	for index := range declaration.Slices {
		expected[declaration.Slices[index].Name] = true
	}
	var unexpected []string
	for _, entry := range entries {
		if !expected[entry.Name()] {
			unexpected = append(unexpected, entry.Name())
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return refuse(
			"existing record proposals/"+declaration.Proposal+" holds unexpected entries: "+strings.Join(unexpected, ", "),
			"a changed slice membership requires a renewed Proposal rather than in-place replacement",
		)
	}
	return nil
}
