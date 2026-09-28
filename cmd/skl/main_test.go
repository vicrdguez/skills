package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// structuredStageCommand preserves the legacy typed-test seam for both stages:
// Watchdog is JSON by default, while Implement now opts in explicitly.
func structuredStageCommand(args ...string) []string {
	command := append([]string{"skl"}, args...)
	if len(args) > 0 && args[0] == "implement" {
		command = append(command, "--format", "json")
	}
	return command
}

type memoryBackend struct {
	repository github.RepositoryID
	prepared   bool
}

func (b *memoryBackend) Validate(context.Context) (string, error) {
	return "trunk", nil
}

func (b *memoryBackend) Prepare(context.Context) error {
	b.prepared = true
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
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend.repository = repository
		return backend, nil
	}, bytes.NewBufferString("n\n"), &stdout, &stderr)
	if err := app.Run([]string{"skl", "setup"}); err != nil {
		t.Fatalf("setup failed: %v\nstderr: %s", err, stderr.String())
	}

	if backend.repository != (github.RepositoryID{Owner: "acme", Name: "widgets"}) {
		t.Fatalf("repository = %#v", backend.repository)
	}
	if got := stdout.String(); got != "Link CLAUDE.md to AGENTS.md? [y/N] Prepared "+root+" for GitHub workflow on trunk.\n" {
		t.Fatalf("stdout = %q", got)
	}
	if !backend.prepared {
		t.Fatal("workflow backend was not prepared")
	}
	if got := readFile(t, filepath.Join(root, ".gitignore")); got != ".worktrees/\n" {
		t.Fatalf(".gitignore = %q", got)
	}
	agentsPath := filepath.Join(root, "AGENTS.md")
	if installed := readFile(t, agentsPath); installed != setup.AgentsBlock {
		t.Fatalf("fresh AGENTS.md = %q, want %q", installed, setup.AgentsBlock)
	}
	t.Run("refresh and repeat", func(t *testing.T) {
		before, after := "# User guidance\r\nKeep this spacing.  \n\n", "\nUser footer\twithout final newline"
		original := before + "<!-- dev-pipeline:start -->\nstale guidance\n<!-- dev-pipeline:end -->\n" + after
		if err := os.WriteFile(agentsPath, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		want := before + setup.AgentsBlock + after
		for range 2 {
			stdout.Reset()
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewBufferString("n\n"), &stdout, &stderr)
			if err := app.Run([]string{"skl", "setup", "--repo", root}); err != nil {
				t.Fatal(err)
			}
			if got := stdout.String(); got != "Link CLAUDE.md to AGENTS.md? [y/N] Prepared "+root+" for GitHub workflow on trunk.\n" {
				t.Errorf("stdout = %q", got)
			}
			got := readFile(t, agentsPath)
			if got != want {
				t.Fatalf("AGENTS.md = %q, want %q", got, want)
			}
			want = got
		}
	})
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
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "setup", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md created without confirmation: %v", err)
	}
}

func TestSetupResolvesRepository(t *testing.T) {
	for _, layout := range []string{"origin", "sole upstream", "non-GitHub origin", "ambiguous", "explicit upstream"} {
		t.Run(layout, func(t *testing.T) {
			root := proposalRepository(t)
			if layout == "origin" {
				runGit(t, root, "remote", "add", "upstream", "https://github.com/other/widgets.git")
			} else {
				runGit(t, root, "remote", "rename", "origin", "upstream")
			}
			switch layout {
			case "non-GitHub origin":
				runGit(t, root, "remote", "add", "origin", "https://example.com/other/widgets.git")
			case "ambiguous":
				runGit(t, root, "remote", "add", "fork", "https://github.com/other/widgets.git")
			case "explicit upstream":
				runGit(t, root, "remote", "add", "origin", "https://github.com/other/widgets.git")
			}
			args := []string{"skl", "setup", "--repo", root}
			if layout == "explicit upstream" {
				args = append(args, "--remote", "upstream")
			}
			backend := &memoryBackend{}
			var output bytes.Buffer
			app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
				backend.repository = repository
				return backend, nil
			}, bytes.NewReader(nil), &output, &output)
			err := app.Run(args)
			if layout == "ambiguous" {
				if err == nil || !strings.Contains(err.Error(), "--remote") || output.Len() != 0 {
					t.Fatalf("ambiguous selection = %v, output=%q", err, &output)
				}
				if backend.prepared {
					t.Fatalf("ambiguous selection mutated backend: %#v", backend)
				}
				if got := runGitOutput(t, root, "status", "--porcelain", "--untracked-files=all"); got != "" {
					t.Fatalf("ambiguous selection changed repository files: %s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if backend.repository != (github.RepositoryID{Owner: "acme", Name: "widgets"}) {
				t.Fatalf("bound repository = %#v", backend.repository)
			}
			if !backend.prepared || !strings.Contains(output.String(), "for GitHub workflow on trunk.") || !strings.Contains(readFile(t, filepath.Join(root, "AGENTS.md")), "## Workflow") {
				t.Fatalf("setup = %q, backend=%#v", &output, backend)
			}
		})
	}
}

