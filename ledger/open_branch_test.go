package ledger_test

import (
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestAutoClaimOpensProposalAndLaterPlainClaimUsesIt(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "feature", "first", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addSlice("widgets", "feature", "second", ledger.ReadyForImplementation, nil, deliveryInitial)
	// A Slice accepted before this change may carry a default that is not yet fixed.
	l.writeStateValue("widgets", "feature", "first", ledger.SliceState{State: ledger.ReadyForImplementation, Title: "first", Branch: "first", Target: &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "main"}})
	l.commitAll("accept legacy Proposal")
	store := l.store()
	opened, err := ledger.StartDeliveryContext(t.Context(), store, deliveryWidgets(), ledger.ImplementPhase, true)
	if err != nil || opened == nil || opened.Item != "feature/first" || opened.State.Target == nil || opened.State.Target.Branch != "proposal/feature" {
		t.Fatalf("opening Claim = %+v, %v", opened, err)
	}
	proposal, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	view, err := proposal.Proposal("widgets", "feature")
	if err != nil || view.Proposal.Branch != "proposal/feature" || view.Proposal.Target == nil || view.Proposal.Target.Branch != "proposal/feature" {
		t.Fatalf("Proposal opening = %+v, %v", view, err)
	}
	plain, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.ImplementPhase)
	if err != nil || plain == nil || plain.Item != "feature/second" || plain.State.Target == nil || plain.State.Target.Branch != "proposal/feature" {
		t.Fatalf("plain Claim on open Proposal = %+v, %v", plain, err)
	}
	if resumed, err := ledger.ResumeDelivery(store, deliveryWidgets(), opened.Item, ledger.ImplementPhase, opened.Claim.Commit); err != nil || resumed.State.Target.Branch != "proposal/feature" {
		t.Fatalf("opening Claim did not retain its target: %+v, %v", resumed, err)
	}
}

func TestPlainClaimKeepsClosedProposalClosed(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "feature", "first", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addFile("projects/widgets/proposals/feature/proposal.json", `{"accepted":"`+deliveryInitial+`","branch":"proposal/feature"}`)
	l.commitAll("accept")
	store := l.store()
	plain := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	if plain.State.Target == nil || plain.State.Target.Branch != "main" {
		t.Fatalf("plain Claim target = %+v", plain.State.Target)
	}
	view, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := view.Proposal("widgets", "feature")
	if err != nil || proposal.Proposal.Target != nil {
		t.Fatalf("closed Proposal opened: %+v, %v", proposal, err)
	}
}

func TestDependenciesRequireBuildableCompletion(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "p", "blocker", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "q", "dependent", ledger.ReadyForImplementation, []string{"proposals/p/blocker"}, deliveryInitial)
	l.addFile("projects/widgets/proposals/p/proposal.json", `{"accepted":"`+deliveryInitial+`","branch":"proposal/p","integration_target":{"repository":"acme/widgets","branch":"proposal/p"}}`)
	l.writeStateValue("widgets", "p", "blocker", ledger.SliceState{State: ledger.Merged, Title: "blocker", Branch: "blocker", Target: &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "proposal/p"}, Completion: &ledger.TerminalEvidence{Target: ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "proposal/p"}}})
	l.commitAll("blocker merged into open branch")
	store := l.store()
	if claimed, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.ImplementPhase); err != nil || claimed != nil {
		t.Fatalf("cross-Proposal dependency was eligible: %+v, %v", claimed, err)
	}
	view, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	detail, err := view.Slice("widgets", "q/dependent")
	if err != nil || detail.Dependencies[0].Satisfied {
		t.Fatalf("browse considered cross-Proposal dependency satisfied: %+v, %v", detail, err)
	}
	l.addFile("projects/widgets/proposals/p/proposal.json", `{"accepted":"`+deliveryInitial+`","branch":"proposal/p","integration_target":{"repository":"acme/widgets","branch":"proposal/p"},"completion":{"target":{"repository":"acme/widgets","branch":"main"}}}`)
	l.commitAll("Proposal reached main")
	if claimed, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.ImplementPhase); err != nil || claimed == nil || claimed.Item != "q/dependent" {
		t.Fatalf("completed Proposal did not unblock dependency: %+v, %v", claimed, err)
	}
}

