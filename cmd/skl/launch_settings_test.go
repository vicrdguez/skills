package main

import (
	"fmt"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
)

func TestHarnessLaunchChoicesReachTheirRoles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, flags, model, thinking, reviewer, helper string }{
		{"configured Pi", "--harness pi", "pi-worker", "deep", "pi-review", ""},
		{"field overrides", "--harness pi --worker-model explicit --reviewer-thinking explicit-review", "explicit", "deep", "pi-review", ""},
		{"team", "--harness pi --mode team", "pi-worker", "deep", "pi-review", "pi-helper"},
		{"team helper override", "--harness pi --mode team --helper-model explicit-helper", "pi-worker", "deep", "pi-review", "explicit-helper"},
		{"Claude isolation", "--harness claude", "", "", "", ""},
		{"OpenCode", "--harness opencode", "oc-worker", "", "", ""},
		{"Codex", "--harness codex", "cx-worker", "", "", ""},
		{"unselected", "--worker-thinking explicit-thinking", "", "explicit-thinking", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPrivateLedgerFixture(t)
			f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"implement":{"model":"pi-worker","thinking":"deep"},"reviewer":{"model":"pi-review","thinking":"review-deep"},"helper":{"model":"pi-helper","thinking":"helper-deep"}},"claude":{},"opencode":{"implement":{"model":"oc-worker"}},"codex":{"implement":{"model":"cx-worker"}}}}`, f.clone))
			source, _ := deliverySourceRepo(t)
			f.acceptDelivery(t, newForgeServer(t), source)
			cli := f.app(deliveryNoForgeApp(t))
			d := cli.dispatched(t, "skl implement dispatch --repo "+source+" "+tc.flags)
			if d.WorkerModel != tc.model || d.WorkerThinking != tc.thinking {
				t.Fatalf("launch = %q/%q, want %q/%q", d.WorkerModel, d.WorkerThinking, tc.model, tc.thinking)
			}
			packet := cli.dispatchJSON(t, d.Worker).Packet
			if packet == nil {
				t.Fatal("worker has no packet")
			}
			facts := packet.Facts.Delivery
			model := func(c *skilldist.SubagentChoice) string {
				if c == nil {
					return ""
				}
				return c.Model
			}
			if model(facts.Reviewer) != tc.reviewer || model(facts.Helper) != tc.helper {
				t.Fatalf("worker roles = %#v/%#v", facts.Reviewer, facts.Helper)
			}
			if tc.name == "field overrides" && facts.Reviewer.Thinking != "explicit-review" {
				t.Fatal("reviewer override lost")
			}
			if tc.name == "team helper override" && facts.Helper.Thinking != "helper-deep" {
				t.Fatal("helper thinking default lost")
			}
		})
	}
}

func TestStartupAndRecoveryRefreshDefaultsWithoutChangingRunningGuidance(t *testing.T) {
	t.Parallel()
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			t.Parallel()
			f := newPrivateLedgerFixture(t)
			configure := func(suffix string) {
				f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"implement":{"model":"worker-%s","thinking":"thinking-%s"},"watchdog":{"model":"watch-%s","thinking":"watch-thinking"},"reviewer":{"model":"review-%s"},"helper":{"thinking":"helper-%s"}}}}`, f.clone, suffix, suffix, suffix, suffix, suffix))
			}
			configure("first")
			source, target := deliverySourceRepo(t)
			f.acceptDelivery(t, newForgeServer(t), source)
			cli := f.app(deliveryNoForgeApp(t))
			flags := " --harness pi --mode team"
			if explicit {
				flags += " --worker-model explicit-worker --reviewer-model explicit-review --helper-thinking explicit-helper"
			}
			first := cli.dispatched(t, "skl implement dispatch --repo "+source+flags)
			wantWorker := "worker-first"
			if explicit {
				wantWorker = "explicit-worker"
			}
			if first.WorkerModel != wantWorker || first.WorkerThinking != "thinking-first" {
				t.Fatalf("dispatch choices = %#v", first)
			}
			configure("startup")
			session := cli.dispatchJSON(t, first.Worker)
			wantReview, wantHelper := "review-startup", "helper-startup"
			if explicit {
				wantReview, wantHelper = "explicit-review", "explicit-helper"
			}
			if session.Packet.Facts.Delivery.Reviewer.Model != wantReview || session.Packet.Facts.Delivery.Helper.Thinking != wantHelper {
				t.Fatal("startup did not resolve current defaults and explicit overrides")
			}
			configure("recovery")
			prepared := cli.dispatchJSON(t, session.Packet.Facts.Delivery.PrepareCommand)
			inspected := cli.dispatchJSON(t, strings.ReplaceAll(prepared.Packet.Facts.Delivery.InspectCommand, "<observed-target-sha>", target))
			for _, followup := range []deliveryOutput{prepared, inspected} {
				// Source observations leave the startup guidance in the worker context;
				// they neither supply new role guidance nor serialize defaults.
				if strings.Contains(followup.Packet.Instructions, "review-recovery") || strings.Contains(followup.Packet.Instructions, "helper-recovery") || strings.Contains(followup.Packet.Instructions, "review-startup") || strings.Contains(followup.Packet.Instructions, "helper-startup") {
					t.Fatalf("follow-up changed or persisted session defaults: %s", followup.Packet.Instructions)
				}
				if followup.Packet.Facts.Delivery.ResumeCommand != session.Packet.Facts.Delivery.ResumeCommand {
					t.Fatal("follow-up changed recovery invocation")
				}
			}
			resumed := cli.dispatchJSON(t, inspected.Packet.Facts.Delivery.ResumeCommand)
			if !explicit {
				wantReview, wantHelper = "review-recovery", "helper-recovery"
			}
			if resumed.Packet.Facts.Delivery.Reviewer.Model != wantReview || resumed.Packet.Facts.Delivery.Helper.Thinking != wantHelper {
				t.Fatal("fresh recovery retained old defaults or lost overrides")
			}
			configure("next")
			dispatchImplementHandoff(t, cli, source, target, first.Item, first.Claim)
			watch := cli.dispatched(t, "skl watchdog dispatch --repo "+source+" --harness pi")
			if watch.WorkerModel != "watch-next" || watch.WorkerThinking != "watch-thinking" {
				t.Fatalf("watchdog used another role: %#v", watch)
			}
			watchPacket := cli.dispatchJSON(t, watch.Worker).Packet
			if !strings.Contains(watchPacket.Facts.Delivery.PrepareCommand, "--harness 'pi'") {
				t.Fatal("watchdog lost selector")
			}
			dispatchVerdict(t, cli, source, watch.Claim, "rework")
			second := cli.dispatched(t, first.Continue)
			wantModel := "worker-next"
			if explicit {
				wantModel = "explicit-worker"
			}
			if second.WorkerModel != wantModel || second.WorkerThinking != "thinking-next" {
				t.Fatalf("continuation choices = %#v", second)
			}
			if strings.Contains(first.Continue, "review-first") || strings.Contains(first.Continue, "thinking-first") {
				t.Fatalf("defaults became loop overrides: %s", first.Continue)
			}
			secondSession := cli.dispatchJSON(t, second.Worker).Packet.Facts.Delivery
			if !explicit {
				wantReview, wantHelper = "review-next", "helper-next"
			}
			if secondSession.Reviewer.Model != wantReview || secondSession.Helper.Thinking != wantHelper {
				t.Fatal("next session did not refresh roles")
			}
		})
	}
}

