package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestStatusObservesHumanMergeIntoProposalBranch(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("branch-status"))); accepted.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, accepted))
	}
	attachFixture(t, fixture.clone, "branch-status")
	statusRecord(t, fixture.clone, "branch-status", "foundation", func(state *ledger.SliceState) {
		state.Target.Branch = "proposal/branch-status"
	})
	app, output := completionStatusApp(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "proposal/branch-status"))
	})
	observed := runCompletionStatus(t, app, output, root, "--item", "branch-status/foundation")
	if len(observed.Items) != 1 || observed.Items[0].State != ledger.Merged || observed.Items[0].Completion == nil || observed.Items[0].Completion.Target.Branch != "proposal/branch-status" {
		t.Fatalf("branch merge observation = %+v", observed)
	}
}

func TestAutoDispatchWorkerResumesRecordedTargetAndContinuationKeepsAuto(t *testing.T) {
	newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	app := newLedgerApp(t, newForgeServer(t))
	if accepted := app.accept(t, source, writeProposal(t, "", dualSlice(deliveryTestProposal))); accepted.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, accepted))
	}
	dispatch := app.dispatched(t, "skl implement next --auto --dispatch --repo "+source)
	worker := app.dispatchJSON(t, dispatch.Worker)
	if worker.Status != ledger.WorkAvailable || worker.Execution == nil || worker.Packet == nil {
		t.Fatalf("dispatched worker = %s, want an Execution Skill", mustJSON(t, worker))
	}
	if worker.Execution.Item != dispatch.Item || worker.Execution.Claim.Commit != dispatch.Claim {
		t.Fatalf("worker resumed a different Claim: %s", mustJSON(t, worker))
	}
	if target := worker.Execution.State.Target; target == nil || target.Branch != "proposal/"+deliveryTestProposal {
		t.Fatalf("worker target = %+v, want the recorded Proposal Branch", target)
	}
	if !slices.Contains(shellWords(t, dispatch.Continue), "--auto") {
		t.Fatalf("continuation lost Auto Mode: %s", dispatch.Continue)
	}
	if ending, status := app.continued(t, dispatch.Continue); ending != ledger.ClaimHeld || status != "stopped" {
		t.Fatalf("continuation with held Claim = %s, %s", ending, status)
	}
}

func TestImplementAutoFlagAppliesToNextAndDispatchOnly(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	app := newLedgerApp(t, newForgeServer(t))
	if accepted := app.accept(t, source, writeProposal(t, "", dualSlice(deliveryTestProposal))); accepted.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, accepted))
	}
	before := runGitOutput(t, fixture.clone, "rev-parse", "HEAD")
	if _, err := app.deliveryJSON(t, "skl", "implement", "resume", "--auto", "--repo", source, "--item", deliveryTestItem, "--claim", before, "--format", "json"); err == nil || !strings.Contains(err.Error(), "auto") {
		t.Fatalf("resume --auto error = %v; want flag refusal", err)
	}
	if after := runGitOutput(t, fixture.clone, "rev-parse", "HEAD"); after != before {
		t.Fatal("refused resume flag mutated the ledger")
	}
	out, err := app.deliveryJSON(t, "skl", "implement", "next", "--auto", "--repo", source, "--format", "json")
	if err != nil || out.Execution == nil || out.Execution.State.Target == nil || out.Execution.State.Target.Branch != "proposal/"+deliveryTestProposal || out.Status != ledger.WorkAvailable {
		t.Fatalf("Auto Mode next = %+v, %v", out, err)
	}
}

// proposalPullForge serves the Slice pull request #21 merged into the
// Proposal Branch and a Proposal Submission pull request the engine creates.
type proposalPullForge struct {
	mu       sync.Mutex
	proposal map[string]any
	created  int
	asDraft  any
	state    string
	merged   bool
}

func (f *proposalPullForge) serve(t *testing.T) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/repos/acme/widgets/pulls/21":
			_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "proposal/branch-status"))
		case r.URL.Path == "/repos/acme/widgets/pulls" && r.Method == http.MethodGet:
			if f.proposal == nil {
				_, _ = fmt.Fprint(w, "[]")
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{f.proposal})
		case r.URL.Path == "/repos/acme/widgets/pulls" && r.Method == http.MethodPost:
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			f.created++
			f.asDraft = payload["draft"]
			f.state = "open"
			f.proposal = map[string]any{"number": 50, "node_id": "PR_50", "state": "open", "merged": false, "draft": payload["draft"], "title": payload["title"], "body": payload["body"],
				"head": map[string]any{"ref": payload["head"], "sha": "branch-tip", "repo": map[string]string{"full_name": "acme/widgets"}},
				"base": map[string]any{"ref": payload["base"], "repo": map[string]string{"full_name": "acme/widgets"}}}
			_ = json.NewEncoder(w).Encode(f.proposal)
		case r.URL.Path == "/graphql" && f.proposal != nil:
			var payload struct{ Query string }
			_ = json.NewDecoder(r.Body).Decode(&payload)
			f.proposal["draft"] = strings.Contains(payload.Query, "convertPullRequestToDraft")
			_, _ = fmt.Fprint(w, `{"data":{}}`)
		case r.URL.Path == "/repos/acme/widgets/pulls/50" && f.proposal != nil:
			if r.Method == http.MethodPatch {
				var payload map[string]any
				_ = json.NewDecoder(r.Body).Decode(&payload)
				f.proposal["body"] = payload["body"]
			}
			f.proposal["state"] = f.state
			f.proposal["merged"] = f.merged
			if f.merged {
				f.proposal["merged_at"] = "2026-01-02T00:00:00Z"
				f.proposal["merge_commit_sha"] = "proposal-merge"
			}
			_ = json.NewEncoder(w).Encode(f.proposal)
		default:
			t.Errorf("unexpected forge request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}
}

