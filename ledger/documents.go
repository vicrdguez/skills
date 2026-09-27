package ledger

import (
	"encoding/json"
	"path"
	"strings"
)

// DocumentKind identifies the ledger record format without interpreting
// Markdown prose.
type DocumentKind string

const (
	ProposalDocumentKind        DocumentKind = "proposal"
	ContractDocumentKind        DocumentKind = "contract"
	ImplementReportDocumentKind DocumentKind = "implement-report"
	WatchdogReportDocumentKind  DocumentKind = "watchdog-report"
	DecisionDocumentKind        DocumentKind = "decision"
	StateDocumentKind           DocumentKind = "state"
)

// LabeledReference is a typed ledger reference extracted from structured
// report or decision metadata. Repeated labels preserve the source order.
type LabeledReference struct {
	Label     string    `json:"label"`
	Reference Reference `json:"reference"`
}

// Document is one exact committed ledger document. Contents are retained even
// when report or decision metadata cannot be interpreted; structured facts
// and their references are available separately when parsing succeeds.
type Document struct {
	Kind        DocumentKind       `json:"kind"`
	Project     string             `json:"project"`
	Proposal    string             `json:"proposal"`
	Slice       string             `json:"slice,omitempty"`
	Reference   Reference          `json:"reference"`
	Contents    string             `json:"contents"`
	Body        string             `json:"body"`
	Report      *Report            `json:"report,omitempty"`
	Decision    *DecisionRecord    `json:"decision,omitempty"`
	References  []LabeledReference `json:"references,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}

// DocumentSet is the document membership of one Proposal or Slice at one
// committed revision. Diagnostics identify unavailable required documents or
// uninterpretable metadata without discarding other readable documents.
type DocumentSet struct {
	Revision    string       `json:"revision"`
	Project     string       `json:"project"`
	Proposal    string       `json:"proposal"`
	Slice       string       `json:"slice,omitempty"`
	Archived    bool         `json:"archived"`
	Documents   []Document   `json:"documents"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Incomplete  bool         `json:"incomplete"`
}

// ProposalDocuments returns the proposal description and the committed
// documents of each member Slice. Membership and current contents are pinned
// to this Snapshot's revision; archived Proposals are resolved at that
// revision and retain their archive paths.
func (v *Snapshot) ProposalDocuments(projectName, proposalName string) (*DocumentSet, error) {
	project, proposal, err := v.proposal(projectName, proposalName)
	if err != nil {
		return nil, err
	}
	result := &DocumentSet{
		Revision: v.Revision, Project: project.name, Proposal: proposal.name,
		Archived: proposal.archived, Documents: []Document{}, Diagnostics: []Diagnostic{},
	}
	path := v.proposalPath(project.name, proposal) + "/proposal.md"
	v.addDocument(result, path, ProposalDocumentKind, ScopeProposal, project.name+"/"+proposal.name)
	for _, slice := range proposal.slices {
		v.addSliceDocuments(result, project, proposal, slice)
	}
	result.Incomplete = len(result.Diagnostics) > 0
	return result, nil
}

// SliceDocuments returns one Slice's accepted content, latest committed
// phase reports, and active Human Decision at this Snapshot's revision.
func (v *Snapshot) SliceDocuments(projectName, item string) (*DocumentSet, error) {
	proposalName, sliceName, found := strings.Cut(item, "/")
	if !found || !ValidRecordName(proposalName) || !ValidRecordName(sliceName) {
		return nil, refuse(
			"Slice reference "+item+" is not a proposal/slice identity",
			"select a Slice listed by its Proposal, such as add-order-cancellation/foundation",
		)
	}
	project, proposal, err := v.proposal(projectName, proposalName)
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
		Documents: []Document{}, Diagnostics: []Diagnostic{},
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
		if !slice.files[name] {
			continue
		}
		kind := ImplementReportDocumentKind
		if phase == WatchdogPhase {
			kind = WatchdogReportDocumentKind
		}
		v.addDocument(result, base+"/"+name, kind, ScopeSlice, subject)
	}

	// decision.md is current only while state.json marks it active. An
	// unreadable state cannot establish current-decision membership.
	statePath := base + "/state.json"
	state, err := ShowReference(v.store, v.Revision, statePath)
	if err != nil {
		result.addDiagnostic(Diagnostic{Scope: ScopeSlice, Subject: subject, Problem: "cannot read state.json to determine current decision: " + err.Error()})
		return
	}
	var record SliceState
	if err := json.Unmarshal([]byte(state.Contents), &record); err != nil {
		result.addDiagnostic(Diagnostic{Scope: ScopeSlice, Subject: subject, Problem: "state.json is malformed, so current decision membership is unknown"})
		return
	}
	if record.Decision {
		v.addDocument(result, base+"/decision.md", DecisionDocumentKind, ScopeSlice, subject)
	}
}

func (v *Snapshot) addDocument(result *DocumentSet, documentPath string, kind DocumentKind, scope, subject string) {
	document, err := v.Document(Reference{Commit: v.Revision, Path: documentPath})
	if err != nil {
		result.addDiagnostic(Diagnostic{Scope: scope, Subject: subject, Problem: "cannot read required document " + documentPath + ": " + err.Error()})
		return
	}
	// The record tree selects membership and the path determines format; this
	// assertion keeps accidental mismatches visible rather than mislabeling a
	// document returned by the exact-reference reader.
	if document.Kind != kind {
		result.addDiagnostic(Diagnostic{Scope: scope, Subject: subject, Problem: "document " + documentPath + " has an unexpected record kind"})
		return
	}
	result.Documents = append(result.Documents, *document)
	for _, diagnostic := range document.Diagnostics {
		result.addDiagnostic(diagnostic)
	}
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
	if !validRecordDocumentPath(value) {
		return "", "", "", "", false
	}
	parts := strings.Split(value, "/")
	project, proposal = parts[1], parts[3]
	if len(parts) == 5 {
		return project, proposal, "", ProposalDocumentKind, true
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
	}
	return project, proposal, slice, kind, kind != ""
}

func validRecordDocumentPath(value string) bool {
	if value == "" || strings.ContainsRune(value, '\\') || path.Clean(value) != value {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) != 5 && len(parts) != 6 || parts[0] != projectsRoot || !ValidRecordName(parts[1]) {
		return false
	}
	if parts[2] != "proposals" && parts[2] != archiveRoot || !ValidRecordName(parts[3]) {
		return false
	}
	if len(parts) == 5 {
		return parts[4] == "proposal.md"
	}
	if !ValidRecordName(parts[4]) {
		return false
	}
	switch parts[5] {
	case "intent.md", "behavior.md", "plan.md", "tasks.md", "implement-report.md", "watchdog-report.md", "decision.md", "state.json":
		return true
	default:
		return false
	}
}
