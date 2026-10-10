package main

// Public-CLI coverage of the Integration Merge in watchdog preparation: a
// Slice targeting a Proposal Branch is reviewed at the merge of the branch's
// current head, a conflict ends the review in rework, and a human route back
// to watchdog merges again.

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// conflictSubmit is the submit command a conflict outcome binds.
var conflictSubmit = regexp.MustCompile("Then submit:\n\n`([^`]+)`")

// integrationJourney is one Auto Mode Slice submitted for review against a
// Proposal Branch on a local bare source remote.
type integrationJourney struct {
	cli                      ledgerCLI
	clone, source, worktree  string
	reviewed, target, pusher string
	reportPath, publicPath   string
}

func newIntegrationJourney(t *testing.T) integrationJourney {
	t.Helper()
	fixture := newLedgerFixture(t)
	source := sourceRepository(t, "acme", "widgets")
	bare := routeSourcePushes(t)
	runGit(t, source, "push", "-q", "origin", "main")
	cli := deliveryNoForgeApp(t)
	if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, source, writeProposal(t, "", singleSlice(deliveryTestProposal))); accepted.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, accepted))
	}
	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--auto", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("implement next --auto = %+v, %v", started, err)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("implement prepare = %+v, %v", prepared, err)
	}
	worktree := prepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "feature.txt"), "slice work\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "implement slice")
	j := integrationJourney{cli: cli, clone: fixture.clone, source: source, worktree: worktree,
		reviewed: deliveryTrimmed(t, worktree, "rev-parse", "HEAD"), target: prepared.Source.Target,
		reportPath: filepath.Join(t.TempDir(), "report.md"), publicPath: filepath.Join(t.TempDir(), "public.md")}
	writeFile(t, j.reportPath, "# Report\n\nEvidence.\n")
	writeFile(t, j.publicPath, "# Summary\n")
	submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", j.reviewed, "--target", j.target, "--body", j.reportPath, "--format", "json")
	if err != nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit = %+v, %v", submitted, err)
	}
	j.pusher = filepath.Join(t.TempDir(), "pusher")
	runGit(t, source, "clone", "-q", "-b", "proposal/"+deliveryTestProposal, bare, j.pusher)
	runGit(t, j.pusher, "config", "user.name", "Other")
	runGit(t, j.pusher, "config", "user.email", "other@example.com")
	return j
}

// advance lands another Slice's change on the Proposal Branch.
func (j integrationJourney) advance(t *testing.T, name, contents string) string {
	t.Helper()
	writeFile(t, filepath.Join(j.pusher, name), contents)
	runGit(t, j.pusher, "add", "-A")
	runGit(t, j.pusher, "commit", "-q", "-m", "land "+name)
	runGit(t, j.pusher, "push", "-q", "origin", "proposal/"+deliveryTestProposal)
	return deliveryTrimmed(t, j.pusher, "rev-parse", "HEAD")
}

func (j integrationJourney) review(t *testing.T, operation, claim string, extra ...string) (deliveryOutput, error) {
	t.Helper()
	args := append([]string{"skl", "watchdog", operation, "--repo", j.source, "--remote", "origin", "--item", deliveryTestItem, "--claim", claim, "--format", "json"}, extra...)
	return j.cli.deliveryJSON(t, args...)
}

func (j integrationJourney) claimReview(t *testing.T) string {
	t.Helper()
	started, err := j.cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", j.source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("watchdog next = %+v, %v", started, err)
	}
	return started.Execution.Claim.Commit
}

func TestWatchdogReviewsTheIntegrationMergeOfAMovedProposalBranch(t *testing.T) {
	j := newIntegrationJourney(t)
	moved := j.advance(t, "other.txt", "another slice\n")
	claim := j.claimReview(t)

	prepared, err := j.review(t, "prepare", claim)
	if err != nil || prepared.Status != "prepared" || prepared.Source == nil {
		t.Fatalf("watchdog prepare = %+v, %v", prepared, err)
	}
	merge := prepared.Source.Head
	if parents := strings.Fields(deliveryTrimmed(t, j.worktree, "rev-list", "--parents", "-n", "1", "HEAD")); !slices.Equal(parents, []string{merge, j.reviewed, moved}) {
		t.Fatalf("work branch head parents = %v; want merge %s of %s and %s", parents, merge, j.reviewed, moved)
	}
	if prepared.Source.Merged != moved {
		t.Fatalf("prepared merged head = %q; want %s", prepared.Source.Merged, moved)
	}
	again, err := j.review(t, "prepare", claim)
	if err != nil || again.Source == nil || again.Source.Head != merge {
		t.Fatalf("repeat prepare = %+v, %v; want unchanged head %s", again, err, merge)
	}
	inspected, err := j.review(t, "inspect", claim)
	if err != nil || inspected.Source == nil || inspected.Source.Head != merge || inspected.Packet == nil {
		t.Fatalf("watchdog inspect = %+v, %v; want head %s", inspected, err, merge)
	}
	if facts := inspected.Packet.Facts.Delivery; facts.RequiredHead != j.reviewed || facts.SourceHead != merge || facts.Merged != moved {
		t.Fatalf("inspection facts = reviewed %s, head %s, merged %s", facts.RequiredHead, facts.SourceHead, facts.Merged)
	}

	submitted, err := j.review(t, "submit", claim, "--outcome", "rework", "--head", merge, "--body", j.reportPath)
	if err != nil || submitted.Status != ledger.Rework || submitted.Result == nil {
		t.Fatalf("rework submit at the merged head = %+v, %v", submitted, err)
	}
	report, _ := deliveryCommittedReport(t, j.cli, submitted.Result.Report, ledger.WatchdogPhase)
	if report.Source.Reviewed != j.reviewed || report.Source.Head != merge || report.Source.Target != j.target {
		t.Fatalf("report source = %+v; want reviewed %s, head %s, target %s", report.Source, j.reviewed, merge, j.target)
	}
}

