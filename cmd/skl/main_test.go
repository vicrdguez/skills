package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type memoryBackend struct {
	repository     github.RepositoryID
	prepared       bool
	items          []workflow.WorkItem
	parents        []workflow.CoordinationItem
	children       [][2]workflow.WorkItemID
	blocks         [][2]workflow.WorkItemID
	failReady      workflow.WorkItemID
	failChild      workflow.WorkItemID
	failDependency workflow.WorkItemID
}

func (b *memoryBackend) Validate(context.Context) (string, error) {
	return "trunk", nil
}

func (b *memoryBackend) Prepare(context.Context) error {
	b.prepared = true
	return nil
}

func (b *memoryBackend) FindWorkItems(_ context.Context, prepared []workflow.WorkItem, _ []workflow.Dependency) ([]workflow.WorkItem, error) {
	var found []workflow.WorkItem
	for _, item := range b.items {
		for _, wanted := range prepared {
			if wanted.Title == item.Title {
				found = append(found, item)
			}
		}
	}
	return found, nil
}

func (b *memoryBackend) ListMergedWorkItems(context.Context) ([]workflow.WorkItem, error) {
	var merged []workflow.WorkItem
	for _, item := range b.items {
		if item.Merged {
			merged = append(merged, item)
		}
	}
	return merged, nil
}

func (b *memoryBackend) CreateWorkItem(_ context.Context, item workflow.WorkItem) (workflow.WorkItem, error) {
	item.ID = workflow.WorkItemID(fmt.Sprintf("work-%d", len(b.items)+1))
	b.items = append(b.items, item)
	return item, nil
}

func (b *memoryBackend) FindCoordinationItems(_ context.Context, title string) ([]workflow.CoordinationItem, error) {
	var found []workflow.CoordinationItem
	for _, item := range b.parents {
		if item.Title == title {
			found = append(found, item)
		}
	}
	return found, nil
}

func (b *memoryBackend) CreateCoordinationItem(_ context.Context, item workflow.CoordinationItem) (workflow.CoordinationItem, error) {
	item.ID = workflow.WorkItemID(fmt.Sprintf("coordination-%d", len(b.parents)+1))
	b.parents = append(b.parents, item)
	return item, nil
}

func (b *memoryBackend) AddChild(_ context.Context, parent, child workflow.WorkItemID) error {
	if b.failChild == child {
		b.failChild = ""
		return errors.New("temporary parent relationship failure")
	}
	b.children = append(b.children, [2]workflow.WorkItemID{parent, child})
	for index := range b.items {
		if b.items[index].ID == child {
			b.items[index].Parent = parent
		}
	}
	return nil
}

func (b *memoryBackend) AddDependency(_ context.Context, dependent, blocker workflow.WorkItemID) error {
	if b.failDependency == dependent {
		b.failDependency = ""
		return errors.New("temporary dependency relationship failure")
	}
	b.blocks = append(b.blocks, [2]workflow.WorkItemID{dependent, blocker})
	for index := range b.items {
		if b.items[index].ID == dependent {
			b.items[index].Blockers = append(b.items[index].Blockers, blocker)
		}
	}
	return nil
}

func (b *memoryBackend) SetReady(_ context.Context, id workflow.WorkItemID) error {
	if b.failReady == id {
		b.failReady = ""
		return errors.New("temporary backend failure")
	}
	for index := range b.items {
		if b.items[index].ID == id {
			b.items[index].Ready = true
		}
	}
	return nil
}

func TestSetupInfersGitHubConsumerRepository(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "git@github.com:acme/widgets.git")
	nested := filepath.Join(root, "some", "nested", "directory")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	backend := &memoryBackend{}
	var stdout, stderr bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend.repository = repository
		return backend, nil
	}, bytes.NewBufferString("n\n"), &stdout, &stderr)
	if err := app.Run([]string{"skl", "setup"}); err != nil {
		t.Fatalf("setup failed: %v\nstderr: %s", err, stderr.String())
	}

	if backend.repository != (github.RepositoryID{Owner: "acme", Name: "widgets"}) {
		t.Fatalf("repository = %#v", backend.repository)
	}
	if got := stdout.String(); got != "Link CLAUDE.md to AGENTS.md? [y/N] Prepared "+root+" for GitHub workflow on trunk.\n" {
		t.Fatalf("stdout = %q", got)
	}
	if !backend.prepared {
		t.Fatal("workflow backend was not prepared")
	}
	if got := readFile(t, filepath.Join(root, ".gitignore")); got != ".worktrees/\n" {
		t.Fatalf(".gitignore = %q", got)
	}
	section := readRepositoryFile(t, "cmd/skl/testdata/simplicity.md")
	workflow := "<!-- dev-pipeline:start -->\n## Workflow\n\nUse `skl` as the Workflow entrypoint. Do not manually mutate Workflow Projections. Only a human merges.\n"
	wantBlock := workflow + "\n" + section + "<!-- dev-pipeline:end -->\n"
	agentsPath := filepath.Join(root, "AGENTS.md")
	installed := readFile(t, agentsPath)
	if installed != wantBlock || strings.Count(installed, "## Simplicity\n") != 1 {
		t.Fatalf("fresh AGENTS.md = %q, want %q", installed, wantBlock)
	}
	t.Run("refresh and repeat", func(t *testing.T) {
		before, after := "# User guidance\r\nKeep this spacing.  \n\n", "\nUser footer\twithout final newline"
		original := before + workflow + "<!-- dev-pipeline:end -->\n" + after
		if err := os.WriteFile(agentsPath, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		want := before + wantBlock + after
		for range 2 {
			stdout.Reset()
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewBufferString("n\n"), &stdout, &stderr)
			if err := app.Run([]string{"skl", "setup", "--repo", root}); err != nil {
				t.Fatal(err)
			}
			if got := stdout.String(); got != "Link CLAUDE.md to AGENTS.md? [y/N] Prepared "+root+" for GitHub workflow on trunk.\n" {
				t.Errorf("stdout = %q", got)
			}
			got := readFile(t, agentsPath)
			if got != want || strings.Count(got, "## Simplicity\n") != 1 {
				t.Fatalf("AGENTS.md = %q, want %q", got, want)
			}
			want = got
		}
	})
	if _, err := os.Stat(filepath.Join(root, ".skl.yml")); !os.IsNotExist(err) {
		t.Fatalf("workflow configuration was written: %v", err)
	}
}

func TestSetupDeclinesClaudeMigrationAtEndOfInput(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	backend := &memoryBackend{}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "setup", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md created without confirmation: %v", err)
	}
}

func TestSetupAndProposalResolveRepository(t *testing.T) {
	for _, operation := range []string{"setup", "publish"} {
		for _, layout := range []string{"origin", "sole upstream", "non-GitHub origin", "ambiguous", "explicit upstream"} {
			t.Run(operation+"/"+layout, func(t *testing.T) {
				root := proposalRepository(t)
				baseline := prepareSlice(t, root, "widget")
				if layout == "origin" {
					runGit(t, root, "remote", "add", "upstream", "https://github.com/other/widgets.git")
				} else {
					runGit(t, root, "remote", "rename", "origin", "upstream")
				}
				switch layout {
				case "non-GitHub origin":
					runGit(t, root, "remote", "add", "origin", "https://example.com/other/widgets.git")
				case "ambiguous":
					runGit(t, root, "remote", "add", "fork", "https://github.com/other/widgets.git")
				case "explicit upstream":
					runGit(t, root, "remote", "add", "origin", "https://github.com/other/widgets.git")
				}
				args := []string{"skl", "setup", "--repo", root}
				if operation == "publish" {
					args = []string{"skl", "propose", "publish", "--repo", root, "--target", "main", "--slice", proposalSliceFlag(t, "widget")}
				}
				if layout == "explicit upstream" {
					args = append(args, "--remote", "upstream")
				}
				backend := &memoryBackend{}
				var output bytes.Buffer
				app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
					backend.repository = repository
					return backend, nil
				}, bytes.NewReader(nil), &output, &output)
				err := app.Run(args)
				if layout == "ambiguous" {
					if err == nil || !strings.Contains(err.Error(), "--remote") || output.Len() != 0 {
						t.Fatalf("ambiguous selection = %v, output=%q", err, &output)
					}
					if backend.prepared || len(backend.items)+len(backend.parents)+len(backend.children)+len(backend.blocks) != 0 {
						t.Fatalf("ambiguous selection mutated backend: %#v", backend)
					}
					if got := runGitOutput(t, root, "status", "--porcelain", "--untracked-files=all"); got != "" {
						t.Fatalf("ambiguous selection changed repository files: %s", got)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if backend.repository != (github.RepositoryID{Owner: "acme", Name: "widgets"}) {
					t.Fatalf("bound repository = %#v", backend.repository)
				}
				if operation == "setup" {
					if !backend.prepared || !strings.Contains(output.String(), "for GitHub workflow on trunk.") || !strings.Contains(readFile(t, filepath.Join(root, "AGENTS.md")), "## Workflow") {
						t.Fatalf("setup = %q, backend=%#v", &output, backend)
					}
				} else if output.String() != "completed\n" || len(backend.items) != 1 || backend.items[0].ID != "work-1" || backend.items[0].ArtifactBaseline != baseline || !backend.items[0].Ready {
					t.Fatalf("publication = %q, backend=%#v", &output, backend)
				}
			})
		}
	}
}

func TestB1PublishAMarkedBaselineBeforeIssueCreation(t *testing.T) {
	for _, test := range []struct {
		name, subject, want string
		extraHead, complete bool
	}{
		{"marked baseline", "[baseline] ship-widget", "completed\n", false, true},
		{"missing marker", "Propose ship-widget", "missing [baseline] ship-widget marker", false, true},
		{"marker before head", "[baseline] ship-widget", "Artifact Baseline must be the published branch head", true, true},
		{"incomplete baseline", "[baseline] ship-widget", "ledger misses behavior.md", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "-c", "ship-widget", "main")
			writeLedger(t, root, "ship-widget", test.complete)
			runGit(t, root, "add", ".changes/ship-widget")
			runGit(t, root, "commit", "-m", test.subject)
			baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			if test.extraHead {
				runGit(t, root, "commit", "--allow-empty", "-m", "implementation")
			}
			runGit(t, root, "update-ref", "refs/remotes/origin/ship-widget", "HEAD")

			backend := &memoryBackend{}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main", "--slice", proposalSliceFlag(t, "ship-widget")})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), test.want) {
				t.Fatalf("output = %q, want %q", &output, test.want)
			}
			if test.want == "completed\n" {
				if len(backend.items) != 1 || backend.items[0].ArtifactBaseline != baseline || backend.items[0].Body != "ship-widget body\n" || !backend.items[0].Ready {
					t.Fatalf("publication = %#v", backend.items)
				}
			} else if len(backend.items) != 0 {
				t.Fatalf("refusal mutated publication: %#v", backend.items)
			}
		})
	}
}

func TestPublishExplicitRemoteChecksItsOwnGitEvidence(t *testing.T) {
	for _, evidence := range []string{"missing target", "target ancestry", "pushed head", "artifact baseline"} {
		t.Run(evidence, func(t *testing.T) {
			root := proposalRepository(t)
			baseline := prepareSlice(t, root, "widget")
			runGit(t, root, "remote", "add", "upstream", "https://github.com/other/widgets.git")
			runGit(t, root, "update-ref", "refs/remotes/upstream/main", "main")
			runGit(t, root, "update-ref", "refs/remotes/upstream/widget", baseline)
			var invariant, repair string
			switch evidence {
			case "missing target":
				runGit(t, root, "update-ref", "-d", "refs/remotes/upstream/main")
				invariant, repair = "target branch is unavailable", "fetch the target branch"
			case "target ancestry":
				tree := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD^{tree}"))
				target := strings.TrimSpace(runGitOutput(t, root, "commit-tree", tree, "-p", baseline, "-m", "target moved"))
				runGit(t, root, "update-ref", "refs/remotes/upstream/main", target)
				invariant, repair = "slice branch widget misses the observed target", "merge the target branch into the slice"
			case "pushed head":
				runGit(t, root, "update-ref", "refs/remotes/upstream/widget", "main")
				invariant, repair = "slice branch widget is not pushed at its local head", "push the slice branch"
			case "artifact baseline":
				runGit(t, root, "update-ref", "refs/heads/widget", "main")
				runGit(t, root, "update-ref", "refs/remotes/upstream/widget", "main")
				invariant, repair = "slice widget is missing [baseline] widget marker", "commit the complete ledger once at the published branch head"
			}
			backend := &memoryBackend{}
			var output bytes.Buffer
			app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
				backend.repository = repository
				return backend, nil
			}, bytes.NewReader(nil), &output, &output)
			args := []string{"skl", "propose", "publish", "--repo", root, "--remote", "upstream", "--target", "main", "--slice", proposalSliceFlag(t, "widget")}
			if err := app.Run(args); err != nil {
				t.Fatal(err)
			}
			if output.String() != "fix_required\n"+invariant+"; "+repair+"\n" || len(backend.items)+len(backend.parents)+len(backend.children)+len(backend.blocks) != 0 {
				t.Fatalf("used origin evidence instead of refusing: output=%q, backend=%#v", &output, backend)
			}
			if backend.repository != (github.RepositoryID{Owner: "other", Name: "widgets"}) {
				t.Fatalf("bound repository = %#v", backend.repository)
			}
			runGit(t, root, "update-ref", "refs/heads/widget", baseline)
			runGit(t, root, "update-ref", "refs/remotes/upstream/main", "main")
			runGit(t, root, "update-ref", "refs/remotes/upstream/widget", baseline)
			output.Reset()
			if err := app.Run(args); err != nil {
				t.Fatal(err)
			}
			if output.String() != "completed\n" || len(backend.items) != 1 || backend.items[0].ID != "work-1" || backend.items[0].ArtifactBaseline != baseline || !backend.items[0].Ready {
				t.Fatalf("corrected evidence did not publish: output=%q, backend=%#v", &output, backend)
			}
		})
	}
}

