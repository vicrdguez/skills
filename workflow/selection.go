package workflow

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
)

// QueueKind names the Work Start queues. Rework is exhausted before Ready.
type QueueKind string

const (
	ReadyQueue  QueueKind = "ready"
	ReworkQueue QueueKind = "rework"
	ReviewQueue QueueKind = "review"
)

// QueueCandidate is one lightweight open queue record: enough to order, skip,
// and reject it without reading discussions or artifact contents.
type QueueCandidate struct {
	ID           WorkItemID
	SubmissionID SubmissionID
	Number       int
	CreatedAt    string
	Claimed      bool
	Problem      string
}

// QueuePage is one lazy observation of a queue. Next continues the stream; an
// empty Next means the queue is complete.
type QueuePage struct {
	Candidates []QueueCandidate
	Next       string
}

// BlockerObservation is the Merged evidence for one referenced Dependency.
type BlockerObservation struct {
	ID     WorkItemID
	Merged bool
}

// SelectionBackend is the candidate-first observation seam used by Work Start.
// It observes only open queue records, hydrates exactly the selected item, and
// never requires local project objects.
type SelectionBackend interface {
	QueuePage(context.Context, QueueKind, string) (QueuePage, error)
	SelectedImplementation(context.Context, QueueCandidate) (ImplementationItem, error)
	ResumedImplementation(context.Context, WorkItemID, string) (ImplementationItem, error)
	ImplementationDependencies(context.Context, WorkItemID) ([]BlockerObservation, error)
	// ClaimSelected adds the queue record's Claim and re-observes only the
	// selected records, returning their verified identity.
	ClaimSelected(context.Context, QueueCandidate, ImplementationItem) (ImplementationItem, error)
}

func compareCandidates(a, b QueueCandidate) int {
	if order := strings.Compare(a.CreatedAt, b.CreatedAt); order != 0 {
		return order
	}
	return cmp.Compare(a.Number, b.Number)
}

// selectQueue observes just enough of one queue to identify its oldest
// eligible unclaimed candidate. Candidate pages arrive in ascending record
// order; a candidate is only accepted once no unobserved record can precede it.
func selectQueue(ctx context.Context, backend SelectionBackend, queue QueueKind) (QueueCandidate, bool, error) {
	var observed []QueueCandidate
	blocked := make(map[WorkItemID]bool)
	seen := make(map[string]bool)
	cursor := ""
	for {
		page, err := backend.QueuePage(ctx, queue, cursor)
		if err != nil {
			return QueueCandidate{}, false, err
		}
		if page.Next != "" {
			if seen[page.Next] {
				return QueueCandidate{}, false, errors.New("queue pagination cursor repeated; required observation is incomplete")
			}
			seen[page.Next] = true
		}
		observed = append(observed, page.Candidates...)
		slices.SortFunc(observed, compareCandidates)
		for i := range observed {
			candidate := observed[i]
			if candidate.Claimed {
				continue
			}
			if candidate.Problem != "" {
				return candidate, false, Refuse(candidate.Problem)
			}
			if queue == ReadyQueue {
				isBlocked, known := blocked[candidate.ID]
				if !known {
					dependencies, err := backend.ImplementationDependencies(ctx, candidate.ID)
					if err != nil {
						return QueueCandidate{}, false, err
					}
					isBlocked = slices.ContainsFunc(dependencies, func(observation BlockerObservation) bool { return !observation.Merged })
					blocked[candidate.ID] = isBlocked
				}
				if isBlocked {
					continue
				}
			}
			if page.Next == "" || observed[len(observed)-1].CreatedAt > candidate.CreatedAt {
				return candidate, true, nil
			}
			break // a later page may still hold an earlier same-time record
		}
		if page.Next == "" {
			return QueueCandidate{}, false, nil
		}
		cursor = page.Next
	}
}

// selectedImplementation hydrates and validates one candidate through its
// explicitly attached records.
func selectedImplementation(ctx context.Context, backend SelectionBackend, candidate QueueCandidate) (ImplementationItem, ImplementationOutcome, error) {
	item, err := backend.SelectedImplementation(ctx, candidate)
	if err != nil {
		return ImplementationItem{}, ImplementationOutcome{}, err
	}
	if item.Problem != "" {
		return item, implementationRefusal(item, item.Problem+"; repair the selected Work Item projections before continuing"), nil
	}
	if item.Branch == "" {
		return item, implementationRefusal(item, "the selected Work Item has no explicit branch attachment; repair it before continuing"), nil
	}
	return item, ImplementationOutcome{}, nil
}

func implementationRefusal(item ImplementationItem, reason string) ImplementationOutcome {
	return ImplementationOutcome{Status: "fix_required", Reason: reason, Item: &item}
}

// claimSelected adds the queue-record Claim and verifies it through the
// selected records alone, without rediscovering the queue.
func claimSelected(ctx context.Context, backend SelectionBackend, candidate QueueCandidate, item ImplementationItem) (ImplementationItem, ImplementationOutcome, error) {
	observed, err := backend.ClaimSelected(ctx, candidate, item)
	if err != nil {
		return ImplementationItem{}, ImplementationOutcome{}, err
	}
	if observed.Problem != "" || !observed.Claimed || observed.ID != item.ID || observed.State != item.State || observed.Branch != item.Branch {
		return observed, implementationRefusal(observed, "Claim read-back contradicts the selected Work Item; inspect it and explicitly resume"), nil
	}
	if item.Submission != nil && (observed.Submission == nil || observed.Submission.ID != item.Submission.ID || observed.Submission.Head != item.Submission.Head) {
		return observed, implementationRefusal(observed, "Claim read-back contradicts the selected Submission; inspect it and explicitly resume"), nil
	}
	return observed, ImplementationOutcome{}, nil
}
