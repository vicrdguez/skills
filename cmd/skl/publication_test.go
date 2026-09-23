package main

// Public-CLI regression coverage for explicit publication inspection and
// recovery. Every observable outcome is taken from the accepted recovery
// behavior (B1-B8, A1-A5) and exercised through `skl publication` against real
// local source/ledger Git and the controlled HTTP forge.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// publicationState reads one committed state.json record.
func publicationState(t *testing.T, clone, proposal, slice string) ledger.SliceState {
	t.Helper()
	path := "projects/widgets/proposals/" + proposal + "/" + slice + "/state.json"
	raw := runGitOutput(t, clone, "show", "HEAD:"+path)
	var state ledger.SliceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("decode committed state %q: %v", raw, err)
	}
	return state
}

// publicationProposalState reads one committed proposal.json record.
func publicationProposalState(t *testing.T, clone, proposal string) ledger.ProposalMeta {
	t.Helper()
	path := "projects/widgets/proposals/" + proposal + "/proposal.json"
	raw := runGitOutput(t, clone, "show", "HEAD:"+path)
	var meta ledger.ProposalMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("decode committed proposal %q: %v", raw, err)
	}
	return meta
}

// publicationNoForgeApp is the read-only surface: constructing a forge is a
// failure, so any inspection that touches it fails visibly.
func publicationNoForgeApp(t *testing.T, calls *int) ledgerCLI {
	t.Helper()
	var output bytes.Buffer
	factory := func(github.RepositoryID) (setup.Backend, error) {
		if calls != nil {
			*calls++
		}
		return nil, errors.New("no forge is available for inspection")
	}
	return ledgerCLI{app: newApp(factory, bytes.NewReader(nil), &output, &output), out: &output}
}

