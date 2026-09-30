package browse

import (
	"fmt"
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
	return max(m.height-lipgloss.Height(header)-lipgloss.Height(footer), 1)
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
	// Indicators run from least to most important. A narrow terminal drops
	// them in that order, after the breadcrumb has given way. The footer
	// already says when a newer document is available.
	var indicators []string
	scrolled := func(view viewport.Model) {
		if view.TotalLineCount() > view.Height {
			indicators = append(indicators, mutedStyle.Render(fmt.Sprintf("scrolled %d%%", int(view.ScrollPercent()*100))))
		}
	}
	document := m.screen == documentScreen && m.currentDocument != nil
	switch {
	case m.screen == sliceScreen || m.screen == diagnosticsScreen:
		scrolled(m.detail)
	case document:
		scrolled(m.docViewport)
	}
	if m.includeArchived {
		indicators = append(indicators, mutedStyle.Render("archived shown"))
	}
	if document {
		identity := "current document"
		if commit := m.currentDocument.Reference.Commit; commit != m.snapshot.Revision {
			identity = "HISTORICAL " + commit[:min(len(commit), 12)]
		}
		indicators = append(indicators, mutedStyle.Render(identity))
	}
	revision := m.snapshot.Revision[:min(len(m.snapshot.Revision), 12)]
	if m.refreshFailure != nil {
		indicators = append(indicators, warningStyle.Bold(true).Render("NOT REFRESHED "+revision))
	} else {
		indicators = append(indicators, mutedStyle.Render(revision))
	}
	separator := mutedStyle.Render(" · ")
	for len(indicators) > 1 && lipgloss.Width(strings.Join(indicators, separator)) > m.width {
		indicators = indicators[1:]
	}
	right := strings.Join(indicators, separator)
	left := ""
	if room := m.width - lipgloss.Width(right) - 2; room > 0 {
		left = titleStyle.Render(keepEnd("skl browse › "+strings.Join(path, " › "), room))
	}
	gap := strings.Repeat(" ", max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 0))
	return truncate(left+gap+right, m.width)
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
	switch {
	case m.typing != nil:
		// Typing takes every other key as text; the prompt names its own keys.
		lines = append(lines, wrap(titleStyle.Render("Search names: ")+*m.typing+"█  (enter apply · esc cancel)", m.width))
	case m.help.ShowAll:
		lines = append(lines, m.allKeys()...)
	default:
		lines = append(lines, m.keyHelp(m.footerKeys()))
	}
	return strings.Join(lines, "\n")
}

