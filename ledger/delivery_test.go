package ledger_test

// Delivery seam tests. They exercise the public Claim/report handoff against
// real local Git ledger fixtures: selection, exclusive Claims, local atomic
// handoff and replay, completed-review counting, and the refusal boundaries
// around incompatible, dirty, and diverged inputs.
//
// Expected outcomes come from the accepted behavior rules (B1/B4/B5/B7/B9/B11)
// and ADR 0006, not from the implementation's own computations. Source revision
// identities are shape-checked by the report codec; Git-level source validation
// belongs to the higher workflow seam, so these module fixtures use exact
// revision strings without a source repository.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
)

const (
	deliveryHead    = "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a"
	deliveryTarget  = "2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b"
	deliveryRefID   = "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c"
	deliveryInitial = "2024-01-01T00:00:00Z"
)

func deliveryWidgets() github.RepositoryID {
	return github.RepositoryID{Owner: "acme", Name: "widgets"}
}

func deliveryGadgets() github.RepositoryID {
	return github.RepositoryID{Owner: "acme", Name: "gadgets"}
}

// deliveryLedger is one real local Git ledger clone holding project records.
type deliveryLedger struct {
	t    *testing.T
	root string
}

func newDeliveryLedger(t *testing.T) *deliveryLedger {
	t.Helper()
	root := filepath.Join(t.TempDir(), "ledger")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, root, "init", "-q", "-b", "main")
	deliveryGit(t, root, "config", "user.name", "Ledger")
	deliveryGit(t, root, "config", "user.email", "ledger@example.com")
	deliveryWrite(t, filepath.Join(root, "README.md"), "workflow ledger\n")
	deliveryGit(t, root, "add", "README.md")
	deliveryGit(t, root, "commit", "-q", "-m", "seed")
	return &deliveryLedger{t: t, root: root}
}

func (l *deliveryLedger) store() *ledger.Store {
	l.t.Helper()
	store, err := ledger.Open(l.root)
	if err != nil {
		l.t.Fatalf("open ledger %s: %v", l.root, err)
	}
	return store
}

func (l *deliveryLedger) head() string {
	return deliveryGitOutput(l.t, l.root, "rev-parse", "HEAD")
}

func (l *deliveryLedger) commitCount() int {
	return len(strings.Fields(deliveryGitOutput(l.t, l.root, "rev-list", "HEAD")))
}

// addProject writes one project.json record. The caller commits.
func (l *deliveryLedger) addProject(project, repository string) {
	l.t.Helper()
	deliveryWrite(l.t, filepath.Join(l.root, "projects", project, "project.json"),
		`{"repository": "`+repository+`"}`+"\n")
}

// addSlice writes one accepted slice record with a frozen minimal Contract.
// The caller commits.
func (l *deliveryLedger) addSlice(project, proposal, slice, state string, dependencies []string, accepted string) {
	l.t.Helper()
	proposalDirectory := filepath.Join(l.root, "projects", project, "proposals", proposal)
	deliveryWrite(l.t, filepath.Join(proposalDirectory, "proposal.json"),
		`{"accepted": "`+accepted+`"}`+"\n")
	deliveryWrite(l.t, filepath.Join(proposalDirectory, "proposal.md"), "proposal "+proposal+"\n")
	sliceDirectory := filepath.Join(proposalDirectory, slice)
	deliveryWrite(l.t, filepath.Join(sliceDirectory, "intent.md"), "intent of "+slice+"\n")
	deliveryWrite(l.t, filepath.Join(sliceDirectory, "behavior.md"), "behavior of "+slice+"\n")
	l.writeStateValue(project, proposal, slice, ledger.SliceState{
		State: state, Title: slice, Branch: slice, Dependencies: dependencies,
	})
}

// addFile writes an extra ledger file. The caller commits.
func (l *deliveryLedger) addFile(relative, contents string) {
	l.t.Helper()
	deliveryWrite(l.t, filepath.Join(l.root, filepath.FromSlash(relative)), contents)
}

// commitAll stages the fixture's pending writes as one ledger commit.
func (l *deliveryLedger) commitAll(message string) string {
	l.t.Helper()
	deliveryGit(l.t, l.root, "add", "-A")
	deliveryGit(l.t, l.root, "commit", "-q", "-m", message)
	return l.head()
}

func (l *deliveryLedger) committedState(project, proposal, slice string) ledger.SliceState {
	l.t.Helper()
	raw := deliveryGitShow(l.t, l.root, "HEAD", deliveryStatePath(project, proposal+"/"+slice))
	var state ledger.SliceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		l.t.Fatalf("decode committed state of %s/%s/%s: %v", project, proposal, slice, err)
	}
	return state
}

func (l *deliveryLedger) writeStateValue(project, proposal, slice string, state ledger.SliceState) {
	l.t.Helper()
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		l.t.Fatal(err)
	}
	deliveryWrite(l.t, filepath.Join(l.root, filepath.FromSlash(deliveryStatePath(project, proposal+"/"+slice))),
		string(encoded)+"\n")
}