func TestDocumentedResourceCommands(t *testing.T) {
	cases := []struct {
		file  string
		skill string
		want  []string
	}{
		{file: "README.md", want: []string{"reference/submission.md", "reference/decision.md", "reference/review.md", "reference/DEEPENING.md"}},
		{file: "skills/dev/implement/SKILL.md", skill: "implement", want: []string{"reference/submission.md", "reference/decision.md"}},
		{file: "skills/dev/watchdog/SKILL.md", skill: "watchdog", want: []string{"reference/review.md"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.file, func(t *testing.T) {
			source := readRepositoryFile(t, testCase.file)
			if testCase.skill != "" {
				var rendered bytes.Buffer
				if err := newApp(nil, bytes.NewReader(nil), &rendered, &rendered).Run([]string{"skl", "skill", testCase.skill}); err != nil {
					t.Fatal(err)
				}
				source = rendered.String()
			}
			seen := map[string]bool{}
			for _, chunk := range strings.Split(source, "`") {
				for _, line := range strings.Split(chunk, "\n") {
					command := strings.TrimSpace(line)
					if !strings.HasPrefix(command, "skl skill ") || !strings.Contains(command, "--resource") || !strings.Contains(command, "reference/") {
						continue
					}
					args := shellArgs(t, command)
					resource := ""
					for _, arg := range args {
						if strings.HasPrefix(arg, "reference/") {
							resource = arg
						}
					}
					if resource == "" {
						continue
					}
					seen[resource] = true
					var output bytes.Buffer
					app := newApp(func(github.RepositoryID) (setup.Backend, error) {
						t.Fatalf("%s reached the Workflow Backend", command)
						return nil, nil
					}, bytes.NewReader(nil), &output, &output)
					if err := app.Run(args); err != nil {
						t.Errorf("%s: %v", command, err)
					}
				}
			}
			for _, want := range testCase.want {
				if !seen[want] {
					t.Errorf("missing documented command for %s: %v", want, seen)
				}
			}
		})
	}
}

// shellArgs resolves a documented command the way a shell would, so quoted
// values survive as the single arguments they name.
func shellArgs(t *testing.T, command string) []string {
	t.Helper()
	output, err := exec.Command("sh", "-c", "printf '%s\\000' "+command).Output()
	if err != nil {
		t.Fatalf("cannot resolve %q: %v", command, err)
	}
	return strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
}

func TestInstallSupportedSkillStubs(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)

	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}

	wantSkills := map[string]string{
		"audit":              "skills/dev/audit/SKILL.md",
		"brainstorm":         "skills/thinking/brainstorm/SKILL.md",
		"design":             "skills/dev/design/SKILL.md",
		"domain":             "skills/dev/domain/SKILL.md",
		"explore":            "skills/dev/explore/SKILL.md",
		"implement":          "skills/dev/implement/SKILL.md",
		"propose":            "skills/dev/propose/SKILL.md",
		"shape":              "skills/thinking/shape/SKILL.md",
		"tdd":                "skills/dev/tdd/SKILL.md",
		"watchdog":           "skills/dev/watchdog/SKILL.md",
		"writing-for-agents": "skills/misc/writing-for-agents/SKILL.md",
	}
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		for name, source := range wantSkills {
			stub := readFile(t, filepath.Join(root, harness, name, "SKILL.md"))
			if !strings.HasPrefix(stub, "---\n") {
				t.Fatalf("%s %s stub has no leading YAML frontmatter:\n%s", harness, name, stub)
			}
			if want := skillFrontmatter(t, readRepositoryFile(t, source)); !strings.HasPrefix(stub, want+"\n") {
				t.Fatalf("%s %s stub changed source frontmatter:\n%s", harness, name, stub)
			}
			command := "skl skill " + name
			if name == "implement" {
				command = "skl implement next"
			}
			if !strings.Contains(stub, command) || !strings.Contains(stub, "skl.stub/v1") {
				t.Fatalf("%s %s stub does not delegate to skl:\n%s", harness, name, stub)
			}
			if harness == ".config/opencode/skills" {
				path := filepath.Join(root, harness, name, "SKILL.md")
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("OpenCode stub is not a regular file: %s: %v", path, err)
				}
				for _, other := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills"} {
					otherPath := filepath.Join(root, other, name, "SKILL.md")
					otherInfo, err := os.Stat(otherPath)
					if err != nil || os.SameFile(info, otherInfo) || stub != readFile(t, otherPath) {
						t.Fatalf("OpenCode stub is not an independent common stub: %s: %v", path, err)
					}
				}
				for _, reference := range []string{".pi/", ".codex/", ".claude/", "skills/dev/", "skills/misc/", "skills/thinking/"} {
					if strings.Contains(stub, reference) {
						t.Fatalf("OpenCode stub references %s: %s", reference, path)
					}
				}
				entries, err := os.ReadDir(filepath.Dir(path))
				if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" {
					t.Fatalf("unexpected OpenCode skill assets: %v: %v", entries, err)
				}
			}
		}
		if _, err := os.Stat(filepath.Join(root, harness, "dev-setup", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("%s contains retired dev-setup stub: %v", harness, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, ".config/opencode"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "skills" || !entries[0].IsDir() {
		t.Fatalf("unexpected OpenCode configuration or adapters: %v: %v", entries, err)
	}
	entries, err = os.ReadDir(filepath.Join(root, ".config/opencode/skills"))
	if err != nil || len(entries) != len(wantSkills) {
		t.Fatalf("unexpected OpenCode catalog: %v: %v", entries, err)
	}
	for _, manifest := range []string{".claude-plugin/plugin.json", ".codex-plugin/plugin.json", "package.json"} {
		if _, err := os.Stat(filepath.Join(root, manifest)); !os.IsNotExist(err) {
			t.Fatalf("plugin manifest written at %s: %v", manifest, err)
		}
	}
}

func TestEmbeddedResourceDelegationSmoke(t *testing.T) {
	home := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	run := func(t *testing.T, command string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) {
			t.Fatal("read-only retrieval and installation must not open a backend")
			return nil, nil
		}, bytes.NewReader(nil), &stdout, &stderr, home)
		if err := app.Run(strings.Fields(command)); err != nil {
			t.Fatalf("%s: %v\n%s", command, err, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%s stderr: %s", command, stderr.String())
		}
		return stdout.String()
	}
	run(t, "skl install")
	for _, command := range []string{
		"skl skill --format json domain",
		"skl skill --format json explore",
		"skl skill --format json implement",
		"skl skill --format json propose",
		"skl skill --resource reference/tasks.md propose",
	} {
		got := run(t, command)
		for _, retired := range []string{"docs/capabilities", "CAPABILITIES-FORMAT.md", "document a capability", "ADRs, capabilities", "capability changes", "capability-doc"} {
			if strings.Contains(got, retired) {
				t.Errorf("%s retains %q", command, retired)
			}
		}
	}
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills"} {
		for _, skill := range []string{"audit", "brainstorm", "design", "domain", "explore", "implement", "propose", "shape", "tdd", "watchdog", "writing-for-agents"} {
			directory := filepath.Join(home, harness, skill)
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" {
				t.Fatalf("%s must contain only a stub: %v (%v)", directory, entries, err)
			}
			stub := readFile(t, filepath.Join(directory, "SKILL.md"))
			_, body, found := strings.Cut(strings.TrimPrefix(stub, "---\n"), "\n---\n")
			command := "skl skill " + skill
			if skill == "implement" {
				command = "skl implement next"
			}
			want := "\n<!-- skl-owned: skl.stub/v1 -->\n\nRun `" + command + "`. Skip activation for every skill named in `included_skills`; its definition is already in the packet.\n"
			if !found || body != want {
				t.Fatalf("%s is not a thin CLI-delegating stub: %s", directory, stub)
			}
		}
	}
	for _, tc := range []struct {
		skill, resource, pointer, command, content string
	}{
		{"domain", "", "Use the format from `skl skill --resource reference/CONTEXT-FORMAT.md domain`.", "skl skill --resource reference/CONTEXT-FORMAT.md domain", "# CONTEXT.md Format"},
		{"domain", "", "If any of the three is missing, skip the ADR. Use the format from `skl skill --resource reference/ADR-FORMAT.md domain`.", "skl skill --resource reference/ADR-FORMAT.md domain", "# ADR Format"},
		{"tdd", "", "See `skl skill --resource reference/tests.md tdd` for examples", "skl skill --resource reference/tests.md tdd", "# Good and Bad Tests"},
		{"tdd", "", "`skl skill --resource reference/mocking.md tdd` for mocking guidelines.", "skl skill --resource reference/mocking.md tdd", "Mock at **system boundaries** only:"},
		{"audit", "", "the Standards axis always carries the **smell baseline** from `skl skill --resource reference/smells.md audit`", "skl skill --resource reference/smells.md audit", "# Smell Baseline"},
		{"audit", "", "The list of standards-source files you found in step 3, plus `skl skill --resource reference/smells.md audit`. Instruct the sub-agent to read those files and the command's output.", "skl skill --resource reference/smells.md audit", "**The repo overrides.** A documented repo standard always wins"},
		{"design", "", "see `skl skill --resource reference/DEEPENING.md design`: dependency categories", "skl skill --resource reference/DEEPENING.md design", "### 2. Local-substitutable"},
		{"design", "", "see `skl skill --resource reference/DESIGN-IT-TWICE.md design`: spin up parallel sub-agents", "skl skill --resource reference/DESIGN-IT-TWICE.md design", "Each must produce a **radically different** interface"},
		{"design", "reference/DEEPENING.md", "the Design definition (retrieve with `skl skill design` if not already supplied)", "skl skill design", "# Codebase Design"},
		{"design", "reference/DESIGN-IT-TWICE.md", "Uses the vocabulary in the Design definition (retrieve with `skl skill design` if not already supplied)", "skl skill design", "**The interface is the test surface.**"},
		{"design", "reference/DESIGN-IT-TWICE.md", "which category they fall into (see `skl skill --resource reference/DEEPENING.md design`)", "skl skill --resource reference/DEEPENING.md design", "## Dependency categories"},
		{"design", "reference/DESIGN-IT-TWICE.md", "dependency category from `skl skill --resource reference/DEEPENING.md design`, what sits behind the seam", "skl skill --resource reference/DEEPENING.md design", "### 4. True external (Mock)"},
		{"design", "reference/DESIGN-IT-TWICE.md", "Include both the Design definition's vocabulary (retrieve with `skl skill design` if not already supplied) and `CONTEXT.md` vocabulary in the brief", "skl skill design", "**Locality**"},
		{"design", "reference/DESIGN-IT-TWICE.md", "Dependency strategy and adapters (see `skl skill --resource reference/DEEPENING.md design`)", "skl skill --resource reference/DEEPENING.md design", "## Testing strategy: replace, don't layer"},
		{"propose", "", "Follow the template from `skl skill --resource reference/intent.md propose`", "skl skill --resource reference/intent.md propose", "## Definition of Done"},
		{"propose", "", "Follow the template from `skl skill --resource reference/behavior.md propose`", "skl skill --resource reference/behavior.md propose", "## Feature: Order cancellation"},
		{"propose", "", "Follow the template from `skl skill --resource reference/plan.md propose`", "skl skill --resource reference/plan.md propose", "### Module shapes & seams"},
		{"propose", "", "`tasks.md`: Follow the template from `skl skill --resource reference/tasks.md propose`", "skl skill --resource reference/tasks.md propose", "Write it when there's more than a couple of scenarios or any non-behavioral chores."},
		{"writing-for-agents", "", "When the document you're writing is a skill, read `skl skill --resource SKILL-MECHANICS.md writing-for-agents` for frontmatter, invocation choice, and router skills.", "skl skill --resource SKILL-MECHANICS.md writing-for-agents", "## Invocation"},
		{"writing-for-agents", "", "**By invocation**, skill-specific: see `skl skill --resource SKILL-MECHANICS.md writing-for-agents`.", "skl skill --resource SKILL-MECHANICS.md writing-for-agents", "## Router skills"},
		{"writing-for-agents", "SKILL-MECHANICS.md", "the Writing for Agents definition (retrieve with `skl skill writing-for-agents` if not already supplied)", "skl skill writing-for-agents", "## Context pointers"},
		{"writing-for-agents", "SKILL-MECHANICS.md", "the pointer-writing rules in the Writing for Agents definition apply in full", "skl skill writing-for-agents", "**One trigger per branch.**"},
		{"writing-for-agents", "SKILL-MECHANICS.md", "the sequence cut lives in the Writing for Agents definition", "skl skill writing-for-agents", "**By sequence**: split a run of steps"},
	} {
		t.Run(tc.skill+"/"+tc.pointer, func(t *testing.T) {
			if tc.resource != "" {
				if got := run(t, "skl skill --resource "+tc.resource+" "+tc.skill); !strings.Contains(got, tc.pointer) {
					t.Errorf("resource lacks %q", tc.pointer)
				}
			} else {
				skills := []string{tc.skill}
				if slices.Contains([]string{"tdd", "audit", "design", "domain"}, tc.skill) {
					skills = append(skills, "implement")
				}
				for _, skill := range skills {
					for _, format := range []string{"markdown", "json"} {
						got := run(t, "skl skill --format "+format+" "+skill)
						if format == "json" {
							var packet skilldist.Packet
							if err := json.Unmarshal([]byte(got), &packet); err != nil {
								t.Fatal(err)
							}
							if skill == "implement" && !slices.Equal(packet.IncludedSkills, []string{"tdd", "audit", "design", "domain"}) {
								t.Fatalf("included skills = %v", packet.IncludedSkills)
							}
							got = packet.Instructions
						}
						if skill == "implement" {
							_, got, _ = strings.Cut(got, "\n\n## Included Skill: "+tc.skill+"\n\n")
						}
						got, _, _ = strings.Cut(got, "\n\n## Included Skill:")
						if !strings.Contains(got, tc.pointer) {
							t.Errorf("%s %s lacks %q", skill, format, tc.pointer)
						}
					}
				}
			}
			if got := run(t, tc.command); !strings.Contains(got, tc.content) {
				t.Errorf("%s lacks meaningful content %q", tc.command, tc.content)
			}
		})
	}
}

