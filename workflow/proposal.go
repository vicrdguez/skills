package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RepositoryID struct {
	Owner string
	Name  string
}

type WorkItem struct {
	Number           int
	Title            string
	Body             string
	Branch           string
	ArtifactBaseline string
	Ready            bool
	Parent           int
	Blockers         []int
}

type CoordinationItem struct {
	Number int
	Title  string
	Body   string
}

type Backend interface {
	ListWorkItems(context.Context, RepositoryID) ([]WorkItem, error)
	CreateWorkItem(context.Context, RepositoryID, WorkItem) (WorkItem, error)
	ListCoordinationItems(context.Context, RepositoryID) ([]CoordinationItem, error)
	CreateCoordinationItem(context.Context, RepositoryID, CoordinationItem) (CoordinationItem, error)
	AddChild(context.Context, RepositoryID, int, int) error
	AddDependency(context.Context, RepositoryID, int, int) error
	SetReady(context.Context, RepositoryID, int) error
}

type Slice struct {
	Slug     string
	BodyPath string
}

type Dependency struct {
	Dependent string
	Blocker   string
}

type PublishRequest struct {
	Root         string
	Remote       string
	Target       string
	Slices       []Slice
	Dependencies []Dependency
	ParentTitle  string
	ParentBody   string
}

type Outcome struct {
	Status string
	Reason string
}

func Publish(ctx context.Context, request PublishRequest, backend Backend) (Outcome, error) {
	prepared, repository, outcome, err := preflight(request)
	if err != nil || outcome.Status != "" {
		return outcome, err
	}
	ordered, outcome := orderSlices(prepared, request.Dependencies)
	if outcome.Status != "" {
		return outcome, nil
	}
	var parent CoordinationItem
	if len(prepared) > 1 {
		if request.ParentTitle == "" || request.ParentBody == "" {
			return Outcome{}, errors.New("multi-slice proposals require --parent-title and --parent-body")
		}
		body, err := os.ReadFile(request.ParentBody)
		if err != nil {
			return Outcome{}, err
		}
		parents, err := backend.ListCoordinationItems(ctx, repository)
		if err != nil {
			return Outcome{}, err
		}
		var matches []CoordinationItem
		for _, item := range parents {
			if item.Title == request.ParentTitle {
				matches = append(matches, item)
			}
		}
		if len(matches) > 1 {
			return Outcome{Status: "needs_human", Reason: "ambiguous existing Coordination Item " + request.ParentTitle}, nil
		}
		if len(matches) == 1 {
			parent = matches[0]
		} else {
			parent, err = backend.CreateCoordinationItem(ctx, repository, CoordinationItem{Title: request.ParentTitle, Body: string(body)})
			if err != nil {
				return Outcome{}, err
			}
		}
	}
	existing, err := backend.ListWorkItems(ctx, repository)
	if err != nil {
		return Outcome{}, err
	}
	published := make(map[string]WorkItem, len(ordered))
	for _, slice := range ordered {
		var matches []WorkItem
		for _, item := range existing {
			if item.Title == slice.Title {
				matches = append(matches, item)
			}
		}
		if len(matches) > 1 || len(matches) == 1 && matches[0].Branch != "" && matches[0].Branch != slice.Branch {
			return Outcome{Status: "needs_human", Reason: "ambiguous existing Work Item " + slice.Title}, nil
		}
		item := slice
		if len(matches) == 0 {
			item, err = backend.CreateWorkItem(ctx, repository, slice)
			if err != nil {
				return Outcome{}, err
			}
		} else {
			item = matches[0]
		}
		if parent.Number != 0 {
			if item.Parent != 0 && item.Parent != parent.Number {
				return Outcome{Status: "needs_human", Reason: "existing Work Item has a conflicting parent: " + slice.Title}, nil
			}
			if item.Parent == 0 {
				if err := backend.AddChild(ctx, repository, parent.Number, item.Number); err != nil {
					return Outcome{}, err
				}
			}
		}
		for _, dependency := range request.Dependencies {
			if dependency.Dependent != slice.Title {
				continue
			}
			blocker := published[dependency.Blocker].Number
			alreadyLinked := false
			for _, existingBlocker := range item.Blockers {
				if existingBlocker == blocker {
					alreadyLinked = true
				}
			}
			if !alreadyLinked {
				if err := backend.AddDependency(ctx, repository, item.Number, blocker); err != nil {
					return Outcome{}, err
				}
			}
		}
		if !item.Ready {
			if err := backend.SetReady(ctx, repository, item.Number); err != nil {
				return Outcome{}, err
			}
		}
		published[slice.Title] = item
	}
	return Outcome{Status: "completed"}, nil
}

func orderSlices(items []WorkItem, dependencies []Dependency) ([]WorkItem, Outcome) {
	byName := make(map[string]WorkItem, len(items))
	remaining := make(map[string]int, len(items))
	for _, item := range items {
		byName[item.Title] = item
		remaining[item.Title] = 0
	}
	for _, dependency := range dependencies {
		if dependency.Dependent == dependency.Blocker || byName[dependency.Dependent].Title == "" || byName[dependency.Blocker].Title == "" {
			return nil, fix("Dependency graph contains an unknown or self-referencing edge", "correct the --depends values")
		}
		remaining[dependency.Dependent]++
	}
	ordered := make([]WorkItem, 0, len(items))
	for len(ordered) < len(items) {
		added := false
		for _, item := range items {
			if remaining[item.Title] != 0 {
				continue
			}
			ordered = append(ordered, item)
			remaining[item.Title] = -1
			for _, dependency := range dependencies {
				if dependency.Blocker == item.Title {
					remaining[dependency.Dependent]--
				}
			}
			added = true
		}
		if !added {
			return nil, fix("Dependency graph contains a cycle", "remove the cyclic --depends edge")
		}
	}
	return ordered, Outcome{}
}

