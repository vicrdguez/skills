package ledger_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// verificationSkill is a well-formed skill declaring corpus, tests and build,
// with four rows of kinds executed and generated.
const verificationSkill = `---
ranks: [corpus, tests, build]
---

# Engine verification

## Launch

Run from the repository root.

## Doctor

` + "`mise doctor`" + ` reports no problem.

## Channels

- ` + "`oracle`" + `: the conformance corpus.
- ` + "`tests`" + `: the package tests.

## Catalog

| Name | Channel | Kind | Run | Expectations | Rank |
| --- | --- | --- | --- | --- | --- |
| gate | oracle | executed | ` + "`mise run gate`" + ` | every corpus case passes | corpus |
| tests | tests | executed | ` + "`go test ./...`" + ` | every test passes | tests |
| build | static | executed | ` + "`go build ./...`" + ` | exits 0 | build |
| tasks | static | generated | ` + "`mise tasks`" + ` | every task the rows run | |

## Cleanup

Nothing to clean.

## Extend

Add a corpus case for a new rule.

## Traps

The gate refuses a dirty checkout.
`

func writeSkill(t *testing.T, contents string, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, file := range append([]string{".skl/verify.md"}, files...) {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "# Journey\n"
		if file == ".skl/verify.md" {
			body = contents
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestReadVerificationReadsTheLadderAndCatalog(t *testing.T) {
	verification, err := ledger.ReadVerification(writeSkill(t, verificationSkill))
	if err != nil {
		t.Fatal(err)
	}
	if got := verification.Ladder.String(); got != "corpus, tests, build" {
		t.Fatalf("ladder = %q, want corpus, tests, build", got)
	}
	if len(verification.Catalog) != 4 {
		t.Fatalf("catalog = %#v, want four rows", verification.Catalog)
	}
	gate := verification.Row("gate")
	if gate == nil || gate.Run != "mise run gate" || gate.Kind != ledger.ExecutedCheck || gate.Rank != "corpus" || gate.Expectations != "every corpus case passes" {
		t.Fatalf("gate = %#v", gate)
	}
	if !slices.Equal(verification.Channels, []string{"oracle", "tests"}) {
		t.Fatalf("channels = %v, want oracle and tests", verification.Channels)
	}
}

func TestReadVerificationIsAbsentWithoutTheFile(t *testing.T) {
	verification, err := ledger.ReadVerification(t.TempDir())
	if err != nil || verification != nil {
		t.Fatalf("a repository without a skill read %#v, %v", verification, err)
	}
	if got := verification.LadderInForce().String(); got != "tests, build, inspection" {
		t.Fatalf("ladder without a skill = %q, want the built-in ladder", got)
	}
}

func TestReadVerificationRefusesMalformedSkills(t *testing.T) {
	replace := func(old, new string) string {
		if !strings.Contains(verificationSkill, old) {
			t.Fatalf("the skill holds no %q", old)
		}
		return strings.Replace(verificationSkill, old, new, 1)
	}
	journey := "| walk | driven-surface | journey | `.skl/journeys/checkout.md` | the order is placed | tests |\n"
	cases := []struct {
		name      string
		contents  string
		files     []string
		fragments []string
	}{
		{"an unknown rank", replace("| exits 0 | build |", "| exits 0 | live |"),
			nil, []string{".skl/verify.md", "line 26", "build", "live", "corpus, tests, build"}},
		{"a journey without its file", replace("| every task the rows run | |\n", "| every task the rows run | |\n"+journey),
			nil, []string{".skl/verify.md", "walk", ".skl/journeys/checkout.md"}},
		{"a journey outside the journey directory", replace("| every task the rows run | |\n", "| every task the rows run | |\n"+strings.Replace(journey, ".skl/journeys/checkout.md", "docs/checkout.md", 1)),
			[]string{"docs/checkout.md"}, []string{"walk", "docs/checkout.md"}},
		{"a generated row with a rank", replace("| every task the rows run | |", "| every task the rows run | tests |"),
			nil, []string{"line 27", "tasks", "tests"}},
		{"a repeated name", replace("| build | static |", "| gate | static |"),
			nil, []string{"line 26", "gate"}},
		{"an unknown kind", replace("| tests | executed |", "| tests | suite |"),
			nil, []string{"line 25", "tests", "suite"}},
		{"a missing heading", replace("## Doctor", "## Health"),
			nil, []string{".skl/verify.md", "Doctor"}},
		{"a catalog without its table", replace("| Name | Channel |", "| Title | Channel |"),
			nil, []string{".skl/verify.md", "line 22", "Catalog"}},
		{"duplicate ranks", replace("ranks: [corpus, tests, build]", "ranks: [corpus, tests, tests]"),
			nil, []string{".skl/verify.md", "line 2", "tests"}},
		{"multiline duplicate ranks", replace("ranks: [corpus, tests, build]", "ranks:\n  - corpus\n  - tests\n  - tests"),
			nil, []string{".skl/verify.md", "line 5", "tests"}},
		{"no ranks", replace("ranks: [corpus, tests, build]", "ladder: [corpus]"),
			nil, []string{".skl/verify.md", "line 2", "ranks"}},
		{"no frontmatter", strings.SplitN(verificationSkill, "---\n", 3)[2],
			nil, []string{".skl/verify.md", "line 1", "frontmatter"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ledger.ReadVerification(writeSkill(t, testCase.contents, testCase.files...))
			if err == nil {
				t.Fatal("the malformed skill was read")
			}
			for _, fragment := range testCase.fragments {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("refusal lacks %q: %v", fragment, err)
				}
			}
		})
	}
	withJourney := replace("| every task the rows run | |\n", "| every task the rows run | |\n"+journey)
	if _, err := ledger.ReadVerification(writeSkill(t, withJourney, ".skl/journeys/checkout.md")); err != nil {
		t.Fatalf("a journey row with its file was refused: %v", err)
	}
}

// Delivery reads the skill committed at a revision, journeys included.
func TestReadVerificationAtReadsACommittedSkill(t *testing.T) {
	journey := "| walk | driven-surface | journey | `.skl/journeys/checkout.md` | the order is placed | tests |\n"
	root := writeSkill(t, strings.Replace(verificationSkill, "| every task the rows run | |\n", "| every task the rows run | |\n"+journey, 1), ".skl/journeys/checkout.md")
	git := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", append([]string{"-C", root, "-c", "user.name=T", "-c", "user.email=t@example.com"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "skill")
	head := git("rev-parse", "HEAD")
	// The working tree no longer matters once the skill is committed.
	if err := os.RemoveAll(filepath.Join(root, ".skl")); err != nil {
		t.Fatal(err)
	}
	verification, err := ledger.ReadVerificationAt(root, head)
	if err != nil || verification == nil || verification.Row("walk") == nil {
		t.Fatalf("committed skill = %#v, %v", verification, err)
	}
	git("rm", "-q", "--cached", ".skl/journeys/checkout.md")
	git("commit", "-q", "-m", "drop the journey")
	if _, err := ledger.ReadVerificationAt(root, "HEAD"); err == nil || !strings.Contains(err.Error(), ".skl/journeys/checkout.md") {
		t.Fatalf("a revision without the journey file was read: %v", err)
	}
	if _, err := ledger.ReadVerificationAt(root, strings.Repeat("0", 40)); err == nil {
		t.Fatal("an unavailable revision read as a repository without a skill")
	}
	git("rm", "-q", "--cached", ".skl/verify.md")
	git("commit", "-q", "-m", "drop the skill")
	if verification, err := ledger.ReadVerificationAt(root, "HEAD"); err != nil || verification != nil {
		t.Fatalf("a revision without the skill read %#v, %v", verification, err)
	}
}

func TestRankBoundIsTheWeakestCitedRowCoveringARule(t *testing.T) {
	verification, err := ledger.ReadVerification(writeSkill(t, verificationSkill))
	if err != nil {
		t.Fatal(err)
	}
	plan := []byte("# Plan\n\n## Verification strategy\n\n| Covers | Check | Rank |\n| --- | --- | --- |\n" +
		"| B1 | gate 17 | corpus |\n| B2 | tests TestTwo | tests |\n| A1 | build | build |\n| A1 | read the parser against A1 | build |\n| B2 | new corpus-case 18 | corpus |\n")
	checks := ledger.CitedChecks(plan, verification)
	var names []string
	for _, check := range checks {
		names = append(names, check.Name)
	}
	if !slices.Equal(names, []string{"gate", "tests", "build", "corpus-case"}) {
		t.Fatalf("cited checks = %v, want gate, tests, build and corpus-case", names)
	}
	if gate := checks[0]; gate.Case != "17" || gate.Run != "mise run gate" || gate.Covers != "B1" {
		t.Fatalf("gate = %#v", gate)
	}
	if added := checks[3]; !added.Added || !added.Missing {
		t.Fatalf("corpus-case = %#v, want an added row the catalog lacks", added)
	}
	if rank, row := ledger.RankBound(checks, verification.Ladder); rank != "tests" || row != "tests" {
		t.Fatalf("bound = %s from %s, want tests from tests", rank, row)
	}
	if ledger.CitedChecks(plan, nil) != nil {
		t.Fatal("a repository without a skill cited checks")
	}
}

// This repository's own skill is well formed and catalogs the goldens, the
// CLI tests, the Go tests and the build.
func TestRepositoryVerificationSkill(t *testing.T) {
	verification, err := ledger.ReadVerification("..")
	if err != nil || verification == nil {
		t.Fatalf("repository skill = %#v, %v", verification, err)
	}
	for _, name := range []string{"goldens", "cli-tests", "go-tests", "build"} {
		if row := verification.Row(name); row == nil || row.Kind != ledger.ExecutedCheck {
			t.Errorf("repository skill has no executed row %s", name)
		}
	}
}
