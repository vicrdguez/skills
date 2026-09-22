package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// DeliverySource is the source-repository state one delivery phase relies on.
// It carries Git identities and the evidence-bearing scope only; Workflow State,
// Claims, review counts, and report persistence stay outside this module.
type DeliverySource struct {
	Worktree    string `json:"worktree"`
	Head        string `json:"head"`
	Target      string `json:"target"`
	Previous    string `json:"previous,omitempty"`
	Scope       string `json:"scope,omitempty"`
	FetchStatus string `json:"fetch_status,omitempty"`
}

// PrepareDeliverySource creates or reuses the planned source branch and worktree
// after a Claim exists. It attempts an ordinary fetch of the selected remote's
// main and the planned branch, then honestly reports whether it used fresh or
// local inputs. Existing branch progress is preserved: Prepare never resets,
// rebases, stashes, or overwrites a local branch with a remote or required head.
// Initial preparation may pass an empty requiredHead. A missing required
// revision or Integration Target is a concrete refusal, never a fabricated
// branch.
func PrepareDeliverySource(root, remote, branch, requiredHead, recordedTarget string) (DeliverySource, error) {
	if !validConventionalBranch(root, branch) {
		return DeliverySource{}, Refuse("invalid conventional branch identity; repair the Work Item attachment")
	}
	primary, err := primaryWorktree(root)
	if err != nil {
		return DeliverySource{}, fmt.Errorf("resolve the primary source worktree: %w", err)
	}
	// Each fetch records its own selected identity; a concurrent preparation in
	// the same checkout must not change which commit this caller observes.
	fetchedMain, mainErr := deliveryFetch(root, remote, "main")
	fetchedBranch, branchErr := deliveryFetch(root, remote, branch)
	status := deliveryFetchStatus(remote, branch, mainErr, branchErr)
	required := ""
	if requiredHead != "" {
		required = deliveryResolveCommit(root, requiredHead)
		if required == "" {
			return DeliverySource{}, Refuse("required source revision " + requiredHead + " is unavailable after fetching; restore that exact input before preparing the branch")
		}
	}
	target := fetchedMain
	if recordedTarget != "" {
		target = deliveryResolveCommit(root, recordedTarget)
		if target == "" {
			return DeliverySource{}, Refuse("recorded Integration Target " + recordedTarget + " is unavailable; restore that exact input instead of substituting a newer target")
		}
	} else if target == "" && remote != "" {
		target = deliveryResolveCommit(root, "refs/remotes/"+remote+"/main")
	}
	if target == "" {
		return DeliverySource{}, Refuse("Integration Target is unavailable; fetch " + deliveryRefName(remote, "main") + " or supply its last observed revision and retry")
	}
	head := deliveryResolveCommit(root, "refs/heads/"+branch)
	if head == "" {
		base := required
		if base == "" {
			base = fetchedBranch
		}
		if base == "" {
			base = target
		}
		if err := gitOK(root, "branch", branch, base); err != nil {
			return DeliverySource{}, fmt.Errorf("create source branch %s at %s: %w", branch, base, err)
		}
		head = deliveryResolveCommit(root, "refs/heads/"+branch)
		if head == "" {
			return DeliverySource{}, fmt.Errorf("source branch %s was not created at %s", branch, base)
		}
	}
	worktree := filepath.Join(primary, ".worktrees", branch)
	registered, err := registeredWorktrees(root)
	if err != nil {
		return DeliverySource{}, fmt.Errorf("list registered source worktrees: %w", err)
	}
	if existing, ok := registered[branch]; ok {
		if filepath.Clean(existing) != filepath.Clean(worktree) {
			return DeliverySource{}, Refuse("branch " + branch + " is already checked out at " + existing + " instead of the expected worktree " + worktree + "; preserve that worktree and resume it rather than recreating it")
		}
		if _, err := os.Stat(worktree); err != nil {
			return DeliverySource{}, Refuse("registered worktree " + worktree + " is unavailable; repair it without deleting branch progress")
		}
	} else {
		// Reuse whatever is there instead of deleting it; a stray path is a repair case.
		if _, err := os.Lstat(worktree); err == nil {
			return DeliverySource{}, Refuse("path " + worktree + " already exists but is not a registered worktree for " + branch + "; inspect and repair it without deleting files")
		}
		if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
			return DeliverySource{}, fmt.Errorf("prepare worktree directory %s: %w", filepath.Dir(worktree), err)
		}
		if err := gitOK(root, "worktree", "add", worktree, branch); err != nil {
			return DeliverySource{}, Refuse("cannot create worktree " + worktree + " for branch " + branch + " while preserving existing checkouts: " + err.Error())
		}
	}
	return DeliverySource{Worktree: worktree, Head: head, Target: target, FetchStatus: status}, nil
}

