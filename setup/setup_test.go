package setup_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/setup"
)

type recordingBackend struct {
	validated int
	labels    int
}

func (b *recordingBackend) Validate(context.Context, setup.RepositoryID) (string, error) {
	b.validated++
	return "main", nil
}

func (b *recordingBackend) EnsureLabels(context.Context, setup.RepositoryID, []setup.Label) error {
	b.labels++
	return nil
}

func TestSetupRefusesAmbiguousGitHubRemoteBeforeMutation(t *testing.T) {
	root := newRepository(t)
	runGit(t, root, "remote", "add", "upstream", "https://github.com/acme/widgets.git")
	runGit(t, root, "remote", "add", "backup", "git@github.com:acme/widgets-backup.git")
	agentsPath := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &recordingBackend{}

	_, err := setup.Run(context.Background(), setup.Request{Location: root}, backend)
	if err == nil || !strings.Contains(err.Error(), "multiple GitHub remotes") {
		t.Fatalf("error = %v", err)
	}
	if backend.validated != 0 || backend.labels != 0 {
		t.Fatalf("backend mutated: %#v", backend)
	}
	if got := readFile(t, agentsPath); got != "keep me\n" {
		t.Fatalf("AGENTS.md = %q", got)
	}
}

func TestSetupMaintainsOnlyOwnedAgentsBlock(t *testing.T) {
	root := newRepository(t)
	runGit(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	agentsPath := filepath.Join(root, "AGENTS.md")
	original := "user before\n<!-- dev-pipeline:start -->\nold workflow\n<!-- dev-pipeline:end -->\nuser after\n"
	if err := os.WriteFile(agentsPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := setup.Run(context.Background(), setup.Request{Location: root}, &recordingBackend{}); err != nil {
		t.Fatal(err)
	}
	want := "user before\n" + setup.AgentsBlock + "user after\n"
	if got := readFile(t, agentsPath); got != want {
		t.Fatalf("AGENTS.md = %q, want %q", got, want)
	}
}

func newRepository(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init")
	return root
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
