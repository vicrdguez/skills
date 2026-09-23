package ledger_test

// Focused ledger recovery tests. They exercise InspectPublication,
// RecoverPublication, and RememberDeliveryBody against real temporary Git
// ledgers, a real source/bare repository pair, and a small RecoveryForge stub.
// Expected statuses come from the accepted recovery behavior (B1-B8, A1-A3),
// not from the implementation's computations.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vicrdguez/skills/ledger"
)

// recoveryForgeStub records the presentations the adapter receives. onRecover
// runs after recording so a test can hold a presentation in flight or exercise
// the deterministic Guard/PrepareSource callbacks.
type recoveryForgeStub struct {
	mu            sync.Mutex
	presentations []ledger.RecoveryPresentation
	receipt       ledger.RecoveryReceipt
	err           error
	onRecover     func(ledger.RecoveryPresentation)
}

var _ ledger.RecoveryForge = (*recoveryForgeStub)(nil)

func (f *recoveryForgeStub) RecoverPresentation(_ context.Context, presentation ledger.RecoveryPresentation) (ledger.RecoveryReceipt, error) {
	f.mu.Lock()
	f.presentations = append(f.presentations, presentation)
	f.mu.Unlock()
	if f.onRecover != nil {
		f.onRecover(presentation)
	}
	f.mu.Lock()
	receipt, err := f.receipt, f.err
	f.mu.Unlock()
	return receipt, err
}

func (f *recoveryForgeStub) calls() []ledger.RecoveryPresentation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ledger.RecoveryPresentation(nil), f.presentations...)
}

func recoveryPullView(t *testing.T, store *ledger.Store, item string) ledger.PublicationView {
	t.Helper()
	view, err := ledger.InspectPublication(store, deliveryWidgets(), item, "pull")
	if err != nil {
		t.Fatalf("inspect pull %s: %v", item, err)
	}
	return view
}

// TestRecoveryViewBindsLatestCommittedInputs covers A2/B2: the view token
// changes with the latest selected result but not with an unrelated ledger
// commit, and a superseded registered body is reported stale rather than
// published.
func TestRecoveryViewBindsLatestCommittedInputs(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	bodyFile := filepath.Join(t.TempDir(), "pull.md")
	deliveryWrite(t, bodyFile, "current implement presentation\n")
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
		t.Fatalf("remember implement body: %v", err)
	}
	first := recoveryPullView(t, store, deliveryPublicationItem)
	if first.Status != "pending" || first.BodyPath != bodyFile || first.Token == "" {
		t.Fatalf("implement view = %#v, want a pending registered body", first)
	}

	// An unrelated ledger commit must not stale the selected inputs.
	l.addFile("notes/unrelated.md", "unrelated ledger note\n")
	l.commitAll("unrelated ledger change")
	unchanged := recoveryPullView(t, store, deliveryPublicationItem)
	if unchanged.Token != first.Token || unchanged.Status != "pending" {
		t.Fatalf("unrelated commit changed the selected view: %#v want %#v", unchanged, first)
	}

	// The latest committed result is now the watchdog review; the registered
	// body describes the superseded implement view.
	watchdog := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.WatchdogPhase, watchdog.Claim.Commit,
		ledger.SourceRevisions{Head: source.head, Target: deliveryTarget, Reviewed: source.head}, "pass", "review body\n")
	superseded := recoveryPullView(t, store, deliveryPublicationItem)
	if superseded.Token == first.Token {
		t.Fatal("the latest selected result did not change the view token")
	}
	if superseded.Status != "stale" || superseded.BodyPath != bodyFile {
		t.Fatalf("superseded view = %#v, want stale with the registered path", superseded)
	}

	// Recovery refuses to reuse the superseded bytes and asks for fresh prose
	// without calling the forge.
	forge := &recoveryForgeStub{}
	result2, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
		ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, forge)
	if err != nil {
		t.Fatalf("recover superseded view: %v", err)
	}
	if result2.Status != "stale" {
		t.Fatalf("recover superseded status = %q, want stale", result2.Status)
	}
	if calls := forge.calls(); len(calls) != 0 {
		t.Fatalf("recovery reused a superseded body: %#v", calls)
	}
	if result2.View.Token != superseded.Token {
		t.Fatalf("stale result view token = %q, want the current view %q", result2.View.Token, superseded.Token)
	}
}

