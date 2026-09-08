package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
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
	if result.Packet != nil && result.Packet.Facts.Watchdog != nil {
		t.Cleanup(func() { os.RemoveAll(result.Packet.Facts.Watchdog.ResultDirectory) })
	}
	return result
}

func TestWatchdogBouncesFirstFailureWithOpaqueFindings(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: head}}}, remoteHeads: map[string]string{"widget": head}}
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	inline := filepath.Join(dir, "inline.md")
	anchors := filepath.Join(dir, "findings.json")
	for path, body := range map[string]string{summary: "\x00W1 [ malformed\nVerdict: pass\n", inline: "opaque inline", anchors: fmt.Sprintf(`[{"path":"main.go","line":12,"side":"RIGHT","body_file":%q}]`, inline)} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := watchdogCLI(t, root, b, "submit", "--item", "7", "--reviewed-head", head, "--verdict", "rework", "--summary", summary, "--findings", anchors)
	if got.Status != "rework" || got.Item.Claimed || got.Item.Submission.Number != 11 || got.Item.Submission.Bounces != 1 {
		t.Fatalf("bounce: %#v", got)
	}
	comments := got.Item.Submission.Comments
	if len(comments) != 2 || comments[0].Body != "\x00W1 [ malformed\nVerdict: pass\n" || comments[1].Body != "opaque inline" || comments[1].Line != 12 || comments[1].Commit != head {
		t.Fatalf("opaque findings: %#v", comments)
	}
}

func TestWatchdogPausesSecondFailure(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: head, Bounces: 1}}}, remoteHeads: map[string]string{"widget": head}}
	summary := filepath.Join(t.TempDir(), "summary.md")
	if err := os.WriteFile(summary, []byte("current W1 BLOCK"), 0600); err != nil {
		t.Fatal(err)
	}
	got := watchdogCLI(t, root, b, "submit", "--item", "7", "--reviewed-head", head, "--verdict", "rework", "--summary", summary)
	if got.Status != "needs_human" || got.Item.Claimed || got.Item.Submission.Bounces != 1 || got.Item.ResumeState != workflow.Rework || got.Item.Submission.Comments[0].Body != "current W1 BLOCK" {
		t.Fatalf("second failure: %#v", got)
	}
}

func TestWatchdogPassReachesHumanMergeBoundary(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: head, Base: "main", Mergeability: "mergeable"}}}, remoteHeads: map[string]string{"widget": head}}
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	body := filepath.Join(dir, "body.md")
	for path, content := range map[string]string{summary: "W1 NOTE opaque", body: "opaque final\n## Manual verification\n- [ ] human check\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := watchdogCLI(t, root, b, "submit", "--item", "7", "--reviewed-head", head, "--verdict", "pass", "--summary", summary, "--body", body)
	if got.Status != "ready_for_merge" || got.Item.Claimed || got.Item.Submission.Body != "opaque final\n## Manual verification\n- [ ] human check\n\n\nCloses #7\n" {
		t.Fatalf("pass: %#v", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".changes/archive")); !os.IsNotExist(err) {
		t.Fatalf("archive created: %v", err)
	}
}

func TestWatchdogConflictPinsSynchronizationReworkWithoutBounce(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "switch", "main")
	runGit(t, root, "commit", "--allow-empty", "-m", "target moved")
	target := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "switch", "widget")
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: head, Base: "main", Mergeability: "conflicting"}}}, remoteHeads: map[string]string{"widget": head, "main": target}}
	summary := filepath.Join(t.TempDir(), "summary.md")
	os.WriteFile(summary, []byte("pass"), 0600)
	got := watchdogCLI(t, root, b, "submit", "--item", "7", "--reviewed-head", head, "--verdict", "pass", "--summary", summary, "--body", summary)
	if got.Status != "rework" || !got.Item.Synchronization || got.Item.Submission.Bounces != 0 || got.Item.TargetSnapshot != target {
		t.Fatalf("conflict: %#v", got)
	}
	start := implementCLI(t, root, b, "next")
	if start.Packet == nil || start.Packet.Facts.Implementation.TargetSnapshot != target || !strings.Contains(start.Packet.Markdown(), "git merge "+target) || strings.Contains(start.Packet.Markdown(), "Finding-driven Rework: sync nothing") {
		t.Fatalf("synchronization packet: %#v", start)
	}
}