func TestSameProposalMergeUnblocksPlainClaim(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "feature", "blocker", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "feature", "dependent", ledger.ReadyForImplementation, []string{"proposals/feature/blocker"}, deliveryInitial)
	l.addFile("projects/widgets/proposals/feature/proposal.json", `{"accepted":"`+deliveryInitial+`","branch":"proposal/feature","integration_target":{"repository":"acme/widgets","branch":"proposal/feature"}}`)
	l.writeStateValue("widgets", "feature", "blocker", ledger.SliceState{State: ledger.Merged, Title: "blocker", Branch: "blocker", Completion: &ledger.TerminalEvidence{Target: ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "proposal/feature"}}})
	l.commitAll("blocker merged into Proposal Branch")
	claimed := deliveryStart(t, l.store(), deliveryWidgets(), ledger.ImplementPhase)
	if claimed.Item != "feature/dependent" || claimed.State.Target == nil || claimed.State.Target.Branch != "proposal/feature" {
		t.Fatalf("same-Proposal dependent = %+v", claimed)
	}
}

func TestStartedSliceKeepsMainWhenProposalOpensLater(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "feature", "first", ledger.Rework, nil, deliveryInitial)
	l.addFile(deliveryReportPath("widgets", "feature/first", ledger.WatchdogPhase), deliveryWatchdogReport("rework", 1, "widgets", "feature/first"))
	l.addFile(deliveryReportPath("widgets", "feature/first", ledger.ImplementPhase), `---
schema: 1
outcome: awaiting_review
source:
  head: `+deliveryHead+`
  target: `+deliveryTarget+`
ledger:
  claim:
    commit: `+deliveryRefID+`
    path: `+deliveryStatePath("widgets", "feature/first")+`
  contract:
    - commit: `+deliveryRefID+`
      path: `+deliveryContractPath("widgets", "feature/first", "behavior.md")+`
---
Earlier implementation.
`)
	l.writeStateValue("widgets", "feature", "first", ledger.SliceState{State: ledger.Rework, Title: "first", Branch: "first", Target: &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "main"}, Submission: &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 17}})
	l.addSlice("widgets", "feature", "second", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addFile("projects/widgets/proposals/feature/proposal.json", `{"accepted":"`+deliveryInitial+`","branch":"proposal/feature","integration_target":{"repository":"acme/widgets","branch":"proposal/feature"}}`)
	l.commitAll("first Slice awaiting rework after branch opening")
	claimed := deliveryStart(t, l.store(), deliveryWidgets(), ledger.ImplementPhase)
	if claimed.Item != "feature/first" || claimed.State.Target == nil || claimed.State.Target.Branch != "main" {
		t.Fatalf("reworked Slice lost its original target: %+v", claimed)
	}
}

func TestOpenBranchIsNotDeliveredOrAutomaticallyArchived(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "feature", "only", ledger.Merged, nil, deliveryInitial)
	l.addFile("projects/widgets/proposals/feature/proposal.json", `{"accepted":"`+deliveryInitial+`","branch":"proposal/feature","integration_target":{"repository":"acme/widgets","branch":"proposal/feature"}}`)
	l.commitAll("open branch")
	store := l.store()
	_, proposals, err := ledger.CompletionStatus(store, deliveryWidgets(), []string{"feature/only"}, true, nil)
	if err != nil || len(proposals) != 1 || proposals[0].FullyDelivered || proposals[0].Retireable {
		t.Fatalf("open Proposal status = %+v, %v", proposals, err)
	}
	archive, err := ledger.ArchiveTerminalProposals(t.Context(), store, deliveryWidgets())
	if err != nil || len(archive.Archived) != 0 || len(archive.Kept) != 1 || !strings.Contains(archive.Kept[0].Reason, "proposal/feature") {
		t.Fatalf("open Proposal cleanup = %+v, %v", archive, err)
	}
}
