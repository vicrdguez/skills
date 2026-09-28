package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

func TestStatusRequiresConfiguredAcceptedProjectWithoutForgeAccess(t *testing.T) {
	// A source repository without an accepted ledger Project cannot derive
	// status from any external records.
	root := sourceRepository(t, "acme", "widgets")
	fixture := newLedgerFixture(t)
	for _, tc := range []struct {
		name, item, want string
		configure        func(*testing.T)
	}{
		{"missing configuration", "", "config", func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		}},
		{"missing configuration fixed item", "old/review", "config", func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		}},
		{"unadopted project", "", "accept", func(t *testing.T) {}},
		{"unadopted project fixed item", "old/review", "accept", func(t *testing.T) {}},
		{"broken configuration", "", "ledger", func(t *testing.T) {
			fixture.misconfigure(t, "{broken json")
		}},
		{"unusable ledger clone", "", "clone", func(t *testing.T) {
			fixture.misconfigure(t, `{"ledger":"`+t.TempDir()+`"}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture.selectWithXDG(t)
			tc.configure(t)
			before := ledgerSnapshot(t, fixture.clone)
			calls := 0
			formats := []string{"json"}
			if tc.name == "missing configuration" {
				formats = append(formats, "markdown")
			}
			for _, format := range formats {
				var output bytes.Buffer
				app := newApp(func(github.RepositoryID) (setup.Backend, error) {
					calls++
					t.Fatal("status accessed GitHub without an accepted Project")
					return nil, nil
				}, bytes.NewReader(nil), &output, &output)
				args := []string{"skl", "status", "--repo", root, "--format", format}
				if tc.item != "" {
					args = append(args, "--item", tc.item)
				}
				if err := app.Run(args); err != nil {
					t.Fatalf("status: %v", err)
				}
				if format == "json" {
					var outcome ledgerOutcome
					if err := json.Unmarshal(output.Bytes(), &outcome); err != nil || outcome.Status != "fix_required" || !strings.Contains(strings.ToLower(outcome.Repair), tc.want) || outcome.Reason == "" {
						t.Fatalf("missing actionable refusal: %v %s", err, &output)
					}
				} else if !strings.Contains(output.String(), "fix_required") || !strings.Contains(strings.ToLower(output.String()), tc.want) {
					t.Fatalf("missing Markdown refusal: %s", &output)
				}
			}
			if calls != 0 || ledgerSnapshot(t, fixture.clone) != before {
				t.Fatal("status accessed GitHub or mutated the ledger")
			}
		})
	}
}

func TestStatusGuidesRepositoryWithCollidingProjectName(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	path := filepath.Join(fixture.clone, "projects", "widgets", "project.json")
	writeFile(t, path, `{"repository":"other/widgets"}`+"\n")
	runGit(t, fixture.clone, "add", "projects/widgets/project.json")
	runGit(t, fixture.clone, "commit", "-q", "-m", "other repository owns name")
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("colliding Project fell back to forge")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", root, "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	var outcome ledgerOutcome
	if err := json.Unmarshal(output.Bytes(), &outcome); err != nil || outcome.Status != "fix_required" || !strings.Contains(outcome.Repair, "name collision") || !strings.Contains(outcome.Repair, "correct ledger") {
		t.Fatalf("missing collision guidance: %v %s", err, &output)
	}
}

func TestStatusShowsAcceptedProjectInBothFormats(t *testing.T) {
	newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("shown")))
	for _, tc := range []struct {
		format, expected string
	}{
		{"json", `"item":"shown/foundation"`},
		{"markdown", "Work Item: shown/foundation"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Fatal("unattached ledger status should not contact the forge")
				return nil, nil
			}, bytes.NewReader(nil), &output, &output)
			if err := app.Run([]string{"skl", "status", "--repo", root, "--item", "shown/foundation", "--format", tc.format}); err != nil || !strings.Contains(output.String(), tc.expected) {
				t.Fatalf("accepted status: %v %s", err, &output)
			}
		})
	}
}

func TestStatusRefusesMalformedAcceptedProjectWithoutForgeFallback(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	path := filepath.Join(fixture.clone, "projects", "widgets", "project.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "{malformed\n")
	runGit(t, fixture.clone, "add", "projects/widgets/project.json")
	runGit(t, fixture.clone, "commit", "-q", "-m", "damaged Project")
	before := ledgerSnapshot(t, fixture.clone)
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("malformed Project fell back to forge")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", root, "--format", "json"}); err != nil {
		t.Fatal(err)
	}
	var outcome ledgerOutcome
	if err := json.Unmarshal(output.Bytes(), &outcome); err != nil || outcome.Status != "fix_required" || outcome.Repair == "" || ledgerSnapshot(t, fixture.clone) != before {
		t.Fatalf("malformed Project was accepted or changed: %v %s", err, &output)
	}
}
