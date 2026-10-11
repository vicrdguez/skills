package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// amend runs skl ledger amend and returns the parsed JSON outcome.
func (c ledgerCLI) amend(t *testing.T, repo, directory string, flags ...string) ledgerOutcome {
	t.Helper()
	outcome, _ := c.run(t, append([]string{"skl", "ledger", "amend", "--repo", repo, "--proposal-dir", directory, "--format", "json"}, flags...))
	return outcome
}

// exportedIntake exports one accepted Proposal into a fresh intake directory
// named after it.
func exportedIntake(t *testing.T, cli ledgerCLI, root, proposal string) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), proposal)
	if outcome := cli.exportProposal(t, root, proposal, directory); outcome.Status != "exported" {
		t.Fatalf("export %s failed: %s", proposal, mustJSON(t, outcome))
	}
	return directory
}

// editIntake rewrites an intake's proposal.json.
func editIntake(t *testing.T, directory string, edit func(declaration map[string]any)) {
	t.Helper()
	declaration := exportedDeclaration(t, directory)
	edit(declaration)
	writeFile(t, filepath.Join(directory, "proposal.json"), mustJSON(t, declaration)+"\n")
}

// addIntakeSlice declares one new Slice with its Contract files, giving a
// single-slice intake the parent title a multi-slice one needs.
func addIntakeSlice(t *testing.T, directory, name string, depends ...string) {
	t.Helper()
	writeFile(t, filepath.Join(directory, name, "intent.md"), "# "+name+" intent\n")
	writeFile(t, filepath.Join(directory, name, "behavior.md"), "# "+name+" behavior\n")
	proposal := filepath.Base(directory)
	editIntake(t, directory, func(declaration map[string]any) {
		if declaration["parent_title"] == nil {
			declaration["parent_title"] = "Deliver " + proposal
		}
		entry := map[string]any{"name": name, "title": "Deliver " + proposal + " " + name, "branch": proposal + "-" + name}
		if len(depends) > 0 {
			entry["depends"] = depends
		}
		declaration["slices"] = append(declaration["slices"].([]any), entry)
	})
}

// setDepends replaces one intake Slice's declared Dependencies.
func setDepends(t *testing.T, directory, name string, depends ...string) {
	t.Helper()
	editIntake(t, directory, func(declaration map[string]any) {
		exportedSlice(t, declaration, name)["depends"] = depends
	})
}

func amendState(t *testing.T, clone, proposal, slice string) ledger.SliceState {
	t.Helper()
	return decisionState(t, clone, decisionItemStatePath("widgets", proposal, slice))
}

// amendCommits lists the subjects of the amendment commits after from.
func amendCommits(t *testing.T, clone, from string) []string {
	t.Helper()
	var subjects []string
	for _, subject := range strings.Split(strings.TrimSpace(runGitOutput(t, clone, "log", "--format=%s", from+"..HEAD")), "\n") {
		if strings.HasPrefix(subject, "amend ") {
			subjects = append(subjects, subject)
		}
	}
	return subjects
}

// requireRefused checks a refusal that names every fragment and left the
// ledger at head with a clean clone.
func requireRefused(t *testing.T, outcome ledgerOutcome, clone, head string, fragments ...string) {
	t.Helper()
	if outcome.Status != "fix_required" {
		t.Fatalf("amendment was not refused: %s", mustJSON(t, outcome))
	}
	for _, fragment := range fragments {
		if !strings.Contains(outcome.Reason+" "+outcome.Repair, fragment) {
			t.Fatalf("refusal lacks %q: %s", fragment, mustJSON(t, outcome))
		}
	}
	requireLedgerUnchanged(t, clone, head)
}

func requireAmended(t *testing.T, outcome ledgerOutcome) *ledger.Amendment {
	t.Helper()
	if outcome.Status != "amended" || outcome.Amendment == nil {
		t.Fatalf("amendment was not recorded: %s", mustJSON(t, outcome))
	}
	return outcome.Amendment
}

