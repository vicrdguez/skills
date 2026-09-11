package setup

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

func (b *GitHubBackend) CompleteReview(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem, target workflow.State, guard func() error) error {
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

func (b *GitHubBackend) ReviewSubmission(ctx context.Context, repository github.RepositoryID, id workflow.SubmissionID) (workflow.Submission, error) {
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
	latest := ""
	synchronizing := false
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
			return workflow.Submission{}, err
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
			if event.Event == "labeled" && (event.Label.Name == "review" || event.Label.Name == "rework" || event.Label.Name == "done" || event.Label.Name == "needs-human") {
				// A conflicting pass retry can add rework before its claimed review is removed.
				synchronizing = event.Label.Name == "rework" && latest == "done" && labels["done"] && labels["sync"] && (!labels["review"] || labels["wip"]) && !labels["needs-human"] && !labels["ready"]
				latest = event.Label.Name
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
	if current["review"] && claimed && states == 1 && labels["wip"] && !claimAmbiguous {
		result.ClaimAcquiredAt = claimAcquiredAt
	}
	if (states == 2 && !current["review"] || states == 3 && current["review"] && claimed && labels["wip"]) && current["review"] == labels["review"] && current["done"] && current["rework"] && current["sync"] && labels["done"] && labels["rework"] && labels["sync"] && synchronizing {
		result.PendingReview = workflow.Rework
		result.State = workflow.Rework
	}
	return result, nil
}

func (b *GitHubBackend) PublishReview(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem, comments []skilldist.ReviewComment, guard func() error) error {
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
			if comment.Verdict != "" {
				if err := b.publishReviewSummary(ctx, repository, number, comment); err != nil {
					return err
				}
				continue
			}
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

func (b *GitHubBackend) publishReviewSummary(ctx context.Context, repository github.RepositoryID, number int, wanted skilldist.ReviewComment) error {
	states := map[string]string{"rework": "CHANGES_REQUESTED", "pass": "APPROVED", "needs-human": "COMMENTED"}
	events := map[string]string{"rework": "REQUEST_CHANGES", "pass": "APPROVE", "needs-human": "COMMENT"}
	state, event := states[wanted.Verdict], events[wanted.Verdict]
	if state == "" {
		return fmt.Errorf("invalid review verdict %q", wanted.Verdict)
	}
	path := b.repositoryPath(repository) + fmt.Sprintf("/pulls/%d/reviews", number)
	published := func() (int, error) {
		type review struct {
			Body   string `json:"body"`
			Commit string `json:"commit_id"`
			State  string `json:"state"`
		}
		matches := 0
		for page := 1; ; page++ {
			var reviews []review
			if err := b.request(ctx, http.MethodGet, path+fmt.Sprintf("?per_page=100&page=%d", page), nil, &reviews); err != nil {
				return 0, err
			}
			for _, review := range reviews {
				if review.Body == wanted.Body && review.Commit == wanted.Commit && review.State == state {
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
	writeErr := b.request(ctx, http.MethodPost, path, map[string]string{"body": wanted.Body, "commit_id": wanted.Commit, "event": event}, nil)
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
