package browse

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicrdguez/skills/ledger"
)

// wideLayout is the width from which the selected entry's facts sit beside
// the list instead of below it.
const wideLayout = 100

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	relationStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "209"})
)

func (m Model) View() string {
	header := m.header()
	footer := m.footer()
	height := m.bodyHeight(header, footer)
	var body string
	switch {
	case m.failure != nil:
		body = wrap(warningStyle.Render("! Unable to show this view: "+m.failure.Error()), m.width)
	case m.screen == sliceScreen:
		note := ""
		if m.detail.TotalLineCount() > m.detail.Height {
			note = fmt.Sprintf("scrolled %d%%", int(m.detail.ScrollPercent()*100))
		}
		body = pane("Slice", note, strings.Split(m.detail.View(), "\n"), m.width, height)
	case m.screen == diagnosticsScreen:
		body = m.detail.View()
	case m.screen == readerScreen:
		body = m.readerBody(height)
	default:
		body = m.listBody(height)
	}
	frame := []string{header, footer}
	if height > 0 {
		frame = []string{header, clip(body, height), footer}
	}
	// Notices taller than the terminal would overflow it, and the renderer
	// would drop the header with the top lines, so the bottom gives way.
	return clip(strings.Join(frame, "\n"), m.height)
}

// bodyHeight leaves the body at least one line, or none while every binding
// is listed, so that the list fits a small terminal.
func (m Model) bodyHeight(header, footer string) int {
	least := 1
	if m.help.ShowAll {
		least = 0
	}
	return max(m.height-lipgloss.Height(header)-lipgloss.Height(footer), least)
}

// layoutDetail fits the detail and document viewports to the current
// terminal and marks the Slice's selected followable relationship as well as
// styling it.
func (m *Model) layoutDetail() {
	height := m.bodyHeight(m.header(), m.footer())
	m.detail.Width, m.detail.Height = m.width, height
	switch m.screen {
	case diagnosticsScreen:
		offset := m.detail.YOffset
		var lines []string
		for _, diagnostic := range m.activeDiagnostics() {
			lines = append(lines, warningStyle.Render(DiagnosticText(diagnostic)))
		}
		if len(lines) == 0 {
			lines = append(lines, "No diagnostics for this view.")
		}
		m.detail.SetContent(wrap(strings.Join(lines, "\n"), m.width))
		m.detail.SetYOffset(offset)
	case sliceScreen:
		// The detail sits in a pane whose border carries its scroll position.
		m.detail.Width, m.detail.Height = max(m.width-paneFrame, 1), max(height-2, 1)
		if m.slice != nil {
			offset := m.detail.YOffset
			m.relation = min(m.relation, max(len(sliceRelations(m.slice))-1, 0))
			lines, starts := renderSliceFacts(sliceSections(m.slice, m.sliceReports), m.detail.Width, m.relation)
			m.selectedRow = 0
			if len(starts) > 0 {
				m.selectedRow = starts[m.relation]
			}
			m.detail.SetContent(strings.Join(lines, "\n"))
			m.detail.SetYOffset(offset)
		}
	}

	if m.screen == readerScreen {
		// The document is rendered again only when its width changes.
		s := &m.reader.shown
		width, paneHeight := m.documentPane(height)
		// A narrow title takes further lines from the pane.
		rows := func(title string) int {
			return max(paneHeight-1-lipgloss.Height(title), 1)
		}
		s.viewport.Width, s.viewport.Height = max(width-paneFrame, 1), rows(focusTitle(m.reader.documentFocus, width, m.documentTitle))
		if s.rendered != s.viewport.Width {
			offset := s.viewport.YOffset
			content, problem := m.shownContent(s.viewport.Width)
			s.renderProblem, s.rendered = problem, s.viewport.Width
			// Long unbroken text, such as an exact reference, wraps rather
			// than being cut at the pane's edge.
			s.viewport.SetContent(wrap(content, s.viewport.Width))
			s.viewport.SetYOffset(offset)
		}
		if r := &m.reader; r.detailsOpen() {
			offset := r.detailsView.YOffset
			r.detailsView.Width, r.detailsView.Height = s.viewport.Width, rows(m.documentPaneTitle(width))
			r.detailsView.SetContent(wrap(strings.Join(detailsLines(s.document), "\n"), s.viewport.Width))
			r.detailsView.SetYOffset(offset)
		}
	}
}

