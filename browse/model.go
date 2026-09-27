package browse

import (
	"fmt"
	"maps"
	"os/exec"
	"runtime"
	"strings"

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
}

type keyMap struct {
	Up, Down, Enter, Back, Next, Previous, Projects, Archived, Search, Facts, Group, Scope, Diagnostics, Documents, References, Issue, PullRequest, Help, Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Back, k.Documents, k.References, k.Projects, k.Help, k.Next, k.Archived, k.Issue, k.PullRequest, k.Quit, k.Search, k.Facts}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Enter, k.Back}, {k.Next, k.Previous}, {k.Projects, k.Archived}, {k.Search, k.Facts, k.Group, k.Scope, k.Diagnostics}, {k.Documents, k.References}, {k.Issue, k.PullRequest}, {k.Help, k.Quit}}
}

func newKeyMap() keyMap {
	return keyMap{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Enter:       key.NewBinding(key.WithKeys("enter", "right", "l"), key.WithHelp("enter", "open")),
		Back:        key.NewBinding(key.WithKeys("esc", "backspace", "left", "h"), key.WithHelp("esc", "back")),
		Next:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next relation")),
		Previous:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous relation")),
		Projects:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "switch project")),
		Archived:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle archived")),
		Documents:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "documents")),
		References:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "references")),
		Search:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search names")),
		Facts:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "find by lifecycle or claim")),
		Group:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "group by proposal/lifecycle")),
		Scope:       key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "project/every project")),
		Diagnostics: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "result diagnostics")),
		Issue:       key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "open issue")),
		PullRequest: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "open PR")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// Model is one browsing session over a pinned ledger Snapshot. Selection and
// navigation live only in the running session.
type Model struct {
	snapshot    *ledger.Snapshot
	open        func(string) error
	keys        keyMap
	help        help.Model
	detail      viewport.Model
	docViewport viewport.Model

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

	docContext        screen
	documents         *ledger.DocumentSet
	currentDocument   *ledger.Document
	documentReturn    screen
	documentHistory   []documentFrame
	references        []ledger.LabeledReference
	referenceOrigin   screen
	referencesFromDoc bool
	renderProblem     string

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

// openedMsg reports the outcome of one explicit external-browser request.
type openedMsg struct {
	url string
	err error
}

// New starts a session at the selected Project, or at the overview.
func New(snapshot *ledger.Snapshot, options Options) Model {
	model := Model{
		snapshot: snapshot, open: options.Open, keys: newKeyMap(), help: help.New(),
		detail: viewport.New(80, 10), docViewport: viewport.New(80, 10),
		location: location{cursor: map[screen]int{}},
		parent:   map[screen]screen{sliceScreen: proposalScreen},
		width:    80, height: 24, status: options.Notice,
	}
	model.help.Width = model.width
	if model.open == nil {
		model.open = systemOpen
	}
	if options.Project != "" {
		model.screen, model.project = projectScreen, options.Project
	}
	model.load()
	return model
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.layoutDetail()
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
	finding := m.finding()
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
	case msg.String() == "pgup" && (m.screen == sliceScreen || m.screen == diagnosticsScreen):
		m.detail.PageUp()
	case msg.String() == "pgdown" && (m.screen == sliceScreen || m.screen == diagnosticsScreen):
		m.detail.PageDown()
	case msg.String() == "pgup" && m.screen == documentScreen:
		m.docViewport.PageUp()
	case msg.String() == "pgdown" && m.screen == documentScreen:
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
	case key.Matches(msg, m.keys.Archived) && !isDocumentOverlay(m.screen):
		m.includeArchived = !m.includeArchived
		m.status = "Archived proposals hidden"
		if m.includeArchived {
			m.status = "Archived proposals shown"
		}
		m.load()
	case key.Matches(msg, m.keys.Search) && !isDocumentOverlay(m.screen):
		text := ""
		m.typing = &text
		m.layoutDetail()
	case key.Matches(msg, m.keys.Facts) && !isDocumentOverlay(m.screen):
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
	case key.Matches(msg, m.keys.Diagnostics) && m.screen == resultsScreen:
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
			return len(m.documents.Documents)
		}
	case referencesScreen:
		return len(m.references)
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
				results = append(results, result{project.Name, prefix + GroupTitle(group), match})
			}
		}
		for _, match := range project.Undecided {
			results = append(results, result{project.Name, prefix + UndecidedTitle, match})
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
	return screen == factsScreen || screen == resultsScreen || screen == diagnosticsScreen || (screen == sliceScreen && m.parent[sliceScreen] == resultsScreen)
}

func isDocumentOverlay(screen screen) bool {
	switch screen {
	case documentsScreen, referencesScreen, documentScreen:
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

func (m *Model) enter() {
	if m.rows() == 0 {
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
		m.openDocument(m.documents.Documents[selected])
		return
	case referencesScreen:
		m.followReference(m.references[selected].Reference)
		return
	}
	m.status = ""
	m.load()
}

// back restores relationship or finding context before leaving the current
// screen. Document and reference overlays return without reloading that context.
func (m *Model) back() {
	fromDocumentOverlay := isDocumentOverlay(m.screen)
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
		m.screen = resultsScreen
	case factsScreen, resultsScreen:
		m.screen = m.parent[m.screen]
	case documentsScreen:
		m.screen = m.docContext
		m.failure = nil
	case referencesScreen:
		m.screen = m.referenceOrigin
		m.failure = nil
	case documentScreen:
		if len(m.documentHistory) > 0 {
			frame := m.documentHistory[len(m.documentHistory)-1]
			m.documentHistory = m.documentHistory[:len(m.documentHistory)-1]
			m.currentDocument, m.docViewport = &frame.document, frame.viewport
			m.documentReturn, m.renderProblem = frame.returnScreen, frame.renderProblem
			m.screen = documentScreen
		} else {
			m.screen = m.documentReturn
			m.currentDocument, m.renderProblem = nil, ""
		}
		m.failure = nil
	}

	if fromDocumentOverlay || isDocumentOverlay(m.screen) {
		m.layoutDetail()
		return
	}
	m.load()
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
	m.documentHistory, m.references = nil, nil
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
	var attachment *ledger.ForgeAttachment
	noun := "pull request"
	switch {
	case m.screen == sliceScreen && m.slice != nil && issue:
		attachment, noun = m.slice.Issue, "issue"
	case m.screen == sliceScreen && m.slice != nil:
		attachment = m.slice.Submission
	case m.screen == proposalScreen && m.members != nil && issue:
		attachment, noun = m.members.Proposal.ParentIssue, "parent issue"
	default:
		m.status = "Open an issue or pull request from a Slice, or a parent issue from a Proposal"
		return nil
	}
	url, ok := pullRequestURL(attachment)
	if issue {
		url, ok = issueURL(attachment)
	}
	if !ok {
		m.status = "No recorded " + noun + " attachment to open"
		return nil
	}
	open := m.open
	return func() tea.Msg { return openedMsg{url: url, err: open(url)} }
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
