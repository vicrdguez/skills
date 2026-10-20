package ledger

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// PrinciplesDirectory is where a repository keeps its project principles,
// one <name>.md file each.
const PrinciplesDirectory = ".skl/principles/"

// StatementLimit is the most characters a principle's statement holds.
const StatementLimit = 280

// Principle is one principle file: its name, the statement its frontmatter
// declares, the body after the frontmatter and the whole file. The engine
// checks the statement's presence and length only.
type Principle struct {
	Name      string `json:"name"`
	Statement string `json:"statement"`
	Body      string `json:"-"`
	File      string `json:"-"`
}

// ParsePrinciple reads one principle file, named file in its refusals. Its
// name is the file name without the .md extension.
func ParsePrinciple(file string, data []byte) (Principle, error) {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	frontmatter, body, err := splitFrontmatter(file, data)
	if err != nil {
		return Principle{}, err
	}
	repair := "give " + file + " a statement in its frontmatter: one line of at most " + strconv.Itoa(StatementLimit) + " characters, such as statement: <the principle>"
	var declared struct {
		Statement yaml.Node `yaml:"statement"`
	}
	if err := yaml.Unmarshal(frontmatter, &declared); err != nil {
		return Principle{}, refuse(file+" frontmatter is malformed: "+err.Error(), repair)
	}
	statement := declared.Statement
	switch {
	case statement.Kind == 0:
		return Principle{}, refuse(file+" frontmatter has no statement", repair)
	case statement.Kind != yaml.ScalarNode || statement.Tag != "!!str":
		return Principle{}, refuse(file+" statement is not a string", repair)
	case strings.TrimSpace(statement.Value) == "":
		return Principle{}, refuse(file+" statement is empty", repair)
	case utf8.RuneCountInString(statement.Value) > StatementLimit:
		return Principle{}, refuse(file+" statement is "+strconv.Itoa(utf8.RuneCountInString(statement.Value))+" characters long, over the limit of "+strconv.Itoa(StatementLimit), repair)
	}
	return Principle{Name: strings.TrimSuffix(path.Base(file), ".md"), Statement: statement.Value, Body: string(body), File: string(data)}, nil
}

// ReadPrinciples reads the project principles of the checkout at root, in
// file-name order. A checkout without the directory has none.
func ReadPrinciples(root string) ([]Principle, error) {
	directory := filepath.Join(root, filepath.FromSlash(PrinciplesDirectory))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	return parsePrinciples(names, func(name string) ([]byte, error) {
		return os.ReadFile(filepath.Join(directory, name))
	})
}

func parsePrinciples(names []string, read func(name string) ([]byte, error)) ([]Principle, error) {
	principles := make([]Principle, 0, len(names))
	for _, name := range names {
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		principle, err := ParsePrinciple(PrinciplesDirectory+name, data)
		if err != nil {
			return nil, err
		}
		principles = append(principles, principle)
	}
	return principles, nil
}
