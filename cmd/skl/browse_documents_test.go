package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

type browseDocumentsFixture struct {
	ledger            *ledgerFixture
	initialRevision   string
	acceptedRevision  string
	decisionRevision  string
	implementRevision string
	currentRevision   string
	implementPath     string
	implementBody     string
	sourceHead        string
	sourceTarget      string
}

func newBrowseDocumentsFixture(t *testing.T) browseDocumentsFixture {
	t.Helper()
	fixture := browseFixture(t)
	result := browseDocumentsFixture{
		ledger:          fixture,
		initialRevision: browseDocumentsHead(t, fixture.clone),
		implementPath:   "projects/widgets/proposals/orders/cancel/implement-report.md",
		implementBody:   "# Implementation evidence\n\nRecorded implementation content.\n",
		sourceHead:      strings.Repeat("a", 40),
		sourceTarget:    strings.Repeat("b", 40),
	}
	files := map[string]string{
		"projects/widgets/proposals/orders/proposal.md":               "# Order cancellation\n\nProposal description stays readable.\n",
		"projects/widgets/proposals/orders/cancel/intent.md":          "# Intent\n\nAccepted intent text.\n",
		"projects/widgets/proposals/orders/cancel/behavior.md":        "# Behavior\n\nFrozen cancellation behavior.\n",
		"projects/widgets/proposals/orders/cancel/plan.md":            "# Plan\n\nAccepted implementation plan.\n",
		"projects/widgets/proposals/orders/cancel/state.json":         `{"state":"awaiting_review","title":"Cancel orders","branch":"feat/cancel","decision":true,"issue":{"repository":"acme/widgets","number":11},"claim":{"phase":"watchdog","basis":"3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c","inputs":{"contract":[]}}}`,
		"projects/widgets/archive/legacy/proposal.md":                 "# Legacy proposal\n\nArchived proposal description.\n",
		"projects/widgets/archive/legacy/old/intent.md":               "# Legacy intent\n\nArchived accepted intent.\n",
		"projects/widgets/archive/legacy/old/behavior.md":             "# Legacy behavior\n\nArchived accepted behavior.\n",
		"projects/widgets/archive/orders/proposal.md":                 "# Archived orders\n\nArchived same-name Proposal.\n",
		"projects/widgets/archive/orders/cancel/intent.md":            "# Archived cancellation intent\n\nArchived same-name Slice.\n",
		"projects/widgets/archive/orders/cancel/behavior.md":          "# Archived cancellation behavior\n\nArchived behavior.\n",
		"projects/widgets/archive/orders/cancel/state.json":           `{"state":"merged","title":"Archived cancellation"}`,
		"projects/gadgets/proposals/tools/hammer/intent.md":           "# Hammer intent\n\nAccepted tools intent.\n",
		"projects/gadgets/proposals/tools/hammer/behavior.md":         "# Hammer behavior\n\nAccepted tools behavior.\n",
		"projects/gadgets/proposals/tools/hammer/implement-report.md": "---\nschema: not-an-integer\noutcome: pass\n---\n# Still readable\n\nMalformed metadata must not hide these bytes.\n",
	}
	for name, contents := range files {
		writeFile(t, filepath.Join(fixture.clone, filepath.FromSlash(name)), contents)
	}
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "document fixtures")
	result.acceptedRevision = browseDocumentsHead(t, fixture.clone)

	decisionPath := "projects/widgets/proposals/orders/cancel/decision.md"
	decision, err := ledger.FormatDecision(ledger.DecisionRecord{
		Schema:  ledger.DecisionSchema,
		Project: "widgets",
		Item:    "orders/cancel",
		AnsweredRequest: ledger.Reference{
			Commit: result.acceptedRevision,
			Path:   "projects/widgets/proposals/orders/cancel/state.json",
		},
		Route: ledger.RouteImplement,
	}, "Proceed with the accepted contract.\n")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fixture.clone, filepath.FromSlash(decisionPath)), string(decision))
	runGit(t, fixture.clone, "add", decisionPath)
	runGit(t, fixture.clone, "commit", "-q", "-m", "record human decision")
	result.decisionRevision = browseDocumentsHead(t, fixture.clone)

	claim := ledger.Reference{Commit: result.acceptedRevision, Path: "projects/widgets/proposals/orders/cancel/state.json"}
	contract := []ledger.Reference{
		{Commit: result.acceptedRevision, Path: "projects/widgets/proposals/orders/cancel/intent.md"},
		{Commit: result.acceptedRevision, Path: "projects/widgets/proposals/orders/cancel/behavior.md"},
	}
	implement, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
		Schema:  ledger.ReportSchema,
		Outcome: "awaiting_review",
		Source:  ledger.SourceRevisions{Head: result.sourceHead, Target: result.sourceTarget},
		Ledger:  ledger.ReportInputs{Claim: claim, Contract: contract},
	}, result.implementBody)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fixture.clone, filepath.FromSlash(result.implementPath)), string(implement))
	runGit(t, fixture.clone, "add", result.implementPath)
	runGit(t, fixture.clone, "commit", "-q", "-m", "record implementation report")
	result.implementRevision = browseDocumentsHead(t, fixture.clone)

	watchdogPath := "projects/widgets/proposals/orders/cancel/watchdog-report.md"
	decisionReference := ledger.Reference{Commit: result.decisionRevision, Path: decisionPath}
	implementReference := ledger.Reference{Commit: result.implementRevision, Path: result.implementPath}
	watchdog, err := ledger.FormatReport(ledger.WatchdogPhase, ledger.Report{
		Schema:  ledger.ReportSchema,
		Outcome: "pass",
		Source:  ledger.SourceRevisions{Head: result.sourceHead, Target: result.sourceTarget, Reviewed: result.sourceHead},
		Ledger: ledger.ReportInputs{
			Claim: claim, Contract: contract, Implement: &implementReference, Decision: &decisionReference,
		},
		Round: 2,
	}, "# Watchdog evidence\n\nThe report records a passing review.\n")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fixture.clone, filepath.FromSlash(watchdogPath)), string(watchdog))
	runGit(t, fixture.clone, "add", watchdogPath)
	runGit(t, fixture.clone, "commit", "-q", "-m", "record watchdog report")
	runGit(t, fixture.clone, "push", "-q")
	result.currentRevision = browseDocumentsHead(t, fixture.clone)
	return result
}

