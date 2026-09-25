package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type deliveryPrepared struct {
	source DeliverySource
	err    error
}

// TestDeliverySourceConcurrentPrepareKeepsSelectedIdentities drives two real
// PrepareDeliverySource callers in one checkout through a deterministic
// interleaving. A synchronization-only Git wrapper pauses the first caller
// after its own fetch has completed but before it resolves the selected
// identity, so the second caller's different fetch lands in between. Each
// caller must still observe the identity of the ref it selected; the wrapper
// only schedules real Git invocations and never fabricates output or objects.
func TestDeliverySourceConcurrentPrepareKeepsSelectedIdentities(t *testing.T) {
	t.Run("main identity", func(t *testing.T) {
		root := newGitRepository(t)
		newBareRemote(t, root)
		mainCommit := gitOutput(t, root, "rev-parse", "main")

		runGit(t, root, "checkout", "-q", "-b", "slice-b")
		commitFile(t, root, "unrelated.txt", "unrelated branch work\n")
		remoteB := gitOutput(t, root, "rev-parse", "HEAD")
		if remoteB == mainCommit {
			t.Fatal("test setup did not create a distinct slice commit")
		}
		runGit(t, root, "push", "-q", "upstream", "slice-b")
		commitFile(t, root, "progress.txt", "local progress\n")
		progress := gitOutput(t, root, "rev-parse", "HEAD")
		runGit(t, root, "checkout", "-q", "main")

		syncDir := synchronizedGitPath(t, "main")
		paused, concurrent := interleaveDeliveryPrepare(t, root, syncDir, "slice-d", "slice-b")

		if paused.Head != mainCommit || paused.Target != mainCommit {
			t.Fatalf("paused caller identities = head %s target %s; want main %s for both", paused.Head, paused.Target, mainCommit)
		}
		if concurrent.Head != progress || concurrent.Target != mainCommit {
			t.Fatalf("concurrent caller = head %s target %s; want its own head %s and main target %s", concurrent.Head, concurrent.Target, progress, mainCommit)
		}
		if _, err := os.Stat(filepath.Join(paused.Worktree, "unrelated.txt")); !os.IsNotExist(err) {
			t.Fatalf("paused caller inherited the other slice's commit content (stat err = %v)", err)
		}
		if got := gitOutput(t, root, "rev-parse", "refs/heads/slice-b"); got != progress {
			t.Fatalf("concurrent caller lost local branch progress: %s; want %s", got, progress)
		}
		if _, err := os.Stat(filepath.Join(concurrent.Worktree, "progress.txt")); err != nil {
			t.Fatalf("concurrent caller worktree lost preserved progress: %v", err)
		}
	})

	t.Run("branch identity", func(t *testing.T) {
		root := newGitRepository(t)
		newBareRemote(t, root)
		mainCommit := gitOutput(t, root, "rev-parse", "main")

		runGit(t, root, "checkout", "-q", "-b", "slice-b")
		commitFile(t, root, "unrelated.txt", "unrelated branch work\n")
		remoteB := gitOutput(t, root, "rev-parse", "HEAD")
		runGit(t, root, "push", "-q", "upstream", "slice-b")
		runGit(t, root, "checkout", "-q", "main")
		runGit(t, root, "checkout", "-q", "-b", "slice-c")
		commitFile(t, root, "other.txt", "other branch work\n")
		remoteC := gitOutput(t, root, "rev-parse", "HEAD")
		runGit(t, root, "push", "-q", "upstream", "slice-c")
		runGit(t, root, "checkout", "-q", "main")
		runGit(t, root, "branch", "-D", "slice-b", "slice-c")
		if remoteB == remoteC || remoteB == mainCommit || remoteC == mainCommit {
			t.Fatal("test setup did not create distinct branch commits")
		}

		syncDir := synchronizedGitPath(t, "slice-b")
		paused, concurrent := interleaveDeliveryPrepare(t, root, syncDir, "slice-b", "slice-c")

		if paused.Head != remoteB || paused.Target != mainCommit {
			t.Fatalf("paused caller branch identities = head %s target %s; want head %s target %s", paused.Head, paused.Target, remoteB, mainCommit)
		}
		if concurrent.Head != remoteC || concurrent.Target != mainCommit {
			t.Fatalf("concurrent caller = head %s target %s; want head %s target %s", concurrent.Head, concurrent.Target, remoteC, mainCommit)
		}
		if _, err := os.Stat(filepath.Join(paused.Worktree, "unrelated.txt")); err != nil {
			t.Fatalf("prepared branch worktree is missing its own fetched content: %v", err)
		}
	})
}

