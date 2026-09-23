package main

// Public-CLI regression coverage for the recovery failure modes: lost creates,
// partial effects, unavailable or duplicated observations, racing local work,
// conflicting attachments, and explicit inline findings. The setup is real
// local source/ledger Git plus the controlled HTTP forge; no fake forge
// interface stands in for the production adapter.

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
)

const publicationPrivateSentinel = "PRIVATE worker exchange sentinel"

// publicationPendingUpdate is one Work Item whose PR attachment was published
// normally but whose latest Ready-for-Merge presentation is still pending with
// a registered temporary body.
type publicationPendingUpdate struct {
	fixture         *ledgerFixture
	forge           *publicationForge
	source          string
	target          string
	remote          string
	proposal        string
	item            string
	number          int
	reviewed        string
	finalPublic     string
	finalPublicPath string
}

func newPublicationPendingUpdate(t *testing.T, proposal string, addDebtMarker ...bool) *publicationPendingUpdate {
	t.Helper()
	u := &publicationPendingUpdate{
		fixture: newLedgerFixture(t), forge: newPublicationForge(t),
		proposal: proposal, item: proposal + "/foundation",
		finalPublic: "approved human-facing final presentation\n",
	}
	u.source, u.target, u.remote = publicationSourceRepo(t)
	u.forge.branchRemote = u.remote

	accept := newPublicationApp(t, u.forge)
	directory, flags, _ := publicationProposal(t, singleSlice(proposal), map[string]string{"foundation": "issue body\n"})
	if outcome := accept.accept(t, u.source, directory, flags...); outcome.Status != "accepted" {
		t.Fatalf("acceptance = %q", outcome.Status)
	}
	online := newPublicationApp(t, u.forge)
	offline := deliveryNoForgeApp(t)

	reportPath := filepath.Join(t.TempDir(), "private-report.md")
	writeFile(t, reportPath, "# Phase report\n\n"+publicationPrivateSentinel+"\n")
	initialPublicPath := filepath.Join(t.TempDir(), "initial-public.md")
	writeFile(t, initialPublicPath, "initial human-facing progress\n")
	u.finalPublicPath = filepath.Join(t.TempDir(), "final-public.md")
	writeFile(t, u.finalPublicPath, u.finalPublic)

	started, err := online.deliveryJSON(t, "skl", "implement", "next", "--repo", u.source, "--format", "json")
	if err != nil {
		t.Fatalf("implement next: %v", err)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := online.deliveryJSON(t, "skl", "implement", "prepare", "--repo", u.source, "--item", u.item, "--claim", claim, "--format", "json")
	if err != nil {
		t.Fatalf("implement prepare: %v", err)
	}
	worktree := prepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "feature.txt"), "delivered foundation\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "implement foundation")
	head := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
	u.reviewed = head
	if _, err := online.deliveryJSON(t, "skl", "implement", "submit", "--repo", u.source, "--item", u.item, "--claim", claim,
		"--head", head, "--target", u.target, "--body", reportPath, "--public-body", initialPublicPath, "--format", "json"); err != nil {
		t.Fatalf("implement submit: %v", err)
	}
	attached := publicationState(t, u.fixture.clone, proposal, "foundation")
	if attached.Submission == nil || attached.State != ledger.AwaitingReview {
		t.Fatalf("normal publication did not attach a Submission: %#v", attached)
	}
	u.number = attached.Submission.Number

	review, err := offline.deliveryJSON(t, "skl", "watchdog", "next", "--repo", u.source, "--format", "json")
	if err != nil {
		t.Fatalf("watchdog next: %v", err)
	}
	reviewClaim := review.Execution.Claim.Commit
	if _, err := offline.deliveryJSON(t, "skl", "watchdog", "prepare", "--repo", u.source, "--item", u.item, "--claim", reviewClaim, "--format", "json"); err != nil {
		t.Fatalf("watchdog prepare: %v", err)
	}
	if _, err := offline.deliveryJSON(t, "skl", "watchdog", "inspect", "--repo", u.source, "--item", u.item, "--claim", reviewClaim, "--format", "json"); err != nil {
		t.Fatalf("watchdog inspect: %v", err)
	}
	if len(addDebtMarker) > 0 && addDebtMarker[0] {
		writeFile(t, filepath.Join(worktree, "feature.txt"), "delivered foundation\n# maintenance note\n")
		runGit(t, worktree, "add", "feature.txt")
		runGit(t, worktree, "commit", "-q", "-m", "record permitted debt marker")
		head = strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
	}
	if _, err := offline.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", u.source, "--item", u.item, "--claim", reviewClaim,
		"--head", head, "--outcome", "pass", "--body", reportPath, "--public-body", u.finalPublicPath, "--format", "json"); err != nil {
		t.Fatalf("passing watchdog submit: %v", err)
	}
	pending := publicationState(t, u.fixture.clone, proposal, "foundation")
	if pending.State != ledger.ReadyForMerge || pending.Publication == nil || pending.Publication.PullBody == nil {
		t.Fatalf("final local state = %#v", pending)
	}
	if pending.Submission == nil {
		t.Fatal("the attachment was lost during the offline presentation")
	}
	return u
}

