package browse

import (
	"fmt"
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
)

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
	Up, Down, Enter, Back, Next, Previous, Projects, Archived, Issue, PullRequest, Help, Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Back, k.Projects, k.Help, k.Next, k.Archived, k.Issue, k.PullRequest, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Enter, k.Back}, {k.Next, k.Previous}, {k.Projects, k.Archived}, {k.Issue, k.PullRequest}, {k.Help, k.Quit}}
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
		Issue:       key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "open issue")),
		PullRequest: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "open PR")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// Model is one browsing session over a pinned ledger Snapshot. Selection and
// navigation live only in the running session.
type Model struct {
	snapshot *ledger.Snapshot
	open     func(string) error
	keys     keyMap
	help     help.Model
	detail   viewport.Model

	screen          screen
	includeArchived bool
	project         string
	proposal        string
	item            string
	cursor          map[screen]int
	// relation selects the current Slice's followable relationship, which
	// starts at detail row selectedRow.
	relation, selectedRow int
	// history holds the browsing context each followed relationship left,
	// most recent last; back restores it.
	history []frame

	overview  *ledger.Overview
	inventory *ledger.ProjectInventory
	members   *ledger.ProposalDetail
	slice     *ledger.SliceDetail
	failure   error

	width, height int
	status        string
}

// frame is the browsing context restored by navigating back from a followed
// relationship.
type frame struct {
	screen                  screen
	includeArchived         bool
	project, proposal, item string
	cursor                  map[screen]int
	relation, offset        int
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
		detail: viewport.New(80, 10), cursor: map[screen]int{},
		width: 80, height: 24, status: options.Notice,
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
	case msg.String() == "pgup" && m.screen == sliceScreen:
		m.detail.PageUp()
	case msg.String() == "pgdown" && m.screen == sliceScreen:
		m.detail.PageDown()
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
	case key.Matches(msg, m.keys.Archived):
		m.includeArchived = !m.includeArchived
		m.status = "Archived proposals hidden"
		if m.includeArchived {
			m.status = "Archived proposals shown"
		}
		m.load()
	case key.Matches(msg, m.keys.Issue):
		return m, m.openAttachment(true)
	case key.Matches(msg, m.keys.PullRequest):
		return m, m.openAttachment(false)
	}
	return m, nil
}

func (m *Model) move(delta int) {
	if m.screen == sliceScreen {
		if delta < 0 {
			m.detail.LineUp(1)
		} else {
			m.detail.LineDown(1)
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
	switch {
	case m.screen == overviewScreen && m.overview != nil:
		return len(m.overview.Projects)
	case m.screen == projectScreen && m.inventory != nil:
		return len(m.inventory.Proposals)
	case m.screen == proposalScreen && m.members != nil:
		return len(m.members.Slices)
	}
	return 0
}

func (m *Model) enter() {
	if m.rows() == 0 {
		return
	}
	selected := m.cursor[m.screen]
	switch m.screen {
	case overviewScreen:
		if name := m.overview.Projects[selected].Name; name != m.project {
			m.project, m.proposal, m.cursor[projectScreen] = name, "", 0
		}
		m.screen = projectScreen
	case projectScreen:
		if name := m.inventory.Proposals[selected].Name; name != m.proposal {
			m.proposal, m.cursor[proposalScreen] = name, 0
		}
		m.screen = proposalScreen
	case proposalScreen:
		m.item = m.members.Slices[selected].Item
		m.screen, m.relation = sliceScreen, 0
	}
	m.status = ""
	m.load()
}

// back restores the context a followed relationship left, or otherwise
// returns to the parent screen.
func (m *Model) back() {
	if count := len(m.history); count > 0 {
		previous := m.history[count-1]
		m.history = m.history[:count-1]
		m.screen, m.includeArchived = previous.screen, previous.includeArchived
		m.project, m.proposal, m.item = previous.project, previous.proposal, previous.item
		m.cursor, m.relation, m.status = previous.cursor, previous.relation, "Returned to "+previous.item
		m.load()
		m.detail.SetYOffset(previous.offset)
		return
	}
	if m.screen == overviewScreen {
		return
	}
	m.screen--
	m.status = ""
	m.load()
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
	cursor := make(map[screen]int, len(m.cursor))
	for screen, index := range m.cursor {
		cursor[screen] = index
	}
	m.history = append(m.history, frame{
		screen: m.screen, includeArchived: m.includeArchived,
		project: m.project, proposal: m.proposal, item: m.item,
		cursor: cursor, relation: m.relation, offset: m.detail.YOffset,
	})
	from := m.item
	m.item = relations[m.relation].item
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
		m.members, m.failure = m.snapshot.Proposal(m.project, m.proposal)
	case sliceScreen:
		m.slice, m.failure = m.snapshot.Slice(m.project, m.item)
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