func openBranchStatusFixture(t *testing.T) (*ledgerFixture, string) {
	t.Helper()
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("branch-status"))); accepted.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, accepted))
	}
	attachFixture(t, fixture.clone, "branch-status")
	statusRecord(t, fixture.clone, "branch-status", "foundation", func(state *ledger.SliceState) {
		state.Target.Branch = "proposal/branch-status"
	})
	path := filepath.Join(fixture.clone, "projects/widgets/proposals/branch-status/proposal.json")
	var meta ledger.ProposalMeta
	if err := json.Unmarshal([]byte(readLedgerFile(t, fixture.clone, "projects/widgets/proposals/branch-status/proposal.json")), &meta); err != nil {
		t.Fatal(err)
	}
	meta.Branch = "proposal/branch-status"
	meta.Target = &ledger.IntegrationTarget{Repository: "acme/widgets", Branch: meta.Branch}
	contents, _ := json.Marshal(meta)
	writeFile(t, path, string(contents)+"\n")
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "open Proposal Branch")
	return fixture, root
}

func TestStatusPresentsProposalSubmissionAndObservesItsMerge(t *testing.T) {
	_, root := openBranchStatusFixture(t)
	forge := &proposalPullForge{}
	app, output := completionStatusApp(t, forge.serve(t))
	first := runCompletionStatus(t, app, output, root)
	if len(first.Proposals) != 1 || first.Proposals[0].Submission == nil || first.Proposals[0].Submission.Number != 50 || first.Proposals[0].BranchState != ledger.BranchOpen || first.Proposals[0].FullyDelivered {
		t.Fatalf("first status = %s", mustJSON(t, first))
	}
	// Created as a draft, then readied because its only Slice is Merged.
	if forge.created != 1 || forge.proposal["base"].(map[string]any)["ref"] != "main" || forge.asDraft != true || forge.proposal["draft"] != false {
		t.Fatalf("Proposal pull request = %+v", forge.proposal)
	}
	if body, _ := forge.proposal["body"].(string); !strings.Contains(body, "Merged into `proposal/branch-status`, acme/widgets#21") {
		t.Fatalf("Proposal body = %q", body)
	}

	forge.mu.Lock()
	forge.state, forge.merged = "closed", true
	forge.mu.Unlock()
	output.Reset()
	if err := app.Run([]string{"skl", "status", "--repo", root}); err != nil {
		t.Fatalf("status: %v (%s)", err, output)
	}
	text := output.String()
	for _, want := range []string{"fully delivered: true", "Proposal Branch: proposal/branch-status (spent)", "Proposal Submission: acme/widgets#50", "Proposal completion: into acme/widgets main at proposal-merge"} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text lacks %q:\n%s", want, text)
		}
	}
	if forge.created != 1 {
		t.Fatalf("Proposal pull requests created = %d", forge.created)
	}
	browse, browsed := browseApp(t)
	if err := browse.Run([]string{"skl", "browse", "proposal", "--project", "widgets", "--proposal", "branch-status"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Proposal Branch: proposal/branch-status (spent)", "Proposal Submission: acme/widgets#50 (merged into main at proposal-merge)", "Merged into: proposal/branch-status"} {
		if !strings.Contains(browsed.String(), want) {
			t.Fatalf("browse lacks %q:\n%s", want, browsed)
		}
	}
}

func TestStatusReportsClosedProposalSubmissionWithoutRecording(t *testing.T) {
	fixture, root := openBranchStatusFixture(t)
	forge := &proposalPullForge{}
	app, output := completionStatusApp(t, forge.serve(t))
	runCompletionStatus(t, app, output, root)
	before := runGitOutput(t, fixture.clone, "rev-parse", "HEAD")
	forge.mu.Lock()
	forge.state = "closed"
	forge.mu.Unlock()
	output.Reset()
	if err := app.Run([]string{"skl", "status", "--repo", root}); err != nil {
		t.Fatalf("status: %v (%s)", err, output)
	}
	if !strings.Contains(output.String(), "Proposal Submission closed without merge") || !strings.Contains(output.String(), "Proposal Branch: proposal/branch-status (open)") {
		t.Fatalf("closed status:\n%s", output)
	}
	if after := runGitOutput(t, fixture.clone, "rev-parse", "HEAD"); after != before {
		t.Fatal("closed Proposal Submission changed the ledger")
	}
}

// A human who merges the Proposal Submission and retires before any status
// observation is refused; the next status records the completion.
func TestDecisionRetireObservesMergedProposalSubmission(t *testing.T) {
	_, root := openBranchStatusFixture(t)
	forge := &proposalPullForge{}
	app, output := completionStatusApp(t, forge.serve(t))
	runCompletionStatus(t, app, output, root)
	forge.mu.Lock()
	forge.state, forge.merged = "closed", true
	forge.mu.Unlock()
	output.Reset()
	if err := app.Run([]string{"skl", "decision", "retire", "--project", "widgets", "--proposal", "branch-status", "--format", "json"}); err != nil {
		t.Fatalf("decision retire: %v (%s)", err, output)
	}
	if text := output.String(); !strings.Contains(text, `"status":"refused"`) || !strings.Contains(text, "has merged") {
		t.Fatalf("retirement of a merged Proposal Submission:\n%s", text)
	}
	status := runCompletionStatus(t, app, output, root)
	if len(status.Proposals) != 1 || status.Proposals[0].Completion == nil || status.Proposals[0].Completion.MergeCommit != "proposal-merge" || !status.Proposals[0].FullyDelivered {
		t.Fatalf("status after refused retirement = %s", mustJSON(t, status))
	}
}