// header is one line: the breadcrumb on the left and the ledger revision on
// the right, preceded by the indicators this view needs.
func (m Model) header() string {
	context := m.screen
	if m.inDocuments() {
		context = m.docContext
	}
	var path []string
	switch {
	case context == factsScreen:
		path = []string{"Find slices", "Facts"}
	case context == resultsScreen:
		path = []string{"Find slices"}
	case context == diagnosticsScreen:
		path = []string{"Find slices", "Diagnostics"}
	case m.isFindingScreen(context):
		path = []string{"Find slices", m.project, m.item}
	default:
		path = []string{"Projects"}
		switch context {
		case projectScreen:
			path = append(path, m.project)
		case proposalScreen:
			path = append(path, m.project, m.proposal)
		case sliceScreen:
			path = append(path, m.project, m.proposal)
			_, slice, _ := strings.Cut(m.item, "/")
			path = append(path, slice)
		}
	}
	switch m.screen {
	case readerScreen:
		path = append(path, "documents")
	case referencesScreen:
		if m.referenceOrigin == readerScreen {
			path = append(path, "documents", m.shownTitle())
		}
		path = append(path, "references")
	case diagnosticsScreen:
		if m.diagnosticReturn == readerScreen {
			path = append(path, "documents", "diagnostics")
		}
	}
	// Each indicator has a full form and shorter ones. A narrow terminal
	// first gives up the breadcrumb, then uses the shorter forms, so every
	// indicator stays in view. The displayed document's identity belongs on
	// its pane's title.
	var indicators []indicator
	scrolled := func(view viewport.Model) {
		if view.TotalLineCount() > view.Height {
			percent := fmt.Sprintf("%d%%", int(view.ScrollPercent()*100))
			indicators = append(indicators, indicator{mutedStyle, []string{"scrolled " + percent, percent}})
		}
	}
	if m.screen == diagnosticsScreen {
		scrolled(m.detail)
	}
	// The reader's document scroll position belongs only on its pane border.
	if m.includeArchived {
		indicators = append(indicators, indicator{mutedStyle, []string{"archived shown", "archived"}})
	}
	revision := indicator{mutedStyle, refForms("", m.snapshot.Revision)}
	if m.refreshFailure != nil {
		revision = indicator{warningStyle.Bold(true), refForms("NOT REFRESHED ", m.snapshot.Revision)}
	}
	indicators = append(indicators, revision)
	right := fitIndicators(indicators, m.width)
	// The breadcrumb stays at least minGap spaces clear of the indicators.
	const minGap = 2
	left := ""
	if room := m.width - lipgloss.Width(right) - minGap; room > 0 {
		left = titleStyle.Render(keepEnd("skl browse › "+strings.Join(path, " › "), room))
	}
	padding := strings.Repeat(" ", max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 0))
	return truncate(left+padding+right, m.width)
}

func (m Model) footer() string {
	lines := []string{}
	if m.status != "" {
		lines = append(lines, wrap(m.status, m.width))
	}
	if m.refreshFailure != nil {
		lines = append(lines, wrap(warningStyle.Render("! Refresh failed; displayed facts are NOT REFRESHED: "+m.refreshFailure.Error()), m.width))
	}
	if m.selectionMissing != "" {
		lines = append(lines, wrap(warningStyle.Render("! "+m.selectionMissing), m.width))
	}
	if m.screen == readerScreen && m.reader.shown.newer {
		lines = append(lines, wrap("Newer document available; select its entry and press enter to read it", m.width))
	}
	// A displayed document's diagnostics take one line, so that they never
	// crowd out the document; the details panel shows each in full.
	if document := m.reader.shown.document; m.screen == readerScreen && document != nil && len(document.Diagnostics) > 0 {
		label := "! Metadata diagnostic: "
		if count := len(document.Diagnostics); count > 1 {
			label = fmt.Sprintf("! Metadata diagnostic 1 of %d: ", count)
		}
		text := strings.Join(strings.Fields(DiagnosticText(document.Diagnostics[0])), " ")
		lines = append(lines, truncate(warningStyle.Render(label+text), m.width))
	}
	if m.screen == readerScreen && m.reader.shown.renderProblem != "" {
		lines = append(lines, wrap(warningStyle.Render("! "+m.reader.shown.renderProblem+"; showing recorded text"), m.width))
	}
	switch {
	case m.typing != nil:
		lines = append(lines, wrap(titleStyle.Render("Search names: ")+*m.typing+"█  (enter apply · esc cancel)", m.width))
		// Typing takes every other key as text, `?` included.
		apply, cancel := m.keys.Enter, m.keys.Back
		apply.SetHelp("enter", "apply")
		cancel.SetHelp("esc", "cancel")
		lines = append(lines, truncate(m.help.ShortHelpView([]key.Binding{apply, cancel}), m.width))
	case m.help.ShowAll:
		// The header and the notices above stay in view; the key list takes
		// the lines left and says when some bindings do not fit.
		keys := m.allKeys()
		room := m.height - 1
		if len(lines) > 0 {
			room -= lipgloss.Height(strings.Join(lines, "\n"))
		}
		if len(keys) > room {
			keys = append(keys[:max(room-1, 0)], truncate(mutedStyle.Render("… more keys fit a larger terminal"), m.width))
		}
		lines = append(lines, keys...)
	default:
		lines = append(lines, m.keyHelp(m.footerKeys()))
	}
	return strings.Join(lines, "\n")
}

