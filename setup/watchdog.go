package setup

import (
	"context"
	"fmt"
	"net/http"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

func (b *GitHubBackend) CompleteReview(ctx context.Context, repository workflow.RepositoryID, item workflow.ImplementationItem, target workflow.State, guard func() error) error {
	label := map[workflow.State]string{workflow.Rework: "rework", workflow.NeedsHuman: "needs-human", workflow.ReadyForMerge: "done"}[target]
	if label == "" || item.Submission == nil {
		return fmt.Errorf("invalid review target or missing Submission")
	}
	if err := guard(); err != nil {
		return err
	}
	if item.Synchronization && target == workflow.Rework {
		if err := b.publishImplementationMetadata(ctx, repository, item.Number, implementationMetadata{SynchronizationTarget: item.TargetSnapshot, TargetBranch: item.TargetBranch}); err != nil {
			return err
		}
		if err := b.implementationLabelMutation(ctx, repository, item.Submission.Number, []string{"sync"}, nil, guard); err != nil {
			return err
		}
	}
	if target == workflow.NeedsHuman {
		if err := b.publishImplementationMetadata(ctx, repository, item.Number, implementationMetadata{ResumeState: item.ResumeState}); err != nil {
			return err
		}
	}
	remove := []string{"review", "wip"}
	if target != workflow.Rework {
		remove = append(remove, "rework")
	}
	return b.implementationLabelMutation(ctx, repository, item.Submission.Number, []string{label}, remove, guard)
}

func (b *GitHubBackend) ReviewSubmission(ctx context.Context, repository workflow.RepositoryID, number int) (workflow.Submission, error) {
	var pull githubPull
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d", number), nil, &pull); err != nil {
		return workflow.Submission{}, err
	}
	state, claimed, _ := implementationLabels(pull.githubIssue)
	result := workflow.Submission{Number: pull.Number, Head: pull.Head.SHA, Base: pull.Base.Ref, Body: pull.Body, Draft: pull.Draft, State: state, Claimed: claimed, Merged: pull.Merged || pull.MergedAt != "", Mergeability: "unknown"}
	if pull.Mergeable != nil {
		result.Mergeability = "conflicting"
		if *pull.Mergeable {
			result.Mergeability = "mergeable"
		}
	}
	labels := map[string]bool{}
	reviewing := false
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
			if event.Event == "labeled" && event.Label.Name == "review" {
				reviewing = true
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
	return result, nil
}

func (b *GitHubBackend) PublishReview(ctx context.Context, repository workflow.RepositoryID, item workflow.ImplementationItem, comments []skilldist.ReviewComment, guard func() error) error {
	for _, comment := range comments {
		if err := guard(); err != nil {
			return err
		}
		if comment.Path == "" {
			if err := b.implementationComment(ctx, repository, item.Submission.Number, comment.Body, false); err != nil {
				return err
			}
			continue
		}
		stream := fmt.Sprintf("/pulls/%d/comments", item.Submission.Number)
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
