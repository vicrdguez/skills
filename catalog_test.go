package skills

import (
	"slices"
	"strings"
	"testing"
)

func TestContractGroundedConstructionGuidanceAcrossImplementProcedures(t *testing.T) {
	for _, procedure := range []ImplementProcedure{InitialSubmission, ResumedSubmission, FindingDrivenRework} {
		t.Run(string(procedure), func(t *testing.T) {
			packet, err := BuildPacket("implement", InvocationFacts{Implementation: &ImplementationFacts{
				Procedure: procedure,
				Branch:    "widget",
				Worktree:  "/tmp/widget",
			}})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(packet.IncludedSkills, []string{"testing", "audit", "design", "domain"}) {
				t.Fatalf("included skills = %v", packet.IncludedSkills)
			}
			for _, want := range []string{
				"suitable existing verification boundaries",
				"complete accepted behavior",
				"in-scope refactoring",
				"explicit behavioral scenarios, architecture commitments, required observations, and mandatory standards as binding",
				"unambiguously implied case",
				"consequential behavioral or architectural choice remains unresolved",
			} {
				if !strings.Contains(packet.Instructions, want) {
					t.Errorf("instructions are missing %q", want)
				}
			}
			for _, forbidden := range []string{
				"Materialize every `behavior.md` scenario as an idiomatic test",
				"follow the bundled TDD red -> green loop",
				"Refactoring belongs in that Audit pass",
				"Fix one active finding at a time",
			} {
				if strings.Contains(packet.Instructions, forbidden) {
					t.Errorf("instructions still prescribe %q", forbidden)
				}
			}
		})
	}

	continuation, err := BuildPacket("implement", InvocationFacts{Implementation: &ImplementationFacts{
		Procedure: InitialSubmission,
		Branch:    "widget",
		Inspection: &InspectionFacts{
			Progress: BaselineOnly,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(continuation.IncludedSkills) != 0 {
		t.Fatalf("inspection included skills = %v", continuation.IncludedSkills)
	}
	for _, want := range []string{"complete accepted contract", "construction and verification choices", "frozen obligations"} {
		if !strings.Contains(continuation.Instructions, want) {
			t.Errorf("inspection continuation is missing %q", want)
		}
	}
	if strings.Contains(continuation.Instructions, "red -> green") {
		t.Errorf("inspection continuation still prescribes red-green:\n%s", continuation.Instructions)
	}
}

func TestTestingPolicyPointersAndDesignGuidanceAgree(t *testing.T) {
	propose, err := BuildPacket("propose", InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(propose.IncludedSkills, []string{"design", "testing"}) {
		t.Fatalf("propose included skills = %v", propose.IncludedSkills)
	}
	for _, want := range []string{"use `testing`", "skl skill --resource reference/tests.md testing"} {
		if !strings.Contains(propose.Instructions, want) {
			t.Errorf("Propose is missing %q", want)
		}
	}
	if strings.Contains(propose.Instructions, "use `tdd`") {
		t.Errorf("Propose retains the retired tdd pointer")
	}

	deepening, err := RenderResource("design", "reference/DEEPENING.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	guidance := string(deepening)
	for _, want := range []string{"required behavioral and failure-mode protection", "does not automatically require another test layer"} {
		if !strings.Contains(guidance, want) {
			t.Errorf("Design deepening guidance is missing %q", want)
		}
	}
}

func TestPacketsUseIntegrationReferences(t *testing.T) {
	packet, err := BuildPacket("implement", InvocationFacts{Implementation: &ImplementationFacts{WorkItemReference: "ticket-7"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packet.Instructions, "Work Item: ticket-7") || strings.Contains(packet.Instructions, "Work Item: #0") {
		t.Fatalf("implementation reference rendered by Catalog:\n%s", packet.Instructions)
	}

	packet, err = BuildPacket("watchdog", InvocationFacts{Watchdog: &WatchdogFacts{WorkItem: 7, Submission: 11}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packet.Instructions, "Work Item #7") || !strings.Contains(packet.Instructions, "Submission is #11") {
		t.Fatalf("review references rendered by Catalog:\n%s", packet.Instructions)
	}
}
