package main

import (
	"fmt"
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
		{"a fenced citation of a missing rule", replace("intent.md", "## Manual", "```\nCites B9.\n```\n\n## Manual"),
			[]string{"foundation", "intent.md", "B9"}},
		{"a wide range missing its interior rules", func(files map[string]string) {
			replace("intent.md", "(B1–B2, A1)", "(B1–B1002, A1)")(files)
			files["behavior.md"] += "\n## B1002: Far rule\n"
		}, []string{"foundation", "intent.md", "B3"}},
		{"a reversed range citing a missing rule", replace("intent.md", "(B1–B2, A1)", "(B2–B0, A1)"),
			[]string{"foundation", "intent.md", "B0"}},
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
	wide := lintedContract()
	var rules strings.Builder
	for number := 3; number <= 1002; number++ {
		fmt.Fprintf(&rules, "\n## B%d: Rule %d\n", number, number)
	}
	wide["behavior.md"] += rules.String()
	wide["intent.md"] = strings.Replace(wide["intent.md"], "(B1–B2, A1)", "(B1–B1002, A1)", 1)
	wide["plan.md"] = strings.Replace(wide["plan.md"], "| B1, B2, A1 |", "| B1-B1002, A1 |", 1)
	if !strings.Contains(wide["intent.md"], "B1–B1002") || !strings.Contains(wide["plan.md"], "B1-B1002") {
		t.Fatal("the wide Contract cites no wide range")
	}
	for name, files := range map[string]map[string]string{"sourced": sourced, "minimal": minimal, "wide": wide} {
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

// lintSkill is a verification skill declaring corpus, tests and build, with
// rows of every kind but journey.
const lintSkill = "---\nranks: [corpus, tests, build]\n---\n\n# Widgets verification\n\n## Launch\n\nRun from the root.\n\n## Doctor\n\nNothing to check.\n\n## Channels\n\n- `oracle`: the corpus.\n\n## Catalog\n\n" +
	"| Name | Channel | Kind | Run | Expectations | Rank |\n| --- | --- | --- | --- | --- | --- |\n" +
	"| gate | oracle | executed | `make gate` | every case passes | corpus |\n" +
	"| tests | tests | executed | `go test ./...` | every test passes | tests |\n" +
	"| routes | driven-surface | generated | `make routes` | every route | |\n" +
	"| review | human-observed | human | the dashboard | a person approves | build |\n" +
	"\n## Cleanup\n\nNothing.\n\n## Extend\n\nAdd a case.\n\n## Traps\n\nNone.\n"

// skillRepository is a source repository whose committed verification skill
// is contents.
func skillRepository(t *testing.T, contents string) string {
	t.Helper()
	root := sourceRepository(t, "acme", "widgets")
	writeFile(t, filepath.Join(root, ".skl", "verify.md"), contents)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "verification skill")
	return root
}

// strategy replaces the linted Contract's Verification strategy rows.
func strategy(rows string) map[string]string {
	files := lintedContract()
	files["plan.md"] = strings.Replace(files["plan.md"], "| B1, B2, A1 | Focused tests | tests |\n", rows, 1)
	return files
}

func TestAcceptLintsVerificationStrategyAgainstTheCatalog(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := skillRepository(t, lintSkill)
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	declared := strategy("| B1, A1 | gate 17 | corpus |\n| B2 | new corpus-case 17 | tests |\n")
	declared["intent.md"] = strings.Replace(declared["intent.md"], "## Manual", "- [ ] The `corpus-case` row holds the second rule's case (B2).\n\n## Manual", 1)

	accepted := map[string]map[string]string{
		"project-rank": strategy("| B1, B2, A1 | gate 17 | corpus |\n"),
		"declared":     declared,
		"commitment":   strategy("| B1, B2 | gate | corpus |\n| A1 | read the parser against A1 | build |\n"),
	}
	for name, files := range accepted {
		if outcome := cli.accept(t, root, writeProposal(t, "", lintSpec(name, files))); outcome.Status != "accepted" {
			t.Fatalf("accept %s: %s", name, mustJSON(t, outcome))
		}
	}

	head := ledgerHead(t, fixture.clone)
	undeclared := strategy("| B1, A1 | gate 17 | corpus |\n| B2 | new corpus-case 17 | tests |\n")
	declaredExisting := func(name string) map[string]string {
		files := strategy("| B1, B2, A1 | new " + name + " | tests |\n")
		files["intent.md"] = strings.Replace(files["intent.md"], "## Manual", "- [ ] Add `"+name+"` (B1, B2).\n\n## Manual", 1)
		return files
	}
	refused := []struct {
		name      string
		files     map[string]string
		fragments []string
	}{
		{"a built-in rank", strategy("| B1, B2, A1 | gate 17 | inspection |\n"), []string{"inspection", "corpus, tests, build"}},
		{"an unknown row", strategy("| B1 | corpus 17 | corpus |\n| B2, A1 | tests | tests |\n"), []string{"foundation", "plan.md", "B1", "corpus"}},
		{"a generated row", strategy("| B1, B2, A1 | routes | tests |\n"), []string{"routes", "generated"}},
		{"a human row", strategy("| B1, B2, A1 | review | build |\n"), []string{"review", "human"}},
		{"a declared existing generated row", declaredExisting("routes"), []string{"routes", "generated"}},
		{"a declared existing human row", declaredExisting("review"), []string{"review", "human"}},
		{"an undeclared new row", undeclared, []string{"corpus-case", "Definition of Done"}},
	}
	for _, testCase := range refused {
		t.Run(testCase.name, func(t *testing.T) {
			outcome, text := cli.run(t, []string{"skl", "proposal", "accept", "--repo", root, "--proposal-dir", writeProposal(t, "", lintSpec("refused", testCase.files)), "--format", "json"})
			if outcome.Status != "fix_required" {
				t.Fatalf("acceptance was not refused: %s", text)
			}
			for _, fragment := range testCase.fragments {
				if !strings.Contains(outcome.Reason+outcome.Repair, fragment) {
					t.Fatalf("refusal lacks %q: %s", fragment, text)
				}
			}
			requireLedgerUnchanged(t, fixture.clone, head)
		})
	}
}

func TestAcceptRefusesAMalformedVerificationSkill(t *testing.T) {
	fixture := newLedgerFixture(t)
	cli := newLedgerApp(t, newForgeServer(t))
	head := ledgerHead(t, fixture.clone)
	journey := "| checkout | driven-surface | journey | `.skl/journeys/checkout.md` | the order is placed | tests |\n"
	for name, testCase := range map[string]struct {
		skill     string
		fragments []string
	}{
		"an unknown rank":            {strings.Replace(lintSkill, "| every test passes | tests |", "| every test passes | live |", 1), []string{".skl/verify.md", "line 24", "tests", "live", "corpus, tests, build"}},
		"a journey without its file": {strings.Replace(lintSkill, "| every route | |\n", "| every route | |\n"+journey, 1), []string{".skl/verify.md", "checkout", ".skl/journeys/checkout.md"}},
	} {
		t.Run(name, func(t *testing.T) {
			root := skillRepository(t, testCase.skill)
			outcome, text := cli.run(t, []string{"skl", "proposal", "accept", "--repo", root, "--proposal-dir", writeProposal(t, "", lintSpec("malformed", strategy("| B1, B2, A1 | gate | corpus |\n"))), "--format", "json"})
			if outcome.Status != "fix_required" {
				t.Fatalf("acceptance was not refused: %s", text)
			}
			for _, fragment := range testCase.fragments {
				if !strings.Contains(outcome.Reason, fragment) {
					t.Fatalf("refusal lacks %q: %s", fragment, text)
				}
			}
			requireLedgerUnchanged(t, fixture.clone, head)
		})
	}
}

// A Forbidden line may cite a project principle by name; acceptance resolves
// the name to a file of the source repository's principles.
func TestAcceptResolvesForbiddenPrincipleCitations(t *testing.T) {
	cited := lintedContract()
	cited["plan.md"] = strings.Replace(cited["plan.md"], "- A second representation.", "- No compatibility facade (principle:no-facade).", 1)
	principles := func(names ...string) string {
		root := sourceRepository(t, "acme", "widgets")
		for _, name := range names {
			writeFile(t, filepath.Join(root, ".skl", "principles", name+".md"), "---\nstatement: The "+name+" principle.\n---\n")
		}
		if len(names) > 0 {
			runGit(t, root, "add", ".")
			runGit(t, root, "commit", "-q", "-m", "principles")
		}
		return root
	}
	for _, testCase := range []struct {
		name      string
		root      string
		fragments []string
	}{
		{"a missing principle", principles("one-representation"), []string{"foundation", "plan.md", "no-facade", ".skl/principles/no-facade.md"}},
		{"no principles directory", principles(), []string{"foundation", "plan.md", "no-facade", ".skl/principles/ directory"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			head := ledgerHead(t, fixture.clone)
			outcome, text := newLedgerApp(t, newForgeServer(t)).run(t, []string{"skl", "proposal", "accept", "--repo", testCase.root, "--proposal-dir", writeProposal(t, "", lintSpec("cited", cited)), "--format", "json"})
			if outcome.Status != "fix_required" {
				t.Fatalf("acceptance was not refused: %s", text)
			}
			for _, fragment := range testCase.fragments {
				if !strings.Contains(outcome.Reason, fragment) {
					t.Fatalf("refusal lacks %q: %s", fragment, text)
				}
			}
			requireLedgerUnchanged(t, fixture.clone, head)
		})
	}
	newLedgerFixture(t)
	if outcome := newLedgerApp(t, newForgeServer(t)).accept(t, principles("no-facade"), writeProposal(t, "", lintSpec("cited", cited))); outcome.Status != "accepted" {
		t.Fatalf("accept citing an existing principle: %s", mustJSON(t, outcome))
	}
}
