package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestDirectWorkerStartupAndRecoveryUseCurrentRoleDefaults(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, flags string }{
		{"standard", "--harness pi --reviewer-thinking explicit-thinking"},
		{"team", "--harness pi --mode team --helper-model explicit-helper --reviewer-thinking explicit-thinking"},
		{"explicit only", "--reviewer-model explicit-review --reviewer-thinking explicit-thinking"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPrivateLedgerFixture(t)
			configure := func(value string) {
				f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"reviewer":{"model":%q,"thinking":"configured-thinking"},"helper":{"model":"configured-helper","thinking":%q}}}}`, f.clone, value, value))
			}
			configure("first")
			source, _ := deliverySourceRepo(t)
			f.commitLaunchProposal(t, singleSlice(deliveryTestProposal))
			cli := f.app(deliveryNoForgeApp(t))
			first := cli.dispatchJSON(t, "skl implement next --repo "+source+" "+tc.flags).Packet.Facts.Delivery
			wantModel := "first"
			if tc.name == "explicit only" {
				wantModel = "explicit-review"
			}
			if first.Reviewer.Model != wantModel || first.Reviewer.Thinking != "explicit-thinking" {
				t.Fatalf("direct startup choices: %#v", first.Reviewer)
			}
			configure("recovery")
			fresh := cli.dispatchJSON(t, first.ResumeCommand).Packet.Facts.Delivery
			if tc.name != "explicit only" {
				wantModel = "recovery"
			}
			if fresh.Reviewer.Model != wantModel || fresh.Reviewer.Thinking != "explicit-thinking" {
				t.Fatalf("fresh recovery choices: %#v", fresh.Reviewer)
			}
			if tc.name == "team" && (first.Helper.Model != "explicit-helper" || first.Helper.Thinking != "first" || fresh.Helper.Model != "explicit-helper" || fresh.Helper.Thinking != "recovery") {
				t.Fatalf("helper field precedence: first %#v, fresh %#v", first.Helper, fresh.Helper)
			}
			if tc.name != "team" && (first.Helper != nil || fresh.Helper != nil) {
				t.Fatal("helper defaults applied outside team mode")
			}
		})
	}
}

func TestLaunchResolvedIsRejectedByTheParser(t *testing.T) {
	t.Parallel()
	f := newPrivateLedgerFixture(t)
	cli := f.app(deliveryNoForgeApp(t))
	before := ledgerSnapshot(t, f.clone)
	for _, phase := range []string{"implement", "watchdog"} {
		for _, operation := range []string{"next", "dispatch", "resume", "prepare", "inspect"} {
			output, err := cli.deliveryRun(t, "skl", phase, operation, "--launch-resolved")
			if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") || strings.Contains(output, "Status: fix_required") {
				t.Fatalf("%s %s: removed marker must be a parser error, got %v\n%s", phase, operation, err, output)
			}
		}
	}
	if ledgerSnapshot(t, f.clone) != before {
		t.Fatal("unsupported syntax changed the ledger")
	}
}
