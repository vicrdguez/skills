package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/vicrdguez/skills/workflow"
)

func (b *implementationMemory) ReviewSubmission(_ context.Context, id workflow.SubmissionID) (workflow.Submission, error) {
	b.reviewSubmissionCalls++
	if b.beforeReviewSubmission != nil {
		b.beforeReviewSubmission(b.reviewSubmissionCalls)
	}
	for i := range b.work {
		if b.work[i].Submission != nil && b.work[i].Submission.ID == id {
			b.work[i] = workflow.ReconcileImplementation(implementationFixture(b.work[i]))
			submission := b.work[i].Submission
			if b.work[i].Claimed && submission.ClaimAcquiredAt == "" {
				submission.ClaimAcquiredAt = b.reviewTime()
			}
			observed := *submission
			observed.Branch = b.work[i].Branch
			return observed, nil
		}
	}
	return workflow.Submission{}, fmt.Errorf("missing Submission")
}

func (b *implementationMemory) CompleteReview(_ context.Context, item workflow.ImplementationItem, target workflow.State, guard func() error) error {
	if err := guard(); err != nil {
		return err
	}
	for i := range b.work {
		if b.work[i].ID == item.ID {
			b.work[i] = implementationFixture(b.work[i])
			b.work[i].Synchronization = item.Synchronization
			if target != workflow.NeedsHuman {
				b.work[i].Source.States = slices.DeleteFunc(b.work[i].Source.States, func(state workflow.State) bool { return state == workflow.NeedsHuman })
			}
			b.work[i].Submission.Lifecycle.States = []workflow.State{target}
			b.work[i].Submission.Lifecycle.Claimed = false
			b.work[i].Submission.PendingReview = ""
			b.work[i].Submission.ClaimAcquiredAt = ""
			b.work[i] = workflow.ReconcileImplementation(b.work[i])
		}
	}
	return nil
}