// TestRecoveryPublicationReusesRegisteredBody covers B2/B4: a current
// registered body is published unchanged, the attachment and published source
// are recorded, and a repeated inspection reports the view as published.
func TestRecoveryPublicationReusesRegisteredBody(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	bodyFile := filepath.Join(t.TempDir(), "pull.md")
	body := "current implement presentation\n"
	deliveryWrite(t, bodyFile, body)
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
		t.Fatalf("remember body: %v", err)
	}
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 42, Status: "published"}}
	recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
		ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, forge)
	if err != nil {
		t.Fatalf("recover pull: %v", err)
	}
	if recovered.Status != "published" {
		t.Fatalf("recover status = %q, want published", recovered.Status)
	}
	calls := forge.calls()
	if len(calls) != 1 {
		t.Fatalf("forge received %d presentations, want 1", len(calls))
	}
	presentation := calls[0]
	if presentation.Body == nil || *presentation.Body != body {
		t.Fatalf("presented body = %#v, want the registered bytes", presentation.Body)
	}
	if presentation.Head != source.head || presentation.Branch != deliveryPublicationBranch || presentation.Approved {
		t.Fatalf("presentation facts = %#v", presentation)
	}
	if presentation.OriginalBodySHA256 != recoveryDigest(body) {
		t.Fatalf("original body digest = %q, want the registered digest", presentation.OriginalBodySHA256)
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.Submission == nil || state.Submission.Number != 42 {
		t.Fatalf("recovered submission = %#v, want number 42", state.Submission)
	}
	if state.Publication == nil || state.Publication.Active != nil || state.Publication.Pull != nil || state.Publication.PullBody != nil {
		t.Fatalf("satisfied recovery left pending records: %#v", state.Publication)
	}
	if state.Publication.PublishedSource != source.head {
		t.Fatalf("published source = %q, want %s", state.Publication.PublishedSource, source.head)
	}
	view := recoveryPullView(t, store, deliveryPublicationItem)
	if view.Status != "published" || view.Attachment == nil || view.Attachment.Number != 42 {
		t.Fatalf("published view = %#v", view)
	}
}

func TestRecoveryRequiresExplicitSatisfactionAndPreservesPartialReceipt(t *testing.T) {
	for _, status := range []ledger.PublicationStatus{"", "unexpected", ledger.PublicationPublished} {
		t.Run(string(status), func(t *testing.T) {
			l, source, store, result := deliveryPublicationFixture(t)
			bodyFile := filepath.Join(t.TempDir(), "public.md")
			deliveryWrite(t, bodyFile, "public material\n")
			if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
				t.Fatal(err)
			}
			forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 42, Status: status}}
			if status == ledger.PublicationPublished {
				forge.err = fmt.Errorf("readback failed after creating the attachment")
			}
			out, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
				ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, forge)
			if err != nil || out.Status != ledger.PublicationPending {
				t.Fatalf("unconfirmed receipt: %#v, %v", out, err)
			}
			state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
			if state.Submission == nil || state.Submission.Number != 42 || state.Publication.Pull == nil || state.Publication.PullBody == nil {
				t.Fatalf("lost partial identity or cleared pending material: %#v", state)
			}
		})
	}
}

// TestRecoveryPublicationLostBodyNeedsProse covers B3: deleting the registered
// temporary bytes produces a prose-needed result without a forge call and
// without mutating the ledger.
func TestRecoveryPublicationLostBodyNeedsProse(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	bodyFile := filepath.Join(t.TempDir(), "pull.md")
	deliveryWrite(t, bodyFile, "lost presentation\n")
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
		t.Fatalf("remember body: %v", err)
	}
	if err := os.Remove(bodyFile); err != nil {
		t.Fatal(err)
	}
	head := l.head()
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 5, Status: "published"}}
	recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
		ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, forge)
	if err != nil {
		t.Fatalf("recover lost body: %v", err)
	}
	if recovered.Status != "prose-needed" {
		t.Fatalf("recover status = %q, want prose-needed", recovered.Status)
	}
	if calls := forge.calls(); len(calls) != 0 {
		t.Fatalf("lost prose still reached the forge: %#v", calls)
	}
	if l.head() != head {
		t.Fatal("prose-needed recovery mutated the ledger")
	}
	if view := recoveryPullView(t, store, deliveryPublicationItem); view.Status != "prose-needed" {
		t.Fatalf("inspect status = %q, want prose-needed", view.Status)
	}
}

