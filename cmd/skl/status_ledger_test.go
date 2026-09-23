package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

func statusRecord(t *testing.T, clone, proposal, slice string, edit func(*ledger.SliceState)) {
	t.Helper()
	path := filepath.Join("projects", "widgets", "proposals", proposal, slice, "state.json")
	var state ledger.SliceState
	if err := json.Unmarshal([]byte(readLedgerFile(t, clone, path)), &state); err != nil {
		t.Fatal(err)
	}
	edit(&state)
	contents, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(clone, path), string(contents)+"\n")
	runGit(t, clone, "add", path)
	runGit(t, clone, "commit", "-q", "-m", "fixture state")
}

func fixturePull(state, merged, head, merge, sourceRepo, targetRepo, targetBranch string) string {
	mergedAt := "null"
	if merged == "true" {
		mergedAt = `"2026-01-01T00:00:00Z"`
	}
	return fmt.Sprintf(`{"number":21,"state":%q,"merged":%s,"merged_at":%s,"merge_commit_sha":%q,"head":{"ref":"foundation","sha":%q,"repo":{"full_name":%q}},"base":{"ref":%q,"repo":{"full_name":%q}}}`, state, merged, mergedAt, merge, head, sourceRepo, targetBranch, targetRepo)
}

func completionStatusApp(t *testing.T, response func(http.ResponseWriter, *http.Request)) (*stageApp, *bytes.Buffer) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(response))
	t.Cleanup(server.Close)
	var output bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
		backend.BindRepository(repository)
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	return app, &output
}

func runCompletionStatus(t *testing.T, app *stageApp, output *bytes.Buffer, repo string, options ...string) ledgerStatusOutcome {
	t.Helper()
	output.Reset()
	arguments := append([]string{"skl", "status", "--repo", repo, "--format", "json"}, options...)
	if err := app.Run(arguments); err != nil {
		t.Fatalf("status: %v (%s)", err, output.String())
	}
	var outcome ledgerStatusOutcome
	if err := json.Unmarshal(output.Bytes(), &outcome); err != nil {
		t.Fatalf("status response %q: %v", output.String(), err)
	}
	return outcome
}

func attachFixture(t *testing.T, clone, proposal string) {
	t.Helper()
	statusRecord(t, clone, proposal, "foundation", func(state *ledger.SliceState) {
		state.State = ledger.ReadyForMerge
		state.Submission = &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 21}
		state.Target = &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "main"}
	})
}

func TestLedgerStatusConfirmsExactMergeAndOfflineDependency(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	proposal := dualSlice("completion")
	proposal.depends = map[string][]string{"feature": {"foundation"}}
	if outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", proposal)); outcome.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, outcome))
	}
	attachFixture(t, fixture.clone, "completion")
	const acceptedHead = "cccccccccccccccccccccccccccccccccccccccc"
	const mergeHead = "dddddddddddddddddddddddddddddddddddddddd"
	if err := exec.Command("git", "-C", root, "cat-file", "-e", acceptedHead+"^{commit}").Run(); err == nil {
		t.Fatal("historical accepted source unexpectedly exists in the local source repository")
	}
	beforeSource := ledgerSnapshot(t, root)
	requests := 0
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.URL.Path != "/repos/acme/widgets/pulls/21" {
			t.Errorf("unbounded forge request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", acceptedHead, mergeHead, "acme/widgets", "acme/widgets", "main"))
	})
	first := runCompletionStatus(t, app, output, root, "--item", "completion/foundation")
	if len(first.Items) != 1 || first.Items[0].State != ledger.Merged || first.Items[0].Completion.SourceHead != acceptedHead || first.Items[0].Completion.MergeCommit != mergeHead || requests != 1 {
		t.Fatalf("merge evidence not persisted from the owned PR: %+v, requests %d", first, requests)
	}
	if ledgerSnapshot(t, root) != beforeSource {
		t.Fatal("status changed source Git state")
	}
	// The committed fact is sufficient even when the forge and ledger remote
	// disappear: status neither re-reads the PR nor needs source history.
	runGit(t, fixture.clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "unavailable"))
	second := runCompletionStatus(t, app, output, root)
	if second.Items[1].State != ledger.Merged || second.Items[0].Dependencies[0].State != ledger.Merged || requests != 1 || second.Proposals[0].FullyDelivered {
		t.Fatalf("offline status or parent account: %+v", second)
	}
	selection := newLedgerApp(t, newForgeServer(t))
	selection.out.Reset()
	if err := selection.app.Run([]string{"skl", "implement", "next", "--repo", root, "--format", "json"}); err != nil || !strings.Contains(selection.out.String(), `"work_available"`) || !strings.Contains(selection.out.String(), "completion/feature") {
		t.Fatalf("confirmed Merged blocker did not enable selection: %v %s", err, selection.out.String())
	}
}