func preflight(request PublishRequest) ([]WorkItem, RepositoryID, Outcome, error) {
	if len(request.Slices) == 0 {
		return nil, RepositoryID{}, Outcome{}, errors.New("at least one --slice is required")
	}
	root, err := git(request.Root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, RepositoryID{}, Outcome{}, errors.New("not a Git repository")
	}
	remote := request.Remote
	if remote == "" {
		remote = "origin"
	}
	remoteURL, err := git(root, "remote", "get-url", remote)
	if err != nil {
		return nil, RepositoryID{}, Outcome{}, fmt.Errorf("resolve remote %q: %w", remote, err)
	}
	repository, err := parseGitHubRemote(remoteURL)
	if err != nil {
		return nil, RepositoryID{}, Outcome{}, err
	}
	target, err := git(root, "rev-parse", "refs/remotes/"+remote+"/"+request.Target)
	if err != nil {
		return nil, RepositoryID{}, fix("target branch is unavailable", "fetch the target branch"), nil
	}
	dirty, err := git(root, "status", "--porcelain", "--untracked-files=all", "--", "CONTEXT.md", "docs/adr", "docs/capabilities")
	if err != nil {
		return nil, RepositoryID{}, Outcome{}, err
	}
	if dirty != "" {
		return nil, RepositoryID{}, fix("durable documents have uncommitted changes", "commit or restore the reported durable-document paths"), nil
	}
	seen := make(map[string]bool)
	prepared := make([]WorkItem, 0, len(request.Slices))
	for _, slice := range request.Slices {
		if slice.Slug == "" || slice.BodyPath == "" || seen[slice.Slug] {
			return nil, RepositoryID{}, Outcome{}, fmt.Errorf("invalid --slice %q", slice.Slug)
		}
		seen[slice.Slug] = true
		head, err := git(root, "rev-parse", "refs/heads/"+slice.Slug)
		if err != nil {
			return nil, RepositoryID{}, fix("slice branch "+slice.Slug+" is unavailable", "create the local slice branch"), nil
		}
		remoteHead, err := git(root, "rev-parse", "refs/remotes/"+remote+"/"+slice.Slug)
		if err != nil || remoteHead != head {
			return nil, RepositoryID{}, fix("slice branch "+slice.Slug+" is not pushed at its local head", "push the slice branch"), nil
		}
		if err := gitOK(root, "merge-base", "--is-ancestor", target, head); err != nil {
			return nil, RepositoryID{}, fix("slice branch "+slice.Slug+" misses the observed target", "merge the target branch into the slice"), nil
		}
		baseline, err := artifactBaseline(root, slice.Slug, head)
		if err != nil {
			return nil, RepositoryID{}, fix(err.Error(), "commit the complete ledger once at the published branch head"), nil
		}
		body, err := os.ReadFile(slice.BodyPath)
		if err != nil {
			return nil, RepositoryID{}, Outcome{}, err
		}
		prepared = append(prepared, WorkItem{Title: slice.Slug, Body: string(body), Branch: slice.Slug, ArtifactBaseline: baseline})
	}
	return prepared, repository, Outcome{}, nil
}

func artifactBaseline(root, slug, head string) (string, error) {
	commits, err := git(root, "rev-list", "--first-parent", "--reverse", head)
	if err != nil {
		return "", err
	}
	path := ".changes/" + slug
	present := false
	var baseline string
	for _, commit := range strings.Fields(commits) {
		now := gitOK(root, "cat-file", "-e", commit+":"+path) == nil
		if !present && now {
			if baseline != "" {
				return "", errors.New("ledger is introduced more than once")
			}
			baseline = commit
		}
		if present && !now {
			return "", errors.New("ledger is removed before publication")
		}
		present = now
	}
	if baseline == "" {
		return "", errors.New("ledger is missing")
	}
	if baseline != head {
		return "", errors.New("Artifact Baseline is not the published branch head")
	}
	for _, required := range []string{"intent.md", "behavior.md"} {
		if gitOK(root, "cat-file", "-e", baseline+":"+filepath.Join(path, required)) != nil {
			return "", fmt.Errorf("ledger misses %s", required)
		}
	}
	return baseline, nil
}

func fix(invariant, repair string) Outcome {
	return Outcome{Status: "fix_required", Reason: invariant + "; " + repair}
}

func parseGitHubRemote(remote string) (RepositoryID, error) {
	remote = strings.TrimSuffix(remote, ".git")
	for _, prefix := range []string{"git@github.com:", "https://github.com/", "ssh://git@github.com/"} {
		if strings.HasPrefix(remote, prefix) {
			parts := strings.Split(strings.TrimPrefix(remote, prefix), "/")
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				return RepositoryID{Owner: parts[0], Name: parts[1]}, nil
			}
		}
	}
	return RepositoryID{}, fmt.Errorf("remote %q is not a GitHub repository", remote)
}

func git(directory string, args ...string) (string, error) {
	output, err := exec.Command("git", append([]string{"-C", directory}, args...)...).Output()
	return strings.TrimSpace(string(output)), err
}

func gitOK(directory string, args ...string) error {
	return exec.Command("git", append([]string{"-C", directory}, args...)...).Run()
}
