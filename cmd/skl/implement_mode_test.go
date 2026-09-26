package main

// Implement mode and subagent choices at the CLI seam, against real ledger and
// source fixtures. Expected values come from the add-implement-team-mode
// behavior: an unknown mode is refused before any Claim, opaque values render
// only where they apply, and supplied values carry through resume.

import (
	"reflect"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
)

func TestImplementNextRefusesAnInvalidModeBeforeClaiming(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)
	before := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")

	for _, invalid := range []string{"--mode swarm", "--helper-model openai-codex/gpt-6-luna", "--mode standard --helper-thinking xhigh"} {
		out := cli.dispatchJSON(t, "skl implement next --repo "+source+" "+invalid)
		if out.Status != "fix_required" || out.Execution != nil {
			t.Errorf("next %s = %s, want a refusal without a Claim", invalid, mustJSON(t, out))
		}
	}
	if after := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD"); after != before {
		t.Fatalf("a refused mode changed the ledger: %s -> %s", before, after)
	}
	if state := deliveryPersistedState(t, fixture.clone); state.Claim != nil {
		t.Fatalf("a refused mode claimed the Slice: %#v", state.Claim)
	}
}

func TestImplementChoicesRenderWhereTheyApplyAndCarryThroughResume(t *testing.T) {
	newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	cli := deliveryNoForgeApp(t)
	delivery := func(out deliveryOutput) *skilldist.DeliveryFacts {
		t.Helper()
		if out.Packet == nil || out.Packet.Facts.Delivery == nil {
			t.Fatalf("no Execution Skill: %s", mustJSON(t, out))
		}
		return out.Packet.Facts.Delivery
	}

	// Standard mode: a reviewer value renders once, an empty value counts as
	// omitted, and nothing about helpers renders.
	started := cli.dispatchJSON(t, "skl implement next --repo "+source+" --reviewer-model openai-codex/gpt-6-sol --reviewer-thinking ''")
	facts := delivery(started)
	if facts.Mode != skilldist.StandardMode || facts.Helper != nil || !reflect.DeepEqual(facts.Reviewer, &skilldist.SubagentChoice{Model: "openai-codex/gpt-6-sol"}) {
		t.Fatalf("standard facts = %s", mustJSON(t, facts))
	}
	// Beside the resume command that carries it, the value renders once.
	prose := strings.ReplaceAll(started.Packet.Instructions, facts.ResumeCommand, "")
	if count := strings.Count(prose, "openai-codex/gpt-6-sol"); count != 1 {
		t.Errorf("the reviewer model renders %d times, want once in the Audit dispatch step", count)
	}
	if strings.Contains(facts.ResumeCommand, "--mode") || strings.Contains(facts.ResumeCommand, "--reviewer-thinking") {
		t.Errorf("resume carries an unsupplied choice: %s", facts.ResumeCommand)
	}
	resumed := delivery(cli.dispatchJSON(t, facts.ResumeCommand))
	if resumed.Mode != facts.Mode || !reflect.DeepEqual(resumed.Reviewer, facts.Reviewer) || resumed.Helper != nil {
		t.Fatalf("standard resume lost its choices: %s", mustJSON(t, resumed))
	}
	cli.dispatchRun(t, facts.ReleaseCommand)

	// Team mode: the bound resume command renders the same choices again.
	team := delivery(cli.dispatchJSON(t, "skl implement next --repo "+source+" --mode team --helper-model openai-codex/gpt-6-luna --helper-thinking xhigh --reviewer-thinking high"))
	want := skilldist.DeliveryFacts{Mode: skilldist.TeamMode, Helper: &skilldist.SubagentChoice{Model: "openai-codex/gpt-6-luna", Thinking: "xhigh"}, Reviewer: &skilldist.SubagentChoice{Thinking: "high"}}
	for _, got := range []*skilldist.DeliveryFacts{team, delivery(cli.dispatchJSON(t, team.ResumeCommand))} {
		if got.Mode != want.Mode || !reflect.DeepEqual(got.Helper, want.Helper) || !reflect.DeepEqual(got.Reviewer, want.Reviewer) {
			t.Errorf("team facts = %s, want %s", mustJSON(t, got), mustJSON(t, want))
		}
	}
	cli.dispatchRun(t, team.ReleaseCommand)

	// A dispatched worker receives the choices its Dispatch was given.
	dispatch := cli.dispatched(t, "skl implement next --dispatch --repo "+source+" --mode team --reviewer-model openai-codex/gpt-6-sol")
	worker := delivery(cli.dispatchJSON(t, dispatch.Worker))
	if worker.Mode != skilldist.TeamMode || !reflect.DeepEqual(worker.Reviewer, &skilldist.SubagentChoice{Model: "openai-codex/gpt-6-sol"}) {
		t.Fatalf("dispatched worker facts = %s", mustJSON(t, worker))
	}
}
