package skills

import (
	"os"
	"path/filepath"
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
			Worktree:    "/tmp/decision",
			Submission:  11,
			SourceHead:  strings.Repeat("a", 40),
			Target:      strings.Repeat("b", 40),
			ReviewCount: 2,
		},
		ApplyCommand: DecisionApplyCommand(project, item, commit, requestPath),
	}
}

func TestDecisionStandaloneRetrievalPointsAtTheInbox(t *testing.T) {
	packet, err := BuildPacket("decision", InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Skill != "decision" || len(packet.IncludedSkills) != 0 {
		t.Fatalf("standalone decision packet = %#v", packet)
	}
	for _, want := range []string{
		"`skl decision inbox`",
		"`skl skill --resource reference/triage.md decision`",
		"records no decision",
	} {
		if !strings.Contains(packet.Instructions, want) {
			t.Errorf("standalone retrieval is missing %q:\n%s", want, packet.Instructions)
		}
	}
	if strings.Contains(packet.Instructions, "## Current requests") {
		t.Error("standalone retrieval rendered a request list without ledger facts")
	}
	if !slices.Equal(packet.Resources, []string{"reference/triage.md"}) {
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
		"ledger-wide",
		"never narrows it",
		"### repair/fix",
		"### upgrade/step",
		"Project: atlas",
		"Project: beacon",
		atlas.RequestCommit + ":" + atlas.RequestPath,
		beacon.RequestCommit + ":" + beacon.RequestPath,
		"Opaque {{.Worktree}} body must stay data, not template source.",
		atlas.ApplyCommand,
		beacon.ApplyCommand,
		"--route <implement|watchdog|supersede>",
		"--answer <human-answer-file>",
		"keep each request's identity",
		"never invented, guessed, or filled in by inference",
		"record no decision",
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

// TestDecisionEmptyIsNotUnavailable proves a readable empty inbox, a filtered
// empty inbox, and an unresolvable ledger render as distinct outcomes.
func TestDecisionEmptyIsNotUnavailable(t *testing.T) {
	empty, err := BuildPacket("decision", InvocationFacts{Decision: &DecisionFacts{Status: DecisionEmpty}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty.Instructions, "empty inbox") || !strings.Contains(empty.Instructions, "creates no work") {
		t.Errorf("empty inbox is unclear:\n%s", empty.Instructions)
	}
	if strings.Contains(empty.Instructions, "could not be resolved or read") {
		t.Error("empty inbox was rendered as an unavailable ledger")
	}

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
	if !strings.Contains(filtered.Instructions, "not recorded in the ledger is reported as unavailable") {
		t.Errorf("filtered empty inbox can silently broaden scope:\n%s", filtered.Instructions)
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
		"not an empty inbox",
		"ledger path is not a usable local Git clone",
		"restore the configured ledger clone and retry",
		"$XDG_CONFIG_HOME/skl/config.json",
	} {
		if !strings.Contains(unavailable.Instructions, want) {
			t.Errorf("unavailable ledger is missing %q:\n%s", want, unavailable.Instructions)
		}
	}
	if strings.Contains(unavailable.Instructions, "empty inbox: it creates no work") {
		t.Error("unavailable ledger was rendered as an empty inbox")
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
	if !strings.Contains(body, "not a successful whole-group decision") {
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
	if strings.Contains(body, "Every selected item below is resolved by one committed answer") {
		t.Error("partial result also rendered the fully applied headline")
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
		"No selected direction was recorded",
		"the selected request was replaced even though its question text repeated",
		"collect renewed human direction against the current request",
		"renewed proposal and re-slicing",
		"never amends the Contract",
		"or authorizes new work",
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
		"was not retired while active or claimed work remains",
		"nothing was released, merged, or silently abandoned",
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
		"reports partial delivery, not all-delivered completion",
		"dependents stay blocked",
		"no archive move, dependency remapping, forge completion observation, or source deletion",
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

// TestDecisionTriageResourceAndInstallation proves the triage rules are
// retrievable as a static named resource and that installation publishes the
// decision stub without authored content duplicating into harnesses.
func TestDecisionTriageResourceAndInstallation(t *testing.T) {
	resource, err := RenderResource("decision", "reference/triage.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"grouped when they concern the same rule",
		"Keep each request's identity, exact references, and item-specific differences intact",
		"do not invent it",
		"not authorization",
		"do not demand a ceremonial second confirmation",
		"applied`, `already_applied`, `refused`, or `unresolved",
		"never claim the whole group succeeded",
		"renewed proposal and re-slicing",
		"reports partial delivery rather than all-delivered completion",
		"A Superseded blocker is not Merged",
	} {
		if !strings.Contains(string(resource), want) {
			t.Errorf("triage resource is missing %q", want)
		}
	}
	description, err := DescribeResourceInputs("decision", "reference/triage.md")
	if err != nil {
		t.Fatal(err)
	}
	if description != "reference/triage.md accepts no inputs.\n" {
		t.Errorf("triage resource unexpectedly parameterized: %q", description)
	}

	home := t.TempDir()
	if _, err := Install(home); err != nil {
		t.Fatal(err)
	}
	stubPath := filepath.Join(home, ".pi/agent/skills/decision/SKILL.md")
	stub, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stub), "skl skill decision") || !strings.Contains(string(stub), "skl.stub/v1") {
		t.Errorf("decision stub does not delegate to the CLI:\n%s", stub)
	}
	if !strings.HasPrefix(string(stub), "---\nname: decision\n") {
		t.Errorf("decision stub lost its frontmatter:\n%s", stub)
	}
}
