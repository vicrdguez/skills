package ledger_test

// Pull request presentation seam tests. They exercise ledger.PublishDelivery
// (normal handoff presentation) and ledger.SelectCurrentResult with
// ledger.PresentCurrent (explicit current-view presentation) against real
// local source/bare repositories and committed ledger fixtures plus a small
// DeliveryForge stub.
//
// Expected outcomes come from the accepted latest-view behavior (B1-B5,
// A1-A2 of publish-current-pulls and ADR0007), not from the implementation's
// own computations: the latest committed result is presented without replaying
// missed updates, obsolete reservations have no authority, no pending,
// source, or reservation record is persisted, later local work survives, and
// only the deliberately public body reaches the forge.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vicrdguez/skills/ledger"
)

const (
	deliveryPublicationProject  = "widgets"
	deliveryPublicationSlice    = "delivery-public"
	deliveryPublicationBranch   = "foundation"
	deliveryPublicationItem     = "delivery-public/foundation"
	deliveryPrivateBody         = "PRIVATE worker exchange: implementation detail.\n"
	deliveryPublicationDeadline = 15 * time.Second
)

// deliverySource is one real source working tree and its bare remote. head is
// the candidate revision the phase report names.
type deliverySource struct {
	root, remote, head string
}

func newDeliverySource(t *testing.T) *deliverySource {
	t.Helper()
	directory := t.TempDir()
	root := filepath.Join(directory, "source")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, root, "init", "-q", "-b", "main")
	deliveryGit(t, root, "config", "user.name", "Worker")
	deliveryGit(t, root, "config", "user.email", "worker@example.com")
	deliveryWrite(t, filepath.Join(root, "code.txt"), "reviewed code\n")
	deliveryGit(t, root, "add", "code.txt")
	deliveryGit(t, root, "commit", "-q", "-m", "candidate")
	remote := filepath.Join(directory, "remote.git")
	deliveryGit(t, directory, "init", "-q", "--bare", "-b", "main", remote)
	return &deliverySource{root: root, remote: remote, head: deliveryGitOutput(t, root, "rev-parse", "HEAD")}
}

// commit adds one more source revision and returns it.
func (s *deliverySource) commit(t *testing.T, name string) string {
	t.Helper()
	deliveryWrite(t, filepath.Join(s.root, name), name+"\n")
	deliveryGit(t, s.root, "add", name)
	deliveryGit(t, s.root, "commit", "-q", "-m", name)
	return deliveryGitOutput(t, s.root, "rev-parse", "HEAD")
}

// deliveryRemoteRef returns one reference from a bare repository, or "" when
// it is absent. A missing reference is an expected observation here, so this
// does not fail the test the way deliveryGitOutput would.
func deliveryRemoteRef(t *testing.T, remote, reference string) string {
	t.Helper()
	output, err := exec.Command("git", "-C", remote, "rev-parse", "--verify", reference).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// deliveryForgeStub records what the presentation adapter received. onPresent
// runs after recording, so a test can hold the presentation in flight or
// observe the supersession check the adapter would consult.
type deliveryForgeStub struct {
	mu            sync.Mutex
	presentations []ledger.PullPresentation
	number        int
	err           error
	onPresent     func(ledger.PullPresentation)
}

var _ ledger.DeliveryForge = (*deliveryForgeStub)(nil)

func (f *deliveryForgeStub) PresentPull(_ context.Context, presentation ledger.PullPresentation) (int, error) {
	f.mu.Lock()
	f.presentations = append(f.presentations, presentation)
	f.mu.Unlock()
	if f.onPresent != nil {
		f.onPresent(presentation)
	}
	if f.err != nil {
		return 0, f.err
	}
	return f.number, nil
}

func (f *deliveryForgeStub) calls() []ledger.PullPresentation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ledger.PullPresentation(nil), f.presentations...)
}

// deliveryPublicFields keeps the comparable presentation fields; the
// adapter-only supersession check is a function and is checked separately.
type deliveryPublicFields struct {
	Number                    int
	Title, Body, Branch, Head string
	Approved                  bool
}

func deliveryFields(p ledger.PullPresentation) deliveryPublicFields {
	return deliveryPublicFields{p.Number, p.Title, p.Body, p.Branch, p.Head, p.Approved}
}

