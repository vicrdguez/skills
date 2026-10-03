package browse_test

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// lifecycleRows maps each Slice of the spectrum Proposal to the lifecycle
// glyph and label its row must show.
var lifecycleRows = map[string]string{
	"a-needs":      "◆ Needs Human",
	"b-merge":      "● Ready for Merge",
	"c-impl":       "○ Ready for Implementation",
	"d-review":     "◐ Awaiting Review",
	"e-review":     "◐ Awaiting Review",
	"f-rework":     "↻ Rework",
	"g-merged":     "✓ Merged",
	"h-superseded": "⊘ Superseded",
}

var lifecycleGlyphs = []string{"◆", "●", "○", "◐", "↻", "✓", "⊘"}

// spectrum adds a gadgets Proposal with a Slice in every lifecycle, an
// implement Claim, a watchdog-claimed and an unclaimed Slice in Awaiting
// Review, and a Slice whose state record is unreadable.
func spectrum(t *testing.T) func(string) {
	return func(root string) {
		states := map[string]string{
			"a-needs":      `{"state": "needs_human"}`,
			"b-merge":      `{"state": "ready_for_merge"}`,
			"c-impl":       `{"state": "ready_for_implementation", "claim": {"phase": "implement", "basis": "b", "inputs": {"contract": []}}}`,
			"d-review":     `{"state": "awaiting_review", "claim": {"phase": "watchdog", "basis": "b", "inputs": {"contract": []}}}`,
			"e-review":     `{"state": "awaiting_review"}`,
			"f-rework":     `{"state": "rework"}`,
			"g-merged":     `{"state": "merged", "title": "Merged work"}`,
			"h-superseded": `{"state": "superseded"}`,
			"i-torn":       `{`,
		}
		write(t, root, "projects/gadgets/proposals/spectrum/proposal.json", `{"accepted": "2024-01-01T00:00:00Z"}`)
		for slice, state := range states {
			write(t, root, "projects/gadgets/proposals/spectrum/"+slice+"/state.json", state)
		}
	}
}

// startSpectrum lists the spectrum Proposal's Slices on a terminal narrow
// enough to stack the panes, so each list line shows one row alone.
func startSpectrum(t *testing.T, width int) *session {
	s := start(t, "gadgets", spectrum(t))
	s.send(tea.WindowSizeMsg{Width: width, Height: 40})
	s.press("enter")
	s.shows("Proposal spectrum", "Slices (9)")
	return s
}

// lineIndex is the index of the first view line showing fragment.
func (s *session) lineIndex(fragment string) int {
	s.t.Helper()
	for index, line := range strings.Split(s.model.View(), "\n") {
		if strings.Contains(line, fragment) {
			return index
		}
	}
	s.t.Fatalf("view lacks %q:\n%s", fragment, s.model.View())
	return -1
}

func TestListAndPreviewPanesSitSideBySideWhenWideAndStackWhenNarrow(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 140, Height: 40}, {Width: 40, Height: 24}} {
		s := start(t, "widgets")
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
}

func TestShortStackedTerminalKeepsTheListOverThePreview(t *testing.T) {
	size := tea.WindowSizeMsg{Width: 40, Height: 12}
	s := start(t, "widgets")
	s.send(size)
	s.press("enter", "down")
	s.within(size)
	s.shows("Delivery: unknown", "╭─ Slices (3) ─", " 2/3 ─╯")
	s.row("> ", "cancel")
}

func TestShortStackedListShowsTheWholeSelectedRow(t *testing.T) {
	size := tea.WindowSizeMsg{Width: 40, Height: 16}
	s := start(t, "widgets")
	s.send(size)
	s.press("enter", "down")
	s.within(size)
	s.shows(" 2/3 ─╯")
	if entry := strings.Join(s.listEntries("broken", "cancel", "refund")["cancel"], "\n"); !strings.Contains(entry, "> ") ||
		!strings.Contains(entry, "◐ Awaiting Review") || !strings.Contains(entry, "▸ watchdog") {
		t.Fatalf("the selected row lacks its lifecycle or Claim: %q\n%s", entry, s.model.View())
	}
}

