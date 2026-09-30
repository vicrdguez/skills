package browse

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vicrdguez/skills/ledger"
)

// wideLayout is the width from which the selected entry's facts sit beside
// the list instead of below it.
const wideLayout = 100

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	selectedStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
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
	case m.screen == sliceScreen || m.screen == diagnosticsScreen:
		body = m.detail.View()
	case m.screen == documentScreen:
		body = m.docViewport.View()
	default:
		body = m.listBody(height)
	}
	return strings.Join([]string{header, clip(body, height), footer}, "\n")
}

func (m Model) bodyHeight(header, footer string) int {
	return max(m.height-lipgloss.Height(header)-lipgloss.Height(footer), 3)
}

// layoutDetail fits both viewports to the current terminal and marks the
// Slice's selected followable relationship as well as styling it.
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
		if m.slice != nil {
			offset := m.detail.YOffset
			lines, relations := sliceLines(m.slice)
			m.relation = min(m.relation, max(len(relations)-1, 0))
			for index, relation := range relations {
				if index == m.relation {
					lines[relation.line] = selectedStyle.Render("> " + lines[relation.line])
				} else {
					lines[relation.line] = "  " + lines[relation.line]
				}
			}
			var rows []string
			m.selectedRow = 0
			for index, line := range lines {
				if len(relations) > 0 && index == relations[m.relation].line {
					m.selectedRow = len(rows)
				}
				rows = append(rows, strings.Split(wrap(line, m.width), "\n")...)
			}
			m.detail.SetContent(strings.Join(rows, "\n"))
			m.detail.SetYOffset(offset)
		}
	}

	m.docViewport.Width, m.docViewport.Height = m.width, height
	if m.screen == documentScreen && m.currentDocument != nil {
		offset := m.docViewport.YOffset
		content, problem := m.documentContent(m.currentDocument, m.width)
		m.renderProblem = problem
		m.docViewport.SetContent(content)
		m.docViewport.SetYOffset(offset)
	}
}

