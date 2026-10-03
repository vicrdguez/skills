package browse

import (
	"fmt"
	"maps"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/vicrdguez/skills/ledger"
)

type screen int

const (
	overviewScreen screen = iota
	projectScreen
	proposalScreen
	sliceScreen
	factsScreen
	resultsScreen
	diagnosticsScreen
	documentsScreen
	referencesScreen
	versionsScreen
	documentScreen
)

// factOption is one navigable lifecycle or Claim fact of the facts screen.
// An empty value selects any along its dimension.
type factOption struct {
	claim bool
	value string
}

var factOptions = func() []factOption {
	options := []factOption{{}}
	for _, lifecycle := range ledger.Lifecycles {
		options = append(options, factOption{value: lifecycle})
	}
	options = append(options, factOption{claim: true})
	for _, claim := range ledger.Claims {
		options = append(options, factOption{claim: true, value: claim})
	}
	return options
}()

// apply narrows the dimension of option in query to its value.
func (option factOption) apply(query ledger.SliceQuery) ledger.SliceQuery {
	var values []string
	if option.value != "" {
		values = []string{option.value}
	}
	if option.claim {
		query.Claims = values
	} else {
		query.Lifecycles = values
	}
	return query
}

// result is one selectable Slice of the results screen under its group
// heading.
type result struct {
	project, heading string
	match            ledger.SliceMatch
}

// Options are the startup choices of one browsing session.
type Options struct {
	// Project is the initial Project; empty starts at the overview.
	Project string
	// Notice explains the startup selection, such as a checkout fallback.
	Notice string
	// Open requests external-browser opening of one URL.
	Open func(url string) error
	// DarkBackground selects the dark Markdown style over the light one. The
	// caller decides it before the program starts; the browser never asks the
	// terminal.
	DarkBackground bool
}

type keyMap struct {
	Up, Down, PageUp, PageDown, Enter, Back, Next, Previous, Projects, Archived, Search, Facts, Group, Scope, Diagnostics, Documents, References, Versions, Issue, PullRequest, Refresh, Help, Quit key.Binding
}

// all lists every binding in the order `?` shows them.
func (k keyMap) all() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.PageUp, k.PageDown, k.Enter, k.Back, k.Next, k.Previous, k.Projects, k.Archived, k.Search, k.Facts, k.Group, k.Scope, k.Diagnostics, k.Documents, k.References, k.Versions, k.Issue, k.PullRequest, k.Refresh, k.Help, k.Quit}
}

func newKeyMap() keyMap {
	return keyMap{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:      key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown:    key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
		Enter:       key.NewBinding(key.WithKeys("enter", "right", "l"), key.WithHelp("enter", "open")),
		Back:        key.NewBinding(key.WithKeys("esc", "backspace", "left", "h"), key.WithHelp("esc", "back")),
		Next:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next relation")),
		Previous:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous relation")),
		Projects:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "switch project")),
		Archived:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle archived")),
		Documents:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "documents")),
		References:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "references")),
		Versions:    key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "report versions")),
		Search:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search names")),
		Facts:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "find by lifecycle or claim")),
		Group:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "toggle grouping")),
		Scope:       key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "toggle scope")),
		Diagnostics: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diagnostics")),
		Issue:       key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "open issue")),
		PullRequest: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "open PR")),
		Refresh:     key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh ledger")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// Model is one browsing session over a committed ledger Snapshot. Selection
// and navigation live only in the running session.
type Model struct {
	snapshot       *ledger.Snapshot
	open           func(string) error
	darkBackground bool
	keys           keyMap
	help           help.Model
	detail         viewport.Model
	docViewport    viewport.Model

	location
	// selectedRow is the detail row where the selected relationship starts.
	selectedRow int
	// history holds the browsing context each followed relationship left,
	// most recent last; back restores it.
	history []frame
	// parent is the screen that back returns to from the Slice detail and
	// from finding.
	parent map[screen]screen

	// Finding suspends its origin and relationship history while results
	// and their own relationships are browsed.
	context        frame
	contextHistory []frame
	// query is the session's Slice selection; typing holds its name search
	// while it is edited.
	query  ledger.SliceQuery
	typing *string

	overview  *ledger.Overview
	inventory *ledger.ProjectInventory
	members   *ledger.ProposalDetail
	slice     *ledger.SliceDetail
	search    *ledger.SliceSearch
	failure   error

	docContext             screen
	documents              *ledger.DocumentList
	diagnosticReturn       screen
	diagnosticOriginDetail viewport.Model
	currentDocument        *ledger.Document
	documentReturn         screen
	documentHistory        []documentFrame
	references             []ledger.LabeledReference
	referenceOrigin        screen
	referencesFromDoc      bool
	referenceHistory       []referenceFrame
	versions               *ledger.ReportVersions
	versionsReturn         screen
	renderProblem          string
	// Opened document presentation is anchored at opening, not rewritten by
	// a later current-facts refresh.
	openedContext  string
	openedRevision string

	refreshRequest   uint64
	refreshHandled   uint64
	refreshFailure   error
	selectionMissing string
	missingIdentity  string
	missingScreen    screen
	newerDocument    bool

	width, height int
	status        string
}

