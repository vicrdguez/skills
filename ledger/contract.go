package ledger

import (
	"bytes"
	"cmp"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// Ladder is an ordered list of evidence rank names, strongest first. A rank
// is recorded by its name, never by its position.
type Ladder []string

// BuiltinLadder is the ladder in force when the project defines none.
var BuiltinLadder = Ladder{"tests", "build", "inspection"}

// Has reports whether rank names a rank of the ladder.
func (l Ladder) Has(rank string) bool {
	for _, name := range l {
		if name == rank {
			return true
		}
	}
	return false
}

// String lists the ladder's ranks, strongest first.
func (l Ladder) String() string { return strings.Join(l, ", ") }

// ContractSources locates what Contract lint resolves outside the intake: the
// source repository that `docs/adr/` references name, and the Project's
// proposals directory in the ledger, which holds each Proposal's experiment
// reports and its recorded Contracts.
type ContractSources struct {
	Repository string
	Proposals  string
}

// ContractSources returns the lint sources of the repository's Project, whose
// source checkout is at root.
func (s *Store) ContractSources(root string, repository github.RepositoryID) ContractSources {
	return ContractSources{Repository: root, Proposals: filepath.Join(s.Root, projectsRoot, repository.Name, "proposals")}
}

// definitionHomes says where each label kind is defined: its document, and
// whether the definition is a heading or a checklist item.
var definitionHomes = map[byte]struct {
	document string
	heading  bool
}{
	'B': {"behavior.md", true},
	'A': {"plan.md", true},
	'T': {"tasks.md", false},
	'M': {"intent.md", false},
}

// contractDocuments is the order lint reads a slice's documents in.
var contractDocuments = []string{"intent.md", "behavior.md", "plan.md", "tasks.md"}

var (
	headingLine   = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)[ \t]*#*[ \t]*$`)
	checklistLine = regexp.MustCompile(`^[ \t]*[-*+][ \t]+\[[ xX]\][ \t]+(.*)$`)
	definedLabel  = regexp.MustCompile(`^([BATM])([0-9]+):`)
	citedLabel    = regexp.MustCompile(`\b([BATM])([0-9]+)(?:[ \t]*[–-][ \t]*([BATM]?)([0-9]+))?\b`)
	fenceLine     = regexp.MustCompile("^ {0,3}(```|~~~)")
)

// contractLine is one line of a Contract document outside fenced code, with
// its heading level (0 when it is no heading) or checklist text.
type contractLine struct {
	text      string
	level     int
	heading   string
	checklist bool
	item      string
}

func readContractLines(contents []byte) []contractLine {
	var lines []contractLine
	fenced := false
	for _, text := range strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n") {
		if fenceLine.MatchString(text) {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		line := contractLine{text: text}
		if match := headingLine.FindStringSubmatch(text); match != nil {
			line.level, line.heading = len(match[1]), match[2]
		} else if match := checklistLine.FindStringSubmatch(text); match != nil {
			line.checklist, line.item = true, match[1]
		}
		lines = append(lines, line)
	}
	return lines
}

// section returns the bounds of the lines under the first heading titled
// title, up to the next heading of the same or a higher level; both are 0
// when no heading has that title.
func section(lines []contractLine, title string) (start, end int) {
	for index, line := range lines {
		if line.level > 0 && strings.EqualFold(strings.TrimSpace(line.heading), title) {
			return index + 1, sectionEnd(lines, index)
		}
	}
	return 0, 0
}

// sectionEnd is the index of the next heading at the level of the heading at
// index or higher, or the end of the document.
func sectionEnd(lines []contractLine, index int) int {
	for next := index + 1; next < len(lines); next++ {
		if lines[next].level > 0 && lines[next].level <= lines[index].level {
			return next
		}
	}
	return len(lines)
}

// maxRange bounds the labels one range citation expands to; a wider range
// cites only its two ends, and its far end resolves to nothing.
const maxRange = 1000

// citations lists the labels a text cites, expanding ranges such as B1–B3.
func citations(text string) []string {
	var labels []string
	for _, match := range citedLabel.FindAllStringSubmatch(text, -1) {
		kind := match[1]
		first, _ := strconv.Atoi(match[2])
		labels = append(labels, kind+match[2])
		if match[4] == "" {
			continue
		}
		last, _ := strconv.Atoi(match[4])
		if (match[3] != "" && match[3] != kind) || last-first > maxRange {
			labels = append(labels, cmp.Or(match[3], kind)+match[4])
			continue
		}
		for number := first + 1; number <= last; number++ {
			labels = append(labels, kind+strconv.Itoa(number))
		}
	}
	return labels
}