// TestPublicationCLIInspectIsReadOnly proves inspection renders each owner's
// specialized packet from the selected view alone: no forge is constructed, no
// source or workflow workspace is prepared, and the ledger is not mutated.
func TestPublicationCLIInspectIsReadOnly(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newPublicationForge(t)
	source := sourceRepository(t, "acme", "widgets")
	accept := newPublicationApp(t, forge)
	directory, flags, _ := publicationProposal(t, singleSlice("inspect-proposal"), map[string]string{"foundation": "foundation issue body\n"})
	accepted := accept.accept(t, source, directory, flags...)
	if accepted.Status != "accepted" {
		t.Fatalf("acceptance = %q, want accepted: %s", accepted.Status, mustJSON(t, accepted))
	}

	// Satisfied: a recorded attachment already identifies the presentation.
	var forgeCalls int
	cli := publicationNoForgeApp(t, &forgeCalls)
	before := ledgerSnapshot(t, fixture.clone)
	satisfied, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "inspect-proposal/foundation", "--kind", "issue", "--format", "json")
	if err != nil {
		t.Fatalf("inspect satisfied issue: %v", err)
	}
	if satisfied.Status != "published" || satisfied.Packet == nil || satisfied.Packet.Facts.Publication == nil {
		t.Fatalf("satisfied inspect = %#v, want a published packet", satisfied)
	}
	facts := satisfied.Packet.Facts.Publication
	if facts.Condition != skilldist.PublicationSatisfied || facts.Operation != "inspect" || facts.Kind != "issue" {
		t.Fatalf("satisfied facts = %#v", facts)
	}
	if satisfied.Packet.Skill != "propose" || len(facts.ReferenceCommands) == 0 || facts.ResultDirectory != "" {
		t.Fatalf("satisfied packet = %#v", satisfied.Packet)
	}
	if forgeCalls != 0 {
		t.Fatalf("inspection constructed a forge %d times", forgeCalls)
	}
	if after := ledgerSnapshot(t, fixture.clone); after != before {
		t.Fatal("inspection mutated the ledger")
	}
	markdown, err := cli.publicationRun(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "inspect-proposal/foundation", "--kind", "issue")
	if err != nil {
		t.Fatalf("markdown inspect: %v", err)
	}
	if !strings.Contains(markdown, "Publication inspection") || !strings.Contains(markdown, "selected presentation is already current") {
		t.Fatalf("markdown inspect = %q", markdown)
	}
	if strings.Contains(markdown, "Retrieve the deferred authoring guidance") {
		t.Fatalf("satisfied inspection asked for new prose:\n%s", markdown)
	}

	// Prose needed: the registered temporary bytes are gone, so inspection
	// must bind the authoring resource and a concrete continuation instead of
	// reporting a false satisfied result.
	forge.setFail(func(method, path string) int {
		if method == "POST" && path == "/repos/acme/widgets/issues" {
			return 500
		}
		return 0
	})
	second, secondFlags, _ := publicationProposal(t, singleSlice("lost-prose"), map[string]string{"foundation": "descriptive issue of lost-prose\n"})
	if outcome := accept.accept(t, source, second, secondFlags...); outcome.Status != "accepted" {
		t.Fatalf("second acceptance = %q", outcome.Status)
	}
	forge.setFail(nil)
	state := publicationState(t, fixture.clone, "lost-prose", "foundation")
	if state.Publication == nil || state.Publication.IssueBody == nil {
		t.Fatalf("acceptance did not register a temporary issue body: %#v", state.Publication)
	}
	if !filepath.IsAbs(state.Publication.IssueBody.Path) {
		t.Fatalf("the registered temporary body path %q is not absolute", state.Publication.IssueBody.Path)
	}
	if err := os.Remove(state.Publication.IssueBody.Path); err != nil {
		t.Fatal(err)
	}

	resultDir := t.TempDir()
	needed, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "lost-prose/foundation", "--kind", "issue", "--result-directory", resultDir, "--format", "json")
	if err != nil {
		t.Fatalf("inspect prose-needed issue: %v", err)
	}
	neededFacts := needed.Packet.Facts.Publication
	if neededFacts.Condition != skilldist.PublicationProseNeeded {
		t.Fatalf("prose-needed condition = %q", neededFacts.Condition)
	}
	if neededFacts.ResultDirectory != resultDir {
		t.Fatalf("prose-needed result directory = %q, want the bound %q", neededFacts.ResultDirectory, resultDir)
	}
	if info, err := os.Stat(neededFacts.ResultDirectory); err != nil || !info.IsDir() {
		t.Fatalf("prose-needed result directory is not an existing directory: %v", err)
	}
	// An unusable bound directory is refused before any continuation.
	relative, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "lost-prose/foundation", "--kind", "issue", "--result-directory", "relative/path", "--format", "json")
	if err != nil {
		t.Fatalf("relative result-directory: %v", err)
	}
	if relative.Status != "fix_required" || !strings.Contains(relative.Reason, "absolute") {
		t.Fatalf("relative result-directory = %#v", relative)
	}
	missingDir, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "lost-prose/foundation", "--kind", "issue", "--result-directory", filepath.Join(t.TempDir(), "absent"), "--format", "json")
	if err != nil {
		t.Fatalf("missing result-directory: %v", err)
	}
	if missingDir.Status != "fix_required" || !strings.Contains(missingDir.Reason, "existing directory") {
		t.Fatalf("missing result-directory = %#v", missingDir)
	}
	if !strings.Contains(neededFacts.ResourceCommand, "reference/publication.md") || !strings.HasSuffix(neededFacts.ResourceCommand, " propose") {
		t.Fatalf("prose-needed resource command = %q", neededFacts.ResourceCommand)
	}
	if !strings.Contains(neededFacts.RecoverCommand, filepath.Join(neededFacts.ResultDirectory, "public.md")) {
		t.Fatalf("prose-needed continuation lost the public body path: %q", neededFacts.RecoverCommand)
	}
	if len(neededFacts.ReferenceCommands) < 2 {
		t.Fatalf("prose-needed references = %#v, want the frozen Contract references", neededFacts.ReferenceCommands)
	}
	for _, reference := range neededFacts.ReferenceCommands {
		if reference.Commit == "" || reference.Path == "" || !strings.Contains(reference.Command, "skl ledger show") {
			t.Fatalf("inexact private reference: %#v", reference)
		}
	}
	neededMarkdown, err := cli.publicationRun(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "lost-prose/foundation", "--kind", "issue", "--result-directory", resultDir)
	if err != nil {
		t.Fatalf("markdown prose-needed inspect: %v", err)
	}
	for _, want := range []string{"No applicable temporary public body remains", neededFacts.ResourceCommand, neededFacts.RecoverCommand, "Manual Verification"} {
		if !strings.Contains(neededMarkdown, want) {
			t.Errorf("prose-needed markdown is missing %q:\n%s", want, neededMarkdown)
		}
	}
	if strings.Contains(neededMarkdown, "Reuse the registered current body") {
		t.Fatalf("prose-needed inspection told the agent to reuse missing bytes:\n%s", neededMarkdown)
	}
	if forgeCalls != 0 {
		t.Fatalf("prose-needed inspection constructed a forge %d times", forgeCalls)
	}
}

