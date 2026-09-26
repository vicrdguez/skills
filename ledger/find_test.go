package ledger_test

// Slice selection seam tests. They read real local Git ledger fixtures
// through the public Snapshot.FindSlices query. Expected selections are
// listed by hand from the fixture records and the accepted find-slices
// behavior (B1–B4), never recomputed from the query's criteria.

import (
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// items lists every matched Slice as project:proposal/slice in result order,
// and the undecided ones separately.
func items(search *ledger.SliceSearch) (matched, undecided []string) {
	for _, project := range search.Projects {
		for _, group := range project.Groups {
			for _, slice := range group.Slices {
				matched = append(matched, project.Name+":"+slice.Item)
			}
		}
		for _, slice := range project.Undecided {
			undecided = append(undecided, project.Name+":"+slice.Item)
		}
	}
	return matched, undecided
}

func find(t *testing.T, snapshot *ledger.Snapshot, query ledger.SliceQuery) *ledger.SliceSearch {
	t.Helper()
	search, err := snapshot.FindSlices(query)
	if err != nil {
		t.Fatalf("find %+v: %v", query, err)
	}
	return search
}

func wantItems(t *testing.T, search *ledger.SliceSearch, matched, undecided string) {
	t.Helper()
	gotMatched, gotUndecided := items(search)
	if strings.Join(gotMatched, " ") != matched || strings.Join(gotUndecided, " ") != undecided {
		t.Fatalf("query %+v matched [%s] undecided [%s], want [%s] and [%s]",
			search.Query, strings.Join(gotMatched, " "), strings.Join(gotUndecided, " "), matched, undecided)
	}
	if search.Matched != len(gotMatched) || search.Undecided != len(gotUndecided) {
		t.Fatalf("counts %d/%d disagree with listed results %v %v", search.Matched, search.Undecided, gotMatched, gotUndecided)
	}
}

// claimsLedger records three Awaiting Review Slices in widgets/review — one
// watchdog-claimed, one unclaimed, one with an unreadable state record — plus
// an implementation-claimed Rework Slice and, in gadgets, another watchdog
// reservation.
func claimsLedger(t *testing.T) *deliveryLedger {
	t.Helper()
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addProject("gadgets", "acme/gadgets")
	l.addSlice("widgets", "review", "watched", ledger.AwaitingReview, nil, deliveryInitial)
	l.writeStateValue("widgets", "review", "watched", ledger.SliceState{State: ledger.AwaitingReview, Title: "watched", Branch: "watched",
		Claim: &ledger.Claim{Phase: ledger.WatchdogPhase, Basis: deliveryRefID}})
	l.addSlice("widgets", "review", "idle", ledger.AwaitingReview, nil, deliveryInitial)
	l.addSlice("widgets", "review", "damaged", ledger.AwaitingReview, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "review/damaged"), "{not json")
	l.addSlice("widgets", "build", "coding", ledger.Rework, nil, deliveryInitial)
	l.writeStateValue("widgets", "build", "coding", ledger.SliceState{State: ledger.Rework, Title: "coding", Branch: "coding",
		Claim: &ledger.Claim{Phase: ledger.ImplementPhase, Basis: deliveryRefID}})
	l.addSlice("gadgets", "tools", "hammer", ledger.AwaitingReview, nil, deliveryInitial)
	l.writeStateValue("gadgets", "tools", "hammer", ledger.SliceState{State: ledger.AwaitingReview, Title: "hammer", Branch: "hammer",
		Claim: &ledger.Claim{Phase: ledger.WatchdogPhase, Basis: deliveryRefID}})
	l.commitAll("record claims")
	return l
}