func deliveryWantPresentation(t *testing.T, forge *deliveryForgeStub, want deliveryPublicFields) ledger.PullPresentation {
	t.Helper()
	calls := forge.calls()
	if len(calls) != 1 {
		t.Fatalf("the forge received %d presentations, want 1: %#v", len(calls), calls)
	}
	if got := deliveryFields(calls[0]); got != want {
		t.Fatalf("presentation = %#v, want %#v", got, want)
	}
	if calls[0].Current == nil {
		t.Fatal("the presentation carries no supersession check for further updates")
	}
	return calls[0]
}

// deliveryPublicationFixture commits one locally accepted implementation
// handoff with a real source head, leaving the Work Item Awaiting Review with
// no active Claim.
func deliveryPublicationFixture(t *testing.T) (*deliveryLedger, *deliverySource, *ledger.Store, *ledger.DeliveryResult) {
	t.Helper()
	l := newDeliveryLedger(t)
	l.addProject(deliveryPublicationProject, "acme/widgets")
	l.addSlice(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept " + deliveryPublicationSlice)
	source := newDeliverySource(t)
	store := l.store()
	execution := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	result := deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.ImplementPhase, execution.Claim.Commit,
		ledger.SourceRevisions{Head: source.head, Target: deliveryTarget}, "awaiting_review", deliveryPrivateBody)
	return l, source, store, result
}

// deliveryReview hands off one review of the current implementation head.
func deliveryReview(t *testing.T, store *ledger.Store, head, outcome, body string) *ledger.DeliveryResult {
	t.Helper()
	watchdog := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	review := ledger.SourceRevisions{Head: head, Target: deliveryTarget, Reviewed: head}
	return deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.WatchdogPhase, watchdog.Claim.Commit, review, outcome, body)
}

func deliverySelect(t *testing.T, store *ledger.Store) ledger.CurrentResult {
	t.Helper()
	selected, err := ledger.SelectCurrentResult(store, deliveryWidgets(), deliveryPublicationItem)
	if err != nil {
		t.Fatalf("select the current result: %v", err)
	}
	return selected
}

// deliveryAssertNoPublicationRecords checks the committed state.json carries
// no pull request reservation, pending-presentation, or source-receipt key.
func deliveryAssertNoPublicationRecords(t *testing.T, l *deliveryLedger) {
	t.Helper()
	raw := deliveryGitShow(t, l.root, "HEAD", deliveryStatePath(deliveryPublicationProject, deliveryPublicationItem))
	var record struct {
		Publication map[string]json.RawMessage `json:"publication"`
	}
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"active_delivery", "source", "pull", "phase", "published_source", "pull_body"} {
		if _, found := record.Publication[key]; found {
			t.Fatalf("state.json persists publication coordination %q: %s", key, raw)
		}
	}
}

// deliveryPresentBegins starts PublishDelivery in the background and returns
// once the forge presentation is in flight. The returned release function lets
// the presentation finish; it is also registered as cleanup so a failing test
// never strands the background publication. After release, the stub records
// what the adapter's supersession check reported.
func deliveryPresentBegins(t *testing.T, source *deliverySource, store *ledger.Store, result *ledger.DeliveryResult, publicBody string) (*deliveryForgeStub, func(), <-chan struct{}, *error) {
	t.Helper()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	releaseForge := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseForge)
	var superseded error
	forge := &deliveryForgeStub{number: 42, onPresent: func(presentation ledger.PullPresentation) {
		close(entered)
		<-release
		superseded = presentation.Current()
	}}
	published := make(chan struct{})
	go func() {
		ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, forge)
		close(published)
	}()
	select {
	case <-entered:
	case <-time.After(deliveryPublicationDeadline):
		t.Fatal("the forge presentation never began")
	}
	return forge, releaseForge, published, &superseded
}

// deliveryRunBounded runs one local ledger operation and fails the test when it
// does not finish. A live mutation lock spanning forge I/O surfaces here.
func deliveryRunBounded(t *testing.T, operation func() error, timeoutMessage string) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- operation() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("concurrent local work during forge I/O: %v", err)
		}
	case <-time.After(deliveryPublicationDeadline):
		t.Fatal(timeoutMessage)
	}
}

