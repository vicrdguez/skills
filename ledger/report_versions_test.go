package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestReportVersionsTrackContentAcrossArchiveWithoutRewindingCurrentFacts(t *testing.T) {
	const item = "records/reader"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "records", "reader", ledger.Merged, nil, deliveryInitial)
	path := deliveryReportPath("widgets", item, ledger.ImplementPhase)
	bad := "---\nschema: 88\noutcome: pass\n---\nunsupported but readable\n"
	l.addFile(path, bad)
	a := l.commitAll("report A")
	l.addFile("projects/widgets/project.json", `{"repository":"acme/widgets","note":"unrelated"}`)
	l.commitAll("unrelated ledger record")
	bContents := "---\nschema: 88\noutcome: pass\n---\nchanged body\n"
	l.addFile(path, bContents)
	b := l.commitAll("report B")
	l.addFile(path, bad)
	c := l.commitAll("return to report A")
	if err := os.MkdirAll(filepath.Join(l.root, "projects/widgets/archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/records", "projects/widgets/archive/records")
	l.commitAll("archive without report change")
	archivedPath := strings.Replace(path, "/proposals/", "/archive/", 1)
	l.addFile(archivedPath, "---\nschema: 89\n---\nnew archive content\n")
	d := l.commitAll("archive report changes")

	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	versions, err := snapshot.Versions("widgets", item, ledger.ImplementPhase)
	if err != nil {
		t.Fatal(err)
	}
	want := []ledger.Reference{{Commit: d, Path: archivedPath}, {Commit: c, Path: path}, {Commit: b, Path: path}, {Commit: a, Path: path}}
	if len(versions.Versions) != len(want) || versions.Incomplete {
		t.Fatalf("archive-aware versions = %+v, want %v", versions, want)
	}
	for index, reference := range want {
		version := versions.Versions[index]
		if version.Reference != reference || version.Report != nil || len(version.Diagnostics) == 0 {
			t.Errorf("version %d = %+v, want exact reference and unsupported metadata diagnostic", index, version)
		}
		read, err := snapshot.Document(version.Reference)
		if err != nil || read.Reference != reference {
			t.Fatalf("exact selected version %d = %+v, %v", index, read, err)
		}
		contents := []string{"new archive content", "unsupported but readable", "changed body", "unsupported but readable"}
		if !strings.Contains(read.Contents, contents[index]) {
			t.Errorf("selected version %d = %q, want %q", index, read.Contents, contents[index])
		}
	}
	facts, err := snapshot.Slice("widgets", item)
	if err != nil || facts.Lifecycle != ledger.Merged || !facts.Archived {
		t.Fatalf("current facts changed after historical selection: %+v, %v", facts, err)
	}
}

func TestReportVersionsDiscloseShallowHistoryAndKeepSameNameRecordsSeparate(t *testing.T) {
	const item = "records/reader"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "records", "reader", ledger.Merged, nil, deliveryInitial)
	path := deliveryReportPath("widgets", item, ledger.ImplementPhase)
	l.addFile(path, "first incarnation\n")
	old := l.commitAll("first incarnation")
	if err := os.MkdirAll(filepath.Join(l.root, "projects/widgets/archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/records", "projects/widgets/archive/records")
	l.commitAll("archive first incarnation")
	l.addSlice("widgets", "records", "reader", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addFile(path, "new incarnation\n")
	latest := l.commitAll("new active incarnation")
	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	active, err := snapshot.Versions("widgets", item, ledger.ImplementPhase)
	if err != nil || len(active.Versions) != 1 || active.Versions[0].Reference.Commit != latest {
		t.Fatalf("new active record inherited archived versions: %+v, %v", active, err)
	}
	archived, err := snapshot.VersionsAt("widgets", item, ledger.ImplementPhase, true)
	if err != nil || len(archived.Versions) != 1 || archived.Versions[0].Reference.Commit != old {
		t.Fatalf("archived record inherited new active version: %+v, %v", archived, err)
	}

	shallowRoot := filepath.Join(t.TempDir(), "shallow")
	deliveryGit(t, l.root, "clone", "-q", "--depth", "1", "file://"+l.root, shallowRoot)
	shallowStore, err := ledger.Open(shallowRoot)
	if err != nil {
		t.Fatal(err)
	}
	shallowSnapshot, err := shallowStore.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	limited, err := shallowSnapshot.VersionsAt("widgets", item, ledger.ImplementPhase, true)
	if err != nil || !limited.Incomplete || len(limited.Diagnostics) == 0 {
		t.Fatalf("shallow history presented as complete: %+v, %v", limited, err)
	}
}

func TestReportVersionsPreserveMetadataChangesAndDiagnoseMissingObjects(t *testing.T) {
	const item = "records/reader"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "records", "reader", ledger.AwaitingReview, nil, deliveryInitial)
	initial := l.commitAll("accepted")
	path := deliveryReportPath("widgets", item, ledger.WatchdogPhase)
	first := formattedWatchdog(t, initial, item, "same report body")
	l.addFile(path, first)
	a := l.commitAll("first watchdog")
	second := strings.Replace(first, "round: 1", "round: 2", 1)
	if second == first {
		t.Fatal("fixture failed to change report metadata")
	}
	l.addFile(path, second)
	b := l.commitAll("metadata-only version")
	blob := deliveryGitOutput(t, l.root, "rev-parse", a+":"+path)
	if err := os.Remove(filepath.Join(l.root, ".git", "objects", blob[:2], blob[2:])); err != nil {
		t.Fatal(err)
	}
	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	versions, err := snapshot.Versions("widgets", item, ledger.WatchdogPhase)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions.Versions) != 2 || versions.Versions[0].Reference.Commit != b || versions.Versions[0].Report == nil || versions.Versions[0].Report.Round != 2 || versions.Versions[1].Reference.Commit != a || len(versions.Versions[1].Diagnostics) == 0 || !versions.Incomplete {
		t.Fatalf("metadata-only and unavailable version = %+v", versions)
	}
}