func TestFindSlicesKeepsClaimIndependentOfLifecycle(t *testing.T) {
	l := claimsLedger(t)
	before := browseObservable(t, l)
	snapshot := browseSnapshot(t, l)

	watchdog := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Claims: []string{ledger.WatchdogPhase}})
	wantItems(t, watchdog, "widgets:review/watched", "widgets:review/damaged")
	if !watchdog.Incomplete || !watchdog.Projects[0].Incomplete || len(watchdog.Projects[0].Undecided[0].Diagnostics) == 0 {
		t.Fatalf("the unreadable Claim must stay diagnosed and mark the result incomplete: %+v", watchdog)
	}

	unclaimed := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Claims: []string{ledger.ClaimNone}})
	wantItems(t, unclaimed, "widgets:review/idle", "widgets:review/damaged")

	implementation := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Claims: []string{ledger.ImplementPhase}})
	wantItems(t, implementation, "widgets:build/coding", "widgets:review/damaged")

	awaiting := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Lifecycles: []string{ledger.AwaitingReview}})
	wantItems(t, awaiting, "widgets:review/idle widgets:review/watched", "widgets:review/damaged")

	everywhere := find(t, snapshot, ledger.SliceQuery{Claims: []string{ledger.WatchdogPhase}})
	wantItems(t, everywhere, "gadgets:tools/hammer widgets:review/watched", "widgets:review/damaged")

	facets := find(t, snapshot, ledger.SliceQuery{Project: "widgets"}).Facets
	if facets.Lifecycles[ledger.AwaitingReview] != 2 || facets.Lifecycles[ledger.Rework] != 1 || facets.UnknownLifecycle != 1 ||
		facets.Claims[ledger.WatchdogPhase] != 1 || facets.Claims[ledger.ImplementPhase] != 1 || facets.Claims[ledger.ClaimNone] != 1 || facets.UnknownClaim != 1 {
		t.Fatalf("scope facets = %+v", facets)
	}
	narrowed := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Lifecycles: []string{ledger.AwaitingReview}}).Facets
	if narrowed.Claims[ledger.WatchdogPhase] != 1 || narrowed.Claims[ledger.ImplementPhase] != 0 || narrowed.Claims[ledger.ClaimNone] != 1 || narrowed.Lifecycles[ledger.Rework] != 1 {
		t.Fatalf("each facet must count what selecting it would match alongside the other criteria: %+v", narrowed)
	}
	if after := browseObservable(t, l); after != before {
		t.Fatalf("finding slices changed the ledger:\n%s\nwant\n%s", after, before)
	}
}

func TestFindSlicesCombinesTextWithStructuredCriteria(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addProject("gadgets", "acme/gadgets")
	l.addSlice("widgets", "refunds", "partial", ledger.NeedsHuman, nil, deliveryInitial)
	l.addSlice("widgets", "refunds", "full", ledger.ReadyForMerge, nil, deliveryInitial)
	l.addSlice("widgets", "orders", "cancel", ledger.NeedsHuman, nil, deliveryInitial)
	l.writeStateValue("widgets", "orders", "cancel", ledger.SliceState{State: ledger.NeedsHuman, Title: "Refund on cancel", Branch: "cancel"})
	l.addSlice("widgets", "orders", "ship", ledger.NeedsHuman, nil, deliveryInitial)
	l.addSlice("gadgets", "refunds", "store-credit", ledger.NeedsHuman, nil, deliveryInitial)
	l.addSlice("widgets", "legacy", "refund-v1", ledger.NeedsHuman, nil, deliveryInitial)
	l.commitAll("record refunds")
	l.addFile("projects/widgets/archive/.keep", "")
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/legacy", "projects/widgets/archive/legacy")
	l.commitAll("archive legacy")
	snapshot := browseSnapshot(t, l)

	combined := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Text: "REFUND", Lifecycles: []string{ledger.NeedsHuman}})
	wantItems(t, combined, "widgets:orders/cancel widgets:refunds/partial", "")
	if combined.Incomplete || len(combined.Projects) != 1 {
		t.Fatalf("a readable scope must yield a complete result: %+v", combined)
	}

	archived := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Text: "refund", Lifecycles: []string{ledger.NeedsHuman}, IncludeArchived: true})
	wantItems(t, archived, "widgets:orders/cancel widgets:refunds/partial widgets:legacy/refund-v1", "")
	if group := archived.Projects[0].Groups[2]; group.Proposal != "legacy" || !group.Archived {
		t.Fatalf("archived matches must stay marked archived: %+v", group)
	}

	identity := find(t, snapshot, ledger.SliceQuery{Text: "gadgets/refunds"})
	wantItems(t, identity, "gadgets:refunds/store-credit", "")

	empty := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Text: "invoice"})
	if empty.Matched != 0 || empty.Undecided != 0 || empty.Incomplete || len(empty.Projects) != 1 {
		t.Fatalf("a genuinely empty readable result must be complete: %+v", empty)
	}
}

