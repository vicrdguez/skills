package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
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

func TestStatusPreservesApprovalRegardlessOfMergeability(t *testing.T) {
	root := proposalRepository(t)
	for _, mergeability := range []string{"conflicting", "unknown"} {
		for _, pending := range []workflow.State{"", workflow.ReadyForMerge} {
			t.Run(mergeability+"/"+string(pending), func(t *testing.T) {
				b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.ReadyForMerge, Claimed: pending != "", Submission: &workflow.Submission{ID: "11", Head: "fixed", Base: "main", Mergeability: mergeability, PendingReview: pending, Claimed: pending != ""}}}}
				b.work[0].Submission.ClaimAcquiredAt = "2026-01-01T00:00:01Z"
				b.work[0].Submission.Comments = []skilldist.ReviewComment{{ReviewNumber: 1, Verdict: "pass", Commit: "fixed", FinalHead: "fixed", CreatedAt: "2026-01-01T00:00:02Z"}}
				got := statusCLI(t, root, b).Items[0]
				if got.State != workflow.ReadyForMerge || got.Synchronization || got.Claimed || got.Submission.Claimed || got.Submission.PendingReview != "" {
					t.Fatalf("approval changed (pending %q): %#v / %#v", pending, got, got.Submission)
				}
			})
		}
	}
}

func TestStatusRefusesPendingPassForNonMainSubmission(t *testing.T) {
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.ReadyForMerge, Claimed: true, Submission: &workflow.Submission{ID: "11", Head: "fixed", Base: "release", PendingReview: workflow.ReadyForMerge, Claimed: true}}}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return b, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", proposalRepository(t)}); err != nil {
		t.Fatalf("repairable refusal exited with an error: %v", err)
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "fix_required" || !strings.Contains(result.Reason, "main") || !b.work[0].Claimed || b.work[0].State != workflow.ReadyForMerge {
		t.Fatalf("repairable status: %#v", result)
	}
}

func TestStatusRecoversOnlyTheCandidateAcceptedByInterruptedPassThroughGitHub(t *testing.T) {
	for _, tc := range []struct {
		name, failDelete, evidence string
		marker                     bool
	}{
		{"replaced during overlap", "review", "replaced", false},
		{"replaced after review cleanup", "wip", "replaced", false},
		{"unchanged reviewed head", "wip", "unchanged", false},
		{"unchanged final marker head", "review", "unchanged", true},
		{"replaced final marker head", "wip", "replaced", true},
		{"missing receipt", "review", "missing", false},
		{"receipt omits final head", "wip", "legacy", false},
		{"duplicate receipt", "wip", "duplicate", false},
		{"receipt predates claim", "wip", "stale", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture(t)
			packet := f.start(t, f.root).Packet
			if packet == nil {
				t.Fatal("review did not start")
			}
			final := f.head
			if tc.marker {
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "debt marker")
				final = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
				f.forge.head = final
			}
			f.forge.mergeable = false
			dir := packet.Facts.Watchdog.ResultDirectory
			summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md")
			if err := os.WriteFile(summary, []byte("accepted candidate"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(body, []byte("final body"), 0600); err != nil {
				t.Fatal(err)
			}
			f.forge.failDelete = tc.failDelete
			command := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--body", body}
			_, err := f.runResult(f.worktree, append(command, "--head", final)...)
			if err == nil || !slices.Contains(f.forge.labels, "done") || !slices.Contains(f.forge.labels, "wip") || slices.Contains(f.forge.labels, "review") != (tc.failDelete == "review") || len(f.forge.summaries) != 1 {
				t.Fatalf("pass was not interrupted at %s: %v labels=%v summaries=%v", tc.failDelete, err, f.forge.labels, f.forge.summaries)
			}
			switch tc.evidence {
			case "replaced":
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "unreviewed replacement")
				f.forge.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
			case "missing":
				f.forge.summaries = nil
			case "legacy":
				f.forge.summaries[0]["body"] = "<!-- skl.watchdog.review/v1\n{\"review_number\":1,\"verdict\":\"pass\"}\n-->\naccepted candidate"
			case "duplicate":
				f.forge.summaries = append(f.forge.summaries, f.forge.summaries[0])
			case "stale":
				f.forge.summaries[0]["submitted_at"] = "2025-01-01T00:00:00Z"
			}
			labels, writes := slices.Clone(f.forge.labels), f.forge.writes
			checkpoint := checkpointSnapshot(f.checkpoint)
			var output bytes.Buffer
			app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
				backend := setup.NewGitHubBackend(f.server.URL, "token", f.server.Client())
				backend.BindRepository(repository)
				return backend, nil
			}, bytes.NewReader(nil), &output, &output)
			if err := app.Run([]string{"skl", "status", "--repo", f.root}); err != nil {
				t.Fatalf("status: %v %s", err, &output)
			}
			if tc.evidence == "unchanged" {
				var got setup.StatusOutput
				if err := json.Unmarshal(output.Bytes(), &got); err != nil || got.Status != "observed" || len(got.Items) == 0 || got.Items[0].State != workflow.ReadyForMerge || got.Items[0].Claimed || !slices.Equal(f.forge.labels, []string{"done"}) {
					t.Fatalf("unchanged candidate did not recover: %v %s labels=%v", err, &output, f.forge.labels)
				}
			} else {
				var got setup.ImplementationOutput
				if err := json.Unmarshal(output.Bytes(), &got); err != nil || got.Status != "fix_required" || !strings.Contains(got.Reason, "interrupted pass") {
					t.Fatalf("unsafe recovery was not refused: %v %s", err, &output)
				}
				if !slices.Equal(f.forge.labels, labels) || f.forge.writes != writes || checkpointSnapshot(f.checkpoint) != checkpoint {
					t.Fatalf("refusal changed handoff: labels=%v writes=%d/%d checkpoint=%s", f.forge.labels, f.forge.writes, writes, checkpointSnapshot(f.checkpoint))
				}
				if tc.evidence == "replaced" {
					got := f.run(t, f.worktree, append(command, "--head", f.forge.head)...)
					if got.Status != "fix_required" || !strings.Contains(got.Reason, "recorded review differs") || f.forge.writes != writes || !slices.Equal(f.forge.labels, labels) || checkpointSnapshot(f.checkpoint) != checkpoint {
						t.Fatalf("retry substituted a different final head: %#v labels=%v writes=%d/%d", got, f.forge.labels, f.forge.writes, writes)
					}
				}
			}
			if readFile(t, summary) != "accepted candidate" || readFile(t, body) != "final body" || f.forge.body != "final body\n\nCloses #7\n" {
				t.Fatal("status discarded repair documents or published body")
			}
		})
	}
}
