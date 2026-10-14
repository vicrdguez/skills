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
	loop    bool
}

var adapters = []adapter{
	{name: "implement", command: "skl implement next", slots: []string{"reviewer-model", "reviewer-thinking"}},
	{name: "implement-team", command: "skl implement next --mode team", slots: []string{"helper-model", "helper-thinking", "reviewer-model", "reviewer-thinking"}},
	{name: "watchdog", command: "skl watchdog next"},
}

// Loop adapters use native argument-taking entry points. Their outcomes own
// the continuation; the entry-point name selects the initial Auto Mode flag.
var loopAdapters = []adapter{
	{name: "implement-loop", command: "skl implement dispatch --wait", slots: []string{"worker-model", "worker-thinking", "reviewer-model", "reviewer-thinking"}, loop: true},
	{name: "implement-team-loop", command: "skl implement dispatch --mode team --wait", slots: []string{"worker-model", "worker-thinking", "helper-model", "helper-thinking", "reviewer-model", "reviewer-thinking"}, loop: true},
	{name: "watchdog-loop", command: "skl watchdog dispatch --wait", slots: []string{"worker-model", "worker-thinking"}, loop: true},
	{name: "implement-auto", command: "skl implement dispatch --wait --auto", slots: []string{"worker-model", "worker-thinking", "reviewer-model", "reviewer-thinking"}, loop: true},
	{name: "implement-team-auto", command: "skl implement dispatch --mode team --wait --auto", slots: []string{"worker-model", "worker-thinking", "helper-model", "helper-thinking", "reviewer-model", "reviewer-thinking"}, loop: true},
	{name: "watchdog-auto", command: "skl watchdog dispatch --wait --auto", slots: []string{"worker-model", "worker-thinking"}, loop: true},
}

// harness is where skl installs into one supported Agent Harness. A harness
// with no argument-taking entry-point mechanism leaves entryPoint empty and
// keeps a Skill Stub for each adapter, without its slots.
type harness struct {
	name       string
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
	{name: "pi", skills: ".pi/agent/skills", entryPoint: ".pi/agent/prompts/%s.md", entryKeys: []string{"description"},
		argument: func(position int, _ string) string {
			return fmt.Sprintf("${%d:-}", position+1)
		},
		slotKeys: func(slots []string) []string { return []string{argumentHint(slots)} }},
	{name: "codex", skills: ".codex/skills"},
	// Claude Code keeps an unfilled positional placeholder verbatim, and expands
	// an unfilled named one to nothing.
	{name: "claude", skills: ".claude/skills", entryPoint: ".claude/skills/%s/SKILL.md", entryKeys: []string{"name", "description", "disable-model-invocation"},
		argument: func(_ int, flag string) string { return "$" + claudeArgument(flag) },
		slotKeys: func(slots []string) []string {
			names := make([]string, len(slots))
			for i, flag := range slots {
				names[i] = claudeArgument(flag)
			}
			return []string{"arguments: [" + strings.Join(names, ", ") + "]", argumentHint(slots)}
		}},
	{name: "opencode", skills: ".config/opencode/skills", entryPoint: ".config/opencode/commands/%s.md", entryKeys: []string{"description"},
		argument: func(position int, _ string) string { return fmt.Sprintf("$%d", position+1) }},
}

// claudeArgument is the Claude Code argument name of a flag.
func claudeArgument(flag string) string {
	return strings.ReplaceAll(flag, "-", "_")
}

func argumentHint(slots []string) string {
	return `argument-hint: "[` + strings.Join(slots, "] [") + `]"`
}

// entry renders an adapter's command, argument slots and frontmatter. Loop
// values stay outside the shell command so the Supervisor can quote each
// supplied value as one opaque argument and omit blank slots.
func (target harness) entry(a adapter, frontmatter string) (command, keys, arguments string) {
	command = a.command + " --harness " + target.name
	for position, flag := range a.slots {
		value := target.argument(position, flag)
		if a.loop {
			arguments += "- --" + flag + ": " + value + "\n"
		} else {
			command += " --" + flag + " '" + value + "'"
		}
	}
	var slotKeys []string
	if len(a.slots) > 0 && target.slotKeys != nil {
		slotKeys = target.slotKeys(a.slots)
	}
	return command, frontmatterKeys(frontmatter, target.entryKeys, slotKeys), arguments
}

// retiredPiFiles are the Pi runners and queue helper earlier installations
// wrote. Old owned loop prompts are replaced in place by the new adapters.
var retiredPiFiles = []string{"prompts/queue-next.mjs", "agents/implement-runner.md", "agents/watchdog-runner.md"}

type installData struct {
	Command     string
	Protocol    string
	Frontmatter string
	Loop        bool
	Arguments   string
	LegacyOwner []byte
}

// InstallAction identifies a file change or removal of an empty legacy directory.
type InstallAction struct {
	Path   string
	Action string
}

// InstallPlan describes the current filesystem without changing it. It is a
// preview, not a reservation: Install inspects ownership again when it runs.
type InstallPlan struct {
	Actions   []InstallAction
	Unchanged int
	Preserved []string
}

type InstallOutcome struct {
	Changed   int
	Unchanged int
	Completed []InstallAction
	Preserved []string
	Failed    *InstallAction
}

