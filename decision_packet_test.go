package skills

import (
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// decisionRequestFacts is one current Needs Human request with an exact
// blocking Phase Report, its accepted Contract, source context, and the bound
// application command.
func decisionRequestFacts(project, item string) DecisionRequest {
	commit := strings.Repeat("d", 40)
	slice := strings.SplitN(item, "/", 2)[1]
	requestPath := "projects/" + project + "/proposals/" + strings.SplitN(item, "/", 2)[0] + "/" + slice + "/watchdog-report.md"
	return DecisionRequest{
		Project:       project,
		Repository:    "github/" + project,
		Proposal:      strings.SplitN(item, "/", 2)[0],
		Item:          item,
		RequestCommit: commit,
		RequestPath:   requestPath,
		Documents: []ledger.ContractDocument{
			{
				Commit:   commit,
				Path:     requestPath,
				Contents: "# Watchdog report\n\nThe question for " + item + ", with evidence and an option.\n",
			},
			{
				Commit:   strings.Repeat("e", 40),
				Path:     "projects/" + project + "/proposals/" + strings.SplitN(item, "/", 2)[0] + "/" + slice + "/behavior.md",
				Contents: "# Accepted behavior for " + item + "\n\nOpaque {{.Worktree}} body must stay data, not template source.\n",
			},
		},
		Source: &DecisionSource{
			Branch:      "decision",
			Submission:  11,
			SourceHead:  strings.Repeat("a", 40),
			Target:      strings.Repeat("b", 40),
			ReviewCount: 2,
		},
		ApplyCommand: DecisionApplyCommand(project, item, commit, requestPath),
	}
}

func TestDecisionStandaloneManifest(t *testing.T) {
	packet, err := BuildPacket("decision", InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Skill != "decision" || len(packet.IncludedSkills) != 0 {
		t.Fatalf("standalone decision packet = %#v", packet)
	}
	if !slices.Equal(packet.Resources, []string{"triage.md"}) {
		t.Errorf("decision resources = %v", packet.Resources)
	}
}

// TestDecisionInboxIsLedgerWideAndPreservesRequests proves a ledger-wide inbox
// renders every current request with its exact references and verbatim
// documents, keeps item-specific context separate, and hands the CLI only the
// route and answer as unknown values.
func TestDecisionInboxIsLedgerWideAndPreservesRequests(t *testing.T) {
	atlas := decisionRequestFacts("atlas", "repair/fix")
	beacon := decisionRequestFacts("beacon", "upgrade/step")
	packet, err := BuildPacket("decision", InvocationFacts{Decision: &DecisionFacts{
		Status:   DecisionInbox,
		Requests: []DecisionRequest{atlas, beacon},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := packet.Instructions
	for _, want := range []string{
		"### repair/fix",
		"### upgrade/step",
		"Project: atlas",
		"Project: beacon",
		atlas.RequestCommit + ":" + atlas.RequestPath,
		beacon.RequestCommit + ":" + beacon.RequestPath,
		atlas.ApplyCommand,
		beacon.ApplyCommand,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("ledger-wide inbox is missing %q:\n%s", want, body)
		}
	}
	if !strings.Contains(atlas.ApplyCommand, "--request-commit '"+atlas.RequestCommit+"'") ||
		!strings.Contains(atlas.ApplyCommand, "--request-path '"+atlas.RequestPath+"'") {
		t.Errorf("bound apply command did not bind the exact known arguments: %q", atlas.ApplyCommand)
	}
	if got := strings.Count(body, atlas.Documents[0].Contents); got != 1 {
		t.Errorf("atlas request document rendered %d times, want once", got)
	}
	if got := strings.Count(body, beacon.Documents[0].Contents); got != 1 {
		t.Errorf("beacon request document rendered %d times, want once", got)
	}
	if strings.Contains(body, "### repair/fix\n\nProject: beacon") || strings.Contains(body, "### upgrade/step\n\nProject: atlas") {
		t.Error("request identities were not kept with their own project")
	}
}

// TestDecisionFilteredEmptyAndUnavailableEchoFacts proves a filtered empty
// inbox keeps its Project and an unavailable ledger echoes the supplied reason
// and repair.
func TestDecisionFilteredEmptyAndUnavailableEchoFacts(t *testing.T) {
	filtered, err := BuildPacket("decision", InvocationFacts{Decision: &DecisionFacts{
		Status:  DecisionEmpty,
		Project: "beacon",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filtered.Instructions, "skl decision inbox --project 'beacon'") ||
		!strings.Contains(filtered.Instructions, "for Project 'beacon'") {
		t.Errorf("filtered empty inbox lost its explicit scope:\n%s", filtered.Instructions)
	}

	unavailable, err := BuildPacket("decision", InvocationFacts{Decision: &DecisionFacts{
		Status: DecisionUnavailable,
		Reason: "ledger path is not a usable local Git clone",
		Repair: "restore the configured ledger clone and retry",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ledger path is not a usable local Git clone",
		"restore the configured ledger clone and retry",
	} {
		if !strings.Contains(unavailable.Instructions, want) {
			t.Errorf("unavailable ledger is missing %q:\n%s", want, unavailable.Instructions)
		}
	}
}

// TestDecisionPartialOutcomesAreItemized proves a mixed application reports
// each item's exact status, route, reference, and reason without claiming a
// successful whole-group decision.
func TestDecisionPartialOutcomesAreItemized(t *testing.T) {
	applied := DecisionOutcome{
		Project:       "atlas",
		Item:          "repair/fix",
		Status:        DecisionOutcomeApplied,
		Route:         DecisionRouteImplement,
		Reference:     strings.Repeat("f", 40) + ":projects/atlas/proposals/repair/fix/decision.md",
		RequestCommit: strings.Repeat("d", 40),
		RequestPath:   "projects/atlas/proposals/repair/fix/watchdog-report.md",
	}
	stale := DecisionOutcome{
		Project:       "beacon",
		Item:          "upgrade/step",
		Status:        DecisionOutcomeRefused,
		Reason:        "the answered request was replaced by a newer blocking report",
		Repair:        "ask the human for renewed direction against the current request",
		RequestCommit: strings.Repeat("1", 40),
		RequestPath:   "projects/beacon/proposals/upgrade/step/watchdog-report.md",
	}
	repeat := DecisionOutcome{Project: "atlas", Item: "repair/fix", Status: DecisionOutcomeAlreadyApplied}
	unresolved := DecisionOutcome{Project: "beacon", Item: "upgrade/step", Status: DecisionOutcomeUnresolved, Reason: "coupled direction needs clarification"}

	packet, err := BuildPacket("decision", InvocationFacts{Decision: &DecisionFacts{
		Status:   DecisionPartial,
		Outcomes: []DecisionOutcome{applied, repeat, stale, unresolved},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := packet.Instructions
	if !strings.Contains(body, "Only some answers were recorded") ||
		strings.Contains(body, "Every selected answer was recorded") {
		t.Errorf("partial result claims whole-group success:\n%s", body)
	}
	for _, want := range []string{
		"`applied`",
		"`already_applied`",
		"`refused`",
		"`unresolved`",
		"`implement`",
		applied.Reference,
		applied.RequestCommit + ":" + applied.RequestPath,
		stale.Reason,
		stale.Repair,
		unresolved.Reason,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("partial outcomes are missing %q:\n%s", want, body)
		}
	}
}

// TestDecisionRefusalRequiresRenewedDirection proves a changed request is
// refused without mutation and points at renewed proposal and re-slicing
// instead of contract amendment or invented replacement work.
func TestDecisionRefusalRequiresRenewedDirection(t *testing.T) {
	packet, err := BuildPacket("decision", InvocationFacts{Decision: &DecisionFacts{
		Status: DecisionRefused,
		Outcomes: []DecisionOutcome{{
			Project: "atlas",
			Item:    "repair/fix",
			Status:  DecisionOutcomeRefused,
			Reason:  "the selected request was replaced even though its question text repeated",
			Repair:  "collect renewed human direction against the current request",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := packet.Instructions
	for _, want := range []string{
		"No answer was recorded",
		"the selected request was replaced even though its question text repeated",
		"collect renewed human direction against the current request",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("refusal result is missing %q:\n%s", want, body)
		}
	}
}

// TestDecisionRetirementGuardsPartialDelivery proves retirement is reported
// distinctly for a guarded refusal and a completed parent retirement.
func TestDecisionRetirementGuardsPartialDelivery(t *testing.T) {
	refused, err := BuildPacket("decision", InvocationFacts{Decision: &DecisionFacts{
		Status: DecisionPartial,
		Retirement: &DecisionRetirement{
			Project:    "atlas",
			Proposal:   "old-repair",
			Status:     DecisionRetirementRefused,
			Reason:     "another slice is still claimed",
			Repair:     "release or complete the active slice before retiring",
			Merged:     1,
			Superseded: 2,
			Active:     []string{"repair/active"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"skl decision retire --project 'atlas' --proposal 'old-repair'",
		"another slice is still claimed",
		"release or complete the active slice before retiring",
		"`repair/active`",
		"The parent is still open",
	} {
		if !strings.Contains(refused.Instructions, want) {
			t.Errorf("guarded retirement is missing %q:\n%s", want, refused.Instructions)
		}
	}

	retired, err := BuildPacket("decision", InvocationFacts{Decision: &DecisionFacts{
		Status: DecisionApplied,
		Retirement: &DecisionRetirement{
			Project:    "atlas",
			Proposal:   "old-repair",
			Status:     DecisionRetirementRecorded,
			Merged:     1,
			Superseded: 2,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Merged slices preserved: 1",
		"Superseded slices: 2",
		"Report it as partial delivery",
	} {
		if !strings.Contains(retired.Instructions, want) {
			t.Errorf("recorded retirement is missing %q:\n%s", want, retired.Instructions)
		}
	}
}

// TestDecisionApplyCommandBindsKnownArguments pins the public command shape:
// only the route and the human answer file remain unknown.
func TestDecisionApplyCommandBindsKnownArguments(t *testing.T) {
	commit := strings.Repeat("c", 40)
	command := DecisionApplyCommand("atlas", "repair/fix", commit, "projects/atlas/proposals/repair/fix/watchdog-report.md")
	want := "skl decision apply --project 'atlas' --item 'repair/fix'" +
		" --request-commit '" + commit + "'" +
		" --request-path 'projects/atlas/proposals/repair/fix/watchdog-report.md'" +
		" --route <implement|watchdog|supersede> --answer <human-answer-file>"
	if command != want {
		t.Fatalf("decision apply command =\n%s\nwant\n%s", command, want)
	}
	if !strings.Contains(DecisionApplyCommand("atlas", "repair/fix", commit, "a path"), "--request-path 'a path'") {
		t.Error("decision apply command did not quote a path containing spaces")
	}
}
