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
	"runtime"
	"slices"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type memoryBackend struct {
	repository     setup.RepositoryID
	labels         []setup.Label
	items          []workflow.WorkItem
	parents        []workflow.CoordinationItem
	children       [][2]int
	blocks         [][2]int
	failReady      int
	failChild      int
	failDependency int
}

func (b *memoryBackend) Validate(_ context.Context, repository setup.RepositoryID) (string, error) {
	b.repository = repository
	return "trunk", nil
}

func (b *memoryBackend) EnsureLabels(_ context.Context, _ setup.RepositoryID, labels []setup.Label) error {
	b.labels = append([]setup.Label(nil), labels...)
	return nil
}

func (b *memoryBackend) FindWorkItems(_ context.Context, _ workflow.RepositoryID, prepared []workflow.WorkItem, _ []workflow.Dependency) ([]workflow.WorkItem, error) {
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

func (b *memoryBackend) ListMergedWorkItems(context.Context, workflow.RepositoryID) ([]workflow.WorkItem, error) {
	var merged []workflow.WorkItem
	for _, item := range b.items {
		if item.Merged {
			merged = append(merged, item)
		}
	}
	return merged, nil
}

func (b *memoryBackend) CreateWorkItem(_ context.Context, _ workflow.RepositoryID, item workflow.WorkItem) (workflow.WorkItem, error) {
	item.Number = len(b.items) + 1
	b.items = append(b.items, item)
	return item, nil
}

func (b *memoryBackend) FindCoordinationItems(_ context.Context, _ workflow.RepositoryID, title string) ([]workflow.CoordinationItem, error) {
	var found []workflow.CoordinationItem
	for _, item := range b.parents {
		if item.Title == title {
			found = append(found, item)
		}
	}
	return found, nil
}

func (b *memoryBackend) CreateCoordinationItem(_ context.Context, _ workflow.RepositoryID, item workflow.CoordinationItem) (workflow.CoordinationItem, error) {
	item.Number = 100 + len(b.parents)
	b.parents = append(b.parents, item)
	return item, nil
}

func (b *memoryBackend) AddChild(_ context.Context, _ workflow.RepositoryID, parent, child int) error {
	if b.failChild == child {
		b.failChild = 0
		return errors.New("temporary parent relationship failure")
	}
	b.children = append(b.children, [2]int{parent, child})
	for index := range b.items {
		if b.items[index].Number == child {
			b.items[index].Parent = parent
		}
	}
	return nil
}

func (b *memoryBackend) AddDependency(_ context.Context, _ workflow.RepositoryID, dependent, blocker int) error {
	if b.failDependency == dependent {
		b.failDependency = 0
		return errors.New("temporary dependency relationship failure")
	}
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
	if len(backend.labels) != 7 {
		t.Fatalf("prepared %d labels", len(backend.labels))
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
			app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewBufferString("n\n"), &stdout, &stderr)
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
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

	if err := app.Run([]string{"skl", "setup", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md created without confirmation: %v", err)
	}
}

func TestDocumentedImplementResourceCommands(t *testing.T) {
	for _, file := range []string{"skills/dev/implement/SKILL.md", "README.md"} {
		t.Run(file, func(t *testing.T) {
			seen := map[string]bool{}
			for _, command := range strings.Split(readRepositoryFile(t, file), "`") {
				if !strings.HasPrefix(command, "skl skill ") || !strings.Contains(command, "--resource") {
					continue
				}
				args := strings.Fields(command)
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
				app := newApp(nil, bytes.NewReader(nil), &output, &output)
				if err := app.Run(args); err != nil {
					t.Errorf("%s: %v", command, err)
					continue
				}
				if output.String() != readRepositoryFile(t, "skills/dev/"+args[len(args)-1]+"/"+resource) {
					t.Errorf("%s returned the wrong resource", command)
				}
			}
			if !seen["reference/submission.md"] || !seen["reference/decision.md"] {
				t.Errorf("missing concrete template commands: %v", seen)
			}
		})
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
			command := "skl skill " + name
			if name == "implement" {
				command = "skl implement next"
			}
			if !strings.Contains(stub, command) || !strings.Contains(stub, "skl.stub/v1") {
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

	got, _, _ := strings.Cut(output.String(), "\n\n## Included Skill:")
	for section, requirements := range map[string][]string{
		"slice judgment":      {"COMPLETE path through every layer", "demoable and verifiable on its own", "single fresh context window", "Iterate until the user approves the breakdown"},
		"seam judgment":       {"Use the `design` skill", "Always prefer existing seams", "Use the highest seam possible", "Check with the user if the seams match their expectations"},
		"artifact authorship": {"## Writing the change artifacts", "`intent.md`: Why / What / Scope / Out of scope / Definition of Done", "*Gherkin notation*", "module shapes and seams chosen for implementation", "Discoveries belong in PR findings or in a new proposal", "./reference/tasks.md"},
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
			app := newAppWithSkillHome(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, t.TempDir())
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
	app := newApp(func() (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output)
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
	app = newApp(func() (setup.Backend, error) {
		return setup.NewGitHubBackend("https://api.github.test", "secret", client), nil
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
				backend.failChild = 2
			} else {
				backend.failDependency = 2
			}
			var output bytes.Buffer
			app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
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
			if output.String() != "completed\n" || len(backend.items) != 2 || !backend.items[0].Ready || !backend.items[1].Ready || len(backend.parents) != 1 || !slices.Equal(backend.children, [][2]int{{100, 1}, {100, 2}}) || !slices.Equal(backend.blocks, [][2]int{{2, 1}}) {
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
		"missing ledger":                          {"ledger is missing", "commit the complete ledger once at the published branch head"},
		"baseline not at head":                    {"Artifact Baseline is not the published branch head", "commit the complete ledger once at the published branch head"},
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
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

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
			app := newApp(func() (setup.Backend, error) {
				return setup.NewGitHubBackend("https://api.github.test", "secret", client), nil
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
			app := newApp(func() (setup.Backend, error) {
				return setup.NewGitHubBackend("https://api.github.test", "secret", client), nil
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
		parents: []workflow.CoordinationItem{{Number: 100, Title: "parent", Body: "parent\n"}},
		items: []workflow.WorkItem{
			{Number: 1, Title: "base", Body: "base\n", Parent: 100, Ready: true},
			{Number: 2, Title: "dependent", Body: "dependent\n", Parent: 100, Blockers: []int{99}, Ready: true},
		},
	}
	var output bytes.Buffer
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)

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
		{Number: 1, Title: "merged-clean", Branch: "merged-clean", Merged: true, AcceptedHead: accepted},
		{Number: 2, Title: "merged-dirty", Branch: "merged-dirty", Merged: true, AcceptedHead: accepted},
		{Number: 3, Title: "merged-unexpected", Branch: "merged-unexpected", Merged: true, AcceptedHead: accepted},
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
	app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
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
			app := newApp(func() (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
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
	if !slices.Equal(packet.Resources, []string{"reference/decision.md", "reference/submission.md"}) {
		t.Fatalf("implementation resources changed: %v", packet.Resources)
	}
	for _, format := range []string{"markdown", "json"} {
		t.Run(format, func(t *testing.T) {
			var direct, bundled bytes.Buffer
			for _, request := range []struct {
				name string
				out  *bytes.Buffer
			}{{"audit", &direct}, {"implement", &bundled}} {
				app := newApp(nil, bytes.NewReader(nil), request.out, request.out)
				if err := app.Run([]string{"skl", "skill", "--format", format, request.name}); err != nil {
					t.Fatal(err)
				}
			}
			audit, implementation := direct.String(), bundled.String()
			if format == "json" {
				var directPacket, bundledPacket skilldist.Packet
				if err := json.Unmarshal(direct.Bytes(), &directPacket); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(bundled.Bytes(), &bundledPacket); err != nil {
					t.Fatal(err)
				}
				audit, implementation = directPacket.Instructions, bundledPacket.Instructions
			} else {
				_, audit, _ = strings.Cut(audit, "\n\n")
				wantHeader := "Protocol: skl.instructions/v1\nSkill: implement\nIncluded skills: tdd, audit, design, domain\nFacts: {}\nResources: reference/decision.md, reference/submission.md\n\n"
				if bundled.String() != wantHeader+wantInstructions {
					t.Fatal("rendered implementation manifest or supporting definitions changed")
				}
			}
			_, includedAudit, found := strings.Cut(implementation, "\n\n## Included Skill: audit\n\n")
			includedAudit, _, _ = strings.Cut(includedAudit, "\n\n## Included Skill:")
			if !found || includedAudit != audit {
				t.Error("bundled Audit differs from direct retrieval")
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
