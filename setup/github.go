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
	"strconv"
	"strings"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/workflow"
)

// Backend is the CLI factory result; each workflow operation checks its own
// supported capability rather than requiring retired Setup methods.
type Backend = any

// GitHubBackend is also the transport of ledger issue publication.
var _ ledger.Forge = (*GitHubBackend)(nil)

type GitHubBackend struct {
	repository  github.RepositoryID
	baseURL     string
	token       string
	tokenSource func() (string, error)
	client      *http.Client
	retryDelay  time.Duration
	timeout     time.Duration
	issueIDs    map[int]int64
}

func (b *GitHubBackend) BindRepository(repository github.RepositoryID) {
	if b.repository != repository {
		clear(b.issueIDs)
	}
	b.repository = repository
}

func (b *GitHubBackend) requireRepository() error {
	if b.repository.Owner == "" || b.repository.Name == "" {
		return errors.New("GitHub backend is not bound to a repository")
	}
	return nil
}

func githubIssueNumber(id workflow.WorkItemID) (int, error) {
	number, err := strconv.Atoi(string(id))
	if err != nil || number <= 0 || strconv.Itoa(number) != string(id) {
		return 0, fmt.Errorf("invalid GitHub issue identity %q", id)
	}
	return number, nil
}

func NewGitHubBackend(baseURL, token string, client *http.Client) *GitHubBackend {
	return &GitHubBackend{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: client, retryDelay: requestRetryDelay, timeout: requestTimeout, issueIDs: make(map[int]int64)}
}

func NewGitHubBackendFromEnv(repository github.RepositoryID) (Backend, error) {
	backend := newGitHubBackend("https://api.github.com", http.DefaultClient, func() (string, error) {
		return resolveGitHubToken(os.Getenv, func() (string, error) {
			output, err := exec.Command("gh", "auth", "token").Output()
			return strings.TrimSpace(string(output)), err
		})
	})
	backend.BindRepository(repository)
	return backend, nil
}

func newGitHubBackend(baseURL string, client *http.Client, tokenSource func() (string, error)) *GitHubBackend {
	return &GitHubBackend{baseURL: strings.TrimRight(baseURL, "/"), tokenSource: tokenSource, client: client, retryDelay: requestRetryDelay, timeout: requestTimeout, issueIDs: make(map[int]int64)}
}

type githubIssue struct {
	CreatedAt        string `json:"created_at"`
	SubIssuesSummary struct {
		Total int `json:"total"`
	} `json:"sub_issues_summary"`
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

func hasWorkflowLabel(issue githubIssue) bool {
	for _, label := range issue.Labels {
		if slices.Contains([]string{"ready", "wip", "review", "rework", "needs-human", "done"}, label.Name) {
			return true
		}
	}
	return false
}

func (b *GitHubBackend) listIssues(ctx context.Context, repository github.RepositoryID) ([]githubIssue, error) {
	var all []githubIssue
	for page := 1; ; page++ {
		var issues []githubIssue
		path := fmt.Sprintf("%s/issues?state=all&per_page=100&page=%d", b.repositoryPath(repository), page)
		if err := b.request(ctx, http.MethodGet, path, nil, &issues); err != nil {
			return nil, err
		}
		for _, issue := range issues {
			b.issueIDs[issue.Number] = issue.ID
		}
		all = append(all, issues...)
		if len(issues) < 100 {
			return all, nil
		}
	}
}

func (b *GitHubBackend) createIssue(ctx context.Context, repository github.RepositoryID, title, body string) (githubIssue, error) {
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

func (b *GitHubBackend) repositoryPath(repository github.RepositoryID) string {
	return "/repos/" + url.PathEscape(repository.Owner) + "/" + url.PathEscape(repository.Name)
}

// TransportFailure marks a forge request whose outcome is unknown: the
// request was sent but no reliable response arrived, so the server may or
// may not have processed it.
type TransportFailure struct{ Cause error }

func (t *TransportFailure) Error() string { return "GitHub transport failure: " + t.Cause.Error() }

func (t *TransportFailure) Unwrap() error { return t.Cause }

// UnknownOutcome marks the failure as an unknown-outcome transport failure
// so callers can distinguish it from a definite forge rejection.
func (t *TransportFailure) UnknownOutcome() bool { return true }

// CreateIssue publishes one descriptive human-facing issue and returns its
// number. It adds no workflow label and writes no state: the body is opaque
// temporary transport authored outside skl. A create is never retried: an
// unknown outcome is returned as a TransportFailure for the caller to report.
func (b *GitHubBackend) CreateIssue(ctx context.Context, title, body string) (int, error) {
	if err := b.requireRepository(); err != nil {
		return 0, err
	}
	issue, err := b.createIssue(ctx, b.repository, title, body)
	if err != nil {
		return 0, err
	}
	if issue.Number <= 0 || issue.ID <= 0 {
		return 0, &TransportFailure{Cause: errors.New("successful issue creation returned no reliable issue identity")}
	}
	b.issueIDs[issue.Number] = issue.ID
	return issue.Number, nil
}

// UpdateIssue presents current prose on one established descriptive issue.
// The update is safely repeatable, so it is retried a bounded number of
// times. proceed is consulted before every attempt, so an update whose local
// selection was superseded between attempts is not sent again; its error is
// returned as is.
func (b *GitHubBackend) UpdateIssue(ctx context.Context, number int, title, body string, proceed func() error) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	if number <= 0 {
		return fmt.Errorf("invalid GitHub issue number %d", number)
	}
	if err := b.requestRetrying(ctx, http.MethodPatch, b.repositoryPath(b.repository)+fmt.Sprintf("/issues/%d", number), map[string]string{"title": title, "body": body}, nil, proceed); err != nil {
		return err
	}
	return nil
}

// ListChildren lists the issue numbers grouped under one parent issue.
func (b *GitHubBackend) ListChildren(ctx context.Context, parent int) ([]int, error) {
	if err := b.requireRepository(); err != nil {
		return nil, err
	}
	var children []struct {
		Number int `json:"number"`
	}
	path := b.repositoryPath(b.repository) + fmt.Sprintf("/issues/%d/sub_issues?per_page=100&page=1", parent)
	if err := b.requestRetrying(ctx, http.MethodGet, path, nil, &children, nil); err != nil {
		return nil, err
	}
	numbers := make([]int, 0, len(children))
	for _, child := range children {
		numbers = append(numbers, child.Number)
	}
	return numbers, nil
}

// AttachChild groups one child issue under its parent issue.
func (b *GitHubBackend) AttachChild(ctx context.Context, parent, child int) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	id, err := b.issueID(ctx, child)
	if err != nil {
		return err
	}
	return b.request(ctx, http.MethodPost, b.repositoryPath(b.repository)+fmt.Sprintf("/issues/%d/sub_issues", parent), map[string]int64{"sub_issue_id": id}, nil)
}

