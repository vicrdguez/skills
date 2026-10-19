package ledger_test

import (
	"os"
	"os/exec"
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

// The working tree and a committed revision read the same principles, in
// file-name order, skipping files that are not principles.
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
	for _, command := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "principles"}} {
		if output, err := exec.Command("git", append([]string{"-C", root}, command...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", command, err, output)
		}
	}
	working, err := ledger.ReadPrinciples(root)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := ledger.ReadPrinciplesAt(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, principles := range [][]ledger.Principle{working, committed} {
		if len(principles) != 2 || principles[0].Name != "a" || principles[0].Statement != "First." || principles[0].File != "---\nstatement: First.\n---\n\nWhy.\n" || principles[1].Name != "b" {
			t.Fatalf("principles = %#v", principles)
		}
	}
	if _, err := ledger.ReadPrinciplesAt(root, "missing-revision"); err == nil {
		t.Fatal("an unavailable revision read as a repository without principles")
	}
}
