package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
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
	for _, want := range []string{"name: watchdog", "Work Item #7", "Submission is #11", head, "Full Gate", "Artifact Baseline", "Audit ledger", "critical", "skl watchdog submit", "skl skill --resource reference/acceptance.md audit", "skl skill --resource reference/review.md", "complete frozen contract", "additional executable challenges", "Post-Marker Check", "Manual verification", "--head <actual-pushed-final-SHA>", "human"} {
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

type failedWatchdogQueue struct{ *implementationMemory }

func (*failedWatchdogQueue) QueuePage(context.Context, workflow.QueueKind, string) (workflow.QueuePage, error) {
	return workflow.QueuePage{}, errors.New("backend unavailable")
}

type uncertainWatchdogClaim struct{ *implementationMemory }

func (*uncertainWatchdogClaim) ClaimSelected(context.Context, workflow.QueueCandidate, workflow.ImplementationItem) (workflow.ImplementationItem, error) {
	return workflow.ImplementationItem{}, errors.New("read-back unavailable")
}

type selectedOnlyWatchdogInspection struct{ *implementationMemory }

func (*selectedOnlyWatchdogInspection) ImplementationItems(context.Context) ([]workflow.ImplementationItem, error) {
	return nil, errors.New("global Work Item scan is unavailable")
}

func TestWatchdogInspectReadsOnlySelectedWorkItem(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	memory := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
	}}, remoteHeads: map[string]string{"widget": head}}
	start := watchdogCLI(t, root, memory, "next")
	facts := start.Packet.Facts.Watchdog
	backend := &selectedOnlyWatchdogInspection{implementationMemory: memory}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	err := app.Run([]string{"skl", "watchdog", "inspect", "--repo", facts.Worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", fmt.Sprintf("%x", sha256.Sum256(nil)), "--review-number", "1", "--reviewed-head", head, "--result-directory", facts.ResultDirectory})
	if err != nil || !strings.Contains(output.String(), "# Watchdog Inspection Continuation") {
		t.Fatalf("selected inspection used a global scan: %v, %s", err, &output)
	}
}

func TestWatchdogInspectAcceptsUnchangedZeroCountCheckpoint(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "switch", "main")
	runGit(t, root, "worktree", "add", worktree, "widget")
	gitDir := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "--absolute-git-dir"))
	if err := os.WriteFile(filepath.Join(gitDir, ".watchdog"), []byte("0:"+head+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
	}}, remoteHeads: map[string]string{"widget": head}}
	started := watchdogCLI(t, root, backend, "next")
	facts := started.Packet.Facts.Watchdog
	if facts.ReviewCount != 0 || facts.ReviewNumber != 1 || facts.PreviousReviewedHead != "" {
		t.Fatalf("zero-count checkpoint changed startup facts: %#v", facts)
	}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	args := []string{"skl", "watchdog", "inspect", "--repo", facts.Worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", facts.SubmissionBodySHA256, "--review-number", "1", "--reviewed-head", head, "--result-directory", facts.ResultDirectory}
	if err := app.Run(args); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "# Watchdog Inspection Continuation") || !strings.Contains(got, "first full PR comparison") || strings.Contains(got, "Status: fix_required") {
		t.Fatalf("unchanged zero-count checkpoint did not inspect successfully: %s", got)
	}
	if data, err := os.ReadFile(filepath.Join(gitDir, ".watchdog")); err != nil || string(data) != "0:"+head+"\n" {
		t.Fatalf("inspection changed the zero-count checkpoint: %q, %v", data, err)
	}
}

func TestWatchdogClaimReadbackFailureRequiresIdentityInspection(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &uncertainWatchdogClaim{implementationMemory: &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
	}}}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	err := app.Run([]string{"skl", "watchdog", "next", "--repo", root})
	if err == nil || !strings.Contains(err.Error(), "Claim state is uncertain") || !strings.Contains(err.Error(), "Work Item 7") || !strings.Contains(err.Error(), "resume") || output.Len() != 0 {
		t.Fatalf("uncertain Claim was presented as success: error=%v output=%s", err, &output)
	}
}

