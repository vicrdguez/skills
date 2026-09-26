package skills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

const StubProtocol = "skl.stub/v1"

var ownedMarker = []byte("<!-- skl-owned: " + StubProtocol + " -->")

//go:embed prose/procedures prose/craft prose/documents prose/adapters/stub.md prose/adapters/stubs
//go:embed prose/adapters/prompts/implement-loop.md prose/adapters/prompts/watchdog-loop.md prose/adapters/prompts/queue-next.mjs prose/adapters/agents/implement-runner.md prose/adapters/agents/watchdog-runner.md
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
	tmpl, err := template.ParseFS(embedded, "prose/adapters/stub.md")
	if err != nil {
		return InstallOutcome{}, err
	}
	var outcome InstallOutcome
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		legacyTDD := filepath.Join(home, harness, "tdd", "SKILL.md")
		legacyContents, err := os.ReadFile(legacyTDD)
		switch {
		case err == nil && bytes.Contains(legacyContents, ownedMarker):
			if err := os.Remove(legacyTDD); err != nil {
				return outcome, fmt.Errorf("retire owned tdd stub for %s: %w", harness, err)
			}
			outcome.Changed++
		case err == nil:
			outcome.Unchanged++
		case !os.IsNotExist(err):
			return outcome, fmt.Errorf("inspect legacy tdd stub for %s: %w", harness, err)
		}

		for _, name := range SkillNames() {
			frontmatter, err := stubFrontmatter(name)
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
	for _, file := range []string{"prompts/implement-loop.md", "prompts/watchdog-loop.md", "prompts/queue-next.mjs", "agents/implement-runner.md", "agents/watchdog-runner.md"} {
		contents, err := embedded.ReadFile(path.Join(proseRoot, "adapters", file))
		if err != nil {
			return outcome, err
		}
		path := filepath.Join(home, ".pi/agent", file)
		current, err := os.ReadFile(path)
		if err == nil && (bytes.Equal(current, contents) || !bytes.Contains(current, []byte("skl-owned: skl.pi/v1"))) {
			outcome.Unchanged++
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			return outcome, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return outcome, err
		}
		if err := os.WriteFile(path, contents, 0644); err != nil {
			return outcome, err
		}
		outcome.Changed++
	}
	return outcome, nil
}

// stubFrontmatter is the discovery metadata a harness reads from a skill's
// installed stub.
func stubFrontmatter(name string) (string, error) {
	source, err := fs.ReadFile(embedded, path.Join(proseRoot, "adapters/stubs", name+".md"))
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(source, []byte("---\n")) || !bytes.HasSuffix(source, []byte("\n---\n")) {
		return "", fmt.Errorf("skill %q has invalid frontmatter", name)
	}
	return strings.TrimSuffix(string(source), "\n"), nil
}
