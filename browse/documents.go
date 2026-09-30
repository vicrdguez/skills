package browse

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/vicrdguez/skills/ledger"
)

type documentFrame struct {
	document          ledger.Document
	hasDocument       bool
	viewport          viewport.Model
	screen            screen
	returnScreen      screen
	fromVersion       bool
	versions          *ledger.ReportVersions
	versionsReturn    screen
	versionsCursor    int
	renderProblem     string
	openedContext     string
	openedRevision    string
	references        []ledger.LabeledReference
	referenceOrigin   screen
	referencesFromDoc bool
	referenceCursor   int
	referenceHistory  []referenceFrame
}

func (m *Model) openDocuments() {
	switch m.screen {
	case proposalScreen:
		if m.members == nil {
			return
		}
		m.docContext = proposalScreen
		m.documents, m.failure = m.snapshot.ProposalDocumentsAt(m.project, m.proposal, m.archived)
	case sliceScreen:
		if m.slice == nil {
			return
		}
		m.docContext = sliceScreen
		m.documents, m.failure = m.snapshot.SliceDocumentsAt(m.project, m.item, m.archived)
	default:
		m.status = "Open documents from a Proposal or Slice"
		return
	}
	m.screen = documentsScreen
	m.documentHistory = nil
	m.currentDocument = nil
	m.references = nil
	m.referenceHistory = nil
	m.cursor[documentsScreen] = 0
	m.status = ""
	m.layoutDetail()
}

func (m *Model) openReferences() {
	switch m.screen {
	case documentScreen:
		if m.currentDocument == nil {
			return
		}
		m.referenceHistory = append(m.referenceHistory, m.referenceFrame(documentScreen))
		m.references = append([]ledger.LabeledReference(nil), m.currentDocument.References...)
		m.referenceOrigin = documentScreen
		m.referencesFromDoc = true
	case sliceScreen:
		m.docContext = sliceScreen
		if m.slice == nil || m.slice.Claim == nil {
			m.status = "No recorded Claim reference for this Slice"
			return
		}
		m.references = []ledger.LabeledReference{{Label: "current Claim state", Reference: m.slice.Claim.Reference}}
		m.referenceOrigin = sliceScreen
		m.referencesFromDoc = false
	default:
		m.status = "Open references from a report, Human Decision, or Slice Claim"
		return
	}
	m.screen = referencesScreen
	m.cursor[referencesScreen] = 0
	m.failure = nil
	m.status = ""
	m.layoutDetail()
}

func (m Model) referenceFrame(returnScreen screen) referenceFrame {
	return referenceFrame{
		references: append([]ledger.LabeledReference(nil), m.references...),
		origin:     m.referenceOrigin, fromDocument: m.referencesFromDoc,
		cursor: m.cursor[referencesScreen], returnScreen: returnScreen,
	}
}

func (m *Model) restoreReferenceFrame(frame referenceFrame) {
	m.references = append([]ledger.LabeledReference(nil), frame.references...)
	m.referenceOrigin, m.referencesFromDoc = frame.origin, frame.fromDocument
	m.cursor[referencesScreen] = frame.cursor
	m.refreshCurrentReferences()
}

func (m *Model) openDocument(document ledger.Document) {
	m.currentDocument = &document
	m.openedContext, m.openedRevision = m.currentContext(), m.snapshot.Revision
	m.newerDocument = false
	m.documentReturn = documentsScreen
	m.documentHistory = nil
	m.referenceHistory = nil
	m.renderProblem = ""
	m.screen = documentScreen
	m.failure = nil
	m.status = ""
	m.docViewport = viewport.New(m.width, m.bodyHeight(m.header(), m.footer()))
	m.layoutDetail()
	m.docViewport.GotoTop()
}

// readingReport reports whether the open document is an implementation or
// watchdog report, whose versions can be browsed.
func (m Model) readingReport() bool {
	return m.screen == documentScreen && m.currentDocument != nil &&
		(m.currentDocument.Kind == ledger.ImplementReportDocumentKind || m.currentDocument.Kind == ledger.WatchdogReportDocumentKind)
}

func (m *Model) openVersions() {
	if !m.readingReport() {
		m.status = "Open a report to browse its versions"
		return
	}
	phase := ledger.ImplementPhase
	if m.currentDocument.Kind == ledger.WatchdogReportDocumentKind {
		phase = ledger.WatchdogPhase
	}
	// A selected version may predate archival, while a followed report may
	// belong to another Slice. Keep the selected report's incarnation when
	// reopening its history, otherwise use the exact document's location.
	archived := m.currentDocument.Archived
	if m.documentReturn == versionsScreen && m.versions != nil {
		archived = m.versions.Archived
	}
	m.openVersionsFor(m.currentDocument.Project, m.currentDocument.Proposal+"/"+m.currentDocument.Slice,
		phase, archived)
}

func (m *Model) openVersionsFor(project, item, phase string, archived bool) {
	versions, err := m.snapshot.VersionsAt(project, item, phase, archived)
	if err != nil {
		m.status = "Cannot discover report versions: " + err.Error()
		return
	}
	m.versions = versions
	m.versionsReturn = m.screen
	m.cursor[versionsScreen] = 0
	m.screen = versionsScreen
	m.status = ""
	m.layoutDetail()
}