func TestProposePublishIsNotACommand(t *testing.T) {
	app := newApp(nil, bytes.NewReader(nil), io.Discard, io.Discard)
	if app.Command("propose").Command("publish") != nil {
		t.Fatal("propose publish is still registered")
	}
	binary := filepath.Join(t.TempDir(), "skl")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build skl: %v\n%s", err, output)
	}
	root := proposalRepository(t)
	command := exec.Command(binary, "propose", "publish", "--repo", root, "--target", "main", "--slice", "widget=issue.md")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "No help topic for 'publish'") {
		t.Fatalf("retired invocation = %v, %q", err, output)
	}
	if got := runGitOutput(t, root, "status", "--porcelain", "--untracked-files=all"); got != "" {
		t.Fatalf("retired invocation changed repository files: %s", got)
	}
}

type httpRoundTripFunc func(*http.Request) (*http.Response, error)

func (function httpRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

// implementBundle renders the complete Implement Execution Skill through the
// active private-ledger path, so checks never depend on the retired
// forge-authoritative selection or on refused read-only retrieval.
func implementBundle(t *testing.T) skilldist.Packet {
	t.Helper()
	newLedgerFixture(t)
	forge := newForgeServer(t)
	source := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, forge)
	accepted := cli.accept(t, source, writeProposal(t, "", singleSlice("widget-proposal")))
	if accepted.Status != "accepted" {
		t.Fatalf("proposal acceptance = %q: %s", accepted.Status, mustJSON(t, accepted))
	}
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "implement", "next", "--repo", source, "--result-directory", t.TempDir(), "--format", "json"}); err != nil {
		t.Fatalf("implement next: %v\n%s", err, cli.out.String())
	}
	var got deliveryOutput
	if err := json.Unmarshal(cli.out.Bytes(), &got); err != nil {
		t.Fatalf("decode implement next %q: %v", cli.out.String(), err)
	}
	if got.Packet == nil {
		t.Fatalf("no Execution Skill: %#v", got)
	}
	return *got.Packet
}

// activeDeliveryPacket renders one private-ledger delivery packet through the
// same production presentation seam the CLI uses. Procedure-specific coverage
// uses it because the complete CLI lifecycle is already covered end to end in
// delivery_test.go.
func activeDeliveryPacket(t *testing.T, root, phase, operation string, state ledger.SliceState) skilldist.Packet {
	t.Helper()
	repository, err := setup.ResolveRepository(root, "")
	if err != nil {
		t.Fatal(err)
	}
	execution := &ledger.Execution{
		Project:    "widgets",
		Repository: "acme/widgets",
		Item:       "widget/foundation",
		State:      state,
		Claim:      ledger.Reference{Commit: strings.Repeat("c", 40), Path: "projects/widgets/proposals/widget/foundation/state.json"},
		Documents: []ledger.ContractDocument{{
			Commit:   strings.Repeat("1", 40),
			Path:     "projects/widgets/proposals/widget/foundation/behavior.md",
			Contents: "# Opaque accepted contract\n",
		}},
	}
	if phase == ledger.WatchdogPhase {
		execution.Implement = &ledger.Report{Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40)}}
		execution.Watchdog = &ledger.Report{Round: 1, Source: ledger.SourceRevisions{Reviewed: strings.Repeat("a", 40)}}
	}
	packet, err := setup.PresentDelivery(execution, repository, phase, operation, nil, t.TempDir(), setup.ImplementChoices{})
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

// TestDocumentedResourceCommands runs every `skl skill` command a rendering
// emits, and the resource commands in the CLI reference. The goldens under
// testdata/prose hold every rendering a worker receives.
func TestDocumentedResourceCommands(t *testing.T) {
	goldens, err := filepath.Glob(filepath.Join(proseDirectory(), "*.md"))
	if err != nil || len(goldens) == 0 {
		t.Fatalf("no prose goldens: %v", err)
	}
	for _, golden := range goldens {
		t.Run(filepath.Base(golden), func(t *testing.T) {
			for _, command := range skillCommands(readFile(t, golden)) {
				runSkillCommand(t, command)
			}
		})
	}

	// The README links to docs/cli.md for the full command reference.
	// Execute its concrete resource examples without requiring them to appear
	// in the README overview as well.
	seen := map[string]bool{}
	for _, command := range skillCommands(readRepositoryFile(t, "docs/cli.md")) {
		if !strings.Contains(command, "--resource") {
			continue
		}
		args := runSkillCommand(t, command)
		if index := slices.Index(args, "--resource"); index >= 0 {
			seen[args[index+1]] = true
		}
	}
	for _, want := range []string{"ledger-submission.md", "ledger-review.md", "DEEPENING.md"} {
		if !seen[want] {
			t.Errorf("docs/cli.md lacks a documented command for %s: %v", want, seen)
		}
	}
}

// skillCommands returns the backticked `skl skill` commands in source that
// carry no placeholder.
func skillCommands(source string) []string {
	var commands []string
	for _, chunk := range strings.Split(source, "`") {
		for _, line := range strings.Split(chunk, "\n") {
			command := strings.TrimSpace(line)
			if strings.HasPrefix(command, "skl skill ") && !strings.Contains(command, "<") {
				commands = append(commands, command)
			}
		}
	}
	return commands
}

