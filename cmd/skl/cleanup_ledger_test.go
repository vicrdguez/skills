package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vicrdguez/skills/ledger"
)

const cleanupMergeCommit = "ffffffffffffffffffffffffffffffffffffffff"

// cleanupSpec is one proposal whose slices own branches named
// <proposal>-<slice>, so several proposals can share one Project.
func cleanupSpec(name string, slices ...string) proposalSpec {
	spec := proposalSpec{name: name, description: "# " + name + "\n\nDurable description.\n", depends: map[string][]string{}}
	if len(slices) > 1 {
		spec.parentTitle = "Deliver " + name
	}
	for _, slice := range slices {
		spec.slices = append(spec.slices, proposalSliceSpec{
			name: slice, title: "Deliver " + name + " " + slice, branch: name + "-" + slice,
			files: map[string]string{"intent.md": "# " + slice + " intent\n", "behavior.md": "# " + slice + " behavior\n"},
		})
	}
	return spec
}

// merged records a confirmed merge of the slice's owned Submission.
func merged(sourceHead string) func(*ledger.SliceState) {
	return func(state *ledger.SliceState) {
		submission := ledger.ForgeAttachment{Repository: "acme/widgets", Number: 21}
		target := ledger.IntegrationTarget{Repository: "acme/widgets", Branch: "main"}
		state.State, state.Submission, state.Target = ledger.Merged, &submission, &target
		state.Completion = &ledger.TerminalEvidence{Submission: submission, Target: target, SourceHead: sourceHead, MergeCommit: cleanupMergeCommit}
	}
}

func lifecycle(value string) func(*ledger.SliceState) {
	return func(state *ledger.SliceState) { state.State = value }
}

// offlineForge fails every forge request; cleanup must not need one.
func offlineForge(t *testing.T) ledgerCLI {
	t.Helper()
	forge := newForgeServer(t)
	forge.fail = func(method, path string) int {
		t.Errorf("cleanup contacted the forge: %s %s", method, path)
		return http.StatusServiceUnavailable
	}
	return newLedgerApp(t, forge)
}

func runCleanup(t *testing.T, cli ledgerCLI, root string) cleanupOutcome {
	t.Helper()
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "propose", "cleanup", "--repo", root, "--format", "json"}); err != nil {
		t.Fatalf("cleanup: %v\n%s", err, cli.out.String())
	}
	var outcome cleanupOutcome
	if err := json.Unmarshal(cli.out.Bytes(), &outcome); err != nil {
		t.Fatalf("cleanup response %q: %v", cli.out.String(), err)
	}
	return outcome
}

func ledgerTree(t *testing.T, clone, revision, path string) string {
	t.Helper()
	return strings.TrimSpace(runGitOutput(t, clone, "rev-parse", revision+":"+path))
}

func ledgerHead(t *testing.T, clone string) string {
	t.Helper()
	return strings.TrimSpace(runGitOutput(t, clone, "rev-parse", "HEAD"))
}

func archivedNames(outcome cleanupOutcome) map[string]bool {
	names := map[string]bool{}
	if outcome.Archive != nil {
		for _, archived := range outcome.Archive.Archived {
			names[archived.Proposal] = archived.FullyDelivered
		}
	}
	return names
}

