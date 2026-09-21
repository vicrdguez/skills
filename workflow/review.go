package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	skilldist "github.com/vicrdguez/skills"
)

type ReviewBackend interface {
	ImplementationBackend
	ReviewSubmission(context.Context, SubmissionID) (Submission, error)
	// AnchorSide reports whether a supplied inline-anchor side is natively publishable.
	AnchorSide(side string) bool
	PublishReview(context.Context, ImplementationItem, []skilldist.ReviewComment, func() error) error
	CompleteReview(context.Context, ImplementationItem, State, func() error) error
}

func SubmitWatchdog(ctx context.Context, root, remote string, id WorkItemID, fixedSubmission SubmissionID, fixedBase, fixedBodySHA256 string, reviewNumber uint64, reviewed, head, verdict, summaryPath, findingsPath, bodyPath string, endpoints ArtifactEndpoints, backend ReviewBackend) (outcome ImplementationOutcome, err error) {
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
	fixedAttachment := fixedSubmission != "" || fixedBase != "" || fixedBodySHA256 != ""
	if id == "" || reviewNumber == 0 || reviewed == "" || verdict != "rework" && verdict != "pass" && verdict != "needs-human" || summaryPath == "" || verdict == "pass" && bodyPath == "" {
		return ImplementationOutcome{}, fmt.Errorf("submit requires --item, positive --review-number, --reviewed-head, --verdict rework|pass|needs-human and --summary; pass also requires --body")
	}
	if fixedAttachment && (fixedSubmission == "" || fixedBase == "" || len(fixedBodySHA256) != 64) {
		return ImplementationOutcome{}, fmt.Errorf("fixed Submission handoff requires --submission, --base, and a 64-character --submission-body-sha256 together")
	}
	finalBody := ""
	if verdict == "pass" {
		body, err := os.ReadFile(bodyPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		finalBody = string(body)
	}
	attachmentMatches := func(submission Submission) (bool, error) {
		if submission.ID != fixedSubmission || submission.Base != fixedBase {
			return false, nil
		}
		if fmt.Sprintf("%x", sha256.Sum256([]byte(submission.Body))) == fixedBodySHA256 {
			return true, nil
		}
		if verdict != "pass" {
			return false, nil
		}
		return backend.SubmissionBodyMatches(id, submission.Body, finalBody)
	}
	items, err := loadImplementation(ctx, backend)
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
	if item.Problem != "" && item.Problem != "contradictory lifecycle projections" || item.Submission == nil || item.State == AwaitingReview && !item.Claimed {
		return ImplementationOutcome{}, Refuse("verdict requires the selected review Claim or an exactly observable fixed-number retry")
	}
	if fixedAttachment {
		matches, err := attachmentMatches(*item.Submission)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if !matches {
			return ImplementationOutcome{}, Refuse("selected Submission attachment changed from this invocation; inspect the fixed handoff and stop")
		}
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
	if err := RefuseNonMainBase(item.Submission.ID, item.Submission.Base); err != nil {
		return ImplementationOutcome{}, err
	}
	if head != reviewed && (verdict != "pass" || gitOK(root, "merge-base", "--is-ancestor", reviewed, head) != nil) {
		return ImplementationOutcome{}, Refuse("post-marker head must descend from the fixed reviewed head on pass")
	}
	guard := func() error {
		local, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
		if err != nil || local != head {
			return Refuse("local reviewed head changed; restore the fixed head")
		}
		remote, err := backend.ImplementationHead(ctx, item.Branch)
		if err != nil {
			return err
		}
		if remote != head {
			return Refuse("remote reviewed head changed; push the fixed head")
		}
		submission, err := backend.ReviewSubmission(ctx, item.Submission.ID)
		if err != nil {
			return err
		}
		if fixedAttachment {
			matches, err := attachmentMatches(submission)
			if err != nil {
				return err
			}
			if !matches {
				return Refuse("selected Submission attachment changed during verdict; inspect the fixed handoff and stop")
			}
		}
		if submission.Head != head || submission.Merged || submission.Draft {
			return Refuse("Submission head changed during verdict")
		}
		if err := RefuseNonMainBase(item.Submission.ID, submission.Base); err != nil {
			return err
		}
		return nil
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	history, err := InspectLedger(root, reviewed, item.Branch, endpoints, RequireRetiredArtifacts)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if history.Phase != "retired" || len(history.Violations) > 0 {
		return ImplementationOutcome{}, Refuse(fmt.Sprint(history.Violations) + "; restore valid retired ledger history at reviewed head " + reviewed)
	}
	if head != reviewed {
		history, err = InspectLedger(root, head, item.Branch, endpoints, RequireRetiredArtifacts)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if history.Phase != "retired" || len(history.Violations) > 0 {
			return ImplementationOutcome{}, Refuse(fmt.Sprint(history.Violations) + "; keep the ledger retired at final head " + head)
		}
	}
	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	comments := []skilldist.ReviewComment{{Body: string(summary), Commit: reviewed, Verdict: verdict, ReviewNumber: reviewNumber}}
	if verdict == "pass" {
		comments[0].FinalHead = head
	}
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
			if a.Path == "" || path.IsAbs(a.Path) || path.Clean(a.Path) != a.Path || strings.HasPrefix(a.Path, "../") || a.Line <= 0 || !backend.AnchorSide(a.Side) {
				return ImplementationOutcome{}, fmt.Errorf("invalid structured inline anchor")
			}
			body, err := os.ReadFile(a.BodyFile)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			comments = append(comments, skilldist.ReviewComment{Body: string(body), Commit: reviewed, Path: a.Path, Line: a.Line, Side: a.Side})
		}
	}
	submission, err := backend.ReviewSubmission(ctx, item.Submission.ID)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for i := range comments {
		comments[i].ClaimAcquiredAt = submission.ClaimAcquiredAt
	}
	receipt, receiptCount := matchingSummaryReceipt(*item.Submission, comments[0])
	bodyMatches := true
	if verdict == "pass" {
		bodyMatches, err = backend.SubmissionBodyMatches(item.ID, item.Submission.Body, finalBody)
		if err != nil {
			return ImplementationOutcome{}, err
		}
	}
	evidenceMatches := bodyMatches && reviewEvidenceMatches(item, comments)
	if retry && item.Claimed && submission.ClaimAcquiredAt != "" {
		var unambiguous bool
		receipt, receiptCount, unambiguous = matchingSummaryReceiptForClaim(*item.Submission, comments[0], submission.ClaimAcquiredAt)
		evidenceMatches = unambiguous && bodyMatches && reviewEvidenceMatchesForClaim(item, comments, submission.ClaimAcquiredAt)
	}
	if retry && (!evidenceMatches || receiptCount != 1) {
		return ImplementationOutcome{}, Refuse("recorded review differs from the supplied summary, verdict, body, or inline evidence; replay the original fixed-number command and Result Documents")
	}
	if retry && item.State == AwaitingReview && !claimPrecedesReceipt(submission.ClaimAcquiredAt, receipt.CreatedAt) {
		return ImplementationOutcome{}, Refuse("recorded review receipt does not belong to the current Awaiting Review Claim; replay the current round's original fixed-number command")
	}
	reviewPublished := retry
	bodyPublished := verdict != "pass" || bodyMatches
	if !retry && item.State == AwaitingReview {
		currentSummaries, unambiguous := reviewSummariesForClaim(item.Submission.Comments, submission.ClaimAcquiredAt)
		if !unambiguous || len(currentSummaries) > 0 && (len(currentSummaries) != 1 || !reviewCommentsMatch(currentSummaries[0], comments[0]) || !reviewEvidenceCompatible(item, comments, submission.ClaimAcquiredAt)) {
			return ImplementationOutcome{}, Refuse("review publication already started under this Claim; replay its original fixed-number command and Result Documents")
		}
		if len(currentSummaries) == 1 {
			reviewPublished = reviewEvidenceMatchesForClaim(item, comments, submission.ClaimAcquiredAt)
		}
	}
	observedItem := item
	target := Rework
	if reviewNumber >= 2 {
		target = NeedsHuman
	}
	if verdict == "needs-human" {
		target = NeedsHuman
	}
	if verdict == "pass" {
		target = ReadyForMerge
	}
	if item.State != AwaitingReview || retry && item.Problem == "contradictory lifecycle projections" {
		if item.Claimed {
			if !retry || submission.ClaimAcquiredAt == "" || !evidenceMatches || receiptCount != 1 || !claimPrecedesReceipt(submission.ClaimAcquiredAt, receipt.CreatedAt) {
				return ImplementationOutcome{}, Refuse("target-only Claim cannot prove it belongs to this review handoff; inspect before replaying the original fixed-number command")
			}
		}
		completedEvidence := bodyMatches && reviewEvidenceMatches(item, comments)
		if item.Claimed && submission.ClaimAcquiredAt != "" {
			completedEvidence = bodyMatches && reviewEvidenceMatchesForClaim(item, comments, submission.ClaimAcquiredAt)
		}
		compatible := verdict == "rework" && (item.State == Rework || item.State == NeedsHuman) || verdict == "needs-human" && item.State == NeedsHuman || verdict == "pass" && item.State == ReadyForMerge
		compatible = compatible || retry && item.Submission.Lifecycle != nil && slices.Contains(item.Submission.Lifecycle.States, target)
		compatible = compatible && completedEvidence
		if !compatible {
			return ImplementationOutcome{}, Refuse("completed or partial review differs from supplied verdict; restore its exact Result Documents")
		}
		if !item.Claimed {
			if !reviewDestinationFinal(observedItem, target) {
				return ImplementationOutcome{}, Refuse("review destination or source cleanup is incomplete; inspect projections without acquiring or releasing a Claim")
			}
		}
		if completedDone {
			return ImplementationOutcome{Status: string(item.State), Item: &item, Head: head}, guard()
		}
		if verdict != "pass" && item.Problem != "contradictory lifecycle projections" {
			target = item.State
		}
		if !retry {
			return ImplementationOutcome{}, Refuse("completed review is missing its matching Review Checkpoint; inspect before changing the handoff")
		}
		if item.Claimed || target != item.State {
			item.Synchronization = false
			if err := backend.CompleteReview(ctx, item, target, guard); err != nil {
				return ImplementationOutcome{}, err
			}
			current, err := backend.ImplementationItems(ctx)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			for _, c := range current {
				if c.ID == item.ID && reviewDestinationFinal(c, target) {
					result := completedReviewOutcome(c, head, checkpoint)
					return result, guard()
				}
			}
			return ImplementationOutcome{}, Refuse("review handoff still incomplete; retain Claim and retry")
		}
		result := completedReviewOutcome(item, head, checkpoint)
		return result, guard()
	}
	if !reviewPublished {
		if err := backend.PublishReview(ctx, item, comments, guard); err != nil {
			return ImplementationOutcome{}, err
		}
	}
	if verdict == "pass" && !bodyPublished {
		wanted := *item.Submission
		wanted.Body = finalBody
		published, err := backend.PublishImplementation(ctx, item, wanted)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if published.Head != head {
			return ImplementationOutcome{}, Refuse("Submission head changed during final body publication")
		}
	}
	published, err := backend.ImplementationItems(ctx)
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
	if matches != 1 || publishedItem.Problem != "" || publishedItem.State != AwaitingReview || !publishedItem.Claimed || publishedItem.Submission == nil {
		return ImplementationOutcome{}, Refuse("published review evidence is not exactly observable; retain the Claim and retry the same fixed-number command and Result Documents")
	}
	if verdict == "pass" {
		bodyMatches, err = backend.SubmissionBodyMatches(item.ID, publishedItem.Submission.Body, finalBody)
		if err != nil {
			return ImplementationOutcome{}, err
		}
	}
	observedEvidence := bodyMatches && reviewEvidenceMatchesForClaim(publishedItem, comments, submission.ClaimAcquiredAt)
	if !observedEvidence {
		return ImplementationOutcome{}, Refuse("published review evidence is not exactly observable; retain the Claim and retry the same fixed-number command and Result Documents")
	}
	if err := checkpoint.replace(reviewNumber, reviewed, guard); err != nil {
		return ImplementationOutcome{}, Refuse(err.Error())
	}
	item.Synchronization = false
	if err := backend.CompleteReview(ctx, item, target, guard); err != nil {
		return ImplementationOutcome{}, err
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	observed, err := backend.ImplementationItems(ctx)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for _, current := range observed {
		if current.ID == id && reviewDestinationFinal(current, target) {
			result := completedReviewOutcome(current, head, checkpoint)
			return result, nil
		}
	}
	return ImplementationOutcome{}, Refuse("review handoff incomplete; retry the same verdict and Result Documents")
}

func reviewDestinationFinal(item ImplementationItem, target State) bool {
	if item.Problem != "" || item.Claimed || item.State != target || item.Synchronization || item.Source == nil || !item.Source.Open || item.Source.Claimed || item.Submission == nil || item.Submission.Lifecycle == nil {
		return false
	}
	if len(item.Source.States) != 0 && (target != NeedsHuman || !slices.Equal(item.Source.States, []State{NeedsHuman})) {
		return false
	}
	pr := item.Submission.Lifecycle
	return pr.Open && !pr.Claimed && slices.Equal(pr.States, []State{target})
}

func reviewEvidenceMatches(item ImplementationItem, wanted []skilldist.ReviewComment) bool {
	for _, comment := range wanted {
		matches := 0
		for _, existing := range item.Submission.Comments {
			if existing.Path != "" && !existing.EvidenceAuthorized {
				continue
			}
			if reviewCommentsMatch(comment, existing) {
				matches++
			}
		}
		if matches != 1 {
			return false
		}
	}
	return true
}

func reviewEvidenceCompatible(item ImplementationItem, wanted []skilldist.ReviewComment, claimedAt string) bool {
	if item.Submission == nil {
		return false
	}
	for _, existing := range item.Submission.Comments {
		if !existing.EvidenceAuthorized || existing.Path == "" || existing.Commit != wanted[0].Commit {
			continue
		}
		after, unambiguous := receiptAfterClaim(claimedAt, existing.CreatedAt)
		if !unambiguous {
			return false
		}
		if !after {
			continue
		}
		matches := 0
		for _, comment := range wanted[1:] {
			if reviewCommentsMatch(comment, existing) {
				matches++
			}
		}
		if matches != 1 {
			return false
		}
	}
	return true
}

func reviewEvidenceMatchesForClaim(item ImplementationItem, wanted []skilldist.ReviewComment, claimedAt string) bool {
	if !reviewEvidenceCompatible(item, wanted, claimedAt) {
		return false
	}
	summaries, unambiguous := reviewSummariesForClaim(item.Submission.Comments, claimedAt)
	if !unambiguous || len(summaries) != 1 || !reviewCommentsMatch(summaries[0], wanted[0]) {
		return false
	}
	for _, comment := range wanted[1:] {
		matches := 0
		for _, existing := range item.Submission.Comments {
			if !existing.EvidenceAuthorized || existing.Path == "" || existing.Commit != comment.Commit {
				continue
			}
			after, unambiguous := receiptAfterClaim(claimedAt, existing.CreatedAt)
			if !unambiguous {
				return false
			}
			if after && reviewCommentsMatch(comment, existing) {
				matches++
			}
		}
		if matches != 1 {
			return false
		}
	}
	return true
}

func reviewCommentsMatch(a, b skilldist.ReviewComment) bool {
	return a.Body == b.Body && a.Path == b.Path && a.Verdict == b.Verdict && a.Commit == b.Commit && a.FinalHead == b.FinalHead && (a.Path != "" || a.ReviewNumber == b.ReviewNumber) && (a.Path == "" || a.Line == b.Line && a.Side == b.Side)
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

func matchingSummaryReceipt(submission Submission, wanted skilldist.ReviewComment) (skilldist.ReviewComment, int) {
	var receipt skilldist.ReviewComment
	count := 0
	for _, existing := range submission.Comments {
		if existing.Path == "" && reviewCommentsMatch(existing, wanted) {
			receipt, count = existing, count+1
		}
	}
	return receipt, count
}

func matchingSummaryReceiptForClaim(submission Submission, wanted skilldist.ReviewComment, claimedAt string) (skilldist.ReviewComment, int, bool) {
	summaries, unambiguous := reviewSummariesForClaim(submission.Comments, claimedAt)
	if !unambiguous {
		return skilldist.ReviewComment{}, 0, false
	}
	receipt, count := matchingSummaryReceipt(Submission{Comments: summaries}, wanted)
	return receipt, count, true
}

func claimPrecedesReceipt(claimedAt, submittedAt string) bool {
	after, unambiguous := receiptAfterClaim(claimedAt, submittedAt)
	return unambiguous && after
}

func receiptAfterClaim(claimedAt, submittedAt string) (bool, bool) {
	claim, claimErr := time.Parse(time.RFC3339Nano, claimedAt)
	receipt, receiptErr := time.Parse(time.RFC3339Nano, submittedAt)
	if claimErr != nil || receiptErr != nil || claim.Equal(receipt) {
		return false, false
	}
	return claim.Before(receipt), true
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