// runSkillCommand runs one command as printed, without a Workflow Backend, and
// returns its resolved arguments.
func runSkillCommand(t *testing.T, command string) []string {
	t.Helper()
	args := shellArgs(t, command)
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatalf("%s reached the Workflow Backend", command)
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run(args); err != nil {
		t.Errorf("%s: %v", command, err)
	}
	return args
}

// shellArgs resolves a documented command the way a shell would, so quoted
// values survive as the single arguments they name.
func shellArgs(t *testing.T, command string) []string {
	t.Helper()
	output, err := exec.Command("sh", "-c", "printf '%s\\000' "+command).Output()
	if err != nil {
		t.Fatalf("cannot resolve %q: %v", command, err)
	}
	return strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
}

func TestInstallSupportedSkillStubs(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)

	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}

	wantSkills := []string{"audit", "brainstorm", "decision", "design", "domain", "explore", "propose", "shape", "testing", "writing-for-agents"}
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		for _, name := range wantSkills {
			path := filepath.Join(root, harness, name, "SKILL.md")
			stub := readFile(t, path)
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" {
				t.Fatalf("%s must contain only a stub: %v: %v", filepath.Dir(path), entries, err)
			}
			if harness == ".config/opencode/skills" {
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("OpenCode stub is not a regular file: %s: %v", path, err)
				}
				for _, other := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills"} {
					otherPath := filepath.Join(root, other, name, "SKILL.md")
					otherInfo, err := os.Stat(otherPath)
					if err != nil || os.SameFile(info, otherInfo) || stub != readFile(t, otherPath) {
						t.Fatalf("OpenCode stub is not an independent common stub: %s: %v", path, err)
					}
				}
			}
		}
		if _, err := os.Stat(filepath.Join(root, harness, "dev-setup", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("%s contains retired dev-setup stub: %v", harness, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, ".config/opencode"))
	if err != nil || len(entries) != 2 || entries[0].Name() != "commands" || entries[1].Name() != "skills" || !entries[0].IsDir() || !entries[1].IsDir() {
		t.Fatalf("unexpected OpenCode configuration or adapters: %v: %v", entries, err)
	}
	entries, err = os.ReadDir(filepath.Join(root, ".config/opencode/skills"))
	if err != nil || len(entries) != len(wantSkills) {
		t.Fatalf("unexpected OpenCode catalog: %v: %v", entries, err)
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
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "install"}); err == nil {
		t.Fatal("install succeeded without a user home")
	}
}

func TestInstallRefreshesOnlyOwnedStubs(t *testing.T) {
	for _, harness := range []string{".codex/skills", ".config/opencode/skills"} {
		t.Run(harness, func(t *testing.T) {
			root := t.TempDir()
			var output bytes.Buffer
			app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}

			testing := filepath.Join(root, harness, "testing/SKILL.md")
			wantTesting := readFile(t, testing)
			if err := os.WriteFile(testing, []byte("---\nname: testing\n---\n\n<!-- skl-owned: skl.stub/v1 -->\nstale"), 0o644); err != nil {
				t.Fatal(err)
			}
			audit := filepath.Join(root, harness, "audit/SKILL.md")
			if err := os.WriteFile(audit, []byte("---\nname: audit\n---\n\n<!-- skl-owned: skl.stub/v2 -->\nmy unrelated skill"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}

			if got := readFile(t, testing); got != wantTesting {
				t.Fatalf("owned stub was not refreshed:\n%s", got)
			}
			if got := readFile(t, audit); got != "---\nname: audit\n---\n\n<!-- skl-owned: skl.stub/v2 -->\nmy unrelated skill" {
				t.Fatalf("unrelated file changed: %q", got)
			}
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, testing); got != wantTesting {
				t.Fatalf("repeat installation changed current stub:\n%s", got)
			}
			if harness == ".config/opencode/skills" {
				entries, err := os.ReadDir(filepath.Join(root, ".config/opencode"))
				if err != nil || len(entries) != 2 || entries[0].Name() != "commands" || entries[1].Name() != "skills" || !entries[0].IsDir() || !entries[1].IsDir() {
					t.Fatalf("reinstallation wrote OpenCode configuration: %v: %v", entries, err)
				}
			}
		})
	}
}