func TestWatchdogQueueFailureStatesNoClaimWasAttempted(t *testing.T) {
	root := proposalRepository(t)
	backend := &failedWatchdogQueue{implementationMemory: &implementationMemory{}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	err := app.Run([]string{"skl", "watchdog", "next", "--repo", root})
	if err == nil || !strings.Contains(err.Error(), "before Claim acquisition") || !strings.Contains(err.Error(), "no Claim acquisition was attempted") || output.Len() != 0 {
		t.Fatalf("queue failure hid Claim certainty: error=%v output=%s", err, &output)
	}
}

func TestWatchdogInspectRefusesChangedSubmissionBody(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", Body: "original evidence"},
	}}, remoteHeads: map[string]string{"widget": head}}
	started := watchdogCLI(t, root, backend, "next")
	facts := started.Packet.Facts.Watchdog
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("original evidence")))
	if !strings.Contains(facts.InspectCommand, "--submission-body-sha256 '"+digest+"'") || !strings.Contains(facts.InspectCommand, "--base 'main'") {
		t.Fatalf("inspection command did not bind the selected attachment: %s", facts.InspectCommand)
	}
	backend.work[0].Submission.Body = "changed evidence"
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	err := app.Run([]string{"skl", "watchdog", "inspect", "--repo", facts.Worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", digest, "--review-number", "1", "--reviewed-head", head, "--result-directory", facts.ResultDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Status: fix_required") || !strings.Contains(output.String(), "Submission") || !strings.Contains(output.String(), "--submission-body-sha256 '"+digest+"'") || !strings.Contains(output.String(), facts.ResultDirectory) {
		t.Fatalf("changed attachment was accepted: %s", &output)
	}
}

func TestWatchdogInspectRefusesHeadAndRoundDrift(t *testing.T) {
	for _, drift := range []string{"PR head", "review checkpoint"} {
		t.Run(drift, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			backend := &implementationMemory{work: []workflow.ImplementationItem{{
				ID: "7", Branch: "widget", State: workflow.AwaitingReview,
				Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
			}}, remoteHeads: map[string]string{"widget": head}}
			started := watchdogCLI(t, root, backend, "next")
			facts := started.Packet.Facts.Watchdog
			if drift == "PR head" {
				backend.work[0].Submission.Head = strings.Repeat("a", 40)
			} else {
				gitDir := strings.TrimSpace(runGitOutput(t, facts.Worktree, "rev-parse", "--absolute-git-dir"))
				if err := os.WriteFile(filepath.Join(gitDir, ".watchdog"), []byte("1:"+head+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			args := []string{"skl", "watchdog", "inspect", "--repo", facts.Worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", fmt.Sprintf("%x", sha256.Sum256(nil)), "--review-number", "1", "--reviewed-head", head, "--result-directory", facts.ResultDirectory}
			if err := app.Run(args); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "Status: fix_required") || !strings.Contains(output.String(), facts.ResultDirectory) || !strings.Contains(output.String(), "--reviewed-head '"+head+"'") || strings.Contains(output.String(), "git -C") {
				t.Fatalf("drift substituted a new review: %s", &output)
			}
		})
	}
}

func TestWatchdogSubmitRefusesReplacementSubmission(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", Body: "original evidence"},
	}}, remoteHeads: map[string]string{"widget": head}}
	started := watchdogCLI(t, root, backend, "next")
	facts := started.Packet.Facts.Watchdog
	if !strings.Contains(facts.SubmitCommand, "--submission 11") || !strings.Contains(facts.SubmitCommand, "--base 'main'") || !strings.Contains(facts.SubmitCommand, "--submission-body-sha256 '") {
		t.Fatalf("submit command did not bind the original attachment: %s", facts.SubmitCommand)
	}
	backend.work[0].Submission.ID = "68"
	summary := filepath.Join(t.TempDir(), "summary.md")
	if err := os.WriteFile(summary, []byte("W1 BLOCK\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	args := []string{"skl", "watchdog", "submit", "--repo", facts.Worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", facts.SubmissionBodySHA256, "--review-number", "1", "--reviewed-head", head, "--verdict", "rework", "--summary", summary}
	if err := app.Run(args); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Status: fix_required") || !strings.Contains(output.String(), "selected Submission") || !backend.work[0].Claimed {
		t.Fatalf("replacement Submission was not refused: %s; item=%#v", output.String(), backend.work[0])
	}
	if len(backend.work[0].Submission.Comments) != 0 {
		t.Fatalf("replacement refusal published review evidence: %#v", backend.work[0].Submission.Comments)
	}
	backend.work[0].Submission.ID = "11"
	output.Reset()
	if err := app.Run(args); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Status: rework") || backend.work[0].Claimed {
		t.Fatalf("authorized retry with the original Submission failed: %s; item=%#v", output.String(), backend.work[0])
	}
}

