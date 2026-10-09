package workflow

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// IntegrationConflict refuses watchdog preparation: merging the Proposal
// Branch head into the work branch conflicts. The merge was aborted and the
// worktree is clean at Reviewed; the conflict belongs to the implementer.
type IntegrationConflict struct {
	Branch   string   `json:"branch"`
	Head     string   `json:"head"`
	Reviewed string   `json:"reviewed"`
	Paths    []string `json:"paths"`
}

func (c *IntegrationConflict) Error() string {
	return "Integration Merge of " + c.Branch + " at " + c.Head + " conflicts in " + strings.Join(c.Paths, ", ") + "; the merge was aborted and the worktree is clean at " + c.Reviewed
}

// IntegrationRef is the remote-tracking ref of a Proposal Branch Integration
// Target, or empty when the target is main or no remote is configured: only a
// Proposal Branch target receives an Integration Merge.
func IntegrationRef(remote, targetBranch string) string {
	if remote == "" || targetBranch == "" || targetBranch == "main" {
		return ""
	}
	return "refs/remotes/" + remote + "/" + targetBranch
}

// PrepareReviewSource prepares the source for a watchdog review. For a
// Proposal Branch target it then performs the Integration Merge: when the
// fetched branch head is not already an ancestor of the work branch, it
// commits one ordinary merge of that head on top of the existing commits. It
// never rebases, resets or rewrites. A conflict aborts the merge, leaves the
// worktree clean at the pre-merge head and refuses with *IntegrationConflict.
// The returned Merged is the branch head the prepared head integrates.
func PrepareReviewSource(root, remote, branch, reviewed, recordedTarget, targetBranch string) (DeliverySource, error) {
	source, err := PrepareDeliverySource(root, remote, branch, reviewed, recordedTarget, targetBranch)
	ref := IntegrationRef(remote, targetBranch)
	if err != nil || ref == "" {
		return source, err
	}
	merged, ok := integratedHead(root, reviewed, source.Head, ref)
	if !ok {
		// Inspection refuses a head that is neither the reviewed revision nor
		// its Integration Merge; nothing is merged onto unexpected progress.
		return source, nil
	}
	source.Merged = merged
	if gitOK(root, "merge-base", "--is-ancestor", source.Target, source.Head) == nil {
		return source, nil
	}
	if err := deliveryClean(source.Worktree); err != nil {
		return source, err
	}
	if _, err := git(source.Worktree, "merge", "--no-ff", "--no-edit", "-m", "Merge "+targetBranch+" into "+branch, source.Target); err != nil {
		return source, abortIntegration(source.Worktree, source.Head, targetBranch, source.Target, err)
	}
	head, err := git(source.Worktree, "rev-parse", "HEAD")
	if err != nil {
		return source, fmt.Errorf("resolve the Integration Merge in %s: %w", source.Worktree, err)
	}
	source.Head, source.Merged = head, source.Target
	return source, nil
}

// abortIntegration undoes a failed Integration Merge and confirms the worktree
// is clean at the pre-merge head before refusing.
func abortIntegration(worktree, pre, targetBranch, merging string, cause error) error {
	conflicted, _ := git(worktree, "diff", "--name-only", "--diff-filter=U")
	if gitOK(worktree, "rev-parse", "-q", "--verify", "MERGE_HEAD") == nil {
		if err := gitOK(worktree, "merge", "--abort"); err != nil {
			return fmt.Errorf("abort the Integration Merge in %s: %w", worktree, err)
		}
	}
	if head, err := git(worktree, "rev-parse", "HEAD"); err != nil || head != pre {
		return fmt.Errorf("after aborting the Integration Merge, %s is at %s instead of %s", worktree, head, pre)
	}
	if err := deliveryClean(worktree); err != nil {
		return err
	}
	if conflicted == "" {
		var exit *exec.ExitError
		if errors.As(cause, &exit) && len(exit.Stderr) > 0 {
			cause = fmt.Errorf("%w: %s", cause, strings.TrimSpace(string(exit.Stderr)))
		}
		return fmt.Errorf("Integration Merge of %s at %s failed: %w", targetBranch, merging, cause)
	}
	return &IntegrationConflict{Branch: targetBranch, Head: merging, Reviewed: pre, Paths: strings.Split(conflicted, "\n")}
}

// integratedHead reports whether head is reviewed, or descends from it only
// through Integration Merges: two-parent commits whose first parent continues
// the chain, whose second parent is on the Proposal Branch at ref, and whose
// tree is the clean merge of the two parents. It returns the branch head the
// latest Integration Merge brought in.
func integratedHead(root, reviewed, head, ref string) (string, bool) {
	merged := ""
	for current := head; current != reviewed; {
		if ref == "" {
			return "", false
		}
		line, err := git(root, "rev-list", "--parents", "-n", "1", current)
		parents := strings.Fields(line)
		if err != nil || len(parents) != 3 || gitOK(root, "merge-base", "--is-ancestor", parents[2], ref) != nil {
			return "", false
		}
		clean, err := git(root, "merge-tree", "--write-tree", parents[1], parents[2])
		if err != nil || clean != deliveryResolveTree(root, current) {
			return "", false
		}
		if merged == "" {
			merged = parents[2]
		}
		current = parents[1]
	}
	return merged, true
}

func deliveryResolveTree(root, commit string) string {
	tree, err := git(root, "rev-parse", "--verify", "--quiet", commit+"^{tree}")
	if err != nil {
		return ""
	}
	return tree
}

// IntegrationMerged returns the Proposal Branch head the branch's current head
// integrates through Integration Merges over reviewed, or empty when it has
// none. It reads local refs only.
func IntegrationMerged(root, branch, reviewed, ref string) string {
	head := deliveryResolveCommit(root, "refs/heads/"+branch)
	if head == "" || reviewed == "" {
		return ""
	}
	merged, _ := integratedHead(root, reviewed, head, ref)
	return merged
}
