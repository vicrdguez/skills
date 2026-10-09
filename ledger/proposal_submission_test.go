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

func refreshProposal(t *testing.T, l *deliveryLedger, merged bool, forge *proposalForgeStub) *ledger.ProposalRefresh {
	t.Helper()
	refreshed, err := ledger.RefreshProposal(context.Background(), l.store(), deliveryWidgets(), "feature", merged, forge.get)
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
	refreshed := refreshProposal(t, l, true, forge)
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
	refreshProposal(t, l, true, forge)
	recorded := l.commitCount()

	mergeIntoBranch(l, "ui")
	l.commitAll("ui merged")
	refreshProposal(t, l, true, forge)
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
	refreshProposal(t, l, true, forge)
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
	refreshProposal(t, l, true, forge)
	if len(forge.presentations) != 1 || forge.presentations[0].Approved {
		t.Fatalf("Superseded member readied the Proposal: %+v", forge.presentations)
	}
}

func TestPresentationFailureIsReportedAndRetriedUntilAttached(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.AwaitingReview})
	forge := &proposalForgeStub{presentErr: errors.New("forge unavailable")}
	before := l.commitCount()
	failed := refreshProposal(t, l, true, forge)
	if failed.Presentation == nil || failed.Presentation.Status != ledger.IssuePending || l.commitCount() != before {
		t.Fatalf("failed presentation = %+v", failed)
	}
	forge.presentErr, forge.number = nil, 41
	retried := refreshProposal(t, l, false, forge)
	if retried.Presentation == nil || retried.Presentation.Status != ledger.PullPresented || proposalMeta(t, l).Submission.Number != 41 {
		t.Fatalf("retry = %+v", retried)
	}
	// Attached and no new Slice merge: observation only.
	forge.observed = ledger.SubmissionObservation{State: "open"}
	if quiet := refreshProposal(t, l, false, forge); quiet.Presentation != nil || len(forge.presentations) != 2 {
		t.Fatalf("observation without a Slice merge presented: %+v", quiet)
	}
}

func TestNoForgeBeforeAnyBranchMerge(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.AwaitingReview})
	refreshed, err := ledger.RefreshProposal(context.Background(), l.store(), deliveryWidgets(), "feature", false, func() (ledger.ProposalForge, error) {
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
	refreshProposal(t, l, true, forge)
	forge.observed = ledger.SubmissionObservation{State: ledger.Merged, SourceHead: "branch-head", MergeCommit: "merge-commit"}
	completed := refreshProposal(t, l, false, forge)
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
	if again, err := ledger.RefreshProposal(context.Background(), l.store(), deliveryWidgets(), "feature", true, forge.get); err != nil || again != nil {
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
	refreshProposal(t, l, true, forge)
	before := l.commitCount()
	forge.observed = ledger.SubmissionObservation{State: "closed", SourceHead: "branch-head"}
	closed := refreshProposal(t, l, true, forge)
	if !closed.ClosedUnmerged || closed.Commit != "" || l.commitCount() != before || len(forge.presentations) != 1 {
		t.Fatalf("closure = %+v", closed)
	}
	summary := proposalMeta(t, l)
	if summary.Submission == nil || summary.Submission.Number != 40 || summary.Completion != nil || summary.BranchState != ledger.BranchOpen || summary.FullyDelivered {
		t.Fatalf("closure changed the Proposal: %+v", summary)
	}
	// A reopened pull request is refreshed at the next observed Slice merge.
	forge.observed = ledger.SubmissionObservation{State: "open"}
	if reopened := refreshProposal(t, l, true, forge); reopened.Presentation == nil || forge.presentations[1].Number != 40 {
		t.Fatalf("reopened refresh = %+v", reopened)
	}
}

func TestRetiringOpenProposalSpendsItsBranch(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.Merged})
	store := l.store()
	retired, err := ledger.RetireProposal(store, "widgets", "feature")
	if err != nil || retired.Status != ledger.RetirementRetired {
		t.Fatalf("retire open Proposal = %+v, %v", retired, err)
	}
	summary := proposalMeta(t, l)
	if !summary.Retired || summary.BranchState != ledger.BranchSpent || summary.Completion != nil || !summary.FullyDelivered {
		t.Fatalf("retired Proposal = %+v", summary)
	}
	if again, err := ledger.RefreshProposal(context.Background(), store, deliveryWidgets(), "feature", true, (&proposalForgeStub{}).get); err != nil || again != nil {
		t.Fatalf("retired Proposal refreshed: %+v, %v", again, err)
	}
	archived, err := ledger.ArchiveTerminalProposals(context.Background(), store, deliveryWidgets())
	if err != nil || len(archived.Archived) != 1 {
		t.Fatalf("cleanup after retirement = %+v, %v", archived, err)
	}
}

func TestRetiringOpenProposalRefusedWhileSliceActive(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.AwaitingReview})
	if _, err := ledger.RetireProposal(l.store(), "widgets", "feature"); err == nil || !strings.Contains(err.Error(), "still has active or claimed work: ui") {
		t.Fatalf("retirement with an active Slice = %v", err)
	}
	if proposalMeta(t, l).Retired {
		t.Fatal("refused retirement was recorded")
	}
}

func TestBrowseShowsProposalBranchSubmissionAndSliceMergeTargets(t *testing.T) {
	l := openProposalLedger(t, map[string]string{"api": ledger.Merged, "ui": ledger.Merged, "docs": ledger.AwaitingReview})
	refreshProposal(t, l, true, &proposalForgeStub{number: 40})
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
