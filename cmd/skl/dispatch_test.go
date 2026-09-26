package main

// Dispatch coverage at the CLI seam against real ledger and source fixtures.
// Expected outcomes come from the dispatch-verified-work behavior (B1-B7):
// commands are parsed from the Supervisor's output and run as a Supervisor
// would run them.

import (
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vicrdguez/skills/ledger"
)

// shellWords splits a bound command into its arguments: bare words and
// single-quoted words, the only forms skl binds.
func shellWords(t *testing.T, command string) []string {
	t.Helper()
	var words []string
	var word strings.Builder
	quoted, started := false, false
	for _, r := range command {
		switch {
		case r == '\'':
			quoted, started = !quoted, true
		case r == ' ' && !quoted:
			if started {
				words = append(words, word.String())
			}
			word.Reset()
			started = false
		default:
			word.WriteRune(r)
			started = true
		}
	}
	if quoted {
		t.Fatalf("unterminated quote in %q", command)
	}
	if started {
		words = append(words, word.String())
	}
	return words
}

// dispatchRun runs one bound command and returns its Markdown output.
func (c ledgerCLI) dispatchRun(t *testing.T, command string) string {
	t.Helper()
	output, err := c.deliveryRun(t, shellWords(t, command)...)
	if err != nil {
		t.Fatalf("%s: %v\n%s", command, err, output)
	}
	return output
}

// dispatchJSON runs one delivery command through the JSON transport.
func (c ledgerCLI) dispatchJSON(t *testing.T, command string) deliveryOutput {
	t.Helper()
	words := shellWords(t, command)
	if !slices.Contains(words, "--format") {
		words = append(words, "--format", "json")
	}
	out, err := c.deliveryJSON(t, words...)
	if err != nil {
		t.Fatalf("%s: %v", command, err)
	}
	return out
}

// dispatched runs one Dispatch and returns the Claim it made.
func (c ledgerCLI) dispatched(t *testing.T, command string) *dispatched {
	t.Helper()
	out := c.dispatchJSON(t, command)
	if out.Status != "dispatched" || out.Dispatch == nil || out.Packet != nil || out.Execution != nil {
		t.Fatalf("%s = %s, want a dispatch without the Execution Skill", command, mustJSON(t, out))
	}
	return out.Dispatch
}

// dispatchImplementHandoff prepares the claimed Work Item, commits a change,
// and submits it for review, as a dispatched Implement worker does.
func dispatchImplementHandoff(t *testing.T, cli ledgerCLI, source, target, item, claim string) {
	t.Helper()
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", item, "--claim", claim, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("implement prepare: %#v %v", prepared, err)
	}
	head := proseCommit(t, prepared.Source.Worktree, "dispatch.txt", "claim "+claim+"\n")
	submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", item, "--claim", claim,
		"--head", head, "--target", target, "--body", proseFixturePath("implement-report.md"), "--public-body", proseFixturePath("public.md"), "--format", "json")
	if err != nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit: %#v %v", submitted, err)
	}
}

// dispatchReview claims the Work Item's review and records outcome.
func dispatchReview(t *testing.T, cli ledgerCLI, source, outcome string) {
	t.Helper()
	started, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("watchdog next: %#v %v", started, err)
	}
	dispatchVerdict(t, cli, source, started.Execution.Claim.Commit, outcome)
}

// dispatchVerdict records a review's outcome for its Claim.
func dispatchVerdict(t *testing.T, cli ledgerCLI, source, claim, outcome string) {
	t.Helper()
	reviewed, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--outcome", outcome, "--body", proseFixturePath("watchdog-report.md"), "--public-body", proseFixturePath("public.md"), "--format", "json")
	if err != nil || reviewed.Result == nil {
		t.Fatalf("watchdog submit %s: %#v %v", outcome, reviewed, err)
	}
}