// commitState rewrites one committed state.json and commits it, standing in
// for the human-decision requeue surface that lives outside this slice.
func (l *deliveryLedger) commitState(project, proposal, slice string, state ledger.SliceState) {
	l.t.Helper()
	l.writeStateValue(project, proposal, slice, state)
	l.commitAll("record state")
}

// recordDecision commits a decision document and returns its commit.
func (l *deliveryLedger) recordDecision(project, proposal, slice, body string) string {
	l.t.Helper()
	l.addFile(deliveryDecisionPath(project, proposal+"/"+slice), body)
	return l.commitAll("record human decision")
}

func deliverySliceDirectory(project, item string) string {
	return "projects/" + project + "/proposals/" + item
}

func deliveryStatePath(project, item string) string {
	return deliverySliceDirectory(project, item) + "/state.json"
}

func deliveryReportPath(project, item, phase string) string {
	return deliverySliceDirectory(project, item) + "/" + phase + "-report.md"
}

func deliveryDecisionPath(project, item string) string {
	return deliverySliceDirectory(project, item) + "/decision.md"
}

func deliveryContractPath(project, item, name string) string {
	return deliverySliceDirectory(project, item) + "/" + name
}

// deliveryWatchdogReport renders an independently authored schema-1 review
// report for fixture states that must already hold review evidence.
func deliveryWatchdogReport(outcome string, round int, project, item string) string {
	claimPath := deliveryStatePath(project, item)
	contractPath := deliveryContractPath(project, item, "behavior.md")
	implementPath := deliveryReportPath(project, item, ledger.ImplementPhase)
	return fmt.Sprintf(`---
schema: 1
outcome: %s
round: %d
source:
  head: %s
  target: %s
  reviewed: %s
ledger:
  claim:
    commit: %s
    path: %s
  contract:
    - commit: %s
      path: %s
  implement:
    commit: %s
    path: %s
---
W1 unresolved.
`, outcome, round, deliveryHead, deliveryTarget, deliveryHead,
		deliveryRefID, claimPath, deliveryRefID, contractPath, deliveryRefID, implementPath)
}

// deliveryStart acquires work and fails the test unless a Claim was granted.
func deliveryStart(t *testing.T, store *ledger.Store, repository github.RepositoryID, phase string) *ledger.Execution {
	t.Helper()
	execution, err := ledger.StartDelivery(store, repository, phase)
	if err != nil {
		t.Fatalf("start %s delivery: %v", phase, err)
	}
	if execution == nil {
		t.Fatalf("start %s delivery selected no work", phase)
	}
	return execution
}

// deliveryHandoff submits a phase result and fails the test on refusal.
func deliveryHandoff(t *testing.T, store *ledger.Store, repository github.RepositoryID, item, phase, claimCommit string, source ledger.SourceRevisions, outcome, body string) *ledger.DeliveryResult {
	t.Helper()
	result, err := ledger.HandoffDelivery(store, repository, item, phase, claimCommit, source, outcome, body)
	if err != nil {
		t.Fatalf("handoff %s %s: %v", phase, item, err)
	}
	return result
}

// deliveryReport parses the report currently committed at HEAD.
func deliveryReport(t *testing.T, l *deliveryLedger, project, item, phase string) (ledger.Report, string) {
	t.Helper()
	raw := deliveryGitShow(t, l.root, "HEAD", deliveryReportPath(project, item, phase))
	report, body, err := ledger.ParseReport(phase, []byte(raw))
	if err != nil {
		t.Fatalf("parse committed %s report of %s: %v", phase, item, err)
	}
	return report, body
}

func deliveryGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("git -C %s %s: %v\n%s", root, strings.Join(args, " "), err, stderr.String())
	}
}

func deliveryGitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git -C %s %s: %v\n%s", root, strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(string(output))
}

// deliveryGitShow returns one path's exact bytes at one revision.
func deliveryGitShow(t *testing.T, root, revision, path string) string {
	t.Helper()
	command := exec.Command("git", "-C", root, "show", revision+":"+path)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git show %s:%s: %v", revision, path, err)
	}
	return string(output)
}

func deliveryWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func deliveryReadFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

// deliveryCommitPaths lists the paths one commit changed.
func deliveryCommitPaths(t *testing.T, root, commit string) []string {
	t.Helper()
	output := deliveryGitOutput(t, root, "show", "--name-only", "--format=", commit)
	var paths []string
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	sort.Strings(paths)
	return paths
}