func TestWatchdogSubmitExplainsVerifiedReadyForHumanMerge(t *testing.T) {
	for _, test := range []struct {
		name      string
		interrupt bool
		drift     bool
		rollback  bool
	}{
		{name: "fixed attachment pass"},
		{name: "fixed attachment retry after final body publication", interrupt: true},
		{name: "unrelated body drift", drift: true},
		{name: "final body rollback", rollback: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			backend := &implementationMemory{work: []workflow.ImplementationItem{{
				ID: "7", Branch: "widget", State: workflow.AwaitingReview, Claimed: true,
				Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", Mergeability: "conflicting"},
			}}, remoteHeads: map[string]string{"widget": head}}
			worktree := filepath.Join(root, ".worktrees", "widget")
			runGit(t, root, "switch", "main")
			runGit(t, root, "worktree", "add", worktree, "widget")
			dir := t.TempDir()
			summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md")
			for path, content := range map[string]string{summary: "no findings\n", body: "final PR body\n"} {
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			args := []string{"skl", "watchdog", "submit", "--repo", worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", fmt.Sprintf("%x", sha256.Sum256(nil)), "--review-number", "1", "--reviewed-head", head, "--verdict", "pass", "--summary", summary, "--body", body}
			if test.drift {
				backend.work[0].Submission.Body = "unrelated body\n"
			}
			if test.interrupt {
				backend.afterPublish = func() { backend.remoteHeads["widget"] = "deadbeef" }
			}
			if test.rollback {
				backend.beforeReviewSubmission = func(call int) {
					if call == 4 {
						backend.work[0].Submission.Body = ""
					}
				}
			}
			err := app.Run(args)
			if test.drift || test.rollback {
				stage := "before publication"
				if test.rollback {
					stage = "after final body publication"
				}
				if err != nil || !strings.Contains(output.String(), "Status: fix_required") || !strings.Contains(output.String(), "attachment changed") || !backend.work[0].Claimed {
					t.Fatalf("unrelated body drift was not refused %s: err=%v; output=%s; item=%#v", stage, err, &output, backend.work[0])
				}
				if test.drift && len(backend.work[0].Submission.Comments) != 0 {
					t.Fatalf("pre-publication body drift published review evidence: %#v", backend.work[0].Submission.Comments)
				}
				return
			}
			if !test.interrupt && err != nil {
				t.Fatal(err)
			}
			if test.interrupt {
				if err != nil || !strings.Contains(output.String(), "Status: fix_required") || !strings.Contains(output.String(), "remote reviewed head changed") || !backend.work[0].Claimed || backend.work[0].Submission.Body != "final PR body\n\n\nCloses #7\n" {
					t.Fatalf("interrupted pass did not retain its Claim and authorized final body: err=%v; output=%s; body=%q; item=%#v", err, &output, backend.work[0].Submission.Body, backend.work[0])
				}
				backend.afterPublish = nil
				backend.remoteHeads["widget"] = head
				output.Reset()
				if err := app.Run(args); err != nil {
					t.Fatal(err)
				}
			}
			got := output.String()
			if !strings.Contains(got, "Status: ready_for_merge") || !strings.Contains(got, "human") || strings.HasPrefix(got, "{") || backend.work[0].Claimed {
				t.Fatalf("verified outcome was not explained in Markdown: %s; item=%#v", got, backend.work[0])
			}
		})
	}
}

func TestWatchdogMarkdownConvergenceOutcomes(t *testing.T) {
	for _, test := range []struct {
		name    string
		count   uint64
		verdict string
		want    string
	}{
		{name: "first pass", verdict: "pass", want: "ready_for_merge"},
		{name: "later pass", count: 2, verdict: "pass", want: "ready_for_merge"},
		{name: "first rework", verdict: "rework", want: "rework"},
		{name: "second rework", count: 1, verdict: "rework", want: "needs_human"},
		{name: "human decision", verdict: "needs-human", want: "needs_human"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			backend := &implementationMemory{work: []workflow.ImplementationItem{{
				ID: "7", Branch: "widget", State: workflow.AwaitingReview, Claimed: true,
				Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", Mergeability: "conflicting"},
			}}, remoteHeads: map[string]string{"widget": head}}
			worktree := filepath.Join(root, ".worktrees", "widget")
			runGit(t, root, "switch", "main")
			runGit(t, root, "worktree", "add", worktree, "widget")
			if test.count > 0 {
				gitDir := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "--absolute-git-dir"))
				if err := os.WriteFile(filepath.Join(gitDir, ".watchdog"), []byte(fmt.Sprintf("%d:%s\n", test.count, head)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			dir := t.TempDir()
			summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md")
			for path, content := range map[string]string{summary: "review summary\n", body: "complete PR body\n"} {
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"skl", "watchdog", "submit", "--repo", worktree, "--item", "7", "--review-number", fmt.Sprint(test.count + 1), "--reviewed-head", head, "--verdict", test.verdict, "--summary", summary}
			if test.verdict == "pass" {
				args = append(args, "--body", body)
			}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			if err := app.Run(args); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "Status: "+test.want) || backend.work[0].Claimed {
				t.Fatalf("verified convergence was not reported: %s; state=%#v", &output, backend.work[0])
			}
		})
	}
}

