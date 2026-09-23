package main

import (
	"path/filepath"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestPublicationCLIAnchorsReviewedRevisionBeforeFinalMarker(t *testing.T) {
	u := newPublicationPendingUpdate(t, "reviewed-before-marker", true)
	report := publicationReport(t, u.fixture.clone, u.proposal, "foundation", ledger.WatchdogPhase)
	if report.Source.Head == u.reviewed || report.Source.Reviewed != u.reviewed {
		t.Fatal("fixture must retain different reviewed and final revisions")
	}
	// Line 1 existed at the reviewed revision. Line 2 exists only in the final
	// marker commit; the current PR files must not substitute for reviewed code.
	u.forge.setPullFiles(publicationReviewFile{Filename: "feature.txt", Status: "added", Patch: "@@ -0,0 +1,2 @@\n+delivered foundation\n+# maintenance note\n"})
	findings := []ledger.SelectedFinding{
		{ID: "W2", Body: "Actionable reviewed-code finding", Commit: u.reviewed, Path: "feature.txt", Line: 1, Side: "RIGHT"},
		{ID: "W3", Body: "Not present at the reviewed revision", Commit: u.reviewed, Path: "feature.txt", Line: 2, Side: "RIGHT"},
	}
	path := filepath.Join(t.TempDir(), "findings.json")
	writeFile(t, path, mustJSON(t, findings))
	u.forge.setDrop(func(method, path string) bool {
		return method == "POST" && path == "/repos/acme/widgets/pulls/11/comments"
	})
	out, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--findings", path, "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, finding := range out.Findings {
		statuses[finding.ID] = finding.Status
	}
	if out.Status != "published" || statuses["W2"] != "satisfied" || statuses["W3"] != "unresolved" {
		t.Fatalf("reviewed/final publication: status=%s findings=%v", out.Status, out.Findings)
	}
	if u.forge.commentCount() != 1 || u.forge.commentCreates[0]["commit_id"] != u.reviewed || u.forge.commentCreates[0]["line"] != 1 {
		t.Fatalf("reviewed anchor was changed or duplicated: %#v", u.forge.commentCreates)
	}
	// Selecting another finding after the body is satisfied must not require
	// reauthoring that body, and repeating W2 must retain its receipt.
	findings = append(findings[:1], ledger.SelectedFinding{ID: "W4", Body: "Another explicit finding", Commit: u.reviewed, Path: "feature.txt", Line: 1, Side: "RIGHT"})
	writeFile(t, path, mustJSON(t, findings))
	out, err = newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--findings", path, "--format", "json")
	if err != nil || len(out.Findings) != 2 || u.forge.commentCount() != 2 {
		t.Fatalf("post-body selection: %#v error=%v comments=%d", out, err, u.forge.commentCount())
	}
	for _, finding := range out.Findings {
		if finding.Status != "satisfied" {
			t.Fatalf("post-body finding not satisfied: %#v", finding)
		}
	}
}
