package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// Acceptance fixes the citations; changes in the reviewed catalog must stay
// visible defects, not strand the Claim or permit a passing handoff.
func TestWatchdogCanReturnInvalidVerificationForRework(t *testing.T) {
	for _, tc := range []struct {
		name, skill, diagnostic, outcome string
	}{
		{"malformed rework", strings.Replace(lintSkill, "## Launch", "## Missing", 1), "Launch", "rework"},
		{"malformed escalation", strings.Replace(lintSkill, "## Launch", "## Missing", 1), "Launch", "needs-human"},
		{"deleted cited row", strings.Replace(lintSkill, "| tests | tests | executed | `go test ./...` | every test passes | tests |\n", "", 1), "tests", "rework"},
		{"changed cited kind", strings.Replace(lintSkill, "| tests | tests | executed |", "| tests | tests | human |", 1), "human", "rework"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			source, _ := deliverySourceRepo(t)
			writeFile(t, filepath.Join(source, ".skl", "verify.md"), lintSkill)
			runGit(t, source, "add", ".")
			runGit(t, source, "commit", "-q", "-m", "verification skill")
			target := deliveryTrimmed(t, source, "rev-parse", "HEAD")
			runGit(t, source, "update-ref", "refs/remotes/origin/main", target)
			files := strategy("| B1, A1 | gate | corpus |\n| B2 | tests TestSecondRule | tests |\n")
			if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, source, writeProposal(t, "", lintSpec(deliveryTestProposal, files))); accepted.Status != "accepted" {
				t.Fatalf("accept: %s", mustJSON(t, accepted))
			}
			cli := deliveryNoForgeApp(t)
			started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
			if err != nil || started.Execution == nil {
				t.Fatalf("implement next: %#v %v", started, err)
			}
			claim := started.Execution.Claim.Commit
			prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--slice", deliveryTestItem, "--claim", claim, "--format", "json")
			if err != nil || prepared.Source == nil {
				t.Fatalf("implement prepare: %#v %v", prepared, err)
			}
			worktree := prepared.Source.Worktree
			writeFile(t, filepath.Join(worktree, ".skl", "verify.md"), tc.skill)
			runGit(t, worktree, "add", ".")
			runGit(t, worktree, "commit", "-q", "-m", "change verification")
			head := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")
			body := filepath.Join(t.TempDir(), "report.md")
			writeFile(t, body, "# Result\n\nVerification defect requires rework.\n")
			if submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--slice", deliveryTestItem, "--claim", claim,
				"--head", head, "--target", target, "--body", body, "--format", "json"); err != nil || submitted.Status != ledger.AwaitingReview {
				t.Fatalf("implement submit: %#v %v", submitted, err)
			}
			review, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
			if err != nil || review.Packet == nil || review.Execution == nil {
				t.Fatalf("watchdog next must render the acquired review: %#v %v", review, err)
			}
			if !strings.Contains(review.Packet.Instructions, tc.diagnostic) {
				t.Fatalf("review lacks verification defect %q: %s", tc.diagnostic, review.Packet.Instructions)
			}
			if tc.name == "deleted cited row" {
				checks := review.Packet.Facts.Delivery.Checks
				if len(checks) != 2 || checks[1].Name != "tests" || !checks[1].Missing || checks[1].Added {
					t.Fatalf("deleted citation disappeared or became new: %#v", checks)
				}
				if !strings.Contains(review.Packet.Instructions, "- `tests` `TestSecondRule`, for B2:") {
					t.Fatalf("missing citation not rendered: %s", review.Packet.Instructions)
				}
			}
			reviewClaim := review.Execution.Claim.Commit
			prepared, err = cli.deliveryJSON(t, "skl", "watchdog", "prepare", "--repo", source, "--slice", deliveryTestItem, "--claim", reviewClaim, "--format", "json")
			if err != nil || prepared.Source == nil {
				t.Fatalf("watchdog prepare: %#v %v", prepared, err)
			}
			for _, rank := range []string{"corpus", "tests"} {
				refused, _ := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--slice", deliveryTestItem, "--claim", reviewClaim,
					"--outcome", "pass", "--rank", rank, "--body", body, "--format", "json")
				if refused.Status != "fix_required" || !strings.Contains(refused.Reason, tc.diagnostic) {
					t.Fatalf("invalid verification permits pass or hides defect: %#v", refused)
				}
				if state := deliveryPersistedState(t, fixture.clone); state.Claim == nil || state.Claim.Phase != ledger.WatchdogPhase {
					t.Fatalf("refusal lost review Claim: %#v", state)
				}
			}
			args := []string{"skl", "watchdog", "submit", "--outcome", "rework"}
			want := ledger.Rework
			if tc.outcome == "needs-human" {
				args, want = []string{"skl", "watchdog", "needs-human"}, ledger.NeedsHuman
			}
			args = append(args, "--repo", source, "--slice", deliveryTestItem, "--claim", reviewClaim, "--body", body, "--format", "json")
			handed, err := cli.deliveryJSON(t, args...)
			if err != nil || handed.Status != want || handed.Result == nil {
				t.Fatalf("negative handoff: %#v %v", handed, err)
			}
			report, contents := deliveryCommittedReport(t, cli, handed.Result.Report, ledger.WatchdogPhase)
			if report.Source.Reviewed != head || !strings.Contains(contents, "Verification defect requires rework.") {
				t.Fatalf("handoff lost reviewed source or report: %#v %s", report, contents)
			}
			if state := deliveryPersistedState(t, fixture.clone); state.Claim != nil {
				t.Fatalf("successful handoff retained Claim: %#v", state)
			}
		})
	}
}