// TestRecoveryReservationPreventsSecondWriter covers A2/B4: while one attempt
// holds the provisional reservation across its network call, a competing
// attempt is refused without a second external mutation.
func TestRecoveryReservationPreventsSecondWriter(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	bodyFile := filepath.Join(t.TempDir(), "pull.md")
	deliveryWrite(t, bodyFile, "reserved presentation\n")
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
		t.Fatalf("remember body: %v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	releaseForge := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseForge)
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 42, Status: "published"}, onRecover: func(ledger.RecoveryPresentation) {
		close(entered)
		<-release
	}}
	done := make(chan ledger.PublicationResult, 1)
	go func() {
		recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
			ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, forge)
		if err != nil {
			done <- ledger.PublicationResult{Status: "error", Detail: err.Error()}
			return
		}
		done <- recovered
	}()
	select {
	case <-entered:
	case <-time.After(deliveryPublicationDeadline):
		t.Fatal("the recovery presentation never began")
	}
	if state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch); state.Publication == nil || state.Publication.Active == nil {
		t.Fatalf("recovery did not hold a reservation: %#v", state.Publication)
	}
	second := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 7, Status: "published"}}
	blocked, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
		ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, second)
	if err != nil {
		t.Fatalf("competing recovery: %v", err)
	}
	if blocked.Status != "pending" || !strings.Contains(blocked.Detail, "publication lease unavailable") {
		t.Fatalf("competing recovery = %#v, want a live-publisher refusal", blocked)
	}
	if calls := second.calls(); len(calls) != 0 {
		t.Fatalf("competing recovery reached the forge: %#v", calls)
	}
	releaseForge()
	select {
	case recovered := <-done:
		if recovered.Status != "published" {
			t.Fatalf("first recovery status = %q, want published", recovered.Status)
		}
	case <-time.After(deliveryPublicationDeadline):
		t.Fatal("the first recovery did not finish")
	}
	if calls := forge.calls(); len(calls) != 1 {
		t.Fatalf("first recovery reached the forge %d times, want 1", len(calls))
	}
}

