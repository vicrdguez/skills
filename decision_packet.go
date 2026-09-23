package skills

import "github.com/vicrdguez/skills/ledger"

// DecisionStatus selects which specialized Decision Inbox or decision-result
// instructions one invocation renders. It is an instruction selector, never a
// Workflow State.
type DecisionStatus string

const (
	// DecisionInbox lists the current ledger-wide Needs Human requests.
	DecisionInbox DecisionStatus = "inbox"
	// DecisionEmpty is a readable configured ledger with no current requests.
	// It is deliberately distinct from an unavailable ledger.
	DecisionEmpty DecisionStatus = "empty"
	// DecisionUnavailable reports a configuration or access problem instead of
	// an empty inbox.
	DecisionUnavailable DecisionStatus = "unavailable"
	// DecisionApplied records direction where every selected item was applied
	// or already applied and no retirement was refused.
	DecisionApplied DecisionStatus = "applied"
	// DecisionPartial records a mixed result: at least one applied item together
	// with a refused or unresolved item, or a refused retirement.
	DecisionPartial DecisionStatus = "partial"
	// DecisionRefused records direction that changed nothing.
	DecisionRefused DecisionStatus = "refused"
	// DecisionUnresolved reports an unconfirmed local result requiring repair.
	DecisionUnresolved DecisionStatus = "unresolved"
)

// Continuation routes a recorded Human Decision may select. The route is part
// of the answer, never inferred from the prose.
const (
	DecisionRouteImplement = "implement"
	DecisionRouteWatchdog  = "watchdog"
	DecisionRouteSupersede = "supersede"
)

// Per-item result statuses. They are the engine's exact outcomes, not worker
// prose.
const (
	DecisionOutcomeApplied        = "applied"
	DecisionOutcomeAlreadyApplied = "already_applied"
	DecisionOutcomeRefused        = "refused"
	DecisionOutcomeUnresolved     = "unresolved"
)

// Explicit parent-retirement outcomes.
const (
	DecisionRetirementRecorded = "retired"
	DecisionRetirementRefused  = "refused"
)

// DecisionFacts specializes one Decision Inbox read or one recorded decision
// result. The CLI populates it from the ledger; the templates only present the
// typed facts. Status/Project/Reason/Repair describe the invocation, Requests
// carry current Needs Human context, Outcomes carry per-item results, and
// Retirement carries an explicit parent disposition.
type DecisionFacts struct {
	Status     DecisionStatus      `json:"status"`
	Project    string              `json:"project,omitempty"`
	Reason     string              `json:"reason,omitempty"`
	Repair     string              `json:"repair,omitempty"`
	Requests   []DecisionRequest   `json:"requests,omitempty"`
	Outcomes   []DecisionOutcome   `json:"outcomes,omitempty"`
	Retirement *DecisionRetirement `json:"retirement,omitempty"`
}

// DecisionRequest is one current Needs Human Work Item with the exact request
// and Contract references a human needs for triage. ApplyCommand is the fully
// bound operation for this request: only the route and the human answer file
// remain for the conversation to choose.
type DecisionRequest struct {
	Project       string                    `json:"project"`
	Repository    string                    `json:"repository"`
	Proposal      string                    `json:"proposal"`
	Item          string                    `json:"item"`
	RequestCommit string                    `json:"request_commit"`
	RequestPath   string                    `json:"request_path"`
	Documents     []ledger.ContractDocument `json:"documents"`
	Source        *DecisionSource           `json:"source,omitempty"`
	ApplyCommand  string                    `json:"apply_command"`
}

// DecisionSource carries the source-repository facts that help a human judge
// the request. It is optional context, never ledger navigation.
type DecisionSource struct {
	Branch      string `json:"branch,omitempty"`
	Submission  int    `json:"submission,omitempty"`
	SourceHead  string `json:"source_head,omitempty"`
	Target      string `json:"target,omitempty"`
	ReviewCount uint64 `json:"review_count,omitempty"`
}

// DecisionOutcome is one item's exact result. Applied and already-applied
// outcomes name the committed route and decision reference; refused and
// unresolved outcomes explain what changed nothing.
type DecisionOutcome struct {
	Project       string `json:"project"`
	Item          string `json:"item"`
	Status        string `json:"status"`
	Route         string `json:"route,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Repair        string `json:"repair,omitempty"`
	Reference     string `json:"reference,omitempty"`
	RequestCommit string `json:"request_commit,omitempty"`
	RequestPath   string `json:"request_path,omitempty"`
}

// DecisionRetirement is the explicit disposition of an abandoned Proposal
// parent. Retiring reports partial delivery, not all-delivered completion.
type DecisionRetirement struct {
	Project    string   `json:"project"`
	Proposal   string   `json:"proposal"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason,omitempty"`
	Repair     string   `json:"repair,omitempty"`
	Merged     int      `json:"merged"`
	Superseded int      `json:"superseded"`
	Active     []string `json:"active,omitempty"`
}

// DecisionApplyCommand renders the public decision-application operation with
// every known argument bound. The continuing route and the human answer file
// are the only values left to the conversation.
func DecisionApplyCommand(project, item, requestCommit, requestPath string) string {
	return "skl decision apply" +
		" --project " + ShellQuote(project) +
		" --item " + ShellQuote(item) +
		" --request-commit " + ShellQuote(requestCommit) +
		" --request-path " + ShellQuote(requestPath) +
		" --route <implement|watchdog|supersede>" +
		" --answer <human-answer-file>"
}