func TestWatchdogMarkdownRefusalPreservesClaimAndReviewDocuments(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview, Claimed: true,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "feature"},
	}}, remoteHeads: map[string]string{"widget": head}}
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "switch", "main")
	runGit(t, root, "worktree", "add", worktree, "widget")
	summary := filepath.Join(t.TempDir(), "summary.md")
	if err := os.WriteFile(summary, []byte("W1 BLOCK\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "watchdog", "submit", "--repo", worktree, "--item", "7", "--review-number", "1", "--reviewed-head", head, "--verdict", "rework", "--summary", summary}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Status: fix_required") || !strings.Contains(output.String(), "main") || !strings.Contains(output.String(), "--reviewed-head '"+head+"'") || !strings.Contains(output.String(), summary) || !backend.work[0].Claimed {
		t.Fatalf("repairable refusal was not precise: %s; claim=%v", &output, backend.work[0].Claimed)
	}
	if data, err := os.ReadFile(summary); err != nil || string(data) != "W1 BLOCK\n" {
		t.Fatalf("review document was lost: %q, %v", data, err)
	}
	var typed bytes.Buffer
	app = newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &typed, &typed)
	if err := app.Run([]string{"skl", "watchdog", "submit", "--repo", worktree, "--item", "7", "--review-number", "1", "--reviewed-head", head, "--verdict", "rework", "--summary", summary, "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	var refusal setup.ImplementationOutput
	if err := json.Unmarshal(typed.Bytes(), &refusal); err != nil || refusal.Status != "fix_required" || !strings.Contains(output.String(), refusal.Reason) || !backend.work[0].Claimed {
		t.Fatalf("submit transports disagreed or changed the Claim: %v, %#v", err, refusal)
	}
}

func TestPlainWatchdogSkillRetrievalRequiresSelectedWork(t *testing.T) {
	for _, format := range []string{"markdown", "json"} {
		t.Run(format, func(t *testing.T) {
			var output bytes.Buffer
			backendCalls := 0
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { backendCalls++; return nil, nil }, bytes.NewReader(nil), &output, &output)
			err := app.Run([]string{"skl", "skill", "--format", format, "watchdog"})
			if err == nil || !strings.Contains(err.Error(), "skl watchdog next") || !strings.Contains(err.Error(), "skl watchdog resume") {
				t.Fatalf("generic Watchdog retrieval was not refused: %v", err)
			}
			if backendCalls != 0 || output.Len() != 0 {
				t.Fatalf("refusal selected work or rendered instructions: calls=%d output=%s", backendCalls, &output)
			}
		})
	}
}

func TestInstallDirectWatchdogStubsForSupportedHarnesses(t *testing.T) {
	home := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return nil, nil }, bytes.NewReader(nil), &output, &output, home)
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		path := filepath.Join(home, harness, "watchdog", "SKILL.md")
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), "skl watchdog next") || strings.Contains(string(contents), "skl skill watchdog") {
			t.Errorf("Watchdog stub is not direct: %s", path)
		}
	}
}

