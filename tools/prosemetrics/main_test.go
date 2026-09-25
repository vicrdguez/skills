package main

import (
	"os"
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

func TestUnchangedTreeMatchesBaseline(t *testing.T) {
	dir := copyGoldens(t)
	if _, err := report(dir, true); err != nil {
		t.Fatal(err)
	}
	rows, err := report(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Current == nil || r.Baseline == nil || *r.Current != *r.Baseline {
			t.Errorf("row %s: current %+v, baseline %+v", r.Name, r.Current, r.Baseline)
		}
	}
	if journey := rowNamed(t, rows, journeyRow); journey.Current.Words <= rowNamed(t, rows, "implement-start").Current.Words {
		t.Errorf("journey total %d does not exceed the Implement start rendering it includes", journey.Current.Words)
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