// sameRendering compares two Execution Skills of one Work Item after
// replacing what differs between two Claims: ledger commits and the Result
// Documents directory.
func sameRendering(t *testing.T, got, want string) {
	t.Helper()
	normalize := func(text string) string {
		text = proseSHA.ReplaceAllString(text, "<commit>")
		return proseResultDir.ReplaceAllString(text, "skl-$1-result")
	}
	if normalize(got) != normalize(want) {
		t.Fatalf("dispatched worker rendering differs from ordinary next:\n--- worker\n%s\n--- next\n%s", got, want)
	}
}

// TestDispatchAnswersWithCommandsAndTheWorkerGetsNextsSkill covers B1-B3 for
// both phases and every Procedure next chooses: initial, rework and review.
func TestDispatchAnswersWithCommandsAndTheWorkerGetsNextsSkill(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)
	root, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}

	// B1/B2: the dispatch claims, and answers with commands only.
	dispatch := cli.dispatchRun(t, "skl implement next --dispatch --repo "+source+" --remote origin --wait=2m --poll=5s --worker-model openai-codex/gpt-6-astra --worker-thinking high")
	state := deliveryPersistedState(t, fixture.clone)
	if state.Claim == nil || state.Claim.Phase != ledger.ImplementPhase {
		t.Fatalf("dispatch did not claim the Slice: %#v", state)
	}
	claim := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	for _, skill := range []string{"Foundation behavior", "Contract-Grounded Testing", "skl implement prepare", "skl implement submit"} {
		if strings.Contains(dispatch, skill) {
			t.Errorf("dispatch output carries the Execution Skill (%q):\n%s", skill, dispatch)
		}
	}
	wantWorker := "skl implement resume --repo '" + root + "' --remote 'origin' --item '" + deliveryTestItem + "' --claim '" + claim + "' --dispatched"
	wantContinue := "skl implement next --repo '" + root + "' --remote 'origin' --wait=2m0s --poll=5s --dispatch --after '" + claim + "' --worker-model 'openai-codex/gpt-6-astra' --worker-thinking 'high'"
	for _, want := range []string{"`openai-codex/gpt-6-astra`", "`high`", "`" + wantWorker + "`", "`" + wantContinue + "`"} {
		if !strings.Contains(dispatch, want) {
			t.Errorf("dispatch output lacks %s:\n%s", want, dispatch)
		}
	}

	// B3: the worker receives the initial Procedure next would have returned.
	dispatchedSkill := cli.dispatchRun(t, wantWorker)
	cli.dispatchRun(t, "skl implement release --repo "+source+" --item "+deliveryTestItem+" --claim "+claim)
	ordinary := cli.dispatchJSON(t, "skl implement next --repo "+source+"")
	if ordinary.Packet == nil || ordinary.Packet.Facts.Delivery.Procedure != "initial" {
		t.Fatalf("ordinary next: %s", mustJSON(t, ordinary))
	}
	sameRendering(t, dispatchedSkill, ordinary.Packet.Instructions)
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, ordinary.Execution.Claim.Commit)

	// Watchdog: the ordinary review Procedure, with omitted worker values
	// rendering no sentence.
	review := cli.dispatchRun(t, "skl watchdog next --dispatch --repo "+source)
	if strings.Contains(review, "model") || strings.Contains(review, "thinking") {
		t.Errorf("omitted worker values rendered a sentence:\n%s", review)
	}
	reviewClaim := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	dispatchedReview := cli.dispatchRun(t, "skl watchdog resume --repo "+source+" --item "+deliveryTestItem+" --claim "+reviewClaim+" --dispatched")
	cli.dispatchRun(t, "skl watchdog release --repo "+source+" --item "+deliveryTestItem+" --claim "+reviewClaim)
	ordinary = cli.dispatchJSON(t, "skl watchdog next --repo "+source)
	if ordinary.Packet == nil {
		t.Fatalf("ordinary watchdog next: %s", mustJSON(t, ordinary))
	}
	sameRendering(t, dispatchedReview, ordinary.Packet.Instructions)
	cli.dispatchRun(t, "skl watchdog release --repo "+source+" --item "+deliveryTestItem+" --claim "+ordinary.Execution.Claim.Commit)
	dispatchReview(t, cli, source, "rework")

	// Rework: the Rework Procedure.
	reworked := cli.dispatched(t, "skl implement next --dispatch --repo "+source)
	dispatchedRework := cli.dispatchRun(t, reworked.Worker)
	cli.dispatchRun(t, "skl implement release --repo "+source+" --item "+deliveryTestItem+" --claim "+reworked.Claim)
	ordinary = cli.dispatchJSON(t, "skl implement next --repo "+source)
	if ordinary.Packet == nil || ordinary.Packet.Facts.Delivery.Procedure != "rework" {
		t.Fatalf("ordinary rework next: %s", mustJSON(t, ordinary))
	}
	sameRendering(t, dispatchedRework, ordinary.Packet.Instructions)
}

