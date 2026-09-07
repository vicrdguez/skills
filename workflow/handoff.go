package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SubmitImplementation(ctx context.Context, root string, number int, bodyPath string, backend ImplementationBackend) (ImplementationOutcome, error) {
	if number <= 0 || bodyPath == "" {
		return ImplementationOutcome{}, errors.New("submit requires --item and --body")
	}
	remote, err := git(root, "remote", "get-url", "origin")
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, err := ParseGitHubRemote(remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	items, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	var item ImplementationItem
	for _, candidate := range items {
		if candidate.Number == number {
			item = candidate
		}
	}
	if item.Number == 0 || !item.Claimed || item.State != Ready && item.State != Rework {
		return ImplementationOutcome{Status: "fix_required", Reason: "Workflow State contradicts submission; repair the claimed Ready or Rework projections and retry"}, nil
	}
	head, err := git(root, "rev-parse", "refs/heads/"+item.Branch)
	if err != nil {
		return ImplementationOutcome{Status: "fix_required", Reason: "local branch unavailable; restore its conventional worktree"}, nil
	}
	remoteHead, err := backend.ImplementationHead(ctx, repository, item.Branch)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if remoteHead != head {
		return ImplementationOutcome{Status: "fix_required", Reason: "local and remote heads differ; push the branch and retry"}, nil
	}
	if item.State == Ready && (item.TargetSnapshot == "" || gitOK(root, "merge-base", "--is-ancestor", item.TargetSnapshot, head) != nil) {
		return ImplementationOutcome{Status: "fix_required", Reason: "Target Snapshot is absent; merge the pinned snapshot, commit and push before retrying"}, nil
	}
	history, err := InspectLedger(root, head, item.Branch)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if history.Phase != "retired" || len(history.Violations) != 0 {
		return ImplementationOutcome{Status: "fix_required", Reason: fmt.Sprint(history.Violations) + "; complete permitted ticks, commit Completion, then delete the entire ledger in a child commit and push"}, nil
	}
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	submission := Submission{Head: head, Base: "main", Body: strings.TrimRight(string(body), "\n") + fmt.Sprintf("\n\nCloses #%d\n", number)}
	if item.Submission != nil {
		submission.Number = item.Submission.Number
	}
	submission, err = backend.PublishImplementation(ctx, repository, item, submission)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	item.Submission = &submission
	if err := backend.AwaitImplementationReview(ctx, repository, item); err != nil {
		return ImplementationOutcome{}, err
	}
	return ImplementationOutcome{Status: "awaiting_review", Item: &item}, nil
}

func removeResultDirectory(bodyPath string) error {
	directory := filepath.Dir(bodyPath)
	marker, err := os.ReadFile(filepath.Join(directory, ".skl-result"))
	if err != nil || string(marker) != "skl.implement/v1\n" || !strings.HasPrefix(filepath.Base(directory), "skl-implement-") {
		return errors.New("Result Document must be in an engine-created private operation directory")
	}
	return os.RemoveAll(directory)
}