// publicationRevisionReads reads one committed ledger path at HEAD.
func publicationRevisionReads(t *testing.T, clone, path string) string {
	t.Helper()
	return runGitOutput(t, clone, "show", "HEAD:"+path)
}

// publicationCommitState rewrites one committed state record, modeling an
// externally changed or lost local attachment for a focused observation.
func publicationCommitState(t *testing.T, clone, proposal string, state ledger.SliceState) {
	t.Helper()
	path := filepath.Join(clone, "projects", "widgets", "proposals", proposal, "foundation", "state.json")
	writeFile(t, path, mustJSON(t, state)+"\n")
	runGit(t, clone, "add", "-A")
	runGit(t, clone, "commit", "-q", "-m", "external attachment change")
}

// TestPublicationCLIReusesRegisteredBodyAcrossUnrelatedCommit proves B2/B3/B7:
// an unrelated ledger commit does not stale the registered presentation, the
// temporary agent prose is reused unchanged, and no private evidence reaches
// the forge.
func TestPublicationCLIReusesRegisteredBodyAcrossUnrelatedCommit(t *testing.T) {
	u := newPublicationPendingUpdate(t, "reuse-current")
	runGit(t, u.fixture.clone, "commit", "--allow-empty", "-m", "unrelated ledger change")

	// Inspection reports the still-pending latest presentation rather than a
	// false satisfied result, and names the reusable registered body.
	inspect := publicationNoForgeApp(t, nil)
	view, err := inspect.publicationJSON(t, "skl", "publication", "inspect", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("inspect current pull: %v", err)
	}
	facts := view.Packet.Facts.Publication
	if facts.Condition != skilldist.PublicationCurrentBody || view.Packet.Skill != "watchdog" || facts.View.BodyPath != u.finalPublicPath {
		t.Fatalf("current-body view = %#v", facts)
	}

	cli := newPublicationApp(t, u.forge)
	recovered, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("recover current pull: %v", err)
	}
	if recovered.Status != "published" {
		t.Fatalf("recovery = %#v, want published", recovered)
	}
	pull := u.forge.pull(u.number)
	if pull.Body != u.finalPublic || pull.Draft {
		t.Fatalf("presented pull = %#v", pull)
	}
	if len(u.forge.pullPatches) != 1 || u.forge.pullPatches[0]["body"] != u.finalPublic {
		t.Fatalf("body patches = %#v", u.forge.pullPatches)
	}
	for _, payload := range append(append(append([]map[string]any{}, u.forge.pullCreates...), u.forge.pullPatches...), u.forge.issueCreates...) {
		if serialized := mustJSON(t, payload); strings.Contains(serialized, publicationPrivateSentinel) {
			t.Fatalf("private evidence reached the forge: %s", serialized)
		}
	}
	if stateJSON := mustJSON(t, publicationState(t, u.fixture.clone, u.proposal, "foundation")); strings.Contains(stateJSON, u.finalPublic) {
		t.Fatalf("ledger persisted public prose: %s", stateJSON)
	}
}