func deliveryWaitPublished(t *testing.T, published <-chan struct{}) {
	t.Helper()
	select {
	case <-published:
	case <-time.After(deliveryPublicationDeadline):
		t.Fatal("publication did not finish after its presentation was released")
	}
}

// TestDeliveryPublicationWithoutPublicMaterialRemainsPending: with no
// separately authored public body or no forge access, presentation is pending
// and no source push, forge call, private-prose fallback, or ledger write
// happens, while the local report, lifecycle, and released Claim stay intact.
func TestDeliveryPublicationWithoutPublicMaterialRemainsPending(t *testing.T) {
	publicBody := "deliberately public body\n"

	t.Run("no separately authored public body", func(t *testing.T) {
		l, source, store, result := deliveryPublicationFixture(t)
		headBefore := l.head()
		forge := &deliveryForgeStub{number: 7}
		ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, nil, forge)

		if result.Publication == nil || result.Publication.Status != ledger.IssuePending {
			t.Fatalf("publication = %#v, want a pending issue fact", result.Publication)
		}
		if calls := forge.calls(); len(calls) != 0 {
			t.Fatalf("the forge was invoked without public material: %#v", calls)
		}
		if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != "" {
			t.Fatalf("source was pushed without public material: %s", reference)
		}
		deliveryAssertLocalHandoffPreserved(t, l, headBefore, result)
	})

	t.Run("no forge access", func(t *testing.T) {
		l, source, store, result := deliveryPublicationFixture(t)
		headBefore := l.head()
		ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, nil)

		if result.Publication == nil || result.Publication.Status != ledger.IssuePending {
			t.Fatalf("publication = %#v, want a pending issue fact", result.Publication)
		}
		if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != "" {
			t.Fatalf("source was pushed without forge access: %s", reference)
		}
		deliveryAssertLocalHandoffPreserved(t, l, headBefore, result)
	})
}

// deliveryAssertLocalHandoffPreserved checks that no publication effect changed
// the committed report, lifecycle, or released Claim, or wrote any record.
func deliveryAssertLocalHandoffPreserved(t *testing.T, l *deliveryLedger, head string, result *ledger.DeliveryResult) {
	t.Helper()
	if l.head() != head {
		t.Fatalf("pending publication mutated the ledger: head %s, want %s", l.head(), head)
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.Claim != nil || state.State != result.Status {
		t.Fatalf("pending publication changed the local handoff: %#v", state)
	}
	deliveryAssertNoPublicationRecords(t, l)
	report, body := deliveryReport(t, l, deliveryPublicationProject, deliveryPublicationItem, ledger.ImplementPhase)
	if body != deliveryPrivateBody || report.Outcome != "awaiting_review" {
		t.Fatalf("pending publication changed the local report: %#v %q", report, body)
	}
}

// TestDeliveryPublicationPushesSourceAndPresentsOnlyPublicProse: a normal
// handoff attempts presentation only after its local commit, pushes the
// recorded source normally, presents exactly the chosen prose for the recorded
// head/branch as draft, and records only the established association.
func TestDeliveryPublicationPushesSourceAndPresentsOnlyPublicProse(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	handoff := l.head()
	publicBody := "deliberately public delivery prose\n"
	forge := &deliveryForgeStub{number: 42}
	ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, forge)

	if result.Publication == nil || result.Publication.Status != ledger.PullPresented || !strings.Contains(result.Publication.Detail, "#42") {
		t.Fatalf("successful publication = %#v, want presented #42", result.Publication)
	}
	deliveryWantPresentation(t, forge, deliveryPublicFields{
		Number: 0, Title: deliveryPublicationBranch, Body: publicBody,
		Branch: deliveryPublicationBranch, Head: source.head, Approved: false,
	})
	if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != source.head {
		t.Fatalf("remote %s = %q, want the reported source head %s", deliveryPublicationBranch, reference, source.head)
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.Claim != nil || state.State != ledger.AwaitingReview {
		t.Fatalf("publication changed the local handoff: %#v", state)
	}
	wantAttachment := &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 42}
	if state.Submission == nil || *state.Submission != *wantAttachment {
		t.Fatalf("recorded submission = %#v, want %#v", state.Submission, wantAttachment)
	}
	if result.State.Submission == nil || *result.State.Submission != *wantAttachment {
		t.Fatalf("result state submission = %#v, want %#v", result.State.Submission, wantAttachment)
	}
	if state.Target == nil || *state.Target != (ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "main"}) {
		t.Fatalf("recorded integration target = %#v, want acme/widgets main", state.Target)
	}
	deliveryAssertNoPublicationRecords(t, l)
	if commits := deliveryGitOutput(t, l.root, "rev-list", "--count", handoff+"..HEAD"); commits != "1" {
		t.Fatalf("presentation wrote %s ledger commits after the handoff, want only the association", commits)
	}
	changed := deliveryCommitPaths(t, l.root, l.head())
	if len(changed) != 1 || changed[0] != deliveryStatePath(deliveryPublicationProject, deliveryPublicationItem) {
		t.Fatalf("bookkeeping commit changed %v, want only the state record", changed)
	}

	// Presenting the same current view again reuses the association and writes
	// nothing further.
	again := &deliveryForgeStub{number: 42}
	head := l.head()
	presentation := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, deliverySelect(t, store), "refreshed prose\n", again)
	if presentation.Publication.Status != ledger.PullPresented || presentation.Replication != nil {
		t.Fatalf("repeated presentation = %#v", presentation)
	}
	if calls := again.calls(); len(calls) != 1 || calls[0].Number != 42 {
		t.Fatalf("repeated presentation did not use the established association: %#v", calls)
	}
	if l.head() != head {
		t.Fatal("repeating a presentation with an established association wrote to the ledger")
	}
}

