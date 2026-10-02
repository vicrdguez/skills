package ledger

import (
	"encoding/json"
	"path"
	"strings"
)

// DocumentKind identifies the ledger record format without interpreting
// Markdown prose.
type DocumentKind string

// DocumentStatus describes the membership or read outcome of an optional document.
type DocumentStatus string

const (
	DocumentAvailable   DocumentStatus = "available"
	DocumentAbsent      DocumentStatus = "absent"
	DocumentUnavailable DocumentStatus = "unavailable"
	DocumentUnknown     DocumentStatus = "unknown"
)

const (
	ProposalDocumentKind        DocumentKind = "proposal"
	ContractDocumentKind        DocumentKind = "contract"
	ImplementReportDocumentKind DocumentKind = "implement-report"
	WatchdogReportDocumentKind  DocumentKind = "watchdog-report"
	DecisionDocumentKind        DocumentKind = "decision"
	StateDocumentKind           DocumentKind = "state"
)

// LabeledReference is a typed ledger reference extracted from structured
// report, decision, or Claim metadata. Repeated labels preserve the source order.
type LabeledReference struct {
	Label     string    `json:"label"`
	Reference Reference `json:"reference"`
}

// DocumentAvailability is the query-owned membership and read status of one
// optional document for a Slice. Reference identifies the committed document
// when available or unavailable, and the state record when decision membership
// is unknown.
type DocumentAvailability struct {
	Slice     string         `json:"slice"`
	Kind      DocumentKind   `json:"kind"`
	Status    DocumentStatus `json:"status"`
	Reference *Reference     `json:"reference,omitempty"`
}

