package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/setup"
)

type memoryBackend struct {
	repository setup.RepositoryID
	labels     []setup.Label
}

func (b *memoryBackend) Validate(_ context.Context, repository setup.RepositoryID) (string, error) {
	b.repository = repository
	return "trunk", nil
}

func (b *memoryBackend) EnsureLabels(_ context.Context, _ setup.RepositoryID, labels []setup.Label) error {
	b.labels = append([]setup.Label(nil), labels...)
	return nil
}

func TestSetupInfersGitHubConsumerRepository(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "git@github.com:acme/widgets.git")
	nested := filepath.Join(root, "some", "nested", "directory")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	backend := &memoryBackend{}
	var stdout, stderr bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewBufferString("n\n"), &stdout, &stderr)
	if err := app.Run([]string{"skl", "setup"}); err != nil {
		t.Fatalf("setup failed: %v\nstderr: %s", err, stderr.String())
	}

	if backend.repository != (setup.RepositoryID{Owner: "acme", Name: "widgets"}) {
		t.Fatalf("repository = %#v", backend.repository)
	}
	if got := stdout.String(); got != "Link CLAUDE.md to AGENTS.md? [y/N] Prepared "+root+" for GitHub workflow on trunk.\n" {
		t.Fatalf("stdout = %q", got)
	}
	if len(backend.labels) != 6 {
		t.Fatalf("prepared %d labels", len(backend.labels))
	}
	if got := readFile(t, filepath.Join(root, ".gitignore")); got != ".worktrees/\n" {
		t.Fatalf(".gitignore = %q", got)
	}
	if got := readFile(t, filepath.Join(root, "AGENTS.md")); got != setup.AgentsBlock {
		t.Fatalf("AGENTS.md = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".skl.yml")); !os.IsNotExist(err) {
		t.Fatalf("workflow configuration was written: %v", err)
	}
}

func TestSetupDeclinesClaudeMigrationAtEndOfInput(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	backend := &memoryBackend{}
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "setup", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md created without confirmation: %v", err)
	}
}

func TestInstallSupportedSkillStubs(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)

	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}

	wantSkills := map[string]string{
		"audit":              "skills/dev/audit/SKILL.md",
		"brainstorm":         "skills/thinking/brainstorm/SKILL.md",
		"design":             "skills/dev/design/SKILL.md",
		"domain":             "skills/dev/domain/SKILL.md",
		"explore":            "skills/dev/explore/SKILL.md",
		"implement":          "skills/dev/implement/SKILL.md",
		"propose":            "skills/dev/propose/SKILL.md",
		"shape":              "skills/thinking/shape/SKILL.md",
		"tdd":                "skills/dev/tdd/SKILL.md",
		"watchdog":           "skills/dev/watchdog/SKILL.md",
		"writing-for-agents": "skills/misc/writing-for-agents/SKILL.md",
	}
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills"} {
		for name, source := range wantSkills {
			stub := readFile(t, filepath.Join(root, harness, name, "SKILL.md"))
			if !strings.HasPrefix(stub, "---\n") {
				t.Fatalf("%s %s stub has no leading YAML frontmatter:\n%s", harness, name, stub)
			}
			if want := skillFrontmatter(t, readRepositoryFile(t, source)); !strings.HasPrefix(stub, want+"\n") {
				t.Fatalf("%s %s stub changed source frontmatter:\n%s", harness, name, stub)
			}
			if !strings.Contains(stub, "skl skill "+name) || !strings.Contains(stub, "skl.stub/v1") {
				t.Fatalf("%s %s stub does not delegate to skl:\n%s", harness, name, stub)
			}
		}
		if _, err := os.Stat(filepath.Join(root, harness, "dev-setup", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("%s contains retired dev-setup stub: %v", harness, err)
		}
	}
	for _, manifest := range []string{".claude-plugin/plugin.json", ".codex-plugin/plugin.json", "package.json"} {
		if _, err := os.Stat(filepath.Join(root, manifest)); !os.IsNotExist(err) {
			t.Fatalf("plugin manifest written at %s: %v", manifest, err)
		}
	}
}

func TestInstallFailsWithoutUserHome(t *testing.T) {
	t.Setenv("HOME", "")
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "install"}); err == nil {
		t.Fatal("install succeeded without a user home")
	}
}

