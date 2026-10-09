package ledger_test

import (
	"context"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestPresentationUsesRecordedProposalBranchAsBaseAndAttachment(t *testing.T) {
	l, source, store, _ := deliveryPublicationFixture(t)
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	state.Target = &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "proposal/delivery-public"}
	l.commitState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, state)
	selected, err := ledger.SelectCurrentResult(store, deliveryWidgets(), deliveryPublicationItem)
	if err != nil {
		t.Fatal(err)
	}
	forge := &deliveryForgeStub{number: 22}
	presented := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, selected, "Public result", forge)
	if presented.Publication.Status != ledger.PullPresented {
		t.Fatalf("presentation = %+v", presented.Publication)
	}
	calls := forge.calls()
	if len(calls) != 1 || calls[0].Base != "proposal/delivery-public" {
		t.Fatalf("pull request base = %+v", calls)
	}
	recorded := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if recorded.Submission == nil || recorded.Submission.Number != 22 || recorded.Target == nil || recorded.Target.Branch != "proposal/delivery-public" {
		t.Fatalf("attached Submission = %+v", recorded)
	}
}