func deliveryWaitForPaths(t *testing.T, paths ...string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		all := true
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				all = false
				break
			}
		}
		if all {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper processes did not reach the barrier: %v", paths)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestDeliverySelectionIsProjectScopedAndLaneOrdered covers B1: eligible
// Rework precedes Ready for Implementation, work is selected only from the
// requested Project, slices blocked by an unmerged dependency are skipped,
// a Merged dependency stays eligible, and independent slices hold Claims
// simultaneously.
func TestDeliverySelectionIsProjectScopedAndLaneOrdered(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addProject("gadgets", "acme/gadgets")
	l.addSlice("widgets", "delivery-deps", "base", ledger.Merged, nil, "2023-01-01T00:00:00Z")
	l.addSlice("widgets", "delivery-deps", "active", ledger.AwaitingReview, nil, "2023-02-01T00:00:00Z")
	l.addSlice("widgets", "delivery-lanes", "waiting", ledger.ReadyForImplementation,
		[]string{"proposals/delivery-deps/active"}, "2024-01-01T00:00:00Z")
	l.addSlice("widgets", "delivery-lanes", "fixup", ledger.Rework, nil, "2024-03-01T00:00:00Z")
	l.addFile(deliveryReportPath("widgets", "delivery-lanes/fixup", ledger.WatchdogPhase),
		deliveryWatchdogReport("rework", 1, "widgets", "delivery-lanes/fixup"))
	l.addSlice("widgets", "delivery-lanes", "feature", ledger.ReadyForImplementation,
		[]string{"proposals/delivery-deps/base"}, "2024-02-01T00:00:00Z")
	l.addSlice("gadgets", "gadget-lanes", "gizmo", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept selection records")

	store := l.store()

	first := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	if first.Item != "delivery-lanes/fixup" {
		t.Fatalf("first selection = %q, want the Rework slice first", first.Item)
	}
	second := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	if second.Item != "delivery-lanes/feature" {
		t.Fatalf("second selection = %q, want the eligible Merged-dependent slice", second.Item)
	}
	// The remaining widgets slice is blocked by an unmerged dependency, and
	// gadgets work must not be selected from the widgets repository.
	if third, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.ImplementPhase); err != nil || third != nil {
		t.Fatalf("blocked/unmerged selection = %#v, %v; want no work", third, err)
	}
	gadget := deliveryStart(t, store, deliveryGadgets(), ledger.ImplementPhase)
	if gadget.Item != "gadget-lanes/gizmo" || gadget.Project != "gadgets" {
		t.Fatalf("gadgets selection = %q in project %q", gadget.Item, gadget.Project)
	}
	for _, item := range []string{"delivery-lanes/fixup", "delivery-lanes/feature"} {
		if l.committedState("widgets", "delivery-lanes", strings.TrimPrefix(item, "delivery-lanes/")).Claim == nil {
			t.Fatalf("independent slice %s lost its simultaneous Claim", item)
		}
	}
}

type deliveryHelperOutcome struct {
	Item        string `json:"item,omitempty"`
	ClaimCommit string `json:"claim_commit,omitempty"`
	Error       string `json:"error,omitempty"`
}

// TestDeliveryConcurrentCallersAcquireAtMostOneClaim covers B1 and A2 with
// actual concurrent subprocess callers released from a file barrier.
func TestDeliveryConcurrentCallersAcquireAtMostOneClaim(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "delivery-race", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept delivery-race")
	commitsBefore := l.commitCount()

	directory := t.TempDir()
	barrier := filepath.Join(directory, "barrier")
	const callers = 3
	type child struct {
		command *exec.Cmd
		result  string
		log     *bytes.Buffer
	}
	children := make([]*child, 0, callers)
	ready := make([]string, 0, callers)
	for index := 0; index < callers; index++ {
		result := filepath.Join(directory, fmt.Sprintf("result-%d.json", index))
		readyPath := filepath.Join(directory, fmt.Sprintf("ready-%d", index))
		ready = append(ready, readyPath)
		log := &bytes.Buffer{}
		command := exec.Command(os.Args[0], "-test.run=^TestDeliveryConcurrentClaimHelper$")
		command.Env = append(os.Environ(),
			"SKL_DELIVERY_HELPER=1",
			"SKL_DELIVERY_HELPER_ROOT="+l.root,
			"SKL_DELIVERY_HELPER_BARRIER="+barrier,
			"SKL_DELIVERY_HELPER_READY="+readyPath,
			"SKL_DELIVERY_HELPER_RESULT="+result,
		)
		command.Stdout = log
		command.Stderr = log
		if err := command.Start(); err != nil {
			t.Fatalf("start concurrent caller %d: %v", index, err)
		}
		children = append(children, &child{command: command, result: result, log: log})
	}
	deliveryWaitForPaths(t, ready...)
	if err := os.WriteFile(barrier, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	claimed, winnerCommit := 0, ""
	for index, child := range children {
		if err := child.command.Wait(); err != nil {
			t.Fatalf("concurrent caller %d failed: %v\n%s", index, err, child.log.String())
		}
		var outcome deliveryHelperOutcome
		if err := json.Unmarshal([]byte(deliveryReadFile(t, child.result)), &outcome); err != nil {
			t.Fatalf("decode concurrent caller %d outcome: %v", index, err)
		}
		if outcome.Error != "" {
			t.Errorf("caller %d reported %q instead of a truthful non-acquisition outcome", index, outcome.Error)
			continue
		}
		if outcome.Item == "" {
			continue
		}
		claimed++
		winnerCommit = outcome.ClaimCommit
		if outcome.Item != "delivery-race/foundation" {
			t.Errorf("caller %d acquired %q", index, outcome.Item)
		}
	}
	if claimed != 1 {
		t.Fatalf("concurrent callers acquired %d Claims, want exactly 1", claimed)
	}
	if l.head() != winnerCommit {
		t.Fatalf("acquisition commit = %s, ledger head = %s", winnerCommit, l.head())
	}
	if got := l.commitCount(); got != commitsBefore+1 {
		t.Fatalf("ledger gained %d commits for one Claim, want 1", got-commitsBefore)
	}
	state := l.committedState("widgets", "delivery-race", "foundation")
	if state.Claim == nil || state.Claim.Phase != ledger.ImplementPhase {
		t.Fatalf("committed state has no implementation Claim: %#v", state.Claim)
	}
	resumed, err := ledger.ResumeDelivery(l.store(), deliveryWidgets(), "delivery-race/foundation", ledger.ImplementPhase, winnerCommit)
	if err != nil || resumed == nil {
		t.Fatalf("winner could not resume its own Claim: %v", err)
	}
}

// TestDeliveryConcurrentClaimHelper is the re-executed helper body.
func TestDeliveryConcurrentClaimHelper(t *testing.T) {
	if os.Getenv("SKL_DELIVERY_HELPER") != "1" {
		return
	}
	store, err := ledger.Open(os.Getenv("SKL_DELIVERY_HELPER_ROOT"))
	if err != nil {
		deliveryWriteHelperOutcome(t, deliveryHelperOutcome{Error: err.Error()})
		return
	}
	if err := os.WriteFile(os.Getenv("SKL_DELIVERY_HELPER_READY"), []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	barrier := os.Getenv("SKL_DELIVERY_HELPER_BARRIER")
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(barrier); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("barrier was never released")
		}
		time.Sleep(2 * time.Millisecond)
	}
	outcome := deliveryHelperOutcome{}
	execution, startErr := ledger.StartDelivery(store, deliveryWidgets(), ledger.ImplementPhase)
	switch {
	case startErr != nil:
		outcome.Error = startErr.Error()
	case execution != nil:
		outcome.Item = execution.Item
		outcome.ClaimCommit = execution.Claim.Commit
	}
	deliveryWriteHelperOutcome(t, outcome)
}

func deliveryWriteHelperOutcome(t *testing.T, outcome deliveryHelperOutcome) {
	t.Helper()
	encoded, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("SKL_DELIVERY_HELPER_RESULT"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDeliveryResumeAndReleaseRequireExactClaim covers B5 and B9: resume and
// release reject a wrong or old commit, unrelated committed Project work does
// not stale the selected Claim, and an explicit release ends the reservation.
func TestDeliveryResumeAndReleaseRequireExactClaim(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "delivery-hold", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept delivery-hold")

	store := l.store()
	item := "delivery-hold/foundation"
	execution := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	claim := execution.Claim.Commit
	parent := deliveryGitOutput(t, l.root, "rev-parse", claim+"^")

	if _, err := ledger.ResumeDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, parent); err == nil {
		t.Fatal("an older commit was accepted as the current Claim")
	}
	if err := ledger.ReleaseDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, parent); err == nil {
		t.Fatal("an older commit released the current Claim")
	}
	if state := l.committedState("widgets", "delivery-hold", "foundation"); state.Claim == nil || state.Claim.Phase != ledger.ImplementPhase {
		t.Fatalf("rejected resume/release changed the Claim: %#v", state.Claim)
	}

	// Unrelated committed ledger work in this and another Project must not
	// invalidate the selected execution's fixed inputs.
	l.addFile("unrelated-note.txt", "unrelated ledger note\n")
	l.addProject("gadgets", "acme/gadgets")
	l.addSlice("gadgets", "gadget-work", "gizmo", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("unrelated project work")
	headAfterUnrelated := l.head()
	resumed, err := ledger.ResumeDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim)
	if err != nil || resumed == nil {
		t.Fatalf("unrelated committed work staled the current Claim: %v", err)
	}
	if resumed.Item != item || resumed.Claim.Commit != claim {
		t.Fatalf("resumed execution = %q at %s, want %s at %s", resumed.Item, resumed.Claim.Commit, item, claim)
	}
	if l.head() != headAfterUnrelated {
		t.Fatal("resume mutated the ledger")
	}

	if err := ledger.ReleaseDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim); err != nil {
		t.Fatalf("explicit release of the current Claim: %v", err)
	}
	if state := l.committedState("widgets", "delivery-hold", "foundation"); state.Claim != nil {
		t.Fatalf("release left a Claim: %#v", state.Claim)
	}
	if _, err := ledger.ResumeDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim); err == nil {
		t.Fatal("a released Claim still resumed")
	}
	released := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	if released.Claim.Commit == claim {
		t.Fatal("a released slice reused the old Claim commit")
	}
}

