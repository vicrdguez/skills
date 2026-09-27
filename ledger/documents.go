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
	result := &DocumentSet{
		Revision: v.Revision, Project: project.name, Proposal: proposal.name,
		Archived: proposal.archived, Documents: []Document{}, Availability: []DocumentAvailability{}, Diagnostics: []Diagnostic{},
	}
	result.Diagnostics = append(result.Diagnostics, proposal.invalid...)
	path := v.proposalPath(project.name, proposal) + "/proposal.md"
	v.addDocument(result, path, ProposalDocumentKind, ScopeProposal, project.name+"/"+proposal.name)
	for _, slice := range proposal.slices {
		v.addSliceDocuments(result, project, proposal, slice)
	}
	result.Incomplete = len(result.Diagnostics) > 0
	return result, nil
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
	proposalName, sliceName, found := strings.Cut(item, "/")
	if !found || !ValidRecordName(proposalName) || !ValidRecordName(sliceName) {
		return nil, refuse(
			"Slice reference "+item+" is not a proposal/slice identity",
			"select a Slice listed by its Proposal, such as add-order-cancellation/foundation",
		)
	}
	project, proposal, err := v.proposalLocated(projectName, proposalName, location)
	if err != nil {
		return nil, err
	}
	slice := proposal.slice(sliceName)
	if slice == nil {
		return nil, refuse(
			"no Slice "+item+" in project "+project.name+" at ledger revision "+v.Revision,
			"select a Slice listed by its Proposal",
		)
	}
	result := &DocumentSet{
		Revision: v.Revision, Project: project.name, Proposal: proposal.name,
		Slice: slice.name, Archived: proposal.archived,
		Documents: []Document{}, Availability: []DocumentAvailability{}, Diagnostics: []Diagnostic{},
	}
	v.addSliceDocuments(result, project, proposal, slice)
	result.Incomplete = len(result.Diagnostics) > 0
	return result, nil
}

// Document reads one exact current or historical ledger reference. It accepts
// only known record-document paths, then delegates exact commit/path access to
// ShowReference without substituting another revision or changing its legacy
// raw-document behavior.
func (v *Snapshot) Document(reference Reference) (*Document, error) {
	project, proposal, slice, kind, ok := recordDocumentIdentity(reference.Path)
	if !ok {
		return nil, refuse(
			"reference path "+reference.Path+" is not a ledger record document",
			"supply an exact proposal.md, accepted Slice document, phase report, decision.md, or state.json path",
		)
	}
	contents, err := ShowReference(v.store, reference.Commit, reference.Path)
	if err != nil {
		return nil, err
	}
	document := &Document{
		Kind: kind, Project: project, Proposal: proposal, Slice: slice,
		Archived:  strings.Split(reference.Path, "/")[2] == archiveRoot,
		Reference: Reference{Commit: contents.Commit, Path: contents.Path},
		Contents:  contents.Contents,
		Body:      contents.Contents,
	}
	document.parseMetadata()
	return document, nil
}

func (v *Snapshot) addSliceDocuments(result *DocumentSet, project *projectTree, proposal *proposalTree, slice *sliceTree) {
	subject := project.name + "/" + proposal.name + "/" + slice.name
	base := v.slicePath(project.name, proposal, slice.name)
	for _, name := range []string{"intent.md", "behavior.md"} {
		v.addDocument(result, base+"/"+name, ContractDocumentKind, ScopeSlice, subject)
	}
	for _, name := range []string{"plan.md", "tasks.md"} {
		if slice.files[name] {
			v.addDocument(result, base+"/"+name, ContractDocumentKind, ScopeSlice, subject)
		}
	}
	for _, phase := range []string{ImplementPhase, WatchdogPhase} {
		name := phase + "-report.md"
		kind := ImplementReportDocumentKind
		if phase == WatchdogPhase {
			kind = WatchdogReportDocumentKind
		}
		v.addOptionalDocument(result, base+"/"+name, kind, slice.name, slice.files[name], subject)
	}

	// decision.md is current only while state.json marks it active. Without
	// readable state the decision's membership is unknown, not absent.
	statePath := base + "/state.json"
	stateReference := Reference{Commit: v.Revision, Path: statePath}
	state, err := ShowReference(v.store, stateReference.Commit, stateReference.Path)
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
	v.addOptionalDocument(result, base+"/decision.md", DecisionDocumentKind, slice.name, true, subject)
}

func (v *Snapshot) addOptionalDocument(result *DocumentSet, documentPath string, kind DocumentKind, slice string, member bool, subject string) {
	if !member {
		result.Availability = append(result.Availability, DocumentAvailability{
			Slice: slice, Kind: kind, Status: DocumentAbsent,
		})
		return
	}
	reference := Reference{Commit: v.Revision, Path: documentPath}
	status := DocumentUnavailable
	if v.addDocument(result, documentPath, kind, ScopeSlice, subject) {
		status = DocumentAvailable
	}
	result.Availability = append(result.Availability, DocumentAvailability{
		Slice: slice, Kind: kind, Status: status, Reference: &reference,
	})
}

func (v *Snapshot) addDocument(result *DocumentSet, documentPath string, kind DocumentKind, scope, subject string) bool {
	document, err := v.Document(Reference{Commit: v.Revision, Path: documentPath})
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
