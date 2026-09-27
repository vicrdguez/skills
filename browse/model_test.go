package browse_test

// Interaction tests drive the browser through key and resize messages and
// observe its rendered view and external-browser requests over a real
// committed ledger fixture.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicrdguez/skills/browse"
	"github.com/vicrdguez/skills/ledger"
)

func write(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// fixtureLedger commits two Projects: widgets with an attached, watchdog
// claimed Slice and an archived Proposal, and gadgets with one Slice.
func fixtureLedger(t *testing.T) *ledger.Snapshot {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.name", "Ledger")
	git(t, root, "config", "user.email", "ledger@example.com")
	write(t, root, "projects/widgets/project.json", `{"repository": "acme/widgets"}`)
	write(t, root, "projects/widgets/proposals/orders/proposal.json", `{"accepted": "2024-01-01T00:00:00Z", "parent_title": "Order cancellation", "parent_issue": {"repository": "acme/widgets", "number": 10}}`)
	write(t, root, "projects/widgets/proposals/orders/proposal.md", "# Order cancellation\n\nProposal description stays readable.\n")
	cancelPath := "projects/widgets/proposals/orders/cancel"
	write(t, root, cancelPath+"/intent.md", "# Intent\n\nCancellation intent.\n")
	write(t, root, cancelPath+"/behavior.md", "# Behavior\n\nCancellation behavior.\n")
	write(t, root, "projects/widgets/proposals/orders/refund/intent.md", "# Intent\n")
	write(t, root, "projects/widgets/proposals/orders/refund/behavior.md", "# Behavior\n")
	write(t, root, "projects/widgets/proposals/orders/broken/intent.md", "# Intent\n")
	write(t, root, "projects/widgets/proposals/orders/broken/behavior.md", "# Behavior\n")
	write(t, root, "projects/widgets/proposals/orders/cancel/state.json", `{"state": "ready_for_implementation", "title": "Cancel orders", "branch": "feat/cancel",
		"issue": {"repository": "acme/widgets", "number": 11}, "submission": {"repository": "acme/widgets", "number": 12}, "decision": true,
		"dependencies": ["orders/refund"], "claim": {"phase": "watchdog", "basis": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "inputs": {"contract": []}}}`)
	write(t, root, "projects/widgets/proposals/orders/refund/state.json", `{"state": "ready_for_implementation", "title": "Refund orders", "branch": "feat/refund"}`)
	write(t, root, "projects/widgets/proposals/orders/broken/state.json", `{broken`)
	write(t, root, "projects/widgets/archive/legacy/proposal.json", `{"accepted": "2023-01-01T00:00:00Z"}`)
	write(t, root, "projects/widgets/archive/legacy/proposal.md", "# Legacy proposal\n\nArchived description.\n")
	write(t, root, "projects/widgets/archive/legacy/old/state.json", `{"state": "merged", "title": "Old work", "branch": "old"}`)
	write(t, root, "projects/widgets/archive/legacy/old/intent.md", "# Intent\n")
	write(t, root, "projects/widgets/archive/legacy/old/behavior.md", "# Behavior\n")
	write(t, root, "projects/gadgets/project.json", `{"repository": "acme/gadgets"}`)
	write(t, root, "projects/gadgets/proposals/tools/proposal.json", `{"accepted": "2024-01-01T00:00:00Z"}`)
	write(t, root, "projects/gadgets/proposals/tools/proposal.md", "# Tools proposal\n")
	write(t, root, "projects/gadgets/proposals/tools/hammer/state.json", `{"state": "rework", "title": "Hammer", "branch": "hammer"}`)
	write(t, root, "projects/gadgets/proposals/tools/hammer/intent.md", "# Intent\n")
	write(t, root, "projects/gadgets/proposals/tools/hammer/behavior.md", "# Behavior\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "accepted records")
	base := gitOutput(t, root, "rev-parse", "HEAD")
	claimRef := ledger.Reference{Commit: base, Path: cancelPath + "/state.json"}
	contractRef := []ledger.Reference{{Commit: base, Path: cancelPath + "/intent.md"}, {Commit: base, Path: cancelPath + "/behavior.md"}}
	write(t, root, cancelPath+"/state.json", `{"state": "awaiting_review", "title": "Cancel orders", "branch": "feat/cancel",
		"issue": {"repository": "acme/widgets", "number": 11}, "submission": {"repository": "acme/widgets", "number": 12}, "decision": true,
		"dependencies": ["orders/refund"], "claim": {"phase": "watchdog", "basis": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "inputs": {"contract": []}}}`)
	implementation, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
		Schema: 1, Outcome: "awaiting_review",
		Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40)},
		Ledger: ledger.ReportInputs{
			Claim: claimRef, Contract: contractRef,
			Watchdog: &ledger.Reference{Commit: strings.Repeat("c", 40), Path: cancelPath + "/watchdog-report.md"},
		},
	}, "## Implementation evidence\n\nThe recorded implementation evidence is readable.\n")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, cancelPath+"/implement-report.md", string(implementation))
	write(t, root, cancelPath+"/watchdog-report.md", "---\nschema: unsupported\nunknown_field: true\n---\n\n## Watchdog bytes remain readable\n\nMalformed metadata does not hide this body.\n")
	decision, err := ledger.FormatDecision(ledger.DecisionRecord{
		Schema: 1, Project: "widgets", Item: "orders/cancel", AnsweredRequest: claimRef, Route: "implement",
	}, "The human direction remains available as Markdown.\n")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, cancelPath+"/decision.md", string(decision))
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "phase evidence")
	store, err := ledger.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

