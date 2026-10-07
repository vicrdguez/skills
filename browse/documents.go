package browse

import (
	"fmt"
	"maps"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/vicrdguez/skills/ledger"
)

// reader is the documents screen of one Proposal or Slice: a navigator of its
// documents, with each report's earlier Report Versions nested under it,
// beside the displayed document. It lives only in the running session.
type reader struct {
	active bool
	list   *ledger.DocumentList
	// versions holds the Report Versions of each loaded report group, keyed
	// by group; a group without a key has not been loaded.
	versions  map[string]versionLoad
	collapsed map[string]bool
	// unreadable marks the entries whose document could not be read when
	// selected.
	unreadable map[string]bool
	entries    []entry
	// documentFocus moves the keys from the navigator to the document.
	documentFocus bool
	shown         shown
	// trail is the path back through followed references, most recent last.
	trail []trailFrame
	// followed adds the displayed report's own versions to the navigator
	// on request, retaining the document and cursor to return to.
	followed *followedVersions
	// details shows the displayed document's metadata in place of its text,
	// which keeps its scroll position underneath.
	details     bool
	detailsView viewport.Model
}

type followedVersions struct {
	document *ledger.Document
	load     versionLoad
	shown    shown
	originID string
}

type versionLoad struct {
	versions *ledger.ReportVersions
	problem  string
}

// entry is one selectable navigator line: a document, a Report Version, or
// a note about one that cannot be shown.
type entry struct {
	// heading names the Slice whose entries this one belongs to in a
	// Proposal's navigator.
	heading string
	// id identifies the entry across refreshes.
	id        string
	columns   []span
	marked    bool
	reference *ledger.Reference
	// note is shown in place of a document when there is none to read.
	note []string
	// group is the report group the entry belongs to; slice and phase
	// identify it.
	group        string
	slice, phase string
	followed     bool
}

// shown is the displayed document, pinned until another is selected or
// followed, with its rendering and scroll position.
type shown struct {
	document *ledger.Document
	// title names the entry when no document is displayed, and lines say
	// why.
	title    string
	lines    []string
	viewport viewport.Model
	// rendered is the width the content was last rendered at; zero renders
	// it again.
	rendered      int
	renderProblem string
	newer         bool
}

// trailFrame is what a followed reference left: the displayed document and
// the reference list it was followed from.
type trailFrame struct {
	shown      shown
	references []ledger.LabeledReference
	origin     screen
	cursor     int
	followed   *followedVersions
	entryID    string
}

func (m *Model) openDocuments() {
	switch m.screen {
	case proposalScreen:
		if m.members == nil {
			return
		}
	case sliceScreen:
		if m.slice == nil {
			return
		}
	default:
		m.status = "Open documents from a Proposal or Slice"
		return
	}
	m.startReader(m.screen)
	m.screen = readerScreen
	if m.failure == nil {
		m.selectEntry()
	}
	m.layoutDetail()
}

// startReader lists the documents of the current Proposal or Slice, opened
// from origin, with the cursor on the first.
func (m *Model) startReader(origin screen) {
	m.docContext = origin
	m.reader = reader{active: true}
	m.cursor[readerScreen] = 0
	m.status = ""
	m.reader, m.failure = m.readDocuments(m.snapshot)
}

// readDocuments lists the reader's documents at snapshot. It loads the
// Report Versions of a Slice's reports, and of every report group of a
// Proposal already expanded.
// Slice opening still loads both histories synchronously; lazy expansion
// would reduce opening latency on large ledgers.
func (m Model) readDocuments(snapshot *ledger.Snapshot) (reader, error) {
	r := m.reader
	var err error
	if m.docContext == sliceScreen {
		r.list, err = snapshot.SliceDocumentListAt(m.project, m.item, m.archived)
	} else {
		r.list, err = snapshot.ProposalDocumentListAt(m.project, m.proposal, m.archived)
	}
	if err != nil {
		r.list, r.entries = nil, nil
		return r, err
	}
	loaded := r.versions
	r.versions = map[string]versionLoad{}
	for _, availability := range r.list.Availability {
		key, ok := groupKey(availability.Slice, availability.Kind)
		if _, expanded := loaded[key]; ok && (expanded || r.list.Slice != "") {
			r.versions[key] = loadVersions(snapshot, r.list, availability.Slice, phaseOf(availability.Kind))
		}
	}
	if r.followed != nil {
		followed := *r.followed
		followed.load = loadFollowedVersions(snapshot, followed.document)
		r.followed = &followed
	}
	r.entries = r.build()
	return r, nil
}