func TestLedgerStatusSquashMergeRetainsReviewedInputsWithoutSourceObjects(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("history")))
	attachFixture(t, fixture.clone, "history")
	const reviewed = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const integratedTarget = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const accepted = "cccccccccccccccccccccccccccccccccccccccc"
	const squash = "dddddddddddddddddddddddddddddddddddddddd"
	for _, revision := range []string{reviewed, integratedTarget, accepted, squash} {
		if exec.Command("git", "-C", root, "cat-file", "-e", revision+"^{commit}").Run() == nil {
			t.Fatalf("test requires absent historical source revision %s", revision)
		}
	}
	directory := filepath.Join("projects", "widgets", "proposals", "history", "foundation")
	statePath := filepath.ToSlash(filepath.Join(directory, "state.json"))
	implementPath := filepath.ToSlash(filepath.Join(directory, "implement-report.md"))
	watchdogPath := filepath.ToSlash(filepath.Join(directory, "watchdog-report.md"))
	contractPath := filepath.ToSlash(filepath.Join(directory, "intent.md"))
	original := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))
	claim := ledger.Reference{Commit: original, Path: statePath}
	contract := ledger.Reference{Commit: original, Path: contractPath}
	implement, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
		Schema: 1, Outcome: ledger.AwaitingReview,
		Source: ledger.SourceRevisions{Head: reviewed, Target: integratedTarget},
		Ledger: ledger.ReportInputs{Claim: claim, Contract: []ledger.Reference{contract}},
	}, "# Implementation evidence\n")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fixture.clone, implementPath), string(implement))
	runGit(t, fixture.clone, "add", implementPath)
	runGit(t, fixture.clone, "commit", "-q", "-m", "implementation fixture")
	implementationRef := ledger.Reference{Commit: strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")), Path: implementPath}
	watchdog, err := ledger.FormatReport(ledger.WatchdogPhase, ledger.Report{
		Schema: 1, Outcome: "pass", Round: 1,
		Source: ledger.SourceRevisions{Head: reviewed, Target: integratedTarget, Reviewed: reviewed},
		Ledger: ledger.ReportInputs{Claim: claim, Contract: []ledger.Reference{contract}, Implement: &implementationRef},
	}, "# Reviewed revision\n")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fixture.clone, watchdogPath), string(watchdog))
	runGit(t, fixture.clone, "add", watchdogPath)
	runGit(t, fixture.clone, "commit", "-q", "-m", "watchdog fixture")
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", accepted, squash, "acme/widgets", "acme/widgets", "main"))
	})
	got := runCompletionStatus(t, app, output, root, "--item", "history/foundation")
	if got.Items[0].State != ledger.Merged || got.Items[0].Completion.SourceHead != accepted || got.Items[0].Completion.MergeCommit != squash {
		t.Fatalf("accepted head and merge revision conflated: %+v", got)
	}
	if raw := readLedgerFile(t, fixture.clone, implementPath); raw != string(implement) {
		t.Fatalf("historical implementation report changed: %q", raw)
	}
	if raw := readLedgerFile(t, fixture.clone, watchdogPath); raw != string(watchdog) {
		t.Fatalf("historical Watchdog report changed: %q", raw)
	}
	store, err := ledger.Open(fixture.clone)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, err := ledger.CurrentReport(store, github.RepositoryID{Owner: "acme", Name: "widgets"}, "history/foundation", ledger.WatchdogPhase)
	if err != nil || parsed.Source.Reviewed != reviewed || parsed.Source.Head != reviewed || parsed.Ledger.Implement == nil || *parsed.Ledger.Implement != implementationRef || parsed.Ledger.Claim != claim || len(parsed.Ledger.Contract) != 1 || parsed.Ledger.Contract[0] != contract {
		t.Fatalf("review inputs were rewritten by terminal observation: %+v, %v", parsed, err)
	}
}

