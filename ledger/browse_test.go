package ledger_test

// Browsing query seam tests. They read real local Git ledger fixtures through
// the public Snapshot queries. Expected facts come from the fixture records
// and the accepted browse-records behavior (B1, B3, B4), not from the query
// implementation.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func browseSnapshot(t *testing.T, l *deliveryLedger) *ledger.Snapshot {
	t.Helper()
	snapshot, err := l.store().Snapshot()
	if err != nil {
		t.Fatalf("select snapshot: %v", err)
	}
	return snapshot
}

func browseObservable(t *testing.T, l *deliveryLedger) string {
	t.Helper()
	return deliveryGitOutput(t, l.root, "for-each-ref") + "\n--\n" + deliveryGitOutput(t, l.root, "status", "--porcelain", "--untracked-files=all")
}

func TestBrowseKeepsLifecycleAndWatchdogClaimIndependent(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "base", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "orders", "cancel", ledger.AwaitingReview, []string{"proposals/orders/base"}, deliveryInitial)
	l.addFile("projects/widgets/proposals/orders/proposal.json",
		`{"accepted": "`+deliveryInitial+`", "parent_title": "Order cancellation", "parent_issue": {"repository": "acme/widgets", "number": 10}}`+"\n")
	l.addFile(deliveryReportPath("widgets", "orders/cancel", ledger.ImplementPhase), "implement report\n")
	l.addFile(deliveryContractPath("widgets", "orders/cancel", "plan.md"), "plan\n")
	l.writeStateValue("widgets", "orders", "cancel", ledger.SliceState{
		State: ledger.AwaitingReview, Title: "Cancel orders", Branch: "feat/cancel",
		Dependencies: []string{"proposals/orders/base"},
		Issue:        &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 11},
		Submission:   &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 12},
		Target:       &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "main"},
		Claim:        &ledger.Claim{Phase: ledger.WatchdogPhase, Basis: deliveryRefID},
	})
	revision := l.commitAll("accept orders")
	before := browseObservable(t, l)

	snapshot := browseSnapshot(t, l)
	slice, err := snapshot.Slice("widgets", "orders/cancel")
	if err != nil {
		t.Fatal(err)
	}
	if !slice.Readable || slice.Lifecycle != ledger.AwaitingReview || slice.Claim == nil || slice.Claim.Phase != ledger.WatchdogPhase || slice.Claim.Basis != deliveryRefID {
		t.Fatalf("lifecycle and Claim must both be available independently: %+v claim %+v", slice, slice.Claim)
	}
	if slice.Revision != revision || slice.Title != "Cancel orders" || slice.Branch != "feat/cancel" || slice.Repository != "acme/widgets" {
		t.Fatalf("recorded identity facts differ: %+v", slice)
	}
	if len(slice.Dependencies) != 1 || slice.Dependencies[0] != (ledger.DependencyFact{RelatedSlice: ledger.RelatedSlice{Item: "orders/base", Recorded: true, Title: "base", Lifecycle: ledger.Merged}, Satisfied: true}) {
		t.Fatalf("dependency facts = %+v, want orders/base Merged", slice.Dependencies)
	}
	if slice.Issue.Number != 11 || slice.Submission.Number != 12 || slice.ParentIssue.Number != 10 || slice.Target.Branch != "main" {
		t.Fatalf("forge attachments differ: issue %+v submission %+v parent %+v target %+v", slice.Issue, slice.Submission, slice.ParentIssue, slice.Target)
	}
	if strings.Join(slice.Documents, ",") != "behavior.md,intent.md,plan.md" || strings.Join(slice.Reports, ",") != "implement" {
		t.Fatalf("documents %v reports %v", slice.Documents, slice.Reports)
	}

	proposal, err := snapshot.Proposal("widgets", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Proposal.ParentTitle != "Order cancellation" || proposal.Proposal.Slices != 2 || proposal.Proposal.Claimed != 1 || proposal.Proposal.FullyDelivered {
		t.Fatalf("proposal summary = %+v", proposal.Proposal)
	}
	if len(proposal.Slices) != 2 || proposal.Slices[1].Lifecycle != ledger.AwaitingReview || proposal.Slices[1].ClaimPhase != ledger.WatchdogPhase {
		t.Fatalf("proposal membership = %+v", proposal.Slices)
	}
	overview, err := snapshot.Overview(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Projects) != 1 || overview.Projects[0].Repository != "acme/widgets" || overview.Projects[0].Lifecycles[ledger.Merged] != 1 || overview.Projects[0].Lifecycles[ledger.AwaitingReview] != 1 {
		t.Fatalf("overview = %+v", overview.Projects)
	}
	if after := browseObservable(t, l); after != before {
		t.Fatalf("browsing changed the ledger:\n%s\nwant\n%s", after, before)
	}
}

