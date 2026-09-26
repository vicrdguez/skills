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
	if len(slice.Dependencies) != 1 || slice.Dependencies[0] != (ledger.DependencyFact{Item: "orders/base", Lifecycle: ledger.Merged}) {
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
	if err != nil {
		t.Fatal(err)
	}
	if !billing.Proposal.Incomplete || billing.Slices[0].Lifecycle != ledger.Rework || billing.Proposal.Diagnostics[0].Scope != ledger.ScopeProposal {
		t.Fatalf("damaged proposal metadata must be diagnosed without hiding members: %+v", billing)
	}
	overview, err := snapshot.Overview(false)
	if err != nil {
		t.Fatal(err)
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
	if err != nil || dependent.Dependencies[0] != (ledger.DependencyFact{Item: "legacy/shipped", Lifecycle: ledger.Merged}) {
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
