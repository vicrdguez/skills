// Command prosemetrics prints the agent-prose metrics of every golden in
// testdata/prose beside its committed baseline. docs/agent-prose.md explains
// the metrics; `-record` rewrites the baseline from the current goldens.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	baselineFile = "baseline.tsv"
	journeyRow   = "journey: implement → watchdog"
)

// journey is what a worker reads during one initial Implement and one first
// Watchdog review: the renderings plus the resources those Procedures always
// retrieve, in reading order. A resource read in both phases counts twice.
var journey = []string{
	"implement-start",
	"implement-prepare",
	"implement-inspect",
	"resource-audit-smells",
	"resource-audit-acceptance",
	"resource-implement-ledger-submission",
	"outcome-implement-submit",
	"watchdog-start",
	"watchdog-prepare",
	"watchdog-inspect",
	"resource-audit-acceptance",
	"resource-watchdog-ledger-review",
	"outcome-watchdog-submit-rework",
}

var negations = words("no not never none nor neither nothing nobody nowhere without cannot")

// abstractTerms name a workflow-policy property rather than an action or an
// object the worker handles.
var abstractTerms = words(`authority authorities authorization obligation obligations invariant invariants
integrity provenance semantics conformance disposition dispositions eligibility accounting attestation
reconciliation boundary boundaries lifecycle identity identities commitment commitments consequence
consequences cardinality chronology sensitivity freshness fidelity precedence`)

func words(list string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(list) {
		set[word] = true
	}
	return set
}

// counts are one text's raw tallies; densities derive from them.
type counts struct {
	Words, Negations, Abstract int
}

func (c counts) add(other counts) counts {
	return counts{c.Words + other.Words, c.Negations + other.Negations, c.Abstract + other.Abstract}
}

func perThousand(n, words int) float64 {
	if words == 0 {
		return 0
	}
	return float64(n) * 1000 / float64(words)
}

// measure counts text after removing every supplied fixture document, so only
// the prose the engine authored is measured.
func measure(text string, fixtures []string) counts {
	for _, fixture := range fixtures {
		text = strings.ReplaceAll(text, fixture, "")
	}
	var c counts
	for _, field := range strings.Fields(text) {
		c.Words++
		token := strings.ToLower(strings.TrimFunc(strings.ReplaceAll(field, "’", "'"), func(r rune) bool {
			return !unicode.IsLetter(r) && r != '\''
		}))
		if negations[token] || strings.HasSuffix(token, "n't") {
			c.Negations++
		}
		if abstractTerms[token] {
			c.Abstract++
		}
	}
	return c
}

// partialFixture returns the heading of a fixture whose heading survives the
// removal of every whole fixture: that document was embedded altered.
func partialFixture(text string, fixtures []string) string {
	for _, fixture := range fixtures {
		text = strings.ReplaceAll(text, fixture, "")
	}
	for _, fixture := range fixtures {
		heading, _, _ := strings.Cut(fixture, "\n")
		if strings.Contains(text, heading+"\n") {
			return heading
		}
	}
	return ""
}

type row struct {
	Name     string
	Current  *counts
	Baseline *counts
}

// measureDirectory measures every golden in dir and appends the journey total.
func measureDirectory(dir string) (map[string]counts, error) {
	fixtures, err := filepath.Glob(filepath.Join(dir, "fixtures", "*.md"))
	if err != nil {
		return nil, err
	}
	var excluded []string
	for _, path := range fixtures {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		excluded = append(excluded, string(contents))
	}
	goldens, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	measured := map[string]counts{}
	for _, path := range goldens {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if fixture := partialFixture(string(contents), excluded); fixture != "" {
			return nil, fmt.Errorf("%s embeds fixture %q altered, so its words would count as authored prose", path, fixture)
		}
		measured[strings.TrimSuffix(filepath.Base(path), ".md")] = measure(string(contents), excluded)
	}
	var total counts
	for _, name := range journey {
		c, ok := measured[name]
		if !ok {
			return nil, fmt.Errorf("journey golden %s.md is missing from %s", name, dir)
		}
		total = total.add(c)
	}
	measured[journeyRow] = total
	return measured, nil
}

func readBaseline(path string) (map[string]counts, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	baseline := map[string]counts{}
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		fields := strings.Split(scanner.Text(), "\t")
		if line == 1 {
			continue
		}
		if len(fields) != 4 {
			return nil, fmt.Errorf("%s:%d: want 4 tab-separated fields", path, line)
		}
		var values [3]int
		for i, field := range fields[1:] {
			if values[i], err = strconv.Atoi(field); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, line, err)
			}
		}
		baseline[fields[0]] = counts{values[0], values[1], values[2]}
	}
	return baseline, scanner.Err()
}

func writeBaseline(path string, measured map[string]counts) error {
	var b strings.Builder
	b.WriteString("rendering\twords\tnegations\tabstract_terms\n")
	for _, name := range ordered(measured, nil) {
		c := measured[name]
		fmt.Fprintf(&b, "%s\t%d\t%d\t%d\n", name, c.Words, c.Negations, c.Abstract)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ordered lists every name alphabetically with the journey total last.
func ordered(sets ...map[string]counts) []string {
	seen := map[string]bool{}
	var names []string
	for _, set := range sets {
		for name := range set {
			if !seen[name] && name != journeyRow {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return append(names, journeyRow)
}

// report measures dir and pairs each row with its baseline; record first
// rewrites the baseline from the current goldens.
func report(dir string, record bool) ([]row, error) {
	measured, err := measureDirectory(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, baselineFile)
	if record {
		if err := writeBaseline(path, measured); err != nil {
			return nil, err
		}
	}
	baseline, err := readBaseline(path)
	if err != nil {
		return nil, err
	}
	var rows []row
	for _, name := range ordered(measured, baseline) {
		r := row{Name: name}
		if c, ok := measured[name]; ok {
			r.Current = &c
		}
		if c, ok := baseline[name]; ok {
			r.Baseline = &c
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func printTable(w io.Writer, rows []row) {
	fmt.Fprintln(w, "| Rendering | Words | Baseline | Negations/1k | Baseline | Abstract/1k | Baseline |")
	fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
	cells := func(c *counts) [3]string {
		if c == nil {
			return [3]string{"—", "—", "—"}
		}
		return [3]string{
			strconv.Itoa(c.Words),
			strconv.FormatFloat(perThousand(c.Negations, c.Words), 'f', 1, 64),
			strconv.FormatFloat(perThousand(c.Abstract, c.Words), 'f', 1, 64),
		}
	}
	for _, r := range rows {
		current, baseline := cells(r.Current), cells(r.Baseline)
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n", r.Name, current[0], baseline[0], current[1], baseline[1], current[2], baseline[2])
	}
}

func main() {
	dir := flag.String("dir", filepath.Join("testdata", "prose"), "directory holding the goldens, fixtures/ and "+baselineFile)
	record := flag.Bool("record", false, "rewrite the baseline from the current goldens before printing")
	flag.Parse()
	rows, err := report(*dir, *record)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	printTable(os.Stdout, rows)
}
