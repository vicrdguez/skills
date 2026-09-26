package ledger

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Diagnostic scopes name the record a browsing diagnostic belongs to.
const (
	ScopeLedger   = "ledger"
	ScopeProject  = "project"
	ScopeProposal = "proposal"
	ScopeSlice    = "slice"
)

// Diagnostic discloses one unreadable or unsupported record at the scope it
// affects. Subject is the entity identity: project, project/proposal, or
// project/proposal/slice.
type Diagnostic struct {
	Scope   string `json:"scope"`
	Subject string `json:"subject"`
	Problem string `json:"problem"`
}

// Tally counts the slices of a scope. Lifecycles holds only readable,
// supported lifecycles; Unknown counts slices whose lifecycle is unreadable or
// unsupported, so their Claims are unknown too.
type Tally struct {
	Slices     int            `json:"slices"`
	Lifecycles map[string]int `json:"lifecycles"`
	Claimed    int            `json:"claimed"`
	Unknown    int            `json:"unknown"`
}

// ProjectSummary is one Project in an overview or inventory. Proposal counts
// and the tally cover active Proposals, plus archived ones when they were
// explicitly included. Diagnostics disclose every unreadable record in that
// scope, and Incomplete is set whenever any exists.
type ProjectSummary struct {
	Name              string `json:"name"`
	Repository        string `json:"repository,omitempty"`
	Proposals         int    `json:"proposals"`
	ArchivedProposals int    `json:"archived_proposals"`
	Tally
	Incomplete  bool         `json:"incomplete"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// ProposalSummary is one Proposal's metadata and progress. Archived and
// Retired record ledger organization and explicit retirement; neither asserts
// delivery. FullyDelivered is set only when every readable member is Merged
// and none is unknown.
type ProposalSummary struct {
	Name           string           `json:"name"`
	Archived       bool             `json:"archived"`
	Retired        bool             `json:"retired"`
	ParentTitle    string           `json:"parent_title,omitempty"`
	Accepted       string           `json:"accepted,omitempty"`
	ParentIssue    *ForgeAttachment `json:"parent_issue,omitempty"`
	FullyDelivered bool             `json:"fully_delivered"`
	Tally
	Incomplete  bool         `json:"incomplete"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// SliceSummary is one Slice as a Proposal member. Readable is false when its
// state record cannot be interpreted: its lifecycle and Claim are then
// unknown, never absent.
type SliceSummary struct {
	Slice       string       `json:"slice"`
	Item        string       `json:"item"`
	Readable    bool         `json:"readable"`
	Title       string       `json:"title,omitempty"`
	Lifecycle   string       `json:"lifecycle,omitempty"`
	ClaimPhase  string       `json:"claim_phase,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// ClaimFacts is a recorded Claim: a reservation for one phase, not evidence
// that a worker is running.
type ClaimFacts struct {
	Phase string `json:"phase"`
	Basis string `json:"basis"`
}

// DependencyFact is one recorded Dependency with its blocker's lifecycle at
// the same revision, or the problem that keeps that lifecycle unknown.
type DependencyFact struct {
	Item      string `json:"item"`
	Lifecycle string `json:"lifecycle,omitempty"`
	Problem   string `json:"problem,omitempty"`
}

// Overview is the ledger-wide Project overview at one committed revision.
type Overview struct {
	Revision        string           `json:"revision"`
	IncludeArchived bool             `json:"include_archived"`
	Projects        []ProjectSummary `json:"projects"`
	Diagnostics     []Diagnostic     `json:"diagnostics,omitempty"`
}

// ProjectInventory is one Project's Proposals at one committed revision.
type ProjectInventory struct {
	Revision        string            `json:"revision"`
	IncludeArchived bool              `json:"include_archived"`
	Project         ProjectSummary    `json:"project"`
	Proposals       []ProposalSummary `json:"proposals"`
}

// ProposalDetail is one Proposal with its Slice membership.
type ProposalDetail struct {
	Revision   string          `json:"revision"`
	Project    string          `json:"project"`
	Repository string          `json:"repository,omitempty"`
	Proposal   ProposalSummary `json:"proposal"`
	Slices     []SliceSummary  `json:"slices"`
}

// SliceDetail is every recorded fact of one Slice at one committed revision.
// Readable is false when its state record cannot be interpreted; the facts it
// would carry are then unknown rather than absent, unclaimed, or complete.
type SliceDetail struct {
	Revision        string             `json:"revision"`
	Project         string             `json:"project"`
	Repository      string             `json:"repository,omitempty"`
	Proposal        string             `json:"proposal"`
	Slice           string             `json:"slice"`
	Item            string             `json:"item"`
	Archived        bool               `json:"archived"`
	ProposalRetired bool               `json:"proposal_retired"`
	Readable        bool               `json:"readable"`
	Title           string             `json:"title,omitempty"`
	Lifecycle       string             `json:"lifecycle,omitempty"`
	Claim           *ClaimFacts        `json:"claim,omitempty"`
	Branch          string             `json:"branch,omitempty"`
	Dependencies    []DependencyFact   `json:"dependencies"`
	Issue           *ForgeAttachment   `json:"issue,omitempty"`
	ParentIssue     *ForgeAttachment   `json:"parent_issue,omitempty"`
	Submission      *ForgeAttachment   `json:"submission,omitempty"`
	Target          *IntegrationTarget `json:"integration_target,omitempty"`
	Completion      *TerminalEvidence  `json:"completion,omitempty"`
	ActiveDecision  bool               `json:"active_decision"`
	Pending         *PublicationState  `json:"pending_publication,omitempty"`
	Documents       []string           `json:"documents"`
	Reports         []string           `json:"reports"`
	Diagnostics     []Diagnostic       `json:"diagnostics,omitempty"`
}

// Snapshot pins one committed ledger revision for a browsing view. Every
// query it answers reads that revision's objects, so inventories, membership,
// and facts describe one revision even while the ledger advances, and
// uncommitted edits never substitute for committed records. Queries never
// fetch, contact a forge, reconcile completion, or write the ledger.
type Snapshot struct {
	store    *Store
	Revision string
	projects map[string]*projectTree
	invalid  []Diagnostic
}

type projectTree struct {
	name        string
	proposals   []*proposalTree
	diagnostics []Diagnostic
}

type proposalTree struct {
	name     string
	archived bool
	slices   []*sliceTree
	// invalid diagnoses member directories whose names cannot be Slice
	// identities; each still counts as a member of unknown lifecycle.
	invalid []Diagnostic
}

type sliceTree struct {
	name  string
	files map[string]bool
}

// Snapshot selects the ledger's current committed revision and its record
// tree. Failure to read the ledger itself is an error; damaged records are
// diagnosed by the queries instead.
func (s *Store) Snapshot() (*Snapshot, error) {
	head, err := s.head()
	if err != nil {
		return nil, refuse(
			"the configured ledger "+s.Root+" has no readable committed revision",
			"repair the ledger clone or the configured path, then retry",
		)
	}
	arguments := []string{"ls-tree", "-r", "-z", "--name-only", head, "--", projectsRoot}
	listing, err := git(s.Root, arguments...)
	if err != nil {
		return nil, refuse(
			"the records of ledger revision "+head+" are unreadable: "+gitError(s.Root, arguments, err).Error(),
			"repair the ledger clone or the configured path, then retry",
		)
	}
	snapshot := &Snapshot{store: s, Revision: head, projects: map[string]*projectTree{}}
	for _, path := range strings.Split(listing, "\x00") {
		if path != "" {
			snapshot.index(strings.Split(path, "/"))
		}
	}
	for _, project := range snapshot.projects {
		sort.Slice(project.proposals, func(i, j int) bool {
			if project.proposals[i].archived != project.proposals[j].archived {
				return !project.proposals[i].archived
			}
			return project.proposals[i].name < project.proposals[j].name
		})
		for _, proposal := range project.proposals {
			sort.Slice(proposal.slices, func(i, j int) bool { return proposal.slices[i].name < proposal.slices[j].name })
		}
	}
	return snapshot, nil
}

// index places one committed path, split into components, in the record
// tree. Records whose names cannot be ledger identities are diagnosed at
// their parent scope instead of being browsed.
func (v *Snapshot) index(parts []string) {
	if len(parts) < 3 {
		return
	}
	if !ValidRecordName(parts[1]) {
		v.invalid = appendDiagnostic(v.invalid, Diagnostic{ScopeLedger, projectsRoot + "/" + parts[1], "is not a valid Project record name"})
		return
	}
	project := v.projects[parts[1]]
	if project == nil {
		project = &projectTree{name: parts[1]}
		v.projects[parts[1]] = project
	}
	if len(parts) < 5 || (parts[2] != "proposals" && parts[2] != archiveRoot) {
		return
	}
	archived := parts[2] == archiveRoot
	if !ValidRecordName(parts[3]) {
		project.diagnostics = appendDiagnostic(project.diagnostics, Diagnostic{ScopeProject, project.name, parts[2] + "/" + parts[3] + " is not a valid Proposal record name"})
		return
	}
	var proposal *proposalTree
	for _, candidate := range project.proposals {
		if candidate.name == parts[3] && candidate.archived == archived {
			proposal = candidate
		}
	}
	if proposal == nil {
		proposal = &proposalTree{name: parts[3], archived: archived}
		project.proposals = append(project.proposals, proposal)
	}
	if len(parts) < 6 {
		return
	}
	subject := project.name + "/" + proposal.name
	if !ValidRecordName(parts[4]) {
		proposal.invalid = appendDiagnostic(proposal.invalid, Diagnostic{ScopeProposal, subject, parts[4] + " is not a valid Slice record name"})
		return
	}
	slice := proposal.slice(parts[4])
	if slice == nil {
		slice = &sliceTree{name: parts[4], files: map[string]bool{}}
		proposal.slices = append(proposal.slices, slice)
	}
	if len(parts) == 6 {
		slice.files[parts[5]] = true
	}
}

func (p *proposalTree) slice(name string) *sliceTree {
	for _, slice := range p.slices {
		if slice.name == name {
			return slice
		}
	}
	return nil
}

func appendDiagnostic(diagnostics []Diagnostic, diagnostic Diagnostic) []Diagnostic {
	for _, existing := range diagnostics {
		if existing == diagnostic {
			return diagnostics
		}
	}
	return append(diagnostics, diagnostic)
}

// ProjectNames lists the Projects recorded at this revision.
func (v *Snapshot) ProjectNames() []string {
	names := make([]string, 0, len(v.projects))
	for name := range v.projects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Overview summarizes every Project. Archived Proposals are counted but their
// members are summarized only when includeArchived is set.
func (v *Snapshot) Overview(includeArchived bool) (*Overview, error) {
	overview := &Overview{Revision: v.Revision, IncludeArchived: includeArchived, Projects: []ProjectSummary{}, Diagnostics: v.invalid}
	var projects []*projectTree
	for _, name := range v.ProjectNames() {
		projects = append(projects, v.projects[name])
	}
	reads, err := v.read(projects, includeArchived)
	if err != nil {
		return nil, err
	}
	for _, read := range reads {
		overview.Projects = append(overview.Projects, read.summary())
	}
	return overview, nil
}

// Project returns one Project's Proposal inventory.
func (v *Snapshot) Project(name string, includeArchived bool) (*ProjectInventory, error) {
	project, err := v.project(name)
	if err != nil {
		return nil, err
	}
	reads, err := v.read([]*projectTree{project}, includeArchived)
	if err != nil {
		return nil, err
	}
	inventory := &ProjectInventory{Revision: v.Revision, IncludeArchived: includeArchived, Project: reads[0].summary(), Proposals: []ProposalSummary{}}
	for _, proposal := range reads[0].proposals {
		inventory.Proposals = append(inventory.Proposals, proposal.summary())
	}
	return inventory, nil
}

// Proposal returns one Proposal, active or archived, with its membership. An
// active record is preferred when both locations hold the same identity.
func (v *Snapshot) Proposal(projectName, name string) (*ProposalDetail, error) {
	project, proposal, err := v.proposal(projectName, name)
	if err != nil {
		return nil, err
	}
	read, err := v.readOne(project, proposal)
	if err != nil {
		return nil, err
	}
	detail := &ProposalDetail{
		Revision: v.Revision, Project: project.name, Repository: read.repository,
		Proposal: read.proposals[0].summary(), Slices: []SliceSummary{},
	}
	for _, slice := range read.proposals[0].slices {
		detail.Slices = append(detail.Slices, slice.summary(proposal.name))
	}
	return detail, nil
}

// Slice returns every recorded fact of one Slice identified as
// proposal/slice, with its Dependencies resolved at the same revision.
func (v *Snapshot) Slice(projectName, item string) (*SliceDetail, error) {
	proposalName, sliceName, found := strings.Cut(item, "/")
	if !found || !ValidRecordName(proposalName) || !ValidRecordName(sliceName) {
		return nil, refuse(
			"Slice reference "+strconv.Quote(item)+" is not a proposal/slice identity",
			"select a Slice listed by its Proposal, such as add-order-cancellation/foundation",
		)
	}
	project, proposal, err := v.proposal(projectName, proposalName)
	if err != nil {
		return nil, err
	}
	tree := proposal.slice(sliceName)
	if tree == nil {
		return nil, refuse(
			"no Slice "+item+" in project "+project.name+" at ledger revision "+v.Revision,
			"select a Slice listed by its Proposal",
		)
	}
	read, err := v.readOne(project, proposal)
	if err != nil {
		return nil, err
	}
	proposalRead := read.proposals[0]
	var slice sliceRead
	for _, candidate := range proposalRead.slices {
		if candidate.tree == tree {
			slice = candidate
		}
	}
	detail := &SliceDetail{
		Revision: v.Revision, Project: project.name, Repository: read.repository,
		Proposal: proposal.name, Slice: tree.name, Item: item,
		Archived: proposal.archived, ProposalRetired: proposalRead.meta.Retired,
		ParentIssue: proposalRead.meta.ParentIssue, Readable: slice.readable,
		Dependencies: []DependencyFact{}, Documents: []string{}, Reports: []string{},
		Diagnostics: append(append([]Diagnostic(nil), read.diagnostics...), proposalRead.diagnostics...),
	}
	detail.Diagnostics = append(detail.Diagnostics, slice.diagnostics...)
	for name := range tree.files {
		if contractFiles[name] {
			detail.Documents = append(detail.Documents, name)
		}
	}
	sort.Strings(detail.Documents)
	for _, phase := range []string{ImplementPhase, WatchdogPhase} {
		if tree.files[phase+"-report.md"] {
			detail.Reports = append(detail.Reports, phase)
		}
	}
	if !slice.readable {
		return detail, nil
	}
	state := slice.state
	detail.Title, detail.Lifecycle, detail.Branch = state.Title, state.State, state.Branch
	detail.Issue, detail.Submission, detail.Target, detail.Completion = state.Issue, state.Submission, state.Target, state.Completion
	detail.ActiveDecision, detail.Pending = state.Decision, state.Publication
	if state.Claim != nil {
		detail.Claim = &ClaimFacts{Phase: state.Claim.Phase, Basis: state.Claim.Basis}
	}
	for _, dependency := range state.Dependencies {
		detail.Dependencies = append(detail.Dependencies, v.dependency(project, dependency))
	}
	return detail, nil
}

// dependency resolves one recorded Dependency's blocker at this revision.
func (v *Snapshot) dependency(project *projectTree, reference string) DependencyFact {
	fact := DependencyFact{Item: strings.TrimPrefix(reference, "proposals/")}
	proposalName, sliceName, ok := workItemReference(reference)
	if !ok {
		fact.Problem = "the recorded reference is not a proposal/slice identity"
		return fact
	}
	_, proposal, err := v.proposal(project.name, proposalName)
	if err != nil || proposal.slice(sliceName) == nil {
		fact.Problem = "no committed record of the blocker exists at this revision"
		return fact
	}
	path := v.slicePath(project.name, proposal, sliceName) + "/state.json"
	blobs, err := v.blobs([]string{path})
	if err != nil {
		fact.Problem = "the blocker record is unreadable: " + err.Error()
		return fact
	}
	contents, present := blobs[path]
	read := decodeSlice(project.name+"/"+proposal.name, proposal.slice(sliceName), contents, present)
	switch {
	case !read.readable:
		fact.Problem = "the blocker's state record is unreadable"
	case !knownLifecycle(read.state.State):
		fact.Lifecycle, fact.Problem = read.state.State, "the blocker records an unsupported lifecycle"
	default:
		fact.Lifecycle = read.state.State
	}
	return fact
}

func (v *Snapshot) project(name string) (*projectTree, error) {
	project := v.projects[name]
	if project == nil {
		return nil, refuse(
			"unknown Project "+strconv.Quote(name)+" in the configured ledger at revision "+v.Revision,
			"select a Project listed by `skl browse projects`",
		)
	}
	return project, nil
}

func (v *Snapshot) proposal(projectName, name string) (*projectTree, *proposalTree, error) {
	project, err := v.project(projectName)
	if err != nil {
		return nil, nil, err
	}
	for _, archived := range []bool{false, true} {
		for _, proposal := range project.proposals {
			if proposal.name == name && proposal.archived == archived {
				return project, proposal, nil
			}
		}
	}
	return nil, nil, refuse(
		"no Proposal "+strconv.Quote(name)+" in project "+project.name+" at ledger revision "+v.Revision,
		"select a Proposal listed by its Project, including archived ones when needed",
	)
}

func (v *Snapshot) proposalPath(project string, proposal *proposalTree) string {
	location := "proposals"
	if proposal.archived {
		location = archiveRoot
	}
	return projectsRoot + "/" + project + "/" + location + "/" + proposal.name
}

func (v *Snapshot) slicePath(project string, proposal *proposalTree, slice string) string {
	return v.proposalPath(project, proposal) + "/" + slice
}

// projectRead, proposalRead, and sliceRead hold decoded records with the
// diagnostics of their own scope.
type projectRead struct {
	name        string
	repository  string
	active      int
	archived    int
	diagnostics []Diagnostic
	proposals   []proposalRead
}

type proposalRead struct {
	tree        *proposalTree
	meta        ProposalMeta
	diagnostics []Diagnostic
	slices      []sliceRead
}

type sliceRead struct {
	tree        *sliceTree
	state       SliceState
	readable    bool
	diagnostics []Diagnostic
}

// read decodes the given Projects with one batched object read, including
// archived Proposals only when requested.
func (v *Snapshot) read(projects []*projectTree, includeArchived bool) ([]projectRead, error) {
	var paths []string
	for _, project := range projects {
		paths = append(paths, projectsRoot+"/"+project.name+"/project.json")
		for _, proposal := range project.proposals {
			if proposal.archived && !includeArchived {
				continue
			}
			paths = append(paths, v.proposalPath(project.name, proposal)+"/proposal.json")
			for _, slice := range proposal.slices {
				paths = append(paths, v.slicePath(project.name, proposal, slice.name)+"/state.json")
			}
		}
	}
	blobs, err := v.blobs(paths)
	if err != nil {
		return nil, err
	}
	reads := make([]projectRead, 0, len(projects))
	for _, project := range projects {
		read := projectRead{name: project.name, diagnostics: append([]Diagnostic(nil), project.diagnostics...)}
		read.repository, read.diagnostics = decodeProject(project.name, blobs[projectsRoot+"/"+project.name+"/project.json"], read.diagnostics)
		for _, proposal := range project.proposals {
			if proposal.archived {
				read.archived++
			} else {
				read.active++
			}
			if proposal.archived && !includeArchived {
				continue
			}
			read.proposals = append(read.proposals, v.decodeProposal(project.name, proposal, blobs))
		}
		reads = append(reads, read)
	}
	return reads, nil
}

// readOne decodes one Proposal and its Project identity.
func (v *Snapshot) readOne(project *projectTree, proposal *proposalTree) (projectRead, error) {
	paths := []string{projectsRoot + "/" + project.name + "/project.json", v.proposalPath(project.name, proposal) + "/proposal.json"}
	for _, slice := range proposal.slices {
		paths = append(paths, v.slicePath(project.name, proposal, slice.name)+"/state.json")
	}
	blobs, err := v.blobs(paths)
	if err != nil {
		return projectRead{}, err
	}
	read := projectRead{name: project.name}
	read.repository, read.diagnostics = decodeProject(project.name, blobs[paths[0]], nil)
	read.proposals = []proposalRead{v.decodeProposal(project.name, proposal, blobs)}
	return read, nil
}

func decodeProject(name string, contents []byte, diagnostics []Diagnostic) (string, []Diagnostic) {
	if contents == nil {
		return "", append(diagnostics, Diagnostic{ScopeProject, name, "has no committed project.json, so its repository is unknown"})
	}
	var identity ProjectIdentity
	if err := json.Unmarshal(contents, &identity); err != nil || identity.Repository == "" {
		return "", append(diagnostics, Diagnostic{ScopeProject, name, "project.json is unreadable, so its repository is unknown"})
	}
	return identity.Repository, diagnostics
}

func (v *Snapshot) decodeProposal(project string, proposal *proposalTree, blobs map[string][]byte) proposalRead {
	subject := project + "/" + proposal.name
	read := proposalRead{tree: proposal, diagnostics: append([]Diagnostic(nil), proposal.invalid...)}
	path := v.proposalPath(project, proposal)
	switch contents, present := blobs[path+"/proposal.json"]; {
	case !present:
		read.diagnostics = append(read.diagnostics, Diagnostic{ScopeProposal, subject, "has no committed proposal.json, so its acceptance metadata is unknown"})
	case json.Unmarshal(contents, &read.meta) != nil:
		read.meta = ProposalMeta{}
		read.diagnostics = append(read.diagnostics, Diagnostic{ScopeProposal, subject, "proposal.json is unreadable, so its acceptance metadata is unknown"})
	}
	if len(proposal.slices) == 0 && len(proposal.invalid) == 0 {
		read.diagnostics = append(read.diagnostics, Diagnostic{ScopeProposal, subject, "records no Slices, so its membership is unknown"})
	}
	for _, slice := range proposal.slices {
		state, present := blobs[path+"/"+slice.name+"/state.json"]
		read.slices = append(read.slices, decodeSlice(subject, slice, state, present))
	}
	return read
}

// decodeSlice interprets one state record. A missing or malformed record
// leaves lifecycle and Claim unknown; an unsupported lifecycle or Claim phase
// keeps the readable facts and is diagnosed.
func decodeSlice(proposalSubject string, slice *sliceTree, contents []byte, present bool) sliceRead {
	subject := proposalSubject + "/" + slice.name
	read := sliceRead{tree: slice}
	switch {
	case !present:
		read.diagnostics = []Diagnostic{{ScopeSlice, subject, "has no committed state.json, so its lifecycle and Claim are unknown"}}
		return read
	case json.Unmarshal(contents, &read.state) != nil:
		read.state = SliceState{}
		read.diagnostics = []Diagnostic{{ScopeSlice, subject, "state.json is unreadable, so its lifecycle and Claim are unknown"}}
		return read
	}
	read.readable = true
	if !knownLifecycle(read.state.State) {
		read.diagnostics = append(read.diagnostics, Diagnostic{ScopeSlice, subject, "records unsupported lifecycle " + strconv.Quote(read.state.State)})
	}
	if claim := read.state.Claim; claim != nil && claim.Phase != ImplementPhase && claim.Phase != WatchdogPhase {
		read.diagnostics = append(read.diagnostics, Diagnostic{ScopeSlice, subject, "records a Claim with unsupported phase " + strconv.Quote(claim.Phase)})
	}
	return read
}

func knownLifecycle(state string) bool {
	switch state {
	case ReadyForImplementation, AwaitingReview, Rework, NeedsHuman, ReadyForMerge, Merged, Superseded:
		return true
	}
	return false
}

func (t *Tally) add(slice sliceRead) {
	t.Slices++
	if t.Lifecycles == nil {
		t.Lifecycles = map[string]int{}
	}
	if !slice.readable || !knownLifecycle(slice.state.State) {
		t.Unknown++
	} else {
		t.Lifecycles[slice.state.State]++
	}
	if slice.readable && slice.state.Claim != nil {
		t.Claimed++
	}
}

func (r proposalRead) summary() ProposalSummary {
	summary := ProposalSummary{
		Name: r.tree.name, Archived: r.tree.archived, Retired: r.meta.Retired,
		ParentTitle: r.meta.ParentTitle, Accepted: r.meta.Accepted, ParentIssue: r.meta.ParentIssue,
		Tally:       Tally{Lifecycles: map[string]int{}},
		Diagnostics: append([]Diagnostic(nil), r.diagnostics...),
	}
	for _, slice := range r.slices {
		summary.Tally.add(slice)
		summary.Diagnostics = append(summary.Diagnostics, slice.diagnostics...)
	}
	summary.Slices += len(r.tree.invalid)
	summary.Unknown += len(r.tree.invalid)
	summary.FullyDelivered = summary.Slices > 0 && summary.Unknown == 0 && summary.Lifecycles[Merged] == summary.Slices
	summary.Incomplete = len(summary.Diagnostics) > 0
	return summary
}

func (r projectRead) summary() ProjectSummary {
	summary := ProjectSummary{
		Name: r.name, Repository: r.repository, Proposals: r.active, ArchivedProposals: r.archived,
		Tally:       Tally{Lifecycles: map[string]int{}},
		Diagnostics: append([]Diagnostic(nil), r.diagnostics...),
	}
	for _, proposal := range r.proposals {
		proposalSummary := proposal.summary()
		summary.Slices += proposalSummary.Slices
		summary.Claimed += proposalSummary.Claimed
		summary.Unknown += proposalSummary.Unknown
		for lifecycle, count := range proposalSummary.Lifecycles {
			summary.Lifecycles[lifecycle] += count
		}
		summary.Diagnostics = append(summary.Diagnostics, proposalSummary.Diagnostics...)
	}
	summary.Incomplete = len(summary.Diagnostics) > 0
	return summary
}

func (r sliceRead) summary(proposal string) SliceSummary {
	summary := SliceSummary{Slice: r.tree.name, Item: proposal + "/" + r.tree.name, Readable: r.readable, Diagnostics: r.diagnostics}
	if r.readable {
		summary.Title, summary.Lifecycle = r.state.Title, r.state.State
		if r.state.Claim != nil {
			summary.ClaimPhase = r.state.Claim.Phase
		}
	}
	return summary
}

// blobs reads the given committed paths at this revision with one Git
// process. A path absent from the revision is absent from the result.
func (v *Snapshot) blobs(paths []string) (map[string][]byte, error) {
	result := make(map[string][]byte, len(paths))
	if len(paths) == 0 {
		return result, nil
	}
	var input strings.Builder
	for _, path := range paths {
		input.WriteString(v.Revision + ":" + path + "\n")
	}
	arguments := []string{"-C", v.store.Root, "cat-file", "--batch"}
	command := exec.Command("git", arguments...)
	command.Stdin = strings.NewReader(input.String())
	output, err := command.Output()
	if err != nil {
		return nil, v.unreadable(gitError(v.store.Root, arguments[2:], err).Error())
	}
	reader := bufio.NewReader(bytes.NewReader(output))
	for _, path := range paths {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, v.unreadable("truncated object output for " + path)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 {
			continue
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, v.unreadable("malformed object header " + strconv.Quote(strings.TrimSpace(header)))
		}
		contents := make([]byte, size+1)
		if _, err := io.ReadFull(reader, contents); err != nil {
			return nil, v.unreadable("truncated object " + path)
		}
		if fields[1] == "blob" {
			result[path] = contents[:size]
		}
	}
	return result, nil
}

func (v *Snapshot) unreadable(cause string) error {
	return refuse(
		"the records of ledger revision "+v.Revision+" are unreadable: "+cause,
		"repair the ledger clone or the configured path, then retry",
	)
}
