package skills

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// deliveryWatchdogFacts is one fully bound private-ledger review invocation: an
// acquired Claim, every engine-bound command, the fixed reviewed head and
// target, and the frozen Contract plus consumed report documents.
func deliveryWatchdogFacts(operation, procedure, scope string, reviewCount uint64) *DeliveryFacts {
	claim := strings.Repeat("c", 40)
	reviewed := strings.Repeat("a", 40)
	target := strings.Repeat("b", 40)
	previous := strings.Repeat("d", 40)
	facts := &DeliveryFacts{
		Phase:           "watchdog",
		Procedure:       procedure,
		Operation:       operation,
		Repository:      "payments",
		Remote:          "origin",
		Item:            "add-refunds/refund",
		Branch:          "add-refunds",
		Worktree:        "/tmp/add-refunds",
		ResultDirectory: "/tmp/result",
		Claim:           claim,
		PrepareCommand:  "skl watchdog prepare --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'",
		InspectCommand:  "skl watchdog inspect --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'",
		ResumeCommand:   "skl watchdog resume --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'",
		ReleaseCommand:  "skl watchdog release --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'",
		SubmitCommand: "skl watchdog submit --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'" +
			" --body '/tmp/result/watchdog-report.md' --public-body '/tmp/result/public.md' --outcome <pass|rework|needs-human>",
		PauseCommand: "skl watchdog submit --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'" +
			" --body '/tmp/result/watchdog-report.md' --public-body '/tmp/result/public.md' --outcome needs-human",
		ResultResourceCommand: "skl skill --resource reference/ledger-review.md --input result_directory='/tmp/result'" +
			" --input round=" + fmt.Sprint(reviewCount+1) + " --input reviewed_head='" + reviewed + "' watchdog",
		RequiredHead:     reviewed,
		RecordedTarget:   target,
		PreviousReviewed: previous,
		SourceHead:       reviewed,
		SourceTarget:     target,
		ReviewScope:      scope,
		FetchStatus:      "local: origin fetch failed (offline); using available local source inputs",
		ReviewCount:      reviewCount,
		ReviewNumber:     reviewCount + 1,
		Documents: []ledger.ContractDocument{
			{
				Commit:   strings.Repeat("1", 40),
				Path:     "projects/payments/proposals/add-refunds/refund/behavior.md",
				Contents: "# Refund behavior\n\nOpaque {{.Worktree}} contract body stays data.\n",
			},
			{
				Commit:   strings.Repeat("2", 40),
				Path:     "projects/payments/proposals/add-refunds/refund/implement-report.md",
				Contents: "# Implementation report\n\nOpaque {{.RequiredHead}} completion table stays data.\n",
			},
		},
	}
	if reviewCount == 0 {
		facts.PreviousReviewed = ""
	}
	return facts
}

// TestDeliveryWatchdogBindsEachProceeding proves the private-ledger path
// binds every command and document once and carries the fixed review identity
// for the initial, repeat, and resumed review.
func TestDeliveryWatchdogBindsEachProceeding(t *testing.T) {
	cases := []struct {
		name      string
		operation string
		procedure string
		scope     string
		count     uint64
	}{
		{"initial", "next", "initial", "full", 0},
		{"repeat", "next", "initial", "incremental", 1},
		{"resume", "resume", "resumed", "incremental", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := deliveryWatchdogFacts(tc.operation, tc.procedure, tc.scope, tc.count)
			packet, err := BuildPacket("watchdog", InvocationFacts{Delivery: facts})
			if err != nil {
				t.Fatal(err)
			}
			if len(packet.IncludedSkills) != 0 {
				t.Fatalf("%s included skills = %v, want none", tc.name, packet.IncludedSkills)
			}
			active := packet.Instructions
			for _, command := range []string{
				facts.PrepareCommand, facts.InspectCommand, facts.ResumeCommand, facts.ReleaseCommand,
				facts.SubmitCommand, facts.PauseCommand, facts.ResultResourceCommand,
			} {
				if command == "" || !strings.Contains(active, command) {
					t.Errorf("%s instructions lost bound command %q", tc.name, command)
				}
			}
			for _, document := range facts.Documents {
				if !strings.Contains(active, document.Commit+":"+document.Path) {
					t.Errorf("%s instructions lost exact document reference %s:%s", tc.name, document.Commit, document.Path)
				}
				if got := strings.Count(active, document.Contents); got != 1 {
					t.Errorf("%s rendered document %q %d times, want exactly once", tc.name, document.Path, got)
				}
			}
			for _, fixed := range []string{
				facts.RequiredHead, facts.RecordedTarget,
				fmt.Sprintf("Completed reviews: %d; this invocation is review number %d", facts.ReviewCount, facts.ReviewNumber),
			} {
				if !strings.Contains(active, fixed) {
					t.Errorf("%s instructions omitted fixed review fact %q", tc.name, fixed)
				}
			}
		})
	}
}