// TestDeliveryHandoffIsAtomicAndReplaySafe covers B5 and A2: one commit carries
// the report and resulting state, Contract bytes stay frozen, the body is data,
// an exact retry is recognized without a new commit or round, and a later
// execution cannot be overwritten or released by the old one.
func TestDeliveryHandoffIsAtomicAndReplaySafe(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "delivery-atomic", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept delivery-atomic")

	store := l.store()
	item := "delivery-atomic/foundation"
	execution := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	claim := execution.Claim.Commit
	reportPath := deliveryReportPath("widgets", item, ledger.ImplementPhase)
	statePath := deliveryStatePath("widgets", item)
	intentBefore := deliveryGitShow(t, l.root, claim, deliveryContractPath("widgets", item, "intent.md"))
	behaviorBefore := deliveryGitShow(t, l.root, claim, deliveryContractPath("widgets", item, "behavior.md"))

	source := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget}
	// The body resembles another verdict and embedded metadata; it is opaque.
	body := "---\nschema: 99\noutcome: pass\nround: 9\n---\nLooks like a verdict: rework.\n"
	result := deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, claim, source, "awaiting_review", body)

	if result.Status != ledger.AwaitingReview || result.AlreadyCompleted {
		t.Fatalf("handoff result = %#v", result)
	}
	if result.Report.Commit != l.head() || result.Report.Path != reportPath {
		t.Fatalf("report reference = %#v at head %s", result.Report, l.head())
	}
	if parent := deliveryGitOutput(t, l.root, "rev-parse", result.Report.Commit+"^"); parent != claim {
		t.Fatalf("handoff parent = %s, want Claim %s", parent, claim)
	}
	changed := deliveryCommitPaths(t, l.root, result.Report.Commit)
	wantChanged := []string{reportPath, statePath}
	sort.Strings(wantChanged)
	if strings.Join(changed, ",") != strings.Join(wantChanged, ",") {
		t.Fatalf("handoff commit changed %v, want %v (one atomic change)", changed, wantChanged)
	}
	if deliveryGitOutput(t, l.root, "status", "--porcelain") != "" {
		t.Fatalf("handoff left a dirty ledger: %s", deliveryGitOutput(t, l.root, "status", "--porcelain"))
	}

	report, recordedBody := deliveryReport(t, l, "widgets", item, ledger.ImplementPhase)
	if report.Schema != ledger.ReportSchema || report.Outcome != "awaiting_review" || report.Round != 0 {
		t.Fatalf("committed report metadata = %#v", report)
	}
	if report.Ledger.Claim.Commit != claim || report.Ledger.Claim.Path != statePath {
		t.Fatalf("report claim reference = %#v, want acquisition %s", report.Ledger.Claim, claim)
	}
	if report.Source != source {
		t.Fatalf("report source = %#v, want %#v", report.Source, source)
	}
	if len(report.Ledger.Contract) != 2 ||
		report.Ledger.Contract[0].Path != deliveryContractPath("widgets", item, "behavior.md") ||
		report.Ledger.Contract[1].Path != deliveryContractPath("widgets", item, "intent.md") {
		t.Fatalf("report contract inputs = %#v", report.Ledger.Contract)
	}
	if recordedBody != body {
		t.Fatalf("committed body = %q, want the opaque bytes %q", recordedBody, body)
	}
	if got := deliveryGitShow(t, l.root, "HEAD", deliveryContractPath("widgets", item, "intent.md")); got != intentBefore {
		t.Fatal("handoff changed intent.md bytes")
	}
	if got := deliveryGitShow(t, l.root, "HEAD", deliveryContractPath("widgets", item, "behavior.md")); got != behaviorBefore {
		t.Fatal("handoff changed behavior.md bytes")
	}

	// An exact retry recognizes the completed effect: no commit, no new round.
	commitsAfter := l.commitCount()
	stateAfter := deliveryGitShow(t, l.root, "HEAD", statePath)
	retry, err := ledger.HandoffDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim, source, "awaiting_review", body)
	if err != nil {
		t.Fatalf("exact retry was not recognized: %v", err)
	}
	if !retry.AlreadyCompleted || retry.Status != ledger.AwaitingReview {
		t.Fatalf("retry result = %#v", retry)
	}
	if l.commitCount() != commitsAfter || l.head() != result.Report.Commit {
		t.Fatal("exact retry created a commit")
	}
	if got := deliveryGitShow(t, l.root, "HEAD", statePath); got != stateAfter {
		t.Fatal("exact retry rewrote state")
	}
	if again, _ := deliveryReport(t, l, "widgets", item, ledger.ImplementPhase); again.Round != 0 {
		t.Fatalf("exact retry advanced the review count to %d", again.Round)
	}

	// A later execution owns the Work Item; the old one cannot interfere.
	later := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	headBeforeOld := l.head()
	if _, err := ledger.HandoffDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim, source, "awaiting_review", body); err == nil {
		t.Fatal("an old execution overwrote a later Claim's result")
	}
	if _, err := ledger.ResumeDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim); err == nil {
		t.Fatal("an old execution resumed while a later Claim is active")
	}
	if err := ledger.ReleaseDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim); err == nil {
		t.Fatal("an old execution released a later Claim")
	}
	if l.head() != headBeforeOld {
		t.Fatal("an old execution mutated the ledger")
	}
	state := l.committedState("widgets", "delivery-atomic", "foundation")
	if state.Claim == nil || state.Claim.Phase != ledger.WatchdogPhase {
		t.Fatalf("later watchdog Claim was not preserved: %#v", state.Claim)
	}
	if _, err := ledger.ResumeDelivery(store, deliveryWidgets(), item, ledger.WatchdogPhase, later.Claim.Commit); err != nil {
		t.Fatalf("the later Claim could no longer be resumed: %v", err)
	}

	// Once the later execution records a changed result and releases its
	// Claim, the old execution still cannot restore its superseded lifecycle.
	review := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget, Reviewed: deliveryHead}
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.WatchdogPhase, later.Claim.Commit, review, "rework", "W1 open.\n")
	superseded := l.committedState("widgets", "delivery-atomic", "foundation")
	if superseded.Claim != nil || superseded.State != ledger.Rework {
		t.Fatalf("later execution did not record its own lifecycle: %#v", superseded)
	}
	headAfterChanged := l.head()
	supersededState := deliveryGitShow(t, l.root, headAfterChanged, statePath)
	if _, err := ledger.HandoffDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim, source, "awaiting_review", body); err == nil {
		t.Fatal("an old execution overwrote a changed result")
	}
	if err := ledger.ReleaseDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim); err == nil {
		t.Fatal("an old execution released a changed lifecycle")
	}
	if l.head() != headAfterChanged || deliveryGitShow(t, l.root, "HEAD", statePath) != supersededState {
		t.Fatal("an old execution mutated a changed result")
	}
}