// TestDeliveryPublicationPresentsApprovedReview: an approved review presents
// its reviewed source with Approved set.
func TestDeliveryPublicationPresentsApprovedReview(t *testing.T) {
	l, source, store, _ := deliveryPublicationFixture(t)
	result := deliveryReview(t, store, source.head, "pass", "review body\n")
	if result.Status != ledger.ReadyForMerge {
		t.Fatalf("review handoff status = %q, want Ready for Merge", result.Status)
	}
	publicBody := "approved public delivery prose\n"
	forge := &deliveryForgeStub{number: 5}
	ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, forge)

	deliveryWantPresentation(t, forge, deliveryPublicFields{
		Number: 0, Title: deliveryPublicationBranch, Body: publicBody,
		Branch: deliveryPublicationBranch, Head: source.head, Approved: true,
	})
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.State != ledger.ReadyForMerge || state.Submission == nil || state.Submission.Number != 5 {
		t.Fatalf("approved publication state = %#v", state)
	}
}

// TestDeliveryPublicationRefusesNonFastForwardSourceWithoutForge: when the
// recorded source revision cannot be published by an ordinary push, the forge
// is not asked to present or approve it, the remote is not forced, nothing is
// persisted, and the local handoff is unchanged.
func TestDeliveryPublicationRefusesNonFastForwardSourceWithoutForge(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	// Advance the remote branch beyond the reported head so the explicit push is
	// refused as a non-fast-forward rewrite.
	source.commit(t, "later.txt")
	deliveryGit(t, source.root, "push", "-q", source.remote, "HEAD:refs/heads/"+deliveryPublicationBranch)
	advanced := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch)
	if advanced == "" || advanced == source.head {
		t.Fatalf("fixture remote = %q, want a descendant of %s", advanced, source.head)
	}

	publicBody := "public body\n"
	forge := &deliveryForgeStub{number: 9}
	headBefore := l.head()
	ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, forge)

	if calls := forge.calls(); len(calls) != 0 {
		t.Fatalf("a source mismatch still presented code: %#v", calls)
	}
	if result.Publication == nil || result.Publication.Status != ledger.IssuePending || !strings.Contains(result.Publication.Detail, "source push unavailable") {
		t.Fatalf("result publication = %#v, want a pending source limitation", result.Publication)
	}
	if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != advanced {
		t.Fatalf("remote branch moved to %q, want %q", reference, advanced)
	}
	deliveryAssertLocalHandoffPreserved(t, l, headBefore, result)
}