func TestInstallFailsWithoutUserHome(t *testing.T) {
	t.Setenv("HOME", "")
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "install"}); err == nil {
		t.Fatal("install succeeded without a user home")
	}
}

func TestInstallRefreshesOnlyOwnedStubs(t *testing.T) {
	for _, harness := range []string{".codex/skills", ".config/opencode/skills"} {
		t.Run(harness, func(t *testing.T) {
			root := t.TempDir()
			var output bytes.Buffer
			app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}

			tdd := filepath.Join(root, harness, "tdd/SKILL.md")
			wantTDD := readFile(t, tdd)
			if err := os.WriteFile(tdd, []byte("---\nname: tdd\n---\n\n<!-- skl-owned: skl.stub/v1 -->\nstale"), 0o644); err != nil {
				t.Fatal(err)
			}
			audit := filepath.Join(root, harness, "audit/SKILL.md")
			if err := os.WriteFile(audit, []byte("---\nname: audit\n---\n\n<!-- skl-owned: skl.stub/v2 -->\nmy unrelated skill"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}

			if got := readFile(t, tdd); got != wantTDD {
				t.Fatalf("owned stub was not refreshed:\n%s", got)
			}
			if got := readFile(t, audit); got != "---\nname: audit\n---\n\n<!-- skl-owned: skl.stub/v2 -->\nmy unrelated skill" {
				t.Fatalf("unrelated file changed: %q", got)
			}
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, tdd); got != wantTDD {
				t.Fatalf("repeat installation changed current stub:\n%s", got)
			}
			if harness == ".config/opencode/skills" {
				entries, err := os.ReadDir(filepath.Join(root, ".config/opencode"))
				if err != nil || len(entries) != 1 || entries[0].Name() != "skills" {
					t.Fatalf("reinstallation wrote OpenCode configuration: %v: %v", entries, err)
				}
			}
		})
	}
}

func TestInstallPreservesOpenCodeSkillsAndConfiguration(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"skills/audit/SKILL.md":    "---\nname: audit\n---\nMy own audit skill\n",
		"skills/personal/SKILL.md": "My unrelated skill\n",
		"opencode.json":            `{"skills":{"paths":["~/.pi/agent/skills","/my/other/skills"]},"theme":"system"}`,
	}
	for file, contents := range files {
		path := filepath.Join(root, ".config/opencode", file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
	for range 2 {
		if err := app.Run([]string{"skl", "install"}); err != nil {
			t.Fatal(err)
		}
		for file, want := range files {
			if got := readFile(t, filepath.Join(root, ".config/opencode", file)); got != want {
				t.Fatalf("user file %s changed: %q", file, got)
			}
		}
		for _, name := range skilldist.SkillNames() {
			if name == "audit" {
				continue
			}
			got := readFile(t, filepath.Join(root, ".config/opencode/skills", name, "SKILL.md"))
			if want := readFile(t, filepath.Join(root, ".codex/skills", name, "SKILL.md")); got != want {
				t.Fatalf("missing nonconflicting common stub for %s: %q", name, got)
			}
		}
	}
}

func TestRetrieveRenderedSkillInstructions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "tdd"}); err != nil {
		t.Fatal(err)
	}

	want := "Protocol: skl.instructions/v1\nSkill: tdd\nIncluded skills: none\nFacts: {}\nResources: reference/mocking.md, reference/tests.md\n\n" + readRepositoryFile(t, "skills/dev/tdd/SKILL.md")
	if got := stdout.String(); got != want {
		t.Fatalf("rendered packet differs from canonical definition:\n%s", got)
	}
}

func TestRetrieveConcreteProposeInstructions(t *testing.T) {
	var output bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "propose"}); err != nil {
		t.Fatal(err)
	}

	got, _, _ := strings.Cut(output.String(), "\n\n## Included Skill:")
	for section, requirements := range map[string][]string{
		"slice judgment":      {"COMPLETE path through every layer", "demoable and verifiable on its own", "single fresh context window", "Iterate until the user approves the breakdown"},
		"seam judgment":       {"Use the `design` skill", "Always prefer existing seams", "Use the highest seam possible", "Check with the user if the seams match their expectations"},
		"artifact authorship": {"## Writing the change artifacts", "`intent.md`: Why / What / Scope / Out of scope / Definition of Done", "*Gherkin notation*", "module shapes and seams chosen for implementation", "Discoveries belong in PR findings or in a new proposal", "skl skill --resource reference/tasks.md propose"},
		"publication":         {"skl propose publish", "skl propose cleanup"},
		"independent delivery": {
			"Prefer separate Work Items for behaviors that deliver safe, useful results independently",
			"after declared Dependencies are Merged, without requiring later Work Items",
			"Combine independently useful behaviors only for a concrete reduction in overall implementation or review burden",
			"Shared files or a shared Workflow stage alone are insufficient",
		},
		"review burden": {
			"Assess review burden from the behavior and materially different correctness, failure, and recovery concerns a reviewer must understand together",
			"using agreed requirements and focused repository inspection rather than line counts or exhaustive implementation planning",
			"Different error cases alone do not require separate Work Items",
			"*Review burden*: Briefly explain those concerns and any concrete reason for combining independently useful behaviors",
			"*Title*", "*Blocked by*", "*What it delivers*",
			"Does the granularity feel right?", "Are the blocking edges correct", "Should any tickets be merged or split further?",
		},
		"bounded reconsideration": {
			"If artifact elaboration materially changes proposed boundaries or Dependencies, return to this approval loop before publication",
			"explain the discovery and propose the revised breakdown for approval before freezing the artifacts",
			"Ordinary elaboration within an unchanged coherent delivery needs no renewed approval",
			"Publishing them freezes them", "There is no later addition and no exception",
		},
	} {
		t.Run(section, func(t *testing.T) {
			for _, want := range requirements {
				if !strings.Contains(got, want) {
					t.Errorf("Propose instructions lack %q", want)
				}
			}
		})
	}
	if strings.Contains(got, "docs/github.md") {
		t.Fatalf("Propose instructions retain copied board protocol:\n%s", got)
	}
}

func TestRetrieveRetiredLedgerInstructions(t *testing.T) {
	for skill, required := range map[string][]string{
		"watchdog": {
			"## Pass -> Ready for Merge",
			"git show <artifact-baseline>:.changes/<slug>/intent.md",
			"Copy its `Manual verification` section into the PR body verbatim, with every checkbox unchecked",
			"Keep the retired Implementation Ledger absent; do not restore or archive it.",
			"The change now awaits the **human's merge**. The watchdog does not merge.",
		},
		"implement": {
			"remove the entire `.changes/<slug>/` ledger in a separate subsequent commit before review",
			"Never bless the changes — that is the watchdog's job.",
			"rework must not recreate or revise it",
		},
	} {
		t.Run(skill, func(t *testing.T) {
			var output bytes.Buffer
			app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, t.TempDir())
			if err := app.Run([]string{"skl", "skill", skill}); err != nil {
				t.Fatal(err)
			}
			got, _, _ := strings.Cut(output.String(), "\n\n## Included Skill:")
			for _, want := range required {
				if !strings.Contains(got, want) {
					t.Errorf("%s instructions lack %q", skill, want)
				}
			}
			for _, forbidden := range []string{
				"archiving the change **inside the branch**",
				"`.changes/<slug>/` → `.changes/archive/<YYYY-MM-DD>-<slug>/`",
				"Push the archive commit to the PR branch",
				"Never archive or bless the changes — that is the watchdog's job",
			} {
				if strings.Contains(got, forbidden) {
					t.Errorf("%s instructions retain legacy archive action %q", skill, forbidden)
				}
			}
		})
	}
}

