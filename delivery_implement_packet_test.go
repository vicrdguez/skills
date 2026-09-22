package skills

import (
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// deliveryImplementFacts is one fully bound private-ledger implementation
// invocation: an acquired Claim, its exact commands, source identities, and the
// frozen accepted Contract documents.
func deliveryImplementFacts(operation, procedure string) *DeliveryFacts {
	claim := strings.Repeat("c", 40)
	return &DeliveryFacts{
		Phase:           "implement",
		Procedure:       procedure,
		Operation:       operation,
		Repository:      "payments",
		Remote:          "origin",
		Item:            "add-refunds/refund",
		Branch:          "add-refunds",
		Worktree:        "/tmp/add-refunds",
		ResultDirectory: "/tmp/result",
		Claim:           claim,
		PrepareCommand:  "skl implement prepare --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'",
		InspectCommand:  "skl implement inspect --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'",
		ResumeCommand:   "skl implement resume --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'",
		ReleaseCommand:  "skl implement release --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'",
		SubmitCommand: "skl implement submit --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'" +
			" --head <final-head> --target <observed-target-sha> --body '/tmp/result/implement-report.md'",
		PauseCommand: "skl implement needs-human --repo '/tmp/add-refunds' --item 'add-refunds/refund' --claim '" + claim + "'" +
			" --body '/tmp/result/implement-report.md'",
		ResultResourceCommand: "skl skill --resource reference/ledger-submission.md --input result_directory='/tmp/result'" +
			" --input procedure='" + procedure + "' implement",
		RequiredHead:     strings.Repeat("a", 40),
		RecordedTarget:   strings.Repeat("b", 40),
		PreviousReviewed: strings.Repeat("d", 40),
		SourceHead:       strings.Repeat("e", 40),
		SourceTarget:     strings.Repeat("f", 40),
		ReviewScope:      "incremental",
		FetchStatus:      "local: origin fetch failed (offline); using available local source inputs",
		Capability:       PiSubagentReview,
		Documents: []ledger.ContractDocument{{
			Commit:   strings.Repeat("1", 40),
			Path:     "projects/payments/proposals/add-refunds/refund/behavior.md",
			Contents: "# Refund behavior\n\nOpaque {{.Worktree}} contract body stays data.\n",
		}},
	}
}

// deliveryImplementActive isolates the specialized Implement definition from
// the bundled testing, audit, design, and domain skill definitions.
func deliveryImplementActive(instructions string) string {
	const marker = "\n\n## Included Skill: "
	if strings.Contains(instructions, marker) {
		return strings.SplitN(instructions, marker, 2)[0]
	}
	return instructions
}

// TestDeliveryImplementSpecializesEachProcedure proves the private-ledger path
// binds every command and the frozen Contract once, and that initial, resumed,
// and rework renders carry only their applicable procedure.
func TestDeliveryImplementSpecializesEachProcedure(t *testing.T) {
	for _, procedure := range []string{"initial", "resumed", "rework"} {
		t.Run(procedure, func(t *testing.T) {
			packet, err := BuildPacket("implement", InvocationFacts{Delivery: deliveryImplementFacts("next", procedure)})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(packet.IncludedSkills, []string{"testing", "audit", "design", "domain"}) {
				t.Fatalf("included skills = %v", packet.IncludedSkills)
			}
			facts := packet.Facts.Delivery
			active := deliveryImplementActive(packet.Instructions)
			for _, command := range []string{
				facts.PrepareCommand, facts.InspectCommand, facts.ResumeCommand, facts.ReleaseCommand,
				facts.SubmitCommand, facts.PauseCommand, facts.ResultResourceCommand,
			} {
				if command == "" || !strings.Contains(active, command) {
					t.Errorf("instructions lost bound command %q", command)
				}
			}

			document := facts.Documents[0]
			if !strings.Contains(active, document.Commit+":"+document.Path) {
				t.Error("instructions lost the exact Contract reference")
			}
			if !strings.Contains(active, "Opaque {{.Worktree}} contract body stays data.") {
				t.Errorf("Contract body was interpreted as template source instead of preserved:\n%s", active)
			}
			if got := strings.Count(active, document.Contents); got != 1 {
				t.Errorf("Contract body rendered %d times, want exactly once", got)
			}

			marker := map[string]string{
				"initial": "implement the complete accepted behavior and architecture",
				"resumed": "Resume this existing Claim without assuming preparation completed",
				"rework":  "findings to resolve, not as new frozen requirements",
			}[procedure]
			if !strings.Contains(active, marker) {
				t.Errorf("%s instructions are missing %q", procedure, marker)
			}
			if !strings.Contains(active, facts.RecordedTarget) {
				t.Errorf("%s instructions omitted the recorded target for offline integration", procedure)
			}
			if !strings.Contains(active, "never authority for this work") {
				t.Errorf("%s instructions do not deny PR/comment authority", procedure)
			}

			for _, forbidden := range []string{
				"Artifact Completion", "Artifact Baseline", ".watchdog", "PR comparison",
				"remove `.changes/", "draft Submission", "Implementation Ledger",
			} {
				if strings.Contains(active, forbidden) {
					t.Errorf("%s active render retains retired machinery %q", procedure, forbidden)
				}
			}
		})
	}
}

// TestDeliveryImplementNarrowInspectAndPrepare proves prepare and inspect
// continuations stay narrow: source facts and the next commands, with no
// repeated Contract evidence and no bundled skills.
func TestDeliveryImplementNarrowInspectAndPrepare(t *testing.T) {
	for _, operation := range []string{"prepare", "inspect"} {
		t.Run(operation, func(t *testing.T) {
			packet, err := BuildPacket("implement", InvocationFacts{Delivery: deliveryImplementFacts(operation, "initial")})
			if err != nil {
				t.Fatal(err)
			}
			if len(packet.IncludedSkills) != 0 {
				t.Fatalf("%s included skills = %v, want none", operation, packet.IncludedSkills)
			}
			facts := packet.Facts.Delivery
			if strings.Contains(packet.Instructions, facts.Documents[0].Contents) {
				t.Error("narrow continuation repeated the Contract evidence")
			}
			for _, fact := range []string{facts.Worktree, facts.Claim, facts.InspectCommand} {
				if !strings.Contains(packet.Instructions, fact) {
					t.Errorf("%s continuation is missing source fact %q", operation, fact)
				}
			}
			if !strings.Contains(packet.Instructions, facts.ResumeCommand) || !strings.Contains(packet.Instructions, facts.ReleaseCommand) {
				t.Errorf("%s continuation is missing its next applicable commands", operation)
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

// TestDeliveryImplementDefersReportResource proves the report instructions are
// deferred behind the bound resource command until the worker asks for them.
func TestDeliveryImplementDefersReportResource(t *testing.T) {
	packet, err := BuildPacket("implement", InvocationFacts{Delivery: deliveryImplementFacts("next", "initial")})
	if err != nil {
		t.Fatal(err)
	}
	const sentinel = "full current completion-and-evidence table"
	if !strings.Contains(packet.Instructions, packet.Facts.Delivery.ResultResourceCommand) {
		t.Fatal("initial instructions lost the deferred report resource command")
	}
	if strings.Contains(packet.Instructions, sentinel) {
		t.Fatal("initial instructions embedded the deferred report resource body")
	}
}

// TestDeliveryImplementSubmissionReportDeclarations proves the report resource
// renders the full current completion declarations, human-owned checks, and
// Audit dispositions for every procedure without an engine prose parser.
func TestDeliveryImplementSubmissionReportDeclarations(t *testing.T) {
	for _, procedure := range []string{"initial", "resumed", "rework"} {
		t.Run(procedure, func(t *testing.T) {
			resource, err := RenderResource("implement", "reference/ledger-submission.md", []string{
				"result_directory=/tmp/result",
				"procedure=" + procedure,
			})
			if err != nil {
				t.Fatal(err)
			}
			body := string(resource)
			for _, want := range []string{
				"`/tmp/result/implement-report.md`",
				"`/tmp/result/public.md`",
				"schema-1 frontmatter",
				"never hand-author `schema`, `outcome`, `source`, `ledger`, or `round`",
				"cross-checks the prose",
				"full current completion-and-evidence table",
				"`B<n>`",
				"`A<n>`",
				"`T<n>`",
				"`complete` or `incomplete`",
				"human-owned `M<n>` checks",
				"Omitted entries are never complete",
				"Full Gate",
				"## Audit ledger",
				"`F<n>`",
				"`Standards`",
				"`Contracts`",
				"severity",
				"disposition",
				"question",
				"options",
				"recommendation",
				"incomplete status",
				"human verification obligations stay accessible privately through `skl`",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("%s report resource is missing %q", procedure, want)
				}
			}
			if !strings.Contains(body, "Never use this private report or the worker exchange as the public body") {
				t.Error("report resource does not forbid publishing the private exchange")
			}
			if procedure == "rework" && !strings.Contains(body, "Preserve every historical `F<n>` and `W<n>` identity") {
				t.Error("rework report resource lost historical finding preservation")
			}
		})
	}
}

// TestDeliveryImplementHandoffSemantics proves the active handoff keeps the
// fixed typed outcomes, honest pending delivery, and exact-Claim retry rules.
func TestDeliveryImplementHandoffSemantics(t *testing.T) {
	packet, err := BuildPacket("implement", InvocationFacts{Delivery: deliveryImplementFacts("next", "initial")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"`awaiting_review`",
		"never inferred from the report prose",
		"`fix_required` outcome retains this Claim",
		"never infer a release",
		"releases this Claim locally even when ledger replication or normal public presentation is still pending",
		"Preserve the Result Documents for a safe retry",
		"`--format json` exists only for callers that explicitly request it",
		"Independent Watchdog Review and the human merge boundary are preserved",
	} {
		if !strings.Contains(packet.Instructions, want) {
			t.Errorf("handoff instructions are missing %q", want)
		}
	}
}