// TestRecoveryPreservesNewerView covers B5/A2: a newer selected result
// committed while the forge call is in flight is not marked published, and the
// newer pending work remains.
func TestRecoveryPreservesNewerView(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	bodyFile := filepath.Join(t.TempDir(), "pull.md")
	deliveryWrite(t, bodyFile, "implement presentation\n")
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
		t.Fatalf("remember body: %v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	releaseForge := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseForge)
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 42, Status: "published"}, onRecover: func(ledger.RecoveryPresentation) {
		close(entered)
		<-release
	}}
	done := make(chan ledger.PublicationResult, 1)
	go func() {
		recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
			ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, forge)
		if err != nil {
			done <- ledger.PublicationResult{Status: "error", Detail: err.Error()}
			return
		}
		done <- recovered
	}()
	select {
	case <-entered:
	case <-time.After(deliveryPublicationDeadline):
		t.Fatal("the recovery presentation never began")
	}
	// The newer review commits while the implement presentation is in flight.
	deliveryRunBounded(t, func() error {
		watchdog, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.WatchdogPhase)
		if err != nil {
			return err
		}
		if watchdog == nil {
			return fmt.Errorf("no review was selected")
		}
		_, err = ledger.HandoffDelivery(store, deliveryWidgets(), deliveryPublicationItem, ledger.WatchdogPhase, watchdog.Claim.Commit,
			ledger.SourceRevisions{Head: source.head, Target: deliveryTarget, Reviewed: source.head}, "pass", "newer review body\n")
		return err
	}, "the recovery reservation held the ledger mutation lock across forge I/O")
	releaseForge()
	var recovered ledger.PublicationResult
	select {
	case recovered = <-done:
	case <-time.After(deliveryPublicationDeadline):
		t.Fatal("recovery did not finish after its presentation was released")
	}
	if recovered.Status != "pending" || !strings.Contains(recovered.Detail, "newer") {
		t.Fatalf("raced recovery = %#v, want a newer-view pending result", recovered)
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.State != ledger.ReadyForMerge {
		t.Fatalf("raced recovery changed the newer lifecycle: %#v", state)
	}
	if state.Publication == nil || state.Publication.Active != nil {
		t.Fatalf("raced recovery left its reservation: %#v", state.Publication)
	}
	if state.Publication.Pull == nil {
		t.Fatalf("raced recovery cleared the newer pending presentation: %#v", state.Publication)
	}
	if state.Submission == nil || state.Submission.Number != 42 {
		t.Fatalf("raced recovery lost its observable attachment: %#v", state.Submission)
	}
	report, _ := deliveryReport(t, l, deliveryPublicationProject, deliveryPublicationItem, ledger.WatchdogPhase)
	if report.Outcome != "pass" {
		t.Fatalf("raced recovery overwrote the newer report: %#v", report)
	}
}

// TestRecoveryValidatesSelectedFindings covers B8: anchors are validated
// against the current schema-1 reviewed input before the adapter, and the
// finding result is reported separately from body publication.
func TestRecoveryValidatesSelectedFindings(t *testing.T) {
	l, source, store, _ := deliveryPublicationFixture(t)
	watchdog := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	result := deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.WatchdogPhase, watchdog.Claim.Commit,
		ledger.SourceRevisions{Head: source.head, Target: deliveryTarget, Reviewed: source.head}, "pass", "review body\n")
	bodyFile := filepath.Join(t.TempDir(), "pull.md")
	deliveryWrite(t, bodyFile, "approved presentation\n")
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
		t.Fatalf("remember body: %v", err)
	}
	view := recoveryPullView(t, store, deliveryPublicationItem)
	valid := ledger.SelectedFinding{ID: "W2", Body: "actionable finding prose\n", Commit: source.head, Path: "code.txt", Line: 1, Side: "RIGHT"}
	wrongRevision := ledger.SelectedFinding{ID: "W3", Body: "stale anchor\n", Commit: deliveryTarget, Path: "code.txt", Line: 1, Side: "RIGHT"}
	badSide := ledger.SelectedFinding{ID: "W4", Body: "bad side\n", Commit: source.head, Path: "code.txt", Line: 1, Side: "MIDDLE"}
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 9, Status: "published"}}
	forge.onRecover = func(presentation ledger.RecoveryPresentation) {
		forge.mu.Lock()
		defer forge.mu.Unlock()
		forge.receipt.Findings = nil
		for _, finding := range presentation.Findings {
			forge.receipt.Findings = append(forge.receipt.Findings, ledger.FindingPublication{ID: finding.ID, Status: "satisfied"})
		}
	}
	recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
		ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull", View: view.Token, BodyPath: bodyFile,
			Findings: []ledger.SelectedFinding{valid, wrongRevision, badSide}}, forge)
	if err != nil {
		t.Fatalf("recover with findings: %v", err)
	}
	if recovered.Status != "published" {
		t.Fatalf("body publication status = %q, want published", recovered.Status)
	}
	calls := forge.calls()
	if len(calls) != 1 || len(calls[0].Findings) != 1 || calls[0].Findings[0].ID != "W2" {
		t.Fatalf("adapter findings = %#v, want only the validated W2", calls)
	}
	statuses := make(map[string]string)
	for _, finding := range recovered.Findings {
		statuses[finding.ID] = finding.Status
	}
	if statuses["W2"] != "satisfied" {
		t.Fatalf("valid finding status = %q, want satisfied", statuses["W2"])
	}
	if statuses["W3"] != "unresolved" || statuses["W4"] != "invalid" {
		t.Fatalf("finding statuses = %#v, want W3 unresolved and W4 invalid", statuses)
	}
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.Publication == nil || len(state.Publication.Findings) != 1 || state.Publication.Findings[0].ID != "W2" {
		t.Fatalf("finding receipt not recorded: %#v", state.Publication)
	}
}