func TestWatchdogPassAcceptsPushedDebtMarkerHead(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	reviewed := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "debt.go"), []byte("package widget\n// DEBT(#11/W1): retained note\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "debt.go")
	runGit(t, root, "commit", "-m", "record debt")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: reviewed, Base: "main", Mergeability: "mergeable"}}}, remoteHeads: map[string]string{"widget": reviewed}}
	summary := filepath.Join(t.TempDir(), "summary.md")
	os.WriteFile(summary, []byte("opaque pass"), 0600)
	args := []string{"submit", "--item", "7", "--reviewed-head", reviewed, "--head", head, "--verdict", "pass", "--summary", summary, "--body", summary}
	if got := watchdogCLI(t, root, b, args...); got.Status != "fix_required" || !b.work[0].Claimed {
		t.Fatalf("accepted unpushed markers: %#v", got)
	}
	b.remoteHeads["widget"] = head
	got := watchdogCLI(t, root, b, args...)
	if got.Status != "ready_for_merge" || got.Head != head {
		t.Fatalf("post-marker pass: %#v", got)
	}
}

func TestWatchdogHumanDirectionRequiresExplicitRequeue(t *testing.T) {
	for _, resume := range []workflow.State{workflow.Rework, workflow.AwaitingReview} {
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")
		runGit(t, root, "rm", "-r", ".changes/widget")
		runGit(t, root, "commit", "-m", "retire")
		head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: head, PreviousReviewedHead: head}}}, remoteHeads: map[string]string{"widget": head}}
		summary := filepath.Join(t.TempDir(), "summary.md")
		os.WriteFile(summary, []byte("W1 HUMAN"), 0600)
		got := watchdogCLI(t, root, b, "submit", "--item", "7", "--reviewed-head", head, "--verdict", "needs-human", "--summary", summary)
		if got.Status != "needs_human" || got.Item.ResumeState != workflow.AwaitingReview {
			t.Fatalf("human verdict: %#v", got)
		}
		human := skilldist.ReviewComment{Body: "W1 resolved [no parsing\n", Association: "OWNER"}
		b.work[0].Submission.Comments = append(b.work[0].Submission.Comments, human)
		if got := watchdogCLI(t, root, b, "next"); got.Status != "no_work" {
			t.Fatalf("prose requeued: %#v", got)
		}
		b.work[0].State = resume
		var comments []skilldist.ReviewComment
		if resume == workflow.Rework {
			got = implementCLI(t, root, b, "next")
			if got.Packet != nil {
				comments = got.Packet.Facts.Implementation.Comments
			}
		} else {
			got = watchdogCLI(t, root, b, "next")
			if got.Packet != nil {
				comments = got.Packet.Facts.Watchdog.Comments
			}
		}
		if got.Status != "work_available" || !slices.Contains(comments, human) {
			t.Fatalf("human requeue %s: %#v", resume, got)
		}
	}
}

func TestWatchdogRetriesCompletedVerdictWithoutAnotherBounce(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: head}}}, remoteHeads: map[string]string{"widget": head}}
	summary := filepath.Join(t.TempDir(), "summary.md")
	os.WriteFile(summary, []byte("W1 BLOCK"), 0600)
	args := []string{"submit", "--item", "7", "--reviewed-head", head, "--verdict", "rework", "--summary", summary}
	for range 2 {
		got := watchdogCLI(t, root, b, args...)
		if got.Status != "rework" || got.Item.Submission.Bounces != 1 || len(got.Item.Submission.Comments) != 1 {
			t.Fatalf("retry: %#v", got)
		}
	}
	os.WriteFile(summary, []byte("different ledger"), 0600)
	if got := watchdogCLI(t, root, b, args...); got.Status != "fix_required" {
		t.Fatalf("accepted changed completed verdict: %#v", got)
	}
}

func (b *implementationMemory) ReviewSubmission(_ context.Context, _ workflow.RepositoryID, number int) (workflow.Submission, error) {
	for _, item := range b.work {
		if item.Submission != nil && item.Submission.Number == number {
			return *item.Submission, nil
		}
	}
	return workflow.Submission{}, fmt.Errorf("missing Submission")
}