func TestInstallUpgradesOwnedTDDStubsWithoutTouchingUserContent(t *testing.T) {
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		t.Run(harness, func(t *testing.T) {
			root := t.TempDir()
			legacyDirectory := filepath.Join(root, harness, "tdd")
			if err := os.MkdirAll(legacyDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(legacyDirectory, "SKILL.md")
			if err := os.WriteFile(legacy, []byte("---\nname: tdd\n---\n\n<!-- skl-owned: skl.stub/v1 -->\nstale\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			extra := filepath.Join(legacyDirectory, "notes.md")
			if err := os.WriteFile(extra, []byte("keep legacy notes\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			userOwned := filepath.Join(root, harness, "my-tdd", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(userOwned), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(userOwned, []byte("my unrelated test skill\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			var output bytes.Buffer
			app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
			for range 2 {
				if err := app.Run([]string{"skl", "install"}); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(legacy); !os.IsNotExist(err) {
					t.Fatalf("owned legacy tdd stub still exists: %v", err)
				}
				if got := readFile(t, extra); got != "keep legacy notes\n" {
					t.Fatalf("legacy sibling changed: %q", got)
				}
				if got := readFile(t, userOwned); got != "my unrelated test skill\n" {
					t.Fatalf("user-owned skill changed: %q", got)
				}
				testing := readFile(t, filepath.Join(root, harness, "testing", "SKILL.md"))
				if !strings.Contains(testing, "skl skill testing") || !strings.Contains(testing, "skl.stub/v1") {
					t.Fatalf("testing stub is not current and owned:\n%s", testing)
				}
			}
		})
	}

	t.Run("user-owned tdd", func(t *testing.T) {
		root := t.TempDir()
		legacy := filepath.Join(root, ".codex/skills/tdd/SKILL.md")
		if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
			t.Fatal(err)
		}
		const userSkill = "---\nname: tdd\n---\nmy own tdd skill\n"
		if err := os.WriteFile(legacy, []byte(userSkill), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
		for range 2 {
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, legacy); got != userSkill {
				t.Fatalf("user-owned tdd changed: %q", got)
			}
		}
	})
}

func TestInstallPreservesOpenCodeSkillsAndConfiguration(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"skills/audit/SKILL.md":    "---\nname: audit\n---\nMy own audit skill\n",
		"skills/personal/SKILL.md": "My unrelated skill\n",
		"opencode.json":            `{"skills":{"paths":["~/.pi/agent/skills","/my/other/skills"]},"theme":"system"}`,
	}
	for file, contents := range files {
		path := filepath.Join(root, ".config/opencode", file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
	for range 2 {
		if err := app.Run([]string{"skl", "install"}); err != nil {
			t.Fatal(err)
		}
		for file, want := range files {
			if got := readFile(t, filepath.Join(root, ".config/opencode", file)); got != want {
				t.Fatalf("user file %s changed: %q", file, got)
			}
		}
		for _, name := range skilldist.SkillNames() {
			if name == "audit" || name == "implement" || name == "watchdog" {
				continue
			}
			got := readFile(t, filepath.Join(root, ".config/opencode/skills", name, "SKILL.md"))
			if want := readFile(t, filepath.Join(root, ".codex/skills", name, "SKILL.md")); got != want {
				t.Fatalf("missing nonconflicting common stub for %s: %q", name, got)
			}
		}
	}
}

func TestRetiredTDDSkillIsUnknown(t *testing.T) {
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("skill retrieval opened a Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "tdd"}); err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("retired tdd retrieval error = %v, output = %q", err, output.String())
	}
}

func TestRetrieveApprovedContractGuidance(t *testing.T) {
	run := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) {
			t.Fatal("public guidance retrieval reached the Workflow Backend")
			return nil, nil
		}, bytes.NewReader(nil), &stdout, &stderr)
		if err := app.Run(args); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v stderr: %s", args, stderr.String())
		}
		return stdout.String()
	}

	for _, tc := range []struct {
		name      string
		included  []string
		resources []string
	}{
		{name: "explore", included: []string{"domain"}},
		{
			name:      "propose",
			resources: []string{"behavior.md", "intent.md", "issue-publication.md", "plan.md", "tasks.md"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			markdown := run("skl", "skill", tc.name)
			typed := run("skl", "skill", "--format", "json", tc.name)
			var packet skilldist.Packet
			if err := json.Unmarshal([]byte(typed), &packet); err != nil {
				t.Fatalf("typed retrieval is not JSON: %v\n%s", err, typed)
			}
			if markdown != packet.Instructions {
				t.Fatalf("Markdown and explicit JSON differ for %s", tc.name)
			}
			if !slices.Equal(packet.IncludedSkills, tc.included) || !slices.Equal(packet.Resources, tc.resources) {
				t.Fatalf("%s manifest = included %v resources %v", tc.name, packet.IncludedSkills, packet.Resources)
			}
		})
	}

}