func TestStackedEmptyListShowsItsWholeMessage(t *testing.T) {
	s := start(t, "empty", func(root string) {
		write(t, root, "projects/empty/project.json", `{"repository": "acme/empty"}`)
		write(t, root, "projects/empty/archive/done/proposal.json", `{"accepted": "2023-01-01T00:00:00Z"}`)
		write(t, root, "projects/empty/archive/done/old/state.json", `{"state": "merged"}`)
	})
	s.send(tea.WindowSizeMsg{Width: 40, Height: 24})
	s.shows("Proposals (0)", "No active Proposals.", "include them.", " 0/0 ─╯")
}

func TestListKeepsTheSelectedRowVisibleAndItsPositionOnTheBorder(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 24}, {Width: 140, Height: 30}} {
		s := start(t, "widgets", func(root string) {
			for i := 0; i < 35; i++ {
				write(t, root, fmt.Sprintf("projects/widgets/proposals/orders/damaged%02d/state.json", i), `{`)
			}
		})
		s.send(size)
		s.press("enter")
		s.press(downs(30)...)
		s.within(size)
		s.shows("> ! damaged28")
		if entry := s.listEntries("damaged28", "damaged29")["damaged28"]; !strings.Contains(strings.Join(entry, "\n"), "! lifecycle unknown") {
			t.Fatalf("%dx%d: the selected row lacks its lifecycle: %q", size.Width, size.Height, entry)
		}
		s.row("╰", "31/38", "╯")
	}
}

func TestSliceRowsAlignNameLifecycleAndClaimColumns(t *testing.T) {
	s := startSpectrum(t, 80)
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
}

func TestClaimMarkerStaysOnAFortyColumnList(t *testing.T) {
	size := tea.WindowSizeMsg{Width: 40, Height: 24}
	s := start(t, "gadgets", func(root string) {
		write(t, root, "projects/gadgets/proposals/pair/proposal.json", `{"accepted": "2024-01-01T00:00:00Z"}`)
		write(t, root, "projects/gadgets/proposals/pair/x-claimed/state.json", `{"state": "awaiting_review", "claim": {"phase": "watchdog", "basis": "b", "inputs": {"contract": []}}}`)
		write(t, root, "projects/gadgets/proposals/pair/y-open/state.json", `{"state": "awaiting_review"}`)
	})
	s.send(size)
	s.press("enter")
	s.within(size)
	s.row("…ed", "◐ Awaiting Review", "▸ watchdog")
	if open := s.row("…en", "◐ Awaiting Review"); strings.Contains(open, "▸") {
		t.Fatalf("an unclaimed Slice shows a Claim marker: %q", open)
	}
}

// listEntries is the lines of the list pane that show each of names in
// turn, from the line naming it to the line before the next name.
func (s *session) listEntries(names ...string) map[string][]string {
	s.t.Helper()
	entries, current, inside := map[string][]string{}, "", false
	for _, line := range strings.Split(s.model.View(), "\n") {
		switch {
		case strings.Contains(line, "╭─ Slices"):
			inside = true
			continue
		case strings.HasPrefix(line, "╰"):
			inside = false
		}
		if !inside {
			continue
		}
		for _, name := range names {
			if strings.Contains(line, name) {
				current = name
			}
		}
		if current != "" {
			entries[current] = append(entries[current], line)
		}
	}
	return entries
}

