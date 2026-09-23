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

func TestPublicationCLIFinalReadPreservesNewerPresentation(t *testing.T) {
	u := newPublicationPendingUpdate(t, "final-read-newer")
	u.forge.branchRemote = ""
	u.forge.setPullHead(u.number, publicationReport(t, u.fixture.clone, u.proposal, "foundation", ledger.WatchdogPhase).Source.Head)
	const newer = "newer human-authored approval of another revision\n"
	ready, reads := false, 0
	u.forge.setBefore(func(method, path string) {
		if method == "POST" && path == "/graphql" {
			ready = true
		}
		if ready && method == "GET" && path == "/repos/acme/widgets/pulls/11" {
			reads++
			if reads == 2 {
				u.forge.setPullHead(u.number, strings.Repeat("9", 40))
				u.forge.setPullBody(u.number, newer)
			}
		}
	})
	out, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil || out.Status != "pending" || reads < 2 {
		t.Fatalf("newer final-read presentation = %#v, %v, reads=%d", out, err, reads)
	}
	if pull := u.forge.pull(u.number); pull.Body != newer || pull.Draft {
		t.Fatalf("a newer presentation was replaced: %#v", pull)
	}
	if u.forge.count("POST", "/graphql") != 1 {
		t.Fatalf("newer readiness was overwritten: %#v", u.forge.recordedRequests())
	}
}

func TestPublicationCLIDoesNotConflateDifferentOriginalAnchors(t *testing.T) {
	u := newPublicationPendingUpdate(t, "other-original-anchor", true)
	u.forge.setPullFiles(publicationReviewFile{Filename: "feature.txt", Status: "added", Patch: "@@ -0,0 +1,2 @@\n+delivered foundation\n+# maintenance note\n"})
	selected := ledger.SelectedFinding{ID: "W2", Body: "Same public words at distinct anchors", Commit: u.reviewed, Path: "feature.txt", Line: 1, Side: "RIGHT"}
	path := filepath.Join(t.TempDir(), "finding.json")
	writeFile(t, path, mustJSON(t, []ledger.SelectedFinding{selected}))
	// Current placement looks identical, but the exact original reviewed
	// commit/line identifies a different publication. Never suppress W2.
	u.forge.mu.Lock()
	u.forge.comments = append(u.forge.comments,
		publicationReviewComment{Body: selected.Body, Commit: selected.Commit, Path: selected.Path, Line: selected.Line,
			Side: selected.Side, OriginalCommit: strings.Repeat("a", 40), OriginalLine: 1, Outdated: true},
		publicationReviewComment{Body: selected.Body, Commit: selected.Commit, Path: selected.Path, Line: selected.Line,
			Side: selected.Side, OriginalCommit: selected.Commit, OriginalLine: 2, Outdated: true})
	u.forge.mu.Unlock()
	out, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--findings", path, "--format", "json")
	if err != nil || len(out.Findings) != 1 || out.Findings[0].Status != "satisfied" {
		t.Fatalf("distinct original anchor = %#v, %v", out, err)
	}
	if len(u.forge.commentCreates) != 1 || u.forge.commentCount() != 3 {
		t.Fatalf("distinct original anchors were conflated: %#v", u.forge.commentCreates)
	}
}