// TestDeliveryCountsCompletedReviewsAndHonorsHumanDirection covers B7/A6: two
// failing reworks reach Needs Human, requeueing needs recorded direction, the
// count is not reset, and an unchanged-code round 3 may pass.
func TestDeliveryCountsCompletedReviewsAndHonorsHumanDirection(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "delivery-flow", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept delivery-flow")

	store := l.store()
	item := "delivery-flow/foundation"
	source := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget}
	review := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget, Reviewed: deliveryHead}

	implementOne := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, implementOne.Claim.Commit, source, "awaiting_review", "implementation one\n")

	watchdogOne := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	firstRework := deliveryHandoff(t, store, deliveryWidgets(), item, ledger.WatchdogPhase, watchdogOne.Claim.Commit, review, "rework", "W1 open.\n")
	if firstRework.Status != ledger.Rework {
		t.Fatalf("first rework routed to %q, want Rework", firstRework.Status)
	}
	if report, _ := deliveryReport(t, l, "widgets", item, ledger.WatchdogPhase); report.Round != 1 || report.Outcome != "rework" {
		t.Fatalf("first review metadata = %#v", report)
	}

	implementTwo := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, implementTwo.Claim.Commit, source, "awaiting_review", "implementation two\n")
	watchdogTwo := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	secondRework := deliveryHandoff(t, store, deliveryWidgets(), item, ledger.WatchdogPhase, watchdogTwo.Claim.Commit, review, "rework", "W1 still open.\n")
	if secondRework.Status != ledger.NeedsHuman {
		t.Fatalf("second rework routed to %q, want Needs Human", secondRework.Status)
	}
	if report, _ := deliveryReport(t, l, "widgets", item, ledger.WatchdogPhase); report.Round != 2 || report.Outcome != "rework" {
		t.Fatalf("second review metadata = %#v", report)
	}

	// Requeueing for rework without recorded direction must refuse rather
	// than grant more work.
	state := l.committedState("widgets", "delivery-flow", "foundation")
	state.State = ledger.Rework
	state.Decision = nil
	l.commitState("widgets", "delivery-flow", "foundation", state)
	headBeforeRefusal := l.head()
	if _, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.ImplementPhase); err == nil {
		t.Fatal("continued rework was granted without recorded human direction")
	}
	if l.head() != headBeforeRefusal {
		t.Fatal("a refused selection mutated the ledger")
	}
	if state := l.committedState("widgets", "delivery-flow", "foundation"); state.Claim != nil {
		t.Fatalf("refused selection left a Claim: %#v", state.Claim)
	}

	deliveryRequeue(t, l, "widgets", item, ledger.Rework, "continue rework within the Contract\n")
	implementThree := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, implementThree.Claim.Commit, source, "awaiting_review", "implementation three\n")
	if state := l.committedState("widgets", "delivery-flow", "foundation"); state.Decision == nil {
		t.Fatal("implementation lost the direction its independent reviewer must consume")
	}
	// The recorded continuation reaches independent review without a second
	// human joining the two operations. It does not reset completed rounds.
	watchdogThree := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	if watchdogThree.State.Claim.Inputs.Decision == nil || watchdogThree.Watchdog.Round != 2 {
		t.Fatalf("continuation inputs/count lost: %#v", watchdogThree)
	}
	passed := deliveryHandoff(t, store, deliveryWidgets(), item, ledger.WatchdogPhase, watchdogThree.Claim.Commit, review, "pass", "W1 resolved.\n")
	if passed.Status != ledger.ReadyForMerge {
		t.Fatalf("round 3 pass routed to %q", passed.Status)
	}
	if passed.State.Decision != nil {
		t.Fatal("completed review retained authorization for a later cycle")
	}
	report, _ := deliveryReport(t, l, "widgets", item, ledger.WatchdogPhase)
	if report.Round != 3 || report.Outcome != "pass" || report.Source.Reviewed != deliveryHead {
		t.Fatalf("round 3 review metadata = %#v", report)
	}
}