// TestRecoverySourceCatchUpUsesRecordedReceipt covers B5/A3: an expected
// lagging source head recorded by normal publication is caught up through the
// non-force source path, while an unexpected head is refused.
func TestRecoverySourceCatchUpUsesRecordedReceipt(t *testing.T) {
	l, source, store, result := deliveryPublicationFixture(t)
	publicBody := "published implement presentation\n"
	deliveryForge := &deliveryForgeStub{number: 42}
	ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, deliveryForge)
	state := l.committedState(deliveryPublicationProject, deliveryPublicationSlice, deliveryPublicationBranch)
	if state.Publication == nil || state.Publication.PublishedSource != source.head {
		t.Fatalf("normal publication did not record the published source: %#v", state.Publication)
	}
	// A permitted post-review commit advances the source beyond the reviewed
	// revision recorded by the implementation report.
	deliveryWrite(t, filepath.Join(source.root, "marker.txt"), "post-review marker\n")
	deliveryGit(t, source.root, "add", "marker.txt")
	deliveryGit(t, source.root, "commit", "-q", "-m", "post-review marker")
	advanced := deliveryGitOutput(t, source.root, "rev-parse", "HEAD")

	watchdog := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	handoff := deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.WatchdogPhase, watchdog.Claim.Commit,
		ledger.SourceRevisions{Head: advanced, Target: deliveryTarget, Reviewed: source.head}, "pass", "review body\n")
	bodyFile := filepath.Join(t.TempDir(), "pull.md")
	deliveryWrite(t, bodyFile, "approved presentation\n")
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), handoff, bodyFile); err != nil {
		t.Fatalf("remember body: %v", err)
	}
	var catchUpErr, unexpectedErr error
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 42, Status: "published"}}
	forge.onRecover = func(presentation ledger.RecoveryPresentation) {
		catchUpErr = presentation.PrepareSource(source.head)
		unexpectedErr = presentation.PrepareSource(deliveryTarget)
	}
	recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
		ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, forge)
	if err != nil {
		t.Fatalf("recover with source catch-up: %v", err)
	}
	if catchUpErr != nil {
		t.Fatalf("expected lagging source was refused: %v", catchUpErr)
	}
	if unexpectedErr == nil {
		t.Fatal("an unexpected observed head was not refused")
	}
	if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != advanced {
		t.Fatalf("caught-up remote head = %q, want %s", reference, advanced)
	}
	if recovered.Status != "published" {
		t.Fatalf("recovery after catch-up status = %q, want published", recovered.Status)
	}
}

