package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type WorkItemID string

type SubmissionID string

type WorkItem struct {
	ID               WorkItemID
	Title            string
	Body             string
	Branch           string
	ArtifactBaseline string
	AcceptedHead     string
	Ready            bool
	Parent           WorkItemID
	Blockers         []WorkItemID
	Merged           bool
	Closed           bool
}

type CoordinationItem struct {
	Children []WorkItemID
	ID       WorkItemID
	Title    string
	Body     string
	Closed   bool
}

type Backend interface {
	FindWorkItems(context.Context, []WorkItem, []Dependency) ([]WorkItem, error)
	ListMergedWorkItems(context.Context) ([]WorkItem, error)
	CreateWorkItem(context.Context, WorkItem) (WorkItem, error)
	FindCoordinationItems(context.Context, string) ([]CoordinationItem, error)
	CreateCoordinationItem(context.Context, CoordinationItem) (CoordinationItem, error)
	AddChild(context.Context, WorkItemID, WorkItemID) error
	AddDependency(context.Context, WorkItemID, WorkItemID) error
	SetReady(context.Context, WorkItemID) error
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

// CleanupOutcome reports removed, preserved, and failed source work
// separately. A preserved branch is not a failure; a failed removal is never
// reported as removed.
type CleanupOutcome struct {
	Removed   []string        `json:"removed,omitempty"`
	Preserved []SourceOutcome `json:"preserved,omitempty"`
	Failed    []SourceOutcome `json:"failed,omitempty"`
}

type SourceOutcome struct {
	Branch string `json:"branch"`
	Reason string `json:"reason"`
}

// SourceCandidate is one branch with recorded deletion authority. Hold withholds
// deletion regardless of Git state; an empty AcceptedHead is unknown evidence.
type SourceCandidate struct {
	Branch       string
	AcceptedHead string
	Hold         string
}

// Cleanup is the legacy forge-authoritative source cleanup.
func Cleanup(ctx context.Context, root string, backend Backend) (CleanupOutcome, error) {
	items, err := backend.ListMergedWorkItems(ctx)
	if err != nil {
		return CleanupOutcome{}, err
	}
	var candidates []SourceCandidate
	for _, item := range items {
		// Without an explicit branch attachment, preserve local Git state instead
		// of redirecting cleanup through a title.
		if item.Merged && item.Branch != "" {
			candidates = append(candidates, SourceCandidate{Branch: item.Branch, AcceptedHead: item.AcceptedHead})
		}
	}
	outcome, err := CleanupSource(root, candidates)
	if err == nil && len(outcome.Failed) > 0 {
		err = fmt.Errorf("remove %s: %s", outcome.Failed[0].Branch, outcome.Failed[0].Reason)
	}
	return outcome, err
}

// CleanupSource removes the local worktree and local branch of each candidate
// only when its worktree is registered at the owned .worktrees location, clean,
// and at exactly the accepted head. Merge confirmation is the caller's
// authority, so squash merges need no ancestry or upstream. Remote branches
// are never touched, and a candidate with no local branch or worktree left
// needs no report.
func CleanupSource(root string, candidates []SourceCandidate) (CleanupOutcome, error) {
	root, err := git(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return CleanupOutcome{}, errors.New("not a Git repository")
	}
	primary, err := primaryWorktree(root)
	if err != nil {
		return CleanupOutcome{}, err
	}
	registered, err := registeredWorktrees(root)
	if err != nil {
		return CleanupOutcome{}, err
	}
	var outcome CleanupOutcome
	for _, candidate := range candidates {
		branch := candidate.Branch
		path, checkedOut := registered[branch]
		if !checkedOut && gitOK(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) != nil {
			continue
		}
		preserve := func(reason string) {
			outcome.Preserved = append(outcome.Preserved, SourceOutcome{Branch: branch, Reason: reason})
		}
		if candidate.Hold != "" {
			preserve(candidate.Hold)
			continue
		}
		if filepath.Clean(path) != filepath.Clean(filepath.Join(primary, ".worktrees", branch)) {
			preserve("not checked out at its owned worktree location")
			continue
		}
		dirty, err := git(path, "status", "--porcelain", "--untracked-files=all")
		if err != nil || dirty != "" {
			preserve("the worktree has uncommitted or unobservable changes")
			continue
		}
		head, err := git(path, "rev-parse", "HEAD")
		if err != nil || candidate.AcceptedHead == "" || head != candidate.AcceptedHead {
			preserve("HEAD is not the confirmed accepted source head")
			continue
		}
		if err := gitOK(root, "worktree", "remove", path); err != nil {
			outcome.Failed = append(outcome.Failed, SourceOutcome{Branch: branch, Reason: "worktree removal failed: " + err.Error()})
			continue
		}
		if err := gitOK(root, "branch", "-D", branch); err != nil {
			outcome.Failed = append(outcome.Failed, SourceOutcome{Branch: branch, Reason: "the worktree was removed but local branch deletion failed: " + err.Error()})
			continue
		}
		outcome.Removed = append(outcome.Removed, branch)
	}
	return outcome, nil
}

func registeredWorktrees(root string) (map[string]string, error) {
	output, err := git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	registered := make(map[string]string)
	var worktree string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			worktree = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch refs/heads/") {
			registered[strings.TrimPrefix(line, "branch refs/heads/")] = worktree
		}
	}
	return registered, nil
}

