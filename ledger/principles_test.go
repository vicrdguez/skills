package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

func TestParsePrincipleRefusesAnInvalidStatement(t *testing.T) {
	for _, testCase := range []struct {
		name, contents, fault string
	}{
		{"no frontmatter", "# Principle\n\nBody.\n", "frontmatter"},
		{"unclosed frontmatter", "---\nstatement: Keep one shape.\n", "closing"},
		{"no statement", "---\nowner: someone\n---\n", "no statement"},
		{"an empty statement", "---\nstatement: \"  \"\n---\n", "empty"},
		{"a non-string statement", "---\nstatement: [one, two]\n---\n", "not a string"},
		{"an over-long statement", "---\nstatement: " + strings.Repeat("é", ledger.StatementLimit+1) + "\n---\n", "281 characters"},
		{"malformed YAML", "---\nstatement: [unclosed\n---\n", "malformed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ledger.ParsePrinciple(".skl/principles/broken.md", []byte(testCase.contents))
			if err == nil || !strings.Contains(err.Error(), ".skl/principles/broken.md") || !strings.Contains(err.Error(), testCase.fault) {
				t.Fatalf("error = %v, want broken.md and %q", err, testCase.fault)
			}
		})
	}
	limit := strings.Repeat("é", ledger.StatementLimit)
	principle, err := ledger.ParsePrinciple(".skl/principles/at-limit.md", []byte("---\r\nstatement: "+limit+"\r\n---\r\n\r\nBody.\r\n"))
	if err != nil || principle.Name != "at-limit" || principle.Statement != limit || principle.Body != "\nBody.\n" {
		t.Fatalf("principle at the limit = %#v, %v", principle, err)
	}
}

// A checkout's principles read in file-name order, skipping files that are
// not principles.
func TestReadPrinciplesInFileNameOrder(t *testing.T) {
	root := t.TempDir()
	if principles, err := ledger.ReadPrinciples(root); err != nil || len(principles) != 0 {
		t.Fatalf("a repository without principles = %#v, %v", principles, err)
	}
	directory := filepath.Join(root, ".skl", "principles")
	for name, contents := range map[string]string{
		"b.md":         "---\nstatement: Second.\n---\n",
		"a.md":         "---\nstatement: First.\n---\n\nWhy.\n",
		"notes.txt":    "not a principle\n",
		"drafts/c.md":  "not a principle either\n",
		"drafts/d.txt": "nor this\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(directory, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	working, err := ledger.ReadPrinciples(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(working) != 2 || working[0].Name != "a" || working[0].Statement != "First." || working[0].File != "---\nstatement: First.\n---\n\nWhy.\n" || working[1].Name != "b" {
		t.Fatalf("principles = %#v", working)
	}
}
