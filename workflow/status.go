package workflow

import "context"

type StatusOutcome struct {
	CompleteProposals []WorkItemID         `json:"complete_proposals,omitempty"`
	Status            string               `json:"status"`
	Items             []ImplementationItem `json:"items"`
}

type StatusBackend interface {
	ImplementationBackend
	CoordinationItems(context.Context) ([]CoordinationItem, error)
	CloseCoordination(context.Context, WorkItemID) error
}

func ObserveStatus(ctx context.Context, root, remote string, backend ImplementationBackend) (StatusOutcome, error) {
	items, err := loadImplementation(ctx, backend)
	if err != nil {
		return StatusOutcome{}, err
	}
	outcome := StatusOutcome{Status: "observed", Items: items}
	for i, item := range items {
		if item.Problem != "" {
			outcome.Items[i].State = NeedsHuman
			continue
		}
		if item.State == ReadyForMerge && item.Submission != nil {
			if port, ok := backend.(ReviewBackend); ok {
				current, err := port.ReviewSubmission(ctx, item.Submission.ID)
				if err != nil {
					return StatusOutcome{}, err
				}
				if current.Merged {
					outcome.Items[i].State = Merged
					outcome.Items[i].Claimed = false
					continue
				}
				if current.Head == item.Submission.Head && current.Mergeability == "conflicting" {
					item.Synchronization = true
					item.TargetBranch = current.Base
					item.TargetSnapshot, err = backend.ImplementationHead(ctx, current.Base)
					if err != nil {
						return StatusOutcome{}, err
					}
					if item.TargetSnapshot == "" {
						return StatusOutcome{}, Refuse("current target unavailable; retry status after restoring the target")
					}
					submission := *item.Submission
					submission.PendingReview = Rework
					item.Submission = &submission
				}
			}
		}
		if item.Submission != nil && item.Submission.PendingReview != "" {
			port, ok := backend.(ReviewBackend)
			if !ok {
				return StatusOutcome{}, Refuse("backend cannot reconcile partial review")
			}
			guard := func() error {
				current, err := port.ReviewSubmission(ctx, item.Submission.ID)
				if err != nil {
					return err
				}
				if current.Head != item.Submission.Head || current.Merged {
					return Refuse("Submission changed during status reconciliation")
				}
				if item.Submission.PendingReview == ReadyForMerge && current.Mergeability != "mergeable" {
					return Refuse("mergeability unavailable or changed during status reconciliation; retry to observe the current target")
				}
				return nil
			}
			if err := port.CompleteReview(ctx, item, item.Submission.PendingReview, guard); err != nil {
				return StatusOutcome{}, err
			}
			current, err := backend.ImplementationItems(ctx)
			if err != nil {
				return StatusOutcome{}, err
			}
			for _, c := range current {
				if c.ID == item.ID {
					outcome.Items[i] = c
				}
			}
			continue
		}
		if item.State == Ready && item.Submission != nil && item.Submission.State == AwaitingReview && !item.Submission.Claimed {
			guard := func() error {
				current, err := backend.ImplementationItems(ctx)
				if err != nil {
					return err
				}
				for _, c := range current {
					if c.ID == item.ID && c.Problem == "" && c.Submission != nil && c.Submission.Head == item.Submission.Head && c.Submission.State == AwaitingReview && !c.Submission.Claimed {
						return nil
					}
				}
				return Refuse("partial Submission changed; inspect before reconciling")
			}
			if err := backend.AwaitImplementationReview(ctx, item, guard); err != nil {
				return StatusOutcome{}, err
			}
			current, err := backend.ImplementationItems(ctx)
			if err != nil {
				return StatusOutcome{}, err
			}
			for _, c := range current {
				if c.ID == item.ID {
					outcome.Items[i] = c
				}
			}
		}
	}
	if port, ok := backend.(StatusBackend); ok {
		parents, err := port.CoordinationItems(ctx)
		if err != nil {
			return StatusOutcome{}, err
		}
		merged := map[WorkItemID]bool{}
		for _, item := range items {
			merged[item.ID] = item.State == Merged && item.Problem == ""
		}
		for _, parent := range parents {
			complete := len(parent.Children) > 0
			for _, child := range parent.Children {
				complete = complete && merged[child]
			}
			if complete {
				if !parent.Closed {
					if err := port.CloseCoordination(ctx, parent.ID); err != nil {
						return StatusOutcome{}, err
					}
				}
				outcome.CompleteProposals = append(outcome.CompleteProposals, parent.ID)
			}
		}
	}
	return outcome, nil
}
