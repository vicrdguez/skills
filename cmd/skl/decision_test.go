package main

// Public-CLI coverage for the Human Decision surface. Every case drives the
// real `skl decision` commands through newApp against real local Git ledgers
// and isolated config, and asserts observable outcomes from the accepted
// behavior rules B1-B8 and ADR 0006, never from the engine's own computations.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

const decisionPauseBody = "# Blocking question\n\nA mandatory rule is at stake.\n\n" +
	"Conflict: the accepted Contract requires an outcome no default satisfies.\n\n" +
	"Evidence: the dashboard reports an unknown state.\n\n" +
	"Options:\n\n- In-contract option: keep the frozen obligation.\n- Out-of-contract option: change it.\n\n" +
	"Recommendation: the in-contract option.\n"

// decisionReadOnlyApp proves the decision surface resolves the machine
// configuration without a source checkout and never constructs a forge. Any
// attempt to open a Workflow Backend is a test failure.
func decisionReadOnlyApp(t *testing.T) ledgerCLI {
	t.Helper()
	factory := func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("decision commands must not construct a forge backend")
		return nil, errors.New("forge must not be constructed")
	}
	var output bytes.Buffer
	return ledgerCLI{app: newApp(factory, bytes.NewReader(nil), &output, &output), out: &output}
}

func (c ledgerCLI) decisionJSON(t *testing.T, args ...string) decisionOutput {
	t.Helper()
	text, err := c.deliveryRun(t, args...)
	if err != nil {
		t.Fatalf("decision command %v: %v\n%s", args, err, text)
	}
	var out decisionOutput
	if decodeErr := json.Unmarshal([]byte(text), &out); decodeErr != nil {
		t.Fatalf("decode decision output %q: %v", text, decodeErr)
	}
	return out
}

// decisionAccept accepts one proposal from a fresh checkout of a named source
// repository. Two different repository names become two Projects in the same
// configured ledger.
func decisionAccept(t *testing.T, forge *forgeServer, owner, name string, spec proposalSpec) string {
	t.Helper()
	source := sourceRepository(t, owner, name)
	accept := newLedgerApp(t, forge)
	outcome := accept.accept(t, source, writeProposal(t, "", spec))
	if outcome.Status != "accepted" {
		t.Fatalf("accept %s/%s/%s = %q", owner, name, spec.name, mustJSON(t, outcome))
	}
	return source
}

// decisionSelect acquires the next eligible Work Item in one source checkout.
func decisionSelect(t *testing.T, cli ledgerCLI, source string) deliveryOutput {
	t.Helper()
	return decisionSelectPhase(t, cli, ledger.ImplementPhase, source)
}

// decisionSelectPhase acquires the next eligible Work Item for one lane.
func decisionSelectPhase(t *testing.T, cli ledgerCLI, phase, source string) deliveryOutput {
	t.Helper()
	started, err := cli.deliveryJSON(t, "skl", phase, "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("%s next in %s: %#v %v", phase, source, started, err)
	}
	return started
}

// decisionReference renders one request's exact answered-request reference.
func decisionReference(request skilldist.DecisionRequest) ledger.Reference {
	return ledger.Reference{Commit: request.RequestCommit, Path: request.RequestPath}
}

// decisionPause pauses one claimed item, optionally supplying fixed source
// revisions through extra flags.
func decisionPause(t *testing.T, cli ledgerCLI, source, item, claim string, extra ...string) deliveryOutput {
	t.Helper()
	bodyPath := filepath.Join(t.TempDir(), "pause.md")
	writeFile(t, bodyPath, decisionPauseBody)
	args := []string{"skl", "implement", "needs-human", "--repo", source, "--item", item, "--claim", claim, "--body", bodyPath, "--format", "json"}
	args = append(args, extra...)
	paused, err := cli.deliveryJSON(t, args...)
	if err != nil || paused.Status != ledger.NeedsHuman {
		t.Fatalf("pause %s: %#v %v", item, paused, err)
	}
	return paused
}

// decisionPauseNext selects and pauses one item, returning its identity and
// the Claim it consumed.
func decisionPauseNext(t *testing.T, cli ledgerCLI, source string) (string, string) {
	t.Helper()
	started := decisionSelect(t, cli, source)
	decisionPause(t, cli, source, started.Execution.Item, started.Execution.Claim.Commit)
	return started.Execution.Item, started.Execution.Claim.Commit
}

// decisionRequest reads one current request for a Work Item from the exact
// bound inbox output.
func decisionRequest(t *testing.T, cli ledgerCLI, project, item string) skilldist.DecisionRequest {
	t.Helper()
	out := cli.decisionJSON(t, "skl", "decision", "inbox", "--project", project, "--format", "json")
	for _, request := range out.Facts.Requests {
		if request.Item == item {
			return request
		}
	}
	t.Fatalf("inbox for %s has no request for %s: %s", project, item, mustJSON(t, out.Facts))
	return skilldist.DecisionRequest{}
}

