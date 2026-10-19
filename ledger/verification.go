package ledger

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// VerificationFile is where a repository keeps its verification skill, and
// JourneyDirectory is where the journeys its catalog names live.
const (
	VerificationFile = ".skl/verify.md"
	JourneyDirectory = ".skl/journeys/"
)

// Catalog row kinds. A generated row is an index command and carries no
// rank; the other kinds name a rank of the skill's ladder.
const (
	GeneratedCheck = "generated"
	ExecutedCheck  = "executed"
	JourneyCheck   = "journey"
	HumanCheck     = "human"
)

var checkKinds = []string{GeneratedCheck, ExecutedCheck, JourneyCheck, HumanCheck}

// verificationHeadings are the sections every verification skill has.
var verificationHeadings = []string{"Launch", "Doctor", "Channels", "Catalog", "Cleanup", "Extend", "Traps"}

// CatalogRow is one check of a verification skill's catalog.
type CatalogRow struct {
	Name         string
	Channel      string
	Kind         string
	Run          string
	Expectations string
	Rank         string
}

// Verification is a repository's verification skill: the ladder its ranks
// declare, its catalog, and the prose a verifier reads. The engine interprets
// only the ladder and the catalog.
type Verification struct {
	Ladder  Ladder
	Catalog []CatalogRow
	// Body is the file after its frontmatter, and Channels the first word of
	// each list item under its Channels heading.
	Body     string
	Channels []string
}

// LadderInForce is the skill's ladder, or the built-in ladder when the
// repository has no skill.
func (v *Verification) LadderInForce() Ladder {
	if v == nil {
		return BuiltinLadder
	}
	return v.Ladder
}

// Row returns the catalog row called name, or nil.
func (v *Verification) Row(name string) *CatalogRow {
	if v == nil {
		return nil
	}
	for index := range v.Catalog {
		if v.Catalog[index].Name == name {
			return &v.Catalog[index]
		}
	}
	return nil
}

// RowNames lists the catalog's row names in catalog order.
func (v *Verification) RowNames() string {
	names := make([]string, len(v.Catalog))
	for index, row := range v.Catalog {
		names[index] = row.Name
	}
	return strings.Join(names, ", ")
}

// ReadVerification reads the verification skill of the checkout at root. It
// returns nil when the checkout has none.
func ReadVerification(root string) (*Verification, error) {
	return parseVerification(func(name string) ([]byte, error) {
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	})
}

// ReadVerificationAt reads the verification skill committed at revision of
// the repository at root. It returns nil when that revision has none.
func ReadVerificationAt(root, revision string) (*Verification, error) {
	return parseVerification(func(name string) ([]byte, error) {
		object := revision + ":" + name
		if !gitOK(root, "cat-file", "-e", object) {
			return nil, fs.ErrNotExist
		}
		contents, err := exec.Command("git", "-C", root, "cat-file", "blob", object).Output()
		if err != nil {
			return nil, gitError(root, []string{"cat-file", "blob", object}, err)
		}
		return contents, nil
	})
}

var (
	listItem      = regexp.MustCompile("^[ \t]*[-*+][ \t]+`?([A-Za-z0-9][A-Za-z0-9_-]*)")
	cellBackticks = regexp.MustCompile("^`([^`]*)`$")
)

