package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeasureCountsAuthoredProseOnly(t *testing.T) {
	fixture := "# Contract\n\nNever mind this obligation.\n"
	text := "Do not edit the Contract. It isn't your authority, and no boundary moves.\n\n" + fixture
	// 13 words; negations: not, isn't, no; abstract terms: authority, boundary.
	if got, want := measure(text, []string{fixture}), (counts{Words: 13, Negations: 3, Abstract: 2}); got != want {
		t.Fatalf("measure = %+v, want %+v", got, want)
	}
}

// copyGoldens copies the committed goldens and fixtures into a scratch
// directory so each scenario owns its baseline.
func copyGoldens(t *testing.T) string {
	t.Helper()
	source := filepath.Join("..", "..", "testdata", "prose")
	dir := t.TempDir()
	for _, pattern := range []string{"*.md", filepath.Join("fixtures", "*.md")} {
		paths, err := filepath.Glob(filepath.Join(source, pattern))
		if err != nil || len(paths) == 0 {
			t.Fatalf("no %s under %s: %v", pattern, source, err)
		}
		for _, path := range paths {
			relative, _ := filepath.Rel(source, path)
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, relative)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, contents, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

func rowNamed(t *testing.T, rows []row, name string) row {
	t.Helper()
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no row %q", name)
	return row{}
}

// TestUnchangedTreeMatchesBaseline measures the tree the committed baseline
// was recorded from: the goldens and fixtures of the commit that last changed
// baseline.tsv. Later goldens may diverge from the baseline on purpose.
func TestUnchangedTreeMatchesBaseline(t *testing.T) {
	root := filepath.Join("..", "..")
	commit := gitOutput(t, root, "log", "-1", "--format=%H", "--", "testdata/prose/baseline.tsv")
	if commit == "" {
		t.Skip("baseline.tsv is not committed yet")
	}
	if gitOutput(t, root, "status", "--porcelain", "--", "testdata/prose/baseline.tsv") != "" {
		t.Skip("baseline.tsv has uncommitted changes")
	}
	dir := t.TempDir()
	archive := exec.Command("git", "-C", root, "archive", "--format=tar", commit, "testdata/prose")
	extract := exec.Command("tar", "-x", "-C", dir)
	var err error
	if extract.Stdin, err = archive.StdoutPipe(); err != nil {
		t.Fatal(err)
	}
	if err := extract.Start(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Run(); err != nil {
		t.Fatal(err)
	}
	if err := extract.Wait(); err != nil {
		t.Fatal(err)
	}
	rows, err := report(filepath.Join(dir, "testdata", "prose"), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Current == nil || r.Baseline == nil || *r.Current != *r.Baseline {
			t.Errorf("row %s at %s: current %+v, baseline %+v", r.Name, commit, r.Current, r.Baseline)
		}
	}
	if journey := rowNamed(t, rows, journeyRow); journey.Current.Words <= rowNamed(t, rows, "implement-start").Current.Words {
		t.Errorf("journey total %d does not exceed the Implement start rendering it includes", journey.Current.Words)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(output))
}

func TestMissingBaselineIsAnError(t *testing.T) {
	dir := copyGoldens(t)
	if _, err := report(dir, false); err == nil {
		t.Fatal("report without baseline.tsv succeeded")
	}
}

func TestAlteredFixtureIsAnError(t *testing.T) {
	dir := copyGoldens(t)
	path := filepath.Join(dir, "implement-start.md")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	altered := strings.Replace(string(contents), "the widget is listed as unhealthy", "the widget is listed", 1)
	if altered == string(contents) {
		t.Fatal("implement-start no longer embeds the behavior fixture")
	}
	if err := os.WriteFile(path, []byte(altered), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := report(dir, true); err == nil || !strings.Contains(err.Error(), "Dashboard foundation behavior") {
		t.Fatalf("altered fixture: err = %v, want a refusal naming the fixture", err)
	}
}

func TestShorterRenderingShowsReduction(t *testing.T) {
	dir := copyGoldens(t)
	if _, err := report(dir, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "implement-start.md")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	paragraphs := strings.Split(string(contents), "\n\n")
	shorter := strings.Join(append(paragraphs[:3:3], paragraphs[4:]...), "\n\n")
	if err := os.WriteFile(path, []byte(shorter), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := report(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"implement-start", journeyRow} {
		r := rowNamed(t, rows, name)
		if r.Current.Words >= r.Baseline.Words {
			t.Errorf("%s: %d words, want fewer than baseline %d", name, r.Current.Words, r.Baseline.Words)
		}
	}
	if r := rowNamed(t, rows, "watchdog-start"); *r.Current != *r.Baseline {
		t.Errorf("unchanged watchdog-start moved: %+v, baseline %+v", r.Current, r.Baseline)
	}
}