// decisionApplyAnswer records one Human Decision through the bound CLI shape.
func decisionApplyAnswer(t *testing.T, cli ledgerCLI, request skilldist.DecisionRequest, route, answer string) decisionOutput {
	t.Helper()
	answerPath := filepath.Join(t.TempDir(), "answer.md")
	writeFile(t, answerPath, answer)
	return cli.decisionJSON(t, "skl", "decision", "apply",
		"--project", request.Project, "--item", request.Item,
		"--request-commit", request.RequestCommit, "--request-path", request.RequestPath,
		"--route", route, "--answer", answerPath, "--format", "json")
}

func decisionItemStatePath(project, proposal, slice string) string {
	return "projects/" + project + "/proposals/" + proposal + "/" + slice + "/state.json"
}

// decisionState reads one committed state.json record.
func decisionState(t *testing.T, clone, path string) ledger.SliceState {
	t.Helper()
	raw := strings.TrimSpace(runGitOutput(t, clone, "show", "HEAD:"+path))
	var state ledger.SliceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("decode committed state %q: %v", raw, err)
	}
	return state
}

// decisionCommitState rewrites one committed state.json and commits it, as
// administrative fixture setup only.
func decisionCommitState(t *testing.T, clone, path string, mutate func(*ledger.SliceState)) {
	t.Helper()
	raw := strings.TrimSpace(runGitOutput(t, clone, "show", "HEAD:"+path))
	var state ledger.SliceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("decode fixture state %q: %v", raw, err)
	}
	mutate(&state)
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(clone, filepath.FromSlash(path)), string(encoded)+"\n")
	runGit(t, clone, "add", "-A")
	runGit(t, clone, "commit", "-q", "-m", "fixture state")
}

// TestDecisionInboxIsLedgerWideReadOnlyAndBound covers B1-B2/A1/A3: the inbox
// is ledger-wide regardless of the working directory, filters explicitly,
// carries exact request and document references, and changes nothing.
func TestDecisionInboxIsLedgerWideReadOnlyAndBound(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	widgetSource := decisionAccept(t, forge, "acme", "widgets", singleSlice("widget-work"))
	gadgetSource := decisionAccept(t, forge, "acme", "gadgets", singleSlice("gadget-work"))

	cli := decisionReadOnlyApp(t)
	widgetItem, _ := decisionPauseNext(t, cli, widgetSource)
	gadgetItem, _ := decisionPauseNext(t, cli, gadgetSource)
	before := ledgerSnapshot(t, fixture.clone)

	// Reading from a non-repository directory still sees every Project.
	t.Chdir(t.TempDir())
	all := cli.decisionJSON(t, "skl", "decision", "inbox", "--format", "json")
	if all.Status != string(skilldist.DecisionInbox) || len(all.Facts.Requests) != 2 {
		t.Fatalf("ledger-wide inbox = %#v", all)
	}
	rendered, err := cli.deliveryRun(t, "skl", "decision", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Project: widgets", "Project: gadgets",
		widgetItem, gadgetItem,
		"--request-commit", "--request-path",
		"--route <implement|watchdog|supersede>", "--answer <human-answer-file>",
		"Conflict: the accepted Contract requires an outcome no default satisfies.",
		"Recommendation: the in-contract option.",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("ledger-wide inbox is missing %q:\n%s", want, rendered)
		}
	}

	// Reading from inside one Project's checkout still does not narrow scope.
	t.Chdir(widgetSource)
	inside := cli.decisionJSON(t, "skl", "decision", "inbox", "--format", "json")
	if len(inside.Facts.Requests) != 2 {
		t.Fatalf("inbox narrowed by the working directory: %#v", inside.Facts.Requests)
	}

	// An explicit filter narrows; the bound command names the exact request.
	filtered := cli.decisionJSON(t, "skl", "decision", "inbox", "--project", "gadgets", "--format", "json")
	if len(filtered.Facts.Requests) != 1 || filtered.Facts.Requests[0].Project != "gadgets" {
		t.Fatalf("filtered inbox = %#v", filtered.Facts.Requests)
	}
	request := decisionRequest(t, cli, "widgets", widgetItem)
	if request.RequestCommit == "" || request.RequestPath == "" || request.Repository != "acme/widgets" {
		t.Fatalf("request facts lack exact references: %#v", request)
	}
	if len(request.Documents) < 3 {
		t.Fatalf("request lacks accepted documents and the blocking report: %#v", request.Documents)
	}
	if !strings.Contains(request.ApplyCommand, "--request-commit '"+request.RequestCommit+"'") ||
		!strings.Contains(request.ApplyCommand, "--request-path '"+request.RequestPath+"'") {
		t.Fatalf("apply command did not bind the exact request: %q", request.ApplyCommand)
	}

	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("reading the inbox changed the ledger")
	}
}

