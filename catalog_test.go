package skills

import (
	"slices"
	"strings"
	"testing"
)

func TestTestingPolicyPointersAndDesignGuidanceAgree(t *testing.T) {
	propose, err := BuildPacket("propose", InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(propose.IncludedSkills, []string{"design", "testing"}) {
		t.Fatalf("propose included skills = %v", propose.IncludedSkills)
	}
	for _, want := range []string{"Use `testing`", "skl skill --resource reference/tests.md testing"} {
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

	tasks, err := RenderResource("propose", "reference/tasks.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"coherent implementation outcome", "Scenario count, test count", "stable IDs are optional"} {
		if !strings.Contains(string(tasks), want) {
			t.Errorf("Propose tasks do not preserve grouped optional contract work %q:\n%s", want, tasks)
		}
	}
	for _, forbidden := range []string{"One behavioral task per Gherkin scenario", "one per scenario → a red-green cycle", "Stable ids (B1"} {
		if strings.Contains(string(tasks), forbidden) {
			t.Errorf("Propose tasks retain %q", forbidden)
		}
	}
}

func TestAuditAppliesContractAcceptanceCriteria(t *testing.T) {
	criteria, err := RenderResource("audit", "reference/acceptance.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"behavioral conformance",
		"architectural conformance",
		"local implementation quality",
		"obligation, the plausible violation, and why existing evidence does not distinguish it",
		"equally valid implementation",
		"preference alone",
		"required behavioral and failure-mode protection",
	} {
		if !strings.Contains(string(criteria), want) {
			t.Errorf("acceptance criteria are missing %q", want)
		}
	}

	audit, err := BuildPacket("audit", InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(audit.Resources, []string{"reference/acceptance.md", "reference/smells.md"}) {
		t.Fatalf("audit resources = %v", audit.Resources)
	}
	for _, want := range []string{"grouped many-to-many evidence", "removed or weakened assertions", "frozen architectural violations"} {
		if !strings.Contains(strings.Join(strings.Fields(audit.Instructions), " "), want) {
			t.Errorf("Audit is missing %q", want)
		}
	}
	for _, forbidden := range []string{"Every `behavior.md` scenario has materialized as a test", "new test for every scenario", "per-test justification ledger"} {
		if strings.Contains(audit.Instructions, forbidden) {
			t.Errorf("Audit retains %q", forbidden)
		}
	}

}