func amendedSlice(t *testing.T, amendment *ledger.Amendment, name string) ledger.SliceAmendment {
	t.Helper()
	for _, change := range amendment.Amended {
		if change.Name == name {
			return change
		}
	}
	t.Fatalf("amendment does not list %s: %s", name, mustJSON(t, amendment))
	return ledger.SliceAmendment{}
}

func acceptSpec(t *testing.T, cli ledgerCLI, root string, spec proposalSpec, flags ...string) {
	t.Helper()
	if outcome := cli.accept(t, root, writeProposal(t, "", spec), flags...); outcome.Status != "accepted" {
		t.Fatalf("accept %s failed: %s", spec.name, mustJSON(t, outcome))
	}
}

func TestAmendUnchangedIntakeReportsExisting(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	spec := cleanupSpec("steady", "alpha", "beta")
	spec.depends["beta"] = []string{"alpha"}
	acceptSpec(t, cli, root, spec)
	head := ledgerHead(t, fixture.clone)
	intake := exportedIntake(t, cli, root, "steady")

	outcome := cli.amend(t, root, intake)
	if outcome.Status != "existing" || outcome.Amendment == nil || len(outcome.Amendment.Amended) != 0 {
		t.Fatalf("an unchanged intake was not reported existing: %s", mustJSON(t, outcome))
	}
	requireLedgerUnchanged(t, fixture.clone, head)

	// A supplied body is still presented, as acceptance presents it.
	outcome = cli.amend(t, root, intake, issueFile(t, "alpha", "Alpha body\n"))
	if outcome.Status != "existing" || len(outcome.Amendment.Slices) != 1 || outcome.Amendment.Slices[0].Issue == nil || forge.createdCount() != 1 {
		t.Fatalf("an unchanged intake did not present its supplied body: %s", mustJSON(t, outcome))
	}
	if commits := amendCommits(t, fixture.clone, head); len(commits) != 0 {
		t.Fatalf("an unchanged intake committed an amendment: %v", commits)
	}
}

func TestAmendRefusesUnrecordedArchivedAndFinishedProposals(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	acceptSpec(t, cli, root, cleanupSpec("done-work", "only"))
	acceptSpec(t, cli, root, cleanupSpec("finished", "only"))
	statusRecord(t, fixture.clone, "finished", "only", merged(""))
	if outcome := runCleanup(t, offlineForge(t), root); len(archivedNames(outcome)) != 1 {
		t.Fatalf("setup did not archive finished: %s", mustJSON(t, outcome))
	}
	intake := exportedIntake(t, cli, root, "done-work")
	statusRecord(t, fixture.clone, "done-work", "only", merged(""))
	addIntakeSlice(t, intake, "more")
	head := ledgerHead(t, fixture.clone)

	requireRefused(t, cli.amend(t, root, writeProposal(t, "", cleanupSpec("fresh", "only"))), fixture.clone, head, "no accepted Proposal fresh", "skl ledger accept")
	requireRefused(t, cli.amend(t, root, writeProposal(t, "", cleanupSpec("finished", "only"))), fixture.clone, head, "archived")
	requireRefused(t, cli.amend(t, root, intake), fixture.clone, head, "finished", "new Proposal")
}

func TestAmendOneRefusedDifferenceStopsEverything(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	cli := newLedgerApp(t, newForgeServer(t))
	worker := deliveryNoForgeApp(t)
	acceptSpec(t, cli, source, cleanupSpec("pair", "first", "second"))
	if claimed := decisionSelect(t, worker, source); claimed.Execution.Item != "pair/first" {
		t.Fatalf("setup claimed %s, want pair/first", claimed.Execution.Item)
	}
	head := ledgerHead(t, fixture.clone)
	intake := exportedIntake(t, cli, source, "pair")
	addIntakeSlice(t, intake, "extra")
	setDepends(t, intake, "first", "second")

	requireRefused(t, cli.amend(t, source, intake), fixture.clone, head, "pair/first", "active implement Claim")
	if _, err := os.Stat(filepath.Join(fixture.clone, "projects", "widgets", "proposals", "pair", "extra")); !os.IsNotExist(err) {
		t.Fatalf("a refused amendment recorded the added slice: %v", err)
	}
}