func TestRetrieveEquivalentTypedInstructions(t *testing.T) {
	wantResources := []string{"mocking.md", "tests.md"}
	var markdown bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &markdown, &markdown, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	app = newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "--format", "json", "testing"}); err != nil {
		t.Fatal(err)
	}
	var packet skilldist.Packet
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatalf("stdout is not a JSON packet: %v\n%s", err, stdout.String())
	}
	if packet.Protocol != "skl.instructions/v1" || packet.Skill != "testing" || packet.Facts != (skilldist.InvocationFacts{}) || len(packet.IncludedSkills) != 0 || !slices.Equal(packet.Resources, wantResources) || packet.Instructions == "" || markdown.String() != packet.Instructions {
		t.Fatalf("JSON and Markdown packets differ: %#v", packet)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRetrieveAuditManifest(t *testing.T) {
	var markdown, typed bytes.Buffer
	if err := newApp(nil, bytes.NewReader(nil), &markdown, &markdown).Run([]string{"skl", "skill", "audit"}); err != nil {
		t.Fatal(err)
	}
	if err := newApp(nil, bytes.NewReader(nil), &typed, &typed).Run([]string{"skl", "skill", "--format", "json", "audit"}); err != nil {
		t.Fatal(err)
	}
	var packet skilldist.Packet
	if err := json.Unmarshal(typed.Bytes(), &packet); err != nil {
		t.Fatal(err)
	}
	if len(packet.IncludedSkills) != 0 || !slices.Equal(packet.Resources, []string{"acceptance.md", "smells.md"}) {
		t.Fatalf("unexpected Audit dependencies: %#v", packet)
	}
}

// renderResource runs one named-resource request through the in-process CLI and
// returns its rendered instructions. Any Workflow Backend access is a defect.
func renderResource(t *testing.T, owner, resource string, inputs ...string) string {
	t.Helper()
	command := []string{"skl", "skill", "--resource", resource}
	for _, input := range inputs {
		command = append(command, "--input", input)
	}
	command = append(command, owner)
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("resource rendering reached the Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run(command); err != nil {
		t.Fatalf("%v: %v", command, err)
	}
	return output.String()
}

func TestRejectInvalidResourceInputs(t *testing.T) {
	directory := t.TempDir()
	head := strings.Repeat("a", 40)
	cases := []struct {
		name     string
		owner    string
		resource string
		inputs   []string
		describe bool
		wants    []string
	}{
		{
			name: "input without separator", owner: "implement", resource: "ledger-submission.md",
			inputs: []string{"result_directory"},
			wants:  []string{"result_directory", "name=value"},
		},
		{
			name: "undeclared input", owner: "implement", resource: "ledger-submission.md",
			inputs: []string{"result_directory=" + directory, "procedure=initial", "findings=1"},
			wants:  []string{"findings", "--describe-inputs"},
		},
		{
			name: "duplicate input", owner: "watchdog", resource: "ledger-review.md",
			inputs: []string{"result_directory=" + directory, "round=1", "round=1", "reviewed_head=" + head},
			wants:  []string{"round", "duplicate"},
		},
		{
			name: "missing required input", owner: "implement", resource: "ledger-submission.md",
			inputs: []string{"result_directory=" + directory},
			wants:  []string{"procedure", "required"},
		},
		{
			name: "invalid integer", owner: "watchdog", resource: "ledger-review.md",
			inputs: []string{"result_directory=" + directory, "round=two", "reviewed_head=" + head},
			wants:  []string{"round", "integer"},
		},
		{
			name: "unsupported choice", owner: "implement", resource: "ledger-submission.md",
			inputs: []string{"result_directory=" + directory, "procedure=later"},
			wants:  []string{"procedure", "initial", "rework"},
		},
		{
			name: "zero round", owner: "watchdog", resource: "ledger-review.md",
			inputs: []string{"result_directory=" + directory, "round=0", "reviewed_head=" + head},
			wants:  []string{"round", "positive"},
		},
		{
			name: "empty result directory", owner: "implement", resource: "ledger-submission.md",
			inputs: []string{"result_directory=", "procedure=initial"},
			wants:  []string{"result_directory", "absolute"},
		},
		{
			name: "relative result directory", owner: "implement", resource: "ledger-submission.md",
			inputs: []string{"result_directory=.worktrees/result", "procedure=initial"},
			wants:  []string{"result_directory", "absolute"},
		},
		{
			name: "malformed reviewed head", owner: "watchdog", resource: "ledger-review.md",
			inputs: []string{"result_directory=" + directory, "round=1", "reviewed_head=" + head[:12] + "nonsense"},
			wants:  []string{"reviewed source SHA", "40 lowercase hexadecimal"},
		},
		{
			name: "input without resource", owner: "implement",
			inputs: []string{"result_directory=" + directory},
			wants:  []string{"--input", "--resource"},
		},
		{
			name: "describe without resource", owner: "implement", describe: true,
			wants: []string{"--describe-inputs", "--resource"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Fatal("invalid resource input reached the Workflow Backend")
				return nil, nil
			}, bytes.NewReader(nil), &output, &output)
			command := []string{"skl", "skill"}
			if testCase.resource != "" {
				command = append(command, "--resource", testCase.resource)
			}
			if testCase.describe {
				command = append(command, "--describe-inputs")
			}
			for _, input := range testCase.inputs {
				command = append(command, "--input", input)
			}
			command = append(command, testCase.owner)
			err := app.Run(command)
			if err == nil {
				t.Fatalf("%v returned no error:\n%s", command, output.String())
			}
			for _, want := range testCase.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%v error %q does not identify %q", command, err, want)
				}
			}
			if output.Len() != 0 {
				t.Errorf("%v returned procedural content: %q", command, output.String())
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Errorf("invalid input changed the Result Document directory: %v, %v", entries, err)
			}
		})
	}
}