func TestLedgerStatusUnmergedClosureAndUnknownIdentityCannotSatisfyBlocker(t *testing.T) {
	for _, scenario := range []struct {
		name, state, merged, head, sourceRepo, targetRepo, base, expected string
	}{
		{"closed", "closed", "false", "last-head", "acme/widgets", "acme/widgets", "main", ledger.Superseded},
		{"foreign source", "closed", "true", "human-head", "other/widgets", "acme/widgets", "main", ledger.ReadyForMerge},
		{"foreign target", "closed", "true", "human-head", "acme/widgets", "other/widgets", "main", ledger.ReadyForMerge},
		{"wrong branch", "closed", "true", "human-head", "acme/widgets", "acme/widgets", "release", ledger.ReadyForMerge},
		{"unknown source", "closed", "true", "human-head", "", "acme/widgets", "main", ledger.ReadyForMerge},
		{"missing confirmation", "closed", "null", "human-head", "acme/widgets", "acme/widgets", "main", ledger.ReadyForMerge},
		{"missing target branch", "closed", "true", "human-head", "acme/widgets", "acme/widgets", "", ledger.ReadyForMerge},
		{"different number", "closed", "true", "human-head", "acme/widgets", "acme/widgets", "main", ledger.ReadyForMerge},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			root := sourceRepository(t, "acme", "widgets")
			proposal := dualSlice("candidate")
			proposal.depends = map[string][]string{"feature": {"foundation"}}
			newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", proposal))
			attachFixture(t, fixture.clone, "candidate")
			app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/acme/widgets/pulls/21" || r.Method != "GET" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				body := fixturePull(scenario.state, scenario.merged, scenario.head, "squash-head", scenario.sourceRepo, scenario.targetRepo, scenario.base)
				if scenario.name == "different number" {
					body = strings.Replace(body, `"number":21`, `"number":22`, 1)
				}
				_, _ = fmt.Fprint(w, body)
			})
			got := runCompletionStatus(t, app, output, root, "--item", "candidate/foundation")
			if got.Items[0].State != scenario.expected || (scenario.expected == ledger.ReadyForMerge) != (got.Items[0].Observation != "") {
				t.Fatalf("incorrect terminal verdict for %s: %+v", scenario.name, got)
			}
			selection := newLedgerApp(t, newForgeServer(t))
			selection.out.Reset()
			if err := selection.app.Run([]string{"skl", "implement", "next", "--repo", root, "--format", "json"}); err != nil || strings.Contains(selection.out.String(), `"work_available"`) {
				t.Fatalf("unmerged or uncertain blocker enabled dependent: %v %s", err, selection.out.String())
			}
		})
	}
}

