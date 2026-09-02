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
	"strings"
)

type GitHubBackend struct {
	baseURL     string
	token       string
	tokenSource func() (string, error)
	client      *http.Client
}

func NewGitHubBackend(baseURL, token string, client *http.Client) *GitHubBackend {
	return &GitHubBackend{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: client}
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
	return &GitHubBackend{baseURL: strings.TrimRight(baseURL, "/"), tokenSource: tokenSource, client: client}
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
	if b.token == "" {
		if b.tokenSource == nil {
			return errors.New("GitHub authentication unavailable")
		}
		token, err := b.tokenSource()
		if err != nil {
			return err
		}
		b.token = token
	}
	var encoded io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		encoded = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, b.baseURL+path, encoded)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+b.token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("GitHub %s %s: %s: %s", method, path, response.Status, strings.TrimSpace(string(message)))
	}
	if destination != nil {
		return json.NewDecoder(response.Body).Decode(destination)
	}
	return nil
}
