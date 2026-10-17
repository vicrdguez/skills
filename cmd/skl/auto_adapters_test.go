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
	harnesses := []string{"pi", "opencode", "claude"}
	// Every installed entry dispatches and starts its worker. Each Implement
	// operation runs the prepare and inspect follow-up through one harness,
	// rotated so every harness also completes that journey.
	for _, tc := range []struct{ operation, followUp string }{
		{"implement-loop", "pi"}, {"implement-team-loop", "opencode"}, {"watchdog-loop", ""},
		{"implement-auto", "claude"}, {"implement-team-auto", "pi"}, {"watchdog-auto", ""},
	} {
		operation := tc.operation
		t.Run(operation, func(t *testing.T) {
			// Each harness's installed entry claims its own fresh Slice (or
			// that Slice's review) of one accepted proposal, so the lifecycle
			// setup runs once per operation.
			f := newLedgerFixture(t)
			source, target := deliverySourceRepo(t)
			routeSourcePushes(t)
			runGit(t, source, "push", "-q", "origin", "main")
			cli := deliveryNoForgeApp(t)
			watchdog := strings.HasPrefix(operation, "watchdog")
			spec := independentSlices(deliveryTestProposal, harnesses...)
			if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, source, writeProposal(t, "", spec)); accepted.Status != "accepted" {
				t.Fatalf("acceptance = %s", mustJSON(t, accepted))
			}
			if watchdog {
				for range harnesses {
					implemented := cli.dispatched(t, "skl implement dispatch --repo "+source)
					dispatchImplementHandoff(t, cli, source, target, implemented.Item, implemented.Claim)
				}
			}
			for _, harness := range harnesses {
				t.Run(harness, func(t *testing.T) {
					f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{%q:{"implement":{"model":"configured-worker","thinking":"configured-thinking"},"watchdog":{"model":"configured-watchdog","thinking":"configured-watch-thinking"},"reviewer":{"model":"configured-reviewer","thinking":"configured-review-thinking"},"helper":{"model":"configured-helper","thinking":"configured-helper-thinking"}}}}`, f.clone, harness))
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
						if harness == tc.followUp {
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
		})
	}
}
