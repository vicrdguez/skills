package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func statusCLI(t *testing.T, root string, b *implementationMemory) workflow.StatusOutcome {
	t.Helper()
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", root}); err != nil {
		t.Fatalf("status: %v %s", err, &output)
	}
	var result workflow.StatusOutcome
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStatusCompletesCoordinationOnlyWhenEveryChildMerged(t *testing.T) {
	b := &implementationMemory{memoryBackend: memoryBackend{parents: []workflow.CoordinationItem{{Number: 100, Children: []int{7, 8}}}}, work: []workflow.ImplementationItem{{Number: 7, State: workflow.Merged}, {Number: 8, State: workflow.ReadyForMerge}}}
	root := proposalRepository(t)
	if got := statusCLI(t, root, b); b.parents[0].Closed || len(got.CompleteProposals) != 0 {
		t.Fatalf("premature completion: %#v", got)
	}
	b.work[1].State = workflow.Merged
	got := statusCLI(t, root, b)
	if !b.parents[0].Closed || len(got.CompleteProposals) != 1 || got.CompleteProposals[0] != 100 {
		t.Fatalf("parent completion: %#v", got)
	}
}

func (b *implementationMemory) CoordinationItems(context.Context, workflow.RepositoryID) ([]workflow.CoordinationItem, error) {
	return b.parents, nil
}
func (b *implementationMemory) CloseCoordination(_ context.Context, _ workflow.RepositoryID, number int) error {
	for i := range b.parents {
		if b.parents[i].Number == number {
			b.parents[i].Closed = true
		}
	}
	return nil
}

func TestStatusObservesHumanMergeAndReleasesDependencies(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "dependent")
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.ReadyForMerge, Submission: &workflow.Submission{Number: 11}}, {Number: 8, Branch: "dependent", State: workflow.Ready, Blockers: []int{7}}}}
	if got := implementCLI(t, root, b, "next"); got.Status != "no_work" {
		t.Fatalf("done released dependency: %#v", got)
	}
	b.work[0].Submission.Merged = true
	got := statusCLI(t, root, b)
	if len(got.Items) != 2 || got.Items[0].State != workflow.Merged {
		t.Fatalf("merge observation: %#v", got)
	}
	if next := implementCLI(t, root, b, "next"); next.Status != "work_available" || next.Item.Number != 8 {
		t.Fatalf("merge did not release dependency: %#v", next)
	}
}

func TestStatusNormalizesPartialAndContradictoryRecords(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{
		{Number: 1, State: workflow.Ready}, {Number: 2, State: workflow.Ready, Claimed: true},
		{Number: 3, State: workflow.AwaitingReview}, {Number: 4, State: workflow.Rework},
		{Number: 5, State: workflow.NeedsHuman, ResumeState: workflow.Rework}, {Number: 6, State: workflow.ReadyForMerge},
		{Number: 7, State: workflow.Merged}, {Number: 8, State: workflow.Superseded, Branch: "retained-reference"},
		{Number: 9, State: workflow.Ready, Claimed: true, Submission: &workflow.Submission{Number: 19, State: workflow.AwaitingReview, Head: "fixed"}},
		{Number: 10, State: workflow.Rework, Problem: "contradictory lifecycle projections"},
	}}
	before := append([]workflow.ImplementationItem(nil), b.work...)
	got := statusCLI(t, proposalRepository(t), b)
	if !reflect.DeepEqual(before[:8], b.work[:8]) || !reflect.DeepEqual(before[9], b.work[9]) {
		t.Fatal("status mutated valid or contradictory state")
	}
	if got.Items[8].State != workflow.AwaitingReview || got.Items[8].Claimed || got.Items[9].State != workflow.NeedsHuman || got.Items[7].Branch != "retained-reference" {
		t.Fatalf("normalized status: %#v", got)
	}
}

func TestStatusCompletesPartiallyProjectedReview(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Rework, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: "fixed", PendingReview: workflow.Rework}}}}
	got := statusCLI(t, proposalRepository(t), b)
	if got.Items[0].Claimed || got.Items[0].Submission.PendingReview != "" || got.Items[0].State != workflow.Rework {
		t.Fatalf("partial review: %#v", got)
	}
}

func TestStatusRoutesAcceptedConflictToSynchronizationRework(t *testing.T) {
	root := proposalRepository(t)
	target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.ReadyForMerge, Submission: &workflow.Submission{Number: 11, Head: "fixed", Base: "main", Mergeability: "conflicting", Bounces: 1}}}, remoteHeads: map[string]string{"main": target}}
	got := statusCLI(t, root, b)
	if got.Items[0].State != workflow.Rework || !got.Items[0].Synchronization || got.Items[0].TargetSnapshot != target || got.Items[0].Submission.Bounces != 1 {
		t.Fatalf("accepted conflict: %#v", got)
	}
}

func TestStatusReturnsStructuredRepairableRefusal(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.ReadyForMerge, Submission: &workflow.Submission{Number: 11, Head: "fixed", Base: "main", Mergeability: "conflicting"}}}}
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", proposalRepository(t)}); err != nil {
		t.Fatalf("repairable refusal exited with an error: %v", err)
	}
	var result workflow.ImplementationOutcome
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "fix_required" || !strings.Contains(result.Reason, "current target unavailable") || b.work[0].State != workflow.ReadyForMerge {
		t.Fatalf("repairable status: %#v", result)
	}
}
