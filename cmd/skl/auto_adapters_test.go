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
	home := t.TempDir()
	installInto(t, home)
	for _, harness := range []string{"pi", "opencode", "claude"} {
		for _, operation := range []string{"implement-loop", "implement-team-loop", "watchdog-loop", "implement-auto", "implement-team-auto", "watchdog-auto"} {
			t.Run(harness+"/"+operation, func(t *testing.T) {
				f := newLedgerFixture(t)
				f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{%q:{"implement":{"model":"configured-worker","thinking":"configured-thinking"},"watchdog":{"model":"configured-watchdog","thinking":"configured-watch-thinking"},"reviewer":{"model":"configured-reviewer","thinking":"configured-review-thinking"},"helper":{"model":"configured-helper","thinking":"configured-helper-thinking"}}}}`, f.clone, harness))
				source, target := deliverySourceRepo(t)
				deliveryAcceptFixture(t, newForgeServer(t), source)
				cli := deliveryNoForgeApp(t)
				watchdog := strings.HasPrefix(operation, "watchdog")
				if watchdog {
					implemented := cli.dispatched(t, "skl implement dispatch --repo "+source)
					dispatchImplementHandoff(t, cli, source, target, implemented.Item, implemented.Claim)
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
				worker := cli.dispatchJSON(t, d.Worker)
				if worker.Execution == nil || worker.Execution.Claim.Commit != d.Claim || worker.Packet == nil {
					t.Fatalf("worker did not resume its dispatched Claim: %s", mustJSON(t, worker))
				}
				if watchdog {
					if worker.Execution.State.Claim.Auto != auto {
						t.Fatal("Watchdog Claim has the wrong Auto Mode")
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
					if facts.Reviewer == nil || facts.Reviewer.Model != "configured-reviewer" || facts.Reviewer.Thinking != "configured-review-thinking" {
						t.Fatalf("reviewer choices lost: %#v", facts.Reviewer)
					}
					team := strings.Contains(operation, "-team-")
					if (facts.Mode == "team") != team {
						t.Fatalf("mode = %s, want team=%t", facts.Mode, team)
					}
					if team && (facts.Helper == nil || facts.Helper.Model != "configured-helper" || facts.Helper.Thinking != "configured-helper-thinking") {
						t.Fatalf("helper choices lost: %#v", facts.Helper)
					}
				}
				if ending, status := cli.continued(t, d.Continue); ending != ledger.ClaimHeld || status != "stopped" {
					t.Fatalf("held Claim continued: %s/%s", ending, status)
				}
			})
		}
	}
}
