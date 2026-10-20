package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Debt: workflow fixtures and the bare remotes they push to still let Git
	// auto-maintenance detach after commits, fetches and pushes, so a repack
	// can outlive the command that triggered it and race TempDir cleanup.
	// Repository-local maintenance.autoDetach=false and gc.autoDetach=false
	// keep it in the foreground.
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	commitFile(t, root, "README.md", "initial\n")
	return root
}

func commitFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", name)
	runGit(t, root, "commit", "-m", "add "+name)
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(output[:len(output)-1])
}