func TestInstallRefreshesOnlyOwnedStubs(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}

	tdd := filepath.Join(root, ".codex/skills/tdd/SKILL.md")
	wantTDD := readFile(t, tdd)
	if err := os.WriteFile(tdd, []byte("---\nname: tdd\n---\n\n<!-- skl-owned: skl.stub/v1 -->\nstale"), 0o644); err != nil {
		t.Fatal(err)
	}
	audit := filepath.Join(root, ".codex/skills/audit/SKILL.md")
	if err := os.WriteFile(audit, []byte("---\nname: audit\n---\n\n<!-- skl-owned: skl.stub/v2 -->\nmy unrelated skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, tdd); got != wantTDD {
		t.Fatalf("owned stub was not refreshed:\n%s", got)
	}
	if got := readFile(t, audit); got != "---\nname: audit\n---\n\n<!-- skl-owned: skl.stub/v2 -->\nmy unrelated skill" {
		t.Fatalf("unrelated file changed: %q", got)
	}
}

func TestRetrieveRenderedSkillInstructions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "tdd"}); err != nil {
		t.Fatal(err)
	}

	got := stdout.String()
	if !strings.Contains(got, "Protocol: skl.instructions/v1") || !strings.Contains(got, "# Test-Driven Development") {
		t.Fatalf("authoritative instructions absent:\n%s", got)
	}
	if strings.Contains(got, "# Good and Bad Tests") {
		t.Fatalf("unrequested resource included:\n%s", got)
	}
}

func TestRetrieveEquivalentTypedInstructions(t *testing.T) {
	var markdown bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &markdown, &markdown, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "tdd"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	app = newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "--format", "json", "tdd"}); err != nil {
		t.Fatal(err)
	}
	var packet skilldist.Packet
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatalf("stdout is not a JSON packet: %v\n%s", err, stdout.String())
	}
	expected, err := skilldist.BuildPacket("tdd", skilldist.InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Protocol != expected.Protocol || packet.Skill != expected.Skill || packet.Facts != expected.Facts || !slices.Equal(packet.IncludedSkills, expected.IncludedSkills) || !slices.Equal(packet.Resources, expected.Resources) || packet.Instructions != expected.Instructions || markdown.String() != expected.Markdown() {
		t.Fatalf("JSON and Markdown packets differ: %#v", packet)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRetrieveOneNamedResource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "--resource", "reference/tests.md", "tdd"}); err != nil {
		t.Fatal(err)
	}

	if got := stdout.String(); !strings.Contains(got, "# Good and Bad Tests") || strings.Contains(got, "# When to Mock") {
		t.Fatalf("stdout did not contain only the requested resource:\n%s", got)
	}
}

func TestBundleGuaranteedSupportingSkills(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, root)
	if err := app.Run([]string{"skl", "skill", "--format", "json", "implement"}); err != nil {
		t.Fatal(err)
	}
	var packet skilldist.Packet
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatal(err)
	}
	want := []string{"tdd", "audit", "design", "domain"}
	if !slices.Equal(packet.IncludedSkills, want) {
		t.Fatalf("included_skills = %v, want %v", packet.IncludedSkills, want)
	}
	for _, name := range want {
		if count := strings.Count(packet.Instructions, "## Included Skill: "+name); count != 1 {
			t.Fatalf("%s included %d times", name, count)
		}
	}

	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	stub := readFile(t, filepath.Join(root, ".codex/skills/implement/SKILL.md"))
	if !strings.Contains(stub, "included_skills") || !strings.Contains(stub, "Skip activation") {
		t.Fatalf("stub lacks included-skill guard:\n%s", stub)
	}
}

func TestIgnoreConsumerRepositoryOverrides(t *testing.T) {
	repository := t.TempDir()
	override := filepath.Join(repository, "skills/dev/tdd/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(override), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(override, []byte("consumer override"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repository); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "tdd"}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !strings.Contains(got, "# Test-Driven Development") || strings.Contains(got, "consumer override") {
		t.Fatalf("consumer repository overrode embedded definition:\n%s", got)
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

func readRepositoryFile(t *testing.T, path string) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository root")
	}
	return readFile(t, filepath.Join(filepath.Dir(testFile), "..", "..", path))
}

func skillFrontmatter(t *testing.T, source string) string {
	t.Helper()
	end := strings.Index(source[4:], "\n---\n")
	if !strings.HasPrefix(source, "---\n") || end < 0 {
		t.Fatal("skill source has invalid frontmatter")
	}
	return source[:end+8]
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
