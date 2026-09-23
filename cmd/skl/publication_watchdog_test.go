package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// The final GET is a separate movement boundary after readiness has already
// been observed. Only this attempt's presentation should be retracted.
func TestPublicationCLIFinalReadRestoresReadyOnMovedSource(t *testing.T) {
	u := newPublicationPendingUpdate(t, "final-read-movement")
	// Disconnect the fixture's branch-following source so this test can model
	// a head moved by a different publisher between two actual GETs.
	u.forge.branchRemote = ""
	u.forge.setPullHead(u.number, publicationReport(t, u.fixture.clone, u.proposal, "foundation", ledger.WatchdogPhase).Source.Head)
	ready := false
	reads := 0
	u.forge.setBefore(func(method, path string) {
		if method == "POST" && path == "/graphql" {
			ready = true
		}
		if ready && method == "GET" && path == "/repos/acme/widgets/pulls/11" {
			reads++
			if reads == 2 {
				u.forge.setPullHead(u.number, strings.Repeat("9", 40))
			}
		}
	})
	out, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil || out.Status != "pending" || reads < 2 {
		t.Fatalf("final-read movement = %#v, %v (reads=%d)", out, err, reads)
	}
	if pull := u.forge.pull(u.number); !pull.Draft || pull.Body == u.finalPublic {
		t.Fatalf("unreviewed head retained this attempt's approval: %#v", pull)
	}
	if u.forge.count("POST", "/graphql") != 2 {
		t.Fatalf("ready/draft mutations = %d, want one correction", u.forge.count("POST", "/graphql"))
	}
}

func TestPublicationCLIOriginalReviewedAnchorDeduplicatesLostResponse(t *testing.T) {
	u := newPublicationPendingUpdate(t, "outdated-reviewed-anchor", true)
	u.forge.setPullFiles(publicationReviewFile{Filename: "feature.txt", Status: "added", Patch: "@@ -0,0 +1,2 @@\n+delivered foundation\n+# maintenance note\n"})
	selected := ledger.SelectedFinding{ID: "W2", Body: "Actionable historical finding", Commit: u.reviewed, Path: "feature.txt", Line: 1, Side: "RIGHT"}
	path := filepath.Join(t.TempDir(), "finding.json")
	writeFile(t, path, mustJSON(t, []ledger.SelectedFinding{selected}))
	u.forge.setAfterMutation(func(method, path string) {
		if method == "POST" && strings.HasSuffix(path, "/comments") {
			comment := &u.forge.comments[len(u.forge.comments)-1]
			comment.OriginalCommit, comment.OriginalLine = comment.Commit, comment.Line
			comment.Commit, comment.Line, comment.Outdated = strings.Repeat("a", 40), 0, true
		}
	})
	u.forge.setDrop(func(method, path string) bool { return method == "POST" && strings.HasSuffix(path, "/comments") })
	for attempt := 0; attempt < 2; attempt++ {
		out, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
			"--item", u.item, "--kind", "pull", "--findings", path, "--format", "json")
		if err != nil || len(out.Findings) != 1 || out.Findings[0].Status != "satisfied" {
			t.Fatalf("attempt %d = %#v, %v", attempt, out, err)
		}
	}
	if u.forge.commentCount() != 1 || len(u.forge.commentCreates) != 1 {
		t.Fatalf("historical comment was duplicated: %#v", u.forge.commentCreates)
	}
}