func TestFortyColumnSliceListsKeepEveryLifecycleAndClaim(t *testing.T) {
	spectrumFacts := map[string][]string{"i-torn": {"! lifecycle unknown", "claim unknown"}}
	for slice, lifecycle := range lifecycleRows {
		spectrumFacts[slice] = []string{lifecycle}
	}
	spectrumFacts["c-impl"] = append(spectrumFacts["c-impl"], "▸ implement")
	spectrumFacts["d-review"] = append(spectrumFacts["d-review"], "▸ watchdog")
	widgets := start(t, "widgets")
	widgets.send(tea.WindowSizeMsg{Width: 40, Height: 24})
	widgets.press("enter")
	for _, list := range []struct {
		s     *session
		size  tea.WindowSizeMsg
		facts map[string][]string
	}{
		{startSpectrum(t, 40), tea.WindowSizeMsg{Width: 40, Height: 40}, spectrumFacts},
		{widgets, tea.WindowSizeMsg{Width: 40, Height: 24}, map[string][]string{
			"broken": {"! lifecycle unknown", "claim unknown"},
			"cancel": {"◐ Awaiting Review", "▸ watchdog"},
			"refund": {"○ Ready for Implementation"},
		}},
	} {
		var names []string
		for name := range list.facts {
			names = append(names, name)
		}
		// Each row is read once the selection has scrolled it into view.
		entries := map[string][]string{}
		for range names {
			list.s.within(list.size)
			for name, entry := range list.s.listEntries(names...) {
				if _, seen := entries[name]; !seen {
					entries[name] = entry
				}
			}
			list.s.press("down")
		}
		lifecycles, claims := map[int]string{}, map[int]string{}
		for name, facts := range list.facts {
			entry := strings.Join(entries[name], "\n")
			for index, fact := range facts {
				at := slices.IndexFunc(entries[name], func(line string) bool { return strings.Contains(line, fact) })
				if at < 0 {
					t.Fatalf("%dx%d: %s lacks %q: %q", list.size.Width, list.size.Height, name, fact, entries[name])
				}
				line := entries[name][at]
				offset := lipgloss.Width(line[:strings.Index(line, fact)])
				if index == 0 {
					lifecycles[offset] = name
				} else {
					claims[offset] = name
				}
			}
			if len(facts) == 1 && (strings.Contains(entry, "▸") || strings.Contains(entry, "claim")) {
				t.Errorf("unclaimed %s shows a Claim: %q", name, entry)
			}
		}
		if len(lifecycles) != 1 || len(claims) != 1 {
			t.Fatalf("columns start at different offsets: lifecycles %v, claims %v:\n%s", lifecycles, claims, list.s.model.View())
		}
	}
}

func TestNarrowResultHeadingsKeepTheWholeLifecycleLabel(t *testing.T) {
	s := start(t, "gadgets", spectrum(t))
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
}

func TestNarrowListShortensNamesToKeepLifecycleAndClaim(t *testing.T) {
	s := startSpectrum(t, 60)
	s.within(tea.WindowSizeMsg{Width: 60, Height: 40})
	s.row("c-impl", "○ Ready for Implementation", "▸ implement")
	s.row("d-review", "◐ Awaiting Review", "▸ watchdog")
	s.row("! i-torn", "! lifecycle unknown", "claim unknown")
	s.row("…perseded", "⊘ Superseded")
}

func TestSliceRowsShowEachLifecycleByGlyphAndLabelWithoutColour(t *testing.T) {
	s := startSpectrum(t, 80)
	if strings.Contains(s.model.View(), "\x1b[") {
		t.Fatal("the test terminal renders colour")
	}
	for slice, lifecycle := range lifecycleRows {
		s.row(slice, lifecycle)
	}

	// The facts screen marks its current selection without a lifecycle glyph.
	s.press("f")
	s.row("◐ Awaiting Review", "(2)")
	current := s.row("*", "Any lifecycle", "(10)")
	for _, glyph := range append(lifecycleGlyphs, "▸") {
		if strings.Contains(current, glyph) {
			t.Fatalf("the current selection is marked with %q: %q", glyph, current)
		}
	}
}

func TestClaimIsItsOwnMarkerBesideTheLifecycle(t *testing.T) {
	s := startSpectrum(t, 80)
	s.row("d-review", "◐ Awaiting Review", "▸ watchdog")
	s.row("c-impl", "○ Ready for Implementation", "▸ implement")
	if unclaimed := s.row("e-review", "◐ Awaiting Review"); strings.Contains(unclaimed, "▸") || strings.Contains(unclaimed, "claim") {
		t.Fatalf("an unclaimed Slice shows a Claim marker: %q", unclaimed)
	}
}