func TestLedgerStatusStaleSelectedClaimAndReportsRefusedWithoutRequeue(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("race")))
	attachFixture(t, fixture.clone, "race")
	// A completed review and a new Claim are orthogonal to the terminal fact.
	original := "review evidence\n"
	reportPath := filepath.Join("projects", "widgets", "proposals", "race", "foundation", "watchdog-report.md")
	writeFile(t, filepath.Join(fixture.clone, reportPath), original)
	runGit(t, fixture.clone, "add", reportPath)
	runGit(t, fixture.clone, "commit", "-q", "-m", "review fixture")
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		statusRecord(t, fixture.clone, "race", "foundation", func(state *ledger.SliceState) {
			state.Claim = &ledger.Claim{Phase: ledger.ImplementPhase, Basis: "later-claim"}
		})
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "human-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
	})
	got := runCompletionStatus(t, app, output, root, "--item", "race/foundation")
	if got.Items[0].State != ledger.ReadyForMerge || !got.Items[0].Claimed || !strings.Contains(got.Items[0].Observation, "changed") || readLedgerFile(t, fixture.clone, reportPath) != original {
		t.Fatalf("stale observation damaged later work: %+v", got)
	}
	// Now observe against current preconditions; recording external completion
	// does not clear that Claim or rewrite the report.
	secondApp, secondOutput := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "human-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
	})
	second := runCompletionStatus(t, secondApp, secondOutput, root, "--item", "race/foundation")
	if second.Items[0].State != ledger.Merged || !second.Items[0].Claimed || readLedgerFile(t, fixture.clone, reportPath) != original {
		t.Fatalf("terminal observation released a Claim or rewrote a report: %+v", second)
	}
	before := runGitOutput(t, fixture.clone, "rev-parse", "HEAD")
	retry := runCompletionStatus(t, secondApp, secondOutput, root, "--item", "race/foundation")
	if retry.Items[0].State != ledger.Merged || before != runGitOutput(t, fixture.clone, "rev-parse", "HEAD") {
		t.Fatalf("terminal retry mutated prior result: %+v", retry)
	}
}

func TestLedgerStatusUnrelatedCommitAndReplicationFailure(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", dualSlice("unrelated")))
	attachFixture(t, fixture.clone, "unrelated")
	// The remote is unavailable, but another selected record's local write is
	// not a stale-write conflict for the PR being observed.
	runGit(t, fixture.clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "offline"))
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		statusRecord(t, fixture.clone, "unrelated", "feature", func(state *ledger.SliceState) { state.Title = "Other work progressed" })
		_, _ = fmt.Fprint(w, fixturePull("closed", "false", "last-head", "", "acme/widgets", "acme/widgets", "main"))
	})
	got := runCompletionStatus(t, app, output, root, "--item", "unrelated/foundation")
	if got.Items[0].State != ledger.Superseded || got.Items[0].Pending == nil || got.Items[0].Pending.Push == nil {
		t.Fatalf("local closure or pending replication lost: %+v", got)
	}
}

func TestLedgerStatusDoesNotClearNewerReplicationNote(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("replication-race")))
	attachFixture(t, fixture.clone, "replication-race")
	path := filepath.Join("projects", "widgets", "proposals", "replication-race", "foundation", "state.json")
	temporary := filepath.Join(t.TempDir(), "state.json")
	errorsPath := filepath.Join(t.TempDir(), "hook-errors")
	// A real bare-remote receive hook advances only the selected pending note
	// during the completion push, after the terminal commit but before the
	// original observer can clear its own replication marker.
	hook := fmt.Sprintf(`#!/bin/sh
set -e
for name in $(git rev-parse --local-env-vars); do unset "$name"; done
unset GIT_QUARANTINE_PATH GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES
p=%q
awk '{ gsub("terminal ledger commit has not been replicated", "newer pending replication"); print }' < %q > %q
mv %q %q
git -C %q add -- "$p" 2> %q
git -C %q commit -q -m 'newer replication note' -- "$p" 2>> %q
`, path, filepath.Join(fixture.clone, path), temporary, temporary, filepath.Join(fixture.clone, path), fixture.clone, errorsPath, fixture.clone, errorsPath)
	hookPath := filepath.Join(fixture.upstream, "hooks", "pre-receive")
	writeFile(t, hookPath, hook)
	if err := os.Chmod(hookPath, 0o755); err != nil {
		t.Fatal(err)
	}
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fixturePull("closed", "false", "last-head", "", "acme/widgets", "acme/widgets", "main"))
	})
	got := runCompletionStatus(t, app, output, root, "--item", "replication-race/foundation")
	if got.Items[0].State != ledger.Superseded || got.Items[0].Pending == nil || got.Items[0].Pending.Push == nil || got.Items[0].Pending.Push.Detail != "newer pending replication" {
		diagnostics, _ := os.ReadFile(errorsPath)
		t.Fatalf("newer pending replication overwritten by stale post-push cleanup: %+v; state %s; log %s; hook errors %s", got, readLedgerFile(t, fixture.clone, path), runGitOutput(t, fixture.clone, "log", "-3", "--oneline"), diagnostics)
	}
}

