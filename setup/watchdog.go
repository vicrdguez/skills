package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

const reviewSummaryPrefix = "<!-- skl.watchdog.review/v1\n"

type reviewSummaryMetadata struct {
	ReviewNumber uint64 `json:"review_number"`
	Verdict      string `json:"verdict"`
	FinalHead    string `json:"final_head,omitempty"`
}

func reviewSummaryBody(comment skilldist.ReviewComment) (string, error) {
	if comment.ReviewNumber == 0 || comment.Verdict != "rework" && comment.Verdict != "pass" && comment.Verdict != "needs-human" {
		return "", fmt.Errorf("invalid review number or verdict")
	}
	metadata, err := json.Marshal(reviewSummaryMetadata{ReviewNumber: comment.ReviewNumber, Verdict: comment.Verdict, FinalHead: comment.FinalHead})
	if err != nil {
		return "", err
	}
	return reviewSummaryPrefix + string(metadata) + "\n-->\n" + comment.Body, nil
}

func parseReviewSummary(body string) (reviewSummaryMetadata, string, bool) {
	body, ok := strings.CutPrefix(body, reviewSummaryPrefix)
	if !ok {
		return reviewSummaryMetadata{}, "", false
	}
	metadata, body, ok := strings.Cut(body, "\n-->\n")
	if !ok {
		return reviewSummaryMetadata{}, "", false
	}
	var parsed reviewSummaryMetadata
	if json.Unmarshal([]byte(metadata), &parsed) != nil || parsed.ReviewNumber == 0 || parsed.Verdict != "rework" && parsed.Verdict != "pass" && parsed.Verdict != "needs-human" {
		return reviewSummaryMetadata{}, "", false
	}
	return parsed, body, true
}

func (b *GitHubBackend) verifyReviewOwnership(ctx context.Context, itemNumber, submissionNumber int) error {
	pull, err := b.pullRecord(ctx, submissionNumber)
	if err != nil {
		return err
	}
	if owner, problem := submissionOwner(pull.Body); problem != "" || owner != itemNumber || pull.Number != submissionNumber || !strings.EqualFold(pull.Head.Repo.FullName, b.repository.Owner+"/"+b.repository.Name) {
		return workflow.Refuse("Submission owning association changed during review; retain the Claim and inspect before retrying")
	}
	return b.verifyOwningAssociation(ctx, itemNumber, submissionNumber)
}

func (b *GitHubBackend) CompleteReview(ctx context.Context, item workflow.ImplementationItem, target workflow.State, guard func() error) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	repository := b.repository
	label := map[workflow.State]string{workflow.Rework: "rework", workflow.NeedsHuman: "needs-human", workflow.ReadyForMerge: "done"}[target]
	if label == "" || item.Submission == nil {
		return fmt.Errorf("invalid review target or missing Submission")
	}
	itemNumber, submissionNumber, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	checkHead := guard
	guard = func() error {
		if err := checkHead(); err != nil {
			return err
		}
		return b.verifyReviewOwnership(ctx, itemNumber, submissionNumber)
	}
	if err := guard(); err != nil {
		return err
	}
	if err := b.implementationLabelMutation(ctx, repository, submissionNumber, []string{"wip"}, nil, guard); err != nil {
		return err
	}
	if err := b.implementationLabelMutation(ctx, repository, submissionNumber, []string{label}, nil, guard); err != nil {
		return err
	}
	// Keep the target overlap and Claim until source cleanup is observed.
	if target != workflow.NeedsHuman {
		if err := b.implementationLabelMutation(ctx, repository, itemNumber, nil, []string{"needs-human"}, guard); err != nil {
			return err
		}
	}
	remove := []string{"review", "sync"}
	if target != workflow.Rework {
		remove = append(remove, "rework")
	}
	if target != workflow.ReadyForMerge {
		remove = append(remove, "done")
	}
	return b.implementationLabelMutation(ctx, repository, submissionNumber, nil, append(remove, "wip"), guard)
}

