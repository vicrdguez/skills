package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

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
				// Human direction cannot create a fixed reviewed source identity from
				// a valid headless implementation pause.
				deliveryRecordHumanDirection(t, fixture.clone, target, ledger.AwaitingReview)
				review, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
				if err != nil || review.Status != "fix_required" || !strings.Contains(review.Reason, "fixed source head") || deliveryPersistedState(t, fixture.clone).Claim != nil {
					t.Fatalf("headless review acquired work: %#v %v", review, err)
				}
				return
			}
			// A new implementation blocker consumes the old direction rather than
			// silently authorizing further work with that stale decision.
			deliveryRecordHumanDirection(t, fixture.clone, target, ledger.ReadyForImplementation)
			directed, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
			if err != nil || directed.Execution == nil {
				t.Fatalf("directed start: %#v %v", directed, err)
			}
			paused, err := cli.deliveryJSON(t, "skl", "implement", "needs-human", "--repo", source, "--item", deliveryTestItem, "--claim", directed.Execution.Claim.Commit, "--head", target, "--target", target, "--body", body, "--format", "json")
			if err != nil || paused.Status != ledger.NeedsHuman || deliveryPersistedState(t, fixture.clone).Decision != nil {
				t.Fatalf("new blocker retained old authority: %#v %v", paused, err)
			}
			recorded, _ := deliveryCommittedReport(t, cli, paused.Result.Report, ledger.ImplementPhase)
			if recorded.Ledger.Decision == nil {
				t.Fatal("paused report lost the exact direction it consumed")
			}
		})
	}
}