func TestAmendReplacesNeverClaimedContract(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	spec := cleanupSpec("draft", "base", "work")
	spec.depends["work"] = []string{"base"}
	spec.slices[1].files["plan.md"] = "# Work plan\n"
	acceptSpec(t, cli, root, spec)
	head := ledgerHead(t, fixture.clone)

	// A changed planned branch keeps Project-wide uniqueness.
	clash := exportedIntake(t, cli, root, "draft")
	editIntake(t, clash, func(declaration map[string]any) { exportedSlice(t, declaration, "work")["branch"] = "proposal/draft" })
	requireRefused(t, cli.amend(t, root, clash), fixture.clone, head, "proposal/draft", "work")

	intake := exportedIntake(t, cli, root, "draft")
	writeFile(t, filepath.Join(intake, "work", "behavior.md"), "# Work behavior, revised\n")
	if err := os.Remove(filepath.Join(intake, "work", "plan.md")); err != nil {
		t.Fatal(err)
	}
	editIntake(t, intake, func(declaration map[string]any) {
		work := exportedSlice(t, declaration, "work")
		work["title"], work["branch"] = "Revised work", "draft-work-revised"
	})
	amendment := requireAmended(t, cli.amend(t, root, intake))
	change := amendedSlice(t, amendment, "work")
	if !equalStrings(change.Contract, []string{"behavior.md", "plan.md"}) || !change.Title || !change.Branch || change.Dependencies || len(amendment.Amended) != 1 {
		t.Fatalf("amendment reported the wrong changes: %s", mustJSON(t, amendment))
	}
	directory := "projects/widgets/proposals/draft/work/"
	if got := readLedgerFile(t, fixture.clone, directory+"behavior.md"); got != "# Work behavior, revised\n" {
		t.Fatalf("recorded behavior.md = %q", got)
	}
	if slices.Contains(ledgerPaths(t, fixture.clone), directory+"plan.md") {
		t.Fatal("the omitted plan.md is still recorded")
	}
	state := amendState(t, fixture.clone, "draft", "work")
	if state.State != ledger.ReadyForImplementation || state.Title != "Revised work" || state.Branch != "draft-work-revised" || !equalStrings(state.Dependencies, []string{"proposals/draft/base"}) {
		t.Fatalf("recorded state = %+v", state)
	}
	commits := amendCommits(t, fixture.clone, head)
	if len(commits) != 1 || commits[0] != "amend widgets/draft" {
		t.Fatalf("amendment commits = %v, want one", commits)
	}
	if parent := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD^"); parent != head {
		t.Fatalf("the amendment commit's parent is %s, want %s", parent, head)
	}
	if status := runGitOutput(t, fixture.clone, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Fatalf("the amendment left changes in the ledger clone:\n%s", status)
	}
}