// TestDeliveryPublicationPublishesSourceLeftBehindByAnOutage covers the B3
// outage scenario: implementation and review completed while the source
// branch lagged; explicit current presentation pushes the exact reviewed
// revision normally and presents the latest approved review. When the push is
// unavailable, it reports the limitation without approval or local change.
func TestDeliveryPublicationPublishesSourceLeftBehindByAnOutage(t *testing.T) {
	t.Run("ordinary push succeeds", func(t *testing.T) {
		l, source, store, _ := deliveryPublicationFixture(t)
		// The remote branch holds only an ancestor of the recorded head.
		deliveryGit(t, source.root, "push", "-q", source.remote, source.head+":refs/heads/"+deliveryPublicationBranch)
		final := source.commit(t, "final.txt")
		// Replace the implementation result with one at the unpublished head
		// through an ordinary rework cycle, then approve it.
		deliveryReview(t, store, source.head, "rework", "review asks for more\n")
		rework := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
		deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.ImplementPhase, rework.Claim.Commit,
			ledger.SourceRevisions{Head: final, Target: deliveryTarget}, "awaiting_review", "rework body\n")
		deliveryReview(t, store, final, "pass", "approve final\n")

		forge := &deliveryForgeStub{number: 11}
		presentation := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, deliverySelect(t, store), "approved after outage\n", forge)
		if presentation.Publication.Status != ledger.PullPresented {
			t.Fatalf("presentation = %#v", presentation.Publication)
		}
		deliveryWantPresentation(t, forge, deliveryPublicFields{Title: deliveryPublicationBranch, Body: "approved after outage\n", Branch: deliveryPublicationBranch, Head: final, Approved: true})
		if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != final {
			t.Fatalf("remote = %q, want the reviewed final revision %s", reference, final)
		}
		if state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch); state.State != ledger.ReadyForMerge {
			t.Fatalf("presentation changed the lifecycle: %#v", state)
		}
	})

	t.Run("push unavailable", func(t *testing.T) {
		l, source, store, _ := deliveryPublicationFixture(t)
		deliveryReview(t, store, source.head, "pass", "approve\n")
		if err := os.RemoveAll(source.remote); err != nil {
			t.Fatal(err)
		}
		head := l.head()
		forge := &deliveryForgeStub{number: 11}
		presentation := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, deliverySelect(t, store), "approved\n", forge)
		if presentation.Publication.Status != ledger.IssuePending || !strings.Contains(presentation.Publication.Detail, "source push unavailable") {
			t.Fatalf("presentation = %#v, want a pending source limitation", presentation.Publication)
		}
		if calls := forge.calls(); len(calls) != 0 {
			t.Fatalf("unpublished source was presented as approved: %#v", calls)
		}
		if l.head() != head {
			t.Fatal("a source limitation changed the local ledger")
		}
	})
}

// TestDeliveryPublicationPresentsLatestReviewAfterInterruptedImplementation
// is the W3 regression (B1/B2): a faithful persisted interruption leaves the
// implementation presentation's durable reservation and pending notes in the
// record; review then completes locally. Presenting the current view with
// review prose presents only the latest review, ignores the obsolete
// reservation, and drops it on the one ordinary write, leaving reports,
// review count, lifecycle, and the later Claim otherwise unchanged.
func TestDeliveryPublicationPresentsLatestReviewAfterInterruptedImplementation(t *testing.T) {
	l, source, store, implementation := deliveryPublicationFixture(t)
	statePath := deliveryStatePath(deliveryPublicationProject, deliveryPublicationItem)
	// The prior implementation's reservation, committed before its forge call
	// and never settled.
	var record map[string]any
	if err := json.Unmarshal([]byte(deliveryGitShow(t, l.root, "HEAD", statePath)), &record); err != nil {
		t.Fatal(err)
	}
	record["publication"] = map[string]any{
		"active_delivery": map[string]string{"commit": implementation.Report.Commit, "path": implementation.Report.Path},
		"source":          map[string]string{"status": "pending", "detail": "source synchronization has not been attempted for this phase result"},
		"pull":            map[string]string{"status": "pending", "detail": "human-facing phase presentation has not been attempted"},
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	l.addFile(statePath, string(encoded)+"\n")
	l.commitAll("reserve delivery presentation (interrupted)")

	review := deliveryReview(t, store, source.head, "pass", "PRIVATE review findings\n")
	if review.Status != ledger.ReadyForMerge {
		t.Fatalf("review status = %q", review.Status)
	}
	reviewReport := deliveryGitShow(t, l.root, "HEAD", review.Report.Path)
	implementReport := deliveryGitShow(t, l.root, "HEAD", implementation.Report.Path)

	selected := deliverySelect(t, store)
	if selected.Phase != ledger.WatchdogPhase || selected.Outcome != "pass" || selected.Round != 1 || !selected.Approved || selected.Implement == nil {
		t.Fatalf("selected = %#v, want the approved first review consuming the implementation", selected)
	}
	forge := &deliveryForgeStub{number: 17}
	presentation := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, selected, "review passed publicly\n", forge)
	if presentation.Publication.Status != ledger.PullPresented {
		t.Fatalf("presentation = %#v", presentation.Publication)
	}
	deliveryWantPresentation(t, forge, deliveryPublicFields{Title: deliveryPublicationBranch, Body: "review passed publicly\n", Branch: deliveryPublicationBranch, Head: source.head, Approved: true})

	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.State != ledger.ReadyForMerge || state.Claim != nil || state.Submission == nil || state.Submission.Number != 17 {
		t.Fatalf("state after presentation = %#v", state)
	}
	deliveryAssertNoPublicationRecords(t, l)
	if deliveryGitShow(t, l.root, "HEAD", review.Report.Path) != reviewReport || deliveryGitShow(t, l.root, "HEAD", implementation.Report.Path) != implementReport {
		t.Fatal("presentation changed a phase report")
	}
	if report, _ := deliveryReport(t, l, deliveryPublicationProject, deliveryPublicationItem, ledger.WatchdogPhase); report.Round != 1 {
		t.Fatalf("review count = %d, want 1", report.Round)
	}
}