// TestDecisionInboxDistinguishesEmptyAndUnavailable covers B1: a readable
// ledger with no requests is empty, while an unusable configuration is
// unavailable rather than an empty inbox.
func TestDecisionInboxDistinguishesEmptyAndUnavailable(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	decisionAccept(t, forge, "acme", "widgets", singleSlice("quiet-work"))
	before := ledgerSnapshot(t, fixture.clone)

	cli := decisionReadOnlyApp(t)
	empty := cli.decisionJSON(t, "skl", "decision", "inbox", "--format", "json")
	if empty.Status != string(skilldist.DecisionEmpty) {
		t.Fatalf("request-free ledger = %#v, want an empty inbox", empty)
	}
	markdown, err := cli.deliveryRun(t, "skl", "decision", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "empty inbox") || strings.Contains(markdown, "could not be resolved or read") {
		t.Fatalf("empty inbox rendered indistinctly:\n%s", markdown)
	}

	unknown := cli.decisionJSON(t, "skl", "decision", "inbox", "--project", "no-such-project", "--format", "json")
	if unknown.Status != string(skilldist.DecisionUnavailable) || unknown.Facts.Reason == "" || unknown.Facts.Repair == "" {
		t.Fatalf("unknown Project = %#v, want an unavailable refusal", unknown)
	}

	fixture.misconfigure(t, `{"ledger": "/does/not/exist"}`+"\n")
	unavailable := cli.decisionJSON(t, "skl", "decision", "inbox", "--format", "json")
	if unavailable.Status != string(skilldist.DecisionUnavailable) {
		t.Fatalf("unusable configuration = %#v, want unavailable", unavailable)
	}
	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("empty or unavailable inbox changed the ledger")
	}
}

// TestDecisionAppliesAtomicallyAndRetries covers B3-B5/A2: one answer and its
// route commit together, unrelated commits do not invalidate the request, an
// exact retry is recognized without disturbing later work, and a differing
// replay is refused.
func TestDecisionAppliesAtomicallyAndRetries(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source := decisionAccept(t, forge, "acme", "widgets", singleSlice("decision-work"))
	decisionAccept(t, forge, "acme", "gadgets", singleSlice("other-work"))

	cli := decisionReadOnlyApp(t)
	item, _ := decisionPauseNext(t, cli, source)
	request := decisionRequest(t, cli, "widgets", item)
	contractPath := "projects/widgets/proposals/decision-work/foundation/behavior.md"
	contractBefore := strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "HEAD:"+contractPath))
	headBefore := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))

	// An unrelated Project commits before the answer is applied.
	writeFile(t, filepath.Join(fixture.clone, "projects/gadgets/notes.txt"), "unrelated\n")
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "unrelated ledger commit")

	answer := "# Human direction\n\nTake the in-contract option for " + item + ".\n"
	applied := decisionApplyAnswer(t, cli, request, ledger.RouteImplement, answer)
	if applied.Status != ledger.DecisionApplied || len(applied.Results) != 1 || applied.Results[0].State != ledger.ReadyForImplementation {
		t.Fatalf("apply = %#v", applied)
	}
	decisionCommit := applied.Results[0].Decision.Commit
	if decisionCommit == "" || decisionCommit == headBefore {
		t.Fatalf("decision did not commit: %#v", applied.Results[0].Decision)
	}
	changed := strings.Fields(strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "--name-only", "--format=", decisionCommit)))
	if len(changed) != 2 || !slices.Contains(changed, "projects/widgets/proposals/decision-work/foundation/decision.md") {
		t.Fatalf("decision commit changed %v, want decision.md and state.json together", changed)
	}
	record, body, err := ledger.ParseDecision([]byte(strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "HEAD:projects/widgets/proposals/decision-work/foundation/decision.md"))))
	if err != nil {
		t.Fatal(err)
	}
	if record.AnsweredRequest != decisionReference(request) || record.Route != ledger.RouteImplement || body != strings.TrimSpace(answer) {
		t.Fatalf("committed decision = %#v / %q", record, body)
	}
	if got := strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "HEAD:"+contractPath)); got != contractBefore {
		t.Fatal("the decision rewrote the frozen Contract")
	}

	// Lost-response retry: the exact operation is recognized with no new commit.
	retried := decisionApplyAnswer(t, cli, request, ledger.RouteImplement, answer)
	if len(retried.Results) != 1 || retried.Results[0].Status != ledger.DecisionAlreadyApplied || strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")) != decisionCommit {
		t.Fatalf("exact retry = %#v", retried)
	}

	// A later Claim survives the exact retry untouched.
	directed := decisionSelect(t, cli, source)
	if directed.Execution.State.Claim.Inputs.Decision == nil {
		t.Fatalf("continued work did not consume the decision: %#v", directed.Execution)
	}
	pinned := *directed.Execution.State.Claim.Inputs.Decision
	shown := cli.ledgerJSON(t, "skl", "ledger", "show", "--commit", pinned.Commit, "--path", pinned.Path, "--format", "json")
	if shown.Document == nil {
		t.Fatalf("ledger show returned no decision document for %#v", pinned)
	}
	shownRecord, shownAnswer, err := ledger.ParseDecision([]byte(shown.Document.Contents))
	if err != nil {
		t.Fatal(err)
	}
	if shownAnswer != answer || shownRecord.AnsweredRequest != decisionReference(request) {
		t.Fatalf("worker consumed a different decision: %#v / %q", shownRecord, shownAnswer)
	}
	retried = decisionApplyAnswer(t, cli, request, ledger.RouteImplement, answer)
	if len(retried.Results) != 1 || retried.Results[0].Status != ledger.DecisionAlreadyApplied {
		t.Fatalf("retry after a later Claim = %#v", retried)
	}
	if state := decisionState(t, fixture.clone, decisionItemStatePath("widgets", "decision-work", "foundation")); state.Claim == nil {
		t.Fatalf("exact retry released the later Claim: %#v", state)
	}

	// A differing replay is refused without overwriting the recorded result.
	headBeforeReplay := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))
	differing := decisionApplyAnswer(t, cli, request, ledger.RouteSupersede, "actually abandon it\n")
	if differing.Status != ledger.DecisionRefused || len(differing.Results) != 1 || differing.Results[0].Refusal == "" {
		t.Fatalf("differing replay = %#v", differing)
	}
	if strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")) != headBeforeReplay {
		t.Fatal("differing replay mutated the ledger")
	}
}