func loadVersions(snapshot *ledger.Snapshot, list *ledger.DocumentList, slice, phase string) versionLoad {
	versions, err := snapshot.VersionsAt(list.Project, list.Proposal+"/"+slice, phase, list.Archived)
	if err != nil {
		return versionLoad{problem: "Cannot discover report versions: " + err.Error()}
	}
	return versionLoad{versions: versions}
}

// groupKey identifies the report group of a phase report kind.
func groupKey(slice string, kind ledger.DocumentKind) (string, bool) {
	if kind != ledger.ImplementReportDocumentKind && kind != ledger.WatchdogReportDocumentKind {
		return "", false
	}
	return slice + "/" + phaseOf(kind), true
}

func phaseOf(kind ledger.DocumentKind) string {
	if kind == ledger.WatchdogReportDocumentKind {
		return ledger.WatchdogPhase
	}
	return ledger.ImplementPhase
}

// build lists the navigator's entries: a Slice's documents, or a Proposal's
// description followed by each Slice's documents under its name.
func (r reader) build() []entry {
	entries := r.baseEntries()
	if r.followed == nil {
		return entries
	}
	followed := r.followed
	heading := "Followed report versions: " + followed.document.Reference.Path
	id := "followed " + followed.document.Reference.Path
	if versions := followed.load.versions; versions != nil {
		for _, version := range versions.Versions {
			reference := version.Reference
			entries = append(entries, entry{
				heading: heading, id: id + " @" + reference.Commit, followed: true,
				columns: append([]span{{text: "  └ " + short(reference.Commit, 7)}}, reportColumns(version.Report)...),
				marked:  len(version.Diagnostics) > 0, reference: &reference,
			})
		}
	}
	var notes []string
	switch {
	case followed.load.problem != "":
		notes = []string{followed.load.problem}
	case followed.load.versions.Incomplete:
		notes = []string{"Local history is incomplete, so earlier versions may be missing:"}
		for _, diagnostic := range followed.load.versions.Diagnostics {
			notes = append(notes, DiagnosticText(diagnostic))
		}
	case len(followed.load.versions.Versions) == 0:
		notes = []string{"No locally available Report Versions were found."}
	}
	if len(notes) > 0 {
		entries = append(entries, entry{heading: heading, id: id + " history", followed: true,
			columns: texts("Report Version availability"), marked: true, note: notes})
	}
	return entries
}

func loadFollowedVersions(snapshot *ledger.Snapshot, document *ledger.Document) versionLoad {
	versions, err := snapshot.VersionsAt(document.Project, document.Proposal+"/"+document.Slice, phaseOf(document.Kind), document.Archived)
	if err != nil {
		return versionLoad{problem: "Cannot discover report versions: " + err.Error()}
	}
	return versionLoad{versions: versions}
}

func (r reader) baseEntries() []entry {
	if r.list == nil {
		return nil
	}
	if r.list.Slice != "" {
		return r.sliceEntries(r.list.Slice, "")
	}
	var entries []entry
	for _, document := range r.list.Documents {
		if document.Slice == "" {
			entries = append(entries, documentEntry(document, ""))
		}
	}
	for _, document := range r.list.Unreadable {
		if document.Slice == "" {
			entries = append(entries, unreadableEntry(document, ""))
		}
	}
	seen := map[string]bool{}
	for _, availability := range r.list.Availability {
		if !seen[availability.Slice] {
			seen[availability.Slice] = true
			entries = append(entries, r.sliceEntries(availability.Slice, "Slice "+availability.Slice)...)
		}
	}
	return entries
}