// InspectDeliverySource resolves the prepared local branch without touching the
// network. It requires the expected worktree on the planned branch with clean
// tracked, index, and untracked state, returns the full head and an available
// target, and derives review scope from the supplied previous reviewed
// revision. A nonempty requiredHead must match the actual head.
func InspectDeliverySource(root, branch, requiredHead, target, previous string) (DeliverySource, error) {
	worktree, err := deliveryWorktree(root, branch)
	if err != nil {
		return DeliverySource{}, err
	}
	current, err := git(worktree, "symbolic-ref", "--short", "HEAD")
	if err != nil || current != branch {
		return DeliverySource{}, Refuse("worktree " + worktree + " is not on the prepared branch " + branch + "; check out the planned branch without discarding work")
	}
	if err := deliveryClean(worktree); err != nil {
		return DeliverySource{}, err
	}
	head, err := git(worktree, "rev-parse", "HEAD")
	if err != nil {
		return DeliverySource{}, fmt.Errorf("resolve prepared branch head in %s: %w", worktree, err)
	}
	if requiredHead != "" {
		required := deliveryResolveCommit(root, requiredHead)
		if required == "" {
			return DeliverySource{}, Refuse("required source revision " + requiredHead + " is unavailable in the selected source repository")
		}
		if required != head {
			return DeliverySource{}, Refuse("prepared branch " + branch + " is at " + head + ", not the required revision " + required + "; preserve that progress and reconcile it explicitly")
		}
	}
	if target == "" {
		return DeliverySource{}, Refuse("Integration Target is unavailable; supply its observed revision before inspecting the prepared branch")
	}
	resolvedTarget := deliveryResolveCommit(root, target)
	if resolvedTarget == "" {
		return DeliverySource{}, Refuse("Integration Target " + target + " is unavailable in the selected source repository")
	}
	result := DeliverySource{Worktree: worktree, Head: head, Target: resolvedTarget, Scope: "full"}
	if previous != "" {
		previousCommit := deliveryResolveCommit(root, previous)
		if previousCommit != "" {
			result.Previous = previousCommit
			if gitOK(root, "merge-base", "--is-ancestor", previousCommit, head) == nil {
				result.Scope = "incremental"
			}
		}
	}
	return result, nil
}

// ValidateUnpreparedPause permits an implementation pause without source
// metadata only before any planned source workspace or branch exists.
func ValidateUnpreparedPause(root, branch string) error {
	worktree, err := DeliveryWorktree(root, branch)
	if err != nil {
		return err
	}
	_, pathErr := os.Lstat(worktree)
	if pathErr == nil || gitOK(root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch) == nil {
		return Refuse("source progress exists; a paused implementation must record its clean head and target instead of omitting source evidence")
	}
	if !os.IsNotExist(pathErr) {
		return pathErr
	}
	return nil
}

// ValidateDeliverySource checks deterministic Git identities for a phase
// handoff. It requires full exact committed head and target object IDs, the
// planned clean branch at that head, and the target as an ancestor of the head.
// A reviewed revision, when supplied, must be available and ancestral; a final
// head distinct from it requires allowMarkers, and the worker - not this
// function - judges whether the marker comments are permitted. No remote-head
// equality is required. Dirty files produce an error and are never deleted.
func ValidateDeliverySource(root, branch, head, target, reviewed string, allowMarkers bool) error {
	worktree, err := deliveryWorktree(root, branch)
	if err != nil {
		return err
	}
	if !deliveryExplicitObjectID(head) {
		return Refuse("final head must be a full exact source object ID (schema 1 requires 40 lowercase hexadecimal characters)")
	}
	if !deliveryExplicitObjectID(target) {
		return Refuse("Integration Target must be a full exact source object ID (schema 1 requires 40 lowercase hexadecimal characters)")
	}
	current, err := git(worktree, "symbolic-ref", "--short", "HEAD")
	if err != nil || current != branch {
		return Refuse("worktree " + worktree + " is not on the planned branch " + branch + "; check out the planned branch without discarding work")
	}
	if err := deliveryClean(worktree); err != nil {
		return err
	}
	actual, err := git(worktree, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolve branch head in %s: %w", worktree, err)
	}
	if actual != head {
		return Refuse("branch " + branch + " is at " + actual + ", not the required final head " + head + "; preserve that progress and reconcile it explicitly")
	}
	if deliveryResolveCommit(root, head) != head {
		return Refuse("final head " + head + " is unavailable in the selected source repository")
	}
	if deliveryResolveCommit(root, target) != target {
		return Refuse("Integration Target " + target + " is unavailable in the selected source repository")
	}
	if gitOK(root, "merge-base", "--is-ancestor", target, head) != nil {
		return Refuse("Integration Target " + target + " is not an ancestor of final head " + head + "; integrate it before delivery")
	}
	if reviewed == "" {
		return nil
	}
	if !deliveryExplicitObjectID(reviewed) {
		return Refuse("reviewed revision must be a full exact source object ID (schema 1 requires 40 lowercase hexadecimal characters)")
	}
	if deliveryResolveCommit(root, reviewed) != reviewed {
		return Refuse("reviewed revision " + reviewed + " is unavailable in the selected source repository")
	}
	if gitOK(root, "merge-base", "--is-ancestor", reviewed, head) != nil {
		return Refuse("reviewed revision " + reviewed + " is not an ancestor of final head " + head)
	}
	if reviewed != head && !allowMarkers {
		return Refuse("reviewed and final source revisions differ; only permitted non-functional marker comments may add a distinct final head")
	}
	return nil
}