func TestProposePacketPublishesDurableThinPointer(t *testing.T) {
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "skill", "propose"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"thin-pointer template", "git rev-parse HEAD", "full commit SHA", "Replace every placeholder"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("packet lacks %q", want)
		}
	}
	_, template, found := strings.Cut(output.String(), "```markdown\n")
	if !found {
		t.Fatal("packet lacks thin-pointer Markdown template")
	}
	template, _, _ = strings.Cut(template, "\n```")
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "ship-widget")
	authored := strings.NewReplacer("<summary>", "Ship widgets [opaque prose", "<slug>", "ship-widget", "<baseline-sha>", baseline).Replace(template) + "\n"
	for _, want := range []string{"Branch: `ship-widget`", "Artifact Baseline: `" + baseline + "`", "`.changes/ship-widget/`"} {
		if !strings.Contains(authored, want) {
			t.Fatalf("thin pointer lacks %q: %s", want, authored)
		}
	}
	bodyPath := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(bodyPath, []byte(authored), 0o644); err != nil {
		t.Fatal(err)
	}
	var durableBody string
	ready := false
	client := &http.Client{Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{}`
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/widgets/issues":
			body = `[]`
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/widgets/issues":
			var payload map[string]string
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			durableBody = payload["body"]
			body = `{"id":501,"number":1}`
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/widgets/issues/1/labels":
			if durableBody != authored {
				t.Fatalf("Ready before opaque thin pointer was durable: %q", durableBody)
			}
			ready = true
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	output.Reset()
	app = newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend("https://api.github.test", "secret", client)
		if repository != (github.RepositoryID{}) {
			backend.BindRepository(repository)
		}
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main", "--slice", "ship-widget=" + bodyPath}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "completed\n" || !ready || durableBody != authored {
		t.Fatalf("publication = %q ready=%v body=%q", output.String(), ready, durableBody)
	}
}

func TestPublishOnePreparedSlice(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "ship-widget")
	body := filepath.Join(t.TempDir(), "issue.md")
	if err := os.WriteFile(body, []byte("agent-authored body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &memoryBackend{}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main", "--slice", "ship-widget=" + body})
	if err != nil {
		t.Fatal(err)
	}

	if got := output.String(); got != "completed\n" {
		t.Fatalf("output = %q", got)
	}
	if len(backend.items) != 1 {
		t.Fatalf("items = %#v", backend.items)
	}
	item := backend.items[0]
	if item.ID != "work-1" || item.Title != "ship-widget" || item.Body != "agent-authored body\n" || item.Branch != "ship-widget" || item.ArtifactBaseline != baseline || !item.Ready {
		t.Fatalf("item = %#v", item)
	}
	if len(backend.parents) != 0 {
		t.Fatalf("parents = %#v", backend.parents)
	}
}

func TestPublishDependencyOrderedMultiSliceProposal(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "foundation")
	prepareSlice(t, root, "feature")
	directory := t.TempDir()
	for name, contents := range map[string]string{"parent.md": "parent prose\n", "foundation.md": "foundation prose\n", "feature.md": "feature prose\n"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backend := &memoryBackend{}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main",
		"--slice", "feature=" + filepath.Join(directory, "feature.md"),
		"--slice", "foundation=" + filepath.Join(directory, "foundation.md"),
		"--depends", "feature:foundation", "--parent-title", "widgets", "--parent-body", filepath.Join(directory, "parent.md")})
	if err != nil {
		t.Fatal(err)
	}

	if len(backend.parents) != 1 || backend.parents[0].ID != "coordination-1" || backend.parents[0].Title != "widgets" || backend.parents[0].Body != "parent prose\n" {
		t.Fatalf("parents = %#v", backend.parents)
	}
	if got := []string{backend.items[0].Title, backend.items[1].Title}; !slices.Equal(got, []string{"foundation", "feature"}) {
		t.Fatalf("publication order = %v", got)
	}
	if !slices.Equal(backend.children, [][2]workflow.WorkItemID{{"coordination-1", "work-1"}, {"coordination-1", "work-2"}}) {
		t.Fatalf("children = %v", backend.children)
	}
	if !slices.Equal(backend.blocks, [][2]workflow.WorkItemID{{"work-2", "work-1"}}) {
		t.Fatalf("dependencies = %v", backend.blocks)
	}
	if !backend.items[0].Ready || !backend.items[1].Ready {
		t.Fatalf("items are not Ready: %#v", backend.items)
	}
}

func TestPublicationRelationshipFailureLeavesChildNotReady(t *testing.T) {
	for _, relationship := range []string{"parent", "dependency"} {
		t.Run(relationship, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "base")
			prepareSlice(t, root, "dependent")
			directory := t.TempDir()
			for _, name := range []string{"parent", "base", "dependent"} {
				if err := os.WriteFile(filepath.Join(directory, name+".md"), []byte(name+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			backend := &memoryBackend{}
			if relationship == "parent" {
				backend.failChild = "work-2"
			} else {
				backend.failDependency = "work-2"
			}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			arguments := []string{"skl", "propose", "publish", "--repo", root, "--target", "main",
				"--slice", "dependent=" + filepath.Join(directory, "dependent.md"), "--slice", "base=" + filepath.Join(directory, "base.md"),
				"--depends", "dependent:base", "--parent-title", "parent", "--parent-body", filepath.Join(directory, "parent.md")}
			if err := app.Run(arguments); err == nil || !strings.Contains(err.Error(), relationship+" relationship failure") {
				t.Fatalf("publication error = %v", err)
			}
			if len(backend.items) != 2 || backend.items[0].Title != "base" || !backend.items[0].Ready || backend.items[1].Title != "dependent" || backend.items[1].Ready {
				t.Fatalf("failed relationship must retain completed child Ready and affected child non-Ready: %#v", backend.items)
			}
			if err := app.Run(arguments); err != nil {
				t.Fatal(err)
			}
			if output.String() != "completed\n" || len(backend.items) != 2 || !backend.items[0].Ready || !backend.items[1].Ready || len(backend.parents) != 1 || !slices.Equal(backend.children, [][2]workflow.WorkItemID{{"coordination-1", "work-1"}, {"coordination-1", "work-2"}}) || !slices.Equal(backend.blocks, [][2]workflow.WorkItemID{{"work-2", "work-1"}}) {
				t.Fatalf("forward retry = %q backend=%#v", output.String(), backend)
			}
		})
	}
}

func TestRefuseInvalidProposalPreflight(t *testing.T) {
	diagnostics := map[string][2]string{
		"dirty durable document":                  {"durable documents have uncommitted changes", "commit or restore the reported durable-document paths"},
		"dirty durable document in main worktree": {"durable documents have uncommitted changes", "commit or restore the reported durable-document paths"},
		"slice misses target":                     {"slice branch divergent misses the observed target", "merge the target branch into the slice"},
		"incomplete baseline":                     {"ledger misses behavior.md", "commit the complete ledger once at the published branch head"},
		"cyclic dependencies":                     {"Dependency graph contains a cycle", "remove the cyclic --depends edge"},
		"missing target":                          {"target branch is unavailable", "fetch the target branch"},
		"missing slice branch":                    {"slice branch missing is unavailable", "create the local slice branch"},
		"unpushed slice":                          {"slice branch unpushed is not pushed at its local head", "push the slice branch"},
		"missing ledger":                          {"missing [baseline] missing marker", "commit the complete ledger once at the published branch head"},
		"baseline not at head":                    {"Artifact Baseline must be the published branch head", "commit the complete ledger once at the published branch head"},
		"unknown dependency":                      {"Dependency graph contains an unknown or self-referencing edge", "correct the --depends values"},
		"self dependency":                         {"Dependency graph contains an unknown or self-referencing edge", "correct the --depends values"},
	}
	tests := map[string]func(*testing.T) (string, []string){
		"missing target": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			runGit(t, root, "update-ref", "-d", "refs/remotes/origin/main")
			return root, []string{"--slice", proposalSliceFlag(t, "missing")}
		},
		"missing slice branch": func(t *testing.T) (string, []string) {
			return proposalRepository(t), []string{"--slice", proposalSliceFlag(t, "missing")}
		},
		"unpushed slice": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			prepareSlice(t, root, "unpushed")
			runGit(t, root, "update-ref", "-d", "refs/remotes/origin/unpushed")
			return root, []string{"--slice", proposalSliceFlag(t, "unpushed")}
		},
		"missing ledger": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			runGit(t, root, "branch", "missing", "main")
			runGit(t, root, "update-ref", "refs/remotes/origin/missing", "main")
			return root, []string{"--slice", proposalSliceFlag(t, "missing")}
		},
		"baseline not at head": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			prepareSlice(t, root, "later")
			if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("later\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "commit", "-am", "later change")
			runGit(t, root, "update-ref", "refs/remotes/origin/later", "HEAD")
			return root, []string{"--slice", proposalSliceFlag(t, "later")}
		},
		"unknown dependency": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			prepareSlice(t, root, "one")
			return root, []string{"--slice", proposalSliceFlag(t, "one"), "--depends", "one:unknown"}
		},
		"self dependency": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			prepareSlice(t, root, "one")
			return root, []string{"--slice", proposalSliceFlag(t, "one"), "--depends", "one:one"}
		},
		"dirty durable document": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			prepareSlice(t, root, "dirty-docs")
			if err := os.WriteFile(filepath.Join(root, "CONTEXT.md"), []byte("uncommitted\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return root, []string{"--slice", proposalSliceFlag(t, "dirty-docs")}
		},
		"dirty durable document in main worktree": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			worktree := filepath.Join(root, ".worktrees", "linked-slice")
			runGit(t, root, "worktree", "add", worktree, "-b", "linked-slice", "main")
			writeLedger(t, worktree, "linked-slice", true)
			runGit(t, worktree, "add", ".changes/linked-slice")
			runGit(t, worktree, "commit", "-m", "linked slice")
			runGit(t, root, "update-ref", "refs/remotes/origin/linked-slice", "refs/heads/linked-slice")
			if err := os.WriteFile(filepath.Join(root, "CONTEXT.md"), []byte("uncommitted\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return worktree, []string{"--slice", proposalSliceFlag(t, "linked-slice")}
		},
		"slice misses target": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "--orphan", "divergent")
			if err := os.Remove(filepath.Join(root, "README.md")); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			writeLedger(t, root, "divergent", true)
			runGit(t, root, "add", "-A")
			runGit(t, root, "commit", "-m", "divergent")
			runGit(t, root, "update-ref", "refs/remotes/origin/divergent", "HEAD")
			return root, []string{"--slice", proposalSliceFlag(t, "divergent")}
		},
		"incomplete baseline": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			runGit(t, root, "switch", "-c", "incomplete", "main")
			writeLedger(t, root, "incomplete", false)
			runGit(t, root, "add", ".changes/incomplete")
			runGit(t, root, "commit", "-m", "[baseline] incomplete")
			runGit(t, root, "update-ref", "refs/remotes/origin/incomplete", "HEAD")
			return root, []string{"--slice", proposalSliceFlag(t, "incomplete")}
		},
		"cyclic dependencies": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			prepareSlice(t, root, "one")
			prepareSlice(t, root, "two")
			return root, []string{"--slice", proposalSliceFlag(t, "one"), "--slice", proposalSliceFlag(t, "two"), "--depends", "one:two", "--depends", "two:one"}
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			root, flags := arrange(t)
			backend := &memoryBackend{}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			arguments := append([]string{"skl", "propose", "publish", "--repo", root, "--target", "main"}, flags...)

			if err := app.Run(arguments); err != nil {
				t.Fatal(err)
			}

			if !strings.HasPrefix(output.String(), "fix_required\n") {
				t.Fatalf("output = %q", output.String())
			}
			want, ok := diagnostics[name]
			if !ok {
				t.Fatal("preflight case lacks invariant and repair expectations")
			}
			for _, message := range want {
				if message == "" || !strings.Contains(output.String(), message) {
					t.Errorf("output lacks %q: %q", message, output.String())
				}
			}
			if len(backend.items)+len(backend.parents)+len(backend.children)+len(backend.blocks) != 0 {
				t.Fatalf("backend mutated: %#v", backend)
			}
		})
	}
}

func TestProposalDurableDocumentPaths(t *testing.T) {
	for _, tc := range []struct{ path, status string }{
		{"CONTEXT.md", "fix_required\n"},
		{"docs/adr/0001-decision.md", "fix_required\n"},
		{"docs/capabilities/legacy.md", "completed\n"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "ship-widget")
			runGit(t, root, "switch", "main")
			worktree := filepath.Join(root, ".worktrees", "ship-widget")
			runGit(t, root, "worktree", "add", worktree, "ship-widget")
			path := filepath.Join(root, tc.path)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("uncommitted\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			backend := &memoryBackend{}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			if err := app.Run([]string{"skl", "propose", "publish", "--repo", worktree, "--target", "main", "--slice", proposalSliceFlag(t, "ship-widget")}); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(output.String(), tc.status) {
				t.Fatalf("output = %q, want %q", output.String(), tc.status)
			}
			if published := len(backend.items) == 1; published != (tc.status == "completed\n") {
				t.Fatalf("unexpected publication: %#v", backend.items)
			}
			if got := readFile(t, path); got != "uncommitted\n" {
				t.Fatalf("uncommitted document changed: %q", got)
			}
		})
	}
}

func TestResumePartialProposalPublication(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "base")
	prepareSlice(t, root, "dependent")
	directory := t.TempDir()
	for _, name := range []string{"parent", "base", "dependent"} {
		if err := os.WriteFile(filepath.Join(directory, name+".md"), []byte(name+" body\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	arguments := []string{"skl", "propose", "publish", "--repo", root, "--target", "main",
		"--slice", "base=" + filepath.Join(directory, "base.md"), "--slice", "dependent=" + filepath.Join(directory, "dependent.md"),
		"--depends", "dependent:base", "--parent-title", "proposal", "--parent-body", filepath.Join(directory, "parent.md")}
	backend := &memoryBackend{failReady: "work-2"}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run(arguments); err == nil || !strings.Contains(err.Error(), "temporary backend failure") {
		t.Fatalf("first publication error = %v", err)
	}
	if err := app.Run(arguments); err != nil {
		t.Fatal(err)
	}

	if len(backend.parents) != 1 || len(backend.items) != 2 {
		t.Fatalf("records duplicated: parents=%#v items=%#v", backend.parents, backend.items)
	}
	if !slices.Equal(backend.children, [][2]workflow.WorkItemID{{"coordination-1", "work-1"}, {"coordination-1", "work-2"}}) || !slices.Equal(backend.blocks, [][2]workflow.WorkItemID{{"work-2", "work-1"}}) {
		t.Fatalf("relationships duplicated: children=%v dependencies=%v", backend.children, backend.blocks)
	}
	if !backend.items[0].Ready || !backend.items[1].Ready {
		t.Fatalf("publication did not resume to Ready: %#v", backend.items)
	}
}

func TestReconcileCompletedProposalThroughGitHubAdapter(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "base")
	prepareSlice(t, root, "dependent")
	directory := t.TempDir()
	for _, name := range []string{"parent", "base", "dependent"} {
		if err := os.WriteFile(filepath.Join(directory, name+".md"), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mutations := 0
	client := &http.Client{Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			mutations++
		}
		body := ""
		switch request.URL.Path {
		case "/repos/acme/widgets/issues":
			body = `[
				{"id":500,"number":100,"title":"parent","body":"parent\n"},
				{"id":501,"number":1,"title":"base","body":"base\n","labels":[{"name":"ready"}]},
				{"id":502,"number":2,"title":"dependent","body":"dependent\n","labels":[{"name":"ready"}]}
			]`
		case "/repos/acme/widgets/issues/1/parent", "/repos/acme/widgets/issues/2/parent":
			body = `{"id":500,"number":100}`
		case "/repos/acme/widgets/issues/1/dependencies/blocked_by":
			body = `[]`
		case "/repos/acme/widgets/issues/2/dependencies/blocked_by":
			body = `[{"id":501,"number":1}]`
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	backend := setup.NewGitHubBackend("https://api.github.test", "secret", client)
	var output bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		if repository != (github.RepositoryID{}) {
			backend.BindRepository(repository)
		}
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)

	err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main",
		"--slice", "base=" + filepath.Join(directory, "base.md"), "--slice", "dependent=" + filepath.Join(directory, "dependent.md"),
		"--depends", "dependent:base", "--parent-title", "parent", "--parent-body", filepath.Join(directory, "parent.md")})
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "completed\n" || mutations != 0 {
		t.Fatalf("reconciliation = %q with %d mutations", output.String(), mutations)
	}
}

func TestReconcileFallbackProposalWithoutMutations(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "base")
			prepareSlice(t, root, "dependent")
			directory := t.TempDir()
			for _, name := range []string{"parent", "base", "dependent"} {
				if err := os.WriteFile(filepath.Join(directory, name+".md"), []byte(name+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			mutations := 0
			client := &http.Client{Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet {
					mutations++
				}
				body := ""
				code := http.StatusOK
				switch request.URL.Path {
				case "/repos/acme/widgets/issues":
					body = `[
						{"id":500,"number":100,"title":"parent","body":"parent\n","state":"open"},
						{"id":501,"number":1,"title":"base","body":"base\n","state":"open","labels":[{"name":"ready"}]},
						{"id":502,"number":2,"title":"dependent","body":"dependent\n\nBlocked by: #1\n","state":"open","labels":[{"name":"ready"}]}
					]`
				case "/repos/acme/widgets/issues/1/parent", "/repos/acme/widgets/issues/2/parent":
					body = `{"id":500,"number":100}`
				case "/repos/acme/widgets/issues/1/dependencies/blocked_by", "/repos/acme/widgets/issues/2/dependencies/blocked_by":
					code, body = status, `[]`
				default:
					t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
				}
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			var output bytes.Buffer
			app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
				backend := setup.NewGitHubBackend("https://api.github.test", "secret", client)
				if repository != (github.RepositoryID{}) {
					backend.BindRepository(repository)
				}
				return backend, nil
			}, bytes.NewReader(nil), &output, &output)
			for range 2 {
				output.Reset()
				err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main",
					"--slice", "base=" + filepath.Join(directory, "base.md"), "--slice", "dependent=" + filepath.Join(directory, "dependent.md"),
					"--depends", "dependent:base", "--parent-title", "parent", "--parent-body", filepath.Join(directory, "parent.md")})
				if err != nil {
					t.Fatal(err)
				}
				if output.String() != "completed\n" || mutations != 0 {
					t.Fatalf("fallback retry = %q with %d mutations", output.String(), mutations)
				}
			}
		})
	}
}

type httpRoundTripFunc func(*http.Request) (*http.Response, error)

func (function httpRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestClosedProposalRecordsStopBeforeMutation(t *testing.T) {
	for _, closed := range []string{"parent", "child"} {
		t.Run(closed, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "base")
			prepareSlice(t, root, "dependent")
			directory := t.TempDir()
			for _, name := range []string{"parent", "base", "dependent"} {
				if err := os.WriteFile(filepath.Join(directory, name+".md"), []byte(name+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			parentState, childState := "open", "open"
			if closed == "parent" {
				parentState = "closed"
			} else {
				childState = "closed"
			}
			mutations := 0
			client := &http.Client{Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet {
					mutations++
				}
				body := `{}`
				switch {
				case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/widgets/issues":
					body = fmt.Sprintf(`[{"id":500,"number":100,"title":"parent","body":"parent\n","state":%q},{"id":501,"number":1,"title":"base","body":"base\n","state":%q}]`, parentState, childState)
				case strings.HasSuffix(request.URL.Path, "/parent"):
					body = `{"id":500,"number":100}`
				case strings.HasSuffix(request.URL.Path, "/dependencies/blocked_by"):
					body = `[]`
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			var output bytes.Buffer
			app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
				backend := setup.NewGitHubBackend("https://api.github.test", "secret", client)
				if repository != (github.RepositoryID{}) {
					backend.BindRepository(repository)
				}
				return backend, nil
			}, bytes.NewReader(nil), &output, &output)
			err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main",
				"--slice", "base=" + filepath.Join(directory, "base.md"), "--slice", "dependent=" + filepath.Join(directory, "dependent.md"),
				"--parent-title", "parent", "--parent-body", filepath.Join(directory, "parent.md")})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(output.String(), "needs_human\n") || !strings.Contains(output.String(), "closed") || mutations != 0 {
				t.Fatalf("closed %s: output=%q mutations=%d", closed, output.String(), mutations)
			}
		})
	}
}

func TestAmbiguousProposalRetryStopsBeforeMutation(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "one")
	prepareSlice(t, root, "two")
	directory := t.TempDir()
	for _, name := range []string{"parent", "one", "two"} {
		if err := os.WriteFile(filepath.Join(directory, name+".md"), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backend := &memoryBackend{items: []workflow.WorkItem{{ID: "work-1", Title: "one"}, {ID: "work-2", Title: "one"}}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main",
		"--slice", "one=" + filepath.Join(directory, "one.md"), "--slice", "two=" + filepath.Join(directory, "two.md"),
		"--parent-title", "parent", "--parent-body", filepath.Join(directory, "parent.md")})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(output.String(), "needs_human\n") {
		t.Fatalf("output = %q", output.String())
	}
	if len(backend.parents) != 0 || len(backend.items) != 2 {
		t.Fatalf("backend mutated before ambiguity was reported: %#v", backend)
	}
}

func TestContradictoryDependenciesStopBeforeMutation(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "base")
	prepareSlice(t, root, "dependent")
	directory := t.TempDir()
	for _, name := range []string{"parent", "base", "dependent"} {
		if err := os.WriteFile(filepath.Join(directory, name+".md"), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backend := &memoryBackend{
		parents: []workflow.CoordinationItem{{ID: "coordination-1", Title: "parent", Body: "parent\n"}},
		items: []workflow.WorkItem{
			{ID: "work-1", Title: "base", Body: "base\n", Parent: "coordination-1", Ready: true},
			{ID: "work-2", Title: "dependent", Body: "dependent\n", Parent: "coordination-1", Blockers: []workflow.WorkItemID{"work-99"}, Ready: true},
		},
	}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main",
		"--slice", "base=" + filepath.Join(directory, "base.md"), "--slice", "dependent=" + filepath.Join(directory, "dependent.md"),
		"--depends", "dependent:base", "--parent-title", "parent", "--parent-body", filepath.Join(directory, "parent.md")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "needs_human\n") || len(backend.children)+len(backend.blocks) != 0 {
		t.Fatalf("contradictory retry mutated backend: output=%q backend=%#v", output.String(), backend)
	}
}

func TestCleanOnlySafeMergedWorktrees(t *testing.T) {
	root := proposalRepository(t)
	worktrees := filepath.Join(root, ".worktrees")
	for _, slug := range []string{"merged-clean", "merged-dirty", "unrelated"} {
		runGit(t, root, "worktree", "add", filepath.Join(worktrees, slug), "-b", slug, "main")
	}
	unexpected := filepath.Join(root, "elsewhere")
	runGit(t, root, "worktree", "add", unexpected, "-b", "merged-unexpected", "main")
	if err := os.WriteFile(filepath.Join(worktrees, "merged-dirty", "local.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"merged-clean", "merged-dirty", "merged-unexpected", "unrelated"} {
		runGit(t, root, "update-ref", "refs/remotes/origin/"+slug, "refs/heads/"+slug)
	}
	accepted := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))
	backend := &memoryBackend{items: []workflow.WorkItem{
		{ID: "work-1", Title: "merged-clean", Branch: "merged-clean", Merged: true, AcceptedHead: accepted},
		{ID: "work-2", Title: "merged-dirty", Branch: "merged-dirty", Merged: true, AcceptedHead: accepted},
		{ID: "work-3", Title: "merged-unexpected", Branch: "merged-unexpected", Merged: true, AcceptedHead: accepted},
	}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "propose", "cleanup", "--repo", root}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(worktrees, "merged-clean")); !os.IsNotExist(err) {
		t.Fatalf("clean merged worktree remains: %v", err)
	}
	if gitRefExists(root, "refs/heads/merged-clean") {
		t.Fatal("clean merged branch remains")
	}
	for _, path := range []string{filepath.Join(worktrees, "merged-dirty"), unexpected, filepath.Join(worktrees, "unrelated")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preserved worktree %s: %v", path, err)
		}
	}
	for _, slug := range []string{"merged-clean", "merged-dirty", "merged-unexpected", "unrelated"} {
		if !gitRefExists(root, "refs/remotes/origin/"+slug) {
			t.Fatalf("remote branch %s removed", slug)
		}
	}
	if got := output.String(); !strings.Contains(got, "removed merged-clean") || !strings.Contains(got, "preserved merged-dirty") || !strings.Contains(got, "preserved merged-unexpected") {
		t.Fatalf("cleanup report = %q", got)
	}
}

func TestCleanMergedBranchWithPrunedUpstream(t *testing.T) {
	root := proposalRepository(t)
	path := filepath.Join(root, ".worktrees", "squashed")
	runGit(t, root, "worktree", "add", path, "-b", "squashed", "main")
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("accepted change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "commit", "-am", "slice change")
	runGit(t, root, "update-ref", "refs/remotes/origin/squashed", "refs/heads/squashed")
	runGit(t, root, "branch", "--set-upstream-to=origin/squashed", "squashed")
	runGit(t, root, "update-ref", "-d", "refs/remotes/origin/squashed")
	// A squash puts the same change on main without the slice commit's ancestry.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("accepted change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-am", "accepted squash")
	backend := &memoryBackend{items: []workflow.WorkItem{{Title: "squashed", Branch: "squashed", Merged: true, AcceptedHead: strings.TrimSpace(runGitOutput(t, path, "rev-parse", "HEAD"))}}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	for range 2 {
		if err := app.Run([]string{"skl", "propose", "cleanup", "--repo", root}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) || gitRefExists(root, "refs/heads/squashed") {
		t.Fatalf("Merged local state remains: worktree error=%v", err)
	}
	if !strings.Contains(output.String(), "removed squashed") {
		t.Fatalf("cleanup report = %q", output.String())
	}
}

func TestCleanupPreservesUnacceptedLocalHead(t *testing.T) {
	for _, evidence := range []string{"extra local commit", "unknown accepted head"} {
		t.Run(evidence, func(t *testing.T) {
			root := proposalRepository(t)
			path := filepath.Join(root, ".worktrees", "merged")
			runGit(t, root, "worktree", "add", path, "-b", "merged", "main")
			accepted := strings.TrimSpace(runGitOutput(t, path, "rev-parse", "HEAD"))
			if evidence == "extra local commit" {
				if err := os.WriteFile(filepath.Join(path, "local.txt"), []byte("user work\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				runGit(t, path, "add", "local.txt")
				runGit(t, path, "commit", "-m", "user work after merge")
			} else {
				accepted = ""
			}
			head := runGitOutput(t, path, "rev-parse", "HEAD")
			backend := &memoryBackend{items: []workflow.WorkItem{{Title: "merged", Branch: "merged", Merged: true, AcceptedHead: accepted}}}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			if err := app.Run([]string{"skl", "propose", "cleanup", "--repo", root}); err != nil {
				t.Fatal(err)
			}
			if !gitRefExists(root, "refs/heads/merged") {
				t.Fatal("cleanup deleted an unaccepted local head")
			}
			if got := runGitOutput(t, path, "rev-parse", "HEAD"); got != head || output.String() != "preserved merged\n" {
				t.Fatalf("head=%q report=%q", got, output.String())
			}
		})
	}
}

func TestRetrieveEquivalentTypedInstructions(t *testing.T) {
	wantInstructions := readRepositoryFile(t, "skills/dev/tdd/SKILL.md")
	wantResources := []string{"reference/mocking.md", "reference/tests.md"}
	wantMarkdown := "Protocol: skl.instructions/v1\nSkill: tdd\nIncluded skills: none\nFacts: {}\nResources: reference/mocking.md, reference/tests.md\n\n" + wantInstructions
	var markdown bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &markdown, &markdown, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "tdd"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	app = newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "--format", "json", "tdd"}); err != nil {
		t.Fatal(err)
	}
	var packet skilldist.Packet
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatalf("stdout is not a JSON packet: %v\n%s", err, stdout.String())
	}
	if packet.Protocol != "skl.instructions/v1" || packet.Skill != "tdd" || packet.Facts != (skilldist.InvocationFacts{}) || len(packet.IncludedSkills) != 0 || !slices.Equal(packet.Resources, wantResources) || packet.Instructions != wantInstructions || markdown.String() != wantMarkdown {
		t.Fatalf("JSON and Markdown packets differ: %#v", packet)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRetrieveAuditWithoutPonytail(t *testing.T) {
	for _, format := range []string{"markdown", "json"} {
		t.Run(format, func(t *testing.T) {
			var output bytes.Buffer
			app := newApp(nil, bytes.NewReader(nil), &output, &output)
			if err := app.Run([]string{"skl", "skill", "--format", format, "audit"}); err != nil {
				t.Fatal(err)
			}
			instructions := output.String()
			if format == "json" {
				var packet skilldist.Packet
				if err := json.Unmarshal(output.Bytes(), &packet); err != nil {
					t.Fatal(err)
				}
				if len(packet.IncludedSkills) != 0 || !slices.Equal(packet.Resources, []string{"reference/smells.md"}) {
					t.Fatalf("unexpected Audit dependencies: %#v", packet)
				}
				instructions = packet.Instructions
			} else {
				header := "Protocol: skl.instructions/v1\nSkill: audit\nIncluded skills: none\nFacts: {}\nResources: reference/smells.md\n\n"
				if !strings.HasPrefix(instructions, header) {
					t.Fatalf("unexpected Audit manifest: %s", instructions)
				}
				instructions = strings.TrimPrefix(instructions, header)
			}
			if instructions != readRepositoryFile(t, "skills/dev/audit/SKILL.md") {
				t.Error("retrieved Audit differs from its authoritative definition")
			}
			for _, forbidden := range []string{"ponytail", "750", "net-lines", "## Simplicity"} {
				if strings.Contains(strings.ToLower(instructions), strings.ToLower(forbidden)) {
					t.Errorf("Audit retains or injects %q", forbidden)
				}
			}
			for _, required := range []string{"**Standards**", "**Artifacts**", "**smell baseline**", "**The documented gate**", "**Artifact integrity**", "`HARD` or `JUDGEMENT`", "Under 500 words.", "### 6. Aggregate", "Do **not** merge or rerank findings"} {
				if !strings.Contains(instructions, required) {
					t.Errorf("Audit lost ordinary review instruction %q", required)
				}
			}
		})
	}
}

func TestRenderImplementSubmissionInstructions(t *testing.T) {
	directory := t.TempDir()
	cases := []struct {
		procedure string
		present   []string
		absent    []string
	}{
		{
			procedure: "initial",
			present: []string{
				"`" + filepath.Join(directory, "submission.md") + "`",
				"## Summary",
				"## Verification",
				"## Audit ledger",
				"scenario",
				"Full Gate",
				"fixed point",
				"Closes #",
			},
			absent: []string{"## Rework", "resolution commit"},
		},
		{
			procedure: "rework",
			present: []string{
				"`" + filepath.Join(directory, "submission.md") + "`",
				"## Summary",
				"## Verification",
				"## Audit ledger",
				"## Rework",
				"stable",
				"resolution commit",
				"Debt Marker",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.procedure, func(t *testing.T) {
			instructions := renderResource(t, "implement", "reference/submission.md",
				"result_directory="+directory, "procedure="+testCase.procedure)
			for _, want := range testCase.present {
				if !strings.Contains(instructions, want) {
					t.Errorf("instructions are missing %q:\n%s", want, instructions)
				}
			}
			for _, unwanted := range testCase.absent {
				if strings.Contains(instructions, unwanted) {
					t.Errorf("instructions include %q:\n%s", unwanted, instructions)
				}
			}
			if sha := regexp.MustCompile(`[0-9a-f]{40}`).FindString(instructions); sha != "" {
				t.Errorf("instructions authored the reviewed %s instead of leaving it to the worker:\n%s", sha, instructions)
			}
		})
	}
}

func TestRenderImplementDecisionInstructions(t *testing.T) {
	directory := t.TempDir()
	obligations := []string{
		"`" + filepath.Join(directory, "decision.md") + "`",
		"blocking requirement",
		"current Workflow State",
		"completed work",
		"options",
		"consequences",
		"recommendation",
		"--reason",
		"Do not invent Completion, tick unfinished work, or retire an incomplete ledger during this pause.",
	}
	cases := []struct {
		preserve string
		present  []string
		absent   []string
	}{
		{
			preserve: "false",
			present:  append(slices.Clone(obligations), "no implementation work"),
			absent:   []string{"--body", "push the branch"},
		},
		{
			preserve: "true",
			present: append(slices.Clone(obligations),
				"--body", "push the branch", "`"+filepath.Join(directory, "submission.md")+"`"),
		},
	}
	for _, testCase := range cases {
		t.Run("preserve="+testCase.preserve, func(t *testing.T) {
			instructions := renderResource(t, "implement", "reference/decision.md",
				"result_directory="+directory, "preserve="+testCase.preserve)
			for _, want := range testCase.present {
				if !strings.Contains(instructions, want) {
					t.Errorf("instructions are missing %q:\n%s", want, instructions)
				}
			}
			for _, unwanted := range testCase.absent {
				if strings.Contains(instructions, unwanted) {
					t.Errorf("instructions include %q:\n%s", unwanted, instructions)
				}
			}
		})
	}
}

func TestRenderWatchdogReviewInstructions(t *testing.T) {
	obligations := []string{
		"W<n>",
		"identity",
		"monotonic",
		"owner",
		"member",
		"collaborator",
		"after the finding",
		"case-insensitive",
		"latest authorized directive wins",
		"WAIVE",
		"BLOCK",
		"NOTE",
		"reactions",
		"silence",
		"deleted",
		"findings.json",
		"verdict",
		"Manual Verification",
		"unchecked",
		"Audit",
		"original reviewed head",
	}
	// A first review and a repeat review are the two distinct contexts. A reset
	// Review Count re-enters at round 1 and must render the same identities and
	// authorization guidance, so it needs no separate case.
	for _, testCase := range []struct {
		round int
		head  string
	}{{1, strings.Repeat("a", 40)}, {2, strings.Repeat("b", 40)}} {
		t.Run(fmt.Sprintf("round %d", testCase.round), func(t *testing.T) {
			directory := t.TempDir()
			// No verdict, finding disposition, or Result Document prose is supplied:
			// the authorization and precedence guidance must already be available.
			instructions := renderResource(t, "watchdog", "reference/review.md",
				"result_directory="+directory,
				fmt.Sprintf("round=%d", testCase.round),
				"reviewed_head="+testCase.head)
			for _, want := range append(slices.Clone(obligations),
				fmt.Sprintf("round %d", testCase.round), testCase.head,
				filepath.Join(directory, "summary.md"), filepath.Join(directory, "submission.md")) {
				if !strings.Contains(instructions, want) {
					t.Errorf("instructions are missing %q:\n%s", want, instructions)
				}
			}
		})
	}
}

// renderResource runs one named-resource request through the in-process CLI and
// returns its rendered instructions. Any Workflow Backend access is a defect.
func renderResource(t *testing.T, owner, resource string, inputs ...string) string {
	t.Helper()
	command := []string{"skl", "skill", "--resource", resource}
	for _, input := range inputs {
		command = append(command, "--input", input)
	}
	command = append(command, owner)
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("resource rendering reached the Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run(command); err != nil {
		t.Fatalf("%v: %v", command, err)
	}
	return output.String()
}

func TestRejectInvalidResourceInputs(t *testing.T) {
	directory := t.TempDir()
	head := strings.Repeat("a", 40)
	cases := []struct {
		name     string
		owner    string
		resource string
		inputs   []string
		describe bool
		wants    []string
	}{
		{
			name: "input without separator", owner: "implement", resource: "reference/submission.md",
			inputs: []string{"result_directory"},
			wants:  []string{"result_directory", "name=value"},
		},
		{
			name: "undeclared input", owner: "implement", resource: "reference/submission.md",
			inputs: []string{"result_directory=" + directory, "procedure=initial", "findings=1"},
			wants:  []string{"findings", "--describe-inputs"},
		},
		{
			name: "duplicate input", owner: "watchdog", resource: "reference/review.md",
			inputs: []string{"result_directory=" + directory, "round=1", "round=1", "reviewed_head=" + head},
			wants:  []string{"round", "duplicate"},
		},
		{
			name: "missing required input", owner: "implement", resource: "reference/decision.md",
			inputs: []string{"result_directory=" + directory},
			wants:  []string{"preserve", "required"},
		},
		{
			name: "invalid boolean", owner: "implement", resource: "reference/decision.md",
			inputs: []string{"result_directory=" + directory, "preserve=maybe"},
			wants:  []string{"preserve", "boolean"},
		},
		{
			name: "invalid integer", owner: "watchdog", resource: "reference/review.md",
			inputs: []string{"result_directory=" + directory, "round=two", "reviewed_head=" + head},
			wants:  []string{"round", "integer"},
		},
		{
			name: "unsupported choice", owner: "implement", resource: "reference/submission.md",
			inputs: []string{"result_directory=" + directory, "procedure=later"},
			wants:  []string{"procedure", "initial", "rework"},
		},
		{
			name: "zero round", owner: "watchdog", resource: "reference/review.md",
			inputs: []string{"result_directory=" + directory, "round=0", "reviewed_head=" + head},
			wants:  []string{"round", "positive"},
		},
		{
			name: "empty result directory", owner: "implement", resource: "reference/decision.md",
			inputs: []string{"result_directory=", "preserve=true"},
			wants:  []string{"result_directory", "absolute"},
		},
		{
			name: "relative result directory", owner: "implement", resource: "reference/submission.md",
			inputs: []string{"result_directory=.worktrees/result", "procedure=initial"},
			wants:  []string{"result_directory", "absolute"},
		},
		{
			name: "malformed reviewed head", owner: "watchdog", resource: "reference/review.md",
			inputs: []string{"result_directory=" + directory, "round=1", "reviewed_head=" + head[:12] + "nonsense"},
			wants:  []string{"reviewed_head", "40-character"},
		},
		{
			name: "input without resource", owner: "implement",
			inputs: []string{"result_directory=" + directory},
			wants:  []string{"--input", "--resource"},
		},
		{
			name: "describe without resource", owner: "implement", describe: true,
			wants: []string{"--describe-inputs", "--resource"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Fatal("invalid resource input reached the Workflow Backend")
				return nil, nil
			}, bytes.NewReader(nil), &output, &output)
			command := []string{"skl", "skill"}
			if testCase.resource != "" {
				command = append(command, "--resource", testCase.resource)
			}
			if testCase.describe {
				command = append(command, "--describe-inputs")
			}
			for _, input := range testCase.inputs {
				command = append(command, "--input", input)
			}
			command = append(command, testCase.owner)
			err := app.Run(command)
			if err == nil {
				t.Fatalf("%v returned no error:\n%s", command, output.String())
			}
			for _, want := range testCase.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%v error %q does not identify %q", command, err, want)
				}
			}
			if output.Len() != 0 {
				t.Errorf("%v returned procedural content: %q", command, output.String())
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Errorf("invalid input changed the Result Document directory: %v, %v", entries, err)
			}
		})
	}
}

func TestPreserveLiteralResourceInputValues(t *testing.T) {
	// Only an absolute private directory is a valid value for this input, so the
	// stress value is absolute while still carrying commas, quotes, embedded
	// equals, literal template syntax, and a trailing space.
	first := `/tmp/one, two=three 'four' "five" {{.ResultDirectory}} `
	second := `/tmp/{{template "result-document" .}}/other`
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("resource rendering reached the Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	render := func(inputs ...string) (string, error) {
		output.Reset()
		command := []string{"skl", "skill", "--resource", "reference/submission.md"}
		for _, input := range inputs {
			command = append(command, "--input", input)
		}
		err := app.Run(append(command, "implement"))
		return output.String(), err
	}

	rendered, err := render("result_directory="+first, "procedure=initial")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, first+"/submission.md") {
		t.Errorf("rendering did not preserve every supplied character:\n%s", rendered)
	}

	rendered, err = render("result_directory="+second, "procedure=rework")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, second+"/submission.md") || strings.Contains(rendered, "one, two=three") {
		t.Errorf("a rendering reused an earlier call's inputs:\n%s", rendered)
	}

	if _, err := render("result_directory=" + second); err == nil || !strings.Contains(err.Error(), "procedure") {
		t.Fatalf("omitted required input was reused instead of rejected: %v", err)
	}

	// The raw collector is local to --input: ordinary slice flags still split on
	// commas and trim each element.
	sliceErr := newApp(nil, bytes.NewReader(nil), &output, &output).Run([]string{
		"skl", "propose", "publish", "--target", "main", "--slice", " one=/tmp/a.md , two ",
	})
	if sliceErr == nil || !strings.Contains(sliceErr.Error(), `invalid --slice "two"`) {
		t.Fatalf("slice flag splitting or trimming changed: %v", sliceErr)
	}
}

func TestDeferredResourceCommandsFromParentInstructions(t *testing.T) {
	// The engine creates the private Result Document directory itself, so a
	// hostile TMPDIR is how a real invocation binds values containing spaces,
	// commas, embedded equals, and shell quotes.
	hostile := filepath.Join(t.TempDir(), `res,ult 'dir' "quoted" =a=`)
	if err := os.MkdirAll(hostile, 0700); err != nil {
		t.Fatal(err)
	}
	submissionObligations := []string{"## Summary", "## Verification", "## Audit ledger", "scenario", "Full Gate"}

	cases := []struct {
		name           string
		state          workflow.State
		draft          bool
		submission     bool
		procedure      string
		resource       string
		laterValue     string
		placeholder    string
		want           []string
		absentResource []string
	}{
		{
			name: "first implementation", state: workflow.Ready, procedure: "initial",
			resource:       "reference/submission.md",
			want:           submissionObligations,
			absentResource: []string{"## Rework", "resolution commit"},
		},
		{
			// A preserved draft Submission is not evidence of finding-driven
			// Rework: the reconciled Workflow State decides the procedure.
			name: "preserved draft submission", state: workflow.Ready, submission: true, draft: true, procedure: "initial",
			resource:       "reference/submission.md",
			want:           submissionObligations,
			absentResource: []string{"## Rework", "resolution commit"},
		},
		{
			name: "finding-driven rework", state: workflow.Rework, submission: true, procedure: "rework",
			resource: "reference/submission.md",
			want:     []string{"## Rework", "resolution commit", "Debt Marker", "## Audit ledger"},
		},
		{
			name: "needs human decision", state: workflow.Ready, procedure: "initial",
			resource: "reference/decision.md", laterValue: "preserve=true", placeholder: "preserve=<true|false>",
			want: []string{"## Human Decision", "blocking requirement", "current Workflow State"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			item := workflow.ImplementationItem{ID: "7", Branch: "widget", State: testCase.state}
			if testCase.submission {
				item.Submission = &workflow.Submission{ID: "11", Head: head, Base: "main", Draft: testCase.draft}
			}
			backend := &implementationMemory{work: []workflow.ImplementationItem{item}}
			t.Setenv("TMPDIR", hostile)

			started := implementCLI(t, root, backend, "next")
			if started.Packet == nil {
				t.Fatalf("no packet: %#v", started)
			}
			facts := started.Packet.Facts.Implementation
			if string(facts.Procedure) != testCase.procedure {
				t.Fatalf("procedure = %q, want %q", facts.Procedure, testCase.procedure)
			}
			if !strings.HasPrefix(facts.ResultDirectory, hostile) {
				t.Fatalf("result directory %q is not the invocation's own private directory", facts.ResultDirectory)
			}
			for _, procedural := range []string{"# Submission Result Document", "# Decision Result Document"} {
				if strings.Contains(started.Packet.Instructions, procedural) {
					t.Errorf("parent instructions disclosed deferred resource content %q", procedural)
				}
			}

			command := deferredCommand(t, started.Packet.Instructions, testCase.resource)
			if !strings.Contains(command, "--input result_directory=") || !strings.Contains(command, "result_directory="+skilldist.ShellQuote(facts.ResultDirectory)) {
				t.Errorf("parent command does not bind the private directory: %s", command)
			}
			if testCase.resource == "reference/submission.md" && !strings.Contains(command, "--input procedure="+testCase.procedure) {
				t.Errorf("parent command does not bind procedure %q: %s", testCase.procedure, command)
			}
			if testCase.placeholder == "" {
				if strings.ContainsAny(command, "<>") {
					t.Errorf("parent command leaves a settled value as a placeholder: %s", command)
				}
			} else if !strings.Contains(command, testCase.placeholder) {
				t.Errorf("parent command does not leave %q for the worker: %s", testCase.placeholder, command)
			}

			resolved := command
			if testCase.placeholder != "" {
				resolved = strings.Replace(command, testCase.placeholder, testCase.laterValue, 1)
			}
			args := shellArgs(t, resolved)
			if args[0] != "skl" || args[len(args)-1] != "implement" {
				t.Fatalf("parent command is not runnable verbatim: %v", args)
			}
			instructions := runDeferredCommand(t, command, testCase.placeholder, testCase.laterValue)
			for _, want := range testCase.want {
				if !strings.Contains(instructions, want) {
					t.Errorf("retrieved resource is missing %q:\n%s", want, instructions)
				}
			}
			for _, unwanted := range testCase.absentResource {
				if strings.Contains(instructions, unwanted) {
					t.Errorf("retrieved resource includes %q:\n%s", unwanted, instructions)
				}
			}
			if !strings.Contains(instructions, "`"+facts.ResultDirectory+"/") {
				t.Errorf("retrieved resource did not bind the invocation's private directory:\n%s", instructions)
			}
		})
	}

	t.Run("watchdog invocation", func(t *testing.T) {
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")
		completeAndRetireSlice(t, root, "widget")
		head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		backend := &implementationMemory{work: []workflow.ImplementationItem{{
			ID: "7", Branch: "widget", State: workflow.AwaitingReview,
			Submission: &workflow.Submission{ID: "11", Head: head, Body: "opaque audit"},
		}}}
		t.Setenv("TMPDIR", hostile)

		started := watchdogCLI(t, root, backend, "next")
		if started.Packet == nil {
			t.Fatalf("no packet: %#v", started)
		}
		facts := started.Packet.Facts.Watchdog
		if !strings.HasPrefix(facts.ResultDirectory, hostile) {
			t.Fatalf("result directory %q is not the invocation's own private directory", facts.ResultDirectory)
		}
		if strings.Contains(started.Packet.Instructions, "# Review Result Documents") {
			t.Error("parent instructions disclosed the deferred review resource")
		}
		command := deferredCommand(t, started.Packet.Instructions, "reference/review.md")
		for _, want := range []string{
			"result_directory=" + skilldist.ShellQuote(facts.ResultDirectory),
			"round=" + strconv.FormatUint(facts.ReviewNumber, 10),
			"reviewed_head=" + skilldist.ShellQuote(head),
		} {
			if !strings.Contains(command, want) {
				t.Errorf("parent command does not bind %q: %s", want, command)
			}
		}
		for _, settled := range []string{"<", ">", "--verdict"} {
			if strings.Contains(command, settled) {
				t.Errorf("parent command includes %q instead of a settled value: %s", settled, command)
			}
		}
		args := shellArgs(t, command)
		if args[0] != "skl" || args[len(args)-1] != "watchdog" {
			t.Fatalf("parent command is not runnable verbatim: %v", args)
		}
		instructions := runDeferredCommand(t, command, "", "")
		for _, want := range []string{
			"`" + facts.ResultDirectory + "/summary.md`",
			"round " + strconv.FormatUint(facts.ReviewNumber, 10),
			head,
			"latest authorized directive wins",
			"Manual Verification",
		} {
			if !strings.Contains(instructions, want) {
				t.Errorf("retrieved review resource is missing %q:\n%s", want, instructions)
			}
		}
	})
}

