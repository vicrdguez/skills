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
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

// structuredStageCommand preserves the legacy typed-test seam for both stages:
// Watchdog is JSON by default, while Implement now opts in explicitly.
func structuredStageCommand(args ...string) []string {
	command := append([]string{"skl"}, args...)
	if len(args) > 0 && args[0] == "implement" {
		command = append(command, "--format", "json")
	}
	return command
}

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

// implementBundle renders the complete Implement Execution Skill through the
// active private-ledger path, so checks never depend on the retired
// forge-authoritative selection or on refused read-only retrieval.
func implementBundle(t *testing.T) skilldist.Packet {
	t.Helper()
	newLedgerFixture(t)
	forge := newForgeServer(t)
	source := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, forge)
	accepted := cli.accept(t, source, writeProposal(t, "", singleSlice("widget-proposal")))
	if accepted.Status != "accepted" {
		t.Fatalf("proposal acceptance = %q: %s", accepted.Status, mustJSON(t, accepted))
	}
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "implement", "next", "--repo", source, "--result-directory", t.TempDir(), "--format", "json"}); err != nil {
		t.Fatalf("implement next: %v\n%s", err, cli.out.String())
	}
	var got deliveryOutput
	if err := json.Unmarshal(cli.out.Bytes(), &got); err != nil {
		t.Fatalf("decode implement next %q: %v", cli.out.String(), err)
	}
	if got.Packet == nil {
		t.Fatalf("no Execution Skill: %#v", got)
	}
	return *got.Packet
}

