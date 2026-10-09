package ledger_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// proposalForgeStub stands in for the forge at the Proposal Submission seams:
// it answers the attached pull request's state and records presentations.
type proposalForgeStub struct {
	observed      ledger.SubmissionObservation
	observations  []ledger.IntegrationTarget
	presentations []ledger.PullPresentation
	number        int
	presentErr    error
}

func (f *proposalForgeStub) ObserveSubmission(_ context.Context, _ ledger.ForgeAttachment, target ledger.IntegrationTarget, _ string) (ledger.SubmissionObservation, error) {
	f.observations = append(f.observations, target)
	return f.observed, nil
}

func (f *proposalForgeStub) PresentPull(_ context.Context, presentation ledger.PullPresentation) (int, error) {
	f.presentations = append(f.presentations, presentation)
	return f.number, f.presentErr
}

func (f *proposalForgeStub) get() (ledger.ProposalForge, error) { return f, nil }

func (f *proposalForgeStub) retire(string) (ledger.CompletionForge, error) { return f, nil }

// noRetirementForge fails a retirement that has no Proposal Submission to
// observe yet contacts the forge.
func noRetirementForge(t *testing.T) func(string) (ledger.CompletionForge, error) {
	return func(string) (ledger.CompletionForge, error) {
		t.Error("retirement contacted the forge without a Proposal Submission to observe")
		return nil, errors.New("no forge")
	}
}

const proposalJSON = `{"accepted":"` + deliveryInitial + `","branch":"proposal/feature","parent_title":"Feature work","integration_target":{"repository":"acme/widgets","branch":"proposal/feature"}}`

// openProposalLedger accepts proposal feature with an open Proposal Branch and
// the given Slice lifecycles; a Merged Slice merged into the branch.
func openProposalLedger(t *testing.T, states map[string]string) *deliveryLedger {
	t.Helper()
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	for slice, state := range states {
		l.addSlice("widgets", "feature", slice, state, nil, deliveryInitial)
		if state == ledger.Merged {
			mergeIntoBranch(l, slice)
		}
	}
	l.addFile("projects/widgets/proposals/feature/proposal.json", proposalJSON)
	l.commitAll("accept feature")
	return l
}

func mergeIntoBranch(l *deliveryLedger, slice string) {
	branch := ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "proposal/feature"}
	l.writeStateValue("widgets", "feature", slice, ledger.SliceState{
		State: ledger.Merged, Title: slice, Branch: slice, Target: &branch,
		Submission: &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 30},
		Completion: &ledger.TerminalEvidence{Submission: ledger.ForgeAttachment{Repository: "acme/widgets", Number: 30}, Target: branch},
	})
}

func proposalMeta(t *testing.T, l *deliveryLedger) ledger.ProposalSummary {
	t.Helper()
	view, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	detail, err := view.Proposal("widgets", "feature")
	if err != nil {
		t.Fatal(err)
	}
	return detail.Proposal
}

func refreshProposal(t *testing.T, l *deliveryLedger, forge *proposalForgeStub) *ledger.ProposalRefresh {
	t.Helper()
	refreshed, err := ledger.RefreshProposal(context.Background(), l.store(), deliveryWidgets(), "feature", forge.get)
	if err != nil {
		t.Fatalf("refresh Proposal: %v", err)
	}
	return refreshed
}

func TestRenderProposalBodyListsSlicesAndClosesIssues(t *testing.T) {
	branch := ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "proposal/feature"}
	body := ledger.RenderProposalBody("# Feature work\n\nWhy it matters.\n", []ledger.ProposalSlice{
		{Name: "api", State: ledger.SliceState{State: ledger.Merged, Title: "Add the API", Issue: &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 4}, Submission: &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 31}, Completion: &ledger.TerminalEvidence{Target: branch}}},
		{Name: "ui", State: ledger.SliceState{State: ledger.AwaitingReview, Title: "Show it", Issue: &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 5}, Submission: &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 32}}},
		{Name: "docs", State: ledger.SliceState{State: ledger.ReadyForImplementation, Title: "Document it"}},
	})
	want := "# Feature work\n\nWhy it matters.\n\n## Slices\n\n" +
		"- Add the API (`api`): Merged into `proposal/feature`, acme/widgets#31\n" +
		"- Show it (`ui`): Awaiting Review, acme/widgets#32\n" +
		"- Document it (`docs`): Ready for Implementation\n" +
		"\nCloses #4\nCloses #5\n"
	if body != want {
		t.Fatalf("body =\n%s\nwant\n%s", body, want)
	}
}

