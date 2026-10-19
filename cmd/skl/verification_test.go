package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// Implement lists the cited rows from the source it starts on, Watchdog lists
// them from the reviewed source, including the row the slice added, and a
// pass is bounded by the weakest row cited for a rule.
func TestDeliveryBindsCitedChecksAndBoundsTheVerdict(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, _ := deliverySourceRepo(t)
	writeFile(t, filepath.Join(source, ".skl", "verify.md"), lintSkill)
	runGit(t, source, "add", ".")
	runGit(t, source, "commit", "-q", "-m", "verification skill")
	target := deliveryTrimmed(t, source, "rev-parse", "HEAD")
	runGit(t, source, "update-ref", "refs/remotes/origin/main", target)

	files := strategy("| B1, A1 | gate 17 | corpus |\n| B2 | tests TestSecondRule | tests |\n| B2 | new corpus-case 18 | corpus |\n")
	files["intent.md"] = strings.Replace(files["intent.md"], "## Manual", "- [ ] The `corpus-case` row holds the second rule's case (B2).\n\n## Manual", 1)
	if outcome := newLedgerApp(t, forge).accept(t, source, writeProposal(t, "", lintSpec(deliveryTestProposal, files))); outcome.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, outcome))
	}
	const item = deliveryTestItem
	cli := deliveryNoForgeApp(t)

	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || started.Packet == nil {
		t.Fatalf("implement next: %#v %v", started, err)
	}
	instructions := started.Packet.Instructions
	for _, fragment := range []string{"- `gate` `17`, for B1, A1: run `make gate`", "- `corpus-case` `18`, for B2: a row this slice adds"} {
		if !strings.Contains(instructions, fragment) {
			t.Fatalf("implement instructions lack %q:\n%s", fragment, instructions)
		}
	}
	claim := started.Execution.Claim.Commit
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--slice", item, "--claim", claim, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("implement prepare: %#v %v", prepared, err)
	}
	worktree := prepared.Source.Worktree
	added := strings.Replace(lintSkill, "| every route | |\n", "| every route | |\n| corpus-case | oracle | executed | `make corpus CASE=18` | case 18 passes | corpus |\n", 1)
	writeFile(t, filepath.Join(worktree, ".skl", "verify.md"), added)
	// Once prepared, Implement reads the worktree, where the row now exists.
	resumed, err := cli.deliveryJSON(t, "skl", "implement", "resume", "--repo", source, "--slice", item, "--claim", claim, "--format", "json")
	if err != nil || resumed.Packet == nil {
		t.Fatalf("implement resume: %#v %v", resumed, err)
	}
	if fragment := "- `corpus-case` `18`, for B2: run `make corpus CASE=18`"; !strings.Contains(resumed.Packet.Instructions, fragment) {
		t.Fatalf("resumed instructions lack %q:\n%s", fragment, resumed.Packet.Instructions)
	}
	writeFile(t, filepath.Join(worktree, "feature.txt"), "second rule\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "add the corpus case")
	head := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")
	body := filepath.Join(t.TempDir(), "report.md")
	writeFile(t, body, "# Implementation result\n")
	if submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--slice", item, "--claim", claim,
		"--head", head, "--target", target, "--body", body, "--format", "json"); err != nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit: %#v %v", submitted, err)
	}

	// The worktree no longer holds the row; the reviewed head does.
	runGit(t, worktree, "checkout", "-q", "--detach", target)
	review, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil || review.Packet == nil {
		t.Fatalf("watchdog next: %#v %v", review, err)
	}
	if fragment := "- `corpus-case` `18`, for B2: run `make corpus CASE=18`"; !strings.Contains(review.Packet.Instructions, fragment) {
		t.Fatalf("watchdog instructions lack %q:\n%s", fragment, review.Packet.Instructions)
	}
	runGit(t, worktree, "checkout", "-q", deliveryTestProposal)

	reviewClaim := review.Execution.Claim.Commit
	verdict := func(rank string) deliveryOutput {
		t.Helper()
		output, _ := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--slice", item, "--claim", reviewClaim,
			"--outcome", "pass", "--rank", rank, "--body", body, "--format", "json")
		return output
	}
	refused := verdict("corpus")
	if refused.Status != "fix_required" || !strings.Contains(refused.Reason, "of tests,") || !strings.Contains(refused.Reason, "--rank corpus") {
		t.Fatalf("a pass above the bound = %#v, want a refusal naming the bounding row tests", refused)
	}
	if kept := deliveryPersistedState(t, fixture.clone); kept.Claim == nil || kept.Claim.Phase != ledger.WatchdogPhase {
		t.Fatalf("the refused pass released the Claim: %#v", kept)
	}
	if unknown := verdict("inspection"); unknown.Status != "fix_required" || !strings.Contains(unknown.Reason, "corpus, tests, build") {
		t.Fatalf("a built-in rank under the project ladder = %#v, want a refusal listing the project ranks", unknown)
	}
	passed := verdict("tests")
	if passed.Status != ledger.ReadyForMerge || passed.Result == nil {
		t.Fatalf("a pass at the bound = %#v, want ready_for_merge", passed)
	}
	if report, _ := deliveryCommittedReport(t, cli, passed.Result.Report, ledger.WatchdogPhase); report.Rank != "tests" {
		t.Fatalf("recorded rank = %q, want tests", report.Rank)
	}
}

// A repository without a verification skill renders neither list.
func TestDeliveryWithoutASkillListsNoChecks(t *testing.T) {
	newLedgerFixture(t)
	forge := newForgeServer(t)
	source, _ := deliverySourceRepo(t)
	if outcome := newLedgerApp(t, forge).accept(t, source, writeProposal(t, "", lintSpec("unverified", lintedContract()))); outcome.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, outcome))
	}
	started, err := deliveryNoForgeApp(t).deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || started.Packet == nil {
		t.Fatalf("implement next: %#v %v", started, err)
	}
	if strings.Contains(started.Packet.Instructions, ".skl/verify.md") || len(started.Packet.Facts.Delivery.Checks) != 0 {
		t.Fatalf("a repository without a skill rendered cited checks:\n%s", started.Packet.Instructions)
	}
}

// skl skill verify finds the skill of the repository containing the working
// directory.
func TestSkillVerifyReadsTheWorkingRepository(t *testing.T) {
	root := skillRepository(t, lintSkill)
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	output, err := deliveryNoForgeApp(t).deliveryRun(t, "skl", "skill", "verify")
	if err != nil || !strings.Contains(output, "`corpus`, `tests`, `build`") || !strings.Contains(output, "`skl skill --resource oracle.md verify`") || !strings.Contains(output, "| gate | oracle |") {
		t.Fatalf("skl skill verify = %v\n%s", err, output)
	}
}