// keyHelp lists as many of bindings as fit the width, always followed by
// help.
func (m Model) keyHelp(bindings []key.Binding) string {
	for count := len(bindings); count > 0; count-- {
		// The capped slice makes append copy rather than overwrite bindings.
		line := m.help.ShortHelpView(append(bindings[:count:count], m.keys.Help))
		if lipgloss.Width(line) <= m.width {
			return line
		}
	}
	return truncate(m.help.ShortHelpView([]key.Binding{m.keys.Help}), m.width)
}

// allKeys lays out every binding, wrapping between bindings so that none is
// cut at the width.
func (m Model) allKeys() []string {
	var lines []string
	var line []key.Binding
	for _, binding := range m.keys.all() {
		if len(line) > 0 && lipgloss.Width(m.help.ShortHelpView(append(line, binding))) > m.width {
			lines = append(lines, truncate(m.help.ShortHelpView(line), m.width))
			line = nil
		}
		line = append(line, binding)
	}
	return append(lines, truncate(m.help.ShortHelpView(line), m.width))
}

// listBody renders the current list screen: its parent context above a list
// pane and a preview pane of the selected entry's facts, side by side when
// the terminal is wide and stacked when it is narrow.
func (m Model) listBody(height int) string {
	content := m.listContent()
	width := m.width
	top := ""
	if len(content.context) > 0 {
		top = wrap(strings.Join(content.context, "\n"), width) + "\n"
	}
	available := max(height-strings.Count(top, "\n"), 3)
	position := m.position()
	preview := content.preview
	if len(preview) == 0 {
		preview = []string{"No entry is selected."}
	}
	previewLines := func(width int) []string {
		if content.facts != nil {
			lines, _ := renderSliceFacts(content.facts, width, -1)
			return lines
		}
		return wrapLines(preview, width)
	}
	if width >= wideLayout {
		// The list widens beyond half the terminal, up to three fifths, to
		// fit its rows; the preview wraps into the rest.
		listWidth := min(max(content.width()+paneFrame, width/2), width*3/5)
		return top + lipgloss.JoinHorizontal(lipgloss.Top,
			pane(content.title, position, content.lines(listWidth-paneFrame, available-2), listWidth, available),
			pane(content.previewTitle, "", previewLines(width-listWidth-paneFrame), width-listWidth, available))
	}
	// Stacked, the list takes up to half the height and the preview the
	// rest; a preview too short for its border gives way to the list.
	// The list never takes less than the selected row's lines.
	listHeight := min(len(content.lines(width-paneFrame, available))+2, max(available/2, len(content.lines(width-paneFrame, 1))+2))
	if available-listHeight < 3 {
		return top + pane(content.title, position, content.lines(width-paneFrame, available-2), width, available)
	}
	return top + pane(content.title, position, content.lines(width-paneFrame, listHeight-2), width, listHeight) + "\n" +
		pane(content.previewTitle, "", previewLines(width-paneFrame), width, available-listHeight)
}

// focusMark marks the title of the reader pane the keys act in, so focus
// never relies on colour alone.
const focusMark = "▶ "

// readerBody renders the reader: the navigator beside the displayed document
// when the terminal is wide, and the focused one alone when it is narrow.
func (m Model) readerBody(height int) string {
	r := m.reader
	documentWidth, _ := m.documentPane(height)
	navigatorWidth := m.width - documentWidth
	if m.width < wideLayout {
		navigatorWidth = m.width
	}
	view := r.shown.viewport
	if r.detailsOpen() {
		view = r.detailsView
	}
	document := pane(m.documentPaneTitle(documentWidth), scrollNote(view), strings.Split(view.View(), "\n"), documentWidth, height)
	if m.width < wideLayout && (r.documentFocus || r.detailsOpen()) {
		return document
	}
	note := m.position()
	if r.list != nil && len(r.list.Diagnostics) > 0 {
		note = fmt.Sprintf("! %s · d · %s", plural(len(r.list.Diagnostics), "diagnostic"), note)
	}
	navigatorTitle := focusTitle(!r.documentFocus && !r.detailsOpen(), navigatorWidth, m.navigatorTitle)
	rows := height - 1 - lipgloss.Height(navigatorTitle)
	navigator := pane(navigatorTitle, note, m.navigator().lines(navigatorWidth-paneFrame, rows), navigatorWidth, height)
	if m.width < wideLayout {
		return navigator
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, navigator, document)
}

// documentPaneTitle is the title of a document pane width cells wide: the
// details panel's while it is open, and otherwise the displayed document's.
func (m Model) documentPaneTitle(width int) string {
	r := m.reader
	if r.detailsOpen() {
		return focusMark + "Details · " + documentLabel(r.shown.document, true)
	}
	return focusTitle(r.documentFocus, width, m.documentTitle)
}

