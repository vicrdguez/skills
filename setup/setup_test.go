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

type statefulBackend struct{ labels map[string]setup.Label }

func (b *statefulBackend) Validate(context.Context, setup.RepositoryID) (string, error) {
	return "main", nil
}

func (b *statefulBackend) EnsureLabels(_ context.Context, _ setup.RepositoryID, labels []setup.Label) error {
	for _, label := range labels {
		b.labels[label.Name] = label
	}
	return nil
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

func TestSetupRefusesMalformedAgentsOwnershipMarkers(t *testing.T) {
	cases := map[string]string{
		"missing":   "<!-- dev-pipeline:start -->\n",
		"nested":    "<!-- dev-pipeline:start -->\n<!-- dev-pipeline:start -->\n<!-- dev-pipeline:end -->\n<!-- dev-pipeline:end -->\n",
		"reversed":  "<!-- dev-pipeline:end -->\n<!-- dev-pipeline:start -->\n",
		"duplicate": "<!-- dev-pipeline:start -->\n<!-- dev-pipeline:end -->\n<!-- dev-pipeline:start -->\n<!-- dev-pipeline:end -->\n",
	}
	for name, original := range cases {
		t.Run(name, func(t *testing.T) {
			root := newRepository(t)
			runGit(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
			path := filepath.Join(root, "AGENTS.md")
			if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			backend := &recordingBackend{}

			_, err := setup.Run(context.Background(), setup.Request{Location: root}, backend)
			if err == nil || !strings.Contains(err.Error(), "malformed workflow markers") {
				t.Fatalf("error = %v", err)
			}
			if backend.validated != 0 || backend.labels != 0 {
				t.Fatalf("backend mutated: %#v", backend)
			}
			if got := readFile(t, path); got != original {
				t.Fatalf("AGENTS.md = %q", got)
			}
		})
	}
}

func TestSetupRefusesOwnedFileSymlinksBeforeMutation(t *testing.T) {
	for _, name := range []string{"AGENTS.md", ".gitignore"} {
		t.Run(name, func(t *testing.T) {
			root := newRepository(t)
			runGit(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("keep me\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			backend := &recordingBackend{}

			_, err := setup.Run(context.Background(), setup.Request{Location: root}, backend)
			if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
				t.Fatalf("error = %v", err)
			}
			if backend.validated != 0 || backend.labels != 0 {
				t.Fatalf("backend mutated: %#v", backend)
			}
			if got := readFile(t, outside); got != "keep me\n" {
				t.Fatalf("outside file = %q", got)
			}
		})
	}
}

func TestSetupOffersSafeClaudeSymlinkMigration(t *testing.T) {
	for _, test := range []struct {
		name     string
		existing *string
		accept   bool
		linked   bool
		prompted bool
	}{
		{name: "absent", accept: true, linked: true, prompted: true},
		{name: "legacy import", existing: stringPointer("@AGENTS.md\n"), accept: true, linked: true, prompted: true},
		{name: "substantive guidance", existing: stringPointer("Keep this guidance.\n"), accept: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newRepository(t)
			runGit(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
			claudePath := filepath.Join(root, "CLAUDE.md")
			if test.existing != nil {
				if err := os.WriteFile(claudePath, []byte(*test.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			prompted := false
			_, err := setup.Run(context.Background(), setup.Request{
				Location: root,
				Confirm: func(string) (bool, error) {
					prompted = true
					return test.accept, nil
				},
			}, &recordingBackend{})
			if err != nil {
				t.Fatal(err)
			}
			if prompted != test.prompted {
				t.Fatalf("prompted = %v", prompted)
			}
			if test.linked {
				target, err := os.Readlink(claudePath)
				if err != nil || target != "AGENTS.md" {
					t.Fatalf("CLAUDE.md link = %q, %v", target, err)
				}
			} else if got := readFile(t, claudePath); got != *test.existing {
				t.Fatalf("CLAUDE.md = %q", got)
			}
		})
	}
}

func TestSetupRetiresLegacySetupArtifacts(t *testing.T) {
	root := newRepository(t)
	runGit(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	gitignore := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("dist/\n.worktrees/\n*.log\n.worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docs := filepath.Join(root, "docs")
	if err := os.Mkdir(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(docs, "github.md")
	if err := os.WriteFile(legacy, []byte("legacy workflow\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := setup.Run(context.Background(), setup.Request{Location: root}, &recordingBackend{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, gitignore); got != "dist/\n*.log\n.worktrees/\n" {
		t.Fatalf(".gitignore = %q", got)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("docs/github.md still exists: %v", err)
	}
}

func TestSetupRepeatsWithoutDrift(t *testing.T) {
	root := newRepository(t)
	runGit(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	backend := &statefulBackend{labels: map[string]setup.Label{
		"ready":  {Name: "ready", Color: "ffffff", Description: "stale"},
		"custom": {Name: "custom", Color: "123456", Description: "unrelated"},
	}}
	confirm := func(string) (bool, error) { return true, nil }
	request := setup.Request{Location: root, Confirm: confirm}

	if _, err := setup.Run(context.Background(), request, backend); err != nil {
		t.Fatal(err)
	}
	agents := readFile(t, filepath.Join(root, "AGENTS.md"))
	gitignore := readFile(t, filepath.Join(root, ".gitignore"))
	if _, err := setup.Run(context.Background(), request, backend); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, "AGENTS.md")); got != agents {
		t.Fatalf("AGENTS.md drifted: %q", got)
	}
	if got := readFile(t, filepath.Join(root, ".gitignore")); got != gitignore {
		t.Fatalf(".gitignore drifted: %q", got)
	}
	for _, want := range setup.WorkflowLabels {
		if got := backend.labels[want.Name]; got != want {
			t.Fatalf("label %q = %#v", want.Name, got)
		}
	}
	if got := backend.labels["custom"]; got.Description != "unrelated" {
		t.Fatalf("unrelated label changed: %#v", got)
	}
}

func stringPointer(value string) *string { return &value }

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