func (b *GitHubBackend) issueID(ctx context.Context, number int) (int64, error) {
	if id, ok := b.issueIDs[number]; ok {
		return id, nil
	}
	var issue githubIssue
	path := b.repositoryPath(b.repository) + fmt.Sprintf("/issues/%d", number)
	if err := b.requestRetrying(ctx, http.MethodGet, path, nil, &issue, nil); err != nil {
		return 0, fmt.Errorf("resolve GitHub issue id for #%d: %w", number, err)
	}
	if issue.Number != number || issue.ID == 0 {
		return 0, fmt.Errorf("GitHub issue #%d returned no stable database id", number)
	}
	b.issueIDs[number] = issue.ID
	return issue.ID, nil
}

func (b *GitHubBackend) request(ctx context.Context, method, path string, body, destination any) error {
	_, err := b.requestStatus(ctx, method, path, body, destination)
	return err
}

// requestTimeout bounds every GitHub request, so a command waits for a
// response or a timeout, never indefinitely.
const requestTimeout = 30 * time.Second

// Ledger issue publication retries reads and safely repeatable updates a
// bounded number of times. Creates are sent once, so an unknown outcome is
// never resolved by sending the request again.
const (
	requestAttempts   = 3
	requestRetryDelay = 100 * time.Millisecond
)

// requestRetrying sends one read or safely repeatable update, retrying
// transport failures, rate limits, and server errors a bounded number of
// times. A non-nil proceed is consulted before every attempt; its error
// stops the request unsent.
func (b *GitHubBackend) requestRetrying(ctx context.Context, method, path string, body, destination any, proceed func() error) error {
	if method != http.MethodGet && method != http.MethodPatch {
		return fmt.Errorf("GitHub %s %s is not safely repeatable", method, path)
	}
	for attempt := 1; ; attempt++ {
		if proceed != nil {
			if err := proceed(); err != nil {
				return err
			}
		}
		status, err := b.requestStatus(ctx, method, path, body, destination)
		var transport *TransportFailure
		transient := errors.As(err, &transport) || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
		if err == nil || !transient || attempt == requestAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(b.retryDelay * time.Duration(attempt)):
		}
	}
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
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
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
		// The outcome is unknown: the server may have processed the request.
		return 0, &TransportFailure{Cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return response.StatusCode, fmt.Errorf("GitHub %s %s: %s: %s", method, path, response.Status, strings.TrimSpace(string(message)))
	}
	if destination != nil {
		if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
			// A successful response with an unreadable body does not establish
			// whether a mutating request completed. Preserve that uncertainty so
			// issue publication resolves by observation before any retry.
			if method != http.MethodGet {
				return response.StatusCode, &TransportFailure{Cause: err}
			}
			return response.StatusCode, err
		}
	}
	return response.StatusCode, nil
}
