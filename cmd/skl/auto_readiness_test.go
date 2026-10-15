package main

import (
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// A new CLI/adapter instance gets no earlier body or attempt state.
func (f *autoMergeFixture) pollReadiness(t *testing.T) deliveryOutput {
	t.Helper()
	fresh := presentForgeApp(t, f.pulls)
	out, err := fresh.deliveryJSON(t, "skl", "watchdog", "next", "--auto", "--repo", f.source, "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAutoReadinessFreshPollRecoversDraftWithoutRepublishingProse(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	passed := f.pass(t, false, false)
	number := passed.Result.State.Submission.Number
	f.pulls.mu.Lock()
	f.pulls.pulls[number]["body"] = "Human-edited earlier implementation account"
	f.pulls.pulls[number]["title"] = "Human title"
	f.pulls.requests = nil
	f.pulls.mu.Unlock()

	polled := f.pollReadiness(t)
	if len(polled.Merges) != 1 || polled.Merges[0].Status != ledger.MergeMerged || polled.Merges[0].Readiness == nil || polled.Merges[0].Readiness.Status != "ready" || polled.Merges[0].Review != "pass" {
		t.Fatalf("recovered poll = %s", mustJSON(t, polled))
	}
	f.pulls.mu.Lock()
	defer f.pulls.mu.Unlock()
	if f.pulls.pulls[number]["body"] != "Human-edited earlier implementation account" || f.pulls.pulls[number]["title"] != "Human title" {
		t.Fatal("readiness recovery changed public prose")
	}
	for _, request := range f.pulls.requests {
		if strings.HasPrefix(request, "PATCH /repos/acme/widgets/pulls/21") {
			t.Fatalf("recovery published prose or created a PR: %v", f.pulls.requests)
		}
	}
	sources := 0
	for _, pull := range f.pulls.pulls {
		if pull["branch"] == deliveryTestBranch {
			sources++
		}
	}
	if sources != 1 {
		t.Fatalf("source PRs = %d, want only the established attachment", sources)
	}
	state := deliveryPersistedState(t, f.clone)
	if state.State != ledger.Merged || state.Claim != nil {
		t.Fatalf("recovery did not observe Merged without a worker Claim: %+v", state)
	}
	report, _ := deliveryCommittedReport(t, f.cli, passed.Result.Report, ledger.WatchdogPhase)
	if report.Round != 1 || report.Outcome != "pass" {
		t.Fatalf("recovery altered review: %+v", report)
	}
}

func TestAutoReadinessMissingAssociationRequiresExplicitRecovery(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	f.pass(t, false, false)
	state := deliveryPersistedState(t, f.clone)
	state.Submission = nil
	deliveryCommitState(t, f.clone, state)
	f.pulls.mu.Lock()
	f.pulls.requests = nil
	f.pulls.mu.Unlock()
	polled := f.pollReadiness(t)
	if len(polled.Merges) != 1 || !strings.Contains(polled.Merges[0].Reason, "explicit PR recovery") || polled.Merges[0].Status != ledger.MergeRefused {
		t.Fatalf("missing association = %s", mustJSON(t, polled))
	}
	f.pulls.mu.Lock()
	defer f.pulls.mu.Unlock()
	if len(f.pulls.requests) != 0 {
		t.Fatalf("missing association attempted discovery/creation: %v", f.pulls.requests)
	}
}

func TestAutoReadinessUncertainReadbackWaitsForALaterFreshPoll(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	passed := f.pass(t, false, false)
	f.pulls.mu.Lock()
	f.pulls.failAfterReady = true
	f.pulls.mu.Unlock()
	first := f.pollReadiness(t)
	if len(first.Merges) != 1 || first.Merges[0].Readiness == nil || first.Merges[0].Readiness.Status != ledger.IssueUncertain || first.Merges[0].Status != ledger.MergeRefused || f.mergeRequests() != 0 {
		t.Fatalf("unconfirmed readiness = %s", mustJSON(t, first))
	}
	if f.draft(passed.Result.State.Submission.Number) != false || deliveryPersistedState(t, f.clone).State != ledger.ReadyForMerge {
		t.Fatal("uncertainty erased local PASS or did not exercise an applied mutation")
	}
	f.pulls.mu.Lock()
	f.pulls.failAfterReady = false
	f.pulls.mu.Unlock()
	second := f.pollReadiness(t)
	if len(second.Merges) != 1 || second.Merges[0].Status != ledger.MergeMerged || second.Merges[0].Readiness.Status != "ready" {
		t.Fatalf("later poll = %s", mustJSON(t, second))
	}
}

func TestAutoReadinessRefusesRealSourceMovementDespiteExactAPIHead(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	passed := f.pass(t, false, false)
	approved := f.branchHead(t, deliveryTestBranch)
	moved := deliveryTrimmed(t, f.bare, "-c", "user.name=Human", "-c", "user.email=human@example.com", "commit-tree", approved+"^{tree}", "-p", approved, "-m", "external move")
	runGit(t, f.bare, "update-ref", "refs/heads/"+deliveryTestBranch, moved, approved)
	f.pulls.mu.Lock()
	f.pulls.headOverride = approved
	f.pulls.mu.Unlock()
	polled := f.pollReadiness(t)
	if len(polled.Merges) != 1 || polled.Merges[0].Status != ledger.MergeRefused || !strings.Contains(polled.Merges[0].Reason, "remote source branch") {
		t.Fatalf("moved source recovery = %s", mustJSON(t, polled))
	}
	if f.mergeRequests() != 0 || f.draft(passed.Result.State.Submission.Number) != true || f.branchHead(t, deliveryTestBranch) != moved {
		t.Fatal("recovery promoted or repaired externally moved source")
	}
}

func TestAutoReadinessUsesFinalIntegrationHeadNotReviewedImplementation(t *testing.T) {
	f := newAutoMergeFixture(t, true)
	implementation := f.branchHead(t, deliveryTestBranch)
	base := f.branchHead(t, autoMergeBranch)
	moved := deliveryTrimmed(t, f.bare, "-c", "user.name=Sibling", "-c", "user.email=sibling@example.com", "commit-tree", base+"^{tree}", "-p", base, "-m", "sibling")
	runGit(t, f.bare, "update-ref", "refs/heads/"+autoMergeBranch, moved, base)
	started, err := f.cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", f.source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("claim review: %+v, %v", started, err)
	}
	prepared, err := f.cli.deliveryJSON(t, "skl", "watchdog", "prepare", "--repo", f.source, "--item", deliveryTestItem, "--claim", started.Execution.Claim.Commit, "--format", "json")
	if err != nil || prepared.Source == nil || prepared.Source.Head == implementation {
		t.Fatalf("integration: %+v, %v", prepared, err)
	}
	final := prepared.Source.Head
	// Source publication completed, but the worker's presentation is absent.
	runGit(t, prepared.Source.Worktree, "push", "-q", "origin", final+":refs/heads/"+deliveryTestBranch)
	passed, err := f.cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", f.source, "--item", deliveryTestItem, "--claim", started.Execution.Claim.Commit,
		"--outcome", "pass", "--head", final, "--body", proseFixturePath("watchdog-report.md"), "--format", "json")
	if err != nil || passed.Status != ledger.ReadyForMerge {
		t.Fatalf("submit = %+v, %v", passed, err)
	}
	report, _ := deliveryCommittedReport(t, f.cli, passed.Result.Report, ledger.WatchdogPhase)
	if report.Source.Reviewed != implementation || report.Source.Head != final {
		t.Fatalf("PASS source = %+v", report.Source)
	}
	polled := f.pollReadiness(t)
	if len(polled.Merges) != 1 || polled.Merges[0].Status != ledger.MergeMerged {
		t.Fatalf("final head recovery: %s", mustJSON(t, polled))
	}
	if landed := f.branchHead(t, autoMergeBranch); deliveryTrimmed(t, f.bare, "rev-parse", landed+"^{tree}") != deliveryTrimmed(t, f.bare, "rev-parse", final+"^{tree}") {
		t.Fatal("recovery did not merge the final PASS tree")
	}
}