// focusTitle is the title of a pane width cells wide, carrying the focus mark
// when the keys act in the pane, and fitted to the room the mark leaves.
func focusTitle(focused bool, width int, title func(width int) string) string {
	mark := ""
	if focused {
		mark = focusMark
	}
	return mark + title(width-5-lipgloss.Width(mark))
}

// scrollNote is a pane's scroll position, when its content overflows it.
func scrollNote(view viewport.Model) string {
	if view.TotalLineCount() <= view.Height {
		return ""
	}
	return fmt.Sprintf("scrolled %d%%", int(view.ScrollPercent()*100))
}

// documentPane is the size of the document pane in a reader body of height
// lines. Beside the navigator it takes what the navigator leaves: the
// navigator fits its entries in up to nine twentieths of the terminal.
func (m Model) documentPane(height int) (int, int) {
	if m.width < wideLayout {
		return m.width, height
	}
	return m.width - min(max(m.navigator().width()+paneFrame, 24), m.width*9/20), height
}

// navigator lists the reader's entries, each Slice's under its heading.
// Rebuilding the listing for sizing and rendering repeats presentation work;
// cache it if larger document sets make cursor movement perceptibly slow.
func (m Model) navigator() listing {
	l := listing{empty: "No documents are listed at this committed revision. Press d for the diagnostics, if any.", cursorRow: -1}
	entries := m.reader.entries
	for index, item := range entries {
		if item.heading != "" && (index == 0 || item.heading != entries[index-1].heading) {
			l.rows = append(l.rows, row{heading: titleStyle.Render(item.heading)})
		}
		if index == m.cursor[readerScreen] && m.selectionMissing == "" {
			l.cursorRow = len(l.rows)
		}
		l.rows = append(l.rows, row{marked: item.marked || m.reader.unreadable[item.id], columns: item.columns})
	}
	return l
}

// position is the selected entry's place among the current list's entries.
func (m Model) position() string {
	count := m.rows()
	switch {
	case count == 0:
		return "0/0"
	case m.selectionMissing != "":
		return fmt.Sprintf("-/%d", count)
	}
	return fmt.Sprintf("%d/%d", m.cursor[m.screen]+1, count)
}

// columnGap is the space between two columns of a list row.
const columnGap = 2

// paneFrame is the width of a pane's side borders and their padding.
const paneFrame = 4

// pane frames lines in a rounded border exactly width cells wide and height
// lines tall, with its title in the top edge and note, when set, in the
// bottom edge. Each further line of the title continues on an edge of its
// own below the top. Lines beyond its height are clipped.
func pane(title, note string, lines []string, width, height int) string {
	inner := max(width-paneFrame, 1)
	edge := mutedStyle.Render
	var framed []string
	for index, line := range strings.Split(title, "\n") {
		line = truncate(line, max(width-5, 1))
		corners := [2]string{"├─ ", "┤"}
		if index == 0 {
			corners = [2]string{"╭─ ", "╮"}
		}
		framed = append(framed, edge(corners[0])+titleStyle.Render(line)+edge(" "+strings.Repeat("─", max(width-5-lipgloss.Width(line), 0))+corners[1]))
	}
	rows := height - 1 - len(framed)
	for index := 0; index < rows; index++ {
		line := ""
		if index < len(lines) {
			line = truncate(lines[index], inner)
		}
		framed = append(framed, edge("│ ")+line+strings.Repeat(" ", max(inner-lipgloss.Width(line), 0))+edge(" │"))
	}
	bottom := "╰" + strings.Repeat("─", max(width-2, 0)) + "╯"
	if note != "" {
		note = truncate(note, max(width-6, 1))
		bottom = "╰" + strings.Repeat("─", max(width-5-lipgloss.Width(note), 0)) + " " + note + " ─╯"
	}
	return strings.Join(append(framed, edge(bottom)), "\n")
}

func wrapLines(lines []string, width int) []string {
	return strings.Split(wrap(strings.Join(lines, "\n"), width), "\n")
}

// listing is one list screen's content: the context above its panes, the
// list pane's title and rows, the row at the cursor, and the preview of the
// selected entry.
type listing struct {
	context []string
	title   string
	rows    []row
	// whole is how many leading columns a narrow pane keeps whole by
	// shortening the first; any later columns are cut at the pane's edge.
	whole        int
	cursorRow    int
	empty        string
	previewTitle string
	preview      []string
	// facts, when set, are the selected Slice's facts shown as the preview.
	facts []factSection
}

// row is one line of a list pane: an entry's facts in columns, marked when
// some of them are unknown or incomplete, or else a heading over the entries
// that follow it.
type row struct {
	marked  bool
	columns []span
	heading string
}

