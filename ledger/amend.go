package ledger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// SliceAmendment is what one amendment changed about one Slice.
type SliceAmendment struct {
	Name  string `json:"name"`
	Added bool   `json:"added,omitempty"`
	// Contract names the replaced Contract files of a never-claimed Slice.
	Contract     []string `json:"contract,omitempty"`
	Title        bool     `json:"title,omitempty"`
	Branch       bool     `json:"branch,omitempty"`
	Dependencies bool     `json:"dependencies,omitempty"`
	// Superseded marks a Slice this amendment superseded.
	Superseded bool `json:"superseded,omitempty"`
	// Dependents are the active Slices this amendment left untouched that
	// depend on this changed Slice, as Work Item identities. A superseded
	// Slice lists them as Blocked instead, since there is nothing to confirm.
	Dependents []string `json:"dependents,omitempty"`
	// Blocked are the active Slices left depending on this superseded Slice,
	// as Work Item identities. The graph rule refuses any in its own Proposal,
	// so they belong to other Proposals.
	Blocked []string `json:"blocked,omitempty"`
}

// presented reports whether the change alters what the Slice's issue
// presents. A supersession alone contacts no forge.
func (c SliceAmendment) presented() bool {
	return c.Added || c.Title || c.Branch || c.Dependencies || len(c.Contract) > 0
}

// Amendment is the outcome of one amendment: what it changed, and the
// publication outcome of the Slices it lists.
type Amendment struct {
	// Acceptance carries the status, "amended" or "existing", the ledger
	// commit, and the listed Slices: every amended Slice, every Slice a
	// supplied body names, and every Slice not declared superseded when the
	// amendment may create the parent issue.
	Acceptance
	DescriptionReplaced bool             `json:"description_replaced,omitempty"`
	ParentTitleChanged  bool             `json:"parent_title_changed,omitempty"`
	Amended             []SliceAmendment `json:"amended,omitempty"`
	// Unpresented names the listed Slices no supplied body presented.
	Unpresented []string `json:"unpresented,omitempty"`
	// Push is the replication of the amended records.
	Push PublicationNote `json:"push"`
}

