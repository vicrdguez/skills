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

// TestAuditStepIgnoresCapability guards harness-agnostic dispatch: the Audit
// step renders the same text for every established capability, a branch the
// golden journey never reaches.
func TestAuditStepIgnoresCapability(t *testing.T) {
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
}
