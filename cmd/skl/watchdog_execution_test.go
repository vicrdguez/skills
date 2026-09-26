package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

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