// layout is the width of each column across every row, not only the
// visible ones, and whether any row carries the unknown-facts marker.
func (l listing) layout() (widths []int, marks bool) {
	for _, row := range l.rows {
		marks = marks || row.marked
		for index, column := range row.columns {
			if index == len(widths) {
				widths = append(widths, 0)
			}
			widths[index] = max(widths[index], lipgloss.Width(column.text))
		}
	}
	return widths, marks
}

// rowWidth is the width of a row whose columns have widths, after its
// selection and unknown-facts markers.
func rowWidth(widths []int, marks bool) int {
	width := 2
	if marks {
		width += 2
	}
	gap := 0
	for _, columnWidth := range widths {
		if columnWidth > 0 {
			width += gap + columnWidth
			gap = columnGap
		}
	}
	return width
}

// width is the width of the list's widest row.
func (l listing) width() int {
	widths, marks := l.layout()
	width := rowWidth(widths, marks)
	for _, row := range l.rows {
		if row.columns == nil {
			width = max(width, 2+lipgloss.Width(row.heading))
		}
	}
	return width
}

// shortestName is the fewest cells a shortened first column keeps on a
// one-line row: the name yields before the lifecycle and Claim do.
const shortestName = 3

// tierIndent is how much further than the first column a row's later lines
// start, so each row's lines read as one entry.
const tierIndent = 2

// lines renders the list in a window of height lines that keeps the cursor
// row visible. The cursor row is marked by text as well as style.
func (l listing) lines(width, height int) []string {
	if len(l.rows) == 0 {
		return wrapLines([]string{l.empty}, width)
	}
	widths, marks := l.layout()
	tiers, widths := l.tiers(widths, marks, width)
	blocks := make([][]string, len(l.rows))
	for index, row := range l.rows {
		blocks[index] = row.render(widths, tiers, marks, index == l.cursorRow, width)
	}
	start := 0
	if l.cursorRow >= 0 {
		start = l.cursorRow
		for used := len(blocks[start]); start > 0 && used+len(blocks[start-1]) <= height; start-- {
			used += len(blocks[start-1])
		}
	}
	var lines []string
	for _, block := range blocks[start:] {
		if len(lines) > 0 && len(lines)+len(block) > height {
			break
		}
		lines = append(lines, block...)
	}
	return lines
}

// tiers groups the columns into the lines every row takes within width,
// and returns widths with the first column narrowed to fit. A row takes one
// line when its whole columns fit, its first column shortened down to
// shortestName if need be. Otherwise the first column takes a line of its
// own and the other whole columns wrap beneath it, so each column still
// starts at one offset on every row. Columns after the whole ones follow on
// the last line and are cut at the pane's edge.
func (l listing) tiers(widths []int, marks bool, width int) ([][]int, []int) {
	widths = slices.Clone(widths)
	every := make([]int, len(widths))
	for index := range every {
		every[index] = index
	}
	whole := min(l.whole, len(widths))
	excess := rowWidth(widths[:whole], marks) - width
	if whole == 0 || excess <= 0 {
		return [][]int{every}, widths
	}
	if widths[0]-excess >= shortestName {
		widths[0] -= excess
		return [][]int{every}, widths
	}
	widths[0] = max(min(widths[0], width-lead(marks)), 1)
	room := width - lead(marks) - tierIndent
	// used is the width taken on the current line; the first column past
	// the name always starts a new line.
	tiers, used := [][]int{{0}}, room
	for index := 1; index < len(widths); index++ {
		if widths[index] == 0 {
			continue
		}
		if index < whole && used+columnGap+widths[index] > room {
			tiers = append(tiers, []int{index})
			used = widths[index]
			continue
		}
		tiers[len(tiers)-1] = append(tiers[len(tiers)-1], index)
		used += columnGap + widths[index]
	}
	return tiers, widths
}

// lead is the width of a row's selection and unknown-facts markers.
func lead(marks bool) int {
	return rowWidth(nil, marks)
}