func TestLedgerStatusUnavailableBlockerAndLocalReadScope(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	proposal := dualSlice("bounded")
	proposal.depends = map[string][]string{"feature": {"foundation"}}
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", proposal))
	attachFixture(t, fixture.clone, "bounded")
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/pulls/21" || r.Method != "GET" {
			t.Errorf("inventory or unrelated request: %s %s", r.Method, r.URL.Path)
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	blocked := runCompletionStatus(t, app, output, root, "--item", "bounded/foundation")
	if blocked.Items[0].State != ledger.ReadyForMerge || !strings.Contains(blocked.Items[0].Observation, "unavailable") {
		t.Fatalf("forge outage invented completion: %+v", blocked)
	}
	selection := newLedgerApp(t, newForgeServer(t))
	selection.out.Reset()
	if err := selection.app.Run([]string{"skl", "implement", "next", "--repo", root, "--format", "json"}); err != nil || strings.Contains(selection.out.String(), `"work_available"`) {
		t.Fatalf("unknown blocker granted a Claim: %v %s", err, selection.out.String())
	}
	// Damage an unrelated child: the selected fixed-item read still needs only
	// the selected state and its exact attachment, not a project rebuild.
	path := filepath.Join("projects", "widgets", "proposals", "bounded", "feature", "state.json")
	writeFile(t, filepath.Join(fixture.clone, path), "not JSON\n")
	runGit(t, fixture.clone, "add", path)
	runGit(t, fixture.clone, "commit", "-q", "-m", "unrelated damaged record")
	fixed := runCompletionStatus(t, app, output, root, "--item", "bounded/foundation")
	if fixed.Items[0].State != ledger.ReadyForMerge || fixed.Items[0].Observation == "" {
		t.Fatalf("fixed-item status hydrated unrelated child: %+v", fixed)
	}
}

func TestLedgerStatusReportInterleaveRefusesStaleObservation(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("report-race")))
	attachFixture(t, fixture.clone, "report-race")
	path := filepath.Join("projects", "widgets", "proposals", "report-race", "foundation", "implement-report.md")
	writeFile(t, filepath.Join(fixture.clone, path), "earlier report\n")
	runGit(t, fixture.clone, "add", path)
	runGit(t, fixture.clone, "commit", "-q", "-m", "earlier report")
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		writeFile(t, filepath.Join(fixture.clone, path), "new report\n")
		runGit(t, fixture.clone, "add", path)
		runGit(t, fixture.clone, "commit", "-q", "-m", "later report")
		_, _ = fmt.Fprint(w, fixturePull("closed", "false", "last-head", "", "acme/widgets", "acme/widgets", "main"))
	})
	got := runCompletionStatus(t, app, output, root, "--item", "report-race/foundation")
	if got.Items[0].State != ledger.ReadyForMerge || !strings.Contains(got.Items[0].Observation, "phase results changed") || readLedgerFile(t, fixture.clone, path) != "new report\n" {
		t.Fatalf("stale report observation clobbered later evidence: %+v", got)
	}
}

