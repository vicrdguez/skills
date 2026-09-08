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
	prepared  int
	ready     bool
}

func (b *recordingBackend) Validate(context.Context) (string, error) {
	b.validated++
	return "main", nil
}

func (b *recordingBackend) Prepare(context.Context) error {
	b.prepared++
	b.ready = true
	return nil
}

func TestSetupMaintainsOnlyOwnedAgentsBlock(t *testing.T) {
	root := newRepository(t)
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
			path := filepath.Join(root, "AGENTS.md")
			if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			backend := &recordingBackend{}

			_, err := setup.Run(context.Background(), setup.Request{Location: root}, backend)
			if err == nil || !strings.Contains(err.Error(), "malformed workflow markers") {
				t.Fatalf("error = %v", err)
			}
			if backend.validated != 0 || backend.prepared != 0 {
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
			if backend.validated != 0 || backend.prepared != 0 {
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

func TestSetupBoundPreparationRepeatsWithoutDrift(t *testing.T) {
	root := newRepository(t)
	for name, contents := range map[string]string{
		"AGENTS.md":  "Keep repository guidance.\n<!-- dev-pipeline:start -->\nold workflow\n<!-- dev-pipeline:end -->\nKeep local conventions.\n",
		"CLAUDE.md":  "Keep substantive harness guidance.\n",
		".gitignore": "dist/\n.worktrees/\n*.log\n.worktrees/\n",
		"README.md":  "Unrelated project documentation.\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backend := &recordingBackend{}
	request := setup.Request{Location: root, Confirm: func(string) (bool, error) {
		t.Fatal("must not offer to replace substantive CLAUDE.md")
		return false, nil
	}}
	for run := 1; run <= 2; run++ {
		outcome, err := setup.Run(context.Background(), request, backend)
		if err != nil {
			t.Fatal(err)
		}
		if want := (setup.Outcome{Root: root, TargetBranch: "main"}); outcome != want {
			t.Fatalf("outcome = %#v, want %#v", outcome, want)
		}
		if !backend.ready || backend.validated != run || backend.prepared != run {
			t.Fatalf("backend not prepared on run %d: %#v", run, backend)
		}
		for name, want := range map[string]string{
			"AGENTS.md":  "Keep repository guidance.\n" + setup.AgentsBlock + "Keep local conventions.\n",
			"CLAUDE.md":  "Keep substantive harness guidance.\n",
			".gitignore": "dist/\n*.log\n.worktrees/\n",
			"README.md":  "Unrelated project documentation.\n",
		} {
			if got := readFile(t, filepath.Join(root, name)); got != want {
				t.Fatalf("%s on run %d = %q, want %q", name, run, got, want)
			}
		}
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
