package ledger_test

// Human Decision tests. They exercise the read-only Decision Inbox and the
// explicitly scoped decision against real local Git ledgers: ledger-wide and
// filtered scope, exact request matching across unrelated commits and
// replacement reports, atomic answer-plus-route commits, exact-retry
// recognition after later work, pre-commit fault atomicity, review-count
// preservation, and guarded retirement.
//
// Expected outcomes come from the accepted behavior rules (B1-B8) and ADR 0006,
// not from the implementation's own computations.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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

// TestDecisionInboxScopesProjectsAndDistinguishesEmpty covers B1/A1: the inbox
// is ledger-wide, filters explicitly, reports an unknown Project, and changes
// nothing.
func TestDecisionInboxScopesProjectsAndDistinguishesEmpty(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addProject("gadgets", "acme/gadgets")
	l.addProject("beacon", "acme/beacon")
	l.addSlice("widgets", "widget-work", "repair", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addSlice("widgets", "widget-work", "zz-ready", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addSlice("gadgets", "gadget-work", "upgrade", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addSlice("beacon", "quiet-work", "steady", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept inbox records")

	store := l.store()
	decisionPauseImplement(t, l, store, deliveryWidgets(), "widget-work/repair", ledger.SourceRevisions{})
	decisionPauseImplement(t, l, store, deliveryGadgets(), "gadget-work/upgrade", ledger.SourceRevisions{})
	head := l.head()

	all := decisionInbox(t, store, "")
	if len(all.Requests) != 2 {
		t.Fatalf("ledger-wide inbox returned %d requests, want 2", len(all.Requests))
	}
	items := []string{all.Requests[0].Project + "/" + all.Requests[0].Item, all.Requests[1].Project + "/" + all.Requests[1].Item}
	sort.Strings(items)
	if got := strings.Join(items, " "); got != "gadgets/gadget-work/upgrade widgets/widget-work/repair" {
		t.Fatalf("inbox items = %q", got)
	}
	repair := all.Requests[1]
	if repair.Phase != ledger.ImplementPhase || repair.Request.Path != deliveryReportPath("widgets", "widget-work/repair", ledger.ImplementPhase) {
		t.Fatalf("request facts = %#v", repair)
	}
	if repair.Report.Outcome != "needs_human" || len(repair.Contract) != 2 || repair.Repository != "acme/widgets" {
		t.Fatalf("inbox entry lacks accepted facts: %#v", repair)
	}
	if repair.Request.Commit != head {
		t.Fatalf("request commit = %s, want current head %s", repair.Request.Commit, head)
	}

	filtered := decisionInbox(t, store, "gadgets")
	if len(filtered.Requests) != 1 || filtered.Requests[0].Project != "gadgets" {
		t.Fatalf("filtered inbox = %#v", filtered.Requests)
	}
	if empty := decisionInbox(t, store, "beacon"); len(empty.Requests) != 0 {
		t.Fatalf("request-free project returned %d requests", len(empty.Requests))
	}
	if _, err := ledger.ReadDecisionInbox(store, ledger.InboxFilter{Project: "no-such-project"}); err == nil || !strings.Contains(err.Error(), "unknown Project") {
		t.Fatalf("unknown Project refusal = %v", err)
	}
	if l.head() != head {
		t.Fatal("reading the inbox changed the ledger")
	}
}

// TestDecisionAppliesImplementAtomically covers B3/B5/A2: an explicitly scoped
// implement answer commits decision.md and its route together, preserves frozen
// documents, and recognizes an exact retry while refusing a different replay.
func TestDecisionAppliesImplementAtomically(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "decision-flow", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept decision-flow")

	store := l.store()
	item := "decision-flow/foundation"
	request := decisionPauseImplement(t, l, store, deliveryWidgets(), item, ledger.SourceRevisions{})
	contractBefore := deliveryGitShow(t, l.root, "HEAD", deliveryContractPath("widgets", item, "behavior.md"))
	headBefore := l.head()

	result, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "take the in-contract option and implement it\n", Route: ledger.RouteImplement,
	})
	if err != nil {
		t.Fatalf("apply decision: %v", err)
	}
	if result.Status != ledger.DecisionApplied || result.State != ledger.ReadyForImplementation {
		t.Fatalf("apply result = %#v", result)
	}
	if result.Decision.Commit != l.head() || result.Decision.Path != deliveryDecisionPath("widgets", item) {
		t.Fatalf("decision reference = %#v", result.Decision)
	}
	if paths := deliveryCommitPaths(t, l.root, l.head()); len(paths) != 2 {
		t.Fatalf("decision commit changed %v, want decision.md and state.json together", paths)
	}
	record, answer, err := ledger.ParseDecision([]byte(deliveryGitShow(t, l.root, "HEAD", result.Decision.Path)))
	if err != nil {
		t.Fatalf("parse committed decision: %v", err)
	}
	if record.AnsweredRequest != request || record.Route != ledger.RouteImplement || record.Item != item || record.Project != "widgets" {
		t.Fatalf("committed decision metadata = %#v", record)
	}
	if !strings.Contains(answer, "in-contract option") {
		t.Fatalf("committed decision answer = %q", answer)
	}
	state := l.committedState("widgets", "decision-flow", "foundation")
	if !state.Decision || state.State != ledger.ReadyForImplementation || state.Claim != nil {
		t.Fatalf("committed route state = %#v", state)
	}
	if got := deliveryGitShow(t, l.root, "HEAD", deliveryContractPath("widgets", item, "behavior.md")); got != contractBefore {
		t.Fatal("decision rewrote the frozen contract")
	}
	if l.head() == headBefore {
		t.Fatal("decision did not commit")
	}

	retried, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "take the in-contract option and implement it\n", Route: ledger.RouteImplement,
	})
	if err != nil {
		t.Fatalf("retry decision: %v", err)
	}
	if retried.Status != ledger.DecisionAlreadyApplied || !retried.AlreadyApplied {
		t.Fatalf("exact retry = %#v", retried)
	}
	if l.head() != result.Decision.Commit {
		t.Fatal("exact retry recorded a second decision")
	}

	differing, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "actually supersede it\n", Route: ledger.RouteSupersede,
	})
	if err != nil {
		t.Fatalf("differing replay returned error: %v", err)
	}
	if differing.Status != ledger.DecisionRefused || !strings.Contains(differing.Refusal, "already recorded") {
		t.Fatalf("differing replay = %#v", differing)
	}
	if l.head() != result.Decision.Commit {
		t.Fatal("differing replay mutated the ledger")
	}
}