func TestWatchdogReviewHistorySelectsApplicableComparison(t *testing.T) {
	for _, test := range []struct {
		name     string
		count    uint64
		previous string
		advance  bool
		diverged bool
		wantFull bool
	}{
		{name: "ancestor", count: 1, previous: "head", advance: true},
		{name: "same head", count: 1, previous: "head"},
		{name: "missing previous", count: 2, previous: strings.Repeat("b", 40), wantFull: true},
		{name: "nonancestor previous", count: 2, diverged: true, wantFull: true},
		{name: "lost checkpoint with findings", count: 0, wantFull: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			previous := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			if test.previous != "head" {
				previous = test.previous
			}
			if test.advance {
				runGit(t, root, "commit", "--allow-empty", "-m", "rework")
			}
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			runGit(t, root, "switch", "main")
			if test.diverged {
				runGit(t, root, "commit", "--allow-empty", "-m", "other review history")
				previous = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			}
			worktree := filepath.Join(root, ".worktrees", "widget")
			runGit(t, root, "worktree", "add", worktree, "widget")
			if test.count > 0 {
				gitDir := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "--absolute-git-dir"))
				if err := os.WriteFile(filepath.Join(gitDir, ".watchdog"), []byte(fmt.Sprintf("%d:%s\n", test.count, previous)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			backend := &implementationMemory{work: []workflow.ImplementationItem{{
				ID: "7", Branch: "widget", State: workflow.AwaitingReview,
				Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", Comments: []skilldist.ReviewComment{{Body: "W9 NOTE", Author: "reviewer"}}},
			}}, remoteHeads: map[string]string{"widget": head}}
			started := watchdogCLI(t, root, backend, "next")
			facts := started.Packet.Facts.Watchdog
			if facts.ReviewCount != test.count || facts.ReviewNumber != test.count+1 || facts.PreviousReviewedHead != previous {
				t.Fatalf("history was not retained: %#v", facts)
			}
			if !strings.Contains(started.Packet.Instructions, "W9 NOTE") {
				t.Fatal("supplied finding history was lost")
			}
			args := []string{"skl", "watchdog", "inspect", "--repo", worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", fmt.Sprintf("%x", sha256.Sum256(nil)), "--review-number", fmt.Sprint(facts.ReviewNumber), "--reviewed-head", head, "--result-directory", facts.ResultDirectory}
			if previous != "" {
				args = append(args, "--previous-reviewed-head", previous)
			}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			if err := app.Run(args); err != nil {
				t.Fatal(err)
			}
			got := output.String()
			if test.wantFull {
				if !strings.Contains(got, "full PR comparison") || previous != "" && strings.Contains(got, previous+"..."+head) {
					t.Fatalf("full fallback was not selected: %.1000s", got)
				}
			} else if !strings.Contains(got, previous+"..."+head) || !strings.Contains(got, "incremental comparison") {
				t.Fatalf("incremental comparison was not selected: %.1000s", got)
			}
		})
	}
}

func TestWatchdogResumePreservesLocalProgressAndEndpointOverrides(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	baseline := strings.TrimSpace(runGitOutput(t, root, "log", "--format=%H", "--fixed-strings", "--grep=[baseline] widget", "-1"))
	completion := strings.TrimSpace(runGitOutput(t, root, "log", "--format=%H", "--fixed-strings", "--grep=[completion] widget", "-1"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview, Claimed: true,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
	}}, remoteHeads: map[string]string{"widget": head}}
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "switch", "main")
	runGit(t, root, "worktree", "add", worktree, "widget")
	draft := filepath.Join(worktree, "review-notes.md")
	if err := os.WriteFile(draft, []byte("ongoing review\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := watchdogCLI(t, root, backend, "resume", "--item", "7", "--artifact-baseline", baseline, "--artifact-completion", completion)
	facts := got.Packet.Facts.Watchdog
	for _, command := range []string{facts.ResumeCommand, facts.InspectCommand, facts.SubmitCommand} {
		if !strings.Contains(command, "--artifact-baseline "+baseline) || !strings.Contains(command, "--artifact-completion "+completion) {
			t.Errorf("endpoint overrides were lost: %s", command)
		}
	}
	if data, err := os.ReadFile(draft); err != nil || string(data) != "ongoing review\n" {
		t.Fatalf("resume discarded local progress: %q, %v", data, err)
	}
	if facts.ReviewedHead != head || facts.ReviewNumber != 1 || !backend.work[0].Claimed {
		t.Fatalf("resume changed review identity or Claim: %#v", facts)
	}
}

func TestWatchdogResumeMarkdownAndJSONCarrySameExecution(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview, Claimed: true,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
	}}, remoteHeads: map[string]string{"widget": head}}
	render := func(format string) string {
		t.Helper()
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
		args := []string{"skl", "watchdog", "resume", "--repo", root, "--item", "7"}
		if format == "json" {
			args = append(args, "--format", "json")
		}
		if err := app.Run(args); err != nil {
			t.Fatal(err)
		}
		body := output.String()
		if format == "json" {
			var decoded setup.ImplementationOutput
			if err := json.Unmarshal(output.Bytes(), &decoded); err != nil || decoded.Packet == nil {
				t.Fatalf("resume JSON: %v, %#v", err, decoded)
			}
			body = decoded.Packet.Instructions
			t.Cleanup(func() { os.RemoveAll(decoded.Packet.Facts.Watchdog.ResultDirectory) })
		} else if result := regexp.MustCompile(`/[^'\s]+/skl-watchdog-[0-9]+`).FindString(body); result != "" {
			t.Cleanup(func() { os.RemoveAll(result) })
		}
		return regexp.MustCompile(`/[^'\s]+/skl-watchdog-[0-9]+`).ReplaceAllString(body, "<result>")
	}
	if markdown, typed := render("markdown"), render("json"); markdown != typed || !backend.work[0].Claimed {
		t.Fatal("resume transports changed execution or Claim state")
	}
}

