package main

// Public-CLI coverage of Auto Mode Watchdog merges into an open Proposal
// Branch (auto-watchdog-merge B1-B6). The source remote is a routed local bare
// repository and the pull request server squash-merges into it for real, so
// the landed tree and the observed completion are checked against Git.

import (
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

const autoMergeBranch = "proposal/" + deliveryTestProposal

type autoMergeFixture struct {
	clone, source, bare string
	pulls               *pullServer
	cli                 ledgerCLI
}

// newAutoMergeFixture accepts one Slice and implements it, opening the
// Proposal Branch when autoImplement is set; otherwise the Slice targets main.
func newAutoMergeFixture(t *testing.T, autoImplement bool) *autoMergeFixture {
	t.Helper()
	ledgerFixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	bare := routeSourcePushes(t)
	runGit(t, source, "push", "-q", "origin", "main")
	deliveryAcceptFixture(t, newForgeServer(t), source)
	pulls := newPullServer(t, bare)
	f := &autoMergeFixture{clone: ledgerFixture.clone, source: source, bare: bare, pulls: pulls, cli: presentForgeApp(t, pulls)}
	next := []string{"skl", "implement", "next", "--repo", source, "--format", "json"}
	if autoImplement {
		next = append(next, "--auto")
	}
	started, err := f.cli.deliveryJSON(t, next...)
	if err != nil || started.Execution == nil {
		t.Fatalf("implement next = %#v, %v", started, err)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := f.cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("implement prepare = %#v, %v", prepared, err)
	}
	head := proseCommit(t, prepared.Source.Worktree, "merge.txt", "merged work\n")
	submitted, err := f.cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", head, "--target", prepared.Source.Target, "--body", proseFixturePath("implement-report.md"), "--public-body", proseFixturePath("public.md"), "--format", "json")
	if err != nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit = %#v, %v", submitted, err)
	}
	return f
}

// pass claims the review, in Auto Mode when auto is set, and submits a pass,
// with public prose when public is set.
func (f *autoMergeFixture) pass(t *testing.T, auto, public bool) deliveryOutput {
	t.Helper()
	next := []string{"skl", "watchdog", "next", "--repo", f.source, "--format", "json"}
	if auto {
		next = append(next, "--auto")
	}
	started, err := f.cli.deliveryJSON(t, next...)
	if err != nil || started.Execution == nil {
		t.Fatalf("watchdog next = %#v, %v", started, err)
	}
	return f.submitPass(t, started.Execution.Claim.Commit, public)
}

func (f *autoMergeFixture) submitPass(t *testing.T, claim string, public bool) deliveryOutput {
	t.Helper()
	submit := []string{"skl", "watchdog", "submit", "--repo", f.source, "--item", deliveryTestItem, "--claim", claim,
		"--outcome", "pass", "--body", proseFixturePath("watchdog-report.md"), "--format", "json"}
	if public {
		submit = append(submit, "--public-body", proseFixturePath("public.md"))
	}
	passed, err := f.cli.deliveryJSON(t, submit...)
	if err != nil || passed.Status != ledger.ReadyForMerge {
		t.Fatalf("watchdog submit pass = %#v, %v", passed, err)
	}
	return passed
}

func (f *autoMergeFixture) branchHead(t *testing.T, branch string) string {
	t.Helper()
	return deliveryTrimmed(t, f.bare, "rev-parse", "refs/heads/"+branch)
}

// mergeRequests counts the merge requests the forge received.
func (f *autoMergeFixture) mergeRequests() int {
	f.pulls.mu.Lock()
	defer f.pulls.mu.Unlock()
	count := 0
	for _, request := range f.pulls.requests {
		if strings.HasPrefix(request, "PUT ") && strings.HasSuffix(request, "/merge") {
			count++
		}
	}
	return count
}

func (f *autoMergeFixture) draft(number int) any {
	f.pulls.mu.Lock()
	defer f.pulls.mu.Unlock()
	return f.pulls.pulls[number]["draft"]
}

// observe runs status for the Slice, which observes completions.
func (f *autoMergeFixture) observe(t *testing.T) ledgerStatusOutcome {
	t.Helper()
	observed := runCompletionStatus(t, f.cli.app, f.cli.out, f.source, "--item", deliveryTestItem)
	if len(observed.Items) != 1 {
		t.Fatalf("status = %+v", observed)
	}
	return observed
}