// sliceEntries lists one Slice's contract documents, its two report groups,
// and its Human Decision when one is active or its membership is unknown.
func (r reader) sliceEntries(slice, heading string) []entry {
	var entries []entry
	for _, document := range r.list.Documents {
		if document.Slice == slice && document.Kind == ledger.ContractDocumentKind {
			entries = append(entries, documentEntry(document, heading))
		}
	}
	for _, document := range r.list.Unreadable {
		if document.Slice == slice && document.Kind == ledger.ContractDocumentKind {
			entries = append(entries, unreadableEntry(document, heading))
		}
	}
	entries = append(entries, r.reportEntries(slice, heading, ledger.ImplementReportDocumentKind)...)
	entries = append(entries, r.reportEntries(slice, heading, ledger.WatchdogReportDocumentKind)...)
	decision := entry{heading: heading, id: documentID(slice, "decision.md"), columns: texts("Human Decision")}
	switch r.availability(slice, ledger.DecisionDocumentKind) {
	case ledger.DocumentAvailable:
		for _, document := range r.list.Documents {
			if document.Slice == slice && document.Kind == ledger.DecisionDocumentKind {
				entries = append(entries, documentEntry(document, heading))
			}
		}
	case ledger.DocumentUnavailable:
		decision.marked = true
		decision.columns = append(decision.columns, span{"recorded but unavailable", warningStyle})
		decision.note = []string{"The active Human Decision is recorded but cannot be read at this revision. Press d for the diagnostics."}
		entries = append(entries, decision)
	case ledger.DocumentUnknown:
		decision.marked = true
		decision.columns = append(decision.columns, span{"membership unknown", warningStyle})
		decision.note = []string{"Whether a Human Decision is active cannot be determined at this revision. Press d for the diagnostics."}
		entries = append(entries, decision)
	}
	return entries
}

func (r reader) availability(slice string, kind ledger.DocumentKind) ledger.DocumentStatus {
	for _, availability := range r.list.Availability {
		if availability.Slice == slice && availability.Kind == kind {
			return availability.Status
		}
	}
	return ""
}

// reportEntries lists one report group: the latest report, or why there is
// none, then its earlier Report Versions, newest first.
func (r reader) reportEntries(slice, heading string, kind ledger.DocumentKind) []entry {
	key, _ := groupKey(slice, kind)
	phase := phaseOf(kind)
	name := "Implementation report"
	if phase == ledger.WatchdogPhase {
		name = "Watchdog report"
	}
	header := entry{heading: heading, id: documentID(slice, phase+"-report.md"), group: key, slice: slice, phase: phase, columns: texts(name)}
	var latest *ledger.DocumentEntry
	for index, document := range r.list.Documents {
		if document.Slice == slice && document.Kind == kind {
			latest = &r.list.Documents[index]
		}
	}
	load, loaded := r.versions[key]
	var versions []ledger.ReportVersion
	if load.versions != nil {
		versions = load.versions.Versions
	}
	switch {
	case latest != nil:
		header.reference = &latest.Reference
		header.columns = append(header.columns, reportColumns(latest.Report)...)
		header.marked = len(latest.Diagnostics) > 0
	case r.availability(slice, kind) == ledger.DocumentUnavailable:
		header.marked = true
		header.columns = append(header.columns, span{"recorded but unavailable", warningStyle})
		header.note = []string{"The latest " + strings.ToLower(name) + " is recorded but cannot be read at this revision. Press d for the diagnostics."}
	case loaded && load.problem == "" && len(versions) == 0 && !load.versions.Incomplete:
		header.columns = append(header.columns, span{"not yet available", mutedStyle})
		header.note = []string{"No " + strings.ToLower(name) + " has been recorded yet. It is optional until its phase runs."}
	case !loaded:
		header.columns = append(header.columns, span{"no latest report", mutedStyle})
		header.note = []string{"There is no latest " + strings.ToLower(name) + " at this revision. Press v to look for earlier versions."}
	default:
		header.columns = append(header.columns, span{"no latest report", mutedStyle})
		header.note = []string{"There is no latest " + strings.ToLower(name) + " at this revision. Its earlier versions are listed under it."}
	}
	earlier := versions
	if latest != nil && len(earlier) > 0 {
		// Report Versions are listed newest first, so the first is the
		// latest report's own content.
		earlier = earlier[1:]
	}
	switch {
	case !loaded:
		header.columns = append(header.columns, span{"[v]", mutedStyle})
	case r.collapsed[key] && len(earlier) > 0:
		header.columns = append(header.columns, span{fmt.Sprintf("%d earlier [v]", len(earlier)), mutedStyle})
	}
	entries := []entry{header}
	if loaded && !r.collapsed[key] {
		for _, version := range earlier {
			commit := version.Reference.Commit
			reference := version.Reference
			entries = append(entries, entry{
				heading: heading, id: header.id + " @" + short(commit, 12), group: key, slice: slice, phase: phase,
				columns:   append([]span{{text: "  └ " + short(commit, 7)}}, reportColumns(version.Report)...),
				marked:    len(version.Diagnostics) > 0,
				reference: &reference,
			})
		}
	}
	switch {
	case loaded && load.problem != "":
		entries = append(entries, entry{heading: heading, id: header.id + " history", group: key, slice: slice, phase: phase, marked: true,
			columns: []span{{text: "  └ "}, {"versions unavailable", warningStyle}}, note: []string{load.problem}})
	case loaded && load.versions.Incomplete:
		note := []string{"Local history is incomplete, so earlier versions may be missing:"}
		for _, diagnostic := range load.versions.Diagnostics {
			note = append(note, DiagnosticText(diagnostic))
		}
		entries = append(entries, entry{heading: heading, id: header.id + " history", group: key, slice: slice, phase: phase, marked: true,
			columns: []span{{text: "  └ "}, {"history incomplete", warningStyle}}, note: note})
	}
	return entries
}