// TestPublicationCLIRefusalsAreActionable proves unusable configuration,
// repository, overlap, and selection inputs become actionable outputs instead
// of a fabricated view.
func TestPublicationCLIRefusalsAreActionable(t *testing.T) {
	fixture := newLedgerFixture(t)
	source := sourceRepository(t, "acme", "widgets")
	cli := publicationNoForgeApp(t, nil)

	missing, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source,
		"--item", "x", "--kind", "issue", "--format", "json")
	if err == nil && missing.Status != "fix_required" {
		t.Fatalf("unknown Work Item = %#v, want fix_required", missing)
	}

	noSelection, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("missing selection select: %v", err)
	}
	if noSelection.Status != "fix_required" || !strings.Contains(noSelection.Reason, "--item") {
		t.Fatalf("missing selection = %#v", noSelection)
	}
	noKind, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source, "--item", "x/y", "--format", "json")
	if err != nil {
		t.Fatalf("missing kind select: %v", err)
	}
	if noKind.Status != "fix_required" || !strings.Contains(noKind.Reason, "--kind") {
		t.Fatalf("missing kind = %#v", noKind)
	}

	// Configuration missing: the repair names the exact config location.
	configPath := filepath.Join(fixture.config, "skl", "config.json")
	if err := os.Remove(configPath); err != nil {
		t.Fatal(err)
	}
	unconfigured, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source, "--item", "x/y", "--kind", "issue", "--format", "json")
	if err != nil {
		t.Fatalf("unconfigured inspect: %v", err)
	}
	if unconfigured.Status != "fix_required" || !strings.Contains(unconfigured.Repair, "config.json") {
		t.Fatalf("unconfigured inspect = %#v", unconfigured)
	}

	// Source overlap: the ledger may not share storage with the source checkout.
	writeFile(t, configPath, `{"ledger": "`+source+`"}`+"\n")
	overlap, err := cli.publicationJSON(t, "skl", "publication", "inspect", "--repo", source, "--item", "x/y", "--kind", "issue", "--format", "json")
	if err != nil {
		t.Fatalf("overlap inspect: %v", err)
	}
	if overlap.Status != "fix_required" || overlap.Repair == "" {
		t.Fatalf("overlap inspect = %#v", overlap)
	}
}

