package ledger_test

import (
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestRefreshSelectsOnlyCommittedCoherentFactsAndKeepsDamagedRecordsLocal(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "base", ledger.ReadyForImplementation, nil, deliveryInitial)
	first := l.commitAll("initial")
	pinned := browseSnapshot(t, l)

	l.addSlice("widgets", "orders", "later", ledger.ReadyForImplementation, nil, deliveryInitial)
	unchanged, err := pinned.Refresh()
	if err != nil || unchanged.Revision != first {
		t.Fatalf("uncommitted changes became current: %v, %+v", err, unchanged)
	}

	l.addSlice("widgets", "orders", "broken", ledger.Merged, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "orders/broken"), "{not json")
	l.writeStateValue("widgets", "orders", "base", ledger.SliceState{State: ledger.Merged, Title: "base", Branch: "base"})
	second := l.commitAll("advance with damaged sibling")
	before := browseObservable(t, l)
	current, err := pinned.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != second || pinned.Revision != first {
		t.Fatalf("refresh changed pinned revision or missed current one: %s, %s, %s", current.Revision, pinned.Revision, second)
	}
	inventory, err := current.Project("widgets", false)
	if err != nil || inventory.Revision != second || inventory.Project.Slices != 3 || inventory.Project.Unknown != 1 || !inventory.Project.Incomplete {
		t.Fatalf("new inventory mixed facts or concealed damage: %+v, %v", inventory, err)
	}
	proposal, err := current.Proposal("widgets", "orders")
	if err != nil || len(proposal.Slices) != 3 || proposal.Slices[0].Lifecycle != ledger.Merged || proposal.Slices[1].Readable {
		t.Fatalf("new membership or facts differ: %+v, %v", proposal, err)
	}
	original, err := pinned.Proposal("widgets", "orders")
	if err != nil || len(original.Slices) != 1 || original.Slices[0].Lifecycle != ledger.ReadyForImplementation {
		t.Fatalf("earlier view changed: %+v, %v", original, err)
	}
	if after := browseObservable(t, l); after != before {
		t.Fatalf("refresh modified ledger refs or worktree:\n%s\nwant:\n%s", after, before)
	}
}