// TestAcceptanceRegistersTemporaryBodyPaths covers the normal acceptance
// metadata registration: without a forge, pending issue and parent publication
// remember only the temporary path and digest, and a later unrelated commit
// does not stale the registered view.
func TestAcceptanceRegistersTemporaryBodyPaths(t *testing.T) {
	l := newDeliveryLedger(t)
	store := l.store()
	directory := filepath.Join(t.TempDir(), "grouped")
	writeRecoveryProposal(t, directory, "grouped", true)
	foundationBody := []byte("foundation descriptive issue\n")
	featureBody := []byte("feature descriptive issue\n")
	parentBody := []byte("grouped parent issue\n")
	declaration, err := ledger.LoadDeclaration(directory,
		map[string][]byte{"foundation": foundationBody, "feature": featureBody}, parentBody)
	if err != nil {
		t.Fatalf("load declaration: %v", err)
	}
	foundationPath := filepath.Join(t.TempDir(), "foundation.md")
	featurePath := filepath.Join(t.TempDir(), "feature.md")
	parentPath := filepath.Join(t.TempDir(), "parent.md")
	deliveryWrite(t, foundationPath, string(foundationBody))
	deliveryWrite(t, featurePath, string(featureBody))
	deliveryWrite(t, parentPath, string(parentBody))
	declaration.IssueBodyPaths = map[string]string{"foundation": foundationPath, "feature": featurePath}
	declaration.ParentBodyPath = parentPath
	if _, err := ledger.Accept(context.Background(), store, deliveryWidgets(), declaration, nil, func() time.Time {
		return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	}); err != nil {
		t.Fatalf("accept without forge: %v", err)
	}
	issue, err := ledger.InspectPublication(store, deliveryWidgets(), "grouped/foundation", "issue")
	if err != nil {
		t.Fatalf("inspect issue: %v", err)
	}
	if issue.Status != "pending" || issue.BodyPath != foundationPath {
		t.Fatalf("issue view = %#v, want a pending registered body", issue)
	}
	parent, err := ledger.InspectPublication(store, deliveryWidgets(), "grouped/feature", "parent")
	if err != nil {
		t.Fatalf("inspect parent: %v", err)
	}
	if parent.Status != "pending" || parent.BodyPath != parentPath || len(parent.Children) != 0 {
		t.Fatalf("parent view = %#v, want a pending registered body", parent)
	}
	// An unrelated ledger commit must not change the registered view tokens.
	l.addFile("notes/unrelated.md", "unrelated\n")
	l.commitAll("unrelated ledger change")
	again, err := ledger.InspectPublication(store, deliveryWidgets(), "grouped/foundation", "issue")
	if err != nil {
		t.Fatalf("inspect issue again: %v", err)
	}
	if again.Token != issue.Token {
		t.Fatalf("unrelated commit changed the issue token: %q want %q", again.Token, issue.Token)
	}
	// No forge is available, so recovery reports the missing surface instead
	// of a fabricated success.
	if _, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), "", "",
		ledger.PublicationRequest{Item: "grouped/foundation", Kind: "issue"}, nil); err == nil {
		t.Fatal("recovery without a forge surface reported success")
	}
}

func writeRecoveryProposal(t *testing.T, directory, proposal string, multi bool) {
	t.Helper()
	declaration := map[string]any{"proposal": proposal, "parent_title": "", "slices": []map[string]any{
		{"name": "foundation", "title": "foundation", "branch": "foundation", "depends": []string{}},
	}}
	if multi {
		declaration["parent_title"] = "Grouped delivery"
		declaration["slices"] = []map[string]any{
			{"name": "foundation", "title": "foundation", "branch": "foundation", "depends": []string{}},
			{"name": "feature", "title": "feature", "branch": "feature", "depends": []string{"foundation"}},
		}
	}
	encoded, err := json.Marshal(declaration)
	if err != nil {
		t.Fatal(err)
	}
	deliveryWrite(t, filepath.Join(directory, "proposal.json"), string(encoded)+"\n")
	deliveryWrite(t, filepath.Join(directory, "proposal.md"), "proposal "+proposal+"\n")
	for _, slice := range []string{"foundation", "feature"} {
		if !multi && slice != "foundation" {
			continue
		}
		deliveryWrite(t, filepath.Join(directory, slice, "intent.md"), "intent of "+slice+"\n")
		deliveryWrite(t, filepath.Join(directory, slice, "behavior.md"), "behavior of "+slice+"\n")
	}
}

func recoveryDigest(contents string) string {
	sum := sha256.Sum256([]byte(contents))
	return hex.EncodeToString(sum[:])
}

