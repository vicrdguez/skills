package main

// Dispatch coverage at the CLI seam against real ledger and source fixtures.
// Expected outcomes come from the dispatch-verified-work behavior (B1-B7):
// commands are parsed from the Supervisor's output and run as a Supervisor
// would run them.

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vicrdguez/skills/ledger"
)

var (
	dispatchWorker   = regexp.MustCompile("run `(skl [a-z]+ resume [^`]+)` and follow its output")
	dispatchContinue = regexp.MustCompile("When the subagent returns, run:\n\n`([^`]+)`")
	dispatchClaim    = regexp.MustCompile("is claimed for [A-Za-z ]+ with Claim `([0-9a-f]{40})`")
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

// dispatchCommands returns the worker and continue commands of a dispatch.
func dispatchCommands(t *testing.T, output string) (worker, continuation string) {
	t.Helper()
	return proseMatch(t, dispatchWorker, output), proseMatch(t, dispatchContinue, output)
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
	reviewed, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", started.Execution.Item, "--claim", started.Execution.Claim.Commit,
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
	dispatch := cli.dispatchRun(t, "skl implement next --dispatch --repo "+source+" --remote origin --capability sequential --worker-model openai-codex/gpt-6-astra --worker-thinking high")
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
	for _, want := range []string{"model `openai-codex/gpt-6-astra`", "`high`", "Claim `" + claim + "`"} {
		if !strings.Contains(dispatch, want) {
			t.Errorf("dispatch output lacks %q:\n%s", want, dispatch)
		}
	}
	worker, continuation := dispatchCommands(t, dispatch)
	wantWorker := "skl implement resume --repo '" + root + "' --remote 'origin' --item '" + deliveryTestItem + "' --claim '" + claim + "' --dispatched --capability 'sequential'"
	if worker != wantWorker {
		t.Errorf("worker command = %s, want %s", worker, wantWorker)
	}
	wantContinue := "skl implement next --repo '" + root + "' --remote 'origin' --capability 'sequential' --dispatch --after '" + claim + "' --worker-model 'openai-codex/gpt-6-astra' --worker-thinking 'high'"
	if continuation != wantContinue {
		t.Errorf("continue command = %s, want %s", continuation, wantContinue)
	}

	// B3: the worker receives the initial Procedure next would have returned.
	dispatchedSkill := cli.dispatchRun(t, worker)
	if !strings.Contains(dispatchedSkill, "(initial)") {
		t.Fatalf("worker did not receive the initial Procedure:\n%s", dispatchedSkill)
	}
	cli.dispatchRun(t, "skl implement release --repo "+source+" --item "+deliveryTestItem+" --claim "+claim)
	ordinary, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--capability", "sequential", "--format", "json")
	if err != nil || ordinary.Packet == nil {
		t.Fatalf("ordinary next: %#v %v", ordinary, err)
	}
	sameRendering(t, dispatchedSkill, ordinary.Packet.Instructions)
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, ordinary.Execution.Claim.Commit)

	// Watchdog: the ordinary review Procedure, with omitted worker values
	// rendering no sentence.
	review := cli.dispatchRun(t, "skl watchdog next --dispatch --repo "+source)
	if strings.Contains(review, "model") || strings.Contains(review, "thinking") {
		t.Errorf("omitted worker values rendered a sentence:\n%s", review)
	}
	worker, _ = dispatchCommands(t, review)
	reviewClaim := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	dispatchedReview := cli.dispatchRun(t, worker)
	cli.dispatchRun(t, "skl watchdog release --repo "+source+" --item "+deliveryTestItem+" --claim "+reviewClaim)
	ordinary, err = cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil || ordinary.Packet == nil {
		t.Fatalf("ordinary watchdog next: %#v %v", ordinary, err)
	}
	sameRendering(t, dispatchedReview, ordinary.Packet.Instructions)
	cli.dispatchRun(t, "skl watchdog release --repo "+source+" --item "+deliveryTestItem+" --claim "+ordinary.Execution.Claim.Commit)
	dispatchReview(t, cli, source, "rework")

	// Rework: the Rework Procedure, and the JSON transport carries the
	// dispatch fields without the Execution Skill.
	reworked, err := cli.deliveryJSON(t, "skl", "implement", "next", "--dispatch", "--repo", source, "--format", "json")
	if err != nil || reworked.Status != "dispatched" || reworked.Dispatch == nil || reworked.Packet != nil || reworked.Execution != nil {
		t.Fatalf("JSON dispatch = %#v %v", reworked, err)
	}
	dispatchedRework := cli.dispatchRun(t, reworked.Dispatch.Worker)
	if !strings.Contains(dispatchedRework, "(rework)") {
		t.Fatalf("worker did not receive the Rework Procedure:\n%s", dispatchedRework)
	}
	cli.dispatchRun(t, "skl implement release --repo "+source+" --item "+deliveryTestItem+" --claim "+reworked.Dispatch.Claim)
	ordinary, err = cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || ordinary.Packet == nil {
		t.Fatalf("ordinary rework next: %#v %v", ordinary, err)
	}
	sameRendering(t, dispatchedRework, ordinary.Packet.Instructions)
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
	_, continueC1 := dispatchCommands(t, cli.dispatchRun(t, "skl implement next --dispatch --repo "+source))
	c1 := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, c1)
	dispatchReview(t, cli, source, "pass")
	if state := deliveryPersistedState(t, fixture.clone); state.State != ledger.ReadyForMerge {
		t.Fatalf("review did not advance the Slice: %#v", state)
	}
	spec := singleSlice("delivery-export")
	spec.slices[0].branch = "export"
	if accepted := newLedgerApp(t, forge).accept(t, source, writeProposal(t, "", spec)); accepted.Status != "accepted" {
		t.Fatalf("accept second proposal: %s", mustJSON(t, accepted))
	}
	next := cli.dispatchRun(t, continueC1)
	if !strings.Contains(next, "Claim `"+c1+"` on Work Item `"+deliveryTestItem+"` was submitted for review") {
		t.Fatalf("continuation did not report C1's handoff:\n%s", next)
	}
	if !strings.Contains(next, "Status: dispatched") || !strings.Contains(next, "delivery-export/foundation") {
		t.Fatalf("continuation did not dispatch the next eligible Slice:\n%s", next)
	}
	exportClaim := proseMatch(t, dispatchClaim, next)

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

	// A human releases it mid-run.
	cli.dispatchRun(t, "skl implement release --repo "+source+" --item delivery-export/foundation --claim "+exportClaim)
	before = ledgerSnapshot(t, fixture.clone)
	released := cli.dispatchRun(t, "skl implement next --dispatch --repo "+source+" --after "+exportClaim)
	if !strings.Contains(released, "Status: stopped") || !strings.Contains(released, "released before any phase handoff") || strings.Contains(released, "skl implement resume") {
		t.Fatalf("released Claim did not stop the Supervisor:\n%s", released)
	}
	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("a stopped continuation claimed work")
	}
}