// reportColumns label a report by its outcome and round, or mark metadata
// that cannot be interpreted without inventing either.
func reportColumns(report *ledger.Report) []span {
	if report == nil {
		return []span{{"bad metadata", warningStyle}}
	}
	columns := []span{outcomeSpan(report.Outcome)}
	if report.Round != 0 {
		columns = append(columns, span{text: fmt.Sprintf("· round %d", report.Round)})
	}
	return columns
}

func documentID(slice, name string) string {
	if slice == "" {
		return name
	}
	return slice + " " + name
}

func documentEntry(document ledger.DocumentEntry, heading string) entry {
	name := documentName(document.Reference.Path)
	reference := document.Reference
	result := entry{heading: heading, id: documentID(document.Slice, name), reference: &reference, marked: len(document.Diagnostics) > 0}
	switch document.Kind {
	case ledger.ProposalDocumentKind:
		result.columns = texts("Proposal description")
	case ledger.DecisionDocumentKind:
		result.columns = texts("Human Decision")
		if document.Decision != nil {
			result.columns = append(result.columns, span{text: "route " + document.Decision.Route})
		}
	default:
		result.columns = texts(name)
	}
	if len(document.Diagnostics) > 0 {
		result.columns = append(result.columns, span{"bad metadata", warningStyle})
	}
	return result
}

func unreadableEntry(document ledger.DocumentEntry, heading string) entry {
	name := documentName(document.Reference.Path)
	return entry{
		heading: heading, id: documentID(document.Slice, name), marked: true,
		columns: []span{{text: name}, {"unreadable", warningStyle}},
		note:    []string{name + " is a member of this set but cannot be read at this revision. Press d for the diagnostics."},
	}
}

// selectEntry displays the entry at the cursor, reading its document, and
// leaves any followed references behind.
func (m *Model) selectEntry() {
	r := &m.reader
	cursor := m.cursor[readerScreen]
	if cursor >= len(r.entries) || !r.entries[cursor].followed {
		r.trail, r.followed = nil, nil
		r.entries = r.build()
	}
	var s shown
	if cursor := m.cursor[readerScreen]; cursor < len(r.entries) {
		selected := r.entries[cursor]
		s.title, s.lines = selected.heading, selected.note
		if s.title != "" {
			s.title += " · "
		}
		s.title += selected.columns[0].text
		if selected.reference != nil {
			document, err := m.snapshot.Document(*selected.reference)
			if err != nil {
				s.lines = []string{warningStyle.Render("! " + unavailableReference(*selected.reference, err))}
				r.unreadable = with(r.unreadable, selected.id, true)
			}
			s.document = document
		}
	} else {
		s.lines = []string{"No document is selected."}
	}
	r.shown = s
	r.shown.newer = m.documentHasNewerVersion(s.document)
	m.status = ""
	m.layoutDetail()
}

// with is a copy of marks with key set to value, so a model copied before
// the change keeps its own marks.
func with(marks map[string]bool, key string, value bool) map[string]bool {
	marks = maps.Clone(marks)
	if marks == nil {
		marks = map[string]bool{}
	}
	marks[key] = value
	return marks
}

// moveEntry moves the navigator's cursor and displays the entry it reaches.
func (m *Model) moveEntry(delta int) {
	count := len(m.reader.entries)
	if count == 0 {
		return
	}
	cursor := min(max(m.cursor[readerScreen]+delta, 0), count-1)
	if cursor == m.cursor[readerScreen] && m.selectionMissing == "" {
		return
	}
	m.cursor[readerScreen] = cursor
	m.selectionMissing, m.missingIdentity = "", ""
	m.selectEntry()
}