// TestRecoveryObservesLostIssueIdentity covers B4: when an unconfirmed create's
// temporary bytes are gone but its exact title-and-digest identity remains, the
// adapter observes and adopts instead of creating a second object.
func TestRecoveryObservesLostIssueIdentity(t *testing.T) {
	l, store, bodyPath := recoveryAcceptedIssue(t, "lost-issue")
	if err := os.Remove(bodyPath); err != nil {
		t.Fatal(err)
	}
	state := l.committedState(deliveryPublicationProject, "lost-issue", "foundation")
	state.Publication.Issue = &ledger.PublicationNote{Status: ledger.IssueUnresolved, Detail: "create response lost"}
	l.writeStateValue(deliveryPublicationProject, "lost-issue", "foundation", state)
	l.commitAll("simulate lost create")

	view, err := ledger.InspectPublication(store, deliveryWidgets(), "lost-issue/foundation", "issue")
	if err != nil {
		t.Fatalf("inspect lost issue: %v", err)
	}
	if view.Status != "ambiguous" {
		t.Fatalf("lost-create view = %#v, want ambiguous", view)
	}
	var observed ledger.RecoveryPresentation
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 5, Status: "published"}}
	forge.onRecover = func(presentation ledger.RecoveryPresentation) { observed = presentation }
	recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), "", "",
		ledger.PublicationRequest{Item: "lost-issue/foundation", Kind: "issue"}, forge)
	if err != nil {
		t.Fatalf("recover lost issue: %v", err)
	}
	if recovered.Status != "published" {
		t.Fatalf("recover status = %q, want published", recovered.Status)
	}
	if observed.Body != nil || !observed.ObserveOnly || !observed.MayHaveCreated || observed.OriginalBodySHA256 == "" {
		t.Fatalf("observation presentation = %#v, want digest-only identity without prose", observed)
	}
	after := l.committedState(deliveryPublicationProject, "lost-issue", "foundation")
	if after.Issue == nil || after.Issue.Number != 5 {
		t.Fatalf("observed attachment = %#v, want number 5", after.Issue)
	}
}

// TestRecoveryLegacyUnresolvedWithoutIdentityIsAmbiguous covers B4: an
// unresolved legacy attempt with no original identity is reported ambiguous
// and never reaches the forge.
func TestRecoveryLegacyUnresolvedWithoutIdentityIsAmbiguous(t *testing.T) {
	l, store, _ := recoveryAcceptedIssue(t, "legacy-issue")
	state := l.committedState(deliveryPublicationProject, "legacy-issue", "foundation")
	state.Publication.Issue = &ledger.PublicationNote{Status: ledger.IssueUnresolved, Detail: "legacy outcome unknown"}
	state.Publication.IssueBody = nil
	l.writeStateValue(deliveryPublicationProject, "legacy-issue", "foundation", state)
	l.commitAll("simulate legacy unresolved attempt")
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 5, Status: "published"}}
	recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), "", "",
		ledger.PublicationRequest{Item: "legacy-issue/foundation", Kind: "issue"}, forge)
	if err != nil {
		t.Fatalf("recover legacy attempt: %v", err)
	}
	if recovered.Status != "ambiguous" {
		t.Fatalf("legacy status = %q, want ambiguous", recovered.Status)
	}
	if calls := forge.calls(); len(calls) != 0 {
		t.Fatalf("legacy ambiguous attempt reached the forge: %#v", calls)
	}
}

// TestRecoveryFreshPullPublishesSourceBeforeCreate covers A3: a pull that was
// never created is preceded by an ordinary non-force source publication so the
// adapter can reread the exact intended head before creating the pull request.
func TestRecoveryFreshPullPublishesSourceBeforeCreate(t *testing.T) {
	_, source, store, result := deliveryPublicationFixture(t)
	bodyFile := filepath.Join(t.TempDir(), "pull.md")
	deliveryWrite(t, bodyFile, "fresh pull presentation\n")
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), result, bodyFile); err != nil {
		t.Fatalf("remember body: %v", err)
	}
	var prepareErr error
	forge := &recoveryForgeStub{receipt: ledger.RecoveryReceipt{Number: 7, Status: "published"}}
	forge.onRecover = func(presentation ledger.RecoveryPresentation) { prepareErr = presentation.PrepareSource("") }
	recovered, err := ledger.RecoverPublication(context.Background(), store, deliveryWidgets(), source.root, source.remote,
		ledger.PublicationRequest{Item: deliveryPublicationItem, Kind: "pull"}, forge)
	if err != nil {
		t.Fatalf("recover fresh pull: %v", err)
	}
	if prepareErr != nil {
		t.Fatalf("fresh source preparation was refused: %v", prepareErr)
	}
	if reference := deliveryRemoteRef(t, source.remote, "refs/heads/"+deliveryPublicationBranch); reference != source.head {
		t.Fatalf("fresh remote head = %q, want %s", reference, source.head)
	}
	if recovered.Status != "published" {
		t.Fatalf("fresh pull status = %q, want published", recovered.Status)
	}
}