func Publish(ctx context.Context, request PublishRequest, backend Backend) (Outcome, error) {
	prepared, outcome, err := preflight(request)
	if err != nil || outcome.Status != "" {
		return outcome, err
	}
	ordered, outcome := orderSlices(prepared, request.Dependencies)
	if outcome.Status != "" {
		return outcome, nil
	}
	var parent CoordinationItem
	var parentBody []byte
	if len(prepared) > 1 {
		if request.ParentTitle == "" || request.ParentBody == "" {
			return Outcome{}, errors.New("multi-slice proposals require --parent-title and --parent-body")
		}
		parentBody, err = os.ReadFile(request.ParentBody)
		if err != nil {
			return Outcome{}, err
		}
		parents, err := backend.FindCoordinationItems(ctx, request.ParentTitle)
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
			if matches[0].Closed {
				return Outcome{Status: "needs_human", Reason: "existing Coordination Item is closed: " + request.ParentTitle}, nil
			}
			if matches[0].Body != string(parentBody) {
				return Outcome{Status: "needs_human", Reason: "existing Coordination Item has conflicting content: " + request.ParentTitle}, nil
			}
			parent = matches[0]
		}
	}
	existing, err := backend.FindWorkItems(ctx, ordered, request.Dependencies)
	if err != nil {
		return Outcome{}, err
	}
	matchesByTitle := make(map[string][]WorkItem, len(ordered))
	for _, slice := range ordered {
		for _, item := range existing {
			if item.Title == slice.Title {
				matchesByTitle[slice.Title] = append(matchesByTitle[slice.Title], item)
			}
		}
	}
	for _, slice := range ordered {
		matches := matchesByTitle[slice.Title]
		if len(matches) == 1 && matches[0].Closed {
			return Outcome{Status: "needs_human", Reason: "existing Work Item is closed: " + slice.Title}, nil
		}
		if len(matches) > 1 || len(matches) == 1 && (matches[0].Body != slice.Body || matches[0].Branch != "" && matches[0].Branch != slice.Branch) {
			return Outcome{Status: "needs_human", Reason: "ambiguous existing Work Item " + slice.Title}, nil
		}
		if len(matches) == 1 {
			wantedParent := parent.ID
			if matches[0].Parent != "" && matches[0].Parent != wantedParent {
				return Outcome{Status: "needs_human", Reason: "existing Work Item has a conflicting parent: " + slice.Title}, nil
			}
		}
	}
	for _, slice := range ordered {
		matches := matchesByTitle[slice.Title]
		if len(matches) != 1 {
			continue
		}
		var wanted []WorkItemID
		for _, dependency := range request.Dependencies {
			blockers := matchesByTitle[dependency.Blocker]
			if dependency.Dependent == slice.Title && len(blockers) == 1 {
				wanted = append(wanted, blockers[0].ID)
			}
		}
		if !containsOnly(matches[0].Blockers, wanted) {
			return Outcome{Status: "needs_human", Reason: "existing Work Item has conflicting Dependencies: " + slice.Title}, nil
		}
	}
	if len(prepared) > 1 && parent.ID == "" {
		parent, err = backend.CreateCoordinationItem(ctx, CoordinationItem{Title: request.ParentTitle, Body: string(parentBody)})
		if err != nil {
			return Outcome{}, err
		}
	}
	published := make(map[string]WorkItem, len(ordered))
	for _, slice := range ordered {
		matches := matchesByTitle[slice.Title]
		item := slice
		if len(matches) == 0 {
			item, err = backend.CreateWorkItem(ctx, slice)
			if err != nil {
				return Outcome{}, err
			}
		} else {
			item = matches[0]
		}
		if parent.ID != "" {
			if item.Parent != "" && item.Parent != parent.ID {
				return Outcome{Status: "needs_human", Reason: "existing Work Item has a conflicting parent: " + slice.Title}, nil
			}
			if item.Parent == "" {
				if err := backend.AddChild(ctx, parent.ID, item.ID); err != nil {
					return Outcome{}, err
				}
			}
		}
		for _, dependency := range request.Dependencies {
			if dependency.Dependent != slice.Title {
				continue
			}
			blocker := published[dependency.Blocker].ID
			alreadyLinked := false
			for _, existingBlocker := range item.Blockers {
				if existingBlocker == blocker {
					alreadyLinked = true
				}
			}
			if !alreadyLinked {
				if err := backend.AddDependency(ctx, item.ID, blocker); err != nil {
					return Outcome{}, err
				}
			}
		}
		if !item.Ready {
			if err := backend.SetReady(ctx, item.ID); err != nil {
				return Outcome{}, err
			}
		}
		published[slice.Title] = item
	}
	return Outcome{Status: "completed"}, nil
}

