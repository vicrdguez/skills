package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestSnapshotDocumentsResolveCurrentAndHistoricalReportReferences(t *testing.T) {
	const item = "browse-records/readback"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "browse-records", "readback", ledger.AwaitingReview, nil, deliveryInitial)
	l.writeStateValue("widgets", "browse-records", "readback", ledger.SliceState{
		State: ledger.AwaitingReview, Title: "Readback", Branch: "readback", Decision: true,
		Claim: &ledger.Claim{Phase: ledger.WatchdogPhase, Basis: deliveryRefID},
	})
	initial := l.commitAll("accept browse records")

	watchdogPath := deliveryReportPath("widgets", item, ledger.WatchdogPhase)
	watchdog := formattedWatchdog(t, initial, item, "older watchdog evidence")
	l.addFile(watchdogPath, watchdog)
	oldWatchdogCommit := l.commitAll("record first watchdog report")
	oldWatchdogReference := ledger.Reference{Commit: oldWatchdogCommit, Path: watchdogPath}

	decision, err := ledger.FormatDecision(ledger.DecisionRecord{
		Schema: 1, Project: "widgets", Item: item,
		AnsweredRequest: oldWatchdogReference, Route: ledger.RouteImplement,
	}, "human direction follows the recorded request\n")
	if err != nil {
		t.Fatalf("format current decision: %v", err)
	}
	l.addFile(deliveryDecisionPath("widgets", item), string(decision))

	implementPath := deliveryReportPath("widgets", item, ledger.ImplementPhase)
	implement, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
		Schema: 1, Outcome: "awaiting_review",
		Source: ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget},
		Ledger: ledger.ReportInputs{
			Claim:    ledger.Reference{Commit: initial, Path: deliveryStatePath("widgets", item)},
			Contract: []ledger.Reference{{Commit: initial, Path: deliveryContractPath("widgets", item, "behavior.md")}},
			Watchdog: &oldWatchdogReference,
		},
	}, "implementation retains an exact older review reference\n")
	if err != nil {
		t.Fatalf("format implementation report: %v", err)
	}
	l.addFile(implementPath, string(implement))
	latestWatchdog := formattedWatchdog(t, initial, item, "current watchdog evidence")
	l.addFile(watchdogPath, latestWatchdog)
	latest := l.commitAll("record latest phase results")

	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	documents, err := snapshot.SliceDocuments("widgets", item)
	if err != nil {
		t.Fatalf("query current Slice documents: %v", err)
	}
	current := documentByKind(t, documents, ledger.WatchdogReportDocumentKind)
	if current.Reference != (ledger.Reference{Commit: latest, Path: watchdogPath}) || current.Contents != latestWatchdog {
		t.Fatalf("current watchdog document = %+v, want latest exact document %s:%s", current, latest, watchdogPath)
	}
	implementation := documentByKind(t, documents, ledger.ImplementReportDocumentKind)
	if implementation.Report == nil || implementation.Report.Outcome != "awaiting_review" {
		t.Fatalf("parsed implementation facts = %+v", implementation.Report)
	}
	if implementation.Contents != string(implement) || implementation.Body != "implementation retains an exact older review reference\n" {
		t.Fatalf("implementation raw contents/body = %q / %q", implementation.Contents, implementation.Body)
	}
	if len(implementation.References) != 3 || implementation.References[0] != (ledger.LabeledReference{
		Label: "claim", Reference: ledger.Reference{Commit: initial, Path: deliveryStatePath("widgets", item)},
	}) || implementation.References[2] != (ledger.LabeledReference{Label: "watchdog", Reference: oldWatchdogReference}) {
		t.Fatalf("typed implementation references = %+v", implementation.References)
	}
	currentDecision := documentByKind(t, documents, ledger.DecisionDocumentKind)
	if currentDecision.Decision == nil || currentDecision.Decision.Route != ledger.RouteImplement || len(currentDecision.References) != 1 || currentDecision.References[0] != (ledger.LabeledReference{Label: "answered_request", Reference: oldWatchdogReference}) {
		t.Fatalf("structured current-decision facts = %+v", currentDecision)
	}
	if currentDecision.Contents != string(decision) || currentDecision.Body != "human direction follows the recorded request\n" {
		t.Fatalf("decision raw contents/body = %q / %q", currentDecision.Contents, currentDecision.Body)
	}
	old, err := snapshot.Document(oldWatchdogReference)
	if err != nil {
		t.Fatalf("read exact older watchdog reference: %v", err)
	}
	if old.Reference != oldWatchdogReference || old.Contents != watchdog || old.Report == nil || old.Report.Round != 1 {
		t.Fatalf("historical watchdog document = %+v", old)
	}

	slice, err := snapshot.Slice("widgets", item)
	if err != nil {
		t.Fatal(err)
	}
	if slice.Claim == nil || slice.Claim.Reference != (ledger.Reference{Commit: latest, Path: deliveryStatePath("widgets", item)}) {
		t.Fatalf("Claim state reference = %+v, want snapshot-pinned state.json", slice.Claim)
	}
}

