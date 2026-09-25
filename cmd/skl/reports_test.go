package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

func TestLedgerShowReportsTraversesExactLocalHistoryReadOnly(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	acceptanceHead := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")

	var factoryCalls int
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		factoryCalls++
		return nil, errors.New("forge unavailable in report inspection test")
	}, bytes.NewReader(nil), &output, &output)
	cli := ledgerCLI{app: app, out: &output}

	// A Work Item is already enough to discover that neither report exists.
	initialFactoryCalls := factoryCalls
	initial := cli.ledgerJSON(t, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--format", "json")
	assertReportPhases(t, initial.Readback, nil, nil)
	markdown, err := reportCLIText(t, cli, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem)
	if err != nil {
		t.Fatalf("initial Markdown readback: %v", err)
	}
	for _, phase := range []string{"implement", "watchdog"} {
		if !strings.Contains(markdown, "- "+phase+": absent") {
			t.Fatalf("Markdown did not mark %s absent:\n%s", phase, markdown)
		}
	}
	absent := cli.ledgerJSON(t, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--phase", "implement", "--format", "json")
	if absent.Status != "fix_required" || absent.Document != nil || !strings.Contains(absent.Repair, "skl ledger show --commit") || !strings.Contains(absent.Repair, "--path") {
		t.Fatalf("absent current report lacked an actionable historical route: %s", mustJSON(t, absent))
	}
	if factoryCalls != initialFactoryCalls {
		t.Fatalf("initial read called backend factory: %d -> %d", initialFactoryCalls, factoryCalls)
	}

	// Build two actual local phase rounds: the earlier Watchdog report authors
	// W3, and the later pass records its disposition without replacing history.
	if err := os.RemoveAll(fixture.upstream); err != nil {
		t.Fatal(err)
	}
	implementOneBody := "# Implementation round one\n\nSource changes for review.\n"
	implementOne, sourceHeadOne, worktree, _ := submitReportImplementation(t, cli, source, target, "one", implementOneBody)
	watchdogOneBody := "# Watchdog round one\n\nW3 [BLOCK]: preserve the original reasoning."
	watchdogOne, _ := submitReportWatchdog(t, cli, source, "rework", watchdogOneBody)
	if watchdogOne.Status != ledger.Rework {
		t.Fatalf("first Watchdog status = %s, want rework", watchdogOne.Status)
	}
	runGit(t, source, "worktree", "remove", worktree)

	implementTwoBody := "# Implementation round two\n\nW3 addressed in source."
	implementTwo, sourceHeadTwo, _, consumedWatchdog := submitReportImplementation(t, cli, source, target, "two", implementTwoBody)
	watchdogTwoBody := "# Watchdog round two\n\nW3 [resolved]: the change addresses the recorded issue.\n"
	watchdogTwo, consumedImplement := submitReportWatchdog(t, cli, source, "pass", watchdogTwoBody)
	if watchdogTwo.Status != ledger.ReadyForMerge {
		t.Fatalf("passing Watchdog status = %s, want Ready for Merge", watchdogTwo.Status)
	}
	if sourceHeadOne == implementOne.Report.Commit || sourceHeadTwo == implementTwo.Report.Commit {
		t.Fatal("source revision was confused with a ledger report commit")
	}

	// Add unrelated committed work and acquire a later Claim on another item.
	later := singleSlice("later-claim")
	later.slices[0].branch = "later-branch"
	if got := newLedgerApp(t, forge).accept(t, source, writeProposal(t, "", later)); got.Status != "accepted" {
		t.Fatalf("accept later Claim item: %s", mustJSON(t, got))
	}
	claim, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || claim.Execution == nil || claim.Execution.Item != "later-claim/foundation" {
		t.Fatalf("later Claim = %#v, err=%v", claim, err)
	}
	unrelated := singleSlice("unrelated-ledger-commit")
	unrelated.slices[0].branch = "unrelated-branch"
	if got := newLedgerApp(t, forge).accept(t, source, writeProposal(t, "", unrelated)); got.Status != "accepted" {
		t.Fatalf("record unrelated committed item: %s", mustJSON(t, got))
	}

	committedHead := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	committedReadyState := deliveryGitShowForReportTest(t, fixture.clone, "HEAD", deliveryItemStatePath())
	committedClaimState := deliveryGitShowForReportTest(t, fixture.clone, "HEAD", "projects/widgets/proposals/later-claim/foundation/state.json")
	var laterState ledger.SliceState
	if err := json.Unmarshal([]byte(committedClaimState), &laterState); err != nil {
		t.Fatal(err)
	}
	if laterState.Claim == nil || laterState.Claim.Phase != ledger.ImplementPhase || laterState.State != ledger.ReadyForImplementation {
		t.Fatalf("later committed Claim state = %#v", laterState)
	}
	factoryCallsBeforeInspection := factoryCalls

	current := cli.ledgerJSON(t, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--format", "json")
	if current.Status != "shown" || current.Readback == nil || current.Readback.State != ledger.ReadyForMerge {
		t.Fatalf("Ready-for-Merge Work Item readback = %s", mustJSON(t, current))
	}
	implementRef := assertReportPhases(t, current.Readback, &ledger.Reference{Commit: committedHead, Path: implementTwo.Report.Path}, &ledger.Reference{Commit: committedHead, Path: watchdogTwo.Report.Path})
	if implementRef == nil {
		t.Fatal("current implementation report reference missing")
	}
	var readyState ledger.SliceState
	if err := json.Unmarshal([]byte(committedReadyState), &readyState); err != nil {
		t.Fatal(err)
	}
	if readyState.State != ledger.ReadyForMerge || readyState.Claim != nil {
		t.Fatalf("committed selected Work Item state = %#v", readyState)
	}

	currentImplement := cli.ledgerJSON(t, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--phase", "implement", "--format", "json")
	if currentImplement.Status != "shown" || currentImplement.Document == nil || currentImplement.Document.Commit != committedHead {
		t.Fatalf("current implementation report = %s", mustJSON(t, currentImplement))
	}
	if currentImplement.Document.Contents != deliveryGitShowForReportTest(t, fixture.clone, committedHead, implementRef.Path) {
		t.Fatal("current implementation report did not return the committed bytes at its exact reference")
	}
	parsedImplement, body, err := ledger.ParseReport(ledger.ImplementPhase, []byte(currentImplement.Document.Contents))
	if err != nil || body != implementTwoBody {
		t.Fatalf("current implementation body = %q, parse err=%v", body, err)
	}
	if parsedImplement.Source.Head != sourceHeadTwo || parsedImplement.Source.Head == currentImplement.Document.Commit {
		t.Fatalf("source and ledger identities were conflated: source=%s ledger=%s", parsedImplement.Source.Head, currentImplement.Document.Commit)
	}
	if consumedWatchdog == nil || parsedImplement.Ledger.Watchdog == nil || *parsedImplement.Ledger.Watchdog != *consumedWatchdog {
		t.Fatalf("rework implementation lost its exact consumed Watchdog reference: got %#v want %#v", parsedImplement.Ledger.Watchdog, consumedWatchdog)
	}

	currentWatchdog := cli.ledgerJSON(t, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--phase", "watchdog", "--format", "json")
	if currentWatchdog.Status != "shown" || currentWatchdog.Document == nil || currentWatchdog.Document.Commit != committedHead {
		t.Fatalf("current Watchdog report = %s", mustJSON(t, currentWatchdog))
	}
	parsedWatchdog, body, err := ledger.ParseReport(ledger.WatchdogPhase, []byte(currentWatchdog.Document.Contents))
	if err != nil || body != watchdogTwoBody || parsedWatchdog.Round != 2 {
		t.Fatalf("current Watchdog body/round = %q/%d, parse err=%v", body, parsedWatchdog.Round, err)
	}
	if consumedImplement == nil || parsedWatchdog.Ledger.Implement == nil || *parsedWatchdog.Ledger.Implement != *consumedImplement {
		t.Fatalf("passing review lost the exact consumed implementation reference: got %#v want %#v", parsedWatchdog.Ledger.Implement, consumedImplement)
	}

	// Recorded input references are the historical route; each exact lookup
	// returns its own original document, not the latest contents at that path.
	oldWatchdog := cli.ledgerJSON(t, "skl", "ledger", "show", "--commit", watchdogOne.Report.Commit, "--path", watchdogOne.Report.Path, "--format", "json")
	if oldWatchdog.Status != "shown" || oldWatchdog.Document == nil || oldWatchdog.Document.Commit != watchdogOne.Report.Commit || oldWatchdog.Document.Path != watchdogOne.Report.Path {
		t.Fatalf("historical Watchdog lookup changed identity: %s", mustJSON(t, oldWatchdog))
	}
	oldParsed, oldBody, err := ledger.ParseReport(ledger.WatchdogPhase, []byte(oldWatchdog.Document.Contents))
	if err != nil || oldParsed.Round != 1 || oldBody != watchdogOneBody || !strings.Contains(oldBody, "W3 [BLOCK]") {
		t.Fatalf("historical W3 evidence changed: round=%d body=%q err=%v", oldParsed.Round, oldBody, err)
	}
	consumedWatchdogDocument := cli.ledgerJSON(t, "skl", "ledger", "show", "--commit", parsedImplement.Ledger.Watchdog.Commit, "--path", parsedImplement.Ledger.Watchdog.Path, "--format", "json")
	if consumedWatchdogDocument.Document == nil || consumedWatchdogDocument.Document.Contents != oldWatchdog.Document.Contents {
		t.Fatal("report's exact consumed-input reference did not retrieve the earlier Watchdog bytes")
	}
	consumedImplementDocument := cli.ledgerJSON(t, "skl", "ledger", "show", "--commit", parsedWatchdog.Ledger.Implement.Commit, "--path", parsedWatchdog.Ledger.Implement.Path, "--format", "json")
	if consumedImplementDocument.Document == nil || consumedImplementDocument.Document.Contents != deliveryGitShowForReportTest(t, fixture.clone, parsedWatchdog.Ledger.Implement.Commit, parsedWatchdog.Ledger.Implement.Path) {
		t.Fatal("report's exact consumed implementation reference did not retrieve its recorded bytes")
	}

	// The exact Markdown transport leaves authored trailing bytes untouched.
	markdownCurrent, err := reportCLIText(t, cli, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--phase", "watchdog")
	currentPrefix := "Status: shown\nDocument: " + currentWatchdog.Document.Path + " at " + committedHead + "\n\n"
	if err != nil || markdownCurrent != currentPrefix+currentWatchdog.Document.Contents {
		t.Fatalf("Markdown current document changed bytes: err=%v output=%q", err, markdownCurrent)
	}
	markdownOld, err := reportCLIText(t, cli, "skl", "ledger", "show", "--commit", watchdogOne.Report.Commit, "--path", watchdogOne.Report.Path)
	oldPrefix := "Status: shown\nDocument: " + watchdogOne.Report.Path + " at " + watchdogOne.Report.Commit + "\n\n"
	if err != nil || markdownOld != oldPrefix+oldWatchdog.Document.Contents {
		t.Fatalf("Markdown historical document normalized its no-newline body: err=%v output=%q", err, markdownOld)
	}
	markdownDiscovery, err := reportCLIText(t, cli, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem)
	if err != nil {
		t.Fatalf("Markdown report availability: %v", err)
	}
	for _, expected := range []string{
		"implement: available at " + committedHead + ":" + implementRef.Path,
		"watchdog: available at " + committedHead + ":" + watchdogOne.Report.Path,
		"skl ledger show --item " + deliveryTestItem + " --phase implement",
		"skl ledger show --commit " + committedHead + " --path " + watchdogOne.Report.Path,
		"Source references identify source revisions, not ledger documents.",
	} {
		if !strings.Contains(markdownDiscovery, expected) {
			t.Errorf("Markdown discovery missing %q:\n%s", expected, markdownDiscovery)
		}
	}

	// A later Claim can be read too, while both missing reports remain honest.
	claimedReadback := cli.ledgerJSON(t, "skl", "ledger", "show", "--repo", source, "--item", "later-claim/foundation", "--format", "json")
	if claimedReadback.Status != "shown" || claimedReadback.Readback == nil || claimedReadback.Readback.Item != "later-claim/foundation" {
		t.Fatalf("readback required a Claim or lifecycle transition: %s", mustJSON(t, claimedReadback))
	}
	assertReportPhases(t, claimedReadback.Readback, nil, nil)
	if got := deliveryGitShowForReportTest(t, fixture.clone, "HEAD", "projects/widgets/proposals/later-claim/foundation/state.json"); got != committedClaimState {
		t.Fatal("report inspection changed the later Claim record")
	}

	// A historical miss is an honest refusal even when a newer version exists.
	missingCommit := strings.Repeat("0", 40)
	missing := cli.ledgerJSON(t, "skl", "ledger", "show", "--commit", missingCommit, "--path", watchdogOne.Report.Path, "--format", "json")
	if missing.Status != "fix_required" || missing.Document != nil || !strings.Contains(missing.Reason, "unavailable") {
		t.Fatalf("unavailable historical commit was substituted: %s", mustJSON(t, missing))
	}
	missingPath := cli.ledgerJSON(t, "skl", "ledger", "show", "--commit", acceptanceHead, "--path", watchdogOne.Report.Path, "--format", "json")
	if missingPath.Status != "fix_required" || missingPath.Document != nil || !strings.Contains(missingPath.Reason, watchdogOne.Report.Path) {
		t.Fatalf("unavailable historical path was substituted: %s", mustJSON(t, missingPath))
	}

	// A phase selector is deliberately narrower than the historical route.
	for _, args := range [][]string{
		{"skl", "ledger", "show", "--phase", "implement", "--format", "json"},
		{"skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--commit", committedHead, "--path", implementRef.Path, "--phase", "implement", "--format", "json"},
		{"skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--phase", "decision", "--format", "json"},
	} {
		refused := cli.ledgerJSON(t, args...)
		if refused.Status != "fix_required" || refused.Document != nil {
			t.Fatalf("invalid phase/history combination accepted (%v): %s", args, mustJSON(t, refused))
		}
	}
	if factoryCalls != factoryCallsBeforeInspection {
		t.Fatalf("report inspection called backend factory: %d -> %d", factoryCallsBeforeInspection, factoryCalls)
	}

	// Exercise the same reads with modified working files and staged content.
	// Every byte, the index, committed HEAD, Claims, and completed round count
	// must survive unchanged.
	reportPath := filepath.Join(fixture.clone, filepath.FromSlash(watchdogTwo.Report.Path))
	statePath := filepath.Join(fixture.clone, filepath.FromSlash(deliveryItemStatePath()))
	writeFile(t, reportPath, "working-tree report edit\n")
	writeFile(t, statePath, "working-tree state edit\n")
	writeFile(t, filepath.Join(fixture.clone, "human-private.txt"), "unstaged private bytes\n")
	writeFile(t, filepath.Join(fixture.clone, "staged-private.txt"), "staged private bytes\n")
	runGit(t, fixture.clone, "add", "staged-private.txt")
	dirtyReportBefore, dirtyStateBefore := readFileString(t, reportPath), readFileString(t, statePath)
	dirtyHeadBefore := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	dirtyIndexBefore := deliveryTrimmed(t, fixture.clone, "write-tree")
	dirtyStatusBefore := runGitOutput(t, fixture.clone, "status", "--porcelain", "--untracked-files=all")
	sourceBefore := ledgerSnapshot(t, source)
	factoryCallsBeforeReads := factoryCalls
	for _, args := range [][]string{
		{"skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--format", "json"},
		{"skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--phase", "watchdog", "--format", "json"},
		{"skl", "ledger", "show", "--commit", watchdogOne.Report.Commit, "--path", watchdogOne.Report.Path, "--format", "json"},
	} {
		read := cli.ledgerJSON(t, args...)
		if read.Status != "shown" {
			t.Fatalf("read failed with dirty ledger records (%v): %s", args, mustJSON(t, read))
		}
	}
	if factoryCalls != factoryCallsBeforeReads {
		t.Fatalf("read operation called backend factory: %d -> %d", factoryCallsBeforeReads, factoryCalls)
	}
	if deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD") != dirtyHeadBefore || deliveryTrimmed(t, fixture.clone, "write-tree") != dirtyIndexBefore || runGitOutput(t, fixture.clone, "status", "--porcelain", "--untracked-files=all") != dirtyStatusBefore {
		t.Fatal("read operations changed ledger HEAD, index, or dirty-file state")
	}
	if readFileString(t, reportPath) != dirtyReportBefore || readFileString(t, statePath) != dirtyStateBefore || readFileString(t, filepath.Join(fixture.clone, "human-private.txt")) != "unstaged private bytes\n" || readFileString(t, filepath.Join(fixture.clone, "staged-private.txt")) != "staged private bytes\n" {
		t.Fatal("read operations changed working-tree or staged private bytes")
	}
	if ledgerSnapshot(t, source) != sourceBefore {
		t.Fatal("read operations changed source Git state")
	}
	if got := deliveryGitShowForReportTest(t, fixture.clone, "HEAD", deliveryItemStatePath()); got != committedReadyState {
		t.Fatal("read operations changed committed Workflow State")
	}
	var stateAfter ledger.SliceState
	if err := json.Unmarshal([]byte(deliveryGitShowForReportTest(t, fixture.clone, "HEAD", "projects/widgets/proposals/later-claim/foundation/state.json")), &stateAfter); err != nil {
		t.Fatal(err)
	}
	if stateAfter.Claim == nil || stateAfter.Claim.Basis != laterState.Claim.Basis {
		t.Fatalf("read operations changed the later Claim: %#v", stateAfter.Claim)
	}
	if currentAfter := cli.ledgerJSON(t, "skl", "ledger", "show", "--repo", source, "--item", deliveryTestItem, "--phase", "watchdog", "--format", "json"); currentAfter.Document == nil || currentAfter.Document.Contents != currentWatchdog.Document.Contents {
		t.Fatal("dirty working-tree report replaced the committed current report")
	}
	if forge.createdCount() != 0 {
		t.Fatalf("read operations contacted/published to the forge: %d issues", forge.createdCount())
	}
}

func assertReportPhases(t *testing.T, readback *ledger.Readback, implement, watchdog *ledger.Reference) *ledger.Reference {
	t.Helper()
	if readback == nil {
		t.Fatal("readback is nil")
	}
	if len(readback.Reports) != 2 || readback.Reports[0].Phase != ledger.ImplementPhase || readback.Reports[1].Phase != ledger.WatchdogPhase {
		t.Fatalf("report availability order = %#v, want implement then watchdog", readback.Reports)
	}
	for index, expected := range []*ledger.Reference{implement, watchdog} {
		got := readback.Reports[index].Reference
		if expected == nil {
			if got != nil {
				t.Fatalf("%s report unexpectedly available at %#v", readback.Reports[index].Phase, got)
			}
			continue
		}
		if got == nil || *got != *expected {
			t.Fatalf("%s report reference = %#v, want %#v", readback.Reports[index].Phase, got, expected)
		}
	}
	return readback.Reports[0].Reference
}

func submitReportImplementation(t *testing.T, cli ledgerCLI, source, target, round, body string) (ledger.DeliveryResult, string, string, *ledger.Reference) {
	t.Helper()
	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil || started.Execution.Item != deliveryTestItem {
		t.Fatalf("implement next round %s = %#v, err=%v", round, started, err)
	}
	var consumedWatchdog *ledger.Reference
	if inputs := started.Execution.State.Claim.Inputs.Watchdog; inputs != nil {
		copy := *inputs
		consumedWatchdog = &copy
	}
	claim := started.Execution.Claim.Commit
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("implement prepare round %s = %#v, err=%v", round, prepared, err)
	}
	worktree := prepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "round-"+round+".txt"), "source evidence for round "+round+"\n")
	runGit(t, worktree, "add", "round-"+round+".txt")
	runGit(t, worktree, "commit", "-q", "-m", "implementation round "+round)
	head := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")
	bodyPath := writeTemp(t, t, body)
	submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", head, "--target", target, "--body", bodyPath, "--format", "json")
	if err != nil || submitted.Result == nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit round %s = %#v, err=%v", round, submitted, err)
	}
	return *submitted.Result, head, worktree, consumedWatchdog
}

