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

func (b *implementationMemory) CoordinationItems(context.Context) ([]workflow.CoordinationItem, error) {
	return b.coordination, nil
}
func (b *implementationMemory) CloseCoordination(_ context.Context, id workflow.WorkItemID) error {
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

func TestStatusCompletesPartiallyProjectedReview(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework, Claimed: true, Submission: &workflow.Submission{ID: "11", Head: "fixed", PendingReview: workflow.Rework}}}}
	got := statusCLI(t, proposalRepository(t), b)
	if got.Items[0].Claimed || got.Items[0].Submission.PendingReview != "" || got.Items[0].State != workflow.Rework {
		t.Fatalf("partial review: %#v", got)
	}
}

// Model multiple guarded writes without changing the shared backend fake.
type statusGuardMemory struct {
	*implementationMemory
	beforeLaterGuard func()
}

func (b *statusGuardMemory) CompleteReview(ctx context.Context, item workflow.ImplementationItem, target workflow.State, guard func() error) error {
	if err := guard(); err != nil {
		return err
	}
	if b.beforeLaterGuard != nil {
		b.beforeLaterGuard()
	}
	return b.implementationMemory.CompleteReview(ctx, item, target, guard)
}

func TestStatusPendingPassRequiresMergeabilityAndRetainsRecovery(t *testing.T) {
	for _, stage := range []string{"entry", "transition", "later-guard"} {
		for _, mergeability := range []string{"unknown", "conflicting"} {
			if stage == "entry" && mergeability == "conflicting" {
				continue // Known conflicts use the existing Synchronization Rework path.
			}
			t.Run(stage+"/"+mergeability, func(t *testing.T) {
				root := proposalRepository(t)
				head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				b := &statusGuardMemory{implementationMemory: &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.ReadyForMerge, Claimed: true, Submission: &workflow.Submission{ID: "11", Head: head, ReviewedHead: head, Base: "main", State: workflow.ReadyForMerge, Claimed: true, PendingReview: workflow.ReadyForMerge, Mergeability: "mergeable", Bounces: 1}}}}}
				change := func() { b.work[0].Submission.Mergeability = mergeability }
				switch stage {
				case "entry":
					change()
				case "transition":
					b.beforeTransition = change
				case "later-guard":
					b.beforeLaterGuard = change
				}
				var output bytes.Buffer
				app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
				if err := app.Run([]string{"skl", "status", "--repo", root}); err != nil {
					t.Fatal(err)
				}
				var result setup.ImplementationOutput
				if err := json.Unmarshal(output.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Status != "fix_required" || !strings.Contains(result.Reason, "mergeability") {
					t.Fatalf("unsafe pending pass: %s", &output)
				}
				item := b.work[0]
				if !item.Claimed || !item.Submission.Claimed || item.Submission.PendingReview != workflow.ReadyForMerge || item.Submission.Head != head || item.Submission.ReviewedHead != head || item.Submission.Bounces != 1 {
					t.Fatalf("lost recovery state: %#v / %#v", item, item.Submission)
				}
				b.beforeTransition = nil
				b.beforeLaterGuard = nil
				b.work[0].Submission.Mergeability = "mergeable"
				got := statusCLI(t, root, b.implementationMemory).Items[0]
				if got.State != workflow.ReadyForMerge || got.Claimed || got.Submission.Claimed || got.Submission.PendingReview != "" || got.Submission.Bounces != 1 {
					t.Fatalf("retry did not finalize pass: %#v / %#v", got, got.Submission)
				}
			})
		}
	}
}

func TestStatusRefusesPendingPassAtReplacedHead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		verdict  string
		reviewed string
	}{
		{"no verdict record", "", "reviewed"},
		{"verdict record", "marker", "reviewed"},
		{"missing identity", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := proposalRepository(t)
			replacement := "replacement"
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.ReadyForMerge, Claimed: true, Submission: &workflow.Submission{ID: "11", Head: replacement, ReviewedHead: tc.reviewed, VerdictHead: tc.verdict, Base: "main", State: workflow.ReadyForMerge, Claimed: true, PendingReview: workflow.ReadyForMerge, Mergeability: "mergeable", Bounces: 1}}}}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
			if err := app.Run([]string{"skl", "status", "--repo", root}); err != nil {
				t.Fatal(err)
			}
			var result setup.ImplementationOutput
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Status != "fix_required" || !strings.Contains(result.Reason, "verdict-accepted head") {
				t.Fatalf("replaced head finalized pass: %s", &output)
			}
			item := b.work[0]
			if !item.Claimed || !item.Submission.Claimed || item.Submission.PendingReview != workflow.ReadyForMerge || item.Submission.Head != replacement || item.Submission.ReviewedHead != tc.reviewed || item.Submission.VerdictHead != tc.verdict || item.Submission.Bounces != 1 {
				t.Fatalf("lost recovery evidence: %#v / %#v", item, item.Submission)
			}
			accepted := tc.verdict
			if accepted == "" {
				accepted = tc.reviewed
			}
			if accepted == "" {
				return // Identity cannot be restored by fixing the head; the review must rerun.
			}
			b.work[0].Submission.Head = accepted
			got := statusCLI(t, root, b).Items[0]
			if got.State != workflow.ReadyForMerge || got.Claimed || got.Submission.Claimed || got.Submission.PendingReview != "" || got.Submission.Bounces != 1 {
				t.Fatalf("restored head did not finalize pass: %#v / %#v", got, got.Submission)
			}
		})
	}
}

func TestStatusRoutesAcceptedConflictToSynchronizationRework(t *testing.T) {
	root := proposalRepository(t)
	target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))
	for _, pending := range []workflow.State{"", workflow.ReadyForMerge} {
		b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.ReadyForMerge, Claimed: pending != "", Submission: &workflow.Submission{ID: "11", Head: "fixed", Base: "main", Mergeability: "conflicting", Bounces: 1, PendingReview: pending, Claimed: pending != ""}}}, remoteHeads: map[string]string{"main": target}}
		got := statusCLI(t, root, b).Items[0]
		if got.State != workflow.Rework || !got.Synchronization || got.TargetSnapshot != target || got.TargetBranch != "main" || got.Submission.Bounces != 1 || got.Claimed || got.Submission.Claimed || got.Submission.PendingReview != "" {
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