// TestDecisionRejectsReplacedRequestAndAllowsUnrelatedCommit covers B4/A2: an
// unrelated ledger commit keeps a request answerable, while a replacement with
// identical prose and a different recorded claim is refused.
func TestDecisionRejectsReplacedRequestAndAllowsUnrelatedCommit(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addProject("gadgets", "acme/gadgets")
	l.addSlice("widgets", "decision-stale", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept decision-stale")

	store := l.store()
	item := "decision-stale/foundation"
	execution := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, execution.Claim.Commit, ledger.SourceRevisions{}, "needs_human", "human judgment remains required.\n")
	request := ledger.Reference{Commit: l.head(), Path: deliveryReportPath("widgets", item, ledger.ImplementPhase)}

	// An unrelated Project records a result before the answer is applied.
	l.addFile("projects/gadgets/notes.txt", "unrelated\n")
	l.commitAll("unrelated ledger commit")

	result, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "continue within the contract\n", Route: ledger.RouteImplement,
	})
	if err != nil {
		t.Fatalf("unrelated commit invalidated the request: %v", err)
	}
	if result.Status != ledger.DecisionApplied {
		t.Fatalf("unrelated commit result = %#v", result)
	}

	// A replacement report repeats the prose with a different claim identity.
	l2 := newDeliveryLedger(t)
	l2.addProject("widgets", "acme/widgets")
	l2.addSlice("widgets", "decision-replaced", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l2.commitAll("accept decision-replaced")
	store2 := l2.store()
	item2 := "decision-replaced/foundation"
	execution2 := deliveryStart(t, store2, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store2, deliveryWidgets(), item2, ledger.ImplementPhase, execution2.Claim.Commit, ledger.SourceRevisions{}, "needs_human", "human judgment remains required.\n")
	viewed := ledger.Reference{Commit: l2.head(), Path: deliveryReportPath("widgets", item2, ledger.ImplementPhase)}
	raw := deliveryGitShow(t, l2.root, "HEAD", viewed.Path)
	replaced := strings.Replace(raw, execution2.Claim.Commit, strings.Repeat("a", 40), 1)
	if replaced == raw {
		t.Fatal("fixture did not alter the recorded claim identity")
	}
	l2.addFile(viewed.Path, replaced)
	l2.commitAll("replace request report")

	refused, err := ledger.ApplyDecision(store2, ledger.DecisionInput{
		Project: "widgets", Item: item2, Request: viewed,
		Answer: "continue within the contract\n", Route: ledger.RouteImplement,
	})
	if err != nil {
		t.Fatalf("replaced request returned error: %v", err)
	}
	if refused.Status != ledger.DecisionRefused || !strings.Contains(refused.Refusal, "replaced") {
		t.Fatalf("replaced request = %#v", refused)
	}
	if state := l2.committedState("widgets", "decision-replaced", "foundation"); state.State != ledger.NeedsHuman || state.Decision || state.Claim != nil {
		t.Fatalf("refusal changed the paused record: %#v", state)
	}
}