// TestDeliveryExplicitNeedsHumanCounts covers B7/A6: an explicit Needs Human
// review is a completed round.
func TestDeliveryExplicitNeedsHumanCounts(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "delivery-pause", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept delivery-pause")

	store := l.store()
	item := "delivery-pause/foundation"
	source := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget}
	review := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget, Reviewed: deliveryHead}

	implement := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, implement.Claim.Commit, source, "awaiting_review", "implementation\n")
	watchdog := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase)
	paused := deliveryHandoff(t, store, deliveryWidgets(), item, ledger.WatchdogPhase, watchdog.Claim.Commit, review, "needs_human", "blocked on a human decision.\n")
	if paused.Status != ledger.NeedsHuman {
		t.Fatalf("explicit needs_human routed to %q", paused.Status)
	}
	report, _ := deliveryReport(t, l, "widgets", item, ledger.WatchdogPhase)
	if report.Round != 1 || report.Outcome != "needs_human" {
		t.Fatalf("needs_human review metadata = %#v", report)
	}
}

// deliveryRequeue records an explicit human decision and makes the slice
// eligible for the supplied lifecycle again, standing in for the decision
// surface owned outside this slice.
func deliveryRequeue(t *testing.T, l *deliveryLedger, project, item, state string, direction string) {
	t.Helper()
	proposal, slice, ok := strings.Cut(item, "/")
	if !ok {
		t.Fatalf("item %q is not a proposal/slice identity", item)
	}
	decision := l.recordDecision(project, proposal, slice, direction)
	current := l.committedState(project, proposal, slice)
	current.State = state
	current.Claim = nil
	current.Decision = &ledger.Reference{Commit: decision, Path: deliveryDecisionPath(project, item)}
	l.commitState(project, proposal, slice, current)
}