func TestWatchdogIntegrationConflictEndsReviewInRework(t *testing.T) {
	j := newIntegrationJourney(t)
	j.advance(t, "other.txt", "another slice\n")
	claim := j.claimReview(t)
	// An earlier Integration Merge is kept; the conflicting later head is not.
	merged, err := j.review(t, "prepare", claim)
	if err != nil || merged.Source == nil {
		t.Fatalf("first prepare = %+v, %v", merged, err)
	}
	prepared := merged.Source.Head
	j.advance(t, "feature.txt", "conflicting slice\n")

	refused, err := j.review(t, "prepare", claim)
	if err != nil || refused.Status != "fix_required" || !strings.Contains(refused.Reason, "feature.txt") {
		t.Fatalf("conflicting prepare = %+v, %v", refused, err)
	}
	if head := deliveryTrimmed(t, j.worktree, "rev-parse", "HEAD"); head != prepared {
		t.Fatalf("conflict left head %s; want the prepared %s", head, prepared)
	}
	if status := strings.TrimSpace(runGitOutput(t, j.worktree, "status", "--porcelain", "--untracked-files=all")); status != "" {
		t.Fatalf("conflict left the worktree dirty: %s", status)
	}

	// The Outcome Instruction's submit command records the rework.
	output, err := j.cli.deliveryRun(t, "skl", "watchdog", "prepare", "--repo", j.source, "--remote", "origin", "--item", deliveryTestItem, "--claim", claim)
	if err != nil {
		t.Fatal(err)
	}
	submit := proseMatch(t, conflictSubmit, output)
	words := shellWords(t, submit)
	outcome, head := slices.Index(words, "--outcome"), slices.Index(words, "--head")
	if outcome < 0 || words[outcome+1] != "rework" || head < 0 || words[head+1] != prepared {
		t.Fatalf("conflict outcome submit = %s", submit)
	}
	writeFile(t, words[slices.Index(words, "--body")+1], "# Review\n\nW1 BLOCK: integration conflict in feature.txt.\n")
	writeFile(t, words[slices.Index(words, "--public-body")+1], "# Summary\n")
	recorded, err := j.cli.deliveryRun(t, words...)
	if err != nil || !strings.Contains(recorded, "Status: "+ledger.Rework) {
		t.Fatalf("conflict rework submit:\n%s%v", recorded, err)
	}
}

func TestHumanRouteToWatchdogRunsTheIntegrationMergeAgain(t *testing.T) {
	j := newIntegrationJourney(t)
	claim := j.claimReview(t)
	if prepared, err := j.review(t, "prepare", claim); err != nil || prepared.Source == nil || prepared.Source.Head != j.reviewed {
		t.Fatalf("prepare with the branch integrated = %+v, %v", prepared, err)
	}
	if paused, err := j.review(t, "submit", claim, "--outcome", "needs-human", "--body", j.reportPath); err != nil || paused.Status != ledger.NeedsHuman {
		t.Fatalf("needs-human submit = %+v, %v", paused, err)
	}
	if applied := deliveryRecordHumanDirection(t, j.cli, j.reviewed, ledger.RouteWatchdog); applied.Status == "fix_required" {
		t.Fatalf("watchdog route: %s", mustJSON(t, applied))
	}
	moved := j.advance(t, "other.txt", "another slice\n")

	claim = j.claimReview(t)
	prepared, err := j.review(t, "prepare", claim)
	if err != nil || prepared.Source == nil || prepared.Source.Merged != moved {
		t.Fatalf("prepare after the watchdog route = %+v, %v; want a merge of %s", prepared, err, moved)
	}
}