// TestDecisionRefusesActiveClaimAndDifferingReplay covers B4/A2: an active
// reservation and a differing replay are refused without mutation.
func TestDecisionRefusesActiveClaimAndDifferingReplay(t *testing.T) {
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

// TestDecisionRetryAfterLaterClaimAndResult covers B5/A2: the exact operation
// is recognized after a later Claim and a later result without altering either.
func TestDecisionRetryAfterLaterClaimAndResult(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "decision-retry", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept decision-retry")

	store := l.store()
	item := "decision-retry/foundation"
	request := decisionPauseImplement(t, l, store, deliveryWidgets(), item, ledger.SourceRevisions{})
	input := ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "continue within the contract\n", Route: ledger.RouteImplement,
	}
	result, err := ledger.ApplyDecision(store, input)
	if err != nil || result.Status != ledger.DecisionApplied {
		t.Fatalf("apply decision = %#v, %v", result, err)
	}

	// A later Claim must survive the exact retry untouched.
	later := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	claimPath := deliveryStatePath("widgets", item)
	claimedState := l.committedState("widgets", "decision-retry", "foundation")
	if claimedState.Claim == nil {
		t.Fatal("later Claim was not recorded")
	}
	retried, err := ledger.ApplyDecision(store, input)
	if err != nil {
		t.Fatalf("retry after Claim: %v", err)
	}
	if retried.Status != ledger.DecisionAlreadyApplied {
		t.Fatalf("retry after Claim = %#v", retried)
	}
	if state := l.committedState("widgets", "decision-retry", "foundation"); state.Claim == nil || state.Claim.Phase != ledger.ImplementPhase {
		t.Fatalf("retry released the later Claim: %#v", state.Claim)
	}

	// A later result keeps the recorded direction; the retry changes nothing.
	source := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget}
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, later.Claim.Commit, source, "awaiting_review", "implementation one\n")
	reportBefore := deliveryGitShow(t, l.root, "HEAD", deliveryReportPath("widgets", item, ledger.ImplementPhase))
	statePath := filepath.Join(l.root, filepath.FromSlash(claimPath))
	stateBefore := deliveryReadFile(t, statePath)
	retried, err = ledger.ApplyDecision(store, input)
	if err != nil {
		t.Fatalf("retry after result: %v", err)
	}
	if retried.Status != ledger.DecisionAlreadyApplied || retried.State != ledger.AwaitingReview {
		t.Fatalf("retry after result = %#v", retried)
	}
	if got := deliveryReadFile(t, statePath); got != stateBefore {
		t.Fatal("retry rewrote the later route state")
	}
	if got := deliveryGitShow(t, l.root, "HEAD", deliveryReportPath("widgets", item, ledger.ImplementPhase)); got != reportBefore {
		t.Fatal("retry rewrote the later report")
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

// TestDecisionWatchdogContinuationPreservesReviewBudget covers B6/A3: a
// finding-driven Implement decision resumes Rework, a Watchdog decision resumes
// Awaiting Review at the same code with the implementation inputs and recorded
// count, and the two-review budget is unchanged.
func TestDecisionWatchdogContinuationPreservesReviewBudget(t *testing.T) {
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

	implementThree := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, implementThree.Claim.Commit, source, "awaiting_review", "implementation three\n")
	watchdogTwo := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.WatchdogPhase, watchdogTwo.Claim.Commit, review, "rework", "W1 still open.\n")

	inbox := decisionInbox(t, store, "widgets")
	if len(inbox.Requests) != 1 || inbox.Requests[0].Phase != ledger.WatchdogPhase {
		t.Fatalf("paused review request = %#v", inbox.Requests)
	}
	request := inbox.Requests[0].Request

	// Review continuation at unchanged code resumes Awaiting Review with the
	// fixed implementation inputs and does not reset the count.
	result, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "reconsider W1 at the same code revision\n", Route: ledger.RouteWatchdog,
	})
	if err != nil || result.Status != ledger.DecisionApplied || result.State != ledger.AwaitingReview {
		t.Fatalf("watchdog decision = %#v, %v", result, err)
	}
	watchdogThree := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	if watchdogThree.State.Claim == nil || watchdogThree.State.Claim.Inputs.Decision == nil {
		t.Fatal("review continuation lost the recorded decision input")
	}
	if watchdogThree.Watchdog == nil || watchdogThree.Watchdog.Round != 2 {
		t.Fatalf("review count was reset: %#v", watchdogThree.Watchdog)
	}
	passed := deliveryHandoff(t, store, deliveryWidgets(), item, ledger.WatchdogPhase, watchdogThree.Claim.Commit, review, "pass", "W1 resolved.\n")
	if passed.Status != ledger.ReadyForMerge || passed.State.Decision {
		t.Fatalf("round 3 pass = %#v", passed)
	}
	report, _ := deliveryReport(t, l, "widgets", item, ledger.WatchdogPhase)
	if report.Round != 3 {
		t.Fatalf("completed review count = %d, want 3", report.Round)
	}
}

