package main

// End-to-end CLI regression coverage for ledger-backed delivery. The public
// `skl` command surface is exercised through newApp against real local Git
// ledger/source fixtures; expected outcomes come from the accepted behavior
// rules B1-B12 and ADR 0006, never from the engine's own computations.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

const (
	deliveryTestProposal = "delivery-proposal"
	deliveryTestSlice    = "foundation"
	deliveryTestItem     = deliveryTestProposal + "/" + deliveryTestSlice
	deliveryTestBranch   = deliveryTestSlice
)

func deliveryItemDirectory() string {
	return "projects/widgets/proposals/" + deliveryTestItem
}

func deliveryItemStatePath() string {
	return deliveryItemDirectory() + "/state.json"
}

// deliveryTrimmed returns one Git command's output without surrounding
// whitespace.
func deliveryTrimmed(t *testing.T, directory string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(runGitOutput(t, directory, args...))
}

// deliverySourceRepo creates an isolated source checkout. The GitHub-shaped
// remote is identity only; every network operation runs the deterministic
// failing local command so no test touches a real remote.
func deliverySourceRepo(t *testing.T) (root, target string) {
	t.Helper()
	t.Setenv("GIT_SSH_COMMAND", "false")
	root = sourceRepository(t, "acme", "widgets")
	target = deliveryTrimmed(t, root, "rev-parse", "HEAD")
	// The recorded Integration Target is available locally without a fetch.
	runGit(t, root, "update-ref", "refs/remotes/origin/main", target)
	return root, target
}

// deliveryNoForgeApp is the ordinary worker surface: the factory cannot
// construct a forge adapter, exactly as an isolated offline caller experiences.
func deliveryNoForgeApp(t *testing.T) ledgerCLI {
	t.Helper()
	factory := func(github.RepositoryID) (setup.Backend, error) {
		return nil, errors.New("forge unavailable in isolated delivery test")
	}
	var output bytes.Buffer
	return ledgerCLI{app: newApp(factory, bytes.NewReader(nil), &output, &output), out: &output}
}

// deliveryForgeFactory binds the controllable forge for the interruption check.
func deliveryForgeFactory(forge *forgeServer) backendFactory {
	return func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(forge.server.URL, "secret", forge.server.Client())
		backend.BindRepository(repository)
		return backend, nil
	}
}

// deliveryInterruptedWriter fails every write, standing in for a caller that
// loses the CLI's acknowledgement after the local handoff already committed.
type deliveryInterruptedWriter struct{}

func (deliveryInterruptedWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated output interruption")
}

// deliveryRun runs one CLI invocation and returns its raw stdout and error.
func (c ledgerCLI) deliveryRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c.out.Reset()
	err := c.app.Run(args)
	return c.out.String(), err
}

// deliveryJSON runs one delivery invocation against the JSON transport.
func (c ledgerCLI) deliveryJSON(t *testing.T, args ...string) (deliveryOutput, error) {
	t.Helper()
	text, err := c.deliveryRun(t, args...)
	if err != nil {
		return deliveryOutput{}, err
	}
	var out deliveryOutput
	if strings.TrimSpace(text) == "" {
		t.Fatalf("delivery command %v produced no output", args)
	}
	if decodeErr := json.Unmarshal([]byte(text), &out); decodeErr != nil {
		t.Fatalf("decode delivery output %q: %v", text, decodeErr)
	}
	return out, nil
}

// ledgerJSON runs one ledger invocation against the JSON transport.
func (c ledgerCLI) ledgerJSON(t *testing.T, args ...string) ledgerOutcome {
	t.Helper()
	text, err := c.deliveryRun(t, args...)
	if err != nil {
		t.Fatalf("ledger command %v: %v\n%s", args, err, text)
	}
	var out ledgerOutcome
	if decodeErr := json.Unmarshal([]byte(text), &out); decodeErr != nil {
		t.Fatalf("decode ledger output %q: %v", text, decodeErr)
	}
	return out
}

// deliveryPersistedState reads the committed state.json record, never a
// derived or recomputed value.
func deliveryPersistedState(t *testing.T, clone string) ledger.SliceState {
	t.Helper()
	raw := runGitOutput(t, clone, "show", "HEAD:"+deliveryItemStatePath())
	var state ledger.SliceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("decode committed state %q: %v", raw, err)
	}
	return state
}