func TestReportVersionsKeepLabelsForMergedChangesAcrossAnUnchangedArchiveMove(t *testing.T) {
	const item = "records/reader"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "records", "reader", ledger.Merged, nil, deliveryInitial)
	initial := l.commitAll("accepted")
	path := deliveryReportPath("widgets", item, ledger.WatchdogPhase)
	first := formattedWatchdog(t, initial, item, "first review")
	l.addFile(path, first)
	a := l.commitAll("first watchdog")

	// The second version reaches the ledger through a merge; its first
	// parent holds the first version.
	deliveryGit(t, l.root, "checkout", "-q", "-b", "side")
	l.addFile(path, strings.Replace(strings.Replace(first, "round: 1", "round: 2", 1), "outcome: pass", "outcome: rework", 1))
	l.commitAll("second watchdog on a side line")
	deliveryGit(t, l.root, "checkout", "-q", "main")
	l.addProject("widgets", "acme/widgets-renamed")
	l.commitAll("unrelated ledger record")
	deliveryGit(t, l.root, "merge", "-q", "--no-ff", "--no-edit", "side")
	merge := l.head()
	if err := os.MkdirAll(filepath.Join(l.root, "projects/widgets/archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/records", "projects/widgets/archive/records")
	l.commitAll("archive without report change")

	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	versions, err := snapshot.Versions("widgets", item, ledger.WatchdogPhase)
	if err != nil {
		t.Fatal(err)
	}
	type label struct {
		reference ledger.Reference
		outcome   string
		round     uint64
	}
	want := []label{{ledger.Reference{Commit: merge, Path: path}, "rework", 2}, {ledger.Reference{Commit: a, Path: path}, "pass", 1}}
	if !versions.Archived || versions.Incomplete || len(versions.Diagnostics) != 0 || len(versions.Versions) != len(want) {
		t.Fatalf("versions across merge and archive = %+v, want %v", versions, want)
	}
	for index, version := range versions.Versions {
		if version.Report == nil || (label{version.Reference, version.Report.Outcome, version.Report.Round}) != want[index] {
			t.Errorf("version %d = %+v (report %+v), want %+v", index, version, version.Report, want[index])
		}
	}
}

func TestReportVersionsDiscloseAnUnreadableHistoricalTreeAndKeepNewerVersions(t *testing.T) {
	const item = "records/reader"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "records", "reader", ledger.Merged, nil, deliveryInitial)
	l.commitAll("first incarnation")
	if err := os.MkdirAll(filepath.Join(l.root, "projects/widgets/archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/records", "projects/widgets/archive/records")
	archived := l.commitAll("archive first incarnation")
	deliveryGit(t, l.root, "rm", "-r", "-q", "projects/widgets/archive/records")
	l.commitAll("remove the archived incarnation")
	l.addSlice("widgets", "records", "reader", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("new active incarnation")
	path := deliveryReportPath("widgets", item, ledger.ImplementPhase)
	l.addFile(path, "first report\n")
	first := l.commitAll("first report")
	l.addFile(path, "second report\n")
	second := l.commitAll("second report")
	tree := deliveryGitOutput(t, l.root, "rev-parse", archived+":projects/widgets/archive/records")
	if err := os.Remove(filepath.Join(l.root, ".git", "objects", tree[:2], tree[2:])); err != nil {
		t.Fatal(err)
	}

	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	versions, err := snapshot.Versions("widgets", item, ledger.ImplementPhase)
	if err != nil {
		t.Fatal(err)
	}
	if !versions.Incomplete || len(versions.Diagnostics) == 0 || !strings.HasPrefix(versions.Diagnostics[0].Problem, "cannot read local report history: ") {
		t.Fatalf("unreadable history was not disclosed: %+v", versions)
	}
	want := []ledger.Reference{{Commit: second, Path: path}, {Commit: first, Path: path}}
	if len(versions.Versions) != len(want) || versions.Versions[0].Reference != want[0] || versions.Versions[1].Reference != want[1] {
		t.Fatalf("versions newer than the unreadable history = %+v, want %v", versions.Versions, want)
	}
}
