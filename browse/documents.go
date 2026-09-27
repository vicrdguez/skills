package browse

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/vicrdguez/skills/ledger"
)

type documentFrame struct {
	document      ledger.Document
	viewport      viewport.Model
	returnScreen  screen
	renderProblem string
}

func (m *Model) openDocuments() {
	switch m.screen {
	case proposalScreen:
		if m.members == nil {
			return
		}
		m.docContext = proposalScreen
		m.documents, m.failure = m.snapshot.ProposalDocuments(m.project, m.proposal)
	case sliceScreen:
		if m.slice == nil {
			return
		}
		m.docContext = sliceScreen
		m.documents, m.failure = m.snapshot.SliceDocuments(m.project, m.item)
	default:
		m.status = "Open documents from a Proposal or Slice"
		return
	}
	m.screen = documentsScreen
	m.documentHistory = nil
	m.currentDocument = nil
	m.references = nil
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

func (m *Model) openDocument(document ledger.Document) {
	m.currentDocument = &document
	m.documentReturn = documentsScreen
	m.documentHistory = nil
	m.renderProblem = ""
	m.screen = documentScreen
	m.failure = nil
	m.status = ""
	m.docViewport = viewport.New(m.width, m.bodyHeight(m.header(), m.footer()))
	m.layoutDetail()
	m.docViewport.GotoTop()
}

func (m *Model) followReference(reference ledger.Reference) {
	document, err := m.snapshot.Document(reference)
	if err != nil {
		m.failure = nil
		m.status = fmt.Sprintf("Exact ledger reference unavailable (%s:%s): %s; no substitute was opened", reference.Commit, reference.Path, err)
		return
	}
	if m.referencesFromDoc && m.screen == referencesScreen && m.currentDocument != nil {
		m.documentHistory = append(m.documentHistory, documentFrame{
			document: *m.currentDocument, viewport: m.docViewport,
			returnScreen: m.documentReturn, renderProblem: m.renderProblem,
		})
	}
	m.currentDocument = document
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
	var preamble []string
	preamble = append(preamble, "## Record metadata", "")
	preamble = append(preamble,
		"Document kind: "+string(document.Kind),
		"Ledger document: "+document.Reference.Commit+":"+document.Reference.Path,
	)
	if document.Reference.Commit == m.snapshot.Revision {
		preamble = append(preamble, "Identity: current snapshot document")
	} else {
		preamble = append(preamble, "Identity: HISTORICAL ledger document; current lifecycle, Claim, and Dependencies remain from the current snapshot")
	}
	preamble = append(preamble, "Current browsing context: "+m.currentContext())

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
					context += "; current Claim: " + claimText(m.slice.Claim.Phase, m.slice.Claim.Basis)
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
	return context + " at current ledger " + m.snapshot.Revision
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
	found := map[ledger.DocumentKind]bool{}
	for _, document := range documents.Documents {
		found[document.Kind] = true
	}
	if !found[ledger.ImplementReportDocumentKind] {
		notes = append(notes, "Latest implementation report: not yet available (optional).")
	}
	if !found[ledger.WatchdogReportDocumentKind] {
		notes = append(notes, "Latest watchdog report: not yet available (optional).")
	}
	if !found[ledger.DecisionDocumentKind] {
		notes = append(notes, "Active Human Decision: not available (optional).")
	}
	return notes
}
