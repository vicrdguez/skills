package workflow

import (
	"cmp"
	"context"
	"slices"
)

type State string

const (
	Ready          State = "ready_for_implementation"
	Rework         State = "rework"
	NeedsHuman     State = "needs_human"
	AwaitingReview State = "awaiting_review"
	ReadyForMerge  State = "ready_for_merge"
	Merged         State = "merged"
	Superseded     State = "superseded"
)

type ImplementationItem struct {
	Number    int
	State     State
	CreatedAt string
	Claimed   bool
	Blockers  []int
}

type ImplementationBackend interface {
	ImplementationItems(context.Context, RepositoryID) ([]ImplementationItem, error)
	ClaimImplementation(context.Context, RepositoryID, int) error
}

type ImplementationOutcome struct {
	Status string              `json:"status"`
	Reason string              `json:"reason,omitempty"`
	Item   *ImplementationItem `json:"item,omitempty"`
}

func StartImplementation(ctx context.Context, root string, backend ImplementationBackend) (ImplementationOutcome, error) {
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
	merged := make(map[int]bool)
	for _, item := range items {
		merged[item.Number] = item.State == Merged
	}
	slices.SortFunc(items, func(a, b ImplementationItem) int {
		if a.State != b.State {
			if a.State == Rework {
				return -1
			}
			if b.State == Rework {
				return 1
			}
		}
		if age := cmp.Compare(a.CreatedAt, b.CreatedAt); age != 0 {
			return age
		}
		return cmp.Compare(a.Number, b.Number)
	})
	for _, item := range items {
		if item.Claimed || item.State != Ready && item.State != Rework {
			continue
		}
		blocked := false
		if item.State == Ready {
			for _, blocker := range item.Blockers {
				blocked = blocked || !merged[blocker]
			}
		}
		if blocked {
			continue
		}
		claimErr := backend.ClaimImplementation(ctx, repository, item.Number)
		observed, err := backend.ImplementationItems(ctx, repository)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		for _, current := range observed {
			if current.Number == item.Number && current.Claimed && current.State == item.State {
				return ImplementationOutcome{Status: "work_available", Item: &current}, nil
			}
		}
		if claimErr != nil {
			return ImplementationOutcome{}, claimErr
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "Claim read-back contradicts selected state; repair the Work Item projections and explicitly resume"}, nil
	}
	return ImplementationOutcome{Status: "no_work"}, nil
}
