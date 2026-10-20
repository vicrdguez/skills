package browse_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Parent-owned temporary directories survive all parallel children. These
// cases only read the same committed data; each startSnapshot creates private
// model, navigation and callback state. Bespoke fixtures and history writers
// continue to use start with their own repositories.
func TestReadOnlyPresentation(t *testing.T) {
	t.Parallel()
	snapshot := fixtureLedger(t)

	t.Run("SessionsAndRefreshHistoryStayIndependent", func(t *testing.T) {
		t.Parallel()
		reader := startSnapshot(t, "widgets", snapshot)
		other := startSnapshot(t, "widgets", snapshot)
		initial := other.text()
		reader.send(tea.WindowSizeMsg{Width: 140, Height: 120})
		reader.press("enter", "down", "enter", "i")
		if len(reader.opened) != 1 || reader.opened[0] != "https://github.com/acme/widgets/issues/11" {
			t.Fatalf("issue action opened %v", reader.opened)
		}
		if other.text() != initial || len(other.opened) != 0 {
			t.Fatal("navigation or external-browser requests escaped into another session")
		}

		writer := start(t, "widgets")
		writer.send(tea.WindowSizeMsg{Width: 140, Height: 120})
		writer.press("enter", "down", "enter", "d", "down", "down", "enter")
		changeFixture(t, writer.root, "projects/widgets/proposals/orders/cancel/implement-report.md",
			"The recorded implementation evidence is readable.", "Independent refresh evidence.")
		commitFixture(t, writer.root, "advance private report history")
		writer.press("R")
		writer.shows("Newer document available", "implement-report.md · HISTORICAL")
		writer.hides("Independent refresh evidence")
		writer.press("esc", "enter")
		writer.shows("Independent refresh evidence", "implement-report.md · current")

		reader.press("R", "d", "down", "down", "enter")
		reader.shows("The recorded implementation evidence is readable", "implement-report.md · current")
		reader.hides("Independent refresh evidence", "Newer document available", "HISTORICAL")
		other.press("R")
		if other.text() != initial || len(other.opened) != 0 {
			t.Fatal("refresh history escaped into the untouched presentation session")
		}
	})

	t.Run("ListAndPreviewPanesSitSideBySideWhenWideAndStackWhenNarrow", func(t *testing.T) {
		t.Parallel()
		for _, size := range []tea.WindowSizeMsg{{Width: 140, Height: 40}, {Width: 40, Height: 24}} {
			s := startSnapshot(t, "widgets", snapshot)
			s.send(size)
			s.press("enter")
			s.within(size)
			s.shows("Delivery: unknown: 0 of 3 Merged", "! Incomplete")
			list, bottom, preview := s.lineIndex("╭─ Slices (3) ─"), s.lineIndex(" 1/3 ─╯"), s.lineIndex("╭─ Slice ─")
			beside, below := list == preview && list < bottom, list < bottom && bottom < preview
			if (size.Width >= 100 && !beside) || (size.Width < 100 && !below) {
				t.Fatalf("%dx%d: list pane from line %d to %d, preview pane from line %d:\n%s", size.Width, size.Height, list, bottom, preview, s.model.View())
			}
			s.row("│", "Slice  orders/broken")

			s.press("down")
			s.row("╰", "2/3", "╯")
			s.row("│", "Slice  orders/cancel", "│")
		}
	})

	t.Run("ShortStackedTerminalKeepsTheListOverThePreview", func(t *testing.T) {
		t.Parallel()
		size := tea.WindowSizeMsg{Width: 40, Height: 12}
		s := startSnapshot(t, "widgets", snapshot)
		s.send(size)
		s.press("enter", "down")
		s.within(size)
		s.shows("Delivery: unknown", "╭─ Slices (3) ─", " 2/3 ─╯")
		s.row("> ", "cancel")
	})

	t.Run("ShortStackedListShowsTheWholeSelectedRow", func(t *testing.T) {
		t.Parallel()
		size := tea.WindowSizeMsg{Width: 40, Height: 16}
		s := startSnapshot(t, "widgets", snapshot)
		s.send(size)
		s.press("enter", "down")
		s.within(size)
		s.shows(" 2/3 ─╯")
		if entry := strings.Join(s.listEntries("broken", "cancel", "refund")["cancel"], "\n"); !strings.Contains(entry, "> ") ||
			!strings.Contains(entry, "◐ Awaiting Review") || !strings.Contains(entry, "▸ watchdog") {
			t.Fatalf("the selected row lacks its lifecycle or Claim: %q\n%s", entry, s.model.View())
		}
	})

	t.Run("SlicePreviewAndDetailShowTheSameSectionedFacts", func(t *testing.T) {
		t.Parallel()
		s := startSnapshot(t, "widgets", snapshot)
		s.send(tea.WindowSizeMsg{Width: 140, Height: 80})
		s.press("enter", "down")
		s.row("> ", "cancel", "◐ Awaiting Review")
		preview := paneText(s.text(), "Slice")

		s.press("enter")
		s.shows("skl browse › Projects › widgets › orders › cancel")
		detail := paneText(s.text(), "Slice")
		if preview == "" || preview != strings.ReplaceAll(detail, ">", "") {
			t.Fatalf("preview and detail facts differ:\npreview %s\ndetail  %s", preview, detail)
		}

		sections := []string{"Status", "Lifecycle◐AwaitingReview", "Claim▸watchdogreservation", "Relationships",
			"Dependsonlegacy/old[archived]—Oldwork✓Merged·satisfied", "Links", "Pullrequestacme/widgets#12",
			"Documents", "Implementationawaiting_review"}
		at := 0
		for _, section := range sections {
			next := strings.Index(preview[at:], section)
			if next < 0 {
				t.Fatalf("preview lacks %q after offset %d: %s", section, at, preview)
			}
			at += next + len(section)
		}
	})

	t.Run("FoundSlicePreviewShowsItsFacts", func(t *testing.T) {
		t.Parallel()
		s := startSnapshot(t, "widgets", snapshot)
		s.send(tea.WindowSizeMsg{Width: 140, Height: 60})
		s.press("/", "cancel", "enter")
		s.row("> ", "orders/cancel")
		s.fact("Lifecycle", "◐ Awaiting Review")
		s.fact("Depends on", "legacy/old [archived] — Old work  ✓ Merged")
		s.fact("Project", "widgets (acme/widgets)")
		s.fact("Implementation", "awaiting_review")
	})

	t.Run("LongSliceDetailKeepsHintsAndScrollPositionOutOfItsFacts", func(t *testing.T) {
		t.Parallel()
		s := startSnapshot(t, "widgets", snapshot)
		s.send(tea.WindowSizeMsg{Width: 80, Height: 16})
		s.press("enter", "down", "enter", "down", "down")
		lines := strings.Split(s.text(), "\n")
		if header := lines[0]; strings.Contains(header, "scrolled") || strings.Contains(header, "%") {
			t.Fatalf("the header repeats the scroll position: %q", header)
		}
		border := ""
		for _, line := range lines {
			if strings.HasPrefix(line, "╰") {
				border = line
			}
		}
		if !strings.Contains(border, "scrolled ") || strings.Contains(border, "scrolled 0%") {
			t.Fatalf("the pane border lacks the scroll position: %q\n%s", border, s.text())
		}
		s.shows("d documents", "? help")

		s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
		s.shows("d documents", "r claim reference")
		facts := strings.ToLower(paneText(s.text(), "Slice"))
		for _, hint := range []string{"press", "dtoread", "rfollows", "toselect"} {
			if strings.Contains(facts, hint) {
				t.Fatalf("the facts tell the human to use %q: %s", hint, facts)
			}
		}
	})
}