func (b *implementationMemory) PublishReview(_ context.Context, _ workflow.RepositoryID, item workflow.ImplementationItem, comments []skilldist.ReviewComment, guard func() error) error {
	if err := guard(); err != nil {
		return err
	}
	for i := range b.work {
		if b.work[i].Number == item.Number {
			for _, c := range comments {
				if !slices.Contains(b.work[i].Submission.Comments, c) {
					b.work[i].Submission.Comments = append(b.work[i].Submission.Comments, c)
				}
			}
		}
	}
	return nil
}

func (b *implementationMemory) CompleteReview(_ context.Context, _ workflow.RepositoryID, item workflow.ImplementationItem, target workflow.State, guard func() error) error {
	if b.beforeTransition != nil {
		b.beforeTransition()
	}
	if err := guard(); err != nil {
		return err
	}
	for i := range b.work {
		if b.work[i].Number == item.Number {
			b.work[i].State = target
			b.work[i].ResumeState = item.ResumeState
			b.work[i].Synchronization = item.Synchronization
			b.work[i].TargetSnapshot = item.TargetSnapshot
			b.work[i].TargetBranch = item.TargetBranch
			b.work[i].Claimed = false
			b.work[i].Submission.State = target
			b.work[i].Submission.Claimed = false
			b.work[i].Submission.PendingReview = ""
			if target == workflow.Rework && !item.Synchronization {
				b.work[i].Submission.Bounces++
			}
		}
	}
	return nil
}

func TestWatchdogClaimsOldestSubmissionAndPinsPacket(t *testing.T) {
	root := proposalRepository(t)
	b := &implementationMemory{}
	for _, number := range []int{9, 7, 8} {
		branch := fmt.Sprintf("slice-%d", number)
		prepareSlice(t, root, branch)
		runGit(t, root, "rm", "-r", ".changes/"+branch)
		runGit(t, root, "commit", "-m", "retire")
		head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		b.work = append(b.work, workflow.ImplementationItem{Number: number, Branch: branch, State: workflow.AwaitingReview, CreatedAt: "2000", Submission: &workflow.Submission{Number: number + 100, Head: head, Body: "opaque audit", CreatedAt: "2026", Comments: []skilldist.ReviewComment{{Body: "W1 prior", Author: "reviewer"}, {Body: "raw human direction", Association: "OWNER"}}}})
	}
	got := watchdogCLI(t, root, b, "next")
	if got.Status != "work_available" || got.Item.Number != 7 || !got.Item.Claimed || got.Item.State != workflow.AwaitingReview {
		t.Fatalf("claim: %#v", got)
	}
	f := got.Packet.Facts.Watchdog
	if f.ReviewedHead != b.work[1].Submission.Head || f.ArtifactBaseline == "" || f.ArtifactCompletion == "" || f.AuditBody != "opaque audit" || !reflect.DeepEqual(f.Comments, b.work[1].Submission.Comments) {
		t.Fatalf("facts: %#v", f)
	}
	if got.Packet.Skill != "watchdog" || len(got.Packet.IncludedSkills) != 0 {
		t.Fatalf("review loaded Audit: %#v", got.Packet)
	}
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

func TestWatchdogResumesFixedClaim(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: head}}, {Number: 1, State: workflow.AwaitingReview}}}
	got := watchdogCLI(t, root, b, "resume", "--item", "7")
	if got.Status != "work_available" || got.Packet.Facts.Watchdog.ReviewedHead != head || b.work[1].Claimed {
		t.Fatalf("resume: %#v", got)
	}
	b.work[0].Submission.Head = strings.Repeat("f", 40)
	got = watchdogCLI(t, root, b, "resume", "--item", "7")
	if got.Status != "fix_required" || b.work[0].Submission.ReviewedHead != head {
		t.Fatalf("resume moved fixed point: %#v", got)
	}
}