func TestAmendReassignsBranchesWithinOneAmendment(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	acceptSpec(t, cli, root, cleanupSpec("archived", "base"))
	statusRecord(t, fixture.clone, "archived", "base", merged(""))
	if outcome := runCleanup(t, offlineForge(t), root); len(archivedNames(outcome)) != 1 {
		t.Fatalf("archive setup: %s", mustJSON(t, outcome))
	}
	acceptSpec(t, cli, root, cleanupSpec("other", "work"))
	acceptSpec(t, cli, root, cleanupSpec("transfer", "first", "second"))
	head := ledgerHead(t, fixture.clone)

	// Branches owned outside this Proposal stay owned.
	for _, taken := range []string{"other-work", "archived-base"} {
		clash := exportedIntake(t, cli, root, "transfer")
		editIntake(t, clash, func(declaration map[string]any) { exportedSlice(t, declaration, "second")["branch"] = taken })
		requireRefused(t, cli.amend(t, root, clash), fixture.clone, head, taken, "transfer/second")
	}

	// A branch the amendment moves away from is free for another of its Slices.
	intake := exportedIntake(t, cli, root, "transfer")
	editIntake(t, intake, func(declaration map[string]any) {
		exportedSlice(t, declaration, "first")["branch"] = "transfer-first-new"
		exportedSlice(t, declaration, "second")["branch"] = "transfer-first"
	})
	requireAmended(t, cli.amend(t, root, intake))
	if first, second := amendState(t, fixture.clone, "transfer", "first").Branch, amendState(t, fixture.clone, "transfer", "second").Branch; first != "transfer-first-new" || second != "transfer-first" {
		t.Fatalf("recorded branches = %s, %s", first, second)
	}
	if commits := amendCommits(t, fixture.clone, head); len(commits) != 1 {
		t.Fatalf("amendment commits = %v, want one", commits)
	}
}

func TestAmendRefusesContractChangeOnceClaimed(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	cli := newLedgerApp(t, newForgeServer(t))
	worker := deliveryNoForgeApp(t)
	acceptSpec(t, cli, source, cleanupSpec("gate", "paused", "released"))
	if item, _ := decisionPauseNext(t, worker, source); item != "gate/paused" {
		t.Fatalf("setup paused %s, want gate/paused", item)
	}
	released := decisionSelect(t, worker, source)
	if output, err := worker.deliveryRun(t, "skl", "implement", "release", "--repo", source, "--item", released.Execution.Item, "--claim", released.Execution.Claim.Commit); err != nil {
		t.Fatalf("release %s: %v\n%s", released.Execution.Item, err, output)
	}
	head := ledgerHead(t, fixture.clone)
	recorded := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/gate/paused/intent.md")

	paused := exportedIntake(t, cli, source, "gate")
	writeFile(t, filepath.Join(paused, "paused", "intent.md"), "# Paused intent, revised\n")
	requireRefused(t, cli.amend(t, source, paused), fixture.clone, head, "gate/paused", "claimed before")
	if got := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/gate/paused/intent.md"); got != recorded {
		t.Fatalf("the refused amendment changed the recorded Contract: %q", got)
	}

	intake := exportedIntake(t, cli, source, "gate")
	writeFile(t, filepath.Join(intake, "released", "behavior.md"), "# Released behavior, revised\n")
	requireAmended(t, cli.amend(t, source, intake))
	if got := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/gate/released/behavior.md"); got != "# Released behavior, revised\n" {
		t.Fatalf("the released slice's Contract was not replaced: %q", got)
	}
}

func TestAmendInsertsBlockerBeforePausedSlice(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	cli := newLedgerApp(t, newForgeServer(t))
	worker := deliveryNoForgeApp(t)
	acceptSpec(t, cli, source, cleanupSpec("checks", "check"))
	decisionPauseNext(t, worker, source)

	intake := exportedIntake(t, cli, source, "checks")
	addIntakeSlice(t, intake, "cleanup")
	setDepends(t, intake, "check", "cleanup")
	amendment := requireAmended(t, cli.amend(t, source, intake))
	if !amendedSlice(t, amendment, "check").Dependencies || !amendedSlice(t, amendment, "cleanup").Added {
		t.Fatalf("amendment reported the wrong changes: %s", mustJSON(t, amendment))
	}
	state := amendState(t, fixture.clone, "checks", "check")
	if state.State != ledger.NeedsHuman || !equalStrings(state.Dependencies, []string{"proposals/checks/cleanup"}) {
		t.Fatalf("check records %+v", state)
	}

	decisionApplyAnswer(t, worker, decisionRequest(t, worker, "widgets", "checks/check"), ledger.RouteImplement, "# Answer\n\nContinue.\n")
	selected := decisionSelect(t, worker, source)
	if selected.Execution.Item != "checks/cleanup" {
		t.Fatalf("selection took %s before its new blocker merged", selected.Execution.Item)
	}
	statusRecord(t, fixture.clone, "checks", "cleanup", func(state *ledger.SliceState) {
		merged("")(state)
		state.Claim = nil
	})
	if selected := decisionSelect(t, worker, source); selected.Execution.Item != "checks/check" {
		t.Fatalf("selection took %s once the blocker merged, want checks/check", selected.Execution.Item)
	}
}

