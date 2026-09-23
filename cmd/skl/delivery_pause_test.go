package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// An unresolved merge is a reason to ask the human, not a completed source
// handoff. Pausing preserves the unmerged index and makes the request visible.
func TestDeliveryCLIConflictCanPauseWithoutResolvingSource(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)
	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("start: %#v %v", started, err)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("prepare: %#v %v", prepared, err)
	}
	worktree := prepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "choice.txt"), "first worker's choice\n")
	runGit(t, worktree, "add", "choice.txt")
	runGit(t, worktree, "commit", "-q", "-m", "first worker")
	head := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(source, "choice.txt"), "target's different choice\n")
	runGit(t, source, "add", "choice.txt")
	runGit(t, source, "commit", "-q", "-m", "target change")
	target := deliveryTrimmed(t, source, "rev-parse", "HEAD")
	merge := exec.Command("git", "-C", worktree, "merge", "--no-edit", target)
	if output, err := merge.CombinedOutput(); err == nil {
		t.Fatalf("expected a real merge conflict: %s", output)
	}
	if unmerged := deliveryTrimmed(t, worktree, "ls-files", "-u"); unmerged == "" {
		t.Fatal("fixture failed to produce an unresolved index")
	}
	body := filepath.Join(t.TempDir(), "question.md")
	writeFile(t, body, "# Human decision needed\n\nThe integration conflict requires a choice between incompatible changes.\n")
	completed, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--head", head, "--target", target, "--body", body, "--format", "json")
	if err != nil || completed.Status != "fix_required" || deliveryPersistedState(t, fixture.clone).Claim == nil {
		t.Fatalf("conflicted source was accepted as completed: %#v %v", completed, err)
	}
	args := []string{"skl", "implement", "needs-human", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--head", head, "--target", target, "--body", body, "--format", "json"}
	paused, err := cli.deliveryJSON(t, args...)
	if err != nil || paused.Status != ledger.NeedsHuman || paused.Result == nil {
		t.Fatalf("conflicted pause: %#v %v", paused, err)
	}
	state := deliveryPersistedState(t, fixture.clone)
	if state.Claim != nil || state.State != ledger.NeedsHuman {
		t.Fatalf("Claim not released on pause: %#v", state)
	}
	report, text := deliveryCommittedReport(t, cli, paused.Result.Report, ledger.ImplementPhase)
	if report.Source.Head != head || report.Source.Target != target || !strings.Contains(text, "integration conflict") {
		t.Fatalf("pause lost source or question: %#v %q", report.Source, text)
	}
	request := decisionRequest(t, cli, "widgets", deliveryTestItem)
	if decisionReference(request) != paused.Result.Report {
		t.Fatalf("inbox request lost exact report: %#v", request)
	}
	refused := deliveryRecordHumanDirection(t, cli, head, ledger.RouteWatchdog)
	if refused.Status != ledger.DecisionRefused || refused.Facts == nil || len(refused.Facts.Outcomes) != 1 || !strings.Contains(refused.Facts.Outcomes[0].Reason, "completed implementation report") {
		t.Fatalf("pause was mistaken for reviewable completion: %#v", refused)
	}
	if unmerged := deliveryTrimmed(t, worktree, "ls-files", "-u"); unmerged == "" || !strings.Contains(readFileString(t, filepath.Join(worktree, "choice.txt")), "<<<<<<<") {
		t.Fatal("pause resolved or discarded the unfinished integration")
	}
}

func TestDeliveryCLIPauseSourceBoundary(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		name := "before preparation"
		if prepared {
			name = "with source progress"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			source, target := deliverySourceRepo(t)
			deliveryAcceptFixture(t, newForgeServer(t), source)
			cli := deliveryNoForgeApp(t)
			start, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
			if err != nil || start.Execution == nil {
				t.Fatalf("start: %#v %v", start, err)
			}
			claim := start.Execution.Claim.Commit
			if prepared {
				out, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
				if err != nil || out.Status != "prepared" {
					t.Fatalf("prepare: %#v %v", out, err)
				}
			}
			body := filepath.Join(t.TempDir(), "decision.md")
			writeFile(t, body, "# Decision needed\n\nB1 incomplete: a consequential choice remains human-owned.\n")
			args := []string{"skl", "implement", "needs-human", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--body", body, "--format", "json"}
			out, err := cli.deliveryJSON(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			if prepared {
				if out.Status != "fix_required" || deliveryPersistedState(t, fixture.clone).Claim == nil {
					t.Fatalf("source-less pause hid existing source: %#v", out)
				}
				args = append(args, "--head", target, "--target", target)
				out, err = cli.deliveryJSON(t, args...)
				if err != nil {
					t.Fatal(err)
				}
			}
			if out.Status != ledger.NeedsHuman || out.Result == nil {
				t.Fatalf("pause: %#v", out)
			}
			state := deliveryPersistedState(t, fixture.clone)
			if state.Claim != nil || state.State != ledger.NeedsHuman {
				t.Fatalf("pause did not commit and release: %#v", state)
			}
			report, _ := deliveryCommittedReport(t, cli, out.Result.Report, ledger.ImplementPhase)
			if prepared && report.Source.Head != target || !prepared && report.Source.Head != "" {
				t.Fatalf("pause source: %#v", report.Source)
			}
			if !prepared {
				// A headless implementation pause has no fixed reviewed source, so the
				// ledger refuses a Watchdog continuation and the item stays paused.
				refused := deliveryRecordHumanDirection(t, cli, target, ledger.RouteWatchdog)
				if refused.Status != ledger.DecisionRefused || refused.Facts == nil || len(refused.Facts.Outcomes) != 1 ||
					!strings.Contains(refused.Facts.Outcomes[0].Reason, "fixed source head") {
					t.Fatalf("headless Watchdog direction was not refused: %#v", refused)
				}
				review, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
				state := deliveryPersistedState(t, fixture.clone)
				if err != nil || review.Status != "no_work" || state.Claim != nil || state.Decision {
					t.Fatalf("headless review acquired work after a refused direction: %#v %v", review, err)
				}
				return
			}
			// A new implementation blocker consumes the old direction rather than
			// silently authorizing further work with that stale decision.
			humanDirection := deliveryRecordHumanDirection(t, cli, target, ledger.RouteImplement)
			if humanDirection.Status != ledger.DecisionApplied {
				t.Fatalf("record human direction = %#v, want applied", humanDirection)
			}
			directed, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
			if err != nil || directed.Execution == nil {
				t.Fatalf("directed start: %#v %v", directed, err)
			}
			paused, err := cli.deliveryJSON(t, "skl", "implement", "needs-human", "--repo", source, "--item", deliveryTestItem, "--claim", directed.Execution.Claim.Commit, "--head", target, "--target", target, "--body", body, "--format", "json")
			if err != nil || paused.Status != ledger.NeedsHuman || deliveryPersistedState(t, fixture.clone).Decision {
				t.Fatalf("new blocker retained old authority: %#v %v", paused, err)
			}
			recorded, _ := deliveryCommittedReport(t, cli, paused.Result.Report, ledger.ImplementPhase)
			if recorded.Ledger.Decision == nil {
				t.Fatal("paused report lost the exact direction it consumed")
			}
		})
	}
}