// deliveryCommittedReport reads the exact phase report referenced by a
// handoff and returns its schema-1 metadata and opaque body.
func deliveryCommittedReport(t *testing.T, cli ledgerCLI, reference ledger.Reference, phase string) (ledger.Report, string) {
	t.Helper()
	shown := cli.ledgerJSON(t, "skl", "ledger", "show", "--commit", reference.Commit, "--path", reference.Path, "--format", "json")
	if shown.Document == nil {
		t.Fatalf("ledger show returned no document for %s at %s", reference.Path, reference.Commit)
	}
	if shown.Document.Commit != reference.Commit || shown.Document.Path != reference.Path {
		t.Fatalf("ledger show returned a different reference: %#v", shown.Document)
	}
	report, body, err := ledger.ParseReport(phase, []byte(shown.Document.Contents))
	if err != nil {
		t.Fatalf("parse committed %s report: %v", phase, err)
	}
	return report, body
}

// deliveryRecordHumanDirection records an explicitly scoped Human Decision
// through the public decision surface: it reads the item's exact current
// request from the ledger-wide inbox and applies the human's answer with its
// continuation route. It performs no direct ledger write.
func deliveryRecordHumanDirection(t *testing.T, cli ledgerCLI, head, route string) decisionOutput {
	t.Helper()
	request := decisionRequest(t, cli, "widgets", deliveryTestItem)
	return decisionApplyAnswer(t, cli, request, route, "# Human direction\n\nContinue at "+head+" within the frozen Contract.\n")
}

// deliveryCommitState rewrites one committed state.json and commits it.
func deliveryCommitState(t *testing.T, clone string, state ledger.SliceState) {
	t.Helper()
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(clone, filepath.FromSlash(deliveryItemStatePath())), string(encoded)+"\n")
	runGit(t, clone, "add", "-A")
	runGit(t, clone, "commit", "-q", "-m", "fixture state")
}

// deliveryAcceptFixture accepts one single-slice proposal locally with a
// private Contract that carries a human Manual Verification check.
func deliveryAcceptFixture(t *testing.T, forge *forgeServer, source string) {
	t.Helper()
	accept := newLedgerApp(t, forge)
	accepted := accept.accept(t, source, writeProposal(t, "", singleSlice(deliveryTestProposal)))
	if accepted.Status != "accepted" {
		t.Fatalf("acceptance status = %q, want accepted: %s", accepted.Status, mustJSON(t, accepted))
	}
}