// render draws one row as the lines of its tiers, its columns padded to
// widths and, when marks is set, a column for the unknown-facts marker. A
// later line with nothing to show, such as an unclaimed Slice's Claim, is
// left out.
func (r row) render(widths []int, tiers [][]int, marks, selected bool, width int) []string {
	if r.columns == nil {
		return wrapLines([]string{"  " + r.heading}, width)
	}
	var lines []string
	for number, tier := range tiers {
		line := ""
		add := func(style lipgloss.Style, text string) {
			// Selection reverses each span and leaves its weight alone, since
			// a lifecycle's weight is part of its look.
			if selected {
				style = style.Reverse(true)
			}
			if text != "" {
				line += style.Render(text)
			}
		}
		plain := lipgloss.NewStyle()
		shown := number == 0
		switch {
		case number > 0:
			add(plain, strings.Repeat(" ", lead(marks)+tierIndent))
		case selected:
			add(plain, "> ")
		default:
			add(plain, "  ")
		}
		if number == 0 && marks && r.marked {
			add(warningStyle, "!")
			add(plain, " ")
		} else if number == 0 && marks {
			add(plain, "  ")
		}
		gap := ""
		for _, index := range tier {
			if widths[index] == 0 {
				continue
			}
			var column span
			if index < len(r.columns) {
				column = r.columns[index]
			}
			shown = shown || column.text != ""
			column.text = shorten(column.text, widths[index])
			add(plain, gap)
			add(column.style, column.text)
			add(plain, strings.Repeat(" ", widths[index]-lipgloss.Width(column.text)))
			gap = strings.Repeat(" ", columnGap)
		}
		if !shown {
			continue
		}
		if selected {
			add(plain, strings.Repeat(" ", max(width-lipgloss.Width(line), 0)))
		}
		lines = append(lines, truncate(line, width))
	}
	return lines
}

// shorten keeps the end of text within width cells, marking the cut with …,
// since the end of a Slice identity is what tells rows apart.
func shorten(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes)) >= width {
		runes = runes[1:]
	}
	return "…" + string(runes)
}

// texts are unstyled columns.
func texts(values ...string) []span {
	columns := make([]span, len(values))
	for index, value := range values {
		columns[index] = span{text: value}
	}
	return columns
}

// sliceRow shows one Slice's name, lifecycle and Claim, then any further
// columns. Unknown facts read as unknown, never as a lifecycle or unclaimed.
func sliceRow(name string, slice ledger.SliceSummary, more ...span) row {
	if !slice.Readable {
		return row{marked: true, columns: append([]span{{text: name}, unknownLifecycleSpan(), unknownClaimSpan()}, more...)}
	}
	columns := []span{{text: name}, lifecycleSpan(slice.Lifecycle), claimSpan(slice.ClaimPhase)}
	return row{marked: len(slice.Diagnostics) > 0, columns: append(columns, more...)}
}

// groupHeading names one group of found Slices, naming a lifecycle group by
// its lifecycle's glyph and label.
func groupHeading(prefix string, group ledger.SliceGroup) string {
	switch {
	case group.UnknownLifecycle:
		return titleStyle.Render(prefix) + unknownLifecycleSpan().render()
	case group.Lifecycle != "":
		return titleStyle.Render(prefix) + lifecycleSpan(group.Lifecycle).render()
	}
	return titleStyle.Render(prefix + GroupTitle(group))
}