// location is the browsing context: the screen, its selections, and the
// archive choice.
type location struct {
	screen          screen
	includeArchived bool
	project         string
	proposal        string
	archived        bool
	item            string
	cursor          map[screen]int
	// relation selects the current Slice's followable relationship.
	relation int
}

// frame is the browsing context restored by navigating back from a followed
// relationship, with the Slice detail's scroll offset.
type frame struct {
	location
	offset int
}

type referenceFrame struct {
	references   []ledger.LabeledReference
	origin       screen
	fromDocument bool
	cursor       int
	returnScreen screen
}

// openedMsg reports the outcome of one explicit external-browser request.
type openedMsg struct {
	url string
	err error
}

// New starts a session at the selected Project, or at the overview.
func New(snapshot *ledger.Snapshot, options Options) Model {
	model := Model{
		snapshot: snapshot, open: options.Open, darkBackground: options.DarkBackground, keys: newKeyMap(), help: help.New(),
		detail: viewport.New(80, 10), docViewport: viewport.New(80, 10),
		location: location{cursor: map[screen]int{}},
		parent:   map[screen]screen{sliceScreen: proposalScreen},
		width:    80, height: 24, status: options.Notice,
	}
	if model.open == nil {
		model.open = systemOpen
	}
	if options.Project != "" {
		model.screen, model.project = projectScreen, options.Project
	}
	model.load()
	return model
}

// footerKeys lists the bindings that act on the current screen, mirroring
// the conditions in key. The most specific come first, so a narrow footer
// trims the general ones. Movement (up, down, page, shift+tab) acts widely
// and is left to `?`, keeping the room for the screen's actions.
func (m Model) footerKeys() []key.Binding {
	keys := m.keys
	var bindings []key.Binding
	add := func(binding key.Binding, acts bool) {
		if acts {
			bindings = append(bindings, binding)
		}
	}
	relations := len(m.relations()) > 0
	follow := keys.Enter
	follow.SetHelp("enter", "follow relation")
	add(follow, relations)
	add(keys.Enter, m.screen != sliceScreen && m.canEnter())
	add(keys.Next, relations)
	add(keys.Back, m.screen != overviewScreen)
	add(keys.Group, m.finding())
	add(keys.Scope, m.finding() && (m.query.Project != "" || m.project != ""))
	diagnostics := keys.Diagnostics
	if m.screen == resultsScreen {
		diagnostics.SetHelp("d", "result diagnostics")
	}
	add(diagnostics, m.screen == resultsScreen || m.screen == documentsScreen)
	add(keys.Documents, (m.screen == proposalScreen && m.members != nil) || (m.screen == sliceScreen && m.slice != nil))
	add(keys.References, (m.screen == documentScreen && m.currentDocument != nil) ||
		(m.screen == sliceScreen && m.slice != nil && m.slice.Claim != nil))
	add(keys.Versions, m.readingReport())
	issue := keys.Issue
	if m.screen == proposalScreen {
		issue.SetHelp("i", "open parent issue")
	}
	_, _, issueRecorded := m.attachmentLink(true)
	add(issue, issueRecorded)
	_, _, pullRequestRecorded := m.attachmentLink(false)
	add(keys.PullRequest, pullRequestRecorded)
	add(keys.Facts, !m.inDocuments() && m.screen != factsScreen)
	add(keys.Search, !m.inDocuments())
	add(keys.Archived, !m.inDocuments())
	add(keys.Projects, m.screen != overviewScreen)
	return append(bindings, keys.Refresh, keys.Quit)
}

