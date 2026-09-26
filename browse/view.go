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
	case m.screen == sliceScreen:
		body = m.detail.View()
	default:
		body = m.listBody(height)
	}
	return strings.Join([]string{header, clip(body, height), footer}, "\n")
}

func (m Model) bodyHeight(header, footer string) int {
	return max(m.height-lipgloss.Height(header)-lipgloss.Height(footer), 3)
}

// layoutDetail fits the Slice detail viewport to the current terminal.
func (m *Model) layoutDetail() {
	m.detail.Width = m.width
	m.detail.Height = m.bodyHeight(m.header(), m.footer())
	if m.screen == sliceScreen && m.slice != nil {
		m.detail.SetContent(wrap(strings.Join(SliceLines(m.slice), "\n"), m.width))
	}
}

func (m Model) header() string {
	var path []string
	switch {
	case m.screen == factsScreen:
		path = []string{"Find slices", "Facts"}
	case m.screen == resultsScreen:
		path = []string{"Find slices"}
	case m.screen == sliceScreen && m.parent[sliceScreen] == resultsScreen:
		path = []string{"Find slices", m.project, m.item}
	default:
		path = []string{"Projects"}
		if m.screen >= projectScreen {
			path = append(path, m.project)
		}
		if m.screen >= proposalScreen {
			path = append(path, m.proposal)
		}
		if m.screen == sliceScreen {
			_, slice, _ := strings.Cut(m.item, "/")
			path = append(path, slice)
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
	facts := "committed ledger " + revision + " · " + archived
	if m.screen == sliceScreen && m.detail.TotalLineCount() > m.detail.Height {
		facts += fmt.Sprintf(" · scrolled %d%%", int(m.detail.ScrollPercent()*100))
	}
	return truncate(titleStyle.Render("skl browse › "+strings.Join(path, " › ")), m.width) + "\n" +
		truncate(mutedStyle.Render(facts), m.width)
}

func (m Model) footer() string {
	lines := []string{}
	if m.status != "" {
		lines = append(lines, wrap(m.status, m.width))
	}
	if m.typing != nil {
		lines = append(lines, wrap(titleStyle.Render("Search names: ")+*m.typing+"█  (enter apply · esc cancel)", m.width))
	}
	return strings.Join(append(lines, truncate(m.help.View(m.keys), m.width)), "\n")
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
		if index == cursor {
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
			selected = SliceSummaryLines(m.members.Slices[cursor])
		}
	case factsScreen:
		context = m.findingContext()
		facets := m.search.Facets
		if facets.UnknownLifecycle > 0 || facets.UnknownClaim > 0 {
			context = append(context, warningStyle.Render(fmt.Sprintf("! Not counted: %d with unknown lifecycle, %d with unknown claim", facets.UnknownLifecycle, facets.UnknownClaim)))
		}
		title = "Facts — select one to find its Slices"
		for _, option := range factOptions {
			rows = append(rows, m.factRow(option))
		}
		selected = []string{"Finds: " + SelectionText(factOptions[cursor].apply(m.search.Query))}
	case resultsScreen:
		context = m.findingContext()
		for _, diagnostic := range m.search.Diagnostics {
			context = append(context, warningStyle.Render(DiagnosticText(diagnostic)))
		}
		for _, project := range m.search.Projects {
			for _, diagnostic := range project.Diagnostics {
				context = append(context, warningStyle.Render(DiagnosticText(diagnostic)))
			}
		}
		title = fmt.Sprintf("Slices (%d)", m.search.Matched)
		if m.search.Undecided > 0 {
			title = fmt.Sprintf("Slices (%d matched, %d undecided)", m.search.Matched, m.search.Undecided)
		}
		empty = ResultText(m.search)
		rows, cursorRow = m.resultRows(cursor)
		if results := m.results(); len(results) > 0 {
			chosen := results[cursor]
			selected = append([]string{"Project: " + chosen.project}, SliceSummaryLines(chosen.match.SliceSummary)...)
		}
	}
	return context, title, rows, cursorRow, selected, empty
}

// findingContext states the current selection and its result.
func (m Model) findingContext() []string {
	result := ResultText(m.search)
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

// resultRows lists the found Slices under their group headings, prefixed by
// Project when every Project is searched, and returns the row of the cursor.
func (m Model) resultRows(cursor int) (rows []string, cursorRow int) {
	index := 0
	add := func(heading string, matches []ledger.SliceMatch) {
		rows = append(rows, titleStyle.Render(heading))
		for _, match := range matches {
			if index == cursor {
				cursorRow = len(rows)
			}
			rows = append(rows, MatchRow(match))
			index++
		}
	}
	for _, project := range m.search.Projects {
		prefix := ""
		if m.search.Query.Project == "" {
			prefix = project.Name + " · "
		}
		for _, group := range project.Groups {
			add(prefix+GroupTitle(group), group.Slices)
		}
		if len(project.Undecided) > 0 {
			add(prefix+UndecidedTitle, project.Undecided)
		}
	}
	return rows, cursorRow
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

func clip(text string, height int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