// listContent supplies the current list screen's content.
func (m Model) listContent() listing {
	var l listing
	cursor := m.cursor[m.screen]
	l.cursorRow = cursor
	switch m.screen {
	case overviewScreen:
		l.title = fmt.Sprintf("Projects (%d)", len(m.overview.Projects))
		l.empty = "No Projects are recorded in the configured ledger at this revision."
		l.previewTitle = "Project"
		for _, diagnostic := range m.overview.Diagnostics {
			l.context = append(l.context, warningStyle.Render(DiagnosticText(diagnostic)))
		}
		for _, project := range m.overview.Projects {
			var claimed, unknown string
			if project.Claimed > 0 {
				claimed = fmt.Sprintf("%d claimed", project.Claimed)
			}
			if project.Unknown > 0 {
				unknown = fmt.Sprintf("%d unknown", project.Unknown)
			}
			l.rows = append(l.rows, row{marked: project.Incomplete,
				columns: texts(project.Name, orUnknown(project.Repository), plural(project.Slices, "slice"), claimed, unknown)})
		}
		if len(l.rows) > 0 {
			l.preview = ProjectLines(m.overview.Projects[cursor])
		}
	case projectScreen:
		project := m.inventory.Project
		l.context = []string{"Project " + project.Name + " (" + orUnknown(project.Repository) + ") · " + tallyText(project.Tally)}
		if project.Incomplete {
			l.context = append(l.context, warningStyle.Render("! Incomplete: "+incompleteText(project.Diagnostics)+" in this Project"))
		}
		l.title = fmt.Sprintf("Proposals (%d)", len(m.inventory.Proposals))
		l.previewTitle = "Proposal"
		l.empty = fmt.Sprintf("No active Proposals. %d archived; press a to include them.", project.ArchivedProposals)
		if m.includeArchived {
			l.empty = "No Proposals are recorded in this Project."
		}
		for _, proposal := range m.inventory.Proposals {
			l.rows = append(l.rows, row{marked: proposal.Incomplete, columns: texts(proposal.Name+flags(proposal), progressText(proposal))})
		}
		if len(l.rows) > 0 {
			l.preview = ProposalLines(m.inventory.Proposals[cursor])
		}
	case referencesScreen:
		l.context = []string{"Structured ledger references · exact commit and path; unavailable references are never replaced."}
		if document := m.reader.shown.document; m.referenceOrigin == readerScreen && document != nil {
			l.context = append(l.context, "From "+m.shownTitle())
			l.context = append(l.context, diagnosticLines(document.Diagnostics)...)
		} else if m.slice == nil || m.slice.Claim == nil {
			l.context = append(l.context, "No current Slice Claim at ledger revision "+m.snapshot.Revision)
		} else {
			l.context = append(l.context, "From the current Slice Claim at ledger revision "+m.snapshot.Revision)
		}
		l.title = fmt.Sprintf("References (%d)", len(m.references))
		l.previewTitle = "Reference"
		l.empty = "No structured exact ledger references are available. Metadata diagnostics remain with the document."
		for _, reference := range m.references {
			l.rows = append(l.rows, row{columns: texts(reference.Label + " — " + reference.Reference.Path)})
		}
		if len(l.rows) > 0 {
			reference := m.references[cursor]
			l.preview = []string{
				"Label: " + reference.Label,
				"Exact ledger reference: " + reference.Reference.Commit + ":" + reference.Reference.Path,
				"Enter follows this exact reference; source-code revisions are not browsed.",
			}
		}
	case proposalScreen:
		proposal := m.members.Proposal
		l.context = []string{
			"Proposal " + proposal.Name + flags(proposal) + " — " + orUnknown(proposal.ParentTitle),
			"Delivery: " + deliveryText(proposal) + " · Parent issue: " + attachmentText(proposal.ParentIssue),
		}
		if proposal.Incomplete {
			l.context = append(l.context, warningStyle.Render("! Incomplete: "+incompleteText(proposal.Diagnostics)+" in this Proposal"))
		}
		l.title = fmt.Sprintf("Slices (%d)", len(m.members.Slices))
		l.whole = 3
		l.previewTitle = "Slice"
		l.empty = "This Proposal records no Slices."
		for _, slice := range m.members.Slices {
			l.rows = append(l.rows, sliceRow(slice.Slice, slice))
		}
		if len(l.rows) > 0 {
			l.preview, l.facts = m.slicePreview()
		}
	case factsScreen:
		l.context = m.findingContext()
		facets := m.search.Facets
		if facets.UnknownLifecycle > 0 || facets.UnknownClaim > 0 {
			l.context = append(l.context, warningStyle.Render(fmt.Sprintf("! Counted only under Any: %d with unknown lifecycle, %d with unknown claim", facets.UnknownLifecycle, facets.UnknownClaim)))
		}
		l.title = fmt.Sprintf("Facts (%d) — select one to find its Slices", len(factOptions))
		l.previewTitle = "Selection"
		for _, option := range factOptions {
			l.rows = append(l.rows, m.factRow(option))
		}
		l.preview = []string{"Finds: " + SelectionText(factOptions[cursor].apply(m.search.Query))}
	case resultsScreen:
		l.context = m.findingContext()
		if count := len(m.resultDiagnostics()); count > 0 {
			l.context = append(l.context, warningStyle.Render(fmt.Sprintf("! %d diagnostics · d to inspect", count)))
		}
		l.title = fmt.Sprintf("Slices (%d)", m.search.Matched)
		if m.search.Undecided > 0 {
			l.title = fmt.Sprintf("Slices (%d matched, %d undecided)", m.search.Matched, m.search.Undecided)
		}
		l.whole = 3
		l.previewTitle = "Slice"
		l.empty = ResultText(m.search)
		results := m.results()
		for index, found := range results {
			if index == 0 || found.heading != results[index-1].heading {
				l.rows = append(l.rows, row{heading: found.heading})
			}
			if index == cursor {
				l.cursorRow = len(l.rows)
			}
			name := found.match.Item
			if found.match.Archived {
				name += " [archived]"
			}
			l.rows = append(l.rows, sliceRow(name, found.match.SliceSummary, span{text: found.match.Title}))
		}
		if len(results) > 0 {
			l.preview, l.facts = m.slicePreview()
		}
	}
	if m.selectionMissing != "" {
		l.cursorRow, l.preview, l.facts = -1, nil, nil
	}
	return l
}

// slicePreview is the preview of the selected Slice: its facts, or why
// they cannot be shown.
func (m Model) slicePreview() ([]string, []factSection) {
	if m.preview.err != nil {
		return []string{warningStyle.Render("! Unable to show this Slice: " + m.preview.err.Error())}, nil
	}
	if m.preview.slice == nil {
		return nil, nil
	}
	return nil, sliceSections(m.preview.slice, m.preview.reports)
}

