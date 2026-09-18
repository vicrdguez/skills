package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func TestWatchdogNextDeliversCompleteFirstReviewInMarkdown(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", Body: "Audit ledger: complete"},
	}}, remoteHeads: map[string]string{"widget": head}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "watchdog", "next", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{"name: watchdog", "Work Item #7", "Submission is #11", head, "Full Gate", "Artifact Baseline", "Audit ledger", "critical", "skl watchdog submit", "skl skill --resource reference/review.md", "human"} {
		if !strings.Contains(got, want) {
			t.Errorf("execution missing %q", want)
		}
	}
	if strings.HasPrefix(got, "{") || strings.Contains(got, "\"packet\"") {
		t.Fatalf("default output is a JSON packet: %s", got)
	}
	if !backend.work[0].Claimed {
		t.Fatal("selected Claim was not acquired")
	}
	worktree := filepath.Join(root, ".worktrees", "widget")
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("startup prepared a worktree: %v", err)
	}
}

func TestWatchdogInspectResolvesHistoricalReadsForFixedReview(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
	}}, remoteHeads: map[string]string{"widget": head}}
	start := watchdogCLI(t, root, backend, "next")
	facts := start.Packet.Facts.Watchdog
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "watchdog", "inspect", "--repo", facts.Worktree, "--item", "7", "--submission", "11", "--review-number", "1", "--reviewed-head", head, "--result-directory", facts.ResultDirectory}); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{"Work Item #7", head, "Artifact Baseline", "Artifact Completion", " show ", "intent.md", "behavior.md", "plan.md", "tasks.md", " diff ", "full"} {
		if !strings.Contains(got, want) {
			t.Errorf("inspection missing %q: %.600s", want, got)
		}
	}
}
