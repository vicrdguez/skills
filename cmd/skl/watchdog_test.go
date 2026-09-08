package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func watchdogCLI(t *testing.T, root string, backend *implementationMemory, args ...string) workflow.ImplementationOutcome {
	t.Helper()
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	command := append([]string{"skl", "watchdog"}, args...)
	command = append(command, "--repo", root)
	if err := app.Run(command); err != nil {
		t.Fatalf("%v: %v\n%s", command, err, &output)
	}
	var result workflow.ImplementationOutcome
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("%v: %s", err, &output)
	}
	return result
}

func TestWatchdogReportsNoEligibleWork(t *testing.T) {
	for _, work := range [][]workflow.ImplementationItem{nil, {
		{Number: 1, State: workflow.AwaitingReview, Claimed: true},
		{Number: 2, State: workflow.NeedsHuman},
		{Number: 3, State: workflow.Ready},
	}} {
		b := &implementationMemory{work: work}
		before := append([]workflow.ImplementationItem(nil), work...)
		got := watchdogCLI(t, proposalRepository(t), b, "next")
		if got.Status != "no_work" || !reflect.DeepEqual(before, b.work) {
			t.Fatalf("no work: %#v %#v", got, b.work)
		}
	}
}
