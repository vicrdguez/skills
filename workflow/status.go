package workflow

import "context"

type StatusOutcome struct {
	CompleteProposals []int                `json:"complete_proposals,omitempty"`
	Status            string               `json:"status"`
	Items             []ImplementationItem `json:"items"`
}

type StatusBackend interface {
	ImplementationBackend
	CoordinationItems(context.Context, RepositoryID) ([]CoordinationItem, error)
	CloseCoordination(context.Context, RepositoryID, int) error
}

func ObserveStatus(ctx context.Context, root, remote string, backend ImplementationBackend) (StatusOutcome, error) {
	remote, err := ResolveGitHubRemote(root, remote)
	if err != nil {
		return StatusOutcome{}, err
	}
	repository, items, err := loadImplementation(ctx, root, remote, backend)
	if err != nil {
		return StatusOutcome{}, err
	}
	outcome := StatusOutcome{Status: "observed", Items: items}
	for i, item := range items {
		if item.Problem != "" {
			outcome.Items[i].State = NeedsHuman
			continue
		}
		if item.State == Ready && item.Submission != nil && item.Submission.State == AwaitingReview && !item.Submission.Claimed {
			guard := func() error {
				current, err := backend.ImplementationItems(ctx, repository)
				if err != nil {
					return err
				}
				for _, c := range current {
					if c.Number == item.Number && c.Problem == "" && c.Submission != nil && c.Submission.Head == item.Submission.Head && c.Submission.State == AwaitingReview && !c.Submission.Claimed {
						return nil
					}
				}
				return Refuse("partial Submission changed; inspect before reconciling")
			}
			if err := backend.AwaitImplementationReview(ctx, repository, item, guard); err != nil {
				return StatusOutcome{}, err
			}
			current, err := backend.ImplementationItems(ctx, repository)
			if err != nil {
				return StatusOutcome{}, err
			}
			for _, c := range current {
				if c.Number == item.Number {
					outcome.Items[i] = c
				}
			}
		}
	}
	if port, ok := backend.(StatusBackend); ok {
		parents, err := port.CoordinationItems(ctx, repository)
		if err != nil {
			return StatusOutcome{}, err
		}
		merged := map[int]bool{}
		for _, item := range items {
			merged[item.Number] = item.State == Merged && item.Problem == ""
		}
		for _, parent := range parents {
			complete := len(parent.Children) > 0
			for _, child := range parent.Children {
				complete = complete && merged[child]
			}
			if complete {
				if !parent.Closed {
					if err := port.CloseCoordination(ctx, repository, parent.Number); err != nil {
						return StatusOutcome{}, err
					}
				}
				outcome.CompleteProposals = append(outcome.CompleteProposals, parent.Number)
			}
		}
	}
	return outcome, nil
}
