package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	skilldist "github.com/vicrdguez/skills"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type implementationMemory struct {
	memoryBackend
	work         []workflow.ImplementationItem
	remoteHeads  map[string]string
	afterPublish func()
	decisions    map[int]string
}

func (b *implementationMemory) ImplementationTarget(context.Context, workflow.RepositoryID) (string, error) {
	return "main", nil
}

func (b *implementationMemory) PauseImplementation(_ context.Context, _ workflow.RepositoryID, item workflow.ImplementationItem, decision string) error {
	if b.decisions == nil {
		b.decisions = make(map[int]string)
	}
	b.decisions[item.Number] = decision
	for i := range b.work {
		if b.work[i].Number == item.Number {
			b.work[i].ResumeState = item.State
			b.work[i].State = workflow.NeedsHuman
			b.work[i].Claimed = false
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationHead(_ context.Context, _ workflow.RepositoryID, branch string) (string, error) {
	return b.remoteHeads[branch], nil
}

func (b *implementationMemory) PublishImplementation(_ context.Context, _ workflow.RepositoryID, item workflow.ImplementationItem, submission workflow.Submission) (workflow.Submission, error) {
	if submission.Number == 0 {
		submission.Number = 11
	}
	for i := range b.work {
		if b.work[i].Number == item.Number {
			b.work[i].Submission = &submission
		}
	}
	if b.afterPublish != nil {
		b.afterPublish()
	}
	return submission, nil
}

func TestImplementRefusesInvalidHandoff(t *testing.T) {
	for _, invariant := range []string{"Target Snapshot", "heads differ", "ledger", "Workflow State", "head changed"} {
		t.Run(invariant, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
			start := implementCLI(t, root, backend, "next")
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			if err := os.WriteFile(body, []byte("opaque\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if invariant != "ledger" {
				runGit(t, root, "rm", "-r", ".changes/widget")
				runGit(t, root, "commit", "-m", "retire")
			}
			backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			switch invariant {
			case "Target Snapshot":
				backend.work[0].TargetSnapshot = "deadbeef"
			case "heads differ":
				backend.remoteHeads["widget"] = "deadbeef"
			case "Workflow State":
				backend.work[0].State = workflow.NeedsHuman
			case "head changed":
				backend.afterPublish = func() { backend.remoteHeads["widget"] = "deadbeef" }
			}
			state := backend.work[0].State
			got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
			if got.Status != "fix_required" || !strings.Contains(got.Reason, invariant) || !backend.work[0].Claimed || backend.work[0].State != state {
				t.Fatalf("invalid handoff: %#v %#v", got, backend.work)
			}
			if _, err := os.Stat(body); err != nil {
				t.Fatalf("failed handoff removed prose: %v", err)
			}
		})
	}
}

func (b *implementationMemory) AwaitImplementationReview(_ context.Context, _ workflow.RepositoryID, item workflow.ImplementationItem) error {
	for i := range b.work {
		if b.work[i].Number == item.Number {
			b.work[i].State = workflow.AwaitingReview
			b.work[i].Claimed = false
		}
	}
	return nil
}

func (b *implementationMemory) ImplementationItems(context.Context, workflow.RepositoryID) ([]workflow.ImplementationItem, error) {
	return append([]workflow.ImplementationItem(nil), b.work...), nil
}

func (b *implementationMemory) ClaimImplementation(_ context.Context, _ workflow.RepositoryID, item workflow.ImplementationItem) error {
	for i := range b.work {
		if b.work[i].Number == item.Number {
			b.work[i].TargetSnapshot = item.TargetSnapshot
			b.work[i].TargetBranch = item.TargetBranch
			if item.Submission != nil {
				b.work[i].Submission = item.Submission
			}
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
	if result.Packet != nil && result.Packet.Facts.Implementation.ResultDirectory != "" {
		t.Cleanup(func() { os.RemoveAll(result.Packet.Facts.Implementation.ResultDirectory) })
	}
	return result
}

func TestImplementSubmitsCompletedFirstImplementation(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
	start := implementCLI(t, root, backend, "next")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(body, []byte("opaque audit [not even Markdown\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
	if got.Status != "awaiting_review" || backend.work[0].Claimed || backend.work[0].State != workflow.AwaitingReview {
		t.Fatalf("submit: %#v %#v", got, backend.work)
	}
	submission := backend.work[0].Submission
	if submission == nil || submission.Number != 11 || submission.Head != backend.remoteHeads["widget"] || submission.Body != "opaque audit [not even Markdown\n\n\nCloses #7\n" {
		t.Fatalf("submission = %#v", submission)
	}
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
	for i := range backend.work {
		item := &backend.work[i]
		item.Branch = fmt.Sprintf("slice-%d", item.Number)
		prepareSlice(t, root, item.Branch)
		if item.State == workflow.Rework {
			runGit(t, root, "rm", "-r", ".changes/"+item.Branch)
			runGit(t, root, "commit", "-m", "retire")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			item.Submission = &workflow.Submission{Number: item.Number + 100, Head: head, PreviousReviewedHead: head, Base: "main"}
		}
	}
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
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{
		{Number: 1, State: workflow.Ready},
		{Number: 7, Branch: "widget", State: workflow.Ready, Claimed: true},
	}}
	got := implementCLI(t, root, backend, "resume", "--item", "7")
	if got.Status != "work_available" || got.Item.Number != 7 || !got.Item.Claimed || backend.work[0].Claimed {
		t.Fatalf("resume: %#v %#v", got, backend.work)
	}
}

func TestImplementResumePreservesTargetAndRejectsAmbiguousHistory(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "origin/main"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready, Claimed: true}}}
	first := implementCLI(t, root, b, "resume", "--item", "7")
	if first.Packet == nil || b.work[0].TargetSnapshot != target {
		t.Fatalf("resume did not persist target: %#v", first)
	}
	runGit(t, root, "commit", "--allow-empty", "-m", "implementation")
	runGit(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	second := implementCLI(t, root, b, "resume", "--item", "7", "--target-snapshot", target)
	if second.Packet == nil || second.Packet.Facts.Implementation.TargetSnapshot != target {
		t.Fatalf("target moved: %#v", second)
	}
	wrong := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	if got := implementCLI(t, root, b, "resume", "--item", "7", "--target-snapshot", wrong); got.Status != "fix_required" {
		t.Fatalf("accepted changed obligation: %#v", got)
	}
	b.work[0].TargetSnapshot = ""
	if got := implementCLI(t, root, b, "resume", "--item", "7"); got.Status != "fix_required" {
		t.Fatalf("guessed after history changed: %#v", got)
	}
	b.work[0].Submission = &workflow.Submission{Number: 11, Draft: true, Base: "main"}
	got := implementCLI(t, root, b, "resume", "--item", "7", "--target-snapshot", target)
	if got.Packet == nil || got.Packet.Facts.Implementation.TargetSnapshot != target {
		t.Fatalf("draft suppressed target: %#v", got)
	}
}

func TestImplementPinsTargetAndBundlesInstructions(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "origin/main"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}}
	got := implementCLI(t, root, backend, "next")
	if got.Packet == nil || got.Packet.Facts.Implementation.TargetSnapshot != target || got.Packet.Facts.Implementation.ArtifactBaseline != baseline {
		t.Fatalf("packet = %#v", got)
	}
	if got.Packet.Skill != "implement" || !reflect.DeepEqual(got.Packet.IncludedSkills, []string{"tdd", "audit", "design", "domain"}) {
		t.Fatalf("manifest = %#v", got.Packet)
	}
	if !strings.Contains(got.Packet.Markdown(), "git merge "+target) || !strings.Contains(got.Packet.Facts.Implementation.ResumeCommand, target) {
		t.Fatalf("missing concrete Git/retry facts: %s", got.Packet.Markdown())
	}
	if strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")) != baseline {
		t.Fatal("Work Start changed Git")
	}
}

func TestImplementStartsFindingDrivenRework(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	comments := []skilldist.ReviewComment{{Body: "W1 BLOCK evidence", Author: "reviewer"}, {Body: "W1 NOTE reason", Author: "owner", Association: "OWNER"}}
	backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Rework, Submission: &workflow.Submission{Number: 11, Head: head, PreviousReviewedHead: head, Comments: comments}}}}
	got := implementCLI(t, root, backend, "next")
	if got.Status != "work_available" || got.Packet == nil {
		t.Fatalf("rework = %#v", got)
	}
	facts := got.Packet.Facts.Implementation
	if facts.Submission != 11 || facts.PreviousReviewedHead != head || !reflect.DeepEqual(facts.Comments, comments) || facts.TargetSnapshot != "" {
		t.Fatalf("facts = %#v", facts)
	}
	if !strings.Contains(got.Packet.Markdown(), head+"...HEAD") || strings.Contains(got.Packet.Markdown(), "git merge ") {
		t.Fatal("rework packet synchronizes target or lacks review fixed point")
	}
}

