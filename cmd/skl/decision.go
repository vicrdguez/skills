package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
)

// The decision commands transport explicitly scoped human direction to the
// ledger and render the specialized Decision instructions. They resolve the
// machine-configured ledger directly: unlike delivery commands they never
// resolve a source checkout or construct a forge, so an inbox read works from
// a non-repository directory and never acquires work or changes state.

// decisionOutput is the JSON transport. It carries the specialized facts the
// packet renders, the engine's typed per-item results and retirement outcome,
// and the rendered packet for callers that want the exact instructions.
type decisionOutput struct {
	Status     string                   `json:"status"`
	Facts      *skilldist.DecisionFacts `json:"facts"`
	Results    []ledger.DecisionResult  `json:"results,omitempty"`
	Retirement *ledger.RetirementResult `json:"retirement,omitempty"`
	Packet     *skilldist.Packet        `json:"packet"`
}

func decisionCommands(stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:  "decision",
		Usage: "Read the ledger-wide Decision Inbox and record explicitly scoped human direction",
		Subcommands: []*cli.Command{{
			Name:  "inbox",
			Usage: "Read current Needs Human requests across Projects, optionally filtered",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "project", Usage: "Explicit Project filter; never taken from the working directory"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error { return runDecisionInbox(command, stdout) },
		}, {
			Name:  "apply",
			Usage: "Record one explicitly scoped human answer with its continuation route",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "project"},
				&cli.StringFlag{Name: "item", Usage: "The exact proposal/slice identity the human answered"},
				&cli.StringFlag{Name: "request-commit", Usage: "The full commit the answered blocking request is valid at"},
				&cli.StringFlag{Name: "request-path", Usage: "The ledger-relative path of the answered blocking request"},
				&cli.StringFlag{Name: "route", Usage: "Continuation route: implement, watchdog, or supersede"},
				&cli.PathFlag{Name: "answer", Usage: "Path to the file holding the human's answer verbatim"},
				&cli.PathFlag{Name: "input", Usage: "Path to a JSON array of explicitly scoped decisions"},
				&cli.BoolFlag{Name: "coupled", Usage: "With --input, refuse the whole set rather than applying independent members"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error { return runDecisionApply(command, stdout) },
		}, {
			Name:  "retire",
			Usage: "Explicitly retire a fully terminal, unclaimed proposal parent",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "project"},
				&cli.StringFlag{Name: "proposal"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error { return runDecisionRetire(command, stdout) },
		}},
	}
}

func runDecisionInbox(command *cli.Context, stdout io.Writer) error {
	format, err := implementationFormat(command.String("format"))
	if err != nil {
		return err
	}
	if command.NArg() != 0 {
		return renderDecisionRefusal(stdout, format,
			"decision inbox takes flags, not positional arguments",
			"read the ledger-wide inbox, or narrow it with --project <name>")
	}
	filter := command.String("project")
	if command.IsSet("project") && strings.TrimSpace(filter) == "" {
		return renderDecisionRefusal(stdout, format, "an explicit Project filter cannot be empty",
			"supply --project <name>, or omit the flag to read the ledger-wide inbox")
	}
	store, failure := openConfiguredLedger()
	if failure != nil {
		return renderDecisionUnavailable(stdout, format, failure)
	}
	inbox, err := ledger.ReadDecisionInbox(store, ledger.InboxFilter{Project: filter})
	if err != nil {
		return renderDecisionUnavailable(stdout, format, err)
	}
	facts := &skilldist.DecisionFacts{Status: skilldist.DecisionInbox, Project: filter}
	for _, entry := range inbox.Requests {
		request, err := decisionRequestFacts(store, entry)
		if err != nil {
			return renderDecisionUnavailable(stdout, format, err)
		}
		facts.Requests = append(facts.Requests, request)
	}
	if len(facts.Requests) == 0 {
		facts.Status = skilldist.DecisionEmpty
	}
	return renderDecision(stdout, format, decisionOutput{Status: string(facts.Status), Facts: facts})
}

