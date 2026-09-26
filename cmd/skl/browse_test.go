package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

// browseFixture commits widgets and gadgets Projects to a configured ledger
// clone and pushes them, so any later push or commit would be observable.
func browseFixture(t *testing.T) *ledgerFixture {
	t.Helper()
	fixture := newLedgerFixture(t)
	records := map[string]string{
		"projects/widgets/project.json":                   `{"repository": "acme/widgets"}`,
		"projects/widgets/proposals/orders/proposal.json": `{"accepted": "2024-01-01T00:00:00Z", "parent_title": "Order cancellation"}`,
		"projects/widgets/proposals/orders/cancel/state.json": `{"state": "awaiting_review", "title": "Cancel orders", "branch": "feat/cancel",
			"issue": {"repository": "acme/widgets", "number": 11},
			"claim": {"phase": "watchdog", "basis": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "inputs": {"contract": []}}}`,
		"projects/widgets/archive/legacy/proposal.json":      `{"accepted": "2023-01-01T00:00:00Z"}`,
		"projects/widgets/archive/legacy/old/state.json":     `{"state": "superseded", "title": "Old work", "branch": "old"}`,
		"projects/gadgets/project.json":                      `{"repository": "acme/gadgets"}`,
		"projects/gadgets/proposals/tools/proposal.json":     `{"accepted": "2024-01-01T00:00:00Z"}`,
		"projects/gadgets/proposals/tools/hammer/state.json": `{"state": "rework", "title": "Hammer", "branch": "hammer"}`,
	}
	for path, contents := range records {
		writeFile(t, filepath.Join(fixture.clone, filepath.FromSlash(path)), contents)
	}
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "records")
	runGit(t, fixture.clone, "push", "-q")
	return fixture
}

// browseApp is the CLI with a backend factory that fails the test when any
// command constructs a forge.
func browseApp(t *testing.T) (*stageApp, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	factory := func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("a browse command constructed a forge backend")
		return nil, nil
	}
	return newApp(factory, bytes.NewReader(nil), &output, &output), &output
}

func browseQuery(t *testing.T, app *stageApp, output *bytes.Buffer, args ...string) browseOutcome {
	t.Helper()
	output.Reset()
	if err := app.Run(append([]string{"skl", "browse"}, append(args, "--format", "json")...)); err != nil {
		t.Fatalf("browse %v: %v\n%s", args, err, output)
	}
	var outcome browseOutcome
	if err := json.Unmarshal(output.Bytes(), &outcome); err != nil {
		t.Fatalf("decode %q: %v", output, err)
	}
	if outcome.Status == "shown" && outcome.Overview == nil && outcome.Inventory == nil && outcome.Proposal == nil && outcome.Slice == nil {
		t.Fatalf("browse %v showed no result: %s", args, output)
	}
	return outcome
}