// TestDeliveryRefusesIncompatibleReportSchema covers B4: an unknown required
// schema refuses review without acquiring a Claim or losing state, and a
// restored compatible report lets the same work proceed.
func TestDeliveryRefusesIncompatibleReportSchema(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "delivery-schema", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept delivery-schema")

	store := l.store()
	item := "delivery-schema/foundation"
	source := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget}
	implement := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, implement.Claim.Commit, source, "awaiting_review", "implementation\n")
	reportPath := deliveryReportPath("widgets", item, ledger.ImplementPhase)
	compatible := deliveryGitShow(t, l.root, "HEAD", reportPath)

	l.addFile(reportPath, "---\nschema: 2\noutcome: awaiting_review\n---\nnewer format\n")
	l.commitAll("write incompatible report")
	headBefore := l.head()
	if _, err := ledger.StartDelivery(store, deliveryWidgets(), ledger.WatchdogPhase); err == nil {
		t.Fatal("review was granted over an incompatible report schema")
	} else if !strings.Contains(err.Error(), "schema") {
		t.Fatalf("incompatible schema refusal = %v", err)
	}
	if l.head() != headBefore {
		t.Fatal("refused review mutated the ledger")
	}
	if got := deliveryGitShow(t, l.root, "HEAD", reportPath); got != deliveryGitShow(t, l.root, headBefore, reportPath) {
		t.Fatal("refusal rewrote the required report")
	}
	state := l.committedState("widgets", "delivery-schema", "foundation")
	if state.Claim != nil || state.State != ledger.AwaitingReview {
		t.Fatalf("refusal lost or changed state: %#v", state)
	}

	l.addFile(reportPath, compatible)
	l.commitAll("restore compatible report")
	if execution := deliveryStart(t, store, deliveryWidgets(), ledger.WatchdogPhase); execution.Item != item {
		t.Fatalf("restored report selected %q", execution.Item)
	}
}