// TestDecisionRefusesReplacedRequestAndMalformedInput covers B3-B4/A2: a
// replacement request with identical prose is refused, and malformed scope or
// route is refused without mutation.
func TestDecisionRefusesReplacedRequestAndMalformedInput(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source := decisionAccept(t, forge, "acme", "widgets", singleSlice("replace-work"))

	cli := decisionReadOnlyApp(t)
	started := decisionSelect(t, cli, source)
	item := started.Execution.Item
	decisionPause(t, cli, source, item, started.Execution.Claim.Commit)
	request := decisionRequest(t, cli, "widgets", item)

	// Replace the blocking report with identical prose and a different claim.
	reportPath := "projects/widgets/proposals/replace-work/foundation/implement-report.md"
	raw := strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "HEAD:"+reportPath))
	replaced := strings.Replace(raw, started.Execution.Claim.Commit, strings.Repeat("a", 40), 1)
	if replaced == raw {
		t.Fatal("fixture did not alter the recorded claim identity")
	}
	writeFile(t, filepath.Join(fixture.clone, filepath.FromSlash(reportPath)), replaced+"\n")
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "replace request report")
	headBefore := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))

	refused := decisionApplyAnswer(t, cli, request, ledger.RouteImplement, "continue within the contract\n")
	if refused.Status != ledger.DecisionRefused || len(refused.Results) != 1 || !strings.Contains(refused.Results[0].Refusal, "replaced") {
		t.Fatalf("replaced request = %#v", refused)
	}
	state := decisionState(t, fixture.clone, decisionItemStatePath("widgets", "replace-work", "foundation"))
	if state.State != ledger.NeedsHuman || state.Decision || state.Claim != nil {
		t.Fatalf("refusal changed the paused record: %#v", state)
	}
	if strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")) != headBefore {
		t.Fatal("replaced-request refusal mutated the ledger")
	}

	// An invalid route is refused by the ledger without mutation.
	request = decisionRequest(t, cli, "widgets", item)
	badRoute := decisionApplyAnswer(t, cli, request, "merge", "continue\n")
	if badRoute.Status != ledger.DecisionRefused || !strings.Contains(badRoute.Results[0].Refusal, "not implement, watchdog, or supersede") {
		t.Fatalf("invalid route = %#v", badRoute)
	}

	// Missing or mixed CLI input is refused before any ledger call.
	missing := cli.decisionJSON(t, "skl", "decision", "apply", "--project", "widgets", "--format", "json")
	if missing.Status != ledger.DecisionRefused || missing.Facts.Reason == "" {
		t.Fatalf("missing scope = %#v", missing)
	}
	inputPath := filepath.Join(t.TempDir(), "batch.json")
	writeFile(t, inputPath, "[]\n")
	mixed := cli.decisionJSON(t, "skl", "decision", "apply", "--project", "widgets", "--item", item, "--input", inputPath, "--format", "json")
	if mixed.Status != ledger.DecisionRefused || mixed.Facts.Reason == "" {
		t.Fatalf("mixed modes = %#v", mixed)
	}
	if after := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")); after != headBefore {
		t.Fatal("a refused invocation mutated the ledger")
	}
}

