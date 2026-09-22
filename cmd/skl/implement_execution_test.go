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

// TestB19GenericImplementRetrievalRefusesWithoutWorkflowEffects materializes B19.
func TestB19GenericImplementRetrievalRefusesWithoutWorkflowEffects(t *testing.T) {
	for _, format := range []string{"markdown", "json"} {
		t.Run(format, func(t *testing.T) {
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Error("read-only Implement retrieval opened a Workflow Backend")
				return nil, nil
			}, bytes.NewReader(nil), &output, &output)
			err := app.Run([]string{"skl", "skill", "--format", format, "implement"})
			if err == nil {
				t.Fatalf("generic Implement retrieval succeeded: %s", &output)
			}
			for _, guidance := range []string{"skl implement next", "skl implement resume --item"} {
				if !strings.Contains(err.Error(), guidance) {
					t.Errorf("refusal lacks %q: %v", guidance, err)
				}
			}
			if output.Len() != 0 {
				t.Errorf("refusal emitted an execution: %q", output.String())
			}
		})
	}
	// Independent reasoning retrieval and the Implement resources stay usable.
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Error("reasoning retrieval opened a Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	for _, retrieval := range [][]string{
		{"skl", "skill", "testing"},
		{"skl", "skill", "--format", "json", "design"},
		{"skl", "skill", "--resource", "reference/submission.md", "--input", "result_directory=" + t.TempDir(), "--input", "procedure=initial", "implement"},
		{"skl", "skill", "--resource", "reference/decision.md", "--describe-inputs", "implement"},
	} {
		output.Reset()
		if err := app.Run(retrieval); err != nil || output.Len() == 0 {
			t.Errorf("%v = %v\n%s", retrieval, err, &output)
		}
	}
}

// TestB20InstallationDisablesOwnedLegacyLoopsWithoutCollateralRemoval
// materializes the B20 outline.
func TestB20InstallationDisablesOwnedLegacyLoopsWithoutCollateralRemoval(t *testing.T) {
	legacy := "---\ndescription: Drain the implementation queue\n---\n\n<!-- skl-owned: skl.pi/v1 -->\n\nLaunch an implement-runner subagent per item and relay its JSON.\n"
	for _, testCase := range []struct {
		name     string
		existing string
		want     func(string) bool
	}{
		{"fresh home", "", func(installed string) bool {
			return strings.Contains(installed, "This queue-draining Implementation loop is disabled") && !strings.Contains(installed, "Launch an implement-runner")
		}},
		{"existing loop with skl.pi/v1 ownership marker", legacy, func(installed string) bool {
			return strings.Contains(installed, "This queue-draining Implementation loop is disabled") && !strings.Contains(installed, "Launch an implement-runner")
		}},
		{"user-owned loop without the owned marker", "user-owned queue\n", func(installed string) bool {
			return installed == "user-owned queue\n"
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			home := t.TempDir()
			loop := filepath.Join(home, ".pi/agent/prompts/implement-loop.md")
			if testCase.existing != "" {
				if err := os.MkdirAll(filepath.Dir(loop), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(loop, []byte(testCase.existing), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, home)
			for range 2 {
				if err := app.Run([]string{"skl", "install"}); err != nil {
					t.Fatal(err)
				}
			}
			installed := readFile(t, loop)
			if !testCase.want(installed) {
				t.Fatalf("installed loop = %q", installed)
			}
			if strings.Contains(installed, "--format json") {
				t.Error("refresh kept the legacy loop alive with JSON flags")
			}
			for _, disabled := range []string{"launches no worker", "claims no Work Item", "one Work Item at a time", "skl implement resume --item <proposal>/<slice> --claim <acquisition-commit>"} {
				if testCase.existing == "user-owned queue\n" {
					break
				}
				if !strings.Contains(installed, disabled) {
					t.Errorf("disabled loop lacks %q:\n%s", disabled, installed)
				}
			}

			if _, err := os.Stat(filepath.Join(home, ".pi/agent/prompts/queue-next.mjs")); err != nil {
				t.Fatalf("still-used queue helper was removed: %v", err)
			}
		})
	}
}

// TestB21ThePiRunnerReportsOneItemInNormalMarkdown materializes B21.
func TestB21ThePiRunnerReportsOneItemInNormalMarkdown(t *testing.T) {
	runner := readRepositoryFile(t, "agents/implement-runner.md")
	for _, required := range []string{
		"Run `skl implement next` and follow the returned Execution Skill exactly",
		"Process at most one Work Item",
		"report the verified outcome in normal Markdown prose",
		"Never reproduce the engine's exact JSON",
		"never report success the engine did not verify",
		"Do not drain the queue",
		"do not launch a replacement worker after an empty, uncertain, or incomplete result",
		"may permit optional `subagent` use for bounded, non-conflicting implementation or testing assignments within this one Claim",
		"authoritative contract inputs",
		"serialize overlaps, inspect and integrate its work",
		"Helpers must not select queue work, acquire Claims, change Workflow State, or publish",
		"Use fresh contexts for the parallel reviewers the bundled `audit` requires",
		"optional serial fallback never weakens that Audit",
	} {
		if !strings.Contains(runner, required) {
			t.Errorf("runner lacks %q", required)
		}
	}
	for _, forbidden := range []string{"return the final structured CLI JSON unchanged", "write its final structured CLI JSON"} {
		if strings.Contains(runner, forbidden) {
			t.Errorf("runner still relays exact CLI JSON: %q", forbidden)
		}
	}

	loop := readRepositoryFile(t, "prompts/implement-loop.md")
	for _, forbidden := range []string{"Launch", "launch a", "spawn", "subagent", "queue-next.mjs", "--format json", "argument-hint"} {
		if strings.Contains(loop, forbidden) {
			t.Errorf("disabled loop still schedules work with %q", forbidden)
		}
	}
}
