package ledger_test

// Package-level Human Decision tests retain fault injection and state
// boundaries that complement the CLI's decision and delivery journeys:
// active Claims, failed commits, review continuation and guarded retirement.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
)

// decisionPauseImplement drives one initial implementation pause and returns
// the new request's exact reference and the paused state.
func decisionPauseImplement(t *testing.T, l *deliveryLedger, store *ledger.Store, repository github.RepositoryID, item string, source ledger.SourceRevisions) ledger.Reference {
	t.Helper()
	execution := deliveryStart(t, store, repository, ledger.ImplementPhase)
	deliveryHandoff(t, store, repository, item, ledger.ImplementPhase, execution.Claim.Commit, source, "needs_human", "human judgment remains required.\n")
	return ledger.Reference{Commit: l.head(), Path: deliveryReportPath(repository.Name, item, ledger.ImplementPhase)}
}

// decisionInbox reads the inbox and fails on refusal.
func decisionInbox(t *testing.T, store *ledger.Store, filter string) *ledger.Inbox {
	t.Helper()
	inbox, err := ledger.ReadDecisionInbox(store, ledger.InboxFilter{Project: filter})
	if err != nil {
		t.Fatalf("read decision inbox (filter %q): %v", filter, err)
	}
	return inbox
}

// TestDecisionRefusesActiveClaim covers the active-Claim refusal without
// changing the authoritative record.
func TestDecisionRefusesActiveClaim(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "decision-claim", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept decision-claim")

	store := l.store()
	item := "decision-claim/foundation"
	request := decisionPauseImplement(t, l, store, deliveryWidgets(), item, ledger.SourceRevisions{})

	claimed := l.committedState("widgets", "decision-claim", "foundation")
	claimed.Claim = &ledger.Claim{Phase: ledger.ImplementPhase, Basis: l.head()}
	l.commitState("widgets", "decision-claim", "foundation", claimed)
	headBefore := l.head()

	result, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "continue\n", Route: ledger.RouteImplement,
	})
	if err != nil {
		t.Fatalf("active claim returned error: %v", err)
	}
	if result.Status != ledger.DecisionRefused || !strings.Contains(result.Refusal, "active Claim") {
		t.Fatalf("active claim result = %#v", result)
	}
	if l.head() != headBefore {
		t.Fatal("active-claim refusal mutated the ledger")
	}
}

// TestDecisionPrecommitFaultLeavesNoPartialState covers B5/A2: a failure before
// the authoritative commit leaves neither decision.md nor its route state, and
// the same operation succeeds after the fault clears.
func TestDecisionPrecommitFaultLeavesNoPartialState(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "decision-fault", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept decision-fault")

	store := l.store()
	item := "decision-fault/foundation"
	request := decisionPauseImplement(t, l, store, deliveryWidgets(), item, ledger.SourceRevisions{})
	pausedHead := l.head()
	statePath := filepath.Join(l.root, filepath.FromSlash(deliveryStatePath("widgets", item)))
	decisionPath := filepath.Join(l.root, filepath.FromSlash(deliveryDecisionPath("widgets", item)))

	hook := filepath.Join(l.root, ".git", "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	input := ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "continue within the contract\n", Route: ledger.RouteImplement,
	}
	if result, err := ledger.ApplyDecision(store, input); err == nil {
		t.Fatalf("pre-commit fault recorded a decision: %#v", result)
	}
	if l.head() != pausedHead {
		t.Fatal("pre-commit fault created a commit")
	}
	if _, err := os.Stat(decisionPath); !os.IsNotExist(err) {
		t.Fatalf("partial decision document survived the fault: %v", err)
	}
	if got := deliveryReadFile(t, statePath); got != deliveryGitShow(t, l.root, "HEAD", deliveryStatePath("widgets", item)) {
		t.Fatal("partial route state survived the fault")
	}
	if dirty := deliveryGitOutput(t, l.root, "status", "--porcelain"); dirty != "" {
		t.Fatalf("fault left the ledger dirty: %q", dirty)
	}

	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	result, err := ledger.ApplyDecision(store, input)
	if err != nil {
		t.Fatalf("retry after fault: %v", err)
	}
	if result.Status != ledger.DecisionApplied || result.State != ledger.ReadyForImplementation {
		t.Fatalf("retry after fault = %#v", result)
	}
}

