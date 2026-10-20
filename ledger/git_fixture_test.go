package ledger_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A ledger fixture must not return from its last commit while Git can still
// write pack files: TempDir cleanup may begin as soon as the command returns.
func TestDeliveryLedgerWaitsForAutoGC(t *testing.T) {
	t.Parallel()
	l := newDeliveryLedger(t)
	var major, minor int
	if _, err := fmt.Sscanf(deliveryGitOutput(t, l.root, "version"), "git version %d.%d", &major, &minor); err != nil {
		t.Fatal(err)
	}
	if major < 2 || (major == 2 && minor < 42) {
		t.Skip("fault injection uses gc.recentObjectsHook, added in Git 2.42")
	}
	// Two small packs force auto-GC without relying on Git's sampled loose
	// object count. This still exercises real packing, not a Git shim.
	deliveryGit(t, l.root, "repack", "-d")
	l.addFile("extra.txt", "another pack\n")
	l.commitAll("add another object")
	deliveryGit(t, l.root, "repack", "-d")
	// The recent-objects hook is consulted while packing unreachable objects.
	l.addFile("unreachable.txt", "uncommitted object\n")
	deliveryGit(t, l.root, "hash-object", "-w", "unreachable.txt")
	if err := os.Remove(filepath.Join(l.root, "unreachable.txt")); err != nil {
		t.Fatal(err)
	}

	// Keep packing in flight long enough to expose an early command return.
	// The marker is outside the repository so even the failing variant can
	// wait for the background writer before removing its fixture.
	hook := filepath.Join(t.TempDir(), "recent-objects")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nsleep 1\n: > \"$0.done\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "config", "gc.recentObjectsHook", "\""+hook+"\"")
	deliveryGit(t, l.root, "config", "gc.autoPackLimit", "1")
	deliveryGit(t, l.root, "config", "gc.pruneExpire", "now")
	deliveryGit(t, l.root, "config", "maintenance.gc.enabled", "true")
	deliveryGit(t, l.root, "config", "maintenance.geometric-repack.enabled", "false")
	// Reap the real auto-GC even if the assertion below fails on a regression.
	t.Cleanup(func() {
		deliveryWaitForPaths(t, hook+".done")
		deadline := time.Now().Add(15 * time.Second)
		for {
			_, err := os.Stat(filepath.Join(l.root, ".git", "gc.pid"))
			if os.IsNotExist(err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatal("auto-GC did not finish before fixture cleanup")
			}
			time.Sleep(2 * time.Millisecond)
		}
	})
	deliveryGit(t, l.root, "commit", "-q", "--allow-empty", "-m", "trigger auto-GC")
	if _, err := os.Stat(hook + ".done"); err != nil {
		t.Errorf("commit returned while auto-GC could still write into the fixture: %v", err)
	}
}