func TestFirstBranchMergePresentsDraftAndRecordsSubmission(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.AwaitingReview})
	forge := &proposalForgeStub{number: 40}
	refreshed := refreshProposal(t, l, forge)
	if refreshed.Presentation == nil || refreshed.Presentation.Status != ledger.PullPresented || len(forge.presentations) != 1 {
		t.Fatalf("presentation = %+v, calls %+v", refreshed, forge.presentations)
	}
	got := forge.presentations[0]
	if got.Number != 0 || got.Branch != "proposal/feature" || got.Base != "main" || got.Title != "Feature work" || got.Approved || !got.Unpinned {
		t.Fatalf("Proposal presentation = %+v", got)
	}
	if !strings.Contains(got.Body, "proposal feature") || !strings.Contains(got.Body, "Merged into `proposal/feature`") || !strings.Contains(got.Body, "(`ui`): Awaiting Review") {
		t.Fatalf("Proposal body = %q", got.Body)
	}
	if summary := proposalMeta(t, l); summary.Submission == nil || summary.Submission.Number != 40 || summary.BranchState != ledger.BranchOpen || summary.Completion != nil {
		t.Fatalf("recorded Proposal = %+v", summary)
	}
}

func TestLaterBranchMergeRefreshesBodyAndLastMarksReady(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.AwaitingReview, "docs": ledger.AwaitingReview})
	forge := &proposalForgeStub{number: 40, observed: ledger.SubmissionObservation{State: "open"}}
	refreshProposal(t, l, forge)
	recorded := l.commitCount()

	mergeIntoBranch(l, "ui")
	l.commitAll("ui merged")
	refreshProposal(t, l, forge)
	second := forge.presentations[1]
	if second.Number != 40 || second.Approved || !strings.Contains(second.Body, "(`api`): Merged") || !strings.Contains(second.Body, "(`ui`): Merged into `proposal/feature`, acme/widgets#30") {
		t.Fatalf("refresh = %+v", second)
	}
	if forge.observations[0] != (ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "main"}) {
		t.Fatalf("observed against %+v", forge.observations)
	}

	mergeIntoBranch(l, "docs")
	l.commitAll("docs merged")
	before := l.commitCount()
	refreshProposal(t, l, forge)
	if last := forge.presentations[2]; !last.Approved || last.Number != 40 {
		t.Fatalf("last merge presentation = %+v", last)
	}
	if l.commitCount() != before || before != recorded+2 {
		t.Fatalf("refresh recorded something beyond the first Submission")
	}
}

func TestSupersededSliceKeepsProposalDraft(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.Superseded})
	forge := &proposalForgeStub{number: 40}
	refreshProposal(t, l, forge)
	if len(forge.presentations) != 1 || forge.presentations[0].Approved {
		t.Fatalf("Superseded member readied the Proposal: %+v", forge.presentations)
	}
}

func TestPresentationFailureIsReportedAndRetriedAtNextObservation(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.AwaitingReview})
	forge := &proposalForgeStub{presentErr: errors.New("forge unavailable")}
	before := l.commitCount()
	failed := refreshProposal(t, l, forge)
	if failed.Presentation == nil || failed.Presentation.Status != ledger.IssuePending || l.commitCount() != before {
		t.Fatalf("failed presentation = %+v", failed)
	}
	forge.presentErr, forge.number = nil, 41
	retried := refreshProposal(t, l, forge)
	if retried.Presentation == nil || retried.Presentation.Status != ledger.PullPresented || proposalMeta(t, l).Submission.Number != 41 {
		t.Fatalf("retry = %+v", retried)
	}
	// The last Slice lands while the forge fails; the next observation, with
	// no further Slice merge, still readies the attached Submission.
	mergeIntoBranch(l, "ui")
	l.commitAll("ui merged")
	forge.observed, forge.presentErr = ledger.SubmissionObservation{State: "open"}, errors.New("forge unavailable")
	if failed := refreshProposal(t, l, forge); failed.Presentation == nil || failed.Presentation.Status != ledger.IssuePending {
		t.Fatalf("failed ready presentation = %+v", failed)
	}
	forge.presentErr = nil
	if ready := refreshProposal(t, l, forge); ready.Presentation == nil || ready.Presentation.Status != ledger.PullPresented {
		t.Fatalf("ready retry = %+v", ready)
	}
	if last := forge.presentations[len(forge.presentations)-1]; !last.Approved || last.Number != 41 || !strings.Contains(last.Body, "(`ui`): Merged into `proposal/feature`") {
		t.Fatalf("retried presentation = %+v", last)
	}
}

