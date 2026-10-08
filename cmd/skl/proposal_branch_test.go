package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func proposalBranchRecord(t *testing.T, clone, name string) map[string]any {
	t.Helper()
	var record map[string]any
	path := "projects/widgets/proposals/" + name + "/proposal.json"
	if err := json.Unmarshal([]byte(readLedgerFile(t, clone, path)), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestAcceptRecordsAndFreezesProposalBranch(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))

	declared := singleSlice("named-proposal")
	declared.branch = "proposal/custom"
	declared.slices[0].branch = "slice/named"
	if got := cli.accept(t, root, writeProposal(t, "", declared)); got.Status != "accepted" {
		t.Fatalf("accept declared branch: %+v", got)
	}
	if got := proposalBranchRecord(t, fixture.clone, declared.name)["branch"]; got != "proposal/custom" {
		t.Fatalf("recorded branch = %v", got)
	}
	defaulted := singleSlice("default-proposal")
	defaulted.slices[0].branch = "slice/default"
	if got := cli.accept(t, root, writeProposal(t, "", defaulted)); got.Status != "accepted" {
		t.Fatalf("accept default branch: %+v", got)
	}
	if got := proposalBranchRecord(t, fixture.clone, defaulted.name)["branch"]; got != "proposal/default-proposal" {
		t.Fatalf("default branch = %v", got)
	}
	head := runGitOutput(t, fixture.clone, "rev-parse", "HEAD")
	for _, spec := range []proposalSpec{declared, defaulted} {
		if got := cli.accept(t, root, writeProposal(t, "", spec)); got.Status != "existing" {
			t.Fatalf("repeat %s: %+v", spec.name, got)
		}
	}
	if got := runGitOutput(t, fixture.clone, "rev-parse", "HEAD"); got != head {
		t.Fatalf("idempotent acceptance committed %s", got)
	}
	declared.branch = "proposal/other"
	if got := cli.accept(t, root, writeProposal(t, "", declared)); got.Status != "fix_required" || !strings.Contains(got.Reason, "different Proposal Branch") {
		t.Fatalf("changed relationship: %+v", got)
	}
	if got := proposalBranchRecord(t, fixture.clone, declared.name)["branch"]; got != "proposal/custom" {
		t.Fatalf("changed name overwrote the recorded branch: %v", got)
	}
	if got := runGitOutput(t, fixture.clone, "rev-parse", "HEAD"); got != head {
		t.Fatalf("refused acceptance committed %s", got)
	}
}

func TestAcceptRefusesInvalidAndOwnedProposalBranchesBeforeWriting(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	seed := singleSlice("first")
	seed.branch = "proposal/first"
	seed.slices[0].branch = "slice/first"
	if got := cli.accept(t, root, writeProposal(t, "", seed)); got.Status != "accepted" {
		t.Fatalf("seed: %+v", got)
	}
	// Archived branch ownership remains authoritative after the active records move.
	writeFile(t, filepath.Join(fixture.clone, "projects/widgets/archive/old/proposal.json"), `{"accepted":"2024-01-01T00:00:00Z","branch":"proposal/archived"}`)
	writeFile(t, filepath.Join(fixture.clone, "projects/widgets/archive/old/member/state.json"), `{"state":"merged","branch":"slice/archived"}`)
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "archived owners")

	for _, tc := range []struct {
		name, proposalBranch, sliceBranch, reason string
	}{
		{"invalid", "-bad branch", "", "-bad branch"},
		{"empty", "", "", "invalid Proposal Branch"},
		{"slice-active", "slice/first", "", "Slice first/foundation"},
		{"slice-archived", "slice/archived", "", "Slice old/member"},
		{"proposal-active", "proposal/first", "", "Proposal first"},
		{"proposal-archived", "proposal/archived", "", "Proposal old"},
		{"slice-takes-proposal", "", "proposal/first", "Proposal first"},
		{"slice-takes-archived-proposal", "", "proposal/archived", "Proposal old"},
		{"same-intake", "proposal/same-intake", "proposal/same-intake", "Proposal same-intake"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := singleSlice(tc.name)
			if tc.name == "same-intake" {
				spec.name = "same-intake"
			}
			spec.branch = tc.proposalBranch
			spec.slices[0].branch = tc.sliceBranch
			directory := writeProposal(t, "", spec)
			if tc.name == "empty" {
				writeFile(t, filepath.Join(directory, "proposal.json"), strings.Replace(spec.declaration(), `"slices":`, `"branch": "", "slices":`, 1))
			}
			before := ledgerSnapshot(t, fixture.clone)
			got := cli.accept(t, root, directory)
			if got.Status != "fix_required" || !strings.Contains(got.Reason, tc.reason) {
				t.Fatalf("expected owner %q: %+v", tc.reason, got)
			}
			if after := ledgerSnapshot(t, fixture.clone); after != before {
				t.Fatalf("refusal changed ledger: %s\nwas %s", after, before)
			}
		})
	}
}

func TestReacceptLegacyProposalLeavesBranchAbsent(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	spec := singleSlice("legacy")
	if got := cli.accept(t, root, writeProposal(t, "", spec)); got.Status != "accepted" {
		t.Fatalf("seed: %+v", got)
	}
	path := "projects/widgets/proposals/legacy/proposal.json"
	record := proposalBranchRecord(t, fixture.clone, spec.name)
	delete(record, "branch")
	commitLedgerRecord(t, fixture.clone, path, mustJSON(t, record)+"\n")
	head := runGitOutput(t, fixture.clone, "rev-parse", "HEAD")
	if got := cli.accept(t, root, writeProposal(t, "", spec)); got.Status != "existing" {
		t.Fatalf("legacy repeat: %+v", got)
	}
	if _, present := proposalBranchRecord(t, fixture.clone, spec.name)["branch"]; present {
		t.Fatal("legacy branch was backfilled")
	}
	if got := runGitOutput(t, fixture.clone, "rev-parse", "HEAD"); got != head {
		t.Fatalf("legacy repeat committed %s", got)
	}
}
