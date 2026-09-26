package main

import (
	"bytes"
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
		{"skl", "skill", "--resource", "ledger-submission.md", "--input", "result_directory=" + t.TempDir(), "--input", "procedure=initial", "implement"},
		{"skl", "skill", "--resource", "ledger-submission.md", "--describe-inputs", "implement"},
	} {
		output.Reset()
		if err := app.Run(retrieval); err != nil || output.Len() == 0 {
			t.Errorf("%v = %v\n%s", retrieval, err, &output)
		}
	}
}
