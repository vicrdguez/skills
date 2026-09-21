package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// contractFiles are the only file names a slice directory may freeze.
// intent.md and behavior.md are required; plan.md and tasks.md are copied
// only when the author supplied them.
var contractFiles = map[string]bool{"intent.md": true, "behavior.md": true, "plan.md": true, "tasks.md": true}

// SliceDeclaration is one declared slice of a proposal intake.
type SliceDeclaration struct {
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Branch   string   `json:"branch"`
	Depends  []string `json:"depends"`
	contract map[string][]byte
}

// ProposalDeclaration is the parsed intake declaration of one proposal.
type ProposalDeclaration struct {
	Proposal    string             `json:"proposal"`
	ParentTitle string             `json:"parent_title"`
	Slices      []SliceDeclaration `json:"slices"`
	Description []byte
	IssueBodies map[string][]byte
	ParentBody  []byte
}

// LoadDeclaration reads and validates the complete intake declaration from
// directory before anything is accepted. bodies supplies the temporary
// descriptive issue bodies keyed by slice name, plus the optional parent
// body; missing bodies are not declaration errors — they leave that
// publication surface pending.
func LoadDeclaration(directory string, bodies map[string][]byte, parentBody []byte) (*ProposalDeclaration, error) {
	raw, err := os.ReadFile(filepath.Join(directory, "proposal.json"))
	if err != nil {
		return nil, refuse("proposal declaration "+filepath.Join(directory, "proposal.json")+" is unavailable", "author the intake declaration with a proposal name, slice list, and optional dependencies, then retry")
	}
	var declaration ProposalDeclaration
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&declaration); err != nil {
		return nil, refuse("proposal declaration is malformed: "+err.Error(), "correct proposal.json so skl can read the proposal and its slices, then retry")
	}
	if !ValidRecordName(declaration.Proposal) {
		return nil, refuse("proposal name "+declaration.Proposal+" is not a valid record name", "use a lowercase kebab-case name such as add-order-cancellation")
	}
	if len(declaration.Slices) == 0 {
		return nil, refuse("proposal declaration declares no slices", "declare at least one slice with its contract files")
	}
	multi := len(declaration.Slices) > 1
	if multi && strings.TrimSpace(declaration.ParentTitle) == "" {
		return nil, refuse("multi-slice proposals require a parent_title", "add a parent_title that names the human-facing coordination grouping")
	}
	if !multi && strings.TrimSpace(declaration.ParentTitle) != "" {
		return nil, refuse("single-slice proposals take no parent_title", "remove parent_title; a single-slice proposal has no coordination parent")
	}
	if !multi && parentBody != nil {
		return nil, refuse("single-slice proposals take no parent body", "remove --parent-body; a single-slice proposal has no coordination parent")
	}
	description, err := os.ReadFile(filepath.Join(directory, "proposal.md"))
	if err != nil {
		return nil, refuse("proposal description "+filepath.Join(directory, "proposal.md")+" is unavailable", "author the durable proposal description, then retry")
	}
	declaration.Description = description
	declaration.IssueBodies = make(map[string][]byte, len(bodies))
	for name, body := range bodies {
		declaration.IssueBodies[name] = body
	}
	declaration.ParentBody = parentBody

	seen := make(map[string]bool, len(declaration.Slices))
	names := make([]string, 0, len(declaration.Slices))
	for index := range declaration.Slices {
		slice := &declaration.Slices[index]
		if !ValidRecordName(slice.Name) {
			return nil, refuse("slice name "+slice.Name+" is not a valid record name", "use lowercase kebab-case slice names that cannot escape the proposal directory")
		}
		if seen[slice.Name] {
			return nil, refuse("slice "+slice.Name+" is declared more than once", "give every slice a distinct name; the shared proposal directory already groups them")
		}
		seen[slice.Name] = true
		names = append(names, slice.Name)
		if strings.TrimSpace(slice.Title) == "" {
			slice.Title = slice.Name
		}
		if slice.Branch == "" {
			slice.Branch = slice.Name
		}
		if err := validBranch(slice.Branch); err != nil {
			return nil, refuse("slice "+slice.Name+" has an invalid planned branch "+slice.Branch, "use a branch name Git can create, such as "+slice.Name)
		}
		contract, err := loadContract(filepath.Join(directory, slice.Name))
		if err != nil {
			return nil, err
		}
		slice.contract = contract
	}
	sort.Strings(names)
	for index := range declaration.Slices {
		slice := &declaration.Slices[index]
		for _, dependency := range slice.Depends {
			// A same-proposal path reference names a sibling; anything else
			// must resolve against the project's existing records.
			if sibling, ok := sameProposalReference(declaration.Proposal, dependency); ok {
				if !seen[sibling] {
					return nil, refuse("slice "+slice.Name+" depends on "+dependency+", which is not a declared slice", "declare the slice or correct the dependency name")
				}
				if sibling == slice.Name {
					return nil, refuse("slice "+slice.Name+" depends on itself", "remove the self-dependency")
				}
				continue
			}
			if dependency == slice.Name {
				return nil, refuse("slice "+slice.Name+" depends on itself", "remove the self-dependency")
			}
			if seen[dependency] {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(dependency, "proposals/"), "/")
			if len(parts) != 2 || !ValidRecordName(parts[0]) || !ValidRecordName(parts[1]) {
				return nil, refuse("dependency "+dependency+" of slice "+slice.Name+" does not resolve", "reference a declared sibling slice or an existing ledger Work Item as proposals/<proposal>/<slice>")
			}
		}
	}
	if cycle := declaration.Cycle(); cycle != "" {
		return nil, refuse("dependency graph contains a cycle through "+cycle, "remove the cyclic dependency edge")
	}
	return &declaration, nil
}