func TestWatchdogPacketCarriesHistoricalContractAndSemanticHandoff(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Submission: &workflow.Submission{Number: 11, Head: head, Body: "opaque audit", Bounces: 1, Comments: []skilldist.ReviewComment{{Body: "W1 prior", Commit: baseline}}}}}}
	got := watchdogCLI(t, root, b, "next")
	f := got.Packet.Facts.Watchdog
	if f.BaselineFiles[".changes/widget/intent.md"] == "" || f.CompletionFiles[".changes/widget/intent.md"] == "" || f.Bounces != 1 || !strings.Contains(f.SubmitCommand, "--reviewed-head "+head) {
		t.Fatalf("concrete facts: %#v", f)
	}
	_, body, ok := strings.Cut(got.Packet.Instructions, "\n\n## Review Start\n")
	if !ok {
		t.Fatal("missing concrete review instructions")
	}
	normalized := strings.NewReplacer(f.Worktree, "<worktree>", f.ResultDirectory, "<result>", head, "<head>", baseline, "<baseline>").Replace("## Review Start\n" + body)
	if want := readRepositoryFile(t, "cmd/skl/testdata/watchdog-start.golden.md"); normalized != want {
		t.Fatalf("packet golden mismatch:\n%s", normalized)
	}
	for _, required := range []string{"Do not re-run `audit`", "formatter or parser", "git diff --check", "push", "--head", "W<n>", "test *strength*"} {
		if !strings.Contains(got.Packet.Instructions, required) {
			t.Errorf("missing preserved instruction %q", required)
		}
	}
	for _, stale := range []string{"docs/github.md", "Through the Board", "lands it"} {
		if strings.Contains(got.Packet.Instructions, stale) {
			t.Errorf("stale mechanics %q", stale)
		}
	}
}

func TestWatchdogCleansPrivateResultsOnlyAfterVerifiedHandoff(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Submission: &workflow.Submission{Number: 11, Head: head}}}, remoteHeads: map[string]string{}}
	start := watchdogCLI(t, root, b, "next")
	dir := start.Packet.Facts.Watchdog.ResultDirectory
	summary := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(summary, []byte("W1 BLOCK"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"submit", "--item", "7", "--reviewed-head", head, "--verdict", "rework", "--summary", summary}
	if got := watchdogCLI(t, root, b, args...); got.Status != "fix_required" {
		t.Fatalf("accepted unpushed head: %#v", got)
	}
	if _, err := os.Stat(summary); err != nil {
		t.Fatal("failure removed Result Document")
	}
	b.remoteHeads["widget"] = head
	if got := watchdogCLI(t, root, b, args...); got.Status != "rework" {
		t.Fatalf("handoff: %#v", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("successful private results retained: %v", err)
	}
}

func TestWatchdogRefusesPassWhenMergeabilityChangesDuringPublication(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 7, Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{Number: 11, Head: head, ReviewedHead: head, Mergeability: "mergeable"}}}, remoteHeads: map[string]string{"widget": head}}
	b.beforeTransition = func() { b.work[0].Submission.Mergeability = "conflicting" }
	summary := filepath.Join(t.TempDir(), "summary.md")
	os.WriteFile(summary, []byte("pass"), 0600)
	got := watchdogCLI(t, root, b, "submit", "--item", "7", "--reviewed-head", head, "--verdict", "pass", "--summary", summary, "--body", summary)
	if got.Status != "fix_required" || !b.work[0].Claimed || b.work[0].State != workflow.AwaitingReview {
		t.Fatalf("accepted late conflict: %#v", got)
	}
}

func TestWatchdogOrdersEligibleSubmissionsDespiteUnrelatedItems(t *testing.T) {
	root := proposalRepository(t)
	b := &implementationMemory{work: []workflow.ImplementationItem{{Number: 1, Branch: "newer", State: workflow.AwaitingReview, CreatedAt: "2000"}, {Number: 2, State: workflow.Ready}, {Number: 3, Branch: "older", State: workflow.AwaitingReview, CreatedAt: "2026"}}}
	for _, index := range []int{0, 2} {
		item := &b.work[index]
		prepareSlice(t, root, item.Branch)
		runGit(t, root, "rm", "-r", ".changes/"+item.Branch)
		runGit(t, root, "commit", "-m", "retire")
		head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		date := "2026"
		if index == 2 {
			date = "2025"
		}
		item.Submission = &workflow.Submission{Number: 11 + index, Head: head, CreatedAt: date}
	}
	got := watchdogCLI(t, root, b, "next")
	if got.Item == nil || got.Item.Number != 3 {
		t.Fatalf("not oldest eligible Submission: %#v", got)
	}
}
