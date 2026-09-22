package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliverySourceRefusesUnsupportedSchemaObjectFormat(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q", "-b", "main", "--object-format=sha256")
	runGit(t, root, "config", "user.name", "Worker")
	runGit(t, root, "config", "user.email", "worker@example.com")
	commitFile(t, root, "source.txt", "local progress\n")
	head := gitOutput(t, root, "rev-parse", "HEAD")
	source, err := PrepareDeliverySource(root, "unavailable", "feature-sha256", head, head)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeliverySource(root, "feature-sha256", source.Head, source.Target, "", false); err == nil {
		t.Fatal("source validation accepted an identity the schema-1 report cannot record")
	}
	if got := gitOutput(t, source.Worktree, "rev-parse", "HEAD"); got != head {
		t.Fatal("unsupported-format refusal changed source progress")
	}
}

func TestDeliverySourcePrepareCreatesInitialBranch(t *testing.T) {
	root := newGitRepository(t)
	newBareRemote(t, root)
	target := gitOutput(t, root, "rev-parse", "HEAD")

	source, err := PrepareDeliverySource(root, "upstream", "feature-one", "", "")
	if err != nil {
		t.Fatalf("PrepareDeliverySource() error = %v", err)
	}
	if source.Head != target || source.Target != target {
		t.Fatalf("head/target = %s/%s; want %s/%s", source.Head, source.Target, target, target)
	}
	if !strings.HasPrefix(source.FetchStatus, "fresh") {
		t.Fatalf("FetchStatus = %q; want a fresh fetch of the available target", source.FetchStatus)
	}
	wantWorktree := deliveryExpectedWorktree(t, root, "feature-one")
	if source.Worktree != wantWorktree {
		t.Fatalf("worktree = %q; want %q", source.Worktree, wantWorktree)
	}
	if info, err := os.Stat(source.Worktree); err != nil || !info.IsDir() {
		t.Fatalf("prepared worktree stat = %v, %v", info, err)
	}
	if got := gitOutput(t, root, "rev-parse", "refs/heads/feature-one"); got != target {
		t.Fatalf("created branch head = %s; want %s", got, target)
	}

	inspection, err := InspectDeliverySource(root, "feature-one", "", source.Target, "")
	if err != nil {
		t.Fatalf("InspectDeliverySource() error = %v", err)
	}
	if inspection.Head != target || inspection.Target != target || inspection.Scope != "full" || inspection.Previous != "" {
		t.Fatalf("inspection = %#v; want clean full-scope inspection at %s", inspection, target)
	}
}

