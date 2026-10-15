package main

import (
	"strings"
	"testing"
)

func TestSliceReadsRejectRemovedAndInapplicableSyntax(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		flag string
	}{
		{"raw-item", []string{"ledger", "show", "--item", "orders/cancel"}, "item"},
		{"raw-phase", []string{"ledger", "show", "--phase", "implement"}, "phase"},
		{"raw-repo", []string{"ledger", "show", "--repo", "."}, "repo"},
		{"raw-remote", []string{"ledger", "show", "--remote", "origin"}, "remote"},
		{"current-item", []string{"slice", "show", "--item", "orders/cancel"}, "item"},
		{"current-exact", []string{"slice", "show", "--slice", "orders/cancel", "--commit", strings.Repeat("a", 40)}, "commit"},
		{"browse-item", []string{"browse", "slice", "--project", "widgets", "--item", "orders/cancel"}, "item"},
		{"documents-item", []string{"browse", "documents", "--item", "orders/cancel"}, "item"},
		{"versions-item", []string{"browse", "versions", "--item", "orders/cancel"}, "item"},
		{"browse-claim", []string{"browse", "slices", "--claim", "watchdog"}, "claim"},
		{"interactive-format", []string{"browse", "--format", "json"}, "format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, output := browseApp(t)
			err := app.Run(append([]string{"skl"}, tc.args...))
			if err == nil || !strings.Contains(err.Error(), "flag provided but not defined: -"+tc.flag) {
				t.Fatalf("removed/inapplicable syntax did not produce a parser error: %v\n%s", err, output)
			}
			if strings.Contains(output.String(), `"status"`) {
				t.Fatalf("parser error became a structured Workflow refusal: %s", output)
			}
		})
	}
}

func TestSliceReadsHelpExposesCanonicalFlags(t *testing.T) {
	for _, tc := range []struct {
		args            []string
		present, absent []string
	}{
		{[]string{"slice", "show"}, []string{"--slice", "--phase", "--repo", "--remote", "--format", "report availability"}, []string{"--item", "--commit", "--path"}},
		{[]string{"ledger", "show"}, []string{"--commit", "--path", "--format", "without interpreting"}, []string{"--item", "--slice", "--phase", "--repo", "--remote"}},
		{[]string{"browse", "slice"}, []string{"--project", "--slice", "--archived", "--format"}, []string{"--item"}},
		{[]string{"browse", "slices"}, []string{"--project", "--claim-phase", "implement", "watchdog", "none", "--lifecycle", "--search", "--group", "--include-archived", "--format"}, []string{"--claim value"}},
		{[]string{"browse", "documents"}, []string{"--project", "--proposal", "--slice", "--archived", "--format"}, []string{"--item"}},
		{[]string{"browse", "versions"}, []string{"--project", "--slice", "--phase", "--archived", "--format"}, []string{"--item"}},
	} {
		t.Run(strings.Join(tc.args, "-"), func(t *testing.T) {
			app, output := browseApp(t)
			args := append(append([]string{"skl"}, tc.args...), "--help")
			if err := app.Run(args); err != nil {
				t.Fatal(err)
			}
			for _, flag := range tc.present {
				if !strings.Contains(output.String(), flag) {
					t.Errorf("help lacks %q: %s", flag, output)
				}
			}
			for _, flag := range tc.absent {
				if strings.Contains(output.String(), flag) {
					t.Errorf("help exposes %q: %s", flag, output)
				}
			}
		})
	}
}
