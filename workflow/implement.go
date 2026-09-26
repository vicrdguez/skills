package workflow

import (
	"context"
	"strings"

	skilldist "github.com/vicrdguez/skills"
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
	Source                *LifecycleObservation
	Synchronization       bool
	Problem               string
	Submission            *Submission
	Branch                string
	ID                    WorkItemID
	Order                 int
	State                 State
	CreatedAt             string
	Claimed               bool
	SourceClaimAcquiredAt string
	Blockers              []WorkItemID
}

type Submission struct {
	Lifecycle       *LifecycleObservation
	PendingReview   State
	ClaimAcquiredAt string
	BodyUpdatedAt   string
	Merged          bool
	Mergeability    string
	CreatedAt       string
	State           State
	Claimed         bool
	ID              SubmissionID
	Branch          string
	Head            string
	Base            string
	Body            string
	Author          string
	Association     string
	Draft           bool
	Comments        []skilldist.ReviewComment
}

// LifecycleObservation retains overlaps while a multi-record transition is in flight.
// Source and every attached Submission must supply an observation, even with no states.
type LifecycleObservation struct {
	States  []State
	Open    bool
	Claimed bool
	Merged  bool
}

type ImplementationBackend interface {
	// ImplementationItems must retain source and Submission lifecycle observations;
	// derived State and Claimed fields are not substitutes for those records.
	ImplementationItems(context.Context) ([]ImplementationItem, error)
}

type InvariantError struct{ Reason string }

func (e *InvariantError) Error() string { return e.Reason }
func Refuse(reason string) error        { return &InvariantError{Reason: reason} }

// RefuseNonMainBase reports an observed Submission destination other than main.
// An unobserved base keeps its existing meaning; earlier valid effects are never rolled back.
func RefuseNonMainBase(id SubmissionID, base string) error {
	if base == "" || base == "main" {
		return nil
	}
	return Refuse("existing Submission " + string(id) + " targets " + base + "; inspect it and explicitly repair its base to main before retrying")
}

type ImplementationOutcome struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func loadImplementation(ctx context.Context, backend ImplementationBackend) ([]ImplementationItem, error) {
	items, err := backend.ImplementationItems(ctx)
	for i := range items {
		items[i] = ReconcileImplementation(items[i])
	}
	return items, err
}

// ReconcileImplementation applies canonical lifecycle rules to backend observations.
// Integrations also use it for the existing shared review and Claim read-backs.
func ReconcileImplementation(item ImplementationItem) ImplementationItem {
	if item.Source == nil || item.Submission != nil && item.Submission.Lifecycle == nil {
		item.Problem = "missing lifecycle observations; repair the Backend record"
		return item
	}
	sourceState, sourceProblem := item.Source.state()
	sourceClaimed := item.Source.Claimed
	item.State, item.Claimed = sourceState, sourceClaimed
	if item.Problem == "" {
		item.Problem = sourceProblem
	}
	if submission := item.Submission; submission != nil {
		copy := *submission
		item.Submission, submission = &copy, &copy
		prState, prProblem := submission.Lifecycle.state()
		submission.State, submission.Claimed = prState, submission.Lifecycle.Claimed
		if item.Problem == "" || item.Problem == "contradictory lifecycle projections" {
			item.Problem = prProblem
			if item.Problem == "" {
				item.Problem = sourceProblem
			}
		}
		state := sourceState
		item.Claimed = item.Claimed || submission.Claimed
		if state == NeedsHuman && (prState == Rework || prState == AwaitingReview) && prProblem == "" {
			item.State = prState
		} else if state == NeedsHuman || prState == NeedsHuman {
			item.State = NeedsHuman
		} else if prState != "" {
			if state == Ready && prState != AwaitingReview {
				if item.Problem == "" || item.Problem == "contradictory lifecycle projections" {
					item.Problem = "source Ready contradicts Submission lifecycle"
				}
			} else if state != Ready {
				item.State = prState
			}
		}
		if submission.Merged || submission.Lifecycle.Merged || sourceState == Merged {
			item.State = Merged
		} else if !submission.Lifecycle.Open || sourceState == Superseded {
			item.State = Superseded
		}
	}
	if !item.Source.Open && item.State != Merged && item.State != ReadyForMerge && item.State != Superseded && (item.Problem == "" || item.Problem == "contradictory lifecycle projections" || item.Problem == "source Ready contradicts Submission lifecycle") {
		item.Problem = "source is closed without a merged Submission"
	}
	if item.Submission != nil && item.Submission.PendingReview != "" && sourceProblem == "" && (item.Problem == "" || item.Problem == "contradictory lifecycle projections") {
		item.Problem = ""
		item.State = item.Submission.PendingReview
	}
	if item.Submission != nil && item.Submission.Merged {
		item.Claimed = false
	}
	return item
}

func (observation LifecycleObservation) state() (State, string) {
	var state State
	problem := ""
	for _, next := range observation.States {
		if state != "" && state != next {
			problem = "contradictory lifecycle projections"
		}
		if state == "" || next == NeedsHuman {
			state = next
		}
	}
	return state, problem
}

func validConventionalBranch(root, branch string) bool {
	return branch != "" && gitOK(root, "check-ref-format", "--branch", branch) == nil && !strings.Contains(branch, "/")
}
