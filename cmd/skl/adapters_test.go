package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

// entryPoints are where each supported harness starts Implement and Watchdog.
// Codex has no argument-taking entry-point mechanism, so it keeps the stub.
var entryPoints = map[string]string{
	"pi":       ".pi/agent/prompts/%s.md",
	"codex":    ".codex/skills/%s/SKILL.md",
	"claude":   ".claude/skills/%s/SKILL.md",
	"opencode": ".config/opencode/commands/%s.md",
}

func installInto(t *testing.T, home string) {
	t.Helper()
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, home)
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
}

func writeHomeFile(t *testing.T, home, file, contents string) string {
	t.Helper()
	path := filepath.Join(home, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstallWritesImplementAndWatchdogEntryPointsForEachHarness(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	first := map[string]string{}
	for harness, location := range entryPoints {
		for _, operation := range []string{"implement", "implement-team", "watchdog"} {
			path := filepath.Join(home, strings.ReplaceAll(location, "%s", operation))
			entry := readFile(t, path)
			if !strings.Contains(entry, "skl "+strings.TrimSuffix(operation, "-team")+" next") || !strings.Contains(entry, "<!-- skl-owned: skl.") {
				t.Errorf("%s %s entry point does not start the owned operation:\n%s", harness, operation, entry)
			}
			first[path] = entry
		}
	}
	for _, replaced := range []string{".pi/agent/skills", ".config/opencode/skills"} {
		for _, operation := range []string{"implement", "implement-team", "watchdog"} {
			if _, err := os.Stat(filepath.Join(home, replaced, operation)); !os.IsNotExist(err) {
				t.Errorf("%s still installs a %s stub beside its entry point: %v", replaced, operation, err)
			}
		}
	}

	installInto(t, home)
	for path, want := range first {
		if got := readFile(t, path); got != want {
			t.Errorf("reinstall changed %s:\n%s", path, got)
		}
	}
}

// invokedCommand is the command an entry point runs when the user passes no
// arguments, expanded as each harness documents its placeholders: pi fills
// ${N:-default} with its default, and every other placeholder is empty.
func invokedCommand(t *testing.T, entry string) []string {
	t.Helper()
	command := regexp.MustCompile("Run `([^`]+)`").FindStringSubmatch(entry)
	if command == nil {
		t.Fatalf("entry point runs no command:\n%s", entry)
	}
	expanded := regexp.MustCompile(`\$\{\d+:-([^}]*)\}`).ReplaceAllString(command[1], "$1")
	return shellWords(t, regexp.MustCompile(`\$\w+`).ReplaceAllString(expanded, ""))
}

func TestInstallPassesEachModeAndItsSlotsWithPiDefaults(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	const sol, luna = "openai-codex/gpt-6-sol", "openai-codex/gpt-6-luna"
	piDefaults := map[string][]string{
		"implement":      {"--reviewer-model", sol, "--reviewer-thinking", "xhigh"},
		"implement-team": {"--mode", "team", "--helper-model", luna, "--helper-thinking", "xhigh", "--reviewer-model", sol, "--reviewer-thinking", "xhigh"},
	}
	empty := map[string][]string{
		"implement":      {"--reviewer-model", "", "--reviewer-thinking", ""},
		"implement-team": {"--mode", "team", "--helper-model", "", "--helper-thinking", "", "--reviewer-model", "", "--reviewer-thinking", ""},
	}
	// Codex keeps stubs, which pass the mode without slots.
	stubs := map[string][]string{"implement-team": {"--mode", "team"}}
	for harness, location := range entryPoints {
		for _, operation := range []string{"implement", "implement-team"} {
			entry := readFile(t, filepath.Join(home, fmt.Sprintf(location, operation)))
			slots := map[string]map[string][]string{"pi": piDefaults, "codex": stubs}[harness]
			if slots == nil {
				slots = empty
			}
			want := append([]string{"skl", "implement", "next"}, slots[operation]...)
			if got := invokedCommand(t, entry); !slices.Equal(got, want) {
				t.Errorf("%s %s runs %q, want %q", harness, operation, got, want)
			}
		}
	}
}

func TestInstallRefreshesOwnedEntryPointsAndLeavesUserTemplates(t *testing.T) {
	home := t.TempDir()
	const userTemplate = "---\ndescription: My own implement flow\n---\nDo it my way.\n"
	user := writeHomeFile(t, home, ".pi/agent/prompts/implement.md", userTemplate)
	owned := writeHomeFile(t, home, ".config/opencode/commands/watchdog.md", "<!-- skl-owned: skl.adapter/v1 -->\nstale\n")
	stub := writeHomeFile(t, home, ".claude/skills/implement/SKILL.md", "---\nname: implement\n---\n\n<!-- skl-owned: skl.stub/v1 -->\n\nRun `skl implement next`.\n")

	installInto(t, home)

	if got := readFile(t, user); got != userTemplate {
		t.Fatalf("user-owned pi template changed:\n%s", got)
	}
	if got := readFile(t, owned); strings.Contains(got, "stale") || !strings.Contains(got, "skl watchdog next") {
		t.Fatalf("owned OpenCode entry point was not refreshed:\n%s", got)
	}
	if got := readFile(t, stub); !strings.Contains(got, "skl.adapter/v1") {
		t.Fatalf("owned Claude Code stub was not replaced by its entry point:\n%s", got)
	}
}

func TestInstallRetiresOwnedPiRunnersLoopsAndReplacedStubs(t *testing.T) {
	home := t.TempDir()
	var retired []string
	for _, file := range []string{"agents/implement-runner.md", "agents/watchdog-runner.md", "prompts/implement-loop.md", "prompts/watchdog-loop.md"} {
		retired = append(retired, writeHomeFile(t, home, ".pi/agent/"+file, "---\ndescription: x\n---\n\n<!-- skl-owned: skl.pi/v1 -->\nlegacy\n"))
	}
	retired = append(retired, writeHomeFile(t, home, ".pi/agent/prompts/queue-next.mjs", "// skl-owned: skl.pi/v1\n"))
	stub := "---\nname: implement\n---\n\n<!-- skl-owned: skl.stub/v1 -->\n\nRun `skl implement next`.\n"
	retired = append(retired, writeHomeFile(t, home, ".pi/agent/skills/implement/SKILL.md", stub))
	openCodeStub := writeHomeFile(t, home, ".config/opencode/skills/watchdog/SKILL.md", stub)
	notes := writeHomeFile(t, home, ".config/opencode/skills/watchdog/notes.md", "keep\n")
	userRunner := writeHomeFile(t, home, ".pi/agent/agents/my-runner.md", "mine\n")
	userHome := t.TempDir()
	userRunnerCopy := writeHomeFile(t, userHome, ".pi/agent/agents/watchdog-runner.md", "my own runner\n")

	installInto(t, home)
	installInto(t, userHome)

	for _, path := range append(retired, openCodeStub) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("owned %s was not retired: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".pi/agent/skills/implement")); !os.IsNotExist(err) {
		t.Errorf("retired stub left its empty skill directory: %v", err)
	}
	if !strings.Contains(readFile(t, filepath.Join(home, ".pi/agent/prompts/implement.md")), "skl implement next") {
		t.Error("the pi Implement entry point was not installed")
	}
	for path, want := range map[string]string{notes: "keep\n", userRunner: "mine\n", userRunnerCopy: "my own runner\n"} {
		if got := readFile(t, path); got != want {
			t.Errorf("user file %s changed: %q", path, got)
		}
	}
}