// TestDecisionFindingDrivenImplementResumesRework checks the package route
// after a finding-driven pause. The CLI continuation journeys protect the
// Watchdog route and the unchanged-code review count.
func TestDecisionFindingDrivenImplementResumesRework(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "decision-review", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept decision-review")

	store := l.store()
	item := "decision-review/foundation"
	source := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget}
	review := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget, Reviewed: deliveryHead}

	implementOne := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, implementOne.Claim.Commit, source, "awaiting_review", "implementation one\n")
	watchdogOne := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.WatchdogPhase, watchdogOne.Claim.Commit, review, "rework", "W1 open.\n")

	// A finding-driven implementation pause resumes Rework, not a fresh start.
	implementTwo := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, implementTwo.Claim.Commit, source, "needs_human", "W1 needs human direction.\n")
	implementPause := ledger.Reference{Commit: l.head(), Path: deliveryReportPath("widgets", item, ledger.ImplementPhase)}
	findingResult, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: implementPause,
		Answer: "address W1 within the contract\n", Route: ledger.RouteImplement,
	})
	if err != nil || findingResult.Status != ledger.DecisionApplied || findingResult.State != ledger.Rework {
		t.Fatalf("finding-driven decision = %#v, %v", findingResult, err)
	}

	// A fresh Claim must use the Rework lane, not reset to initial work.
	continued := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	if continued.State.State != ledger.Rework || continued.State.Claim.Inputs.Decision == nil {
		t.Fatalf("finding-driven continuation = %#v, want Rework with direction", continued.State)
	}
}

// TestDecisionWatchdogRouteRequiresImplementationInputs covers B6/A2: review
// continuation is refused when the implementation report has no fixed source
// revisions, and the paused record is unchanged.
func TestDecisionWatchdogRouteRequiresImplementationInputs(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "decision-inputs", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept decision-inputs")

	store := l.store()
	item := "decision-inputs/foundation"
	request := decisionPauseImplement(t, l, store, deliveryWidgets(), item, ledger.SourceRevisions{})
	headBefore := l.head()
	result, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "reconsider the finding at the same code\n", Route: ledger.RouteWatchdog,
	})
	if err != nil {
		t.Fatalf("watchdog route returned error: %v", err)
	}
	if result.Status != ledger.DecisionRefused || !strings.Contains(result.Refusal, "implementation report") {
		t.Fatalf("missing implementation inputs = %#v", result)
	}
	if l.head() != headBefore {
		t.Fatal("refused review continuation mutated the ledger")
	}
	if state := l.committedState("widgets", "decision-inputs", "foundation"); state.State != ledger.NeedsHuman || state.Decision {
		t.Fatalf("refused review continuation changed the pause: %#v", state)
	}
}