// TestDecisionBatchIndependentAndCoupled covers B7/A2: independent members
// report partial results honestly, while a coupled set is refused whole rather
// than silently split, and an exact coupled retry is recognized.
func TestDecisionBatchIndependentAndCoupled(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	widgetSource := decisionAccept(t, forge, "acme", "widgets", singleSlice("batch-work"))
	gadgetSource := decisionAccept(t, forge, "acme", "gadgets", singleSlice("batch-other"))

	cli := decisionReadOnlyApp(t)
	widgetItem, _ := decisionPauseNext(t, cli, widgetSource)
	gadgetItem, _ := decisionPauseNext(t, cli, gadgetSource)
	widgetRequest := decisionRequest(t, cli, "widgets", widgetItem)
	gadgetRequest := decisionRequest(t, cli, "gadgets", gadgetItem)

	// Independent direction: one current request applies, one stale member
	// refuses, and the result never claims the whole group succeeded.
	batch := filepath.Join(t.TempDir(), "decisions.json")
	writeFile(t, batch, mustJSON(t, []ledger.DecisionInput{
		{Project: "widgets", Item: widgetItem, Request: decisionReference(widgetRequest), Answer: "implement it\n", Route: ledger.RouteImplement},
		{Project: "gadgets", Item: gadgetItem, Request: ledger.Reference{Commit: strings.Repeat("b", 40), Path: gadgetRequest.RequestPath}, Answer: "implement it\n", Route: ledger.RouteImplement},
	})+"\n")
	partial := cli.decisionJSON(t, "skl", "decision", "apply", "--input", batch, "--format", "json")
	if partial.Status != string(skilldist.DecisionPartial) {
		t.Fatalf("independent batch = %#v, want a partial result", partial)
	}
	statuses := map[string]string{}
	for _, result := range partial.Results {
		statuses[result.Item] = result.Status
	}
	if statuses[widgetItem] != ledger.DecisionApplied || statuses[gadgetItem] != ledger.DecisionRefused {
		t.Fatalf("independent batch statuses = %#v", statuses)
	}
	if state := decisionState(t, fixture.clone, decisionItemStatePath("widgets", "batch-work", "foundation")); state.State != ledger.ReadyForImplementation || !state.Decision {
		t.Fatalf("the applied member did not commit: %#v", state)
	}

	// Coupled direction refuses whole when one member cannot apply.
	headBefore := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))
	coupled := cli.decisionJSON(t, "skl", "decision", "apply", "--input", batch, "--coupled", "--format", "json")
	if coupled.Status != ledger.DecisionRefused || len(coupled.Facts.Outcomes) != 2 {
		t.Fatalf("coupled refusal = %#v", coupled)
	}
	for _, outcome := range coupled.Facts.Outcomes {
		if outcome.Status != string(skilldist.DecisionOutcomeUnresolved) {
			t.Fatalf("coupled member was silently split: %#v", outcome)
		}
	}
	if strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")) != headBefore {
		t.Fatal("coupled refusal mutated the ledger")
	}

	// A coupled set whose members are all valid is recognized, including the
	// already-applied member, without recording a second decision.
	freshBatch := filepath.Join(t.TempDir(), "coupled.json")
	writeFile(t, freshBatch, mustJSON(t, []ledger.DecisionInput{
		{Project: "widgets", Item: widgetItem, Request: decisionReference(widgetRequest), Answer: "implement it\n", Route: ledger.RouteImplement},
		{Project: "gadgets", Item: gadgetItem, Request: decisionReference(gadgetRequest), Answer: "implement it\n", Route: ledger.RouteImplement},
	})+"\n")
	applied := cli.decisionJSON(t, "skl", "decision", "apply", "--input", freshBatch, "--coupled", "--format", "json")
	if applied.Status != ledger.DecisionApplied || len(applied.Results) != 2 {
		t.Fatalf("coupled success = %#v", applied)
	}
	if applied.Results[0].Status != ledger.DecisionAlreadyApplied || applied.Results[1].Status != ledger.DecisionApplied {
		t.Fatalf("coupled statuses = %#v", applied.Results)
	}
}

// TestDecisionPendingReplicationIsVisible covers B5/A2/B6: local success is
// authoritative with an unavailable remote, and the pending replication is
// reported instead of blocking the result.
func TestDecisionPendingReplicationIsVisible(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source := decisionAccept(t, forge, "acme", "widgets", singleSlice("offline-work"))
	other := decisionAccept(t, forge, "acme", "gadgets", singleSlice("offline-other"))

	cli := decisionReadOnlyApp(t)
	item, _ := decisionPauseNext(t, cli, source)
	otherItem, _ := decisionPauseNext(t, cli, other)
	request := decisionRequest(t, cli, "widgets", item)
	otherRequest := decisionRequest(t, cli, "gadgets", otherItem)

	if err := os.RemoveAll(fixture.upstream); err != nil {
		t.Fatal(err)
	}
	applied := decisionApplyAnswer(t, cli, request, ledger.RouteImplement, "continue offline\n")
	if applied.Status != ledger.DecisionApplied || len(applied.Results) != 1 {
		t.Fatalf("offline apply = %#v", applied)
	}
	if applied.Results[0].Replication == nil || applied.Results[0].Replication.Status != ledger.PushPending {
		t.Fatalf("pending replication = %#v, want pending despite the local success", applied.Results[0].Replication)
	}

	// The Markdown transport reports the same pending replication.
	answerPath := filepath.Join(t.TempDir(), "answer.md")
	writeFile(t, answerPath, "continue offline\n")
	markdown, err := cli.deliveryRun(t, "skl", "decision", "apply",
		"--project", otherRequest.Project, "--item", otherRequest.Item,
		"--request-commit", otherRequest.RequestCommit, "--request-path", otherRequest.RequestPath,
		"--route", ledger.RouteImplement, "--answer", answerPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Ledger replication", ledger.PushPending} {
		if !strings.Contains(markdown, want) {
			t.Errorf("offline Markdown is missing %q:\n%s", want, markdown)
		}
	}

	// Both local results remain readable after the unavailable replication.
	inbox := cli.decisionJSON(t, "skl", "decision", "inbox", "--format", "json")
	if inbox.Status != string(skilldist.DecisionEmpty) {
		t.Fatalf("decided items are still paused: %#v", inbox)
	}
}