type session struct {
	t      *testing.T
	model  tea.Model
	opened []string
}

func start(t *testing.T, project string) *session {
	s := &session{t: t}
	s.model = browse.New(fixtureLedger(t), browse.Options{Project: project, Open: func(url string) error {
		s.opened = append(s.opened, url)
		return nil
	}})
	return s
}

// press sends keys and runs any command they return, as the program would.
func (s *session) press(keys ...string) {
	s.t.Helper()
	for _, name := range keys {
		var msg tea.KeyMsg
		switch name {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
		}
		s.send(msg)
	}
}

func (s *session) send(msg tea.Msg) {
	var command tea.Cmd
	s.model, command = s.model.Update(msg)
	if command != nil {
		s.model, _ = s.model.Update(command())
	}
}

func (s *session) shows(fragments ...string) {
	s.t.Helper()
	view := s.model.View()
	for _, fragment := range fragments {
		if !strings.Contains(view, fragment) {
			s.t.Fatalf("view lacks %q:\n%s", fragment, view)
		}
	}
}

func (s *session) hides(fragments ...string) {
	s.t.Helper()
	view := s.model.View()
	for _, fragment := range fragments {
		if strings.Contains(view, fragment) {
			s.t.Fatalf("view unexpectedly shows %q:\n%s", fragment, view)
		}
	}
}

func TestBrowserNavigatesProjectsProposalsAndSliceFacts(t *testing.T) {
	s := start(t, "")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.shows("Projects (2)", "> gadgets — acme/gadgets — 1 slice", "! widgets — acme/widgets")

	s.press("down", "enter")
	s.shows("Project widgets (acme/widgets)", "Proposals (1)", "! orders — 0 of 3 Merged · 1 unknown", "! Incomplete")
	s.hides("legacy")

	s.press("enter")
	s.shows("Slices (3)", "Order cancellation", "cancel — Awaiting Review · watchdog claim", "refund — Ready for Implementation · unclaimed", "! broken — lifecycle unknown · claim unknown")

	s.press("down", "enter")
	s.shows("Lifecycle: Awaiting Review", "Claim: watchdog reservation", "a reservation, not a running",
		"Branch: feat/cancel", "Issue: acme/widgets#11", "Pull request: acme/widgets#12")
	s.hides("running worker status")

	s.press("esc", "k", "enter")
	s.shows("Lifecycle: unknown", "Claim: unknown", "state.json is unreadable")

	s.press("s")
	s.shows("Projects (2)", "> ! widgets — acme/widgets — 3 slices · 1 claimed")
	s.press("k", "enter", "enter", "enter")
	s.shows("Slice: tools/hammer", "Lifecycle: Rework")
	if len(s.opened) != 0 {
		t.Fatalf("navigation opened links: %v", s.opened)
	}
}

func TestBrowserStartsAtExplicitProjectAndSwitches(t *testing.T) {
	s := start(t, "gadgets")
	s.shows("skl browse › Projects › gadgets", "tools")
	s.press("s", "down", "enter")
	s.shows("skl browse › Projects › widgets", "orders")
}

func TestBrowserShowsArchivedProposalsOnRequest(t *testing.T) {
	s := start(t, "widgets")
	s.shows("archived hidden", "Proposals (1)")
	s.hides("legacy")
	s.press("a")
	s.shows("archived shown", "legacy [archived] — 1 of 1 Merged · fully delivered")
}