// runDeferredCommand resolves one emitted resource command the way a shell
// would, fills only its documented later value, and returns the retrieved
// resource.
func runDeferredCommand(t *testing.T, command, placeholder, value string) string {
	t.Helper()
	if placeholder != "" {
		command = strings.Replace(command, placeholder, value, 1)
	}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("resource rendering reached the Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run(shellArgs(t, command)); err != nil {
		t.Fatalf("%s: %v", command, err)
	}
	return output.String()
}

// deferredCommand returns the single deferred resource command a parent's
// instructions publish for one resource.
func deferredCommand(t *testing.T, instructions, resource string) string {
	t.Helper()
	var commands []string
	for _, chunk := range strings.Split(instructions, "`") {
		if strings.HasPrefix(chunk, "skl skill --resource "+resource+" ") {
			commands = append(commands, chunk)
		}
	}
	if len(commands) != 1 {
		t.Fatalf("want exactly one %s command in the parent instructions, got %v", resource, commands)
	}
	return commands[0]
}

func TestContextFreeResourcesRetainOwnershipAndDefaults(t *testing.T) {
	// Each marker is an independent expectation about the authored reference, so
	// these checks never take the template source as their own oracle.
	cases := []struct {
		owner, resource string
		markers         []string
	}{
		{"audit", "reference/smells.md", []string{"# Smell Baseline", "Feature Envy"}},
		{"design", "reference/DEEPENING.md", []string{"# Deepening", "Adapters"}},
		{"design", "reference/DESIGN-IT-TWICE.md", []string{"design constraint", "sub-agent"}},
		{"domain", "reference/ADR-FORMAT.md", []string{"# ADR Format", "Consequences"}},
		{"domain", "reference/CONTEXT-FORMAT.md", []string{"# {Context Name}", "## Contexts"}},
		{"propose", "reference/intent.md", []string{"Definition of Done", "## Out of Scope"}},
		{"propose", "reference/behavior.md", []string{"Gherkin", "Scenario"}},
		{"propose", "reference/plan.md", []string{"Module shapes", "## Approach"}},
		{"propose", "reference/tasks.md", []string{"Behavioral", "## Docs"}},
		{"tdd", "reference/mocking.md", []string{"# When to Mock", "mock"}},
		{"tdd", "reference/tests.md", []string{"# Good and Bad Tests", "Integration-style"}},
		{"writing-for-agents", "SKILL-MECHANICS.md", []string{"# Skill mechanics", "## Invocation"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.owner+"/"+testCase.resource, func(t *testing.T) {
			resource := renderResource(t, testCase.owner, testCase.resource)
			for _, marker := range testCase.markers {
				if !strings.Contains(resource, marker) {
					t.Errorf("%s is missing %q:\n%s", testCase.resource, marker, resource)
				}
			}
			if strings.Contains(resource, "{{") {
				t.Errorf("context-free %s leaked an unrendered template action:\n%s", testCase.resource, resource)
			}
			for _, other := range []string{"implement", "watchdog", "tdd"} {
				if other == testCase.owner {
					continue
				}
				var output bytes.Buffer
				app := newApp(nil, bytes.NewReader(nil), &output, &output)
				if err := app.Run([]string{"skl", "skill", "--resource", testCase.resource, other}); err == nil || !strings.Contains(err.Error(), "unknown resource") {
					t.Errorf("%s became a resource of %s: %v", testCase.resource, other, err)
				}
			}
		})
	}

	packet, err := skilldist.BuildPacket("implement", skilldist.InvocationFacts{})
	if err != nil {
		t.Fatal(err)
	}
	for _, owned := range []string{"reference/smells.md audit", "reference/tests.md tdd", "reference/DEEPENING.md design", "reference/CONTEXT-FORMAT.md domain"} {
		if !strings.Contains(packet.Instructions, owned) {
			t.Errorf("bundled definition lost its owning reference %q", owned)
		}
	}
	for _, stolen := range []string{"reference/smells.md implement", "reference/tests.md implement", "reference/smells.md, ", "reference/tests.md, "} {
		if strings.Contains(packet.Instructions+strings.Join(packet.Resources, ", "), stolen) {
			t.Errorf("bundled instructions took over %q", stolen)
		}
	}

	var markdown, jsonPacket bytes.Buffer
	if err := newApp(nil, bytes.NewReader(nil), &markdown, &markdown).Run([]string{"skl", "skill", "tdd"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(markdown.String(), "Protocol: skl.instructions/v1\nSkill: tdd\nIncluded skills: none\nFacts: {}\nResources: reference/mocking.md, reference/tests.md\n\n") {
		t.Errorf("default Markdown rendering changed:\n%s", markdown.String())
	}
	if err := newApp(nil, bytes.NewReader(nil), &jsonPacket, &jsonPacket).Run([]string{"skl", "skill", "--format", "json", "tdd"}); err != nil {
		t.Fatal(err)
	}
	var decoded skilldist.Packet
	if err := json.Unmarshal(jsonPacket.Bytes(), &decoded); err != nil || decoded.Skill != "tdd" {
		t.Fatalf("typed packet default changed: %v, %v", err, jsonPacket.String())
	}
}

func TestPrivateSkillModulesAreNotPublicResources(t *testing.T) {
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("private module request reached the Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)

	// SKILL.md is a definition and authored modules are internal: neither is a
	// public resource, by enumeration, retrieval, or input discovery.
	for _, resource := range []string{"modules/result-document.md", "SKILL.md"} {
		for _, request := range [][]string{
			{"skl", "skill", "--resource", resource, "implement"},
			{"skl", "skill", "--resource", resource, "--describe-inputs", "implement"},
		} {
			output.Reset()
			err := app.Run(request)
			if err == nil || !strings.Contains(err.Error(), "unknown resource") {
				t.Errorf("%v = %v, want an unavailable resource", request, err)
			}
			if output.Len() != 0 || (err != nil && strings.Contains(err.Error(), "never parses, judges, or cross-checks")) {
				t.Errorf("%v disclosed private content: %q", request, output.String())
			}
		}
	}

	var rendered bytes.Buffer
	if err := newApp(nil, bytes.NewReader(nil), &rendered, &rendered).Run([]string{"skl", "skill", "--format", "json", "implement"}); err != nil {
		t.Fatal(err)
	}
	var packet skilldist.Packet
	if err := json.Unmarshal(rendered.Bytes(), &packet); err != nil {
		t.Fatal(err)
	}
	for _, resource := range packet.Resources {
		if strings.HasPrefix(resource, "modules/") {
			t.Errorf("private module %s is listed as a public resource", resource)
		}
	}

	// The embedded module composes into both Implement result documents.
	for resource, inputs := range map[string][]string{
		"reference/submission.md": {"result_directory=" + t.TempDir(), "procedure=initial"},
		"reference/decision.md":   {"result_directory=" + t.TempDir(), "preserve=false"},
	} {
		if instructions := renderResource(t, "implement", resource, inputs...); !strings.Contains(instructions, "never parses, judges, or cross-checks the prose") {
			t.Errorf("%s did not compose the shared Result Document module:\n%s", resource, instructions)
		}
	}
}

func TestRetrieveOneNamedResource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "--resource", "reference/tests.md", "tdd"}); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), readRepositoryFile(t, "skills/dev/tdd/reference/tests.md"); got != want {
		t.Fatalf("stdout did not contain only the requested resource:\n%s", got)
	}
	stdout.Reset()
	if err := app.Run([]string{"skl", "skill", "--resource", "reference/CAPABILITIES-FORMAT.md", "domain"}); err == nil || !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("retired capability resource error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("retired capability resource returned content: %q", stdout.String())
	}
}