// decisionRequestFacts supplies one request's exact references, accepted
// Contract documents, and the blocking Phase Report body. The inbox entry
// carries the report metadata but never its opaque body, so the exact document
// is read at its recorded reference; a missing body is reported rather than
// invented.
func decisionRequestFacts(store *ledger.Store, entry ledger.InboxEntry) (skilldist.DecisionRequest, error) {
	request := skilldist.DecisionRequest{
		Project: entry.Project, Repository: entry.Repository, Proposal: entry.Proposal,
		Item: entry.Item, RequestCommit: entry.Request.Commit, RequestPath: entry.Request.Path,
		Documents:    append([]ledger.ContractDocument(nil), entry.Contract...),
		Source:       decisionSourceFacts(entry),
		ApplyCommand: skilldist.DecisionApplyCommand(entry.Project, entry.Item, entry.Request.Commit, entry.Request.Path),
	}
	report, err := ledger.ShowReference(store, entry.Request.Commit, entry.Request.Path)
	if err != nil {
		return skilldist.DecisionRequest{}, err
	}
	request.Documents = append(request.Documents, report)
	return request, nil
}

// decisionSourceFacts carries the optional source context a human judges the
// request against. Only facts the ledger recorded are supplied; unrecorded
// context stays absent rather than guessed.
func decisionSourceFacts(entry ledger.InboxEntry) *skilldist.DecisionSource {
	source := &skilldist.DecisionSource{Branch: entry.Branch}
	switch entry.Phase {
	case ledger.WatchdogPhase:
		source.SourceHead = entry.Report.Source.Reviewed
		if source.SourceHead == "" {
			source.SourceHead = entry.Report.Source.Head
		}
		source.Target = entry.Report.Source.Target
		source.ReviewCount = entry.Report.Round
	default:
		source.SourceHead = entry.Report.Source.Head
		source.Target = entry.Report.Source.Target
		if entry.Watchdog != nil {
			source.ReviewCount = entry.Watchdog.Round
		}
	}
	if source.Branch == "" && source.SourceHead == "" && source.Target == "" && source.ReviewCount == 0 {
		return nil
	}
	return source
}

func runDecisionApply(command *cli.Context, stdout io.Writer) error {
	format, err := implementationFormat(command.String("format"))
	if err != nil {
		return err
	}
	if command.NArg() != 0 {
		return renderDecisionRefusal(stdout, format,
			"decision apply takes flags, not positional arguments",
			"submit the bound single-request command, or one explicitly scoped --input file")
	}
	inputPath := command.Path("input")
	explicit := command.IsSet("project") || command.IsSet("item") || command.IsSet("request-commit") ||
		command.IsSet("request-path") || command.IsSet("route") || command.IsSet("answer")
	coupled := command.Bool("coupled")
	if inputPath != "" {
		if explicit {
			return renderDecisionRefusal(stdout, format,
				"decision apply takes either one --input file or the single-request flags, not both",
				"submit one explicitly scoped decision or one batch input file")
		}
		return applyDecisionBatch(stdout, format, inputPath, coupled)
	}
	if coupled {
		return renderDecisionRefusal(stdout, format,
			"--coupled applies only with --input",
			"omit --coupled for one request, or supply an explicitly scoped batch with --input and --coupled")
	}
	project, item := command.String("project"), command.String("item")
	commit, path, route := command.String("request-commit"), command.String("request-path"), command.String("route")
	if project == "" || item == "" || commit == "" || path == "" || route == "" || command.Path("answer") == "" {
		return renderDecisionRefusal(stdout, format,
			"an apply requires --project, --item, --request-commit, --request-path, --route, and --answer",
			"use the exact bound command the inbox reported for the answered request")
	}
	answer, err := os.ReadFile(command.Path("answer"))
	if err != nil {
		return renderDecisionRefusal(stdout, format,
			"read the human answer file: "+err.Error(),
			"write the human's exact answer to a readable file and retry the same request")
	}
	if strings.TrimSpace(string(answer)) == "" {
		return renderDecisionRefusal(stdout, format,
			"human decision carries no answer",
			"record the human's exact direction; a route alone is not an answer")
	}
	store, failure := openConfiguredLedger()
	if failure != nil {
		return renderDecisionUnavailable(stdout, format, failure)
	}
	results, err := ledger.ApplyDecisions(store, []ledger.DecisionInput{{
		Project: project, Item: item, Request: ledger.Reference{Commit: commit, Path: path},
		Answer: string(answer), Route: route,
	}}, false)
	if err != nil {
		return renderDecisionApplyError(stdout, format, nil, err)
	}
	return renderDecisionResults(stdout, format, results)
}