func TestAutoWatchdogPassSquashMergesTheReviewedTreeIntoTheProposalBranch(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	before := f.branchHead(t, autoMergeBranch)
	final := f.branchHead(t, deliveryTestBranch)

	passed := f.pass(t, true, true)
	merge := passed.Merge
	if merge == nil || merge.Status != ledger.MergeMerged || merge.Branch != autoMergeBranch || merge.Commit == "" {
		t.Fatalf("Auto Mode pass merge = %s", mustJSON(t, passed))
	}
	landed := f.branchHead(t, autoMergeBranch)
	if landed != merge.Commit || deliveryTrimmed(t, f.bare, "rev-parse", landed+"^") != before {
		t.Fatalf("Proposal Branch = %s with parent %s, want one squash commit %s on %s", landed, deliveryTrimmed(t, f.bare, "rev-parse", landed+"^"), merge.Commit, before)
	}
	if got, want := deliveryTrimmed(t, f.bare, "rev-parse", landed+"^{tree}"), deliveryTrimmed(t, f.bare, "rev-parse", final+"^{tree}"); got != want {
		t.Fatalf("landed tree %s, want the reviewed tree %s", got, want)
	}
	// The merge writes no Workflow State; Merged comes from observation.
	if state := deliveryPersistedState(t, f.clone); state.State != ledger.ReadyForMerge || state.Completion != nil {
		t.Fatalf("state after merge = %+v", state)
	}
	observed := f.observe(t)
	completion := observed.Items[0].Completion
	if observed.Items[0].State != ledger.Merged || completion == nil || completion.Target.Branch != autoMergeBranch || completion.MergeCommit != merge.Commit {
		t.Fatalf("observed completion = %+v", observed.Items[0])
	}
}

