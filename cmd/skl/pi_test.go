package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

func TestInstallPiQueueAdaptersWithoutDuplicatingSkills(t *testing.T) {
	home := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, home)
	for range 2 {
		if err := app.Run([]string{"skl", "install"}); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{"prompts/implement-loop.md", "prompts/watchdog-loop.md", "prompts/queue-next.mjs", "agents/implement-runner.md", "agents/watchdog-runner.md"} {
			got := readFile(t, filepath.Join(home, ".pi/agent", file))
			want := readRepositoryFile(t, filepath.Join("prose/adapters", file))
			if got != want {
				t.Fatalf("installed adapter %s differs", file)
			}
			for _, other := range []string{".codex", ".claude", ".config/opencode"} {
				if _, err := os.Stat(filepath.Join(home, other, file)); !os.IsNotExist(err) {
					t.Fatalf("Pi adapter installed in %s", other)
				}
			}
		}
	}
	path := filepath.Join(home, ".pi/agent/prompts/implement-loop.md")
	if err := os.WriteFile(path, []byte("user-owned queue"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "user-owned queue" {
		t.Fatal("overwrote user's adapter")
	}

	runner := filepath.Join(home, ".pi/agent/agents/implement-runner.md")
	if err := os.WriteFile(runner, []byte("user-owned implement runner\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, runner); got != "user-owned implement runner\n" {
		t.Fatal("overwrote user's implement runner")
	}

	upgradeHome := t.TempDir()
	legacyRunner := filepath.Join(upgradeHome, ".pi/agent/agents/implement-runner.md")
	if err := os.MkdirAll(filepath.Dir(legacyRunner), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyRunner, []byte("<!-- skl-owned: skl.pi/v1 -->\nUse `subagent` only for audit.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	upgradeApp := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, upgradeHome)
	if err := upgradeApp.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	upgraded := readFile(t, legacyRunner)
	if !strings.Contains(upgraded, "optional `subagent` use for bounded, non-conflicting implementation or testing assignments") || strings.Contains(upgraded, "Use `subagent` only for audit") {
		t.Fatalf("owned implement runner was not upgraded: %s", upgraded)
	}
}

func TestInstallDisablesOwnedWatchdogLoopButPreservesOneItemRunner(t *testing.T) {
	home := t.TempDir()
	owned := filepath.Join(home, ".pi/agent/prompts/watchdog-loop.md")
	if err := os.MkdirAll(filepath.Dir(owned), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(owned, []byte("<!-- skl-owned: skl.pi/v1 -->\nlaunch watchdog-runner repeatedly\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, home)
	for range 2 {
		if err := app.Run([]string{"skl", "install"}); err != nil {
			t.Fatal(err)
		}
		loop := readFile(t, owned)
		if !strings.Contains(loop, "disabled") || strings.Contains(loop, "launch") || strings.Contains(loop, "skl watchdog next") {
			t.Fatalf("owned loop could still claim work: %s", loop)
		}
	}
	if !strings.Contains(readFile(t, filepath.Join(home, ".pi/agent/agents/watchdog-runner.md")), "skl watchdog next") {
		t.Fatal("one-item runner was removed")
	}
	if !strings.Contains(readFile(t, filepath.Join(home, ".pi/agent/prompts/implement-loop.md")), "implement-runner") {
		t.Fatal("independent implementation loop was changed")
	}
	if _, err := os.Stat(filepath.Join(home, ".pi/agent/prompts/queue-next.mjs")); err != nil {
		t.Fatal("shared queue helper was removed")
	}
	userHome := t.TempDir()
	userPrompt := filepath.Join(userHome, ".pi/agent/prompts/watchdog-loop.md")
	if err := os.MkdirAll(filepath.Dir(userPrompt), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPrompt, []byte("my own review prompt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	userApp := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, userHome)
	if err := userApp.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, userPrompt); got != "my own review prompt\n" {
		t.Fatalf("user-owned Watchdog prompt changed: %s", got)
	}
}
