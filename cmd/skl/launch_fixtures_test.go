package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// commitReadyProposal arranges accepted work, not acceptance or publication behavior.
// Each caller owns its ledger, upstream, source, configuration and later Claims.
// The real acceptance journey remains in ledger_test.go and delivery_test.go.
func (f *ledgerFixture) commitReadyProposal(t *testing.T, spec proposalSpec) {
	t.Helper()
	base := "projects/widgets/proposals/" + spec.name
	writeFile(t, filepath.Join(f.clone, "projects/widgets/project.json"), `{"repository":"acme/widgets"}`)
	branch := spec.branch
	if branch == "" {
		branch = "proposal/" + spec.name
	}
	meta := ledger.ProposalMeta{Accepted: "2026-01-01T00:00:00Z", Branch: branch, ParentTitle: spec.parentTitle}
	writeFile(t, filepath.Join(f.clone, base, "proposal.json"), mustJSON(t, meta))
	writeFile(t, filepath.Join(f.clone, base, "proposal.md"), spec.description)
	for _, slice := range spec.slices {
		branch := slice.branch
		if branch == "" {
			branch = slice.name
		}
		state := ledger.SliceState{State: ledger.ReadyForImplementation, Title: slice.title, Branch: branch}
		for _, dependency := range spec.depends[slice.name] {
			if !strings.HasPrefix(dependency, "proposals/") {
				dependency = "proposals/" + spec.name + "/" + dependency
			}
			state.Dependencies = append(state.Dependencies, dependency)
		}
		contents, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(f.clone, base, slice.name, "state.json"), string(contents)+"\n")
		for name, contents := range slice.files {
			writeFile(t, filepath.Join(f.clone, base, slice.name, name), contents)
		}
	}
	runGit(t, f.clone, "add", "-A")
	runGit(t, f.clone, "commit", "-q", "-m", "accepted test fixture")
	runGit(t, f.clone, "push", "-q")
}

// commitReviewClaim supplies the historical implement Claim consumed by
// a review fixture. It is separate from finishing a real Claim in the wait test.
func (f *ledgerFixture) commitReviewClaim(t *testing.T, targetBranch string) {
	t.Helper()
	basis := deliveryTrimmed(t, f.clone, "rev-parse", "HEAD")
	state := deliveryPersistedState(t, f.clone)
	state.Target = &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: targetBranch}
	state.Claim = &ledger.Claim{Phase: ledger.ImplementPhase, Basis: basis, Inputs: ledger.ClaimInputs{
		Contract: []ledger.Reference{
			{Commit: basis, Path: deliveryItemDirectory() + "/behavior.md"},
			{Commit: basis, Path: deliveryItemDirectory() + "/intent.md"},
		},
	}}
	deliveryCommitState(t, f.clone, state)
}

// commitLaunchAwaitingReview preserves real source objects and exact historical
// Claim/Contract references without running prepare and submit as incidental
// setup. dispatch_test.go and delivery_test.go still exercise those transitions.
func (f *ledgerFixture) commitLaunchAwaitingReview(t *testing.T, source, target string) {
	t.Helper()
	claim := deliveryTrimmed(t, f.clone, "rev-parse", "HEAD")
	state := deliveryPersistedState(t, f.clone)
	if state.Claim == nil || state.Claim.Phase != ledger.ImplementPhase {
		t.Fatal("review fixture requires a committed implement Claim")
	}
	runGit(t, source, "checkout", "-q", "-b", state.Branch, target)
	head := proseCommit(t, source, "launch.txt", "implementation fixture\n")
	runGit(t, source, "checkout", "-q", "main")
	report, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
		Schema: ledger.ReportSchema, Outcome: ledger.AwaitingReview,
		Source: ledger.SourceRevisions{Head: head, Target: target},
		Ledger: ledger.ReportInputs{
			Claim:    ledger.Reference{Commit: claim, Path: deliveryItemStatePath()},
			Contract: state.Claim.Inputs.Contract,
		},
	}, "# Implementation evidence\n\nLaunch fixture ready for review.\n")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.clone, deliveryItemDirectory(), "implement-report.md"), string(report))
	state.State, state.Claim = ledger.AwaitingReview, nil
	deliveryCommitState(t, f.clone, state)
	runGit(t, f.clone, "push", "-q")
}