func TestCleanupArchivesWholeTerminalProposals(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	// Issue publication fails: archival needs no public forge state.
	unavailable := newForgeServer(t)
	unavailable.fail = func(string, string) int { return http.StatusServiceUnavailable }
	accepting := newLedgerApp(t, unavailable)
	accept := func(spec proposalSpec) {
		t.Helper()
		if outcome := accepting.accept(t, root, writeProposal(t, "", spec)); outcome.Status != "accepted" {
			t.Fatalf("accept %s: %s", spec.name, mustJSON(t, outcome))
		}
	}
	accept(cleanupSpec("widget", "core", "extra"))
	accept(cleanupSpec("mixed", "kept", "dropped"))
	others := []struct {
		name string
		edit func(*ledger.SliceState)
	}{
		{"ready-merge", lifecycle(ledger.ReadyForMerge)},
		{"needs-human", lifecycle(ledger.NeedsHuman)},
		{"rework", lifecycle(ledger.Rework)},
		{"ready-implementation", nil}, // as accepted
		{"awaiting-review", lifecycle(ledger.AwaitingReview)},
		{"claimed-terminal", func(state *ledger.SliceState) {
			merged("")(state)
			state.Claim = &ledger.Claim{Phase: ledger.ImplementPhase, Basis: cleanupMergeCommit}
		}},
		{"unknown-state", lifecycle("imaginary")},
	}
	for _, other := range others {
		accept(cleanupSpec(other.name, "done", "other"))
		statusRecord(t, fixture.clone, other.name, "done", merged(""))
		if other.edit != nil {
			statusRecord(t, fixture.clone, other.name, "other", other.edit)
		}
	}
	consumer := cleanupSpec("consumer", "uses-widget", "uses-dropped")
	consumer.depends = map[string][]string{"uses-widget": {"proposals/widget/core"}, "uses-dropped": {"proposals/mixed/dropped"}}
	accept(consumer)

	// widget: both slices Merged. Its implementation report changes after a
	// Watchdog report cited the earlier version by exact commit and path.
	// A pending replication fact is an ordinary record the archive retains.
	statusRecord(t, fixture.clone, "widget", "core", func(state *ledger.SliceState) {
		merged("")(state)
		state.Publication = &ledger.PublicationState{Push: &ledger.PublicationNote{Status: "pending", Detail: "ledger remote unavailable"}}
	})
	statusRecord(t, fixture.clone, "widget", "extra", merged(""))
	reportPath := "projects/widgets/proposals/widget/core/implement-report.md"
	writeFile(t, filepath.Join(fixture.clone, reportPath), "# Implementation v1\n")
	runGit(t, fixture.clone, "add", reportPath)
	runGit(t, fixture.clone, "commit", "-q", "-m", "implementation v1")
	cited := ledgerHead(t, fixture.clone)
	watchdogPath := "projects/widgets/proposals/widget/core/watchdog-report.md"
	watchdog := "---\nimplement: {commit: " + cited + ", path: " + reportPath + "}\n---\n# Review\n"
	writeFile(t, filepath.Join(fixture.clone, watchdogPath), watchdog)
	writeFile(t, filepath.Join(fixture.clone, reportPath), "# Implementation v2\n")
	runGit(t, fixture.clone, "add", reportPath, watchdogPath)
	runGit(t, fixture.clone, "commit", "-q", "-m", "implementation v2 and review")
	statusRecord(t, fixture.clone, "mixed", "kept", merged(""))
	statusRecord(t, fixture.clone, "mixed", "dropped", lifecycle(ledger.Superseded))

	before := ledgerHead(t, fixture.clone)
	activeTrees := map[string]string{}
	for _, name := range []string{"widget", "mixed", "consumer"} {
		activeTrees[name] = ledgerTree(t, fixture.clone, before, "projects/widgets/proposals/"+name)
	}
	for _, other := range others {
		activeTrees[other.name] = ledgerTree(t, fixture.clone, before, "projects/widgets/proposals/"+other.name)
	}
	// The ledger remote is unavailable: the local archive stays authoritative.
	runGit(t, fixture.clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing"))
	sourceBefore := ledgerSnapshot(t, root)

	cli := offlineForge(t)
	first := runCleanup(t, cli, root)
	if first.Status != "completed" || len(first.Archive.Archived) != 2 || !archivedNames(first)["widget"] || archivedNames(first)["mixed"] {
		t.Fatalf("expected widget fully delivered and mixed retired: %s", mustJSON(t, first))
	}
	if first.Archive.Replication == nil || first.Archive.Replication.Status != ledger.PushPending {
		t.Fatalf("offline ledger push not reported pending: %s", mustJSON(t, first))
	}
	head := ledgerHead(t, fixture.clone)
	for _, name := range []string{"widget", "mixed"} {
		// Identical tree IDs: every document, state, report, and pending
		// publication fact moved together with unchanged bytes.
		if got := ledgerTree(t, fixture.clone, head, "projects/widgets/archive/"+name); got != activeTrees[name] {
			t.Fatalf("%s archive tree %s differs from its active tree %s", name, got, activeTrees[name])
		}
		if _, err := os.Stat(filepath.Join(fixture.clone, "projects", "widgets", "proposals", name)); !os.IsNotExist(err) {
			t.Fatalf("%s still present in the active proposals directory: %v", name, err)
		}
	}
	var pending ledger.SliceState
	if err := json.Unmarshal([]byte(readLedgerFile(t, fixture.clone, "projects/widgets/archive/widget/core/state.json")), &pending); err != nil || pending.Publication == nil || pending.Publication.Push == nil || pending.Publication.Push.Status != "pending" {
		t.Fatalf("pending publication not retained: %+v %v", pending, err)
	}
	kept := map[string]string{}
	for _, entry := range first.Archive.Kept {
		kept[entry.Proposal] = entry.Reason
	}
	for _, other := range append(others, struct {
		name string
		edit func(*ledger.SliceState)
	}{name: "consumer"}) {
		if kept[other.name] == "" || ledgerTree(t, fixture.clone, head, "projects/widgets/proposals/"+other.name) != activeTrees[other.name] {
			t.Fatalf("ineligible %s not kept whole with a reason: %q", other.name, kept[other.name])
		}
		if _, err := os.Stat(filepath.Join(fixture.clone, "projects", "widgets", "archive", other.name)); !os.IsNotExist(err) {
			t.Fatalf("ineligible %s leaked children into the archive: %v", other.name, err)
		}
	}
	if !strings.Contains(kept["claimed-terminal"], "claimed") || !strings.Contains(kept["unknown-state"], "unknown") {
		t.Fatalf("preservation reasons not observable: %v", kept)
	}
	if ledgerSnapshot(t, root) != sourceBefore {
		t.Fatal("archival changed source Git state without owned merged source work")
	}

	// Historical and current reads after the move.
	historical, _ := cli.run(t, []string{"skl", "ledger", "show", "--commit", cited, "--path", reportPath, "--format", "json"})
	if historical.Document == nil || historical.Document.Contents != "# Implementation v1\n" {
		t.Fatalf("cited historical report not retrievable: %s", mustJSON(t, historical))
	}
	current, _ := cli.run(t, []string{"skl", "ledger", "show", "--commit", head, "--path", "projects/widgets/archive/widget/core/implement-report.md", "--format", "json"})
	if current.Document == nil || current.Document.Contents != "# Implementation v2\n" {
		t.Fatalf("current archived report not readable: %s", mustJSON(t, current))
	}
	if readLedgerFile(t, fixture.clone, "projects/widgets/archive/widget/core/watchdog-report.md") != watchdog {
		t.Fatal("review reference rewritten by archival")
	}
	shown, _ := cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--item", "widget/core", "--format", "json"})
	if shown.Readback == nil || shown.Readback.State != ledger.Merged || shown.Readback.Documents[0].Path != "projects/widgets/archive/widget/core/behavior.md" {
		t.Fatalf("archived identity not resolvable: %s", mustJSON(t, shown))
	}
	dependencies := map[string]string{}
	for _, item := range []string{"consumer/uses-widget", "consumer/uses-dropped"} {
		readback, _ := cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--item", item, "--format", "json"})
		dependencies[readback.Readback.Dependencies[0].Item] = readback.Readback.Dependencies[0].State
	}
	if dependencies["proposals/widget/core"] != ledger.Merged || dependencies["proposals/mixed/dropped"] != ledger.Superseded {
		t.Fatalf("archived blockers changed meaning: %v", dependencies)
	}
	// New work may name an archived blocker; an archived name is not re-accepted.
	later := cleanupSpec("later", "follow-up")
	later.depends = map[string][]string{"follow-up": {"proposals/widget/core"}}
	accept(later)
	if again := accepting.accept(t, root, writeProposal(t, "", cleanupSpec("widget", "core", "extra"))); again.Status != "fix_required" || !strings.Contains(again.Reason, "archived") {
		t.Fatalf("archived proposal accepted again: %s", mustJSON(t, again))
	}

	// Retry is harmless offline, and competing remote history is left for
	// explicit reconciliation rather than merged.
	beforeRetry := ledgerHead(t, fixture.clone)
	if retry := runCleanup(t, cli, root); retry.Status != "no_work" || len(archivedNames(retry)) != 0 || ledgerHead(t, fixture.clone) != beforeRetry {
		t.Fatalf("retry moved or rewrote records: %s", mustJSON(t, retry))
	}
	runGit(t, fixture.clone, "remote", "set-url", "origin", fixture.upstream)
	competitor := filepath.Join(t.TempDir(), "competitor")
	runGit(t, t.TempDir(), "clone", "-q", fixture.upstream, competitor)
	runGit(t, competitor, "-c", "user.name=Other", "-c", "user.email=other@example.com", "commit", "-q", "--allow-empty", "-m", "competing")
	runGit(t, competitor, "push", "-q", "origin", "main")
	upstream := strings.TrimSpace(runGitOutput(t, competitor, "rev-parse", "HEAD"))
	statusRecord(t, fixture.clone, "rework", "other", merged(""))
	competing := runCleanup(t, cli, root)
	if competing.Status != "fix_required" || competing.ArchiveRepair == nil || !strings.Contains(competing.ArchiveRepair.Reason, "reconciliation") {
		t.Fatalf("competing history not reported for reconciliation: %s", mustJSON(t, competing))
	}
	if strings.TrimSpace(runGitOutput(t, fixture.upstream, "rev-parse", "main")) != upstream || !exists(t, fixture.clone, "projects/widgets/archive/widget") || !exists(t, fixture.clone, "projects/widgets/proposals/rework") {
		t.Fatal("reconciliation refusal changed remote history or local records")
	}
}

func exists(t *testing.T, root, path string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
	return err == nil
}

func TestCleanupArchiveFailuresKeepOneCompleteProposal(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	accepting := newLedgerApp(t, newForgeServer(t))
	names := []string{"collide", "hooked", "interrupted", "partial", "moded", "dirty", "staged", "renamed", "matching"}
	for _, name := range names {
		if outcome := accepting.accept(t, root, writeProposal(t, "", cleanupSpec(name, "only"))); outcome.Status != "accepted" {
			t.Fatalf("accept %s: %s", name, mustJSON(t, outcome))
		}
		statusRecord(t, fixture.clone, name, "only", lifecycle(ledger.Superseded))
	}
	active := func(name string) string { return "projects/widgets/proposals/" + name }
	archived := func(name string) string { return "projects/widgets/archive/" + name }
	// A distinct committed record already occupies one destination.
	writeFile(t, filepath.Join(fixture.clone, archived("collide"), "proposal.md"), "# a different record\n")
	runGit(t, fixture.clone, "add", archived("collide"))
	runGit(t, fixture.clone, "commit", "-q", "-m", "distinct archive")
	original := ledgerHead(t, fixture.clone)
	trees := map[string]string{}
	for _, name := range names {
		trees[name] = ledgerTree(t, fixture.clone, original, active(name))
	}
	// An interrupted run moved a directory without committing it.
	interrupt := func(name string) {
		t.Helper()
		if err := os.Rename(filepath.Join(fixture.clone, active(name)), filepath.Join(fixture.clone, archived(name))); err != nil {
			t.Fatal(err)
		}
	}
	// This partial tree no longer matches its committed record.
	interrupt("partial")
	// Same bytes, but a changed mode is not the committed record either.
	interrupt("moded")
	if err := os.Chmod(filepath.Join(fixture.clone, archived("moded"), "only", "intent.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fixture.clone, archived("partial"), "only", "intent.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fixture.clone, active("dirty"), "only", "notes.md"), "uncommitted\n")
	// A complete move whose working tree matches the committed record but
	// whose index holds a distinct version, staged at the destination only
	// or as the whole rename.
	const unique = "unique staged content that must survive\n"
	stageDistinct := func(name string, paths ...string) {
		t.Helper()
		interrupt(name)
		intent := filepath.Join(fixture.clone, archived(name), "only", "intent.md")
		writeFile(t, intent, unique)
		runGit(t, fixture.clone, append([]string{"add", "-A", "--"}, paths...)...)
		writeFile(t, intent, "# only intent\n")
	}
	stageDistinct("staged", archived("staged"))
	stageDistinct("renamed", active("renamed"), archived("renamed"))
	stagedIntent := func(name string) string {
		t.Helper()
		return runGitOutput(t, fixture.clone, "show", ":"+archived(name)+"/only/intent.md")
	}
	// Every commit fails while the hook exists.
	hook := filepath.Join(fixture.clone, ".git", "hooks", "pre-commit")
	writeFile(t, hook, "#!/bin/sh\nexit 1\n")
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}

	cli := offlineForge(t)
	failed := runCleanup(t, cli, root)
	repairs := map[string]string{}
	for _, repair := range failed.Archive.Repairs {
		repairs[repair.Proposal] = repair.Reason
	}
	if failed.Status != "fix_required" || len(failed.Archive.Archived) != 0 || len(repairs) != 9 || ledgerHead(t, fixture.clone) != original {
		t.Fatalf("a failed move reported success or committed: %s", mustJSON(t, failed))
	}
	for name, reason := range map[string]string{
		"collide": "committed record", "hooked": "commit failed", "interrupted": "commit failed", "matching": "commit failed",
		"partial": "does not match", "moded": "does not match", "dirty": "uncommitted",
		"staged": "staged changes", "renamed": "staged changes",
	} {
		if !strings.Contains(repairs[name], reason) {
			t.Fatalf("%s repair reason %q lacks %q", name, repairs[name], reason)
		}
	}
	if got := strings.TrimSpace(runGitOutput(t, fixture.clone, "status", "--porcelain", "--", active("hooked"), archived("hooked"))); got != "" {
		t.Fatalf("failed commit did not restore the original location: %q", got)
	}
	if !strings.Contains(readLedgerFile(t, fixture.clone, archived("collide")+"/proposal.md"), "different record") || !exists(t, fixture.clone, active("collide")+"/only/state.json") {
		t.Fatal("collision overwrote or removed a record")
	}
	if exists(t, fixture.clone, active("partial")) || !exists(t, fixture.clone, archived("partial")+"/only/behavior.md") || readLedgerFile(t, fixture.clone, active("dirty")+"/only/notes.md") != "uncommitted\n" {
		t.Fatal("repair discarded a partial tree or uncommitted edit")
	}
	for _, name := range []string{"staged", "renamed"} {
		if stagedIntent(name) != unique {
			t.Fatalf("%s lost its distinct staged content", name)
		}
	}

	// The failed commit left "interrupted" whole at its original location; now
	// an interruption leaves a complete but uncommitted move.
	interrupt("interrupted")
	// A staged rename holding exactly the committed record is still resumable.
	interrupt("matching")
	runGit(t, fixture.clone, "add", "-A", "--", active("matching"), archived("matching"))
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	retried := runCleanup(t, cli, root)
	head := ledgerHead(t, fixture.clone)
	resumed := map[string]bool{}
	for _, archive := range retried.Archive.Archived {
		resumed[archive.Proposal] = archive.Resumed
	}
	if len(resumed) != 3 || resumed["hooked"] || !resumed["interrupted"] || !resumed["matching"] {
		t.Fatalf("retry did not finish the identified move and archive the repaired one: %s", mustJSON(t, retried))
	}
	for _, name := range []string{"hooked", "interrupted", "matching"} {
		if ledgerTree(t, fixture.clone, head, archived(name)) != trees[name] || exists(t, fixture.clone, active(name)) {
			t.Fatalf("%s not archived whole", name)
		}
	}
	if ledgerTree(t, fixture.clone, head, active("partial")) != trees["partial"] {
		t.Fatal("the committed record of the ambiguous partial move changed")
	}
	for _, name := range []string{"staged", "renamed"} {
		if ledgerTree(t, fixture.clone, head, active(name)) != trees[name] || stagedIntent(name) != unique {
			t.Fatalf("retry changed %s or its distinct staged content", name)
		}
	}
}

