package skills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
)

const (
	StubProtocol    = "skl.stub/v1"
	AdapterProtocol = "skl.adapter/v1"
)

var (
	stubMarker    = ownedMarker(StubProtocol)
	adapterMarker = ownedMarker(AdapterProtocol)
	// piMarker owned the Pi runners, loop prompts and queue helper that
	// Harness Adapters replace.
	piMarker = []byte("skl-owned: skl.pi/v1")
)

func ownedMarker(protocol string) []byte {
	return []byte("<!-- skl-owned: " + protocol + " -->")
}

//go:embed prose/procedures prose/craft prose/documents prose/outcomes prose/adapters/stub.md prose/adapters/entry.md prose/adapters/stubs
var embedded embed.FS

// entryPoints are the skills a harness starts as Workflow operations rather
// than discovers as reusable knowledge.
var entryPoints = []string{"implement", "watchdog"}

// harness is where skl installs into one supported Agent Harness. A harness
// with no argument-taking entry-point mechanism leaves entryPoint empty and
// keeps the Skill Stub of each entry point.
type harness struct {
	skills     string
	entryPoint string
	entryKeys  []string
}

var harnesses = []harness{
	{".pi/agent/skills", ".pi/agent/prompts/%s.md", []string{"description"}},
	{".codex/skills", "", nil},
	{".claude/skills", ".claude/skills/%s/SKILL.md", []string{"name", "description", "disable-model-invocation"}},
	{".config/opencode/skills", ".config/opencode/commands/%s.md", []string{"description"}},
}

// retiredPiFiles are the Pi runners, loop prompts and queue helper that
// earlier installations wrote.
var retiredPiFiles = []string{"prompts/implement-loop.md", "prompts/watchdog-loop.md", "prompts/queue-next.mjs", "agents/implement-runner.md", "agents/watchdog-runner.md"}

type installData struct {
	Name        string
	Protocol    string
	Frontmatter string
}

type InstallOutcome struct {
	Changed   int
	Unchanged int
}

func Install(home string) (InstallOutcome, error) {
	stub, err := template.ParseFS(embedded, "prose/adapters/stub.md")
	if err != nil {
		return InstallOutcome{}, err
	}
	entry, err := template.ParseFS(embedded, "prose/adapters/entry.md")
	if err != nil {
		return InstallOutcome{}, err
	}
	var outcome InstallOutcome
	for _, target := range harnesses {
		if err := outcome.retire(filepath.Join(home, target.skills, "tdd", "SKILL.md"), stubMarker); err != nil {
			return outcome, err
		}
		for _, name := range SkillNames() {
			frontmatter, err := stubFrontmatter(name)
			if err != nil {
				return outcome, err
			}
			stubPath := filepath.Join(home, target.skills, name, "SKILL.md")
			if target.entryPoint == "" || !slices.Contains(entryPoints, name) {
				data := installData{Name: name, Protocol: StubProtocol, Frontmatter: frontmatter}
				if err := outcome.write(stubPath, stub, data); err != nil {
					return outcome, fmt.Errorf("install %s for %s: %w", name, target.skills, err)
				}
				continue
			}
			adapterPath := filepath.Join(home, fmt.Sprintf(target.entryPoint, name))
			data := installData{Name: name, Protocol: AdapterProtocol, Frontmatter: frontmatterKeys(frontmatter, target.entryKeys)}
			if err := outcome.write(adapterPath, entry, data); err != nil {
				return outcome, fmt.Errorf("install %s adapter for %s: %w", name, target.skills, err)
			}
			if adapterPath != stubPath {
				if err := outcome.retire(stubPath, stubMarker); err != nil {
					return outcome, err
				}
			}
		}
	}
	for _, file := range retiredPiFiles {
		if err := outcome.retire(filepath.Join(home, ".pi/agent", file), piMarker); err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}

// write renders an installed file, replacing an existing one only when skl
// owns it.
func (outcome *InstallOutcome) write(file string, tmpl *template.Template, data installData) error {
	current, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && !bytes.Contains(current, stubMarker) && !bytes.Contains(current, adapterMarker) {
		outcome.Unchanged++
		return nil
	}
	var contents bytes.Buffer
	if err := tmpl.Execute(&contents, data); err != nil {
		return err
	}
	if bytes.Equal(current, contents.Bytes()) {
		outcome.Unchanged++
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(file, contents.Bytes(), 0o644); err != nil {
		return err
	}
	outcome.Changed++
	return nil
}

// retire removes a file an earlier installation wrote, when it still carries
// that installation's marker, and then its directory if nothing else is in it.
func (outcome *InstallOutcome) retire(file string, marker []byte) error {
	current, err := os.ReadFile(file)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return fmt.Errorf("inspect retired %s: %w", file, err)
	case !bytes.Contains(current, marker):
		outcome.Unchanged++
		return nil
	}
	if err := os.Remove(file); err != nil {
		return fmt.Errorf("retire %s: %w", file, err)
	}
	outcome.Changed++
	if entries, err := os.ReadDir(filepath.Dir(file)); err == nil && len(entries) == 0 && filepath.Base(file) == "SKILL.md" {
		return os.Remove(filepath.Dir(file))
	}
	return nil
}

// stubFrontmatter is the discovery metadata a harness reads from a skill's
// installed stub.
func stubFrontmatter(name string) (string, error) {
	source, err := fs.ReadFile(embedded, path.Join(proseRoot, "adapters/stubs", name+".md"))
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(source, []byte("---\n")) || !bytes.HasSuffix(source, []byte("\n---\n")) {
		return "", fmt.Errorf("prose/adapters/stubs/%s.md is not frontmatter", name)
	}
	return strings.TrimSuffix(string(source), "\n"), nil
}

// frontmatterKeys keeps only the frontmatter lines a harness's entry point
// recognizes.
func frontmatterKeys(frontmatter string, keys []string) string {
	var kept []string
	for _, line := range strings.Split(frontmatter, "\n") {
		key, _, found := strings.Cut(line, ":")
		if line == "---" || found && slices.Contains(keys, key) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
