package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
)

func TestDecisionExplicitEmptyProjectRefuses(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source := decisionAccept(t, forge, "acme", "widgets", singleSlice("empty-filter"))
	cli := decisionReadOnlyApp(t)
	decisionPauseNext(t, cli, source)
	before := ledgerSnapshot(t, fixture.clone)
	out := cli.decisionJSON(t, "skl", "decision", "inbox", "--project=", "--format", "json")
	if out.Status != string(skilldist.DecisionRefused) || len(out.Facts.Requests) != 0 {
		t.Fatalf("explicit empty filter broadened scope: %#v", out)
	}
	if ledgerSnapshot(t, fixture.clone) != before {
		t.Fatal("refused filter mutated the ledger")
	}
}

func TestDecisionCoupledDuplicateItemsRefuse(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source := decisionAccept(t, forge, "acme", "widgets", singleSlice("duplicate-work"))
	cli := decisionReadOnlyApp(t)
	item, _ := decisionPauseNext(t, cli, source)
	request := decisionRequest(t, cli, "widgets", item)
	batch := filepath.Join(t.TempDir(), "decisions.json")
	writeFile(t, batch, mustJSON(t, []ledger.DecisionInput{
		{Project: "widgets", Item: item, Request: decisionReference(request), Answer: "continue\n", Route: ledger.RouteImplement},
		{Project: "widgets", Item: item, Request: decisionReference(request), Answer: "abandon\n", Route: ledger.RouteSupersede},
	}))
	before := ledgerSnapshot(t, fixture.clone)
	out := cli.decisionJSON(t, "skl", "decision", "apply", "--input", batch, "--coupled", "--format", "json")
	if out.Status != string(skilldist.DecisionRefused) || len(out.Facts.Outcomes) != 2 {
		t.Fatalf("conflicting coupled entries were not refused: %#v", out)
	}
	for _, result := range out.Facts.Outcomes {
		if result.Status != "unresolved" || !strings.Contains(result.Reason, "duplicate") {
			t.Fatalf("duplicate result claimed an effect: %#v", result)
		}
	}
	if ledgerSnapshot(t, fixture.clone) != before {
		t.Fatal("duplicate coupled inputs changed the ledger")
	}
}

func TestDecisionBatchReportsEarlierCommitAfterWriteFailure(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	widgets := decisionAccept(t, forge, "acme", "widgets", singleSlice("first-work"))
	gadgets := decisionAccept(t, forge, "acme", "gadgets", singleSlice("second-work"))
	cli := decisionReadOnlyApp(t)
	widgetItem, _ := decisionPauseNext(t, cli, widgets)
	gadgetItem, _ := decisionPauseNext(t, cli, gadgets)
	widgetRequest := decisionRequest(t, cli, "widgets", widgetItem)
	gadgetRequest := decisionRequest(t, cli, "gadgets", gadgetItem)
	batch := filepath.Join(t.TempDir(), "decisions.json")
	writeFile(t, batch, mustJSON(t, []ledger.DecisionInput{
		{Project: "widgets", Item: widgetItem, Request: decisionReference(widgetRequest), Answer: "continue first\n", Route: ledger.RouteImplement},
		{Project: "gadgets", Item: gadgetItem, Request: decisionReference(gadgetRequest), Answer: "continue second\n", Route: ledger.RouteImplement},
	}))
	hook := filepath.Join(fixture.clone, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nif git diff --cached --name-only | grep -q 'projects/gadgets/'; then exit 1; fi\n"), 0755); err != nil {
		t.Fatal(err)
	}
	out := cli.decisionJSON(t, "skl", "decision", "apply", "--input", batch, "--format", "json")
	if out.Status != string(skilldist.DecisionPartial) || len(out.Results) != 2 || out.Results[0].Status != ledger.DecisionApplied || out.Results[1].Status != "unresolved" {
		t.Fatalf("lost per-item outcomes after a later write failed: %#v", out)
	}
	if out.Results[1].Refusal == "" {
		t.Fatal("unresolved item has no failure reason")
	}
	if state := decisionState(t, fixture.clone, decisionItemStatePath("widgets", "first-work", "foundation")); state.State != ledger.ReadyForImplementation || !state.Decision {
		t.Fatalf("first result was not committed: %#v", state)
	}
	if state := decisionState(t, fixture.clone, decisionItemStatePath("gadgets", "second-work", "foundation")); state.State != ledger.NeedsHuman || state.Decision {
		t.Fatalf("failed result changed paused state: %#v", state)
	}
	if dirty := deliveryTrimmed(t, fixture.clone, "status", "--porcelain"); dirty != "" {
		t.Fatalf("failed write left partial files: %s", dirty)
	}
}