// keyHelp lists as many of bindings as fit the width, always followed by
// help.
func (m Model) keyHelp(bindings []key.Binding) string {
	for count := len(bindings); count > 0; count-- {
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

// listBody renders the current list screen: its parent context, the list,
// and the selected entry's facts, side by side when the terminal is wide.
func (m Model) listBody(height int) string {
	context, title, rows, cursor, selected, empty := m.listContent()
	width := m.width
	top := ""
	if len(context) > 0 {
		top = wrap(strings.Join(context, "\n"), width) + "\n"
	}
	available := max(height-lipgloss.Height(top)-1, 2)
	title = truncate(titleStyle.Render(title), width)
	if len(rows) == 0 {
		return top + title + "\n" + wrap(empty, width)
	}
	if width >= wideLayout {
		listWidth := width / 2
		list := m.list(rows, cursor, available, listWidth-2)
		facts := clip(wrap(strings.Join(selected, "\n"), width-listWidth-2), available)
		return top + title + "\n" + lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(listWidth).Render(list), facts)
	}
	listHeight := min(len(rows), max(available/2, 3))
	list := m.list(rows, cursor, listHeight, width)
	facts := clip(wrap(strings.Join(selected, "\n"), width), max(available-listHeight-1, 1))
	return top + title + "\n" + list + "\n" + mutedStyle.Render(strings.Repeat("─", min(width, 40))) + "\n" + facts
}

// list renders rows in a window that keeps the cursor row visible. The
// cursor is marked by text as well as style.
func (m Model) list(rows []string, cursor, height, width int) string {
	start := 0
	if cursor >= height {
		start = cursor - height + 1
	}
	var lines []string
	for index := start; index < len(rows) && index < start+height; index++ {
		line := "  " + rows[index]
		if index == cursor && m.selectionMissing == "" {
			line = selectedStyle.Render(truncate("> "+rows[index], width))
		}
		lines = append(lines, truncate(line, width))
	}
	return strings.Join(lines, "\n")
}

// listContent supplies the current list screen's parent context, list title,
// rows, the cursor row, the selected entry's facts, and the text shown for an
// empty list.
func (m Model) listContent() (context []string, title string, rows []string, cursorRow int, selected []string, empty string) {
	cursor := m.cursor[m.screen]
	cursorRow = cursor
	switch m.screen {
	case overviewScreen:
		title = fmt.Sprintf("Projects (%d)", len(m.overview.Projects))
		empty = "No Projects are recorded in the configured ledger at this revision."
		for _, diagnostic := range m.overview.Diagnostics {
			context = append(context, warningStyle.Render(DiagnosticText(diagnostic)))
		}
		for _, project := range m.overview.Projects {
			row := project.Name + " — " + orUnknown(project.Repository) + " — " + plural(project.Slices, "slice")
			if project.Claimed > 0 {
				row += fmt.Sprintf(" · %d claimed", project.Claimed)
			}
			if project.Unknown > 0 {
				row += fmt.Sprintf(" · %d unknown", project.Unknown)
			}
			rows = append(rows, marked(project.Incomplete, row))
		}
		if len(rows) > 0 {
			selected = ProjectLines(m.overview.Projects[cursor])
		}
	case projectScreen:
		project := m.inventory.Project
		context = []string{"Project " + project.Name + " (" + orUnknown(project.Repository) + ") · " + tallyText(project.Tally)}
		if project.Incomplete {
			context = append(context, warningStyle.Render("! Incomplete: "+incompleteText(project.Diagnostics)+" in this Project"))
		}
		title = fmt.Sprintf("Proposals (%d)", len(m.inventory.Proposals))
		empty = fmt.Sprintf("No active Proposals. %d archived; press a to include them.", project.ArchivedProposals)
		if m.includeArchived {
			empty = "No Proposals are recorded in this Project."
		}
		for _, proposal := range m.inventory.Proposals {
			rows = append(rows, marked(proposal.Incomplete, proposal.Name+flags(proposal)+" — "+progressText(proposal)))
		}
		if len(rows) > 0 {
			selected = ProposalLines(m.inventory.Proposals[cursor])
		}
	case documentsScreen:
		if m.docContext == proposalScreen {
			context = []string{"Documents for Proposal " + m.project + "/" + m.proposal}
		} else {
			context = []string{"Documents for Slice " + m.project + "/" + m.item}
		}
		if m.documents != nil && m.documents.Archived {
			context = append(context, "Archive documents · current ledger revision "+m.documents.Revision)
		}
		title = fmt.Sprintf("Available documents (%d)", len(m.rowsForDocumentList()))
		empty = "No readable documents are available at this committed revision. See diagnostics, if any."
		if m.documents != nil {
			if count := len(m.documents.Diagnostics); count > 0 {
				context = append(context, warningStyle.Render(fmt.Sprintf("! %d diagnostics · d to inspect", count)))
			}
			context = append(context, optionalDocumentNotes(m.documents)...)
			for _, document := range m.documents.Documents {
				rows = append(rows, documentLabel(document))
			}
			if m.documents.Slice != "" {
				rows = append(rows, "Implementation report versions", "Watchdog report versions")
			}
			if cursor < len(m.documents.Documents) {
				document := m.documents.Documents[cursor]
				selected = []string{documentSummary(document), "Exact ledger identity: " + document.Reference.Commit + ":" + document.Reference.Path}
				if len(document.Diagnostics) > 0 {
					selected = append(selected, diagnosticLines(document.Diagnostics)...)
				}
				selected = append(selected, "Enter reads this committed document; d opens this set's diagnostics.")
			} else if len(rows) > 0 {
				selected = []string{"Enter discovers this phase's locally available content versions, even when no latest report exists."}
			}
		}
	case versionsScreen:
		context = []string{"Report content changes · newest first · exact ledger references; current Slice facts remain at the snapshot revision."}
		if m.versions != nil {
			if m.versions.Incomplete {
				context = append(context, warningStyle.Render("! Local history incomplete: "+m.versions.Diagnostics[0].Problem))
			} else if len(m.versions.Diagnostics) > 0 {
				context = append(context, warningStyle.Render(fmt.Sprintf("! %d version metadata diagnostics; select a version for details", len(m.versions.Diagnostics))))
			}
			title = fmt.Sprintf("%s report versions (%d)", m.versions.Phase, len(m.versions.Versions))
			empty = "No locally available report content versions."
			for _, version := range m.versions.Versions {
				label := version.Reference.Commit[:min(12, len(version.Reference.Commit))]
				if version.Report != nil {
					label += " · " + version.Report.Outcome
					if version.Report.Round != 0 {
						label += fmt.Sprintf(" · round %d", version.Report.Round)
					}
				}
				rows = append(rows, label+" · "+version.Reference.Path)
			}
			if len(rows) > 0 {
				version := m.versions.Versions[cursor]
				selected = []string{"Exact ledger version: " + version.Reference.Commit + ":" + version.Reference.Path, "Enter reads this version; esc returns to the open report."}
				selected = append(selected, diagnosticLines(version.Diagnostics)...)
			}
		}
	case referencesScreen:
		context = []string{"Structured ledger references · exact commit and path; unavailable references are never replaced."}
		if m.referencesFromDoc && m.currentDocument != nil {
			context = append(context, "From "+documentLabel(*m.currentDocument))
			context = append(context, diagnosticLines(m.currentDocument.Diagnostics)...)
		} else if m.slice == nil || m.slice.Claim == nil {
			context = append(context, "No current Slice Claim at ledger revision "+m.snapshot.Revision)
		} else {
			context = append(context, "From the current Slice Claim at ledger revision "+m.snapshot.Revision)
		}
		title = fmt.Sprintf("References (%d)", len(m.references))
		empty = "No structured exact ledger references are available. Metadata diagnostics remain with the document."
		for _, reference := range m.references {
			rows = append(rows, reference.Label+" — "+reference.Reference.Path)
		}
		if len(rows) > 0 {
			reference := m.references[cursor]
			selected = []string{
				"Label: " + reference.Label,
				"Exact ledger reference: " + reference.Reference.Commit + ":" + reference.Reference.Path,
				"Enter follows this exact reference; source-code revisions are not browsed.",
			}
		}
	case proposalScreen:
		proposal := m.members.Proposal
		context = []string{
			"Proposal " + proposal.Name + flags(proposal) + " — " + orUnknown(proposal.ParentTitle),
			"Delivery: " + deliveryText(proposal) + " · Parent issue: " + attachmentText(proposal.ParentIssue),
		}
		if proposal.Incomplete {
			context = append(context, warningStyle.Render("! Incomplete: "+incompleteText(proposal.Diagnostics)+" in this Proposal"))
		}
		title = fmt.Sprintf("Slices (%d)", len(m.members.Slices))
		empty = "This Proposal records no Slices."
		for _, slice := range m.members.Slices {
			rows = append(rows, SliceRow(slice.Slice, slice))
		}
		if len(rows) > 0 {
			selected = append(SliceSummaryLines(m.members.Slices[cursor]), "Press d to read this Proposal's documents.")
		}
	case factsScreen:
		context = m.findingContext()
		facets := m.search.Facets
		if facets.UnknownLifecycle > 0 || facets.UnknownClaim > 0 {
			context = append(context, warningStyle.Render(fmt.Sprintf("! Counted only under Any: %d with unknown lifecycle, %d with unknown claim", facets.UnknownLifecycle, facets.UnknownClaim)))
		}
		title = "Facts — select one to find its Slices"
		for _, option := range factOptions {
			rows = append(rows, m.factRow(option))
		}
		selected = []string{"Finds: " + SelectionText(factOptions[cursor].apply(m.search.Query))}
	case resultsScreen:
		context = m.findingContext()
		if count := len(m.resultDiagnostics()); count > 0 {
			context = append(context, warningStyle.Render(fmt.Sprintf("! %d diagnostics · d to inspect", count)))
		}
		title = fmt.Sprintf("Slices (%d)", m.search.Matched)
		if m.search.Undecided > 0 {
			title = fmt.Sprintf("Slices (%d matched, %d undecided)", m.search.Matched, m.search.Undecided)
		}
		empty = ResultText(m.search)
		results := m.results()
		for index, found := range results {
			if index == 0 || found.heading != results[index-1].heading {
				rows = append(rows, titleStyle.Render(found.heading))
			}
			if index == cursor {
				cursorRow = len(rows)
			}
			rows = append(rows, MatchRow(found.match))
		}
		if len(results) > 0 {
			chosen := results[cursor]
			selected = append([]string{"Project: " + chosen.project}, SliceSummaryLines(chosen.match.SliceSummary)...)
		}
	}
	if m.selectionMissing != "" {
		cursorRow, selected = -1, nil
	}
	return context, title, rows, cursorRow, selected, empty
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
// find, marked when it is the current selection.
func (m Model) factRow(option factOption) string {
	facets := m.search.Facets
	counts, unknown, current, label := facets.Lifecycles, facets.UnknownLifecycle, m.search.Query.Lifecycles, "Any lifecycle"
	if option.claim {
		counts, unknown, current, label = facets.Claims, facets.UnknownClaim, m.search.Query.Claims, "Any claim"
	}
	count := counts[option.value]
	switch {
	case option.value == "":
		count = unknown
		for _, value := range counts {
			count += value
		}
	case option.claim:
		label = claimLabels[option.value]
	default:
		label = lifecycleLabel(option.value)
	}
	mark := "  "
	if (option.value == "" && len(current) == 0) || (len(current) == 1 && current[0] == option.value) {
		mark = "✓ "
	}
	return fmt.Sprintf("%s%s (%d)", mark, label, count)
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

func marked(incomplete bool, text string) string {
	if incomplete {
		return "! " + text
	}
	return text
}

func wrap(text string, width int) string {
	return lipgloss.NewStyle().Width(max(width, 1)).Render(text)
}

func truncate(text string, width int) string {
	return lipgloss.NewStyle().MaxWidth(max(width, 1)).Render(text)
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
