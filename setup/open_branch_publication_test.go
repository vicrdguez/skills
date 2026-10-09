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
