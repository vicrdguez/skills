package skills

import (
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
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
		if strings.HasPrefix(file, principlesRoot+"/") && strings.Contains(string(contents), "{{") {
			t.Errorf("principle file %s contains a template action", file)
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

// Every skl-owned principle declares a statement of the project-principle
// form, and its rendering leaves that frontmatter out.
func TestEverySklPrincipleDeclaresAStatement(t *testing.T) {
	entries, err := fs.ReadDir(embedded, principlesRoot)
	if err != nil {
		t.Fatal(err)
	}
	principles := 0
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".md" {
			continue
		}
		principles++
		file := path.Join(principlesRoot, entry.Name())
		contents, err := fs.ReadFile(embedded, file)
		if err != nil {
			t.Fatal(err)
		}
		principle, err := ledger.ParsePrinciple(file, contents)
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		rendered, err := renderDocument(strings.TrimPrefix(file, proseRoot+"/"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(rendered, principle.Statement) || strings.HasPrefix(rendered, "---") {
			t.Errorf("%s renders its statement frontmatter:\n%s", file, rendered)
		}
	}
	if principles == 0 {
		t.Fatalf("%s holds no principles", principlesRoot)
	}
}
