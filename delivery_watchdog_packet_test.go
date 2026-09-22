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

// TestDeliveryWatchdogSpecializesEachProceeding proves the private-ledger path
// binds every command and document once, carries the fixed review identity, and
// specializes the initial, repeat, and resumed review.
func TestDeliveryWatchdogSpecializesEachProceeding(t *testing.T) {
	cases := []struct {
		name      string
		operation string
		procedure string
		scope     string
		count     uint64
		marker    string
	}{
		{"initial", "next", "initial", "full", 0, "This review is the first completed review for the Work Item."},
		{"repeat", "next", "initial", "incremental", 1, "## Incremental repeat review"},
		{"resume", "resume", "resumed", "incremental", 1, "## Incremental repeat review"},
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
			if !strings.Contains(active, "Opaque {{.Worktree}} contract body stays data.") {
				t.Errorf("%s document body was interpreted as template source instead of preserved", tc.name)
			}
			if !strings.Contains(active, tc.marker) {
				t.Errorf("%s instructions are missing %q", tc.name, tc.marker)
			}
			for _, fixed := range []string{
				facts.RequiredHead, facts.RecordedTarget,
				fmt.Sprintf("Completed reviews: %d; this invocation is review number %d", facts.ReviewCount, facts.ReviewNumber),
				"do not launch `watchdog-runner`",
			} {
				if !strings.Contains(active, fixed) {
					t.Errorf("%s instructions omitted fixed review fact %q", tc.name, fixed)
				}
			}
			for _, forbidden := range []string{".watchdog", ".changes", "findings.json", "submission.md", "Artifact Baseline", "Artifact Completion", "PR comparison"} {
				if strings.Contains(active, forbidden) {
					t.Errorf("%s active render retains retired machinery %q", tc.name, forbidden)
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

// TestDeliveryWatchdogDefersReportResource proves the report instructions are
// deferred behind the bound resource command until the worker asks for them.
func TestDeliveryWatchdogDefersReportResource(t *testing.T) {
	facts := deliveryWatchdogFacts("next", "initial", "full", 0)
	packet, err := BuildPacket("watchdog", InvocationFacts{Delivery: facts})
	if err != nil {
		t.Fatal(err)
	}
	const sentinel = "never hand-author `schema`, `outcome`, `source`, `ledger`, or `round`"
	if !strings.Contains(packet.Instructions, facts.ResultResourceCommand) {
		t.Fatal("initial instructions lost the deferred report resource command")
	}
	if strings.Contains(packet.Instructions, sentinel) {
		t.Fatal("initial instructions embedded the deferred report resource body")
	}
}

// TestDeliveryWatchdogReportResource proves the report resource names the
// private report, keeps engine metadata and the public body out of worker prose,
// and requires no PR input.
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
		"schema-1 frontmatter",
		"never hand-author `schema`, `outcome`, `source`, `ledger`, or `round`",
		"review round 2",
		reviewed,
		"`W<n>`",
		"`BLOCK`",
		"`HUMAN`",
		"`NOTE`",
		"`M<n>`",
		"privately through `skl`",
		"Never use this private report or the worker exchange as the public body",
		"no automatic inline comments",
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

// TestDeliveryWatchdogScopeAndFixedIdentity proves the incremental/full
// selection comes from the supplied scope facts while the fixed reviewed head
// and completed-review count survive either branch.
func TestDeliveryWatchdogScopeAndFixedIdentity(t *testing.T) {
	incrementalFacts := deliveryWatchdogFacts("next", "initial", "incremental", 1)
	incremental, err := BuildPacket("watchdog", InvocationFacts{Delivery: incrementalFacts})
	if err != nil {
		t.Fatal(err)
	}
	fullFacts := deliveryWatchdogFacts("next", "initial", "full", 2)
	full, err := BuildPacket("watchdog", InvocationFacts{Delivery: fullFacts})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(incremental.Instructions, "## Incremental repeat review") {
		t.Error("incremental scope did not select the incremental repeat review")
	}
	if strings.Contains(incremental.Instructions, "## Full review") {
		t.Error("incremental scope also rendered the full review branch")
	}
	if !strings.Contains(full.Instructions, "## Full review") {
		t.Error("full scope did not select the full review")
	}
	if strings.Contains(full.Instructions, "## Incremental repeat review") {
		t.Error("full scope also rendered the incremental review branch")
	}
	for _, packet := range []struct {
		name  string
		facts *DeliveryFacts
		body  string
		count string
	}{
		{"incremental", incrementalFacts, incremental.Instructions, "completed-review count (1) and every prior"},
		{"full", fullFacts, full.Instructions, "completed-review count (2) and every prior"},
	} {
		if !strings.Contains(packet.body, packet.count) {
			t.Errorf("%s branch did not retain the completed-review count %q", packet.name, packet.count)
		}
		if !strings.Contains(packet.body, packet.facts.RequiredHead) || !strings.Contains(packet.body, packet.facts.RecordedTarget) {
			t.Errorf("%s branch did not retain the fixed reviewed head and target", packet.name)
		}
		if !strings.Contains(packet.body, "Do not fetch or merge a newer target snapshot") {
			t.Errorf("%s branch lost the fixed-target cutoff", packet.name)
		}
	}
}

// TestDeliveryWatchdogUnresolvedScope proves a next/resume render made before
// inspection still carries the bounded repeat-review policy until the scope is
// reported.
func TestDeliveryWatchdogUnresolvedScope(t *testing.T) {
	facts := deliveryWatchdogFacts("next", "initial", "", 1)
	packet, err := BuildPacket("watchdog", InvocationFacts{Delivery: facts})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Review scope",
		"bounded to regressions, integration effects, and false claims",
		"retain the completed-review count and every prior `W<n>` finding identity",
	} {
		if !strings.Contains(packet.Instructions, want) {
			t.Errorf("unresolved scope instructions are missing %q", want)
		}
	}
}

// TestDeliveryWatchdogHandoffBoundary proves the active handoff keeps the typed
// outcome routing, honest pending delivery, marker post-check, and the human
// merge boundary without an engine comment parser.
func TestDeliveryWatchdogHandoffBoundary(t *testing.T) {
	facts := deliveryWatchdogFacts("next", "initial", "full", 0)
	packet, err := BuildPacket("watchdog", InvocationFacts{Delivery: facts})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<pass|rework|needs-human>",
		"`pass` only when no `BLOCK` or `HUMAN` finding remains active",
		"second or later completed review routes it to Needs Human",
		"`needs-human` when a human decision is required; it counts as a completed review too",
		"never records a second round",
		"releases this Claim even when ledger replication or public presentation is still pending",
		"`fix_required` result retains this Claim",
		"only comments changed",
		"`git diff --check`",
		"not the full suite again for comments",
		"`--head <actual-final-source-SHA>`",
		"Only a human performs the final integration and merge",
		"verifies Git identities, not comment prose",
		"public PR body, label, or comment is never authority",
		"never keep a worktree-local counter",
	} {
		if !strings.Contains(packet.Instructions, want) {
			t.Errorf("handoff instructions are missing %q", want)
		}
	}
}