// TestPublicationCLIRefusesStaleAndNeedsProseForLostBody proves B2/B5: a
// superseded registered body is never published, and a missing body yields
// exact references and a bound authoring continuation instead of a fake
// satisfaction.
func TestPublicationCLIRefusesStaleAndNeedsProseForLostBody(t *testing.T) {
	stale := newPublicationPendingUpdate(t, "stale-body")
	reportPath := "projects/widgets/proposals/stale-body/foundation/watchdog-report.md"
	writeFile(t, filepath.Join(stale.fixture.clone, filepath.FromSlash(reportPath)),
		publicationRevisionReads(t, stale.fixture.clone, reportPath)+"\nnewer reviewed result\n")
	runGit(t, stale.fixture.clone, "add", "-A")
	runGit(t, stale.fixture.clone, "commit", "-q", "-m", "record newer reviewed result")

	cli := newPublicationApp(t, stale.forge)
	before := len(stale.forge.recordedRequests())
	refused, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", stale.source,
		"--item", stale.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("stale recovery: %v", err)
	}
	if refused.Status != "stale" {
		t.Fatalf("stale recovery = %#v, want stale", refused)
	}
	if len(stale.forge.recordedRequests()) != before || len(stale.forge.pullPatches) != 0 {
		t.Fatal("stale recovery mutated the forge")
	}
	if refused.Packet.Facts.Publication.Condition != skilldist.PublicationStale || refused.Packet.Facts.Publication.ResultDirectory == "" {
		t.Fatalf("stale continuation = %#v", refused.Packet.Facts.Publication)
	}

	lost := newPublicationPendingUpdate(t, "lost-final-body")
	if err := os.Remove(lost.finalPublicPath); err != nil {
		t.Fatal(err)
	}
	before = len(lost.forge.recordedRequests())
	needed, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", lost.source,
		"--item", lost.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("lost-body recovery: %v", err)
	}
	if needed.Status != "prose-needed" {
		t.Fatalf("lost-body recovery = %#v, want prose-needed", needed)
	}
	facts := needed.Packet.Facts.Publication
	if facts.Condition != skilldist.PublicationProseNeeded || !strings.Contains(facts.ResourceCommand, "reference/publication.md") {
		t.Fatalf("lost-body continuation = %#v", facts)
	}
	if len(facts.ReferenceCommands) == 0 || facts.View.Report == nil {
		t.Fatalf("lost-body references = %#v", facts.ReferenceCommands)
	}
	if len(lost.forge.recordedRequests()) != before {
		t.Fatal("lost-body recovery mutated the forge")
	}
	// Read-only inspection of the same lost body reports the authoring
	// continuation instead of a false satisfied attachment.
	lostInspect := publicationNoForgeApp(t, nil)
	inspection, err := lostInspect.publicationJSON(t, "skl", "publication", "inspect", "--repo", lost.source,
		"--item", lost.item, "--kind", "pull", "--result-directory", filepath.Dir(lost.finalPublicPath), "--format", "json")
	if err != nil {
		t.Fatalf("inspect lost pull body: %v", err)
	}
	if inspection.Status != "prose-needed" || inspection.Packet.Facts.Publication.Condition != skilldist.PublicationProseNeeded {
		t.Fatalf("inspect lost pull body = %#v", inspection)
	}
}

// TestPublicationCLIRecoversLostPullCreateWithoutDuplicate proves B4/A3: a
// pull create whose response and resolution listing were both lost is adopted
// from the exact observable branch occupant, then recognized as satisfied
// without a second create.
func TestPublicationCLIRecoversLostPullCreateWithoutDuplicate(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newPublicationForge(t)
	source, target, remote := publicationSourceRepo(t)
	forge.branchRemote = remote
	accept := newPublicationApp(t, forge)
	directory, flags, _ := publicationProposal(t, singleSlice("lost-pull"), map[string]string{"foundation": "issue body\n"})
	if outcome := accept.accept(t, source, directory, flags...); outcome.Status != "accepted" {
		t.Fatalf("acceptance = %q", outcome.Status)
	}
	// The pre-create listing succeeds so creation is attempted; only the
	// fallback listing that would resolve the lost response fails.
	listings := 0
	forge.setDrop(func(method, path string) bool {
		return method == "POST" && path == "/repos/acme/widgets/pulls"
	})
	forge.setFail(func(method, path string) int {
		if method == "GET" && path == "/repos/acme/widgets/pulls" {
			listings++
			if listings > 1 {
				return 503
			}
		}
		return 0
	})

	online := newPublicationApp(t, forge)
	reportPath := filepath.Join(t.TempDir(), "private-report.md")
	writeFile(t, reportPath, "# Phase report\n\n"+publicationPrivateSentinel+"\n")
	publicPath := filepath.Join(t.TempDir(), "public.md")
	writeFile(t, publicPath, "initial human-facing progress\n")
	const item = "lost-pull/foundation"
	started, err := online.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("implement next: %v", err)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := online.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", item, "--claim", claim, "--format", "json")
	if err != nil {
		t.Fatalf("implement prepare: %v", err)
	}
	worktree := prepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "feature.txt"), "delivered\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "implement")
	head := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
	if _, err := online.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", item, "--claim", claim,
		"--head", head, "--target", target, "--body", reportPath, "--public-body", publicPath, "--format", "json"); err != nil {
		t.Fatalf("implement submit: %v", err)
	}
	forge.setDrop(nil)
	forge.setFail(nil)
	if forge.pullCount() != 1 {
		t.Fatalf("the dropped create stored %d pulls, want 1", forge.pullCount())
	}
	unattached := publicationState(t, fixture.clone, "lost-pull", "foundation")
	if unattached.Submission != nil || unattached.Publication == nil || unattached.Publication.Pull == nil {
		t.Fatalf("lost create state = %#v", unattached)
	}

	cli := newPublicationApp(t, forge)
	recovered, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("recover lost create: %v", err)
	}
	if recovered.Status != "already-satisfied" && recovered.Status != "published" {
		t.Fatalf("lost-create recovery = %#v", recovered)
	}
	adopted := publicationState(t, fixture.clone, "lost-pull", "foundation")
	if adopted.Submission == nil || adopted.Submission.Number == 0 {
		t.Fatalf("lost create was not adopted: %#v", adopted.Submission)
	}
	if forge.count("POST", "/repos/acme/widgets/pulls") != 1 {
		t.Fatalf("recovery created a duplicate pull request")
	}

	requests := len(forge.recordedRequests())
	repeat, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("repeat lost-create recovery: %v", err)
	}
	if repeat.Status != "already-satisfied" {
		t.Fatalf("repeated recovery = %q", repeat.Status)
	}
	if len(forge.recordedRequests()) != requests {
		t.Fatal("repeated recovery issued another forge request")
	}
}