// TestDecisionContinuationReachesWorkers covers B6/A3: the exact recorded
// answer and reference flow through worker startup, the handoff consumes it,
// and the continuation keeps its route.
func TestDecisionContinuationReachesWorkers(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)

	cli := decisionReadOnlyApp(t)
	started := decisionSelect(t, cli, source)
	decisionPause(t, cli, source, deliveryTestItem, started.Execution.Claim.Commit)
	request := decisionRequest(t, cli, "widgets", deliveryTestItem)

	answer := "# Human direction\n\nContinue the frozen Contract at " + target + ".\n"
	applied := decisionApplyAnswer(t, cli, request, ledger.RouteImplement, answer)
	if applied.Status != ledger.DecisionApplied {
		t.Fatalf("apply = %#v", applied)
	}

	directed := decisionSelect(t, cli, source)
	if directed.Execution.State.Claim.Inputs.Decision == nil {
		t.Fatalf("implement did not consume the decision: %#v", directed.Execution)
	}
	shown := cli.ledgerJSON(t, "skl", "ledger", "show",
		"--commit", directed.Execution.State.Claim.Inputs.Decision.Commit,
		"--path", directed.Execution.State.Claim.Inputs.Decision.Path, "--format", "json")
	if shown.Document == nil {
		t.Fatal("worker decision reference is unreadable")
	}
	record, body, err := ledger.ParseDecision([]byte(shown.Document.Contents))
	if err != nil {
		t.Fatal(err)
	}
	if body != answer || record.Route != ledger.RouteImplement || record.AnsweredRequest != decisionReference(request) {
		t.Fatalf("implement consumed a different decision: %#v / %q", record, body)
	}

	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", directed.Execution.Claim.Commit, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("prepare: %#v %v", prepared, err)
	}
	bodyPath := filepath.Join(t.TempDir(), "implement-report.md")
	writeFile(t, bodyPath, "# implementation result\n")
	submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", directed.Execution.Claim.Commit,
		"--head", target, "--target", target, "--body", bodyPath, "--format", "json")
	if err != nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit: %#v %v", submitted, err)
	}
	if carried := deliveryPersistedState(t, fixture.clone); !carried.Decision {
		t.Fatal("the implementation handoff dropped the direction the review must consume")
	}

	// The exact lost-response retry is recognized after the later result and
	// never rewrites it.
	reportBefore := strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "HEAD:"+request.RequestPath))
	retriedApply := decisionApplyAnswer(t, cli, request, ledger.RouteImplement, answer)
	if len(retriedApply.Results) != 1 || retriedApply.Results[0].Status != ledger.DecisionAlreadyApplied {
		t.Fatalf("retry after a later result = %#v", retriedApply)
	}
	if after := strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "HEAD:"+request.RequestPath)); after != reportBefore {
		t.Fatal("exact retry rewrote the later implement report")
	}

	review := decisionSelectPhase(t, cli, ledger.WatchdogPhase, source)
	if review.Execution.State.Claim.Inputs.Decision == nil {
		t.Fatalf("review did not consume the direction: %#v", review.Execution)
	}
	reviewShown := cli.ledgerJSON(t, "skl", "ledger", "show",
		"--commit", review.Execution.State.Claim.Inputs.Decision.Commit,
		"--path", review.Execution.State.Claim.Inputs.Decision.Path, "--format", "json")
	_, reviewAnswer, err := ledger.ParseDecision([]byte(reviewShown.Document.Contents))
	if err != nil {
		t.Fatal(err)
	}
	if reviewAnswer != answer {
		t.Fatalf("review consumed a different answer: %q", reviewAnswer)
	}
	reviewBody := filepath.Join(t.TempDir(), "watchdog-report.md")
	writeFile(t, reviewBody, "# review result\n")
	passed, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem,
		"--claim", review.Execution.Claim.Commit, "--outcome", "pass", "--head", target, "--body", reviewBody, "--format", "json")
	if err != nil || passed.Status != ledger.ReadyForMerge {
		t.Fatalf("watchdog pass: %#v %v", passed, err)
	}
	if final := deliveryPersistedState(t, fixture.clone); final.Claim != nil || final.State != ledger.ReadyForMerge || final.Decision {
		t.Fatalf("final state = %#v, want a consumed direction ready for merge", final)
	}
}