func TestAmendChangesDependenciesOnUnclaimedSlicesOnly(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	acceptSpec(t, cli, root, cleanupSpec("states", "base", "done", "merge", "paused", "review", "rework"))
	for slice, state := range map[string]string{"rework": ledger.Rework, "paused": ledger.NeedsHuman, "review": ledger.AwaitingReview, "merge": ledger.ReadyForMerge} {
		statusRecord(t, fixture.clone, "states", slice, lifecycle(state))
	}
	statusRecord(t, fixture.clone, "states", "done", merged(""))
	head := ledgerHead(t, fixture.clone)

	refused := exportedIntake(t, cli, root, "states")
	setDepends(t, refused, "done", "base")
	requireRefused(t, cli.amend(t, root, refused), fixture.clone, head, "states/done", "merged")

	intake := exportedIntake(t, cli, root, "states")
	for _, slice := range []string{"rework", "paused", "review", "merge"} {
		setDepends(t, intake, slice, "base")
	}
	amendment := requireAmended(t, cli.amend(t, root, intake))
	for slice, want := range map[string]string{"rework": ledger.Rework, "paused": ledger.NeedsHuman, "review": ledger.AwaitingReview, "merge": ledger.ReadyForMerge} {
		state := amendState(t, fixture.clone, "states", slice)
		if state.State != want || !equalStrings(state.Dependencies, []string{"proposals/states/base"}) || !amendedSlice(t, amendment, slice).Dependencies {
			t.Fatalf("%s records %+v", slice, state)
		}
	}
}

