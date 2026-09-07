package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func PauseImplementation(ctx context.Context, root string, number int, reason, decisionPath, bodyPath string, backend ImplementationBackend) (ImplementationOutcome, error) {
	if number <= 0 || decisionPath == "" || !slices.Contains([]string{"contradictory_artifacts", "mandatory_rule", "frozen_interface", "disputed_blocker", "bounce_cap"}, reason) {
		return ImplementationOutcome{}, errors.New("Needs Human requires --item, --decision and a permitted --reason")
	}
	if err := validateResultDirectory(decisionPath); err != nil {
		return ImplementationOutcome{}, err
	}
	if bodyPath != "" && filepath.Dir(bodyPath) != filepath.Dir(decisionPath) {
		return ImplementationOutcome{}, errors.New("decision and Submission must share one private operation directory")
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
	if !item.Claimed || item.State != Ready && item.State != Rework {
		return ImplementationOutcome{Status: "fix_required", Reason: "Workflow State contradicts pause; repair the implementation Claim and retry"}, nil
	}
	decision, err := os.ReadFile(decisionPath)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if bodyPath != "" {
		head, err := git(root, "rev-parse", "refs/heads/"+item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		remoteHead, err := backend.ImplementationHead(ctx, repository, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if head != remoteHead {
			return ImplementationOutcome{Status: "fix_required", Reason: "local and remote heads differ; push the work before pausing"}, nil
		}
		body, err := os.ReadFile(bodyPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		submission := Submission{Head: head, Base: "main", Draft: true, Body: string(body) + fmt.Sprintf("\n\nCloses #%d\n", number)}
		if item.Submission != nil {
			submission.Number = item.Submission.Number
		}
		submission, err = backend.PublishImplementation(ctx, repository, item, submission)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		current, localErr := git(root, "rev-parse", "refs/heads/"+item.Branch)
		remoteHead, err = backend.ImplementationHead(ctx, repository, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if localErr != nil || current != head || remoteHead != head || submission.Head != head || !submission.Draft {
			return ImplementationOutcome{Status: "fix_required", Reason: "draft head changed or draft is not durable; inspect the Submission and retry"}, nil
		}
		item.Submission = &submission
	} else {
		head, err := git(root, "rev-parse", "refs/heads/"+item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		history, err := InspectLedger(root, head, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if history.Baseline == "" {
			return ImplementationOutcome{Status: "fix_required", Reason: "ledger baseline missing; repair history before pausing"}, nil
		}
		changed, err := git(root, "diff", "--name-only", history.Baseline, head, "--", ".", ":(exclude).changes/"+item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if changed != "" || item.Submission != nil {
			return ImplementationOutcome{Status: "fix_required", Reason: "implementation work needs preservation; push and supply --body for a draft Submission"}, nil
		}
	}
	if err := backend.PauseImplementation(ctx, repository, item, string(decision)); err != nil {
		return ImplementationOutcome{}, err
	}
	observed, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for _, current := range observed {
		if current.Number == number && current.State == NeedsHuman && !current.Claimed && current.ResumeState == item.State {
			if err := removeResultDirectory(decisionPath); err != nil {
				return ImplementationOutcome{}, err
			}
			return ImplementationOutcome{Status: "needs_human", Item: &current}, nil
		}
	}
	return ImplementationOutcome{Status: "fix_required", Reason: "Needs Human publication is incomplete; retry the same handoff"}, nil
}

func SubmitImplementation(ctx context.Context, root string, number int, bodyPath string, backend ImplementationBackend) (ImplementationOutcome, error) {
	if number <= 0 || bodyPath == "" {
		return ImplementationOutcome{}, errors.New("submit requires --item and --body")
	}
	if err := validateResultDirectory(bodyPath); err != nil {
		return ImplementationOutcome{}, err
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
	if item.State == Rework && item.Submission == nil {
		return ImplementationOutcome{Status: "fix_required", Reason: "Rework has no existing Submission; repair its attachment before resubmitting"}, nil
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
	submission := Submission{Head: head, Base: "main", Body: string(body) + fmt.Sprintf("\n\nCloses #%d\n", number)}
	if item.Submission != nil {
		submission.Number = item.Submission.Number
	}
	submission, err = backend.PublishImplementation(ctx, repository, item, submission)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	localHead, localErr := git(root, "rev-parse", "refs/heads/"+item.Branch)
	remoteHead, err = backend.ImplementationHead(ctx, repository, item.Branch)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if localErr != nil || localHead != head || remoteHead != head || submission.Head != head {
		return ImplementationOutcome{Status: "fix_required", Reason: "head changed during publication; inspect the Submission, push a fixed head and retry"}, nil
	}
	item.Submission = &submission
	if err := backend.AwaitImplementationReview(ctx, repository, item); err != nil {
		return ImplementationOutcome{}, err
	}
	observed, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for _, current := range observed {
		if current.Number == item.Number && current.State == AwaitingReview && !current.Claimed && current.Submission != nil && current.Submission.Head == head {
			if err := removeResultDirectory(bodyPath); err != nil {
				return ImplementationOutcome{}, err
			}
			return ImplementationOutcome{Status: "awaiting_review", Item: &current}, nil
		}
	}
	return ImplementationOutcome{Status: "fix_required", Reason: "Awaiting Review read-back is incomplete; inspect the projections and retry the same handoff"}, nil
}

func removeResultDirectory(bodyPath string) error {
	if err := validateResultDirectory(bodyPath); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Dir(bodyPath))
}

func validateResultDirectory(bodyPath string) error {
	directory := filepath.Dir(bodyPath)
	if !filepath.IsAbs(bodyPath) || !slices.Contains([]string{"submission.md", "decision.md"}, filepath.Base(bodyPath)) {
		return errors.New("use the absolute Result Document path from the packet")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("Result Document directory must be private and not a symlink")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(directory))
	temporary, tempErr := filepath.EvalSymlinks(os.TempDir())
	if err != nil || tempErr != nil || parent != temporary {
		return errors.New("Result Document directory must be engine-created outside the repository")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !slices.Contains([]string{".skl-result", "submission.md", "decision.md"}, entry.Name()) || !entry.Type().IsRegular() {
			return errors.New("private Result Document directory contains unexpected files or symlinks")
		}
	}
	marker, err := os.ReadFile(filepath.Join(directory, ".skl-result"))
	if err != nil || string(marker) != "skl.implement/v1\n" || !strings.HasPrefix(filepath.Base(directory), "skl-implement-") {
		return errors.New("Result Document must be in an engine-created private operation directory")
	}
	return nil
}
