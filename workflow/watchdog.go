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

func StartWatchdog(ctx context.Context, root, remote string, id WorkItemID, endpoints ArtifactEndpoints, backend ImplementationBackend) (outcome ImplementationOutcome, err error) {
	var selected *ImplementationItem
	defer func() { dispatchRecovery(WatchdogLane, selected, &outcome, &err) }()
	url, err := git(root, "remote", "get-url", remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, err := github.ParseGitHubRemote(url)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	items, err := loadImplementation(ctx, backend)
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
		if id != "" && item.Submission.PendingReview != "" {
			return ImplementationOutcome{Status: "fix_required", Reason: "partial review requires its original fixed-number watchdog submit command and Result Documents"}, nil
		}
		if item.State != AwaitingReview || id == "" && item.Claimed || id != "" && !item.Claimed || item.Problem != "" {
			continue
		}
		checkpoint, err := loadReviewCheckpoint(root, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, Refuse(err.Error())
		}
		if checkpoint.Count == ^uint64(0) {
			return ImplementationOutcome{}, Refuse("Review Count cannot be incremented; repair the checkpoint explicitly")
		}
		history, err := InspectLedger(root, item.Submission.Head, item.Branch, endpoints, RequireRetiredArtifacts)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if history.Phase != "retired" || len(history.Violations) != 0 {
			return ImplementationOutcome{Status: "fix_required", Reason: fmt.Sprint(history.Violations) + "; fetch and restore retired ledger history"}, nil
		}
		submission := *item.Submission
		submission.ReviewedHead = submission.Head
		item.Submission = &submission
		selected = &item
		if err := backend.ClaimImplementation(ctx, item); err != nil {
			return ImplementationOutcome{}, err
		}
		observed, err := backend.ImplementationItems(ctx)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		for _, current := range observed {
			if current.ID != item.ID || !current.Claimed || current.State != AwaitingReview || current.Problem != "" || current.Submission == nil || current.Submission.Head != item.Submission.Head {
				continue
			}
			submission := *current.Submission
			submission.ReviewedHead = item.Submission.ReviewedHead
			current.Submission = &submission
			facts := skilldist.WatchdogFacts{Branch: item.Branch, ReviewedHead: current.Submission.Head, ArtifactBaseline: history.Baseline, ArtifactCompletion: history.Completion, SuppliedArtifactBaseline: endpoints.Baseline, SuppliedArtifactCompletion: endpoints.Completion, AuditBody: current.Submission.Body, Comments: current.Submission.Comments, ReviewCount: checkpoint.Count, ReviewNumber: checkpoint.Count + 1, ReviewScope: skilldist.FullReview}
			facts.BaselineFiles, err = endpointFiles(root, history.Baseline, item.Branch)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			facts.CompletionFiles, err = endpointFiles(root, history.Completion, item.Branch)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			if port, ok := backend.(ReviewBackend); ok {
				observed, err := port.ReviewSubmission(ctx, current.Submission.ID)
				if err != nil {
					return ImplementationOutcome{}, err
				}
				if observed.Head != current.Submission.Head {
					return ImplementationOutcome{}, Refuse("Submission changed during packet construction")
				}
				summaries, unambiguous := reviewSummariesForClaim(current.Submission.Comments, observed.ClaimAcquiredAt)
				if !unambiguous || len(summaries) != 0 {
					return ImplementationOutcome{Status: "fix_required", Reason: "review publication already started under this Claim; replay the original fixed-number watchdog submit command and Result Documents"}, nil
				}
			}
			round, reference, err := prepareDispatch(ctx, repository, current, WatchdogLane, id != "", backend)
			if err != nil {
				return ImplementationOutcome{}, err
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
			facts.ResultDirectory = filepath.Join(os.TempDir(), round.Directory)
			return ImplementationOutcome{Status: "work_available", Item: &current, Facts: &skilldist.InvocationFacts{Watchdog: &facts}, Dispatch: &DispatchFacts{Reference: reference, Lane: WatchdogLane, Root: main, Remote: remote}}, nil
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "Watchdog Claim changed; inspect and explicitly resume"}, nil
	}
	if id != "" {
		return ImplementationOutcome{Status: "fix_required", Reason: "explicit Work Item is not an unambiguous Awaiting Review Claim"}, nil
	}
	return ImplementationOutcome{Status: "no_work"}, nil
}
