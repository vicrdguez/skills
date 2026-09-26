package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

func TestHandoffOutcomeRendersIdenticallyEveryTime(t *testing.T) {
	result := &ledger.DeliveryResult{
		Status: ledger.AwaitingReview, Item: "widget/foundation",
		Report:      ledger.Reference{Commit: strings.Repeat("a", 40), Path: "projects/widgets/proposals/widget/foundation/implement-report.md"},
		Replication: &ledger.PublicationNote{Status: ledger.PushPending, Detail: "remote unavailable"},
		Publication: &ledger.PublicationNote{Status: ledger.IssuePending, Detail: "forge unavailable"},
	}
	out := handoffOutput(ledger.ImplementPhase, setup.RepositoryContext{Root: "/work/widgets", Remote: "origin"}, result)
	var first string
	for range 50 {
		var rendered bytes.Buffer
		if err := renderDelivery(&rendered, formatMarkdown, ledger.ImplementPhase, "submit", out); err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = rendered.String()
			replication, publication := strings.Index(first, "Ledger replication: pending"), strings.Index(first, "Public presentation: pending")
			if replication < 0 || publication < replication {
				t.Fatalf("ledger replication must precede public presentation:\n%s", first)
			}
		} else if rendered.String() != first {
			t.Fatalf("handoff outcome changed between renders:\n%s\n---\n%s", first, rendered.String())
		}
	}
}

// Outcome branches the golden journey does not reach render their bound
// commands and branch markers.
func TestUnreachedOutcomeBranchesRender(t *testing.T) {
	repository := setup.RepositoryContext{Root: "/work/widgets", Remote: "origin"}
	claim := strings.Repeat("c", 40)
	failed := renderingFailedOutput(ledger.ImplementPhase, repository, "widget/foundation", claim, errors.New("template broke"))
	var encoded bytes.Buffer
	if err := renderDelivery(&encoded, formatJSON, ledger.ImplementPhase, "next", failed); err != nil {
		t.Fatal(err)
	}
	var decoded deliveryOutput
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != "fix_required" || decoded.Reason != "Claim "+claim+" remains acquired for widget/foundation, but execution rendering failed: template broke" || decoded.Repair != deliveryJSONRepair {
		t.Errorf("rendering failure JSON changed: %s", &encoded)
	}
	result := ledger.CurrentResult{Item: "widget/foundation", Lifecycle: ledger.AwaitingReview, Phase: ledger.ImplementPhase, Outcome: ledger.AwaitingReview, Branch: "widget"}
	guidance := &presentationGuidance{Result: result, Evidence: []string{"skl ledger show --commit 'a' --path 'r'"}, Authoring: "skl skill --resource pull-presentation.md implement", Continue: "skl ledger present --item 'widget/foundation' --public-body <file>"}
	archive := &ledger.ArchiveResult{Archived: []ledger.ArchivedProposal{{Proposal: "done", FullyDelivered: true, Commit: "abc"}}}
	cases := []struct {
		name, kind string
		facts      any
		want       []string
	}{
		{"rendering failed", failed.kind, failed.facts, []string{"`skl implement resume --repo '/work/widgets' --remote 'origin' --item 'widget/foundation' --claim '" + claim + "'`", "`skl implement release --repo", "template broke"}},
		{"refusal with uncertain Claim", "refused", refusalFacts{Status: "fix_required", Reason: "ledger commit failed", ClaimState: claimUncertain, StatusCommand: statusInvocation(repository), Rerun: "skl implement next"}, []string{"may have been acquired", "`skl status --repo '/work/widgets' --remote 'origin'`", "`skl implement next`"}},
		{"presented", "ledger-present", presentFacts{Status: ledger.PullPresented, Result: &result, Notes: []noteFact{{Label: "Public presentation", Status: ledger.PullPresented, Detail: "pull request #21"}}}, []string{"is presented", "pull request #21", "You are done"}},
		{"presentation uncertain", "ledger-present", presentFacts{Status: ledger.IssueUncertain, Result: &result, Guidance: guidance}, []string{"may or may not", "Check the forge", "`" + guidance.Continue + "`"}},
		{"cleanup completed", "cleanup", cleanupFacts{cleanupOutcome{Status: "completed", Archive: archive}, "skl propose cleanup"}, []string{"Archived proposal: done (fully delivered)", "Cleanup is complete"}},
		{"cleanup needs repair", "cleanup", cleanupFacts{cleanupOutcome{Status: "fix_required", SourceRepair: &ledgerOutcome{Reason: "branch busy", Repair: "free it"}}, "skl propose cleanup --repo '/work/widgets'"}, []string{"Source cleanup refused: branch busy", "Repair: free it", "`skl propose cleanup --repo '/work/widgets'`"}},
		{"acceptance with pending issue", "ledger-accept", proposalFacts{Status: "accepted", Proposal: "widget", Unpublished: true, Publish: "skl ledger publish --repo '/work/widgets' --remote 'origin' --proposal 'widget'"}, []string{"`skl ledger publish --repo '/work/widgets' --remote 'origin' --proposal 'widget'`"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rendered strings.Builder
			if err := writeOutcome(&rendered, tc.kind, tc.facts); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(rendered.String(), want) {
					t.Errorf("%s lacks %q:\n%s", tc.kind, want, rendered.String())
				}
			}
		})
	}
}