// interleaveDeliveryPrepare starts the paused caller, waits until its selected
// fetch has completed, runs the concurrent caller to completion, and only then
// lets the paused caller resolve its selected identity.
func interleaveDeliveryPrepare(t *testing.T, root, syncDir, pausedBranch, concurrentBranch string) (DeliverySource, DeliverySource) {
	t.Helper()
	pausedCh := make(chan deliveryPrepared, 1)
	concurrentCh := make(chan deliveryPrepared, 1)

	release := filepath.Join(syncDir, "release")
	released := false
	releaseNow := func() {
		if !released {
			released = true
			if err := os.WriteFile(release, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	defer releaseNow()

	go func() {
		source, err := PrepareDeliverySource(root, "upstream", pausedBranch, "", "")
		pausedCh <- deliveryPrepared{source, err}
	}()
	waitForSyncMarker(t, filepath.Join(syncDir, "paused"))

	go func() {
		source, err := PrepareDeliverySource(root, "upstream", concurrentBranch, "", "")
		concurrentCh <- deliveryPrepared{source, err}
	}()
	concurrent := receiveDeliveryPrepare(t, concurrentCh, "concurrent caller")
	releaseNow()
	paused := receiveDeliveryPrepare(t, pausedCh, "paused caller")

	if paused.err != nil {
		t.Fatalf("paused caller PrepareDeliverySource() error = %v", paused.err)
	}
	if concurrent.err != nil {
		t.Fatalf("concurrent caller PrepareDeliverySource() error = %v", concurrent.err)
	}
	return paused.source, concurrent.source
}

// synchronizedGitPath installs a Git wrapper that pauses once, after the real
// fetch matching pauseRef succeeds, and delegates every other invocation to
// real Git unchanged.
func synchronizedGitPath(t *testing.T, pauseRef string) string {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	syncDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ -n \"$SKL_DELIVERY_SYNC\" ]; then\n" +
		"  is_fetch=\"\"\n" +
		"  for arg in \"$@\"; do\n" +
		"    [ \"$arg\" = \"fetch\" ] && is_fetch=1\n" +
		"  done\n" +
		"  matched=\"\"\n" +
		"  if [ -n \"$is_fetch\" ]; then\n" +
		"    for arg in \"$@\"; do\n" +
		"      case \"$arg\" in \"$SKL_DELIVERY_PAUSE_REF\"|\"+$SKL_DELIVERY_PAUSE_REF:\"*|\"$SKL_DELIVERY_PAUSE_REF:\"*) matched=1 ;; esac\n" +
		"    done\n" +
		"  fi\n" +
		"  if [ -n \"$matched\" ] && [ ! -e \"$SKL_DELIVERY_SYNC/paused\" ]; then\n" +
		"    \"$SKL_DELIVERY_REAL_GIT\" \"$@\"\n" +
		"    status=$?\n" +
		"    if [ \"$status\" -eq 0 ]; then\n" +
		"      : > \"$SKL_DELIVERY_SYNC/paused\"\n" +
		"      while [ ! -e \"$SKL_DELIVERY_SYNC/release\" ]; do sleep 0.02; done\n" +
		"    fi\n" +
		"    exit \"$status\"\n" +
		"  fi\n" +
		"fi\n" +
		"exec \"$SKL_DELIVERY_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SKL_DELIVERY_REAL_GIT", realGit)
	t.Setenv("SKL_DELIVERY_SYNC", syncDir)
	t.Setenv("SKL_DELIVERY_PAUSE_REF", pauseRef)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return syncDir
}

func waitForSyncMarker(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the interleaving marker %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func receiveDeliveryPrepare(t *testing.T, results <-chan deliveryPrepared, caller string) deliveryPrepared {
	t.Helper()
	select {
	case prepared := <-results:
		return prepared
	case <-time.After(30 * time.Second):
		t.Fatalf("%s did not complete", caller)
		return deliveryPrepared{}
	}
}
