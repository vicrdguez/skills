package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
)

func TestSkillRejectsInapplicableOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"resource JSON", []string{"--resource", "tests.md", "--format", "json", "testing"}, "--format"},
		{"resource explicit Markdown", []string{"--resource", "tests.md", "--format", "markdown", "testing"}, "--format"},
		{"description format", []string{"--resource", "tests.md", "--describe-inputs", "--format", "json", "testing"}, "--format"},
		{"description inputs", []string{"--resource", "ledger-submission.md", "--describe-inputs", "--input", "procedure=initial", "implement"}, "--input"},
		{"disabled description without resource", []string{"--describe-inputs=false", "testing"}, "--resource"},
		{"empty resource", []string{"--resource", "", "testing"}, "--resource"},
		{"unknown whole-skill format", []string{"--format", "raw", "testing"}, "unsupported format"},
		{"repository for another skill", []string{"--repo", ".", "testing"}, "--repo"},
		{"repository for a verify resource", []string{"--repo", ".", "--resource", "tests.md", "verify"}, "--repo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := newApp(nil, strings.NewReader(""), &stdout, &stderr).Run(append([]string{"skl", "skill"}, tc.args...))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("invalid options returned content: %q", stdout.String())
			}
		})
	}
}

func TestShapeIsUnavailableAndSurvivingStandaloneSkillsRemain(t *testing.T) {
	want := []string{"audit", "brainstorm", "decision", "design", "domain", "explore", "implement", "propose", "testing", "verify", "verify-create", "verify-maintain", "walkthrough", "watchdog", "writing-for-agents"}
	if got := skilldist.SkillNames(); !slices.Equal(got, want) {
		t.Fatalf("skill catalog = %v, want %v", got, want)
	}
	for _, name := range want {
		if name == "implement" || name == "watchdog" || name == "verify" {
			continue // Implement and Watchdog refuse here, and verify reads a repository's own skill.
		}
		var output bytes.Buffer
		if err := newApp(nil, strings.NewReader(""), &output, &output).Run([]string{"skl", "skill", name}); err != nil || output.Len() == 0 {
			t.Errorf("standalone %s = %v, %q", name, err, output.String())
		}
	}
	for _, args := range [][]string{
		{"shape"},
		{"--format", "json", "shape"},
		{"--resource", "shape.md", "shape"},
		{"--resource", "shape.md", "--describe-inputs", "shape"},
	} {
		var output bytes.Buffer
		err := newApp(nil, strings.NewReader(""), &output, &output).Run(append([]string{"skl", "skill"}, args...))
		if err == nil || !strings.Contains(err.Error(), "unknown skill") || output.Len() != 0 {
			t.Errorf("retired %v = %v, %q", args, err, output.String())
		}
	}
}

func TestSkillHelpExplainsOutputModes(t *testing.T) {
	var output bytes.Buffer
	if err := newApp(nil, strings.NewReader(""), &output, &output).Run([]string{"skl", "skill", "--help"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--format", "markdown|json", "whole-skill", "--resource", "raw", "--input", "name=value", "--describe-inputs"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("help missing %q: %s", want, output.String())
		}
	}
}