func TestUnreadableSliceStaysMarkedUnknownBesideSelectableHealthyOnes(t *testing.T) {
	s := startSpectrum(t, 80)
	torn := s.row("! i-torn", "! lifecycle unknown", "claim unknown")
	for _, glyph := range append(lifecycleGlyphs, "▸") {
		if strings.Contains(torn, glyph) {
			t.Fatalf("the unreadable Slice shows %q: %q", glyph, torn)
		}
	}
	s.press(downs(6)...)
	s.row("> ", "g-merged", "✓ Merged")
	s.press("enter")
	s.shows("Slice  spectrum/g-merged", "Title  Merged work")
	s.fact("Lifecycle", "Merged")
}

// sgr matches the Select Graphic Rendition sequence that ends a prefix.
var sgr = regexp.MustCompile(`\x1b\[([0-9;]*)m$`)

// styleOf is the SGR parameters that start fragment after row on its view
// line, sorted, with bright ANSI colours read as their normal variants.
func (s *session) styleOf(row, fragment string) string {
	s.t.Helper()
	line := s.row(row, fragment)
	at := strings.Index(line, row) + len(row)
	at += strings.Index(line[at:], fragment)
	match := sgr.FindStringSubmatch(line[:at])
	if match == nil {
		s.t.Fatalf("%q in %q is not styled", fragment, line)
	}
	var parameters []string
	for _, parameter := range strings.Split(match[1], ";") {
		if len(parameter) == 2 && parameter[0] == '9' {
			parameter = "3" + parameter[1:]
		}
		parameters = append(parameters, parameter)
	}
	sort.Strings(parameters)
	return strings.Join(parameters, ";")
}

func TestLifecycleAndClaimUseTheTerminalsAnsiColoursAndWeights(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	s := startSpectrum(t, 80)
	s.press(downs(4)...)
	s.row("> ", "e-review")

	// Bold for what waits on the human, normal for in-flight work and dim
	// for finished work, in the terminal's own red, green, blue, yellow and
	// magenta; the Claim alone is cyan.
	for slice, want := range map[string]string{
		"a-needs": "1;31", "b-merge": "1;32", "c-impl": "34", "d-review": "33",
		"f-rework": "35", "g-merged": "2", "h-superseded": "2",
	} {
		if got := s.styleOf(slice, lifecycleRows[slice]); got != want {
			t.Errorf("%s lifecycle style = %q, want %q", slice, got, want)
		}
	}
	for slice, claim := range map[string]string{"c-impl": "▸ implement", "d-review": "▸ watchdog"} {
		if got := s.styleOf(slice, claim); got != "36" {
			t.Errorf("%s Claim style = %q, want cyan", slice, got)
		}
	}
	if unknown, warning := s.styleOf("i-torn", "! lifecycle unknown"), s.styleOf("", "! Incomplete"); unknown != warning {
		t.Errorf("unknown lifecycle style = %q, want the warning style %q", unknown, warning)
	}
}

func TestSelectedRowKeepsItsLifecycleWeight(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	s := startSpectrum(t, 80)
	// Selection adds reverse video (7) to the lifecycle's own colour and
	// weight: bold stays on Needs Human alone, dim on finished work.
	for _, selected := range []struct{ slice, want string }{
		{"a-needs", "1;31;7"}, {"e-review", "33;7"}, {"f-rework", "35;7"}, {"g-merged", "2;7"},
	} {
		for step := 0; !strings.Contains(s.row("> "), selected.slice); step++ {
			if step == len(lifecycleRows) {
				t.Fatalf("no row of %s can be selected", selected.slice)
			}
			s.press("down")
		}
		if got := s.styleOf(selected.slice, lifecycleRows[selected.slice]); got != selected.want {
			t.Errorf("selected %s lifecycle style = %q, want %q", selected.slice, got, selected.want)
		}
	}
}