// TestPublicationCLIRecoversPartialBodyAndReadinessEffects proves B4/A3: a
// body update whose readiness response was lost is confirmed from observation
// without repeating either mutation, and a remaining missing effect is applied
// on its own.
func TestPublicationCLIRecoversPartialBodyAndReadinessEffects(t *testing.T) {
	u := newPublicationPendingUpdate(t, "partial-effects")
	u.forge.setDrop(func(method, path string) bool {
		return method == "POST" && path == "/graphql"
	})
	cli := newPublicationApp(t, u.forge)
	recovered, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("recover partial effects: %v", err)
	}
	if recovered.Status != "published" || u.forge.pull(u.number).Draft {
		t.Fatalf("partial recovery = %#v, pull = %#v", recovered, u.forge.pull(u.number))
	}
	if len(u.forge.pullPatches) != 1 || u.forge.count("POST", "/graphql") != 1 {
		t.Fatalf("partial recovery effects: patches=%d graphql=%d", len(u.forge.pullPatches), u.forge.count("POST", "/graphql"))
	}
	requests := len(u.forge.recordedRequests())
	satisfied, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("repeat partial recovery: %v", err)
	}
	if satisfied.Status != "already-satisfied" || len(u.forge.recordedRequests()) != requests {
		t.Fatalf("repeated partial recovery = %#v, requests unchanged = %v", satisfied, len(u.forge.recordedRequests()) == requests)
	}

	// A restored body with only readiness missing applies readiness alone.
	onlyReady := newPublicationPendingUpdate(t, "readiness-only")
	onlyReady.forge.setPullBody(onlyReady.number, onlyReady.finalPublic)
	beforePatches := len(onlyReady.forge.pullPatches)
	applied, err := newPublicationApp(t, onlyReady.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", onlyReady.source,
		"--item", onlyReady.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("readiness-only recovery: %v", err)
	}
	if applied.Status != "published" || onlyReady.forge.pull(onlyReady.number).Draft {
		t.Fatalf("readiness-only recovery = %#v", applied)
	}
	if len(onlyReady.forge.pullPatches) != beforePatches || onlyReady.forge.count("POST", "/graphql") != 1 {
		t.Fatalf("readiness-only recovery repeated a body write: patches=%d", len(onlyReady.forge.pullPatches))
	}
}

// TestPublicationCLIStopsOnUnavailableOrDuplicateObservation proves B4: an
// unreadable attachment or several conflicting branch occupants never causes a
// blind duplicate write, and the pending work is preserved.
func TestPublicationCLIStopsOnUnavailableOrDuplicateObservation(t *testing.T) {
	unavailable := newPublicationPendingUpdate(t, "unavailable-read")
	unavailable.forge.setFail(func(method, path string) int {
		if method == "GET" && strings.HasPrefix(path, "/repos/acme/widgets/pulls/") {
			return 503
		}
		return 0
	})
	cli := newPublicationApp(t, unavailable.forge)
	blocked, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", unavailable.source,
		"--item", unavailable.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("unavailable readback: %v", err)
	}
	if blocked.Status != "pending" || len(unavailable.forge.pullPatches) != 0 || unavailable.forge.count("POST", "/graphql") != 0 {
		t.Fatalf("unavailable readback = %#v", blocked)
	}
	if state := publicationState(t, unavailable.fixture.clone, unavailable.proposal, "foundation"); state.Publication == nil || state.Publication.Pull == nil {
		t.Fatalf("unavailable readback cleared pending work: %#v", state.Publication)
	}

	duplicate := newPublicationPendingUpdate(t, "duplicate-candidates")
	// The recorded attachment is gone, so resolution must observe the branch
	// population instead of reading one known number.
	state := publicationState(t, duplicate.fixture.clone, duplicate.proposal, "foundation")
	state.Submission = nil
	publicationCommitState(t, duplicate.fixture.clone, duplicate.proposal, state)
	duplicate.forge.addPull(publicationPull{Branch: "foundation", Base: "main", Body: "other branch occupant"})
	creates := len(duplicate.forge.pullCreates)
	cli = newPublicationApp(t, duplicate.forge)
	stale, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", duplicate.source,
		"--item", duplicate.item, "--kind", "pull", "--format", "json")
	if err != nil || stale.Status != "stale" {
		t.Fatalf("changed attachment must stale the original request: %#v, %v", stale, err)
	}
	conflicting, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", duplicate.source,
		"--item", duplicate.item, "--kind", "pull", "--view", stale.Packet.Facts.Publication.View.Token,
		"--body", duplicate.finalPublicPath, "--format", "json")
	if err != nil {
		t.Fatalf("duplicate candidates: %v", err)
	}
	if len(duplicate.forge.pullPatches) != 0 || duplicate.forge.count("POST", "/graphql") != 0 {
		t.Fatal("duplicate candidates caused a mutation")
	}
	if conflicting.Status != "pending" || len(duplicate.forge.pullCreates) != creates {
		t.Fatalf("duplicate candidates = %#v", conflicting)
	}
}