func TestImplementResubmitsExistingRework(t *testing.T) {
	for _, hasSubmission := range []bool{true, false} {
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")
		backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
		start := implementCLI(t, root, backend, "next")
		body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
		if err := os.WriteFile(body, []byte("current Audit and rework dispositions\n"), 0600); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "rm", "-r", ".changes/widget")
		runGit(t, root, "commit", "-m", "retire")
		head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		backend.work[0].State, backend.work[0].TargetSnapshot = workflow.Rework, ""
		if hasSubmission {
			backend.work[0].Submission = &workflow.Submission{Number: 42, Head: head, PreviousReviewedHead: head}
		}
		backend.remoteHeads["widget"] = head
		got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
		if hasSubmission {
			if got.Status != "awaiting_review" || backend.work[0].Submission.Number != 42 {
				t.Fatalf("rework = %#v", got)
			}
		} else if got.Status != "fix_required" || backend.work[0].Submission != nil {
			t.Fatalf("rework created new PR: %#v", got)
		}
	}
}

func TestImplementPausesBeforeCodeExists(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}}
	start := implementCLI(t, root, backend, "next")
	decision := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "decision.md")
	if err := os.WriteFile(decision, []byte("contradiction and recommendation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := implementCLI(t, root, backend, "needs-human", "--item", "7", "--reason", "contradictory_artifacts", "--decision", decision)
	if got.Status != "needs_human" || backend.work[0].Claimed || backend.work[0].ResumeState != workflow.Ready || backend.work[0].Submission != nil || backend.decisions[7] != "contradiction and recommendation\n" {
		t.Fatalf("pause: %#v %#v", got, backend)
	}
	if _, err := os.Stat(filepath.Join(root, ".changes/widget/intent.md")); err != nil {
		t.Fatal("pause retired incomplete ledger")
	}
}

func TestImplementPausesPreservingWork(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
	start := implementCLI(t, root, backend, "next")
	directory := start.Packet.Facts.Implementation.ResultDirectory
	for _, name := range []string{"submission.md", "decision.md"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name+" opaque\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("completed work\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-am", "implement")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend.remoteHeads["widget"] = head
	got := implementCLI(t, root, backend, "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", filepath.Join(directory, "decision.md"), "--body", filepath.Join(directory, "submission.md"))
	if got.Status != "needs_human" || got.Item.Submission == nil || !got.Item.Submission.Draft || got.Item.Submission.Head != head || got.Item.Claimed || got.Item.ResumeState != workflow.Ready {
		t.Fatalf("draft pause: %#v", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".changes/widget/intent.md")); err != nil {
		t.Fatal("draft retired incomplete artifacts")
	}
}

func TestImplementPublishesOpaqueResultAndCleansSuccessfulDirectory(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{}}
	start := implementCLI(t, root, backend, "next")
	directory := start.Packet.Facts.Implementation.ResultDirectory
	body := filepath.Join(directory, "submission.md")
	prose := "\x00[ broken Markdown\nVerdict: needs-human\n\n\n"
	if err := os.WriteFile(body, []byte(prose), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	backend.remoteHeads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	got := implementCLI(t, root, backend, "submit", "--item", "7", "--body", body)
	if got.Status != "awaiting_review" || backend.work[0].Submission.Body != prose+"\n\nCloses #7\n" {
		t.Fatalf("opaque publication = %#v", got)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("successful operation remains: %v", err)
	}
}

func TestImplementResumesConventionalWorktreeWithoutSelectingAnotherItem(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "switch", "main")
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "worktree", "add", worktree, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 1, Branch: "other", State: workflow.Ready}, {Number: 7, Branch: "widget", State: workflow.Ready, Claimed: true}}}
	got := implementCLI(t, worktree, backend, "resume")
	if got.Status != "work_available" || got.Item.Number != 7 || backend.work[0].Claimed {
		t.Fatalf("resume = %#v", got)
	}
	got = implementCLI(t, root, backend, "resume")
	if got.Status != "fix_required" || backend.work[0].Claimed {
		t.Fatalf("root resume claimed another item: %#v", got)
	}
}

func TestImplementInspectionSuppliesFixedLedgerEvidenceWithoutClaiming(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.Ready}}}
	got := implementCLI(t, root, backend, "inspect", "--item", "7")
	if got.Status != "inspected" || got.Ledger == nil || got.Ledger.Baseline != baseline || got.Head != baseline || backend.work[0].Claimed {
		t.Fatalf("inspection = %#v", got)
	}
}
