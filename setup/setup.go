package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const AgentsBlock = `<!-- dev-pipeline:start -->
## Workflow

Use ` + "`skl`" + ` as the Workflow entrypoint. Do not manually mutate Workflow Projections. Only a human merges.
<!-- dev-pipeline:end -->
`

type RepositoryID struct {
	Owner string
	Name  string
}

type Label struct {
	Name        string
	Color       string
	Description string
}

var WorkflowLabels = []Label{
	{Name: "ready", Color: "0e8a16", Description: "Proposed change awaiting an implementor"},
	{Name: "wip", Color: "fbca04", Description: "Additive Worker Claim"},
	{Name: "review", Color: "1d76db", Description: "Built change awaiting a reviewer"},
	{Name: "rework", Color: "d93f0b", Description: "Reviewer bounced it back to the implementor"},
	{Name: "needs-human", Color: "b60205", Description: "Automation paused for a narrow human decision"},
	{Name: "done", Color: "5319e7", Description: "Passed review, awaiting human approval to merge"},
}

type Backend interface {
	Validate(context.Context, RepositoryID) (targetBranch string, err error)
	EnsureLabels(context.Context, RepositoryID, []Label) error
}

func NewGitHubBackendFromEnv() (Backend, error) {
	return nil, errors.New("GitHub backend is not configured")
}

type Request struct {
	Location string
	Remote   string
	Confirm  func(string) (bool, error)
}

type Outcome struct {
	Root         string
	Repository   RepositoryID
	TargetBranch string
}

func Run(ctx context.Context, request Request, backend Backend) (Outcome, error) {
	root, err := git(request.Location, "rev-parse", "--show-toplevel")
	if err != nil {
		return Outcome{}, errors.New("not a Git repository")
	}
	remote, err := resolveRemote(root, request.Remote)
	if err != nil {
		return Outcome{}, err
	}
	remoteURL, err := git(root, "remote", "get-url", remote)
	if err != nil {
		return Outcome{}, fmt.Errorf("resolve GitHub remote %q: %w", remote, err)
	}
	repository, err := parseGitHubRemote(remoteURL)
	if err != nil {
		return Outcome{}, err
	}
	agents, err := planAgents(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return Outcome{}, err
	}
	targetBranch, err := backend.Validate(ctx, repository)
	if err != nil {
		return Outcome{}, fmt.Errorf("validate GitHub repository: %w", err)
	}
	linkClaude := false
	if request.Confirm != nil {
		linkClaude, err = request.Confirm("Link CLAUDE.md to AGENTS.md? [y/N] ")
		if err != nil {
			return Outcome{}, err
		}
	}

	if err := backend.EnsureLabels(ctx, repository, WorkflowLabels); err != nil {
		return Outcome{}, fmt.Errorf("prepare workflow labels: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), agents, 0o644); err != nil {
		return Outcome{}, err
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".worktrees/\n"), 0o644); err != nil {
		return Outcome{}, err
	}
	if linkClaude {
		if err := os.Symlink("AGENTS.md", filepath.Join(root, "CLAUDE.md")); err != nil {
			return Outcome{}, err
		}
	}
	return Outcome{Root: root, Repository: repository, TargetBranch: targetBranch}, nil
}

func planAgents(path string) ([]byte, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte(AgentsBlock), nil
	}
	if err != nil {
		return nil, err
	}
	const start = "<!-- dev-pipeline:start -->"
	const end = "<!-- dev-pipeline:end -->"
	startAt := strings.Index(string(contents), start)
	endAt := strings.Index(string(contents), end)
	if startAt >= 0 && endAt >= 0 {
		endAt += len(end)
		return []byte(string(contents[:startAt]) + strings.TrimSuffix(AgentsBlock, "\n") + string(contents[endAt:])), nil
	}
	prefix := string(contents)
	if prefix != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	return []byte(prefix + AgentsBlock), nil
}

func resolveRemote(root, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if origin, err := git(root, "remote", "get-url", "origin"); err == nil {
		if _, err := parseGitHubRemote(origin); err == nil {
			return "origin", nil
		}
	}
	names, err := git(root, "remote")
	if err != nil {
		return "", err
	}
	var githubRemotes []string
	for _, name := range strings.Fields(names) {
		remoteURL, err := git(root, "remote", "get-url", name)
		if err == nil {
			if _, err := parseGitHubRemote(remoteURL); err == nil {
				githubRemotes = append(githubRemotes, name)
			}
		}
	}
	switch len(githubRemotes) {
	case 1:
		return githubRemotes[0], nil
	case 0:
		return "", errors.New("no GitHub remote found")
	default:
		return "", fmt.Errorf("multiple GitHub remotes (%s); choose one with --remote", strings.Join(githubRemotes, ", "))
	}
}

func git(directory string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}

func parseGitHubRemote(remote string) (RepositoryID, error) {
	remote = strings.TrimSuffix(remote, ".git")
	var path string
	switch {
	case strings.HasPrefix(remote, "git@github.com:"):
		path = strings.TrimPrefix(remote, "git@github.com:")
	case strings.HasPrefix(remote, "https://github.com/"):
		path = strings.TrimPrefix(remote, "https://github.com/")
	case strings.HasPrefix(remote, "ssh://git@github.com/"):
		path = strings.TrimPrefix(remote, "ssh://git@github.com/")
	default:
		return RepositoryID{}, fmt.Errorf("remote %q is not a GitHub repository", remote)
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RepositoryID{}, fmt.Errorf("invalid GitHub remote %q", remote)
	}
	return RepositoryID{Owner: parts[0], Name: parts[1]}, nil
}