// TestDecisionAppliesLocallyDuringReplicationOutage covers B5/A2: an
// unavailable remote leaves the local decision applied and its replication
// reported as pending without requiring a forge.
func TestDecisionAppliesLocallyDuringReplicationOutage(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "decision-outage", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept decision-outage")
	deliveryGit(t, l.root, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing-upstream.git"))

	store := l.store()
	item := "decision-outage/foundation"
	request := decisionPauseImplement(t, l, store, deliveryWidgets(), item, ledger.SourceRevisions{})
	result, err := ledger.ApplyDecision(store, ledger.DecisionInput{
		Project: "widgets", Item: item, Request: request,
		Answer: "continue within the contract\n", Route: ledger.RouteImplement,
	})
	if err != nil || result.Status != ledger.DecisionApplied {
		t.Fatalf("outage decision = %#v, %v", result, err)
	}
	if result.Replication == nil || result.Replication.Status != ledger.PushPending {
		t.Fatalf("outage replication = %#v, want pending", result.Replication)
	}
	if state := l.committedState("widgets", "decision-outage", "foundation"); !state.Decision || state.State != ledger.ReadyForImplementation {
		t.Fatalf("outage changed the local result: %#v", state)
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
	if _, err := ledger.RetireProposal(store, "widgets", "active-work"); err == nil || !strings.Contains(err.Error(), "active or claimed") {
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
	if _, err := ledger.RetireProposal(store, "widgets", "active-work"); err == nil || !strings.Contains(err.Error(), "active or claimed") {
		t.Fatalf("claimed retirement refusal = %v", err)
	}

	retired, err := ledger.RetireProposal(store, "widgets", "legacy")
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
	again, err := ledger.RetireProposal(store, "widgets", "legacy")
	if err != nil || again.Status != ledger.RetirementAlreadyRetired {
		t.Fatalf("repeated retirement = %#v, %v", again, err)
	}
}
