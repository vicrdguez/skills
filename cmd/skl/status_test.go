package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func statusCLI(t *testing.T, root string, b *implementationMemory) setup.StatusOutput {
	t.Helper()
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", root}); err != nil {
		t.Fatalf("status: %v %s", err, &output)
	}
	var result setup.StatusOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStatusCompletesCoordinationOnlyWhenEveryChildMerged(t *testing.T) {
	b := &implementationMemory{coordination: []workflow.CoordinationItem{{ID: "100", Children: []workflow.WorkItemID{"7", "8"}}}, work: []workflow.ImplementationItem{{ID: "7", State: workflow.Merged}, {ID: "8", State: workflow.ReadyForMerge}}}
	root := proposalRepository(t)
	if got := statusCLI(t, root, b); b.coordination[0].Closed || len(got.CompleteProposals) != 0 {
		t.Fatalf("premature completion: %#v", got)
	}
	b.work[1].State = workflow.Merged
	got := statusCLI(t, root, b)
	if !b.coordination[0].Closed || len(got.CompleteProposals) != 1 || got.CompleteProposals[0] != 100 {
		t.Fatalf("parent completion: %#v", got)
	}
}

func (b *implementationMemory) CoordinationItems(context.Context, github.RepositoryID) ([]workflow.CoordinationItem, error) {
	return b.coordination, nil
}
func (b *implementationMemory) CloseCoordination(_ context.Context, _ github.RepositoryID, id workflow.WorkItemID) error {
	for i := range b.coordination {
		if b.coordination[i].ID == id {
			b.coordination[i].Closed = true
		}
	}
	return nil
}

func TestStatusObservesHumanMergeAndReleasesDependencies(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "dependent")
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.ReadyForMerge, Submission: &workflow.Submission{ID: "11"}}, {ID: "8", Branch: "dependent", State: workflow.Ready, Blockers: []workflow.WorkItemID{"7"}}}}
	if got := implementCLI(t, root, b, "next"); got.Status != "no_work" {
		t.Fatalf("done released dependency: %#v", got)
	}
	b.work[0].Submission.Merged = true
	got := statusCLI(t, root, b)
	if len(got.Items) != 2 || got.Items[0].State != workflow.Merged {
		t.Fatalf("merge observation: %#v", got)
	}
	if got.Items[0].Number != 7 || got.Items[0].Submission.Number != 11 || got.Items[1].Number != 8 || !reflect.DeepEqual(got.Items[1].Blockers, []int{7}) {
		t.Fatalf("status lost numeric projections: %#v", got)
	}
	if next := implementCLI(t, root, b, "next"); next.Status != "work_available" || next.Item.Number != 8 {
		t.Fatalf("merge did not release dependency: %#v", next)
	}
}

func TestStatusNormalizesPartialAndContradictoryRecords(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{
		{ID: "1", State: workflow.Ready}, {ID: "2", State: workflow.Ready, Claimed: true},
		{ID: "3", State: workflow.AwaitingReview}, {ID: "4", State: workflow.Rework},
		{ID: "5", State: workflow.NeedsHuman, ResumeState: workflow.Rework}, {ID: "6", State: workflow.ReadyForMerge},
		{ID: "7", State: workflow.Merged}, {ID: "8", State: workflow.Superseded, Branch: "retained-reference"},
		{ID: "9", State: workflow.Ready, Claimed: true, Submission: &workflow.Submission{ID: "19", State: workflow.AwaitingReview, Head: "fixed"}},
		{ID: "10", State: workflow.Rework, Problem: "contradictory lifecycle projections"},
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

// Model multiple guarded writes without changing the shared backend fake.
type statusGuardMemory struct {
	*implementationMemory
	beforeLaterGuard func()
}

func (b *statusGuardMemory) CompleteReview(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem, target workflow.State, guard func() error) error {
	if err := guard(); err != nil {
		return err
	}
	if b.beforeLaterGuard != nil {
		b.beforeLaterGuard()
	}
	return b.implementationMemory.CompleteReview(ctx, repository, item, target, guard)
}

func TestStatusRoutesAcceptedConflictToSynchronizationRework(t *testing.T) {
	root := proposalRepository(t)
	target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))
	for _, pending := range []workflow.State{"", workflow.ReadyForMerge} {
		b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.ReadyForMerge, Claimed: pending != "", Submission: &workflow.Submission{ID: "11", Head: "fixed", Base: "main", Mergeability: "conflicting", PendingReview: pending, Claimed: pending != ""}}}, remoteHeads: map[string]string{"main": target}}
		got := statusCLI(t, root, b).Items[0]
		if got.State != workflow.Rework || !got.Synchronization || got.TargetSnapshot != target || got.TargetBranch != "main" || got.Claimed || got.Submission.Claimed || got.Submission.PendingReview != "" {
			t.Fatalf("accepted conflict (pending %q): %#v / %#v", pending, got, got.Submission)
		}
	}
}

func TestStatusReturnsStructuredRepairableRefusal(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.ReadyForMerge, Submission: &workflow.Submission{ID: "11", Head: "fixed", Base: "main", Mergeability: "conflicting"}}}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", proposalRepository(t)}); err != nil {
		t.Fatalf("repairable refusal exited with an error: %v", err)
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "fix_required" || !strings.Contains(result.Reason, "current target unavailable") || b.work[0].State != workflow.ReadyForMerge {
		t.Fatalf("repairable status: %#v", result)
	}
}
