package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type implementationMemory struct {
	memoryBackend
	work []workflow.ImplementationItem
}

func (b *implementationMemory) ImplementationItems(context.Context, workflow.RepositoryID) ([]workflow.ImplementationItem, error) {
	return append([]workflow.ImplementationItem(nil), b.work...), nil
}

func (b *implementationMemory) ClaimImplementation(_ context.Context, _ workflow.RepositoryID, number int) error {
	for i := range b.work {
		if b.work[i].Number == number {
			b.work[i].Claimed = true
		}
	}
	return nil
}

func implementCLI(t *testing.T, root string, backend *implementationMemory, args ...string) workflow.ImplementationOutcome {
	t.Helper()
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	command := append([]string{"skl", "implement"}, args...)
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

func TestImplementClaimsOldestEligibleWork(t *testing.T) {
	root := proposalRepository(t)
	backend := &implementationMemory{work: []workflow.ImplementationItem{
		{Number: 1, State: workflow.Ready, CreatedAt: "2020", Blockers: []int{9}},
		{Number: 2, State: workflow.Ready, CreatedAt: "2021"},
		{Number: 3, State: workflow.Rework, CreatedAt: "2023"},
		{Number: 5, State: workflow.Rework, CreatedAt: "2022"},
		{Number: 4, State: workflow.Rework, CreatedAt: "2022"},
		{Number: 6, State: workflow.Rework, CreatedAt: "2010", Claimed: true},
		{Number: 7, State: workflow.NeedsHuman, CreatedAt: "2010"},
		{Number: 9, State: workflow.ReadyForMerge},
	}}
	for _, want := range []int{4, 5, 3, 2} {
		got := implementCLI(t, root, backend, "next")
		if got.Status != "work_available" || got.Item.Number != want || !got.Item.Claimed {
			t.Fatalf("got %#v, want #%d", got, want)
		}
	}
	if backend.work[0].Claimed || backend.work[6].Claimed || backend.work[1].State != workflow.Ready {
		t.Fatalf("unexpected projections: %#v", backend.work)
	}
}

func TestImplementReportsNoEligibleWork(t *testing.T) {
	for _, work := range [][]workflow.ImplementationItem{nil, {
		{Number: 1, State: workflow.Ready, Blockers: []int{4}},
		{Number: 2, State: workflow.Rework, Claimed: true},
		{Number: 3, State: workflow.NeedsHuman},
		{Number: 4, State: workflow.ReadyForMerge},
	}} {
		backend := &implementationMemory{work: work}
		before := append([]workflow.ImplementationItem(nil), work...)
		got := implementCLI(t, proposalRepository(t), backend, "next")
		if got.Status != "no_work" || got.Item != nil || !reflect.DeepEqual(before, backend.work) {
			t.Fatalf("no-work mutated projections: %#v %#v", got, backend.work)
		}
	}
}

func TestImplementResumesInterruptedClaim(t *testing.T) {
	backend := &implementationMemory{work: []workflow.ImplementationItem{
		{Number: 1, State: workflow.Ready},
		{Number: 7, State: workflow.Ready, Claimed: true},
	}}
	got := implementCLI(t, proposalRepository(t), backend, "resume", "--item", "7")
	if got.Status != "work_available" || got.Item.Number != 7 || !got.Item.Claimed || backend.work[0].Claimed {
		t.Fatalf("resume: %#v %#v", got, backend.work)
	}
}
