package skills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"
)

const StubProtocol = "skl.stub/v1"

var ownedMarker = []byte("<!-- skl-owned: " + StubProtocol + " -->")

//go:embed skills/dev/audit skills/dev/design skills/dev/domain skills/dev/explore skills/dev/implement skills/dev/propose skills/dev/tdd skills/dev/watchdog skills/misc/writing-for-agents skills/thinking/brainstorm skills/thinking/shape stubs/common.md
var embedded embed.FS

type stubData struct {
	Name        string
	Protocol    string
	Frontmatter string
}

type InstallOutcome struct {
	Changed   int
	Unchanged int
}

func Install(home string) (InstallOutcome, error) {
	tmpl, err := template.ParseFS(embedded, "stubs/common.md")
	if err != nil {
		return InstallOutcome{}, err
	}
	var outcome InstallOutcome
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills"} {
		for _, name := range SkillNames() {
			frontmatter, err := definitionFrontmatter(name)
			if err != nil {
				return outcome, err
			}
			directory := filepath.Join(home, harness, name)
			if err := os.MkdirAll(directory, 0o755); err != nil {
				return outcome, err
			}
			path := filepath.Join(directory, "SKILL.md")
			current, err := os.ReadFile(path)
			if err == nil && !bytes.Contains(current, ownedMarker) {
				outcome.Unchanged++
				continue
			}
			if err != nil && !os.IsNotExist(err) {
				return outcome, err
			}
			var contents bytes.Buffer
			err = tmpl.Execute(&contents, stubData{Name: name, Protocol: StubProtocol, Frontmatter: frontmatter})
			if err == nil && bytes.Equal(current, contents.Bytes()) {
				outcome.Unchanged++
				continue
			}
			if err == nil {
				err = os.WriteFile(path, contents.Bytes(), 0o644)
			}
			if err != nil {
				return outcome, fmt.Errorf("install %s for %s: %w", name, harness, err)
			}
			outcome.Changed++
		}
	}
	return outcome, nil
}

func definitionFrontmatter(name string) (string, error) {
	source, err := fs.ReadFile(embedded, definitionPaths[name])
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(source, []byte("---\n")) {
		return "", fmt.Errorf("skill %q has invalid frontmatter", name)
	}
	end := bytes.Index(source[4:], []byte("\n---\n"))
	if end < 0 {
		return "", fmt.Errorf("skill %q has invalid frontmatter", name)
	}
	return string(source[:end+8]), nil
}