// TestDeliveryCLIEndToEnd is the cohesive public-CLI regression: local
// acceptance through independent review, rework, human direction, and pass,
// with offline publication throughout.
func TestDeliveryCLIEndToEnd(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)

	contractDirectory := filepath.Join(fixture.clone, filepath.FromSlash(deliveryItemDirectory()))
	contractBefore := map[string]string{}
	for _, name := range []string{"intent.md", "behavior.md"} {
		contractBefore[name] = readFileString(t, filepath.Join(contractDirectory, name))
	}

	cli := deliveryNoForgeApp(t)

	// Rule B1/B2: selection returns one exact Claim and no source branch yet.
	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("implement next: %v", err)
	}
	if started.Status != "work_available" || started.Execution == nil {
		t.Fatalf("implement next = %#v, want one execution", started)
	}
	claim := started.Execution.Claim.Commit
	if claim == "" || started.Execution.Claim.Path != deliveryItemStatePath() {
		t.Fatalf("claim reference = %#v, want the selected state record", started.Execution.Claim)
	}
	runGit(t, fixture.clone, "cat-file", "-e", claim+"^{commit}")
	if gitRefExists(source, "refs/heads/"+deliveryTestBranch) {
		t.Fatal("source branch existed before prepare")
	}
	if started.Packet == nil {
		t.Fatal("implement next returned no execution packet")
	}
	startInstructions := started.Packet.Instructions
	if !strings.Contains(startInstructions, "--claim '"+claim+"'") {
		t.Errorf("initial instructions do not bind the acquired Claim %s", claim)
	}
	if len(started.Execution.Documents) == 0 {
		t.Fatal("initial execution did not carry the frozen accepted Contract")
	}
	for _, document := range started.Execution.Documents {
		if !strings.Contains(startInstructions, document.Commit+":"+document.Path) {
			t.Errorf("instructions lost the exact Contract reference %s:%s", document.Commit, document.Path)
		}
	}

	// Rule B2: prepare uses the recorded available target without a remote.
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil {
		t.Fatalf("implement prepare: %v", err)
	}
	if prepared.Status != "prepared" || prepared.Source == nil {
		t.Fatalf("implement prepare = %#v, want prepared source facts", prepared)
	}
	if prepared.Source.Target != target {
		t.Fatalf("prepare target = %s, want recorded %s", prepared.Source.Target, target)
	}
	if !gitRefExists(source, "refs/heads/"+deliveryTestBranch) {
		t.Fatal("prepare did not create the planned source branch")
	}
	worktree := prepared.Source.Worktree
	if prepared.Source.Head != target {
		t.Fatalf("prepared head = %s, want the recorded target %s", prepared.Source.Head, target)
	}

	// Rule B3/B8: inspect resolves the prepared branch against the exact target.
	inspected, err := cli.deliveryJSON(t, "skl", "implement", "inspect", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--target", target, "--format", "json")
	if err != nil {
		t.Fatalf("implement inspect: %v", err)
	}
	if inspected.Status != "inspected" || inspected.Source == nil {
		t.Fatalf("implement inspect = %#v", inspected)
	}
	if inspected.Source.Head != prepared.Source.Head || inspected.Source.Target != target {
		t.Fatalf("inspect source = %#v, want head %s target %s", inspected.Source, prepared.Source.Head, target)
	}
	if inspected.Source.Scope != "full" {
		t.Fatalf("first implementation inspection scope = %q, want full", inspected.Source.Scope)
	}

	// Unrelated malformed source markers are committed history, not authority.
	writeFile(t, filepath.Join(worktree, "feature.txt"), "delivered foundation\n")
	writeFile(t, filepath.Join(worktree, ".changes", "legacy", "behavior.md"), "not a schema report\n")
	writeFile(t, filepath.Join(worktree, ".changes", "legacy", "state.json"), "{not json\n")
	writeFile(t, filepath.Join(worktree, ".watchdog"), "round: 99\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "implement foundation")
	head := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")

	privateBody := "# Implementation result\n\n" +
		"## Completion and evidence\n\n| Item | Status | Evidence |\n| --- | --- | --- |\n| B1 | complete | focused check |\n| M1 | incomplete | human-owned Manual Verification |\n\n" +
		"---\n\nLiteral delimiter-looking text stays opaque.\n<!-- outcome: pass -->\n"
	publicBody := "# Delivered foundation\n\nA separately authored temporary human-facing summary.\n"
	bodyPath := filepath.Join(t.TempDir(), "implement-report.md")
	publicPath := filepath.Join(t.TempDir(), "public.md")
	writeFile(t, bodyPath, privateBody)
	writeFile(t, publicPath, publicBody)

	// Rule B5/B10: the local handoff succeeds while every remote is unavailable.
	if err := os.RemoveAll(fixture.upstream); err != nil {
		t.Fatal(err)
	}
	submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", head, "--target", target, "--body", bodyPath, "--public-body", publicPath, "--format", "json")
	if err != nil {
		t.Fatalf("implement submit: %v", err)
	}
	if submitted.Status != ledger.AwaitingReview || submitted.Result == nil {
		t.Fatalf("implement submit = %#v, want awaiting_review", submitted)
	}
	if submitted.Result.AlreadyCompleted {
		t.Fatal("first handoff was reported as a replay")
	}
	if submitted.Result.Report.Path != deliveryItemDirectory()+"/implement-report.md" {
		t.Fatalf("report path = %q", submitted.Result.Report.Path)
	}
	if submitted.Result.Publication == nil || submitted.Result.Publication.Status != ledger.IssuePending {
		t.Fatalf("publication = %#v, want pending without falsified success", submitted.Result.Publication)
	}
	if submitted.Result.Replication == nil || submitted.Result.Replication.Status == ledger.PushPushed {
		t.Fatalf("replication = %#v, want pending with the ledger remote unavailable", submitted.Result.Replication)
	}
	state := deliveryPersistedState(t, fixture.clone)
	if state.Claim != nil || state.State != ledger.AwaitingReview {
		t.Fatalf("committed state = %#v, want released Claim awaiting review", state)
	}

	// Rule B4: the persisted report carries schema 1, repository namespaces,
	// and the opaque private body byte for byte.
	report, committedBody := deliveryCommittedReport(t, cli, submitted.Result.Report, ledger.ImplementPhase)
	if report.Schema != 1 {
		t.Fatalf("report schema = %d, want 1", report.Schema)
	}
	if report.Source.Head != head || report.Source.Target != target {
		t.Fatalf("report source = %#v, want head %s target %s", report.Source, head, target)
	}
	if report.Ledger.Claim.Commit != claim || len(report.Ledger.Contract) == 0 {
		t.Fatalf("report ledger inputs = %#v", report.Ledger)
	}
	if committedBody != privateBody {
		t.Fatalf("committed body changed:\n%q\nwant\n%q", committedBody, privateBody)
	}

	// Rule B6/B7/B8: independent review consumes the report without any source
	// `.changes` or `.watchdog`, then records round 1 and routes to Rework.
	review, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("watchdog next: %v", err)
	}
	if review.Execution == nil || review.Execution.Implement == nil {
		t.Fatalf("watchdog next = %#v, want the consumed implementation report", review)
	}
	if review.Execution.Implement.Source.Head != head {
		t.Fatalf("watchdog consumed head %s, want %s", review.Execution.Implement.Source.Head, head)
	}
	if review.Packet == nil {
		t.Fatal("watchdog next returned no execution packet")
	}
	reviewClaim := review.Execution.Claim.Commit
	reviewInstructions := review.Packet.Instructions
	if !strings.Contains(reviewInstructions, "--claim '"+reviewClaim+"'") {
		t.Errorf("watchdog instructions do not bind the acquired Claim %s", reviewClaim)
	}
	reviewPrepared, err := cli.deliveryJSON(t, "skl", "watchdog", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", reviewClaim, "--format", "json")
	if err != nil {
		t.Fatalf("watchdog prepare: %v", err)
	}
	if reviewPrepared.Source == nil || reviewPrepared.Source.Head != head || reviewPrepared.Source.Target != target {
		t.Fatalf("watchdog prepare = %#v, want fixed head and target", reviewPrepared.Source)
	}
	reviewInspected, err := cli.deliveryJSON(t, "skl", "watchdog", "inspect", "--repo", source, "--item", deliveryTestItem, "--claim", reviewClaim, "--format", "json")
	if err != nil {
		t.Fatalf("watchdog inspect: %v", err)
	}
	if reviewInspected.Source == nil || reviewInspected.Source.Head != head || reviewInspected.Source.Scope != "full" {
		t.Fatalf("first watchdog inspect = %#v, want full review of %s", reviewInspected.Source, head)
	}

	reviewBody := "# Watchdog review\n\nW1 resolved as rework.\n"
	reviewBodyPath := filepath.Join(t.TempDir(), "watchdog-report.md")
	writeFile(t, reviewBodyPath, reviewBody)
	reviewPublicPath := filepath.Join(t.TempDir(), "watchdog-public.md")
	writeFile(t, reviewPublicPath, "# Review pending\n")
	watchdogArguments := []string{"skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", reviewClaim,
		"--outcome", "rework", "--body", reviewBodyPath, "--public-body", reviewPublicPath, "--format", "json"}

	// Rule B5: an interrupted acknowledgement leaves the committed handoff and
	// the same-command retry recognizes it without recording a second round.
	interrupted := newApp(deliveryForgeFactory(forge), bytes.NewReader(nil), deliveryInterruptedWriter{}, deliveryInterruptedWriter{})
	if runErr := interrupted.Run(watchdogArguments); runErr == nil {
		t.Fatal("interrupted output write did not fail the CLI invocation")
	}
	interruptedState := deliveryPersistedState(t, fixture.clone)
	if interruptedState.State != ledger.Rework || interruptedState.Claim != nil {
		t.Fatalf("interrupted handoff state = %#v, want committed Rework with a released Claim", interruptedState)
	}
	retried, err := cli.deliveryJSON(t, watchdogArguments...)
	if err != nil {
		t.Fatalf("watchdog submit retry: %v", err)
	}
	if retried.Status != ledger.Rework || retried.Result == nil || !retried.Result.AlreadyCompleted {
		t.Fatalf("retry = %#v, want the recognized completed rework", retried)
	}
	roundOne, _ := deliveryCommittedReport(t, cli, retried.Result.Report, ledger.WatchdogPhase)
	if roundOne.Round != 1 {
		t.Fatalf("retry advanced the completed-review count to %d, want 1", roundOne.Round)
	}

	// Rule B2/B7: recreate the source worktree, resume the preserved branch
	// progress, and record a new implementation submission.
	runGit(t, source, "worktree", "remove", worktree)
	if preserved := deliveryTrimmed(t, source, "rev-parse", "refs/heads/"+deliveryTestBranch); preserved != head {
		t.Fatalf("worktree removal changed branch progress: %s, want %s", preserved, head)
	}
	rework, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("rework implement next: %v", err)
	}
	if rework.Execution == nil || rework.Execution.Claim.Commit == claim {
		t.Fatalf("rework did not acquire a fresh Claim: %#v", rework.Execution)
	}
	reworkClaim := rework.Execution.Claim.Commit
	if rework.Packet == nil || !strings.Contains(rework.Packet.Instructions, "--claim '"+reworkClaim+"'") {
		t.Fatal("rework instructions do not bind the fresh Claim")
	}
	reprepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", reworkClaim, "--format", "json")
	if err != nil {
		t.Fatalf("rework implement prepare: %v", err)
	}
	if reprepared.Source == nil || reprepared.Source.Head != head {
		t.Fatalf("rework prepare = %#v, want preserved branch progress at %s", reprepared.Source, head)
	}
	worktree = reprepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "feature.txt"), "delivered foundation, findings resolved\n")
	runGit(t, worktree, "add", "feature.txt")
	runGit(t, worktree, "commit", "-q", "-m", "resolve findings")
	head2 := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")
	second, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", reworkClaim,
		"--head", head2, "--target", target, "--body", bodyPath, "--public-body", publicPath, "--format", "json")
	if err != nil {
		t.Fatalf("rework implement submit: %v", err)
	}
	if second.Status != ledger.AwaitingReview {
		t.Fatalf("rework submit = %#v, want awaiting_review", second)
	}

	// Round 2 fails: the recorded completed-review count advances to 2 and the
	// Work Item requires human direction.
	review2, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("second watchdog next: %v", err)
	}
	reviewClaim2 := review2.Execution.Claim.Commit
	inspected2, err := cli.deliveryJSON(t, "skl", "watchdog", "inspect", "--repo", source, "--item", deliveryTestItem, "--claim", reviewClaim2, "--format", "json")
	if err != nil {
		t.Fatalf("second watchdog inspect: %v", err)
	}
	if inspected2.Source == nil || inspected2.Source.Scope != "incremental" || inspected2.Source.Previous != head {
		t.Fatalf("second watchdog inspect = %#v, want incremental since %s", inspected2.Source, head)
	}
	round2, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", reviewClaim2,
		"--outcome", "rework", "--body", reviewBodyPath, "--public-body", reviewPublicPath, "--format", "json")
	if err != nil {
		t.Fatalf("second watchdog submit: %v", err)
	}
	if round2.Status != ledger.NeedsHuman || round2.Result == nil {
		t.Fatalf("second rework = %#v, want needs_human", round2)
	}
	roundTwoReport, _ := deliveryCommittedReport(t, cli, round2.Result.Report, ledger.WatchdogPhase)
	if roundTwoReport.Round != 2 {
		t.Fatalf("completed-review count = %d, want 2 (retained through worktree recreation)", roundTwoReport.Round)
	}
	if after := deliveryPersistedState(t, fixture.clone); after.Claim != nil || after.State != ledger.NeedsHuman {
		t.Fatalf("state after second rework = %#v", after)
	}

	// Rule B7: recorded human direction authorizes continued implementation;
	// the same direction carries through the implementation handoff into the
	// independent review that follows, with no second directive needed.
	recorded := deliveryRecordHumanDirection(t, cli, head2, ledger.RouteImplement)
	if recorded.Status != ledger.DecisionApplied {
		t.Fatalf("record human direction = %#v, want applied", recorded)
	}
	directed, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("directed implement next: %v", err)
	}
	if directed.Execution == nil || directed.Execution.State.Claim.Inputs.Decision == nil {
		t.Fatalf("directed implementation did not consume the human direction: %#v", directed.Execution)
	}
	directedClaim := directed.Execution.Claim.Commit
	directedPrepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", directedClaim, "--format", "json")
	if err != nil {
		t.Fatalf("directed implement prepare: %v", err)
	}
	if directedPrepared.Source == nil || directedPrepared.Source.Head != head2 {
		t.Fatalf("directed prepare = %#v, want preserved progress at %s", directedPrepared.Source, head2)
	}
	// The direction resolves as no further source change: the directed
	// implementation hands off the unchanged source revision for review.
	directedSubmit, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", directedClaim,
		"--head", head2, "--target", target, "--body", bodyPath, "--public-body", publicPath, "--format", "json")
	if err != nil {
		t.Fatalf("directed implement submit: %v", err)
	}
	if directedSubmit.Status != ledger.AwaitingReview {
		t.Fatalf("directed submit = %#v, want awaiting_review", directedSubmit)
	}
	if carried := deliveryPersistedState(t, fixture.clone); !carried.Decision {
		t.Fatal("implementation handoff consumed the direction the following review must consume")
	}

	// Rule B7/B8: the review consumes that same direction and records round 3.
	review3, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("third watchdog next: %v", err)
	}
	if review3.Execution == nil || review3.Execution.Implement == nil || review3.Execution.Implement.Source.Head != head2 {
		t.Fatalf("review did not consume the unchanged source revision %s: %#v", head2, review3.Execution)
	}
	if review3.Execution.State.Claim.Inputs.Decision == nil {
		t.Fatalf("review did not consume the carried human direction: %#v", review3.Execution)
	}
	reviewClaim3 := review3.Execution.Claim.Commit
	passed, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", reviewClaim3,
		"--outcome", "pass", "--body", reviewBodyPath, "--public-body", reviewPublicPath, "--format", "json")
	if err != nil {
		t.Fatalf("passing watchdog submit: %v", err)
	}
	if passed.Status != ledger.ReadyForMerge || passed.Result == nil {
		t.Fatalf("pass = %#v, want ready_for_merge", passed)
	}
	roundThree, _ := deliveryCommittedReport(t, cli, passed.Result.Report, ledger.WatchdogPhase)
	if roundThree.Round != 3 || roundThree.Source.Reviewed != head2 {
		t.Fatalf("passing review = round %d of %s, want round 3 of the unchanged %s", roundThree.Round, roundThree.Source.Reviewed, head2)
	}
	if final := deliveryPersistedState(t, fixture.clone); final.Claim != nil || final.State != ledger.ReadyForMerge || final.Decision {
		t.Fatalf("final state = %#v, want released Claim ready for merge with the direction consumed", final)
	}

	// Rule B6/B11: the frozen Contract bytes never changed and the malformed
	// source markers remain untouched, unreferenced history.
	for name, before := range contractBefore {
		if after := readFileString(t, filepath.Join(contractDirectory, name)); after != before {
			t.Fatalf("accepted Contract %s changed:\n%s\nwant\n%s", name, after, before)
		}
	}
	runGit(t, source, "cat-file", "-e", head+":.changes/legacy/behavior.md")
	if !gitRefExists(source, "refs/heads/"+deliveryTestBranch) {
		t.Fatal("engine rewrote the delivered source branch")
	}
}

