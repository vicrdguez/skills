package main

import (
	"strings"
	"testing"
)

func TestDeliveryRejectsRemovedAndInapplicableFlags(t *testing.T) {
	for _, phase := range []string{"implement", "watchdog"} {
		for _, args := range [][]string{
			{"next", "--dispatch"},
			{"next", "--slice", "proposal/slice"},
			{"resume", "--item", "proposal/slice"},
			{"resume", "--auto"},
			{"prepare", "--dispatched"},
			{"prepare", "--head", "source-head"},
			{"inspect", "--body", "report.md"},
			{"release", "--harness", "pi"},
			{"release", "--result-directory", "/tmp/results"},
			{"submit", "--wait"},
			{"needs-human", "--outcome", "needs-human"},
		} {
			t.Run(phase+"/"+strings.Join(args, " "), func(t *testing.T) {
				cli := deliveryNoForgeApp(t)
				if _, err := cli.deliveryRun(t, append([]string{"skl", phase}, args...)...); err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
					t.Fatalf("inapplicable syntax = %v", err)
				}
			})
		}
	}
	for _, args := range [][]string{
		{"implement", "submit", "--outcome", "pass"},
		{"watchdog", "dispatch", "--mode", "team"},
		{"watchdog", "resume", "--reviewer-model", "reviewer"},
	} {
		cli := deliveryNoForgeApp(t)
		if _, err := cli.deliveryRun(t, append([]string{"skl"}, args...)...); err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
			t.Fatalf("%v = %v", args, err)
		}
	}
}

func TestDeliveryHelpExposesCanonicalOperationsAndRelevantFlags(t *testing.T) {
	for _, phase := range []string{"implement", "watchdog"} {
		cli := deliveryNoForgeApp(t)
		text, err := cli.deliveryRun(t, "skl", phase, "--help")
		if err != nil || !strings.Contains(text, "dispatch") || !strings.Contains(text, "needs-human") || strings.Contains(text, "next, start") {
			t.Fatalf("%s help = %v:\n%s", phase, err, text)
		}
		text, err = cli.deliveryRun(t, "skl", phase, "release", "--help")
		if err != nil || !strings.Contains(text, "--slice") || !strings.Contains(text, "--claim") || strings.Contains(text, "--head") || strings.Contains(text, "--harness") {
			t.Fatalf("%s release help = %v:\n%s", phase, err, text)
		}
	}
}

func TestWatchdogSubmitNeedsHumanIsRefusedWithoutAHandoff(t *testing.T) {
	t.Parallel()
	fixture := newPrivateLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	cli := fixture.app(deliveryNoForgeApp(t))
	fixture.acceptDelivery(t, newForgeServer(t), source)
	started := cli.dispatchJSON(t, "skl implement next --repo "+source)
	dispatchImplementHandoff(t, cli, source, target, deliveryTestItem, started.Execution.Claim.Commit)
	review := cli.dispatched(t, "skl watchdog dispatch --repo "+source)
	before := ledgerSnapshot(t, fixture.clone)
	out, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--slice", deliveryTestItem, "--claim", review.Claim,
		"--outcome", "needs-human", "--body", proseFixturePath("watchdog-report.md"), "--format", "json")
	if err != nil || out.Status != "fix_required" || !strings.Contains(out.Reason, "pass or rework") {
		t.Fatalf("removed outcome = %#v, %v", out, err)
	}
	if ledgerSnapshot(t, fixture.clone) != before {
		t.Fatal("removed outcome recorded a handoff or released its Claim")
	}
	paused, err := cli.deliveryJSON(t, "skl", "watchdog", "needs-human", "--repo", source, "--slice", deliveryTestItem, "--claim", review.Claim,
		"--body", proseFixturePath("watchdog-report.md"), "--format", "json")
	if err != nil || paused.Status != "needs_human" || paused.Result == nil || paused.Result.State.Claim != nil {
		t.Fatalf("human handoff = %#v, %v", paused, err)
	}
}
