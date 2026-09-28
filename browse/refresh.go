package browse

import (
	"errors"
	"maps"
	"strings"

	"github.com/vicrdguez/skills/ledger"
)

// selectedIdentity identifies a list row by record identity rather than its
// position; a fresh grouping or membership can move that row.
func (m Model) selectedIdentity(at screen) string {
	cursor := m.cursor[at]
	if cursor < 0 {
		return ""
	}
	switch at {
	case overviewScreen:
		if m.overview != nil && cursor < len(m.overview.Projects) {
			return m.overview.Projects[cursor].Name
		}
	case projectScreen:
		if m.inventory != nil && cursor < len(m.inventory.Proposals) {
			p := m.inventory.Proposals[cursor]
			return p.Name + locationKey(p.Archived)
		}
	case proposalScreen:
		if m.members != nil && cursor < len(m.members.Slices) {
			return m.members.Slices[cursor].Item
		}
	case resultsScreen:
		if m.search != nil && cursor < len(m.results()) {
			row := m.results()[cursor]
			return row.project + "/" + row.match.Item + locationKey(row.match.Archived)
		}
	case documentsScreen:
		if m.documents != nil && cursor < len(m.documents.Documents) {
			return m.documents.Documents[cursor].Reference.Path
		}
	}
	return ""
}

func locationKey(archived bool) string {
	if archived {
		return " [archived]"
	}
	return " [active]"
}

func (m *Model) preserveSelection(previous Model, at screen) {
	identity := previous.selectedIdentity(at)
	if previous.missingScreen == at && previous.missingIdentity != "" {
		identity = previous.missingIdentity
	}
	if identity == "" {
		return
	}
	for index := 0; index < m.rows(); index++ {
		m.cursor[at] = index
		if m.selectedIdentity(at) == identity {
			return
		}
	}
	m.cursor[at] = 0
	m.missingIdentity = identity
	m.missingScreen = at
	m.selectionMissing = "Selected " + identity + " is no longer available in these results; move to select another"
}

// The query API currently exposes missing-record refusals by invariant text;
// these prefixes must change with its wording until it exposes a category.
func missingRecord(err error) bool {
	var refusal *ledger.Refusal
	if !errors.As(err, &refusal) {
		return false
	}
	return strings.HasPrefix(refusal.Invariant, "no Slice ") ||
		strings.HasPrefix(refusal.Invariant, "no Proposal ") ||
		strings.HasPrefix(refusal.Invariant, "unknown Project ")
}

// publish stages all facts used by the visible screen at the new revision
// before changing the selected view. An opened document is never reselected:
// only its surrounding current facts and its available-document list advance.
func (m *Model) publish(snapshot *ledger.Snapshot) {
	previous := *m
	staged := *m
	staged.cursor = maps.Clone(m.cursor)
	staged.snapshot = snapshot
	staged.selectionMissing, staged.missingIdentity, staged.missingScreen = "", "", 0
	at := m.screen
	overlay := isDocumentOverlay(at) || (at == diagnosticsScreen && m.diagnosticReturn == documentsScreen)
	if overlay {
		at = m.docContext
	} else if at == diagnosticsScreen {
		at = resultsScreen
	}
	staged.screen = at
	staged.load()
	if staged.failure != nil && !missingRecord(staged.failure) {
		m.refreshFailure = staged.failure
		return
	}
	if staged.failure != nil {
		staged.selectionMissing = "Selected entity is no longer available at the new ledger revision; go back to choose another"
		staged.missingScreen = at
		staged.overview, staged.inventory, staged.members, staged.slice, staged.search, staged.documents = nil, nil, nil, nil, nil, nil
	} else if at == overviewScreen || at == projectScreen || at == proposalScreen || at == resultsScreen {
		staged.preserveSelection(previous, at)
	}
	if overlay && staged.failure == nil {
		var err error
		if m.docContext == sliceScreen {
			staged.documents, err = snapshot.SliceDocumentsAt(m.project, m.item, m.archived)
		} else {
			staged.documents, err = snapshot.ProposalDocumentsAt(m.project, m.proposal, m.archived)
		}
		if err != nil && !missingRecord(err) {
			m.refreshFailure = err
			return
		}
		if err != nil {
			staged.documents = nil
			staged.selectionMissing = "Document selection is no longer available at the new ledger revision"
			staged.missingScreen = documentsScreen
		}
		if staged.documents != nil {
			// The viewer and its references can outlive their return-list row.
			staged.screen = documentsScreen
			staged.preserveSelection(previous, documentsScreen)
			staged.screen = at
		}
	}
	if at == sliceScreen && staged.failure == nil {
		staged.detail.SetYOffset(previous.detail.YOffset)
		if m.parent[sliceScreen] == resultsScreen && m.search != nil {
			var err error
			staged.query.IncludeArchived = m.includeArchived
			staged.search, err = snapshot.FindSlices(staged.query)
			if err != nil {
				m.refreshFailure = err
				return
			}
			staged.screen = resultsScreen
			staged.preserveSelection(previous, resultsScreen)
			staged.screen = at
		}
	}
	staged.screen = m.screen
	staged.refreshCurrentReferences()
	if overlay {
		if at == sliceScreen && m.screen != diagnosticsScreen {
			staged.detail.SetYOffset(previous.detail.YOffset)
		}
		// Keep an exact opened document's rendered text, selection and scroll.
		staged.docViewport = previous.docViewport
		staged.renderProblem = previous.renderProblem
		staged.failure = nil
		staged.newerDocument = staged.documentHasNewerVersion()
	}
	if m.screen == diagnosticsScreen {
		staged.layoutDetail()
		staged.detail.SetYOffset(previous.detail.YOffset)
	}
	staged.refreshFailure = nil
	*m = staged
}

// A current Claim reference is a fact of the selected snapshot, not of the
// pinned document or the return frame that originally opened it.
func (m *Model) refreshCurrentReferences() {
	if m.referencesFromDoc || m.referenceOrigin != sliceScreen {
		return
	}
	m.references = nil
	if m.slice != nil && m.slice.Claim != nil {
		m.references = []ledger.LabeledReference{{Label: "current Claim state", Reference: m.slice.Claim.Reference}}
	}
	m.cursor[referencesScreen] = 0
}

func (m Model) documentHasNewerVersion() bool {
	if m.currentDocument == nil {
		return false
	}
	for _, current := range m.rowsForDocumentList() {
		if current.Project == m.currentDocument.Project && current.Proposal == m.currentDocument.Proposal &&
			current.Slice == m.currentDocument.Slice && current.Kind == m.currentDocument.Kind &&
			strings.HasSuffix(current.Reference.Path, "/"+documentName(m.currentDocument.Reference.Path)) {
			// Moving a record into the archive without changing its bytes is
			// not a newer document version.
			return current.Contents != m.currentDocument.Contents
		}
	}
	// Exact referenced documents (notably state.json) need not occur in
	// the current document list. Read their current committed path directly.
	if m.currentDocument.Reference.Commit == m.snapshot.Revision {
		return false
	}
	current, err := m.snapshot.Document(ledger.Reference{Commit: m.snapshot.Revision, Path: m.currentDocument.Reference.Path})
	return err == nil && current.Contents != m.currentDocument.Contents
}

func documentName(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}