// TestDeliveryCLINoWorkAndFormats proves truthful non-work and refusal
// outcomes across Markdown and JSON, and that an unknown transport acquires
// nothing.
func TestDeliveryCLINoWorkAndFormats(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	cli := deliveryNoForgeApp(t)

	noWork, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("watchdog next: %v", err)
	}
	if noWork.Status != "no_work" || noWork.Execution != nil {
		t.Fatalf("JSON non-work = %#v, want truthful no_work without an execution", noWork)
	}
	markdown, err := cli.deliveryRun(t, "skl", "watchdog", "next", "--repo", source)
	if err != nil {
		t.Fatalf("markdown watchdog next: %v", err)
	}
	if !strings.Contains(markdown, "Status: no_work") {
		t.Fatalf("markdown non-work = %q, want a truthful no_work status", markdown)
	}

	refused, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", "missing/slice", "--claim", strings.Repeat("a", 40), "--format", "json")
	if err != nil {
		t.Fatalf("JSON refusal: %v", err)
	}
	if refused.Status != "fix_required" || refused.Reason == "" || refused.Repair == "" {
		t.Fatalf("JSON refusal = %#v, want fix_required with reason and repair", refused)
	}
	markdown, err = cli.deliveryRun(t, "skl", "implement", "prepare", "--repo", source, "--item", "missing/slice", "--claim", strings.Repeat("a", 40))
	if err != nil {
		t.Fatalf("markdown refusal: %v", err)
	}
	if !strings.Contains(markdown, "Status: fix_required") || !strings.Contains(markdown, "Repair:") {
		t.Fatalf("markdown refusal = %q, want fix_required and repair", markdown)
	}

	before := ledgerSnapshot(t, fixture.clone)
	if _, err := cli.deliveryRun(t, "skl", "implement", "next", "--repo", source, "--format", "xml"); err == nil {
		t.Fatal("unknown output format was accepted")
	}
	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("unknown output format acquired a Claim or mutated the ledger")
	}
}