// PlanInstall applies the same destination and ownership rules as Install,
// without writing or removing anything.
func PlanInstall(home string) (InstallPlan, error) {
	outcome, err := install(home, true)
	return InstallPlan{Actions: outcome.Completed, Unchanged: outcome.Unchanged, Preserved: outcome.Preserved}, err
}

func Install(home string) (InstallOutcome, error) {
	outcome, err := install(home, false)
	return outcome.InstallOutcome, err
}

type installer struct {
	InstallOutcome
	preview bool
}

func install(home string, preview bool) (installer, error) {
	stub, err := template.ParseFS(embedded, "prose/adapters/stub.md")
	if err != nil {
		return installer{}, err
	}
	entry, err := template.ParseFS(embedded, "prose/adapters/entry.md")
	if err != nil {
		return installer{}, err
	}
	outcome := installer{preview: preview}
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
		available := adapters
		if target.entryPoint != "" {
			available = append(slices.Clone(adapters), loopAdapters...)
		}
		for _, a := range available {
			stubPath := filepath.Join(home, target.skills, a.name, "SKILL.md")
			if target.entryPoint == "" {
				if err := outcome.writeStub(stubPath, stub, a.name, a.command+" --harness "+target.name); err != nil {
					return outcome, fmt.Errorf("install %s for %s: %w", a.name, target.skills, err)
				}
				continue
			}
			frontmatter, err := stubFrontmatter(a.name)
			if err != nil {
				return outcome, err
			}
			command, keys, arguments := target.entry(a, frontmatter)
			adapterPath := filepath.Join(home, fmt.Sprintf(target.entryPoint, a.name))
			data := installData{Command: command, Protocol: AdapterProtocol, Frontmatter: keys, Loop: a.loop, Arguments: arguments}
			if a.loop && target.skills == ".pi/agent/skills" {
				data.LegacyOwner = piMarker
			}
			if err := outcome.write(adapterPath, entry, data); err != nil {
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
func (outcome *installer) writeStub(file string, stub *template.Template, name, command string) error {
	frontmatter, err := stubFrontmatter(name)
	if err != nil {
		return err
	}
	return outcome.write(file, stub, installData{Command: command, Protocol: StubProtocol, Frontmatter: frontmatter})
}

// write renders an installed file, replacing an existing one only when skl
// owns it.
func (outcome *installer) write(file string, tmpl *template.Template, data installData) error {
	current, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return outcome.fail(file, "inspect", err)
	}
	action := "update"
	if os.IsNotExist(err) {
		action = "create"
	}
	if err == nil && !bytes.Contains(current, stubMarker) && !bytes.Contains(current, adapterMarker) &&
		(len(data.LegacyOwner) == 0 || !bytes.Contains(current, data.LegacyOwner)) {
		outcome.Preserved = append(outcome.Preserved, file)
		outcome.Unchanged++
		return nil
	}
	var contents bytes.Buffer
	if err := tmpl.Execute(&contents, data); err != nil {
		return outcome.fail(file, action, err)
	}
	if bytes.Equal(current, contents.Bytes()) {
		outcome.Unchanged++
		return nil
	}
	if !outcome.preview {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return outcome.fail(file, action, err)
		}
		if err := os.WriteFile(file, contents.Bytes(), 0o644); err != nil {
			return outcome.fail(file, action, err)
		}
	}
	outcome.Completed = append(outcome.Completed, InstallAction{Path: file, Action: action})
	outcome.Changed++
	return nil
}

// retire removes a file an earlier installation wrote, when it still carries
// that installation's marker, and reports whether it did.
func (outcome *installer) retire(file string, marker []byte) (bool, error) {
	current, err := os.ReadFile(file)
	switch {
	case os.IsNotExist(err):
		return false, nil
	case err != nil:
		return false, outcome.fail(file, "inspect", err)
	case !bytes.Contains(current, marker):
		outcome.Preserved = append(outcome.Preserved, file)
		outcome.Unchanged++
		return false, nil
	}
	if !outcome.preview {
		if err := os.Remove(file); err != nil {
			return false, outcome.fail(file, "remove", err)
		}
	}
	outcome.Completed = append(outcome.Completed, InstallAction{Path: file, Action: "remove"})
	outcome.Changed++
	return true, nil
}

// retireStub retires an owned Skill Stub and then its skill directory, unless
// something else is in it.
func (outcome *installer) retireStub(file string) error {
	retired, err := outcome.retire(file, stubMarker)
	if err != nil || !retired {
		return err
	}
	directory := filepath.Dir(file)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return outcome.fail(directory, "inspect", err)
	}
	// During preview the stub has not yet been removed.
	if outcome.preview {
		if len(entries) != 1 || entries[0].Name() != filepath.Base(file) {
			return nil
		}
	} else {
		if len(entries) > 0 {
			return nil
		}
		if err := os.Remove(directory); err != nil {
			return outcome.fail(directory, "remove", err)
		}
	}
	outcome.Completed = append(outcome.Completed, InstallAction{Path: directory, Action: "remove"})
	return nil
}

func (outcome *installer) fail(file, action string, err error) error {
	outcome.Failed = &InstallAction{Path: file, Action: action}
	return fmt.Errorf("%s %s: %w", action, file, err)
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