// Refresh requests are asynchronous so a slow ledger read cannot stall
// navigation. A periodic check is local and does not fetch or reconcile.
type refreshTick struct{}
type refreshResult struct {
	request  uint64
	snapshot *ledger.Snapshot
	err      error
}

func refreshTimer() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return refreshTick{} })
}

func (m Model) Init() tea.Cmd { return refreshTimer() }

func (m *Model) requestRefresh() tea.Cmd {
	m.refreshRequest++
	request, snapshot := m.refreshRequest, m.snapshot
	return func() tea.Msg {
		fresh, err := snapshot.Refresh()
		return refreshResult{request: request, snapshot: fresh, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layoutDetail()
	case refreshTick:
		return m, tea.Batch(refreshTimer(), m.requestRefresh())
	case refreshResult:
		if msg.request <= m.refreshHandled {
			return m, nil
		}
		m.refreshHandled = msg.request
		if msg.err != nil {
			m.refreshFailure = msg.err
			return m, nil
		}
		// Commands can execute in a different order from issuance or delivery.
		// Check the committed HEAD at publication, not the request number alone.
		current, err := m.snapshot.CurrentRevision()
		if err != nil {
			m.refreshFailure = err
			return m, nil
		}
		if msg.snapshot.Revision != current {
			return m, m.requestRefresh()
		}
		m.refreshFailure = nil
		if msg.snapshot.Revision != m.snapshot.Revision {
			m.publish(msg.snapshot)
		}
	case openedMsg:
		if msg.err != nil {
			m.status = "Could not open " + msg.url + ": " + msg.err.Error()
		} else {
			m.status = "Requested " + msg.url + " in the external browser"
		}
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.typing != nil {
		return m.typeSearch(msg)
	}
	if key.Matches(msg, m.keys.Refresh) {
		return m, m.requestRefresh()
	}
	finding := m.finding()
	documentOverlay := m.inDocuments()
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.layoutDetail()
	case key.Matches(msg, m.keys.Up):
		m.move(-1)
	case key.Matches(msg, m.keys.Down):
		m.move(1)
	case key.Matches(msg, m.keys.PageUp) && (m.screen == sliceScreen || m.screen == diagnosticsScreen):
		m.detail.PageUp()
	case key.Matches(msg, m.keys.PageDown) && (m.screen == sliceScreen || m.screen == diagnosticsScreen):
		m.detail.PageDown()
	case key.Matches(msg, m.keys.PageUp) && m.screen == documentScreen:
		m.docViewport.PageUp()
	case key.Matches(msg, m.keys.PageDown) && m.screen == documentScreen:
		m.docViewport.PageDown()
	case key.Matches(msg, m.keys.Next) && m.screen == sliceScreen:
		m.selectRelation(1)
	case key.Matches(msg, m.keys.Previous) && m.screen == sliceScreen:
		m.selectRelation(-1)
	case key.Matches(msg, m.keys.Enter) && m.screen == sliceScreen:
		m.follow()
	case key.Matches(msg, m.keys.Enter):
		m.enter()
	case key.Matches(msg, m.keys.Back):
		m.back()
	case key.Matches(msg, m.keys.Projects):
		m.switchProject()
	case key.Matches(msg, m.keys.Archived) && !documentOverlay:
		m.includeArchived = !m.includeArchived
		m.status = "Archived proposals hidden"
		if m.includeArchived {
			m.status = "Archived proposals shown"
		}
		m.load()
	case key.Matches(msg, m.keys.Search) && !documentOverlay:
		text := ""
		m.typing = &text
		m.layoutDetail()
	case key.Matches(msg, m.keys.Facts) && !documentOverlay:
		m.find(factsScreen)
	case key.Matches(msg, m.keys.Group) && finding:
		m.query.GroupBy = ledger.GroupByLifecycle
		if m.search != nil && m.search.Query.GroupBy == ledger.GroupByLifecycle {
			m.query.GroupBy = ledger.GroupByProposal
		}
		m.cursor[resultsScreen] = 0
		m.load()
	case key.Matches(msg, m.keys.Scope) && finding:
		m.toggleScope()
	case key.Matches(msg, m.keys.Diagnostics) && (m.screen == resultsScreen || m.screen == documentsScreen):
		m.diagnosticReturn = m.screen
		m.diagnosticOriginDetail = m.detail
		m.screen = diagnosticsScreen
		m.layoutDetail()
		m.detail.GotoTop()
	case key.Matches(msg, m.keys.Issue):
		return m, m.openAttachment(true)
	case key.Matches(msg, m.keys.PullRequest):
		return m, m.openAttachment(false)
	case key.Matches(msg, m.keys.Documents):
		m.openDocuments()
	case key.Matches(msg, m.keys.References):
		m.openReferences()
	case key.Matches(msg, m.keys.Versions):
		m.openVersions()
	}
	return m, nil
}

func (m *Model) move(delta int) {
	switch m.screen {
	case sliceScreen, diagnosticsScreen:
		if delta < 0 {
			m.detail.LineUp(1)
		} else {
			m.detail.LineDown(1)
		}
		return
	case documentScreen:
		if delta < 0 {
			m.docViewport.LineUp(1)
		} else {
			m.docViewport.LineDown(1)
		}
		return
	}
	count := m.rows()
	if count == 0 {
		return
	}
	m.cursor[m.screen] = min(max(m.cursor[m.screen]+delta, 0), count-1)
	m.selectionMissing, m.missingIdentity = "", ""
}

// rows counts the selectable entries of the current list screen.
func (m Model) rows() int {
	switch m.screen {
	case overviewScreen:
		if m.overview != nil {
			return len(m.overview.Projects)
		}
	case projectScreen:
		if m.inventory != nil {
			return len(m.inventory.Proposals)
		}
	case proposalScreen:
		if m.members != nil {
			return len(m.members.Slices)
		}
	case factsScreen:
		if m.search != nil {
			return len(factOptions)
		}
	case resultsScreen:
		if m.search != nil {
			return len(m.results())
		}
	case documentsScreen:
		if m.documents != nil {
			count := len(m.documents.Documents)
			if m.documents.Slice != "" {
				count += 2 // Each phase history remains accessible if its latest report is absent.
			}
			return count
		}
	case referencesScreen:
		return len(m.references)
	case versionsScreen:
		if m.versions != nil {
			return len(m.versions.Versions)
		}
	}
	return 0
}

// results lists the selectable Slices of the current search in display
// order: each Project's groups, then its undecided Slices. Headings name the
// Project too when every Project is searched.
func (m Model) results() []result {
	var results []result
	for _, project := range m.search.Projects {
		prefix := ""
		if m.search.Query.Project == "" {
			prefix = project.Name + " · "
		}
		for _, group := range project.Groups {
			for _, match := range group.Slices {
				results = append(results, result{project.Name, groupHeading(prefix, group), match})
			}
		}
		for _, match := range project.Undecided {
			results = append(results, result{project.Name, titleStyle.Render(prefix + UndecidedTitle), match})
		}
	}
	return results
}

// typeSearch edits the name search until it is applied or abandoned.
func (m Model) typeSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	text := *m.typing
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEnter:
		m.typing = nil
		m.query.Text = strings.TrimSpace(text)
		m.cursor[resultsScreen] = 0
		m.find(resultsScreen)
		return m, nil
	case tea.KeyEsc:
		m.typing = nil
	case tea.KeyBackspace:
		runes := []rune(text)
		text = string(runes[:max(len(runes)-1, 0)])
		m.typing = &text
	case tea.KeySpace:
		text += " "
		m.typing = &text
	case tea.KeyRunes:
		text += string(msg.Runes)
		m.typing = &text
	}
	m.layoutDetail()
	return m, nil
}