// toggleVersions loads and shows the earlier versions of the report group at
// the cursor, or hides and shows them again once loaded.
func (m *Model) toggleVersions() {
	r := &m.reader
	if r.followed != nil {
		m.closeFollowedVersions()
		return
	}
	if r.documentFocus && len(r.trail) > 0 {
		document := r.shown.document
		if document == nil || (document.Kind != ledger.ImplementReportDocumentKind && document.Kind != ledger.WatchdogReportDocumentKind) {
			m.status = "The displayed document is not a phase report"
			return
		}
		r.followed = &followedVersions{document: document, load: loadFollowedVersions(m.snapshot, document),
			shown: r.shown, originID: m.selectedIdentity(readerScreen)}
		m.cursor[readerScreen] = len(r.entries)
		r.entries = r.build()
		r.documentFocus = false
		m.selectEntry()
		return
	}
	cursor := m.cursor[readerScreen]
	if cursor >= len(r.entries) || r.entries[cursor].group == "" {
		m.status = "Select a report to show or hide its earlier versions"
		return
	}
	selected := r.entries[cursor]
	if _, loaded := r.versions[selected.group]; !loaded {
		r.versions = maps.Clone(r.versions)
		r.versions[selected.group] = loadVersions(m.snapshot, r.list, selected.slice, selected.phase)
	} else {
		r.collapsed = with(r.collapsed, selected.group, !r.collapsed[selected.group])
	}
	r.entries = r.build()
	m.status = ""
	for index, candidate := range r.entries {
		if candidate.id == selected.id {
			m.cursor[readerScreen] = index
			if candidate.reference == nil {
				// A note can change with what was found.
				m.selectEntry()
			}
			return
		}
	}
	// A hidden version leaves the cursor on its report.
	for index, candidate := range r.entries {
		if candidate.group == selected.group {
			m.cursor[readerScreen] = index
			break
		}
	}
	m.selectEntry()
}

// closeFollowedVersions returns to the exact document and navigator cursor
// from before its versions were opened, without consuming a reference frame.
func (m *Model) closeFollowedVersions() {
	r := &m.reader
	followed := r.followed
	r.followed = nil
	r.entries = r.build()
	r.shown = followed.shown
	r.shown.rendered = 0
	r.shown.newer = m.documentHasNewerVersion(r.shown.document)
	r.documentFocus = true
	m.restoreEntry(followed.originID)
	m.status = ""
}

func (m *Model) restoreEntry(id string) {
	for index, candidate := range m.reader.entries {
		if candidate.id == id {
			m.cursor[readerScreen] = index
			m.selectionMissing, m.missingIdentity = "", ""
			return
		}
	}
	m.cursor[readerScreen] = 0
	m.missingIdentity, m.missingScreen = id, readerScreen
	m.selectionMissing = "Selected " + id + " is no longer available in these results; move to select another"
}

func (m *Model) openReferences() {
	switch m.screen {
	case readerScreen:
		document := m.reader.shown.document
		if document == nil {
			m.status = "The displayed entry has no document whose references to list"
			return
		}
		m.references = append([]ledger.LabeledReference(nil), document.References...)
	case sliceScreen:
		if m.slice == nil || m.slice.Claim == nil {
			m.status = "No recorded Claim reference for this Slice"
			return
		}
		m.references = []ledger.LabeledReference{{Label: "current Claim state", Reference: m.slice.Claim.Reference}}
		m.docContext = sliceScreen
	default:
		m.status = "Open references from a displayed document or a Slice Claim"
		return
	}
	m.referenceOrigin = m.screen
	m.screen = referencesScreen
	m.cursor[referencesScreen] = 0
	m.failure = nil
	m.status = ""
	m.layoutDetail()
}