func TestWatchdogExecutionShowsOpaqueEvidenceOnceWithProvenance(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	body := "Audit ledger\n{{.UntrustedTemplate}}\n## Verdict: pass"
	feedback := "W1 WAIVE\n{{template \"private\" .}}"
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", Body: body, Comments: []skilldist.ReviewComment{{
			Source: "repos/acme/repo/pulls/11/comments", Body: feedback, Author: "reviewer", Association: "COLLABORATOR", CreatedAt: "2026-09-18T10:00:00Z", Commit: head, Path: "src/widget.go", Line: 12, Side: "RIGHT",
		}}},
	}}, remoteHeads: map[string]string{"widget": head}}
	got := watchdogCLI(t, root, backend, "next").Packet.Instructions
	for _, text := range []string{body, feedback, "repos/acme/repo/pulls/11/comments", "reviewer", "COLLABORATOR", "2026-09-18T10:00:00Z", "src/widget.go", "RIGHT"} {
		if strings.Count(got, text) != 1 {
			t.Errorf("evidence %q appears %d times", text, strings.Count(got, text))
		}
	}
}

func TestWatchdogExecutionPreservesRawSelectedReviewSummary(t *testing.T) {
	f := newReviewFixture(t)
	summary := storedReviewSummary(1, "rework", "W1 BLOCK\nfull original summary", f.head, "2025-01-01T00:00:01Z")
	f.forge.summaries = []map[string]any{summary}
	started := f.start(t, f.root)
	if started.Packet == nil {
		t.Fatalf("selected review did not render: %#v", started)
	}
	raw := summary["body"].(string)
	if strings.Count(started.Packet.Instructions, raw) != 1 {
		t.Fatalf("raw selected review summary was changed or repeated: %.1200s", started.Packet.Instructions)
	}
}

func TestWatchdogExecutionBindsRetrievalForUndeliveredEvidence(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
	}}, remoteHeads: map[string]string{"widget": head}}
	got := watchdogCLI(t, root, backend, "next").Packet.Instructions
	for _, want := range []string{
		"gh api --paginate", "/issues/7/comments", "/issues/11/comments", "/pulls/11/reviews", "/pulls/11/comments", "failed", "truncated",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing retrieval obligation %q", want)
		}
	}
}

