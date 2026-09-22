package ledger_test

// Delivery publication seam tests. They exercise ledger.PublishDelivery against
// real local source/bare repositories and committed ledger fixtures plus a
// small DeliveryForge stub: pending publication without public material, normal
// source push and deliberately public presentation, non-fast-forward source
// refusal, and bookkeeping that stays outside the mutation lock while a later
// local handoff or reservation proceeds.
//
// Expected outcomes come from the accepted publication behavior (B9, B10, A7),
// not from the implementation's own computations. Only the deliberately public
// body reaches the forge; the private phase report body never does.

import (
	"context"
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
// runs after recording, so a test can hold the presentation in flight.
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
	return f.number, f.err
}

func (f *deliveryForgeStub) calls() []ledger.PullPresentation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ledger.PullPresentation(nil), f.presentations...)
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

// deliveryPresentBegins starts PublishDelivery in the background and returns
// once the forge presentation is in flight. The returned release function lets
// the presentation finish; it is also registered as cleanup so a failing test
// never strands the background publication.
func deliveryPresentBegins(t *testing.T, source *deliverySource, store *ledger.Store, result *ledger.DeliveryResult, publicBody string) (*deliveryForgeStub, func(), <-chan struct{}) {
	t.Helper()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	releaseForge := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseForge)
	forge := &deliveryForgeStub{number: 42, onPresent: func(ledger.PullPresentation) {
		close(entered)
		<-release
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
	return forge, releaseForge, published
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

// TestDeliveryPublicationWithoutPublicMaterialRemainsPending covers A7: with
// no separately authored public body or no forge access, publication is pending
// and no source push, forge call, or private-prose fallback happens, while the
// local report, lifecycle, and released Claim stay intact.
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
// the committed report, lifecycle, or released Claim.
func deliveryAssertLocalHandoffPreserved(t *testing.T, l *deliveryLedger, head string, result *ledger.DeliveryResult) {
	t.Helper()
	if l.head() != head {
		t.Fatalf("pending publication mutated the ledger: head %s, want %s", l.head(), head)
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.Claim != nil || state.State != result.Status {
		t.Fatalf("pending publication changed the local handoff: %#v", state)
	}
	if state.Publication != nil && state.Publication.Active != nil {
		t.Fatalf("pending publication left a reservation: %#v", state.Publication.Active)
	}
	report, body := deliveryReport(t, l, deliveryPublicationProject, deliveryPublicationItem, ledger.ImplementPhase)
	if body != deliveryPrivateBody || report.Outcome != "awaiting_review" {
		t.Fatalf("pending publication changed the local report: %#v %q", report, body)
	}
}

// TestDeliveryPublicationPushesSourceAndPresentsOnlyPublicProse covers B10/A7:
// a normal source push and explicit public body reach the forge as exactly the
// chosen prose and recorded head/branch, the attachment is recorded, and the
// private report body is never presented.
func TestDeliveryPublicationPushesSourceAndPresentsOnlyPublicProse(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	publicBody := "deliberately public delivery prose\n"
	forge := &deliveryForgeStub{number: 42}
	ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, forge)

	if result.Publication != nil {
		t.Fatalf("successful publication left a pending fact: %#v", result.Publication)
	}
	calls := forge.calls()
	if len(calls) != 1 {
		t.Fatalf("the forge received %d presentations, want 1", len(calls))
	}
	want := ledger.PullPresentation{
		Number: 0, Title: deliveryPublicationBranch, Body: publicBody,
		Branch: deliveryPublicationBranch, Head: source.head, Approved: false,
	}
	if calls[0] != want {
		t.Fatalf("presentation = %#v, want %#v", calls[0], want)
	}
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
	if state.Publication == nil || state.Publication.Active != nil || state.Publication.Source != nil || state.Publication.Pull != nil {
		t.Fatalf("successful publication left records: %#v", state.Publication)
	}
	if result.State.Submission == nil || *result.State.Submission != *wantAttachment {
		t.Fatalf("result state submission = %#v, want %#v", result.State.Submission, wantAttachment)
	}
	changed := deliveryCommitPaths(t, l.root, l.head())
	if len(changed) != 1 || changed[0] != deliveryStatePath(deliveryPublicationProject, deliveryPublicationItem) {
		t.Fatalf("bookkeeping commit changed %v, want only the state record", changed)
	}
}

// TestDeliveryPublicationPresentsApprovedReview covers B10: an approved review
// presents the same source with Approved set.
func TestDeliveryPublicationPresentsApprovedReview(t *testing.T) {
	l, source, store, _ := deliveryPublicationFixture(t)
	watchdog := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	review := ledger.SourceRevisions{Head: source.head, Target: deliveryTarget, Reviewed: source.head}
	result := deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.WatchdogPhase, watchdog.Claim.Commit, review, "pass", "review body\n")
	if result.Status != ledger.ReadyForMerge {
		t.Fatalf("review handoff status = %q, want Ready for Merge", result.Status)
	}
	publicBody := "approved public delivery prose\n"
	forge := &deliveryForgeStub{number: 5}
	ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, forge)

	calls := forge.calls()
	if len(calls) != 1 {
		t.Fatalf("the forge received %d presentations, want 1", len(calls))
	}
	want := ledger.PullPresentation{
		Number: 0, Title: deliveryPublicationBranch, Body: publicBody,
		Branch: deliveryPublicationBranch, Head: source.head, Approved: true,
	}
	if calls[0] != want {
		t.Fatalf("presentation = %#v, want %#v", calls[0], want)
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.State != ledger.ReadyForMerge || state.Submission == nil || state.Submission.Number != 5 {
		t.Fatalf("approved publication state = %#v", state)
	}
}

// TestDeliveryPublicationRefusesNonFastForwardSourceWithoutForge covers B10:
// when the reported source revision cannot be published normally, the forge is
// not asked to approve reviewed code, publication stays pending, and the local
// handoff is unchanged.
func TestDeliveryPublicationRefusesNonFastForwardSourceWithoutForge(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	// Advance the remote branch beyond the reported head so the explicit push is
	// refused as a non-fast-forward rewrite.
	deliveryWrite(t, filepath.Join(source.root, "later.txt"), "later work\n")
	deliveryGit(t, source.root, "add", "later.txt")
	deliveryGit(t, source.root, "commit", "-q", "-m", "later")
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
		t.Fatalf("a source mismatch still presented code as approval: %#v", calls)
	}
	if result.Publication == nil || result.Publication.Status != ledger.IssuePending {
		t.Fatalf("result publication = %#v, want pending", result.Publication)
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.Submission != nil {
		t.Fatalf("a failed source synchronization recorded a submission: %#v", state.Submission)
	}
	if state.Publication == nil || state.Publication.Active != nil {
		t.Fatalf("failed publication left a reservation: %#v", state.Publication)
	}
	if state.Publication.Source == nil || state.Publication.Source.Status != ledger.PushPending {
		t.Fatalf("source synchronization = %#v, want pending", state.Publication.Source)
	}
	if state.Publication.Pull == nil || state.Publication.Pull.Status != ledger.IssuePending {
		t.Fatalf("pull presentation = %#v, want pending", state.Publication.Pull)
	}
	if state.Claim != nil || state.State != result.Status {
		t.Fatalf("failed publication changed the local handoff: %#v", state)
	}
	if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != advanced {
		t.Fatalf("remote branch moved to %q, want %q", reference, advanced)
	}
	reportPath := deliveryReportPath(deliveryPublicationProject, deliveryPublicationItem, ledger.ImplementPhase)
	if deliveryGitShow(t, l.root, "HEAD", reportPath) != deliveryGitShow(t, l.root, headBefore, reportPath) {
		t.Fatal("failed publication changed the local report")
	}
}

// TestDeliveryPublicationBookkeepingPreservesConcurrentLocalWork covers A7:
// the reservation and bookkeeping never hold the ledger mutation lock across
// forge I/O, so a later local phase handoff or reservation completes while a
// presentation is in flight, and the original bookkeeping neither overwrites
// later records nor re-presents a superseded result.
func TestDeliveryPublicationBookkeepingPreservesConcurrentLocalWork(t *testing.T) {
	t.Run("later review handoff and stale refusal", func(t *testing.T) {
		l, source, store, result := deliveryPublicationFixture(t)
		publicBody := "public body\n"
		forge, releaseForge, published := deliveryPresentBegins(t, source, store, result, publicBody)

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

		if calls := forge.calls(); len(calls) != 1 {
			t.Fatalf("the in-flight presentation reached the forge %d times, want 1", len(calls))
		}
		state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
		if state.State != ledger.ReadyForMerge {
			t.Fatalf("original bookkeeping overwrote a later lifecycle: %#v", state)
		}
		if state.Claim != nil {
			t.Fatalf("original bookkeeping changed the later Claim: %#v", state.Claim)
		}
		if state.Publication == nil || state.Publication.Active != nil {
			t.Fatalf("original bookkeeping left its reservation active: %#v", state.Publication)
		}
		if state.Publication.Source == nil || state.Publication.Source.Status != ledger.PushPending {
			t.Fatalf("original bookkeeping overwrote later pending source work: %#v", state.Publication.Source)
		}
		if state.Publication.Pull == nil || state.Publication.Pull.Status != ledger.IssuePending {
			t.Fatalf("original bookkeeping overwrote later pending presentation: %#v", state.Publication.Pull)
		}
		if report := deliveryGitShow(t, l.root, "HEAD", deliveryReportPath(deliveryPublicationProject, deliveryPublicationItem, ledger.WatchdogPhase)); !strings.Contains(report, "concurrent review body") {
			t.Fatalf("original bookkeeping removed the later report: %s", report)
		}
		if state.Submission == nil || state.Submission.Number != 42 {
			t.Fatalf("the successful attachment was not recorded: %#v", state.Submission)
		}

		// The implement result is now superseded. Publishing it again must be
		// refused before any source or forge effect, even with the source branch
		// absent so a stray push would be visible.
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

	t.Run("later reservation survives", func(t *testing.T) {
		l, source, store, result := deliveryPublicationFixture(t)
		publicBody := "public body\n"
		forge, releaseForge, published := deliveryPresentBegins(t, source, store, result, publicBody)

		// While the presentation is in flight, an independent review reserves
		// the Work Item without handing off.
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

		if calls := forge.calls(); len(calls) != 1 {
			t.Fatalf("the in-flight presentation reached the forge %d times, want 1", len(calls))
		}
		if result.Publication != nil {
			t.Fatalf("the successful presentation did not clear its pending fact: %#v", result.Publication)
		}
		state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
		if state.Claim == nil || state.Claim.Phase != ledger.WatchdogPhase {
			t.Fatalf("original bookkeeping overwrote a later Claim: %#v", state.Claim)
		}
		if state.State != ledger.AwaitingReview {
			t.Fatalf("original bookkeeping changed the lifecycle under a later Claim: %#v", state)
		}
		if state.Publication == nil || state.Publication.Active != nil || state.Publication.Source != nil || state.Publication.Pull != nil {
			t.Fatalf("publication records = %#v", state.Publication)
		}
		report, body := deliveryReport(t, l, deliveryPublicationProject, deliveryPublicationItem, ledger.ImplementPhase)
		if body != deliveryPrivateBody || report.Outcome != "awaiting_review" {
			t.Fatalf("original bookkeeping changed the local report: %#v %q", report, body)
		}
	})
}

// TestDeliveryPublicationRecordsUnconfirmedPresentation covers B4/A7: a normal
// presentation whose response is lost is recorded as an unconfirmed attempt,
// so later recovery observes the forge instead of treating the generic error
// as observable absence and repeating the create.
func TestDeliveryPublicationRecordsUnconfirmedPresentation(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	bodyFile := filepath.Join(t.TempDir(), "public.md")
	publicBody := "deliberately public body\n"
	deliveryWrite(t, bodyFile, publicBody)
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
		t.Fatalf("remember body: %v", err)
	}
	forge := &deliveryForgeStub{err: fmt.Errorf("connection reset after the request was sent")}
	ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, forge)

	if calls := forge.calls(); len(calls) != 1 {
		t.Fatalf("the forge received %d presentations, want 1", len(calls))
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.Submission != nil {
		t.Fatalf("an unconfirmed presentation recorded a submission: %#v", state.Submission)
	}
	if state.Publication == nil || state.Publication.Pull == nil || state.Publication.Pull.Status != ledger.IssueUnresolved {
		t.Fatalf("unconfirmed presentation = %#v, want an unresolved attempt", state.Publication)
	}

	// Recovery must hand the unconfirmed create to the adapter so it observes
	// before writing; a fresh create is not authorized.
	var observed ledger.RecoveryPresentation
	recovery := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Status: "ambiguous"}}
	recovery.onRecover = func(presentation ledger.RecoveryPresentation) { observed = presentation }
	outcome, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
		ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, recovery)
	if err != nil {
		t.Fatalf("recover unconfirmed presentation: %v", err)
	}
	if outcome.Status != "ambiguous" {
		t.Fatalf("recovery of unconfirmed presentation = %#v, want ambiguous", outcome)
	}
	if !observed.MayHaveCreated || observed.ObserveOnly || observed.Number != 0 {
		t.Fatalf("unconfirmed presentation facts = %#v, want observation of a possible create", observed)
	}
}