// TestDispatchEarlierHandoffCannotAuthorizeALaterRound covers B4's later
// round: C1's handoff says nothing about C2 on the same Slice.
func TestDispatchEarlierHandoffCannotAuthorizeALaterRound(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)

	cli.dispatchRun(t, "skl implement next --dispatch --repo "+source)
	c1 := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, c1)
	dispatchReview(t, cli, source, "rework")
	cli.dispatchRun(t, "skl implement next --dispatch --repo "+source+" --after "+c1)
	c2 := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	if c2 == c1 || deliveryPersistedState(t, fixture.clone).Claim == nil {
		t.Fatal("the Rework round was not dispatched")
	}

	before := ledgerSnapshot(t, fixture.clone)
	stopped := cli.dispatchRun(t, "skl implement next --dispatch --repo "+source+" --after "+c2)
	if !strings.Contains(stopped, "Status: stopped") || !strings.Contains(stopped, c2) {
		t.Fatalf("C2 still held did not stop the Supervisor:\n%s", stopped)
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

	// A handed-off watchdog Claim, and a Claim of another Project.
	implementClaim := proseMatch(t, dispatchClaim, cli.dispatchRun(t, "skl implement next --dispatch --repo "+source))
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, implementClaim)
	cli.dispatchRun(t, "skl watchdog next --dispatch --repo "+source)
	watchdogClaim := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	gadgets := sourceRepository(t, "acme", "gadgets")
	if accepted := newLedgerApp(t, forge).accept(t, gadgets, writeProposal(t, "", singleSlice("gadget"))); accepted.Status != "accepted" {
		t.Fatalf("accept gadgets: %s", mustJSON(t, accepted))
	}
	cli.dispatchRun(t, "skl implement next --dispatch --repo "+gadgets)
	gadgetClaim := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	// A fresh eligible Slice would be claimed by any continuation that passed.
	spec := singleSlice("delivery-export")
	spec.slices[0].branch = "export"
	if accepted := newLedgerApp(t, forge).accept(t, source, writeProposal(t, "", spec)); accepted.Status != "accepted" {
		t.Fatalf("accept second proposal: %s", mustJSON(t, accepted))
	}
	acceptance := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")

	for name, args := range map[string][]string{
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
			if !strings.Contains(output, "Status: fix_required") || !strings.Contains(output, "and stop.") || strings.Contains(output, "rerun") {
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
	for _, args := range [][]string{{"--after", strings.Repeat("a", 40)}, {"--worker-model", "m"}, {"--worker-thinking", "high"}} {
		before := ledgerSnapshot(t, fixture.clone)
		output, err := cli.deliveryRun(t, append([]string{"skl", "implement", "next", "--repo", source}, args...)...)
		if err != nil || !strings.Contains(output, "Status: fix_required") || !strings.Contains(output, "--dispatch") {
			t.Fatalf("%v accepted without --dispatch: %v\n%s", args, err, output)
		}
		if after := ledgerSnapshot(t, fixture.clone); after != before {
			t.Fatalf("%v changed the ledger", args)
		}
	}
}

// TestDispatchWaitingKeepsTheWaitContract covers B7: an empty idle window
// stops the lane, and an interrupted wait names what to inspect.
func TestDispatchWaitingKeepsTheWaitContract(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)

	before := ledgerSnapshot(t, fixture.clone)
	idle := cli.dispatchRun(t, "skl watchdog next --dispatch --repo "+source+" --wait 20ms --poll 5ms")
	if !strings.Contains(idle, "Status: idle_timeout") || !strings.Contains(idle, "and stop.") {
		t.Fatalf("idle window did not stop the Supervisor:\n%s", idle)
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