// activeDeliveryPacket renders one private-ledger delivery packet through the
// same production presentation seam the CLI uses. Procedure-specific coverage
// uses it because the complete CLI lifecycle is already covered end to end in
// delivery_test.go.
func activeDeliveryPacket(t *testing.T, root, phase, operation string, state ledger.SliceState) skilldist.Packet {
	t.Helper()
	repository, err := setup.ResolveRepository(root, "")
	if err != nil {
		t.Fatal(err)
	}
	execution := &ledger.Execution{
		Project:    "widgets",
		Repository: "acme/widgets",
		Item:       "widget/foundation",
		State:      state,
		Claim:      ledger.Reference{Commit: strings.Repeat("c", 40), Path: "projects/widgets/proposals/widget/foundation/state.json"},
		Documents: []ledger.ContractDocument{{
			Commit:   strings.Repeat("1", 40),
			Path:     "projects/widgets/proposals/widget/foundation/behavior.md",
			Contents: "# Opaque accepted contract\n",
		}},
	}
	if phase == ledger.WatchdogPhase {
		execution.Implement = &ledger.Report{Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40)}}
		execution.Watchdog = &ledger.Report{Round: 1, Source: ledger.SourceRevisions{Reviewed: strings.Repeat("a", 40)}}
	}
	packet, err := setup.PresentDelivery(execution, repository, phase, operation, skilldist.UnknownCapability, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func TestDocumentedResourceCommands(t *testing.T) {
	cases := []struct {
		file  string
		skill string
		want  []string
	}{
		{file: "README.md", want: []string{"reference/ledger-submission.md", "reference/report-schema.md", "reference/ledger-review.md", "reference/DEEPENING.md"}},
		{file: "skills/dev/implement/SKILL.md", skill: "implement", want: []string{"reference/ledger-submission.md"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.file, func(t *testing.T) {
			source := readRepositoryFile(t, testCase.file)
			if testCase.skill == "implement" {
				source = implementBundle(t).Instructions
			} else if testCase.skill != "" {
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
					// The decision resource documents its genuine later value as a
					// choice; resolve it the way a worker would before running.
					args := shellArgs(t, strings.ReplaceAll(command, "preserve=<true|false>", "preserve=true"))
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
		"decision":           "skills/dev/decision/SKILL.md",
		"design":             "skills/dev/design/SKILL.md",
		"domain":             "skills/dev/domain/SKILL.md",
		"explore":            "skills/dev/explore/SKILL.md",
		"implement":          "skills/dev/implement/SKILL.md",
		"propose":            "skills/dev/propose/SKILL.md",
		"shape":              "skills/thinking/shape/SKILL.md",
		"testing":            "skills/dev/testing/SKILL.md",
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
			if name == "implement" || name == "watchdog" {
				command = "skl " + name + " next"
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
	bundle := implementBundle(t).Instructions
	for _, command := range []string{
		"skl skill --format json domain",
		"skl skill --format json explore",
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
		for _, skill := range []string{"audit", "brainstorm", "design", "domain", "explore", "implement", "propose", "shape", "testing", "watchdog", "writing-for-agents"} {
			directory := filepath.Join(home, harness, skill)
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" {
				t.Fatalf("%s must contain only a stub: %v (%v)", directory, entries, err)
			}
			stub := readFile(t, filepath.Join(directory, "SKILL.md"))
			_, body, found := strings.Cut(strings.TrimPrefix(stub, "---\n"), "\n---\n")
			command := "skl skill " + skill
			if skill == "implement" || skill == "watchdog" {
				command = "skl " + skill + " next"
			}
			want := "\n<!-- skl-owned: skl.stub/v1 -->\n\nRun `" + command + "`. Skip activation for every skill named in `included_skills`; its definition is already in the packet.\n"
			if skill == "watchdog" {
				want = strings.TrimSuffix(want, "\n") + " Start Watchdog in a fresh session, process one Work Item, and report the verified result in normal Markdown.\n"
			}
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
		{"testing", "", "See `skl skill --resource reference/tests.md testing` for examples", "skl skill --resource reference/tests.md testing", "# Behavioral and Regression Tests"},
		{"testing", "", "`skl skill --resource reference/mocking.md testing` for boundary substitutes", "skl skill --resource reference/mocking.md testing", "# Boundary Substitutes and Controlled Reproductions"},
		{"audit", "", "Retrieve `skl skill --resource reference/smells.md audit`", "skl skill --resource reference/smells.md audit", "# Smell Baseline"},
		{"audit", "", "`skl skill --resource reference/acceptance.md audit`. Supply both review", "skl skill --resource reference/acceptance.md audit", "# Contract Acceptance and Finding Criteria"},
		{"audit", "", "Report every documented-standard violation", "skl skill --resource reference/smells.md audit", "**The repo overrides.** A documented repo standard always wins"},
		{"design", "", "see `skl skill --resource reference/DEEPENING.md design`: dependency categories", "skl skill --resource reference/DEEPENING.md design", "### 2. Local-substitutable"},
		{"design", "", "see `skl skill --resource reference/DESIGN-IT-TWICE.md design`: spin up parallel sub-agents", "skl skill --resource reference/DESIGN-IT-TWICE.md design", "Each must produce a **radically different** interface"},
		{"design", "reference/DEEPENING.md", "the Design definition (retrieve with `skl skill design` if not already supplied)", "skl skill design", "# Codebase Design"},
		{"design", "reference/DESIGN-IT-TWICE.md", "Uses the vocabulary in the Design definition (retrieve with `skl skill design` if not already supplied)", "skl skill design", "**The interface is the test surface.**"},
		{"design", "reference/DESIGN-IT-TWICE.md", "which category they fall into (see `skl skill --resource reference/DEEPENING.md design`)", "skl skill --resource reference/DEEPENING.md design", "## Dependency categories"},
		{"design", "reference/DESIGN-IT-TWICE.md", "dependency category from `skl skill --resource reference/DEEPENING.md design`, what sits behind the seam", "skl skill --resource reference/DEEPENING.md design", "### 4. True external (Mock)"},
		{"design", "reference/DESIGN-IT-TWICE.md", "Include both the Design definition's vocabulary (retrieve with `skl skill design` if not already supplied) and `CONTEXT.md` vocabulary in the brief", "skl skill design", "**Locality**"},
		{"design", "reference/DESIGN-IT-TWICE.md", "Dependency strategy and adapters (see `skl skill --resource reference/DEEPENING.md design`)", "skl skill --resource reference/DEEPENING.md design", "## Testing strategy: replace, don't layer"},
		{"propose", "", "Follow the template from `skl skill --resource reference/intent.md propose`", "skl skill --resource reference/intent.md propose", "## Definition of Done"},
		{"propose", "", "Follow the template from `skl skill --resource reference/behavior.md propose`", "skl skill --resource reference/behavior.md propose", "## Rule: Cancellation is available only before shipment"},
		{"propose", "", "Follow the template from `skl skill --resource reference/plan.md propose`", "skl skill --resource reference/plan.md propose", "### Module shapes & seams"},
		{"propose", "", "Follow the template from `skl skill --resource reference/tasks.md propose`", "skl skill --resource reference/tasks.md propose", "exists only when useful sequencing"},
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
				if slices.Contains([]string{"testing", "audit", "design", "domain"}, tc.skill) {
					skills = append(skills, "implement")
				}
				for _, skill := range skills {
					if skill == "implement" {
						_, instructions, _ := strings.Cut(bundle, "\n\n## Included Skill: "+tc.skill+"\n\n")
						if !strings.Contains(instructions, tc.pointer) {
							t.Errorf("bundled %s instructions lack %q", tc.skill, tc.pointer)
						}
						continue
					}
					for _, format := range []string{"markdown", "json"} {
						got := run(t, "skl skill --format "+format+" "+skill)
						if format == "json" {
							var packet skilldist.Packet
							if err := json.Unmarshal([]byte(got), &packet); err != nil {
								t.Fatal(err)
							}
							if skill == "implement" && !slices.Equal(packet.IncludedSkills, []string{"testing", "audit", "design", "domain"}) {
								t.Fatalf("included skills = %v", packet.IncludedSkills)
							}
							got = packet.Instructions
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

			testing := filepath.Join(root, harness, "testing/SKILL.md")
			wantTesting := readFile(t, testing)
			if err := os.WriteFile(testing, []byte("---\nname: testing\n---\n\n<!-- skl-owned: skl.stub/v1 -->\nstale"), 0o644); err != nil {
				t.Fatal(err)
			}
			audit := filepath.Join(root, harness, "audit/SKILL.md")
			if err := os.WriteFile(audit, []byte("---\nname: audit\n---\n\n<!-- skl-owned: skl.stub/v2 -->\nmy unrelated skill"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}

			if got := readFile(t, testing); got != wantTesting {
				t.Fatalf("owned stub was not refreshed:\n%s", got)
			}
			if got := readFile(t, audit); got != "---\nname: audit\n---\n\n<!-- skl-owned: skl.stub/v2 -->\nmy unrelated skill" {
				t.Fatalf("unrelated file changed: %q", got)
			}
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, testing); got != wantTesting {
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

func TestInstallUpgradesOwnedTDDStubsWithoutTouchingUserContent(t *testing.T) {
	for _, harness := range []string{".pi/agent/skills", ".codex/skills", ".claude/skills", ".config/opencode/skills"} {
		t.Run(harness, func(t *testing.T) {
			root := t.TempDir()
			legacyDirectory := filepath.Join(root, harness, "tdd")
			if err := os.MkdirAll(legacyDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(legacyDirectory, "SKILL.md")
			if err := os.WriteFile(legacy, []byte("---\nname: tdd\n---\n\n<!-- skl-owned: skl.stub/v1 -->\nstale\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			extra := filepath.Join(legacyDirectory, "notes.md")
			if err := os.WriteFile(extra, []byte("keep legacy notes\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			userOwned := filepath.Join(root, harness, "my-tdd", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(userOwned), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(userOwned, []byte("my unrelated test skill\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			var output bytes.Buffer
			app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
			for range 2 {
				if err := app.Run([]string{"skl", "install"}); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(legacy); !os.IsNotExist(err) {
					t.Fatalf("owned legacy tdd stub still exists: %v", err)
				}
				if got := readFile(t, extra); got != "keep legacy notes\n" {
					t.Fatalf("legacy sibling changed: %q", got)
				}
				if got := readFile(t, userOwned); got != "my unrelated test skill\n" {
					t.Fatalf("user-owned skill changed: %q", got)
				}
				testing := readFile(t, filepath.Join(root, harness, "testing", "SKILL.md"))
				if !strings.Contains(testing, "skl skill testing") || !strings.Contains(testing, "skl.stub/v1") {
					t.Fatalf("testing stub is not current and owned:\n%s", testing)
				}
			}
		})
	}

	t.Run("user-owned tdd", func(t *testing.T) {
		root := t.TempDir()
		legacy := filepath.Join(root, ".codex/skills/tdd/SKILL.md")
		if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
			t.Fatal(err)
		}
		const userSkill = "---\nname: tdd\n---\nmy own tdd skill\n"
		if err := os.WriteFile(legacy, []byte(userSkill), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output, root)
		for range 2 {
			if err := app.Run([]string{"skl", "install"}); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, legacy); got != userSkill {
				t.Fatalf("user-owned tdd changed: %q", got)
			}
		}
	})
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

func TestTestingIsTheCanonicalContractGroundedPolicy(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("reasoning-skill retrieval opened a Workflow Backend")
		return nil, nil
	}, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{
		"Skill: testing",
		"Resources: reference/mocking.md, reference/tests.md",
		"observable behavior",
		"independent expectation",
		"before or after the fix",
		"faithful isolated reproduction",
		"material limitations",
		"Required behavioral and failure-mode protection matters more than test inventory",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("testing policy is missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"Red before green",
		"One scenario → one test",
		"Refactoring is not part of the loop",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("testing policy still mandates %q:\n%s", forbidden, got)
		}
	}

	stdout.Reset()
	if err := app.Run([]string{"skl", "skill", "tdd"}); err == nil || !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("retired tdd retrieval error = %v, output = %q", err, stdout.String())
	}
}

func TestRetrieveRenderedSkillInstructions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}

	header := "Protocol: skl.instructions/v1\nSkill: testing\nIncluded skills: none\nFacts: {}\nResources: reference/mocking.md, reference/tests.md\n\n"
	if got, want := stdout.String(), readRepositoryFile(t, "cmd/skl/testdata/testing-standalone.golden.md"); got != want {
		t.Fatalf("rendered Testing packet differs from the independently reviewed golden:\n%s", got)
	}
	instructions := strings.TrimPrefix(stdout.String(), header)
	// A standalone retrieval keeps its independent decision path rather than
	// referring to an Implement-only human-decision command.
	if !strings.Contains(instructions, "Clarify a consequential unresolved behavioral or architectural choice with the user") || strings.Contains(instructions, "execution's human-decision path") {
		t.Fatalf("standalone retrieval lost its own mode:\n%s", instructions)
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
		"slice judgment": {"COMPLETE path through every layer", "demoable and verifiable on its own", "single fresh context window", "Iterate until the user approves the breakdown"},
		"seam judgment": {
			"Use the `design` skill to materialize the approved seams", "Preserve any deliberately agreed seam",
			"Where seam choice was delegated, prefer an existing seam", "return it to the human before drafting rather than redesigning the accepted architecture",
		},
		"artifact authorship": {
			"## Writing the change artifacts", "desired result, scope, exclusions, Definition of Done", "scoped named rules", "binding scenarios that discriminate plausible interpretations",
			"responsibility ownership, boundary assumptions, deliberately agreed interfaces, and verification strategy", "Do not manufacture tasks from scenario or test counts",
			"readable, uniquely referenceable descriptive headings without requiring IDs", "Discoveries belong in PR findings or in a new proposal",
			"skl skill --resource reference/tasks.md propose",
		},
		"fidelity review": {
			"user-confirmed final recap of consequential rules, architectural commitments, and delegated choices",
			"one bounded review in a fresh context across the complete proposed slice set",
			"the exact user-confirmed Explore recap", "every referenced decision or ADR", "all proposed slices and their artifact drafts",
			"omits, weakens, strengthens, contradicts, or invents obligations", "does not redesign the solution",
			"Correct demonstrable transcription errors", "returns to the human for resolution",
			"neither routine artifact-by-artifact rereading nor a second semantic approval ceremony",
		},
		"ledger intake": {"skl ledger accept", "skl ledger show", "No source branch, worktree, or source artifact commit is prepared", "skl ledger publish --repo <root> --proposal <proposal>", "rather than repeating acceptance", "Never publish with `gh` directly", "human-directed administrative cutover with normal workers stopped", "renewed proposal"},
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

func TestRetrieveApprovedContractGuidance(t *testing.T) {
	run := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) {
			t.Fatal("public guidance retrieval reached the Workflow Backend")
			return nil, nil
		}, bytes.NewReader(nil), &stdout, &stderr)
		if err := app.Run(args); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v stderr: %s", args, stderr.String())
		}
		return stdout.String()
	}

	for _, tc := range []struct {
		name      string
		included  []string
		resources []string
		markers   []string
	}{
		{
			name: "explore", included: []string{"domain"},
			markers: []string{
				"one final approval recap", "consequential rules", "architectural commitments and responsibility ownership",
				"choices deliberately delegated to implementation", "confirmation is the semantic approval for the whole Proposal",
				"do not require the user to reread or separately reapprove each one",
			},
		},
		{
			name: "propose", included: []string{"design", "testing"},
			resources: []string{"reference/behavior.md", "reference/intent.md", "reference/issue-publication.md", "reference/plan.md", "reference/tasks.md"},
			markers: []string{
				"one bounded review in a fresh context across the complete proposed slice set", "the exact user-confirmed Explore recap",
				"every referenced decision or ADR", "omits, weakens, strengthens, contradicts, or invents obligations",
				"Correct demonstrable transcription errors", "returns to the human for resolution",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			markdown := run("skl", "skill", tc.name)
			typed := run("skl", "skill", "--format", "json", tc.name)
			var packet skilldist.Packet
			if err := json.Unmarshal([]byte(typed), &packet); err != nil {
				t.Fatalf("typed retrieval is not JSON: %v\n%s", err, typed)
			}
			if markdown != packet.Markdown() {
				t.Fatalf("Markdown and explicit JSON differ for %s", tc.name)
			}
			if !slices.Equal(packet.IncludedSkills, tc.included) || !slices.Equal(packet.Resources, tc.resources) {
				t.Fatalf("%s manifest = included %v resources %v", tc.name, packet.IncludedSkills, packet.Resources)
			}
			primary, _, _ := strings.Cut(packet.Instructions, "\n\n## Included Skill:")
			for _, marker := range tc.markers {
				if !strings.Contains(primary, marker) {
					t.Errorf("%s guidance lacks %q", tc.name, marker)
				}
			}
			for _, included := range tc.included {
				marker := "## Included Skill: " + included
				if count := strings.Count(packet.Instructions, marker); count != 1 {
					t.Errorf("%s included marker %q appears %d times", tc.name, marker, count)
				}
			}
		})
	}

	templates := map[string][]string{
		"reference/intent.md": {
			"rather than repeating them here", "Rules and scenarios may map many-to-many", "Human-owned checks",
		},
		"reference/behavior.md": {
			"authoritative, scoped named rules", "governs its class of situations beyond the scenarios", "discriminate plausible interpretations",
			"one test or task per scenario", "precision aids, not compulsory headings", "readable, uniquely referenceable descriptive heading",
			"IDs are not required", "unambiguously implied",
		},
		"reference/plan.md": {
			"when the approved design pins architecture", "Responsibility ownership", "boundary", "Illustrative", "Verification strategy", "grouped many-to-many",
		},
		"reference/tasks.md": {
			"optional coordination ledger", "Scenario count, test count", "coherent implementation outcome", "stable IDs are optional",
		},
	}
	for resource, markers := range templates {
		t.Run(resource, func(t *testing.T) {
			rendered := run("skl", "skill", "--resource", resource, "propose")
			for _, marker := range markers {
				if !strings.Contains(rendered, marker) {
					t.Errorf("%s lacks %q", resource, marker)
				}
			}
			for _, forbidden := range []string{"becomes one test", "One behavior per Scenario", "EVERY Gherkin scenario", "Stable ids (B1"} {
				if strings.Contains(rendered, forbidden) {
					t.Errorf("%s retains cardinality requirement %q", resource, forbidden)
				}
			}
		})
	}
}

func TestRetrieveRetiredLedgerInstructions(t *testing.T) {
	for skill, required := range map[string][]string{
		"implement": {
			"Never amend, tick, or retire the accepted documents",
			"Never create, tick, or delete `.changes`",
			"never rewrite the accepted Contract documents",
			"Independent Watchdog Review and the human merge boundary are preserved",
			"only a human merges the reviewed work",
		},
		"watchdog": {
			"never look for the accepted Contract in source history or tick, retire, or recreate it",
			"Only a human performs the final integration and merge",
		},
	} {
		t.Run(skill, func(t *testing.T) {
			var got string
			switch skill {
			case "implement":
				got = implementBundle(t).Instructions
			case "watchdog":
				got = activeDeliveryPacket(t, sourceRepository(t, "acme", "widgets"), ledger.WatchdogPhase, "next", ledger.SliceState{
					State: ledger.AwaitingReview, Title: "Foundation", Branch: "foundation",
				}).Instructions
			}
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

func TestProposePacketGuidesLedgerIntake(t *testing.T) {
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "skill", "propose"}); err != nil {
		t.Fatal(err)
	}
	packet := output.String()
	for _, want := range []string{
		"skl ledger accept", "skl ledger show", "proposal.json", "proposal.md",
		"planned source branch", "temporary Markdown files",
		"transport, not Contract content",
	} {
		if !strings.Contains(packet, want) {
			t.Fatalf("packet lacks %q", want)
		}
	}
	for _, forbidden := range []string{
		"thin-pointer template",
		"git rev-parse HEAD",
		"[baseline]",
		"skl propose publish",
		"skl propose cleanup",
		"create `.worktrees/",
		"worktree add",
		"skl implement cleanup",
	} {
		if strings.Contains(packet, forbidden) {
			t.Fatalf("packet retains the retired source-preparation guidance %q", forbidden)
		}
	}
	// Labels are authored locally; no CLI numbering service or mandatory tasks
	// file exists.
	for _, want := range []string{"B<n>", "A<n>", "T<n>", "M<n>"} {
		if !strings.Contains(packet, want) {
			t.Fatalf("packet lacks the label guidance %q", want)
		}
	}
	if !strings.Contains(packet, "no CLI numbering service") || !strings.Contains(packet, "only when warranted") {
		t.Fatal("packet turns labels into a numbering service or tasks into a mandate")
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
	wantResources := []string{"reference/mocking.md", "reference/tests.md"}
	wantHeader := "Protocol: skl.instructions/v1\nSkill: testing\nIncluded skills: none\nFacts: {}\nResources: reference/mocking.md, reference/tests.md\n\n"
	var markdown bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &markdown, &markdown, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	app = newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())
	if err := app.Run([]string{"skl", "skill", "--format", "json", "testing"}); err != nil {
		t.Fatal(err)
	}
	var packet skilldist.Packet
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatalf("stdout is not a JSON packet: %v\n%s", err, stdout.String())
	}
	if packet.Protocol != "skl.instructions/v1" || packet.Skill != "testing" || packet.Facts != (skilldist.InvocationFacts{}) || len(packet.IncludedSkills) != 0 || !slices.Equal(packet.Resources, wantResources) || packet.Instructions == "" || markdown.String() != wantHeader+packet.Instructions {
		t.Fatalf("JSON and Markdown packets differ: %#v", packet)
	}
	if want := strings.TrimPrefix(readRepositoryFile(t, "cmd/skl/testdata/testing-standalone.golden.md"), wantHeader); packet.Instructions != want {
		t.Fatalf("JSON Testing instructions differ from the independently reviewed golden:\n%s", packet.Instructions)
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
				if len(packet.IncludedSkills) != 0 || !slices.Equal(packet.Resources, []string{"reference/acceptance.md", "reference/smells.md"}) {
					t.Fatalf("unexpected Audit dependencies: %#v", packet)
				}
				instructions = packet.Instructions
			} else {
				header := "Protocol: skl.instructions/v1\nSkill: audit\nIncluded skills: none\nFacts: {}\nResources: reference/acceptance.md, reference/smells.md\n\n"
				if !strings.HasPrefix(instructions, header) {
					t.Fatalf("unexpected Audit manifest: %s", instructions)
				}
				instructions = strings.TrimPrefix(instructions, header)
			}
			header := "Protocol: skl.instructions/v1\nSkill: audit\nIncluded skills: none\nFacts: {}\nResources: reference/acceptance.md, reference/smells.md\n\n"
			if want := strings.TrimPrefix(readRepositoryFile(t, "cmd/skl/testdata/audit-standalone.golden.md"), header); instructions != want {
				t.Fatalf("rendered Audit instructions differ from the independently reviewed golden:\n%s", instructions)
			}
			// A standalone Audit keeps its own caller-driven discovery.
			if !strings.Contains(instructions, "Ask if no comparison was supplied") || strings.Contains(instructions, "This bundled Audit reviews one claimed change") {
				t.Error("standalone Audit lost its own mode")
			}
			for _, forbidden := range []string{"ponytail", "750", "net-lines", "## Simplicity"} {
				if strings.Contains(strings.ToLower(instructions), strings.ToLower(forbidden)) {
					t.Errorf("Audit retains or injects %q", forbidden)
				}
			}
			for _, required := range []string{"**Standards**", "**Contracts**", "reference/smells.md", "**Full Gate**", "exact frozen Contract references", "`HARD` or `JUDGEMENT`", "Keep the report under 500 words", "## Aggregate without reranking", "Do not merge findings across axes"} {
				if !strings.Contains(strings.Join(strings.Fields(instructions), " "), required) {
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
				"pr=11",
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
			inputs: []string{"result_directory=" + directory, "pr=11", "round=1", "round=1", "reviewed_head=" + head},
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
			inputs: []string{"result_directory=" + directory, "pr=11", "round=two", "reviewed_head=" + head},
			wants:  []string{"round", "integer"},
		},
		{
			name: "unsupported choice", owner: "implement", resource: "reference/submission.md",
			inputs: []string{"result_directory=" + directory, "procedure=later"},
			wants:  []string{"procedure", "initial", "rework"},
		},
		{
			name: "zero round", owner: "watchdog", resource: "reference/review.md",
			inputs: []string{"result_directory=" + directory, "pr=11", "round=0", "reviewed_head=" + head},
			wants:  []string{"round", "positive"},
		},
		{
			name: "missing PR", owner: "watchdog", resource: "reference/review.md",
			inputs: []string{"result_directory=" + directory, "round=1", "reviewed_head=" + head},
			wants:  []string{"pr", "required"},
		},
		{
			name: "zero PR", owner: "watchdog", resource: "reference/review.md",
			inputs: []string{"result_directory=" + directory, "pr=0", "round=1", "reviewed_head=" + head},
			wants:  []string{"pr", "positive"},
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
			inputs: []string{"result_directory=" + directory, "pr=11", "round=1", "reviewed_head=" + head[:12] + "nonsense"},
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
	t.Setenv("TMPDIR", hostile)
	root := sourceRepository(t, "acme", "widgets")

	cases := []struct {
		name      string
		phase     string
		operation string
		state     ledger.SliceState
		owner     string
		resource  string
		procedure string
		want      []string
		absent    []string
	}{
		{
			name: "first implementation", phase: ledger.ImplementPhase, operation: "next", owner: "implement",
			state:    ledger.SliceState{State: ledger.ReadyForImplementation, Title: "Foundation", Branch: "foundation"},
			resource: "reference/ledger-submission.md", procedure: "initial",
			want:   []string{"## Summary", "## Verification", "## Audit ledger", "Full Gate", "completion-and-evidence table"},
			absent: []string{"Preserve every historical"},
		},
		{
			// The engine's operation and reconciled Workflow State select the
			// procedure; the bound report resource receives it as typed input.
			name: "resumed implementation", phase: ledger.ImplementPhase, operation: "resume", owner: "implement",
			state:    ledger.SliceState{State: ledger.ReadyForImplementation, Title: "Foundation", Branch: "foundation"},
			resource: "reference/ledger-submission.md", procedure: "resumed",
			want:   []string{"Identify what was already complete and what this continuation added", "## Summary", "## Audit ledger"},
			absent: []string{"Preserve every historical"},
		},
		{
			name: "finding-driven rework", phase: ledger.ImplementPhase, operation: "next", owner: "implement",
			state:    ledger.SliceState{State: ledger.Rework, Title: "Foundation", Branch: "foundation"},
			resource: "reference/ledger-submission.md", procedure: "rework",
			want:   []string{"Preserve every historical `F<n>` and `W<n>` identity", "## Audit ledger", "## Summary"},
			absent: []string{"Identify what was already complete"},
		},
		{
			name: "watchdog invocation", phase: ledger.WatchdogPhase, operation: "next", owner: "watchdog",
			state:    ledger.SliceState{State: ledger.AwaitingReview, Title: "Foundation", Branch: "foundation"},
			resource: "reference/ledger-review.md",
			want: []string{
				"review round 2", "stable Work-Item-local `W<n>` identities",
				"`BLOCK`, `HUMAN`, or `NOTE`", "human-owned `M<n>` Manual Verification",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			packet := activeDeliveryPacket(t, root, testCase.phase, testCase.operation, testCase.state)
			facts := packet.Facts.Delivery
			if facts == nil {
				t.Fatalf("no delivery facts: %#v", packet.Facts)
			}
			if !strings.HasPrefix(facts.ResultDirectory, hostile) {
				t.Fatalf("result directory %q is not the invocation's own private directory", facts.ResultDirectory)
			}
			for _, progressive := range []string{"# Implementation report result document", "# Watchdog review report"} {
				if strings.Contains(packet.Instructions, progressive) {
					t.Errorf("parent instructions disclosed deferred resource content %q", progressive)
				}
			}

			command := deferredCommand(t, packet.Instructions, testCase.resource)
			if !strings.Contains(command, "--input result_directory=") || !strings.Contains(command, "result_directory="+skilldist.ShellQuote(facts.ResultDirectory)) {
				t.Errorf("parent command does not bind the private directory: %s", command)
			}
			if testCase.procedure != "" && !strings.Contains(command, "--input procedure="+testCase.procedure) {
				t.Errorf("parent command does not bind procedure %q: %s", testCase.procedure, command)
			}
			if !strings.Contains(packet.Instructions, "`"+facts.PauseCommand+"`") {
				t.Errorf("packet lost the engine-bound human-decision path %q", facts.PauseCommand)
			}
			if testCase.phase == ledger.WatchdogPhase {
				for _, want := range []string{
					"result_directory=" + skilldist.ShellQuote(facts.ResultDirectory),
					"--input round=" + strconv.FormatUint(facts.ReviewNumber, 10),
					"reviewed_head=" + skilldist.ShellQuote(facts.RequiredHead),
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
			}

			args := shellArgs(t, command)
			if len(args) == 0 || args[0] != "skl" || args[len(args)-1] != testCase.owner {
				t.Fatalf("parent command is not runnable verbatim: %v", args)
			}
			instructions := runDeferredCommand(t, command, "", "")
			for _, want := range testCase.want {
				if !strings.Contains(instructions, want) {
					t.Errorf("retrieved resource is missing %q:\n%s", want, instructions)
				}
			}
			for _, unwanted := range testCase.absent {
				if strings.Contains(instructions, unwanted) {
					t.Errorf("retrieved resource includes %q:\n%s", unwanted, instructions)
				}
			}
			if !strings.Contains(instructions, "`"+facts.ResultDirectory+"/") {
				t.Errorf("retrieved resource did not bind the invocation's private directory:\n%s", instructions)
			}
		})
	}
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
		{"audit", "reference/acceptance.md", []string{"# Contract Acceptance and Finding Criteria", "specific evidence gap"}},
		{"audit", "reference/smells.md", []string{"# Smell Baseline", "Feature Envy"}},
		{"design", "reference/DEEPENING.md", []string{"# Deepening", "Adapters"}},
		{"design", "reference/DESIGN-IT-TWICE.md", []string{"design constraint", "sub-agent"}},
		{"domain", "reference/ADR-FORMAT.md", []string{"# ADR Format", "Consequences"}},
		{"domain", "reference/CONTEXT-FORMAT.md", []string{"# {Context Name}", "## Contexts"}},
		{"propose", "reference/intent.md", []string{"Definition of Done", "## Out of Scope"}},
		{"propose", "reference/behavior.md", []string{"Gherkin", "Scenario"}},
		{"propose", "reference/plan.md", []string{"Module shapes", "## Approach"}},
		{"propose", "reference/tasks.md", []string{"Behavioral", "## Docs"}},
		{"testing", "reference/mocking.md", []string{"# Boundary Substitutes and Controlled Reproductions", "faithful"}},
		{"testing", "reference/tests.md", []string{"# Behavioral and Regression Tests", "independent expectation"}},
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
			for _, other := range []string{"audit", "implement", "testing", "watchdog"} {
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
	for _, owned := range []string{"reference/acceptance.md audit", "reference/smells.md audit", "reference/tests.md testing", "reference/DEEPENING.md design", "reference/CONTEXT-FORMAT.md domain"} {
		if !strings.Contains(packet.Instructions, owned) {
			t.Errorf("bundled definition lost its owning reference %q", owned)
		}
	}
	for _, stolen := range []string{"reference/acceptance.md implement", "reference/smells.md implement", "reference/tests.md implement", "reference/acceptance.md, ", "reference/smells.md, ", "reference/tests.md, "} {
		if strings.Contains(packet.Instructions+strings.Join(packet.Resources, ", "), stolen) {
			t.Errorf("bundled instructions took over %q", stolen)
		}
	}

	var markdown, jsonPacket bytes.Buffer
	if err := newApp(nil, bytes.NewReader(nil), &markdown, &markdown).Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(markdown.String(), "Protocol: skl.instructions/v1\nSkill: testing\nIncluded skills: none\nFacts: {}\nResources: reference/mocking.md, reference/tests.md\n\n") {
		t.Errorf("default Markdown rendering changed:\n%s", markdown.String())
	}
	if err := newApp(nil, bytes.NewReader(nil), &jsonPacket, &jsonPacket).Run([]string{"skl", "skill", "--format", "json", "testing"}); err != nil {
		t.Fatal(err)
	}
	var decoded skilldist.Packet
	if err := json.Unmarshal(jsonPacket.Bytes(), &decoded); err != nil || decoded.Skill != "testing" {
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

	packet := implementBundle(t)
	for _, resource := range packet.Resources {
		if strings.HasPrefix(resource, "modules/") {
			t.Errorf("private module %s is listed as a public resource", resource)
		}
	}

	// The embedded module composes into every Implement result document,
	// including the active ledger-submission resource, so the engine never
	// reads worker prose as a second decision.
	for resource, inputs := range map[string][]string{
		"reference/submission.md":        {"result_directory=" + t.TempDir(), "procedure=initial"},
		"reference/ledger-submission.md": {"result_directory=" + t.TempDir(), "procedure=initial"},
		"reference/decision.md":          {"result_directory=" + t.TempDir(), "preserve=false"},
	} {
		if instructions := renderResource(t, "implement", resource, inputs...); !strings.Contains(instructions, "never parses, judges, or cross-checks the prose") {
			t.Errorf("%s did not compose the shared Result Document module:\n%s", resource, instructions)
		}
	}
}

func TestRetrieveOneNamedResource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &stdout, &stderr, t.TempDir())

	if err := app.Run([]string{"skl", "skill", "--resource", "reference/tests.md", "testing"}); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), readRepositoryFile(t, "skills/dev/testing/reference/tests.md"); got != want {
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
				"procedure (string, required, one of: initial, resumed, rework): Which submission procedure to render.",
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
			inputs: []string{"result_directory=/tmp/result", "pr=11", "round=2", "reviewed_head=" + strings.Repeat("a", 40)},
			described: []string{
				"result_directory (string, required): Absolute path of the private Result Document directory this invocation created.",
				"pr (integer, required): Selected Submission PR number.",
				"round (integer, required): Review round number for this Submission.",
				"reviewed_head (string, required): Original full SHA of the reviewed head.",
			},
			procedural: "# Review Result Documents",
		},
		{
			owner: "testing", resource: "reference/tests.md",
			described:  []string{"reference/tests.md accepts no inputs."},
			procedural: "# Behavioral and Regression Tests",
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
	packet := implementBundle(t)
	root := t.TempDir()
	want := []string{"testing", "audit", "design", "domain"}
	if !slices.Equal(packet.IncludedSkills, want) {
		t.Fatalf("included_skills = %v, want %v", packet.IncludedSkills, want)
	}
	if !slices.Equal(packet.Resources, []string{"reference/decision.md", "reference/ledger-submission.md", "reference/pull-presentation.md", "reference/report-schema.md", "reference/submission.md"}) {
		t.Fatalf("implementation resources changed: %v", packet.Resources)
	}
	// Definitions are authored templates, so the rendered outcome is what a
	// worker reads: every guaranteed definition appears exactly once and the
	// deferred resource bodies stay out of a no-facts packet.
	for _, marker := range []struct{ name, text string }{
		{"implement", "## Start the accepted change"},
		{"implement", "## Audit once"},
		{"testing", "## Verify observable behavior"},
		{"audit", "## Aggregate without reranking"},
		{"design", "## Deep vs shallow"},
		{"domain", "### Offer ADRs sparingly"},
	} {
		if count := strings.Count(packet.Instructions, marker.text); count != 1 {
			t.Errorf("%s marker %q appears %d times, want once", marker.name, marker.text, count)
		}
	}
	for _, deferred := range []string{"# Submission Result Document", "# Decision Result Document", "# Implementation report result document"} {
		if strings.Contains(packet.Instructions, deferred) {
			t.Errorf("no-facts Implement packet disclosed the deferred %q body", deferred)
		}
	}
	// Every supporting definition preserves the accepted-change scope when
	// specialized for an active private-ledger delivery.
	bundled := map[string][]string{
		"testing": {"After preparation, read the exact frozen Contract"},
		"audit":   {"That normal PR-base merge-base is the default fixed point"},
		"design":  {"does not require a new design exercise"},
		"domain":  {"outside the accepted change"},
	}
	for _, included := range want {
		section := bundledSection(t, packet.Instructions, included)
		for _, marker := range bundled[included] {
			if !strings.Contains(section, marker) {
				t.Errorf("bundled %s lacks the active reference %q:\n%s", included, marker, section)
			}
		}
		for _, forbidden := range []string{"ponytail", "## simplicity"} {
			if strings.Contains(strings.ToLower(section), forbidden) {
				t.Errorf("bundled %s retains or injects %q", included, forbidden)
			}
		}
	}
	// The authored definition stays retrievable on its own for an independent
	// caller, which keeps the standalone branch instead.
	var standalone bytes.Buffer
	if err := newApp(nil, bytes.NewReader(nil), &standalone, &standalone).Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(standalone.String(), "Clarify a consequential unresolved behavioral or architectural choice with the user") || strings.Contains(standalone.String(), "execution's human-decision path") {
		t.Errorf("standalone Testing lost its own mode:\n%s", standalone.String())
	}

	var installOutput bytes.Buffer
	install := newAppWithSkillHome(func(github.RepositoryID) (setup.Backend, error) { return &memoryBackend{}, nil }, bytes.NewReader(nil), &installOutput, &installOutput, root)
	if err := install.Run([]string{"skl", "install"}); err != nil {
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
	override := filepath.Join(repository, "skills/dev/testing/SKILL.md")
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
	if err := app.Run([]string{"skl", "skill", "testing"}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !strings.Contains(got, "# Contract-Grounded Testing") || strings.Contains(got, "consumer override") {
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
