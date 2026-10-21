package main

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/vicrdguez/skills/ledger"
)

func TestWaitingDispatchRefreshesLaunchSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, phase   string
		explicit      bool
		invalid       bool
		changedLedger bool
	}{
		{name: "implementation", phase: "implement"},
		{name: "partial overrides", phase: "implement", explicit: true},
		{name: "watchdog", phase: "watchdog"},
		{name: "invalid edit", phase: "implement", invalid: true},
		{name: "changed ledger", phase: "implement", changedLedger: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := newPrivateLedgerFixture(t)
			configure := func(value string) {
				fixture.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"implement":{"model":%q,"thinking":%q},"watchdog":{"model":%q,"thinking":%q},"reviewer":{"model":%q,"thinking":%q},"helper":{"model":%q,"thinking":%q}}}}`, fixture.clone, value, value, value, value, value, value, value, value))
			}
			configure("before-wait")
			source, target := deliverySourceRepo(t)
			spec := dualSlice(deliveryTestProposal)
			spec.depends = map[string][]string{"feature": {"foundation"}}
			fixture.commitReadyProposal(t, spec)
			cli := fixture.app(deliveryNoForgeApp(t))
			held, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
			if err != nil || held.Execution == nil {
				t.Fatalf("claim foundation: %#v %v", held, err)
			}
			synctest.Test(t, func(t *testing.T) {
				done := make(chan struct{})
				var out deliveryOutput
				var runErr error
				args := []string{"skl", tc.phase, "dispatch", "--repo", source, "--wait=2m", "--poll=30s", "--harness", "pi", "--format", "json"}
				if tc.phase == "implement" {
					args = append(args, "--mode", "team")
				}
				if tc.explicit {
					args = append(args, "--worker-model", "explicit", "--reviewer-thinking", "explicit", "--helper-model", "explicit")
				}
				go func() {
					defer close(done)
					out, runErr = cli.deliveryJSON(t, args...)
				}()
				time.Sleep(45 * time.Second)
				configure("edited-before-dispatch")
				if tc.invalid {
					fixture.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"implement":{"model":3}}}}`, fixture.clone))
				}
				if tc.changedLedger {
					fixture.misconfigure(t, fmt.Sprintf(`{"ledger":%q}`, source))
				}
				if tc.phase == "watchdog" {
					fixture.commitLaunchAwaitingReview(t, source, target)
				} else {
					state := deliveryPersistedState(t, fixture.clone)
					state.Claim, state.State = nil, ledger.Merged
					state.Completion = &ledger.TerminalEvidence{Target: *state.Target}
					deliveryCommitState(t, fixture.clone, state)
				}
				before := ledgerSnapshot(t, fixture.clone)
				<-done
				if runErr != nil {
					t.Fatal(runErr)
				}
				if tc.invalid || tc.changedLedger {
					if out.Status != "fix_required" || out.Dispatch != nil || ledgerSnapshot(t, fixture.clone) != before {
						t.Fatalf("edited configuration did not refuse before acquisition: %#v", out)
					}
					return
				}
				if out.Dispatch == nil {
					t.Fatalf("dispatch: %#v", out)
				}
				wantModel := "edited-before-dispatch"
				if tc.explicit {
					wantModel = "explicit"
				}
				if out.Dispatch.WorkerModel != wantModel || out.Dispatch.WorkerThinking != "edited-before-dispatch" {
					t.Fatalf("waiting worker choices = %q/%q, want %q/edited-before-dispatch", out.Dispatch.WorkerModel, out.Dispatch.WorkerThinking, wantModel)
				}
				configure("after-dispatch")
				facts := cli.dispatchJSON(t, out.Dispatch.Worker).Packet.Facts.Delivery
				if tc.phase == "implement" {
					wantReviewThinking, wantHelperModel := "after-dispatch", "after-dispatch"
					if tc.explicit {
						wantReviewThinking, wantHelperModel = "explicit", "explicit"
					}
					if facts.Reviewer == nil || facts.Helper == nil || facts.Reviewer.Model != "after-dispatch" || facts.Reviewer.Thinking != wantReviewThinking || facts.Helper.Model != wantHelperModel || facts.Helper.Thinking != "after-dispatch" {
						t.Fatalf("waiting session choices = reviewer %#v, helper %#v", facts.Reviewer, facts.Helper)
					}
				}
			})
		})
	}
}