// TestPublicationCLIPreservesNewerViewDuringInFlightEffect proves A2/B5: while
// an external effect is in flight, a newer selected result can be committed
// because no ledger lock is held, and recovery retains its receipt without
// claiming the newer view published or clearing its pending work.
func TestPublicationCLIPreservesNewerViewDuringInFlightEffect(t *testing.T) {
	u := newPublicationPendingUpdate(t, "racing-view")
	entered := make(chan struct{})
	release := make(chan struct{})
	u.forge.setAfterMutation(func(method, path string) {
		if method == "PATCH" && strings.HasSuffix(path, "/pulls/"+strconv.Itoa(u.number)) {
			close(entered)
			<-release
		}
	})
	done := make(chan publicationOutput, 1)
	go func() {
		out, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
			"--item", u.item, "--kind", "pull", "--format", "json")
		if err != nil {
			out.Status = "error: " + err.Error()
		}
		done <- out
	}()
	<-entered

	reportPath := "projects/widgets/proposals/racing-view/foundation/watchdog-report.md"
	writeFile(t, filepath.Join(u.fixture.clone, filepath.FromSlash(reportPath)),
		publicationRevisionReads(t, u.fixture.clone, reportPath)+"\nnewer reviewed result\n")
	runGit(t, u.fixture.clone, "commit", "--allow-empty", "-m", "later claim edge")
	claimBasis := strings.TrimSpace(runGitOutput(t, u.fixture.clone, "rev-parse", "HEAD"))
	newer := publicationState(t, u.fixture.clone, u.proposal, "foundation")
	newer.Claim = &ledger.Claim{Phase: ledger.ImplementPhase, Basis: claimBasis}
	publicationCommitState(t, u.fixture.clone, u.proposal, newer)
	current, err := publicationNoForgeApp(t, nil).publicationJSON(t, "skl", "publication", "inspect", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	newBodyPath := filepath.Join(t.TempDir(), "newer-public.md")
	newBody := "newer human-facing result\n"
	writeFile(t, newBodyPath, newBody)
	newer.Publication.PullBody = &ledger.PublicationBody{
		Path: newBodyPath, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(newBody))), View: current.Packet.Facts.Publication.View.Token,
	}
	publicationCommitState(t, u.fixture.clone, u.proposal, newer)
	close(release)

	raced := <-done
	if raced.Status != "pending" || !strings.Contains(raced.Packet.Facts.Publication.View.Detail, "newer") {
		t.Fatalf("raced recovery = %#v", raced)
	}
	state := publicationState(t, u.fixture.clone, u.proposal, "foundation")
	if state.State != ledger.ReadyForMerge || state.Publication == nil || state.Publication.Pull == nil {
		t.Fatalf("raced recovery changed the newer lifecycle: %#v", state)
	}
	if state.Claim == nil || state.Claim.Basis != claimBasis {
		t.Fatalf("raced recovery released the later Claim: %#v", state.Claim)
	}
	if state.Submission == nil {
		t.Fatal("raced recovery lost its observable attachment")
	}
	if state.Publication.PullBody == nil || *state.Publication.PullBody != *newer.Publication.PullBody {
		t.Fatalf("raced recovery replaced the newer public body registration: %#v", state.Publication.PullBody)
	}
	report := publicationReport(t, u.fixture.clone, u.proposal, "foundation", ledger.WatchdogPhase)
	if !strings.Contains(publicationRevisionReads(t, u.fixture.clone, reportPath), "newer reviewed result") || report.Round == 0 {
		t.Fatal("raced recovery overwrote the newer report")
	}
}