// lintContract checks one slice's Contract against the label grammar, the
// Verification strategy and the sources its architectural commitments cite.
// It returns the first refusal, naming the slice, the document and the label.
func lintContract(slice string, contract map[string][]byte, ladder Ladder, sources ContractSources, proposal string) error {
	if _, ok := contract["plan.md"]; !ok {
		return refuse(
			"slice "+slice+" has no plan.md",
			"add plan.md to slice "+slice+" with at least its Verification strategy, Delegated and Forbidden sections",
		)
	}
	documents := make(map[string][]contractLine, len(contract))
	for name, contents := range contract {
		documents[name] = readContractLines(contents)
	}
	defined := map[string]bool{}
	var labels []string
	for _, name := range contractDocuments {
		lines := documents[name]
		manualStart, manualEnd := section(lines, "Manual verification")
		for index, line := range lines {
			text := line.heading
			if line.checklist {
				text = line.item
			}
			match := definedLabel.FindStringSubmatch(text)
			if match == nil || (line.level == 0 && !line.checklist) {
				continue
			}
			label := match[1] + match[2]
			home := definitionHomes[match[1][0]]
			form := "a checklist item"
			if home.heading {
				form = "a heading"
			}
			inManual := index >= manualStart && index < manualEnd
			if name != home.document || home.heading != (line.level > 0) || (label[0] == 'M' && !inManual) {
				where := home.document
				if label[0] == 'M' {
					where += " under Manual verification"
				}
				return refuse(
					"slice "+slice+" defines "+label+" in "+name+", and "+match[1]+"<n> labels are defined by "+form+" in "+where,
					"move "+label+" to "+form+" in "+where+", or relabel it",
				)
			}
			if defined[label] {
				return refuse(
					"slice "+slice+" defines "+label+" more than once in "+name,
					"define each label of slice "+slice+" exactly once",
				)
			}
			defined[label] = true
			labels = append(labels, label)
		}
	}
	for _, name := range contractDocuments {
		for _, line := range documents[name] {
			for _, label := range citations(line.text) {
				if !defined[label] {
					return refuse(
						"slice "+slice+" cites "+label+" in "+name+", and no document of the slice defines it",
						"define "+label+" in its own document, or correct the citation",
					)
				}
			}
		}
	}
	if err := lintDefinitionOfDone(slice, documents["intent.md"]); err != nil {
		return err
	}
	if err := lintVerificationStrategy(slice, documents["plan.md"], labels, ladder); err != nil {
		return err
	}
	return lintSources(slice, documents["plan.md"], sources, proposal)
}

// lintDefinitionOfDone requires every checklist bullet under the Definition of
// Done of intent.md, with its continuation lines, to cite at least one label.
func lintDefinitionOfDone(slice string, lines []contractLine) error {
	start, end := section(lines, "Definition of Done")
	for index := start; index < end; index++ {
		if !lines[index].checklist {
			continue
		}
		text := lines[index].item
		for next := index + 1; next < end; next++ {
			line := lines[next]
			if line.checklist || line.level > 0 || strings.TrimSpace(line.text) == "" {
				break
			}
			text += " " + strings.TrimSpace(line.text)
		}
		if len(citations(text)) == 0 {
			return refuse(
				"slice "+slice+" has a Definition of Done bullet in intent.md that cites no label: "+strconv.Quote(lines[index].item),
				"cite the B<n>, A<n> or T<n> labels the bullet covers",
			)
		}
	}
	return nil
}

// lintVerificationStrategy requires the first table under the Verification
// strategy of plan.md to have Covers, Check and Rank columns, a known rank in
// every row, and a row covering every B<n> and A<n> label.
func lintVerificationStrategy(slice string, lines []contractLine, labels []string, ladder Ladder) error {
	start, end := section(lines, "Verification strategy")
	var table [][]string
	for index := start; index < end; index++ {
		text := strings.TrimSpace(lines[index].text)
		if !strings.HasPrefix(text, "|") {
			if table != nil {
				break
			}
			continue
		}
		table = append(table, tableCells(text))
	}
	if len(table) < 2 || !tableHeader(table[0]) || !tableDelimiter(table[1]) {
		return refuse(
			"slice "+slice+" has no Verification strategy table with Covers, Check and Rank columns in plan.md",
			"add a Markdown table under ## Verification strategy in plan.md whose header is | Covers | Check | Rank |",
		)
	}
	covered := map[string]bool{}
	for row, cells := range table[2:] {
		cells = append(cells, "", "", "")
		rank := strings.Trim(cells[2], "` ")
		if !ladder.Has(rank) {
			return refuse(
				"slice "+slice+" names the unknown rank "+strconv.Quote(rank)+" in row "+strconv.Itoa(row+1)+" (Covers "+cells[0]+") of the plan.md Verification strategy; the ladder in force is "+ladder.String(),
				"name one of "+ladder.String()+" in the Rank cell",
			)
		}
		for _, label := range citations(cells[0]) {
			covered[label] = true
		}
	}
	for _, label := range labels {
		if (label[0] == 'B' || label[0] == 'A') && !covered[label] {
			return refuse(
				"slice "+slice+" has no plan.md Verification strategy row covering "+label,
				"add "+label+" to the Covers cell of the rows whose checks verify it",
			)
		}
	}
	return nil
}

