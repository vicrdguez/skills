package workflow

import (
	"context"
	"time"

	skilldist "github.com/vicrdguez/skills"
)

type ReviewBackend interface {
	ImplementationBackend
	ReviewSubmission(context.Context, SubmissionID) (Submission, error)
	CompleteReview(context.Context, ImplementationItem, State, func() error) error
}

func reviewSummariesForClaim(comments []skilldist.ReviewComment, claimedAt string) ([]skilldist.ReviewComment, bool) {
	hasSummary := false
	for _, comment := range comments {
		hasSummary = hasSummary || comment.Path == "" && comment.ReviewNumber != 0
	}
	if !hasSummary {
		return nil, true
	}
	var current []skilldist.ReviewComment
	for _, comment := range comments {
		if comment.Path != "" || comment.ReviewNumber == 0 {
			continue
		}
		after, unambiguous := receiptAfterClaim(claimedAt, comment.CreatedAt)
		if !unambiguous {
			return nil, false
		}
		if after {
			current = append(current, comment)
		}
	}
	return current, true
}

func receiptAfterClaim(claimedAt, submittedAt string) (bool, bool) {
	claim, claimErr := time.Parse(time.RFC3339Nano, claimedAt)
	receipt, receiptErr := time.Parse(time.RFC3339Nano, submittedAt)
	if claimErr != nil || receiptErr != nil || claim.Equal(receipt) {
		return false, false
	}
	return claim.Before(receipt), true
}