func applyDecisionBatch(stdout io.Writer, format implementationFormatKind, inputPath string, coupled bool) error {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return renderDecisionRefusal(stdout, format,
			"read the batch decision input: "+err.Error(),
			"supply one JSON array of explicitly scoped decisions")
	}
	var inputs []ledger.DecisionInput
	if err := json.Unmarshal(raw, &inputs); err != nil {
		return renderDecisionRefusal(stdout, format,
			"batch decision input is not a JSON array of decisions: "+err.Error(),
			`supply [{"project":...,"item":...,"request":{"commit":...,"path":...},"answer":...,"route":...}]`)
	}
	if len(inputs) == 0 {
		return renderDecisionRefusal(stdout, format,
			"batch decision input holds no decisions",
			"name at least one current request with its exact reference, answer, and route")
	}
	store, failure := openConfiguredLedger()
	if failure != nil {
		return renderDecisionUnavailable(stdout, format, failure)
	}
	results, err := ledger.ApplyDecisions(store, inputs, coupled)
	if err != nil {
		return renderDecisionApplyError(stdout, format, inputs, err)
	}
	return renderDecisionResults(stdout, format, results)
}

func runDecisionRetire(command *cli.Context, stdout io.Writer) error {
	format, err := implementationFormat(command.String("format"))
	if err != nil {
		return err
	}
	if command.NArg() != 0 {
		return renderDecisionRefusal(stdout, format,
			"decision retire takes flags, not positional arguments",
			"supply --project <name> and --proposal <name>")
	}
	project, proposal := command.String("project"), command.String("proposal")
	if project == "" || proposal == "" {
		return renderDecisionRefusal(stdout, format,
			"decision retire requires --project <name> and --proposal <name>",
			"select the recorded Project and proposal whose every slice is terminal and unclaimed")
	}
	store, failure := openConfiguredLedger()
	if failure != nil {
		return renderDecisionUnavailable(stdout, format, failure)
	}
	result, err := ledger.RetireProposal(store, project, proposal)
	if err != nil {
		var refusal *ledger.Refusal
		if !errors.As(err, &refusal) {
			return err
		}
		facts := &skilldist.DecisionFacts{
			Status: skilldist.DecisionRefused,
			Retirement: &skilldist.DecisionRetirement{
				Project: project, Proposal: proposal, Status: skilldist.DecisionRetirementRefused,
				Reason: refusal.Invariant, Repair: refusal.Repair,
			},
		}
		return renderDecision(stdout, format, decisionOutput{Status: string(facts.Status), Facts: facts})
	}
	status := skilldist.DecisionRetirementRecorded
	reason := ""
	if result.Status == ledger.RetirementAlreadyRetired {
		reason = "the proposal was already retired; nothing changed"
	}
	facts := &skilldist.DecisionFacts{
		Status: skilldist.DecisionApplied,
		Retirement: &skilldist.DecisionRetirement{
			Project: result.Project, Proposal: result.Proposal, Status: status,
			Reason: reason, Merged: len(result.Merged), Superseded: len(result.Superseded), Active: result.Active,
		},
	}
	return renderDecision(stdout, format, decisionOutput{Status: string(facts.Status), Facts: facts, Retirement: result})
}

// renderDecisionResults maps the engine's typed per-item results into the
// specialized facts and an honest overall status: applied only when every
// selected item is resolved, partial when some are, refused when none are.
func renderDecisionResults(stdout io.Writer, format implementationFormatKind, results []ledger.DecisionResult) error {
	facts := &skilldist.DecisionFacts{Outcomes: make([]skilldist.DecisionOutcome, 0, len(results))}
	resolved, unresolved := 0, 0
	for _, result := range results {
		outcome := skilldist.DecisionOutcome{
			Project: result.Project, Item: result.Item, Status: result.Status,
			Route: result.Route, Reason: result.Refusal,
			RequestCommit: result.Request.Commit, RequestPath: result.Request.Path,
		}
		if result.Decision.Commit != "" {
			outcome.Reference = result.Decision.Commit + ":" + result.Decision.Path
		}
		if result.Status == ledger.DecisionRefused {
			outcome.Repair = "read `skl decision inbox` for the current request and submit renewed human direction; do not retry the stale answer"
		}
		if result.Status == ledger.DecisionApplied || result.Status == ledger.DecisionAlreadyApplied {
			resolved++
		}
		if result.Status == ledger.DecisionUnresolved {
			unresolved++
			outcome.Repair = "repair the reported local failure and retry this exact scoped operation; preserve already-applied outcomes"
		}
		facts.Outcomes = append(facts.Outcomes, outcome)
	}
	switch {
	case resolved == len(results):
		facts.Status = skilldist.DecisionApplied
	case resolved > 0:
		facts.Status = skilldist.DecisionPartial
	case unresolved > 0:
		facts.Status = skilldist.DecisionUnresolved
	default:
		facts.Status = skilldist.DecisionRefused
	}
	return renderDecision(stdout, format, decisionOutput{Status: string(facts.Status), Facts: facts, Results: results})
}