func TestSnapshotDocumentsResolveArchivedProposalAndHistoricalPath(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "legacy-proposal", "finished", ledger.Merged, nil, deliveryInitial)
	accepted := l.commitAll("accept proposal")
	proposalPath := "projects/widgets/proposals/legacy-proposal/proposal.md"
	proposalContents := deliveryGitShow(t, l.root, accepted, proposalPath)
	if err := os.MkdirAll(filepath.Join(l.root, "projects", "widgets", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/legacy-proposal", "projects/widgets/archive/legacy-proposal")
	archived := l.commitAll("archive proposal")

	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	set, err := snapshot.ProposalDocuments("widgets", "legacy-proposal")
	if err != nil {
		t.Fatalf("query archived Proposal: %v", err)
	}
	if !set.Archived || set.Revision != archived || set.Incomplete {
		t.Fatalf("archived document set = %+v, want complete revision %s and Archived", set, archived)
	}
	for _, document := range set.Documents {
		if document.Kind == ledger.ImplementReportDocumentKind || document.Kind == ledger.WatchdogReportDocumentKind || document.Kind == ledger.DecisionDocumentKind {
			t.Fatalf("absent optional evidence was represented as a document: %+v", document)
		}
	}
	proposal := documentByKind(t, set, ledger.ProposalDocumentKind)
	wantCurrentPath := "projects/widgets/archive/legacy-proposal/proposal.md"
	if proposal.Reference != (ledger.Reference{Commit: archived, Path: wantCurrentPath}) || proposal.Contents != proposalContents || proposal.Body != proposalContents {
		t.Fatalf("archived proposal description = %+v", proposal)
	}
	old, err := snapshot.Document(ledger.Reference{Commit: accepted, Path: proposalPath})
	if err != nil {
		t.Fatalf("read proposal from before archive move: %v", err)
	}
	if old.Reference.Commit != accepted || old.Reference.Path != proposalPath || old.Contents != proposalContents {
		t.Fatalf("pre-archive exact proposal identity changed: %+v", old)
	}
}

func TestSnapshotDocumentsSelectActiveOrArchivedSameName(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "cancel", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addFile("projects/widgets/proposals/orders/proposal.md", "active proposal description\n")
	l.addFile(deliveryContractPath("widgets", "orders/cancel", "intent.md"), "active Slice intent\n")
	activeRevision := l.commitAll("accept active proposal")

	if err := os.MkdirAll(filepath.Join(l.root, "projects", "widgets", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/orders", "projects/widgets/archive/orders")
	l.commitAll("archive proposal")

	l.addSlice("widgets", "orders", "cancel", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addFile("projects/widgets/proposals/orders/proposal.md", "new active proposal description\n")
	l.addFile(deliveryContractPath("widgets", "orders/cancel", "intent.md"), "new active Slice intent\n")
	currentRevision := l.commitAll("record replacement active proposal")
	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if currentRevision == activeRevision {
		t.Fatal("fixture did not advance after adding the active replacement")
	}

	for _, query := range []struct {
		name     string
		wantPath string
		wantText string
		get      func(bool) (*ledger.DocumentSet, error)
	}{
		{
			name: "Proposal", wantPath: "projects/widgets/proposals/orders/proposal.md", wantText: "new active proposal description",
			get: func(archived bool) (*ledger.DocumentSet, error) {
				return snapshot.ProposalDocumentsAt("widgets", "orders", archived)
			},
		},
		{
			name: "Slice", wantPath: "projects/widgets/proposals/orders/cancel/intent.md", wantText: "new active Slice intent",
			get: func(archived bool) (*ledger.DocumentSet, error) {
				return snapshot.SliceDocumentsAt("widgets", "orders/cancel", archived)
			},
		},
	} {
		t.Run(query.name, func(t *testing.T) {
			for _, archived := range []bool{false, true} {
				set, err := query.get(archived)
				if err != nil {
					t.Fatalf("query archived=%t documents: %v", archived, err)
				}
				wantPath, wantText := query.wantPath, query.wantText
				if archived {
					wantPath = strings.Replace(wantPath, "/proposals/", "/archive/", 1)
					if query.name == "Proposal" {
						wantText = "active proposal description"
					} else {
						wantText = "active Slice intent"
					}
				}
				if set.Archived != archived {
					t.Fatalf("archived=%t returned set %+v", archived, set)
				}
				kind := ledger.ContractDocumentKind
				if query.name == "Proposal" {
					kind = ledger.ProposalDocumentKind
				}
				document := documentByKind(t, set, kind)
				if document.Reference != (ledger.Reference{Commit: currentRevision, Path: wantPath}) || !strings.Contains(document.Contents, wantText) {
					t.Fatalf("archived=%t selected document %+v, want %s containing %q", archived, document, wantPath, wantText)
				}
			}
		})
	}

	// The long-standing APIs keep active-first fallback behavior, while the
	// location-specific queries must honor their explicit archived scope.
	defaultProposal, err := snapshot.ProposalDocuments("widgets", "orders")
	if err != nil || defaultProposal.Archived {
		t.Fatalf("default Proposal documents = %+v, %v; want active", defaultProposal, err)
	}
	defaultSlice, err := snapshot.SliceDocuments("widgets", "orders/cancel")
	if err != nil || defaultSlice.Archived {
		t.Fatalf("default Slice documents = %+v, %v; want active", defaultSlice, err)
	}
}

func TestSnapshotDocumentsPreserveMalformedMetadataAndIsolateMissingContent(t *testing.T) {
	const item = "browse-records/readback"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "browse-records", "readback", ledger.NeedsHuman, nil, deliveryInitial)
	l.writeStateValue("widgets", "browse-records", "readback", ledger.SliceState{
		State: ledger.NeedsHuman, Title: "Readback", Branch: "readback", Decision: true,
	})
	l.addSlice("widgets", "browse-records", "healthy", ledger.ReadyForImplementation, nil, deliveryInitial)
	badReport := "---\nschema: 88\noutcome: pass\n---\nreport bytes remain readable\n"
	badDecision := "---\nschema: not-an-integer\n---\ndecision bytes remain readable\n"
	l.addFile(deliveryReportPath("widgets", item, ledger.WatchdogPhase), badReport)
	l.addFile(deliveryDecisionPath("widgets", item), badDecision)
	missingRequired := deliveryContractPath("widgets", item, "behavior.md")
	if err := os.Remove(filepath.Join(l.root, filepath.FromSlash(missingRequired))); err != nil {
		t.Fatal(err)
	}
	l.commitAll("record damaged and healthy documents")

	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	set, err := snapshot.ProposalDocuments("widgets", "browse-records")
	if err != nil {
		t.Fatalf("query Proposal documents: %v", err)
	}
	if !set.Incomplete {
		t.Fatalf("document summary did not disclose damaged records: %+v", set)
	}
	readback, err := snapshot.SliceDocuments("widgets", item)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDocument(readback, ledger.ContractDocumentKind, "intent.md") || hasDocument(readback, ledger.ContractDocumentKind, "behavior.md") {
		t.Fatalf("required-content membership did not preserve the readable document and missing behavior: %+v", readback.Documents)
	}
	report := documentByKind(t, set, ledger.WatchdogReportDocumentKind)
	if report.Contents != badReport || report.Body != report.Contents || report.Report != nil || len(report.Diagnostics) == 0 {
		t.Fatalf("malformed report lost its readable content or diagnostic: %+v", report)
	}
	decision := documentByKind(t, set, ledger.DecisionDocumentKind)
	if decision.Contents != badDecision || decision.Body != decision.Contents || decision.Decision != nil || len(decision.Diagnostics) == 0 {
		t.Fatalf("malformed decision lost its readable content or diagnostic: %+v", decision)
	}
	if !hasDiagnosticSubject(set.Diagnostics, "widgets/browse-records/readback") || hasDiagnosticSubject(set.Diagnostics, "widgets/browse-records/healthy") {
		t.Fatalf("document diagnostics must scope damage without marking a healthy sibling incomplete: %+v", set.Diagnostics)
	}
	missingBehaviorDiagnosed := false
	for _, diagnostic := range set.Diagnostics {
		missingBehaviorDiagnosed = missingBehaviorDiagnosed || strings.Contains(diagnostic.Problem, "behavior.md")
	}
	if !missingBehaviorDiagnosed {
		t.Fatalf("missing required accepted content was not diagnosed: %+v", set.Diagnostics)
	}

	sliceDocuments, err := snapshot.SliceDocuments("widgets", "browse-records/healthy")
	if err != nil {
		t.Fatal(err)
	}
	if sliceDocuments.Incomplete || len(sliceDocuments.Diagnostics) != 0 {
		t.Fatalf("unaffected Slice should remain complete: %+v", sliceDocuments)
	}
}

func TestSnapshotExactDocumentMissIsNotSubstitutedAndReadsAreReadOnly(t *testing.T) {
	const item = "browse-records/readback"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "browse-records", "readback", ledger.ReadyForImplementation, nil, deliveryInitial)
	old := l.commitAll("accept without reports")
	watchdogPath := deliveryReportPath("widgets", item, ledger.WatchdogPhase)
	l.addFile(watchdogPath, "current raw watchdog content\n")
	latest := l.commitAll("add watchdog")
	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.Document(ledger.Reference{Commit: old, Path: watchdogPath}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing exact historical reference error = %v, want no substitution", err)
	}
	if _, err := snapshot.Document(ledger.Reference{Commit: latest, Path: "projects/widgets/proposals/browse-records/readback/../state.json"}); err == nil {
		t.Fatal("non-canonical record path was accepted")
	}
	l.addFile(watchdogPath, "newer committed replacement must not be returned\n")
	newer := l.commitAll("advance documents after snapshot")
	if newer == latest {
		t.Fatal("fixture did not advance the ledger revision")
	}
	l.addFile(watchdogPath, "uncommitted replacement must not be returned\n")
	readOnlyBefore := browseObservable(t, l)
	set, err := snapshot.SliceDocuments("widgets", item)
	if err != nil {
		t.Fatalf("read documents through pinned Snapshot: %v", err)
	}
	current := documentByKind(t, set, ledger.WatchdogReportDocumentKind)
	if set.Revision != latest || current.Reference.Commit != latest || current.Contents != "current raw watchdog content\n" {
		t.Fatalf("query mixed revisions or read working-tree content: %+v", current)
	}
	if after := browseObservable(t, l); after != readOnlyBefore {
		t.Fatalf("document query changed ledger refs or working tree:\nbefore:\n%s\nafter:\n%s", readOnlyBefore, after)
	}
}

func formattedWatchdog(t *testing.T, claimCommit, item, body string) string {
	t.Helper()
	path := deliveryReportPath("widgets", item, ledger.WatchdogPhase)
	implement := ledger.Reference{Commit: claimCommit, Path: deliveryReportPath("widgets", item, ledger.ImplementPhase)}
	encoded, err := ledger.FormatReport(ledger.WatchdogPhase, ledger.Report{
		Schema: 1, Outcome: "pass", Round: 1,
		Source: ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget, Reviewed: deliveryHead},
		Ledger: ledger.ReportInputs{
			Claim:     ledger.Reference{Commit: claimCommit, Path: deliveryStatePath("widgets", item)},
			Contract:  []ledger.Reference{{Commit: claimCommit, Path: deliveryContractPath("widgets", item, "behavior.md")}},
			Implement: &implement,
		},
	}, body+"\n")
	if err != nil {
		t.Fatalf("format watchdog report %s: %v", path, err)
	}
	return string(encoded)
}

func documentByKind(t *testing.T, set *ledger.DocumentSet, kind ledger.DocumentKind) ledger.Document {
	t.Helper()
	for _, document := range set.Documents {
		if document.Kind == kind {
			return document
		}
	}
	t.Fatalf("no %s in documents: %+v", kind, set.Documents)
	return ledger.Document{}
}

func hasDocument(set *ledger.DocumentSet, kind ledger.DocumentKind, name string) bool {
	for _, document := range set.Documents {
		if document.Kind == kind && strings.HasSuffix(document.Reference.Path, "/"+name) {
			return true
		}
	}
	return false
}

func hasDiagnosticSubject(diagnostics []ledger.Diagnostic, subject string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Subject == subject {
			return true
		}
	}
	return false
}
