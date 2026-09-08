package setup

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

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
	if err := guard(); err != nil {
		return err
	}
	if item.Synchronization && target == workflow.Rework {
		if err := b.publishImplementationMetadata(ctx, repository, itemNumber, implementationMetadata{SynchronizationTarget: item.TargetSnapshot, TargetBranch: item.TargetBranch}); err != nil {
			return err
		}
		if err := b.implementationLabelMutation(ctx, repository, submissionNumber, []string{"sync"}, nil, guard); err != nil {
			return err
		}
	}
	if target == workflow.NeedsHuman {
		if err := b.publishImplementationMetadata(ctx, repository, itemNumber, implementationMetadata{ResumeState: item.ResumeState}); err != nil {
			return err
		}
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
	remove := []string{"review"}
	if target != workflow.Rework {
		remove = append(remove, "rework")
	}
	if target != workflow.ReadyForMerge {
		remove = append(remove, "done")
	}
	return b.implementationLabelMutation(ctx, repository, submissionNumber, nil, append(remove, "wip"), guard)
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
	state, claimed, _ := implementationLabels(pull.githubIssue)
	result := workflow.Submission{ID: workflow.SubmissionID(strconv.Itoa(pull.Number)), Head: pull.Head.SHA, Base: pull.Base.Ref, Body: pull.Body, Draft: pull.Draft, State: state, Claimed: claimed, Merged: pull.Merged || pull.MergedAt != "", Mergeability: "unknown"}
	if pull.Mergeable != nil {
		result.Mergeability = "conflicting"
		if *pull.Mergeable {
			result.Mergeability = "mergeable"
		}
	}
	labels := map[string]bool{}
	reviewing := false
	latest := ""
	reviewExited := false
	synchronizing := false
	for page := 1; ; page++ {
		var events []struct {
			Event string `json:"event"`
			Label struct {
				Name string `json:"name"`
			} `json:"label"`
		}
		if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/timeline?per_page=100&page=%d", number, page), nil, &events); err != nil {
			return workflow.Submission{}, err
		}
		for _, event := range events {
			if event.Event != "labeled" && event.Event != "unlabeled" {
				continue
			}
			labels[event.Label.Name] = event.Event == "labeled"
			if event.Event == "unlabeled" && event.Label.Name == "review" && labels["rework"] {
				reviewExited = true
			}
			if event.Event == "labeled" && event.Label.Name == "wip" {
				reviewExited = false
			}
			if event.Event == "labeled" && (event.Label.Name == "review" || event.Label.Name == "rework" || event.Label.Name == "done" || event.Label.Name == "needs-human") {
				// A conflicting pass retry can add rework before its claimed review is removed.
				synchronizing = event.Label.Name == "rework" && latest == "done" && labels["done"] && labels["sync"] && (!labels["review"] || labels["wip"]) && !labels["needs-human"] && !labels["ready"]
				latest = event.Label.Name
			}
			if event.Event == "labeled" && event.Label.Name == "review" {
				reviewing = true
			}
			// A completed pause ends this review; a later human requeue is not a bounce.
			if labels["needs-human"] && !labels["review"] && !labels["wip"] {
				reviewing = false
			}
			if reviewing && labels["rework"] && !labels["review"] && !labels["wip"] {
				if !labels["sync"] {
					result.Bounces++
				}
				reviewing = false
			}
		}
		if len(events) < 100 {
			break
		}
	}
	current := map[string]bool{}
	states := 0
	for _, label := range pull.Labels {
		current[label.Name] = true
		if label.Name == "review" || label.Name == "rework" || label.Name == "done" || label.Name == "needs-human" || label.Name == "ready" {
			states++
		}
	}
	if current["review"] && states == 2 && current[latest] && latest != "review" && latest != "ready" {
		result.PendingReview = map[string]workflow.State{"rework": workflow.Rework, "done": workflow.ReadyForMerge, "needs-human": workflow.NeedsHuman}[latest]
		result.State = result.PendingReview
	}
	if (states == 2 && !current["review"] || states == 3 && current["review"] && claimed && labels["wip"]) && current["review"] == labels["review"] && current["done"] && current["rework"] && current["sync"] && labels["done"] && labels["rework"] && labels["sync"] && synchronizing {
		result.PendingReview = workflow.Rework
		result.State = workflow.Rework
	}
	if states == 1 && claimed && (state == workflow.ReadyForMerge || state == workflow.NeedsHuman) {
		result.PendingReview = state
	}
	if states == 1 && claimed && state == workflow.Rework && reviewExited {
		result.PendingReview = state
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
	number, err := githubIssueNumber(workflow.WorkItemID(item.Submission.ID))
	if err != nil {
		return err
	}
	for _, comment := range comments {
		if err := guard(); err != nil {
			return err
		}
		if comment.Path == "" {
			if err := b.implementationComment(ctx, repository, number, comment.Body, false); err != nil {
				return err
			}
			continue
		}
		stream := fmt.Sprintf("/pulls/%d/comments", number)
		published := func() (bool, error) {
			current, err := b.implementationComments(ctx, repository, stream)
			for _, c := range current {
				if c.Body == comment.Body && c.Commit == comment.Commit && c.Path == comment.Path && c.Line == comment.Line && c.Side == comment.Side {
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
