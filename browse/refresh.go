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
	case readerScreen:
		if cursor < len(m.reader.entries) {
			return m.reader.entries[cursor].id
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
// before changing the selected view. The displayed document is never
// replaced: only its surrounding current facts and the reader's navigator
// advance.
func (m *Model) publish(snapshot *ledger.Snapshot) {
	previous := *m
	staged := *m
	staged.cursor = maps.Clone(m.cursor)
	staged.snapshot = snapshot
	staged.selectionMissing, staged.missingIdentity, staged.missingScreen = "", "", 0
	at := m.screen
	overlay := isDocumentOverlay(at) || (at == diagnosticsScreen && m.diagnosticReturn == readerScreen)
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
		staged.overview, staged.inventory, staged.members, staged.slice, staged.search = nil, nil, nil, nil, nil
		staged.reader.list, staged.reader.entries = nil, nil
	} else if at == overviewScreen || at == projectScreen || at == proposalScreen || at == resultsScreen {
		staged.preserveSelection(previous, at)
	}
	if overlay && staged.failure == nil && m.reader.active {
		reader, err := staged.readDocuments(snapshot)
		if err != nil && !missingRecord(err) {
			m.refreshFailure = err
			return
		}
		staged.reader = reader
		if err != nil {
			staged.selectionMissing = "Document selection is no longer available at the new ledger revision"
			staged.missingScreen = readerScreen
		} else {
			staged.screen = readerScreen
			staged.preserveSelection(previous, readerScreen)
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
		staged.failure = nil
		staged.reader.shown.newer = staged.documentHasNewerVersion(staged.reader.shown.document)
		staged.layoutDetail()
	}
	if m.screen == diagnosticsScreen {
		staged.layoutDetail()
		staged.detail.SetYOffset(previous.detail.YOffset)
	}
	staged.refreshFailure = nil
	*m = staged
}

// A current Claim reference is a fact of the selected snapshot, not of a
// displayed document or the return path that originally opened it.
func (m *Model) refreshCurrentReferences() {
	if m.referenceOrigin != sliceScreen {
		return
	}
	m.references = nil
	if m.slice != nil && m.slice.Claim != nil {
		m.references = []ledger.LabeledReference{{Label: "current Claim state", Reference: m.slice.Claim.Reference}}
	}
	m.cursor[referencesScreen] = 0
}

// documentHasNewerVersion compares a displayed document's text with the
// current committed text of the same document, read at its listed location.
func (m Model) documentHasNewerVersion(document *ledger.Document) bool {
	if document == nil {
		return false
	}
	// Exact referenced documents (notably state.json) need not occur in the
	// current document list; their current committed path is the same one.
	current := ledger.Reference{Commit: m.snapshot.Revision, Path: document.Reference.Path}
	var listed []ledger.DocumentEntry
	if m.reader.list != nil {
		listed = m.reader.list.Documents
	}
	for _, entry := range listed {
		if entry.Project == document.Project && entry.Proposal == document.Proposal &&
			entry.Slice == document.Slice && entry.Kind == document.Kind &&
			documentName(entry.Reference.Path) == documentName(document.Reference.Path) {
			// An archive move lists the same document at another path; it is
			// newer only if its bytes changed.
			current = entry.Reference
			break
		}
	}
	if current == document.Reference {
		return false
	}
	latest, err := m.snapshot.Document(current)
	return err == nil && latest.Contents != document.Contents
}

func documentName(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}
