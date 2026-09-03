package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type memoryBackend struct {
	repository setup.RepositoryID
	labels     []setup.Label
	items      []workflow.WorkItem
	parents    []workflow.CoordinationItem
	children   [][2]int
	blocks     [][2]int
	failReady  int
}

func (b *memoryBackend) Validate(_ context.Context, repository setup.RepositoryID) (string, error) {
	b.repository = repository
	return "trunk", nil
}

func (b *memoryBackend) EnsureLabels(_ context.Context, _ setup.RepositoryID, labels []setup.Label) error {
	b.labels = append([]setup.Label(nil), labels...)
	return nil
}

func (b *memoryBackend) ListWorkItems(context.Context, workflow.RepositoryID) ([]workflow.WorkItem, error) {
	return append([]workflow.WorkItem(nil), b.items...), nil
}

func (b *memoryBackend) CreateWorkItem(_ context.Context, _ workflow.RepositoryID, item workflow.WorkItem) (workflow.WorkItem, error) {
	item.Number = len(b.items) + 1
	b.items = append(b.items, item)
	return item, nil
}

func (b *memoryBackend) ListCoordinationItems(context.Context, workflow.RepositoryID) ([]workflow.CoordinationItem, error) {
	return append([]workflow.CoordinationItem(nil), b.parents...), nil
}

func (b *memoryBackend) CreateCoordinationItem(_ context.Context, _ workflow.RepositoryID, item workflow.CoordinationItem) (workflow.CoordinationItem, error) {
	item.Number = 100 + len(b.parents)
	b.parents = append(b.parents, item)
	return item, nil
}

func (b *memoryBackend) AddChild(_ context.Context, _ workflow.RepositoryID, parent, child int) error {
	b.children = append(b.children, [2]int{parent, child})
	for index := range b.items {
		if b.items[index].Number == child {
			b.items[index].Parent = parent
		}
	}
	return nil
}

func (b *memoryBackend) AddDependency(_ context.Context, _ workflow.RepositoryID, dependent, blocker int) error {
	b.blocks = append(b.blocks, [2]int{dependent, blocker})
	for index := range b.items {
		if b.items[index].Number == dependent {
			b.items[index].Blockers = append(b.items[index].Blockers, blocker)
		}
	}
	return nil
}

func (b *memoryBackend) SetReady(_ context.Context, _ workflow.RepositoryID, number int) error {
	if b.failReady == number {
		b.failReady = 0
		return errors.New("temporary backend failure")
	}
	for index := range b.items {
		if b.items[index].Number == number {
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
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewBufferString("n\n"), &stdout, &stderr)
	if err := app.Run([]string{"skl", "setup"}); err != nil {
		t.Fatalf("setup failed: %v\nstderr: %s", err, stderr.String())
	}

	if backend.repository != (setup.RepositoryID{Owner: "acme", Name: "widgets"}) {
		t.Fatalf("repository = %#v", backend.repository)
	}
	if got := stdout.String(); got != "Link CLAUDE.md to AGENTS.md? [y/N] Prepared "+root+" for GitHub workflow on trunk.\n" {
		t.Fatalf("stdout = %q", got)
	}
	if len(backend.labels) != 6 {
		t.Fatalf("prepared %d labels", len(backend.labels))
	}
	if got := readFile(t, filepath.Join(root, ".gitignore")); got != ".worktrees/\n" {
		t.Fatalf(".gitignore = %q", got)
	}
	if got := readFile(t, filepath.Join(root, "AGENTS.md")); got != setup.AgentsBlock {
		t.Fatalf("AGENTS.md = %q", got)
	}
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
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "setup", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md created without confirmation: %v", err)
	}
}