// finding reports whether the current screen belongs to finding: the facts,
// the results, or a Slice opened from the results.
func (m Model) finding() bool {
	return m.isFindingScreen(m.screen)
}

func (m Model) isFindingScreen(screen screen) bool {
	return screen == factsScreen || screen == resultsScreen || (screen == diagnosticsScreen && m.diagnosticReturn == resultsScreen) || (screen == sliceScreen && m.parent[sliceScreen] == resultsScreen)
}

// inDocuments reports whether the current screen belongs to a Proposal's or
// Slice's documents, including their diagnostics.
func (m Model) inDocuments() bool {
	return isDocumentOverlay(m.screen) || (m.screen == diagnosticsScreen && m.diagnosticReturn == documentsScreen)
}

func isDocumentOverlay(screen screen) bool {
	switch screen {
	case documentsScreen, referencesScreen, versionsScreen, documentScreen:
		return true
	default:
		return false
	}
}

// find opens target within finding. Finding started from the hierarchy
// searches the current Project, or every Project from the overview, and
// returns to where it started.
func (m *Model) find(target screen) {
	finding := m.finding()
	if m.screen == sliceScreen && finding {
		m.restoreFindingIdentity()
		m.history = nil
	}
	if !finding {
		m.context = m.frame()
		m.contextHistory, m.history = m.history, nil
		m.parent[resultsScreen] = m.screen
		m.query.Project = ""
		if m.screen != overviewScreen {
			m.query.Project = m.project
		}
	}
	if target == factsScreen && m.screen != factsScreen {
		m.parent[factsScreen] = m.parent[resultsScreen]
		if finding {
			m.parent[factsScreen] = resultsScreen
		}
	}
	m.screen, m.status = target, ""
	m.load()
}