// TestPublicationCLIRefusesConflictingAttachment proves B5/A3: a different
// repository, base, branch, or a closed attachment is refused without
// replacement, retargeting, or a forced source update.
func TestPublicationCLIRefusesConflictingAttachment(t *testing.T) {
	cases := []struct {
		name   string
		change func(*publicationForge, int)
	}{
		{"wrong-base", func(forge *publicationForge, number int) { forge.setPullBase(number, "develop") }},
		{"wrong-branch", func(forge *publicationForge, number int) { forge.setPullBranch(number, "other") }},
		{"wrong-repository", func(forge *publicationForge, number int) { forge.setPullOwner(number, "other/widgets") }},
		{"closed", func(forge *publicationForge, number int) { forge.setPullState(number, "closed") }},
		{"unexpected-head", func(forge *publicationForge, number int) {
			forge.branchRemote = ""
			forge.setPullHead(number, strings.Repeat("9", 40))
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			u := newPublicationPendingUpdate(t, "conflict-"+testCase.name)
			testCase.change(u.forge, u.number)
			cli := newPublicationApp(t, u.forge)
			out, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
				"--item", u.item, "--kind", "pull", "--format", "json")
			if err != nil {
				t.Fatalf("%s recovery: %v", testCase.name, err)
			}
			if out.Status != "pending" && out.Status != "ambiguous" {
				t.Fatalf("%s = %#v, want a refusal", testCase.name, out)
			}
			if len(u.forge.pullPatches) != 0 || u.forge.count("POST", "/graphql") != 0 || len(u.forge.pullCreates) != 1 {
				t.Fatalf("%s performed a replacement, retarget, or readiness write", testCase.name)
			}
			if testCase.name == "unexpected-head" {
				if remoteHead := gitOutputString(u.remote, "rev-parse", "--verify", "refs/heads/foundation"); remoteHead == "" || remoteHead != u.reviewed {
					t.Fatalf("unexpected head forced a source update: remote=%q", remoteHead)
				}
			}
		})
	}
}

// TestPublicationCLISelectsInlineFindingsIndependently proves B8: only the
// explicitly selected finding at its exact reviewed anchor is published, lost
// responses deduplicate, invalid or stale anchors stay unresolved, and the body
// result remains independently visible. No selection publishes nothing.
func TestPublicationCLISelectsInlineFindingsIndependently(t *testing.T) {
	u := newPublicationPendingUpdate(t, "inline-findings")
	u.forge.setPullFiles(publicationReviewFile{
		Filename: "code.txt", Status: "modified",
		Patch: "@@ -1,2 +1,3 @@\n line one\n-old line\n+new line\n+added line\n",
	})
	valid := ledger.SelectedFinding{ID: "W2", Body: "actionable human-facing finding\n", Commit: u.reviewed, Path: "code.txt", Line: 2, Side: "RIGHT"}
	missingPath := ledger.SelectedFinding{ID: "W1", Body: "unanchored finding\n", Commit: u.reviewed, Path: "missing.txt", Line: 1, Side: "RIGHT"}
	staleRevision := ledger.SelectedFinding{ID: "W3", Body: "stale finding\n", Commit: strings.Repeat("b", 40), Path: "code.txt", Line: 1, Side: "RIGHT"}
	findingsPath := filepath.Join(t.TempDir(), "findings.json")
	writeFile(t, findingsPath, mustJSON(t, []ledger.SelectedFinding{valid, missingPath, staleRevision})+"\n")

	// A lost inline-comment response still deduplicates from observation.
	u.forge.setDrop(func(method, path string) bool {
		return method == "POST" && strings.HasSuffix(path, "/comments")
	})
	cli := newPublicationApp(t, u.forge)
	recovered, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--findings", findingsPath, "--format", "json")
	if err != nil {
		t.Fatalf("recover with findings: %v", err)
	}
	if recovered.Status != "published" {
		t.Fatalf("body publication = %q, want an independent published result", recovered.Status)
	}
	statuses := map[string]string{}
	for _, finding := range recovered.Findings {
		statuses[finding.ID] = finding.Status
	}
	if statuses["W2"] != "satisfied" || statuses["W1"] != "unresolved" || statuses["W3"] != "unresolved" {
		t.Fatalf("finding outcomes = %#v", statuses)
	}
	if u.forge.commentCount() != 1 || len(u.forge.commentCreates) != 1 || u.forge.commentCreates[0]["body"] != valid.Body {
		t.Fatalf("inline effects = %#v", u.forge.commentCreates)
	}
	if u.forge.pull(u.number).Body != u.finalPublic {
		t.Fatal("finding publication suppressed or replaced the body result")
	}

	// No selection publishes no private report finding inline.
	unselected := newPublicationPendingUpdate(t, "no-inline-selection")
	body, err := newPublicationApp(t, unselected.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", unselected.source,
		"--item", unselected.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("recover without findings: %v", err)
	}
	if body.Status != "published" || unselected.forge.commentCount() != 0 {
		t.Fatalf("unselected recovery published %d inline findings", unselected.forge.commentCount())
	}
}