func TestBrowseViewDescribesOneCommittedRevision(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "base", ledger.ReadyForImplementation, nil, deliveryInitial)
	revision := l.commitAll("accept orders")
	snapshot := browseSnapshot(t, l)

	l.addSlice("widgets", "orders", "later", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addSlice("widgets", "billing", "invoice", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.writeStateValue("widgets", "orders", "base", ledger.SliceState{State: ledger.Merged, Title: "base", Branch: "base"})
	advanced := l.commitAll("advance the ledger")
	l.writeStateValue("widgets", "orders", "base", ledger.SliceState{State: ledger.Superseded, Title: "base", Branch: "base"})

	inventory, err := snapshot.Project("widgets", false)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Revision != revision || len(inventory.Proposals) != 1 || inventory.Proposals[0].Slices != 1 {
		t.Fatalf("membership mixed revisions: %+v", inventory)
	}
	proposal, err := snapshot.Proposal("widgets", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Slices) != 1 || proposal.Slices[0].Lifecycle != ledger.ReadyForImplementation {
		t.Fatalf("facts mixed revisions: %+v", proposal.Slices)
	}
	if _, err := snapshot.Proposal("widgets", "billing"); err == nil {
		t.Fatal("a Proposal committed after the selected revision was returned")
	}

	current := browseSnapshot(t, l)
	slice, err := current.Slice("widgets", "orders/base")
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != advanced || slice.Lifecycle != ledger.Merged {
		t.Fatalf("a new view read %s lifecycle %s, want committed %s Merged, never the uncommitted edit", current.Revision, slice.Lifecycle, advanced)
	}
}

func TestBrowseIsolatesUnreadableRecords(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "healthy", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "orders", "damaged", ledger.Merged, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "orders/damaged"), "{not json")
	l.addFile(deliveryContractPath("widgets", "orders/stateless", "intent.md"), "intent\n")
	l.addSlice("widgets", "orders", "future", ledger.Merged, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "orders/future"), `{"state": "deployed", "title": "future", "branch": "future"}`)
	l.addSlice("widgets", "billing", "invoice", ledger.Rework, nil, deliveryInitial)
	l.addFile("projects/widgets/proposals/billing/proposal.json", "[")
	l.addSlice("gadgets", "tools", "hammer", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.addFile("projects/gadgets/project.json", "{")
	l.commitAll("record damaged records")

	snapshot := browseSnapshot(t, l)
	inventory, err := snapshot.Project("widgets", false)
	if err != nil {
		t.Fatal(err)
	}
	if !inventory.Project.Incomplete || inventory.Project.Unknown != 3 || inventory.Project.Lifecycles[ledger.Merged] != 1 || inventory.Project.Lifecycles[ledger.Rework] != 1 {
		t.Fatalf("project summary must disclose the unknown slices: %+v", inventory.Project)
	}
	proposal, err := snapshot.Proposal("widgets", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Proposal.FullyDelivered || !proposal.Proposal.Incomplete || proposal.Proposal.Slices != 4 || proposal.Proposal.Unknown != 3 {
		t.Fatalf("proposal summary claims completeness it cannot know: %+v", proposal.Proposal)
	}
	diagnosed := map[string]bool{}
	for _, diagnostic := range proposal.Proposal.Diagnostics {
		diagnosed[diagnostic.Subject] = diagnostic.Scope == ledger.ScopeSlice
	}
	for _, subject := range []string{"widgets/orders/damaged", "widgets/orders/stateless", "widgets/orders/future"} {
		if !diagnosed[subject] {
			t.Fatalf("slice %s is not diagnosed at slice scope: %+v", subject, proposal.Proposal.Diagnostics)
		}
	}
	healthy, err := snapshot.Slice("widgets", "orders/healthy")
	if err != nil || !healthy.Readable || healthy.Lifecycle != ledger.Merged || len(healthy.Diagnostics) != 0 {
		t.Fatalf("healthy slice = %+v, %v", healthy, err)
	}
	damaged, err := snapshot.Slice("widgets", "orders/damaged")
	if err != nil {
		t.Fatal(err)
	}
	if damaged.Readable || damaged.Lifecycle != "" || damaged.Claim != nil || len(damaged.Diagnostics) == 0 {
		t.Fatalf("damaged slice must be unreadable and diagnosed, not unclaimed: %+v", damaged)
	}

	billing, err := snapshot.Proposal("widgets", "billing")
	if err != nil || len(billing.Slices) != 1 || len(billing.Proposal.Diagnostics) == 0 {
		t.Fatalf("billing = %+v, %v", billing, err)
	}
	if !billing.Proposal.Incomplete || billing.Slices[0].Lifecycle != ledger.Rework || billing.Proposal.Diagnostics[0].Scope != ledger.ScopeProposal {
		t.Fatalf("damaged proposal metadata must be diagnosed without hiding members: %+v", billing)
	}
	overview, err := snapshot.Overview(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Projects) != 2 {
		t.Fatalf("overview = %+v", overview.Projects)
	}
	gadgets := overview.Projects[0]
	if gadgets.Name != "gadgets" || gadgets.Repository != "" || !gadgets.Incomplete || gadgets.Lifecycles[ledger.ReadyForImplementation] != 1 {
		t.Fatalf("damaged project identity must be diagnosed while its slices stay browsable: %+v", gadgets)
	}
}

func TestBrowseArchivedProposalsOnExplicitSelection(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "cancel", ledger.ReadyForImplementation, []string{"legacy/shipped"}, deliveryInitial)
	l.addSlice("widgets", "legacy", "shipped", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "legacy", "dropped", ledger.Superseded, nil, deliveryInitial)
	l.addFile("projects/widgets/proposals/legacy/proposal.json", `{"accepted": "`+deliveryInitial+`", "retired": true}`+"\n")
	l.commitAll("accept")
	if err := os.MkdirAll(filepath.Join(l.root, "projects", "widgets", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/legacy", "projects/widgets/archive/legacy")
	l.commitAll("archive legacy")

	snapshot := browseSnapshot(t, l)
	ordinary, err := snapshot.Project("widgets", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordinary.Proposals) != 1 || ordinary.Proposals[0].Name != "orders" || ordinary.Project.ArchivedProposals != 1 || ordinary.Project.Slices != 1 {
		t.Fatalf("ordinary inventory must exclude archived proposals: %+v", ordinary)
	}
	everything, err := snapshot.Project("widgets", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(everything.Proposals) != 2 || everything.Project.Slices != 3 {
		t.Fatalf("include-archived inventory = %+v", everything)
	}
	archived := everything.Proposals[1]
	if archived.Name != "legacy" || !archived.Archived || !archived.Retired || archived.FullyDelivered || archived.Lifecycles[ledger.Superseded] != 1 {
		t.Fatalf("archival and retirement must not read as full delivery: %+v", archived)
	}
	slice, err := snapshot.Slice("widgets", "legacy/shipped")
	if err != nil || !slice.Archived || !slice.ProposalRetired || slice.Lifecycle != ledger.Merged {
		t.Fatalf("archived slice keeps its identity and lifecycle: %+v, %v", slice, err)
	}
	dependent, err := snapshot.Slice("widgets", "orders/cancel")
	if err != nil || len(dependent.Dependencies) != 1 || dependent.Dependencies[0] != (ledger.DependencyFact{RelatedSlice: ledger.RelatedSlice{Item: "legacy/shipped", Recorded: true, Archived: true, Title: "shipped", Lifecycle: ledger.Merged}, Satisfied: true}) {
		t.Fatalf("dependency on an archived blocker = %+v, %v", dependent, err)
	}
}

func TestBrowseRefusesUnknownSelectionsAndUnreadableLedger(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "cancel", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept")
	snapshot := browseSnapshot(t, l)
	for name, query := range map[string]func() error{
		"project":  func() error { _, err := snapshot.Project("gadgets", false); return err },
		"proposal": func() error { _, err := snapshot.Proposal("widgets", "billing"); return err },
		"slice":    func() error { _, err := snapshot.Slice("widgets", "orders/refund"); return err },
		"identity": func() error { _, err := snapshot.Slice("widgets", "orders"); return err },
	} {
		if err := query(); err == nil {
			t.Fatalf("unknown %s selection was not refused", name)
		}
	}

	store := l.store()
	if err := os.RemoveAll(filepath.Join(l.root, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Snapshot(); err == nil {
		t.Fatal("an unreadable ledger must block browsing")
	}
}

func TestBrowseCountsInvalidRecordNamesAsUnknown(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "shipped", ledger.Merged, nil, deliveryInitial)
	l.addFile("projects/widgets/proposals/orders/Bad Name/state.json", `{"state": "merged", "title": "bad", "branch": "bad"}`)
	l.addFile("projects/widgets/proposals/Odd_Proposal/proposal.json", "{}")
	l.addFile("projects/Not Valid/project.json", `{"repository": "acme/not-valid"}`)
	l.commitAll("record invalid names")

	snapshot := browseSnapshot(t, l)
	proposal, err := snapshot.Proposal("widgets", "orders")
	if err != nil {
		t.Fatal(err)
	}
	summary := proposal.Proposal
	if summary.FullyDelivered || !summary.Incomplete || summary.Slices != 2 || summary.Unknown != 1 || summary.Lifecycles[ledger.Merged] != 1 {
		t.Fatalf("an invalid-named member must count as unknown, never as delivered: %+v", summary)
	}
	overview, err := snapshot.Overview(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Projects) != 1 || !overview.Projects[0].Incomplete || len(overview.Diagnostics) != 1 || overview.Diagnostics[0].Scope != ledger.ScopeLedger {
		t.Fatalf("invalid Project and Proposal names must be diagnosed: %+v", overview)
	}
}

func related(item string, archived bool, lifecycle string) ledger.RelatedSlice {
	_, title, _ := strings.Cut(item, "/")
	return ledger.RelatedSlice{Item: item, Recorded: true, Archived: archived, Title: title, Lifecycle: lifecycle}
}

func TestBrowseRelatesDependenciesInBothDirectionsAcrossProposals(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "base", ledger.ReadyForMerge, nil, deliveryInitial)
	l.addSlice("widgets", "orders", "cancel", ledger.ReadyForImplementation, []string{"proposals/orders/base", "proposals/legacy/shipped", "proposals/legacy/dropped"}, deliveryInitial)
	l.addSlice("widgets", "billing", "invoice", ledger.Merged, []string{"proposals/orders/base"}, deliveryInitial)
	l.addSlice("widgets", "billing", "unrelated", ledger.Rework, nil, deliveryInitial)
	l.addSlice("widgets", "legacy", "shipped", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "legacy", "dropped", ledger.Superseded, nil, deliveryInitial)
	l.commitAll("accept")
	if err := os.MkdirAll(filepath.Join(l.root, "projects", "widgets", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, l.root, "mv", "projects/widgets/proposals/legacy", "projects/widgets/archive/legacy")
	revision := l.commitAll("archive legacy")
	before := browseObservable(t, l)
	snapshot := browseSnapshot(t, l)

	cancel, err := snapshot.Slice("widgets", "orders/cancel")
	if err != nil {
		t.Fatal(err)
	}
	want := []ledger.DependencyFact{
		{RelatedSlice: related("orders/base", false, ledger.ReadyForMerge)},
		{RelatedSlice: related("legacy/shipped", true, ledger.Merged), Satisfied: true},
		{RelatedSlice: related("legacy/dropped", true, ledger.Superseded)},
	}
	if len(cancel.Dependencies) != len(want) {
		t.Fatalf("dependencies = %+v, want %+v", cancel.Dependencies, want)
	}
	for index := range want {
		if cancel.Dependencies[index] != want[index] {
			t.Fatalf("dependency %d = %+v, want %+v: only a Merged blocker satisfies it", index, cancel.Dependencies[index], want[index])
		}
	}

	base, err := snapshot.Slice("widgets", "orders/base")
	if err != nil {
		t.Fatal(err)
	}
	blocked := []ledger.RelatedSlice{related("billing/invoice", false, ledger.Merged), related("orders/cancel", false, ledger.ReadyForImplementation)}
	if base.Revision != revision || base.Blocks.Incomplete || len(base.Blocks.Slices) != 2 || base.Blocks.Slices[0] != blocked[0] || base.Blocks.Slices[1] != blocked[1] {
		t.Fatalf("orders/base blocks = %+v at %s, want %+v at %s with each lifecycle as recorded", base.Blocks, base.Revision, blocked, revision)
	}
	if base.Lifecycle != ledger.ReadyForMerge {
		t.Fatalf("a relation changed the blocker's recorded lifecycle: %s", base.Lifecycle)
	}

	shipped, err := snapshot.Slice("widgets", "legacy/shipped")
	if err != nil {
		t.Fatal(err)
	}
	if !shipped.Archived || len(shipped.Blocks.Slices) != 1 || shipped.Blocks.Slices[0] != related("orders/cancel", false, ledger.ReadyForImplementation) {
		t.Fatalf("an archived blocker keeps its reverse relationship: %+v", shipped.Blocks)
	}
	unrelated, err := snapshot.Slice("widgets", "billing/unrelated")
	if err != nil || len(unrelated.Dependencies) != 0 || len(unrelated.Blocks.Slices) != 0 || unrelated.Blocks.Incomplete {
		t.Fatalf("an unrelated slice has no relationships: %+v, %v", unrelated, err)
	}
	if after := browseObservable(t, l); after != before {
		t.Fatalf("querying relationships changed the ledger:\n%s\nwant\n%s", after, before)
	}
}

func TestBrowseDisclosesUnresolvedAndIncompleteRelationships(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "orders", "base", ledger.Merged, nil, deliveryInitial)
	l.addSlice("widgets", "orders", "damaged", ledger.Merged, nil, deliveryInitial)
	l.addFile(deliveryStatePath("widgets", "orders/damaged"), "{not json")
	l.addSlice("widgets", "orders", "cancel", ledger.ReadyForImplementation, []string{"proposals/orders/base", "proposals/orders/damaged", "proposals/gone/missing"}, deliveryInitial)
	l.addFile("projects/widgets/proposals/orders/Bad Name/state.json", `{"state": "merged", "title": "bad", "branch": "bad"}`)
	l.commitAll("record damaged relationships")
	snapshot := browseSnapshot(t, l)

	cancel, err := snapshot.Slice("widgets", "orders/cancel")
	if err != nil {
		t.Fatal(err)
	}
	if len(cancel.Dependencies) != 3 || cancel.Dependencies[0] != (ledger.DependencyFact{RelatedSlice: related("orders/base", false, ledger.Merged), Satisfied: true}) {
		t.Fatalf("the healthy dependency stays resolved: %+v", cancel.Dependencies)
	}
	damaged, missing := cancel.Dependencies[1], cancel.Dependencies[2]
	if damaged.Item != "orders/damaged" || !damaged.Recorded || damaged.Lifecycle != "" || damaged.Problem == "" || damaged.Satisfied {
		t.Fatalf("an unreadable blocker must stay an unknown, unsatisfied edge: %+v", damaged)
	}
	if missing.Item != "gone/missing" || missing.Recorded || missing.Problem == "" || missing.Satisfied {
		t.Fatalf("a missing blocker must stay an unresolved, unsatisfied edge: %+v", missing)
	}

	base, err := snapshot.Slice("widgets", "orders/base")
	if err != nil {
		t.Fatal(err)
	}
	if !base.Blocks.Incomplete || len(base.Blocks.Slices) != 1 || base.Blocks.Slices[0].Item != "orders/cancel" {
		t.Fatalf("reverse dependencies must list what is readable and disclose the rest: %+v", base.Blocks)
	}
	diagnosed := map[string]bool{}
	for _, diagnostic := range base.Blocks.Diagnostics {
		diagnosed[diagnostic.Subject] = true
	}
	if !diagnosed["widgets/orders/damaged"] || !diagnosed["widgets/orders"] {
		t.Fatalf("the unreadable state and the invalid-named member must both be disclosed: %+v", base.Blocks.Diagnostics)
	}

	unreadable, err := snapshot.Slice("widgets", "orders/damaged")
	if err != nil {
		t.Fatal(err)
	}
	if unreadable.Readable || len(unreadable.Blocks.Slices) != 1 || unreadable.Blocks.Slices[0].Item != "orders/cancel" {
		t.Fatalf("an unreadable slice still shows the recorded slices it blocks: %+v", unreadable.Blocks)
	}
}