func submitReportWatchdog(t *testing.T, cli ledgerCLI, source, outcome, body string) (ledger.DeliveryResult, *ledger.Reference) {
	t.Helper()
	started, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil || started.Execution.Item != deliveryTestItem {
		t.Fatalf("watchdog next = %#v, err=%v", started, err)
	}
	var consumedImplement *ledger.Reference
	if inputs := started.Execution.State.Claim.Inputs.Implement; inputs != nil {
		copy := *inputs
		consumedImplement = &copy
	}
	bodyPath := writeTemp(t, t, body)
	submitted, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem,
		"--claim", started.Execution.Claim.Commit, "--outcome", outcome, "--body", bodyPath, "--format", "json")
	if err != nil || submitted.Result == nil {
		t.Fatalf("watchdog submit %s = %#v, err=%v", outcome, submitted, err)
	}
	return *submitted.Result, consumedImplement
}

func reportCLIText(t *testing.T, cli ledgerCLI, args ...string) (string, error) {
	t.Helper()
	cli.out.Reset()
	err := cli.app.Run(args)
	return cli.out.String(), err
}

func deliveryGitShowForReportTest(t *testing.T, root, commit, path string) string {
	t.Helper()
	return runGitOutput(t, root, "show", fmt.Sprintf("%s:%s", commit, path))
}