// continued runs a continuation and returns how it reports the previous
// Claim's ending, with its status.
func (c ledgerCLI) continued(t *testing.T, command string) (string, string) {
	t.Helper()
	out := c.dispatchJSON(t, command)
	if out.Previous == nil {
		t.Fatalf("%s reported no previous Claim: %s", command, mustJSON(t, out))
	}
	return out.Previous.Ending, out.Status
}

// TestDispatchContinuationFollowsTheNamedClaim covers B4 and B5: only the
// named Claim's own ending decides, whatever the Slice's current state.
func TestDispatchContinuationFollowsTheNamedClaim(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	cli := deliveryNoForgeApp(t)

	// Watchdog advances the Slice to Ready for Merge before the Implement
	// Supervisor continues after C1.
	c1 := cli.dispatched(t, "skl implement next --dispatch --repo "+source)
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, c1.Claim)
	dispatchReview(t, cli, source, "pass")
	if state := deliveryPersistedState(t, fixture.clone); state.State != ledger.ReadyForMerge {
		t.Fatalf("review did not advance the Slice: %#v", state)
	}
	spec := singleSlice("delivery-export")
	spec.slices[0].branch = "export"
	if accepted := newLedgerApp(t, forge).accept(t, source, writeProposal(t, "", spec)); accepted.Status != "accepted" {
		t.Fatalf("accept second proposal: %s", mustJSON(t, accepted))
	}
	next := cli.dispatchJSON(t, c1.Continue)
	if next.Previous == nil || *next.Previous != (ledger.ClaimEnding{Item: deliveryTestItem, Claim: c1.Claim, Ending: ledger.AwaitingReview}) {
		t.Fatalf("continuation did not report C1's submission: %s", mustJSON(t, next))
	}
	if next.Status != "dispatched" || next.Dispatch.Item != "delivery-export/foundation" {
		t.Fatalf("continuation did not dispatch the next eligible Slice: %s", mustJSON(t, next))
	}
	exportClaim := next.Dispatch.Claim

	// The export worker crashed with its Claim held.
	before := ledgerSnapshot(t, fixture.clone)
	held := cli.dispatchRun(t, "skl implement next --dispatch --repo "+source+" --after "+exportClaim+" --wait 1h")
	if !strings.Contains(held, "Status: stopped") {
		t.Fatalf("held Claim did not stop the Supervisor:\n%s", held)
	}
	for _, command := range []string{"resume", "release"} {
		bound := regexp.MustCompile("`skl implement " + command + " --repo '[^']+' --remote 'origin' --item 'delivery-export/foundation' --claim '" + exportClaim + "'`")
		if !bound.MatchString(held) {
			t.Errorf("stop outcome lacks %s:\n%s", bound, held)
		}
	}
	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("a stopped continuation changed the ledger")
	}

	// A human releases it mid-run; the Slice is eligible again.
	cli.dispatchRun(t, "skl implement release --repo "+source+" --item delivery-export/foundation --claim "+exportClaim)
	before = ledgerSnapshot(t, fixture.clone)
	if ending, status := cli.continued(t, "skl implement next --dispatch --repo "+source+" --after "+exportClaim); ending != ledger.ClaimReleased || status != "stopped" {
		t.Fatalf("released Claim continued: %s %s", ending, status)
	}
	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("a stopped continuation claimed work")
	}
}