// followReference displays the exact referenced document in the reader,
// remembering the reference list and the document it was followed from. A
// Slice's Claim reference opens that Slice's reader.
func (m *Model) followReference(reference ledger.Reference) {
	document, err := m.snapshot.Document(reference)
	if err != nil {
		m.failure = nil
		m.status = unavailableReference(reference, err)
		return
	}
	if m.referenceOrigin == sliceScreen {
		m.startReader(sliceScreen)
	}
	r := &m.reader
	r.trail = append(r.trail, trailFrame{
		shown:      r.shown,
		references: append([]ledger.LabeledReference(nil), m.references...),
		origin:     m.referenceOrigin, cursor: m.cursor[referencesScreen],
		followed: r.followed, entryID: m.selectedIdentity(readerScreen),
	})
	if r.followed != nil {
		id := r.followed.originID
		r.followed = nil
		r.entries = r.build()
		m.restoreEntry(id)
	}
	r.shown = shown{document: document}
	r.shown.newer = m.documentHasNewerVersion(document)
	r.documentFocus = true
	m.screen = readerScreen
	m.failure = nil
	m.status = ""
	m.layoutDetail()
}

// returnAlongTrail goes back from a followed document to the reference list
// it was followed from, with the document that list belongs to displayed
// again at its scroll position.
func (m *Model) returnAlongTrail() {
	r := &m.reader
	frame := r.trail[len(r.trail)-1]
	r.trail = r.trail[:len(r.trail)-1]
	r.followed = frame.followed
	if r.followed != nil {
		followed := *r.followed
		followed.load = loadFollowedVersions(m.snapshot, followed.document)
		r.followed = &followed
	}
	r.entries = r.build()
	m.restoreEntry(frame.entryID)
	r.shown = frame.shown
	r.shown.rendered = 0
	r.shown.newer = m.documentHasNewerVersion(r.shown.document)
	m.references = frame.references
	m.referenceOrigin = frame.origin
	m.cursor[referencesScreen] = frame.cursor
	m.refreshCurrentReferences()
	m.screen = referencesScreen
	m.status = ""
	m.layoutDetail()
}

func unavailableReference(reference ledger.Reference, err error) string {
	return fmt.Sprintf("Exact ledger reference unavailable (%s:%s): %s; no substitute was opened", reference.Commit, reference.Path, err)
}

// shownTitle names the displayed document, marking whether it is the
// current one or an exact historical revision.
func (m Model) shownTitle() string {
	document := m.reader.shown.document
	if document == nil {
		return m.reader.shown.title
	}
	return documentLabel(document, true) + " · " + m.identityForms(document)[0]
}

// documentLabel names a document; full names it with its Slice, and
// otherwise by its file name alone.
func documentLabel(document *ledger.Document, full bool) string {
	switch {
	case document.Kind == ledger.ProposalDocumentKind:
		return "Proposal description"
	case full && document.Slice != "":
		return document.Slice + "/" + documentName(document.Reference.Path)
	}
	return documentName(document.Reference.Path)
}

// identityForms mark a document current, or historical at its ledger
// commit, in forms from full to shortest.
func (m Model) identityForms(document *ledger.Document) []string {
	if commit := document.Reference.Commit; commit != m.snapshot.Revision {
		return []string{"HISTORICAL " + short(commit, 12), "HIST " + short(commit, 7)}
	}
	return []string{"current"}
}

// documentTitle is the document pane's title within width: the displayed
// document's name and identity, then what a reader needs at a glance from
// its metadata.
func (m Model) documentTitle(width int) string {
	document := m.reader.shown.document
	if document == nil {
		return m.reader.shown.title
	}
	plain := lipgloss.NewStyle()
	names := []string{documentLabel(document, true)}
	// Only a document of the reader's own Slice is named without it.
	if m.docContext == sliceScreen && document.Project == m.project && document.Proposal+"/"+document.Slice == m.item {
		names = append(names, documentLabel(document, false))
	}
	parts := []indicator{{plain, names}, {plain, m.identityForms(document)}}
	if report := document.Report; report != nil {
		outcome := outcomeSpan(report.Outcome)
		parts = append(parts, indicator{outcome.style, []string{outcome.text}})
		if report.Round != 0 {
			parts = append(parts, indicator{plain, []string{fmt.Sprintf("round %d", report.Round)}})
		}
	}
	if decision := document.Decision; decision != nil {
		parts = append(parts, indicator{plain, []string{"route " + decision.Route}})
	}
	if claim := document.Claim; claim != nil {
		parts = append(parts, claimIndicator(claim.Phase))
	}
	if m.reader.shown.newer {
		parts = append(parts, indicator{plain, []string{"newer available", "newer"}})
	}
	switch {
	case metadataUnreadable(document):
		parts = append(parts, indicator{warningStyle, []string{"! metadata unreadable", "! unreadable"}})
	case len(document.Diagnostics) > 0:
		parts = append(parts, indicator{warningStyle, []string{"! " + plural(len(document.Diagnostics), "diagnostic"), "!"}})
	}
	return fitTitle(parts, width)
}

