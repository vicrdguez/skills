package main

import (
	"fmt"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
)

func TestHarnessLaunchChoicesReachTheirRoles(t *testing.T) {
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
			f := newLedgerFixture(t)
			f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"implement":{"model":"pi-worker","thinking":"deep"},"reviewer":{"model":"pi-review","thinking":"review-deep"},"helper":{"model":"pi-helper","thinking":"helper-deep"}},"claude":{},"opencode":{"implement":{"model":"oc-worker"}},"codex":{"implement":{"model":"cx-worker"}}}}`, f.clone))
			source, _ := deliverySourceRepo(t)
			deliveryAcceptFixture(t, newForgeServer(t), source)
			cli := deliveryNoForgeApp(t)
			d := cli.dispatched(t, "skl implement next --dispatch --repo "+source+" "+tc.flags)
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

func TestDispatchRefreshesDefaultsButFreezesSessionChoices(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			f := newLedgerFixture(t)
			configure := func(suffix string) {
				f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"implement":{"model":"worker-%s","thinking":"thinking-%s"},"watchdog":{"model":"watch-%s","thinking":"watch-thinking"},"reviewer":{"model":"review-%s"},"helper":{"thinking":"helper-%s"}}}}`, f.clone, suffix, suffix, suffix, suffix, suffix))
			}
			configure("first")
			source, target := deliverySourceRepo(t)
			deliveryAcceptFixture(t, newForgeServer(t), source)
			cli := deliveryNoForgeApp(t)
			flags := " --harness pi --mode team"
			if explicit {
				flags += " --worker-model explicit-worker --reviewer-model explicit-review --helper-thinking explicit-helper"
			}
			first := cli.dispatched(t, "skl implement next --dispatch --repo "+source+flags)
			configure("next")
			session := cli.dispatchJSON(t, first.Worker)
			wantReview, wantHelper := "review-first", "helper-first"
			if explicit {
				wantReview, wantHelper = "explicit-review", "explicit-helper"
			}
			if session.Packet.Facts.Delivery.Reviewer.Model != wantReview || session.Packet.Facts.Delivery.Helper.Thinking != wantHelper {
				t.Fatal("configuration edit changed session choices")
			}
			prepared := cli.dispatchJSON(t, session.Packet.Facts.Delivery.PrepareCommand)
			resumed := cli.dispatchJSON(t, prepared.Packet.Facts.Delivery.ResumeCommand)
			if resumed.Packet.Facts.Delivery.Reviewer.Model != wantReview {
				t.Fatal("prepare/resume lost session choices")
			}
			dispatchImplementHandoff(t, cli, source, target, first.Item, first.Claim)
			watch := cli.dispatched(t, "skl watchdog next --dispatch --repo "+source+" --harness pi")
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
			if strings.Contains(first.Continue, "review-first") || strings.Contains(first.Continue, "thinking-first") || strings.Contains(first.Continue, "--launch-resolved") {
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
	f := newLedgerFixture(t)
	f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"implement":{},"watchdog":{},"helper":{},"reviewer":{}},"opencode":{},"claude":{},"codex":{}}}`, f.clone))
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)
	d := cli.dispatched(t, "skl implement next --dispatch --repo "+source+" --harness pi --mode team")
	f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"reviewer":{"model":"later-review"},"helper":{"model":"later-helper"}}}}`, f.clone))
	facts := cli.dispatchJSON(t, d.Worker).Packet.Facts.Delivery
	if facts.Reviewer != nil || facts.Helper != nil {
		t.Fatal("omitted session choices picked up later defaults")
	}
}

func TestLaunchConfigurationRefusalsDoNotClaimWork(t *testing.T) {
	for _, config := range []string{
		`{"pi":{"unknown":{}}}`, `{"other":{}}`, `{"pi":{"implement":{"extra":"x"}}}`, `{"pi":{"implement":{"model":3}}}`,
	} {
		t.Run(config, func(t *testing.T) {
			f := newLedgerFixture(t)
			f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":%s}`, f.clone, config))
			source, _ := deliverySourceRepo(t)
			before := ledgerSnapshot(t, f.clone)
			out := deliveryNoForgeApp(t).dispatchJSON(t, "skl implement next --dispatch --repo "+source+" --harness pi")
			if out.Status != "fix_required" || ledgerSnapshot(t, f.clone) != before {
				t.Fatalf("invalid configuration claimed: %#v", out)
			}
		})
	}
	f := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	before := ledgerSnapshot(t, f.clone)
	out := deliveryNoForgeApp(t).dispatchJSON(t, "skl implement next --dispatch --repo "+source+" --harness unknown")
	if out.Status != "fix_required" || ledgerSnapshot(t, f.clone) != before {
		t.Fatal("unknown selector did not refuse before claim")
	}
}
