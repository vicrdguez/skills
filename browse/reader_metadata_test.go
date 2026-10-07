package browse_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/vicrdguez/skills/browse"
)

// firstDocumentLine is the first line inside the document pane, below its
// title, of a reader narrow enough to show the focused document alone.
func (s *session) firstDocumentLine() string {
	s.t.Helper()
	for _, line := range strings.Split(s.text(), "\n")[1:] {
		if strings.HasPrefix(line, "│") {
			return strings.TrimSpace(strings.Trim(line, "│"))
		}
	}
	s.t.Fatalf("the document pane shows no line:\n%s", s.text())
	return ""
}

// documentPane is the right-hand pane of a wide reader's body: from its
// title to its bottom edge.
func documentPane(text string) string {
	lines := strings.Split(text, "\n")
	var pane []string
	for _, line := range lines[1 : len(lines)-1] {
		if at := strings.LastIndex(line, "│ "); strings.HasPrefix(strings.TrimSpace(line), "│") && at > 0 {
			pane = append(pane, line[at:])
		} else if at := strings.LastIndex(line, "╭─"); at > 0 {
			pane = append(pane, line[at:])
		} else if at := strings.LastIndex(line, "╰"); at > 0 {
			pane = append(pane, line[at:])
		}
	}
	return strings.Join(pane, "\n")
}

