package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestPublicationCLIReconcilesAbandonedIssueReservation(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newPublicationForge(t)
	source := sourceRepository(t, "acme", "widgets")
	forge.setFail(func(method, path string) int {
		if method == "POST" && path == "/repos/acme/widgets/issues" {
			return 503
		}
		return 0
	})
	directory, flags, _ := publicationProposal(t, singleSlice("abandoned-issue"), map[string]string{"foundation": "unique issue prose\n"})
	cli := newPublicationApp(t, forge)
	if out := cli.accept(t, source, directory, flags...); out.Status != "accepted" {
		t.Fatal(out.Status)
	}
	forge.setFail(nil)
	state := publicationState(t, fixture.clone, "abandoned-issue", "foundation")
	state.Publication.Issue = &ledger.PublicationNote{Status: "reserved", Detail: "interrupted after creation"}
	publicationCommitState(t, fixture.clone, "abandoned-issue", state)
	number := forge.addIssue(state.Title, "unique issue prose\n")
	item := "abandoned-issue/foundation"
	view, err := publicationNoForgeApp(t, nil).publicationJSON(t, "skl", "publication", "inspect", "--repo", source,
		"--item", item, "--kind", "issue", "--format", "json")
	if err != nil || view.Status != "ambiguous" || !strings.Contains(view.Packet.Facts.Publication.View.Detail, "--reconcile-reservation") {
		t.Fatalf("reservation inspection = %#v, %v", view, err)
	}
	token := view.Packet.Facts.Publication.View.Token
	blocked, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", item, "--kind", "issue", "--format", "json")
	if err != nil || blocked.Status != "pending" {
		t.Fatalf("ordinary recovery bypassed reservation: %#v, %v", blocked, err)
	}
	wrong, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", item, "--kind", "issue", "--reconcile-reservation", "--view", "wrong", "--format", "json")
	if err != nil || wrong.Status != "stale" {
		t.Fatalf("wrong confirmation = %#v, %v", wrong, err)
	}
	observed, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", item, "--kind", "issue", "--reconcile-reservation", "--view", token, "--format", "json")
	if err != nil || (observed.Status != "already-satisfied" && observed.Status != "published") {
		t.Fatalf("reconcile issue = %#v, %v", observed, err)
	}
	attached := publicationState(t, fixture.clone, "abandoned-issue", "foundation")
	if attached.Issue == nil || attached.Issue.Number != number || attached.Publication.Issue != nil || forge.count("POST", "/repos/acme/widgets/issues") != 1 {
		t.Fatalf("abandoned issue not adopted without a new create: %#v", attached)
	}
}

func TestPublicationCLIReconcilesAbandonedPullReservation(t *testing.T) {
	u := newPublicationPendingUpdate(t, "abandoned-pull")
	// The old publisher applied the approved body and readiness but exited
	// before it could settle its durable Active reference.
	u.forge.setPullBody(u.number, u.finalPublic)
	u.forge.setPullDraft(u.number, false)
	state := publicationState(t, u.fixture.clone, u.proposal, "foundation")
	state.Publication.Active = &ledger.Reference{
		Commit: strings.TrimSpace(runGitOutput(t, u.fixture.clone, "rev-parse", "HEAD")),
		Path:   filepath.ToSlash(filepath.Join("projects", "widgets", "proposals", u.proposal, "foundation", "watchdog-report.md")),
	}
	publicationCommitState(t, u.fixture.clone, u.proposal, state)
	view, err := publicationNoForgeApp(t, nil).publicationJSON(t, "skl", "publication", "inspect", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil || view.Status != "ambiguous" {
		t.Fatalf("reserved pull inspection = %#v, %v", view, err)
	}
	before := len(u.forge.recordedRequests())
	out, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--view", view.Packet.Facts.Publication.View.Token,
		"--reconcile-reservation", "--format", "json")
	if err != nil || (out.Status != "already-satisfied" && out.Status != "published") {
		t.Fatalf("reserved pull observation = %#v, %v", out, err)
	}
	for _, request := range u.forge.recordedRequests()[before:] {
		if strings.HasPrefix(request, "POST ") || strings.HasPrefix(request, "PATCH ") {
			t.Fatalf("reconciliation performed a forge write: %s", request)
		}
	}
	if state = publicationState(t, u.fixture.clone, u.proposal, "foundation"); state.Publication.Active != nil || state.Publication.Pull != nil {
		t.Fatalf("satisfied pull remained reserved: %#v", state.Publication)
	}
}

func TestPublicationCLIReconcilesAbandonedParentReservation(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newPublicationForge(t)
	source := sourceRepository(t, "acme", "widgets")
	forge.setFail(func(method, path string) int {
		if method == "POST" && path == "/repos/acme/widgets/issues" && forge.issueCount() >= 2 {
			return 503
		}
		return 0
	})
	directory, flags, parentBody := publicationProposal(t, dualSlice("abandoned-parent"), map[string]string{
		"foundation": "child one\n", "feature": "child two\n", "parent": "unique parent prose\n",
	})
	cli := newPublicationApp(t, forge)
	if out := cli.accept(t, source, directory, append(flags, "--parent-body", parentBody)...); out.Status != "accepted" {
		t.Fatal(out.Status)
	}
	forge.setFail(nil)
	meta := publicationProposalState(t, fixture.clone, "abandoned-parent")
	if meta.ParentBody == nil || meta.ParentIssue != nil {
		t.Fatalf("parent fixture = %#v", meta)
	}
	meta.ParentPublication = &ledger.PublicationNote{Status: "reserved", Detail: "interrupted after creation"}
	path := filepath.Join(fixture.clone, "projects", "widgets", "proposals", "abandoned-parent", "proposal.json")
	writeFile(t, path, mustJSON(t, meta)+"\n")
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "interrupted parent reservation")
	number := forge.addIssue(strings.TrimSpace(meta.ParentTitle), "unique parent prose\n")
	view, err := publicationNoForgeApp(t, nil).publicationJSON(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "abandoned-parent/foundation", "--kind", "parent", "--format", "json")
	if err != nil || view.Status != "ambiguous" {
		t.Fatalf("parent inspection = %#v, %v", view, err)
	}
	out, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", "abandoned-parent/foundation", "--kind", "parent", "--reconcile-reservation", "--view", view.Packet.Facts.Publication.View.Token, "--format", "json")
	if err != nil || out.Status != "pending" {
		t.Fatalf("partial parent observation = %#v, %v", out, err)
	}
	meta = publicationProposalState(t, fixture.clone, "abandoned-parent")
	if meta.ParentIssue == nil || meta.ParentIssue.Number != number || meta.ParentPublication == nil || forge.issueCount() != 3 {
		t.Fatalf("parent identity was not safely attached: %#v", meta)
	}
	finished, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", "abandoned-parent/foundation", "--kind", "parent", "--format", "json")
	if err != nil || finished.Status != "published" {
		t.Fatalf("resume missing grouping = %#v, %v", finished, err)
	}
	meta = publicationProposalState(t, fixture.clone, "abandoned-parent")
	if meta.ParentIssue == nil || meta.ParentIssue.Number != number || meta.ParentPublication != nil || forge.issueCount() != 3 {
		t.Fatalf("parent was not safely grouped: %#v", meta)
	}
}