func TestBrowseQueriesReadCommittedRecordsWithoutSideEffects(t *testing.T) {
	fixture := browseFixture(t)
	app, output := browseApp(t)
	clone, upstream := ledgerSnapshot(t, fixture.clone), runGitOutput(t, fixture.upstream, "for-each-ref")

	projects := browseQuery(t, app, output, "projects")
	if projects.Status != "shown" || len(projects.Overview.Projects) != 2 || projects.Overview.Projects[1].Name != "widgets" || projects.Overview.Projects[1].ArchivedProposals != 1 {
		t.Fatalf("projects = %+v", projects.Overview)
	}
	ordinary := browseQuery(t, app, output, "project", "--project", "widgets")
	archived := browseQuery(t, app, output, "project", "--project", "widgets", "--include-archived")
	if len(ordinary.Inventory.Proposals) != 1 || len(archived.Inventory.Proposals) != 2 || !archived.Inventory.Proposals[1].Archived {
		t.Fatalf("archive selection: ordinary %+v, included %+v", ordinary.Inventory.Proposals, archived.Inventory.Proposals)
	}
	proposal := browseQuery(t, app, output, "proposal", "--project", "widgets", "--proposal", "legacy")
	if !proposal.Proposal.Proposal.Archived || proposal.Proposal.Proposal.FullyDelivered || proposal.Proposal.Slices[0].Lifecycle != "superseded" {
		t.Fatalf("archived proposal = %+v", proposal.Proposal)
	}
	slice := browseQuery(t, app, output, "slice", "--project", "widgets", "--item", "orders/cancel")
	if slice.Slice.Lifecycle != "awaiting_review" || slice.Slice.Claim == nil || slice.Slice.Claim.Phase != "watchdog" || slice.Slice.Issue.Number != 11 {
		t.Fatalf("slice = %+v", slice.Slice)
	}
	refused := browseQuery(t, app, output, "slice", "--project", "widgets", "--item", "orders/refund")
	if refused.Status != "fix_required" || !strings.Contains(refused.Reason, "orders/refund") || refused.Repair == "" {
		t.Fatalf("unknown slice = %+v", refused)
	}

	output.Reset()
	if err := app.Run([]string{"skl", "browse", "slice", "--project", "widgets", "--item", "orders/cancel"}); err != nil {
		t.Fatal(err)
	}
	for _, fact := range []string{"Lifecycle: Awaiting Review", "Claim: watchdog reservation", "Issue: acme/widgets#11"} {
		if !strings.Contains(output.String(), fact) {
			t.Fatalf("markdown lacks %q:\n%s", fact, output)
		}
	}
	if ledgerSnapshot(t, fixture.clone) != clone || runGitOutput(t, fixture.upstream, "for-each-ref") != upstream {
		t.Fatal("browse queries changed the ledger clone or its upstream")
	}
}

func TestBrowseStartupSelection(t *testing.T) {
	browseFixture(t)
	widgets := sourceRepository(t, "acme", "widgets")
	unregistered := sourceRepository(t, "acme", "other")
	ambiguous := sourceRepository(t, "acme", "widgets")
	runGit(t, ambiguous, "remote", "rename", "origin", "first")
	runGit(t, ambiguous, "remote", "add", "second", "git@github.com:acme/gadgets.git")

	view := func(explicit string, explicitSet bool, location string) tea.Model {
		t.Helper()
		model, err := startBrowser(explicit, explicitSet, location)
		if err != nil {
			t.Fatalf("start browser at %s: %v", location, err)
		}
		sized, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
		return sized
	}
	shows := func(model tea.Model, fragments ...string) {
		t.Helper()
		for _, fragment := range fragments {
			if !strings.Contains(model.View(), fragment) {
				t.Fatalf("view lacks %q:\n%s", fragment, model.View())
			}
		}
	}

	shows(view("", false, widgets), "skl browse › Projects › widgets")
	explicit := view("gadgets", true, widgets)
	shows(explicit, "skl browse › Projects › gadgets")
	explicit, _ = explicit.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	explicit, _ = explicit.Update(tea.KeyMsg{Type: tea.KeyDown})
	explicit, _ = explicit.Update(tea.KeyMsg{Type: tea.KeyEnter})
	shows(explicit, "skl browse › Projects › widgets")

	shows(view("", false, t.TempDir()), "Projects (2)", "the current directory identifies no single GitHub repository")
	shows(view("", false, unregistered), "Projects (2)", "no Project records acme/other")
	shows(view("", false, ambiguous), "Projects (2)", "identifies no single GitHub repository")

	if _, err := startBrowser("nope", true, widgets); err == nil || !strings.Contains(err.Error(), `unknown Project "nope"`) {
		t.Fatalf("invalid explicit selection = %v", err)
	}
	app, output := browseApp(t)
	if err := app.Run([]string{"skl", "browse", "--project", ""}); err == nil {
		t.Fatalf("an empty explicit selection started browsing: %s", output)
	}
}