func TestNoForgeBeforeAnyBranchMerge(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.AwaitingReview})
	refreshed, err := ledger.RefreshProposal(context.Background(), l.store(), deliveryWidgets(), "feature", func() (ledger.ProposalForge, error) {
		t.Fatal("forge requested before any Slice merged into the branch")
		return nil, nil
	})
	if err != nil || refreshed != nil {
		t.Fatalf("refresh = %+v, %v", refreshed, err)
	}
}

func TestMergedProposalSubmissionRecordsCompletion(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged})
	forge := &proposalForgeStub{number: 40}
	refreshProposal(t, l, forge)
	forge.observed = ledger.SubmissionObservation{State: ledger.Merged, SourceHead: "branch-head", MergeCommit: "merge-commit"}
	completed := refreshProposal(t, l, forge)
	if completed.Commit == "" || len(forge.presentations) != 1 {
		t.Fatalf("completion = %+v", completed)
	}
	summary := proposalMeta(t, l)
	if summary.Completion == nil || summary.Completion.MergeCommit != "merge-commit" || summary.Completion.Target.Branch != "main" || summary.Completion.Submission.Number != 40 {
		t.Fatalf("completion record = %+v", summary.Completion)
	}
	if summary.BranchState != ledger.BranchSpent || !summary.FullyDelivered {
		t.Fatalf("completed Proposal = %+v", summary)
	}
	if again, err := ledger.RefreshProposal(context.Background(), l.store(), deliveryWidgets(), "feature", forge.get); err != nil || again != nil {
		t.Fatalf("spent branch refreshed again: %+v, %v", again, err)
	}
	archived, err := ledger.ArchiveTerminalProposals(context.Background(), l.store(), deliveryWidgets())
	if err != nil || len(archived.Archived) != 1 || !archived.Archived[0].FullyDelivered {
		t.Fatalf("cleanup after completion = %+v, %v", archived, err)
	}
}

func TestClosedProposalSubmissionIsReportedOnly(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged})
	forge := &proposalForgeStub{number: 40}
	refreshProposal(t, l, forge)
	before := l.commitCount()
	forge.observed = ledger.SubmissionObservation{State: "closed", SourceHead: "branch-head"}
	closed := refreshProposal(t, l, forge)
	if !closed.ClosedUnmerged || closed.Commit != "" || l.commitCount() != before || len(forge.presentations) != 1 {
		t.Fatalf("closure = %+v", closed)
	}
	summary := proposalMeta(t, l)
	if summary.Submission == nil || summary.Submission.Number != 40 || summary.Completion != nil || summary.BranchState != ledger.BranchOpen || summary.FullyDelivered {
		t.Fatalf("closure changed the Proposal: %+v", summary)
	}
	// A reopened pull request is refreshed at the next observed Slice merge.
	forge.observed = ledger.SubmissionObservation{State: "open"}
	if reopened := refreshProposal(t, l, forge); reopened.Presentation == nil || forge.presentations[1].Number != 40 {
		t.Fatalf("reopened refresh = %+v", reopened)
	}
}

func TestRetiringOpenProposalSpendsItsBranch(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.Merged})
	store := l.store()
	retired, err := ledger.RetireProposal(context.Background(), store, "widgets", "feature", noRetirementForge(t))
	if err != nil || retired.Status != ledger.RetirementRetired {
		t.Fatalf("retire open Proposal = %+v, %v", retired, err)
	}
	summary := proposalMeta(t, l)
	if !summary.Retired || summary.BranchState != ledger.BranchSpent || summary.Completion != nil || !summary.FullyDelivered {
		t.Fatalf("retired Proposal = %+v", summary)
	}
	if again, err := ledger.RefreshProposal(context.Background(), store, deliveryWidgets(), "feature", (&proposalForgeStub{}).get); err != nil || again != nil {
		t.Fatalf("retired Proposal refreshed: %+v, %v", again, err)
	}
	archived, err := ledger.ArchiveTerminalProposals(context.Background(), store, deliveryWidgets())
	if err != nil || len(archived.Archived) != 1 {
		t.Fatalf("cleanup after retirement = %+v, %v", archived, err)
	}
}