// Amend records every permitted difference between an intake declaration and
// the accepted record of the Proposal it names as one ledger commit, or
// refuses and writes nothing. It then presents the listed Slices and the
// parent through the publication path acceptance uses.
func Amend(ctx context.Context, store *Store, repository github.RepositoryID, declaration *ProposalDeclaration, forge Forge) (*Amendment, error) {
	identity := repository.Owner + "/" + repository.Name
	outcome := &Amendment{Acceptance: Acceptance{
		Project: repository.Name, Repository: identity, Proposal: declaration.Proposal,
		ParentTitle: strings.TrimSpace(declaration.ParentTitle),
	}}
	var revision string
	err := store.withMutation(func() error {
		project, err := store.resolveProject(repository)
		if err != nil {
			return err
		}
		if err := store.requireCleanTree(); err != nil {
			return err
		}
		if exists(filepath.Join(store.Root, projectsRoot, project.Name, archiveRoot, declaration.Proposal)) {
			return archivedRefusal(declaration.Proposal)
		}
		meta, recorded, err := store.readProposalMeta(project.Name, declaration.Proposal)
		if err != nil {
			return err
		}
		if project.Created || !recorded {
			return refuse(
				"no accepted Proposal "+declaration.Proposal+" is recorded for "+identity,
				"accept the intake with `skl ledger accept` instead",
			)
		}
		states, err := store.recordedStates(project.Name, declaration.Proposal)
		if err != nil {
			return err
		}
		if finished(states) {
			return refuse(
				"Proposal "+declaration.Proposal+" is finished: every Slice is Merged or Superseded",
				"propose and accept the new work as a new Proposal",
			)
		}
		differences, err := store.declarationDifferences(project.Name, declaration, meta)
		if err != nil {
			return err
		}
		plan, err := store.classifyAmendment(project.Name, declaration, meta, states, differences)
		if err != nil {
			return err
		}
		for name := range declaration.IssueBodies {
			slice := declaration.sliceByName(name)
			if slice == nil {
				return refuse(
					"issue prose names "+name+", which is not a slice of proposal "+declaration.Proposal,
					"supply --issue only for the declared slices: "+strings.Join(declaration.SliceNames(), ", "),
				)
			}
			if slice.Superseded {
				return refuse(
					"issue prose names "+name+", which is declared superseded, and an amendment leaves a Superseded Slice's issue as it is",
					"leave out --issue for slice "+name,
				)
			}
		}
		outcome.Status = "existing"
		if len(differences) > 0 {
			if err := store.writeAmendment(project.Name, declaration, meta, states, plan); err != nil {
				return err
			}
			outcome.Status = "amended"
		}
		outcome.DescriptionReplaced, outcome.ParentTitleChanged, outcome.Amended = plan.description, plan.parentTitle, plan.slices
		outcome.ParentIssue = meta.ParentIssue
		listed := map[string]bool{}
		for _, change := range plan.slices {
			listed[change.Name] = change.presented()
		}
		for name := range declaration.IssueBodies {
			listed[name] = true
		}
		// A created parent groups every established child issue except a
		// Superseded Slice's, whose issue the amendment leaves as it is.
		mayCreateParent := outcome.ParentTitle != "" && meta.ParentIssue == nil && declaration.ParentBody != nil
		var names []string
		for _, name := range declaration.SliceNames() {
			if declaration.sliceByName(name).Superseded {
				continue
			}
			if listed[name] || mayCreateParent {
				names = append(names, name)
				if _, supplied := declaration.IssueBodies[name]; !supplied {
					outcome.Unpresented = append(outcome.Unpresented, name)
				}
			}
		}
		sort.Strings(outcome.Unpresented)
		if err := store.loadAcceptedStates(project.Name, declaration.Proposal, names, &outcome.Acceptance); err != nil {
			return err
		}
		revision, err = store.head()
		return err
	})
	if err != nil {
		return nil, err
	}
	outcome.Commit = revision
	outcome.HeadRef = projectsRoot + "/" + repository.Name
	outcome.Push = Publish(ctx, store, &outcome.Acceptance, IssueProse{Bodies: declaration.IssueBodies, Parent: declaration.ParentBody}, forge, revision)
	return outcome, nil
}

// archivedRefusal refuses any intake that names an archived Proposal.
func archivedRefusal(proposal string) error {
	return refuse(
		"proposal "+proposal+" is already accepted and archived",
		"archived records are never re-accepted or copied back; choose a new proposal name for renewed work",
	)
}

// recordedStates reads the state of every recorded Slice of one Proposal.
func (s *Store) recordedStates(project, proposal string) (map[string]SliceState, error) {
	names, err := s.acceptedSlices(project, proposal)
	if err != nil {
		return nil, err
	}
	states := make(map[string]SliceState, len(names))
	for _, name := range names {
		state, found, err := s.readSliceState(project, proposal, name)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, refuse(
				"the record of slice "+ItemPath(proposal, name)+" has no state.json",
				"repair or remove the damaged record with human direction, then retry",
			)
		}
		states[name] = state
	}
	return states, nil
}

func finished(states map[string]SliceState) bool {
	for _, state := range states {
		if state.State != Merged && state.State != Superseded {
			return false
		}
	}
	return true
}

// amendmentPlan is the permitted set of differences, in declaration order.
type amendmentPlan struct {
	description bool
	parentTitle bool
	slices      []SliceAmendment
}