// TestDispatchContinuesAfterEveryHandoff covers B4's handoff endings of both
// phases: Implement's pause, and Watchdog's rework and pass reviews.
func TestDispatchContinuesAfterEveryHandoff(t *testing.T) {
	newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)

	paused := cli.dispatched(t, "skl implement next --dispatch --repo "+source)
	out := cli.dispatchJSON(t, "skl implement needs-human --repo "+source+" --item "+deliveryTestItem+" --claim "+paused.Claim+" --body "+proseFixturePath("pause.md"))
	if out.Status != ledger.NeedsHuman {
		t.Fatalf("pause: %s", mustJSON(t, out))
	}
	markers := map[string]string{}
	continueAfter := func(d *dispatched, want string) {
		t.Helper()
		if ending, status := cli.continued(t, d.Continue); ending != want || status != ledger.NoWork {
			t.Fatalf("continue after %s = %s %s, want %s then no work", d.Claim, ending, status, want)
		}
		markers[want] = cli.dispatchRun(t, strings.Replace(d.Continue, " --format 'json'", "", 1))
	}
	continueAfter(paused, ledger.NeedsHuman)
	request := decisionRequest(t, cli, "widgets", deliveryTestItem)
	decisionApplyAnswer(t, cli, request, ledger.RouteImplement, "# Human direction\n\nContinue.\n")

	directed := cli.dispatched(t, "skl implement next --dispatch --repo "+source)
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, directed.Claim)
	reworked := cli.dispatched(t, "skl watchdog next --dispatch --repo "+source)
	dispatchVerdict(t, cli, source, reworked.Claim, "rework")
	continueAfter(reworked, ledger.Rework)

	fixed := cli.dispatched(t, "skl implement next --dispatch --repo "+source)
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, fixed.Claim)
	passed := cli.dispatched(t, "skl watchdog next --dispatch --repo "+source)
	dispatchVerdict(t, cli, source, passed.Claim, "pass")
	continueAfter(passed, "pass")

	// The golden journey reaches only the submission's ending sentence.
	for ending, marker := range map[string]string{ledger.NeedsHuman: "paused for a human decision", ledger.Rework: "returned for rework", "pass": "passed review"} {
		if !strings.Contains(markers[ending], marker) {
			t.Errorf("continuation after %s lacks %q:\n%s", ending, marker, markers[ending])
		}
	}
}

// TestDispatchEarlierHandoffCannotAuthorizeALaterRound covers B4's later
// round: C1's handoff says nothing about C2 on the same Slice.
func TestDispatchEarlierHandoffCannotAuthorizeALaterRound(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)

	c1 := cli.dispatched(t, "skl implement next --dispatch --repo "+source)
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, c1.Claim)
	dispatchReview(t, cli, source, "rework")
	c2 := cli.dispatched(t, c1.Continue)
	if c2.Item != deliveryTestItem || c2.Claim == c1.Claim {
		t.Fatalf("the Rework round was not dispatched: %#v", c2)
	}

	before := ledgerSnapshot(t, fixture.clone)
	if ending, status := cli.continued(t, c2.Continue); ending != ledger.ClaimHeld || status != "stopped" {
		t.Fatalf("C2 still held continued: %s %s", ending, status)
	}
	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("a stopped continuation changed the ledger")
	}
}