// toggleScope switches finding between the current Project and every
// Project.
func (m *Model) toggleScope() {
	switch {
	case m.query.Project != "":
		m.query.Project = ""
	case m.project != "":
		m.query.Project = m.project
	default:
		m.status = "Open a Project to narrow finding to it"
		return
	}
	m.cursor[resultsScreen] = 0
	m.load()
}

// canEnter reports whether the current list has a selected entry to open.
func (m Model) canEnter() bool {
	return m.selectionMissing == "" && m.failure == nil && m.rows() > 0
}

func (m *Model) enter() {
	if !m.canEnter() {
		return
	}
	selected := m.cursor[m.screen]
	switch m.screen {
	case overviewScreen:
		if name := m.overview.Projects[selected].Name; name != m.project {
			m.project, m.proposal, m.cursor[projectScreen], m.archived = name, "", 0, false
		}
		m.screen = projectScreen
	case projectScreen:
		proposal := m.inventory.Proposals[selected]
		if proposal.Name != m.proposal || proposal.Archived != m.archived {
			m.proposal, m.archived, m.cursor[proposalScreen] = proposal.Name, proposal.Archived, 0
		}
		m.screen = proposalScreen
	case proposalScreen:
		m.item = m.members.Slices[selected].Item
		m.screen, m.parent[sliceScreen], m.relation = sliceScreen, proposalScreen, 0
	case factsScreen:
		m.query = factOptions[selected].apply(m.query)
		m.screen, m.cursor[resultsScreen] = resultsScreen, 0
	case resultsScreen:
		chosen := m.results()[selected]
		m.project, m.proposal, m.item, m.archived = chosen.project, chosen.match.Proposal, chosen.match.Item, chosen.match.Archived
		m.screen, m.parent[sliceScreen], m.relation = sliceScreen, resultsScreen, 0
	case documentsScreen:
		if selected < len(m.documents.Documents) {
			m.openDocument(m.documents.Documents[selected])
		} else {
			phase := ledger.ImplementPhase
			if selected > len(m.documents.Documents) {
				phase = ledger.WatchdogPhase
			}
			m.openVersionsFor(m.project, m.item, phase, m.archived)
		}
		return
	case referencesScreen:
		m.followReference(m.references[selected].Reference)
		return
	case versionsScreen:
		m.followVersion(m.versions.Versions[selected].Reference)
		return
	}
	m.status = ""
	m.load()
}