func TestInstallSupportedSkillStubs(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)

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
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills"} {
		for name, source := range wantSkills {
			stub := readFile(t, filepath.Join(root, harness, name, "SKILL.md"))
			if !strings.HasPrefix(stub, "---\n") {
				t.Fatalf("%s %s stub has no leading YAML frontmatter:\n%s", harness, name, stub)
			}
			if want := skillFrontmatter(t, readRepositoryFile(t, source)); !strings.HasPrefix(stub, want+"\n") {
				t.Fatalf("%s %s stub changed source frontmatter:\n%s", harness, name, stub)
			}
			if !strings.Contains(stub, "skl skill "+name) || !strings.Contains(stub, "skl.stub/v1") {
				t.Fatalf("%s %s stub does not delegate to skl:\n%s", harness, name, stub)
			}
		}
		if _, err := os.Stat(filepath.Join(root, harness, "dev-setup", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("%s contains retired dev-setup stub: %v", harness, err)
		}
	}
	for _, manifest := range []string{".claude-plugin/plugin.json", ".codex-plugin/plugin.json", "package.json"} {
		if _, err := os.Stat(filepath.Join(root, manifest)); !os.IsNotExist(err) {
			t.Fatalf("plugin manifest written at %s: %v", manifest, err)
		}
	}
}

func TestInstallFailsWithoutUserHome(t *testing.T) {
	t.Setenv("HOME", "")
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "install"}); err == nil {
		t.Fatal("install succeeded without a user home")
	}
}

func TestInstallRefreshesOnlyOwnedStubs(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}

	tdd := filepath.Join(root, ".codex/skills/tdd/SKILL.md")
	wantTDD := readFile(t, tdd)
	if err := os.WriteFile(tdd, []byte("---\nname: tdd\n---\n\n<!-- skl-owned: skl.stub/v1 -->\nstale"), 0o644); err != nil {
		t.Fatal(err)
	}
	audit := filepath.Join(root, ".codex/skills/audit/SKILL.md")
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
}

func TestRetrieveRenderedSkillInstructions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

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
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "propose"}); err != nil {
		t.Fatal(err)
	}

	got := output.String()
	for _, want := range []string{"tracer-bullet", "skl propose publish", "skl propose cleanup"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Propose instructions lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "docs/github.md") {
		t.Fatalf("Propose instructions retain copied board protocol:\n%s", got)
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
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

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
	if item.Title != "ship-widget" || item.Body != "agent-authored body\n" || item.Branch != "ship-widget" || item.ArtifactBaseline != baseline || !item.Ready {
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
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	err := app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main",
		"--slice", "feature=" + filepath.Join(directory, "feature.md"),
		"--slice", "foundation=" + filepath.Join(directory, "foundation.md"),
		"--depends", "feature:foundation", "--parent-title", "widgets", "--parent-body", filepath.Join(directory, "parent.md")})
	if err != nil {
		t.Fatal(err)
	}

	if len(backend.parents) != 1 || backend.parents[0].Title != "widgets" || backend.parents[0].Body != "parent prose\n" {
		t.Fatalf("parents = %#v", backend.parents)
	}
	if got := []string{backend.items[0].Title, backend.items[1].Title}; !slices.Equal(got, []string{"foundation", "feature"}) {
		t.Fatalf("publication order = %v", got)
	}
	if !slices.Equal(backend.children, [][2]int{{100, 1}, {100, 2}}) {
		t.Fatalf("children = %v", backend.children)
	}
	if !slices.Equal(backend.blocks, [][2]int{{2, 1}}) {
		t.Fatalf("dependencies = %v", backend.blocks)
	}
	if !backend.items[0].Ready || !backend.items[1].Ready {
		t.Fatalf("items are not Ready: %#v", backend.items)
	}
}