// TestDeliveryCLIDeliversSlashedPlannedBranch proves a planned branch that
// acceptance admits, such as feat/<slice>, is claimable and preparable.
func TestDeliveryCLIDeliversSlashedPlannedBranch(t *testing.T) {
	newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	spec := singleSlice(deliveryTestProposal)
	spec.slices[0].branch = "feat/foundation"
	if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, source, writeProposal(t, "", spec)); accepted.Status != "accepted" {
		t.Fatalf("accept slashed branch: %s", mustJSON(t, accepted))
	}
	cli := deliveryNoForgeApp(t)

	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || started.Status != "work_available" || started.Execution == nil {
		t.Fatalf("implement next = %#v, err=%v", started, err)
	}
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", started.Execution.Item, "--claim", started.Execution.Claim.Commit, "--format", "json")
	if err != nil || prepared.Status != "prepared" || prepared.Source == nil {
		t.Fatalf("implement prepare = %#v, err=%v", prepared, err)
	}
	if prepared.Source.Head != target {
		t.Fatalf("prepared head = %s, want %s", prepared.Source.Head, target)
	}
	if !strings.HasSuffix(prepared.Source.Worktree, filepath.Join(".worktrees", "feat", "foundation")) {
		t.Fatalf("worktree = %s, want it under .worktrees/feat/foundation", prepared.Source.Worktree)
	}
	if got := deliveryTrimmed(t, prepared.Source.Worktree, "symbolic-ref", "--short", "HEAD"); got != "feat/foundation" {
		t.Fatalf("prepared worktree is on %q, want feat/foundation", got)
	}
}