func (m *Model) followVersion(reference ledger.Reference) {
	m.followReference(reference)
	if m.screen == documentScreen && len(m.documentHistory) > 0 {
		m.documentReturn = versionsScreen
		m.documentHistory[len(m.documentHistory)-1].fromVersion = true
		m.layoutDetail()
	}
}

func (m *Model) followReference(reference ledger.Reference) {
	document, err := m.snapshot.Document(reference)
	if err != nil {
		m.failure = nil
		m.status = fmt.Sprintf("Exact ledger reference unavailable (%s:%s): %s; no substitute was opened", reference.Commit, reference.Path, err)
		return
	}
	frame := documentFrame{
		viewport: m.docViewport, screen: m.screen,
		returnScreen: m.documentReturn, renderProblem: m.renderProblem,
		versions: m.versions, versionsReturn: m.versionsReturn, versionsCursor: m.cursor[versionsScreen],
		openedContext: m.openedContext, openedRevision: m.openedRevision,
		references:      append([]ledger.LabeledReference(nil), m.references...),
		referenceOrigin: m.referenceOrigin, referencesFromDoc: m.referencesFromDoc,
		referenceCursor:  m.cursor[referencesScreen],
		referenceHistory: append([]referenceFrame(nil), m.referenceHistory...),
	}
	if m.currentDocument != nil {
		frame.document, frame.hasDocument = *m.currentDocument, true
	}
	m.documentHistory = append(m.documentHistory, frame)
	m.currentDocument = document
	m.openedContext, m.openedRevision = m.currentContext(), m.snapshot.Revision
	m.newerDocument = m.documentHasNewerVersion()
	m.documentReturn = referencesScreen
	m.renderProblem = ""
	m.screen = documentScreen
	m.failure = nil
	m.status = ""
	m.docViewport = viewport.New(m.width, m.bodyHeight(m.header(), m.footer()))
	m.layoutDetail()
	m.docViewport.GotoTop()
}

func (m Model) documentContent(document *ledger.Document, width int) (string, string) {
	context := strings.Replace("Context at open: "+m.openedContext, " — current lifecycle:", "\n\ncurrent lifecycle:", 1)
	context = strings.ReplaceAll(context, "; current ", "\n\ncurrent ")
	preamble := []string{context, "", "## Record metadata", ""}
	preamble = append(preamble,
		"Document kind: "+string(document.Kind),
		"Ledger document: "+document.Reference.Commit+":"+document.Reference.Path,
	)
	if document.Reference.Commit == m.openedRevision {
		preamble = append(preamble, "Identity when opened: current snapshot document")
	} else {
		preamble = append(preamble, "Identity when opened: HISTORICAL ledger document; context above describes the ledger when opened, not a historical whole-workflow view")
	}

	if report := document.Report; report != nil {
		phase := "Implementation"
		if document.Kind == ledger.WatchdogReportDocumentKind {
			phase = "Watchdog"
		}
		preamble = append(preamble, "", "### "+phase+" report", "Schema: "+fmt.Sprint(report.Schema), "Outcome: "+report.Outcome)
		if report.Round != 0 {
			preamble = append(preamble, fmt.Sprintf("Review round: %d", report.Round))
		}
		if report.Source.Head != "" || report.Source.Target != "" || report.Source.Reviewed != "" {
			preamble = append(preamble, "", "Source repository revisions:")
			if report.Source.Head != "" {
				preamble = append(preamble, "  Candidate head: "+report.Source.Head)
			}
			if report.Source.Target != "" {
				preamble = append(preamble, "  Integration target: "+report.Source.Target)
			}
			if report.Source.Reviewed != "" {
				preamble = append(preamble, "  Reviewed source revision: "+report.Source.Reviewed)
			}
		}
		if len(document.References) > 0 {
			preamble = append(preamble, "", "Ledger input references:")
			for _, reference := range document.References {
				preamble = append(preamble, "  "+reference.Label+": "+reference.Reference.Commit+":"+reference.Reference.Path)
			}
		}
	}
	if claim := document.Claim; claim != nil {
		preamble = append(preamble, "", "### Recorded Claim", "Phase: "+claim.Phase, "Basis (ledger revision): "+claim.Basis)
		if len(document.References) > 0 {
			preamble = append(preamble, "", "Claim input ledger references:")
			for _, reference := range document.References {
				preamble = append(preamble, "  "+reference.Label+": "+reference.Reference.Commit+":"+reference.Reference.Path)
			}
		}
	}
	if decision := document.Decision; decision != nil {
		preamble = append(preamble,
			"", "### Human Decision", "Schema: "+fmt.Sprint(decision.Schema),
			"Project: "+decision.Project, "Slice: "+decision.Item, "Route: "+decision.Route,
			"Answered request: "+decision.AnsweredRequest.Commit+":"+decision.AnsweredRequest.Path,
		)
	}
	for _, diagnostic := range document.Diagnostics {
		preamble = append(preamble, "", "! Metadata diagnostic: "+DiagnosticText(diagnostic))
	}

	body := document.Body
	if document.Kind == ledger.StateDocumentKind {
		body = "```json\n" + document.Contents + "\n```"
	}
	if body != "" {
		preamble = append(preamble, "", "---", "", body)
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(max(width, 1)))
	if err != nil {
		return strings.Join(preamble, "\n"), "Markdown renderer unavailable: " + err.Error()
	}
	rendered, err := renderer.Render(strings.Join(preamble, "\n"))
	if err != nil {
		return strings.Join(preamble, "\n"), "Markdown rendering failed: " + err.Error()
	}
	return rendered, ""
}