// issueClaimAcquiredAt observes the latest unambiguous `wip` acquisition from a
// record timeline. An empty result means no currently observed Claim or an
// ambiguous one; it is never direction inference.
func (b *GitHubBackend) issueClaimAcquiredAt(ctx context.Context, repository github.RepositoryID, number int) (string, error) {
	labels := map[string]bool{}
	claimAcquiredAt := ""
	claimAmbiguous := false
	for page := 1; ; page++ {
		var events []struct {
			Event     string `json:"event"`
			CreatedAt string `json:"created_at"`
			Label     struct {
				Name string `json:"name"`
			} `json:"label"`
		}
		if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/timeline?per_page=100&page=%d", number, page), nil, &events); err != nil {
			return "", err
		}
		for _, event := range events {
			if event.Event != "labeled" && event.Event != "unlabeled" {
				continue
			}
			if event.Label.Name == "wip" {
				if event.Event == "labeled" {
					claimAmbiguous = claimAmbiguous || labels["wip"]
					claimAcquiredAt = event.CreatedAt
				} else {
					claimAcquiredAt = ""
					claimAmbiguous = false
				}
			}
			labels[event.Label.Name] = event.Event == "labeled"
		}
		if len(events) < 100 {
			break
		}
	}
	if !labels["wip"] || claimAmbiguous {
		return "", nil
	}
	return claimAcquiredAt, nil
}

func (b *GitHubBackend) AnchorSide(side string) bool {
	return side == "LEFT" || side == "RIGHT"
}

func (b *GitHubBackend) ReviewSubmission(ctx context.Context, id workflow.SubmissionID) (workflow.Submission, error) {
	if err := b.requireRepository(); err != nil {
		return workflow.Submission{}, err
	}
	repository := b.repository
	number, err := githubIssueNumber(workflow.WorkItemID(id))
	if err != nil {
		return workflow.Submission{}, err
	}
	var pull githubPull
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d", number), nil, &pull); err != nil {
		return workflow.Submission{}, err
	}
	if pull.Number != number || !strings.EqualFold(pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name) {
		return workflow.Submission{}, workflow.Refuse("observed Submission identity or head repository is outside the selected repository attachment; inspect and repair its association")
	}
	state, claimed, _ := implementationLabels(pull.githubIssue)
	result := workflow.Submission{ID: workflow.SubmissionID(strconv.Itoa(pull.Number)), Branch: pull.Head.Ref, Head: pull.Head.SHA, Base: pull.Base.Ref, Body: pull.Body, Draft: pull.Draft, State: state, Claimed: claimed, Merged: pull.Merged || pull.MergedAt != "", Mergeability: "unknown", Lifecycle: implementationLifecycle(pull.githubIssue)}
	result.Lifecycle.Merged = result.Merged
	if pull.Mergeable != nil {
		result.Mergeability = "conflicting"
		if *pull.Mergeable {
			result.Mergeability = "mergeable"
		}
	}
	// An interrupted review is identified by its observed labels, never by
	// timeline order: the pending target is the other target label beside a
	// retained review, or the single lifecycle state of a claimed record.
	current := map[string]bool{}
	states := 0
	for _, label := range pull.Labels {
		current[label.Name] = true
		if label.Name == "review" || label.Name == "rework" || label.Name == "done" || label.Name == "needs-human" || label.Name == "ready" {
			states++
		}
	}
	if current["review"] && states == 2 && current["done"] {
		result.PendingReview = workflow.ReadyForMerge
		result.State = workflow.ReadyForMerge
	}
	if states == 1 && claimed && (state == workflow.ReadyForMerge || state == workflow.NeedsHuman) {
		result.PendingReview = state
	}
	if claimed {
		result.ClaimAcquiredAt, err = b.issueClaimAcquiredAt(ctx, repository, number)
		if err != nil {
			return workflow.Submission{}, err
		}
	}
	return result, nil
}

