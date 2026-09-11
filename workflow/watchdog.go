package workflow

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
)

func StartWatchdog(ctx context.Context, root, remote string, id WorkItemID, backend ImplementationBackend) (ImplementationOutcome, error) {
	remote, err := github.ResolveGitHubRemote(root, remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, items, err := loadImplementation(ctx, root, remote, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	items = slices.DeleteFunc(items, func(item ImplementationItem) bool { return item.Submission == nil })
	slices.SortFunc(items, func(a, b ImplementationItem) int {
		if order := cmp.Compare(a.Submission.CreatedAt, b.Submission.CreatedAt); order != 0 {
			return order
		}
		return cmp.Compare(a.Order, b.Order)
	})
	for _, item := range items {
		if id != "" && item.ID != id {
			continue
		}
		if item.State != AwaitingReview || id == "" && item.Claimed || id != "" && !item.Claimed || item.Problem != "" {
			continue
		}
		checkpoint, err := loadReviewCheckpoint(root, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, Refuse(err.Error())
		}
		history, err := InspectLedger(root, item.Submission.Head, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if history.Phase != "retired" || len(history.Violations) != 0 {
			return ImplementationOutcome{Status: "fix_required", Reason: fmt.Sprint(history.Violations) + "; fetch and restore retired ledger history"}, nil
		}
		if err := backend.ClaimImplementation(ctx, repository, item); err != nil {
			return ImplementationOutcome{}, err
		}
		observed, err := backend.ImplementationItems(ctx, repository)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		for _, current := range observed {
			if current.ID != item.ID || !current.Claimed || current.State != AwaitingReview || current.Problem != "" || current.Submission == nil || current.Submission.Head != item.Submission.Head {
				continue
			}
			if checkpoint.Count == ^uint64(0) {
				return ImplementationOutcome{}, Refuse("Review Count cannot be incremented; repair the checkpoint explicitly")
			}
			facts := skilldist.WatchdogFacts{Branch: item.Branch, ReviewedHead: current.Submission.Head, ArtifactBaseline: history.Baseline, ArtifactCompletion: history.Completion, AuditBody: current.Submission.Body, Comments: current.Submission.Comments, ReviewCount: checkpoint.Count, ReviewNumber: checkpoint.Count + 1, ReviewScope: skilldist.FullReview}
			facts.BaselineFiles, err = ledgerFiles(root, history.Baseline, ".changes/"+item.Branch)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			facts.CompletionFiles, err = ledgerFiles(root, history.Completion, ".changes/"+item.Branch)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			if port, ok := backend.(ReviewBackend); ok {
				observed, err := port.ReviewSubmission(ctx, repository, current.Submission.ID)
				if err != nil {
					return ImplementationOutcome{}, err
				}
				if observed.Head != current.Submission.Head {
					return ImplementationOutcome{}, Refuse("Submission changed during packet construction")
				}
			}
			main, err := primaryWorktree(root)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			facts.Worktree = filepath.Join(main, ".worktrees", item.Branch)
			if checkpoint.Count > 0 && checkpoint.Head != "" && gitOK(facts.Worktree, "cat-file", "-e", checkpoint.Head+"^{commit}") == nil && gitOK(facts.Worktree, "merge-base", "--is-ancestor", checkpoint.Head, facts.ReviewedHead) == nil {
				facts.ReviewScope = skilldist.IncrementalReview
				facts.PreviousReviewedHead = checkpoint.Head
			}
			facts.Remote = remote
			facts.ResultDirectory, err = os.MkdirTemp("", "skl-watchdog-")
			if err != nil {
				return ImplementationOutcome{}, err
			}
			if err := os.WriteFile(filepath.Join(facts.ResultDirectory, ".skl-result"), []byte("skl.watchdog/v1\n"), 0600); err != nil {
				os.RemoveAll(facts.ResultDirectory)
				return ImplementationOutcome{}, err
			}
			return ImplementationOutcome{Status: "work_available", Item: &current, Facts: &skilldist.InvocationFacts{Watchdog: &facts}}, nil
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "Watchdog Claim changed; inspect and explicitly resume"}, nil
	}
	if id != "" {
		return ImplementationOutcome{Status: "fix_required", Reason: "explicit Work Item is not an unambiguous Awaiting Review Claim"}, nil
	}
	return ImplementationOutcome{Status: "no_work"}, nil
}