// navigatorTitle names the reader's Proposal or Slice with its current
// facts, which follow each refresh while the displayed document stays
// pinned.
func (m Model) navigatorTitle(width int) string {
	plain := lipgloss.NewStyle()
	if m.docContext != sliceScreen {
		parts := []indicator{{plain, []string{"Proposal " + m.proposal, m.proposal}}}
		if m.members != nil && m.members.Proposal.Archived {
			parts = append(parts, indicator{mutedStyle, []string{"archived"}})
		}
		return fitTitle(parts, width)
	}
	name := strings.TrimPrefix(m.item, m.proposal+"/")
	parts := []indicator{{plain, []string{"Slice " + name, name}}}
	if m.slice != nil {
		lifecycle, claim := unknownLifecycleSpan(), indicator{warningStyle, []string{unknownClaimSpan().text}}
		if m.slice.Readable {
			lifecycle, claim = lifecycleSpan(m.slice.Lifecycle), indicator{plain, []string{claimLabels[ledger.ClaimNone]}}
			if m.slice.Claim != nil {
				claim = claimIndicator(m.slice.Claim.Phase)
			}
		}
		parts = append(parts, indicator{lifecycle.style, []string{lifecycle.text}}, claim)
	}
	return fitTitle(parts, width)
}

// fitTitle renders a pane title's parts on as few lines of width as hold
// them, in the longest forms that fit. A line holds as many leading parts as
// fit in their shortest forms, and the rest continue on the next line. The
// first line always holds the name and the part after it, a document's
// identity. Unlike the header's indicators, no part is dropped, so a narrow
// title keeps every fact.
func fitTitle(parts []indicator, width int) string {
	return strings.Join(titleLines(parts, width, min(len(parts), 2)), "\n")
}

// titleLines fits parts on lines of width, the first line holding at least
// least of them. When even their shortest forms do not fit, the first of them
// gives way from its start.
func titleLines(parts []indicator, width, least int) []string {
	if len(parts) == 0 {
		return nil
	}
	count := len(parts)
	line, fits := fitLine(parts, width)
	for !fits && count > least {
		count--
		line, fits = fitLine(parts[:count], width)
	}
	if !fits {
		rest := ""
		for _, part := range parts[1:count] {
			rest += " · " + part.style.Render(part.forms[len(part.forms)-1])
		}
		first := parts[0]
		line = first.style.Render(keepEnd(first.forms[len(first.forms)-1], max(width-lipgloss.Width(rest), 1))) + rest
	}
	return append([]string{line}, titleLines(parts[count:], width, 1)...)
}

// fitLine renders parts on one line in the longest forms that fit width, and
// reports whether they fit. The first part takes its shorter form first, then
// the other parts from the last.
func fitLine(parts []indicator, width int) (string, bool) {
	levels := make([]int, len(parts))
	render := func() string {
		texts := make([]string, 0, len(parts))
		for index, part := range parts {
			texts = append(texts, part.style.Render(part.forms[levels[index]]))
		}
		return strings.Join(texts, " · ")
	}
	order := []int{0}
	for index := len(parts) - 1; index > 0; index-- {
		order = append(order, index)
	}
	for _, index := range order {
		if line := render(); lipgloss.Width(line) <= width {
			return line, true
		}
		levels[index] = len(parts[index].forms) - 1
	}
	line := render()
	return line, lipgloss.Width(line) <= width
}

// metadataUnreadable reports whether a document's recorded metadata could
// not be interpreted, so that only its recorded text and diagnostics remain.
// It follows the ledger's parseMetadata, which leaves a report's or
// decision's metadata unset when it fails, and gives a state record without a
// Claim diagnostics only when its JSON cannot be read. A document whose bytes
// could not be confirmed fails the same way and carries that diagnostic.
func metadataUnreadable(document *ledger.Document) bool {
	switch document.Kind {
	case ledger.ImplementReportDocumentKind, ledger.WatchdogReportDocumentKind:
		return document.Report == nil
	case ledger.DecisionDocumentKind:
		return document.Decision == nil
	case ledger.StateDocumentKind:
		return document.Claim == nil && len(document.Diagnostics) > 0
	}
	return false
}