// TestDeliveryPublicationSelectsTheLatestResult covers B1/B5 selection: after
// implementation, rejection, and rework the implementation is current; review
// of it then becomes current; a later Claim does not supersede it, while
// recorded human direction or a missing result is reported, not presented.
func TestDeliveryPublicationSelectsTheLatestResult(t *testing.T) {
	l, source, store, _ := deliveryPublicationFixture(t)
	if selected := deliverySelect(t, store); selected.Phase != ledger.ImplementPhase || selected.Outcome != "awaiting_review" || selected.Approved {
		t.Fatalf("selected = %#v, want the implementation", selected)
	}
	deliveryReview(t, store, source.head, "rework", "reject\n")
	if selected := deliverySelect(t, store); selected.Phase != ledger.WatchdogPhase || selected.Outcome != ledger.Rework || selected.Lifecycle != ledger.Rework {
		t.Fatalf("selected = %#v, want the rejecting review", selected)
	}
	rework := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	if selected := deliverySelect(t, store); selected.Phase != ledger.WatchdogPhase || !selected.Claimed {
		t.Fatalf("selected under a later Claim = %#v, want the same review with the Claim noted", selected)
	}
	final := source.commit(t, "rework.txt")
	deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.ImplementPhase, rework.Claim.Commit,
		ledger.SourceRevisions{Head: final, Target: deliveryTarget}, "awaiting_review", "rework body\n")
	selected := deliverySelect(t, store)
	if selected.Phase != ledger.ImplementPhase || selected.Source.Head != final || selected.Implement != nil {
		t.Fatalf("selected after rework = %#v, want the reworked implementation", selected)
	}
	deliveryReview(t, store, final, ledger.NeedsHuman, "pause\n")
	if selected := deliverySelect(t, store); selected.Phase != ledger.WatchdogPhase || selected.Round != 2 || selected.Source.Reviewed != final {
		t.Fatalf("selected = %#v, want the second review", selected)
	}

	// Human direction recorded after the paused review supersedes it.
	deliveryRequeue(t, l, deliveryPublicationProject, deliveryPublicationItem, ledger.Rework, "continue\n")
	if _, err := ledger.SelectCurrentResult(store, deliveryWidgets(), deliveryPublicationItem); err == nil || !strings.Contains(err.Error(), "supersedes") {
		t.Fatalf("selection over human direction = %v, want a supersession limitation", err)
	}

	fresh := newDeliveryLedger(t)
	fresh.addProject(deliveryPublicationProject, "acme/widgets")
	fresh.addSlice(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, ledger.ReadyForImplementation, nil, deliveryInitial)
	fresh.commitAll("accept")
	if _, err := ledger.SelectCurrentResult(fresh.store(), deliveryWidgets(), deliveryPublicationItem); err == nil || !strings.Contains(err.Error(), "no committed phase result") {
		t.Fatalf("selection without a result = %v", err)
	}
}