func TestAutoWatchdogPassMarkdownReportsTheMerge(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	started, err := f.cli.deliveryJSON(t, "skl", "watchdog", "next", "--auto", "--repo", f.source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("watchdog next = %#v, %v", started, err)
	}
	markdown, err := f.cli.deliveryRun(t, "skl", "watchdog", "submit", "--repo", f.source, "--item", deliveryTestItem, "--claim", started.Execution.Claim.Commit,
		"--outcome", "pass", "--body", proseFixturePath("watchdog-report.md"), "--public-body", proseFixturePath("public.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "Proposal Branch merge of `" + deliveryTestItem + "`: squash-merged into `" + autoMergeBranch + "` as `" + f.branchHead(t, autoMergeBranch) + "`"
	if !strings.Contains(markdown, want) || strings.Contains(markdown, "ready for a human to merge") {
		t.Fatalf("Markdown pass lacks %q:\n%s", want, markdown)
	}
}

func TestAutoWatchdogRefusedMergeKeepsTheLaneAndRetriesEachPoll(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	// The Proposal Branch moves past the reviewed base after preparation.
	base := f.branchHead(t, autoMergeBranch)
	moved := deliveryTrimmed(t, f.bare, "-c", "user.name=Human", "-c", "user.email=human@example.com", "commit-tree", base+"^{tree}", "-p", base, "-m", "moved")
	runGit(t, f.bare, "update-ref", "refs/heads/"+autoMergeBranch, moved, base)

	dispatch := f.cli.dispatched(t, "skl watchdog next --auto --dispatch --repo "+f.source)
	if slices.Contains(shellWords(t, dispatch.Worker), "--auto") || !slices.Contains(shellWords(t, dispatch.Continue), "--auto") {
		t.Fatalf("Auto Mode reaches the worker through its Claim and stays on the lane: worker %q, continue %q", dispatch.Worker, dispatch.Continue)
	}
	passed := f.submitPass(t, dispatch.Claim, true)
	if passed.Merge == nil || passed.Merge.Status != ledger.MergeRefused || !strings.Contains(passed.Merge.Reason, "not an ancestor") {
		t.Fatalf("pass on a moved branch = %s", mustJSON(t, passed))
	}
	number := passed.Result.State.Submission.Number
	if f.mergeRequests() != 0 || f.draft(number) != false || f.branchHead(t, autoMergeBranch) != moved {
		t.Fatalf("refused merge reached the forge or left the pull request draft")
	}
	ledgerHead := deliveryTrimmed(t, f.clone, "rev-parse", "HEAD")

	continued := f.cli.dispatchJSON(t, dispatch.Continue)
	if continued.Previous == nil || continued.Previous.Ending != "pass" || continued.Status != ledger.NoWork {
		t.Fatalf("continuation after a refused merge = %s", mustJSON(t, continued))
	}
	if len(continued.Merges) != 1 || continued.Merges[0].Item != deliveryTestItem || continued.Merges[0].Status != ledger.MergeRefused || !strings.Contains(continued.Merges[0].Reason, "not an ancestor") {
		t.Fatalf("retry = %+v, want the ancestry refusal reported again", continued.Merges)
	}
	if deliveryTrimmed(t, f.clone, "rev-parse", "HEAD") != ledgerHead || deliveryPersistedState(t, f.clone).State != ledger.ReadyForMerge {
		t.Fatal("a refused retry changed Workflow State")
	}

	// A human merges by hand; completion observation records it.
	f.pulls.mu.Lock()
	squashed := f.pulls.squash(number)
	f.pulls.mu.Unlock()
	if observed := f.observe(t); observed.Items[0].State != ledger.Merged || observed.Items[0].Completion.MergeCommit != squashed {
		t.Fatalf("hand merge observation = %+v", observed.Items[0])
	}
}

func TestAutoPollMergesASlicePassedWithoutTheFlag(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	passed := f.pass(t, false, true)
	if passed.Merge != nil || f.mergeRequests() != 0 {
		t.Fatalf("pass without Auto Mode merged: %s", mustJSON(t, passed))
	}
	markdown, err := f.cli.deliveryRun(t, "skl", "watchdog", "next", "--auto", "--repo", f.source)
	if err != nil {
		t.Fatal(err)
	}
	landed := f.branchHead(t, autoMergeBranch)
	if want := "Proposal Branch merge of `" + deliveryTestItem + "`: squash-merged into `" + autoMergeBranch + "` as `" + landed + "`"; !strings.Contains(markdown, want) {
		t.Fatalf("poll lacks %q:\n%s", want, markdown)
	}
	// The same poll observes the merge before selecting.
	if state := deliveryPersistedState(t, f.clone); state.State != ledger.Merged || state.Completion == nil || state.Completion.MergeCommit != landed {
		t.Fatalf("state after the retried merge = %+v", state)
	}
}

func TestAutoPassRefusalsAreReportedWithoutFailingTheHandoff(t *testing.T) {
	t.Run("forge refusal", func(t *testing.T) {
		f := newAutoMergeFixture(t, true)
		f.pulls.mergeRefusal = "Repository rule violations found: required status check"
		passed := f.pass(t, true, true)
		if passed.Merge == nil || passed.Merge.Status != ledger.MergeRefused || !strings.Contains(passed.Merge.Reason, f.pulls.mergeRefusal) {
			t.Fatalf("forge refusal = %s", mustJSON(t, passed))
		}
		if f.draft(passed.Result.State.Submission.Number) != false || deliveryPersistedState(t, f.clone).State != ledger.ReadyForMerge {
			t.Fatal("a forge refusal left the Slice other than Ready for Merge with a ready pull request")
		}
	})
	t.Run("presentation pending", func(t *testing.T) {
		f := newAutoMergeFixture(t, true)
		passed := f.pass(t, true, false)
		if passed.Merge == nil || passed.Merge.Status != ledger.MergeRefused || !strings.Contains(passed.Merge.Reason, "does not present the final head") || f.mergeRequests() != 0 {
			t.Fatalf("pending presentation = %s", mustJSON(t, passed))
		}
	})
	t.Run("main target", func(t *testing.T) {
		f := newAutoMergeFixture(t, false)
		passed := f.pass(t, true, true)
		if passed.Merge != nil || f.mergeRequests() != 0 {
			t.Fatalf("Auto Mode merged a main-targeted Slice: %s", mustJSON(t, passed))
		}
		polled, err := f.cli.deliveryJSON(t, "skl", "watchdog", "next", "--auto", "--repo", f.source, "--format", "json")
		if err != nil || len(polled.Merges) != 0 || f.mergeRequests() != 0 {
			t.Fatalf("Auto Mode poll merged a main-targeted Slice: %s, %v", mustJSON(t, polled), err)
		}
	})
}