// renderDecisionApplyError reports a coupled refusal that changed nothing.
// Every selected item is unresolved rather than silently split into partial
// authorization, and per-item validation never overwrote newer work.
func renderDecisionApplyError(stdout io.Writer, format implementationFormatKind, inputs []ledger.DecisionInput, err error) error {
	var refusal *ledger.Refusal
	if !errors.As(err, &refusal) {
		return err
	}
	facts := &skilldist.DecisionFacts{
		Status: skilldist.DecisionRefused,
		Reason: refusal.Invariant,
		Repair: refusal.Repair,
	}
	for _, input := range inputs {
		facts.Outcomes = append(facts.Outcomes, skilldist.DecisionOutcome{
			Project: input.Project, Item: input.Item, Status: skilldist.DecisionOutcomeUnresolved,
			Route: input.Route, Reason: refusal.Invariant,
			RequestCommit: input.Request.Commit, RequestPath: input.Request.Path,
		})
	}
	return renderDecision(stdout, format, decisionOutput{Status: string(facts.Status), Facts: facts})
}

// renderDecisionRefusal reports a refused invocation that changed nothing.
func renderDecisionRefusal(stdout io.Writer, format implementationFormatKind, reason, repair string) error {
	facts := &skilldist.DecisionFacts{Status: skilldist.DecisionRefused, Reason: reason, Repair: repair}
	return renderDecision(stdout, format, decisionOutput{Status: string(facts.Status), Facts: facts})
}

// renderDecisionUnavailable reports a configuration or access problem rather
// than an empty inbox or a silently changed state.
func renderDecisionUnavailable(stdout io.Writer, format implementationFormatKind, err error) error {
	var refusal *ledger.Refusal
	if errors.As(err, &refusal) {
		return renderDecision(stdout, format, decisionOutput{Status: string(skilldist.DecisionUnavailable), Facts: &skilldist.DecisionFacts{
			Status: skilldist.DecisionUnavailable, Reason: refusal.Invariant, Repair: refusal.Repair,
		}})
	}
	return renderDecision(stdout, format, decisionOutput{Status: string(skilldist.DecisionUnavailable), Facts: &skilldist.DecisionFacts{
		Status: skilldist.DecisionUnavailable, Reason: err.Error(),
		Repair: "repair the machine configuration or the configured ledger clone, then read the inbox again",
	}})
}

// renderDecision writes one invocation in the requested transport. Markdown is
// the specialized decision packet; JSON carries the same facts, the typed
// engine results, and that packet.
func renderDecision(stdout io.Writer, format implementationFormatKind, out decisionOutput) error {
	packet, err := skilldist.BuildPacket("decision", skilldist.InvocationFacts{Decision: out.Facts})
	if err != nil {
		return err
	}
	out.Packet = &packet
	if format == formatJSON {
		return json.NewEncoder(stdout).Encode(out)
	}
	if _, err := fmt.Fprint(stdout, packet.Instructions); err != nil {
		return err
	}
	return renderDecisionReplication(stdout, out.Results, out.Retirement)
}

// renderDecisionReplication reports the pending ledger replication of locally
// committed results. Local success never waits for a remote; an unavailable
// remote leaves the result authoritative and the replication visible.
func renderDecisionReplication(stdout io.Writer, results []ledger.DecisionResult, retirement *ledger.RetirementResult) error {
	type entry struct {
		item string
		note *ledger.PublicationNote
	}
	var entries []entry
	for _, result := range results {
		if result.Replication != nil {
			entries = append(entries, entry{item: result.Project + "/" + result.Item, note: result.Replication})
		}
	}
	if retirement != nil && retirement.Replication != nil {
		entries = append(entries, entry{item: retirement.Project + "/" + retirement.Proposal, note: retirement.Replication})
	}
	if len(entries) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(stdout, "\n## Ledger replication\n\n"); err != nil {
		return err
	}
	for _, item := range entries {
		detail := item.note.Detail
		if detail == "" {
			detail = "the local result is authoritative; no further detail was reported"
		}
		if _, err := fmt.Fprintf(stdout, "- %s: %s — %s\n", item.item, item.note.Status, detail); err != nil {
			return err
		}
	}
	return nil
}