// DeliveryWorktree binds the planned location without requiring preparation.
func DeliveryWorktree(root, branch string) (string, error) {
	if !validConventionalBranch(root, branch) {
		return "", Refuse("invalid conventional branch identity; repair the Work Item attachment")
	}
	primary, err := primaryWorktree(root)
	if err != nil {
		return "", fmt.Errorf("resolve the primary source worktree: %w", err)
	}
	return filepath.Join(primary, ".worktrees", branch), nil
}

func deliveryWorktree(root, branch string) (string, error) {
	worktree, err := DeliveryWorktree(root, branch)
	if err != nil {
		return "", err
	}
	registered, err := registeredWorktrees(root)
	if err != nil {
		return "", fmt.Errorf("list registered source worktrees: %w", err)
	}
	existing, ok := registered[branch]
	if !ok {
		return "", Refuse("branch " + branch + " has no prepared worktree at " + worktree + "; prepare the source branch before delivery")
	}
	if filepath.Clean(existing) != filepath.Clean(worktree) {
		return "", Refuse("branch " + branch + " is checked out at " + existing + " instead of the expected worktree " + worktree + "; inspect that worktree")
	}
	if _, err := os.Stat(worktree); err != nil {
		return "", Refuse("prepared worktree " + worktree + " is unavailable; restore it without deleting branch progress")
	}
	return worktree, nil
}

func deliveryClean(worktree string) error {
	dirty, err := git(worktree, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("read worktree status in %s: %w", worktree, err)
	}
	if dirty != "" {
		return Refuse("worktree " + worktree + " has uncommitted or untracked files; commit or discard them explicitly without deleting unreviewed work: " + strings.Join(strings.Split(dirty, "\n"), "; "))
	}
	return nil
}

// deliveryFetch fetches one selected remote ref into a private per-invocation
// destination ref and returns the exact commit that fetch selected. It never
// reads the shared FETCH_HEAD, so a concurrent fetch of a different ref cannot
// substitute its commit for this caller's selected identity.
func deliveryFetch(root, remote, ref string) (string, error) {
	if remote == "" {
		return "", errors.New("no Git remote is configured")
	}
	destination := deliveryFetchRef(ref)
	defer func() { _ = gitOK(root, "update-ref", "-d", destination) }()
	if err := gitOK(root, "fetch", "--no-tags", remote, "+"+ref+":"+destination); err != nil {
		return "", err
	}
	fetched := deliveryResolveCommit(root, destination)
	if fetched == "" {
		return "", errors.New("fetched " + deliveryRefName(remote, ref) + " did not resolve to a commit")
	}
	return fetched, nil
}

// deliveryFetchRef names a per-invocation destination ref that no concurrent
// caller shares, so simultaneous preparations in one checkout cannot collide.
func deliveryFetchRef(ref string) string {
	token := atomic.AddUint64(&deliveryFetchSequence, 1)
	return fmt.Sprintf("refs/skl-delivery/%d-%d/%s", os.Getpid(), token, ref)
}

var deliveryFetchSequence uint64

func deliveryFetchStatus(remote, branch string, mainErr, branchErr error) string {
	if remote == "" {
		return "local: no Git remote is configured; using available local source inputs"
	}
	switch {
	case mainErr == nil && branchErr == nil:
		return "fresh: fetched " + remote + " main and " + branch
	case mainErr == nil:
		return "fresh: fetched " + remote + " main; " + branch + " is not available from the remote: " + branchErr.Error()
	case branchErr == nil:
		return "partial: fetched " + remote + " " + branch + "; main fetch failed: " + mainErr.Error() + "; target resolved from available local inputs"
	default:
		return "local: " + remote + " fetch failed (" + mainErr.Error() + "); using available local source inputs"
	}
}

func deliveryRefName(remote, ref string) string {
	if remote == "" {
		return ref
	}
	return remote + "/" + ref
}

func deliveryResolveCommit(root, ref string) string {
	id, err := git(root, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return ""
	}
	return id
}

// Schema 1 uses full SHA-1 identities for both source and ledger references.
func deliveryExplicitObjectID(value string) bool {
	if len(value) != 40 {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool {
		return !('0' <= r && r <= '9' || 'a' <= r && r <= 'f')
	}) < 0
}
