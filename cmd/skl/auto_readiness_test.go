package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// A new CLI/adapter instance gets no earlier body or attempt state.
func (f *autoMergeFixture) pollReadiness(t *testing.T) deliveryOutput {
	t.Helper()
	fresh := f.ledger.app(presentForgeApp(t, f.pulls))
	out, err := fresh.deliveryJSON(t, "skl", "watchdog", "next", "--auto", "--repo", f.source, "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// newReadinessFixture arranges real committed source/report history at the
// recovery boundary. The Auto Mode lifecycle itself remains in auto_merge_test.go.
func newReadinessFixture(t *testing.T) *autoMergeFixture {
	t.Helper()
	fixture := newPrivateLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	bare := routeSourcePushes(t, source)
	runGit(t, source, "push", "-q", "origin", "main", "main:refs/heads/"+autoMergeBranch)
	fixture.commitReadyProposal(t, singleSlice(deliveryTestProposal))
	path := filepath.Join(fixture.clone, "projects/widgets/proposals", deliveryTestProposal, "proposal.json")
	meta := ledger.ProposalMeta{
		Accepted: "2026-01-01T00:00:00Z", Branch: autoMergeBranch,
		Target: &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: autoMergeBranch},
	}
	writeFile(t, path, mustJSON(t, meta))
	runGit(t, fixture.clone, "add", path)
	runGit(t, fixture.clone, "commit", "-q", "-m", "open readiness Proposal Branch")
	fixture.commitReviewClaim(t, autoMergeBranch)
	fixture.commitLaunchAwaitingReview(t, source, target)
	worktree := filepath.Join(source, ".worktrees", deliveryTestBranch)
	runGit(t, source, "worktree", "add", "-q", worktree, deliveryTestBranch)
	runGit(t, source, "push", "-q", "origin", deliveryTestBranch)
	state := deliveryPersistedState(t, fixture.clone)
	state.Submission = &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 21}
	deliveryCommitState(t, fixture.clone, state)
	pulls := newPullServer(t, bare)
	pulls.pulls[21] = map[string]any{
		"branch": deliveryTestBranch, "base": autoMergeBranch,
		"body": "Earlier implementation account", "title": "Implementation ready for review", "draft": true,
	}
	return &autoMergeFixture{
		ledger: fixture, clone: fixture.clone, source: source, bare: bare,
		pulls: pulls, cli: fixture.app(presentForgeApp(t, pulls)),
	}
}

func TestAutoReadinessFreshPollRecoversDraftWithoutRepublishingProse(t *testing.T) {
	t.Parallel()
	f := newReadinessFixture(t)
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
	t.Parallel()
	f := newReadinessFixture(t)
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
	offline, err := f.ledger.app(deliveryNoForgeApp(t)).deliveryJSON(t, "skl", "watchdog", "next", "--auto", "--repo", f.source, "--format", "json")
	if err != nil || len(offline.Merges) != 1 || !strings.Contains(offline.Merges[0].Reason, "explicit PR recovery") || offline.Merges[0].Readiness == nil || offline.Merges[0].Review != "pass" {
		t.Fatalf("offline missing association = %s, %v", mustJSON(t, offline), err)
	}
	f.pulls.mu.Lock()
	defer f.pulls.mu.Unlock()
	if len(f.pulls.requests) != 0 {
		t.Fatalf("missing association attempted discovery/creation: %v", f.pulls.requests)
	}
}

func TestAutoReadinessUncertainReadbackWaitsForALaterFreshPoll(t *testing.T) {
	t.Parallel()
	f := newReadinessFixture(t)
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
	t.Parallel()
	f := newReadinessFixture(t)
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
	t.Parallel()
	f := newReadinessFixture(t)
	implementation := f.branchHead(t, deliveryTestBranch)
	base := f.branchHead(t, autoMergeBranch)
	moved := deliveryTrimmed(t, f.bare, "-c", "user.name=Sibling", "-c", "user.email=sibling@example.com", "commit-tree", base+"^{tree}", "-p", base, "-m", "sibling")
	runGit(t, f.bare, "update-ref", "refs/heads/"+autoMergeBranch, moved, base)
	started, err := f.cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", f.source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("claim review: %+v, %v", started, err)
	}
	prepared, err := f.cli.deliveryJSON(t, "skl", "watchdog", "prepare", "--repo", f.source, "--slice", deliveryTestItem, "--claim", started.Execution.Claim.Commit, "--format", "json")
	if err != nil || prepared.Source == nil || prepared.Source.Head == implementation {
		t.Fatalf("integration: %+v, %v", prepared, err)
	}
	final := prepared.Source.Head
	// Source publication completed, but the worker's presentation is absent.
	runGit(t, prepared.Source.Worktree, "push", "-q", "origin", final+":refs/heads/"+deliveryTestBranch)
	passed, err := f.cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", f.source, "--slice", deliveryTestItem, "--claim", started.Execution.Claim.Commit,
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
