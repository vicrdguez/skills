package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

const reviewSummaryPrefix = "<!-- skl.watchdog.review/v1\n"

type reviewSummaryMetadata struct {
	ReviewNumber uint64 `json:"review_number"`
	Verdict      string `json:"verdict"`
	FinalHead    string `json:"final_head,omitempty"`
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
		return b.verifySubmissionOwnership(ctx, itemNumber, submissionNumber)
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
