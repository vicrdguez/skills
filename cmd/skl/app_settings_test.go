package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Keep this serial: it exercises the production environment adapter, not the
// private lookup used by parallel scenarios. The app predates every selection.
func TestAppReloadsEnvironmentAndConfigurationSelection(t *testing.T) {
	first, second := newPrivateLedgerFixture(t), newPrivateLedgerFixture(t)
	for index, fixture := range []*ledgerFixture{first, second} {
		writeFile(t, filepath.Join(fixture.clone, "selected.txt"), fmt.Sprintf("ledger %d\n", index))
		runGit(t, fixture.clone, "add", "selected.txt")
		runGit(t, fixture.clone, "commit", "-qm", "distinct selection")
	}
	firstHead := deliveryTrimmed(t, first.clone, "rev-parse", "HEAD")
	secondHead := deliveryTrimmed(t, second.clone, "rev-parse", "HEAD")
	cli := newLedgerApp(t, newForgeServer(t))
	show := func(head, want string) {
		t.Helper()
		out, _ := cli.run(t, []string{"skl", "ledger", "show", "--commit", head, "--path", "selected.txt", "--format", "json"})
		if out.Status != "shown" || out.Document == nil || out.Document.Contents != want {
			t.Fatalf("selection = %s, want %q", mustJSON(t, out), want)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", first.config)
	t.Setenv("HOME", second.home)
	writeFile(t, filepath.Join(second.home, ".config", "skl", "config.json"), fmt.Sprintf(`{"ledger":%q}`, first.clone))
	show(firstHead, "ledger 0\n")

	// Editing the selected file changes this same app's next operation.
	first.misconfigure(t, fmt.Sprintf(`{"ledger":%q}`, second.clone))
	show(secondHead, "ledger 1\n")
	t.Setenv("XDG_CONFIG_HOME", "")
	show(firstHead, "ledger 0\n")
	writeFile(t, filepath.Join(first.home, ".config", "skl", "config.json"), fmt.Sprintf(`{"ledger":%q}`, second.clone))
	t.Setenv("HOME", first.home)
	show(secondHead, "ledger 1\n")

	// A selected but absent XDG file must not fall back to the valid HOME.
	missing := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", missing)
	out, _ := cli.run(t, []string{"skl", "ledger", "show", "--commit", secondHead, "--path", "selected.txt", "--format", "json"})
	if out.Status != "fix_required" || !strings.Contains(out.Reason, missing) || !strings.Contains(out.Repair, "create") {
		t.Fatalf("missing XDG selection fell back or lost repair: %s", mustJSON(t, out))
	}
	t.Setenv("XDG_CONFIG_HOME", first.config)
	first.misconfigure(t, "{malformed")
	out, _ = cli.run(t, []string{"skl", "ledger", "show", "--commit", secondHead, "--path", "selected.txt", "--format", "json"})
	if out.Status != "fix_required" || out.Document != nil || !strings.Contains(out.Reason, "config") {
		t.Fatalf("changed malformed settings were not refused: %s", mustJSON(t, out))
	}
}

func TestAppConfigurationsAreIndependent(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"first", "second"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newPrivateLedgerFixture(t)
			source, _ := deliverySourceRepo(t)
			cli := fixture.app(newLedgerApp(t, newForgeServer(t)))
			if accepted := cli.accept(t, source, writeProposal(t, "", singleSlice(name))); accepted.Status != "accepted" {
				t.Fatalf("accept private selection: %s", mustJSON(t, accepted))
			}
			out, _ := cli.run(t, []string{"skl", "slice", "show", "--repo", source, "--slice", name + "/foundation", "--format", "json"})
			if out.Readback == nil || out.Readback.Item != name+"/foundation" {
				t.Fatalf("private readback = %s", mustJSON(t, out))
			}
		})
	}
}

func TestSourceTransportProtectionFollowsWorktreesNotClones(t *testing.T) {
	t.Parallel()
	source, _ := deliverySourceRepo(t)
	worktree := filepath.Join(t.TempDir(), "worktree")
	runGit(t, source, "worktree", "add", "-qb", "worker", worktree)
	for _, root := range []string{source, worktree} {
		if got := deliveryTrimmed(t, root, "config", "core.sshCommand"); got != "false" {
			t.Fatalf("%s has no offline SSH protection: %q", root, got)
		}
		// Even a changed URL cannot activate a different real network transport.
		command := exec.Command("git", "-C", root, "ls-remote", "https://example.invalid/widgets.git")
		if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "transport 'https' not allowed") {
			t.Fatalf("network transport was not blocked: %v: %s", err, output)
		}
	}
	bare := routeSourcePushes(t, source)
	runGit(t, worktree, "push", "-q", "origin", "HEAD:main")
	if got := deliveryTrimmed(t, bare, "rev-parse", "main"); got != deliveryTrimmed(t, source, "rev-parse", "HEAD") {
		t.Fatal("repository-local SSH shim did not reach its local remote from the worktree")
	}
	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, source, "clone", "-q", bare, clone)
	// A clone owns a local-file remote, not the source's SSH identity or shim.
	if got := deliveryTrimmed(t, clone, "remote", "get-url", "origin"); got != bare {
		t.Fatalf("clone remote = %q, want %q", got, bare)
	}
	if output, err := exec.Command("git", "-C", clone, "config", "--local", "--get", "core.sshCommand").CombinedOutput(); err == nil || len(output) != 0 {
		t.Fatalf("clone inherited the source SSH shim: %v: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(clone, "README.md")); err != nil {
		t.Fatal(err)
	}
}