func (m Model) currentContext() string {
	context := m.project
	if m.proposal != "" {
		context += "/" + m.proposal
	}
	if m.docContext == sliceScreen && m.item != "" {
		context += "/" + strings.TrimPrefix(m.item, m.proposal+"/")
		if m.slice != nil {
			if m.slice.Readable {
				context += " — current lifecycle: " + lifecycleLabel(m.slice.Lifecycle)
				if m.slice.Claim == nil {
					context += "; current Claim: none"
				} else {
					context += "; current Claim: " + m.slice.Claim.Phase + " reservation"
				}
				if len(m.slice.Dependencies) == 0 {
					context += "; current Dependencies: none"
				} else {
					states := make([]string, 0, len(m.slice.Dependencies))
					for _, dependency := range m.slice.Dependencies {
						if dependency.Problem != "" {
							states = append(states, dependency.Item+" (unknown)")
						} else {
							states = append(states, dependency.Item+" ("+lifecycleLabel(dependency.Lifecycle)+")")
						}
					}
					context += "; current Dependencies: " + strings.Join(states, ", ")
				}
			} else {
				context += " — current lifecycle, Claim, and Dependencies unknown"
			}
		}
	}
	if m.docContext == proposalScreen && m.members != nil && m.members.Proposal.Archived {
		context += " [archived Proposal]"
	}
	return context + " at current ledger " + m.snapshot.Revision[:min(len(m.snapshot.Revision), 12)]
}

func (m Model) rowsForDocumentList() []ledger.Document {
	if m.documents == nil {
		return nil
	}
	return m.documents.Documents
}

func documentLabel(document ledger.Document) string {
	path := document.Reference.Path
	name := path[strings.LastIndex(path, "/")+1:]
	prefix := ""
	if document.Slice != "" {
		prefix = document.Slice + " — "
	}
	switch document.Kind {
	case ledger.ProposalDocumentKind:
		return "Proposal description"
	case ledger.ContractDocumentKind:
		return prefix + name + " — accepted contract"
	case ledger.ImplementReportDocumentKind:
		return prefix + "Latest implementation report"
	case ledger.WatchdogReportDocumentKind:
		return prefix + "Latest watchdog report"
	case ledger.DecisionDocumentKind:
		return prefix + "Current Human Decision"
	case ledger.StateDocumentKind:
		return prefix + "Claim state (" + name + ")"
	default:
		return prefix + name + " — " + string(document.Kind)
	}
}

func documentSummary(document ledger.Document) string {
	if document.Report != nil {
		text := "Outcome: " + document.Report.Outcome
		if document.Report.Round != 0 {
			text += fmt.Sprintf(" · round %d", document.Report.Round)
		}
		return text
	}
	if document.Decision != nil {
		return "Route: " + document.Decision.Route + " · answered " + document.Decision.AnsweredRequest.Path
	}
	if len(document.Diagnostics) > 0 {
		return "Metadata unavailable; recorded content remains readable"
	}
	return document.Reference.Commit + ":" + document.Reference.Path
}

func optionalDocumentNotes(documents *ledger.DocumentSet) []string {
	if documents == nil {
		return nil
	}
	if documents.Slice == "" {
		return []string{"Only documents present at this committed revision are listed; absent optional reports are not malformed records."}
	}
	var notes []string
	for _, availability := range documents.Availability {
		if availability.Slice != documents.Slice {
			continue
		}
		label := optionalDocumentLabel(availability.Kind)
		switch availability.Status {
		case ledger.DocumentAbsent:
			note := "not yet available (optional)."
			if availability.Kind == ledger.DecisionDocumentKind {
				note = "not available (optional)."
			}
			notes = append(notes, label+": "+note)
		case ledger.DocumentUnavailable:
			notes = append(notes, label+": recorded but unavailable; see diagnostics.")
		case ledger.DocumentUnknown:
			notes = append(notes, label+": membership unknown; see diagnostics.")
		}
	}
	return notes
}

func optionalDocumentLabel(kind ledger.DocumentKind) string {
	switch kind {
	case ledger.ImplementReportDocumentKind:
		return "Latest implementation report"
	case ledger.WatchdogReportDocumentKind:
		return "Latest watchdog report"
	case ledger.DecisionDocumentKind:
		return "Active Human Decision"
	default:
		return string(kind)
	}
}
