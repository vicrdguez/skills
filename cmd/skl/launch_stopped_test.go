package main

import (
	"strings"
	"testing"
)

func TestStoppedDispatchPreservesHarnessAndExplicitSessionOverrides(t *testing.T) {
	newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)
	d := cli.dispatched(t, "skl implement dispatch --repo "+source+" --harness pi --reviewer-model explicit-review")
	stopped := cli.dispatchRun(t, d.Continue+" --format markdown")
	for _, flag := range []string{"--harness 'pi'", "--reviewer-model 'explicit-review'"} {
		if !strings.Contains(stopped, flag) {
			t.Fatalf("stopped resume dropped %s: %s", flag, stopped)
		}
	}
}