func containsOnly(existing, wanted []WorkItemID) bool {
	for _, id := range existing {
		if !slices.Contains(wanted, id) {
			return false
		}
	}
	return true
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

func preflight(request PublishRequest) ([]WorkItem, Outcome, error) {
	if len(request.Slices) == 0 {
		return nil, Outcome{}, errors.New("at least one --slice is required")
	}
	root, err := git(request.Root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, Outcome{}, errors.New("not a Git repository")
	}
	remote := request.Remote
	if remote == "" {
		return nil, Outcome{}, errors.New("publication requires an explicit Git remote")
	}
	target, err := git(root, "rev-parse", "refs/remotes/"+remote+"/"+request.Target)
	if err != nil {
		return nil, fix("target branch is unavailable", "fetch the target branch"), nil
	}
	mainWorktree, err := primaryWorktree(root)
	if err != nil {
		return nil, Outcome{}, err
	}
	dirty, err := git(mainWorktree, "status", "--porcelain", "--untracked-files=all", "--", "CONTEXT.md", "docs/adr")
	if err != nil {
		return nil, Outcome{}, err
	}
	if dirty != "" {
		return nil, fix("durable documents have uncommitted changes", "commit or restore the reported durable-document paths"), nil
	}
	seen := make(map[string]bool)
	prepared := make([]WorkItem, 0, len(request.Slices))
	for _, slice := range request.Slices {
		if slice.Slug == "" || slice.BodyPath == "" || seen[slice.Slug] {
			return nil, Outcome{}, fmt.Errorf("invalid --slice %q", slice.Slug)
		}
		seen[slice.Slug] = true
		head, err := git(root, "rev-parse", "refs/heads/"+slice.Slug)
		if err != nil {
			return nil, fix("slice branch "+slice.Slug+" is unavailable", "create the local slice branch"), nil
		}
		remoteHead, err := git(root, "rev-parse", "refs/remotes/"+remote+"/"+slice.Slug)
		if err != nil || remoteHead != head {
			return nil, fix("slice branch "+slice.Slug+" is not pushed at its local head", "push the slice branch"), nil
		}
		if err := gitOK(root, "merge-base", "--is-ancestor", target, head); err != nil {
			return nil, fix("slice branch "+slice.Slug+" misses the observed target", "merge the target branch into the slice"), nil
		}
		baseline, err := artifactBaseline(root, slice.Slug, head)
		if err != nil {
			return nil, fix(err.Error(), "commit the complete ledger once at the published branch head"), nil
		}
		body, err := os.ReadFile(slice.BodyPath)
		if err != nil {
			return nil, Outcome{}, err
		}
		prepared = append(prepared, WorkItem{Title: slice.Slug, Body: string(body), Branch: slice.Slug, ArtifactBaseline: baseline})
	}
	return prepared, Outcome{}, nil
}

func primaryWorktree(root string) (string, error) {
	output, err := git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(output, "\n")
	path, found := strings.CutPrefix(first, "worktree ")
	if !found {
		return "", errors.New("Git returned no main worktree")
	}
	return path, nil
}

func artifactBaseline(root, slug, head string) (string, error) {
	history, err := InspectLedger(root, head, slug)
	if err != nil {
		return "", err
	}
	if len(history.Violations) > 0 {
		return "", errors.New(strings.Join(history.Violations, "; "))
	}
	if history.Phase == "retired" {
		return "", errors.New("ledger is removed before publication")
	}
	if history.Baseline != head {
		return "", errors.New("Artifact Baseline must be the published branch head")
	}
	return history.Baseline, nil
}

func fix(invariant, repair string) Outcome {
	return Outcome{Status: "fix_required", Reason: invariant + "; " + repair}
}

func git(directory string, args ...string) (string, error) {
	output, err := exec.Command("git", append([]string{"-C", directory}, args...)...).Output()
	return strings.TrimSpace(string(output)), err
}

func gitOK(directory string, args ...string) error {
	return exec.Command("git", append([]string{"-C", directory}, args...)...).Run()
}
