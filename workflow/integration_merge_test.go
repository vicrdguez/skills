package workflow

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// integrationFixture is a source repository with a local bare remote, a
// Proposal Branch on it, and a Slice work branch submitted at Reviewed.
type integrationFixture struct {
	root, branch, reviewed, target string
	// pusher is a second clone that advances the Proposal Branch.
	pusher string
}

func newIntegrationFixture(t *testing.T) integrationFixture {
	t.Helper()
	root := newGitRepository(t)
	remote := newBareRemote(t, root)
	runGit(t, root, "push", "-q", "upstream", "main:refs/heads/proposal/feature")
	target := gitOutput(t, root, "rev-parse", "HEAD")
	source, err := PrepareDeliverySource(root, "upstream", "slice", "", "", "proposal/feature")
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, source.Worktree, "slice.txt", "slice work\n")
	pusher := filepath.Join(t.TempDir(), "pusher")
	runGit(t, root, "clone", "-q", "-b", "proposal/feature", remote, pusher)
	runGit(t, pusher, "config", "user.name", "Other")
	runGit(t, pusher, "config", "user.email", "other@example.com")
	return integrationFixture{root: root, branch: "slice", reviewed: gitOutput(t, source.Worktree, "rev-parse", "HEAD"), target: target, pusher: pusher}
}

// advance lands another Slice's commit on the Proposal Branch.
func (f integrationFixture) advance(t *testing.T, name, contents string) string {
	t.Helper()
	commitFile(t, f.pusher, name, contents)
	runGit(t, f.pusher, "push", "-q", "origin", "proposal/feature")
	return gitOutput(t, f.pusher, "rev-parse", "HEAD")
}

func (f integrationFixture) prepare(t *testing.T) (DeliverySource, error) {
	t.Helper()
	return PrepareReviewSource(f.root, "upstream", f.branch, f.reviewed, f.target, "proposal/feature")
}

const integrationRef = "refs/remotes/upstream/proposal/feature"

func TestReviewPreparationMergesMovedProposalBranch(t *testing.T) {
	f := newIntegrationFixture(t)
	moved := f.advance(t, "other.txt", "other slice\n")

	prepared, err := f.prepare(t)
	if err != nil {
		t.Fatal(err)
	}
	if parents := strings.Fields(gitOutput(t, prepared.Worktree, "rev-list", "--parents", "-n", "1", "HEAD")); len(parents) != 3 || parents[1] != f.reviewed || parents[2] != moved {
		t.Fatalf("prepared head parents = %v; want a merge of %s and %s", parents, f.reviewed, moved)
	}
	if prepared.Head != gitOutput(t, prepared.Worktree, "rev-parse", "HEAD") || prepared.Merged != moved {
		t.Fatalf("prepared = %+v; want the merge commit with merged head %s", prepared, moved)
	}
	inspected, err := InspectDeliverySource(f.root, f.branch, f.reviewed, f.target, "", integrationRef)
	if err != nil || inspected.Head != prepared.Head || inspected.Merged != moved {
		t.Fatalf("inspection = %+v, %v; want the merged head %s", inspected, err, prepared.Head)
	}

	// Re-running with the branch unchanged commits nothing further.
	again, err := f.prepare(t)
	if err != nil || again.Head != prepared.Head || again.Merged != moved {
		t.Fatalf("re-run = %+v, %v; want unchanged head %s", again, err, prepared.Head)
	}
	// A later branch head gets one more merge on top, never a rewrite.
	later := f.advance(t, "third.txt", "third slice\n")
	remerged, err := f.prepare(t)
	if err != nil || remerged.Merged != later || gitOutput(t, f.root, "rev-parse", remerged.Head+"^1") != prepared.Head {
		t.Fatalf("later preparation = %+v, %v; want a merge of %s over %s", remerged, err, later, prepared.Head)
	}
	if err := ValidateDeliverySource(f.root, f.branch, remerged.Head, f.target, f.reviewed, false, integrationRef); err != nil {
		t.Fatalf("rework at the merged head refused: %v", err)
	}
}

func TestReviewPreparationLeavesIntegratedBranchAlone(t *testing.T) {
	f := newIntegrationFixture(t)
	for range 2 {
		prepared, err := f.prepare(t)
		if err != nil || prepared.Head != f.reviewed || prepared.Merged != "" {
			t.Fatalf("preparation = %+v, %v; want the unchanged head %s", prepared, err, f.reviewed)
		}
	}
}

