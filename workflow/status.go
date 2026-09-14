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

func ObserveStatus(ctx context.Context, backend ImplementationBackend) (StatusOutcome, error) {
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
			}
		}
		if item.Submission != nil && item.Submission.PendingReview != "" {
			if item.Submission.PendingReview != ReadyForMerge {
				return StatusOutcome{}, Refuse("partial review cannot prove its original submit context or evidence; retry its original fixed-number watchdog submit command")
			}
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
				if item.Submission.PendingReview == ReadyForMerge {
					if err := RefuseNonMainBase(item.Submission.ID, current.Base); err != nil {
						return err
					}
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
					if c.ID != item.ID || c.Problem != "" || c.Submission == nil || c.Submission.Head != item.Submission.Head || c.Submission.State != AwaitingReview || c.Submission.Claimed {
						continue
					}
					return RefuseNonMainBase(c.Submission.ID, c.Submission.Base)
				}
				return Refuse("partial Submission changed; inspect before reconciling")
			}
			if err := projectImplementation(ctx, backend, item, AwaitingReview, "", guard); err != nil {
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
