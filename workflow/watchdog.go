package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	skilldist "github.com/vicrdguez/skills"
)

func StartWatchdog(ctx context.Context, root, remote string, id WorkItemID, endpoints ArtifactEndpoints, backend ImplementationBackend) (ImplementationOutcome, error) {
	selection, ok := backend.(SelectionBackend)
	if !ok {
		return ImplementationOutcome{}, errors.New("workflow backend does not support candidate selection")
	}
	if id != "" {
		item, err := selection.ResumedImplementation(ctx, id, "")
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if item.Problem != "" {
			return implementationRefusal(item, "explicit Work Item is not an unambiguous Awaiting Review Claim: "+item.Problem+"; repair its projections before resuming"), nil
		}
		if !validConventionalBranch(root, item.Branch) {
			return implementationRefusal(item, "invalid conventional branch identity; repair the Work Item attachment"), nil
		}
		checkpoint, err := loadReviewCheckpoint(root, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, Refuse(err.Error())
		}
		return watchdogSelection(ctx, root, remote, item, endpoints, checkpoint, backend)
	}
	candidate, found, err := selectQueue(ctx, selection, ReviewQueue)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if !found {
		return ImplementationOutcome{Status: "no_work"}, nil
	}
	item, outcome, err := selectedImplementation(ctx, selection, candidate)
	if err != nil || outcome.Status != "" {
		return outcome, err
	}
	if item.State != AwaitingReview {
		return implementationRefusal(item, "the selected Work Item is not an eligible Awaiting Review record; repair its projections"), nil
	}
	if item.Claimed {
		return implementationRefusal(item, "the selected Work Item was claimed before acquisition; resume with `skl watchdog resume --item "+string(item.ID)+"` if it is yours, otherwise inspect its projections"), nil
	}
	if !validConventionalBranch(root, item.Branch) {
		return implementationRefusal(item, "invalid conventional branch identity; repair the Work Item attachment"), nil
	}
	checkpoint, err := loadReviewCheckpoint(root, item.Branch)
	if err != nil {
		return ImplementationOutcome{}, Refuse(err.Error())
	}
	if checkpoint.Count == ^uint64(0) {
		return ImplementationOutcome{}, Refuse("Review Count cannot be incremented; repair the checkpoint explicitly")
	}
	observed, outcome, err := claimSelected(ctx, selection, candidate, item)
	if err != nil || outcome.Status != "" {
		return outcome, err
	}
	return watchdogSelection(ctx, root, remote, observed, endpoints, checkpoint, backend)
}

func watchdogSelection(ctx context.Context, root, remote string, item ImplementationItem, endpoints ArtifactEndpoints, checkpoint reviewCheckpoint, backend ImplementationBackend) (ImplementationOutcome, error) {
	if item.Problem != "" || !item.Claimed || item.State != AwaitingReview || item.Submission == nil {
		return implementationRefusal(item, "explicit Work Item is not an unambiguous Awaiting Review Claim: "+item.Problem+"; repair its projections before resuming"), nil
	}
	if !validConventionalBranch(root, item.Branch) {
		return implementationRefusal(item, "invalid conventional branch identity; repair the Work Item attachment"), nil
	}
	if checkpoint.Count == ^uint64(0) {
		return ImplementationOutcome{}, Refuse("Review Count cannot be incremented; repair the checkpoint explicitly")
	}
	return watchdogPacket(ctx, root, remote, item, endpoints, checkpoint, backend)
}

func watchdogPacket(ctx context.Context, root, remote string, item ImplementationItem, endpoints ArtifactEndpoints, checkpoint reviewCheckpoint, backend ImplementationBackend) (ImplementationOutcome, error) {
	main, err := primaryWorktree(root)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	recovery := "; Claim left unchanged; inspect Work Item " + string(item.ID) + " and explicitly resume with its --item instead of retrying next"
	port, ok := backend.(submissionReader)
	if !ok {
		return ImplementationOutcome{}, Refuse("backend cannot verify Submission during packet construction" + recovery)
	}
	observed, err := port.ReviewSubmission(ctx, item.Submission.ID)
	if err != nil {
		return ImplementationOutcome{}, Refuse("cannot verify Submission during packet construction: " + err.Error() + recovery)
	}
	lifecycle := observed.Lifecycle
	if observed.ID != item.Submission.ID || observed.Branch != item.Branch || observed.Head != item.Submission.Head || observed.Base != item.Submission.Base || observed.Body != item.Submission.Body || observed.Draft != item.Submission.Draft || observed.Merged ||
		lifecycle == nil || !lifecycle.Open || !lifecycle.Claimed || len(lifecycle.States) != 1 || lifecycle.States[0] != AwaitingReview {
		return ImplementationOutcome{}, Refuse("Submission attachment, revision, or Claim changed during packet construction" + recovery)
	}
	// A review already published under this Claim must be replayed at its fixed
	// number rather than superseded by a fresh packet.
	summaries, unambiguous := reviewSummariesForClaim(item.Submission.Comments, observed.ClaimAcquiredAt)
	if !unambiguous || len(summaries) != 0 {
		return ImplementationOutcome{Status: "fix_required", Reason: "review publication already started under this Claim; replay the original fixed-number watchdog submit command and Result Documents"}, nil
	}
	facts := skilldist.WatchdogFacts{
		Branch: item.Branch, ReviewedHead: item.Submission.Head, AuditBody: item.Submission.Body, Comments: item.Submission.Comments,
		ReviewCount: checkpoint.Count, ReviewNumber: checkpoint.Count + 1, ReviewScope: skilldist.FullReview,
		Remote: remote, Worktree: filepath.Join(main, ".worktrees", item.Branch),
		SuppliedArtifactBaseline: endpoints.Baseline, SuppliedArtifactCompletion: endpoints.Completion,
	}
	if checkpoint.Count > 0 && checkpoint.Head != "" {
		facts.PreviousReviewedHead = checkpoint.Head
	}
	facts.ResultDirectory, err = os.MkdirTemp("", "skl-watchdog-")
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if err := os.WriteFile(filepath.Join(facts.ResultDirectory, ".skl-result"), []byte("skl.watchdog/v1\n"), 0600); err != nil {
		os.RemoveAll(facts.ResultDirectory)
		return ImplementationOutcome{}, err
	}
	return ImplementationOutcome{Status: "work_available", Item: &item, Facts: &skilldist.InvocationFacts{Watchdog: &facts}}, nil
}