func TestDescribeNamedResourceInputs(t *testing.T) {
	cases := []struct {
		owner      string
		resource   string
		inputs     []string
		described  []string
		procedural string
	}{
		{
			owner: "implement", resource: "reference/submission.md",
			inputs: []string{"result_directory=/tmp/result", "procedure=initial"},
			described: []string{
				"result_directory (string, required): Absolute path of the private Result Document directory this invocation created.",
				"procedure (string, required, one of: initial, rework): Which submission procedure to render.",
			},
			procedural: "# Submission Result Document",
		},
		{
			owner: "implement", resource: "reference/decision.md",
			inputs: []string{"result_directory=/tmp/result", "preserve=true"},
			described: []string{
				"result_directory (string, required): Absolute path of the private Result Document directory this invocation created.",
				"preserve (boolean, required): Whether implementation work exists that a draft Submission must preserve.",
			},
			procedural: "# Decision Result Document",
		},
		{
			owner: "watchdog", resource: "reference/review.md",
			inputs: []string{"result_directory=/tmp/result", "round=2", "reviewed_head=" + strings.Repeat("a", 40)},
			described: []string{
				"result_directory (string, required): Absolute path of the private Result Document directory this invocation created.",
				"round (integer, required): Review round number for this Submission.",
				"reviewed_head (string, required): Original full SHA of the reviewed head.",
			},
			procedural: "# Review Result Documents",
		},
		{
			owner: "tdd", resource: "reference/tests.md",
			described:  []string{"reference/tests.md accepts no inputs."},
			procedural: "# Good and Bad Tests",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.owner+"/"+testCase.resource, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Fatal("resource discovery reached the Workflow Backend")
				return nil, nil
			}, bytes.NewReader(nil), &stdout, &stderr)
			describe := []string{"skl", "skill", "--resource", testCase.resource, "--describe-inputs", testCase.owner}
			if err := app.Run(describe); err != nil {
				t.Fatalf("%v: %v", describe, err)
			}
			description := stdout.String()
			if !strings.Contains(description, testCase.resource+" accepts") {
				t.Errorf("description does not report the requested resource:\n%s", description)
			}
			for _, want := range testCase.described {
				if !strings.Contains(description, want) {
					t.Errorf("description is missing %q:\n%s", want, description)
				}
			}
			if strings.Contains(description, testCase.procedural) {
				t.Errorf("description rendered procedural content %q:\n%s", testCase.procedural, description)
			}

			run := func(inputs []string) error {
				stdout.Reset()
				command := []string{"skl", "skill", "--resource", testCase.resource}
				for _, input := range inputs {
					command = append(command, "--input", input)
				}
				return app.Run(append(command, testCase.owner))
			}
			if err := run(testCase.inputs); err != nil {
				t.Fatalf("described inputs are not retrievable: %v", err)
			}
			if !strings.Contains(stdout.String(), testCase.procedural) {
				t.Errorf("retrieval with described inputs lacks %q:\n%s", testCase.procedural, stdout.String())
			}
			if err := run(append(slices.Clone(testCase.inputs), "undeclared=1")); err == nil || !strings.Contains(err.Error(), "undeclared") {
				t.Fatalf("description described more inputs than retrieval accepts: %v", err)
			}
		})
	}
}

