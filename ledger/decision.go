package ledger

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The Human Decision surface: the read-only ledger-wide Decision Inbox, the
// explicitly scoped decision that commits decision.md together with its route
// state, and human-directed retirement of a fully terminal proposal. Every
// operation reuses the ledger's existing mutation lock, exact references, and
// replication safeguards; it adds no second storage or transaction mechanism.

// DecisionSchema is the only decision.md frontmatter schema this codec reads
// or writes.
const DecisionSchema = 1

// Continuation routes a Human Decision may select. A route is not a command
// spelling; it is the semantic continuation the Workflow records.
const (
	RouteImplement = "implement"
	RouteWatchdog  = "watchdog"
	RouteSupersede = "supersede"
)

// DecisionResult statuses. Per-item outcomes let independent multi-item
// direction report an applied, already-applied, or refused member without
// claiming the whole group succeeded.
const (
	DecisionApplied        = "applied"
	DecisionAlreadyApplied = "already_applied"
	DecisionRefused        = "refused"
	DecisionUnresolved     = "unresolved"
)

// Proposal retirement statuses.
const (
	RetirementRetired        = "retired"
	RetirementAlreadyRetired = "already_retired"
)

// DecisionRecord is the structured metadata of one decision.md. The opaque
// Markdown body holds the human's exact answer; the metadata records which
// request that answer addressed and the chosen continuation, never the
// agent-authored question.
type DecisionRecord struct {
	Schema          int       `yaml:"schema" json:"schema"`
	Project         string    `yaml:"project" json:"project"`
	Item            string    `yaml:"item" json:"item"`
	AnsweredRequest Reference `yaml:"answered_request" json:"answered_request"`
	Route           string    `yaml:"route" json:"route"`
}

// DecisionInput is one explicitly scoped human answer. It names the Work Item,
// the exact current blocking request the human answered, the opaque answer,
// and the permitted route. The CLI transports it; it never infers
// authorization from prose, discussion, or forge comments.
type DecisionInput struct {
	Project string    `json:"project"`
	Item    string    `json:"item"`
	Request Reference `json:"request"`
	Answer  string    `json:"answer"`
	Route   string    `json:"route"`
}

// DecisionResult is one applied, already-applied, or refused item outcome.
type DecisionResult struct {
	Project        string           `json:"project"`
	Item           string           `json:"item"`
	Status         string           `json:"status"`
	Request        Reference        `json:"request"`
	Route          string           `json:"route"`
	State          string           `json:"state,omitempty"`
	Decision       Reference        `json:"decision,omitempty"`
	AlreadyApplied bool             `json:"already_applied,omitempty"`
	Refusal        string           `json:"refusal,omitempty"`
	Replication    *PublicationNote `json:"replication,omitempty"`
}

// InboxFilter narrows the ledger-wide Decision Inbox. An empty Project lists
// every Project; the scope never follows the working directory.
type InboxFilter struct {
	Project string `json:"project,omitempty"`
}

// Inbox is the read-only set of current Needs Human requests. An empty
// Requests list is an empty inbox, distinct from an unknown Project or an
// unreadable ledger, which are refusals.
type Inbox struct {
	Requests []InboxEntry `json:"requests"`
}

// InboxEntry is one current Needs Human request with its exact Work Item and
// blocking-report references, the complete accepted documents, and the
// reports and source revisions needed for human triage.
type InboxEntry struct {
	Project      string             `json:"project"`
	Repository   string             `json:"repository"`
	Proposal     string             `json:"proposal"`
	Item         string             `json:"item"`
	Title        string             `json:"title"`
	Branch       string             `json:"branch"`
	Phase        string             `json:"phase"`
	Request      Reference          `json:"request"`
	Report       Report             `json:"report"`
	Contract     []ContractDocument `json:"contract"`
	Implement    *Report            `json:"implement,omitempty"`
	Watchdog     *Report            `json:"watchdog,omitempty"`
	Dependencies []DependencyState  `json:"dependencies,omitempty"`
}

// RetirementResult reports one explicit proposal retirement. Merged and
// Superseded name the preserved child outcomes; PartialDelivery is true when
// at least one slice was abandoned rather than delivered.
type RetirementResult struct {
	Project         string           `json:"project"`
	Proposal        string           `json:"proposal"`
	Status          string           `json:"status"`
	Merged          []string         `json:"merged"`
	Superseded      []string         `json:"superseded"`
	Active          []string         `json:"active,omitempty"`
	Claimed         []string         `json:"claimed,omitempty"`
	PartialDelivery bool             `json:"partial_delivery"`
	Replication     *PublicationNote `json:"replication,omitempty"`
}