// tableCells splits one Markdown table row on its unescaped pipes.
func tableCells(row string) []string {
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	var cells []string
	var cell strings.Builder
	for index := 0; index < len(row); index++ {
		switch {
		case row[index] == '\\' && index+1 < len(row) && row[index+1] == '|':
			cell.WriteByte('|')
			index++
		case row[index] == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteByte(row[index])
		}
	}
	return append(cells, strings.TrimSpace(cell.String()))
}

func tableHeader(cells []string) bool {
	return len(cells) == 3 && strings.EqualFold(cells[0], "Covers") && strings.EqualFold(cells[1], "Check") && strings.EqualFold(cells[2], "Rank")
}

var delimiterCell = regexp.MustCompile(`^:?-+:?$`)

func tableDelimiter(cells []string) bool {
	for _, cell := range cells {
		if !delimiterCell.MatchString(cell) {
			return false
		}
	}
	return true
}

var experimentReport = regexp.MustCompile(`^experiments/[^/]+\.md$`)

// lintSources requires every A<n> section of plan.md to carry a Source: line
// whose every reference resolves: an ADR in the source repository, an
// experiment report of the Proposal in the ledger, or proposal.md.
func lintSources(slice string, lines []contractLine, sources ContractSources, proposal string) error {
	for index, line := range lines {
		match := definedLabel.FindStringSubmatch(line.heading)
		if line.level == 0 || match == nil || match[1] != "A" {
			continue
		}
		label := match[1] + match[2]
		var references []string
		found := false
		for _, inner := range lines[index+1 : sectionEnd(lines, index)] {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(inner.text), "Source:"); ok {
				found = true
				for _, reference := range strings.Split(rest, ",") {
					if reference = strings.Trim(reference, "` \t"); reference != "" {
						references = append(references, reference)
					}
				}
				break
			}
		}
		if !found || len(references) == 0 {
			return refuse(
				"slice "+slice+" has no Source: line naming a reference in the "+label+" section of plan.md",
				"add a Source: line to "+label+" naming its ADR under docs/adr/, its experiments/<name>.md report or proposal.md",
			)
		}
		for _, reference := range references {
			if !sources.resolves(reference, proposal) {
				return refuse(
					"slice "+slice+" cites the source "+reference+" in the "+label+" section of plan.md, and it resolves to nothing",
					"cite an ADR that exists under docs/adr/ in the source repository, an experiments/<name>.md report recorded for proposal "+proposal+", or proposal.md",
				)
			}
		}
	}
	return nil
}

func (c ContractSources) resolves(reference, proposal string) bool {
	if reference == "proposal.md" {
		return true
	}
	if path.Clean(reference) != reference {
		return false
	}
	var file string
	switch {
	case strings.HasPrefix(reference, "docs/adr/") && c.Repository != "":
		file = filepath.Join(c.Repository, filepath.FromSlash(reference))
	case experimentReport.MatchString(reference) && c.Proposals != "":
		file = filepath.Join(c.Proposals, proposal, filepath.FromSlash(reference))
	default:
		return false
	}
	info, err := os.Stat(file)
	return err == nil && info.Mode().IsRegular()
}

// recordedUnchanged reports whether the slice's recorded Contract in the
// ledger holds exactly the intake's Contract files, so lint leaves it as
// recorded.
func (c ContractSources) recordedUnchanged(proposal, slice string, contract map[string][]byte) bool {
	if c.Proposals == "" {
		return false
	}
	for _, name := range contractDocuments {
		recorded, err := os.ReadFile(filepath.Join(c.Proposals, proposal, slice, name))
		declared, supplied := contract[name]
		switch {
		case err != nil && !os.IsNotExist(err):
			return false
		case err != nil && supplied, err == nil && (!supplied || !bytes.Equal(recorded, declared)):
			return false
		}
	}
	return exists(filepath.Join(c.Proposals, proposal, slice, "state.json"))
}