// classifyAmendment gives every difference its verdict against the Slice's
// current records, then checks branch ownership and the resulting
// dependency graph. Any refused difference refuses the whole amendment.
func (s *Store) classifyAmendment(project string, declaration *ProposalDeclaration, meta ProposalMeta, states map[string]SliceState, differences []difference) (*amendmentPlan, error) {
	plan := &amendmentPlan{}
	changes := map[string]*SliceAmendment{}
	change := func(name string) *SliceAmendment {
		if changes[name] == nil {
			changes[name] = &SliceAmendment{Name: name}
		}
		return changes[name]
	}
	for _, difference := range differences {
		item := declaration.Proposal + "/" + difference.slice
		state := states[difference.slice]
		switch difference.kind {
		case diffDescription:
			plan.description = true
		case diffParentTitle:
			plan.parentTitle = true
		case diffProposalBranch:
			return nil, refuse(
				"Proposal "+declaration.Proposal+" is accepted with Proposal Branch "+meta.Branch+", and the intake declares "+declaration.Branch,
				"restore \"branch\": \""+meta.Branch+"\" in proposal.json; an accepted Proposal keeps its Proposal Branch",
			)
		case diffUnexpected:
			return nil, refuse(
				"the record of Proposal "+declaration.Proposal+" holds the unexpected entry "+difference.entry,
				"repair the record with human direction, then retry",
			)
		case diffMissing:
			return nil, refuse(
				"recorded slice "+item+" is missing from the declaration",
				"restore its entry in proposal.json and its directory of Contract files; an amendment keeps every recorded Slice",
			)
		case diffSuperseded:
			switch {
			case state.State == Superseded:
				return nil, refuse(
					"slice "+item+" is recorded Superseded, and the declaration drops its \"superseded\" flag",
					"restore \"superseded\": true on slice "+difference.slice+"; a Superseded Slice keeps its name, so declare its replacement as a new Slice under a new name",
				)
			case state.Claim != nil:
				return nil, refuse(
					"slice "+item+" holds an active "+state.Claim.Phase+" Claim on ledger basis "+state.Claim.Basis+", so it is not superseded",
					"let that Claim end with its handoff or release, then amend again",
				)
			case state.State == Merged:
				return nil, refuse(
					"slice "+item+" is merged, so it is not superseded",
					"remove \"superseded\" from slice "+difference.slice,
				)
			}
			change(difference.slice).Superseded = true
		case diffAdded:
			if declaration.sliceByName(difference.slice).Superseded {
				return nil, refuse(
					"added slice "+item+" is declared superseded, and only a recorded Slice is superseded",
					"remove \"superseded\" from slice "+difference.slice+", or leave the slice out of the declaration",
				)
			}
			change(difference.slice).Added = true
		case diffTitle, diffBranch, diffContract:
			if evidence := s.claimEvidence(project, declaration.Proposal, difference.slice, state); evidence != "" {
				return nil, refuse(
					"slice "+item+" was claimed before ("+evidence+"), so its Contract, title and planned branch stay as recorded",
					"restore its recorded "+map[string]string{diffTitle: "title", diffBranch: "planned branch", diffContract: difference.entry}[difference.kind]+" in the intake; carry new obligations in an added Slice",
				)
			}
			switch difference.kind {
			case diffTitle:
				change(difference.slice).Title = true
			case diffBranch:
				change(difference.slice).Branch = true
			default:
				change(difference.slice).Contract = append(change(difference.slice).Contract, difference.entry)
			}
		case diffDependencies:
			switch {
			case state.Claim != nil:
				return nil, refuse(
					"slice "+item+" holds an active "+state.Claim.Phase+" Claim on ledger basis "+state.Claim.Basis+", so its Dependencies stay as recorded",
					"let that Claim end with its handoff or release, then amend again",
				)
			case state.State == Merged || state.State == Superseded:
				return nil, refuse(
					"slice "+item+" is "+state.State+", so its Dependencies stay as recorded",
					"restore its recorded dependencies in the intake",
				)
			}
			change(difference.slice).Dependencies = true
		}
	}
	for _, slice := range declaration.Slices {
		change := changes[slice.Name]
		if change == nil {
			continue
		}
		if change.Superseded && change.presented() {
			return nil, refuse(
				"slice "+declaration.Proposal+"/"+slice.Name+" is superseded by this amendment, so its Contract, title, planned branch and Dependencies stay as recorded",
				"restore its recorded entries in the intake, or remove \"superseded\" from slice "+slice.Name,
			)
		}
		plan.slices = append(plan.slices, *change)
	}
	if err := s.requireAmendedBranches(project, declaration, meta, changes); err != nil {
		return nil, err
	}
	if err := s.checkAmendedGraph(project, declaration, states, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// claimEvidence names the current record that shows a Slice was claimed
// before, or returns "" for a never-claimed Slice. Only current records
// count; ledger history is never consulted.
func (s *Store) claimEvidence(project, proposal, slice string, state SliceState) string {
	switch {
	case state.Claim != nil:
		return "it holds an active " + state.Claim.Phase + " Claim"
	case state.State != ReadyForImplementation:
		return "it is " + state.State
	case state.Submission != nil:
		return "it has a Submission"
	case state.Decision:
		return "it has a Human Decision"
	}
	for _, phase := range []string{ImplementPhase, WatchdogPhase} {
		if exists(filepath.Join(s.Root, projectsRoot, project, "proposals", proposal, slice, phase+"-report.md")) {
			return "it has a " + phase + " report"
		}
	}
	return ""
}

// requireAmendedBranches keeps every planned branch unique within the
// Project for added Slices and changed planned branches. The declaration
// holds every recorded Slice of the Proposal with its resulting branch, so
// their active committed branches are left out of the Project check: a branch this
// amendment moves away from is free for another of its Slices.
func (s *Store) requireAmendedBranches(project string, declaration *ProposalDeclaration, meta ProposalMeta, changes map[string]*SliceAmendment) error {
	ownSlice := func(other string) bool { return strings.HasPrefix(other, declaration.Proposal+"/") }
	owners := map[string]string{}
	if meta.Branch != "" {
		owners[meta.Branch] = "Proposal " + declaration.Proposal
	}
	for _, slice := range declaration.Slices {
		if owner := owners[slice.Branch]; owner != "" {
			return refuse("branch "+slice.Branch+" belongs to both "+owner+" and Slice "+declaration.Proposal+"/"+slice.Name, "give each Proposal and Slice in the Project its own branch")
		}
		owners[slice.Branch] = "Slice " + declaration.Proposal + "/" + slice.Name
		if change := changes[slice.Name]; change != nil && (change.Added || change.Branch) {
			if err := s.requireBranchFree(project, declaration.Proposal+"/"+slice.Name, slice.Branch, ownSlice); err != nil {
				return err
			}
		}
	}
	return nil
}

// graphNode is one Slice of the Project's dependency graph, keyed by its
// <proposal>/<slice> identity.
type graphNode struct {
	state        string
	archived     bool
	dependencies []string
}

// checkAmendedGraph refuses an added or changed Dependency that names no
// recorded Slice, any active Slice of the amended Proposal that depends on a
// Superseded Slice, changed edge or not, and any cycle in the resulting
// graph, including recorded edges of the Project's other Proposals.
func (s *Store) checkAmendedGraph(project string, declaration *ProposalDeclaration, states map[string]SliceState, plan *amendmentPlan) error {
	graph, err := s.amendedGraph(project, declaration, states)
	if err != nil {
		return err
	}
	for _, change := range plan.slices {
		if !change.Added && !change.Dependencies {
			continue
		}
		slice := declaration.sliceByName(change.Name)
		for _, reference := range declaration.CanonicalDependencies(*slice) {
			if slices.Contains(states[change.Name].Dependencies, reference) {
				continue
			}
			edge := "dependency " + reference + " of slice " + declaration.Proposal + "/" + change.Name
			if _, recorded := graph[strings.TrimPrefix(reference, "proposals/")]; !recorded {
				return refuse(edge+" names no recorded Slice of project "+project, "name a recorded Slice as proposals/<proposal>/<slice>, or declare it in this Proposal")
			}
		}
	}
	if stuck := stuckDependents(graph, declaration); len(stuck) > 0 {
		return refuse(
			"active slices of proposal "+declaration.Proposal+" depend on Superseded Slices: "+strings.Join(stuck, ", "),
			"in the same intake, make each depend on the Slice that carries the work instead, or mark it \"superseded\": true",
		)
	}
	if cycle := graphCycle(graph, declaration); cycle != nil {
		return refuse("the amended dependencies form the cycle "+strings.Join(cycle, " -> "), "remove one edge of the cycle from the intake")
	}
	for index := range plan.slices {
		found := dependents(graph, declaration.Proposal, plan.slices[index].Name, plan.slices)
		if plan.slices[index].Superseded {
			plan.slices[index].Blocked = found
		} else {
			plan.slices[index].Dependents = found
		}
	}
	return nil
}

// stuckDependents names every active Slice of the amended Proposal behind a
// Superseded Slice of the Project, as "<dependent> behind <blocker>".
func stuckDependents(graph map[string]graphNode, declaration *ProposalDeclaration) []string {
	var stuck []string
	for _, slice := range declaration.Slices {
		item := declaration.Proposal + "/" + slice.Name
		node := graph[item]
		if node.state == Merged || node.state == Superseded {
			continue
		}
		for _, dependency := range node.dependencies {
			if graph[dependency].state == Superseded {
				stuck = append(stuck, item+" behind "+dependency)
			}
		}
	}
	return stuck
}

// amendedGraph reads every recorded Slice of the Project, active and
// archived, and overlays the amended Proposal's declared Slices with their
// resulting state.
func (s *Store) amendedGraph(project string, declaration *ProposalDeclaration, states map[string]SliceState) (map[string]graphNode, error) {
	graph := map[string]graphNode{}
	for _, root := range []string{"proposals", archiveRoot} {
		proposals, err := os.ReadDir(filepath.Join(s.Root, projectsRoot, project, root))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, proposal := range proposals {
			if !proposal.IsDir() {
				continue
			}
			entries, err := os.ReadDir(filepath.Join(s.Root, projectsRoot, project, root, proposal.Name()))
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				path := filepath.Join(s.Root, projectsRoot, project, root, proposal.Name(), entry.Name(), "state.json")
				if !entry.IsDir() || !exists(path) {
					continue
				}
				contents, err := os.ReadFile(path)
				if err != nil {
					return nil, err
				}
				var state SliceState
				if err := json.Unmarshal(contents, &state); err != nil {
					return nil, refuse("state.json of "+root+"/"+proposal.Name()+"/"+entry.Name()+" is unreadable", "repair or remove the damaged record with human direction, then retry")
				}
				graph[proposal.Name()+"/"+entry.Name()] = graphNode{state: state.State, archived: root == archiveRoot, dependencies: trimReferences(state.Dependencies)}
			}
		}
	}
	for _, slice := range declaration.Slices {
		state := ReadyForImplementation
		if recorded, found := states[slice.Name]; found {
			state = recorded.State
		}
		if slice.Superseded {
			state = Superseded
		}
		graph[declaration.Proposal+"/"+slice.Name] = graphNode{state: state, dependencies: trimReferences(declaration.CanonicalDependencies(slice))}
	}
	return graph, nil
}

func trimReferences(references []string) []string {
	trimmed := make([]string, len(references))
	for index, reference := range references {
		trimmed[index] = strings.TrimPrefix(reference, "proposals/")
	}
	return trimmed
}

// graphCycle returns one cycle reachable from the amended Proposal's Slices
// as its Work Item path, first node repeated last, or nil.
func graphCycle(graph map[string]graphNode, declaration *ProposalDeclaration) []string {
	const (
		visiting = 1
		done     = 2
	)
	marks := map[string]int{}
	var path []string
	var visit func(string) []string
	visit = func(node string) []string {
		switch marks[node] {
		case done:
			return nil
		case visiting:
			start := slices.Index(path, node)
			return append(slices.Clone(path[start:]), node)
		}
		marks[node] = visiting
		path = append(path, node)
		for _, next := range graph[node].dependencies {
			if _, known := graph[next]; !known {
				continue
			}
			if cycle := visit(next); cycle != nil {
				return cycle
			}
		}
		path = path[:len(path)-1]
		marks[node] = done
		return nil
	}
	for _, slice := range declaration.Slices {
		if cycle := visit(declaration.Proposal + "/" + slice.Name); cycle != nil {
			return cycle
		}
	}
	return nil
}

// dependents lists the active Slices outside the amendment that depend on one
// amended Slice.
func dependents(graph map[string]graphNode, proposal, slice string, amended []SliceAmendment) []string {
	target := proposal + "/" + slice
	var found []string
	for item, node := range graph {
		owner, name, _ := strings.Cut(item, "/")
		touched := owner == proposal && slices.ContainsFunc(amended, func(change SliceAmendment) bool { return change.Name == name })
		if node.archived || touched || node.state == Merged || node.state == Superseded || !slices.Contains(node.dependencies, target) {
			continue
		}
		found = append(found, item)
	}
	sort.Strings(found)
	return found
}

// writeAmendment writes the permitted changes with acceptance's record
// writers and commits them as one ledger revision staged by explicit paths.
func (s *Store) writeAmendment(project string, declaration *ProposalDeclaration, meta ProposalMeta, states map[string]SliceState, plan *amendmentPlan) error {
	proposalPath := filepath.Join(projectsRoot, project, "proposals", declaration.Proposal)
	var paths, summary []string
	if plan.description {
		if err := s.writeProposalDescription(project, declaration.Proposal, declaration.Description); err != nil {
			return err
		}
		paths = append(paths, filepath.Join(proposalPath, "proposal.md"))
		summary = append(summary, "Replace proposal.md")
	}
	if plan.parentTitle {
		meta.ParentTitle = strings.TrimSpace(declaration.ParentTitle)
		if err := s.writeProposalMeta(project, declaration.Proposal, meta); err != nil {
			return err
		}
		paths = append(paths, filepath.Join(proposalPath, "proposal.json"))
		summary = append(summary, "Set parent title "+meta.ParentTitle)
	}
	for _, change := range plan.slices {
		slice := *declaration.sliceByName(change.Name)
		slicePath := filepath.Join(proposalPath, slice.Name)
		if change.Added || len(change.Contract) > 0 {
			for name := range contractFiles {
				if _, declared := slice.contract[name]; declared || !exists(filepath.Join(s.Root, slicePath, name)) {
					continue
				}
				if err := os.Remove(filepath.Join(s.Root, slicePath, name)); err != nil {
					return err
				}
				paths = append(paths, filepath.Join(slicePath, name))
			}
			if err := s.writeContract(project, declaration.Proposal, slice); err != nil {
				return err
			}
			for _, name := range contractFileNames(slice.contract) {
				paths = append(paths, filepath.Join(slicePath, name))
			}
		}
		state, found := states[slice.Name]
		if !found {
			state = SliceState{State: ReadyForImplementation}
		}
		state.Title, state.Branch, state.Dependencies = slice.Title, slice.Branch, declaration.CanonicalDependencies(slice)
		if change.Superseded {
			state.State = Superseded
		}
		if err := s.writeSliceState(project, declaration.Proposal, slice.Name, state); err != nil {
			return err
		}
		paths = append(paths, filepath.Join(slicePath, "state.json"))
		summary = append(summary, change.summary(slice)...)
	}
	return s.commit("amend "+project+"/"+declaration.Proposal+"\n\n"+strings.Join(summary, "\n"), paths...)
}

// summary lines one Slice's changes for the amendment commit message.
func (c SliceAmendment) summary(slice SliceDeclaration) []string {
	if c.Added {
		return []string{"Add Slice " + c.Name}
	}
	var lines []string
	if len(c.Contract) > 0 {
		lines = append(lines, "Replace "+strings.Join(c.Contract, ", ")+" of "+c.Name)
	}
	if c.Title {
		lines = append(lines, "Retitle "+c.Name+": "+slice.Title)
	}
	if c.Branch {
		lines = append(lines, "Plan branch "+slice.Branch+" for "+c.Name)
	}
	if c.Dependencies {
		lines = append(lines, "Change Dependencies of "+c.Name)
	}
	if c.Superseded {
		lines = append(lines, "Supersede "+c.Name)
	}
	return lines
}