// back restores relationship or finding context before leaving the current
// screen. Document and reference overlays return without reloading that context.
func (m *Model) back() {
	fromDocumentOverlay := isDocumentOverlay(m.screen)
	wasSlice := m.screen == sliceScreen
	selectedResult := ""
	if wasSlice && m.parent[sliceScreen] == resultsScreen {
		selectedResult = m.project + "/" + m.item + locationKey(m.archived)
	}
	if count := len(m.history); m.screen == sliceScreen && count > 0 {
		previous := m.history[count-1]
		m.history = m.history[:count-1]
		m.location, m.status = previous.location, "Returned to "+previous.item
		m.load()
		m.detail.SetYOffset(previous.offset)
		return
	}
	if (m.screen == factsScreen || m.screen == resultsScreen) && m.parent[m.screen] == m.context.screen {
		includeArchived := m.includeArchived
		m.location, m.history = m.context.location, m.contextHistory
		m.includeArchived = includeArchived
		m.contextHistory = nil
		m.parent[sliceScreen] = proposalScreen
		m.status = ""
		m.load()
		m.detail.SetYOffset(m.context.offset)
		return
	}

	m.status = ""
	switch m.screen {
	case overviewScreen:
		return
	case projectScreen:
		m.screen = overviewScreen
	case proposalScreen:
		m.screen = projectScreen
	case sliceScreen:
		if m.parent[sliceScreen] == resultsScreen {
			m.restoreFindingIdentity()
		}
		m.screen = m.parent[sliceScreen]
	case diagnosticsScreen:
		m.screen = m.diagnosticReturn
		m.detail = m.diagnosticOriginDetail
	case factsScreen, resultsScreen:
		m.screen = m.parent[m.screen]
	case documentsScreen:
		m.screen = m.docContext
		m.failure = nil
	case versionsScreen:
		m.screen = m.versionsReturn
	case referencesScreen:
		if len(m.referenceHistory) > 0 {
			previous := m.referenceHistory[len(m.referenceHistory)-1]
			m.referenceHistory = m.referenceHistory[:len(m.referenceHistory)-1]
			m.restoreReferenceFrame(previous)
			m.screen = previous.returnScreen
		} else {
			m.screen = m.referenceOrigin
		}
		m.failure = nil
	case documentScreen:
		if len(m.documentHistory) > 0 {
			frame := m.documentHistory[len(m.documentHistory)-1]
			m.documentHistory = m.documentHistory[:len(m.documentHistory)-1]
			if frame.hasDocument {
				document := frame.document
				m.currentDocument = &document
			} else {
				m.currentDocument = nil
			}
			m.docViewport = frame.viewport
			m.documentReturn, m.renderProblem = frame.returnScreen, frame.renderProblem
			m.versions, m.versionsReturn, m.cursor[versionsScreen] = frame.versions, frame.versionsReturn, frame.versionsCursor
			m.openedContext, m.openedRevision = frame.openedContext, frame.openedRevision
			m.newerDocument = m.documentHasNewerVersion()
			m.references = append([]ledger.LabeledReference(nil), frame.references...)
			m.referenceOrigin, m.referencesFromDoc = frame.referenceOrigin, frame.referencesFromDoc
			m.cursor[referencesScreen] = frame.referenceCursor
			m.referenceHistory = append([]referenceFrame(nil), frame.referenceHistory...)
			m.refreshCurrentReferences()
			if frame.hasDocument {
				if count := len(m.referenceHistory); count > 0 && !frame.fromVersion {
					previous := m.referenceHistory[count-1]
					m.referenceHistory = m.referenceHistory[:count-1]
					m.restoreReferenceFrame(previous)
				}
				m.screen = documentScreen
			} else {
				m.screen = frame.screen
			}
		} else {
			m.screen = m.documentReturn
			m.currentDocument, m.renderProblem = nil, ""
			m.newerDocument = false
		}
		m.failure = nil
	}

	if isDocumentOverlay(m.screen) || (fromDocumentOverlay && m.screen == diagnosticsScreen) {
		m.layoutDetail()
		return
	}
	previous := *m
	previous.cursor = maps.Clone(m.cursor)
	offset := m.detail.YOffset
	m.load()
	if fromDocumentOverlay && m.screen == sliceScreen {
		m.detail.SetYOffset(offset)
	}
	if wasSlice && m.screen == resultsScreen {
		previous.missingIdentity = selectedResult
		previous.missingScreen = resultsScreen
		m.preserveSelection(previous, resultsScreen)
	}
}

func (m Model) frame() frame {
	previous := frame{location: m.location, offset: m.detail.YOffset}
	previous.cursor = maps.Clone(m.cursor)
	return previous
}

// restoreFindingIdentity restores the scope anchor without changing the
// result selection or filters.
func (m *Model) restoreFindingIdentity() {
	m.project, m.proposal, m.item, m.archived = m.context.project, m.context.proposal, m.context.item, m.context.archived
}

// selectRelation moves the relationship selection of the current Slice,
// keeping the selected line in view.
func (m *Model) selectRelation(delta int) {
	count := len(m.relations())
	if count == 0 {
		m.status = "This Slice records no relationship to follow"
		return
	}
	m.relation = (m.relation + delta + count) % count
	m.layoutDetail()
	if m.selectedRow < m.detail.YOffset || m.selectedRow >= m.detail.YOffset+m.detail.Height {
		m.detail.SetYOffset(m.selectedRow)
	}
}