func TestAmendAddsCleanupSliceBehindDeliveredSlice(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	spec := cleanupSpec("minos", "check", "outputs")
	spec.depends["outputs"] = []string{"check"}
	parentBody := writeTemp(t, t, "Parent body\n")
	acceptSpec(t, cli, root, spec, issueFile(t, "check", "Check body\n"), issueFile(t, "outputs", "Outputs body\n"), "--parent-body", parentBody)
	statusRecord(t, fixture.clone, "minos", "check", func(state *ledger.SliceState) {
		state.State, state.Submission = ledger.AwaitingReview, &ledger.ForgeAttachment{Repository: "acme/widgets", Number: 21}
	})
	checkPath := "projects/widgets/proposals/minos/check/state.json"
	checkBefore := readLedgerFile(t, fixture.clone, checkPath)
	outputsIssue := amendState(t, fixture.clone, "minos", "outputs").Issue.Number
	created := forge.createdCount()
	head := ledgerHead(t, fixture.clone)

	intake := exportedIntake(t, cli, root, "minos")
	addIntakeSlice(t, intake, "cleanup", "check")
	setDepends(t, intake, "outputs", "cleanup")
	amendment := requireAmended(t, cli.amend(t, root, intake, issueFile(t, "cleanup", "Cleanup body\n"), "--parent-body", writeTemp(t, t, "Amended parent body\n")))

	cleanup := amendState(t, fixture.clone, "minos", "cleanup")
	if cleanup.State != ledger.ReadyForImplementation || cleanup.Title != "Deliver minos cleanup" || cleanup.Branch != "minos-cleanup" || cleanup.Target != nil ||
		!equalStrings(cleanup.Dependencies, []string{"proposals/minos/check"}) {
		t.Fatalf("cleanup records %+v", cleanup)
	}
	if got := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/minos/cleanup/behavior.md"); got != "# cleanup behavior\n" {
		t.Fatalf("cleanup's Contract was not frozen: %q", got)
	}
	if outputs := amendState(t, fixture.clone, "minos", "outputs"); !equalStrings(outputs.Dependencies, []string{"proposals/minos/cleanup"}) {
		t.Fatalf("outputs records dependencies %v", outputs.Dependencies)
	}
	if after := readLedgerFile(t, fixture.clone, checkPath); after != checkBefore {
		t.Fatalf("check's record changed:\n%s", after)
	}
	if commits := amendCommits(t, fixture.clone, head); len(commits) != 1 {
		t.Fatalf("amendment commits = %v, want one", commits)
	}
	if meta := decodeRecord(t, readLedgerFile(t, fixture.clone, "projects/widgets/proposals/minos/proposal.json")); meta["accepted"] == nil {
		t.Fatalf("the Proposal lost its acceptance time: %v", meta)
	}

	// Publication: cleanup's issue is created under the parent, the parent
	// carries its new body, and outputs is named as not presented.
	if forge.createdCount() != created+1 {
		t.Fatalf("the amendment created %d issues, want only cleanup's: %s", forge.createdCount()-created, mustJSON(t, amendment))
	}
	cleanupIssue := amendState(t, fixture.clone, "minos", "cleanup").Issue
	parent := amendment.ParentIssue
	if cleanupIssue == nil || parent == nil || !containsNumber(forge.children[parent.Number], cleanupIssue.Number) {
		t.Fatalf("cleanup's issue is not grouped under the parent: %s", mustJSON(t, amendment))
	}
	if updated := forge.updatedIssues(); !slices.Equal(updated, []int{parent.Number}) || containsNumber(updated, outputsIssue) {
		t.Fatalf("updated issues = %v, want only the parent %d", updated, parent.Number)
	}
	if body := forge.list[slices.IndexFunc(forge.list, func(issue map[string]any) bool { return issue["number"] == parent.Number })]["body"]; body != "Amended parent body\n" {
		t.Fatalf("the parent issue carries %q, want the supplied body", body)
	}
	if !equalStrings(amendment.Unpresented, []string{"outputs"}) {
		t.Fatalf("unpresented = %v, want outputs", amendment.Unpresented)
	}
}

func TestAmendListsUntouchedDependents(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	spec := cleanupSpec("deps", "a", "b", "c", "d")
	spec.depends = map[string][]string{"b": {"a"}, "c": {"a"}, "d": {"a"}}
	acceptSpec(t, cli, root, spec)
	statusRecord(t, fixture.clone, "deps", "d", merged(""))

	intake := exportedIntake(t, cli, root, "deps")
	writeFile(t, filepath.Join(intake, "a", "behavior.md"), "# a behavior, revised\n")
	amendment := requireAmended(t, cli.amend(t, root, intake))
	if dependents := amendedSlice(t, amendment, "a").Dependents; !equalStrings(dependents, []string{"deps/b", "deps/c"}) {
		t.Fatalf("dependents of a = %v, want deps/b and deps/c", dependents)
	}
}

func TestAmendedSliceKeepsProposalPlaceInSelection(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	cli := newLedgerApp(t, newForgeServer(t))
	worker := deliveryNoForgeApp(t)
	acceptSpec(t, cli, source, cleanupSpec("older", "first"))
	acceptSpec(t, cli, source, cleanupSpec("newer", "alpha"))
	metaPath := "projects/widgets/proposals/older/proposal.json"
	meta := decodeRecord(t, readLedgerFile(t, fixture.clone, metaPath))
	meta["accepted"] = "2020-01-01T00:00:00Z"
	commitLedgerRecord(t, fixture.clone, metaPath, mustJSON(t, meta)+"\n")
	statusRecord(t, fixture.clone, "older", "first", lifecycle(ledger.NeedsHuman))

	intake := exportedIntake(t, cli, source, "older")
	addIntakeSlice(t, intake, "extra")
	requireAmended(t, cli.amend(t, source, intake))
	if accepted := decodeRecord(t, readLedgerFile(t, fixture.clone, metaPath))["accepted"]; accepted != "2020-01-01T00:00:00Z" {
		t.Fatalf("the amendment changed the acceptance time to %v", accepted)
	}
	if selected := decisionSelect(t, worker, source); selected.Execution.Item != "older/extra" {
		t.Fatalf("selection took %s, want the added older/extra", selected.Execution.Item)
	}
}