// TestDecisionWatchdogContinuationAtSameCode covers B6/A3: a Watchdog
// continuation returns round 2 to review at the unchanged code revision,
// retains the completed-review count, produces round 3, and can reach Ready
// for Merge without a new source commit.
func TestDecisionWatchdogContinuationAtSameCode(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	cli := decisionReadOnlyApp(t)

	// Round 1: a change is implemented and reviewed as Rework.
	started := decisionSelect(t, cli, source)
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", started.Execution.Claim.Commit, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("prepare: %#v %v", prepared, err)
	}
	worktree := prepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "feature.txt"), "foundation\n")
	runGit(t, worktree, "add", "feature.txt")
	runGit(t, worktree, "commit", "-q", "-m", "implement foundation")
	head := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")
	bodyPath := filepath.Join(t.TempDir(), "implement-report.md")
	writeFile(t, bodyPath, "# implementation result\n")
	first, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", started.Execution.Claim.Commit,
		"--head", head, "--target", target, "--body", bodyPath, "--format", "json")
	if err != nil || first.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit: %#v %v", first, err)
	}
	reviewBody := filepath.Join(t.TempDir(), "watchdog-report.md")
	writeFile(t, reviewBody, "# review result\n")
	review1 := decisionSelectPhase(t, cli, ledger.WatchdogPhase, source)
	reworked, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", review1.Execution.Claim.Commit,
		"--outcome", "rework", "--body", reviewBody, "--format", "json")
	if err != nil || reworked.Status != ledger.Rework {
		t.Fatalf("round 1 rework: %#v %v", reworked, err)
	}

	// Round 2: rework without a new commit, then a failing review reaches the
	// automatic-rework cap and pauses for human direction.
	reworkClaim := decisionSelect(t, cli, source)
	prepared2, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", reworkClaim.Execution.Claim.Commit, "--format", "json")
	if err != nil || prepared2.Source == nil || prepared2.Source.Head != head {
		t.Fatalf("rework prepare: %#v %v", prepared2, err)
	}
	second, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", reworkClaim.Execution.Claim.Commit,
		"--head", head, "--target", target, "--body", bodyPath, "--format", "json")
	if err != nil || second.Status != ledger.AwaitingReview {
		t.Fatalf("rework submit: %#v %v", second, err)
	}
	review2 := decisionSelectPhase(t, cli, ledger.WatchdogPhase, source)
	roundTwo, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", review2.Execution.Claim.Commit,
		"--outcome", "rework", "--body", reviewBody, "--format", "json")
	if err != nil || roundTwo.Status != ledger.NeedsHuman {
		t.Fatalf("round 2 rework: %#v %v", roundTwo, err)
	}
	if state := deliveryPersistedState(t, fixture.clone); state.Claim != nil || state.State != ledger.NeedsHuman {
		t.Fatalf("round 2 pause = %#v", state)
	}

	// Human direction returns the item to review at the unchanged revision.
	request := decisionRequest(t, cli, "widgets", deliveryTestItem)
	answer := "# Human direction\n\nReconsider the findings at the unchanged " + head + ".\n"
	applied := decisionApplyAnswer(t, cli, request, ledger.RouteWatchdog, answer)
	if applied.Status != ledger.DecisionApplied || applied.Results[0].State != ledger.AwaitingReview {
		t.Fatalf("watchdog direction = %#v", applied)
	}

	// Round 3 reviews the same code with the recorded count and can pass.
	review3 := decisionSelectPhase(t, cli, ledger.WatchdogPhase, source)
	if review3.Execution.Implement == nil || review3.Execution.Implement.Source.Head != head {
		t.Fatalf("round 3 did not review the unchanged revision %s: %#v", head, review3.Execution)
	}
	if review3.Execution.State.Claim.Inputs.Decision == nil {
		t.Fatalf("round 3 did not consume the direction: %#v", review3.Execution)
	}
	passed, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", review3.Execution.Claim.Commit,
		"--outcome", "pass", "--body", reviewBody, "--format", "json")
	if err != nil || passed.Status != ledger.ReadyForMerge || passed.Result == nil {
		t.Fatalf("round 3 pass: %#v %v", passed, err)
	}
	report, _ := deliveryCommittedReport(t, cli, passed.Result.Report, ledger.WatchdogPhase)
	if report.Round != 3 || report.Source.Reviewed != head {
		t.Fatalf("round 3 review = round %d of %s, want round 3 of the unchanged %s", report.Round, report.Source.Reviewed, head)
	}
}

