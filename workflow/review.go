package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
)

type ReviewBackend interface {
	ImplementationBackend
	ReviewSubmission(context.Context, github.RepositoryID, SubmissionID) (Submission, error)
	PublishReview(context.Context, github.RepositoryID, ImplementationItem, []skilldist.ReviewComment, func() error) error
	CompleteReview(context.Context, github.RepositoryID, ImplementationItem, State, func() error) error
}

func SubmitWatchdog(ctx context.Context, root, remote string, id WorkItemID, reviewNumber uint64, reviewed, head, verdict, summaryPath, findingsPath, bodyPath string, backend ReviewBackend) (outcome ImplementationOutcome, err error) {
	defer func() {
		warn := func(message string) {
			if outcome.Reason == "" {
				outcome.Reason = message
			} else {
				outcome.Reason += "; " + message
			}
		}
		if err != nil || outcome.Item == nil || outcome.Item.Claimed {
			return
		}
		dir := filepath.Dir(summaryPath)
		if !filepath.IsAbs(summaryPath) || !strings.HasPrefix(filepath.Base(dir), "skl-watchdog-") {
			return
		}
		parent, e := filepath.EvalSymlinks(filepath.Dir(dir))
		temp, te := filepath.EvalSymlinks(os.TempDir())
		if e != nil || te != nil || parent != temp {
			return
		}
		info, e := os.Lstat(dir)
		if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return
		}
		marker, e := os.ReadFile(filepath.Join(dir, ".skl-result"))
		if e != nil || string(marker) != "skl.watchdog/v1\n" {
			return
		}
		entries, e := os.ReadDir(dir)
		if e != nil {
			warn("handoff completed but private Result Document directory cleanup failed: " + e.Error())
			return
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() || entry.Name() != ".skl-result" && filepath.Ext(entry.Name()) != ".md" && filepath.Ext(entry.Name()) != ".json" {
				warn("handoff completed but private directory has unexpected files; preserve it for explicit cleanup")
				return
			}
		}
		if e := os.RemoveAll(dir); e != nil {
			warn("handoff completed but private Result Document directory cleanup failed: " + e.Error())
		}
	}()
	if head == "" {
		head = reviewed
	}
	if id == "" || reviewNumber == 0 || reviewed == "" || verdict != "rework" && verdict != "pass" && verdict != "needs-human" || summaryPath == "" || verdict == "pass" && bodyPath == "" {
		return ImplementationOutcome{}, fmt.Errorf("submit requires --item, positive --review-number, --reviewed-head, --verdict rework|pass|needs-human and --summary; pass also requires --body")
	}
	remote, err = github.ResolveGitHubRemote(root, remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, items, err := loadImplementation(ctx, root, remote, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	var item ImplementationItem
	for _, candidate := range items {
		if candidate.ID == id {
			if item.ID != "" {
				return ImplementationOutcome{}, Refuse("ambiguous Work Item")
			}
			item = candidate
		}
	}
	if item.Problem != "" || item.Submission == nil || item.State == AwaitingReview && !item.Claimed {
		return ImplementationOutcome{}, Refuse("verdict requires the selected review Claim or an exactly observable fixed-number retry")
	}
	checkpoint, err := loadReviewCheckpoint(root, item.Branch)
	if err != nil {
		return ImplementationOutcome{}, Refuse(err.Error())
	}
	if !checkpoint.validHead(reviewed) {
		return ImplementationOutcome{}, Refuse(fmt.Sprintf("reviewed and final heads must be full %d-character hexadecimal object IDs", checkpoint.ObjectIDWidth))
	}
	if !checkpoint.validHead(head) {
		return ImplementationOutcome{}, Refuse(fmt.Sprintf("reviewed and final heads must be full %d-character hexadecimal object IDs", checkpoint.ObjectIDWidth))
	}
	retry := reviewNumber == checkpoint.Count && checkpoint.Count > 0
	completedDone := checkpoint.Count == 0 && item.State == ReadyForMerge && !item.Claimed
	if retry && checkpoint.Head != reviewed {
		return ImplementationOutcome{}, Refuse("review-number names a recorded round at a different reviewed head; inspect and replay the original fixed-number command")
	}
	if !retry && !completedDone && (checkpoint.Count == ^uint64(0) || reviewNumber != checkpoint.Count+1) {
		return ImplementationOutcome{}, Refuse("review-number must equal the retained completed count or exactly the next round; inspect and replay the original fixed-number command")
	}
	if !retry && !completedDone && item.State != AwaitingReview {
		return ImplementationOutcome{}, Refuse("a new review completion requires the selected Awaiting Review Claim")
	}
	if head != reviewed && (verdict != "pass" || gitOK(root, "merge-base", "--is-ancestor", reviewed, head) != nil) {
		return ImplementationOutcome{}, Refuse("post-marker head must descend from the fixed reviewed head on pass")
	}
	requireMergeable := false
	guard := func() error {
		local, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
		if err != nil || local != head {
			return Refuse("local reviewed head changed; restore the fixed head")
		}
		remote, err := backend.ImplementationHead(ctx, repository, item.Branch)
		if err != nil {
			return err
		}
		if remote != head {
			return Refuse("remote reviewed head changed; push the fixed head")
		}
		submission, err := backend.ReviewSubmission(ctx, repository, item.Submission.ID)
		if err != nil {
			return err
		}
		if submission.Head != head || submission.Merged || submission.Draft {
			return Refuse("Submission head changed during verdict")
		}
		if requireMergeable && submission.Mergeability != "mergeable" {
			return Refuse("mergeability changed during verdict; retry to observe the current target")
		}
		return nil
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	history, err := InspectLedger(root, head, item.Branch)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if history.Phase != "retired" || len(history.Violations) > 0 {
		return ImplementationOutcome{}, Refuse("restore valid retired ledger history")
	}
	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	comments := []skilldist.ReviewComment{{Body: string(summary), Commit: reviewed, Verdict: verdict}}
	if findingsPath != "" {
		data, err := os.ReadFile(findingsPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		var anchors []struct {
			Path     string `json:"path"`
			Line     int    `json:"line"`
			Side     string `json:"side"`
			BodyFile string `json:"body_file"`
		}
		if err := json.Unmarshal(data, &anchors); err != nil {
			return ImplementationOutcome{}, err
		}
		for _, a := range anchors {
			if a.Path == "" || path.IsAbs(a.Path) || path.Clean(a.Path) != a.Path || strings.HasPrefix(a.Path, "../") || a.Line <= 0 || a.Side != "LEFT" && a.Side != "RIGHT" {
				return ImplementationOutcome{}, fmt.Errorf("invalid structured inline anchor")
			}
			body, err := os.ReadFile(a.BodyFile)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			comments = append(comments, skilldist.ReviewComment{Body: string(body), Commit: reviewed, Path: a.Path, Line: a.Line, Side: a.Side})
		}
	}
	finalBody := ""
	if verdict == "pass" {
		body, err := os.ReadFile(bodyPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		finalBody = withClosingReference(string(body), item.ClosingReference)
	}
	submission, err := backend.ReviewSubmission(ctx, repository, item.Submission.ID)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	receipt, receiptCount := matchingSummaryReceipt(*item.Submission, comments[0])
	evidenceMatches := reviewEvidenceMatches(item, comments, finalBody)
	if retry && (!evidenceMatches || receiptCount != 1) {
		return ImplementationOutcome{}, Refuse("recorded review differs from the supplied summary, verdict, body, or inline evidence; replay the original fixed-number command and Result Documents")
	}
	if receiptCount > 0 && item.State == AwaitingReview && (receiptCount != 1 || !evidenceMatches || !claimPrecedesReceipt(submission.ClaimAcquiredAt, receipt.CreatedAt)) {
		return ImplementationOutcome{}, Refuse("exact review receipt cannot be assigned unambiguously to the current Awaiting Review Claim; replay its original fixed-number command or submit a fresh next round")
	}
	target := Rework
	if reviewNumber >= 2 {
		target = NeedsHuman
		item.ResumeState = Rework
	}
	if verdict == "needs-human" {
		target = NeedsHuman
		item.ResumeState = AwaitingReview
	}
	if verdict == "pass" {
		if submission.Mergeability != "mergeable" && submission.Mergeability != "conflicting" {
			return ImplementationOutcome{}, Refuse("mergeability unavailable; wait for backend evaluation and retry")
		}
		target = ReadyForMerge
		if item.State == Rework && item.Synchronization {
			target = Rework
		} else if submission.Mergeability == "conflicting" && (item.State == AwaitingReview || item.State == ReadyForMerge) {
			target = Rework
			item.Synchronization = true
			item.TargetBranch = submission.Base
			item.TargetSnapshot, err = backend.ImplementationHead(ctx, repository, submission.Base)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			if item.TargetSnapshot == "" {
				return ImplementationOutcome{}, Refuse("current target unavailable; restore it and retry")
			}
		}
		requireMergeable = target == ReadyForMerge
	}
	if item.State != AwaitingReview {
		if item.Claimed && item.Submission.PendingReview == "" {
			return ImplementationOutcome{}, Refuse("target-only Claim cannot prove it belongs to this review handoff; inspect before replaying the original fixed-number command")
		}
		compatible := verdict == "rework" && (item.State == Rework || item.State == NeedsHuman) || verdict == "needs-human" && item.State == NeedsHuman || verdict == "pass" && (item.State == ReadyForMerge || item.State == Rework && item.Synchronization)
		compatible = compatible && reviewEvidenceMatches(item, comments, finalBody)
		if !compatible {
			return ImplementationOutcome{}, Refuse("completed or partial review differs from supplied verdict; restore its exact Result Documents")
		}
		if completedDone {
			return ImplementationOutcome{Status: string(item.State), Item: &item, Head: head}, guard()
		}
		if verdict != "pass" {
			target = item.State
		}
		if !retry {
			return ImplementationOutcome{}, Refuse("completed review is missing its matching Review Checkpoint; inspect before changing the handoff")
		}
		if item.Claimed || target != item.State {
			if err := backend.CompleteReview(ctx, repository, item, target, guard); err != nil {
				return ImplementationOutcome{}, err
			}
			current, err := backend.ImplementationItems(ctx, repository)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			for _, c := range current {
				if c.ID == item.ID && c.Problem == "" && !c.Claimed && c.State == target {
					result := completedReviewOutcome(c, head, checkpoint)
					return result, guard()
				}
			}
			return ImplementationOutcome{}, Refuse("review handoff still incomplete; retain Claim and retry")
		}
		result := completedReviewOutcome(item, head, checkpoint)
		return result, guard()
	}
	if verdict == "pass" {
		wanted := *item.Submission
		wanted.Body = finalBody
		published, err := backend.PublishImplementation(ctx, repository, item, wanted)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if published.Head != head {
			return ImplementationOutcome{}, Refuse("Submission head changed during final body publication")
		}
	}
	if err := backend.PublishReview(ctx, repository, item, comments, guard); err != nil {
		return ImplementationOutcome{}, err
	}
	published, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	matches := 0
	var publishedItem ImplementationItem
	for _, current := range published {
		if current.ID == id {
			matches++
			publishedItem = current
		}
	}
	if matches != 1 || publishedItem.Problem != "" || publishedItem.State != AwaitingReview || !publishedItem.Claimed || publishedItem.Submission == nil || !reviewEvidenceMatches(publishedItem, comments, finalBody) {
		return ImplementationOutcome{}, Refuse("published review evidence is not exactly observable; retain the Claim and retry the same fixed-number command and Result Documents")
	}
	if err := checkpoint.replace(reviewNumber, reviewed); err != nil {
		return ImplementationOutcome{}, Refuse(err.Error())
	}
	if err := backend.CompleteReview(ctx, repository, item, target, guard); err != nil {
		return ImplementationOutcome{}, err
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	observed, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for _, current := range observed {
		if current.ID == id && current.Problem == "" && current.State == target && !current.Claimed {
			result := completedReviewOutcome(current, head, checkpoint)
			return result, nil
		}
	}
	return ImplementationOutcome{}, Refuse("review handoff incomplete; retry the same verdict and Result Documents")
}

func reviewEvidenceMatches(item ImplementationItem, wanted []skilldist.ReviewComment, finalBody string) bool {
	for _, comment := range wanted {
		matches := 0
		for _, existing := range item.Submission.Comments {
			if comment.Body == existing.Body && comment.Path == existing.Path && comment.Verdict == existing.Verdict && comment.Commit == existing.Commit && (comment.Path == "" || comment.Line == existing.Line && comment.Side == existing.Side) {
				matches++
			}
		}
		if matches != 1 {
			return false
		}
	}
	return finalBody == "" || finalBody == item.Submission.Body
}

func matchingSummaryReceipt(submission Submission, wanted skilldist.ReviewComment) (skilldist.ReviewComment, int) {
	var receipt skilldist.ReviewComment
	count := 0
	for _, existing := range submission.Comments {
		if existing.Path == "" && existing.Body == wanted.Body && existing.Verdict == wanted.Verdict && existing.Commit == wanted.Commit {
			receipt, count = existing, count+1
		}
	}
	return receipt, count
}

func claimPrecedesReceipt(claimedAt, submittedAt string) bool {
	claim, claimErr := time.Parse(time.RFC3339Nano, claimedAt)
	receipt, receiptErr := time.Parse(time.RFC3339Nano, submittedAt)
	return claimErr == nil && receiptErr == nil && claim.Before(receipt)
}

func cleanupReviewCheckpoint(checkpoint reviewCheckpoint) string {
	if err := checkpoint.remove(); err != nil {
		return err.Error()
	}
	return ""
}

func completedReviewOutcome(item ImplementationItem, head string, checkpoint reviewCheckpoint) ImplementationOutcome {
	result := ImplementationOutcome{Status: string(item.State), Item: &item, Head: head}
	if item.State == ReadyForMerge {
		result.Reason = cleanupReviewCheckpoint(checkpoint)
	}
	return result
}