// shownContent renders the displayed document, or why there is none, to
// width.
func (m Model) shownContent(width int) (string, string) {
	if m.reader.shown.document == nil {
		return wrap(strings.Join(m.reader.shown.lines, "\n"), width), ""
	}
	return m.documentContent(m.reader.shown.document, width)
}

// documentContent is the authored document alone, from its first line: a
// report's or decision's body without its frontmatter, a state record as its
// recorded JSON, and the recorded text in full when its metadata cannot be
// interpreted.
func (m Model) documentContent(document *ledger.Document, width int) (string, string) {
	if metadataUnreadable(document) {
		return document.Contents, ""
	}
	body := document.Body
	if document.Kind == ledger.StateDocumentKind {
		body = "```json\n" + document.Contents + "\n```"
	}
	style := styles.LightStyle
	if m.darkBackground {
		style = styles.DarkStyle
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(max(width, 1)))
	if err != nil {
		return body, "Markdown renderer unavailable: " + err.Error()
	}
	rendered, err := renderer.Render(body)
	if err != nil {
		return body, "Markdown rendering failed: " + err.Error()
	}
	// The renderer opens with blank lines of its own.
	lines := strings.Split(rendered, "\n")
	for len(lines) > 1 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n"), ""
}

// detailsOpen reports whether the details panel shows, which it does only
// while a document is displayed.
func (r reader) detailsOpen() bool {
	return r.details && r.shown.document != nil
}

// toggleDetails opens the details panel at its top, or closes it.
func (m *Model) toggleDetails() {
	r := &m.reader
	if r.shown.document == nil {
		m.status = "The displayed entry has no document whose details to show"
		return
	}
	r.details = !r.detailsOpen()
	r.detailsView.SetYOffset(0)
	m.status = ""
}

// detailsLines are the displayed document's recorded metadata: its full
// identity, each structured field, including those its title shows, and its
// diagnostics.
func detailsLines(document *ledger.Document) []string {
	heading := titleStyle.Render
	lines := []string{
		heading("Ledger document"),
		"  Commit: " + document.Reference.Commit,
		"  Path: " + document.Reference.Path,
		"  Kind: " + string(document.Kind),
	}
	references := func(title string) {
		if len(document.References) == 0 {
			return
		}
		lines = append(lines, "", heading(title))
		for _, reference := range document.References {
			lines = append(lines, "  "+reference.Label, "    Commit: "+reference.Reference.Commit, "    Path: "+reference.Reference.Path)
		}
	}
	if report := document.Report; report != nil {
		lines = append(lines, "", fmt.Sprintf("Schema: %d", report.Schema), "Outcome: "+report.Outcome)
		if report.Round != 0 {
			lines = append(lines, fmt.Sprintf("Review round: %d", report.Round))
		}
		source := report.Source
		if source.Head != "" || source.Target != "" || source.Reviewed != "" {
			lines = append(lines, "", heading("Source repository revisions (not ledger revisions)"))
			for _, revision := range []struct{ label, value string }{
				{"Candidate head", source.Head}, {"Integration Target", source.Target}, {"Reviewed revision", source.Reviewed},
			} {
				if revision.value != "" {
					lines = append(lines, "  "+revision.label+": "+revision.value)
				}
			}
		}
		references("Consumed ledger references")
	}
	if decision := document.Decision; decision != nil {
		lines = append(lines, "",
			fmt.Sprintf("Schema: %d", decision.Schema),
			"Route: "+decision.Route,
			"Project: "+decision.Project,
			"Slice: "+decision.Item,
			"", heading("Answered request"),
			"  Commit: "+decision.AnsweredRequest.Commit,
			"  Path: "+decision.AnsweredRequest.Path,
		)
	}
	switch claim := document.Claim; {
	case claim != nil:
		lines = append(lines, "", "Claim phase: "+claim.Phase, "Claim basis (ledger revision): "+claim.Basis)
		references("Claim input references")
	case document.Kind == ledger.StateDocumentKind && !metadataUnreadable(document):
		lines = append(lines, "", "No Claim is recorded.")
	}
	if len(document.Diagnostics) > 0 {
		lines = append(lines, "", heading("Diagnostics"))
		for _, diagnostic := range document.Diagnostics {
			lines = append(lines, warningStyle.Render("! "+DiagnosticText(diagnostic)))
		}
	}
	return lines
}