func TestReaderDocumentPaneStartsOnTheAuthoredFirstLine(t *testing.T) {
	s := start(t, "widgets")
	review(t, s.root, "pass", 1, "## Review findings\n\nThe cancellation is sound.\n")
	s.model = browse.New(snapshotAfterChange(t, s.root), browse.Options{Project: "widgets"})
	s.send(tea.WindowSizeMsg{Width: 90, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "enter")
	if first := s.firstDocumentLine(); first != "## Review findings" {
		t.Fatalf("the watchdog report's pane starts with %q, want its heading:\n%s", first, s.text())
	}
	s.hides("Schema", "Outcome", "Round", "Candidate head", strings.Repeat("a", 40), "Context at open", "Record metadata")

	// A Human Decision shows its body, and a state record its recorded JSON.
	s.press("esc", "down", "down", "enter")
	if first := s.firstDocumentLine(); first != "The human direction remains available as Markdown." {
		t.Fatalf("the decision's pane starts with %q, want its body:\n%s", first, s.text())
	}
	s.hides("Answered request", "Route:")
	s.press("r", "enter")
	if first := s.firstDocumentLine(); !strings.HasPrefix(first, `{"state": "ready_for_implementation"`) {
		t.Fatalf("the state record's pane starts with %q, want its recorded JSON:\n%s", first, s.text())
	}
	s.hides("Basis", "Claim input")
}

func TestReaderTitleShowsWhatAReaderNeedsAtAGlance(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	s, commits := reviewed(t)
	s.send(tea.WindowSizeMsg{Width: 160, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down", "down")

	latest := s.row("╭─ ", "cancel/watchdog-report.md · current · ", "pass", " · round 3")
	if strings.Contains(ansi.Strip(latest), "HIST") {
		t.Fatalf("the latest report's title is marked historical: %q", ansi.Strip(latest))
	}
	if got := s.styleOf("cancel/watchdog-report.md · current · ", "pass"); got != "1;32" {
		t.Errorf("latest outcome style = %q, want green", got)
	}
	s.press("down")
	s.row("╭─ ", "cancel/watchdog-report.md · HISTORICAL "+commits[1][:12]+" · ", "rework", " · round 2 · newer available")
	if got := s.styleOf("HISTORICAL "+commits[1][:12]+" · ", "rework"); got != "35" {
		t.Errorf("earlier outcome style = %q, want magenta", got)
	}
	if header := ansi.Strip(s.header()); strings.Contains(header, "HIST") || strings.Contains(header, "rework") || strings.Contains(header, "round") {
		t.Fatalf("the header repeats the title: %q", header)
	}

	// Each other kind shows its own facts: a route, a Claim's phase, or that
	// its metadata is unreadable.
	s = start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 160, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "down")
	s.row("╭─ ", "cancel/decision.md · current · route implement")
	s.press("r", "enter")
	s.row("╭─ ", "cancel/state.json · HISTORICAL", "▸ watchdog claim")
	s.press("esc", "esc", "esc", "up")
	s.row("╭─ ", "cancel/watchdog-report.md · current · ", "! metadata unreadable")
}

func TestReaderNarrowTitleKeepsEveryFactOnFurtherLines(t *testing.T) {
	s, commits := reviewed(t)
	narrow := tea.WindowSizeMsg{Width: 40, Height: 14}
	s.send(narrow)
	s.press("enter", "down", "enter", "d", "down", "down", "down", "down", "enter")
	s.within(narrow)
	s.row("╭─ ▶ watchdog-report.md · HIST " + commits[1][:7] + " ")
	s.row("├─ rework · round 2 · newer ")
	if first := s.firstDocumentLine(); first != "Review 2 of cancellation." {
		t.Fatalf("below its title, the narrow pane starts with %q, want the report's first line:\n%s", first, s.text())
	}
	// Narrower still, the name gives way from its start to keep its identity
	// beside it.
	narrower := tea.WindowSizeMsg{Width: 30, Height: 14}
	s.send(narrower)
	s.within(narrower)
	s.row("╭─ ▶ …", ".md · HIST "+commits[1][:7]+" ")
	s.row("├─ rework · round 2 ")
	s.row("├─ newer ")
	s.send(narrow)
	// The navigator's title keeps the Slice's facts the same way.
	s.press("esc")
	s.row("╭─ ▶ Slice cancel · ", "Awaiting Review")
	s.row("├─ ", "watchdog claim")
}

func TestReaderTitleMarksADiagnosticOnReadableMetadata(t *testing.T) {
	s := start(t, "widgets", func(root string) {
		write(t, root, cancelPath+"state.json", `{"state": "ready_for_implementation", "title": "Cancel orders", "branch": "feat/cancel",
			"claim": {"phase": "implement", "basis": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "inputs": {"contract": [{"commit": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "path": "notes/elsewhere.md"}], "implement": {"commit": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "path": "notes/other.md"}}}}`)
	})
	base := gitOutput(t, s.root, "rev-list", "--max-parents=0", "HEAD")
	s.send(tea.WindowSizeMsg{Width: 160, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down", "enter", "r", "enter")
	s.row("╭─ ", "cancel/state.json · HISTORICAL "+base[:12], "▸ implement claim", "! 2 diagnostics ─")
	// The footer leads with the first diagnostic's cause; the details list
	// each.
	s.row("! Metadata diagnostic 1 of 2: Claim contract reference is not a ledger record document path")
	s.hides("Claim implement reference")
	s.press("m")
	s.row("Diagnostics")
	s.row("│ ! slice widgets/orders/cancel: Claim contract reference is not a ledger record document path")
	s.row("│ ! slice widgets/orders/cancel: Claim implement reference is not a ledger record document path")
}

func TestReaderDetailsPanelShowsTheRemainingMetadataAndKeepsThePlace(t *testing.T) {
	s, commits := reviewed(t)
	base := gitOutput(t, s.root, "rev-list", "--max-parents=0", "HEAD")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 50})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "tab", "pgdown")
	s.shows("scrolled ", "m details")
	before := s.model.View()

	s.press("m")
	s.shows("▶ Details · cancel/watchdog-report.md",
		"Commit: "+commits[2], "Path: "+cancelPath+"watchdog-report.md",
		"Schema: 1", "Source repository revisions (not ledger revisions)",
		"Candidate head: "+strings.Repeat("a", 40), "Integration Target: "+strings.Repeat("b", 40), "Reviewed revision: "+strings.Repeat("a", 40),
		"Consumed ledger references", "claim", "contract", "implement",
		"Commit: "+base, "Path: "+cancelPath+"state.json", "Path: "+cancelPath+"intent.md", "Path: "+cancelPath+"implement-report.md",
		"m close details")
	s.hides("Reviewed line.")
	s.press("m")
	if after := s.model.View(); after != before {
		t.Fatalf("opening and closing the details changed the reader:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	// Keys that would select or focus elsewhere wait for the panel to close.
	s.press("m", "pgdown", "enter", "tab", "v", "esc")
	if after := s.model.View(); after != before {
		t.Fatalf("the details changed the reader's place:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	// A narrow terminal shows the panel alone, scrolling within it, and it
	// holds every fact a shortened title can lose.
	narrow := tea.WindowSizeMsg{Width: 40, Height: 14}
	s.send(narrow)
	s.press("m")
	s.within(narrow)
	s.shows("▶ Details · cancel/watchdog-", "Ledger document", "scrolled 0%")
	s.press("pgdown")
	s.hides("scrolled 0%")
	s.shows("Outcome: pass", "Review round: 3")
	s.press("?")
	s.shows("m details")
}

func TestReaderDetailsShowADecisionsAndAClaimsRecordedMetadata(t *testing.T) {
	s := start(t, "widgets")
	base := gitOutput(t, s.root, "rev-list", "--max-parents=0", "HEAD")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 50})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "down", "enter", "m")
	s.shows("Schema: 1", "Route: implement", "Project: widgets", "Slice: orders/cancel", "Answered request",
		"Commit: "+base, "Path: "+cancelPath+"state.json")

	// The panel follows the displayed document along a followed reference.
	s.press("r", "enter")
	s.shows("▶ Details · cancel/state.json", "Commit: "+base, "Claim phase: watchdog", "Claim basis (ledger revision): "+strings.Repeat("3c", 20))
	// The current Claim's state record lists its inputs.
	s = start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 50})
	s.press("enter", "down", "enter", "r", "enter", "m")
	s.shows("Claim input references", "contract", "Path: "+cancelPath+"intent.md")
}