// TestPublicationCLIRejectsMalformedFindingSelections proves the CLI requires
// a complete explicit JSON array with unique identities before any effect.
func TestPublicationCLIRejectsMalformedFindingSelections(t *testing.T) {
	u := newPublicationPendingUpdate(t, "malformed-findings")
	cli := newPublicationApp(t, u.forge)
	duplicate := []ledger.SelectedFinding{
		{ID: "W1", Body: "one\n", Commit: u.reviewed, Path: "code.txt", Line: 1, Side: "RIGHT"},
		{ID: "W1", Body: "two\n", Commit: u.reviewed, Path: "code.txt", Line: 1, Side: "RIGHT"},
	}
	path := filepath.Join(t.TempDir(), "duplicate.json")
	writeFile(t, path, mustJSON(t, duplicate)+"\n")
	before := len(u.forge.recordedRequests())
	out, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--findings", path, "--format", "json")
	if err != nil {
		t.Fatalf("duplicate findings: %v", err)
	}
	if out.Status != "fix_required" || !strings.Contains(out.Reason, "more than once") {
		t.Fatalf("duplicate findings = %#v", out)
	}
	writeFile(t, path, `[{"id": "W1", "body": ""}]`+"\n")
	incomplete, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--findings", path, "--format", "json")
	if err != nil {
		t.Fatalf("incomplete findings: %v", err)
	}
	if incomplete.Status != "fix_required" {
		t.Fatalf("incomplete findings = %#v", incomplete)
	}
	writeFile(t, path, `{"id": "W1"}`+"\n")
	notArray, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--findings", path, "--format", "json")
	if err != nil {
		t.Fatalf("non-array findings: %v", err)
	}
	if notArray.Status != "fix_required" || len(u.forge.recordedRequests()) != before {
		t.Fatalf("non-array findings = %#v, forge requests changed", notArray)
	}

	// Newly authored prose requires the exact current view token before any
	// effect.
	body := filepath.Join(t.TempDir(), "body.md")
	writeFile(t, body, "supplied body without a view\n")
	noView, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--body", body, "--format", "json")
	if err != nil {
		t.Fatalf("body without view: %v", err)
	}
	if noView.Status != "stale" || !strings.Contains(noView.Packet.Facts.Publication.View.Detail, "view token") {
		t.Fatalf("body without view = %#v", noView)
	}
	if len(u.forge.recordedRequests()) != before {
		t.Fatal("body without view reached the forge")
	}
}

// TestPublicationCLIPreservesSuppliedBodyOnOutputInterruption proves the
// supplied public body and the committed effect survive a lost CLI
// acknowledgement.
func TestPublicationCLIPreservesSuppliedBodyOnOutputInterruption(t *testing.T) {
	u := newPublicationPendingUpdate(t, "interrupted-output")
	suppliedPath := filepath.Join(t.TempDir(), "supplied.md")
	supplied := "fresh supplied public body\n"
	writeFile(t, suppliedPath, supplied)
	token := publicationState(t, u.fixture.clone, u.proposal, "foundation").Publication.PullBody.View

	interrupted := newApp(publicationBackendFactory(u.forge), bytes.NewReader(nil), deliveryInterruptedWriter{}, deliveryInterruptedWriter{})
	err := interrupted.Run([]string{"skl", "publication", "recover", "--repo", u.source, "--item", u.item, "--kind", "pull", "--view", token, "--body", suppliedPath})
	if err == nil {
		t.Fatal("interrupted output did not fail the invocation")
	}
	if _, statErr := os.Stat(suppliedPath); statErr != nil {
		t.Fatalf("the supplied public body was removed: %v", statErr)
	}
	if presented := u.forge.pull(u.number).Body; presented != supplied {
		t.Fatalf("presented body = %q, want the supplied prose", presented)
	}
}

