package main

import (
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestAutoReadinessRefusalContinuesIndependentSiblingWhileDependentWaits(t *testing.T) {
	spec := dualSlice(deliveryTestProposal)
	spec.slices[1].name = "z-independent"
	dependent := singleSlice("unused").slices[0]
	dependent.name, dependent.title = "dependent", "Dependent"
	spec.slices = append(spec.slices, dependent)
	spec.depends = map[string][]string{"dependent": {"foundation"}}
	f := newAutoMergeProposalFixture(t, true, spec)
	f.pass(t, false, false)

	// B is independent; C needs A's qualifying Merged, not its PASS.
	b, err := f.cli.deliveryJSON(t, "skl", "implement", "next", "--repo", f.source, "--format", "json")
	if err != nil || b.Execution == nil || b.Execution.Item != deliveryTestProposal+"/z-independent" {
		t.Fatalf("independent implement = %+v, %v", b, err)
	}
	prepared, err := f.cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", f.source, "--slice", b.Execution.Item, "--claim", b.Execution.Claim.Commit, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("prepare B = %+v, %v", prepared, err)
	}
	head := proseCommit(t, prepared.Source.Worktree, "independent.txt", "B\n")
	submitted, err := f.cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", f.source, "--slice", b.Execution.Item, "--claim", b.Execution.Claim.Commit,
		"--head", head, "--target", prepared.Source.Target, "--body", proseFixturePath("implement-report.md"), "--format", "json")
	if err != nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("submit B = %+v, %v", submitted, err)
	}

	// A's old approval cannot authorize integration of the advanced target.
	base := f.branchHead(t, autoMergeBranch)
	moved := deliveryTrimmed(t, f.bare, "-c", "user.name=Human", "-c", "user.email=human@example.com", "commit-tree", base+"^{tree}", "-p", base, "-m", "target advances")
	runGit(t, f.bare, "update-ref", "refs/heads/"+autoMergeBranch, moved, base)
	polled := f.pollReadiness(t)
	if len(polled.Merges) != 1 || polled.Merges[0].Status != ledger.MergeRefused || polled.Execution == nil || polled.Execution.Item != b.Execution.Item {
		t.Fatalf("refusal blocked independent review: %s", mustJSON(t, polled))
	}
	reason := polled.Merges[0].Reason
	for _, required := range []string{"not an ancestor", "skl decision apply --route watchdog", "--project widgets", "--item " + deliveryTestItem, "--request-commit", "--request-path", "--answer"} {
		if !strings.Contains(reason, required) {
			t.Fatalf("reintegration guidance lacks %q: %s", required, reason)
		}
	}
	state := deliveryPersistedState(t, f.clone)
	if state.State != ledger.ReadyForMerge || f.mergeRequests() != 0 {
		t.Fatalf("A approval changed: %+v", state)
	}
	waiting, err := f.cli.deliveryJSON(t, "skl", "implement", "next", "--repo", f.source, "--format", "json")
	if err != nil || waiting.Status != ledger.NoWork {
		t.Fatalf("dependent started before A merged: %+v, %v", waiting, err)
	}
}