func TestRefuseInvalidProposalPreflight(t *testing.T) {
	tests := map[string]func(*testing.T) (string, []string){
		"dirty durable document": func(t *testing.T) (string, []string) {
			root := proposalRepository(t)
			prepareSlice(t, root, "dirty-docs")
			if err := os.WriteFile(filepath.Join(root, "CONTEXT.md"), []byte("uncommitted\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return root, []string{"--slice", proposalSliceFlag(t, "dirty-docs")}
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
			runGit(t, root, "commit", "-m", "incomplete")
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
			app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			arguments := append([]string{"skl", "propose", "publish", "--repo", root, "--target", "main"}, flags...)

			if err := app.Run(arguments); err != nil {
				t.Fatal(err)
			}

			if !strings.HasPrefix(output.String(), "fix_required\n") {
				t.Fatalf("output = %q", output.String())
			}
			if len(backend.items)+len(backend.parents)+len(backend.children)+len(backend.blocks) != 0 {
				t.Fatalf("backend mutated: %#v", backend)
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
	backend := &memoryBackend{failReady: 2}
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run(arguments); err == nil || !strings.Contains(err.Error(), "temporary backend failure") {
		t.Fatalf("first publication error = %v", err)
	}
	if err := app.Run(arguments); err != nil {
		t.Fatal(err)
	}

	if len(backend.parents) != 1 || len(backend.items) != 2 {
		t.Fatalf("records duplicated: parents=%#v items=%#v", backend.parents, backend.items)
	}
	if !slices.Equal(backend.children, [][2]int{{100, 1}, {100, 2}}) || !slices.Equal(backend.blocks, [][2]int{{2, 1}}) {
		t.Fatalf("relationships duplicated: children=%v dependencies=%v", backend.children, backend.blocks)
	}
	if !backend.items[0].Ready || !backend.items[1].Ready {
		t.Fatalf("publication did not resume to Ready: %#v", backend.items)
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
	backend := &memoryBackend{items: []workflow.WorkItem{{Number: 1, Title: "one"}, {Number: 2, Title: "one"}}}
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

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
	backend := &memoryBackend{items: []workflow.WorkItem{
		{Number: 1, Title: "merged-clean", Branch: "merged-clean", Merged: true},
		{Number: 2, Title: "merged-dirty", Branch: "merged-dirty", Merged: true},
		{Number: 3, Title: "merged-unexpected", Branch: "merged-unexpected", Merged: true},
	}}
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

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

func TestRetrieveEquivalentTypedInstructions(t *testing.T) {
	wantInstructions := readRepositoryFile(t, "skills/dev/tdd/SKILL.md")
	wantResources := []string{"reference/mocking.md", "reference/tests.md"}
	wantMarkdown := "Protocol: skl.instructions/v1\nSkill: tdd\nIncluded skills: none\nFacts: {}\nResources: reference/mocking.md, reference/tests.md\n\n" + wantInstructions
	var markdown bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &markdown, &markdown, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "tdd"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	app = newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
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

func TestRetrieveOneNamedResource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "--resource", "reference/tests.md", "tdd"}); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), readRepositoryFile(t, "skills/dev/tdd/reference/tests.md"); got != want {
		t.Fatalf("stdout did not contain only the requested resource:\n%s", got)
	}
}

func TestBundleGuaranteedSupportingSkills(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, root)
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
	wantInstructions := readRepositoryFile(t, "skills/dev/implement/SKILL.md")
	for _, included := range []struct{ name, path string }{
		{"tdd", "skills/dev/tdd/SKILL.md"},
		{"audit", "skills/dev/audit/SKILL.md"},
		{"design", "skills/dev/design/SKILL.md"},
		{"domain", "skills/dev/domain/SKILL.md"},
	} {
		wantInstructions += "\n\n## Included Skill: " + included.name + "\n\n" + readRepositoryFile(t, included.path)
	}
	if packet.Instructions != wantInstructions {
		t.Fatalf("bundled instructions differ from canonical definitions:\n%s", packet.Instructions)
	}

	if err := app.Run([]string{"skl", "install"}); err != nil {
		t.Fatal(err)
	}
	stub := readFile(t, filepath.Join(root, ".codex/skills/implement/SKILL.md"))
	if !strings.Contains(stub, "included_skills") || !strings.Contains(stub, "Skip activation") {
		t.Fatalf("stub lacks included-skill guard:\n%s", stub)
	}
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
	app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
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
	runGit(t, root, "commit", "-m", "Propose "+slug)
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "update-ref", "refs/remotes/origin/"+slug, head)
	return head
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