// resultDiagnostics lists membership uncertainty separately from the
// result list, so long diagnostic sets cannot hide selectable Slices.
func (m Model) resultDiagnostics() []ledger.Diagnostic {
	diagnostics := append([]ledger.Diagnostic(nil), m.search.Diagnostics...)
	for _, project := range m.search.Projects {
		diagnostics = append(diagnostics, project.Diagnostics...)
	}
	return diagnostics
}

func (m Model) activeDiagnostics() []ledger.Diagnostic {
	if m.diagnosticReturn == readerScreen {
		if m.reader.list == nil {
			return nil
		}
		return m.reader.list.Diagnostics
	}
	return m.resultDiagnostics()
}

// findingContext states the current selection and its result.
func (m Model) findingContext() []string {
	result := ResultText(m.search)
	if m.screen == resultsScreen && m.width < 60 && m.height < 20 {
		result = fmt.Sprintf("%d matched, %d undecided", m.search.Matched, m.search.Undecided)
		if m.search.Incomplete {
			result += "; incomplete"
		}
	}
	if m.search.Incomplete {
		result = warningStyle.Render("! " + result)
	}
	return []string{"Finding: " + SelectionText(m.search.Query), result}
}

// factRow is one navigable fact with the number of Slices selecting it would
// find, marked * when it is the current selection. A Claim fact carries the
// Claim marker, except unclaimed, which has none.
func (m Model) factRow(option factOption) row {
	facets := m.search.Facets
	counts, unknown, current, label := facets.Lifecycles, facets.UnknownLifecycle, m.search.Query.Lifecycles, span{text: "Any lifecycle"}
	if option.claim {
		counts, unknown, current, label = facets.Claims, facets.UnknownClaim, m.search.Query.Claims, span{text: "Any claim"}
	}
	count := counts[option.value]
	switch {
	case option.value == "":
		count = unknown
		for _, value := range counts {
			count += value
		}
	case option.value == ledger.ClaimNone:
		label = span{text: claimLabels[option.value]}
	case option.claim:
		label = claimSpan(option.value)
	default:
		label = lifecycleSpan(option.value)
	}
	mark := ""
	if (option.value == "" && len(current) == 0) || (len(current) == 1 && current[0] == option.value) {
		mark = "*"
	}
	return row{columns: []span{{text: mark}, label, {text: fmt.Sprintf("(%d)", count)}}}
}

// progressText is the compact delivery state of one Proposal row.
func progressText(proposal ledger.ProposalSummary) string {
	text := fmt.Sprintf("%d of %d Merged", proposal.Lifecycles[ledger.Merged], proposal.Slices)
	if proposal.FullyDelivered {
		text += " · fully delivered"
	}
	if proposal.Unknown > 0 {
		text += fmt.Sprintf(" · %d unknown", proposal.Unknown)
	}
	return text
}

func flags(proposal ledger.ProposalSummary) string {
	var text string
	if proposal.Archived {
		text += " [archived]"
	}
	if proposal.Retired {
		text += " [retired]"
	}
	return text
}

func wrap(text string, width int) string {
	return lipgloss.NewStyle().Width(max(width, 1)).Render(text)
}

func truncate(text string, width int) string {
	return lipgloss.NewStyle().MaxWidth(max(width, 1)).Render(text)
}

// indicator is one header indicator: its forms run from full to shortest.
type indicator struct {
	style lipgloss.Style
	forms []string
}

// fitIndicators renders indicators in the longest forms that fit width. Only
// when even the shortest forms do not fit are the first, least important
// indicators left out.
func fitIndicators(indicators []indicator, width int) string {
	wide, tight := mutedStyle.Render(" · "), mutedStyle.Render("·")
	render := func(indicators []indicator, level int) string {
		separator := wide
		if level > 0 {
			separator = tight
		}
		var parts []string
		for _, indicator := range indicators {
			parts = append(parts, indicator.style.Render(indicator.forms[min(level, len(indicator.forms)-1)]))
		}
		return strings.Join(parts, separator)
	}
	shortest := 0
	for _, indicator := range indicators {
		shortest = max(shortest, len(indicator.forms)-1)
	}
	for level := 0; level <= shortest; level++ {
		if line := render(indicators, level); lipgloss.Width(line) <= width {
			return line
		}
	}
	for len(indicators) > 1 && lipgloss.Width(render(indicators, shortest)) > width {
		indicators = indicators[1:]
	}
	return render(indicators, shortest)
}

// refForms names commit after prefix with a 12- and then a 7-character
// reference.
func refForms(prefix, commit string) []string {
	return []string{prefix + short(commit, 12), prefix + short(commit, 7)}
}

// short cuts a commit to at most length characters.
func short(commit string, length int) string {
	return commit[:min(len(commit), length)]
}

// keepEnd fits unstyled text to width by cutting its start, so a long
// breadcrumb keeps the current place.
func keepEnd(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width("…"+string(runes)) > width {
		runes = runes[1:]
	}
	return "…" + string(runes)
}

func clip(text string, height int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
