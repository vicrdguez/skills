package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestCurrentReportsShareCommittedIdentityAndIgnoreWorkingTree(t *testing.T) {
	const item = "inspect-private-reports/readback"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "inspect-private-reports", "readback", ledger.ReadyForMerge, nil, deliveryInitial)
	implement := "---\r\nschema: 37\r\n---\r\nopaque implementation bytes\r\n"
	watchdog := "not schema-1 YAML\nW1: authored review evidence\n"
	l.addFile(deliveryReportPath("widgets", item, ledger.ImplementPhase), implement)
	l.addFile(deliveryReportPath("widgets", item, ledger.WatchdogPhase), watchdog)
	committed := l.commitAll("commit private reports")

	readback, err := ledger.ShowItem(l.store(), deliveryWidgets(), item)
	if err != nil {
		t.Fatalf("ShowItem at Ready for Merge without a Claim: %v", err)
	}
	assertCurrentReportAvailability(t, readback.Reports, committed, item)
	for _, phase := range []struct {
		name string
		body string
	}{{ledger.ImplementPhase, implement}, {ledger.WatchdogPhase, watchdog}} {
		document, err := ledger.ShowReport(l.store(), deliveryWidgets(), item, phase.name)
		if err != nil {
			t.Fatalf("ShowReport %s: %v", phase.name, err)
		}
		assertReportDocument(t, document, committed, item, phase.name, phase.body)
	}

	l.addFile("unrelated.txt", "unrelated committed change\n")
	newHead := l.commitAll("unrelated commit")
	readback, err = ledger.ShowItem(l.store(), deliveryWidgets(), item)
	if err != nil {
		t.Fatalf("ShowItem after unrelated commit: %v", err)
	}
	assertCurrentReportAvailability(t, readback.Reports, newHead, item)

	dirtyReports := []struct {
		name      string
		committed string
		dirty     string
	}{{ledger.ImplementPhase, implement, "dirty implement report\n"}, {ledger.WatchdogPhase, watchdog, "dirty watchdog report\n"}}
	for _, phase := range dirtyReports {
		l.addFile(deliveryReportPath("widgets", item, phase.name), phase.dirty)
	}
	readback, err = ledger.ShowItem(l.store(), deliveryWidgets(), item)
	if err != nil {
		t.Fatalf("ShowItem with dirty report files: %v", err)
	}
	assertCurrentReportAvailability(t, readback.Reports, newHead, item)
	for _, phase := range dirtyReports {
		path := deliveryReportPath("widgets", item, phase.name)
		document, err := ledger.ShowReport(l.store(), deliveryWidgets(), item, phase.name)
		if err != nil {
			t.Fatalf("ShowReport %s with dirty working file: %v", phase.name, err)
		}
		assertReportDocument(t, document, newHead, item, phase.name, phase.committed)
		working, err := os.ReadFile(filepath.Join(l.root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read dirty working file %s: %v", path, err)
		}
		if string(working) != phase.dirty {
			t.Errorf("ShowReport changed dirty working file %s: got %q", path, working)
		}
	}
}

func TestCurrentReportAvailabilityAndShowReportAbsence(t *testing.T) {
	const item = "inspect-private-reports/readback"
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "inspect-private-reports", "readback", ledger.ReadyForMerge, nil, deliveryInitial)
	head := l.commitAll("commit Work Item without reports")

	readback, err := ledger.ShowItem(l.store(), deliveryWidgets(), item)
	if err != nil {
		t.Fatalf("ShowItem without reports: %v", err)
	}
	if len(readback.Reports) != 2 {
		t.Fatalf("report availability count = %d, want 2", len(readback.Reports))
	}
	for _, report := range readback.Reports {
		if report.Reference != nil {
			t.Errorf("%s reference = %+v, want nil for absent committed report", report.Phase, report.Reference)
		}
	}

	missingPath := deliveryReportPath("widgets", item, ledger.WatchdogPhase)
	l.addFile(missingPath, "uncommitted watchdog report\n")
	readback, err = ledger.ShowItem(l.store(), deliveryWidgets(), item)
	if err != nil {
		t.Fatalf("ShowItem with uncommitted report: %v", err)
	}
	for _, report := range readback.Reports {
		if report.Phase == ledger.WatchdogPhase && report.Reference != nil {
			t.Errorf("uncommitted report discovery = %+v, want nil", report.Reference)
		}
	}

	_, err = ledger.ShowReport(l.store(), deliveryWidgets(), item, ledger.WatchdogPhase)
	if err == nil || !strings.Contains(err.Error(), "no committed watchdog report") || !strings.Contains(err.Error(), head+":"+missingPath) {
		t.Errorf("ShowReport absent report error = %v, want actionable absent committed reference %s:%s", err, head, missingPath)
	}
}

func TestShowReportRejectsInvalidPhaseActionably(t *testing.T) {
	_, err := ledger.ShowReport(nil, deliveryWidgets(), "bad", "archive")
	if err == nil || !strings.Contains(err.Error(), "phase") || !strings.Contains(err.Error(), ledger.ImplementPhase) || !strings.Contains(err.Error(), ledger.WatchdogPhase) {
		t.Fatalf("invalid phase error = %v, want actionable implement/watchdog refusal", err)
	}
}

func assertCurrentReportAvailability(t *testing.T, reports []ledger.ReportAvailability, commit, item string) {
	t.Helper()
	if len(reports) != 2 {
		t.Fatalf("report availability count = %d, want 2", len(reports))
	}
	for index, phase := range []string{ledger.ImplementPhase, ledger.WatchdogPhase} {
		report := reports[index]
		wantPath := deliveryReportPath("widgets", item, phase)
		if report.Phase != phase {
			t.Errorf("reports[%d].Phase = %q, want %q", index, report.Phase, phase)
		}
		if report.Reference == nil {
			t.Errorf("reports[%d].Reference = nil, want committed reference", index)
			continue
		}
		if report.Reference.Commit != commit || report.Reference.Path != wantPath {
			t.Errorf("reports[%d].Reference = %+v, want commit %s path %s", index, report.Reference, commit, wantPath)
		}
	}
}

func assertReportDocument(t *testing.T, document ledger.ContractDocument, commit, item, phase, contents string) {
	t.Helper()
	wantPath := deliveryReportPath("widgets", item, phase)
	if document.Commit != commit || document.Path != wantPath {
		t.Errorf("ShowReport identity = %s:%s, want %s:%s", document.Commit, document.Path, commit, wantPath)
	}
	if document.Contents != contents {
		t.Errorf("ShowReport %s contents = %q, want exact original bytes %q", phase, document.Contents, contents)
	}
}
