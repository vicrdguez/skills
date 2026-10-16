package main

import (
	"path/filepath"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
)

func TestDecisionApplyHelpExposesSliceAndBatch(t *testing.T) {
	cli := decisionReadOnlyApp(t)
	help, err := cli.deliveryRun(t, "skl", "decision", "apply", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--project", "--slice", "--request-commit", "--request-path", "--route", "--answer", "--input", "--coupled", "--format"} {
		if !strings.Contains(help, flag) {
			t.Errorf("help is missing %s:\n%s", flag, help)
		}
	}
	if strings.Contains(help, "--item") {
		t.Fatalf("help advertises removed selector:\n%s", help)
	}
}

func TestDecisionRejectsRemovedAndInapplicableSelectors(t *testing.T) {
	for _, args := range [][]string{
		{"apply", "--item", "repair/fix"},
		{"inbox", "--slice", "repair/fix"},
	} {
		t.Run(args[0], func(t *testing.T) {
			cli := decisionReadOnlyApp(t)
			_, err := cli.deliveryRun(t, append([]string{"skl", "decision"}, args...)...)
			if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
				t.Fatalf("removed/inapplicable selector = %v, want parser error", err)
			}
		})
	}
}

func TestDecisionSliceKeepsStructuredInputRefusals(t *testing.T) {
	batch := filepath.Join(t.TempDir(), "decisions.json")
	writeFile(t, batch, "[]\n")
	for _, tc := range []struct {
		name   string
		args   []string
		reason string
	}{
		{"slice alone", []string{"--slice", "repair/fix"}, "--project, --slice, --request-commit, --request-path, --route, and --answer"},
		{"slice mixed with batch", []string{"--slice", "repair/fix", "--input", batch}, "single-request flags"},
		{"coupled without batch", []string{"--coupled", "--slice", "repair/fix"}, "--coupled applies only with --input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := decisionReadOnlyApp(t)
			args := append([]string{"skl", "decision", "apply", "--format", "json"}, tc.args...)
			out := cli.decisionJSON(t, args...)
			if out.Status != string(skilldist.DecisionRefused) || !strings.Contains(out.Facts.Reason, tc.reason) || len(out.Results) != 0 {
				t.Fatalf("semantic refusal changed transport: %#v", out)
			}
		})
	}
}
