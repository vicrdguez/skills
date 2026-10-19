package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lintADR = "docs/adr/0005-verify-contracts-instead-of-prescribing-test-order.md"

// lintedContract is a conforming labelled Contract: B1 and B2 in behavior.md,
// A1 in plan.md sourced from the Proposal description, one row covering all
// three, a Definition of Done citing a range and an M1 Manual Verification.
func lintedContract() map[string]string {
	return map[string]string{
		"intent.md":   "# Lint intent\n\n## Definition of Done\n\n- [ ] Both rules hold\n  through the commitment (B1–B2, A1).\n\n## Manual verification\n\n- [ ] M1: Look at it by hand.\n",
		"behavior.md": "# Lint behavior\n\n## B1: First rule\n\nIt holds.\n\n### Scenario: It holds\n- Given a widget\n- Then it holds\n\n## B2: Second rule\n\nIt holds too.\n",
		"plan.md":     "# Lint plan\n\n## Delegated\n\n- Helper names.\n\n## Forbidden\n\n- A second representation.\n\n## Architectural commitments\n\n### A1: One shape\nSource: proposal.md\n\n- **Interface:** one function.\n\n## Verification strategy\n\n| Covers | Check | Rank |\n| --- | --- | --- |\n| B1, B2, A1 | Focused tests | tests |\n",
	}
}

func lintSpec(name string, files map[string]string) proposalSpec {
	return proposalSpec{name: name, description: "# " + name + "\n\nDurable description.\n", slices: []proposalSliceSpec{{name: "foundation", title: "Lint foundation", branch: name, files: files}}}
}

func TestAcceptRefusesContractsThatFailLint(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	head := ledgerHead(t, fixture.clone)

	replace := func(name, old, new string) func(map[string]string) {
		return func(files map[string]string) {
			if !strings.Contains(files[name], old) {
				t.Fatalf("%s holds no %q", name, old)
			}
			files[name] = strings.Replace(files[name], old, new, 1)
		}
	}
	cases := []struct {
		name      string
		mutate    func(map[string]string)
		fragments []string
	}{
		{"a rule defined twice", func(files map[string]string) { files["behavior.md"] += "\n## B2: Second rule again\n" },
			[]string{"foundation", "behavior.md", "B2"}},
		{"a Definition of Done citing a missing rule", replace("intent.md", "(B1–B2, A1)", "(B1–B2, A1, B4)"),
			[]string{"foundation", "intent.md", "B4"}},
		{"a range citing a missing rule", replace("intent.md", "(B1–B2, A1)", "(B1–B3, A1)"),
			[]string{"foundation", "intent.md", "B3"}},
		{"a Definition of Done bullet citing nothing", replace("intent.md", "## Manual", "- [ ] Everything works.\n\n## Manual"),
			[]string{"foundation", "intent.md", "Everything works."}},
		{"a commitment defined in behavior.md", func(files map[string]string) { files["behavior.md"] += "\n## A1: Misplaced commitment\n" },
			[]string{"foundation", "behavior.md", "A1"}},
		{"a Manual Verification item outside its section", replace("intent.md", "- [ ] Both rules", "- [ ] M2: Checked by hand (B1).\n- [ ] Both rules"),
			[]string{"foundation", "intent.md", "M2"}},
		{"a slice without plan.md", func(files map[string]string) { delete(files, "plan.md") },
			[]string{"foundation", "plan.md"}},
		{"a rule without a covering row", func(files map[string]string) { files["behavior.md"] += "\n## B3: Third rule\n" },
			[]string{"foundation", "plan.md", "B3"}},
		{"a Verification strategy without the table", replace("plan.md", "| Covers | Check | Rank |", "| Covers | Check |"),
			[]string{"foundation", "plan.md", "Verification strategy"}},
		{"an unknown rank", replace("plan.md", "| tests |", "| corpus |"),
			[]string{"foundation", "plan.md", "row 1", "corpus", "tests, build, inspection"}},
		{"a commitment without a source", replace("plan.md", "Source: proposal.md\n", ""),
			[]string{"foundation", "plan.md", "A1"}},
		{"a missing ADR", replace("plan.md", "Source: proposal.md", "Source: docs/adr/0099-missing.md"),
			[]string{"foundation", "plan.md", "A1", "docs/adr/0099-missing.md"}},
		{"an unrecorded experiment report", replace("plan.md", "Source: proposal.md", "Source: proposal.md, experiments/layout.md"),
			[]string{"foundation", "plan.md", "A1", "experiments/layout.md"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			files := lintedContract()
			testCase.mutate(files)
			outcome, text := cli.run(t, []string{"skl", "proposal", "accept", "--repo", root, "--proposal-dir", writeProposal(t, "", lintSpec("linted", files)), "--format", "json"})
			if outcome.Status != "fix_required" {
				t.Fatalf("acceptance was not refused: %s", text)
			}
			for _, fragment := range testCase.fragments {
				if !strings.Contains(outcome.Reason, fragment) {
					t.Fatalf("refusal lacks %q: %s", fragment, text)
				}
			}
			requireLedgerUnchanged(t, fixture.clone, head)
			if forge.createdCount() != 0 {
				t.Fatal("a refused intake published an issue")
			}
		})
	}
}

