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

	// An explicit standard mode renders exactly what the default renders.
	plain := cli.dispatchJSON(t, "skl implement next --repo "+source)
	cli.dispatchRun(t, delivery(plain).ReleaseCommand)
	explicit := cli.dispatchJSON(t, "skl implement next --repo "+source+" --mode standard")
	sameRendering(t, explicit.Packet.Instructions, plain.Packet.Instructions)
	cli.dispatchRun(t, delivery(explicit).ReleaseCommand)

	// Standard mode: a reviewer value renders once, an empty value counts as
	// omitted, and nothing about helpers renders.
	started := cli.dispatchJSON(t, "skl implement next --repo "+source+" --reviewer-model openai-codex/gpt-6-sol --reviewer-thinking ''")
	facts := delivery(started)
	if facts.Mode != skilldist.StandardMode || facts.Helper != nil || !reflect.DeepEqual(facts.Reviewer, &skilldist.SubagentChoice{Model: "openai-codex/gpt-6-sol"}) {
		t.Fatalf("standard facts = %s", mustJSON(t, facts))
	}
	// Beside the bound commands that carry it, the value renders once.
	prose := started.Packet.Instructions
	for _, command := range []string{facts.PrepareCommand, facts.InspectCommand, facts.ResumeCommand} {
		prose = strings.ReplaceAll(prose, command, "")
	}
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

	// Team mode without a helper value renders no helper sentence.
	bare := cli.dispatchJSON(t, "skl implement next --repo "+source+" --mode team")
	if !strings.Contains(bare.Packet.Instructions, "### Split the work") || strings.Contains(bare.Packet.Instructions, "Run each implementer") {
		t.Errorf("team mode without a helper value:\n%s", bare.Packet.Instructions)
	}
	cli.dispatchRun(t, delivery(bare).ReleaseCommand)

	// Team mode: a thinking value alone renders, and the commands the
	// Execution Skill binds (prepare, the inspection it prints, and resume)
	// carry the same choices.
	started = cli.dispatchJSON(t, "skl implement next --repo "+source+" --mode team --helper-model openai-codex/gpt-6-luna --helper-thinking xhigh --reviewer-thinking high")
	team := delivery(started)
	if !strings.Contains(started.Packet.Instructions, "at `high` thinking") {
		t.Errorf("the reviewer thinking level does not render alone:\n%s", started.Packet.Instructions)
	}
	want := skilldist.DeliveryFacts{Mode: skilldist.TeamMode, Helper: &skilldist.SubagentChoice{Model: "openai-codex/gpt-6-luna", Thinking: "xhigh"}, Reviewer: &skilldist.SubagentChoice{Thinking: "high"}}
	prepared := delivery(cli.dispatchJSON(t, team.PrepareCommand))
	inspected := delivery(cli.dispatchJSON(t, strings.ReplaceAll(prepared.InspectCommand, "<observed-target-sha>", "HEAD")))
	for name, got := range map[string]*skilldist.DeliveryFacts{"next": team, "prepare": prepared, "inspect": inspected, "resume": delivery(cli.dispatchJSON(t, inspected.ResumeCommand))} {
		if got.Mode != want.Mode || !reflect.DeepEqual(got.Helper, want.Helper) || !reflect.DeepEqual(got.Reviewer, want.Reviewer) || got.ResumeCommand != team.ResumeCommand {
			t.Errorf("team %s facts = %s, want %s", name, mustJSON(t, got), mustJSON(t, want))
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