func TestDeliverySourcePreparePreservesProgress(t *testing.T) {
	root := newGitRepository(t)
	newBareRemote(t, root)
	base := gitOutput(t, root, "rev-parse", "HEAD")
	source, err := PrepareDeliverySource(root, "upstream", "feature-two", "", "")
	if err != nil {
		t.Fatalf("initial PrepareDeliverySource() error = %v", err)
	}
	commitFile(t, source.Worktree, "feature.txt", "work\n")
	progress := gitOutput(t, source.Worktree, "rev-parse", "HEAD")
	if progress == base {
		t.Fatal("test setup did not create source progress")
	}
	scratch := filepath.Join(source.Worktree, "notes.tmp")
	if err := os.WriteFile(scratch, []byte("scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	again, err := PrepareDeliverySource(root, "upstream", "feature-two", base, "")
	if err != nil {
		t.Fatalf("resumed PrepareDeliverySource() error = %v", err)
	}
	if again.Head != progress {
		t.Fatalf("resumed head = %s; want preserved progress %s", again.Head, progress)
	}
	if again.Worktree != source.Worktree {
		t.Fatalf("resumed worktree = %q; want preserved %q", again.Worktree, source.Worktree)
	}
	if _, err := os.Stat(scratch); err != nil {
		t.Fatalf("resumed prepare removed the dirty worktree file: %v", err)
	}
}

func TestDeliverySourcePrepareUsesAvailableLocalInputsOffline(t *testing.T) {
	root := newGitRepository(t)
	runGit(t, root, "remote", "add", "upstream", filepath.Join(t.TempDir(), "absent"))
	head := gitOutput(t, root, "rev-parse", "HEAD")
	runGit(t, root, "update-ref", "refs/remotes/upstream/main", head)

	source, err := PrepareDeliverySource(root, "upstream", "feature-offline", head, head)
	if err != nil {
		t.Fatalf("PrepareDeliverySource() error = %v", err)
	}
	if source.Head != head || source.Target != head {
		t.Fatalf("head/target = %s/%s; want available local inputs %s", source.Head, source.Target, head)
	}
	if strings.HasPrefix(source.FetchStatus, "fresh") {
		t.Fatalf("FetchStatus claimed freshness during a failed fetch: %q", source.FetchStatus)
	}
	if !strings.Contains(source.FetchStatus, "local") {
		t.Fatalf("FetchStatus = %q; want an honest local-input report", source.FetchStatus)
	}
}

func TestDeliverySourcePrepareRetainsFetchedTargetForOfflineUse(t *testing.T) {
	root := newGitRepository(t)
	remote := newBareRemote(t, root)
	publisher := t.TempDir()
	runGit(t, publisher, "clone", "-q", "--branch", "main", remote, ".")
	runGit(t, publisher, "config", "user.name", "Publisher")
	runGit(t, publisher, "config", "user.email", "publisher@example.com")
	commitFile(t, publisher, "later-main.txt", "later target\n")
	runGit(t, publisher, "push", "-q", "origin", "main")
	target := gitOutput(t, publisher, "rev-parse", "HEAD")

	// A remote may track only selected branches. Preparation still observes
	// main explicitly and must retain that observation for offline fallback.
	runGit(t, root, "config", "remote.upstream.fetch", "+refs/heads/tracked-only:refs/remotes/upstream/tracked-only")
	online, err := PrepareDeliverySource(root, "upstream", "online-slice", "", "")
	if err != nil || online.Target != target {
		t.Fatalf("online preparation = %#v, %v; want target %s", online, err, target)
	}
	runGit(t, root, "remote", "set-url", "upstream", filepath.Join(t.TempDir(), "unavailable"))
	offline, err := PrepareDeliverySource(root, "upstream", "offline-slice", "", "")
	if err != nil || offline.Target != target || offline.Head != target {
		t.Fatalf("offline preparation = %#v, %v; want last observed target %s", offline, err, target)
	}
	if !strings.HasPrefix(offline.FetchStatus, "local:") {
		t.Fatalf("offline preparation claimed freshness: %q", offline.FetchStatus)
	}
}

func TestDeliverySourcePreparePreservesRecordedTargetAfterFetch(t *testing.T) {
	root := newGitRepository(t)
	newBareRemote(t, root)
	target := gitOutput(t, root, "rev-parse", "HEAD")
	commitFile(t, root, "later-main.txt", "later target\n")
	runGit(t, root, "push", "upstream", "main")
	source, err := PrepareDeliverySource(root, "upstream", "fixed-review", target, target)
	if err != nil || source.Target != target || source.Head != target {
		t.Fatalf("fixed source inputs changed: %#v, %v", source, err)
	}
	if _, err := PrepareDeliverySource(root, "upstream", "missing-fixed-target", "", strings.Repeat("a", 40)); err == nil {
		t.Fatal("unavailable recorded target was replaced by freshly fetched main")
	}
}

func TestDeliverySourcePrepareRefusesMissingInputs(t *testing.T) {
	t.Run("missing required revision", func(t *testing.T) {
		root := newGitRepository(t)
		runGit(t, root, "remote", "add", "upstream", filepath.Join(t.TempDir(), "absent"))
		head := gitOutput(t, root, "rev-parse", "HEAD")
		runGit(t, root, "update-ref", "refs/remotes/upstream/main", head)
		missing := strings.Repeat("a", 40)
		if _, err := PrepareDeliverySource(root, "upstream", "feature-missing", missing, head); err == nil {
			t.Fatal("PrepareDeliverySource() accepted an unavailable required revision")
		}
		if gitOK(root, "rev-parse", "--verify", "refs/heads/feature-missing") == nil {
			t.Fatal("refusal fabricated a source branch")
		}
	})
	t.Run("missing target", func(t *testing.T) {
		root := newGitRepository(t)
		runGit(t, root, "remote", "add", "upstream", filepath.Join(t.TempDir(), "absent"))
		if _, err := PrepareDeliverySource(root, "upstream", "feature-notarget", "", ""); err == nil {
			t.Fatal("PrepareDeliverySource() accepted a missing Integration Target")
		}
		if gitOK(root, "rev-parse", "--verify", "refs/heads/feature-notarget") == nil {
			t.Fatal("refusal fabricated a source branch")
		}
	})
}

func TestDeliverySourceInspectRefusesUncleanOrWrongHead(t *testing.T) {
	root := newGitRepository(t)
	newBareRemote(t, root)
	source, err := PrepareDeliverySource(root, "upstream", "feature-inspect", "", "")
	if err != nil {
		t.Fatalf("PrepareDeliverySource() error = %v", err)
	}
	if _, err := InspectDeliverySource(root, "feature-inspect", "", source.Target, ""); err != nil {
		t.Fatalf("clean InspectDeliverySource() error = %v", err)
	}
	if _, err := InspectDeliverySource(root, "feature-inspect", strings.Repeat("b", 40), source.Target, ""); err == nil {
		t.Fatal("InspectDeliverySource() ignored a required head mismatch")
	}
	if err := os.WriteFile(filepath.Join(source.Worktree, "scratch.txt"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDeliverySource(root, "feature-inspect", "", source.Target, ""); err == nil {
		t.Fatal("InspectDeliverySource() accepted a dirty worktree as unambiguous evidence")
	}
}

func TestDeliverySourceValidateChecksHeadAndTarget(t *testing.T) {
	root := newGitRepository(t)
	newBareRemote(t, root)
	source, err := PrepareDeliverySource(root, "upstream", "feature-validate", "", "")
	if err != nil {
		t.Fatalf("PrepareDeliverySource() error = %v", err)
	}
	head, target := source.Head, source.Target
	if err := ValidateDeliverySource(root, "feature-validate", head, target, head, false); err != nil {
		t.Fatalf("valid ValidateDeliverySource() error = %v", err)
	}
	other := deliveryDanglingCommit(t, root)
	if err := ValidateDeliverySource(root, "feature-validate", other, target, other, false); err == nil {
		t.Fatal("ValidateDeliverySource() accepted a branch head other than the actual one")
	}
	if err := ValidateDeliverySource(root, "feature-validate", head, other, head, false); err == nil {
		t.Fatal("ValidateDeliverySource() accepted a target that is not an ancestor of the head")
	}
	if err := ValidateDeliverySource(root, "feature-validate", head[:12], target, head[:12], false); err == nil {
		t.Fatal("ValidateDeliverySource() accepted an abbreviated head object ID")
	}
	if err := os.WriteFile(filepath.Join(source.Worktree, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeliverySource(root, "feature-validate", head, target, head, false); err == nil {
		t.Fatal("ValidateDeliverySource() accepted a dirty worktree")
	}
}

func TestDeliverySourceValidateReviewedDistinction(t *testing.T) {
	root := newGitRepository(t)
	newBareRemote(t, root)
	source, err := PrepareDeliverySource(root, "upstream", "feature-review", "", "")
	if err != nil {
		t.Fatalf("PrepareDeliverySource() error = %v", err)
	}
	reviewed := source.Head
	commitFile(t, source.Worktree, "marker.txt", "debt marker\n")
	final := gitOutput(t, source.Worktree, "rev-parse", "HEAD")
	if final == reviewed {
		t.Fatal("test setup did not add a post-review marker commit")
	}
	if err := ValidateDeliverySource(root, "feature-review", final, source.Target, reviewed, true); err != nil {
		t.Fatalf("marker-permitted ValidateDeliverySource() error = %v", err)
	}
	if err := ValidateDeliverySource(root, "feature-review", final, source.Target, reviewed, false); err == nil {
		t.Fatal("ValidateDeliverySource() accepted a distinct final head without marker permission")
	}
	if err := ValidateDeliverySource(root, "feature-review", final, source.Target, final, false); err != nil {
		t.Fatalf("equal reviewed/final ValidateDeliverySource() error = %v", err)
	}
	if err := ValidateDeliverySource(root, "feature-review", final, source.Target, strings.Repeat("c", 40), true); err == nil {
		t.Fatal("ValidateDeliverySource() accepted an unavailable reviewed revision")
	}
	other := deliveryDanglingCommit(t, root)
	if err := ValidateDeliverySource(root, "feature-review", final, source.Target, other, true); err == nil {
		t.Fatal("ValidateDeliverySource() accepted a non-ancestral reviewed revision")
	}
}

func TestDeliverySourceInspectReviewScope(t *testing.T) {
	root := newGitRepository(t)
	newBareRemote(t, root)
	source, err := PrepareDeliverySource(root, "upstream", "feature-scope", "", "")
	if err != nil {
		t.Fatalf("PrepareDeliverySource() error = %v", err)
	}
	base := gitOutput(t, source.Worktree, "rev-parse", "HEAD")
	commitFile(t, source.Worktree, "one.txt", "1\n")
	commitFile(t, source.Worktree, "two.txt", "2\n")
	head := gitOutput(t, source.Worktree, "rev-parse", "HEAD")

	inspected, err := InspectDeliverySource(root, "feature-scope", head, source.Target, base)
	if err != nil {
		t.Fatalf("ancestral InspectDeliverySource() error = %v", err)
	}
	if inspected.Scope != "incremental" || inspected.Previous != base || inspected.Head != head {
		t.Fatalf("ancestral inspection = %#v; want incremental scope over %s", inspected, base)
	}

	other := deliveryDanglingCommit(t, root)
	inspected, err = InspectDeliverySource(root, "feature-scope", "", source.Target, other)
	if err != nil {
		t.Fatalf("non-ancestral InspectDeliverySource() error = %v", err)
	}
	if inspected.Scope != "full" {
		t.Fatalf("non-ancestral scope = %q; want full", inspected.Scope)
	}

	missing := strings.Repeat("d", 40)
	inspected, err = InspectDeliverySource(root, "feature-scope", "", source.Target, missing)
	if err != nil {
		t.Fatalf("unavailable InspectDeliverySource() error = %v", err)
	}
	if inspected.Scope != "full" || inspected.Previous != "" {
		t.Fatalf("unavailable previous inspection = %#v; want full with no previous", inspected)
	}

	if _, err := InspectDeliverySource(root, "feature-scope", other, source.Target, ""); err == nil {
		t.Fatal("InspectDeliverySource() ignored a required head mismatch")
	}
}

func newBareRemote(t *testing.T, root string) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	runGit(t, root, "init", "--bare", remote)
	runGit(t, root, "remote", "add", "upstream", remote)
	runGit(t, root, "push", "upstream", "main")
	return remote
}

func deliveryExpectedWorktree(t *testing.T, root, branch string) string {
	t.Helper()
	primary, err := primaryWorktree(root)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(primary, ".worktrees", branch)
}

func deliveryDanglingCommit(t *testing.T, root string) string {
	t.Helper()
	tree := gitOutput(t, root, "rev-parse", "HEAD^{tree}")
	output, err := exec.Command("git", "-C", root, "commit-tree", tree, "-m", "dangling").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}
