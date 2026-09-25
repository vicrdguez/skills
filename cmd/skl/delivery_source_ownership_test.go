package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// A Project's planned branch belongs to one Work Item, even when the other
// proposal has not yet acquired a Claim or created a source checkout.
func TestDeliveryRefusesCrossProposalSourceOwnership(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "default branch"
		if explicit {
			name = "explicit shared branch"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			source, _ := deliverySourceRepo(t)
			forge := newForgeServer(t)
			accept := newLedgerApp(t, forge)
			first := singleSlice("collision-one")
			if explicit {
				first.slices[0].branch = "shared-work"
			}
			if got := accept.accept(t, source, writeProposal(t, "", first)); got.Status != "accepted" {
				t.Fatalf("first acceptance: %#v", got)
			}
			cli := deliveryNoForgeApp(t)
			claimed, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
			if err != nil || claimed.Execution == nil {
				t.Fatalf("first Claim: %#v %v", claimed, err)
			}
			prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", "collision-one/foundation", "--claim", claimed.Execution.Claim.Commit, "--format", "json")
			if err != nil || prepared.Source == nil {
				t.Fatalf("first preparation: %#v %v", prepared, err)
			}
			writeFile(t, filepath.Join(prepared.Source.Worktree, "owned-by-first.txt"), "private unfinished progress\n")
			before := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
			second := singleSlice("collision-two")
			if explicit {
				second.slices[0].branch = "shared-work"
			}
			refused := accept.accept(t, source, writeProposal(t, "", second))
			if refused.Status != "fix_required" || !strings.Contains(refused.Reason, "branch") {
				t.Fatalf("second acceptance should refuse shared source: %#v", refused)
			}
			if deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD") != before || !strings.Contains(readFileString(t, filepath.Join(prepared.Source.Worktree, "owned-by-first.txt")), "private unfinished progress") {
				t.Fatal("refusal changed accepted records or unfinished source work")
			}
			var state ledger.SliceState
			err = json.Unmarshal([]byte(runGitOutput(t, fixture.clone, "show", "HEAD:projects/widgets/proposals/collision-one/foundation/state.json")), &state)
			if err != nil || state.Claim == nil || state.Claim.Basis == "" {
				t.Fatalf("first Claim not preserved: %#v %v", state, err)
			}
		})
	}
}