func TestFindSlicesDisclosesUnknownFactsItCannotDecide(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "cancel", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "orders", "future", ledger.Merged, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "orders/future"), `{"state": "deployed", "title": "future", "branch": "future"}`)
	l.addSlice("widgets", "orders", "odd", ledger.Merged, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "orders/odd"), `{"state": "merged", "title": "odd", "branch": "odd", "claim": {"phase": "deploy", "basis": "`+deliveryRefID+`"}}`)
	l.addSlice("widgets", "orders", "broken", ledger.Merged, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "orders/broken"), "{")
	l.addFile("projects/widgets/proposals/orders/Bad Name/state.json", `{"state": "merged"}`)
	l.commitAll("record unknown facts")
	snapshot := browseSnapshot(t, l)

	merged := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Lifecycles: []string{ledger.Merged}})
	wantItems(t, merged, "widgets:orders/cancel widgets:orders/odd", "widgets:orders/broken widgets:orders/future")
	unclaimed := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Claims: []string{ledger.ClaimNone}})
	wantItems(t, unclaimed, "widgets:orders/cancel widgets:orders/future", "widgets:orders/broken widgets:orders/odd")
	titled := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Text: "cancel"})
	wantItems(t, titled, "widgets:orders/cancel", "widgets:orders/broken")

	everything := find(t, snapshot, ledger.SliceQuery{Project: "widgets"})
	wantItems(t, everything, "widgets:orders/broken widgets:orders/cancel widgets:orders/future widgets:orders/odd", "")
	if !everything.Incomplete || len(everything.Projects[0].Diagnostics) != 1 || !strings.Contains(everything.Projects[0].Diagnostics[0].Problem, "Bad Name") {
		t.Fatalf("an unreadable member must keep the result incomplete: %+v", everything)
	}
}

func TestFindSlicesDisclosesUnknownMembership(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "cancel", ledger.Merged, nil, deliveryInitial)
	l.addProject("gadgets", "acme/gadgets")
	l.addSlice("gadgets", "tools", "hammer", ledger.Merged, nil, deliveryInitial)
	l.addProject("sprockets", "acme/sprockets")
	l.addSlice("sprockets", "gears", "cog", ledger.Merged, nil, deliveryInitial)
	l.addFile("projects/Not Valid/project.json", `{"repository": "acme/not-valid"}`)
	l.addFile("projects/gadgets/proposals/Odd_Proposal/cog/state.json", `{"state": "merged"}`)
	l.addFile("projects/sprockets/proposals/empty/proposal.json", "{}")
	l.commitAll("record unknown membership")
	snapshot := browseSnapshot(t, l)

	widgets := find(t, snapshot, ledger.SliceQuery{Project: "widgets", Lifecycles: []string{ledger.Merged}})
	if widgets.Incomplete || widgets.Matched != 1 {
		t.Fatalf("a readable Project scope must stay complete beside damaged Projects: %+v", widgets)
	}
	for _, project := range []string{"gadgets", "sprockets"} {
		scoped := find(t, snapshot, ledger.SliceQuery{Project: project, Lifecycles: []string{ledger.Merged}})
		if !scoped.Incomplete || !scoped.Projects[0].Incomplete || len(scoped.Projects[0].Diagnostics) != 1 || scoped.Matched != 1 {
			t.Fatalf("%s: unknown membership must keep healthy matches and mark the result incomplete: %+v", project, scoped)
		}
	}
	everywhere := find(t, snapshot, ledger.SliceQuery{Text: "nothing matches this"})
	if everywhere.Matched != 0 || !everywhere.Incomplete || len(everywhere.Diagnostics) != 1 || everywhere.Diagnostics[0].Scope != ledger.ScopeLedger || len(everywhere.Projects) != 2 {
		t.Fatalf("an invalid Project name must keep a ledger-wide empty result incomplete: %+v", everywhere)
	}
}

