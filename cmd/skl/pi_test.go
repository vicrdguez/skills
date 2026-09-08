package main

import (
	"bytes"
	"os"
	"path/filepath"
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
			want := readRepositoryFile(t, file)
			if got != want {
				t.Fatalf("installed adapter %s differs", file)
			}
			for _, other := range []string{".codex", ".claude"} {
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
}