// TestDeliveryWatchdogNarrowPrepareAndInspect proves prepare and inspect
// continuations stay narrow: source facts and the next commands, with no
// repeated document evidence and no bundled skills.
func TestDeliveryWatchdogNarrowPrepareAndInspect(t *testing.T) {
	for _, operation := range []string{"prepare", "inspect"} {
		t.Run(operation, func(t *testing.T) {
			facts := deliveryWatchdogFacts(operation, "initial", "full", 0)
			packet, err := BuildPacket("watchdog", InvocationFacts{Delivery: facts})
			if err != nil {
				t.Fatal(err)
			}
			if len(packet.IncludedSkills) != 0 {
				t.Fatalf("%s included skills = %v, want none", operation, packet.IncludedSkills)
			}
			if strings.Contains(packet.Instructions, facts.Documents[0].Contents) {
				t.Error("narrow continuation repeated the ledger document evidence")
			}
			for _, fact := range []string{facts.Worktree, facts.Claim, facts.InspectCommand, facts.ResumeCommand, facts.ReleaseCommand} {
				if !strings.Contains(packet.Instructions, fact) {
					t.Errorf("%s continuation is missing source fact %q", operation, fact)
				}
			}
			if operation == "prepare" && strings.Contains(packet.Instructions, facts.SubmitCommand) {
				t.Error("prepare continuation exposed the handoff before preparation")
			}
			if operation == "inspect" && !strings.Contains(packet.Instructions, facts.SubmitCommand) {
				t.Error("inspect continuation omitted the next applicable handoff")
			}
		})
	}
}

// TestDeliveryWatchdogReportResource proves the report resource binds the
// Result Documents, round and reviewed head, and refuses invalid round and head
// inputs.
func TestDeliveryWatchdogReportResource(t *testing.T) {
	reviewed := strings.Repeat("a", 40)
	resource, err := RenderResource("watchdog", "reference/ledger-review.md", []string{
		"result_directory=/tmp/result",
		"round=2",
		"reviewed_head=" + reviewed,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(resource)
	for _, want := range []string{
		"`/tmp/result/watchdog-report.md`",
		"`/tmp/result/public.md`",
		"review round 2",
		reviewed,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("review report resource is missing %q", want)
		}
	}

	// Every representable engine round works, but the resource must not
	// advertise object identities the documented schema-1 codec refuses.
	if _, err := RenderResource("watchdog", "reference/ledger-review.md", []string{
		"result_directory=/tmp/result", "round=18446744073709551615", "reviewed_head=" + reviewed,
	}); err != nil {
		t.Fatalf("valid engine facts were not renderable: %v", err)
	}
	if _, err := RenderResource("watchdog", "reference/ledger-review.md", []string{
		"result_directory=/tmp/result", "round=1", "reviewed_head=" + strings.Repeat("a", 64),
	}); err == nil {
		t.Fatal("resource advertised a SHA-256 identity unsupported by schema 1")
	}

	if _, err := RenderResource("watchdog", "reference/ledger-review.md", []string{
		"result_directory=/tmp/result",
		"round=0",
		"reviewed_head=" + reviewed,
	}); err == nil {
		t.Error("review report resource accepted round zero")
	}
	if _, err := RenderResource("watchdog", "reference/ledger-review.md", []string{
		"result_directory=/tmp/result",
		"round=2",
		"reviewed_head=not-a-sha",
	}); err == nil {
		t.Error("review report resource accepted a malformed reviewed head")
	}
}

// TestDeliveryWatchdogScopeAndFixedIdentity proves the fixed reviewed head,
// target and completed-review count survive every supplied scope, and that a
// render made before inspection reports its scope carries the unresolved-scope
// branch.
func TestDeliveryWatchdogScopeAndFixedIdentity(t *testing.T) {
	for _, tc := range []struct {
		scope  string
		count  uint64
		marker string
	}{
		{"incremental", 1, "completed-review count (1)"},
		{"full", 2, "completed-review count (2)"},
		{"", 1, "## Review scope"},
	} {
		facts := deliveryWatchdogFacts("next", "initial", tc.scope, tc.count)
		packet, err := BuildPacket("watchdog", InvocationFacts{Delivery: facts})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(packet.Instructions, tc.marker) {
			t.Errorf("scope %q is missing %q", tc.scope, tc.marker)
		}
		if !strings.Contains(packet.Instructions, facts.RequiredHead) || !strings.Contains(packet.Instructions, facts.RecordedTarget) {
			t.Errorf("scope %q did not retain the fixed reviewed head and target", tc.scope)
		}
	}
}