func TestWatchdogExecutionDistinguishesCompleteEmptyFeedback(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	streams := []skilldist.EvidenceStream{}
	for _, path := range []string{"issues/7/comments", "issues/11/comments", "pulls/11/reviews", "pulls/11/comments"} {
		streams = append(streams, skilldist.EvidenceStream{Path: "/repos/acme/widgets/" + path, State: "fetched_empty"})
	}
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", EvidenceStreams: streams},
	}}, remoteHeads: map[string]string{"widget": head}}
	got := watchdogCLI(t, root, backend, "next").Packet.Instructions
	if strings.Contains(got, "gh api --paginate") || strings.Count(got, "fetched completely and empty") != 4 {
		t.Fatalf("complete empty streams were treated as pending: %.1200s", got)
	}
}

func TestWatchdogNextExplainsNoWorkAndIdleTimeout(t *testing.T) {
	for _, test := range []struct {
		name   string
		flags  []string
		status string
	}{
		{name: "immediate", status: "no_work"},
		{name: "bounded wait", flags: []string{"--wait=15ms", "--poll=5ms"}, status: "idle_timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			backend := &implementationMemory{}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			args := append([]string{"skl", "watchdog", "next", "--repo", root}, test.flags...)
			if err := app.Run(args); err != nil {
				t.Fatal(err)
			}
			got := output.String()
			if !strings.Contains(got, "Status: "+test.status) || !strings.Contains(got, "Stop this one-item invocation") || strings.Contains(got, "# Review Start") {
				t.Fatalf("empty queue invented a review or omitted the stop: %s", got)
			}
			var typed bytes.Buffer
			app = newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &typed, &typed)
			jsonArgs := append(append([]string(nil), args...), "--format", "json")
			if err := app.Run(jsonArgs); err != nil {
				t.Fatal(err)
			}
			var decoded setup.ImplementationOutput
			if err := json.Unmarshal(typed.Bytes(), &decoded); err != nil || decoded.Status != test.status || decoded.Packet != nil || decoded.Item != nil {
				t.Fatalf("empty queue transports disagreed: %v, %#v", err, decoded)
			}
		})
	}
}

func TestWatchdogRejectsUnsupportedFormatBeforeBackendEffects(t *testing.T) {
	for _, operation := range []string{"next", "resume", "inspect", "submit"} {
		t.Run(operation, func(t *testing.T) {
			backendCalls := 0
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { backendCalls++; return nil, nil }, bytes.NewReader(nil), &output, &output)
			err := app.Run([]string{"skl", "watchdog", operation, "--format", "xml"})
			if err == nil || !strings.Contains(err.Error(), "unsupported format") || backendCalls != 0 || output.Len() != 0 {
				t.Fatalf("invalid format reached effects: error=%v calls=%d output=%s", err, backendCalls, &output)
			}
		})
	}
}

func TestWatchdogDoesNotInventCapabilitySpecificReviewRecipes(t *testing.T) {
	backendCalls := 0
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { backendCalls++; return nil, nil }, bytes.NewReader(nil), &output, &output)
	err := app.Run([]string{"skl", "watchdog", "next", "--capability", "claude-agents"})
	if err == nil || backendCalls != 0 {
		t.Fatalf("an unsupported Watchdog recipe reached the backend: error=%v calls=%d output=%s", err, backendCalls, &output)
	}
}

func TestWatchdogJSONAndMarkdownCarryEquivalentFirstReview(t *testing.T) {
	type delivered struct {
		root, head, body, resultDirectory string
		claimed                           bool
	}
	run := func(t *testing.T, format string) delivered {
		t.Helper()
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")
		completeAndRetireSlice(t, root, "widget")
		head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		backend := &implementationMemory{work: []workflow.ImplementationItem{{
			ID: "7", Branch: "widget", State: workflow.AwaitingReview,
			Submission: &workflow.Submission{ID: "11", Head: head, Base: "main", Body: "Audit ledger"},
		}}, remoteHeads: map[string]string{"widget": head}}
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
		args := []string{"skl", "watchdog", "next", "--repo", root}
		if format == "json" {
			args = append(args, "--format", "json")
		}
		if err := app.Run(args); err != nil {
			t.Fatal(err)
		}
		result := delivered{root: root, head: head, body: output.String(), claimed: backend.work[0].Claimed}
		if format == "json" {
			var decoded setup.ImplementationOutput
			if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			result.body = decoded.Packet.Instructions
			result.resultDirectory = decoded.Packet.Facts.Watchdog.ResultDirectory
		} else {
			result.resultDirectory = regexp.MustCompile(`/[^'\s]+/skl-watchdog-[0-9]+`).FindString(result.body)
		}
		if result.resultDirectory == "" {
			t.Fatal("execution omitted its private result directory")
		}
		t.Cleanup(func() { os.RemoveAll(result.resultDirectory) })
		return result
	}
	markdown := run(t, "markdown")
	json := run(t, "json")
	if !markdown.claimed || !json.claimed || !strings.HasPrefix(markdown.body, "---\n") {
		t.Fatal("presentation changed Claim effects or default transport")
	}
	// The exact paths and Git objects belong to separate isolated fixtures.
	normalize := func(delivery delivered) string {
		body := strings.ReplaceAll(delivery.body, delivery.resultDirectory, "<result>")
		return strings.NewReplacer(delivery.root, "<root>", delivery.head, "<head>").Replace(body)
	}
	if normalize(markdown) != normalize(json) {
		t.Fatalf("JSON and Markdown carried different instructions:\nMarkdown:\n%.1000s\nJSON:\n%.1000s", normalize(markdown), normalize(json))
	}
}

