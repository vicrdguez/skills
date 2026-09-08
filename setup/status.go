package setup

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

func (b *GitHubBackend) CoordinationItems(ctx context.Context, repository github.RepositoryID) ([]workflow.CoordinationItem, error) {
	issues, err := b.listIssues(ctx, repository)
	if err != nil {
		return nil, err
	}
	var parents []workflow.CoordinationItem
	for _, issue := range issues {
		if len(issue.PullRequest) != 0 || issue.SubIssuesSummary.Total == 0 {
			continue
		}
		parent := workflow.CoordinationItem{ID: workflow.WorkItemID(strconv.Itoa(issue.Number)), Closed: issue.State == "closed"}
		for page := 1; ; page++ {
			var children []githubIssue
			if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/sub_issues?per_page=100&page=%d", issue.Number, page), nil, &children); err != nil {
				return nil, err
			}
			for _, child := range children {
				parent.Children = append(parent.Children, workflow.WorkItemID(strconv.Itoa(child.Number)))
			}
			if len(children) < 100 {
				break
			}
		}
		if len(parent.Children) != issue.SubIssuesSummary.Total {
			return nil, fmt.Errorf("Coordination Item children changed during observation; retry status")
		}
		parents = append(parents, parent)
	}
	return parents, nil
}

func (b *GitHubBackend) CloseCoordination(ctx context.Context, repository github.RepositoryID, id workflow.WorkItemID) error {
	number, err := githubIssueNumber(id)
	if err != nil {
		return err
	}
	path := b.repositoryPath(repository) + fmt.Sprintf("/issues/%d", number)
	var issue githubIssue
	if err := b.request(ctx, http.MethodGet, path, nil, &issue); err != nil {
		return err
	}
	if issue.State == "closed" {
		return nil
	}
	writeErr := b.request(ctx, http.MethodPatch, path, map[string]string{"state": "closed"}, nil)
	if err := b.request(ctx, http.MethodGet, path, nil, &issue); err != nil {
		return err
	}
	if issue.State == "closed" {
		return nil
	}
	if writeErr != nil {
		return writeErr
	}
	return fmt.Errorf("Coordination Item closure not observed; retry status")
}
