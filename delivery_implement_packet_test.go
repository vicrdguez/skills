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
		ResultResourceCommand: "skl skill --resource ledger-submission.md --input result_directory='/tmp/result'" +
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

// TestDeliveryImplementBindsEachProcedure proves the private-ledger path
// binds every command, the recorded target and the frozen Contract once for
// the initial, resumed, and rework procedures.
func TestDeliveryImplementBindsEachProcedure(t *testing.T) {
	for _, procedure := range []string{"initial", "resumed", "rework"} {
		t.Run(procedure, func(t *testing.T) {
			packet, err := BuildPacket("implement", InvocationFacts{Delivery: deliveryImplementFacts("next", procedure)})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(packet.IncludedSkills, []string{"testing", "audit"}) {
				t.Fatalf("included skills = %v", packet.IncludedSkills)
			}
			facts := packet.Facts.Delivery
			active := packet.Instructions
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
			if got := strings.Count(active, document.Contents); got != 1 {
				t.Errorf("Contract body rendered %d times, want exactly once", got)
			}
			if !strings.Contains(active, facts.RecordedTarget) {
				t.Errorf("%s instructions omitted the recorded target for offline integration", procedure)
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