func TestEmptySessionChoicesStayInheritedAfterConfigurationEdit(t *testing.T) {
	t.Parallel()
	f := newPrivateLedgerFixture(t)
	f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"implement":{},"watchdog":{},"helper":{},"reviewer":{}},"opencode":{},"claude":{},"codex":{}}}`, f.clone))
	source, _ := deliverySourceRepo(t)
	f.acceptDelivery(t, newForgeServer(t), source)
	cli := f.app(deliveryNoForgeApp(t))
	d := cli.dispatched(t, "skl implement dispatch --repo "+source+" --harness pi --mode team")
	session := cli.dispatchJSON(t, d.Worker)
	facts := session.Packet.Facts.Delivery
	if facts.Reviewer != nil || facts.Helper != nil {
		t.Fatal("empty settings did not inherit")
	}
	f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"reviewer":{"model":"later-review"},"helper":{"model":"later-helper"}}}}`, f.clone))
	prepared := cli.dispatchJSON(t, facts.PrepareCommand)
	inspected := cli.dispatchJSON(t, strings.ReplaceAll(prepared.Packet.Facts.Delivery.InspectCommand, "<observed-target-sha>", "HEAD"))
	for _, out := range []deliveryOutput{prepared, inspected} {
		if out.Packet.Facts.Delivery.Reviewer != nil || out.Packet.Facts.Delivery.Helper != nil || strings.Contains(out.Packet.Instructions, "later-") {
			t.Fatal("source observation supplied new choices to the running session")
		}
	}
	fresh := cli.dispatchJSON(t, inspected.Packet.Facts.Delivery.ResumeCommand).Packet.Facts.Delivery
	if fresh.Reviewer == nil || fresh.Helper == nil || fresh.Reviewer.Model != "later-review" || fresh.Helper.Model != "later-helper" {
		t.Fatal("fresh recovery did not resolve new defaults")
	}
}

func TestLaunchConfigurationRefusalsDoNotClaimWork(t *testing.T) {
	t.Parallel()
	for _, config := range []string{
		`{"pi":{"unknown":{}}}`, `{"other":{}}`, `{"pi":{"implement":{"extra":"x"}}}`, `{"pi":{"implement":{"model":3}}}`,
	} {
		t.Run(config, func(t *testing.T) {
			t.Parallel()
			f := newPrivateLedgerFixture(t)
			f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":%s}`, f.clone, config))
			source, _ := deliverySourceRepo(t)
			before := ledgerSnapshot(t, f.clone)
			out := f.app(deliveryNoForgeApp(t)).dispatchJSON(t, "skl implement dispatch --repo "+source+" --harness pi")
			if out.Status != "fix_required" || ledgerSnapshot(t, f.clone) != before {
				t.Fatalf("invalid configuration claimed: %#v", out)
			}
		})
	}
	f := newPrivateLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	before := ledgerSnapshot(t, f.clone)
	out := f.app(deliveryNoForgeApp(t)).dispatchJSON(t, "skl implement dispatch --repo "+source+" --harness unknown")
	if out.Status != "fix_required" || ledgerSnapshot(t, f.clone) != before {
		t.Fatal("unknown selector did not refuse before claim")
	}
}