// TestDeliveryPublicationPausedWithoutSourceIsPending: a paused result with
// no source revision reports that limitation without any external effect.
func TestDeliveryPublicationPausedWithoutSourceIsPending(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject(deliveryPublicationProject, "acme/widgets")
	l.addSlice(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept")
	source := newDeliverySource(t)
	store := l.store()
	execution := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.ImplementPhase, execution.Claim.Commit, ledger.SourceRevisions{}, ledger.NeedsHuman, "paused\n")
	forge := &deliveryForgeStub{number: 3}
	head := l.head()
	presentation := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, deliverySelect(t, store), "paused publicly\n", forge)
	if presentation.Publication.Status != ledger.IssuePending || !strings.Contains(presentation.Publication.Detail, "no source revision") {
		t.Fatalf("presentation = %#v", presentation.Publication)
	}
	if len(forge.calls()) != 0 || l.head() != head || deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch) != "" {
		t.Fatal("a result without source produced an external or local effect")
	}
}

// TestDeliveryPublicationUncertainCreateIsReportedOnce covers B4 create
// response loss: the uncertainty is reported as unresolved after one
// presentation, with no durable attempt record, association, or local change.
func TestDeliveryPublicationUncertainCreateIsReportedOnce(t *testing.T) {
	l, source, store, _ := deliveryPublicationFixture(t)
	head := l.head()
	forge := &deliveryForgeStub{err: fmt.Errorf("%w: creation response lost", ledger.ErrPresentationUncertain)}
	presentation := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, deliverySelect(t, store), "public\n", forge)
	if presentation.Publication.Status != ledger.IssueUnresolved || !strings.Contains(presentation.Publication.Detail, "response lost") {
		t.Fatalf("presentation = %#v, want unresolved uncertainty", presentation.Publication)
	}
	if calls := forge.calls(); len(calls) != 1 {
		t.Fatalf("uncertain creation was presented %d times, want 1", len(calls))
	}
	if l.head() != head {
		t.Fatal("uncertain creation wrote a durable record")
	}
	if state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch); state.Submission != nil || state.State != ledger.AwaitingReview {
		t.Fatalf("uncertain creation changed the record: %#v", state)
	}

	definite := &deliveryForgeStub{err: errors.New("validation failed")}
	if presentation := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, deliverySelect(t, store), "public\n", definite); presentation.Publication.Status != ledger.IssuePending {
		t.Fatalf("definite failure = %#v, want pending", presentation.Publication)
	}
}

