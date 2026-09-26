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
	write(t, root, "projects/widgets/proposals/orders/cancel/state.json", `{"state": "awaiting_review", "title": "Cancel orders", "branch": "feat/cancel",
		"issue": {"repository": "acme/widgets", "number": 11}, "submission": {"repository": "acme/widgets", "number": 12},
		"claim": {"phase": "watchdog", "basis": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "inputs": {"contract": []}}}`)
	write(t, root, "projects/widgets/proposals/orders/refund/state.json", `{"state": "ready_for_implementation", "title": "Refund orders", "branch": "feat/refund"}`)
	write(t, root, "projects/widgets/proposals/orders/broken/state.json", `{broken`)
	write(t, root, "projects/widgets/archive/legacy/proposal.json", `{"accepted": "2023-01-01T00:00:00Z"}`)
	write(t, root, "projects/widgets/archive/legacy/old/state.json", `{"state": "merged", "title": "Old work", "branch": "old"}`)
	write(t, root, "projects/gadgets/project.json", `{"repository": "acme/gadgets"}`)
	write(t, root, "projects/gadgets/proposals/tools/proposal.json", `{"accepted": "2024-01-01T00:00:00Z"}`)
	write(t, root, "projects/gadgets/proposals/tools/hammer/state.json", `{"state": "rework", "title": "Hammer", "branch": "hammer"}`)
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "records")
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

// fits presses each key sequence in turn at both terminal sizes and fails
// when a view overflows the terminal.
func fits(t *testing.T, sequences [][]string) []*session {
	t.Helper()
	var sessions []*session
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 14}, {Width: 140, Height: 30}} {
		s := start(t, "widgets")
		s.send(size)
		for _, keys := range sequences {
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
		sessions = append(sessions, s)
	}
	return sessions
}

func TestBrowserFitsNarrowAndWideTerminals(t *testing.T) {
	for _, s := range fits(t, [][]string{nil, {"enter"}, {"down", "enter"}}) {
		s.shows("Slice: orders/cancel")
	}
}

func downs(count int) []string {
	return strings.Fields(strings.Repeat("down ", count))
}

func TestBrowserNavigatesFromClaimFactToSlices(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("f")
	s.shows("skl browse › Find slices › Facts", "✓ Any lifecycle (3)", "Awaiting Review (1)", "watchdog claim (1)", "unclaimed (1)",
		"! Counted only under Any: 1 with unknown lifecycle, 1 with unknown claim")

	s.press(downs(10)...)
	s.shows("Finds: any lifecycle · watchdog claim in Project widgets")
	s.press("enter")
	s.shows("skl browse › Find slices", "Finding: any lifecycle · watchdog claim in Project widgets",
		"Proposal orders", "orders/cancel — Awaiting Review · watchdog claim — Cancel orders",
		"Undecided: unknown facts", "! orders/broken — lifecycle unknown · claim unknown",
		"! 1 matching slice; 1 undecided by unknown facts; incomplete")
	s.hides("orders/refund", "tools/hammer")

	s.press("enter")
	s.shows("skl browse › Find slices › widgets › orders/cancel", "Lifecycle: Awaiting Review", "Claim: watchdog reservation")
	s.press("esc")
	s.shows("Finding: any lifecycle · watchdog claim")
	s.press("esc")
	s.shows("skl browse › Projects › widgets", "Proposals (1)")
}

func TestBrowserCombinesNameSearchWithFactsScopeAndGrouping(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("/", "q", "u", "i", "t")
	s.shows("Search names: quit█")
	s.press("esc")
	s.hides("Search names:")

	s.press("/", "ORDERS", "enter")
	s.shows(`Finding: any lifecycle · any claim · name contains "ORDERS" in Project widgets`, "3 matching slices", "orders/refund")
	s.press("f", "down", "enter")
	s.shows(`Finding: Ready for Implementation · any claim · name contains "ORDERS"`, "orders/refund — Ready for Implementation · unclaimed",
		"! orders/broken", "1 matching slice; 1 undecided")
	s.hides("orders/cancel")

	s.press("/", "hammer", "enter")
	s.shows(`Finding: Ready for Implementation · any claim · name contains "hammer" in Project widgets`, "No Slice is known to match; 1 undecided by unknown facts")
	s.press("w")
	s.shows("in every Project", "No Slice is known to match")
	s.hides("tools/hammer")
	s.press("f", "k", "enter")
	s.shows("any lifecycle · any claim", "gadgets · Proposal tools", "tools/hammer — Rework · unclaimed — Hammer", "widgets · Undecided")
	s.hides("orders/refund")
	s.press("g")
	s.shows("grouped by lifecycle", "gadgets · Rework", "widgets · Undecided")
	s.press("enter")
	s.shows("skl browse › Find slices › gadgets › tools/hammer", "Lifecycle: Rework")
}

func TestBrowserTellsEmptyResultsFromUnknownOnes(t *testing.T) {
	s := start(t, "gadgets")
	s.press("/", "zzz", "enter")
	s.shows("No Slice matches this selection.")
	s.hides("incomplete")

	s.press("w")
	s.shows("No Slice is known to match; 1 undecided by unknown facts; incomplete", "widgets · Undecided", "orders/broken")
	s.press("a", "/", "old", "enter")
	s.shows("archived shown", "legacy/old [archived] — Merged · unclaimed — Old work")
}

func TestBrowserFitsFindingScreens(t *testing.T) {
	for _, s := range fits(t, [][]string{{"f"}, {"enter"}, {"w", "g"}, {"/", "orders"}, {"enter"}}) {
		s.shows("Find slices", "Finding:", "> ")
	}
}

func TestBrowserKeepsItsContextAfterOpeningAResultElsewhere(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("enter", "/", "hammer", "enter")
	s.shows(`name contains "hammer" in Project widgets`)
	s.press("w", "enter")
	s.shows("skl browse › Find slices › gadgets › tools/hammer", "Lifecycle: Rework")
	s.press("f")
	s.shows("skl browse › Find slices › Facts")
	s.press("esc")
	s.shows(`name contains "hammer" in every Project`, "tools/hammer")
	s.press("w")
	s.shows(`name contains "hammer" in Project widgets`)
	s.press("esc")
	s.shows("skl browse › Projects › widgets › orders", "Slices (3)")
}