func TestReaderNavigatorTitleFollowsRefreshWhileTheDocumentStaysPinned(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	s, _ := reviewed(t)
	state := cancelPath + "state.json"
	changeFixture(t, s.root, state, `"state": "awaiting_review"`, `"state": "rework"`)
	commitFixture(t, s.root, "rework")
	s.model = browse.New(snapshotAfterChange(t, s.root), browse.Options{Project: "widgets"})
	s.send(tea.WindowSizeMsg{Width: 160, Height: 30})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "down", "tab", "pgdown")
	s.row("╭─ ", "↻ Rework", "▸ watchdog claim", "╭─ ", "▶ cancel/watchdog-report.md · HISTORICAL")
	if got := s.styleOf("╭─ ", "↻ Rework"); got != "35" {
		t.Errorf("navigator lifecycle style = %q, want the Rework colour", got)
	}
	if got := s.styleOf("↻ Rework", "▸ watchdog claim"); got != "36" {
		t.Errorf("navigator Claim style = %q, want the Claim colour", got)
	}
	before := documentPane(s.text())

	changeFixture(t, s.root, state, `"state": "rework"`, `"state": "ready_for_merge"`)
	commitFixture(t, s.root, "ready for merge")
	s.press("R")
	s.row("╭─ ", "● Ready for Merge", "▸ watchdog claim", "╭─ ")
	if after := documentPane(s.text()); after != before {
		t.Fatalf("refresh changed the displayed document, its title or its scroll position:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	// A Slice whose state cannot be read has unknown facts.
	s = start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 160, Height: 30})
	s.press("enter", "enter", "d")
	s.row("╭─ ", "▶ Slice broken · ", "! lifecycle unknown")
	s.row("├─ ", "claim unknown")
}

func TestReaderShowsUnreadableMetadataAsItsRecordedText(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 90, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "enter")
	if first := s.firstDocumentLine(); first != "---" {
		t.Fatalf("the unreadable report's pane starts with %q, want its recorded frontmatter:\n%s", first, s.text())
	}
	s.shows("schema: unsupported", "unknown_field: true", "## Watchdog bytes remain readable",
		"! Metadata diagnostic:", "watchdog report metadata is")
	title := ansi.Strip(s.row("╭─ ▶ cancel/watchdog-report.md · current · ! metadata unreadable ─"))
	for _, invented := range []string{"pass", "rework", "round", "awaiting"} {
		if strings.Contains(title, invented) {
			t.Fatalf("the unreadable report's title %q shows %q", title, invented)
		}
	}
	s.send(tea.WindowSizeMsg{Width: 160, Height: 40})
	s.press("m")
	s.shows("Commit: "+gitOutput(t, s.root, "rev-parse", "HEAD"), "Path: "+cancelPath+"watchdog-report.md",
		"Diagnostics", "watchdog report metadata is unreadable")
	s.hides("Schema:", "Source repository revisions")
}

func TestReaderNarrowUnreadableDocumentAndDetailsStayReadable(t *testing.T) {
	s := start(t, "widgets")
	narrow := tea.WindowSizeMsg{Width: 40, Height: 14}
	s.send(narrow)
	s.press("enter", "down", "enter", "d", "down", "down", "down", "enter")
	s.within(narrow)
	if first := s.firstDocumentLine(); first != "---" {
		t.Fatalf("the narrow unreadable report starts with %q, want its recorded frontmatter:\n%s", first, s.text())
	}
	s.shows("schema: unsupported", "! Metadata diagnostic: ")
	s.press("pgdown")
	s.shows("Malformed metadata does not hide")

	// The details panel scrolls to the diagnostic in full.
	s.press("m")
	s.shows("Ledger document", "Commit:")
	s.press("pgdown", "pgdown")
	s.within(narrow)
	s.shows("field unknown_field not", "documented schema-1 fields with", "exact types", "scrolled 100%")
}