// TestDeliveryPublicationBookkeepingPreservesConcurrentLocalWork covers the
// B4 in-flight scenario: no lock is held across forge I/O, so a later local
// handoff or Claim completes while a presentation is in flight. The late
// association write preserves the newer records; further updates of a
// superseded result stop; a Claim alone does not supersede it; and a
// different concurrently recorded association is not reassigned.
func TestDeliveryPublicationBookkeepingPreservesConcurrentLocalWork(t *testing.T) {
	t.Run("later review handoff supersedes further updates", func(t *testing.T) {
		l, source, store, result := deliveryPublicationFixture(t)
		publicBody := "public body\n"
		forge, releaseForge, published, superseded := deliveryPresentBegins(t, source, store, result, publicBody)

		// The presentation is in flight; the ledger lock must be free.
		deliveryRunBounded(t, func() error {
			watchdog, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.WatchdogPhase)
			if err != nil {
				return err
			}
			if watchdog == nil {
				return fmt.Errorf("no review was selected")
			}
			review := ledger.SourceRevisions{Head: source.head, Target: deliveryTarget, Reviewed: source.head}
			_, err = ledger.HandoffDelivery(store, deliveryWidgets(), deliveryPublicationItem, ledger.WatchdogPhase, watchdog.Claim.Commit, review, "pass", "concurrent review body\n")
			return err
		}, "the ledger mutation lock spans forge I/O: no concurrent local handoff completed")

		releaseForge()
		deliveryWaitPublished(t, published)

		if *superseded == nil || !strings.Contains((*superseded).Error(), "supersedes") {
			t.Fatalf("supersession check after the later review = %v, want further updates stopped", *superseded)
		}
		if calls := forge.calls(); len(calls) != 1 {
			t.Fatalf("the in-flight presentation reached the forge %d times, want 1", len(calls))
		}
		state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
		if state.State != ledger.ReadyForMerge || state.Claim != nil {
			t.Fatalf("original bookkeeping overwrote a later lifecycle or Claim: %#v", state)
		}
		if report := deliveryGitShow(t, l.root, "HEAD", deliveryReportPath(deliveryPublicationProject, deliveryPublicationItem, ledger.WatchdogPhase)); !strings.Contains(report, "concurrent review body") {
			t.Fatalf("original bookkeeping removed the later report: %s", report)
		}
		if state.Submission == nil || state.Submission.Number != 42 {
			t.Fatalf("the successful attachment was not recorded: %#v", state.Submission)
		}
		deliveryAssertNoPublicationRecords(t, l)

		// The implementation handoff is superseded: presenting it again is
		// refused before any source or forge effect, and nothing is replayed.
		deliveryGit(t, source.root, "push", "-q", source.remote, "--delete", "refs/heads/"+deliveryPublicationBranch)
		headAfter := l.head()
		stale := &deliveryForgeStub{number: 99}
		ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, stale)
		if calls := stale.calls(); len(calls) != 0 {
			t.Fatalf("a superseded result still presented code: %#v", calls)
		}
		if result.Publication == nil || result.Publication.Status != ledger.IssuePending || !strings.Contains(result.Publication.Detail, "supersede") {
			t.Fatalf("superseded publication = %#v, want a pending supersede refusal", result.Publication)
		}
		if l.head() != headAfter {
			t.Fatal("a superseded result mutated the ledger")
		}
		if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != "" {
			t.Fatalf("a superseded result pushed source %q", reference)
		}
	})

	t.Run("later Claim survives and does not supersede", func(t *testing.T) {
		l, source, store, result := deliveryPublicationFixture(t)
		forge, releaseForge, published, superseded := deliveryPresentBegins(t, source, store, result, "public body\n")

		deliveryRunBounded(t, func() error {
			watchdog, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.WatchdogPhase)
			if err != nil {
				return err
			}
			if watchdog == nil {
				return fmt.Errorf("no review was selected")
			}
			return nil
		}, "the ledger mutation lock spans forge I/O: no concurrent local reservation completed")

		releaseForge()
		deliveryWaitPublished(t, published)

		if *superseded != nil {
			t.Fatalf("a later Claim alone superseded the presented result: %v", *superseded)
		}
		if calls := forge.calls(); len(calls) != 1 {
			t.Fatalf("the in-flight presentation reached the forge %d times, want 1", len(calls))
		}
		if result.Publication == nil || result.Publication.Status != ledger.PullPresented {
			t.Fatalf("publication = %#v, want presented", result.Publication)
		}
		state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
		if state.Claim == nil || state.Claim.Phase != ledger.WatchdogPhase || state.State != ledger.AwaitingReview {
			t.Fatalf("original bookkeeping overwrote a later Claim or lifecycle: %#v", state)
		}
		if state.Submission == nil || state.Submission.Number != 42 {
			t.Fatalf("association = %#v", state.Submission)
		}
		report, body := deliveryReport(t, l, deliveryPublicationProject, deliveryPublicationItem, ledger.ImplementPhase)
		if body != deliveryPrivateBody || report.Outcome != "awaiting_review" {
			t.Fatalf("original bookkeeping changed the local report: %#v %q", report, body)
		}
	})

	t.Run("different association is not reassigned", func(t *testing.T) {
		l, source, store, _ := deliveryPublicationFixture(t)
		forge := &deliveryForgeStub{number: 42, onPresent: func(ledger.PullPresentation) {
			state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
			state.Submission = &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 7}
			l.commitState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch, state)
		}}
		presentation := ledger.PresentCurrent(context.Background(), store, deliveryWidgets(), source.root, source.remote, deliverySelect(t, store), "public\n", forge)
		if presentation.Publication.Status != ledger.IssuePending || !strings.Contains(presentation.Publication.Detail, "#7") {
			t.Fatalf("presentation = %#v, want the conflicting association reported", presentation.Publication)
		}
		if state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch); state.Submission == nil || state.Submission.Number != 7 {
			t.Fatalf("association = %#v, want the known #7 preserved", state.Submission)
		}
	})
}