func TestBundleGuaranteedSupportingSkills(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, root)
	if err := app.Run([]string{"skl", "skill", "--format", "json", "implement"}); err != nil {
		t.Fatal(err)
	}
	var packet skilldist.Packet
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatal(err)
	}
	want := []string{"tdd", "audit", "design", "domain"}
	if !slices.Equal(packet.IncludedSkills, want) {
		t.Fatalf("included_skills = %v, want %v", packet.IncludedSkills, want)
	}
	if !slices.Equal(packet.Resources, []string{"reference/decision.md", "reference/submission.md"}) {
		t.Fatalf("implementation resources changed: %v", packet.Resources)
	}
	// Definitions are authored templates, so the rendered outcome is what a
	// worker reads: every guaranteed definition appears exactly once and the
	// deferred resource bodies stay out of a no-facts packet.
	for _, marker := range []struct{ name, text string }{
		{"implement", "## The scope is already decided"},
		{"implement", "## When only a human can decide"},
		{"tdd", "## What a good test is"},
		{"audit", "### 6. Aggregate"},
		{"design", "## Deep vs shallow"},
		{"domain", "### Offer ADRs sparingly"},
	} {
		if count := strings.Count(packet.Instructions, marker.text); count != 1 {
			t.Errorf("%s marker %q appears %d times, want once", marker.name, marker.text, count)
		}
	}
	for _, deferred := range []string{"# Submission Result Document", "# Decision Result Document"} {
		if strings.Contains(packet.Instructions, deferred) {
			t.Errorf("no-facts Implement packet disclosed the deferred %q body", deferred)
		}
	}
	for _, format := range []string{"markdown", "json"} {
		t.Run(format, func(t *testing.T) {
			raw := func(name string) string {
				var out bytes.Buffer
				if err := newApp(nil, bytes.NewReader(nil), &out, &out).Run([]string{"skl", "skill", "--format", format, name}); err != nil {
					t.Fatal(err)
				}
				return out.String()
			}
			instructions := func(name string) string {
				if format == "json" {
					var rendered skilldist.Packet
					if err := json.Unmarshal([]byte(raw(name)), &rendered); err != nil {
						t.Fatal(err)
					}
					return rendered.Instructions
				}
				_, rendered, _ := strings.Cut(raw(name), "\n\n")
				return rendered
			}
			if format == "markdown" {
				wantHeader := "Protocol: skl.instructions/v1\nSkill: implement\nIncluded skills: tdd, audit, design, domain\nFacts: {}\nResources: reference/decision.md, reference/submission.md\n\n"
				if rendered := raw("implement"); !strings.HasPrefix(rendered, wantHeader) {
					t.Fatalf("rendered implementation manifest changed:\n%s", rendered)
				}
			}
			implementation := instructions("implement")
			for _, included := range want {
				if section := bundledSection(t, implementation, included); section != instructions(included) {
					t.Errorf("bundled %s differs from direct retrieval:\n%s", included, section)
				}
			}
			for _, forbidden := range []string{"ponytail", "## simplicity"} {
				if strings.Contains(strings.ToLower(implementation), forbidden) {
					t.Errorf("implementation packet retains or injects %q", forbidden)
				}
			}
		})
	}

	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	stub := readFile(t, filepath.Join(root, ".codex/skills/implement/SKILL.md"))
	if !strings.Contains(stub, "included_skills") || !strings.Contains(stub, "Skip activation") {
		t.Fatalf("stub lacks included-skill guard:\n%s", stub)
	}
}