// TestDeliveryCLIClaimBoundaries proves a released or foreign Claim cannot
// resume, release, or overwrite a later reservation.
func TestDeliveryCLIClaimBoundaries(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	cli := deliveryNoForgeApp(t)

	first, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("first implement next: %v", err)
	}
	oldClaim := first.Execution.Claim.Commit
	released, err := cli.deliveryJSON(t, "skl", "implement", "release", "--repo", source, "--item", deliveryTestItem, "--claim", oldClaim, "--format", "json")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Status != "released" {
		t.Fatalf("release = %#v", released)
	}

	later, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("second implement next: %v", err)
	}
	laterClaim := later.Execution.Claim.Commit
	if laterClaim == oldClaim {
		t.Fatal("reacquisition reused the released Claim commit")
	}

	for _, args := range [][]string{
		{"skl", "implement", "resume", "--repo", source, "--item", deliveryTestItem, "--claim", oldClaim, "--format", "json"},
		{"skl", "implement", "release", "--repo", source, "--item", deliveryTestItem, "--claim", oldClaim, "--format", "json"},
	} {
		out, err := cli.deliveryJSON(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if out.Status != "fix_required" {
			t.Fatalf("%v = %#v, want fix_required for the old Claim", args, out)
		}
	}

	bodyPath := filepath.Join(t.TempDir(), "implement-report.md")
	writeFile(t, bodyPath, "# stale result\n")
	stale, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", oldClaim, "--body", bodyPath, "--format", "json")
	if err != nil {
		t.Fatalf("stale submit: %v", err)
	}
	if stale.Status != "fix_required" {
		t.Fatalf("stale submit = %#v, want fix_required", stale)
	}
	state := deliveryPersistedState(t, fixture.clone)
	if state.Claim == nil || state.Claim.Phase != ledger.ImplementPhase || state.State != ledger.ReadyForImplementation {
		t.Fatalf("later Claim was disturbed: %#v", state)
	}
}