func TestDecisionRetrySurvivesCompetingReplication(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source := decisionAccept(t, forge, "acme", "widgets", singleSlice("competing-decision"))
	cli := decisionReadOnlyApp(t)
	item, _ := decisionPauseNext(t, cli, source)
	request := decisionRequest(t, cli, "widgets", item)
	competitor := filepath.Join(t.TempDir(), "competitor")
	runGit(t, t.TempDir(), "clone", "-q", fixture.upstream, competitor)
	runGit(t, competitor, "config", "user.name", "Other")
	runGit(t, competitor, "config", "user.email", "other@example.com")
	writeFile(t, filepath.Join(competitor, "other.txt"), "competing history\n")
	runGit(t, competitor, "add", "other.txt")
	runGit(t, competitor, "commit", "-q", "-m", "competing ledger history")
	runGit(t, competitor, "push", "-q", "origin", "main")
	answer := "continue despite offline replication\n"
	applied := decisionApplyAnswer(t, cli, request, ledger.RouteImplement, answer)
	if applied.Status != ledger.DecisionApplied || applied.Results[0].Replication == nil || applied.Results[0].Replication.Status != ledger.PushReconciliation {
		t.Fatalf("local result did not discover competing replication: %#v", applied)
	}
	before := ledgerSnapshot(t, fixture.clone)
	retried := decisionApplyAnswer(t, cli, request, ledger.RouteImplement, answer)
	if len(retried.Results) != 1 || retried.Results[0].Status != ledger.DecisionAlreadyApplied {
		t.Fatalf("competing replication hid committed result: %#v", retried)
	}
	batch := filepath.Join(t.TempDir(), "retry.json")
	writeFile(t, batch, mustJSON(t, []ledger.DecisionInput{{
		Project: request.Project, Item: request.Item, Request: decisionReference(request), Answer: answer, Route: ledger.RouteImplement,
	}}))
	coupled := cli.decisionJSON(t, "skl", "decision", "apply", "--input", batch, "--coupled", "--format", "json")
	if len(coupled.Results) != 1 || coupled.Results[0].Status != ledger.DecisionAlreadyApplied {
		t.Fatalf("coupled retry hid committed result: %#v", coupled)
	}
	if ledgerSnapshot(t, fixture.clone) != before {
		t.Fatal("exact retry mutated competing history")
	}
}

func TestDecisionRetirementRefusesAllMerged(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	decisionAccept(t, forge, "acme", "widgets", singleSlice("delivered-work"))
	decisionCommitState(t, fixture.clone, decisionItemStatePath("widgets", "delivered-work", "foundation"), func(state *ledger.SliceState) {
		state.State = ledger.Merged
	})
	cli := decisionReadOnlyApp(t)
	before := ledgerSnapshot(t, fixture.clone)
	out := cli.decisionJSON(t, "skl", "decision", "retire", "--project", "widgets", "--proposal", "delivered-work", "--format", "json")
	if out.Status != string(skilldist.DecisionRefused) || !strings.Contains(out.Facts.Retirement.Reason, "no Superseded") {
		t.Fatalf("all-delivered proposal was retired as abandoned work: %#v", out)
	}
	text, err := cli.deliveryRun(t, "skl", "decision", "retire", "--project", "widgets", "--proposal", "delivered-work")
	if err != nil || !strings.Contains(text, "no Superseded") || strings.Contains(text, "The parent is retired.") {
		t.Fatalf("retirement transports disagree: %s / %v", text, err)
	}
	if ledgerSnapshot(t, fixture.clone) != before {
		t.Fatal("all-Merged refusal changed the proposal")
	}
}
