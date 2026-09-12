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
			if item.Problem == "contradictory lifecycle projections" && item.Claimed {
				return StatusOutcome{}, Refuse("ambiguous claimed lifecycle projections require the original semantic command and Result Documents; inspect without changing the Claim")
			}
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
					if item.Claimed {
						return StatusOutcome{}, Refuse("claimed conflicting Submission cannot prove status owns its handoff; inspect without changing the Claim")
					}
					item.Synchronization = true
					item.TargetBranch = current.Base
					item.TargetSnapshot, err = backend.ImplementationHead(ctx, current.Base)
					if err != nil {
						return StatusOutcome{}, err
					}
					if item.TargetSnapshot == "" {
						return StatusOutcome{}, Refuse("current target unavailable; retry status after restoring the target")
					}
					guard := func() error {
						observed, err := port.ReviewSubmission(ctx, item.Submission.ID)
						if err != nil {
							return err
						}
						if observed.Head != item.Submission.Head || observed.Merged || observed.Mergeability != "conflicting" {
							return Refuse("Submission changed during conflict diversion; inspect before retrying")
						}
						return nil
					}
					if err := port.CompleteReview(ctx, item, Rework, guard); err != nil {
						return StatusOutcome{}, err
					}
					currentItems, err := loadImplementation(ctx, backend)
					if err != nil {
						return StatusOutcome{}, err
					}
					for _, currentItem := range currentItems {
						if currentItem.ID == item.ID {
							outcome.Items[i] = currentItem
						}
					}
					continue
				}
			}
		}
		if item.State == Ready && item.Submission != nil && item.Submission.State == AwaitingReview && !item.Submission.Claimed {
			return StatusOutcome{}, Refuse("partial implementation handoff requires its original Result Document; status cannot establish publication authority")
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