func TestAcceptRecordsConformingContractsUnchanged(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	writeFile(t, filepath.Join(root, filepath.FromSlash(lintADR)), "# Verify Contracts\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "adr")
	cli := newLedgerApp(t, newForgeServer(t))

	sourced := lintedContract()
	sourced["plan.md"] = strings.Replace(sourced["plan.md"], "Source: proposal.md", "Source: proposal.md, `"+lintADR+"`", 1)
	minimal := map[string]string{
		"intent.md":   "# Minimal intent\n\n## Definition of Done\n\n- [ ] The rule holds (B1).\n",
		"behavior.md": "# Minimal behavior\n\n## B1: The rule\n\nIt holds.\n",
		"plan.md":     "# Minimal plan\n\n## Verification strategy\n\n| Covers | Check | Rank |\n| :--- | --- | ---: |\n| B1 | A test | `tests` |\n\n## Delegated\n\n## Forbidden\n",
	}
	for name, files := range map[string]map[string]string{"sourced": sourced, "minimal": minimal} {
		if outcome := cli.accept(t, root, writeProposal(t, "", lintSpec(name, files))); outcome.Status != "accepted" {
			t.Fatalf("accept %s: %s", name, mustJSON(t, outcome))
		}
		for document, contents := range files {
			if got := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/"+name+"/foundation/"+document); got != contents {
				t.Fatalf("recorded %s of %s = %q, want the intake bytes %q", document, name, got, contents)
			}
		}
	}
}

func TestAmendLintsOnlyTheContractsItChanges(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	acceptSpec(t, cli, root, lintSpec("historic", lintedContract()))
	// A Contract recorded before lint existed has no plan.md.
	recordedPlan := filepath.Join(fixture.clone, "projects", "widgets", "proposals", "historic", "foundation", "plan.md")
	if err := os.Remove(recordedPlan); err != nil {
		t.Fatal(err)
	}
	runGit(t, fixture.clone, "commit", "-q", "-am", "historic record")
	head := ledgerHead(t, fixture.clone)

	refused := exportedIntake(t, cli, root, "historic")
	addIntakeSlice(t, refused, "two")
	writeFile(t, filepath.Join(refused, "two", "behavior.md"), "# two behavior\n\n## A1: Misplaced commitment\n")
	requireRefused(t, cli.amend(t, root, refused), fixture.clone, head, "two", "behavior.md", "A1")

	grown := exportedIntake(t, cli, root, "historic")
	addIntakeSlice(t, grown, "two")
	requireAmended(t, cli.amend(t, root, grown))
	if _, err := os.Stat(recordedPlan); !os.IsNotExist(err) {
		t.Fatalf("the unchanged recorded Slice gained a plan.md: %v", err)
	}
	if got := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/historic/foundation/behavior.md"); got != lintedContract()["behavior.md"] {
		t.Fatalf("the unchanged recorded Slice changed: %q", got)
	}
}