func TestReadOnlySpectrumPresentation(t *testing.T) {
	t.Parallel()
	snapshot := fixtureLedger(t, spectrum(t))

	t.Run("SliceRowsAlignNameLifecycleAndClaimColumns", func(t *testing.T) {
		t.Parallel()
		s := startSnapshot(t, "gadgets", snapshot)
		s.send(tea.WindowSizeMsg{Width: 80, Height: 40})
		s.press("enter")
		s.shows("Proposal spectrum", "Slices (9)")
		// column is the display offset of fragment on the row of slice.
		column := func(slice, fragment string) int {
			line := s.row(slice)
			return lipgloss.Width(line[:strings.Index(line, fragment)])
		}
		names, lifecycles := map[int]string{}, map[int]string{}
		for slice, lifecycle := range lifecycleRows {
			names[column(slice, slice)] = slice
			lifecycles[column(slice, lifecycle)] = slice
		}
		names[column("i-torn", "i-torn")] = "i-torn"
		lifecycles[column("i-torn", "! lifecycle unknown")] = "i-torn"
		claims := map[int]string{
			column("c-impl", "▸ implement"):   "c-impl",
			column("d-review", "▸ watchdog"):  "d-review",
			column("i-torn", "claim unknown"): "i-torn",
		}
		if len(names) != 1 || len(lifecycles) != 1 || len(claims) != 1 {
			t.Fatalf("columns start at different offsets: names %v, lifecycles %v, claims %v:\n%s", names, lifecycles, claims, s.model.View())
		}
	})

	t.Run("NarrowResultHeadingsKeepTheWholeLifecycleLabel", func(t *testing.T) {
		t.Parallel()
		s := startSnapshot(t, "gadgets", snapshot)
		size := tea.WindowSizeMsg{Width: 40, Height: 40}
		s.send(size)
		s.press("/", "c-impl", "enter", "w", "g")
		s.within(size)
		var content []string
		for _, line := range strings.Split(s.model.View(), "\n") {
			if strings.HasPrefix(line, "│") {
				content = append(content, strings.Trim(line, "│ "))
			}
		}
		// A heading may wrap, but its Project and complete lifecycle label
		// remain readable in order before the Slice rows.
		text := strings.Join(strings.Fields(strings.Join(content, " ")), " ")
		if !strings.Contains(text, "gadgets · ○ Ready for Implementation") {
			t.Fatalf("result heading loses its lifecycle label:\n%s", s.model.View())
		}
	})
}
