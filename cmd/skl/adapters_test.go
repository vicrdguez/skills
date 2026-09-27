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

// loopCommand interprets the installed adapter's instruction: each nonempty
// slot becomes one opaque argument, and empty slots leave the harness default.
func loopCommand(t *testing.T, entry string) []string {
	t.Helper()
	command := regexp.MustCompile("Run `([^`]+)`").FindStringSubmatch(entry)
	if command == nil {
		t.Fatalf("loop entry runs no command:\n%s", entry)
	}
	words := shellWords(t, command[1])
	for _, slot := range regexp.MustCompile(`(?m)^- (--[a-z-]+): (.*)$`).FindAllStringSubmatch(entry, -1) {
		value := regexp.MustCompile(`\$\{\d+:-([^}]*)\}`).ReplaceAllString(slot[2], "$1")
		if regexp.MustCompile(`^\$\d+$`).MatchString(value) {
			value = ""
		}
		if value != "" {
			words = append(words, slot[1], value)
		}
	}
	return words
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

func TestInstallWritesDispatchLoopsForPiAndOpenCode(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	const sol = "openai-codex/gpt-6-sol"
	cases := map[string]map[string][]string{
		"pi": {
			"implement-loop":      {"skl", "implement", "next", "--dispatch", "--wait", "--worker-model", sol, "--worker-thinking", "xhigh", "--reviewer-model", "openai-codex/gpt-6-astra", "--reviewer-thinking", "low"},
			"implement-team-loop": {"skl", "implement", "next", "--mode", "team", "--dispatch", "--wait", "--worker-model", sol, "--worker-thinking", "xhigh", "--helper-model", "openai-codex/gpt-6-luna", "--helper-thinking", "xhigh", "--reviewer-model", sol, "--reviewer-thinking", "xhigh"},
			"watchdog-loop":       {"skl", "watchdog", "next", "--dispatch", "--wait", "--worker-model", "openai-codex/gpt-6-astra", "--worker-thinking", "high"},
		},
		"opencode": {
			"implement-loop":      {"skl", "implement", "next", "--dispatch", "--wait"},
			"implement-team-loop": {"skl", "implement", "next", "--mode", "team", "--dispatch", "--wait"},
			"watchdog-loop":       {"skl", "watchdog", "next", "--dispatch", "--wait"},
		},
	}
	for harness, operations := range cases {
		for operation, want := range operations {
			entry := readFile(t, filepath.Join(home, fmt.Sprintf(entryPoints[harness], operation)))
			if !strings.Contains(entry, "<!-- skl-owned: skl.adapter/v1 -->") || !strings.Contains(entry, "Follow each Outcome Instruction until one tells you to stop") {
				t.Errorf("%s %s is not an outcome-driven adapter:\n%s", harness, operation, entry)
			}
			if got := loopCommand(t, entry); !slices.Equal(got, want) {
				t.Errorf("%s %s runs %q, want %q", harness, operation, got, want)
			}
			if strings.Contains(entry, "Execution Skill") || strings.Contains(entry, "--after") || !strings.Contains(entry, "Quote values for the shell without changing them; omit empty slots") {
				t.Errorf("%s %s contains worker logic or lacks opaque argument guidance:\n%s", harness, operation, entry)
			}
			// Every supplied slot is passed unchanged, including punctuation
			// that would break an interpolated shell command.
			piWant := cases["pi"][operation]
			i := 0
			for position, flag := range piWant {
				if !strings.HasPrefix(flag, "--") || flag == "--dispatch" || flag == "--wait" || flag == "--mode" {
					continue
				}
				i++
				value := "user/o'brien-" + fmt.Sprint(i)
				custom := entry
				if harness == "pi" {
					custom = regexp.MustCompile(fmt.Sprintf(`\$\{%d:-[^}]*\}`, i)).ReplaceAllString(custom, value)
				} else {
					custom = strings.ReplaceAll(custom, fmt.Sprintf("$%d", i), value)
				}
				overridden := slices.Clone(want)
				if harness == "pi" {
					overridden[position+1] = value
				} else {
					overridden = append(overridden, flag, value)
				}
				if got := loopCommand(t, custom); !slices.Equal(got, overridden) {
					t.Errorf("%s %s slot %d runs %q, want %q", harness, operation, i, got, overridden)
				}
			}
		}
	}
	for _, harness := range []string{"codex", "claude"} {
		for operation := range cases["pi"] {
			if _, err := os.Stat(filepath.Join(home, fmt.Sprintf(entryPoints[harness], operation))); !os.IsNotExist(err) {
				t.Errorf("unexpected %s %s loop adapter: %v", harness, operation, err)
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
	userLoop := writeHomeFile(t, home, ".config/opencode/commands/implement-loop.md", "my loop\n")
	userPiLoop := writeHomeFile(t, home, ".pi/agent/prompts/implement-team-loop.md", "my pi loop\n")
	ownedLoop := writeHomeFile(t, home, ".pi/agent/prompts/watchdog-loop.md", "<!-- skl-owned: skl.adapter/v1 -->\nstale\n")

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
	if got := readFile(t, userLoop); got != "my loop\n" {
		t.Fatalf("user-owned OpenCode loop changed: %q", got)
	}
	if got := readFile(t, userPiLoop); got != "my pi loop\n" {
		t.Fatalf("user-owned pi loop changed: %q", got)
	}
	if got := readFile(t, ownedLoop); strings.Contains(got, "stale") || !strings.Contains(got, "skl watchdog next --dispatch --wait") {
		t.Fatalf("owned pi loop was not refreshed:\n%s", got)
	}
}

func TestInstallRetiresOwnedPiRunnersAndReplacedStubs(t *testing.T) {
	home := t.TempDir()
	var retired []string
	for _, file := range []string{"agents/implement-runner.md", "agents/watchdog-runner.md"} {
		retired = append(retired, writeHomeFile(t, home, ".pi/agent/"+file, "---\ndescription: x\n---\n\n<!-- skl-owned: skl.pi/v1 -->\nlegacy\n"))
	}
	retired = append(retired, writeHomeFile(t, home, ".pi/agent/prompts/queue-next.mjs", "// skl-owned: skl.pi/v1\n"))
	legacyLoop := writeHomeFile(t, home, ".pi/agent/prompts/implement-loop.md", "<!-- skl-owned: skl.pi/v1 -->\nlegacy loop\n")
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
	if got := readFile(t, legacyLoop); strings.Contains(got, "legacy loop") || !strings.Contains(got, "skl implement next --dispatch --wait") {
		t.Errorf("previously owned pi loop was not migrated: %s", got)
	}
	for path, want := range map[string]string{notes: "keep\n", userRunner: "mine\n", userRunnerCopy: "my own runner\n"} {
		if got := readFile(t, path); got != want {
			t.Errorf("user file %s changed: %q", path, got)
		}
	}
}