func TestAmendMembershipChangesAreExplicit(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	acceptSpec(t, cli, root, cleanupSpec("members", "keep", "drop", "old"))
	statusRecord(t, fixture.clone, "members", "old", lifecycle(ledger.Superseded))
	head := ledgerHead(t, fixture.clone)

	missing := exportedIntake(t, cli, root, "members")
	if err := os.RemoveAll(filepath.Join(missing, "drop")); err != nil {
		t.Fatal(err)
	}
	editIntake(t, missing, func(declaration map[string]any) {
		declaration["slices"] = slices.DeleteFunc(declaration["slices"].([]any), func(entry any) bool { return entry.(map[string]any)["name"] == "drop" })
	})
	requireRefused(t, cli.amend(t, root, missing), fixture.clone, head, "members/drop", "restore its entry")

	flagged := exportedIntake(t, cli, root, "members")
	editIntake(t, flagged, func(declaration map[string]any) { exportedSlice(t, declaration, "keep")["superseded"] = true })
	requireRefused(t, cli.amend(t, root, flagged), fixture.clone, head, "members/keep", "Decision route `supersede`")

	unflagged := exportedIntake(t, cli, root, "members")
	editIntake(t, unflagged, func(declaration map[string]any) { delete(exportedSlice(t, declaration, "old"), "superseded") })
	requireRefused(t, cli.amend(t, root, unflagged), fixture.clone, head, "members/old", "restore \"superseded\": true")
}

func TestAmendKeepsTheGraphConsistent(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	spec := cleanupSpec("graph", "live", "old", "stale")
	spec.depends["stale"] = []string{"old"}
	acceptSpec(t, cli, root, spec)
	acceptSpec(t, cli, root, cleanupSpec("base-work", "a"))
	other := cleanupSpec("other", "b")
	other.depends["b"] = []string{"proposals/base-work/a"}
	acceptSpec(t, cli, root, other)
	statusRecord(t, fixture.clone, "graph", "old", lifecycle(ledger.Superseded))
	head := ledgerHead(t, fixture.clone)

	superseded := exportedIntake(t, cli, root, "graph")
	setDepends(t, superseded, "live", "old")
	requireRefused(t, cli.amend(t, root, superseded), fixture.clone, head, "dependency proposals/graph/old of slice graph/live", "Superseded")

	unrecorded := exportedIntake(t, cli, root, "graph")
	setDepends(t, unrecorded, "live", "proposals/other/ghost")
	requireRefused(t, cli.amend(t, root, unrecorded), fixture.clone, head, "dependency proposals/other/ghost of slice graph/live", "no recorded Slice")

	cycle := exportedIntake(t, cli, root, "base-work")
	setDepends(t, cycle, "a", "proposals/other/b")
	requireRefused(t, cli.amend(t, root, cycle), fixture.clone, head, "cycle", "base-work/a", "other/b")

	// An untouched edge to a Superseded Slice is not re-examined.
	retitled := exportedIntake(t, cli, root, "graph")
	editIntake(t, retitled, func(declaration map[string]any) { exportedSlice(t, declaration, "stale")["title"] = "Stale, retitled" })
	requireAmended(t, cli.amend(t, root, retitled))
}