func TestFindSlicesGroupsTheSameFacts(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addProject("gadgets", "acme/gadgets")
	l.addSlice("widgets", "orders", "cancel", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "orders", "refund", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addSlice("widgets", "billing", "invoice", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "billing", "broken", ledger.Merged, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "billing/broken"), "{")
	l.addSlice("gadgets", "tools", "hammer", ledger.Rework, nil, deliveryInitial)
	l.commitAll("record")
	snapshot := browseSnapshot(t, l)

	shape := func(search *ledger.SliceSearch) string {
		var parts []string
		for _, project := range search.Projects {
			for _, group := range project.Groups {
				key := group.Proposal + group.Lifecycle
				if group.UnknownLifecycle {
					key = "unknown"
				}
				var members []string
				for _, slice := range group.Slices {
					members = append(members, slice.Item)
				}
				parts = append(parts, project.Name+"/"+key+"="+strings.Join(members, ","))
			}
		}
		return strings.Join(parts, " ")
	}
	byProposal := find(t, snapshot, ledger.SliceQuery{})
	if got := shape(byProposal); got != "gadgets/tools=tools/hammer widgets/billing=billing/broken,billing/invoice widgets/orders=orders/cancel,orders/refund" {
		t.Fatalf("proposal grouping = %s", got)
	}
	byLifecycle := find(t, snapshot, ledger.SliceQuery{GroupBy: ledger.GroupByLifecycle})
	if got := shape(byLifecycle); got != "gadgets/rework=tools/hammer widgets/ready_for_implementation=orders/refund widgets/merged=billing/invoice,orders/cancel widgets/unknown=billing/broken" {
		t.Fatalf("lifecycle grouping = %s", got)
	}
	if byProposal.Query.GroupBy != ledger.GroupByProposal || byLifecycle.Matched != byProposal.Matched {
		t.Fatalf("grouping must not change the selection: %+v %+v", byProposal.Query, byLifecycle.Query)
	}
}

func TestFindSlicesDescribesOneRevisionAndRefusesUnknownSelections(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "cancel", ledger.NeedsHuman, nil, deliveryInitial)
	revision := l.commitAll("record")
	snapshot := browseSnapshot(t, l)
	l.addSlice("widgets", "orders", "refund", ledger.NeedsHuman, nil, deliveryInitial)
	l.commitAll("advance")
	l.writeStateValue("widgets", "orders", "cancel", ledger.SliceState{State: ledger.Merged, Title: "cancel", Branch: "cancel"})

	search := find(t, snapshot, ledger.SliceQuery{Lifecycles: []string{ledger.NeedsHuman}})
	if search.Revision != revision {
		t.Fatalf("revision = %s, want %s", search.Revision, revision)
	}
	wantItems(t, search, "widgets:orders/cancel", "")

	for name, query := range map[string]ledger.SliceQuery{
		"project":   {Project: "gadgets"},
		"lifecycle": {Lifecycles: []string{"deployed"}},
		"claim":     {Claims: []string{"deploy"}},
		"grouping":  {GroupBy: "owner"},
	} {
		if _, err := snapshot.FindSlices(query); err == nil {
			t.Fatalf("unknown %s selection was not refused", name)
		}
	}
}