func TestPreserveLiteralResourceInputValues(t *testing.T) {
	// Only an absolute private directory is a valid value for this input, so the
	// stress value is absolute while still carrying commas, quotes, embedded
	// equals, literal template syntax, and a trailing space.
	first := `/tmp/one, two=three 'four' "five" {{.ResultDirectory}} `
	second := `/tmp/{{template "result-document" .}}/other`
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("resource rendering reached the Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	render := func(inputs ...string) (string, error) {
		output.Reset()
		command := []string{"skl", "skill", "--resource", "ledger-submission.md"}
		for _, input := range inputs {
			command = append(command, "--input", input)
		}
		err := app.Run(append(command, "implement"))
		return output.String(), err
	}

	rendered, err := render("result_directory="+first, "procedure=initial")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, first+"/implement-report.md") {
		t.Errorf("rendering did not preserve every supplied character:\n%s", rendered)
	}

	rendered, err = render("result_directory="+second, "procedure=rework")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, second+"/implement-report.md") || strings.Contains(rendered, "one, two=three") {
		t.Errorf("a rendering reused an earlier call's inputs:\n%s", rendered)
	}

	if _, err := render("result_directory=" + second); err == nil || !strings.Contains(err.Error(), "procedure") {
		t.Fatalf("omitted required input was reused instead of rejected: %v", err)
	}
}

func TestDeferredResourceCommandsFromParentInstructions(t *testing.T) {
	// The engine creates the private Result Document directory itself, so a
	// hostile TMPDIR is how a real invocation binds values containing spaces,
	// commas, embedded equals, and shell quotes.
	hostile := filepath.Join(t.TempDir(), `res,ult 'dir' "quoted" =a=`)
	if err := os.MkdirAll(hostile, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", hostile)
	root := sourceRepository(t, "acme", "widgets")

	cases := []struct {
		name      string
		phase     string
		operation string
		state     ledger.SliceState
		owner     string
		resource  string
		procedure string
	}{
		{
			name: "first implementation", phase: ledger.ImplementPhase, operation: "next", owner: "implement",
			state:    ledger.SliceState{State: ledger.ReadyForImplementation, Title: "Foundation", Branch: "foundation"},
			resource: "ledger-submission.md", procedure: "initial",
		},
		{
			// The engine's operation and reconciled Workflow State select the
			// procedure; the bound report resource receives it as typed input.
			name: "resumed implementation", phase: ledger.ImplementPhase, operation: "resume", owner: "implement",
			state:    ledger.SliceState{State: ledger.ReadyForImplementation, Title: "Foundation", Branch: "foundation"},
			resource: "ledger-submission.md", procedure: "resumed",
		},
		{
			name: "finding-driven rework", phase: ledger.ImplementPhase, operation: "next", owner: "implement",
			state:    ledger.SliceState{State: ledger.Rework, Title: "Foundation", Branch: "foundation"},
			resource: "ledger-submission.md", procedure: "rework",
		},
		{
			name: "watchdog invocation", phase: ledger.WatchdogPhase, operation: "next", owner: "watchdog",
			state:    ledger.SliceState{State: ledger.AwaitingReview, Title: "Foundation", Branch: "foundation"},
			resource: "ledger-review.md",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			packet := activeDeliveryPacket(t, root, testCase.phase, testCase.operation, testCase.state)
			facts := packet.Facts.Delivery
			if facts == nil {
				t.Fatalf("no delivery facts: %#v", packet.Facts)
			}
			if !strings.HasPrefix(facts.ResultDirectory, hostile) {
				t.Fatalf("result directory %q is not the invocation's own private directory", facts.ResultDirectory)
			}
			command := deferredCommand(t, packet.Instructions, testCase.resource)
			if !strings.Contains(command, "--input result_directory=") || !strings.Contains(command, "result_directory="+skilldist.ShellQuote(facts.ResultDirectory)) {
				t.Errorf("parent command does not bind the private directory: %s", command)
			}
			if testCase.procedure != "" && !strings.Contains(command, "--input procedure="+testCase.procedure) {
				t.Errorf("parent command does not bind procedure %q: %s", testCase.procedure, command)
			}
			if !strings.Contains(packet.Instructions, "`"+facts.PauseCommand+"`") {
				t.Errorf("packet lost the engine-bound human-decision path %q", facts.PauseCommand)
			}
			if testCase.phase == ledger.WatchdogPhase {
				for _, want := range []string{
					"result_directory=" + skilldist.ShellQuote(facts.ResultDirectory),
					"--input round=" + strconv.FormatUint(facts.ReviewNumber, 10),
					"reviewed_head=" + skilldist.ShellQuote(facts.RequiredHead),
				} {
					if !strings.Contains(command, want) {
						t.Errorf("parent command does not bind %q: %s", want, command)
					}
				}
				for _, settled := range []string{"<", ">", "--verdict"} {
					if strings.Contains(command, settled) {
						t.Errorf("parent command includes %q instead of a settled value: %s", settled, command)
					}
				}
			}

			args := shellArgs(t, command)
			if len(args) == 0 || args[0] != "skl" || args[len(args)-1] != testCase.owner {
				t.Fatalf("parent command is not runnable verbatim: %v", args)
			}
			instructions := runDeferredCommand(t, command, "", "")
			// The golden journey renders only the initial report resource, so the
			// resumed and rework branches keep one marker each.
			if testCase.resource == "ledger-submission.md" {
				for procedure, marker := range map[string]string{
					"resumed": "What was already done",
					"rework":  "Keep every earlier",
				} {
					if strings.Contains(instructions, marker) != (procedure == testCase.procedure) {
						t.Errorf("%s resource and the %s marker %q disagree:\n%s", testCase.procedure, procedure, marker, instructions)
					}
				}
			}
			if !strings.Contains(instructions, "`"+facts.ResultDirectory+"/") {
				t.Errorf("retrieved resource did not bind the invocation's private directory:\n%s", instructions)
			}
		})
	}
}