func TestLedgerStatusMixedParentRetirementAndMissingAttachment(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", dualSlice("parent")))
	statusRecord(t, fixture.clone, "parent", "foundation", func(state *ledger.SliceState) {
		state.State = ledger.Merged
		state.Completion = &ledger.TerminalEvidence{Submission: ledger.ForgeAttachment{Repository: "acme/widgets", Number: 21}, Target: ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "main"}}
	})
	statusRecord(t, fixture.clone, "parent", "feature", func(state *ledger.SliceState) { state.State = ledger.Superseded })
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("terminal or unattached status called forge: %s", r.URL.Path)
	})
	mixed := runCompletionStatus(t, app, output, root)
	if len(mixed.Proposals) != 1 || mixed.Proposals[0].FullyDelivered || !mixed.Proposals[0].Retireable {
		t.Fatalf("mixed terminal proposal mistaken for delivery: %+v", mixed)
	}
	statusRecord(t, fixture.clone, "parent", "feature", func(state *ledger.SliceState) {
		state.Claim = &ledger.Claim{Phase: ledger.ImplementPhase, Basis: "retained"}
	})
	claimed := runCompletionStatus(t, app, output, root)
	if claimed.Proposals[0].Retireable || !claimed.Items[0].Claimed {
		t.Fatalf("retained Claim hidden from parent: %+v", claimed)
	}
	statusRecord(t, fixture.clone, "parent", "feature", func(state *ledger.SliceState) {
		state.Claim = nil
		state.State = ledger.Merged
	})
	full := runCompletionStatus(t, app, output, root)
	if !full.Proposals[0].FullyDelivered || !full.Proposals[0].Retireable {
		t.Fatalf("all-merged parent not delivered: %+v", full)
	}
	statusRecord(t, fixture.clone, "parent", "foundation", func(state *ledger.SliceState) {
		state.State = ledger.Superseded
		state.Completion = nil
	})
	statusRecord(t, fixture.clone, "parent", "feature", func(state *ledger.SliceState) { state.State = ledger.Superseded })
	abandoned := runCompletionStatus(t, app, output, root)
	if abandoned.Proposals[0].FullyDelivered || !abandoned.Proposals[0].Retireable {
		t.Fatalf("all-superseded parent mistaken for delivery: %+v", abandoned)
	}
	// A new local approval without a published PR is not abandonment.
	unpublished := singleSlice("unpublished")
	unpublished.slices[0].branch = "unpublished-branch"
	if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", unpublished)); accepted.Status != "accepted" {
		t.Fatalf("accept unpublished: %s", mustJSON(t, accepted))
	}
	statusRecord(t, fixture.clone, "unpublished", "foundation", func(state *ledger.SliceState) { state.State = ledger.ReadyForMerge })
	missing := runCompletionStatus(t, app, output, root, "--item", "unpublished/foundation")
	if missing.Items[0].State != ledger.ReadyForMerge || !strings.Contains(missing.Items[0].Observation, "no exact owned Submission") {
		t.Fatalf("unpublished approval misreported: %+v", missing)
	}
	statusRecord(t, fixture.clone, "unpublished", "foundation", func(state *ledger.SliceState) {
		state.Submission = &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 21}
	})
	lateApp, lateOutput := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		body := fixturePull("closed", "true", "accepted-head", "merged-head", "acme/widgets", "acme/widgets", "main")
		_, _ = fmt.Fprint(w, strings.Replace(body, `"ref":"foundation"`, `"ref":"unpublished-branch"`, 1))
	})
	late := runCompletionStatus(t, lateApp, lateOutput, root, "--item", "unpublished/foundation")
	if late.Items[0].State != ledger.Merged {
		t.Fatalf("late ordinary attachment not observed: %+v", late)
	}
}