// FormatDecision renders one decision.md: structured answered-request and
// route metadata followed by the human's opaque answer. It refuses an invalid
// record before producing bytes, so a refused decision is never persisted.
func FormatDecision(record DecisionRecord, answer string) ([]byte, error) {
	if err := validateDecision(record); err != nil {
		return nil, err
	}
	var frontmatter bytes.Buffer
	encoder := yaml.NewEncoder(&frontmatter)
	encoder.SetIndent(2)
	if err := encoder.Encode(record); err != nil {
		return nil, fmt.Errorf("encode decision frontmatter: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("encode decision frontmatter: %w", err)
	}
	var document bytes.Buffer
	document.WriteString(reportDelimiter + "\n")
	document.Write(frontmatter.Bytes())
	document.WriteString(reportDelimiter + "\n")
	document.WriteString(answer)
	return document.Bytes(), nil
}

// ParseDecision reads one decision.md. Unknown schemas and fields are refused
// explicitly; the answer body is returned byte for byte as opaque data.
func ParseDecision(data []byte) (DecisionRecord, string, error) {
	frontmatter, body, err := splitFrontmatter("decision", data)
	if err != nil {
		return DecisionRecord{}, "", err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(frontmatter))
	decoder.KnownFields(true)
	var record DecisionRecord
	if err := decoder.Decode(&record); err != nil {
		if errors.Is(err, io.EOF) {
			return DecisionRecord{}, "", refuse(
				"decision holds no YAML frontmatter metadata",
				"persist the documented schema-1 fields between the --- delimiters",
			)
		}
		return DecisionRecord{}, "", refuse(
			"decision frontmatter is malformed: "+err.Error(),
			"write the documented schema-1 fields with their exact types",
		)
	}
	var extra any
	switch err := decoder.Decode(&extra); {
	case errors.Is(err, io.EOF):
	case err == nil:
		return DecisionRecord{}, "", refuse(
			"decision frontmatter holds more than one YAML document",
			"keep exactly one YAML document between the --- delimiters",
		)
	default:
		return DecisionRecord{}, "", refuse(
			"decision frontmatter is malformed: "+err.Error(),
			"keep exactly one YAML document between the --- delimiters",
		)
	}
	if err := checkScalarTags("decision", frontmatter, map[string]bool{"schema": true}); err != nil {
		return DecisionRecord{}, "", err
	}
	if err := validateDecision(record); err != nil {
		return DecisionRecord{}, "", err
	}
	return record, string(body), nil
}

func validateDecision(record DecisionRecord) error {
	if record.Schema != DecisionSchema {
		return refuse(
			"decision schema "+strconv.Itoa(record.Schema)+" is not the supported schema 1",
			"write schema as the bare integer 1; a different version requires a CLI that understands it",
		)
	}
	if !ValidRecordName(record.Project) {
		return refuse(
			"decision records invalid project "+strconv.Quote(record.Project),
			"record the Project whose Work Item this answer resolves",
		)
	}
	proposal, slice, found := strings.Cut(record.Item, "/")
	if !found || !ValidRecordName(proposal) || !ValidRecordName(slice) {
		return refuse(
			"decision records invalid Work Item "+strconv.Quote(record.Item),
			"record the exact proposal/slice identity whose request was answered",
		)
	}
	switch record.Route {
	case RouteImplement, RouteWatchdog, RouteSupersede:
	default:
		return refuse(
			"decision route "+strconv.Quote(record.Route)+" is not implement, watchdog, or supersede",
			"select one of the permitted continuations",
		)
	}
	return validateReference("answered request", record.AnsweredRequest)
}

// ReadDecisionInbox returns the current Needs Human requests across the
// configured ledger, optionally narrowed to one Project. It reads only
// committed records, needs no source checkout or forge access, and changes no
// Claim or Workflow State.
func ReadDecisionInbox(s *Store, filter InboxFilter) (*Inbox, error) {
	head, err := s.head()
	if err != nil {
		return nil, err
	}
	projects, err := s.inboxProjects(head, filter.Project)
	if err != nil {
		return nil, err
	}
	inbox := &Inbox{Requests: []InboxEntry{}}
	for _, project := range projects {
		var identity ProjectIdentity
		if err := readJSONAt(s, head, filepath.Join(projectsRoot, project, "project.json"), &identity); err != nil || identity.Repository == "" {
			return nil, refuse(
				"project "+project+" has no readable project.json at ledger head "+head,
				"repair the ledger Project with human direction; the inbox reports access problems rather than an empty scope",
			)
		}
		paths, err := git(s.Root, "ls-tree", "-r", "--name-only", head, "--", filepath.Join(projectsRoot, project, "proposals"))
		if err != nil {
			return nil, refuse(
				"project "+project+" records are unreadable at ledger head "+head+": "+err.Error(),
				"repair the ledger clone or the configured path, then retry",
			)
		}
		for _, path := range strings.Split(paths, "\n") {
			if !strings.HasSuffix(path, "/state.json") {
				continue
			}
			relative := strings.TrimPrefix(path, projectsRoot+"/"+project+"/proposals/")
			item := strings.TrimSuffix(relative, "/state.json")
			proposal, slice, found := strings.Cut(item, "/")
			if !found || !ValidRecordName(proposal) || !ValidRecordName(slice) {
				return nil, refuse(
					"project "+project+" holds invalid Work Item path "+path,
					"repair the damaged record with human direction",
				)
			}
			var state SliceState
			if err := readJSONAt(s, head, path, &state); err != nil {
				return nil, refuse(
					"Work Item record "+path+" is unreadable at ledger head "+head+": "+err.Error(),
					"repair the damaged record with human direction",
				)
			}
			if state.State != NeedsHuman {
				continue
			}
			entry, err := s.inboxEntry(head, project, identity.Repository, proposal, slice, state)
			if err != nil {
				return nil, err
			}
			inbox.Requests = append(inbox.Requests, *entry)
		}
	}
	sort.Slice(inbox.Requests, func(i, j int) bool {
		if inbox.Requests[i].Project != inbox.Requests[j].Project {
			return inbox.Requests[i].Project < inbox.Requests[j].Project
		}
		return inbox.Requests[i].Item < inbox.Requests[j].Item
	})
	return inbox, nil
}

// inboxProjects resolves the Project scope. An unknown explicit filter is a
// refusal, never a silently widened or emptied scope.
func (s *Store) inboxProjects(head, filter string) ([]string, error) {
	if filter != "" {
		if !ValidRecordName(filter) {
			return nil, refuse(
				"unknown Project "+strconv.Quote(filter),
				"select a Project recorded in the configured ledger, or read the inbox without a filter",
			)
		}
		if _, err := showPath(s, head, filepath.Join(projectsRoot, filter, "project.json")); err != nil {
			return nil, refuse(
				"unknown Project "+filter+" in the configured ledger",
				"select a Project recorded in the configured ledger, or read the inbox without a filter",
			)
		}
		return []string{filter}, nil
	}
	if !gitOK(s.Root, "cat-file", "-e", head+":"+projectsRoot) {
		return nil, nil
	}
	output, err := git(s.Root, "ls-tree", "--name-only", head+":"+projectsRoot)
	if err != nil || strings.TrimSpace(output) == "" {
		return nil, nil
	}
	projects := strings.Fields(output)
	sort.Strings(projects)
	return projects, nil
}

// inboxEntry assembles one current request with its accepted documents and
// the reports and source revisions the human needs for triage.
func (s *Store) inboxEntry(head, project, repository, proposal, slice string, state SliceState) (*InboxEntry, error) {
	directory, err := itemDirectory(project, proposal+"/"+slice)
	if err != nil {
		return nil, err
	}
	phase, request, report, err := s.currentRequest(head, directory, state)
	if err != nil {
		return nil, err
	}
	entry := &InboxEntry{
		Project: project, Repository: repository, Proposal: proposal,
		Item: proposal + "/" + slice, Title: state.Title, Branch: state.Branch,
		Phase: phase, Request: request, Report: report,
	}
	names, err := acceptedFileNamesAt(s, head, directory)
	if err != nil {
		return nil, refuse(
			"accepted documents of "+proposal+"/"+slice+" are unreadable at ledger head "+head+": "+err.Error(),
			"repair or restore the complete accepted record with human direction",
		)
	}
	for _, required := range []string{"behavior.md", "intent.md"} {
		if !containsString(names, required) {
			return nil, refuse(
				"Needs Human record of "+proposal+"/"+slice+" is incomplete: "+required+" is missing",
				"repair or restore the complete accepted record with human direction",
			)
		}
	}
	for _, name := range names {
		path := directory + "/" + name
		contents, err := showPath(s, head, path)
		if err != nil {
			return nil, refuse(
				"accepted document "+path+" is unavailable at ledger head "+head,
				"repair or restore the complete accepted record with human direction",
			)
		}
		entry.Contract = append(entry.Contract, ContractDocument{Path: path, Commit: head, Contents: contents})
	}
	if entry.Implement, err = s.readReportAt(head, directory, ImplementPhase); err != nil {
		return nil, err
	}
	if entry.Watchdog, err = s.readReportAt(head, directory, WatchdogPhase); err != nil {
		return nil, err
	}
	for _, dependency := range state.Dependencies {
		dependencyState := DependencyState{Item: dependency}
		if blocker, err := s.readStateByReferenceAt(head, project, dependency); err == nil {
			dependencyState.State = blocker
		}
		entry.Dependencies = append(entry.Dependencies, dependencyState)
	}
	return entry, nil
}

// readReportAt reads one phase report at the current head, or nil when that
// phase has produced no report yet.
func (s *Store) readReportAt(head, directory, phase string) (*Report, error) {
	path := directory + "/" + phase + "-report.md"
	if !gitOK(s.Root, "cat-file", "-e", head+":"+path) {
		return nil, nil
	}
	raw, err := showPath(s, head, path)
	if err != nil {
		return nil, err
	}
	report, _, err := ParseReport(phase, []byte(raw))
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// currentRequest resolves the phase report that raised the current Needs Human
// pause. When both reports qualify, the later recorded Claim identifies the
// current one, so a stale pause cannot be answered again.
func (s *Store) currentRequest(head, directory string, state SliceState) (string, Reference, Report, error) {
	type candidate struct {
		phase  string
		path   string
		report Report
		claim  string
	}
	var candidates []candidate
	for _, phase := range []string{WatchdogPhase, ImplementPhase} {
		path := directory + "/" + phase + "-report.md"
		if !gitOK(s.Root, "cat-file", "-e", head+":"+path) {
			continue
		}
		raw, err := showPath(s, head, path)
		if err != nil {
			return "", Reference{}, Report{}, err
		}
		report, _, err := ParseReport(phase, []byte(raw))
		if err != nil {
			return "", Reference{}, Report{}, err
		}
		if phase == WatchdogPhase {
			if report.Outcome != outcomeNeedsHuman && !(report.Outcome == outcomeRework && report.Round >= 2) {
				continue
			}
		} else if report.Outcome != outcomeNeedsHuman {
			continue
		}
		candidates = append(candidates, candidate{phase: phase, path: path, report: report, claim: report.Ledger.Claim.Commit})
	}
	switch len(candidates) {
	case 0:
		return "", Reference{}, Report{}, refuse(
			"Needs Human record of "+directory+" holds no current blocking request",
			"restore the phase report that raised the pause with human direction; a question lives in its Phase Report",
		)
	case 1:
		found := candidates[0]
		return found.phase, Reference{Commit: head, Path: found.path}, found.report, nil
	}
	best := 0
	for index := 1; index < len(candidates); index++ {
		switch {
		case gitOK(s.Root, "merge-base", "--is-ancestor", candidates[best].claim, candidates[index].claim):
			best = index
		case gitOK(s.Root, "merge-base", "--is-ancestor", candidates[index].claim, candidates[best].claim):
		default:
			return "", Reference{}, Report{}, refuse(
				"Needs Human record of "+directory+" holds conflicting blocking requests",
				"repair the conflicting reports with human direction",
			)
		}
	}
	found := candidates[best]
	return found.phase, Reference{Commit: head, Path: found.path}, found.report, nil
}

// ApplyDecision applies one explicitly scoped Human Decision and returns its
// per-item outcome. A refusal is reported in the result; only an unusable
// ledger or store is returned as an error.
func ApplyDecision(s *Store, input DecisionInput) (*DecisionResult, error) {
	result, err := applyIndependentDecision(s, input)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ApplyDecisions applies a set of explicitly scoped decisions. When coupled
// is false each request is validated, committed, and reported independently.
// When coupled is true every member must apply together; otherwise the whole
// set is refused without mutation rather than silently enacting part of
// direction whose meaning depends on an unresolved member.
func ApplyDecisions(s *Store, inputs []DecisionInput, coupled bool) ([]DecisionResult, error) {
	if len(inputs) == 0 {
		return nil, refuse(
			"no human decision was supplied",
			"select at least one current request, its exact reference, and a permitted route",
		)
	}
	if coupled {
		return applyCoupledDecisions(s, inputs)
	}
	results := make([]DecisionResult, 0, len(inputs))
	for _, input := range inputs {
		result, err := applyIndependentDecision(s, input)
		if err != nil {
			// An independent member's failure cannot erase already committed
			// outcomes or imply that the entire group changed nothing.
			result = DecisionResult{Project: input.Project, Item: input.Item,
				Status: DecisionUnresolved, Request: input.Request, Route: input.Route,
				Refusal: err.Error()}
		}
		results = append(results, result)
	}
	return results, nil
}

// decisionWrite is one validated, not-yet-committed decision.md and route
// state. already marks an exact retry whose committed result is recognized.
type decisionWrite struct {
	input        DecisionInput
	proposal     string
	slice        string
	directory    string
	decisionPath string
	statePath    string
	head         string
	contents     []byte
	state        SliceState
	already      bool
}

func applyIndependentDecision(s *Store, input DecisionInput) (DecisionResult, error) {
	var plan *decisionWrite
	var committed string
	err := s.withMutation(func() error {
		var err error
		plan, err = s.planDecision(input)
		if err != nil {
			return err
		}
		if plan.already {
			return nil
		}
		if err := s.requireReconciled(); err != nil {
			return err
		}
		committed, err = s.commitDecisions(plan)
		return err
	})
	if err != nil {
		var refusal *Refusal
		if errors.As(err, &refusal) {
			return DecisionResult{
				Project: input.Project, Item: input.Item, Status: DecisionRefused,
				Request: input.Request, Route: input.Route, Refusal: err.Error(),
			}, nil
		}
		return DecisionResult{}, err
	}
	result := plan.result(committed)
	if result.Status == DecisionApplied {
		replicateDecision(s, &result)
	}
	return result, nil
}

func applyCoupledDecisions(s *Store, inputs []DecisionInput) ([]DecisionResult, error) {
	seen := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		key := input.Project + "/" + input.Item
		if seen[key] {
			return nil, refuse("duplicate Work Item in coupled direction: "+key,
				"clarify one answer and route per selected Work Item before submitting the group")
		}
		seen[key] = true
	}
	var plans []*decisionWrite
	var committed string
	err := s.withMutation(func() error {
		pending := false
		for _, input := range inputs {
			plan, err := s.planDecision(input)
			if err != nil {
				return err
			}
			plans = append(plans, plan)
			pending = pending || !plan.already
		}
		if pending {
			if err := s.requireReconciled(); err != nil {
				return err
			}
		}
		var err error
		committed, err = s.commitDecisions(plans...)
		return err
	})
	if err != nil {
		return nil, err
	}
	results := make([]DecisionResult, 0, len(plans))
	for _, plan := range plans {
		results = append(results, plan.result(committed))
	}
	if committed != "" {
		note, _ := s.push(committed)
		for index := range results {
			copied := note
			results[index].Replication = &copied
		}
	}
	return results, nil
}

// planDecision validates one scoped decision under the mutation lock and
// returns the exact write it authorizes, or the recognized committed result of
// an exact retry. Replacement requests, active Claims, and differing replays
// become concrete refusals before any byte is written.
func (s *Store) planDecision(input DecisionInput) (*decisionWrite, error) {
	if strings.TrimSpace(input.Answer) == "" {
		return nil, refuse(
			"human decision carries no answer",
			"record the human's exact direction; a route alone is not an answer",
		)
	}
	if !ValidRecordName(input.Project) {
		return nil, refuse(
			"unknown Project "+strconv.Quote(input.Project),
			"select a Project recorded in the configured ledger",
		)
	}
	proposal, slice, found := strings.Cut(input.Item, "/")
	if !found || !ValidRecordName(proposal) || !ValidRecordName(slice) {
		return nil, refuse(
			"Work Item identity "+strconv.Quote(input.Item)+" is not a proposal/slice identity",
			"select the exact Work Item from the Decision Inbox",
		)
	}
	switch input.Route {
	case RouteImplement, RouteWatchdog, RouteSupersede:
	default:
		return nil, refuse(
			"decision route "+strconv.Quote(input.Route)+" is not implement, watchdog, or supersede",
			"select one of the permitted continuations",
		)
	}
	if err := validateReference("answered request", input.Request); err != nil {
		return nil, err
	}
	head, err := s.head()
	if err != nil {
		return nil, err
	}
	var identity ProjectIdentity
	if err := readJSONAt(s, head, filepath.Join(projectsRoot, input.Project, "project.json"), &identity); err != nil || identity.Repository == "" {
		return nil, refuse(
			"unknown Project "+input.Project+" in the configured ledger",
			"select a Project recorded in the configured ledger",
		)
	}
	directory, err := itemDirectory(input.Project, input.Item)
	if err != nil {
		return nil, err
	}
	state, present, err := s.readSliceState(input.Project, proposal, slice)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, refuse(
			"Work Item "+input.Item+" has no recorded state in project "+input.Project,
			"select a current request from the Decision Inbox",
		)
	}
	if err := s.requireCleanPaths(directory); err != nil {
		return nil, err
	}
	plan := &decisionWrite{
		input: input, proposal: proposal, slice: slice, directory: directory,
		decisionPath: directory + "/decision.md", statePath: directory + "/state.json", head: head,
	}
	// Consuming a decision clears its active marker, not its replay evidence.
	// The current retained document is sufficient; do not search history or
	// reroute later work to reconstruct an old effect.
	if state.Decision || gitOK(s.Root, "cat-file", "-e", head+":"+plan.decisionPath) {
		raw, err := showPath(s, head, plan.decisionPath)
		if err != nil {
			return nil, refuse(
				"active Human Decision document of "+input.Item+" is unreadable at ledger head "+head,
				"repair decision.md or clear the active marker with human direction",
			)
		}
		record, answer, err := ParseDecision([]byte(raw))
		if err != nil {
			return nil, err
		}
		if record.Project == input.Project && record.Item == input.Item &&
			record.AnsweredRequest == input.Request && record.Route == input.Route && answer == input.Answer {
			plan.state = state
			plan.already = true
			return plan, nil
		}
		if state.Decision || record.AnsweredRequest == input.Request {
			return nil, refuse(
				"a Human Decision is already recorded for "+input.Item,
				"inspect the recorded decision; a different answer requires renewed human direction rather than replacing the recorded result",
			)
		}
	}
	if state.State != NeedsHuman {
		return nil, refuse(
			"Work Item "+input.Item+" is not paused on a current request",
			"read the Decision Inbox and resolve a current Needs Human request",
		)
	}
	if state.Claim != nil {
		return nil, refuse(
			"Work Item "+input.Item+" has an active Claim",
			"resolve or release the reservation before recording human direction",
		)
	}
	if err := s.requireCurrentRequest(head, directory, state, input.Request); err != nil {
		return nil, err
	}
	target, err := s.decisionTarget(head, directory, input.Route)
	if err != nil {
		return nil, err
	}
	contents, err := FormatDecision(DecisionRecord{
		Schema: DecisionSchema, Project: input.Project, Item: input.Item,
		AnsweredRequest: input.Request, Route: input.Route,
	}, input.Answer)
	if err != nil {
		return nil, err
	}
	state.Decision = true
	state.State = target
	state.Claim = nil
	plan.contents = contents
	plan.state = state
	return plan, nil
}

// requireCurrentRequest verifies the supplied request is still the current
// blocking request. An unrelated ledger commit keeps the request current; a
// replacement request with identical prose does not, because its recorded
// Claim identity differs. The request's selected inputs must still hold their
// exact bytes.
func (s *Store) requireCurrentRequest(head, directory string, state SliceState, supplied Reference) error {
	phase, current, currentReport, err := s.currentRequest(head, directory, state)
	if err != nil {
		return err
	}
	if supplied.Path != current.Path {
		return refuse(
			"the selected request is no longer the current blocking request",
			"renew human direction for the current request; the answered request changed",
		)
	}
	document, err := ShowReference(s, supplied.Commit, supplied.Path)
	if err != nil {
		return err
	}
	currentContents, err := showPath(s, head, current.Path)
	if err != nil {
		return err
	}
	if document.Contents != currentContents {
		return refuse(
			"the selected request was replaced since it was presented",
			"renew human direction for the current request; identical prose with a different recorded claim is a different request",
		)
	}
	suppliedReport, _, err := ParseReport(phase, []byte(document.Contents))
	if err != nil {
		return err
	}
	if suppliedReport.Ledger.Claim != currentReport.Ledger.Claim {
		return refuse(
			"the selected request belongs to a different recorded claim",
			"renew human direction for the current request",
		)
	}
	return s.requireRequestInputsCurrent(head, current.Path, suppliedReport)
}

// requireRequestInputsCurrent compares the request's consumed contract and
// report references against the current bytes at the same paths. It never
// compares the whole ledger tip, so unrelated commits keep the request valid.
// A report's own phase path is skipped because the request report supersedes
// its predecessor at that path; that predecessor is not an invalidating input.
func (s *Store) requireRequestInputsCurrent(head, requestPath string, report Report) error {
	refs := append([]Reference(nil), report.Ledger.Contract...)
	for _, optional := range []*Reference{report.Ledger.Implement, report.Ledger.Watchdog, report.Ledger.Decision} {
		if optional != nil {
			refs = append(refs, *optional)
		}
	}
	for _, ref := range refs {
		if ref.Path == requestPath {
			continue
		}
		before, err := showPath(s, ref.Commit, ref.Path)
		if err != nil {
			return refuse(
				"selected input "+ref.Path+" is unavailable at its recorded revision",
				"restore the fixed input or renew human direction for the changed request",
			)
		}
		current, err := showPath(s, head, ref.Path)
		if err != nil {
			return refuse(
				"selected input "+ref.Path+" is missing at the current ledger head",
				"restore the fixed input or renew human direction for the changed request",
			)
		}
		if before != current {
			return refuse(
				"selected input "+ref.Path+" changed since the request was recorded",
				"resolve the competing change or renew human direction for the changed request",
			)
		}
	}
	return nil
}

// decisionTarget selects the continuation state for one route. Initial
// implementation resumes Ready for Implementation; finding-driven
// implementation resumes Rework; review continuation resumes Awaiting Review
// with its required implementation inputs; supersession is terminal.
func (s *Store) decisionTarget(head, directory, route string) (string, error) {
	switch route {
	case RouteImplement:
		if gitOK(s.Root, "cat-file", "-e", head+":"+directory+"/"+WatchdogPhase+"-report.md") {
			return Rework, nil
		}
		return ReadyForImplementation, nil
	case RouteWatchdog:
		if err := s.requireImplementationInputs(head, directory); err != nil {
			return "", err
		}
		return AwaitingReview, nil
	case RouteSupersede:
		return Superseded, nil
	}
	return "", refuse("unsupported decision route "+strconv.Quote(route), "select implement, watchdog, or supersede")
}

// requireImplementationInputs verifies the continued review will have its
// fixed implementation report, even when the code revision is unchanged.
func (s *Store) requireImplementationInputs(head, directory string) error {
	path := directory + "/" + ImplementPhase + "-report.md"
	raw, err := showPath(s, head, path)
	if err != nil {
		return refuse(
			"continued review has no fixed implementation report",
			"route to Implement to establish the reviewed source revisions",
		)
	}
	report, _, err := ParseReport(ImplementPhase, []byte(raw))
	if err != nil {
		return err
	}
	if report.Source.Head == "" || report.Source.Target == "" {
		return refuse(
			"continued review requires an implementation report with a fixed source head and target",
			"route to Implement to establish the reviewed source revisions",
		)
	}
	return nil
}

// commitDecisions writes every pending decision.md and route state and records
// them as one commit. Any failure before the commit restores the worktree so
// neither half of an item's result becomes authoritative.
func (s *Store) commitDecisions(plans ...*decisionWrite) (string, error) {
	var paths []string
	for _, plan := range plans {
		if plan.already {
			continue
		}
		if err := os.WriteFile(filepath.Join(s.Root, filepath.FromSlash(plan.decisionPath)), plan.contents, 0600); err != nil {
			s.discardDecisionWrites(plans)
			return "", fmt.Errorf("write decision document %s: %w", plan.decisionPath, err)
		}
		if err := writeJSON(filepath.Join(s.Root, filepath.FromSlash(plan.statePath)), plan.state); err != nil {
			s.discardDecisionWrites(plans)
			return "", fmt.Errorf("write route state %s: %w", plan.statePath, err)
		}
		paths = append(paths, plan.decisionPath, plan.statePath)
	}
	if len(paths) == 0 {
		return "", nil
	}
	message := "record human decision"
	if len(plans) == 1 {
		message += " " + plans[0].input.Project + "/" + plans[0].input.Item
	}
	if err := s.commit(message, paths...); err != nil {
		s.discardDecisionWrites(plans)
		return "", err
	}
	return s.head()
}

// discardDecisionWrites returns the touched paths to their committed bytes
// after a pre-commit failure. Only paths this operation cleaned beforehand are
// restored, so unrelated edits are never discarded.
func (s *Store) discardDecisionWrites(plans []*decisionWrite) {
	for _, plan := range plans {
		if plan.already {
			continue
		}
		_ = exec.Command("git", "-C", s.Root, "reset", "-q", "--", plan.decisionPath, plan.statePath).Run()
	}
	for _, plan := range plans {
		if plan.already {
			continue
		}
		s.restorePath(plan.decisionPath)
		s.restorePath(plan.statePath)
	}
}

func (s *Store) restorePath(path string) {
	if gitOK(s.Root, "cat-file", "-e", "HEAD:"+path) {
		_ = exec.Command("git", "-C", s.Root, "checkout", "--", path).Run()
		return
	}
	_ = os.Remove(filepath.Join(s.Root, filepath.FromSlash(path)))
}

func (plan *decisionWrite) result(committed string) DecisionResult {
	if plan.already {
		return DecisionResult{
			Project: plan.input.Project, Item: plan.input.Item, Status: DecisionAlreadyApplied,
			Request: plan.input.Request, Route: plan.input.Route, State: plan.state.State,
			Decision: Reference{Commit: plan.head, Path: plan.decisionPath}, AlreadyApplied: true,
		}
	}
	return DecisionResult{
		Project: plan.input.Project, Item: plan.input.Item, Status: DecisionApplied,
		Request: plan.input.Request, Route: plan.input.Route, State: plan.state.State,
		Decision: Reference{Commit: committed, Path: plan.decisionPath},
	}
}

// replicateDecision attempts the configured ledger push outside the mutation
// lock. An unavailable remote leaves the local result applied and the
// replication pending; no forge is required.
func replicateDecision(s *Store, result *DecisionResult) {
	note, _ := s.push(result.Decision.Commit)
	result.Replication = &note
}

// RetireProposal records explicit human-directed retirement of one proposal
// once every child is Merged or Superseded and unclaimed. It preserves merged
// history, frozen contracts, dependencies, and source work, and reports
// partial delivery rather than all-delivered completion.
func RetireProposal(s *Store, project, proposal string) (*RetirementResult, error) {
	if !ValidRecordName(project) || !ValidRecordName(proposal) {
		return nil, refuse(
			"invalid proposal retirement scope "+strconv.Quote(project+"/"+proposal),
			"select a recorded Project and proposal identity",
		)
	}
	var result *RetirementResult
	var replicate string
	err := s.withMutation(func() error {
		if err := s.requireReconciled(); err != nil {
			return err
		}
		head, err := s.head()
		if err != nil {
			return err
		}
		var identity ProjectIdentity
		if err := readJSONAt(s, head, filepath.Join(projectsRoot, project, "project.json"), &identity); err != nil || identity.Repository == "" {
			return refuse(
				"unknown Project "+project+" in the configured ledger",
				"select a Project recorded in the configured ledger",
			)
		}
		meta, present, err := s.readProposalMeta(project, proposal)
		if err != nil {
			return err
		}
		if !present {
			return refuse(
				"unknown Proposal "+proposal+" in project "+project,
				"select a recorded proposal identity",
			)
		}
		proposalDirectory := filepath.Join(projectsRoot, project, "proposals", proposal)
		children, err := s.proposalChildren(head, proposalDirectory)
		if err != nil {
			return err
		}
		if len(children) == 0 {
			return refuse(
				"proposal "+proposal+" records no slices",
				"repair the proposal record with human direction",
			)
		}
		result = &RetirementResult{Project: project, Proposal: proposal, Status: RetirementRetired}
		for _, slice := range children {
			var state SliceState
			if err := readJSONAt(s, head, proposalDirectory+"/"+slice+"/state.json", &state); err != nil {
				return refuse(
					"slice "+slice+" of proposal "+proposal+" is unreadable at ledger head "+head,
					"repair the damaged record with human direction",
				)
			}
			switch state.State {
			case Merged:
				result.Merged = append(result.Merged, slice)
			case Superseded:
				result.Superseded = append(result.Superseded, slice)
			default:
				result.Active = append(result.Active, slice)
			}
			if state.Claim != nil {
				result.Claimed = append(result.Claimed, slice)
			}
		}
		sort.Strings(result.Merged)
		sort.Strings(result.Superseded)
		sort.Strings(result.Active)
		sort.Strings(result.Claimed)
		if len(result.Active) > 0 || len(result.Claimed) > 0 {
			remaining := append(append([]string(nil), result.Active...), result.Claimed...)
			sort.Strings(remaining)
			return refuse(
				"proposal "+proposal+" still has active or claimed work: "+strings.Join(remaining, ", "),
				"retire only a proposal whose every slice is Merged or Superseded and unclaimed",
			)
		}
		if len(result.Superseded) == 0 {
			return refuse("proposal "+proposal+" has no Superseded slices",
				"this operation retires abandoned work, not all-delivered proposals; leave completion observation to its own operation")
		}
		result.PartialDelivery = true
		if meta.Retired {
			result.Status = RetirementAlreadyRetired
			return nil
		}
		proposalPath := proposalDirectory + "/proposal.json"
		if err := s.requireCleanPaths(proposalPath); err != nil {
			return err
		}
		meta.Retired = true
		if err := s.writeProposalMeta(project, proposal, meta); err != nil {
			return err
		}
		if err := s.commit("retire proposal "+project+"/"+proposal, proposalPath); err != nil {
			return err
		}
		replicate, err = s.head()
		return err
	})
	if err != nil {
		return nil, err
	}
	if replicate != "" {
		note, _ := s.push(replicate)
		result.Replication = &note
	}
	return result, nil
}

// proposalChildren lists the committed slice directories of one proposal.
func (s *Store) proposalChildren(head, proposalDirectory string) ([]string, error) {
	output, err := git(s.Root, "ls-tree", "-r", "--name-only", head, "--", proposalDirectory)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var children []string
	for _, path := range strings.Split(output, "\n") {
		if !strings.HasSuffix(path, "/state.json") {
			continue
		}
		relative := strings.TrimPrefix(path, proposalDirectory+"/")
		slice := strings.TrimSuffix(relative, "/state.json")
		if slice == "" || strings.Contains(slice, "/") || seen[slice] {
			continue
		}
		seen[slice] = true
		children = append(children, slice)
	}
	sort.Strings(children)
	return children, nil
}
