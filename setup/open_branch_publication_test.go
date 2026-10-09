package setup

import (
	"context"
	"net/http"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestPullPublicationCreatesAndMatchesProposalBranchBase(t *testing.T) {
	forge := newDeliveryForge(t)
	server := forge.server()
	defer server.Close()
	presentation := ledger.PullPresentation{Title: "feature", Body: "Public feature", Branch: "widget", Head: "aaa", Base: "proposal/feature"}
	backend := deliveryBackend(server)
	number, err := backend.PresentPull(context.Background(), presentation)
	if err != nil || number != 11 {
		t.Fatalf("create on Proposal Branch = %d, %v", number, err)
	}
	if got := forge.pull(number); got == nil || got.Base != "proposal/feature" {
		t.Fatalf("pull request base = %+v", got)
	}
	again, err := backend.PresentPull(context.Background(), presentation)
	if err != nil || again != number || forge.count(http.MethodPost, "/repos/acme/widgets/pulls") != 1 {
		t.Fatalf("present existing Proposal Branch pull = %d, %v", again, err)
	}
}

func TestUnpinnedProposalPresentationAttachesHumanPullAndKeepsItReady(t *testing.T) {
	forge := newDeliveryForge(t)
	forge.add(deliveryPull{Number: 7, Title: "human title", Body: "human body", Draft: false, Branch: "proposal/feature", Head: "moved-by-merges"})
	server := forge.server()
	defer server.Close()
	presentation := ledger.PullPresentation{Title: "Feature work", Body: "Rendered body", Branch: "proposal/feature", Base: "main", Unpinned: true}
	number, err := deliveryBackend(server).PresentPull(context.Background(), presentation)
	if err != nil || number != 7 {
		t.Fatalf("attach human Proposal pull = %d, %v", number, err)
	}
	if forge.count(http.MethodPost, "/repos/acme/widgets/pulls") != 0 {
		t.Fatal("a second Proposal pull request was created")
	}
	if pull := forge.pull(7); pull.Body != "Rendered body" || pull.Draft || len(forge.mutations()) != 0 {
		t.Fatalf("attached pull = %+v, mutations %v", pull, forge.mutations())
	}
}

func TestUnpinnedProposalPresentationCreatesDraftThenReadies(t *testing.T) {
	forge := newDeliveryForge(t)
	server := forge.server()
	defer server.Close()
	backend := deliveryBackend(server)
	presentation := ledger.PullPresentation{Title: "Feature work", Body: "Rendered body", Branch: "proposal/feature", Base: "main", Unpinned: true}
	number, err := backend.PresentPull(context.Background(), presentation)
	if err != nil || forge.pull(number) == nil || !forge.pull(number).Draft || forge.pull(number).Base != "main" {
		t.Fatalf("create Proposal draft = %d, %v, %+v", number, err, forge.pull(number))
	}
	presentation.Number, presentation.Approved = number, true
	if again, err := backend.PresentPull(context.Background(), presentation); err != nil || again != number || forge.pull(number).Draft {
		t.Fatalf("ready Proposal pull = %d, %v, %+v", again, err, forge.pull(number))
	}
}