func (b *GitHubBackend) PublishReview(ctx context.Context, item workflow.ImplementationItem, comments []skilldist.ReviewComment, guard func() error) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	repository := b.repository
	if item.Submission == nil {
		return fmt.Errorf("review requires a Submission")
	}
	itemNumber, number, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	checkHead := guard
	guard = func() error {
		if err := checkHead(); err != nil {
			return err
		}
		return b.verifyReviewOwnership(ctx, itemNumber, number)
	}
	for _, comment := range comments {
		if err := guard(); err != nil {
			return err
		}
		if comment.Path == "" {
			if comment.Verdict != "" {
				if err := b.publishReviewSummary(ctx, repository, number, comment); err != nil {
					return err
				}
				continue
			}
			if err := b.implementationComment(ctx, repository, number, comment.Body, ""); err != nil {
				return err
			}
			continue
		}
		stream := fmt.Sprintf("/pulls/%d/comments", number)
		published := func() (bool, error) {
			current, err := b.implementationComments(ctx, repository, stream)
			for _, c := range current {
				if c.EvidenceAuthorized && c.Body == comment.Body && c.Commit == comment.Commit && c.Path == comment.Path && c.Line == comment.Line && c.Side == comment.Side && afterClaim(comment.ClaimAcquiredAt, c.CreatedAt) {
					return true, err
				}
			}
			return false, err
		}
		if found, err := published(); err != nil {
			return err
		} else if found {
			continue
		}
		writeErr := b.request(ctx, http.MethodPost, b.repositoryPath(repository)+stream, map[string]any{"body": comment.Body, "commit_id": comment.Commit, "path": comment.Path, "line": comment.Line, "side": comment.Side}, nil)
		if found, err := published(); err != nil {
			return err
		} else if !found {
			if writeErr != nil {
				return writeErr
			}
			return fmt.Errorf("inline publication not observed; retry the same finding")
		}
	}
	return guard()
}

func (b *GitHubBackend) publishReviewSummary(ctx context.Context, repository github.RepositoryID, number int, wanted skilldist.ReviewComment) error {
	body, err := reviewSummaryBody(wanted)
	if err != nil {
		return err
	}
	path := b.repositoryPath(repository) + fmt.Sprintf("/pulls/%d/reviews", number)
	published := func() (int, error) {
		type review struct {
			Body        string `json:"body"`
			Commit      string `json:"commit_id"`
			State       string `json:"state"`
			SubmittedAt string `json:"submitted_at"`
			Association string `json:"author_association"`
		}
		matches := 0
		for page := 1; ; page++ {
			var reviews []review
			if err := b.request(ctx, http.MethodGet, path+fmt.Sprintf("?per_page=100&page=%d", page), nil, &reviews); err != nil {
				return 0, err
			}
			for _, review := range reviews {
				if trustedMetadata(skilldist.ReviewComment{Association: review.Association}) && review.Body == body && review.Commit == wanted.Commit && review.State == "COMMENTED" && afterClaim(wanted.ClaimAcquiredAt, review.SubmittedAt) {
					matches++
				}
			}
			if len(reviews) < 100 {
				return matches, nil
			}
		}
	}
	if found, err := published(); err != nil || found == 1 {
		return err
	} else if found > 1 {
		return fmt.Errorf("multiple exact review summary receipts observed; inspect before retrying")
	}
	writeErr := b.request(ctx, http.MethodPost, path, map[string]string{"body": body, "commit_id": wanted.Commit, "event": "COMMENT"}, nil)
	if found, err := published(); err != nil {
		return err
	} else if found == 1 {
		return nil
	} else if found > 1 {
		return fmt.Errorf("multiple exact review summary receipts observed after publication; inspect before retrying")
	}
	if writeErr != nil {
		return writeErr
	}
	return fmt.Errorf("review summary publication not observed; retry the same fixed-number command")
}

func afterClaim(claimedAt, createdAt string) bool {
	if claimedAt == "" {
		return true
	}
	claim, claimErr := time.Parse(time.RFC3339Nano, claimedAt)
	created, createdErr := time.Parse(time.RFC3339Nano, createdAt)
	return claimErr == nil && createdErr == nil && claim.Before(created)
}
