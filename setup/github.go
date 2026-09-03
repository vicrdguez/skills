package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/vicrdguez/skills/workflow"
)

type GitHubBackend struct {
	baseURL     string
	token       string
	tokenSource func() (string, error)
	client      *http.Client
	issueIDs    map[int]int64
	issueBodies map[int]string
}

func NewGitHubBackend(baseURL, token string, client *http.Client) *GitHubBackend {
	return &GitHubBackend{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: client, issueIDs: make(map[int]int64), issueBodies: make(map[int]string)}
}

func NewGitHubBackendFromEnv() (Backend, error) {
	return newGitHubBackend("https://api.github.com", http.DefaultClient, func() (string, error) {
		return resolveGitHubToken(os.Getenv, func() (string, error) {
			output, err := exec.Command("gh", "auth", "token").Output()
			return strings.TrimSpace(string(output)), err
		})
	}), nil
}

func newGitHubBackend(baseURL string, client *http.Client, tokenSource func() (string, error)) *GitHubBackend {
	return &GitHubBackend{baseURL: strings.TrimRight(baseURL, "/"), tokenSource: tokenSource, client: client, issueIDs: make(map[int]int64), issueBodies: make(map[int]string)}
}

type githubIssue struct {
	ID     int64  `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest json.RawMessage `json:"pull_request"`
}

func (b *GitHubBackend) FindWorkItems(ctx context.Context, repository workflow.RepositoryID, titles []string) ([]workflow.WorkItem, error) {
	issues, err := b.listIssues(ctx, repository)
	if err != nil {
		return nil, err
	}
	items := make([]workflow.WorkItem, 0, len(issues))
	for _, issue := range issues {
		if len(issue.PullRequest) != 0 || !slices.Contains(titles, issue.Title) {
			continue
		}
		item, err := b.normalizeWorkItem(ctx, repository, issue)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (b *GitHubBackend) ListMergedWorkItems(ctx context.Context, repository workflow.RepositoryID) ([]workflow.WorkItem, error) {
	issues, err := b.listIssues(ctx, repository)
	if err != nil {
		return nil, err
	}
	var items []workflow.WorkItem
	for _, issue := range issues {
		if len(issue.PullRequest) != 0 || issue.State != "closed" || !hasWorkflowLabel(issue) {
			continue
		}
		var events []struct {
			Event    string `json:"event"`
			CommitID string `json:"commit_id"`
		}
		path := b.repositoryPath(repository) + fmt.Sprintf("/issues/%d/timeline?per_page=100", issue.Number)
		if err := b.request(ctx, http.MethodGet, path, nil, &events); err != nil {
			return nil, err
		}
		merged := false
		for _, event := range events {
			merged = merged || event.Event == "closed" && event.CommitID != ""
		}
		if merged {
			items = append(items, workflow.WorkItem{Number: issue.Number, Title: issue.Title, Body: issue.Body, Branch: issue.Title, Merged: true})
		}
	}
	return items, nil
}

func hasWorkflowLabel(issue githubIssue) bool {
	for _, label := range issue.Labels {
		if slices.Contains([]string{"ready", "wip", "review", "rework", "needs-human", "done"}, label.Name) {
			return true
		}
	}
	return false
}

func (b *GitHubBackend) normalizeWorkItem(ctx context.Context, repository workflow.RepositoryID, issue githubIssue) (workflow.WorkItem, error) {
	item := workflow.WorkItem{Number: issue.Number, Title: issue.Title, Body: issue.Body}
	for _, label := range issue.Labels {
		item.Ready = item.Ready || label.Name == "ready"
	}
	var parent githubIssue
	found, err := b.requestOptional(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/parent", issue.Number), &parent)
	if err != nil {
		return workflow.WorkItem{}, err
	}
	if found {
		item.Parent = parent.Number
		b.issueIDs[parent.Number] = parent.ID
	}
	var blockers []githubIssue
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/dependencies/blocked_by", issue.Number), nil, &blockers); err != nil {
		return workflow.WorkItem{}, err
	}
	for _, blocker := range blockers {
		item.Blockers = append(item.Blockers, blocker.Number)
		b.issueIDs[blocker.Number] = blocker.ID
	}
	return item, nil
}

func (b *GitHubBackend) CreateWorkItem(ctx context.Context, repository workflow.RepositoryID, item workflow.WorkItem) (workflow.WorkItem, error) {
	issue, err := b.createIssue(ctx, repository, item.Title, item.Body)
	if err != nil {
		return workflow.WorkItem{}, err
	}
	item.Number = issue.Number
	b.issueIDs[issue.Number] = issue.ID
	b.issueBodies[issue.Number] = item.Body
	return item, nil
}

func (b *GitHubBackend) FindCoordinationItems(ctx context.Context, repository workflow.RepositoryID, title string) ([]workflow.CoordinationItem, error) {
	issues, err := b.listIssues(ctx, repository)
	if err != nil {
		return nil, err
	}
	items := make([]workflow.CoordinationItem, 0, len(issues))
	for _, issue := range issues {
		if len(issue.PullRequest) == 0 && issue.Title == title {
			items = append(items, workflow.CoordinationItem{Number: issue.Number, Title: issue.Title, Body: issue.Body})
		}
	}
	return items, nil
}

func (b *GitHubBackend) CreateCoordinationItem(ctx context.Context, repository workflow.RepositoryID, item workflow.CoordinationItem) (workflow.CoordinationItem, error) {
	issue, err := b.createIssue(ctx, repository, item.Title, item.Body)
	if err != nil {
		return workflow.CoordinationItem{}, err
	}
	item.Number = issue.Number
	b.issueIDs[issue.Number] = issue.ID
	b.issueBodies[issue.Number] = item.Body
	return item, nil
}

func (b *GitHubBackend) AddChild(ctx context.Context, repository workflow.RepositoryID, parent, child int) error {
	id, ok := b.issueIDs[child]
	if !ok {
		return fmt.Errorf("GitHub issue id unavailable for #%d", child)
	}
	return b.request(ctx, http.MethodPost, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/sub_issues", parent), map[string]int64{"sub_issue_id": id}, nil)
}

func (b *GitHubBackend) AddDependency(ctx context.Context, repository workflow.RepositoryID, dependent, blocker int) error {
	id, ok := b.issueIDs[blocker]
	if !ok {
		return fmt.Errorf("GitHub issue id unavailable for #%d", blocker)
	}
	status, err := b.requestStatus(ctx, http.MethodPost, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/dependencies/blocked_by", dependent), map[string]int64{"issue_id": id}, nil)
	if err == nil || status != http.StatusNotFound && status != http.StatusGone {
		return err
	}
	body, ok := b.issueBodies[dependent]
	if !ok {
		return fmt.Errorf("GitHub issue body unavailable for #%d", dependent)
	}
	body = strings.TrimRight(body, "\n") + fmt.Sprintf("\n\nBlocked by: #%d\n", blocker)
	b.issueBodies[dependent] = body
	return b.request(ctx, http.MethodPatch, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d", dependent), map[string]string{"body": body}, nil)
}

func (b *GitHubBackend) SetReady(ctx context.Context, repository workflow.RepositoryID, number int) error {
	return b.request(ctx, http.MethodPost, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/labels", number), map[string][]string{"labels": {"ready"}}, nil)
}

func (b *GitHubBackend) listIssues(ctx context.Context, repository workflow.RepositoryID) ([]githubIssue, error) {
	var all []githubIssue
	for page := 1; ; page++ {
		var issues []githubIssue
		path := fmt.Sprintf("%s/issues?state=all&per_page=100&page=%d", b.repositoryPath(repository), page)
		if err := b.request(ctx, http.MethodGet, path, nil, &issues); err != nil {
			return nil, err
		}
		for _, issue := range issues {
			b.issueIDs[issue.Number] = issue.ID
			b.issueBodies[issue.Number] = issue.Body
		}
		all = append(all, issues...)
		if len(issues) < 100 {
			return all, nil
		}
	}
}

func (b *GitHubBackend) createIssue(ctx context.Context, repository workflow.RepositoryID, title, body string) (githubIssue, error) {
	var issue githubIssue
	err := b.request(ctx, http.MethodPost, b.repositoryPath(repository)+"/issues", map[string]string{"title": title, "body": body}, &issue)
	return issue, err
}

func resolveGitHubToken(getenv func(string) string, ghToken func() (string, error)) (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(getenv(name)); token != "" {
			return token, nil
		}
	}
	token, err := ghToken()
	if err != nil || strings.TrimSpace(token) == "" {
		return "", errors.New("GitHub authentication unavailable: set GH_TOKEN or GITHUB_TOKEN, or run gh auth login")
	}
	return strings.TrimSpace(token), nil
}

func (b *GitHubBackend) Validate(ctx context.Context, repository RepositoryID) (string, error) {
	var response struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository), nil, &response); err != nil {
		return "", err
	}
	if response.DefaultBranch == "" {
		return "", errors.New("GitHub repository returned no default branch")
	}
	return response.DefaultBranch, nil
}

func (b *GitHubBackend) EnsureLabels(ctx context.Context, repository RepositoryID, wanted []Label) error {
	existing := make(map[string]Label)
	for page := 1; ; page++ {
		var labels []Label
		path := fmt.Sprintf("%s/labels?per_page=100&page=%d", b.repositoryPath(repository), page)
		if err := b.request(ctx, http.MethodGet, path, nil, &labels); err != nil {
			return err
		}
		for _, label := range labels {
			existing[label.Name] = label
		}
		if len(labels) < 100 {
			break
		}
	}
	for _, label := range wanted {
		current, found := existing[label.Name]
		if found && current == label {
			continue
		}
		method := http.MethodPost
		path := b.repositoryPath(repository) + "/labels"
		if found {
			method = http.MethodPatch
			path += "/" + url.PathEscape(label.Name)
		}
		if err := b.request(ctx, method, path, label, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *GitHubBackend) repositoryPath(repository RepositoryID) string {
	return "/repos/" + url.PathEscape(repository.Owner) + "/" + url.PathEscape(repository.Name)
}

func (b *GitHubBackend) request(ctx context.Context, method, path string, body, destination any) error {
	_, err := b.requestStatus(ctx, method, path, body, destination)
	return err
}

func (b *GitHubBackend) requestOptional(ctx context.Context, method, path string, destination any) (bool, error) {
	status, err := b.requestStatus(ctx, method, path, nil, destination)
	if status == http.StatusNotFound {
		return false, nil
	}
	return err == nil, err
}

func (b *GitHubBackend) requestStatus(ctx context.Context, method, path string, body, destination any) (int, error) {
	if b.token == "" {
		if b.tokenSource == nil {
			return 0, errors.New("GitHub authentication unavailable")
		}
		token, err := b.tokenSource()
		if err != nil {
			return 0, err
		}
		b.token = token
	}
	var encoded io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		encoded = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, b.baseURL+path, encoded)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+b.token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return response.StatusCode, fmt.Errorf("GitHub %s %s: %s: %s", method, path, response.Status, strings.TrimSpace(string(message)))
	}
	if destination != nil {
		return response.StatusCode, json.NewDecoder(response.Body).Decode(destination)
	}
	return response.StatusCode, nil
}
