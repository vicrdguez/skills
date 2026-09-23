package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublicationCLIPreservesUnconfirmedCreateWhenProseChanges(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newPublicationForge(t)
	source, _, _ := publicationSourceRepo(t)
	forge.setFail(func(method, path string) int {
		if method == "POST" {
			return 503
		}
		return 0
	})
	dir, flags, _ := publicationProposal(t, singleSlice("uncertain-issue"), map[string]string{"foundation": "original public body\n"})
	cli := newPublicationApp(t, forge)
	if out := cli.accept(t, source, dir, flags...); out.Status != "accepted" {
		t.Fatal(out.Status)
	}
	originalPath := publicationState(t, fixture.clone, "uncertain-issue", "foundation").Publication.IssueBody.Path
	forge.setFail(func(method, path string) int {
		if method == "GET" && path == "/repos/acme/widgets/issues" {
			return 503
		}
		return 0
	})
	forge.setDrop(func(method, path string) bool { return method == "POST" && path == "/repos/acme/widgets/issues" })
	for attempt := 0; attempt < 2; attempt++ {
		out, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
			"--item", "uncertain-issue/foundation", "--kind", "issue", "--format", "json")
		if err != nil || out.Status != "ambiguous" || forge.issueCount() != 1 {
			t.Fatalf("uncertain attempt %d: status=%s err=%v created=%d", attempt, out.Status, err, forge.issueCount())
		}
	}
	forge.setDrop(nil)
	forge.setFail(nil)
	if err := os.Remove(originalPath); err != nil {
		t.Fatal(err)
	}
	view, err := publicationNoForgeApp(t, nil).publicationJSON(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "uncertain-issue/foundation", "--kind", "issue", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	freshPath := filepath.Join(t.TempDir(), "fresh.md")
	fresh := "fresh human-facing description\n"
	writeFile(t, freshPath, fresh)
	out, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", "uncertain-issue/foundation", "--kind", "issue", "--view", view.Packet.Facts.Publication.View.Token,
		"--body", freshPath, "--format", "json")
	if err != nil || out.Status != "published" || forge.issueCount() != 1 {
		t.Fatalf("reauthored recovery: status=%s err=%v created=%d", out.Status, err, forge.issueCount())
	}
	attached := publicationState(t, fixture.clone, "uncertain-issue", "foundation").Issue
	if attached == nil || forge.issue(attached.Number).Body != fresh {
		t.Fatal("fresh body did not update the original attachment")
	}
}