func browseDocumentsHead(t *testing.T, root string) string {
	t.Helper()
	return strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
}

func browseDocumentsJSON(t *testing.T, app *stageApp, output *bytes.Buffer, args ...string) browseOutcome {
	t.Helper()
	output.Reset()
	command := append([]string{"skl", "browse"}, args...)
	command = append(command, "--format", "json")
	if err := app.Run(command); err != nil {
		t.Fatalf("browse %v: %v\n%s", args, err, output)
	}
	var outcome browseOutcome
	if err := json.Unmarshal(output.Bytes(), &outcome); err != nil {
		t.Fatalf("decode browse response %q: %v", output, err)
	}
	return outcome
}

func TestBrowseDocumentsExposeCurrentAndArchivedEvidenceReadOnly(t *testing.T) {
	fixture := newBrowseDocumentsFixture(t)
	app, output := browseApp(t)
	cloneBefore := ledgerSnapshot(t, fixture.ledger.clone)
	upstreamBefore := runGitOutput(t, fixture.ledger.upstream, "for-each-ref")
	headBefore := runGitOutput(t, fixture.ledger.clone, "rev-parse", "HEAD")

	proposal := browseDocumentsJSON(t, app, output, "documents", "--project", "widgets", "--proposal", "orders")
	if proposal.Status != "shown" || proposal.Documents == nil || proposal.Documents.Revision != fixture.currentRevision || proposal.Documents.Archived {
		t.Fatalf("current Proposal documents = %+v", proposal)
	}
	if !hasDocument(proposal.Documents.Documents, ledger.ProposalDocumentKind, "projects/widgets/proposals/orders/proposal.md") ||
		!hasDocument(proposal.Documents.Documents, ledger.ContractDocumentKind, "projects/widgets/proposals/orders/cancel/plan.md") ||
		!hasDocument(proposal.Documents.Documents, ledger.ImplementReportDocumentKind, fixture.implementPath) ||
		!hasDocument(proposal.Documents.Documents, ledger.DecisionDocumentKind, "projects/widgets/proposals/orders/cancel/decision.md") {
		t.Fatalf("Proposal discovery omitted its description, accepted plan, current report or Human Decision: %+v", proposal.Documents.Documents)
	}
	archivedSameName := browseDocumentsJSON(t, app, output, "documents", "--project", "widgets", "--proposal", "orders", "--archived")
	if archivedSameName.Status != "shown" || archivedSameName.Documents == nil || !archivedSameName.Documents.Archived ||
		!hasDocument(archivedSameName.Documents.Documents, ledger.ProposalDocumentKind, "projects/widgets/archive/orders/proposal.md") {
		t.Fatalf("--archived did not select the same-name archived Proposal: %+v", archivedSameName)
	}

	current := browseDocumentsJSON(t, app, output, "documents", "--project", "widgets", "--item", "orders/cancel")
	if current.Status != "shown" || current.Documents == nil || current.Documents.Revision != fixture.currentRevision || current.Documents.Slice != "cancel" {
		t.Fatalf("current Slice documents = %+v", current)
	}
	archivedSameNameSlice := browseDocumentsJSON(t, app, output, "documents", "--project", "widgets", "--item", "orders/cancel", "--archived")
	if archivedSameNameSlice.Status != "shown" || archivedSameNameSlice.Documents == nil || !archivedSameNameSlice.Documents.Archived ||
		!hasDocument(archivedSameNameSlice.Documents.Documents, ledger.ContractDocumentKind, "projects/widgets/archive/orders/cancel/intent.md") {
		t.Fatalf("--archived did not select the same-name archived Slice: %+v", archivedSameNameSlice)
	}
	if !hasDocument(current.Documents.Documents, ledger.ImplementReportDocumentKind, fixture.implementPath) ||
		!hasDocument(current.Documents.Documents, ledger.WatchdogReportDocumentKind, "projects/widgets/proposals/orders/cancel/watchdog-report.md") ||
		!hasDocument(current.Documents.Documents, ledger.DecisionDocumentKind, "projects/widgets/proposals/orders/cancel/decision.md") {
		t.Fatalf("Slice discovery omitted current phase or decision evidence: %+v", current.Documents.Documents)
	}
	for _, kind := range []ledger.DocumentKind{ledger.ImplementReportDocumentKind, ledger.WatchdogReportDocumentKind, ledger.DecisionDocumentKind} {
		availability := browseDocumentAvailability(t, current.Documents, kind)
		if availability.Status != ledger.DocumentAvailable || availability.Reference == nil || availability.Reference.Commit != fixture.currentRevision {
			t.Errorf("current %s availability = %+v", kind, availability)
		}
	}
	for _, document := range current.Documents.Documents {
		if document.Reference.Commit != fixture.currentRevision {
			t.Fatalf("current document reference did not use the pinned snapshot: %+v", document.Reference)
		}
	}
	watchdog := findDocument(t, current.Documents.Documents, ledger.WatchdogReportDocumentKind)
	if watchdog.Report == nil || watchdog.Report.Outcome != "pass" || watchdog.Report.Round != 2 || watchdog.Report.Source.Head != fixture.sourceHead || watchdog.Report.Source.Target != fixture.sourceTarget || watchdog.Report.Source.Head == watchdog.Reference.Commit {
		t.Fatalf("structured watchdog evidence conflated source and ledger revisions: %+v", watchdog)
	}
	if len(watchdog.References) < 4 {
		t.Fatalf("watchdog ledger inputs were not exposed: %+v", watchdog.References)
	}
	decision := findDocument(t, current.Documents.Documents, ledger.DecisionDocumentKind)
	if decision.Decision == nil || decision.Decision.Route != ledger.RouteImplement || decision.Decision.AnsweredRequest.Commit != fixture.acceptedRevision {
		t.Fatalf("human decision metadata = %+v", decision)
	}

	archived := browseDocumentsJSON(t, app, output, "documents", "--project", "widgets", "--proposal", "legacy")
	if archived.Status != "shown" || archived.Documents == nil || !archived.Documents.Archived ||
		!hasDocument(archived.Documents.Documents, ledger.ProposalDocumentKind, "projects/widgets/archive/legacy/proposal.md") ||
		!hasDocument(archived.Documents.Documents, ledger.ContractDocumentKind, "projects/widgets/archive/legacy/old/intent.md") {
		t.Fatalf("archived Proposal documents = %+v", archived)
	}

	malformed := browseDocumentsJSON(t, app, output, "documents", "--project", "gadgets", "--item", "tools/hammer")
	if malformed.Status != "shown" || malformed.Documents == nil || !malformed.Documents.Incomplete {
		t.Fatalf("malformed report metadata was not diagnosed: %+v", malformed)
	}
	badReport := findDocument(t, malformed.Documents.Documents, ledger.ImplementReportDocumentKind)
	if badReport.Report != nil || len(badReport.Diagnostics) == 0 || !strings.Contains(badReport.Contents, "Malformed metadata must not hide these bytes.") {
		t.Fatalf("malformed report lost content or fabricated metadata: %+v", badReport)
	}
	if len(malformed.Documents.Diagnostics) == 0 {
		t.Fatalf("DocumentSet did not disclose malformed metadata: %+v", malformed.Documents)
	}

	// A genuinely absent optional Watchdog report is not classified as a
	// malformed record; the readable current content remains available.
	if hasDocument(malformed.Documents.Documents, ledger.WatchdogReportDocumentKind, "") || diagnosticMentions(malformed.Documents.Diagnostics, "watchdog-report.md") ||
		hasDocument(malformed.Documents.Documents, ledger.DecisionDocumentKind, "") || diagnosticMentions(malformed.Documents.Diagnostics, "decision.md") {
		t.Fatalf("absent optional report or decision was treated as malformed: %+v", malformed.Documents)
	}
	for _, kind := range []ledger.DocumentKind{ledger.WatchdogReportDocumentKind, ledger.DecisionDocumentKind} {
		availability := browseDocumentAvailability(t, malformed.Documents, kind)
		if availability.Status != ledger.DocumentAbsent || availability.Reference != nil {
			t.Errorf("absent %s availability = %+v", kind, availability)
		}
	}
	if availability := browseDocumentAvailability(t, malformed.Documents, ledger.ImplementReportDocumentKind); availability.Status != ledger.DocumentAvailable || availability.Reference == nil {
		t.Errorf("readable malformed report availability = %+v", availability)
	}
	output.Reset()
	if err := app.Run([]string{"skl", "browse", "documents", "--project", "gadgets", "--item", "tools/hammer"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "watchdog for hammer — not yet available") ||
		!strings.Contains(output.String(), "Human Decision availability for hammer — not yet available") ||
		!strings.Contains(output.String(), "Malformed metadata must not hide these bytes.") {
		t.Fatalf("Markdown did not distinguish optional absence from readable malformed evidence:\n%s", output)
	}

	if ledgerSnapshot(t, fixture.ledger.clone) != cloneBefore || runGitOutput(t, fixture.ledger.upstream, "for-each-ref") != upstreamBefore || runGitOutput(t, fixture.ledger.clone, "rev-parse", "HEAD") != headBefore {
		t.Fatal("browse document queries changed local or upstream Git state")
	}
}

func TestBrowseDocumentsReportUnavailableOptionalReadFromTypedFact(t *testing.T) {
	fixture := newBrowseDocumentsFixture(t)
	app, output := browseApp(t)
	path := "projects/widgets/proposals/orders/cancel/watchdog-report.md"
	blob := strings.TrimSpace(runGitOutput(t, fixture.ledger.clone, "rev-parse", "HEAD:"+path))
	objectPath := filepath.Join(fixture.ledger.clone, ".git", "objects", blob[:2], blob[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatalf("remove report blob %s: %v", objectPath, err)
	}

	outcome := browseDocumentsJSON(t, app, output, "documents", "--project", "widgets", "--item", "orders/cancel")
	if outcome.Documents == nil {
		t.Fatalf("documents query returned no set: %+v", outcome)
	}
	availability := browseDocumentAvailability(t, outcome.Documents, ledger.WatchdogReportDocumentKind)
	wantReference := ledger.Reference{Commit: outcome.Documents.Revision, Path: path}
	if availability.Status != ledger.DocumentUnavailable || availability.Reference == nil || *availability.Reference != wantReference {
		t.Fatalf("failed report read availability = %+v, want exact unavailable reference %+v", availability, wantReference)
	}
	output.Reset()
	if err := app.Run([]string{"skl", "browse", "documents", "--project", "widgets", "--item", "orders/cancel"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "watchdog for cancel — unavailable; see diagnostic") ||
		!strings.Contains(output.String(), "cannot read document "+path) {
		t.Fatalf("Markdown did not report the unavailable typed fact and its diagnostic:\n%s", output)
	}
}

func TestBrowseDocumentsReportUnknownDecisionWhenStateIsMalformed(t *testing.T) {
	fixture := newBrowseDocumentsFixture(t)
	app, output := browseApp(t)
	statePath := "projects/widgets/proposals/orders/cancel/state.json"
	writeFile(t, filepath.Join(fixture.ledger.clone, filepath.FromSlash(statePath)), "{malformed state\n")
	runGit(t, fixture.ledger.clone, "add", statePath)
	runGit(t, fixture.ledger.clone, "commit", "-q", "-m", "damage decision membership state")

	outcome := browseDocumentsJSON(t, app, output, "documents", "--project", "widgets", "--item", "orders/cancel")
	if outcome.Documents == nil {
		t.Fatalf("documents query returned no set: %+v", outcome)
	}
	availability := browseDocumentAvailability(t, outcome.Documents, ledger.DecisionDocumentKind)
	wantState := ledger.Reference{Commit: outcome.Documents.Revision, Path: statePath}
	if availability.Status != ledger.DocumentUnknown || availability.Reference == nil || *availability.Reference != wantState {
		t.Fatalf("malformed state decision availability = %+v, want unknown with state reference %+v", availability, wantState)
	}
	output.Reset()
	if err := app.Run([]string{"skl", "browse", "documents", "--project", "widgets", "--item", "orders/cancel"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Human Decision availability for cancel — unknown") {
		t.Fatalf("malformed state was presented as an absent decision instead of unknown:\n%s", output)
	}
}

func TestBrowseDocumentFollowsExactReferenceAndPreservesLedgerShowRaw(t *testing.T) {
	fixture := newBrowseDocumentsFixture(t)
	app, output := browseApp(t)
	cloneBefore := ledgerSnapshot(t, fixture.ledger.clone)
	upstreamBefore := runGitOutput(t, fixture.ledger.upstream, "for-each-ref")

	exact := browseDocumentsJSON(t, app, output, "document", "--commit", fixture.implementRevision, "--path", fixture.implementPath)
	if exact.Status != "shown" || exact.Document == nil || exact.Document.Reference.Commit != fixture.implementRevision || exact.Document.Reference.Path != fixture.implementPath || exact.SnapshotRevision != fixture.currentRevision {
		t.Fatalf("exact reference identity = %+v", exact)
	}
	if exact.Document.Contents == "" || !strings.Contains(exact.Document.Contents, fixture.implementBody) || exact.Document.Report == nil || exact.Document.Report.Source.Head != fixture.sourceHead {
		t.Fatalf("exact document content/metadata = %+v", exact.Document)
	}
	output.Reset()
	if err := app.Run([]string{"skl", "browse", "document", "--commit", fixture.implementRevision, "--path", fixture.implementPath}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Ledger snapshot revision: " + fixture.currentRevision,
		"Document ledger revision: " + fixture.implementRevision + " (historical document",
		"Source repository revisions (not ledger revisions):",
		"Source head: " + fixture.sourceHead,
		"Consumed ledger references:",
		"Ledger document reference: " + fixture.implementRevision + ":" + fixture.implementPath,
		fixture.implementBody,
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("exact Markdown lacks %q:\n%s", expected, output)
		}
	}

	missingCommit := strings.Repeat("0", 40)
	missing := browseDocumentsJSON(t, app, output, "document", "--commit", missingCommit, "--path", fixture.implementPath)
	if missing.Status != "fix_required" || missing.Document != nil || !strings.Contains(missing.Reason, "unavailable") {
		t.Fatalf("unavailable exact commit fell back to current: %+v", missing)
	}
	missingPath := browseDocumentsJSON(t, app, output, "document", "--commit", fixture.initialRevision, "--path", "projects/widgets/proposals/orders/cancel/intent.md")
	if missingPath.Status != "fix_required" || missingPath.Document != nil || !strings.Contains(missingPath.Reason, "unavailable") {
		t.Fatalf("unavailable exact path fell back to current: %+v", missingPath)
	}

	// Legacy ledger show remains a byte-preserving raw route, independently
	// of document metadata presentation in browse.
	output.Reset()
	if err := app.Run([]string{"skl", "ledger", "show", "--commit", fixture.implementRevision, "--path", fixture.implementPath}); err != nil {
		t.Fatal(err)
	}
	wantRaw := "Status: shown\nDocument: " + fixture.implementPath + " at " + fixture.implementRevision + "\n\n"
	exactBody := runGitOutput(t, fixture.ledger.clone, "show", fixture.implementRevision+":"+fixture.implementPath)
	if output.String() != wantRaw+exactBody {
		t.Fatalf("legacy ledger show changed exact raw output:\nwant %q\ngot  %q", wantRaw+exactBody, output.String())
	}

	if ledgerSnapshot(t, fixture.ledger.clone) != cloneBefore || runGitOutput(t, fixture.ledger.upstream, "for-each-ref") != upstreamBefore {
		t.Fatal("exact browsing or legacy raw readback changed local or upstream Git state")
	}
}

func TestBrowseDocumentExposesClaimInputsAsExactCLIFacts(t *testing.T) {
	fixture := newBrowseDocumentsFixture(t)
	app, output := browseApp(t)
	statePath := "projects/widgets/proposals/orders/cancel/state.json"
	intentPath := "projects/widgets/proposals/orders/cancel/intent.md"
	input := ledger.Reference{Commit: fixture.acceptedRevision, Path: intentPath}
	writeFile(t, filepath.Join(fixture.ledger.clone, filepath.FromSlash(statePath)), `{"state":"awaiting_review","title":"Cancel orders","branch":"feat/cancel","claim":{"phase":"watchdog","basis":"`+fixture.acceptedRevision+`","inputs":{"contract":[{"commit":"`+input.Commit+`","path":"`+input.Path+`"}]}}}`)
	runGit(t, fixture.ledger.clone, "add", statePath)
	runGit(t, fixture.ledger.clone, "commit", "-q", "-m", "record fixed Claim input")
	claimRevision := browseDocumentsHead(t, fixture.ledger.clone)
	outcome := browseDocumentsJSON(t, app, output, "document", "--commit", claimRevision, "--path", statePath)
	if outcome.Status != "shown" || outcome.Document == nil || outcome.Document.Claim == nil || outcome.Document.Claim.Phase != ledger.WatchdogPhase ||
		len(outcome.Document.References) != 1 || outcome.Document.References[0] != (ledger.LabeledReference{Label: "contract", Reference: input}) {
		t.Fatalf("CLI Claim facts = %+v", outcome)
	}
	followed := browseDocumentsJSON(t, app, output, "document", "--commit", input.Commit, "--path", input.Path)
	if followed.Document == nil || followed.Document.Reference != input || !strings.Contains(followed.Document.Contents, "Accepted intent text.") {
		t.Fatalf("CLI exact Claim input = %+v", followed)
	}
	output.Reset()
	if err := app.Run([]string{"skl", "browse", "document", "--commit", claimRevision, "--path", statePath}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Claim input ledger references:") || !strings.Contains(output.String(), "- contract: "+input.Commit+":"+input.Path) || !strings.Contains(output.String(), "Claim metadata: phase watchdog") {
		t.Fatalf("CLI Markdown omitted structured Claim inputs:\n%s", output)
	}
}

func TestBrowseVersionsListExactContentChangesAndSelectHistoricalCLIBytes(t *testing.T) {
	fixture := newBrowseDocumentsFixture(t)
	app, output := browseApp(t)
	older := fixture.implementRevision
	path := fixture.implementPath
	metadataOnly := strings.Replace(runGitOutput(t, fixture.ledger.clone, "show", older+":"+path), "outcome: awaiting_review", "outcome: needs_human", 1)
	if metadataOnly == runGitOutput(t, fixture.ledger.clone, "show", older+":"+path) {
		t.Fatal("fixture did not alter report metadata")
	}
	writeFile(t, filepath.Join(fixture.ledger.clone, filepath.FromSlash(path)), metadataOnly)
	runGit(t, fixture.ledger.clone, "add", path)
	runGit(t, fixture.ledger.clone, "commit", "-q", "-m", "metadata-only version")
	latest := browseDocumentsHead(t, fixture.ledger.clone)
	runGit(t, fixture.ledger.clone, "commit", "--allow-empty", "-q", "-m", "unrelated ledger commit")
	versions := browseDocumentsJSON(t, app, output, "versions", "--project", "widgets", "--item", "orders/cancel", "--phase", "implement")
	if versions.Status != "shown" || versions.Versions == nil || versions.Versions.Incomplete || len(versions.Versions.Versions) != 2 ||
		versions.Versions.Versions[0].Reference != (ledger.Reference{Commit: latest, Path: path}) ||
		versions.Versions.Versions[1].Reference != (ledger.Reference{Commit: older, Path: path}) ||
		versions.Versions.Versions[0].Report == nil || versions.Versions.Versions[0].Report.Outcome != "needs_human" {
		t.Fatalf("CLI versions = %+v", versions)
	}
	old := browseDocumentsJSON(t, app, output, "document", "--commit", older, "--path", path)
	if old.Document == nil || old.Document.Contents == metadataOnly || !strings.Contains(old.Document.Contents, fixture.implementBody) || old.SnapshotRevision != versions.Versions.Revision {
		t.Fatalf("CLI historical selection = %+v", old)
	}
	output.Reset()
	if err := app.Run([]string{"skl", "browse", "versions", "--project", "widgets", "--item", "orders/cancel", "--phase", "implement"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Ledger version: "+latest+":"+path) || !strings.Contains(output.String(), "Ledger version: "+older+":"+path) || !strings.Contains(output.String(), "Outcome: needs_human") {
		t.Fatalf("Markdown versions omitted exact identities or metadata:\n%s", output)
	}
}

func TestBrowseDocumentsRequiresUnambiguousSelection(t *testing.T) {
	newBrowseDocumentsFixture(t)
	app, output := browseApp(t)
	for _, args := range [][]string{
		{"documents", "--project", "widgets"},
		{"documents", "--project", "widgets", "--proposal", "orders", "--item", "orders/cancel"},
	} {
		outcome := browseDocumentsJSON(t, app, output, args...)
		if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "exactly one") {
			t.Errorf("ambiguous document query %v = %+v", args, outcome)
		}
	}
}

func browseDocumentAvailability(t *testing.T, set *ledger.DocumentSet, kind ledger.DocumentKind) ledger.DocumentAvailability {
	t.Helper()
	for _, availability := range set.Availability {
		if availability.Kind == kind {
			return availability
		}
	}
	t.Fatalf("no availability for %s in %+v", kind, set.Availability)
	return ledger.DocumentAvailability{}
}

func hasDocument(documents []ledger.Document, kind ledger.DocumentKind, path string) bool {
	for _, document := range documents {
		if document.Kind == kind && (path == "" || document.Reference.Path == path) {
			return true
		}
	}
	return false
}

func findDocument(t *testing.T, documents []ledger.Document, kind ledger.DocumentKind) ledger.Document {
	t.Helper()
	for _, document := range documents {
		if document.Kind == kind {
			return document
		}
	}
	t.Fatalf("no document of kind %q in %+v", kind, documents)
	return ledger.Document{}
}

func diagnosticMentions(diagnostics []ledger.Diagnostic, text string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Problem, text) {
			return true
		}
	}
	return false
}