// runDeferredCommand resolves one emitted resource command the way a shell
// would, fills only its documented later value, and returns the retrieved
// resource.
func runDeferredCommand(t *testing.T, command, placeholder, value string) string {
	t.Helper()
	if placeholder != "" {
		command = strings.Replace(command, placeholder, value, 1)
	}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("resource rendering reached the Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run(shellArgs(t, command)); err != nil {
		t.Fatalf("%s: %v", command, err)
	}
	return output.String()
}

// deferredCommand returns the single deferred resource command a parent's
// instructions publish for one resource.
func deferredCommand(t *testing.T, instructions, resource string) string {
	t.Helper()
	var commands []string
	for _, chunk := range strings.Split(instructions, "`") {
		if strings.HasPrefix(chunk, "skl skill --resource "+resource+" ") {
			commands = append(commands, chunk)
		}
	}
	if len(commands) != 1 {
		t.Fatalf("want exactly one %s command in the parent instructions, got %v", resource, commands)
	}
	return commands[0]
}

func TestContextFreeResourcesRetainOwnershipAndDefaults(t *testing.T) {
	cases := []struct{ owner, resource string }{
		{"audit", "acceptance.md"},
		{"audit", "smells.md"},
		{"design", "DEEPENING.md"},
		{"design", "DESIGN-IT-TWICE.md"},
		{"domain", "ADR-FORMAT.md"},
		{"domain", "CONTEXT-FORMAT.md"},
		{"propose", "intent.md"},
		{"propose", "behavior.md"},
		{"propose", "plan.md"},
		{"propose", "tasks.md"},
		{"testing", "mocking.md"},
		{"testing", "tests.md"},
		{"writing-for-agents", "SKILL-MECHANICS.md"},
	}
	for _, testCase := range cases {
		t.Run(testCase.owner+"/"+testCase.resource, func(t *testing.T) {
			resource := renderResource(t, testCase.owner, testCase.resource)
			if strings.Contains(resource, "{{") {
				t.Errorf("context-free %s leaked an unrendered template action:\n%s", testCase.resource, resource)
			}
			for _, other := range []string{"audit", "implement", "testing", "watchdog"} {
				if other == testCase.owner {
					continue
				}
				var output bytes.Buffer
				app := newApp(nil, bytes.NewReader(nil), &output, &output)
				if err := app.Run([]string{"skl", "skill", "--resource", testCase.resource, other}); err == nil || !strings.Contains(err.Error(), "unknown resource") {
					t.Errorf("%s became a resource of %s: %v", testCase.resource, other, err)
				}
			}
		})
	}

	var markdown, jsonPacket bytes.Buffer
	if err := newApp(nil, bytes.NewReader(nil), &markdown, &markdown).Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}
	if err := newApp(nil, bytes.NewReader(nil), &jsonPacket, &jsonPacket).Run([]string{"skl", "skill", "--format", "json", "testing"}); err != nil {
		t.Fatal(err)
	}
	var decoded skilldist.Packet
	if err := json.Unmarshal(jsonPacket.Bytes(), &decoded); err != nil || decoded.Skill != "testing" {
		t.Fatalf("typed packet default changed: %v, %v", err, jsonPacket.String())
	}
}

func TestPrivateSkillModulesAreNotPublicResources(t *testing.T) {
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("private module request reached the Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)

	// The definition and shared modules are internal: neither is a public
	// resource, by enumeration, retrieval, or input discovery.
	for _, resource := range []string{"procedures/modules/implement.md", "procedures/implement.md", "implement.md"} {
		for _, request := range [][]string{
			{"skl", "skill", "--resource", resource, "implement"},
			{"skl", "skill", "--resource", resource, "--describe-inputs", "implement"},
		} {
			output.Reset()
			err := app.Run(request)
			if err == nil || !strings.Contains(err.Error(), "unknown resource") {
				t.Errorf("%v = %v, want an unavailable resource", request, err)
			}
			if output.Len() != 0 || (err != nil && strings.Contains(err.Error(), "never parses, judges, or cross-checks")) {
				t.Errorf("%v disclosed private content: %q", request, output.String())
			}
		}
	}

	packet := implementBundle(t)
	for _, resource := range packet.Resources {
		if strings.HasPrefix(resource, "modules/") {
			t.Errorf("private module %s is listed as a public resource", resource)
		}
	}

}

func TestRetiredGitHubResourcesAreUnknown(t *testing.T) {
	for _, request := range [][]string{
		{"submission.md", "implement"},
		{"decision.md", "implement"},
		{"review.md", "watchdog"},
	} {
		var output bytes.Buffer
		err := newApp(nil, bytes.NewReader(nil), &output, &output).Run([]string{"skl", "skill", "--resource", request[0], request[1]})
		if err == nil || !strings.Contains(err.Error(), "unknown resource") || output.Len() != 0 {
			t.Errorf("%s of %s is still served: %v\n%s", request[0], request[1], err, output.String())
		}
	}
}

func TestRetrieveOneNamedResource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "--resource", "tests.md", "testing"}); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), readRepositoryFile(t, "prose/craft/testing/tests.md"); got != want {
		t.Fatalf("stdout did not contain only the requested resource:\n%s", got)
	}
	stdout.Reset()
	if err := app.Run([]string{"skl", "skill", "--resource", "CAPABILITIES-FORMAT.md", "domain"}); err == nil || !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("retired capability resource error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("retired capability resource returned content: %q", stdout.String())
	}
}