// TestDeliveryCLIRefusalsPreserveClaim proves a dirty source or a wrong
// Integration Target refuses the handoff while retaining the Claim and
// progress, and that the repaired inputs then succeed.
func TestDeliveryCLIRefusalsPreserveClaim(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	cli := deliveryNoForgeApp(t)

	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("implement next: %v", err)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil {
		t.Fatalf("implement prepare: %v", err)
	}
	worktree := prepared.Source.Worktree
	progress := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")

	writeFile(t, filepath.Join(worktree, "dirty.txt"), "uncommitted\n")
	dirty, err := cli.deliveryJSON(t, "skl", "implement", "inspect", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--target", target, "--format", "json")
	if err != nil {
		t.Fatalf("dirty inspect: %v", err)
	}
	if dirty.Status != "fix_required" {
		t.Fatalf("dirty inspect = %#v, want fix_required", dirty)
	}
	if state := deliveryPersistedState(t, fixture.clone); state.Claim == nil {
		t.Fatal("dirty refusal lost the Claim")
	}
	if preserved := deliveryTrimmed(t, source, "rev-parse", "refs/heads/"+deliveryTestBranch); preserved != progress {
		t.Fatalf("dirty refusal changed branch progress: %s, want %s", preserved, progress)
	}

	if err := os.Remove(filepath.Join(worktree, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(worktree, "feature.txt"), "foundation\n")
	runGit(t, worktree, "add", "feature.txt")
	runGit(t, worktree, "commit", "-q", "-m", "implement foundation")
	head := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")

	tree := deliveryTrimmed(t, source, "rev-parse", head+"^{tree}")
	divergent := deliveryTrimmed(t, source, "commit-tree", tree, "-m", "divergent root")
	bodyPath := filepath.Join(t.TempDir(), "implement-report.md")
	writeFile(t, bodyPath, "# result\n")
	wrong, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", head, "--target", divergent, "--body", bodyPath, "--format", "json")
	if err != nil {
		t.Fatalf("wrong-target submit: %v", err)
	}
	if wrong.Status != "fix_required" {
		t.Fatalf("wrong-target submit = %#v, want fix_required", wrong)
	}
	state := deliveryPersistedState(t, fixture.clone)
	if state.Claim == nil || state.State != ledger.ReadyForImplementation {
		t.Fatalf("wrong-target refusal advanced or released the Claim: %#v", state)
	}

	// The Markdown refusal of a verified Claim says the Claim is kept and binds
	// the submit to rerun.
	text, err := cli.deliveryRun(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", head, "--target", divergent, "--body", bodyPath)
	if err != nil {
		t.Fatalf("markdown wrong-target submit: %v", err)
	}
	refused := regexp.MustCompile(`(?m)^Refused: (.+)$`).FindStringSubmatch(text)
	if refused == nil || refused[1] != wrong.Reason {
		t.Fatalf("markdown refusal does not state the refused invariant %q:\n%s", wrong.Reason, text)
	}
	if !strings.Contains(text, "`"+claim+"` is kept") || strings.Contains(text, "changed no Claim") {
		t.Fatalf("markdown refusal does not keep the verified Claim:\n%s", text)
	}
	rerun := regexp.MustCompile("`(skl implement submit [^`]*)`").FindStringSubmatch(text)
	if rerun == nil || !strings.Contains(rerun[1], "--claim "+skilldist.ShellQuote(claim)) || !strings.Contains(rerun[1], "--target "+skilldist.ShellQuote(divergent)) {
		t.Fatalf("markdown refusal does not bind the submit to rerun:\n%s", text)
	}

	good, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", head, "--target", target, "--body", bodyPath, "--format", "json")
	if err != nil {
		t.Fatalf("correct submit: %v", err)
	}
	if good.Status != ledger.AwaitingReview {
		t.Fatalf("correct submit = %#v, want awaiting_review", good)
	}
}

// TestDeliveryCLIRejectsUnknownSchema proves an incompatible required report
// refuses before any replacement Claim is acquired.
func TestDeliveryCLIRejectsUnknownSchema(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	cli := deliveryNoForgeApp(t)

	state := deliveryPersistedState(t, fixture.clone)
	state.State = ledger.AwaitingReview
	deliveryCommitState(t, fixture.clone, state)

	claim := strings.Repeat("a", 40)
	report := fmt.Sprintf(`---
schema: 2
outcome: awaiting_review
source:
  head: %s
  target: %s
ledger:
  claim:
    commit: %s
    path: %s
  contract:
    - commit: %s
      path: %s
---
body
`, target, target, claim, deliveryItemStatePath(), target, deliveryItemDirectory()+"/behavior.md")
	writeFile(t, filepath.Join(fixture.clone, filepath.FromSlash(deliveryItemDirectory()+"/implement-report.md")), report)
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "fixture incompatible report")

	before := ledgerSnapshot(t, fixture.clone)
	refused, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("watchdog next with schema 2: %v", err)
	}
	if refused.Status != "fix_required" || refused.Execution != nil {
		t.Fatalf("incompatible schema = %#v, want fix_required without an execution", refused)
	}
	if !strings.Contains(refused.Reason, "schema") {
		t.Fatalf("refusal reason = %q, want the incompatible schema named", refused.Reason)
	}
	after := deliveryPersistedState(t, fixture.clone)
	if after.Claim != nil {
		t.Fatalf("incompatible schema acquired a Claim: %#v", after.Claim)
	}
	if snapshot := ledgerSnapshot(t, fixture.clone); snapshot != before {
		t.Fatal("incompatible schema mutated the ledger")
	}
}