// TestDispatchRefusesInvalidContinuationBeforeAnyEffect covers B6: each
// refusal stops the Supervisor at once, without waiting or claiming.
func TestDispatchRefusesInvalidContinuationBeforeAnyEffect(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	cli := deliveryNoForgeApp(t)

	// A watchdog Claim, and a Claim of another Project.
	implemented := cli.dispatched(t, "skl implement next --dispatch --repo "+source)
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, implemented.Claim)
	watchdogClaim := cli.dispatched(t, "skl watchdog next --dispatch --repo "+source).Claim
	gadgets := sourceRepository(t, "acme", "gadgets")
	if accepted := newLedgerApp(t, forge).accept(t, gadgets, writeProposal(t, "", singleSlice("gadget"))); accepted.Status != "accepted" {
		t.Fatalf("accept gadgets: %s", mustJSON(t, accepted))
	}
	gadgetClaim := cli.dispatched(t, "skl implement next --dispatch --repo "+gadgets).Claim
	// A fresh eligible Slice would be claimed by any continuation that passed.
	spec := singleSlice("delivery-export")
	spec.slices[0].branch = "export"
	if accepted := newLedgerApp(t, forge).accept(t, source, writeProposal(t, "", spec)); accepted.Status != "accepted" {
		t.Fatalf("accept second proposal: %s", mustJSON(t, accepted))
	}
	acceptance := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")

	for name, args := range map[string][]string{
		"missing":       {"--dispatch", "--after"},
		"empty":         {"--dispatch", "--after", ""},
		"malformed":     {"--dispatch", "--after", "not-a-claim"},
		"unknown":       {"--dispatch", "--after", strings.Repeat("a", 40)},
		"not a Claim":   {"--dispatch", "--after", acceptance},
		"other phase":   {"--dispatch", "--after", watchdogClaim},
		"other Project": {"--dispatch", "--after", gadgetClaim},
	} {
		t.Run(name, func(t *testing.T) {
			before := ledgerSnapshot(t, fixture.clone)
			start := time.Now()
			output, err := cli.deliveryRun(t, append([]string{"skl", "implement", "next", "--repo", source, "--wait", "1h"}, args...)...)
			if err != nil {
				t.Fatalf("refusal failed: %v\n%s", err, output)
			}
			// The stop variant binds no rerun of the refused Dispatch.
			if !strings.Contains(output, "Status: fix_required") || strings.Contains(output, "skl implement next") {
				t.Fatalf("refusal did not stop the Supervisor:\n%s", output)
			}
			if time.Since(start) > 30*time.Second {
				t.Fatal("refusal waited")
			}
			if after := ledgerSnapshot(t, fixture.clone); after != before {
				t.Fatal("refusal changed the ledger")
			}
		})
	}
}

// TestDispatchOptionsRequireDispatch keeps worker values and continuation
// references off ordinary next, before any effect.
func TestDispatchOptionsRequireDispatch(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)
	for _, args := range []string{" --after " + strings.Repeat("a", 40), " --worker-model m", " --worker-thinking high"} {
		before := ledgerSnapshot(t, fixture.clone)
		if out := cli.dispatchJSON(t, "skl implement next --repo "+source+args); out.Status != "fix_required" {
			t.Fatalf("%s accepted without --dispatch: %s", args, mustJSON(t, out))
		}
		if after := ledgerSnapshot(t, fixture.clone); after != before {
			t.Fatalf("%s changed the ledger", args)
		}
	}
}

// TestDispatchWaitingKeepsTheWaitContract covers B7: an empty idle window
// stops the lane as queue-local inactivity, and an interrupted wait names
// what to inspect.
func TestDispatchWaitingKeepsTheWaitContract(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)

	before := ledgerSnapshot(t, fixture.clone)
	idle := cli.dispatchJSON(t, "skl watchdog next --dispatch --repo "+source+" --wait 20ms --poll 5ms")
	if idle.Status != ledger.IdleTimeout || !strings.Contains(idle.Reason, "not global completion") {
		t.Fatalf("idle window = %s", mustJSON(t, idle))
	}
	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("idle timeout wrote to the ledger")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cli.out.Reset()
	err := cli.app.RunContext(ctx, []string{"skl", "watchdog", "next", "--dispatch", "--repo", source, "--wait", "1m"})
	if err == nil || !strings.Contains(cli.out.String(), "Status: interrupted") || !strings.Contains(cli.out.String(), "`skl status --repo '") {
		t.Fatalf("interrupted dispatch = %v:\n%s", err, cli.out)
	}
}