func TestAmendDescriptionWhileWorkIsClaimed(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	cli := newLedgerApp(t, newForgeServer(t))
	acceptSpec(t, cli, source, cleanupSpec("described", "work"))
	decisionSelect(t, deliveryNoForgeApp(t), source)
	statePath := "projects/widgets/proposals/described/work/state.json"
	before := readLedgerFile(t, fixture.clone, statePath)

	intake := exportedIntake(t, cli, source, "described")
	writeFile(t, filepath.Join(intake, "proposal.md"), "# described\n\nA revised description.\n")
	if amendment := requireAmended(t, cli.amend(t, source, intake)); !amendment.DescriptionReplaced || len(amendment.Amended) != 0 {
		t.Fatalf("amendment reported the wrong changes: %s", mustJSON(t, amendment))
	}
	if got := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/described/proposal.md"); got != "# described\n\nA revised description.\n" {
		t.Fatalf("recorded proposal.md = %q", got)
	}
	if after := readLedgerFile(t, fixture.clone, statePath); after != before {
		t.Fatalf("the claimed slice's record changed:\n%s", after)
	}
}

func TestAmendGrowsSingleSliceProposal(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	acceptSpec(t, cli, root, cleanupSpec("grow", "first"), issueFile(t, "first", "First body\n"))
	first := amendState(t, fixture.clone, "grow", "first").Issue

	intake := exportedIntake(t, cli, root, "grow")
	addIntakeSlice(t, intake, "second", "first")
	amendment := requireAmended(t, cli.amend(t, root, intake, issueFile(t, "second", "Second body\n"), "--parent-body", writeTemp(t, t, "Parent body\n")))
	if !amendment.ParentTitleChanged || amendment.ParentIssue == nil || amendment.ParentNote == nil || amendment.ParentNote.Status != ledger.IssueCreated {
		t.Fatalf("the parent issue was not created: %s", mustJSON(t, amendment))
	}
	meta := decodeRecord(t, readLedgerFile(t, fixture.clone, "projects/widgets/proposals/grow/proposal.json"))
	if meta["parent_title"] != "Deliver grow" || meta["parent_issue"] == nil {
		t.Fatalf("proposal.json records %v", meta)
	}
	second := amendState(t, fixture.clone, "grow", "second").Issue
	children := forge.children[amendment.ParentIssue.Number]
	if first == nil || second == nil || !containsNumber(children, first.Number) || !containsNumber(children, second.Number) {
		t.Fatalf("parent %d groups %v, want both slices' issues", amendment.ParentIssue.Number, children)
	}
}

func TestAmendKeepsTheProposalBranch(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	acceptSpec(t, cli, root, cleanupSpec("branched", "only"))
	acceptSpec(t, cli, root, cleanupSpec("historical", "only"))
	metaPath := "projects/widgets/proposals/historical/proposal.json"
	meta := decodeRecord(t, readLedgerFile(t, fixture.clone, metaPath))
	delete(meta, "branch")
	commitLedgerRecord(t, fixture.clone, metaPath, mustJSON(t, meta)+"\n")
	head := ledgerHead(t, fixture.clone)

	moved := exportedIntake(t, cli, root, "branched")
	editIntake(t, moved, func(declaration map[string]any) { declaration["branch"] = "proposal/elsewhere" })
	requireRefused(t, cli.amend(t, root, moved), fixture.clone, head, "proposal/branched", "proposal/elsewhere")

	// A Proposal recorded without a Proposal Branch neither compares nor
	// records the defaulted name.
	spec := cleanupSpec("historical", "only")
	spec.description = "# historical\n\nRevised.\n"
	requireAmended(t, cli.amend(t, root, writeProposal(t, "", spec)))
	if _, recorded := decodeRecord(t, readLedgerFile(t, fixture.clone, metaPath))["branch"]; recorded {
		t.Fatal("the amendment recorded a Proposal Branch for a historical Proposal")
	}
}
