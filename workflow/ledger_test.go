package workflow

import (
	"strings"
	"testing"
)

func TestInspectLedgerRetirement(t *testing.T) {
	root := newGitRepository(t)
	commitLedger(t, root, "slice", true)
	baseline := gitOutput(t, root, "rev-parse", "HEAD")
	commitFile(t, root, "code.go", "package demo\n")
	completion := gitOutput(t, root, "rev-parse", "HEAD")
	runGit(t, root, "rm", "-r", ".changes/slice")
	runGit(t, root, "commit", "-m", "retire")
	deletion := gitOutput(t, root, "rev-parse", "HEAD")
	commitFile(t, root, "later", "later\n")
	got, err := InspectLedger(root, "HEAD", "slice")
	if err != nil || len(got.Violations) != 0 || got.Phase != "retired" || got.Baseline != baseline || got.Completion != completion || got.Deletion != deletion {
		t.Fatalf("InspectLedger = %#v, %v", got, err)
	}
}

func TestInspectLedgerFrozenTicks(t *testing.T) {
	for _, change := range []struct{ name, body, violation string }{
		{"permitted tick", "# Intent\n- [x] Ship\n## Manual verification\n- [ ] Human\n", ""},
		{"unchecked completion", "# Intent\n- [ ] Ship\n## Manual verification\n- [ ] Human\n", "unchecked"},
		{"manual tick", "# Intent\n- [x] Ship\n## Manual verification\n- [x] Human\n", "Manual Verification"},
		{"rewritten prose", "# Changed\n- [x] Ship\n## Manual verification\n- [ ] Human\n", "frozen"},
	} {
		t.Run(change.name, func(t *testing.T) {
			root := newGitRepository(t)
			commitLedger(t, root, "slice", true)
			// Amend only the fixture's unpublished baseline, preserving literal content.
			runGit(t, root, "reset", "--soft", "HEAD~1")
			commitFile(t, root, ".changes/slice/intent.md", "# Intent\n- [ ] Ship\n## Manual verification\n- [ ] Human\n")
			if change.name != "unchecked completion" {
				commitFile(t, root, ".changes/slice/intent.md", change.body)
			}
			runGit(t, root, "rm", "-r", ".changes/slice")
			runGit(t, root, "commit", "-m", "retire")
			got, err := InspectLedger(root, "HEAD", "slice")
			if err != nil {
				t.Fatal(err)
			}
			if change.violation == "" && len(got.Violations) != 0 || change.violation != "" && !strings.Contains(strings.Join(got.Violations, "\n"), change.violation) {
				t.Fatalf("violations = %v", got.Violations)
			}
		})
	}
}

func TestInspectLedgerRejectsInvalidGraphs(t *testing.T) {
	for name, arrange := range map[string]func(*testing.T, string){
		"missing": func(t *testing.T, root string) {},
		"split introduction": func(t *testing.T, root string) {
			commitLedger(t, root, "slice", false)
			commitFile(t, root, ".changes/slice/behavior.md", "behavior\n")
		},
		"recreation": func(t *testing.T, root string) {
			commitLedger(t, root, "slice", true)
			runGit(t, root, "rm", "-r", ".changes/slice")
			runGit(t, root, "commit", "-m", "retire")
			commitLedger(t, root, "slice", true)
		},
		"importing merge": func(t *testing.T, root string) {
			runGit(t, root, "switch", "-c", "side")
			commitLedger(t, root, "slice", true)
			runGit(t, root, "switch", "main")
			runGit(t, root, "merge", "--no-ff", "side", "-m", "import")
		},
		"path-changing merge": func(t *testing.T, root string) {
			commitLedger(t, root, "slice", true)
			runGit(t, root, "switch", "-c", "side")
			commitFile(t, root, ".changes/slice/tasks.md", "tasks\n")
			runGit(t, root, "switch", "main")
			runGit(t, root, "merge", "--no-ff", "side", "-m", "import")
		},
		"reverted prose edit": func(t *testing.T, root string) {
			commitLedger(t, root, "slice", true)
			commitFile(t, root, ".changes/slice/intent.md", "changed\n")
			commitFile(t, root, ".changes/slice/intent.md", "intent\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := newGitRepository(t)
			arrange(t, root)
			got, err := InspectLedger(root, "HEAD", "slice")
			if err != nil || len(got.Violations) == 0 {
				t.Fatalf("accepted invalid graph: %#v %v", got, err)
			}
		})
	}
}
