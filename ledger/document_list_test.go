package ledger_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// A document list must offer exactly what the full documents query offers,
// read by exact reads of every document, while reading no document text.
func TestDocumentListsMatchTheDocumentsQueryAcrossDamagedRecords(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	for _, slice := range []string{"readback", "damaged", "lost", "nostate"} {
		l.addSlice("widgets", "records", slice, ledger.NeedsHuman, nil, deliveryInitial)
		l.writeStateValue("widgets", "records", slice, ledger.SliceState{State: ledger.NeedsHuman, Title: slice, Branch: slice, Decision: true})
	}
	l.addSlice("widgets", "legacy", "old", ledger.Merged, nil, deliveryInitial)
	initial := l.commitAll("accept records")

	// readback: healthy report, Human Decision and optional plan.
	l.addFile(deliveryContractPath("widgets", "records/readback", "plan.md"), "plan of readback\n")
	l.addFile(deliveryReportPath("widgets", "records/readback", ledger.WatchdogPhase), formattedWatchdog(t, initial, "records/readback", "healthy review"))
	decision, err := ledger.FormatDecision(ledger.DecisionRecord{
		Schema: 1, Project: "widgets", Item: "records/readback", Route: ledger.RouteImplement,
		AnsweredRequest: ledger.Reference{Commit: initial, Path: deliveryStatePath("widgets", "records/readback")},
	}, "human direction\n")
	if err != nil {
		t.Fatal(err)
	}
	l.addFile(deliveryDecisionPath("widgets", "records/readback"), string(decision))
	// damaged: uninterpretable metadata and a missing required document.
	l.addFile(deliveryReportPath("widgets", "records/damaged", ledger.WatchdogPhase), "---\nschema: 88\n---\nreport bytes remain readable\n")
	l.addFile(deliveryDecisionPath("widgets", "records/damaged"), "---\nschema: not-an-integer\n---\ndecision bytes remain readable\n")
	if err := os.Remove(filepath.Join(l.root, filepath.FromSlash(deliveryContractPath("widgets", "records/damaged", "behavior.md")))); err != nil {
		t.Fatal(err)
	}
	// lost: an active decision without its record, and unreadable objects.
	l.addFile(deliveryReportPath("widgets", "records/lost", ledger.ImplementPhase), "implementation report of lost\n")
	// nostate: decision membership cannot be determined.
	if err := os.Remove(filepath.Join(l.root, filepath.FromSlash(deliveryStatePath("widgets", "records/nostate")))); err != nil {
		t.Fatal(err)
	}
	l.addFile("projects/widgets/proposals/records/Bad Name/intent.md", "invalid member\n")
	l.addFile(deliveryReportPath("widgets", "legacy/old", ledger.ImplementPhase), "archived implementation report\n")
	l.commitAll("record documents")
	if err := os.MkdirAll(filepath.Join(l.root, "projects/widgets/archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/legacy", "projects/widgets/archive/legacy")
	l.commitAll("archive legacy")
	for _, path := range []string{
		deliveryReportPath("widgets", "records/lost", ledger.ImplementPhase),
		deliveryContractPath("widgets", "records/lost", "intent.md"),
	} {
		blob := deliveryGitOutput(t, l.root, "rev-parse", "HEAD:"+path)
		if err := os.Remove(filepath.Join(l.root, ".git", "objects", blob[:2], blob[2:])); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	before := browseObservable(t, l)
	list, err := snapshot.ProposalDocumentListAt("widgets", "records", false)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snapshot.ProposalDocumentsAt("widgets", "records", false)
	if err != nil {
		t.Fatal(err)
	}
	sameAsDocuments(t, list, set)
	statuses := map[string]ledger.DocumentStatus{}
	for _, availability := range list.Availability {
		statuses[availability.Slice+" "+string(availability.Kind)] = availability.Status
	}
	for key, want := range map[string]ledger.DocumentStatus{
		"readback decision": ledger.DocumentAvailable, "damaged watchdog-report": ledger.DocumentAvailable,
		"lost implement-report": ledger.DocumentUnavailable, "lost decision": ledger.DocumentUnavailable,
		"nostate decision": ledger.DocumentUnknown, "readback implement-report": ledger.DocumentAbsent,
	} {
		if statuses[key] != want {
			t.Errorf("%s availability = %q, want %q", key, statuses[key], want)
		}
	}
	for _, entry := range list.Documents {
		if entry.Reference == (ledger.Reference{Commit: snapshot.Revision, Path: deliveryContractPath("widgets", "records/lost", "intent.md")}) {
			t.Errorf("unreadable intent.md listed as readable: %+v", entry)
		}
	}

	for _, slice := range []string{"readback", "damaged", "lost", "nostate"} {
		list, err := snapshot.SliceDocumentListAt("widgets", "records/"+slice, false)
		if err != nil {
			t.Fatal(err)
		}
		set, err := snapshot.SliceDocumentsAt("widgets", "records/"+slice, false)
		if err != nil {
			t.Fatal(err)
		}
		sameAsDocuments(t, list, set)
	}
	archivedList, err := snapshot.ProposalDocumentListAt("widgets", "legacy", true)
	if err != nil {
		t.Fatal(err)
	}
	archivedSet, err := snapshot.ProposalDocumentsAt("widgets", "legacy", true)
	if err != nil {
		t.Fatal(err)
	}
	sameAsDocuments(t, archivedList, archivedSet)
	if _, err := snapshot.SliceDocumentListAt("widgets", "records/missing", false); err == nil {
		t.Fatal("listing an unknown Slice was not refused")
	}
	if after := browseObservable(t, l); after != before {
		t.Fatalf("document listing changed ledger refs or working tree:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// sameAsDocuments requires the list to offer set's entries, availability and
// diagnostics, in the same order.
func sameAsDocuments(t *testing.T, list *ledger.DocumentList, set *ledger.DocumentSet) {
	t.Helper()
	want := ledger.DocumentList{
		Revision: set.Revision, Project: set.Project, Proposal: set.Proposal, Slice: set.Slice, Archived: set.Archived,
		Documents: []ledger.DocumentEntry{}, Availability: set.Availability, Diagnostics: set.Diagnostics,
	}
	for _, document := range set.Documents {
		want.Documents = append(want.Documents, ledger.DocumentEntry{
			Kind: document.Kind, Project: document.Project, Proposal: document.Proposal, Slice: document.Slice,
			Reference: document.Reference, Report: document.Report, Decision: document.Decision, Diagnostics: document.Diagnostics,
		})
	}
	if !reflect.DeepEqual(*list, want) {
		t.Fatalf("document list differs from the documents query:\nlist: %+v\nwant: %+v", *list, want)
	}
}
