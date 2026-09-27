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

// adapter is one Harness Adapter: the command it runs and the flags whose
// values a user passes as its arguments, in order.
type adapter struct {
	name    string
	command string
	slots   []string
}

var adapters = []adapter{
	{"implement", "skl implement next", []string{"reviewer-model", "reviewer-thinking"}},
	{"implement-team", "skl implement next --mode team", []string{"helper-model", "helper-thinking", "reviewer-model", "reviewer-thinking"}},
	{"watchdog", "skl watchdog next", nil},
}

// piDefaults are the values the pi adapters pass for an omitted argument.
var piDefaults = map[string]string{
	"helper-model":      "openai-codex/gpt-6-luna",
	"helper-thinking":   "xhigh",
	"reviewer-model":    "openai-codex/gpt-6-sol",
	"reviewer-thinking": "xhigh",
}

// harness is where skl installs into one supported Agent Harness. A harness
// with no argument-taking entry-point mechanism leaves entryPoint empty and
// keeps a Skill Stub for each adapter, without its slots.
type harness struct {
	skills     string
	entryPoint string
	entryKeys  []string
	// argument is the harness's placeholder for an adapter's argument at a
	// 0-based position.
	argument func(position int, flag string) string
	// slotKeys are the frontmatter lines that declare an adapter's arguments.
	slotKeys func(slots []string) []string
}

var harnesses = []harness{
	{skills: ".pi/agent/skills", entryPoint: ".pi/agent/prompts/%s.md", entryKeys: []string{"description"},
		argument: func(position int, flag string) string { return fmt.Sprintf("${%d:-%s}", position+1, piDefaults[flag]) },
		slotKeys: func(slots []string) []string { return []string{argumentHint(slots)} }},
	{skills: ".codex/skills"},
	// Claude Code keeps an unfilled positional placeholder verbatim, and expands
	// an unfilled named one to nothing.
	{skills: ".claude/skills", entryPoint: ".claude/skills/%s/SKILL.md", entryKeys: []string{"name", "description", "disable-model-invocation"},
		argument: func(_ int, flag string) string { return "$" + claudeArgument(flag) },
		slotKeys: func(slots []string) []string {
			names := make([]string, len(slots))
			for i, flag := range slots {
				names[i] = claudeArgument(flag)
			}
			return []string{"arguments: [" + strings.Join(names, ", ") + "]", argumentHint(slots)}
		}},
	{skills: ".config/opencode/skills", entryPoint: ".config/opencode/commands/%s.md", entryKeys: []string{"description"},
		argument: func(position int, _ string) string { return fmt.Sprintf("$%d", position+1) }},
}

// claudeArgument is the Claude Code argument name of a flag.
func claudeArgument(flag string) string {
	return strings.ReplaceAll(flag, "-", "_")
}

func argumentHint(slots []string) string {
	return `argument-hint: "[` + strings.Join(slots, "] [") + `]"`
}

// entry renders an adapter's command and frontmatter for one harness. An
// unfilled argument passes an empty value, which skl treats as omitted.
func (target harness) entry(a adapter, frontmatter string) (command, keys string) {
	command = a.command
	for position, flag := range a.slots {
		command += " --" + flag + " '" + target.argument(position, flag) + "'"
	}
	var slotKeys []string
	if len(a.slots) > 0 && target.slotKeys != nil {
		slotKeys = target.slotKeys(a.slots)
	}
	return command, frontmatterKeys(frontmatter, target.entryKeys, slotKeys)
}

// retiredPiFiles are the Pi runners, loop prompts and queue helper that
// earlier installations wrote.
var retiredPiFiles = []string{"prompts/implement-loop.md", "prompts/watchdog-loop.md", "prompts/queue-next.mjs", "agents/implement-runner.md", "agents/watchdog-runner.md"}

type installData struct {
	Command     string
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
		if err := outcome.retireStub(filepath.Join(home, target.skills, "tdd", "SKILL.md")); err != nil {
			return outcome, err
		}
		for _, name := range SkillNames() {
			if slices.ContainsFunc(adapters, func(a adapter) bool { return a.name == name }) {
				continue
			}
			if err := outcome.writeStub(filepath.Join(home, target.skills, name, "SKILL.md"), stub, name, "skl skill "+name); err != nil {
				return outcome, fmt.Errorf("install %s for %s: %w", name, target.skills, err)
			}
		}
		for _, a := range adapters {
			stubPath := filepath.Join(home, target.skills, a.name, "SKILL.md")
			if target.entryPoint == "" {
				if err := outcome.writeStub(stubPath, stub, a.name, a.command); err != nil {
					return outcome, fmt.Errorf("install %s for %s: %w", a.name, target.skills, err)
				}
				continue
			}
			frontmatter, err := stubFrontmatter(a.name)
			if err != nil {
				return outcome, err
			}
			command, keys := target.entry(a, frontmatter)
			adapterPath := filepath.Join(home, fmt.Sprintf(target.entryPoint, a.name))
			if err := outcome.write(adapterPath, entry, installData{Command: command, Protocol: AdapterProtocol, Frontmatter: keys}); err != nil {
				return outcome, fmt.Errorf("install %s adapter for %s: %w", a.name, target.skills, err)
			}
			if adapterPath != stubPath {
				if err := outcome.retireStub(stubPath); err != nil {
					return outcome, err
				}
			}
		}
	}
	for _, file := range retiredPiFiles {
		if _, err := outcome.retire(filepath.Join(home, ".pi/agent", file), piMarker); err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}

// writeStub installs the Skill Stub of name, which runs command.
func (outcome *InstallOutcome) writeStub(file string, stub *template.Template, name, command string) error {
	frontmatter, err := stubFrontmatter(name)
	if err != nil {
		return err
	}
	return outcome.write(file, stub, installData{Command: command, Protocol: StubProtocol, Frontmatter: frontmatter})
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
// that installation's marker, and reports whether it did.
func (outcome *InstallOutcome) retire(file string, marker []byte) (bool, error) {
	current, err := os.ReadFile(file)
	switch {
	case os.IsNotExist(err):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("inspect retired %s: %w", file, err)
	case !bytes.Contains(current, marker):
		outcome.Unchanged++
		return false, nil
	}
	if err := os.Remove(file); err != nil {
		return false, fmt.Errorf("retire %s: %w", file, err)
	}
	outcome.Changed++
	return true, nil
}

// retireStub retires an owned Skill Stub and then its skill directory, unless
// something else is in it.
func (outcome *InstallOutcome) retireStub(file string) error {
	retired, err := outcome.retire(file, stubMarker)
	if err != nil || !retired {
		return err
	}
	directory := filepath.Dir(file)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("inspect retired %s: %w", directory, err)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(directory); err != nil {
		return fmt.Errorf("retire %s: %w", directory, err)
	}
	return nil
}

// stubFrontmatter is the discovery metadata a harness reads from a skill's or
// adapter's installed stub.
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
// recognizes, then adds extra lines. Every stub frontmatter key holds a
// one-line value.
func frontmatterKeys(frontmatter string, keys, extra []string) string {
	lines := strings.Split(frontmatter, "\n")
	kept := []string{lines[0]}
	for _, line := range lines[1 : len(lines)-1] {
		key, _, found := strings.Cut(line, ":")
		if found && slices.Contains(keys, key) {
			kept = append(kept, line)
		}
	}
	return strings.Join(append(append(kept, extra...), "---"), "\n")
}
