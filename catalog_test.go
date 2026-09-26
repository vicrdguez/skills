package skills

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
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

// TestProseTemplateSetIsUnambiguous guards the one template set every render
// parses: a repeated block name would silently replace another Procedure's
// text, and Craft never branches on invocation facts.
func TestProseTemplateSetIsUnambiguous(t *testing.T) {
	defined := map[string]string{}
	err := fs.WalkDir(embedded, proseRoot, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		contents, err := fs.ReadFile(embedded, file)
		if err != nil {
			return err
		}
		if strings.HasPrefix(file, proseRoot+"/craft/") && strings.Contains(string(contents), "{{") {
			t.Errorf("craft file %s contains a template action", file)
		}
		for _, match := range regexp.MustCompile(`\{\{-?\s*define "([^"]+)"`).FindAllStringSubmatch(string(contents), -1) {
			if previous, ok := defined[match[1]]; ok {
				t.Errorf("block %q is defined in both %s and %s", match[1], previous, file)
			}
			defined[match[1]] = file
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAuditRenderingsNameNoHarnessOrWorkflowContext guards the two Audit
// Procedures: the Audit step renders the same dispatch for every established
// capability, neither rendering names a harness recipe, and standalone Audit
// carries no Implement context.
func TestAuditRenderingsNameNoHarnessOrWorkflowContext(t *testing.T) {
	var step string
	for _, capability := range []ExecutionCapability{UnknownCapability, ClaudeAgentReview, PiSubagentReview, SequentialReview} {
		facts := deliveryImplementFacts("next", "rework")
		facts.Capability = capability
		file, data := procedure("audit", InvocationFacts{Delivery: facts})
		rendered, err := renderDocument(file, data)
		if err != nil {
			t.Fatal(err)
		}
		if step == "" {
			step = rendered
		} else if rendered != step {
			t.Errorf("Audit step for capability %q differs from the unknown-capability rendering", capability)
		}
	}
	standalone, err := BuildPacket("audit", InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	for name, rendered := range map[string]string{"step": step, "standalone": standalone.Instructions} {
		for _, recipe := range []string{"Claude", "Pi ", "`Agent`", "`subagent`", "general-purpose"} {
			if strings.Contains(rendered, recipe) {
				t.Errorf("%s Audit names harness recipe %q", name, recipe)
			}
		}
	}
	for _, context := range []string{"Implement", "Rework", "rework", "Claim", "review count", "completed-review", "handoff", "Watchdog"} {
		if strings.Contains(standalone.Instructions, context) {
			t.Errorf("standalone Audit carries workflow context %q", context)
		}
	}
}
