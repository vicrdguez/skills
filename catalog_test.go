package skills

import (
	"slices"
	"testing"
)

func TestAuditAppliesContractAcceptanceCriteria(t *testing.T) {
	audit, err := BuildPacket("audit", InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(audit.Resources, []string{"acceptance.md", "smells.md"}) {
		t.Fatalf("audit resources = %v", audit.Resources)
	}
}