func TestDescribeNamedResourceInputs(t *testing.T) {
	cases := []struct {
		owner     string
		resource  string
		inputs    []string
		described []string
	}{
		{
			owner: "implement", resource: "ledger-submission.md",
			inputs: []string{"result_directory=/tmp/result", "procedure=initial"},
			described: []string{
				"result_directory (string, required): Absolute path of the private Result Document directory this invocation created.",
				"procedure (string, required, one of: initial, resumed, rework): Which submission procedure to render.",
			},
		},
		{
			owner: "watchdog", resource: "ledger-review.md",
			inputs: []string{"result_directory=/tmp/result", "round=2", "reviewed_head=" + strings.Repeat("a", 40)},
			described: []string{
				"result_directory (string, required): Absolute path of the private Result Document directory this invocation created.",
				"round (integer, required): Next completed Work Item review round.",
				"reviewed_head (string, required): Fixed reviewed source commit.",
			},
		},
		{
			owner: "testing", resource: "tests.md",
			described: []string{"tests.md accepts no inputs."},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.owner+"/"+testCase.resource, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Fatal("resource discovery reached the Workflow Backend")
				return nil, nil
			}, bytes.NewReader(nil), &stdout, &stderr)
			describe := []string{"skl", "skill", "--resource", testCase.resource, "--describe-inputs", testCase.owner}
			if err := app.Run(describe); err != nil {
				t.Fatalf("%v: %v", describe, err)
			}
			description := stdout.String()
			if !strings.Contains(description, testCase.resource+" accepts") {
				t.Errorf("description does not report the requested resource:\n%s", description)
			}
			for _, want := range testCase.described {
				if !strings.Contains(description, want) {
					t.Errorf("description is missing %q:\n%s", want, description)
				}
			}
			run := func(inputs []string) error {
				stdout.Reset()
				command := []string{"skl", "skill", "--resource", testCase.resource}
				for _, input := range inputs {
					command = append(command, "--input", input)
				}
				return app.Run(append(command, testCase.owner))
			}
			if err := run(testCase.inputs); err != nil {
				t.Fatalf("described inputs are not retrievable: %v", err)
			}
			// The description names the inputs; it never renders the resource.
			title, _, _ := strings.Cut(stdout.String(), "\n")
			if title == "" || strings.Contains(description, title) {
				t.Errorf("description rendered the resource %q:\n%s", title, description)
			}
			if err := run(append(slices.Clone(testCase.inputs), "undeclared=1")); err == nil || !strings.Contains(err.Error(), "undeclared") {
				t.Fatalf("description described more inputs than retrieval accepts: %v", err)
			}
		})
	}
}

func TestBundleGuaranteedSupportingSkills(t *testing.T) {
	packet := implementBundle(t)
	want := []string{"testing", "audit"}
	if !slices.Equal(packet.IncludedSkills, want) {
		t.Fatalf("included_skills = %v, want %v", packet.IncludedSkills, want)
	}
	if !slices.Equal(packet.Resources, []string{"ledger-submission.md", "pull-presentation.md"}) {
		t.Fatalf("implementation resources changed: %v", packet.Resources)
	}
}

func TestIgnoreConsumerRepositoryOverrides(t *testing.T) {
	repository := t.TempDir()
	override := filepath.Join(repository, "prose/craft/testing.md")
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
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), readRepositoryFile(t, "prose/craft/testing.md"); got != want {
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

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func proposalRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "remote", "add", "origin", "git@github.com:acme/widgets.git")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("widget\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "initial")
	runGit(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	return root
}

func prepareSlice(t *testing.T, root, slug string) string {
	t.Helper()
	runGit(t, root, "switch", "-c", slug, "main")
	writeLedger(t, root, slug, true)
	runGit(t, root, "add", filepath.Join(".changes", slug))
	runGit(t, root, "commit", "-m", "[baseline] "+slug)
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "update-ref", "refs/remotes/origin/"+slug, head)
	return head
}

func completeAndRetireSlice(t *testing.T, root, slug string) string {
	t.Helper()
	runGit(t, root, "commit", "--allow-empty", "-m", "[completion] "+slug)
	runGit(t, root, "rm", "-r", filepath.Join(".changes", slug))
	runGit(t, root, "commit", "-m", "retire "+slug)
	return strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
}

func writeLedger(t *testing.T, root, slug string, complete bool) {
	t.Helper()
	directory := filepath.Join(root, ".changes", slug)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{"intent.md"}
	if complete {
		names = append(names, "behavior.md")
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func runGitOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func gitRefExists(root, ref string) bool {
	return exec.Command("git", "-C", root, "show-ref", "--verify", "--quiet", ref).Run() == nil
}