// TestDecisionRetirementGuardsPartialDelivery covers B8/A4: retirement is
// refused while active work remains, succeeds only when every slice is
// terminal and unclaimed, reports partial delivery, and never unblocks a
// dependent.
func TestDecisionRetirementGuardsPartialDelivery(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)

	retiring := proposalSpec{
		name:        "retire-proposal",
		description: "# Retire proposal\n",
		parentTitle: "Retire proposal",
		slices: []proposalSliceSpec{
			{name: "foundation", title: "Foundation", files: map[string]string{"intent.md": "# Foundation\n", "behavior.md": "# Foundation behavior\n"}},
			{name: "feature", title: "Feature", files: map[string]string{"intent.md": "# Feature\n", "behavior.md": "# Feature behavior\n"}},
		},
	}
	decisionAccept(t, forge, "acme", "widgets", retiring)
	dependent := singleSlice("dependent-proposal")
	dependent.depends = map[string][]string{"foundation": {"proposals/retire-proposal/feature"}}
	dependentSource := decisionAccept(t, forge, "acme", "widgets", dependent)

	// Administrative fixture setup: the first slice was already delivered.
	decisionCommitState(t, fixture.clone, decisionItemStatePath("widgets", "retire-proposal", "foundation"), func(state *ledger.SliceState) {
		state.State = ledger.Merged
	})

	cli := decisionReadOnlyApp(t)
	item, _ := decisionPauseNext(t, cli, dependentSource)
	if item != "retire-proposal/feature" {
		t.Fatalf("paused the wrong item %q", item)
	}

	// Active work prevents retirement.
	headBefore := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))
	refused := cli.decisionJSON(t, "skl", "decision", "retire", "--project", "widgets", "--proposal", "retire-proposal", "--format", "json")
	if refused.Status != ledger.DecisionRefused || refused.Facts.Retirement == nil || !strings.Contains(refused.Facts.Retirement.Reason, "active or claimed work") {
		t.Fatalf("retirement guard = %#v", refused)
	}
	if strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")) != headBefore {
		t.Fatal("a refused retirement mutated the ledger")
	}
	if strings.Contains(strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "HEAD:projects/widgets/proposals/retire-proposal/proposal.json")), `"retired": true`) {
		t.Fatal("a refused retirement recorded the parent as retired")
	}

	// Abandon the paused slice, then retire the fully terminal parent.
	request := decisionRequest(t, cli, "widgets", "retire-proposal/feature")
	superseded := decisionApplyAnswer(t, cli, request, ledger.RouteSupersede, "abandon the wrong-scope feature\n")
	if superseded.Status != ledger.DecisionApplied || superseded.Results[0].State != ledger.Superseded {
		t.Fatalf("supersede = %#v", superseded)
	}
	retired := cli.decisionJSON(t, "skl", "decision", "retire", "--project", "widgets", "--proposal", "retire-proposal", "--format", "json")
	if retired.Status != ledger.DecisionApplied || retired.Facts.Retirement == nil {
		t.Fatalf("retirement = %#v", retired)
	}
	if retired.Facts.Retirement.Status != skilldist.DecisionRetirementRecorded || retired.Facts.Retirement.Merged != 1 || retired.Facts.Retirement.Superseded != 1 {
		t.Fatalf("retirement facts = %#v", retired.Facts.Retirement)
	}
	if retired.Retirement == nil || !retired.Retirement.PartialDelivery {
		t.Fatalf("retirement claimed full delivery: %#v", retired.Retirement)
	}
	markdown, err := cli.deliveryRun(t, "skl", "decision", "retire", "--project", "widgets", "--proposal", "retire-proposal")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "reports partial delivery, not all-delivered completion") {
		t.Fatalf("retirement markdown = %q", markdown)
	}

	// The dependent remains blocked: the Superseded blocker is not Merged and
	// retirement remapped nothing.
	dependentState := strings.TrimSpace(runGitOutput(t, fixture.clone, "show", "HEAD:"+decisionItemStatePath("widgets", "dependent-proposal", "foundation")))
	if !strings.Contains(dependentState, "retire-proposal/feature") {
		t.Fatalf("retirement remapped or dropped the dependency: %s", dependentState)
	}
	blocked, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", dependentSource, "--format", "json")
	if err != nil || blocked.Status != "no_work" {
		t.Fatalf("a dependent of superseded work became eligible: %#v %v", blocked, err)
	}
}

// TestDecisionStandaloneSkillAndTriageResource covers A3: the conversation
// skill and its static triage resource retrieve through public skl without
// ledger facts or forge construction.
func TestDecisionStandaloneSkillAndTriageResource(t *testing.T) {
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("read-only decision retrieval must not construct a forge")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "skill", "decision"}); err != nil {
		t.Fatal(err)
	}
	standalone := output.String()
	if !strings.Contains(standalone, "skl decision inbox") || !strings.Contains(standalone, "records no decision") {
		t.Fatalf("standalone decision retrieval is unclear:\n%s", standalone)
	}
	if strings.Contains(standalone, "## Current requests") {
		t.Error("standalone retrieval rendered a request list without ledger facts")
	}

	output.Reset()
	if err := app.Run([]string{"skl", "skill", "--resource", "reference/triage.md", "decision"}); err != nil {
		t.Fatal(err)
	}
	triage := output.String()
	for _, want := range []string{
		"only narrowing",
		"ask for it; do not invent it",
		"not authorization",
		"do not demand a ceremonial second confirmation",
		"applied`, `already_applied`, `refused`, or `unresolved",
		"renewed proposal and re-slicing",
		"Superseded blocker is not Merged",
	} {
		if !strings.Contains(triage, want) {
			t.Errorf("triage resource is missing %q", want)
		}
	}
}