// follow opens the selected relationship's Slice in its own Proposal,
// keeping the archive choice, and remembers the context to return to.
func (m *Model) follow() {
	relations := m.relations()
	if len(relations) == 0 {
		m.status = "This Slice records no relationship to follow"
		return
	}
	m.history = append(m.history, m.frame())
	from := m.item
	m.item, m.archived = relations[m.relation].item, relations[m.relation].archived
	m.proposal, _, _ = strings.Cut(m.item, "/")
	m.relation, m.status = 0, "Followed from "+from+"; esc returns"
	m.load()
}

// relations lists the current Slice's followable relationships.
func (m Model) relations() []relation {
	if m.screen != sliceScreen || m.slice == nil || m.failure != nil {
		return nil
	}
	_, relations := sliceLines(m.slice)
	return relations
}

// switchProject returns to the overview with the current Project selected.
// Followed relationships are left behind.
func (m *Model) switchProject() {
	m.screen, m.status, m.history = overviewScreen, "", nil
	m.contextHistory = nil
	m.docContext, m.documents, m.currentDocument = overviewScreen, nil, nil
	m.documentHistory, m.references, m.referenceHistory, m.versions = nil, nil, nil, nil
	m.selectionMissing, m.missingIdentity = "", ""
	m.referenceOrigin, m.referencesFromDoc = overviewScreen, false
	m.load()
	if m.overview == nil {
		return
	}
	for index, project := range m.overview.Projects {
		if project.Name == m.project {
			m.cursor[overviewScreen] = index
		}
	}
}

// load reads the current screen's facts from the pinned Snapshot.
func (m *Model) load() {
	m.failure = nil
	m.selectionMissing, m.missingIdentity = "", ""
	switch m.screen {
	case overviewScreen:
		m.overview, m.failure = m.snapshot.Overview(m.includeArchived)
	case projectScreen:
		m.inventory, m.failure = m.snapshot.Project(m.project, m.includeArchived)
	case proposalScreen:
		m.members, m.failure = m.snapshot.ProposalAt(m.project, m.proposal, m.archived)
	case sliceScreen:
		m.slice, m.failure = m.snapshot.SliceAt(m.project, m.item, m.archived)
	case factsScreen, resultsScreen, diagnosticsScreen:
		m.query.IncludeArchived = m.includeArchived
		m.search, m.failure = m.snapshot.FindSlices(m.query)
	}
	if count := m.rows(); m.cursor[m.screen] >= count {
		m.cursor[m.screen] = max(count-1, 0)
	}
	m.layoutDetail()
	m.detail.GotoTop()
}

// openAttachment builds the explicit external-browser request for the
// current Slice's issue or pull request, or the current Proposal's parent
// issue. Nothing is opened without this action.
func (m *Model) openAttachment(issue bool) tea.Cmd {
	noun, url, ok := m.attachmentLink(issue)
	switch {
	case noun == "":
		m.status = "Open an issue or pull request from a Slice, or a parent issue from a Proposal"
		return nil
	case !ok:
		m.status = "No recorded " + noun + " attachment to open"
		return nil
	}
	open := m.open
	return func() tea.Msg { return openedMsg{url: url, err: open(url)} }
}

// attachmentLink names the attachment an issue or pull request request opens
// on the current screen, with its URL when one is recorded. The noun is empty
// on a screen without such an attachment.
func (m Model) attachmentLink(issue bool) (noun, url string, ok bool) {
	var attachment *ledger.ForgeAttachment
	switch {
	case m.screen == sliceScreen && m.slice != nil && issue:
		attachment, noun = m.slice.Issue, "issue"
	case m.screen == sliceScreen && m.slice != nil:
		attachment, noun = m.slice.Submission, "pull request"
	case m.screen == proposalScreen && m.members != nil && issue:
		attachment, noun = m.members.Proposal.ParentIssue, "parent issue"
	default:
		return "", "", false
	}
	url, ok = pullRequestURL(attachment)
	if issue {
		url, ok = issueURL(attachment)
	}
	return noun, url, ok
}

// systemOpen asks the operating system to open url in the external browser.
func systemOpen(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