// TestDeliveryRefusesDirtyRecord covers B9/A2: an interrupted or ambiguous
// record refuses without discarding the ambiguous files.
func TestDeliveryRefusesDirtyRecord(t *testing.T) {
	t.Run("handoff preserves an interrupted report", func(t *testing.T) {
		l := newDeliveryLedger(t)
		l.addProject("widgets", "acme/widgets")
		l.addSlice("widgets", "delivery-dirty", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
		l.commitAll("accept delivery-dirty")

		store := l.store()
		item := "delivery-dirty/foundation"
		execution := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
		claim := execution.Claim.Commit
		leftoverPath := filepath.Join(l.root, filepath.FromSlash(deliveryReportPath("widgets", item, ledger.ImplementPhase)))
		deliveryWrite(t, leftoverPath, "half written result")
		if _, err := ledger.HandoffDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, claim,
			ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget}, "awaiting_review", "body\n"); err == nil {
			t.Fatal("handoff proceeded over an interrupted record")
		}
		if l.head() != claim {
			t.Fatal("refused handoff created a commit")
		}
		if got := deliveryReadFile(t, leftoverPath); got != "half written result" {
			t.Fatalf("ambiguous file was changed to %q", got)
		}
		if state := l.committedState("widgets", "delivery-dirty", "foundation"); state.Claim == nil {
			t.Fatal("refused handoff released the Claim")
		}
	})

	t.Run("selection preserves a dirty state record", func(t *testing.T) {
		l := newDeliveryLedger(t)
		l.addProject("widgets", "acme/widgets")
		l.addSlice("widgets", "delivery-dirty-state", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
		l.commitAll("accept delivery-dirty-state")

		statePath := filepath.Join(l.root, filepath.FromSlash(deliveryStatePath("widgets", "delivery-dirty-state/foundation")))
		deliveryWrite(t, statePath, "{")
		if _, err := ledger.StartDelivery(l.store(), deliveryWidgets(), ledger.ImplementPhase); err == nil {
			t.Fatal("selection proceeded over a dirty state record")
		}
		if got := deliveryReadFile(t, statePath); got != "{" {
			t.Fatalf("ambiguous state record was changed to %q", got)
		}
	})
}

// TestDeliveryDivergentUpstreamBlocksWork covers B9/A2: known competing ledger
// history refuses new work.
func TestDeliveryDivergentUpstreamBlocksWork(t *testing.T) {
	upstream := filepath.Join(t.TempDir(), "upstream.git")
	deliveryGit(t, t.TempDir(), "init", "-q", "--bare", "-b", "main", upstream)
	root := filepath.Join(t.TempDir(), "ledger")
	deliveryGit(t, t.TempDir(), "clone", "-q", upstream, root)
	deliveryGit(t, root, "config", "user.name", "Ledger")
	deliveryGit(t, root, "config", "user.email", "ledger@example.com")
	deliveryWrite(t, filepath.Join(root, "README.md"), "workflow ledger\n")
	deliveryGit(t, root, "add", "README.md")
	deliveryGit(t, root, "commit", "-q", "-m", "seed")
	deliveryGit(t, root, "push", "-q", "-u", "origin", "main")

	other := filepath.Join(t.TempDir(), "other")
	deliveryGit(t, t.TempDir(), "clone", "-q", upstream, other)
	deliveryGit(t, other, "config", "user.name", "Other")
	deliveryGit(t, other, "config", "user.email", "other@example.com")
	deliveryWrite(t, filepath.Join(other, "competing.txt"), "competing\n")
	deliveryGit(t, other, "add", "competing.txt")
	deliveryGit(t, other, "commit", "-q", "-m", "competing")
	deliveryGit(t, other, "push", "-q", "origin", "main")

	l := &deliveryLedger{t: t, root: root}
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "delivery-diverged", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept delivery-diverged")
	deliveryGit(t, root, "fetch", "-q", "origin")
	headBefore := l.head()

	execution, err := ledger.StartDelivery(l.store(), deliveryWidgets(), ledger.ImplementPhase)
	if err == nil {
		t.Fatalf("divergent upstream granted work: %#v", execution)
	}
	if !strings.Contains(err.Error(), "reconcil") {
		t.Fatalf("divergence refusal = %v", err)
	}
	if l.head() != headBefore {
		t.Fatal("refused selection mutated the ledger")
	}
	if state := l.committedState("widgets", "delivery-diverged", "foundation"); state.Claim != nil {
		t.Fatalf("divergent selection reserved work: %#v", state.Claim)
	}
}

// TestDeliveryUnavailableUpstreamStillDeliversLocally covers B9: an outage
// with locally available records does not prevent local selection or handoff.
func TestDeliveryUnavailableUpstreamStillDeliversLocally(t *testing.T) {
	l := newDeliveryLedger(t)
	l.addProject("widgets", "acme/widgets")
	l.addSlice("widgets", "delivery-outage", "foundation", ledger.ReadyForImplementation, nil, deliveryInitial)
	l.commitAll("accept delivery-outage")
	deliveryGit(t, l.root, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing-upstream.git"))

	store := l.store()
	item := "delivery-outage/foundation"
	execution := deliveryStart(t, store, deliveryWidgets(), ledger.ImplementPhase)
	resumed, err := ledger.ResumeDelivery(store, deliveryWidgets(), item, ledger.ImplementPhase, execution.Claim.Commit)
	if err != nil || resumed == nil {
		t.Fatalf("resume with an unavailable upstream: %v", err)
	}
	source := ledger.SourceRevisions{Head: deliveryHead, Target: deliveryTarget}
	result := deliveryHandoff(t, store, deliveryWidgets(), item, ledger.ImplementPhase, execution.Claim.Commit, source, "awaiting_review", "local result\n")
	if result.Status != ledger.AwaitingReview {
		t.Fatalf("local handoff during an outage = %#v", result)
	}
	if state := l.committedState("widgets", "delivery-outage", "foundation"); state.Claim != nil {
		t.Fatal("local handoff did not release the Claim")
	}
}