// ParentBodySupplied reports whether a parent body input was given; it is
// a publication input for multi-slice work, never a declaration field.
func (d *ProposalDeclaration) ParentBodySupplied() bool { return d.ParentBody != nil }

// Cycle returns one slice name on a dependency cycle, or "" when the
// declared sibling graph is acyclic.
func (d *ProposalDeclaration) Cycle() string {
	siblings := make(map[string][]string, len(d.Slices))
	for _, slice := range d.Slices {
		var edges []string
		for _, dependency := range slice.Depends {
			if sibling, ok := sameProposalReference(d.Proposal, dependency); ok {
				dependency = sibling
			}
			edges = append(edges, dependency)
		}
		siblings[slice.Name] = edges
	}
	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(siblings))
	var visit func(string) string
	visit = func(name string) string {
		switch state[name] {
		case done:
			return ""
		case visiting:
			return name
		}
		state[name] = visiting
		for _, next := range siblings[name] {
			if _, known := siblings[next]; !known {
				continue
			}
			if found := visit(next); found != "" {
				return found
			}
		}
		state[name] = done
		return ""
	}
	for _, slice := range d.Slices {
		if found := visit(slice.Name); found != "" {
			return found
		}
	}
	return ""
}

// loadContract reads one slice directory and refuses anything beyond the
// accepted contract files.
func loadContract(directory string) (map[string][]byte, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, refuse("slice directory "+directory+" is unavailable", "create one directory per slice holding its contract files, then retry")
	}
	contract := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, refuse("slice directory "+directory+" contains the nested directory "+entry.Name(), "keep the slice directory flat: only intent.md, behavior.md, and warranted plan.md/tasks.md belong in it")
		}
		if !contractFiles[entry.Name()] {
			return nil, refuse("slice directory "+directory+" contains the unexpected file "+entry.Name(), "keep only contract files in the slice directory: intent.md, behavior.md, and warranted plan.md/tasks.md")
		}
		contents, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filepath.Join(directory, entry.Name()), err)
		}
		contract[entry.Name()] = contents
	}
	for _, required := range []string{"intent.md", "behavior.md"} {
		if _, ok := contract[required]; !ok {
			return nil, refuse("slice directory "+directory+" misses "+required, "supply at least intent.md and behavior.md; add plan.md and tasks.md only when warranted")
		}
	}
	return contract, nil
}

// sameProposalReference normalizes a reference of the form
// proposals/<this-proposal>/<slice> to the sibling slice name.
func sameProposalReference(proposal, dependency string) (string, bool) {
	prefix := "proposals/" + proposal + "/"
	sibling, found := strings.CutPrefix(dependency, prefix)
	if !found || strings.Contains(sibling, "/") {
		return "", false
	}
	return sibling, true
}

// validBranch rejects planned branch identities Git could not create. It is
// a safety check, not a full ref-format parser.
func validBranch(branch string) error {
	if branch == "" || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") ||
		strings.Contains(branch, "..") || strings.Contains(branch, "//") || strings.HasSuffix(branch, ".lock") || strings.ContainsAny(branch, " ~^:?*[\\\x00") {
		return errors.New("invalid branch")
	}
	return nil
}

// CanonicalDependencies returns the ledger Work Item references a slice
// depends on, with sibling names resolved to their canonical paths.
func (d *ProposalDeclaration) CanonicalDependencies(slice SliceDeclaration) []string {
	var references []string
	for _, dependency := range slice.Depends {
		if sibling, ok := sameProposalReference(d.Proposal, dependency); ok {
			dependency = sibling
		}
		if d.sliceByName(dependency) != nil {
			references = append(references, ItemPath(d.Proposal, dependency))
			continue
		}
		references = append(references, dependency)
	}
	return references
}

func (d *ProposalDeclaration) sliceByName(name string) *SliceDeclaration {
	for index := range d.Slices {
		if d.Slices[index].Name == name {
			return &d.Slices[index]
		}
	}
	return nil
}