// TestRecoveryAttachedViewReportsNewerPendingPresentation covers B5/A2: a
// committed attachment satisfies only the view it was recorded for. A newer
// pending presentation on the same attachment is pending with a current body,
// prose-needed with an unregistered or lost one, and published again once no
// newer presentation remains.
func TestRecoveryAttachedViewReportsNewerPendingPresentation(t *testing.T) {
	_, source, store, result := deliveryPublicationFixture(t)
	publicBody := "initial implement presentation\n"
	forge := &deliveryForgeStub{number: 42}
	ledger.PublishDelivery(context.Background(), store, deliveryWidgets(), source.root, source.remote, result, &publicBody, forge)
	satisfied := recoveryPullView(t, store, deliveryPublicationItem)
	if satisfied.Status != "published" || satisfied.Attachment == nil || satisfied.Attachment.Number != 42 {
		t.Fatalf("satisfied attached view = %#v, want the recorded attachment", satisfied)
	}

	// The latest committed view is now the watchdog review while the attachment
	// still belongs to the earlier implement presentation.
	watchdog := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	handoff := deliveryHandoff(t, store, deliveryWidgets(), deliveryPublicationItem, ledger.WatchdogPhase, watchdog.Claim.Commit,
		ledger.SourceRevisions{Head: source.head, Target: deliveryTarget, Reviewed: source.head}, "pass", "review body\n")
	unregistered := recoveryPullView(t, store, deliveryPublicationItem)
	if unregistered.Status != "prose-needed" || unregistered.Attachment == nil {
		t.Fatalf("unregistered newer view = %#v, want prose-needed with the attachment retained", unregistered)
	}

	bodyFile := filepath.Join(t.TempDir(), "newer.md")
	deliveryWrite(t, bodyFile, "newer approved presentation\n")
	if err := ledger.RememberDeliveryBody(store, deliveryWidgets(), handoff, bodyFile); err != nil {
		t.Fatalf("remember newer body: %v", err)
	}
	pending := recoveryPullView(t, store, deliveryPublicationItem)
	if pending.Status != "pending" || pending.BodyPath != bodyFile || pending.Token == satisfied.Token {
		t.Fatalf("current newer view = %#v, want a distinct pending body", pending)
	}

	if err := os.Remove(bodyFile); err != nil {
		t.Fatal(err)
	}
	lost := recoveryPullView(t, store, deliveryPublicationItem)
	if lost.Status != "prose-needed" || lost.BodyPath != bodyFile {
		t.Fatalf("lost newer body = %#v, want prose-needed with the registered path", lost)
	}
}

// recoveryAcceptedIssue accepts one single-slice proposal without a forge and
// returns the ledger, its store, and the registered temporary issue body path.
func recoveryAcceptedIssue(t *testing.T, proposal string) (*deliveryLedger, *ledger.Store, string) {
	t.Helper()
	l := newDeliveryLedger(t)
	store := l.store()
	directory := filepath.Join(t.TempDir(), proposal)
	writeRecoveryProposal(t, directory, proposal, false)
	body := []byte("descriptive issue of " + proposal + "\n")
	declaration, err := ledger.LoadDeclaration(directory, map[string][]byte{"foundation": body}, nil)
	if err != nil {
		t.Fatalf("load declaration: %v", err)
	}
	bodyPath := filepath.Join(t.TempDir(), "issue.md")
	deliveryWrite(t, bodyPath, string(body))
	declaration.IssueBodyPaths = map[string]string{"foundation": bodyPath}
	if _, err := ledger.Accept(context.Background(), store, deliveryWidgets(), declaration, nil, func() time.Time {
		return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	}); err != nil {
		t.Fatalf("accept %s: %v", proposal, err)
	}
	return l, store, bodyPath
}