// cleanupWorktree checks out a new owned branch at .worktrees/<branch> and
// returns its head; a remote-tracking ref stands in for the pushed branch.
func cleanupWorktree(t *testing.T, root, branch string) string {
	t.Helper()
	runGit(t, root, "worktree", "add", "-q", filepath.Join(root, ".worktrees", branch), "-b", branch, "main")
	runGit(t, root, "update-ref", "refs/remotes/origin/"+branch, "refs/heads/"+branch)
	return strings.TrimSpace(runGitOutput(t, root, "rev-parse", branch))
}

func TestLedgerCleanupRemovesOnlySafeMergedSourceWork(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	accepting := newLedgerApp(t, newForgeServer(t))
	slices := []string{"clean", "squashed", "dirty", "moved", "unknown", "contradicted", "dropped", "claimed", "locked"}
	for _, spec := range []proposalSpec{cleanupSpec("done", slices...), cleanupSpec("active", "safe", "pending")} {
		if outcome := accepting.accept(t, root, writeProposal(t, "", spec)); outcome.Status != "accepted" {
			t.Fatalf("accept %s: %s", spec.name, mustJSON(t, outcome))
		}
	}
	heads := map[string]string{}
	for _, branch := range []string{"done-clean", "done-squashed", "done-dirty", "done-unknown", "done-contradicted", "done-dropped", "done-claimed", "done-locked", "active-safe", "active-pending"} {
		heads[branch] = cleanupWorktree(t, root, branch)
	}
	// A squash merge: the accepted head is not an ancestor of the target and
	// its upstream branch has been pruned.
	squashed := filepath.Join(root, ".worktrees", "done-squashed")
	writeFile(t, filepath.Join(squashed, "README.md"), "accepted change\n")
	runGit(t, squashed, "commit", "-qam", "slice change")
	heads["done-squashed"] = strings.TrimSpace(runGitOutput(t, squashed, "rev-parse", "HEAD"))
	runGit(t, root, "update-ref", "-d", "refs/remotes/origin/done-squashed")
	writeFile(t, filepath.Join(root, "README.md"), "accepted change\n")
	runGit(t, root, "commit", "-qam", "squash")
	writeFile(t, filepath.Join(root, ".worktrees", "done-dirty", "local.txt"), "keep\n")
	runGit(t, root, "branch", "done-moved", "main")
	runGit(t, root, "worktree", "add", "-q", filepath.Join(root, "elsewhere"), "done-moved")
	heads["done-moved"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "done-moved"))
	runGit(t, root, "update-ref", "refs/remotes/origin/done-moved", "refs/heads/done-moved")
	runGit(t, root, "worktree", "lock", filepath.Join(root, ".worktrees", "done-locked"))

	for _, slice := range []string{"clean", "squashed", "dirty", "moved", "locked"} {
		statusRecord(t, fixture.clone, "done", slice, merged(heads["done-"+slice]))
	}
	statusRecord(t, fixture.clone, "done", "unknown", merged(""))
	statusRecord(t, fixture.clone, "done", "contradicted", func(state *ledger.SliceState) {
		merged(heads["done-contradicted"])(state)
		state.Completion.Submission.Number = 99
	})
	// Unmerged closure records the same exact evidence, but never authorizes deletion.
	statusRecord(t, fixture.clone, "done", "dropped", func(state *ledger.SliceState) {
		merged(heads["done-dropped"])(state)
		state.State = ledger.Superseded
	})
	// A terminal Claim preserves source work and keeps the proposal active.
	statusRecord(t, fixture.clone, "done", "claimed", func(state *ledger.SliceState) {
		merged(heads["done-claimed"])(state)
		state.Claim = &ledger.Claim{Phase: ledger.WatchdogPhase, Basis: cleanupMergeCommit}
	})
	statusRecord(t, fixture.clone, "active", "safe", merged(heads["active-safe"]))
	statusRecord(t, fixture.clone, "active", "pending", lifecycle(ledger.ReadyForMerge))
	remoteRefs := runGitOutput(t, root, "for-each-ref", "refs/remotes")

	cli := offlineForge(t)
	first := runCleanup(t, cli, root)
	if len(first.Archive.Archived) != 0 {
		t.Fatalf("a claimed proposal was archived: %s", mustJSON(t, first))
	}
	removed := strings.Join(first.Source.Removed, " ")
	if removed != "done-clean done-squashed active-safe" && removed != "active-safe done-clean done-squashed" {
		t.Fatalf("removed %q: %s", removed, mustJSON(t, first))
	}
	preserved := map[string]string{}
	for _, entry := range first.Source.Preserved {
		preserved[entry.Branch] = entry.Reason
	}
	for _, branch := range []string{"done-dirty", "done-moved", "done-unknown", "done-contradicted", "done-dropped", "done-claimed"} {
		if preserved[branch] == "" {
			t.Fatalf("%s not reported preserved: %s", branch, mustJSON(t, first))
		}
	}
	if len(first.Source.Failed) != 1 || first.Source.Failed[0].Branch != "done-locked" || first.Status != "fix_required" {
		t.Fatalf("removal failure hidden: %s", mustJSON(t, first))
	}
	for _, branch := range []string{"done-clean", "done-squashed", "active-safe"} {
		if gitRefExists(root, "refs/heads/"+branch) || exists(t, root, ".worktrees/"+branch) {
			t.Fatalf("%s local work remains", branch)
		}
	}
	for branch, head := range heads {
		if branch == "done-clean" || branch == "done-squashed" || branch == "active-safe" {
			continue
		}
		if got := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "refs/heads/"+branch)); got != head {
			t.Fatalf("%s branch changed to %q", branch, got)
		}
	}
	if readFileString(t, filepath.Join(root, ".worktrees", "done-dirty", "local.txt")) != "keep\n" || !exists(t, root, ".worktrees/active-pending") || !exists(t, root, "elsewhere") {
		t.Fatal("preserved source contents changed")
	}
	if runGitOutput(t, root, "for-each-ref", "refs/remotes") != remoteRefs {
		t.Fatal("cleanup changed remote-tracking branches")
	}

	// Releasing the terminal Claim makes the proposal archivable even though
	// its dirty worktree stays preserved and a removal still fails.
	statusRecord(t, fixture.clone, "done", "claimed", func(state *ledger.SliceState) { state.Claim = nil })
	// The default Markdown transport, which Propose reads, keeps every
	// outcome visible and distinct.
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "propose", "cleanup", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	second := cli.out.String()
	for _, want := range []string{
		"Status: fix_required\n",
		"Archived proposal: done (retired without full delivery) at ",
		"Ledger replication: pushed\n",
		"Kept active proposal: active: active slices: pending (ready_for_merge)\n",
		"Removed local source work: done-claimed\n",
		"Preserved local source work: done-dirty: ",
		"Source removal failed: done-locked: worktree removal failed",
	} {
		if !strings.Contains(second, want) {
			t.Fatalf("Markdown cleanup lacks %q:\n%s", want, second)
		}
	}
	if !exists(t, fixture.clone, "projects/widgets/archive/done/locked/state.json") || !exists(t, root, ".worktrees/done-dirty/local.txt") {
		t.Fatal("archive or preserved source missing")
	}
	runGit(t, root, "worktree", "unlock", filepath.Join(root, ".worktrees", "done-locked"))
	archivedHead := ledgerHead(t, fixture.clone)
	third := runCleanup(t, cli, root)
	if third.Status != "completed" || len(third.Archive.Archived) != 0 || strings.Join(third.Source.Removed, " ") != "done-locked" || ledgerHead(t, fixture.clone) != archivedHead {
		t.Fatalf("retry after a source failure moved records again or missed the safe branch: %s", mustJSON(t, third))
	}
	if !exists(t, fixture.clone, "projects/widgets/proposals/active/pending/state.json") {
		t.Fatal("proposal with an active sibling left the proposals directory")
	}
}

