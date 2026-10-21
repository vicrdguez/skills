package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestInstalledFamiliesDispatchWithRuntimeSettingsAndContinueSafely(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	installInto(t, home)
	for _, harness := range []string{"pi", "opencode", "claude"} {
		for _, operation := range []string{"implement-loop", "implement-team-loop", "watchdog-loop", "implement-auto", "implement-team-auto", "watchdog-auto"} {
			t.Run(harness+"/"+operation, func(t *testing.T) {
				t.Parallel()
				f := newPrivateLedgerFixture(t)
				f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{%q:{"implement":{"model":"configured-worker","thinking":"configured-thinking"},"watchdog":{"model":"configured-watchdog","thinking":"configured-watch-thinking"},"reviewer":{"model":"configured-reviewer","thinking":"configured-review-thinking"},"helper":{"model":"configured-helper","thinking":"configured-helper-thinking"}}}}`, f.clone, harness))
				source, target := deliverySourceRepo(t)
				routeSourcePushes(t, source)
				runGit(t, source, "push", "-q", "origin", "main")
				f.commitLaunchProposal(t, singleSlice(deliveryTestProposal))
				cli := f.app(deliveryNoForgeApp(t))
				watchdog := strings.HasPrefix(operation, "watchdog")
				var implementClaim string
				if watchdog {
					f.commitLaunchReviewClaim(t)
					implementClaim = deliveryTrimmed(t, f.clone, "rev-parse", "HEAD")
					f.commitLaunchAwaitingReview(t, source, target)
				}
				entry := readFile(t, filepath.Join(home, fmt.Sprintf(entryPoints[harness], operation)))
				wantModel, wantThinking := "configured-worker", "configured-thinking"
				if watchdog {
					wantModel, wantThinking = "configured-watchdog", "configured-watch-thinking"
				}
				// Only standard implement-auto supplies an override; team-auto
				// starts Auto Mode with no arguments, using runtime choices.
				if operation == "implement-auto" {
					switch harness {
					case "pi":
						entry = strings.ReplaceAll(entry, "${1:-}", "explicit-worker")
					case "claude":
						entry = strings.ReplaceAll(entry, "$worker_model", "explicit-worker")
					default:
						entry = strings.ReplaceAll(entry, "$1", "explicit-worker")
					}
					wantModel = "explicit-worker"
				}
				args := append(loopCommand(t, entry), "--repo", source, "--format", "json")
				out, err := cli.deliveryJSON(t, args...)
				if err != nil || out.Status != "dispatched" || out.Dispatch == nil {
					t.Fatalf("installed command = %#v: %v", out, err)
				}
				d := out.Dispatch
				if d.WorkerModel != wantModel || d.WorkerThinking != wantThinking {
					t.Fatalf("worker choices = %q/%q, want %q/%q", d.WorkerModel, d.WorkerThinking, wantModel, wantThinking)
				}
				auto := strings.HasSuffix(operation, "-auto")
				continuation := shellWords(t, d.Continue)
				if slices.Contains(continuation, "--auto") != auto || !slices.Contains(continuation, "--wait=15m0s") {
					t.Fatalf("continuation lost mode or waiting: %s", d.Continue)
				}
				// Installed Dispatch commands defer role defaults to worker startup.
				f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{%q:{"reviewer":{"model":"startup-reviewer","thinking":"startup-review-thinking"},"helper":{"model":"startup-helper","thinking":"startup-helper-thinking"}}}}`, f.clone, harness))
				worker := cli.dispatchJSON(t, d.Worker)
				if worker.Execution == nil || worker.Execution.Claim.Commit != d.Claim || worker.Packet == nil {
					t.Fatalf("worker did not resume its dispatched Claim: %s", mustJSON(t, worker))
				}
				if watchdog {
					if worker.Execution.State.Claim.Auto != auto {
						t.Fatal("Watchdog Claim has the wrong Auto Mode")
					}
					report := worker.Execution.Implement
					if report == nil || report.Source.Head != deliveryTrimmed(t, source, "rev-parse", deliveryTestBranch) || report.Source.Target != target || report.Ledger.Claim != (ledger.Reference{Commit: implementClaim, Path: deliveryItemStatePath()}) {
						t.Fatalf("watchdog lost its implementation evidence: %#v", report)
					}
				} else {
					wantTarget := "main"
					if auto {
						wantTarget = "proposal/" + deliveryTestProposal
					}
					if worker.Execution.State.Target.Branch != wantTarget {
						t.Fatalf("target = %s, want %s", worker.Execution.State.Target.Branch, wantTarget)
					}
					facts := worker.Packet.Facts.Delivery
					if facts.Reviewer == nil || facts.Reviewer.Model != "startup-reviewer" || facts.Reviewer.Thinking != "startup-review-thinking" {
						t.Fatalf("reviewer choices lost: %#v", facts.Reviewer)
					}
					team := strings.Contains(operation, "-team-")
					if (facts.Mode == "team") != team {
						t.Fatalf("mode = %s, want team=%t", facts.Mode, team)
					}
					if team && (facts.Helper == nil || facts.Helper.Model != "startup-helper" || facts.Helper.Thinking != "startup-helper-thinking") {
						t.Fatalf("helper choices lost: %#v", facts.Helper)
					}
					// One installed journey per harness protects source/recovery wiring;
					// every row above still dispatches and starts its actual worker.
					if operation == "implement-team-auto" {
						prepared := cli.dispatchJSON(t, facts.PrepareCommand)
						if prepared.Packet == nil {
							t.Fatalf("installed prepare: %s", mustJSON(t, prepared))
						}
						inspected := cli.dispatchJSON(t, strings.ReplaceAll(prepared.Packet.Facts.Delivery.InspectCommand, "<observed-target-sha>", "HEAD"))
						if inspected.Source == nil || inspected.Execution.Claim.Commit != d.Claim || inspected.Packet.Facts.Delivery.ResumeCommand != facts.ResumeCommand {
							t.Fatal("installed worker follow-up journey lost source or recovery identity")
						}
					}
				}
				if ending, status := cli.continued(t, d.Continue); ending != ledger.ClaimHeld || status != "stopped" {
					t.Fatalf("held Claim continued: %s/%s", ending, status)
				}
			})
		}
	}
}