func TestReviewPreparationRefusesConflictWithCleanWorktree(t *testing.T) {
	f := newIntegrationFixture(t)
	moved := f.advance(t, "slice.txt", "a different slice\n")

	_, err := f.prepare(t)
	var conflict *IntegrationConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("conflicting preparation error = %v; want an IntegrationConflict", err)
	}
	if conflict.Branch != "proposal/feature" || conflict.Head != moved || conflict.Prepared != f.reviewed || !slices.Equal(conflict.Paths, []string{"slice.txt"}) {
		t.Fatalf("conflict = %+v", conflict)
	}
	worktree := deliveryExpectedWorktree(t, f.root, f.branch)
	if head := gitOutput(t, worktree, "rev-parse", "HEAD"); head != f.reviewed {
		t.Fatalf("conflict left head %s; want %s", head, f.reviewed)
	}
	if status, _ := git(worktree, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Fatalf("conflict left the worktree dirty: %s", status)
	}
	if _, err := git(worktree, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err == nil {
		t.Fatal("conflict left a merge in progress")
	}
}

func TestMainTargetedReviewPreparationMergesNothing(t *testing.T) {
	root := newGitRepository(t)
	newBareRemote(t, root)
	target := gitOutput(t, root, "rev-parse", "HEAD")
	source, err := PrepareDeliverySource(root, "upstream", "slice", "", "")
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, source.Worktree, "slice.txt", "slice work\n")
	reviewed := gitOutput(t, source.Worktree, "rev-parse", "HEAD")
	commitFile(t, root, "newer.txt", "newer main\n")
	runGit(t, root, "push", "-q", "upstream", "main")

	prepared, err := PrepareReviewSource(root, "upstream", "slice", reviewed, target, "main")
	if err != nil || prepared.Head != reviewed || prepared.Target != target || prepared.Merged != "" {
		t.Fatalf("main-targeted preparation = %+v, %v; want head %s at recorded target %s", prepared, err, reviewed, target)
	}
}

func TestValidationAcceptsOnlyTheIntegrationMerge(t *testing.T) {
	f := newIntegrationFixture(t)
	f.advance(t, "other.txt", "other slice\n")
	prepared, err := f.prepare(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, pass := range []bool{false, true} {
		if err := ValidateDeliverySource(f.root, f.branch, prepared.Head, f.target, f.reviewed, pass, integrationRef); err != nil {
			t.Fatalf("merged head (pass=%t) refused: %v", pass, err)
		}
	}
	// The same merge is not accepted for a main-targeted Slice.
	if err := ValidateDeliverySource(f.root, f.branch, prepared.Head, f.target, f.reviewed, false, ""); err == nil {
		t.Fatal("a main-targeted rework accepted a merged head")
	}

	// A functional commit beyond the merge is refused except on pass.
	commitFile(t, prepared.Worktree, "extra.txt", "functional change\n")
	extra := gitOutput(t, prepared.Worktree, "rev-parse", "HEAD")
	if err := ValidateDeliverySource(f.root, f.branch, extra, f.target, f.reviewed, false, integrationRef); err == nil {
		t.Fatal("rework accepted a functional commit beyond the Integration Merge")
	}
	if err := ValidateDeliverySource(f.root, f.branch, extra, f.target, f.reviewed, true, integrationRef); err != nil {
		t.Fatalf("pass refused a marker commit after the Integration Merge: %v", err)
	}

	// A merge that carries edits of its own is not an Integration Merge.
	runGit(t, prepared.Worktree, "reset", "-q", "--hard", f.reviewed)
	runGit(t, prepared.Worktree, "merge", "-q", "--no-ff", "--no-commit", prepared.Merged)
	commitFile(t, prepared.Worktree, "hidden.txt", "edit inside the merge\n")
	amended := gitOutput(t, prepared.Worktree, "rev-parse", "HEAD")
	if err := ValidateDeliverySource(f.root, f.branch, amended, f.target, f.reviewed, false, integrationRef); err == nil {
		t.Fatal("rework accepted a merge commit carrying its own edits")
	}
	if _, err := InspectDeliverySource(f.root, f.branch, f.reviewed, f.target, "", integrationRef); err == nil {
		t.Fatal("inspection accepted a merge commit carrying its own edits")
	}

	// A merge of a commit off the Proposal Branch is not an Integration Merge.
	runGit(t, prepared.Worktree, "reset", "-q", "--hard", f.reviewed)
	runGit(t, prepared.Worktree, "switch", "-q", "-c", "side", f.target)
	commitFile(t, prepared.Worktree, "side.txt", "side work\n")
	side := gitOutput(t, prepared.Worktree, "rev-parse", "HEAD")
	runGit(t, prepared.Worktree, "switch", "-q", f.branch)
	runGit(t, prepared.Worktree, "merge", "-q", "--no-ff", "--no-edit", side)
	offBranch := gitOutput(t, prepared.Worktree, "rev-parse", "HEAD")
	if err := ValidateDeliverySource(f.root, f.branch, offBranch, f.target, f.reviewed, false, integrationRef); err == nil {
		t.Fatal("rework accepted a merge of a commit off the Proposal Branch")
	}
}