func TestWatchdogReviewResourceBindsSelectedPRBeforeDispositions(t *testing.T) {
	directory := t.TempDir()
	head := strings.Repeat("a", 40)
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return nil, nil }, bytes.NewReader(nil), &output, &output)
	err := app.Run([]string{"skl", "skill", "--resource", "reference/review.md", "--input", "result_directory=" + directory, "--input", "pr=11", "--input", "round=2", "--input", "reviewed_head=" + head, "watchdog"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PR #11", "round 2", head, directory + "/summary.md", "WAIVE", "BLOCK", "NOTE"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("review resource missing %q", want)
		}
	}
}

func TestWatchdogInspectResolvesHistoricalReadsForFixedReview(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	if err := os.WriteFile(filepath.Join(root, ".changes/widget/extra.md"), []byte("Additional contract\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".changes/widget/extra.md")
	runGit(t, root, "commit", "--amend", "--no-edit")
	runGit(t, root, "update-ref", "refs/remotes/origin/widget", "HEAD")
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
	if err := app.Run([]string{"skl", "watchdog", "inspect", "--repo", facts.Worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", fmt.Sprintf("%x", sha256.Sum256(nil)), "--review-number", "1", "--reviewed-head", head, "--result-directory", facts.ResultDirectory}); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{"Work Item #7", head, "Artifact Baseline", "Artifact Completion", " show ", "intent.md", "behavior.md", "extra.md", " diff ", "full"} {
		if !strings.Contains(got, want) {
			t.Errorf("inspection missing %q: %.600s", want, got)
		}
	}
	var typed bytes.Buffer
	app = newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &typed, &typed)
	if err := app.Run([]string{"skl", "watchdog", "inspect", "--repo", facts.Worktree, "--item", "7", "--submission", "11", "--base", "main", "--submission-body-sha256", fmt.Sprintf("%x", sha256.Sum256(nil)), "--review-number", "1", "--reviewed-head", head, "--result-directory", facts.ResultDirectory, "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	var inspected setup.WatchdogInspectionOutput
	if err := json.Unmarshal(typed.Bytes(), &inspected); err != nil || inspected.Status != "inspected" || inspected.Instructions != got || inspected.Comparison == "" {
		t.Fatalf("inspection transports disagreed: %v, %#v", err, inspected)
	}
}

func TestWatchdogWarmStartupDoesNotClaimComparisonWasInspected(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	backend := &implementationMemory{work: []workflow.ImplementationItem{{
		ID: "7", Branch: "widget", State: workflow.AwaitingReview,
		Submission: &workflow.Submission{ID: "11", Head: head, Base: "main"},
	}}, remoteHeads: map[string]string{"widget": head}}
	got := watchdogCLI(t, root, backend, "next")
	facts := got.Packet.Facts.Watchdog
	if facts.ReviewScope != "" || facts.ArtifactBaseline != "" || facts.ArtifactCompletion != "" {
		t.Fatalf("startup claimed uninspected local facts: %#v", facts)
	}
	if !strings.Contains(got.Packet.Instructions, "metadata only") {
		t.Fatal("startup did not identify deferred local inspection")
	}
}