// TestPublicationCLIRecoversAcceptanceParentAndChildren proves B1/A2: one
// existing child, one unresolved child, and an unresolved parent recover
// through the public CLI without reacceptance, a new Claim, or source work,
// while the frozen Contracts and planned branches stay unchanged.
func TestPublicationCLIRecoversAcceptanceParentAndChildren(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newPublicationForge(t)
	source := sourceRepository(t, "acme", "widgets")

	// The two failing creates take effect without a response and their
	// resolution listing is unavailable, so acceptance leaves them honestly
	// unresolved instead of guessing.
	creates := 0
	forge.setDrop(func(method, path string) bool {
		if method == "POST" && path == "/repos/acme/widgets/issues" {
			creates++
			return creates >= 2
		}
		return false
	})
	forge.setFail(func(method, path string) int {
		if method == "GET" && path == "/repos/acme/widgets/issues" {
			return 503
		}
		return 0
	})

	accept := newPublicationApp(t, forge)
	directory, flags, parentBody := publicationProposal(t, dualSlice("grouped"), map[string]string{
		"foundation": "foundation descriptive issue\n",
		"feature":    "feature descriptive issue\n",
		"parent":     "grouped parent issue\n",
	})
	accepted := accept.accept(t, source, directory, append(flags, "--parent-body", parentBody)...)
	if accepted.Status != "accepted" {
		t.Fatalf("acceptance = %q: %s", accepted.Status, mustJSON(t, accepted))
	}
	forge.setDrop(nil)
	forge.setFail(nil)

	contractBefore := map[string]string{}
	contractDirectory := filepath.Join(fixture.clone, "projects", "widgets", "proposals", "grouped")
	for _, name := range []string{"foundation/behavior.md", "foundation/intent.md", "feature/behavior.md", "feature/intent.md"} {
		contractBefore[name] = readFileString(t, filepath.Join(contractDirectory, filepath.FromSlash(name)))
	}
	foundationBefore := publicationState(t, fixture.clone, "grouped", "foundation")
	featureBefore := publicationState(t, fixture.clone, "grouped", "feature")
	metaBefore := publicationProposalState(t, fixture.clone, "grouped")
	if foundationBefore.Issue == nil {
		t.Fatalf("foundation issue was not attached during acceptance: %#v", foundationBefore.Publication)
	}
	if featureBefore.Publication == nil || featureBefore.Publication.Issue == nil || featureBefore.Publication.Issue.Status != ledger.IssueUnresolved {
		t.Fatalf("feature issue = %#v, want an unresolved attempt", featureBefore.Publication)
	}
	if metaBefore.ParentPublication == nil || metaBefore.ParentPublication.Status != ledger.IssueUnresolved {
		t.Fatalf("parent attempt = %#v, want unresolved", metaBefore.ParentPublication)
	}
	if metaBefore.ParentBody == nil || !filepath.IsAbs(metaBefore.ParentBody.Path) {
		t.Fatalf("the parent temporary body was not registered with an absolute path: %#v", metaBefore.ParentBody)
	}
	created := forge.issueCount()

	cli := newPublicationApp(t, forge)
	feature, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", "grouped/feature", "--kind", "issue", "--format", "json")
	if err != nil {
		t.Fatalf("recover feature issue: %v", err)
	}
	if (feature.Status != "published" && feature.Status != "already-satisfied") || feature.Packet == nil {
		t.Fatalf("feature recovery = %#v, want a satisfied adoption", feature)
	}
	featureAfter := publicationState(t, fixture.clone, "grouped", "feature")
	if featureAfter.Issue == nil {
		t.Fatal("feature recovery did not adopt the observed issue")
	}
	adoptedFeature := featureAfter.Issue.Number
	if observed := forge.issueNumberByTitle("Add feature"); observed == 0 || adoptedFeature != observed {
		t.Fatalf("feature recovery adopted #%d instead of the observed #%d", adoptedFeature, observed)
	}

	parent, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", "grouped/foundation", "--kind", "parent", "--format", "json")
	if err != nil {
		t.Fatalf("recover parent: %v", err)
	}
	if parent.Status != "published" {
		t.Fatalf("parent recovery = %#v, want published", parent)
	}
	if forge.issueCount() != created {
		t.Fatalf("recovery created %d issues, want the %d already recorded", forge.issueCount()-created, created)
	}
	metaAfter := publicationProposalState(t, fixture.clone, "grouped")
	if metaAfter.ParentIssue == nil {
		t.Fatal("parent recovery did not adopt the observed parent issue")
	}
	for _, child := range []int{foundationBefore.Issue.Number, adoptedFeature} {
		if !containsNumber(forge.links[metaAfter.ParentIssue.Number], child) {
			t.Fatalf("child #%d is not grouped under parent #%d: %#v", child, metaAfter.ParentIssue.Number, forge.links)
		}
	}
	if metaAfter.ParentPublication != nil || metaAfter.ParentBody != nil {
		t.Fatalf("satisfied parent left pending records: %#v", metaAfter)
	}
	foundationAfter := publicationState(t, fixture.clone, "grouped", "foundation")
	if foundationAfter.Issue == nil || foundationAfter.Issue.Number != foundationBefore.Issue.Number {
		t.Fatalf("existing foundation issue was replaced: %#v", foundationAfter.Issue)
	}

	// Recovery is presentation only: no Claim, no source branch, no changed
	// Contract, and the parent's declared branch remains only declarative.
	state := publicationState(t, fixture.clone, "grouped", "foundation")
	if state.Claim != nil {
		t.Fatalf("recovery acquired a Claim: %#v", state.Claim)
	}
	for _, branch := range []string{"foundation", "feature"} {
		if gitRefExists(source, "refs/heads/"+branch) {
			t.Fatalf("recovery prepared source branch %q", branch)
		}
	}
	for name, before := range contractBefore {
		if after := readFileString(t, filepath.Join(contractDirectory, filepath.FromSlash(name))); after != before {
			t.Fatalf("frozen Contract %s changed during recovery", name)
		}
	}

	// A repeated recovery recognizes the satisfied presentation without a
	// duplicate issue or grouping write.
	requests := len(forge.recordedRequests())
	repeat, err := cli.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", "grouped/feature", "--kind", "issue", "--format", "json")
	if err != nil {
		t.Fatalf("repeat feature recovery: %v", err)
	}
	if repeat.Status != "already-satisfied" {
		t.Fatalf("repeated recovery = %q, want already-satisfied", repeat.Status)
	}
	if forge.issueCount() != created {
		t.Fatalf("repeated recovery created another issue")
	}
	if len(forge.recordedRequests()) != requests {
		t.Fatal("repeated recovery performed another forge request instead of observing the receipt")
	}
}

