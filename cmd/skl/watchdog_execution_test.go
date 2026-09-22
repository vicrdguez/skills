package main

import (
	"bytes"
	"context"

	"errors"

	"os"
	"path/filepath"

	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type failedWatchdogQueue struct{ *implementationMemory }

func (*failedWatchdogQueue) QueuePage(context.Context, workflow.QueueKind, string) (workflow.QueuePage, error) {
	return workflow.QueuePage{}, errors.New("backend unavailable")
}

type uncertainWatchdogClaim struct{ *implementationMemory }

func (*uncertainWatchdogClaim) ClaimSelected(context.Context, workflow.QueueCandidate, workflow.ImplementationItem) (workflow.ImplementationItem, error) {
	return workflow.ImplementationItem{}, errors.New("read-back unavailable")
}

type selectedOnlyWatchdogInspection struct{ *implementationMemory }

func (*selectedOnlyWatchdogInspection) ImplementationItems(context.Context) ([]workflow.ImplementationItem, error) {
	return nil, errors.New("global Work Item scan is unavailable")
}

func TestPlainWatchdogSkillRetrievalRequiresSelectedWork(t *testing.T) {
	for _, format := range []string{"markdown", "json"} {
		t.Run(format, func(t *testing.T) {
			var output bytes.Buffer
			backendCalls := 0
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { backendCalls++; return nil, nil }, bytes.NewReader(nil), &output, &output)
			err := app.Run([]string{"skl", "skill", "--format", format, "watchdog"})
			if err == nil || !strings.Contains(err.Error(), "skl watchdog next") || !strings.Contains(err.Error(), "skl watchdog resume") {
				t.Fatalf("generic Watchdog retrieval was not refused: %v", err)
			}
			if backendCalls != 0 || output.Len() != 0 {
				t.Fatalf("refusal selected work or rendered instructions: calls=%d output=%s", backendCalls, &output)
			}
		})
	}
}

func TestInstallDirectWatchdogStubsForSupportedHarnesses(t *testing.T) {
	home := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return nil, nil }, bytes.NewReader(nil), &output, &output, home)
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		path := filepath.Join(home, harness, "watchdog", "SKILL.md")
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), "skl watchdog next") || strings.Contains(string(contents), "skl skill watchdog") {
			t.Errorf("Watchdog stub is not direct: %s", path)
		}
	}
}

func TestWatchdogRejectsUnsupportedFormatBeforeBackendEffects(t *testing.T) {
	for _, operation := range []string{"next", "resume", "inspect", "submit"} {
		t.Run(operation, func(t *testing.T) {
			backendCalls := 0
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { backendCalls++; return nil, nil }, bytes.NewReader(nil), &output, &output)
			err := app.Run([]string{"skl", "watchdog", operation, "--format", "xml"})
			if err == nil || !strings.Contains(err.Error(), "unsupported format") || backendCalls != 0 || output.Len() != 0 {
				t.Fatalf("invalid format reached effects: error=%v calls=%d output=%s", err, backendCalls, &output)
			}
		})
	}
}
