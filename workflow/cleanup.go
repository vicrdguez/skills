package workflow

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type WorkItemID string

type SubmissionID string

type WorkItem struct {
	ID           WorkItemID
	Title        string
	Body         string
	Branch       string
	AcceptedHead string
	Merged       bool
}

type CoordinationItem struct {
	Children []WorkItemID
	ID       WorkItemID
	Title    string
	Body     string
	Closed   bool
}

type Backend interface {
	ListMergedWorkItems(context.Context) ([]WorkItem, error)
}

// CleanupOutcome reports removed, preserved, and failed source work
// separately. A preserved branch is not a failure; a failed removal is never
// reported as removed.
type CleanupOutcome struct {
	Removed   []string        `json:"removed,omitempty"`
	Preserved []SourceOutcome `json:"preserved,omitempty"`
	Failed    []SourceOutcome `json:"failed,omitempty"`
}

type SourceOutcome struct {
	Branch string `json:"branch"`
	Reason string `json:"reason"`
}

// SourceCandidate is one branch with recorded deletion authority. Hold withholds
// deletion regardless of Git state; an empty AcceptedHead is unknown evidence.
type SourceCandidate struct {
	Branch       string
	AcceptedHead string
	Hold         string
}

// Cleanup is the legacy forge-authoritative source cleanup.
func Cleanup(ctx context.Context, root string, backend Backend) (CleanupOutcome, error) {
	items, err := backend.ListMergedWorkItems(ctx)
	if err != nil {
		return CleanupOutcome{}, err
	}
	var candidates []SourceCandidate
	for _, item := range items {
		// Without an explicit branch attachment, preserve local Git state instead
		// of redirecting cleanup through a title.
		if item.Merged && item.Branch != "" {
			candidates = append(candidates, SourceCandidate{Branch: item.Branch, AcceptedHead: item.AcceptedHead})
		}
	}
	outcome, err := CleanupSource(root, candidates)
	if err == nil && len(outcome.Failed) > 0 {
		err = fmt.Errorf("remove %s: %s", outcome.Failed[0].Branch, outcome.Failed[0].Reason)
	}
	return outcome, err
}

// CleanupSource removes the local worktree and local branch of each candidate
// only when its worktree is registered at the owned .worktrees location, clean,
// and at exactly the accepted head. Merge confirmation is the caller's
// authority, so squash merges need no ancestry or upstream. Remote branches
// are never touched, and a candidate with no local branch or worktree left
// needs no report.
func CleanupSource(root string, candidates []SourceCandidate) (CleanupOutcome, error) {
	root, err := git(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return CleanupOutcome{}, errors.New("not a Git repository")
	}
	primary, err := primaryWorktree(root)
	if err != nil {
		return CleanupOutcome{}, err
	}
	registered, err := registeredWorktrees(root)
	if err != nil {
		return CleanupOutcome{}, err
	}
	var outcome CleanupOutcome
	for _, candidate := range candidates {
		branch := candidate.Branch
		path, checkedOut := registered[branch]
		if !checkedOut && gitOK(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) != nil {
			continue
		}
		preserve := func(reason string) {
			outcome.Preserved = append(outcome.Preserved, SourceOutcome{Branch: branch, Reason: reason})
		}
		if candidate.Hold != "" {
			preserve(candidate.Hold)
			continue
		}
		if filepath.Clean(path) != filepath.Clean(filepath.Join(primary, ".worktrees", branch)) {
			preserve("not checked out at its owned worktree location")
			continue
		}
		dirty, err := git(path, "status", "--porcelain", "--untracked-files=all")
		if err != nil || dirty != "" {
			preserve("the worktree has uncommitted or unobservable changes")
			continue
		}
		head, err := git(path, "rev-parse", "HEAD")
		if err != nil || candidate.AcceptedHead == "" || head != candidate.AcceptedHead {
			preserve("HEAD is not the confirmed accepted source head")
			continue
		}
		if err := gitOK(root, "worktree", "remove", path); err != nil {
			outcome.Failed = append(outcome.Failed, SourceOutcome{Branch: branch, Reason: "worktree removal failed: " + err.Error()})
			continue
		}
		if err := gitOK(root, "branch", "-D", branch); err != nil {
			outcome.Failed = append(outcome.Failed, SourceOutcome{Branch: branch, Reason: "the worktree was removed but local branch deletion failed: " + err.Error()})
			continue
		}
		outcome.Removed = append(outcome.Removed, branch)
	}
	return outcome, nil
}

func registeredWorktrees(root string) (map[string]string, error) {
	output, err := git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	registered := make(map[string]string)
	var worktree string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			worktree = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch refs/heads/") {
			registered[strings.TrimPrefix(line, "branch refs/heads/")] = worktree
		}
	}
	return registered, nil
}

func primaryWorktree(root string) (string, error) {
	output, err := git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(output, "\n")
	path, found := strings.CutPrefix(first, "worktree ")
	if !found {
		return "", errors.New("Git returned no main worktree")
	}
	return path, nil
}

func git(directory string, args ...string) (string, error) {
	output, err := exec.Command("git", append([]string{"-C", directory}, args...)...).Output()
	return strings.TrimSpace(string(output)), err
}

func gitOK(directory string, args ...string) error {
	return exec.Command("git", append([]string{"-C", directory}, args...)...).Run()
}
