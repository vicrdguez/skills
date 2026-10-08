package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestStatusObservesHumanMergeIntoProposalBranch(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("branch-status"))); accepted.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, accepted))
	}
	attachFixture(t, fixture.clone, "branch-status")
	statusRecord(t, fixture.clone, "branch-status", "foundation", func(state *ledger.SliceState) {
		state.Target.Branch = "proposal/branch-status"
	})
	app, output := completionStatusApp(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "proposal/branch-status"))
	})
	observed := runCompletionStatus(t, app, output, root, "--item", "branch-status/foundation")
	if len(observed.Items) != 1 || observed.Items[0].State != ledger.Merged || observed.Items[0].Completion == nil || observed.Items[0].Completion.Target.Branch != "proposal/branch-status" {
		t.Fatalf("branch merge observation = %+v", observed)
	}
}

func TestImplementAutoFlagAppliesToNextAndDispatchOnly(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	app := newLedgerApp(t, newForgeServer(t))
	if accepted := app.accept(t, source, writeProposal(t, "", dualSlice(deliveryTestProposal))); accepted.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, accepted))
	}
	before := runGitOutput(t, fixture.clone, "rev-parse", "HEAD")
	if _, err := app.deliveryJSON(t, "skl", "implement", "resume", "--auto", "--repo", source, "--item", deliveryTestItem, "--claim", before, "--format", "json"); err == nil || !strings.Contains(err.Error(), "auto") {
		t.Fatalf("resume --auto error = %v; want flag refusal", err)
	}
	if after := runGitOutput(t, fixture.clone, "rev-parse", "HEAD"); after != before {
		t.Fatal("refused resume flag mutated the ledger")
	}
	out, err := app.deliveryJSON(t, "skl", "implement", "next", "--auto", "--repo", source, "--format", "json")
	if err != nil || out.Execution == nil || out.Execution.State.Target == nil || out.Execution.State.Target.Branch != "proposal/"+deliveryTestProposal || out.Status != ledger.WorkAvailable {
		t.Fatalf("Auto Mode next = %+v, %v", out, err)
	}
}