func (m Model) header() string {
	context := m.screen
	if isDocumentOverlay(m.screen) || (m.screen == diagnosticsScreen && m.diagnosticReturn == documentsScreen) {
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
	case documentsScreen:
		path = append(path, "documents")
	case versionsScreen:
		path = append(path, "report versions")
	case referencesScreen:
		if m.referencesFromDoc && m.currentDocument != nil {
			path = append(path, documentLabel(*m.currentDocument))
		}
		path = append(path, "references")
	case documentScreen:
		if m.documentReturn == referencesScreen {
			path = append(path, "references")
		}
		if m.currentDocument != nil {
			path = append(path, documentLabel(*m.currentDocument))
		}
	case diagnosticsScreen:
		if m.diagnosticReturn == documentsScreen {
			path = append(path, "documents", "diagnostics")
		}
	}
	archived := "archived hidden"
	if m.includeArchived {
		archived = "archived shown"
	}
	revision := m.snapshot.Revision
	if len(revision) > 12 {
		revision = revision[:12]
	}
	facts := "current committed ledger " + revision + " · " + archived
	if m.refreshFailure != nil {
		facts = "NOT REFRESHED · last committed ledger " + revision + " · " + archived
	}
	if (m.screen == sliceScreen || m.screen == diagnosticsScreen) && m.detail.TotalLineCount() > m.detail.Height {
		facts += fmt.Sprintf(" · scrolled %d%%", int(m.detail.ScrollPercent()*100))
	}
	if m.screen == documentScreen && m.currentDocument != nil {
		identity := "current document"
		if m.currentDocument.Reference.Commit != m.snapshot.Revision {
			identity = "HISTORICAL document"
		}
		facts += " · " + identity
		if m.docViewport.TotalLineCount() > m.docViewport.Height {
			facts += fmt.Sprintf(" · scrolled %d%%", int(m.docViewport.ScrollPercent()*100))
		}
		if m.newerDocument {
			facts += " · newer document available"
		}
		facts += " · ref " + m.currentDocument.Reference.Commit[:min(len(m.currentDocument.Reference.Commit), 12)]
	}
	return truncate(titleStyle.Render("skl browse › "+strings.Join(path, " › ")), m.width) + "\n" +
		truncate(mutedStyle.Render(facts), m.width)
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
	if m.screen == documentScreen && m.newerDocument {
		lines = append(lines, wrap("Newer document available; esc to documents and select latest", m.width))
	}
	if m.screen == documentScreen && m.renderProblem != "" {
		lines = append(lines, wrap(warningStyle.Render("! "+m.renderProblem+"; showing recorded text"), m.width))
	}
	if m.typing != nil {
		lines = append(lines, wrap(titleStyle.Render("Search names: ")+*m.typing+"█  (enter apply · esc cancel)", m.width))
	}
	return strings.Join(append(lines, truncate(m.help.View(m.helpKeys()), m.width)), "\n")
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
	if width >= wideLayout {
		// The list widens beyond half the terminal, up to three fifths, to
		// fit its rows; the preview wraps into the rest.
		listWidth := min(max(content.width()+paneFrame, width/2), width*3/5)
		return top + lipgloss.JoinHorizontal(lipgloss.Top,
			pane(content.title, position, content.lines(listWidth-paneFrame, available-2), listWidth, available),
			pane(content.previewTitle, "", wrapLines(preview, width-listWidth-paneFrame), width-listWidth, available))
	}
	// Stacked, the list takes up to half the height and the preview the
	// rest; a preview too short for its border gives way to the list.
	listHeight := min(len(content.lines(width-paneFrame, available))+2, max(available/2, 3))
	if available-listHeight < 3 {
		return top + pane(content.title, position, content.lines(width-paneFrame, available-2), width, available)
	}
	return top + pane(content.title, position, content.lines(width-paneFrame, listHeight-2), width, listHeight) + "\n" +
		pane(content.previewTitle, "", wrapLines(preview, width-paneFrame), width, available-listHeight)
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

// paneFrame is the width of a pane's side borders and their padding.
const paneFrame = 4

// pane frames lines in a rounded border exactly width cells wide and height
// lines tall, with its title in the top edge and note, when set, in the
// bottom edge. Lines beyond its height are clipped.
func pane(title, note string, lines []string, width, height int) string {
	inner := max(width-paneFrame, 1)
	title = truncate(title, max(width-5, 1))
	edge := mutedStyle.Render
	framed := []string{edge("╭─ ") + titleStyle.Render(title) + edge(" "+strings.Repeat("─", max(width-5-lipgloss.Width(title), 0))+"╮")}
	for index := 0; index < height-2; index++ {
		line := ""
		if index < len(lines) {
			line = truncate(lines[index], inner)
		}
		framed = append(framed, edge("│ ")+line+strings.Repeat(" ", max(inner-lipgloss.Width(line), 0))+edge(" │"))
	}
	bottom := "╰" + strings.Repeat("─", max(width-2, 0)) + "╯"
	if note != "" {
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
			gap = 2
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

// shortestName is the fewest cells a shortened first column keeps: the name
// yields before the lifecycle and Claim do. A pane too narrow for the whole
// columns even then cuts rows at its edge instead.
const shortestName = 3

// lines renders the list in a window of height lines that keeps the cursor
// row visible. The cursor row is marked by text as well as style.
func (l listing) lines(width, height int) []string {
	if len(l.rows) == 0 {
		return wrapLines([]string{l.empty}, width)
	}
	widths, marks := l.layout()
	if l.whole > 0 {
		excess := rowWidth(widths[:min(l.whole, len(widths))], marks) - width
		if excess > 0 && widths[0]-excess >= shortestName {
			widths[0] -= excess
		}
	}
	start := max(l.cursorRow-height+1, 0)
	var lines []string
	for index := start; index < len(l.rows) && index < start+height; index++ {
		lines = append(lines, l.rows[index].render(widths, marks, index == l.cursorRow, width))
	}
	return lines
}

// render draws one row with its columns padded to widths and, when marks is
// set, a column for the unknown-facts marker.
func (r row) render(widths []int, marks, selected bool, width int) string {
	if r.columns == nil {
		return truncate("  "+r.heading, width)
	}
	line := ""
	add := func(style lipgloss.Style, text string) {
		// Selection reverses each span and leaves its weight alone, since a
		// lifecycle's weight is part of its look.
		if selected {
			style = style.Reverse(true)
		}
		if text != "" {
			line += style.Render(text)
		}
	}
	plain := lipgloss.NewStyle()
	if selected {
		add(plain, "> ")
	} else {
		add(plain, "  ")
	}
	if marks && r.marked {
		add(warningStyle, "!")
		add(plain, " ")
	} else if marks {
		add(plain, "  ")
	}
	gap := ""
	for index, columnWidth := range widths {
		if columnWidth == 0 {
			continue
		}
		var column span
		if index < len(r.columns) {
			column = r.columns[index]
		}
		column.text = shorten(column.text, columnWidth)
		add(plain, gap)
		add(column.style, column.text)
		add(plain, strings.Repeat(" ", columnWidth-lipgloss.Width(column.text)))
		gap = "  "
	}
	if selected {
		add(plain, strings.Repeat(" ", max(width-lipgloss.Width(line), 0)))
	}
	return truncate(line, width)
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
	case documentsScreen:
		if m.docContext == proposalScreen {
			l.context = []string{"Documents for Proposal " + m.project + "/" + m.proposal}
		} else {
			l.context = []string{"Documents for Slice " + m.project + "/" + m.item}
		}
		if m.documents != nil && m.documents.Archived {
			l.context = append(l.context, "Archive documents · current ledger revision "+m.documents.Revision)
		}
		l.title = fmt.Sprintf("Available documents (%d)", len(m.rowsForDocumentList()))
		l.previewTitle = "Document"
		l.empty = "No readable documents are available at this committed revision. See diagnostics, if any."
		if m.documents != nil {
			if count := len(m.documents.Diagnostics); count > 0 {
				l.context = append(l.context, warningStyle.Render(fmt.Sprintf("! %d diagnostics · d to inspect", count)))
			}
			l.context = append(l.context, optionalDocumentNotes(m.documents)...)
			for _, document := range m.documents.Documents {
				l.rows = append(l.rows, row{columns: texts(documentLabel(document))})
			}
			if m.documents.Slice != "" {
				l.rows = append(l.rows, row{columns: texts("Implementation report versions")}, row{columns: texts("Watchdog report versions")})
			}
			if cursor < len(m.documents.Documents) {
				document := m.documents.Documents[cursor]
				l.preview = []string{documentSummary(document), "Exact ledger identity: " + document.Reference.Commit + ":" + document.Reference.Path}
				if len(document.Diagnostics) > 0 {
					l.preview = append(l.preview, diagnosticLines(document.Diagnostics)...)
				}
				l.preview = append(l.preview, "Enter reads this committed document; d opens this set's diagnostics.")
			} else if len(l.rows) > 0 {
				l.preview = []string{"Enter discovers this phase's locally available content versions, even when no latest report exists."}
			}
		}
	case versionsScreen:
		l.context = []string{"Report content changes · newest first · exact ledger references; current Slice facts remain at the snapshot revision."}
		l.previewTitle = "Version"
		if m.versions != nil {
			if m.versions.Incomplete {
				l.context = append(l.context, warningStyle.Render("! Local history incomplete: "+m.versions.Diagnostics[0].Problem))
			} else if len(m.versions.Diagnostics) > 0 {
				l.context = append(l.context, warningStyle.Render(fmt.Sprintf("! %d version metadata diagnostics; select a version for details", len(m.versions.Diagnostics))))
			}
			l.title = fmt.Sprintf("%s report versions (%d)", m.versions.Phase, len(m.versions.Versions))
			l.empty = "No locally available report content versions."
			for _, version := range m.versions.Versions {
				label := version.Reference.Commit[:min(12, len(version.Reference.Commit))]
				if version.Report != nil {
					label += " · " + version.Report.Outcome
					if version.Report.Round != 0 {
						label += fmt.Sprintf(" · round %d", version.Report.Round)
					}
				}
				l.rows = append(l.rows, row{columns: texts(label + " · " + version.Reference.Path)})
			}
			if len(l.rows) > 0 {
				version := m.versions.Versions[cursor]
				l.preview = []string{"Exact ledger version: " + version.Reference.Commit + ":" + version.Reference.Path, "Enter reads this version; esc returns to the open report."}
				l.preview = append(l.preview, diagnosticLines(version.Diagnostics)...)
			}
		}
	case referencesScreen:
		l.context = []string{"Structured ledger references · exact commit and path; unavailable references are never replaced."}
		if m.referencesFromDoc && m.currentDocument != nil {
			l.context = append(l.context, "From "+documentLabel(*m.currentDocument))
			l.context = append(l.context, diagnosticLines(m.currentDocument.Diagnostics)...)
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
			l.preview = append(SliceSummaryLines(m.members.Slices[cursor]), "Press d to read this Proposal's documents.")
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
			chosen := results[cursor]
			l.preview = append([]string{"Project: " + chosen.project}, SliceSummaryLines(chosen.match.SliceSummary)...)
		}
	}
	if m.selectionMissing != "" {
		l.cursorRow, l.preview = -1, nil
	}
	return l
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
	if m.diagnosticReturn == documentsScreen {
		if m.documents == nil {
			return nil
		}
		return m.documents.Diagnostics
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

func clip(text string, height int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