// publicationReport reads one committed phase report from the ledger.
func publicationReport(t *testing.T, clone, proposal, slice, phase string) ledger.Report {
	t.Helper()
	path := "projects/widgets/proposals/" + proposal + "/" + slice + "/" + phase + "-report.md"
	raw := runGitOutput(t, clone, "show", "HEAD:"+path)
	report, _, err := ledger.ParseReport(phase, []byte(raw))
	if err != nil {
		t.Fatalf("parse committed %s report: %v", phase, err)
	}
	return report
}

// TestPublicationCLIRecoversLatestPhaseWithoutReplay proves B1/B2/B5/B6/B7:
// after implement, a rejected review, rework, and the final passing review, one
// recovery presents only the latest Final Review Package, catches up the
// expected source lag, applies the draft-to-ready effect, keeps the completed
// review count, and never transports private evidence.
func TestPublicationCLIRecoversLatestPhaseWithoutReplay(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newPublicationForge(t)
	source, target, remote := publicationSourceRepo(t)
	forge.branchRemote = remote
	const proposal = "phase-publication"
	const item = proposal + "/foundation"

	accept := newPublicationApp(t, forge)
	directory, flags, _ := publicationProposal(t, singleSlice(proposal), map[string]string{"foundation": "issue body\n"})
	if outcome := accept.accept(t, source, directory, flags...); outcome.Status != "accepted" {
		t.Fatalf("acceptance = %q", outcome.Status)
	}
	online := newPublicationApp(t, forge)
	offline := deliveryNoForgeApp(t)

	const privateSentinel = "PRIVATE worker exchange sentinel"
	bodyPath := filepath.Join(t.TempDir(), "phase-report.md")
	writeFile(t, bodyPath, "# Phase report\n\n"+privateSentinel+"\n")
	firstPublic := filepath.Join(t.TempDir(), "implement-one.md")
	writeFile(t, firstPublic, "implement progress one\n")
	reworkPublic := filepath.Join(t.TempDir(), "review-rework.md")
	writeFile(t, reworkPublic, "review rework progress\n")
	secondPublic := filepath.Join(t.TempDir(), "implement-two.md")
	writeFile(t, secondPublic, "implement progress two\n")
	finalPublicPath := filepath.Join(t.TempDir(), "final-review.md")
	finalPublic := "Final Review Package for the approved revision\n"
	writeFile(t, finalPublicPath, finalPublic)

	// Implement round 1 publishes normally through the forge, so the PR
	// attachment is recorded and every later presentation is an update.
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
	writeFile(t, filepath.Join(worktree, "feature.txt"), "delivered foundation\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "implement foundation")
	head := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
	if _, err := online.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", item, "--claim", claim,
		"--head", head, "--target", target, "--body", bodyPath, "--public-body", firstPublic, "--format", "json"); err != nil {
		t.Fatalf("implement submit: %v", err)
	}
	first := publicationState(t, fixture.clone, proposal, "foundation")
	if first.Submission == nil || first.State != ledger.AwaitingReview {
		t.Fatalf("initial publication did not attach a Submission: %#v", first)
	}
	number := first.Submission.Number

	// Round 1 review rejects offline: the latest recorded result becomes a
	// watchdog rework with its own registered temporary body.
	review, err := offline.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("watchdog next: %v", err)
	}
	reviewClaim := review.Execution.Claim.Commit
	if _, err := offline.deliveryJSON(t, "skl", "watchdog", "prepare", "--repo", source, "--item", item, "--claim", reviewClaim, "--format", "json"); err != nil {
		t.Fatalf("watchdog prepare: %v", err)
	}
	if _, err := offline.deliveryJSON(t, "skl", "watchdog", "inspect", "--repo", source, "--item", item, "--claim", reviewClaim, "--format", "json"); err != nil {
		t.Fatalf("watchdog inspect: %v", err)
	}
	if _, err := offline.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", item, "--claim", reviewClaim,
		"--outcome", "rework", "--body", bodyPath, "--public-body", reworkPublic, "--format", "json"); err != nil {
		t.Fatalf("watchdog rework: %v", err)
	}

	// Directed rework then hands off the resolved revision offline.
	runGit(t, source, "worktree", "remove", worktree)
	rework, err := offline.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("rework implement next: %v", err)
	}
	reworkClaim := rework.Execution.Claim.Commit
	reprepared, err := offline.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", item, "--claim", reworkClaim, "--format", "json")
	if err != nil {
		t.Fatalf("rework implement prepare: %v", err)
	}
	worktree = reprepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "feature.txt"), "delivered foundation, findings resolved\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "resolve findings")
	head2 := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
	if _, err := offline.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", item, "--claim", reworkClaim,
		"--head", head2, "--target", target, "--body", bodyPath, "--public-body", secondPublic, "--format", "json"); err != nil {
		t.Fatalf("rework implement submit: %v", err)
	}

	// Round 2 passes offline: the ledger is Ready for Merge with the final
	// temporary body registered and the presentation pending.
	review2, err := offline.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil {
		t.Fatalf("second watchdog next: %v", err)
	}
	reviewClaim2 := review2.Execution.Claim.Commit
	if _, err := offline.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", item, "--claim", reviewClaim2,
		"--outcome", "pass", "--body", bodyPath, "--public-body", finalPublicPath, "--format", "json"); err != nil {
		t.Fatalf("passing watchdog submit: %v", err)
	}
	pending := publicationState(t, fixture.clone, proposal, "foundation")
	if pending.State != ledger.ReadyForMerge || pending.Publication == nil || pending.Publication.Phase != ledger.WatchdogPhase {
		t.Fatalf("final local state = %#v", pending)
	}
	if pending.Publication.Pull == nil || pending.Publication.PullBody == nil || pending.Publication.PullBody.Path != finalPublicPath {
		t.Fatalf("final registered body = %#v", pending.Publication)
	}

	recovery := newPublicationApp(t, forge)
	recovered, err := recovery.publicationJSON(t, "skl", "publication", "recover", "--repo", source,
		"--item", item, "--kind", "pull", "--format", "json")
	if err != nil {
		t.Fatalf("recover pull: %v", err)
	}
	if recovered.Status != "published" || recovered.Packet == nil || recovered.Packet.Skill != "watchdog" {
		t.Fatalf("recovery = %#v, want a published watchdog presentation", recovered)
	}
	pull := forge.pull(number)
	if pull.Body != finalPublic || pull.Draft {
		t.Fatalf("presented pull = %#v, want the final package ready", pull)
	}
	if forge.count("POST", "/repos/acme/widgets/pulls") != 1 {
		t.Fatalf("recovery performed %d PR creates, want only the initial one", forge.count("POST", "/repos/acme/widgets/pulls"))
	}
	if len(forge.commentCreates) != 0 {
		t.Fatalf("recovery published %d unselected findings", len(forge.commentCreates))
	}
	// Only the final body reached the recovery; no intermediate progress or
	// rejection body was replayed.
	if len(forge.pullPatches) != 1 || forge.pullPatches[0]["body"] != finalPublic {
		t.Fatalf("recovery body patches = %#v", forge.pullPatches)
	}
	for _, payload := range append(append(append([]map[string]any{}, forge.pullCreates...), forge.pullPatches...), forge.issueCreates...) {
		if serialized := mustJSON(t, payload); strings.Contains(serialized, privateSentinel) {
			t.Fatalf("private evidence reached the forge: %s", serialized)
		}
	}

	// Review Count stays at the locally committed value; recovery reruns no
	// review. The exact private Manual Verification obligation stays readable.
	if report := publicationReport(t, fixture.clone, proposal, "foundation", ledger.WatchdogPhase); report.Round != 2 {
		t.Fatalf("review count = %d, want the unchanged 2", report.Round)
	}
	intent := runGitOutput(t, fixture.clone, "show", "HEAD:projects/widgets/proposals/"+item+"/intent.md")
	if !strings.Contains(intent, "Confirm the dashboard by hand") {
		t.Fatalf("frozen Manual Verification is not privately retrievable:\n%s", intent)
	}
	// The ledger never stores the public prose snapshot.
	stateJSON := mustJSON(t, publicationState(t, fixture.clone, proposal, "foundation"))
	if strings.Contains(stateJSON, finalPublic) {
		t.Fatalf("ledger persisted public prose: %s", stateJSON)
	}
}