// TestPublicationCLIFreshPullCreateEvidence covers B6: a phase whose initial
// presentation was never dispatched, because the backend was unavailable,
// still permits recovery to create its first draft pull request.
func TestPublicationCLIFreshPullCreateEvidence(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newPublicationForge(t)
	source, target, remote := publicationSourceRepo(t)
	forge.branchRemote = remote
	accept := newPublicationApp(t, forge)
	directory, flags, _ := publicationProposal(t, singleSlice("fresh-pull"), map[string]string{"foundation": "issue body\n"})
	if outcome := accept.accept(t, source, directory, flags...); outcome.Status != "accepted" {
		t.Fatalf("acceptance = %q", outcome.Status)
	}
	offline := deliveryNoForgeApp(t)
	reportPath := filepath.Join(t.TempDir(), "private-report.md")
	writeFile(t, reportPath, "# Phase report\n\n"+publicationPrivateSentinel+"\n")
	publicPath := filepath.Join(t.TempDir(), "public.md")
	writeFile(t, publicPath, "human-facing progress\n")
	const item = "fresh-pull/foundation"
	started, err := offline.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("implement next: %v", err)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := offline.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", item, "--claim", claim, "--format", "json")
	if err != nil {
		t.Fatalf("implement prepare: %v", err)
	}
	worktree := prepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "feature.txt"), "delivered\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "implement")
	head := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
	if _, err := offline.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", item, "--claim", claim,
		"--head", head, "--target", target, "--body", reportPath, "--public-body", publicPath, "--format", "json"); err != nil {
		t.Fatalf("implement submit: %v", err)
	}

	recovered, err := newPublicationApp(t, forge).publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("recover fresh pull: %v", err)
	}
	state := publicationState(t, fixture.clone, "fresh-pull", "foundation")
	if recovered.Status != "published" || state.Submission == nil || forge.pullCount() != 1 || !forge.pull(state.Submission.Number).Draft {
		t.Fatalf("fresh pull recovery = %#v, creates = %d", recovered, forge.pullCount())
	}
}

// TestPublicationCLIReportsNewerPendingOnAttachedPull proves B5/A2: a recorded
// attachment satisfies only the view it was recorded for. A newer pending
// presentation on the same attached pull is pending with a current body and
// prose-needed once no body remains, so the CLI honors the ledger status
// instead of re-checking the file system. Supplying current prose updates the
// attachment rather than creating a duplicate, and a later inspection reports
// the satisfied attachment.
func TestPublicationCLIReportsNewerPendingOnAttachedPull(t *testing.T) {
	u := newPublicationPendingUpdate(t, "attached-newer")
	// Drop the registered temporary body to model a newer selected
	// presentation that is pending with no authored prose yet.
	state := publicationState(t, u.fixture.clone, u.proposal, "foundation")
	if state.Submission == nil || state.Publication == nil || state.Publication.Pull == nil {
		t.Fatalf("fixture is not an attached pending presentation: %#v", state)
	}
	state.Publication.PullBody = nil
	publicationCommitState(t, u.fixture.clone, u.proposal, state)

	inspect := publicationNoForgeApp(t, nil)
	needed, err := inspect.publicationJSON(t, "skl", "publication", "inspect", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--result-directory", t.TempDir(), "--format", "json")
	if err != nil {
		t.Fatalf("inspect attached pending pull: %v", err)
	}
	if needed.Status != "prose-needed" || needed.Packet.Facts.Publication.Condition != skilldist.PublicationProseNeeded {
		t.Fatalf("attached pending inspection = %#v, want prose-needed", needed)
	}

	// A current authored body updates the existing attachment, and a later
	// inspection reports the satisfied attachment rather than a pending view.
	token := needed.Packet.Facts.Publication.View.Token
	bodyPath := filepath.Join(t.TempDir(), "authored.md")
	const authored = "newer human-facing presentation\n"
	writeFile(t, bodyPath, authored)
	creates := len(u.forge.pullCreates)
	recovered, err := newPublicationApp(t, u.forge).publicationJSON(t, "skl", "publication", "recover", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--view", token, "--body", bodyPath, "--format", "json")
	if err != nil {
		t.Fatalf("recover attached pending pull: %v", err)
	}
	if recovered.Status != "published" {
		t.Fatalf("attached pending recovery = %#v, want published", recovered)
	}
	if len(u.forge.pullCreates) != creates || u.forge.pull(u.number).Body != authored {
		t.Fatalf("recovery did not reuse the attachment: creates=%d pull=%#v", len(u.forge.pullCreates), u.forge.pull(u.number))
	}
	satisfied, err := inspect.publicationJSON(t, "skl", "publication", "inspect", "--repo", u.source,
		"--item", u.item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("inspect satisfied attached pull: %v", err)
	}
	if satisfied.Status != "published" || satisfied.Packet.Facts.Publication.Condition != skilldist.PublicationSatisfied {
		t.Fatalf("satisfied attached inspection = %#v, want a satisfied attachment", satisfied)
	}
}