func TestRetiringOpenProposalRefusedWhileSliceActive(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.AwaitingReview})
	if _, err := ledger.RetireProposal(context.Background(), l.store(), "widgets", "feature", noRetirementForge(t)); err == nil || !strings.Contains(err.Error(), "still has active or claimed work: ui") {
		t.Fatalf("retirement with an active Slice = %v", err)
	}
	if proposalMeta(t, l).Retired {
		t.Fatal("refused retirement was recorded")
	}
}

func TestRetiringProposalWithClosedSubmissionSpendsItsBranch(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged})
	forge := &proposalForgeStub{number: 40}
	refreshProposal(t, l, forge)
	forge.observed = ledger.SubmissionObservation{State: "closed", SourceHead: "branch-head"}
	retired, err := ledger.RetireProposal(context.Background(), l.store(), "widgets", "feature", forge.retire)
	if err != nil || retired.Status != ledger.RetirementRetired {
		t.Fatalf("retire with a closed Submission = %+v, %v", retired, err)
	}
	if len(forge.observations) != 1 || forge.observations[0].Branch != "main" {
		t.Fatalf("retirement observed %v, want the Submission into main once", forge.observations)
	}
	summary := proposalMeta(t, l)
	if !summary.Retired || summary.BranchState != ledger.BranchSpent || summary.Completion != nil || summary.Submission == nil {
		t.Fatalf("retired Proposal = %+v", summary)
	}
}

// A human may merge the Proposal Submission and run decision retire before
// any status observes the merge; the merge still completes the Proposal.
func TestRetiringMergedProposalSubmissionIsRefusedAndCompletionStillObserved(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged})
	forge := &proposalForgeStub{number: 40}
	refreshProposal(t, l, forge)
	forge.observed = ledger.SubmissionObservation{State: ledger.Merged, SourceHead: "branch-head", MergeCommit: "proposal-merge"}
	before := l.commitCount()
	if _, err := ledger.RetireProposal(context.Background(), l.store(), "widgets", "feature", forge.retire); err == nil || !strings.Contains(err.Error(), "has merged") {
		t.Fatalf("retire with a merged Submission = %v", err)
	}
	if l.commitCount() != before || proposalMeta(t, l).Retired {
		t.Fatal("refused retirement was recorded")
	}
	refreshProposal(t, l, forge)
	if summary := proposalMeta(t, l); summary.Retired || summary.Completion == nil || summary.Completion.MergeCommit != "proposal-merge" {
		t.Fatalf("Proposal after the merge was observed = %+v", summary)
	}
	if _, err := ledger.RetireProposal(context.Background(), l.store(), "widgets", "feature", noRetirementForge(t)); err == nil || !strings.Contains(err.Error(), "recorded completion") {
		t.Fatalf("retire a completed Proposal = %v", err)
	}
}

func TestRetiringProposalWithUnobservableSubmissionIsRefused(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged})
	refreshProposal(t, l, &proposalForgeStub{number: 40})
	unreachable := func(string) (ledger.CompletionForge, error) { return nil, errors.New("forge unreachable") }
	if _, err := ledger.RetireProposal(context.Background(), l.store(), "widgets", "feature", unreachable); err == nil || !strings.Contains(err.Error(), "could not be observed") {
		t.Fatalf("retire with an unobservable Submission = %v", err)
	}
	if proposalMeta(t, l).Retired {
		t.Fatal("refused retirement was recorded")
	}
}

func TestBrowseShowsProposalBranchSubmissionAndSliceMergeTargets(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.Merged, "docs": ledger.AwaitingReview})
	refreshProposal(t, l, &proposalForgeStub{number: 40})
	view, err := l.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	detail, err := view.Proposal("widgets", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Proposal.Branch != "proposal/feature" || detail.Proposal.BranchState != ledger.BranchOpen || detail.Proposal.Submission == nil || detail.Proposal.Submission.Number != 40 {
		t.Fatalf("Proposal facts = %+v", detail.Proposal)
	}
	merged := map[string]string{}
	for _, slice := range detail.Slices {
		merged[slice.Slice] = slice.MergedInto
	}
	if merged["api"] != "proposal/feature" || merged["ui"] != "proposal/feature" || merged["docs"] != "" {
		t.Fatalf("Slice merge targets = %v", merged)
	}
}