// parseVerification reads and validates the skill through read, which
// returns an error satisfying fs.ErrNotExist for a missing file.
func parseVerification(read func(name string) ([]byte, error)) (*Verification, error) {
	data, err := read(VerificationFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	frontmatter, body, err := splitFrontmatter(VerificationFile, data)
	if err != nil {
		return nil, err
	}
	verification := &Verification{Body: string(body)}
	var declared struct {
		Ranks []string `yaml:"ranks"`
	}
	if err := yaml.Unmarshal(frontmatter, &declared); err != nil {
		return nil, refuse(VerificationFile+" frontmatter is malformed: "+err.Error(), "declare ranks in the YAML frontmatter as a list of rank names, strongest first")
	}
	if len(declared.Ranks) == 0 {
		return nil, refuse(VerificationFile+" frontmatter declares no ranks", "add ranks to the frontmatter: a list of rank names, strongest first")
	}
	for index, rank := range declared.Ranks {
		if strings.TrimSpace(rank) == "" || slices.Contains(declared.Ranks[:index], rank) {
			return nil, refuse(VerificationFile+" frontmatter declares the rank "+strconv.Quote(rank)+" empty or more than once", "give every rank of the ladder a distinct, nonempty name")
		}
	}
	verification.Ladder = Ladder(declared.Ranks)

	// Line numbers count from the top of the file, frontmatter included.
	offset := bytes.Count(data[:len(data)-len(body)], []byte("\n"))
	lines := readContractLines(body)
	for index := range lines {
		lines[index].number += offset
	}
	for _, heading := range verificationHeadings {
		if start, end := section(lines, heading); start == 0 && end == 0 {
			return nil, refuse(VerificationFile+" has no "+heading+" heading", "give "+VerificationFile+" the headings "+strings.Join(verificationHeadings, ", "))
		}
	}
	start, end := section(lines, "Channels")
	for _, line := range lines[start:end] {
		if match := listItem.FindStringSubmatch(line.text); match != nil {
			verification.Channels = append(verification.Channels, match[1])
		}
	}
	start, end = section(lines, "Catalog")
	table, numbers := firstTable(lines, start, end)
	header := []string{"Name", "Channel", "Kind", "Run", "Expectations", "Rank"}
	if len(table) < 2 || !tableHeader(table[0], header...) || !tableDelimiter(table[1]) {
		return nil, refuse(
			VerificationFile+" has no catalog table under its Catalog heading",
			"add a Markdown table under Catalog whose header is | "+strings.Join(header, " | ")+" |",
		)
	}
	for index, cells := range table[2:] {
		line := VerificationFile + " line " + strconv.Itoa(numbers[index+2])
		cells = append(cells, "", "", "", "", "", "")
		row := CatalogRow{Name: plainCell(cells[0]), Channel: plainCell(cells[1]), Kind: plainCell(cells[2]), Run: plainCell(cells[3]), Expectations: cells[4], Rank: plainCell(cells[5])}
		switch {
		case row.Name == "" || strings.ContainsAny(row.Name, " \t"):
			return nil, refuse(line+": catalog row name "+strconv.Quote(row.Name)+" is empty or holds a space", "name every catalog row with one word that a Check cell can cite")
		case verification.Row(row.Name) != nil:
			return nil, refuse(line+": catalog row "+row.Name+" is named more than once", "give every catalog row a distinct name")
		case !slices.Contains(checkKinds, row.Kind):
			return nil, refuse(line+": catalog row "+row.Name+" has the kind "+strconv.Quote(row.Kind), "set the Kind of "+row.Name+" to one of "+strings.Join(checkKinds, ", "))
		case row.Kind == GeneratedCheck && row.Rank != "":
			return nil, refuse(line+": generated catalog row "+row.Name+" names the rank "+row.Rank, "leave the Rank of a generated row empty")
		case row.Kind != GeneratedCheck && !verification.Ladder.Has(row.Rank):
			return nil, refuse(
				line+": catalog row "+row.Name+" names the rank "+strconv.Quote(row.Rank)+", and the declared ranks are "+verification.Ladder.String(),
				"name one of "+verification.Ladder.String()+" in the Rank of "+row.Name+", or declare that rank in the frontmatter",
			)
		}
		if row.Kind == JourneyCheck {
			if err := journeyExists(read, line, row); err != nil {
				return nil, err
			}
		}
		verification.Catalog = append(verification.Catalog, row)
	}
	return verification, nil
}

// journeyExists requires a journey row's Run cell to name a file under the
// journey directory that the repository holds.
func journeyExists(read func(name string) ([]byte, error), line string, row CatalogRow) error {
	missing := refuse(
		line+": journey row "+row.Name+" runs "+strconv.Quote(row.Run)+", and no such file exists under "+JourneyDirectory,
		"write the journey at "+JourneyDirectory+"<name>.md and name that path in the Run cell of "+row.Name,
	)
	if !strings.HasPrefix(row.Run, JourneyDirectory) || path.Clean(row.Run) != row.Run {
		return missing
	}
	if _, err := read(row.Run); errors.Is(err, fs.ErrNotExist) {
		return missing
	} else if err != nil {
		return err
	}
	return nil
}

// plainCell strips the code span that wraps a whole cell.
func plainCell(cell string) string {
	if match := cellBackticks.FindStringSubmatch(cell); match != nil {
		return strings.TrimSpace(match[1])
	}
	return cell
}

// CitedCheck is one Verification strategy row that cites a catalog row,
// joined with the catalog of the source in use. Added marks a row the slice
// declares new, and Missing a cited row that catalog lacks.
type CitedCheck struct {
	Covers       string `json:"covers"`
	Rule         string `json:"rule,omitempty"`
	Name         string `json:"name"`
	Case         string `json:"case,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Run          string `json:"run,omitempty"`
	Expectations string `json:"expectations,omitempty"`
	Rank         string `json:"rank,omitempty"`
	Added        bool   `json:"added,omitempty"`
	Missing      bool   `json:"missing,omitempty"`
}

// CitedChecks joins the Verification strategy of plan with the catalog of
// verification, in strategy order. A Check cell that names no catalog row and
// declares none is free text and cites nothing. It returns nil without a
// skill.
func CitedChecks(plan []byte, verification *Verification) []CitedCheck {
	if verification == nil {
		return nil
	}
	rows, _ := verificationStrategy(readContractLines(plan), nil)
	var checks []CitedCheck
	for _, row := range rows {
		name, identifier, added := catalogCitation(row.check)
		catalog := verification.Row(name)
		if catalog == nil && !added {
			continue
		}
		check := CitedCheck{Covers: row.covers, Rule: row.rule(), Name: name, Case: identifier, Added: added, Missing: catalog == nil, Rank: row.rank}
		if catalog != nil {
			check.Kind, check.Run, check.Expectations, check.Rank = catalog.Kind, catalog.Run, catalog.Expectations, catalog.Rank
		}
		checks = append(checks, check)
	}
	return checks
}

// RankBound is the weakest rank of the ladder among the cited checks that
// cover a B<n> and resolve to a catalog row, with the row that sets it; both
// are empty when no check does.
func RankBound(checks []CitedCheck, ladder Ladder) (rank, row string) {
	weakest := -1
	for _, check := range checks {
		position := slices.Index(ladder, check.Rank)
		if check.Rule != "" && !check.Missing && position > weakest {
			weakest, rank, row = position, check.Rank, check.Name
		}
	}
	return rank, row
}

// Stronger reports whether rank comes before other on the ladder.
func (l Ladder) Stronger(rank, other string) bool {
	return slices.Index(l, rank) < slices.Index(l, other)
}
