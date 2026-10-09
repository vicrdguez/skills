package ledger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

const mergeProposalBranch = "proposal/" + deliveryPublicationSlice

// mergeForgeStub records merge requests and answers with commit or err.
type mergeForgeStub struct {
	requests []ledger.PullMerge
	commit   string
	err      error
}

func (f *mergeForgeStub) MergePull(_ context.Context, merge ledger.PullMerge) (string, error) {
	f.requests = append(f.requests, merge)
	return f.commit, f.err
}

// mergeFixture passes a review of the source head on a Slice targeting the
// open Proposal Branch, whose remote head is the reviewed head itself.
func mergeFixture(t *testing.T) (*deliveryLedger, *deliverySource, *ledger.Store) {
	t.Helper()
	l, source, store, _ := deliveryPublicationFixture(t)
	deliveryGit(t, source.root, "push", "-q", source.remote, source.head+":refs/heads/"+mergeProposalBranch, source.head+":refs/heads/"+deliveryPublicationBranch)
	// Fetch observations are kept under the remote's name.
	deliveryGit(t, source.root, "remote", "add", "origin", source.remote)
	source.remote = "origin"
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	state.Target = &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: mergeProposalBranch}
	l.commitState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, state)
	deliveryReview(t, store, source.head, "pass", deliveryPrivateBody)
	state = l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	state.Submission = &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 9}
	l.commitState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, state)
	mergeProposal(l, `,"integration_target":{"repository":"acme/widgets","branch":"`+mergeProposalBranch+`"}`)
	return l, source, store
}

func mergeProposal(l *deliveryLedger, facts string) {
	l.addFile("projects/widgets/proposals/"+deliveryPublicationSlice+"/proposal.json", `{"accepted":"`+deliveryInitial+`","branch":"`+mergeProposalBranch+`"`+facts+`}`)
	l.commitAll("Proposal facts")
}

func TestMergeReadyMergesAtTheFinalHeadWithoutWritingState(t *testing.T) {
	l, source, store := mergeFixture(t)
	if pending, err := ledger.PendingMerges(store, deliveryWidgets()); err != nil || len(pending) != 1 || pending[0] != deliveryPublicationItem {
		t.Fatalf("pending merges = %v, %v", pending, err)
	}
	before := deliveryGitOutput(t, l.root, "rev-parse", "HEAD")
	forge := &mergeForgeStub{commit: "squash"}
	attempt := ledger.MergeReady(t.Context(), store, deliveryWidgets(), source.root, source.remote, deliveryPublicationItem, forge)
	want := ledger.PullMerge{Number: 9, Branch: deliveryPublicationBranch, Head: source.head, Base: mergeProposalBranch}
	if attempt.Status != ledger.MergeMerged || attempt.Commit != "squash" || len(forge.requests) != 1 || forge.requests[0] != want {
		t.Fatalf("merge = %+v with requests %+v", attempt, forge.requests)
	}
	forge.err = errors.New("branch protection")
	if refused := ledger.MergeReady(t.Context(), store, deliveryWidgets(), source.root, source.remote, deliveryPublicationItem, forge); refused.Status != ledger.MergeRefused || refused.Reason == "" {
		t.Fatalf("forge refusal = %+v", refused)
	}
	if after := deliveryGitOutput(t, l.root, "rev-parse", "HEAD"); after != before {
		t.Fatal("a merge attempt wrote Workflow State")
	}
}

func TestMergeReadyRefusesWithoutAnOpenProposalBranch(t *testing.T) {
	for name, edit := range map[string]func(*deliveryLedger){
		"completed Proposal": func(l *deliveryLedger) {
			mergeProposal(l, `,"integration_target":{"repository":"acme/widgets","branch":"`+mergeProposalBranch+`"},"completion":{"target":{"repository":"acme/widgets","branch":"main"}}`)
		},
		"main target": func(l *deliveryLedger) {
			state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
			state.Target.Branch = "main"
			l.commitState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, state)
		},
	} {
		t.Run(name, func(t *testing.T) {
			l, source, store := mergeFixture(t)
			edit(l)
			if pending, err := ledger.PendingMerges(store, deliveryWidgets()); err != nil || len(pending) != 0 {
				t.Fatalf("pending merges = %v, %v", pending, err)
			}
			forge := &mergeForgeStub{commit: "squash"}
			if attempt := ledger.MergeReady(t.Context(), store, deliveryWidgets(), source.root, source.remote, deliveryPublicationItem, forge); attempt.Status != ledger.MergeRefused || len(forge.requests) != 0 {
				t.Fatalf("merge = %+v with requests %+v", attempt, forge.requests)
			}
		})
	}
}
