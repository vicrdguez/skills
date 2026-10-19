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
	// Verification is the source repository's verification skill, or nil
	// when it has none.
	Verification *Verification
}

// ContractSources returns the lint sources of the repository's Project, whose
// source checkout is at root, together with that checkout's verification
// skill. A malformed skill is refused.
func (s *Store) ContractSources(root string, repository github.RepositoryID) (ContractSources, error) {
	verification, err := ReadVerification(root)
	if err != nil {
		return ContractSources{}, err
	}
	return ContractSources{Repository: root, Proposals: filepath.Join(s.Root, projectsRoot, repository.Name, "proposals"), Verification: verification}, nil
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
// its 1-based line number, its heading level (0 when it is no heading) or
// checklist text.
type contractLine struct {
	number    int
	text      string
	level     int
	heading   string
	checklist bool
	item      string
}

func readContractLines(contents []byte) []contractLine {
	var lines []contractLine
	fenced := false
	for index, text := range strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n") {
		if fenceLine.MatchString(text) {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		line := contractLine{number: index + 1, text: text}
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

// citations lists the labels a text cites, expanding ranges such as B1–B3;
// a range across kinds or written high to low cites its two ends.
// A range expands only while its labels are defined: the list ends at the
// first undefined one, so a range of any width costs at most one label more
// than the slice defines.
func citations(text string, defined map[string]bool) []string {
	var labels []string
	for _, match := range citedLabel.FindAllStringSubmatch(text, -1) {
		kind := match[1]
		first, _ := strconv.Atoi(match[2])
		labels = append(labels, kind+match[2])
		if match[4] == "" {
			continue
		}
		last, _ := strconv.Atoi(match[4])
		if (match[3] != "" && match[3] != kind) || last < first {
			labels = append(labels, cmp.Or(match[3], kind)+match[4])
			continue
		}
		for number := first + 1; number <= last; number++ {
			label := kind + strconv.Itoa(number)
			labels = append(labels, label)
			if !defined[label] {
				break
			}
		}
	}
	return labels
}

// lintContract checks one slice's Contract against the label grammar, the
// Verification strategy and the sources its architectural commitments cite.
// It returns the first refusal, naming the slice, the document and the label.
func lintContract(slice string, contract map[string][]byte, sources ContractSources, proposal string) error {
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
	// Citations count anywhere in the text, fenced code included.
	for _, name := range contractDocuments {
		for _, label := range citations(string(contract[name]), defined) {
			if !defined[label] {
				return refuse(
					"slice "+slice+" cites "+label+" in "+name+", and no document of the slice defines it",
					"define "+label+" in its own document, or correct the citation",
				)
			}
		}
	}
	done := definitionOfDone(documents["intent.md"])
	if err := lintDefinitionOfDone(slice, done, defined); err != nil {
		return err
	}
	if err := lintVerificationStrategy(slice, documents["plan.md"], labels, defined, sources.Verification, done); err != nil {
		return err
	}
	if err := lintSources(slice, documents["plan.md"], sources, proposal); err != nil {
		return err
	}
	return lintForbidden(slice, documents["plan.md"], sources)
}

// principleCitation is the one form the engine reads from Forbidden: a
// project principle cited by name.
var principleCitation = regexp.MustCompile(`principle:([A-Za-z0-9][A-Za-z0-9_-]*)`)

// lintForbidden resolves each principle the plan.md Forbidden section cites
// to a file of the source repository's principles directory.
func lintForbidden(slice string, lines []contractLine, sources ContractSources) error {
	start, end := section(lines, "Forbidden")
	for _, line := range lines[start:end] {
		for _, match := range principleCitation.FindAllStringSubmatch(line.text, -1) {
			file := PrinciplesDirectory + match[1] + ".md"
			if info, err := os.Stat(filepath.Join(sources.Repository, filepath.FromSlash(file))); sources.Repository != "" && err == nil && info.Mode().IsRegular() {
				continue
			}
			missing := "the repository has no " + file
			if sources.Repository == "" || !exists(filepath.Join(sources.Repository, filepath.FromSlash(PrinciplesDirectory))) {
				missing = "the repository has no " + PrinciplesDirectory + " directory"
			}
			return refuse(
				"slice "+slice+" cites principle:"+match[1]+" in the Forbidden section of plan.md, and "+missing,
				"cite a principle the source repository keeps in "+PrinciplesDirectory+"<name>.md, or write "+file+" there first",
			)
		}
	}
	return nil
}

// doneBullet is one checklist bullet under the Definition of Done of
// intent.md: its first line, and its text with the continuation lines.
type doneBullet struct {
	item string
	text string
}

func definitionOfDone(lines []contractLine) []doneBullet {
	var bullets []doneBullet
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
		bullets = append(bullets, doneBullet{item: lines[index].item, text: text})
	}
	return bullets
}

// lintDefinitionOfDone requires every Definition of Done bullet to cite at
// least one label.
func lintDefinitionOfDone(slice string, bullets []doneBullet, defined map[string]bool) error {
	for _, bullet := range bullets {
		if len(citations(bullet.text, defined)) == 0 {
			return refuse(
				"slice "+slice+" has a Definition of Done bullet in intent.md that cites no label: "+strconv.Quote(bullet.item),
				"cite the B<n>, A<n> or T<n> labels the bullet covers",
			)
		}
	}
	return nil
}

// strategyRow is one row of the plan.md Verification strategy, with the
// labels its Covers cell cites.
type strategyRow struct {
	covers, check, rank string
	labels              []string
}

// rule is the first B<n> label the row covers, or "" when it covers none.
func (r strategyRow) rule() string {
	for _, label := range r.labels {
		if label[0] == 'B' {
			return label
		}
	}
	return ""
}

// verificationStrategy reads the first table under the Verification strategy
// of plan.md; ok is false when the table or its Covers, Check and Rank header
// is missing.
func verificationStrategy(lines []contractLine, defined map[string]bool) (rows []strategyRow, ok bool) {
	start, end := section(lines, "Verification strategy")
	table, _ := firstTable(lines, start, end)
	if len(table) < 2 || !tableHeader(table[0], "Covers", "Check", "Rank") || !tableDelimiter(table[1]) {
		return nil, false
	}
	for _, cells := range table[2:] {
		// Pad a short row so its Covers and Rank cells read as empty.
		cells = append(cells, "", "", "")
		rows = append(rows, strategyRow{covers: cells[0], check: cells[1], rank: strings.Trim(cells[2], "` "), labels: citations(cells[0], defined)})
	}
	return rows, true
}

// catalogCitation reads the catalog row a Check cell names: its first word,
// or the word after a leading new, which declares a row the slice adds. The
// rest of the cell is the case identifier.
func catalogCitation(check string) (name, identifier string, added bool) {
	words := strings.Fields(check)
	if len(words) > 1 && strings.Trim(words[0], "`") == "new" {
		added, words = true, words[1:]
	}
	if len(words) == 0 {
		return "", "", false
	}
	return strings.Trim(words[0], "`"), strings.Join(words[1:], " "), added
}

// lintVerificationStrategy requires the Verification strategy table, a rank of
// the ladder in force in every row, and a row covering every B<n> and A<n>
// label. With a catalog, a row covering a B<n> cites an executed or journey
// row, and a row declared new is named by a Definition of Done bullet.
func lintVerificationStrategy(slice string, lines []contractLine, labels []string, defined map[string]bool, verification *Verification, done []doneBullet) error {
	rows, ok := verificationStrategy(lines, defined)
	if !ok {
		return refuse(
			"slice "+slice+" has no Verification strategy table with Covers, Check and Rank columns in plan.md",
			"add a Markdown table under ## Verification strategy in plan.md whose header is | Covers | Check | Rank |",
		)
	}
	ladder := verification.LadderInForce()
	covered := map[string]bool{}
	for index, row := range rows {
		if !ladder.Has(row.rank) {
			return refuse(
				"slice "+slice+" names the unknown rank "+strconv.Quote(row.rank)+" in row "+strconv.Itoa(index+1)+" (Covers "+row.covers+") of the plan.md Verification strategy; the ladder in force is "+ladder.String(),
				"name one of "+ladder.String()+" in the Rank cell",
			)
		}
		if err := lintCitation(slice, row, verification, done); err != nil {
			return err
		}
		for _, label := range row.labels {
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

// lintCitation checks the catalog row one Verification strategy row cites.
// Without a catalog, every Check cell is free text.
func lintCitation(slice string, row strategyRow, verification *Verification, done []doneBullet) error {
	if verification == nil {
		return nil
	}
	name, _, added := catalogCitation(row.check)
	rule := row.rule()
	cited := verification.Row(name)
	if rule != "" && cited != nil && cited.Kind != ExecutedCheck && cited.Kind != JourneyCheck {
		return refuse(
			"slice "+slice+" cites the "+cited.Kind+" catalog row "+strconv.Quote(name)+" for "+rule+" in the plan.md Verification strategy, and only an executed or journey row can cover a B<n> rule",
			"cite an executed or journey row of "+VerificationFile+" for "+rule+"; a "+cited.Kind+" row may still support A<n> rows",
		)
	}
	if added {
		named := regexp.MustCompile(`(^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(name) + `($|[^A-Za-z0-9_-])`)
		for _, bullet := range done {
			if named.MatchString(bullet.text) {
				return nil
			}
		}
		return refuse(
			"slice "+slice+" declares the new catalog row "+strconv.Quote(name)+" in the plan.md Verification strategy (Covers "+row.covers+"), and no Definition of Done bullet in intent.md names it",
			"add a Definition of Done bullet to intent.md that names "+name+" and the labels it covers",
		)
	}
	if rule == "" {
		return nil
	}
	if cited == nil {
		return refuse(
			"slice "+slice+" cites the catalog row "+strconv.Quote(name)+" for "+rule+" in the plan.md Verification strategy, and "+VerificationFile+" has no row of that name; its rows are "+verification.RowNames(),
			"start the Check cell with the name of an executed or journey row of "+VerificationFile+", or declare a row this slice adds with new <name> and a Definition of Done bullet naming it",
		)
	}
	return nil
}

// firstTable returns the rows of the first Markdown table between start and
// end, split into cells, with the line number of each row.
func firstTable(lines []contractLine, start, end int) (rows [][]string, numbers []int) {
	for index := start; index < end; index++ {
		text := strings.TrimSpace(lines[index].text)
		if !strings.HasPrefix(text, "|") {
			if rows != nil {
				break
			}
			continue
		}
		rows = append(rows, tableCells(text))
		numbers = append(numbers, lines[index].number)
	}
	return rows, numbers
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

// tableHeader reports whether cells are exactly the header names, in order,
// ignoring case.
func tableHeader(cells []string, names ...string) bool {
	if len(cells) != len(names) {
		return false
	}
	for index, name := range names {
		if !strings.EqualFold(cells[index], name) {
			return false
		}
	}
	return true
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