func TestBrowserReadsReportsFollowsExactHistoricalReferencesAndPreservesScroll(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 100, Height: 50})
	s.press("enter", "down", "enter", "d", "down", "down", "enter")
	s.shows("Implementation report", "Outcome: awaiting_review", "Source repository revisions", "Ledger input references",
		"The recorded implementation evidence is readable", "current lifecycle: Awaiting Review")

	s.send(tea.WindowSizeMsg{Width: 88, Height: 16})
	s.send(tea.KeyMsg{Type: tea.KeyPgDown})
	if !strings.Contains(s.model.View(), "scrolled ") {
		t.Fatalf("document did not scroll:\n%s", s.model.View())
	}
	if !strings.Contains(s.model.View(), "scrolled ") {
		t.Fatalf("resize reset document scroll:\n%s", s.model.View())
	}

	s.press("r", "down", "down", "down", "enter")
	s.shows("Exact ledger reference unavailable", strings.Repeat("c", 40), "watchdog-report.md", "no substitute was opened")
	s.hides("Watchdog bytes remain readable")
	s.press("esc", "r", "enter")
	s.shows("HISTORICAL", "current lifecycle: Awaiting Review", "current Claim: watchdog reservation", "current Dependencies: orders/refund (Ready for Implementation)")
	s.send(tea.KeyMsg{Type: tea.KeyPgDown})
	s.shows("ready_for_implementation")
	s.press("esc")
	s.shows("Latest implementation", "scrolled ", "current committed ledger")

	// The latest malformed report remains selectable and readable, while its
	// schema failure is visible and never supplies invented metadata.
	s.press("esc")
	s.send(tea.WindowSizeMsg{Width: 100, Height: 50})
	s.press("down", "enter")
	s.shows("Metadata diagnostic", "watchdog report metadata is", "unreadable", "Watchdog bytes remain readable", "watchdog reservation")
	s.hides("Outcome:")
	if len(s.opened) != 0 {
		t.Fatalf("document navigation opened external URLs: %v", s.opened)
	}
}

func TestBrowserFollowsDecisionAndClaimReferencesWithoutChangingSliceContext(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 100, Height: 50})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "down", "enter")
	s.shows("Human Decision", "Route: implement", "The human direction remains available as Markdown", "Answered request:")
	s.press("r", "enter")
	s.shows("HISTORICAL", "ready_for_implementation", "current lifecycle: Awaiting", "current Claim: watchdog reservation")
	s.press("esc", "esc", "esc")
	s.shows("Slice: orders/cancel", "Lifecycle: Awaiting Review", "Claim state reference:")
	s.press("r", "enter")
	s.shows("projects/widgets/proposals/orders/cancel/state.json", "lifecycle: Awaiting Review", "current Claim: watchdog reservation", "Dependencies: orders/refund (Ready for Implementation)")
	s.press("esc", "esc")
	s.shows("Slice: orders/cancel", "Lifecycle: Awaiting Review", "Depends on: orders/refund (Ready for Implementation)")
}

func TestBrowserReadsArchivedDocumentsAndNamesAbsentOptionalRecords(t *testing.T) {
	s := start(t, "widgets")
	s.press("a", "down", "enter", "d", "enter")
	s.shows("Archived description", "archived Proposal", "Legacy proposal")

	s = start(t, "widgets")
	s.press("enter", "down", "down", "enter", "d")
	s.shows("Latest implementation report: not yet available", "Latest watchdog report: not yet available", "Active Human Decision: not available")
	s.hides("Metadata diagnostic", "malformed record")
}

func TestBrowserOpensRecordedAttachmentsOnlyOnExplicitAction(t *testing.T) {
	s := start(t, "widgets")
	s.press("enter")
	s.press("i")
	s.press("down", "enter")
	if len(s.opened) != 1 || s.opened[0] != "https://github.com/acme/widgets/issues/10" {
		t.Fatalf("proposal parent issue opened %v", s.opened)
	}
	s.press("i", "p")
	want := []string{"https://github.com/acme/widgets/issues/10", "https://github.com/acme/widgets/issues/11", "https://github.com/acme/widgets/pull/12"}
	if strings.Join(s.opened, " ") != strings.Join(want, " ") {
		t.Fatalf("opened %v, want %v", s.opened, want)
	}
	s.shows("Requested https://github.com/acme/widgets/pull/12 in the external browser")

	s.press("esc", "down", "enter", "p")
	if len(s.opened) != 3 {
		t.Fatalf("a Slice without a recorded pull request opened %v", s.opened)
	}
	s.shows("No recorded pull request attachment to open")
}

func TestBrowserFitsNarrowAndWideTerminals(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 14}, {Width: 140, Height: 30}} {
		s := start(t, "widgets")
		s.send(size)
		for _, keys := range [][]string{nil, {"enter"}, {"down", "enter"}} {
			s.press(keys...)
			view := s.model.View()
			if height := lipgloss.Height(view); height > size.Height {
				t.Fatalf("%dx%d view is %d lines tall:\n%s", size.Width, size.Height, height, view)
			}
			for _, line := range strings.Split(view, "\n") {
				if width := lipgloss.Width(line); width > size.Width {
					t.Fatalf("%dx%d view line is %d wide: %q", size.Width, size.Height, width, line)
				}
			}
		}
		s.shows("Slice: orders/cancel")
	}
}
