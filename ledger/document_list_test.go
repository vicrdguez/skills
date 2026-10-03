package ledger_test

import (
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// Missing objects and malformed metadata have the same diagnostics as the
// full documents query. Prose body corruption is diagnosed only on open.
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
	unreadable := map[string]ledger.DocumentKind{}
	for _, entry := range list.Unreadable {
		if entry.Reference.Commit != snapshot.Revision {
			t.Errorf("unreadable %s is not identified at the listed revision: %+v", entry.Reference.Path, entry)
		}
		unreadable[entry.Reference.Path] = entry.Kind
	}
	for path, kind := range map[string]ledger.DocumentKind{
		deliveryContractPath("widgets", "records/lost", "intent.md"):         ledger.ContractDocumentKind,
		deliveryContractPath("widgets", "records/damaged", "behavior.md"):    ledger.ContractDocumentKind,
		deliveryReportPath("widgets", "records/lost", ledger.ImplementPhase): ledger.ImplementReportDocumentKind,
	} {
		if unreadable[path] != kind {
			t.Errorf("unreadable %s identified as %q, want %q", path, unreadable[path], kind)
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

// Prose headers remain listable when their bodies are corrupt. Only an exact
// read discovers that corruption, without substituting another document.
func TestDocumentListDefersProseBodyCorruptionUntilOpen(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "records", "readback", ledger.NeedsHuman, nil, deliveryInitial)
	// A poorly compressible body keeps the damaged checksum beyond the
	// prefix Git inflates when checking only an object's header.
	body := make([]byte, 100_000)
	random := rand.New(rand.NewSource(1))
	for i := range body {
		body[i] = byte('!' + random.Intn(90))
	}
	intentPath := deliveryContractPath("widgets", "records/readback", "intent.md")
	l.addFile(intentPath, string(body))
	l.commitAll("accept records")
	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	blob := deliveryGitOutput(t, l.root, "rev-parse", "HEAD:"+intentPath)
	objectPath := filepath.Join(l.root, ".git", "objects", blob[:2], blob[2:])
	object, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	object[len(object)-1] ^= 1
	if err := os.Chmod(objectPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, object, 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := snapshot.SliceDocumentListAt("widgets", "records/readback", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Documents) != 2 || len(list.Diagnostics) != 0 {
		t.Fatalf("a header-readable contract must remain listed without a body diagnosis: %+v", list)
	}
	for _, entry := range list.Documents {
		document, err := snapshot.Document(entry.Reference)
		if entry.Reference.Path == intentPath {
			if err == nil || document != nil || !strings.Contains(err.Error(), "inflate") {
				t.Errorf("corrupt prose read = %+v, %v; want refusal for body corruption", document, err)
			}
		} else if err != nil || document == nil || !strings.Contains(document.Contents, "behavior of readback") {
			t.Errorf("healthy contract read = %+v, %v", document, err)
		}
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
	got := *list
	got.Unreadable = nil // The documents query reports these only as diagnostics.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("document list differs from the documents query:\nlist: %+v\nwant: %+v", *list, want)
	}
}

// When Git cannot confirm which prose documents exist, the list still offers
// them, each with a diagnostic, and reads no prose text to find out.
func TestDocumentListReadsNoProseWhenItsExistenceCheckFails(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "records", "readback", ledger.NeedsHuman, nil, deliveryInitial)
	initial := l.commitAll("accept records")
	l.addFile(deliveryReportPath("widgets", "records/readback", ledger.WatchdogPhase), formattedWatchdog(t, initial, "records/readback", "healthy review"))
	l.commitAll("record review")
	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin, reads := t.TempDir(), filepath.Join(t.TempDir(), "reads")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  [ \"$arg\" = --batch-check ] && exit 1\n" +
		"  [ \"$arg\" = show ] && echo \"$*\" >> \"$SKL_TEST_READS\"\n" +
		"done\n" +
		"exec \"$SKL_TEST_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SKL_TEST_REAL_GIT", realGit)
	t.Setenv("SKL_TEST_READS", reads)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	list, err := snapshot.SliceDocumentListAt("widgets", "records/readback", false)
	if err != nil {
		t.Fatal(err)
	}
	if logged, err := os.ReadFile(reads); err == nil && strings.Contains(string(logged), ".md") {
		t.Errorf("listing read document text:\n%s", logged)
	}
	listed := map[string][]ledger.Diagnostic{}
	for _, entry := range list.Documents {
		listed[path.Base(entry.Reference.Path)] = entry.Diagnostics
	}
	for _, name := range []string{"intent.md", "behavior.md"} {
		diagnostics, ok := listed[name]
		if !ok {
			t.Errorf("%s is not listed: %+v", name, list.Documents)
			continue
		}
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Problem, "cannot confirm document "+deliveryContractPath("widgets", "records/readback", name)) {
			t.Errorf("%s diagnostics = %+v, want one naming the failed check", name, diagnostics)
		}
	}
	if diagnostics, ok := listed["watchdog-report.md"]; !ok || len(diagnostics) != 0 {
		t.Errorf("healthy watchdog report listed = %t with diagnostics %+v", ok, diagnostics)
	}
}