// TestDecisionCoupledGroupRefusesRatherThanSplitting covers B7/A2: independent
// members may individually fail, while a coupled set is refused whole.
func TestDecisionCoupledGroupRefusesRatherThanSplitting(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addProject("gadgets", "acme/gadgets")
	l.addSlice("widgets", "coupled-work", "repair", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addSlice("gadgets", "coupled-work", "upgrade", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept coupled records")

	store := l.store()
	widgetRequest := decisionPauseImplement(t, l, store, deliveryWidgets(), "coupled-work/repair", ledger.SourceRevisions{})
	gadgetRequest := decisionPauseImplement(t, l, store, deliveryGadgets(), "coupled-work/upgrade", ledger.SourceRevisions{})
	// Replace the gadgets request so only the widgets member is answerable.
	raw := deliveryGitShow(t, l.root, "HEAD", gadgetRequest.Path)
	l.addFile(gadgetRequest.Path, raw+"# replaced\n")
	l.commitAll("replace gadget request")

	inputs := []ledger.DecisionInput{
		{Project: "widgets", Item: "coupled-work/repair", Request: widgetRequest, Answer: "continue widgets\n", Route: ledger.RouteImplement},
		{Project: "gadgets", Item: "coupled-work/upgrade", Request: gadgetRequest, Answer: "continue gadgets\n", Route: ledger.RouteImplement},
	}
	headBefore := l.head()
	if _, err := ledger.ApplyDecisions(store, inputs, true); err == nil {
		t.Fatal("coupled direction was split across a failed member")
	}
	if l.head() != headBefore {
		t.Fatal("coupled refusal mutated the ledger")
	}
	if state := l.committedState("widgets", "coupled-work", "repair"); state.Decision || state.State != ledger.NeedsHuman {
		t.Fatalf("coupled refusal applied a member: %#v", state)
	}

	results, err := ledger.ApplyDecisions(store, inputs, false)
	if err != nil {
		t.Fatalf("independent decisions: %v", err)
	}
	if len(results) != 2 || results[0].Status != ledger.DecisionApplied || results[1].Status != ledger.DecisionRefused {
		t.Fatalf("independent outcomes = %#v", results)
	}
}

// TestRetireProposalGuardsTerminalChildren covers B8/A4: supersession preserves
// merged history, retirement succeeds only for terminal unclaimed children,
// reports partial delivery, and refuses active or claimed work.
func TestRetireProposalGuardsTerminalChildren(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "legacy", "delivered", ledger.Merged, nil, "2021-01-01T00:00:00Z")
	l.addSlice("widgets", "legacy", "abandoned", ledger.ReadyForImplementation, nil, "2021-01-01T00:00:00Z")
	l.addSlice("widgets", "active-work", "blocked", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addSlice("widgets", "active-work", "gone", ledger.Superseded, nil, deliveryInitial)
	l.commitAll("accept retirement records")

	store := l.store()
	request := decisionPauseImplement(t, l, store, deliveryWidgets(), "legacy/abandoned", ledger.SourceRevisions{})
	superseded, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: "legacy/abandoned", Request: request,
		Answer: "abandon this slice\n", Route: ledger.RouteSupersede,
	})
	if err != nil || superseded.Status != ledger.DecisionApplied || superseded.State != ledger.Superseded {
		t.Fatalf("supersede = %#v, %v", superseded, err)
	}
	if state := l.committedState("widgets", "legacy", "delivered"); state.State != ledger.Merged {
		t.Fatalf("supersession changed merged history: %#v", state)
	}

	// Active work refuses retirement without abandoning the other child.
	headBefore := l.head()
	if _, err := ledger.RetireProposal(context.Background(), store, "widgets", "active-work", noRetirementForge(t)); err == nil || !strings.Contains(err.Error(), "active or claimed") {
		t.Fatalf("active retirement refusal = %v", err)
	}
	if l.head() != headBefore {
		t.Fatal("refused retirement mutated the ledger")
	}
	if state := l.committedState("widgets", "active-work", "gone"); state.State != ledger.Superseded {
		t.Fatalf("refused retirement abandoned another child: %#v", state)
	}

	// A claimed terminal child also refuses retirement.
	claimed := l.committedState("widgets", "active-work", "blocked")
	claimed.State = ledger.Merged
	claimed.Claim = &ledger.Claim{Phase: ledger.ImplementPhase, Basis: l.head()}
	l.commitState("widgets", "active-work", "blocked", claimed)
	if _, err := ledger.RetireProposal(context.Background(), store, "widgets", "active-work", noRetirementForge(t)); err == nil || !strings.Contains(err.Error(), "active or claimed") {
		t.Fatalf("claimed retirement refusal = %v", err)
	}

	retired, err := ledger.RetireProposal(context.Background(), store, "widgets", "legacy", noRetirementForge(t))
	if err != nil {
		t.Fatalf("retire legacy: %v", err)
	}
	if retired.Status != ledger.RetirementRetired || !retired.PartialDelivery {
		t.Fatalf("retirement result = %#v", retired)
	}
	if strings.Join(retired.Merged, ",") != "delivered" || strings.Join(retired.Superseded, ",") != "abandoned" {
		t.Fatalf("retirement children = %#v", retired)
	}
	var meta ledger.ProposalMeta
	raw := deliveryGitShow(t, l.root, "HEAD", "projects/widgets/proposals/legacy/proposal.json")
	if err := json.Unmarshal([]byte(raw), &meta); err != nil || !meta.Retired {
		t.Fatalf("proposal retirement marker = %#v, %v", meta, err)
	}
	again, err := ledger.RetireProposal(context.Background(), store, "widgets", "legacy", noRetirementForge(t))
	if err != nil || again.Status != ledger.RetirementAlreadyRetired {
		t.Fatalf("repeated retirement = %#v, %v", again, err)
	}
}