// bundledSection returns one included definition's rendered section.
func bundledSection(t *testing.T, instructions, name string) string {
	t.Helper()
	_, after, found := strings.Cut(instructions, "\n\n## Included Skill: "+name+"\n\n")
	if !found {
		t.Fatalf("bundled instructions lack the %s section", name)
	}
	section, _, _ := strings.Cut(after, "\n\n## Included Skill: ")
	return section
}

func TestIgnoreConsumerRepositoryOverrides(t *testing.T) {
	repository := t.TempDir()
	override := filepath.Join(repository, "skills/dev/tdd/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(override), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(override, []byte("consumer override"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repository); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "tdd"}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !strings.Contains(got, "# Test-Driven Development") || strings.Contains(got, "consumer override") {
		t.Fatalf("consumer repository overrode embedded definition:\n%s", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func readRepositoryFile(t *testing.T, path string) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository root")
	}
	return readFile(t, filepath.Join(filepath.Dir(testFile), "..", "..", path))
}

func skillFrontmatter(t *testing.T, source string) string {
	t.Helper()
	end := strings.Index(source[4:], "\n---\n")
	if !strings.HasPrefix(source, "---\n") || end < 0 {
		t.Fatal("skill source has invalid frontmatter")
	}
	return source[:end+8]
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func proposalRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "remote", "add", "origin", "git@github.com:acme/widgets.git")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("widget\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "initial")
	runGit(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	return root
}

func prepareSlice(t *testing.T, root, slug string) string {
	t.Helper()
	runGit(t, root, "switch", "-c", slug, "main")
	writeLedger(t, root, slug, true)
	runGit(t, root, "add", filepath.Join(".changes", slug))
	runGit(t, root, "commit", "-m", "[baseline] "+slug)
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "update-ref", "refs/remotes/origin/"+slug, head)
	return head
}

func completeAndRetireSlice(t *testing.T, root, slug string) string {
	t.Helper()
	runGit(t, root, "commit", "--allow-empty", "-m", "[completion] "+slug)
	runGit(t, root, "rm", "-r", filepath.Join(".changes", slug))
	runGit(t, root, "commit", "-m", "retire "+slug)
	return strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
}

func writeLedger(t *testing.T, root, slug string, complete bool) {
	t.Helper()
	directory := filepath.Join(root, ".changes", slug)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{"intent.md"}
	if complete {
		names = append(names, "behavior.md")
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func proposalSliceFlag(t *testing.T, slug string) string {
	t.Helper()
	body := filepath.Join(t.TempDir(), slug+".md")
	if err := os.WriteFile(body, []byte(slug+" body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return slug + "=" + body
}

func runGitOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func gitRefExists(root, ref string) bool {
	return exec.Command("git", "-C", root, "show-ref", "--verify", "--quiet", ref).Run() == nil
}