// Document is one exact committed ledger document. Contents are retained even
// when metadata cannot be interpreted; structured facts
// and their references are available separately when parsing succeeds.
type Document struct {
	Kind     DocumentKind `json:"kind"`
	Project  string       `json:"project"`
	Proposal string       `json:"proposal"`
	Slice    string       `json:"slice,omitempty"`
	// Archived records the document's location at its exact revision, not
	// necessarily the location of its current Proposal after archival.
	Archived    bool               `json:"archived"`
	Reference   Reference          `json:"reference"`
	Contents    string             `json:"contents"`
	Body        string             `json:"body"`
	Report      *Report            `json:"report,omitempty"`
	Decision    *DecisionRecord    `json:"decision,omitempty"`
	Claim       *Claim             `json:"claim,omitempty"`
	References  []LabeledReference `json:"references,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}

// DocumentSet is the document membership of one Proposal or Slice at one
// committed revision. Diagnostics identify unreadable documents or
// uninterpretable metadata without discarding other readable documents.
type DocumentSet struct {
	Revision     string                 `json:"revision"`
	Project      string                 `json:"project"`
	Proposal     string                 `json:"proposal"`
	Slice        string                 `json:"slice,omitempty"`
	Archived     bool                   `json:"archived"`
	Documents    []Document             `json:"documents"`
	Availability []DocumentAvailability `json:"availability"`
	Diagnostics  []Diagnostic           `json:"diagnostics,omitempty"`
	Incomplete   bool                   `json:"incomplete"`
}

// DocumentEntry is one listed document: its identity and the metadata that
// labels and diagnoses it, without its text. Document reads the text.
type DocumentEntry struct {
	Kind        DocumentKind
	Project     string
	Proposal    string
	Slice       string
	Reference   Reference
	Report      *Report
	Decision    *DecisionRecord
	Diagnostics []Diagnostic
}

// DocumentList carries a DocumentSet's entries, order, availability and
// diagnostics without retaining document text. It reads metadata-bearing
// records (phase reports, the Human Decision and Slice state), but checks
// only prose object headers. Prose with a readable header and corrupt body
// is diagnosed by Document when opened, not by the list.
type DocumentList struct {
	Revision     string
	Project      string
	Proposal     string
	Slice        string
	Archived     bool
	Documents    []DocumentEntry
	Availability []DocumentAvailability
	Diagnostics  []Diagnostic
}

// ProposalDocumentListAt lists a Proposal's documents without reading prose
// bodies. Metadata-bearing records are read to label and diagnose entries.
func (v *Snapshot) ProposalDocumentListAt(projectName, proposalName string, archived bool) (*DocumentList, error) {
	project, proposal, err := v.proposalLocated(projectName, proposalName, &archived)
	if err != nil {
		return nil, err
	}
	paths := []string{v.proposalPath(project.name, proposal) + "/proposal.md"}
	for _, slice := range proposal.slices {
		paths = append(paths, v.sliceFiles(project, proposal, slice)...)
	}
	return v.proposalDocuments(project, proposal, v.listingRead(paths)).list(), nil
}

// SliceDocumentListAt lists a Slice's documents without reading prose bodies.
// Metadata-bearing records are read to label and diagnose entries.
func (v *Snapshot) SliceDocumentListAt(projectName, item string, archived bool) (*DocumentList, error) {
	project, proposal, slice, err := v.documentSlice(projectName, item, &archived)
	if err != nil {
		return nil, err
	}
	return v.sliceDocuments(project, proposal, slice, v.listingRead(v.sliceFiles(project, proposal, slice))).list(), nil
}

func (v *Snapshot) sliceFiles(project *projectTree, proposal *proposalTree, slice *sliceTree) []string {
	base := v.slicePath(project.name, proposal, slice.name)
	paths := make([]string, 0, len(slice.files))
	for name := range slice.files {
		paths = append(paths, base+"/"+name)
	}
	return paths
}

// listingRead serves a listing's reads from two batched lookups over the
// given committed paths: metadata-bearing records are read and interpreted,
// while prose documents (proposal descriptions and accepted contracts) are
// only confirmed to exist. A record the batch leaves unanswered is an exact
// read with its own diagnostic, and so is prose the batch reports missing,
// which reads no text. When the prose lookup itself fails, each prose
// document is listed with a diagnostic instead, so no prose text is read.
func (v *Snapshot) listingRead(paths []string) documentRead {
	var records, prose []Reference
	for _, documentPath := range paths {
		switch _, _, _, kind, ok := recordDocumentIdentity(documentPath); {
		case !ok:
		case kind == ProposalDocumentKind || kind == ContractDocumentKind:
			prose = append(prose, Reference{Commit: v.Revision, Path: documentPath})
		default:
			records = append(records, Reference{Commit: v.Revision, Path: documentPath})
		}
	}
	known := map[string]*Document{}
	if objects, err := v.objects(records, true); err == nil {
		for index, object := range objects {
			if object.kind == "blob" {
				known[records[index].Path] = recordDocument(records[index], string(object.contents))
			}
		}
	}
	objects, err := v.objects(prose, false)
	for index, reference := range prose {
		switch {
		case err != nil:
			document := recordDocument(reference, "")
			scope, subject := ScopeSlice, document.Project+"/"+document.Proposal+"/"+document.Slice
			if document.Kind == ProposalDocumentKind {
				scope, subject = ScopeProposal, document.Project+"/"+document.Proposal
			}
			document.Diagnostics = []Diagnostic{{Scope: scope, Subject: subject, Problem: "cannot confirm document " + reference.Path + " is readable: " + err.Error()}}
			known[reference.Path] = document
		case objects[index].kind == "blob":
			known[reference.Path] = recordDocument(reference, "")
		}
	}
	return func(documentPath string) (*Document, error) {
		if document, ok := known[documentPath]; ok {
			return document, nil
		}
		return v.currentDocument(documentPath)
	}
}

func (set *DocumentSet) list() *DocumentList {
	list := &DocumentList{
		Revision: set.Revision, Project: set.Project, Proposal: set.Proposal, Slice: set.Slice, Archived: set.Archived,
		Documents: make([]DocumentEntry, 0, len(set.Documents)), Availability: set.Availability, Diagnostics: set.Diagnostics,
	}
	for _, document := range set.Documents {
		list.Documents = append(list.Documents, document.Entry())
	}
	return list
}

// Entry is the document as a document list offers it, without its text.
func (d *Document) Entry() DocumentEntry {
	return DocumentEntry{
		Kind: d.Kind, Project: d.Project, Proposal: d.Proposal, Slice: d.Slice,
		Reference: d.Reference, Report: d.Report, Decision: d.Decision, Diagnostics: d.Diagnostics,
	}
}

// ProposalDocuments returns the proposal description and the committed
// documents of each member Slice. Membership and current contents are pinned
// to this Snapshot's revision; an active Proposal is preferred when both
// locations hold the same identity.
func (v *Snapshot) ProposalDocuments(projectName, proposalName string) (*DocumentSet, error) {
	return v.proposalDocumentsLocated(projectName, proposalName, nil)
}

// ProposalDocumentsAt selects the recorded location when both active and
// archived Proposals have the same name.
func (v *Snapshot) ProposalDocumentsAt(projectName, proposalName string, archived bool) (*DocumentSet, error) {
	return v.proposalDocumentsLocated(projectName, proposalName, &archived)
}

func (v *Snapshot) proposalDocumentsLocated(projectName, proposalName string, location *bool) (*DocumentSet, error) {
	project, proposal, err := v.proposalLocated(projectName, proposalName, location)
	if err != nil {
		return nil, err
	}
	return v.proposalDocuments(project, proposal, v.currentDocument), nil
}

func (v *Snapshot) proposalDocuments(project *projectTree, proposal *proposalTree, read documentRead) *DocumentSet {
	result := &DocumentSet{
		Revision: v.Revision, Project: project.name, Proposal: proposal.name,
		Archived: proposal.archived, Documents: []Document{}, Availability: []DocumentAvailability{}, Diagnostics: []Diagnostic{},
	}
	result.Diagnostics = append(result.Diagnostics, proposal.invalid...)
	path := v.proposalPath(project.name, proposal) + "/proposal.md"
	addDocument(result, read, path, ProposalDocumentKind, ScopeProposal, project.name+"/"+proposal.name)
	for _, slice := range proposal.slices {
		v.addSliceDocuments(result, read, project, proposal, slice)
	}
	result.Incomplete = len(result.Diagnostics) > 0
	return result
}

// SliceDocuments returns one Slice's accepted content, latest committed
// phase reports, and active Human Decision at this Snapshot's revision. An
// active Proposal is preferred when both locations hold the same identity.
func (v *Snapshot) SliceDocuments(projectName, item string) (*DocumentSet, error) {
	return v.sliceDocumentsLocated(projectName, item, nil)
}

// SliceDocumentsAt selects the recorded location when active and archived
// Proposals contain the same Slice identity.
func (v *Snapshot) SliceDocumentsAt(projectName, item string, archived bool) (*DocumentSet, error) {
	return v.sliceDocumentsLocated(projectName, item, &archived)
}

func (v *Snapshot) sliceDocumentsLocated(projectName, item string, location *bool) (*DocumentSet, error) {
	project, proposal, slice, err := v.documentSlice(projectName, item, location)
	if err != nil {
		return nil, err
	}
	return v.sliceDocuments(project, proposal, slice, v.currentDocument), nil
}

func (v *Snapshot) documentSlice(projectName, item string, location *bool) (*projectTree, *proposalTree, *sliceTree, error) {
	proposalName, sliceName, found := strings.Cut(item, "/")
	if !found || !ValidRecordName(proposalName) || !ValidRecordName(sliceName) {
		return nil, nil, nil, refuse(
			"Slice reference "+item+" is not a proposal/slice identity",
			"select a Slice listed by its Proposal, such as add-order-cancellation/foundation",
		)
	}
	project, proposal, err := v.proposalLocated(projectName, proposalName, location)
	if err != nil {
		return nil, nil, nil, err
	}
	slice := proposal.slice(sliceName)
	if slice == nil {
		return nil, nil, nil, refuse(
			"no Slice "+item+" in project "+project.name+" at ledger revision "+v.Revision,
			"select a Slice listed by its Proposal",
		)
	}
	return project, proposal, slice, nil
}

func (v *Snapshot) sliceDocuments(project *projectTree, proposal *proposalTree, slice *sliceTree, read documentRead) *DocumentSet {
	result := &DocumentSet{
		Revision: v.Revision, Project: project.name, Proposal: proposal.name,
		Slice: slice.name, Archived: proposal.archived,
		Documents: []Document{}, Availability: []DocumentAvailability{}, Diagnostics: []Diagnostic{},
	}
	v.addSliceDocuments(result, read, project, proposal, slice)
	result.Incomplete = len(result.Diagnostics) > 0
	return result
}

// Document reads one exact current or historical ledger reference. It accepts
// only known record-document paths, then delegates exact commit/path access to
// ShowReference without substituting another revision or changing its legacy
// raw-document behavior.
func (v *Snapshot) Document(reference Reference) (*Document, error) {
	if !validRecordDocumentPath(reference.Path) {
		return nil, refuse(
			"reference path "+reference.Path+" is not a ledger record document",
			"supply an exact proposal.md, accepted Slice document, phase report, decision.md, or state.json path",
		)
	}
	contents, err := ShowReference(v.store, reference.Commit, reference.Path)
	if err != nil {
		return nil, err
	}
	return recordDocument(Reference{Commit: contents.Commit, Path: contents.Path}, contents.Contents), nil
}

// recordDocument interprets the committed contents at reference, whose path
// the caller has already recognized as a record document path.
func recordDocument(reference Reference, contents string) *Document {
	project, proposal, slice, kind, _ := recordDocumentIdentity(reference.Path)
	document := &Document{
		Kind: kind, Project: project, Proposal: proposal, Slice: slice,
		Archived:  strings.Split(reference.Path, "/")[2] == archiveRoot,
		Reference: reference,
		Contents:  contents,
		Body:      contents,
	}
	document.parseMetadata()
	return document
}

// documentRead reads one current record document at the Snapshot's revision.
type documentRead func(documentPath string) (*Document, error)

func (v *Snapshot) currentDocument(documentPath string) (*Document, error) {
	return v.Document(Reference{Commit: v.Revision, Path: documentPath})
}

func (v *Snapshot) addSliceDocuments(result *DocumentSet, read documentRead, project *projectTree, proposal *proposalTree, slice *sliceTree) {
	subject := project.name + "/" + proposal.name + "/" + slice.name
	base := v.slicePath(project.name, proposal, slice.name)
	for _, name := range []string{"intent.md", "behavior.md"} {
		addDocument(result, read, base+"/"+name, ContractDocumentKind, ScopeSlice, subject)
	}
	for _, name := range []string{"plan.md", "tasks.md"} {
		if slice.files[name] {
			addDocument(result, read, base+"/"+name, ContractDocumentKind, ScopeSlice, subject)
		}
	}
	for _, phase := range []string{ImplementPhase, WatchdogPhase} {
		name := phase + "-report.md"
		kind := ImplementReportDocumentKind
		if phase == WatchdogPhase {
			kind = WatchdogReportDocumentKind
		}
		v.addOptionalDocument(result, read, base+"/"+name, kind, slice.name, slice.files[name], subject)
	}

	// decision.md is current only while state.json marks it active. Without
	// readable state the decision's membership is unknown, not absent.
	statePath := base + "/state.json"
	stateReference := Reference{Commit: v.Revision, Path: statePath}
	state, err := read(statePath)
	if err != nil {
		result.addDiagnostic(Diagnostic{Scope: ScopeSlice, Subject: subject, Problem: "cannot read state.json to determine current decision: " + err.Error()})
		result.Availability = append(result.Availability, DocumentAvailability{
			Slice: slice.name, Kind: DecisionDocumentKind, Status: DocumentUnknown, Reference: &stateReference,
		})
		return
	}
	var record SliceState
	if err := json.Unmarshal([]byte(state.Contents), &record); err != nil {
		result.addDiagnostic(Diagnostic{Scope: ScopeSlice, Subject: subject, Problem: "state.json is malformed, so current decision membership is unknown"})
		result.Availability = append(result.Availability, DocumentAvailability{
			Slice: slice.name, Kind: DecisionDocumentKind, Status: DocumentUnknown, Reference: &stateReference,
		})
		return
	}
	if !record.Decision {
		result.Availability = append(result.Availability, DocumentAvailability{
			Slice: slice.name, Kind: DecisionDocumentKind, Status: DocumentAbsent,
		})
		return
	}
	v.addOptionalDocument(result, read, base+"/decision.md", DecisionDocumentKind, slice.name, true, subject)
}

func (v *Snapshot) addOptionalDocument(result *DocumentSet, read documentRead, documentPath string, kind DocumentKind, slice string, member bool, subject string) {
	if !member {
		result.Availability = append(result.Availability, DocumentAvailability{
			Slice: slice, Kind: kind, Status: DocumentAbsent,
		})
		return
	}
	reference := Reference{Commit: v.Revision, Path: documentPath}
	status := DocumentUnavailable
	if addDocument(result, read, documentPath, kind, ScopeSlice, subject) {
		status = DocumentAvailable
	}
	result.Availability = append(result.Availability, DocumentAvailability{
		Slice: slice, Kind: kind, Status: status, Reference: &reference,
	})
}

func addDocument(result *DocumentSet, read documentRead, documentPath string, kind DocumentKind, scope, subject string) bool {
	document, err := read(documentPath)
	if err != nil {
		result.addDiagnostic(Diagnostic{Scope: scope, Subject: subject, Problem: "cannot read document " + documentPath + ": " + err.Error()})
		return false
	}
	// The record tree selects membership and the path determines format; this
	// assertion keeps accidental mismatches visible rather than mislabeling a
	// document returned by the exact-reference reader.
	if document.Kind != kind {
		result.addDiagnostic(Diagnostic{Scope: scope, Subject: subject, Problem: "document " + documentPath + " has an unexpected record kind"})
		return false
	}
	result.Documents = append(result.Documents, *document)
	for _, diagnostic := range document.Diagnostics {
		result.addDiagnostic(diagnostic)
	}
	return true
}

func (result *DocumentSet) addDiagnostic(diagnostic Diagnostic) {
	result.Diagnostics = appendDiagnostic(result.Diagnostics, diagnostic)
}

func (document *Document) parseMetadata() {
	scope, subject := ScopeSlice, document.Project+"/"+document.Proposal
	if document.Slice != "" {
		subject += "/" + document.Slice
	}
	switch document.Kind {
	case ImplementReportDocumentKind, WatchdogReportDocumentKind:
		phase := ImplementPhase
		if document.Kind == WatchdogReportDocumentKind {
			phase = WatchdogPhase
		}
		report, body, err := ParseReport(phase, []byte(document.Contents))
		if err != nil {
			document.Diagnostics = []Diagnostic{{Scope: scope, Subject: subject, Problem: phase + " report metadata is unreadable: " + err.Error()}}
			return
		}
		document.Report = &report
		document.Body = body
		document.References = reportReferences(report)
		for _, reference := range document.References {
			if !validRecordDocumentPath(reference.Reference.Path) {
				document.Diagnostics = appendDiagnostic(document.Diagnostics, Diagnostic{
					Scope: scope, Subject: subject,
					Problem: phase + " report " + reference.Label + " reference is not a ledger record document path",
				})
			}
		}
	case StateDocumentKind:
		var state SliceState
		if err := json.Unmarshal([]byte(document.Contents), &state); err != nil {
			document.Diagnostics = []Diagnostic{{Scope: scope, Subject: subject, Problem: "state metadata is unreadable: " + err.Error()}}
			return
		}
		if state.Claim == nil {
			return
		}
		document.Claim = state.Claim
		for _, reference := range state.Claim.Inputs.Contract {
			document.References = append(document.References, LabeledReference{Label: "contract", Reference: reference})
		}
		for _, entry := range []struct {
			label string
			ref   *Reference
		}{{"implement", state.Claim.Inputs.Implement}, {"watchdog", state.Claim.Inputs.Watchdog}, {"decision", state.Claim.Inputs.Decision}} {
			if entry.ref != nil {
				document.References = append(document.References, LabeledReference{Label: entry.label, Reference: *entry.ref})
			}
		}
		for _, reference := range document.References {
			if !validRecordDocumentPath(reference.Reference.Path) {
				document.Diagnostics = appendDiagnostic(document.Diagnostics, Diagnostic{
					Scope: scope, Subject: subject,
					Problem: "Claim " + reference.Label + " reference is not a ledger record document path",
				})
			}
		}
	case DecisionDocumentKind:
		record, body, err := ParseDecision([]byte(document.Contents))
		if err != nil {
			document.Diagnostics = []Diagnostic{{Scope: scope, Subject: subject, Problem: "decision metadata is unreadable: " + err.Error()}}
			return
		}
		document.Decision = &record
		document.Body = body
		document.References = []LabeledReference{{Label: "answered_request", Reference: record.AnsweredRequest}}
		if !validRecordDocumentPath(record.AnsweredRequest.Path) {
			document.Diagnostics = []Diagnostic{{
				Scope: scope, Subject: subject,
				Problem: "decision answered_request reference is not a ledger record document path",
			}}
		}
	}
}

func reportReferences(report Report) []LabeledReference {
	references := make([]LabeledReference, 0, 4+len(report.Ledger.Contract))
	references = append(references, LabeledReference{Label: "claim", Reference: report.Ledger.Claim})
	for _, reference := range report.Ledger.Contract {
		references = append(references, LabeledReference{Label: "contract", Reference: reference})
	}
	if report.Ledger.Implement != nil {
		references = append(references, LabeledReference{Label: "implement", Reference: *report.Ledger.Implement})
	}
	if report.Ledger.Watchdog != nil {
		references = append(references, LabeledReference{Label: "watchdog", Reference: *report.Ledger.Watchdog})
	}
	if report.Ledger.Decision != nil {
		references = append(references, LabeledReference{Label: "decision", Reference: *report.Ledger.Decision})
	}
	return references
}

// recordDocumentIdentity strictly recognizes supported ledger record paths.
// In particular, it rejects paths that ShowReference's historical raw reader
// would otherwise clean before access.
func recordDocumentIdentity(value string) (project, proposal, slice string, kind DocumentKind, ok bool) {
	if value == "" || strings.ContainsRune(value, '\\') || path.Clean(value) != value {
		return "", "", "", "", false
	}
	parts := strings.Split(value, "/")
	if len(parts) != 5 && len(parts) != 6 || parts[0] != projectsRoot || !ValidRecordName(parts[1]) {
		return "", "", "", "", false
	}
	if parts[2] != "proposals" && parts[2] != archiveRoot || !ValidRecordName(parts[3]) {
		return "", "", "", "", false
	}
	project, proposal = parts[1], parts[3]
	if len(parts) == 5 {
		if parts[4] == "proposal.md" {
			return project, proposal, "", ProposalDocumentKind, true
		}
		return "", "", "", "", false
	}
	if !ValidRecordName(parts[4]) {
		return "", "", "", "", false
	}
	slice = parts[4]
	switch parts[5] {
	case "intent.md", "behavior.md", "plan.md", "tasks.md":
		kind = ContractDocumentKind
	case "implement-report.md":
		kind = ImplementReportDocumentKind
	case "watchdog-report.md":
		kind = WatchdogReportDocumentKind
	case "decision.md":
		kind = DecisionDocumentKind
	case "state.json":
		kind = StateDocumentKind
	default:
		return "", "", "", "", false
	}
	return project, proposal, slice, kind, true
}

func validRecordDocumentPath(value string) bool {
	_, _, _, _, ok := recordDocumentIdentity(value)
	return ok
}