func TestCompletionObservationDoesNotArchive(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("observed")))
	attachFixture(t, fixture.clone, "observed")
	app, output := completionStatusApp(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "cccccccccccccccccccccccccccccccccccccccc", cleanupMergeCommit, "acme/widgets", "acme/widgets", "main"))
	})
	if got := runCompletionStatus(t, app, output, root); got.Items[0].State != ledger.Merged || !got.Proposals[0].Retireable {
		t.Fatalf("merge not observed: %+v", got)
	}
	if !exists(t, fixture.clone, "projects/widgets/proposals/observed/foundation/state.json") || exists(t, fixture.clone, "projects/widgets/archive") {
		t.Fatal("completion observation archived the proposal without explicit cleanup")
	}
}

func TestCleanupDoesNotArchiveFromAStaleSnapshot(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", cleanupSpec("raced", "only")))
	statusRecord(t, fixture.clone, "raced", "only", merged(""))
	// Hold the ledger mutation lock so cleanup must wait for it, then record a
	// Claim a stale pre-lock read could not have seen.
	gitDirectory := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "--absolute-git-dir"))
	lock, err := os.OpenFile(filepath.Join(gitDirectory, "skl-ledger.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	cli := offlineForge(t)
	done := make(chan error, 1)
	go func() { done <- cli.app.Run([]string{"skl", "propose", "cleanup", "--repo", root, "--format", "json"}) }()
	time.Sleep(500 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("cleanup finished without waiting for the ledger lock: %v %s", err, cli.out.String())
	default:
	}
	statusRecord(t, fixture.clone, "raced", "only", func(state *ledger.SliceState) {
		state.Claim = &ledger.Claim{Phase: ledger.ImplementPhase, Basis: cleanupMergeCommit}
	})
	claimed := ledgerHead(t, fixture.clone)
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var outcome cleanupOutcome
	if err := json.Unmarshal(cli.out.Bytes(), &outcome); err != nil {
		t.Fatalf("cleanup response %q: %v", cli.out.String(), err)
	}
	if len(outcome.Archive.Archived) != 0 || len(outcome.Archive.Kept) != 1 || !strings.Contains(outcome.Archive.Kept[0].Reason, "claimed") || ledgerHead(t, fixture.clone) != claimed || exists(t, fixture.clone, "projects/widgets/archive") {
		t.Fatalf("cleanup archived from a stale snapshot: %s", mustJSON(t, outcome))
	}
}

func TestUnreadableRecordWithholdsSourceDeletion(t *testing.T) {
	// A damaged record's owner is unknowable, so it could own safe-only.
	for _, damage := range []struct {
		kind  string
		apply func(t *testing.T, clone, state string)
	}{
		{"malformed", func(t *testing.T, clone, state string) {
			writeFile(t, filepath.Join(clone, state), "{not json\n")
			runGit(t, clone, "commit", "-qam", "damage a record")
		}},
		{"missing", func(t *testing.T, clone, state string) {
			runGit(t, clone, "rm", "-q", state)
			runGit(t, clone, "commit", "-qm", "lose a record")
		}},
	} {
		t.Run(damage.kind, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			root := sourceRepository(t, "acme", "widgets")
			accepting := newLedgerApp(t, newForgeServer(t))
			for _, proposal := range []string{"safe", "damaged"} {
				accepting.accept(t, root, writeProposal(t, "", cleanupSpec(proposal, "only")))
			}
			cleanupWorktree(t, root, "safe-only")
			worktree := filepath.Join(root, ".worktrees", "safe-only")
			writeFile(t, filepath.Join(worktree, "README.md"), "committed work\n")
			runGit(t, worktree, "commit", "-qam", "slice change")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "safe-only"))
			statusRecord(t, fixture.clone, "safe", "only", merged(head))
			damage.apply(t, fixture.clone, "projects/widgets/proposals/damaged/only/state.json")

			outcome := runCleanup(t, offlineForge(t), root)
			if outcome.Source == nil || len(outcome.Source.Removed) != 0 || len(outcome.Source.Preserved) != 1 || !strings.Contains(outcome.Source.Preserved[0].Reason, "damaged/only") {
				t.Fatalf("damaged state did not withhold deletion: %s", mustJSON(t, outcome))
			}
			if !gitRefExists(root, "refs/heads/safe-only") || strings.TrimSpace(runGitOutput(t, root, "rev-parse", "safe-only")) != head ||
				readFileString(t, filepath.Join(worktree, "README.md")) != "committed work\n" {
				t.Fatal("source work removed or changed despite unknown ownership")
			}
			// Archival stays independent: the terminal proposal moves while the
			// damaged one is kept for its unknown state.
			if !archivedNames(outcome)["safe"] || len(outcome.Archive.Kept) != 1 || !strings.Contains(outcome.Archive.Kept[0].Reason, "unknown state") {
				t.Fatalf("archive outcomes were not independent: %s", mustJSON(t, outcome))
			}
		})
	}
}
