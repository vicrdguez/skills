package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

// TestMain isolates every test in this package from any real machine
// configuration: the legacy flow stays the default until a test installs
// its own ledger configuration with t.Setenv.
func TestMain(m *testing.M) {
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		temp, err := os.MkdirTemp("", "skl-test-config")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer os.RemoveAll(temp)
		os.Setenv("XDG_CONFIG_HOME", temp)
	}
	os.Exit(m.Run())
}

// ledgerFixture is one complete ledger environment: a bare upstream, its
// local ledger clone, and machine configuration selecting that clone.
type ledgerFixture struct {
	upstream string
	clone    string
	config   string // the XDG config root
	home     string // a HOME with its own .config for selection tests
}

// newLedgerFixture creates the clone and points XDG_CONFIG_HOME at a
// configuration selecting it.
func newLedgerFixture(t *testing.T) *ledgerFixture {
	t.Helper()
	upstream := t.TempDir()
	runGit(t, upstream, "init", "-q", "-b", "main", "--bare")
	clone := filepath.Join(t.TempDir(), "ledger")
	runGit(t, t.TempDir(), "clone", "-q", upstream, clone)
	runGit(t, clone, "config", "user.name", "Ledger")
	runGit(t, clone, "config", "user.email", "ledger@example.com")
	writeFile(t, filepath.Join(clone, "README.md"), "workflow ledger\n")
	runGit(t, clone, "add", "README.md")
	runGit(t, clone, "commit", "-q", "-m", "seed")
	runGit(t, clone, "push", "-q", "-u", "origin", "main")
	fixture := &ledgerFixture{upstream: upstream, clone: clone, config: t.TempDir(), home: t.TempDir()}
	fixture.selectWithXDG(t)
	return fixture
}

func (f *ledgerFixture) selectWithXDG(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", f.config)
	t.Setenv("HOME", f.home)
	writeFile(t, filepath.Join(f.config, "skl", "config.json"), `{"ledger": "`+f.clone+`"}`+"\n")
}

// selectWithHome moves the same selection to the home configuration and
// unsets XDG_CONFIG_HOME.
func (f *ledgerFixture) selectWithHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", f.home)
	writeFile(t, filepath.Join(f.home, ".config", "skl", "config.json"), `{"ledger": "`+f.clone+`"}`+"\n")
}

// misconfigure writes an arbitrary config body at the current location.
func (f *ledgerFixture) misconfigure(t *testing.T, body string) {
	t.Helper()
	writeFile(t, filepath.Join(f.config, "skl", "config.json"), body)
}

// ledgerSnapshot captures the observable Git state of a repository.
func ledgerSnapshot(t *testing.T, root string) string {
	t.Helper()
	refs := runGitOutput(t, root, "for-each-ref")
	status := runGitOutput(t, root, "status", "--porcelain", "--untracked-files=all")
	return refs + "\n--\n" + status
}

// forgeServer is a controllable forge for the ledger tests. It records the
// bodies it received and can fail selected surfaces.
type forgeServer struct {
	server *httptest.Server
	mu     sync.Mutex
	issues []map[string]any // created issues in order
	list   []map[string]any // listed open issues
	fail   func(method, path string) int
	// drop simulates an unknown-outcome transport failure: the server acts
	// but the caller receives a broken connection.
	drop          func(method, path string) bool
	truncate      func(method, path string) bool
	before        func(method, path string)
	outsideBefore func(method, path string)
	children      map[int][]int
	bodies        []string
	updates       []int // issue numbers updated in order
}

func (f *forgeServer) handler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		f.mu.Lock()
		outsideBefore := f.outsideBefore
		f.mu.Unlock()
		if outsideBefore != nil {
			outsideBefore(request.Method, request.URL.Path)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		body := ""
		if request.Body != nil {
			raw := new(bytes.Buffer)
			_, _ = raw.ReadFrom(request.Body)
			body = raw.String()
		}
		if f.before != nil {
			f.before(request.Method, request.URL.Path)
		}
		if f.fail != nil {
			if code := f.fail(request.Method, request.URL.Path); code != 0 {
				http.Error(response, "forced failure", code)
				return
			}
		}
		if f.drop != nil && f.drop(request.Method, request.URL.Path) {
			if hijacker, ok := response.(http.Hijacker); ok {
				connection, _, err := hijacker.Hijack()
				if err == nil {
					_ = connection.Close()
					return
				}
			}
			panic("test server cannot hijack connections")
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/widgets/issues":
			_ = json.NewEncoder(response).Encode(f.list)
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/widgets/issues":
			f.bodies = append(f.bodies, body)
			number := len(f.issues) + 101
			issue := map[string]any{"id": number + 1000, "number": number, "state": "open", "title": "", "body": ""}
			_ = json.Unmarshal([]byte(body), &issue)
			f.issues = append(f.issues, issue)
			f.list = append(f.list, issue)
			if f.truncate != nil && f.truncate(request.Method, request.URL.Path) {
				_, _ = io.WriteString(response, `{"number":`)
				return
			}
			_ = json.NewEncoder(response).Encode(issue)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/sub_issues"):
			parent := issueNumberFromPath(request.URL.Path, "sub_issues")
			var children []map[string]any
			for _, child := range f.children[parent] {
				children = append(children, map[string]any{"number": child})
			}
			_ = json.NewEncoder(response).Encode(children)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/sub_issues"):
			f.bodies = append(f.bodies, body)
			parent := issueNumberFromPath(request.URL.Path, "sub_issues")
			var payload struct {
				ID int `json:"sub_issue_id"`
			}
			_ = json.Unmarshal([]byte(body), &payload)
			for _, issue := range f.issues {
				if id, _ := issue["id"].(int); id == payload.ID {
					if !containsNumber(f.children[parent], issue["number"].(int)) {
						f.children[parent] = append(f.children[parent], issue["number"].(int))
					}
				}
			}
			response.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPatch && strings.HasPrefix(request.URL.Path, "/repos/acme/widgets/issues/"):
			number := issueNumberFromPath(request.URL.Path, "")
			var update map[string]any
			_ = json.Unmarshal([]byte(body), &update)
			for _, issue := range f.list {
				if issue["number"] == number {
					issue["title"], issue["body"] = update["title"], update["body"]
					f.updates = append(f.updates, number)
					_ = json.NewEncoder(response).Encode(issue)
					return
				}
			}
			http.Error(response, "missing issue", http.StatusNotFound)
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/repos/acme/widgets/issues/"):
			number := issueNumberFromPath(request.URL.Path, "")
			for _, issue := range f.issues {
				if issue["number"] == number {
					_ = json.NewEncoder(response).Encode(issue)
					return
				}
			}
			http.Error(response, "missing issue", http.StatusNotFound)
		default:
			http.Error(response, "unexpected "+request.Method+" "+request.URL.Path, http.StatusNotFound)
		}
	})
}

func newForgeServer(t *testing.T) *forgeServer {
	t.Helper()
	forge := &forgeServer{children: make(map[int][]int)}
	forge.server = httptest.NewServer(forge.handler())
	t.Cleanup(forge.server.Close)
	return forge
}

func (f *forgeServer) createdCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.issues)
}

func (f *forgeServer) updatedIssues() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.updates...)
}

func (f *forgeServer) receivedBodies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.bodies...)
}

func issueNumberFromPath(path, trailing string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	index := len(parts) - 1
	if trailing != "" {
		index--
	}
	number, _ := strconv.Atoi(parts[index])
	return number
}

func containsNumber(numbers []int, wanted int) bool {
	for _, number := range numbers {
		if number == wanted {
			return true
		}
	}
	return false
}

// ledgerCLI is the skl CLI bound to a controllable forge with its own
// output buffer.
type ledgerCLI struct {
	app *stageApp
	out *bytes.Buffer
}

func newLedgerApp(t *testing.T, forge *forgeServer) ledgerCLI {
	t.Helper()
	var output bytes.Buffer
	factory := func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(forge.server.URL, "secret", forge.server.Client())
		backend.BindRepository(repository)
		return backend, nil
	}
	return ledgerCLI{app: newApp(factory, bytes.NewReader(nil), &output, &output), out: &output}
}

// accept runs skl ledger accept and returns the parsed JSON outcome.
func (c ledgerCLI) accept(t *testing.T, repo, directory string, issueFlags ...string) ledgerOutcome {
	t.Helper()
	args := append([]string{"skl", "ledger", "accept", "--repo", repo, "--proposal-dir", directory, "--format", "json"}, issueFlags...)
	outcome, text := c.run(t, args)
	_ = text
	return outcome
}

func (c ledgerCLI) run(t *testing.T, args []string) (ledgerOutcome, string) {
	t.Helper()
	c.out.Reset()
	if err := c.app.Run(args); err != nil {
		t.Fatalf("run %v: %v\n%s", args, err, c.out.String())
	}
	var outcome ledgerOutcome
	if err := json.Unmarshal(c.out.Bytes(), &outcome); err != nil {
		t.Fatalf("decode outcome %q: %v", c.out.String(), err)
	}
	return outcome, c.out.String()
}

// writeProposal writes one proposal intake directory.
func writeProposal(t *testing.T, parent string, proposal proposalSpec) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), proposal.name)
	writeFile(t, filepath.Join(directory, "proposal.md"), proposal.description)
	writeFile(t, filepath.Join(directory, "proposal.json"), proposal.declaration())
	for _, slice := range proposal.slices {
		for name, contents := range slice.files {
			writeFile(t, filepath.Join(directory, slice.name, name), contents)
		}
	}
	return directory
}

type proposalSliceSpec struct {
	name   string
	title  string
	branch string
	files  map[string]string
}

type proposalSpec struct {
	name        string
	description string
	parentTitle string
	slices      []proposalSliceSpec
	// depends maps a slice name to its declared dependencies.
	depends map[string][]string
}

func (p proposalSpec) declaration() string {
	var builder strings.Builder
	builder.WriteString(`{"proposal": "` + p.name + `"`)
	if p.parentTitle != "" {
		builder.WriteString(`, "parent_title": "` + p.parentTitle + `"`)
	}
	builder.WriteString(`, "slices": [`)
	for index, slice := range p.slices {
		if index > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(`{"name": "` + slice.name + `"`)
		if slice.title != "" {
			builder.WriteString(`, "title": "` + slice.title + `"`)
		}
		if slice.branch != "" {
			builder.WriteString(`, "branch": "` + slice.branch + `"`)
		}
		if edges := p.depends[slice.name]; len(edges) > 0 {
			builder.WriteString(`, "depends": ["` + strings.Join(edges, `", "`) + `"]`)
		}
		builder.WriteString(`}`)
	}
	builder.WriteString("]}")
	return builder.String()
}

// sourceRepository creates one source repository checkout bound to the
// acme/widgets GitHub repository. The directory name is deliberately not
// the repository name.
func sourceRepository(t *testing.T, owner, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "local-copy")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "-q", "-b", "main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "remote", "add", "origin", "git@github.com:"+owner+"/"+name+".git")
	writeFile(t, filepath.Join(root, "README.md"), "widget\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-q", "-m", "initial")
	return root
}

// issueFile writes one temporary issue body and returns its --issue flag.
func issueFile(t *testing.T, slice, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), slice+".md")
	writeFile(t, path, body)
	return "--issue=" + slice + "=" + path
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	return string(readFile(t, path))
}

// singleSlice is one minimal but complete single-slice proposal whose only
// slice is named foundation. dualSlice adds a feature slice and a parent.
func singleSlice(name string) proposalSpec {
	return proposalSpec{
		name:        name,
		description: "# " + name + "\n\nDurable description.\n",
		slices: []proposalSliceSpec{{
			name:  "foundation",
			title: "Add foundation",
			files: map[string]string{
				"intent.md":   "# Foundation intent\n\n## Manual verification\n\n- [ ] Confirm the dashboard by hand.\n",
				"behavior.md": "# Foundation behavior\n\n## Rule: Foundation works\n\nIt works.\n",
			},
		}},
	}
}

func dualSlice(name string) proposalSpec {
	spec := singleSlice(name)
	spec.parentTitle = "Deliver " + name
	spec.slices = append(spec.slices, proposalSliceSpec{
		name:  "feature",
		title: "Add feature",
		files: map[string]string{
			"intent.md":   "# Feature intent\n",
			"behavior.md": "# Feature behavior\n",
			"plan.md":     "# Feature plan\n",
		},
	})
	return spec
}

// ledgerPaths lists the record paths present in the ledger working tree.
func ledgerPaths(t *testing.T, clone string) []string {
	t.Helper()
	output := runGitOutput(t, clone, "ls-files")
	return strings.Fields(output)
}

func readLedgerFile(t *testing.T, clone, path string) string {
	t.Helper()
	return readFileString(t, filepath.Join(clone, filepath.FromSlash(path)))
}

// --- Rule B1: one machine configuration selects the ledger ---

func TestLedgerConfigurationSelectsOneClone(t *testing.T) {
	first := newLedgerFixture(t)
	second := newLedgerFixture(t)
	// Restore first's selection, then point home at the second clone.
	first.selectWithXDG(t)
	writeFile(t, filepath.Join(first.home, ".config", "skl", "config.json"), `{"ledger": "`+second.clone+`"}`+"\n")

	spec := singleSlice("first-proposal")
	directory := writeProposal(t, "", spec)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	if outcome := cli.accept(t, root, directory); outcome.Status != "accepted" {
		t.Fatalf("XDG selection did not accept: %s", mustJSON(t, outcome))
	}
	if _, err := os.Stat(filepath.Join(first.clone, "projects", "widgets")); err != nil {
		t.Fatalf("acceptance not recorded in the XDG-selected clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second.clone, "projects")); err == nil {
		t.Fatal("home-selected clone received work while XDG_CONFIG_HOME was set")
	}

	// XDG unset: the home configuration selects the other clone.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", first.home)
	spec2 := singleSlice("second-proposal")
	directory2 := writeProposal(t, "", spec2)
	outcome2 := cli.accept(t, root, directory2)
	if outcome2.Status != "accepted" {
		t.Fatalf("home selection did not accept: %s", mustJSON(t, outcome2))
	}
	if _, err := os.Stat(filepath.Join(second.clone, "projects", "widgets")); err != nil {
		t.Fatalf("acceptance not recorded in the home-configured clone: %v", err)
	}
	// Neither operation wrote ledger configuration into the source repository.
	if strings.Contains(ledgerSnapshot(t, root), "skl") {
		t.Fatalf("ledger configuration leaked into the source repository:\n%s", ledgerSnapshot(t, root))
	}
	if _, err := os.Stat(filepath.Join(root, ".config")); err == nil {
		t.Fatal("ledger configuration leaked into the source repository")
	}
}

func TestLedgerConfigurationRefusals(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	directory := writeProposal(t, "", singleSlice("refused-proposal"))
	before := ledgerSnapshot(t, fixture.clone)

	cases := map[string]string{
		"missing":     "",
		"malformed":   "{not json",
		"no setting":  "{}",
		"relative":    `{"ledger": "relative/ledger"}`,
		"not a clone": ``,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			switch name {
			case "missing":
				if err := os.Remove(filepath.Join(fixture.config, "skl", "config.json")); err != nil {
					t.Fatal(err)
				}
			case "not a clone":
				fixture.misconfigure(t, `{"ledger": "`+t.TempDir()+`"}`)
			default:
				fixture.misconfigure(t, body)
			}
			args := []string{"skl", "ledger", "accept", "--repo", root, "--proposal-dir", directory, "--format", "json"}
			outcome, text := cli.run(t, args)
			if outcome.Status != "fix_required" || outcome.Reason == "" || outcome.Repair == "" {
				t.Fatalf("%s did not refuse with a concrete repair: %s", name, text)
			}
			if ledgerSnapshot(t, fixture.clone) != before {
				t.Fatalf("%s changed the ledger:\n%s", name, ledgerSnapshot(t, fixture.clone))
			}
			if forge.createdCount() != 0 {
				t.Fatalf("%s published issues", name)
			}
		})
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// --- Rule B2: project identity belongs to the source repository ---

func TestProjectIdentityFollowsRepositoryAcrossCheckouts(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	first := sourceRepository(t, "acme", "widgets")
	if outcome := cli.accept(t, first, writeProposal(t, "", singleSlice("first-proposal"))); outcome.Status != "accepted" {
		t.Fatalf("first checkout did not accept: %s", mustJSON(t, outcome))
	}
	// A differently named checkout resolving to the same repository, over a
	// different transport spelling of the same remote.
	second := filepath.Join(t.TempDir(), "totally-elsewhere")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, second, "init", "-q", "-b", "main")
	runGit(t, second, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	secondSpec := singleSlice("second-proposal")
	secondSpec.slices[0].branch = "second-foundation"
	if outcome := cli.accept(t, second, writeProposal(t, "", secondSpec)); outcome.Status != "accepted" {
		t.Fatalf("renamed checkout did not accept: %s", mustJSON(t, outcome))
	}
	// A worktree of the first checkout resolves to the same repository.
	worktree := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, first, "worktree", "add", "-q", "--detach", worktree, "main")
	thirdSpec := singleSlice("third-proposal")
	thirdSpec.slices[0].branch = "third-foundation"
	if outcome := cli.accept(t, worktree, writeProposal(t, "", thirdSpec)); outcome.Status != "accepted" {
		t.Fatalf("worktree did not accept: %s", mustJSON(t, outcome))
	}

	project := readLedgerFile(t, fixture.clone, "projects/widgets/project.json")
	if project != "{\n  \"repository\": \"acme/widgets\"\n}\n" {
		t.Fatalf("project identity = %q", project)
	}
	for _, proposal := range []string{"first-proposal", "second-proposal", "third-proposal"} {
		if _, err := os.Stat(filepath.Join(fixture.clone, "projects", "widgets", "proposals", proposal)); err != nil {
			t.Fatalf("proposal %s missing from the shared project: %v", proposal, err)
		}
	}
	fourthSpec := singleSlice("fourth-proposal")
	fourthSpec.slices[0].branch = "fourth-foundation"
	if outcome := cli.accept(t, first, writeProposal(t, "", fourthSpec)); outcome.Status != "accepted" || outcome.Acceptance.Project != "widgets" || outcome.Acceptance.Repository != "acme/widgets" {
		t.Fatalf("project identity did not follow the repository: %s", mustJSON(t, outcome))
	}
}

func TestProjectNameCollisionRefused(t *testing.T) {
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	widgets := sourceRepository(t, "acme", "widgets")
	if outcome := cli.accept(t, widgets, writeProposal(t, "", singleSlice("acme-work"))); outcome.Status != "accepted" {
		t.Fatalf("setup acceptance failed: %s", mustJSON(t, outcome))
	}
	before := ledgerSnapshot(t, fixture.clone)
	issuesBefore := forge.createdCount()

	other := sourceRepository(t, "other", "widgets")
	collide := newLedgerApp(t, forge)
	outcome := collide.accept(t, other, writeProposal(t, "", singleSlice("other-work")))
	if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "acme/widgets") || !strings.Contains(outcome.Reason, "other/widgets") {
		t.Fatalf("collision not identified: %s", mustJSON(t, outcome))
	}
	if ledgerSnapshot(t, fixture.clone) != before {
		t.Fatal("collision refusal changed the ledger")
	}
	if forge.createdCount() != issuesBefore {
		t.Fatal("collision refusal published issues")
	}
	// The source repository is untouched too.
	if strings.Contains(ledgerSnapshot(t, other), "projects") {
		t.Fatal("collision refusal wrote into the source repository")
	}
}

// --- Rule B3: acceptance freezes a complete proposal locally ---

func TestAcceptSingleSliceFreezesExactLayout(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	before := ledgerSnapshot(t, root)
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	spec := singleSlice("record-foundation")
	spec.slices[0].branch = "add-foundation"
	directory := writeProposal(t, "", spec)
	outcome := cli.accept(t, root, directory)
	if outcome.Status != "accepted" || outcome.Acceptance.Project != "widgets" || outcome.Acceptance.Proposal != "record-foundation" {
		t.Fatalf("unexpected outcome: %s", mustJSON(t, outcome))
	}
	if outcome.Acceptance.Commit != strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")) {
		t.Fatalf("reported commit %s is not the ledger head %s; outcome=%s", outcome.Acceptance.Commit, runGitOutput(t, fixture.clone, "rev-parse", "HEAD"), mustJSON(t, outcome))
	}

	// Exact ADR 0006 layout; optional files are not scaffolded.
	want := []string{
		"README.md",
		"projects/widgets/project.json",
		"projects/widgets/proposals/record-foundation/foundation/behavior.md",
		"projects/widgets/proposals/record-foundation/foundation/intent.md",
		"projects/widgets/proposals/record-foundation/foundation/state.json",
		"projects/widgets/proposals/record-foundation/proposal.json",
		"projects/widgets/proposals/record-foundation/proposal.md",
	}
	if got := ledgerPaths(t, fixture.clone); !equalStrings(got, want) {
		t.Fatalf("ledger layout = %v, want %v", got, want)
	}
	// Contract bytes are preserved exactly.
	if got := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/record-foundation/foundation/intent.md"); got != spec.slices[0].files["intent.md"] {
		t.Fatalf("intent bytes changed: %q", got)
	}
	// state.json owns the initial lifecycle, planned branch, and dependencies; no Claim.
	var state map[string]any
	if err := json.Unmarshal([]byte(readLedgerFile(t, fixture.clone, "projects/widgets/proposals/record-foundation/foundation/state.json")), &state); err != nil {
		t.Fatal(err)
	}
	if state["state"] != "ready_for_implementation" || state["branch"] != "add-foundation" || state["title"] != "Add foundation" {
		t.Fatalf("state.json = %v", state)
	}
	if _, claimed := state["claim"]; claimed {
		t.Fatalf("initial acceptance recorded a Claim: %v", state)
	}
	// proposal.json holds metadata, not a child inventory.
	var meta map[string]any
	if err := json.Unmarshal([]byte(readLedgerFile(t, fixture.clone, "projects/widgets/proposals/record-foundation/proposal.json")), &meta); err != nil {
		t.Fatal(err)
	}
	if _, inventory := meta["slices"]; inventory {
		t.Fatalf("proposal.json duplicates a child inventory: %v", meta)
	}
	// Source repository state is unchanged: no refs, files, or worktrees.
	if snapshot := ledgerSnapshot(t, root); snapshot != before {
		t.Fatalf("source repository changed:\n%s\n--was--\n%s", snapshot, before)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// --- Rule B3: invalid declarations and repeated acceptance ---

func TestAcceptRefusesInvalidDeclarations(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	before := ledgerSnapshot(t, fixture.clone)

	// A valid existing ledger Work Item to reference.
	if outcome := cli.accept(t, root, writeProposal(t, "", singleSlice("earlier-work"))); outcome.Status != "accepted" {
		t.Fatalf("setup acceptance failed: %s", mustJSON(t, outcome))
	}
	before = ledgerSnapshot(t, fixture.clone)
	issuesBefore := forge.createdCount()

	cases := []struct {
		name   string
		mutate func(spec *proposalSpec)
		reason string
	}{
		{"missing intent.md", func(spec *proposalSpec) {
			delete(spec.slices[0].files, "intent.md")
		}, "misses intent.md"},
		{"missing behavior.md", func(spec *proposalSpec) {
			delete(spec.slices[0].files, "behavior.md")
		}, "misses behavior.md"},
		{"unknown file", func(spec *proposalSpec) {
			spec.slices[0].files["notes.md"] = "unexpected"
		}, "unexpected file"},
		{"nested directory", func(spec *proposalSpec) {
			spec.slices[0].files["plan.md/sub"] = "nested"
		}, ""},
		{"escaping slice name", func(spec *proposalSpec) {
			spec.slices[0].name = "../escape"
		}, "not a valid record name"},
		{"unknown dependency", func(spec *proposalSpec) {
			spec.depends = map[string][]string{spec.slices[0].name: {"missing-slice"}}
		}, "does not resolve"},
		{"self dependency", func(spec *proposalSpec) {
			spec.depends = map[string][]string{spec.slices[0].name: {spec.slices[0].name}}
		}, "depends on itself"},
		{"cycle", func(spec *proposalSpec) {
			spec.parentTitle = "Break the cycle"
			spec.slices = append(spec.slices, proposalSliceSpec{name: "second", files: map[string]string{"intent.md": "i", "behavior.md": "b"}})
			spec.depends = map[string][]string{
				spec.slices[0].name: {"second"},
				"second":            {spec.slices[0].name},
			}
		}, "cycle"},
		{"unrecorded external dependency", func(spec *proposalSpec) {
			spec.depends = map[string][]string{spec.slices[0].name: {"proposals/never-accepted/base"}}
		}, "not recorded"},
		{"invalid branch", func(spec *proposalSpec) {
			spec.slices[0].branch = "-bad branch"
		}, "invalid planned branch"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			spec := singleSlice("invalid-" + strings.ReplaceAll(testCase.name, " ", "-"))
			// Keep names kebab-safe.
			spec.name = "invalid-case"
			testCase.mutate(&spec)
			directory := writeProposal(t, "", spec)
			outcome, text := cli.run(t, []string{"skl", "ledger", "accept", "--repo", root, "--proposal-dir", directory, "--format", "json"})
			if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, testCase.reason) {
				t.Fatalf("refusal did not name %q: %s", testCase.reason, text)
			}
			if ledgerSnapshot(t, fixture.clone) != before {
				t.Fatalf("%s partially accepted work", testCase.name)
			}
			if forge.createdCount() != issuesBefore {
				t.Fatalf("%s published issues", testCase.name)
			}
		})
	}
}

func TestAcceptRefusesMalformedDeclarations(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	directory := filepath.Join(t.TempDir(), "malformed")
	writeFile(t, filepath.Join(directory, "proposal.md"), "description\n")
	writeFile(t, filepath.Join(directory, "foundation", "intent.md"), "i\n")
	writeFile(t, filepath.Join(directory, "foundation", "behavior.md"), "b\n")
	before := ledgerSnapshot(t, fixture.clone)

	for name, body := range map[string]string{
		"missing declaration": "",
		"invalid json":        "{",
		"no proposal":         `{"slices": [{"name": "foundation"}]}`,
		"no slices":           `{"proposal": "malformed", "slices": []}`,
		"unknown field":       `{"proposal": "malformed", "backend": "local", "slices": [{"name": "foundation"}]}`,
		"no slice names":      `{"proposal": "malformed", "slices": [{"title": "x"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if body != "" {
				writeFile(t, filepath.Join(directory, "proposal.json"), body)
			}
			outcome, _ := cli.run(t, []string{"skl", "ledger", "accept", "--repo", root, "--proposal-dir", directory, "--format", "json"})
			if outcome.Status != "fix_required" {
				t.Fatalf("%s did not refuse: %s", name, mustJSON(t, outcome))
			}
			if ledgerSnapshot(t, fixture.clone) != before {
				t.Fatalf("%s changed the ledger", name)
			}
		})
	}
}

func TestRepeatAcceptanceIsIdempotentAndRefusesChanges(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	spec := singleSlice("stable-proposal")
	if outcome := cli.accept(t, root, writeProposal(t, "", spec)); outcome.Status != "accepted" {
		t.Fatalf("initial acceptance failed: %s", mustJSON(t, outcome))
	}
	head := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))

	// Unchanged repetition identifies the existing accepted work.
	outcome := cli.accept(t, root, writeProposal(t, "", spec))
	if outcome.Status != "existing" {
		t.Fatalf("unchanged repetition did not identify existing work: %s", mustJSON(t, outcome))
	}
	if after := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")); after != head {
		t.Fatalf("unchanged repetition committed %s", after)
	}

	// Changed contract content refuses replacement in place.
	changed := singleSlice("stable-proposal")
	changed.slices[0].files["intent.md"] = "# Changed intent\n"
	if outcome := cli.accept(t, root, writeProposal(t, "", changed)); outcome.Status != "fix_required" || !strings.Contains(outcome.Reason+outcome.Repair, "renewed Proposal") {
		t.Fatalf("changed contract was not refused: %s", mustJSON(t, outcome))
	}
	// Changed declared relationships refuse too.
	relinked := singleSlice("stable-proposal")
	relinked.slices[0].branch = "other-branch"
	if outcome := cli.accept(t, root, writeProposal(t, "", relinked)); outcome.Status != "fix_required" || !strings.Contains(outcome.Reason+outcome.Repair, "renewed Proposal") {
		t.Fatalf("changed relationship was not refused: %s", mustJSON(t, outcome))
	}
	if after := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")); after != head {
		t.Fatalf("refused repetition committed %s", after)
	}
	if got := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/stable-proposal/foundation/intent.md"); got != spec.slices[0].files["intent.md"] {
		t.Fatalf("accepted content changed: %q", got)
	}
}

func TestAcceptMultiSliceWithDependencies(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	// An existing ledger Work Item recorded by an earlier proposal.
	if outcome := cli.accept(t, root, writeProposal(t, "", singleSlice("earlier-work"))); outcome.Status != "accepted" {
		t.Fatalf("setup acceptance failed: %s", mustJSON(t, outcome))
	}

	spec := dualSlice("dependent-work")
	spec.slices[0].branch = "dependent-foundation"
	spec.depends = map[string][]string{"feature": {"foundation", "proposals/earlier-work/foundation"}}
	outcome := cli.accept(t, root, writeProposal(t, "", spec),
		"--issue=foundation="+writeTemp(t, t, "foundation body\n"),
		"--issue=feature="+writeTemp(t, t, "feature body\n"),
		"--parent-body="+writeTemp(t, t, "parent body\n"))
	if outcome.Status != "accepted" {
		t.Fatalf("multi-slice acceptance failed: %s", mustJSON(t, outcome))
	}
	// Both slices accepted together; membership from the shared directory.
	for _, slice := range []string{"foundation", "feature"} {
		if _, err := os.Stat(filepath.Join(fixture.clone, "projects", "widgets", "proposals", "dependent-work", slice, "state.json")); err != nil {
			t.Fatalf("slice %s missing: %v", slice, err)
		}
	}
	// Dependencies recorded canonically, including the existing ledger item.
	var feature map[string]any
	if err := json.Unmarshal([]byte(readLedgerFile(t, fixture.clone, "projects/widgets/proposals/dependent-work/feature/state.json")), &feature); err != nil {
		t.Fatal(err)
	}
	dependencies, _ := feature["dependencies"].([]any)
	if len(dependencies) != 2 {
		t.Fatalf("feature dependencies = %v", dependencies)
	}
	// plan.md was warranted and frozen; the parent is recorded without branch
	// or submission of its own.
	if _, err := os.Stat(filepath.Join(fixture.clone, "projects", "widgets", "proposals", "dependent-work", "feature", "plan.md")); err != nil {
		t.Fatalf("warranted plan.md not frozen: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(readLedgerFile(t, fixture.clone, "projects/widgets/proposals/dependent-work/proposal.json")), &meta); err != nil {
		t.Fatal(err)
	}
	if _, hasParent := meta["parent_issue"]; !hasParent {
		t.Fatalf("multi-slice parent attachment missing: %v", meta)
	}
	if _, hasBranch := meta["branch"]; hasBranch {
		t.Fatalf("parent owns a branch: %v", meta)
	}
	if forge.createdCount() != 3 { // two children plus the parent
		t.Fatalf("published %d issues, want two children and a parent", forge.createdCount())
	}
}

func writeTemp(t *testing.T, tb testing.TB, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body.md")
	writeFile(t, path, contents)
	return path
}

// --- Rule B4: readback supplies exact contracts without discovery work ---

func TestLedgerShowReturnsExactAcceptedContent(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	spec := singleSlice("readback-work")
	manual := "- [ ] Confirm the refund in the Stripe dashboard.\n"
	spec.slices[0].files["intent.md"] = "# Readback intent\n\n## Manual verification\n\n" + manual
	directory := writeProposal(t, "", spec)
	acceptance := cli.accept(t, root, directory)
	if acceptance.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, acceptance))
	}
	// The acceptance output binds the concrete public readback command.
	report := ledgerMarkdown(ledgerOutcome{Status: acceptance.Status, Acceptance: acceptance.Acceptance})
	readbackCommand := "skl ledger show --item readback-work/foundation"
	if !strings.Contains(report, readbackCommand) {
		t.Fatalf("acceptance output names no concrete readback command:\n%s", report)
	}

	// Later activity: another proposal committed, temporary inputs removed,
	// forge closed, source markers absent.
	later := singleSlice("later-work")
	later.slices[0].branch = "later-foundation"
	if outcome := cli.accept(t, root, writeProposal(t, "", later)); outcome.Status != "accepted" {
		t.Fatalf("later acceptance failed: %s", mustJSON(t, outcome))
	}
	os.RemoveAll(directory)
	forge.server.Close()

	args := []string{"skl", "ledger", "show", "--repo", root, "--item", "readback-work/foundation", "--format", "json"}
	outcome, _ := cli.run(t, args)
	if outcome.Status != "shown" || outcome.Readback == nil {
		t.Fatalf("readback failed after other activity: %s", mustJSON(t, outcome))
	}
	readback := outcome.Readback
	if readback.Item != "readback-work/foundation" || readback.State != "ready_for_implementation" || readback.Branch != "foundation" {
		t.Fatalf("readback facts = %s", mustJSON(t, readback))
	}
	found := false
	for _, document := range readback.Documents {
		if strings.HasSuffix(document.Path, "/intent.md") {
			found = true
			if document.Contents != spec.slices[0].files["intent.md"] {
				t.Fatalf("intent bytes changed: %q", document.Contents)
			}
			if !strings.Contains(document.Contents, manual) {
				t.Fatal("Manual Verification obligations missing from readback")
			}
			if len(document.Commit) != 40 || document.Commit != strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")) {
				t.Fatalf("document reference = %s", document.Commit)
			}
			if !strings.HasPrefix(document.Path, "projects/widgets/proposals/readback-work/foundation/") {
				t.Fatalf("document path = %s", document.Path)
			}
		}
	}
	if !found {
		t.Fatalf("intent.md missing from readback: %s", mustJSON(t, readback))
	}

	// Default Markdown and explicit JSON convey equivalent facts.
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "ledger", "show", "--repo", root, "--item", "readback-work/foundation"}); err != nil {
		t.Fatalf("markdown readback failed: %v\n%s", err, cli.out.String())
	}
	markdown := cli.out.String()
	for _, want := range []string{"Work Item: readback-work/foundation", "Planned branch: foundation", "State: ready_for_implementation"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown readback lacks %q:\n%s", want, markdown)
		}
	}
	if !strings.Contains(markdown, manual) {
		t.Fatalf("markdown readback lacks the manual verification content:\n%s", markdown)
	}
}

func TestLedgerShowRefusesMissingReferencesWithoutSubstitution(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	spec := singleSlice("reference-work")
	if outcome := cli.accept(t, root, writeProposal(t, "", spec)); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	head := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))

	// Exact reference readback works.
	outcome, _ := cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--commit", head,
		"--path", "projects/widgets/proposals/reference-work/foundation/intent.md", "--format", "json"})
	if outcome.Status != "shown" || outcome.Document == nil || outcome.Document.Contents != spec.slices[0].files["intent.md"] {
		t.Fatalf("exact reference readback failed: %s", mustJSON(t, outcome))
	}

	// A missing path at a valid commit is diagnosed, never substituted.
	outcome, _ = cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--commit", head,
		"--path", "projects/widgets/proposals/reference-work/foundation/plan.md", "--format", "json"})
	if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "plan.md") {
		t.Fatalf("missing path not diagnosed: %s", mustJSON(t, outcome))
	}
	if outcome.Document != nil && outcome.Document.Contents != "" {
		t.Fatal("missing path returned substituted content")
	}

	// A missing commit is diagnosed, never substituted.
	missing := strings.Repeat("0", 40)
	outcome, _ = cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--commit", missing,
		"--path", "projects/widgets/proposals/reference-work/foundation/intent.md", "--format", "json"})
	if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "unavailable") {
		t.Fatalf("missing commit not diagnosed: %s", mustJSON(t, outcome))
	}
	if outcome.Document != nil {
		t.Fatal("missing commit returned substituted content")
	}

	// An unknown item identity is diagnosed without guessed content.
	outcome, _ = cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--item", "never-accepted/foundation", "--format", "json"})
	if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "no accepted record") {
		t.Fatalf("unknown item not diagnosed: %s", mustJSON(t, outcome))
	}
}

// --- Rules B5-B6: publication follows local acceptance ---

func TestPublicationSucceedsWithoutPersistingBodies(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	spec := dualSlice("published-work")
	directory := writeProposal(t, "", spec)
	outcome := cli.accept(t, root, directory,
		"--issue=foundation="+writeTemp(t, t, "# Foundation\n\nDescriptive temporary body.\n"),
		"--issue=feature="+writeTemp(t, t, "# Feature\n\nDescriptive temporary body.\n"),
		"--parent-body="+writeTemp(t, t, "# Grouping\n\nParent description.\n"))
	if outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}

	// The configured ledger upstream received the accepted record.
	upstreamHead := strings.TrimSpace(runGitOutput(t, fixture.upstream, "rev-parse", "refs/heads/main"))
	localHead := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))
	if upstreamHead != localHead {
		t.Fatalf("upstream head %s does not carry the acceptance %s", upstreamHead, localHead)
	}

	// The forge received the supplied descriptions verbatim and a parent
	// grouping the children; the ledger records attachments without
	// persisting any public-body file.
	bodies := forge.receivedBodies()
	if len(bodies) != 5 { // 3 issue creations + 2 sub-issue links
		t.Fatalf("forge calls = %d: %v", len(bodies), bodies)
	}
	for _, body := range bodies[3:] {
		if !strings.Contains(body, "sub_issue_id") {
			t.Fatalf("no grouping links recorded: %v", bodies)
		}
	}
	for _, wanted := range []string{"# Foundation", "# Feature", "# Grouping"} {
		found := false
		for _, body := range bodies {
			if strings.Contains(body, wanted) {
				found = true
			}
		}
		if !found {
			t.Fatalf("forge never received %q: %v", wanted, bodies)
		}
	}
	for _, path := range ledgerPaths(t, fixture.clone) {
		if strings.HasSuffix(path, "issue.md") || strings.HasSuffix(path, "pr.md") {
			t.Fatalf("ledger persisted a public body file: %s", path)
		}
	}
	// Attachments are recorded in state.json and proposal.json.
	stateJSON := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/published-work/foundation/state.json")
	if !strings.Contains(stateJSON, `"issue"`) || !strings.Contains(stateJSON, `"repository": "acme/widgets"`) {
		t.Fatalf("slice attachment not recorded: %s", stateJSON)
	}
	metaJSON := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/published-work/proposal.json")
	if !strings.Contains(metaJSON, `"parent_issue"`) {
		t.Fatalf("parent attachment not recorded: %s", metaJSON)
	}

	// Edited public bodies, titles, or comments never change local readback:
	// the accepted bytes, not the edited forge content, come back.
	forge.mu.Lock()
	for index := range forge.list {
		forge.list[index]["body"] = "edited by a human"
		forge.list[index]["title"] = "Renamed by a human"
	}
	forge.mu.Unlock()
	readback, _ := cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--item", "published-work/foundation", "--format", "json"})
	if readback.Status != "shown" || readback.Readback.Item != "published-work/foundation" {
		t.Fatalf("readback changed after forge edits: %s", mustJSON(t, readback))
	}
	intentSeen := false
	for _, document := range readback.Readback.Documents {
		if strings.HasSuffix(document.Path, "/intent.md") {
			intentSeen = true
			if document.Contents != spec.slices[0].files["intent.md"] {
				t.Fatalf("readback substituted edited forge content: %q", document.Contents)
			}
		}
	}
	if !intentSeen {
		t.Fatalf("intent.md missing from readback after forge edits: %s", mustJSON(t, readback))
	}

	// Repeating acceptance does not recreate known attached issues.
	before := forge.createdCount()
	repeat := cli.accept(t, root, directory,
		"--issue=foundation="+writeTemp(t, t, "# Foundation\n\nDescriptive temporary body.\n"),
		"--issue=feature="+writeTemp(t, t, "# Feature\n\nDescriptive temporary body.\n"),
		"--parent-body="+writeTemp(t, t, "# Grouping\n\nParent description.\n"))
	if repeat.Status != "existing" {
		t.Fatalf("repeat acceptance: %s", mustJSON(t, repeat))
	}
	if forge.createdCount() != before {
		t.Fatalf("repeat acceptance recreated attached issues: %d -> %d", before, forge.createdCount())
	}
	// It presents the current prose on the established objects instead.
	if updates := forge.updatedIssues(); len(updates) != 3 {
		t.Fatalf("established issues were not updated with current prose: %v", updates)
	}
}

func TestPublicationFailuresAreReportedWithoutPublicationState(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	spec := dualSlice("partial-publication")
	directory := writeProposal(t, "", spec)
	forge.mu.Lock()
	created := 0
	forge.fail = func(method, path string) int {
		// Fail authorization on every issue creation after the first, and
		// fail the parent's sub-issue listing.
		if method == http.MethodPost && path == "/repos/acme/widgets/issues" {
			created++
			if created > 1 {
				return http.StatusUnauthorized
			}
		}
		if method == http.MethodGet && strings.HasSuffix(path, "/sub_issues") {
			return http.StatusInternalServerError
		}
		return 0
	}
	forge.mu.Unlock()

	outcome := cli.accept(t, root, directory,
		"--issue=foundation="+writeTemp(t, t, "foundation body\n"),
		"--issue=feature="+writeTemp(t, t, "feature body\n"),
		"--parent-body="+writeTemp(t, t, "parent body\n"))
	if outcome.Status != "accepted" {
		t.Fatalf("publication failure undid acceptance: %s", mustJSON(t, outcome))
	}
	// The push still happened, and the first attachment is retained.
	if outcome.Acceptance.Slices[0].PushStatus.Status != ledger.PushPushed {
		t.Fatalf("push not attempted despite forge failure: %s", mustJSON(t, outcome))
	}
	attached, failed := 0, 0
	for _, slice := range outcome.Acceptance.Slices {
		if slice.Issue != nil {
			attached++
			continue
		}
		if slice.IssueStatus == nil || slice.IssueStatus.Status != ledger.IssueFailed || slice.IssueStatus.Detail == "" {
			t.Fatalf("failed publication was not reported with its cause: %s", mustJSON(t, slice))
		}
		failed++
	}
	if attached != 1 || failed != 1 {
		t.Fatalf("attachment and failure were not both reported: %s", mustJSON(t, outcome))
	}
	if outcome.Acceptance.ParentIssue != nil || outcome.Acceptance.ParentNote == nil || outcome.Acceptance.ParentNote.Status != ledger.IssueFailed {
		t.Fatalf("failed parent publication was not reported: %s", mustJSON(t, outcome))
	}
	// Failures are immediate results, never durable publication state.
	readback, _ := cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--item", "partial-publication/foundation", "--format", "json"})
	if readback.Status != "shown" {
		t.Fatalf("readback unavailable after publication failure: %s", mustJSON(t, readback))
	}
	for _, path := range []string{"feature/state.json", "foundation/state.json", "proposal.json"} {
		recorded := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/partial-publication/"+path)
		for _, forbidden := range []string{`"publication"`, `"parent_publication"`, `"failed"`, `"pending"`} {
			if strings.Contains(recorded, forbidden) {
				t.Fatalf("%s persisted issue publication state %s: %s", path, forbidden, recorded)
			}
		}
	}

	// A later explicit publication creates the missing child and parent from
	// fresh prose without repeating acceptance or recreating the attached one.
	forge.mu.Lock()
	forge.fail = nil
	forge.mu.Unlock()
	before := forge.createdCount()
	missing := ""
	for _, slice := range outcome.Acceptance.Slices {
		if slice.Issue == nil {
			missing = slice.Name
		}
	}
	acceptanceCommits := runGitOutput(t, fixture.clone, "log", "--format=%H", "--grep=^accept ")
	publication, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "partial-publication", "--format", "json",
		"--issue=" + missing + "=" + writeTemp(t, t, "fresh body\n"),
		"--parent-body=" + writeTemp(t, t, "fresh parent body\n")})
	if publication.Status != "attempted" || publication.Publication == nil || publication.Publication.ParentIssue == nil {
		t.Fatalf("explicit publication did not present the current view: %s", mustJSON(t, publication))
	}
	if forge.createdCount() != before+2 { // the missing child and the parent
		t.Fatalf("explicit publication created %d new issues, want the missing child and parent", forge.createdCount()-before)
	}
	if after := runGitOutput(t, fixture.clone, "log", "--format=%H", "--grep=^accept "); after != acceptanceCommits {
		t.Fatal("explicit publication repeated acceptance")
	}
}

func TestUncertainIssueCreationIsReportedWithoutRetry(t *testing.T) {
	for _, loss := range []string{"dropped connection", "truncated success"} {
		t.Run(loss, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			root := sourceRepository(t, "acme", "widgets")
			forge := newForgeServer(t)
			cli := newLedgerApp(t, forge)
			directory := writeProposal(t, "", singleSlice("uncertain-work"))
			// The forge creates the issue, but the caller never receives a
			// usable response.
			forge.mu.Lock()
			lost := false
			if loss == "dropped connection" {
				forge.drop = func(method, path string) bool {
					if lost || method != http.MethodPost || path != "/repos/acme/widgets/issues" {
						return false
					}
					lost = true
					issue := map[string]any{"id": 1555, "number": 555, "state": "open", "title": "Add foundation", "body": "uncertain body\n"}
					forge.issues = append(forge.issues, issue)
					forge.list = append(forge.list, issue)
					return true
				}
			} else {
				forge.truncate = func(method, path string) bool {
					if lost || method != http.MethodPost || path != "/repos/acme/widgets/issues" {
						return false
					}
					lost = true
					return true
				}
			}
			forge.mu.Unlock()

			outcome := cli.accept(t, root, directory, "--issue=foundation="+writeTemp(t, t, "uncertain body\n"))
			if outcome.Status != "accepted" {
				t.Fatalf("uncertain creation undid acceptance: %s", mustJSON(t, outcome))
			}
			slice := outcome.Acceptance.Slices[0]
			if slice.Issue != nil || slice.IssueStatus == nil || slice.IssueStatus.Status != ledger.IssueUncertain {
				t.Fatalf("uncertain creation was not reported as uncertain: %s", mustJSON(t, slice))
			}
			// Exactly one create was sent: no automatic retry, and no listing
			// guesses an attachment from the matching title and body.
			if forge.createdCount() != 1 {
				t.Fatalf("uncertain creation was retried: %d issues", forge.createdCount())
			}
			state := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/uncertain-work/foundation/state.json")
			if strings.Contains(state, `"issue"`) || strings.Contains(state, `"publication"`) || strings.Contains(state, "uncertain") {
				t.Fatalf("uncertain publication was persisted: %s", state)
			}
			if !strings.Contains(state, `"state": "ready_for_implementation"`) {
				t.Fatalf("uncertain publication changed the local lifecycle: %s", state)
			}

			// A later explicit publication uses the current inputs; it does
			// not promise deduplication across invocations.
			forge.mu.Lock()
			forge.drop, forge.truncate = nil, nil
			forge.mu.Unlock()
			later, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "uncertain-work", "--format", "json",
				"--issue=foundation=" + writeTemp(t, t, "fresh body\n")})
			if later.Publication == nil || later.Publication.Slices[0].Issue == nil || later.Publication.Slices[0].IssueStatus.Status != ledger.IssueCreated {
				t.Fatalf("later explicit publication did not publish the current view: %s", mustJSON(t, later))
			}
			if forge.createdCount() != 2 {
				t.Fatalf("later publication created %d issues in total, want a fresh second create", forge.createdCount())
			}
		})
	}
}

func TestParentUncertaintyIsReportedWithoutRecord(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	postCount := 0
	forge.drop = func(method, path string) bool {
		if method != http.MethodPost || path != "/repos/acme/widgets/issues" {
			return false
		}
		postCount++
		return postCount == 3 // the parent
	}
	directory := writeProposal(t, "", dualSlice("parent-uncertain"))
	outcome := newLedgerApp(t, forge).accept(t, root, directory,
		"--issue=foundation="+writeTemp(t, t, "foundation body\n"),
		"--issue=feature="+writeTemp(t, t, "feature body\n"),
		"--parent-body="+writeTemp(t, t, "parent body\n"))
	if outcome.Acceptance.ParentIssue != nil || outcome.Acceptance.ParentNote == nil || outcome.Acceptance.ParentNote.Status != ledger.IssueUncertain {
		t.Fatalf("uncertain parent was not reported: %s", mustJSON(t, outcome))
	}
	if postCount != 3 {
		t.Fatalf("uncertain parent creation was retried: %d creates", postCount)
	}
	proposal := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/parent-uncertain/proposal.json")
	if strings.Contains(proposal, `"parent_issue"`) || strings.Contains(proposal, `"parent_publication"`) {
		t.Fatalf("uncertain parent was persisted: %s", proposal)
	}
}

func TestCompetingLedgerHistoryRequiresReconciliation(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	spec := singleSlice("competing-work")
	if outcome := cli.accept(t, root, writeProposal(t, "", spec)); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	localHead := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))

	// Another clone pushes competing workflow history to the upstream.
	competitor := filepath.Join(t.TempDir(), "competitor")
	runGit(t, t.TempDir(), "clone", "-q", fixture.upstream, competitor)
	runGit(t, competitor, "config", "user.name", "Other")
	runGit(t, competitor, "config", "user.email", "other@example.com")
	writeFile(t, filepath.Join(competitor, "competing.txt"), "history\n")
	runGit(t, competitor, "add", "competing.txt")
	runGit(t, competitor, "commit", "-q", "-m", "competing history")
	runGit(t, competitor, "push", "-q", "origin", "main")
	upstreamHead := strings.TrimSpace(runGitOutput(t, competitor, "rev-parse", "HEAD"))

	after := singleSlice("after-divergence")
	after.slices[0].branch = "after-foundation"
	outcome := cli.accept(t, root, writeProposal(t, "", after))
	if outcome.Status != "accepted" {
		t.Fatalf("competing history refused acceptance: %s", mustJSON(t, outcome))
	}
	if outcome.Acceptance.Slices[0].PushStatus.Status != ledger.PushReconciliation {
		t.Fatalf("competing history not reported distinctly: %s", mustJSON(t, outcome))
	}
	// Local acceptance and remote history are both preserved.
	if after := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")); after == localHead {
		t.Fatal("local acceptance missing")
	}
	if strings.TrimSpace(runGitOutput(t, competitor, "rev-parse", "origin/main")) != upstreamHead {
		t.Fatal("remote history was modified")
	}
	if runGitOutput(t, fixture.clone, "log", "--oneline", "--all") == "" {
		t.Fatal("ledger history lost")
	}
	// Readback remains available.
	readback, _ := cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--item", "competing-work/foundation", "--format", "json"})
	if readback.Status != "shown" {
		t.Fatalf("readback unavailable during reconciliation: %s", mustJSON(t, readback))
	}
	// A pending push is distinguishable from reconciliation: an unreachable
	// remote reports pending, not reconciliation.
	broken := filepath.Join(t.TempDir(), "broken-upstream")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, broken, "init", "-q", "-b", "main", "--bare")
	runGit(t, fixture.clone, "remote", "set-url", "origin", filepath.Join(broken, "missing.git"))
	unreachable := singleSlice("unreachable-remote")
	unreachable.slices[0].branch = "unreachable-foundation"
	pending := cli.accept(t, root, writeProposal(t, "", unreachable))
	if pending.Status != "accepted" || pending.Acceptance.Slices[0].PushStatus.Status != ledger.PushPending {
		t.Fatalf("unreachable remote not pending: %s", mustJSON(t, pending))
	}
}

func TestDirtyLedgerRefusalPreservesEdits(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	spec := singleSlice("clean-work")
	if outcome := cli.accept(t, root, writeProposal(t, "", spec)); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	head := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))
	issuesBefore := forge.createdCount()

	// Uncommitted ledger edits prevent safe acceptance.
	writeFile(t, filepath.Join(fixture.clone, "projects", "widgets", "uncommitted.txt"), "human edit\n")
	outcome := cli.accept(t, root, writeProposal(t, "", singleSlice("blocked-work")))
	if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "uncommitted changes") {
		t.Fatalf("dirty ledger not refused: %s", mustJSON(t, outcome))
	}
	if after := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")); after != head {
		t.Fatalf("refusal committed %s", after)
	}
	if contents := readFileString(t, filepath.Join(fixture.clone, "projects", "widgets", "uncommitted.txt")); contents != "human edit\n" {
		t.Fatalf("refusal discarded human edits: %q", contents)
	}
	if forge.createdCount() != issuesBefore {
		t.Fatal("refused acceptance published issues")
	}

	// Leftovers of an interrupted write are equally refused.
	if err := os.Remove(filepath.Join(fixture.clone, "projects", "widgets", "uncommitted.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fixture.clone, "projects", "widgets", "proposals", "interrupted", "proposal.md"), "half-written\n")
	outcome = cli.accept(t, root, writeProposal(t, "", singleSlice("blocked-work")))
	if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "uncommitted changes") {
		t.Fatalf("interrupted write not refused: %s", mustJSON(t, outcome))
	}
}

// Accepted work is delivered only through the private ledger authority.

func TestLedgerWorkUsesPrivateDelivery(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)

	if outcome := cli.accept(t, root, writeProposal(t, "", singleSlice("awaiting-delivery"))); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	beforeWorktree := ledgerSnapshot(t, root)

	for _, lane := range []string{"implement", "watchdog"} {
		t.Run(lane+" next", func(t *testing.T) {
			cli.out.Reset()
			if err := cli.app.Run([]string{"skl", lane, "next", "--repo", root, "--format", "json"}); err != nil {
				t.Fatalf("run: %v\n%s", err, cli.out.String())
			}
			var outcome ledgerOutcome
			if err := json.Unmarshal(cli.out.Bytes(), &outcome); err != nil {
				// The legacy stage outcomes have their own envelopes; the
				// gate refusal reuses the ledger envelope.
				t.Fatalf("unexpected outcome shape %q: %v", cli.out.String(), err)
			}
			wanted := "no_work"
			if lane == "implement" {
				wanted = "work_available"
			}
			if outcome.Status != wanted {
				t.Fatalf("%s next status = %s; want %s", lane, cli.out.String(), wanted)
			}
		})
	}

	// Claim acquisition does not prepare source or consult the forge.
	if ledgerSnapshot(t, root) != beforeWorktree {
		t.Fatalf("legacy entry mutated source state:\n%s", ledgerSnapshot(t, root))
	}
	if head := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")); head == "" {
		t.Fatal("ledger disappeared")
	}
	if forge.createdCount() != 0 {
		t.Fatalf("legacy entry published to the forge")
	}
	// Markdown reports claimed work as unavailable, not another execution.
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "implement", "next", "--repo", root}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(cli.out.String(), "no_work") {
		t.Fatalf("markdown refusal lacks the explanation:\n%s", cli.out.String())
	}
}

func TestDeliveryNeverFallsBackToForgeAuthority(t *testing.T) {
	// No ledger configuration authorizes no forge fallback.
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
	var output bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "implement", "next", "--repo", root, "--format", "json"}); err != nil {
		t.Fatalf("legacy selection refused without adoption: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), `"fix_required"`) || strings.Contains(output.String(), `"packet"`) || backend.work[0].Claimed {
		t.Fatalf("unconfigured delivery used the forge queue: %s", output.String())
	}

	// Once accepted, the private item is selected instead of the public #7.
	fixture := newLedgerFixture(t)
	if outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("adopted-work"))); outcome.Status != "accepted" {
		t.Fatalf("adoption acceptance failed: %s", mustJSON(t, outcome))
	}
	if _, err := os.Stat(filepath.Join(fixture.clone, "projects", "widgets", "proposals", "adopted-work")); err != nil {
		t.Fatalf("adoption not recorded: %v", err)
	}
	app2 := newApp(func(repository github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	output.Reset()
	if err := app2.Run([]string{"skl", "implement", "next", "--repo", root, "--format", "json"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(output.String(), `"status":"work_available"`) || !strings.Contains(output.String(), "adopted-work/foundation") || backend.work[0].Claimed {
		t.Fatalf("adopted project fell back to the forge queue: %s", output.String())
	}
}

func TestLedgerRefusesSourceStorageOverlap(t *testing.T) {
	for _, test := range []struct {
		name       string
		ledgerPath func(*testing.T, string) string
	}{
		{name: "same checkout", ledgerPath: func(_ *testing.T, root string) string { return root }},
		{name: "linked worktree", ledgerPath: func(t *testing.T, root string) string {
			worktree := filepath.Join(t.TempDir(), "ledger-worktree")
			runGit(t, root, "worktree", "add", "-q", "-b", "ledger-storage", worktree, "HEAD")
			return worktree
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := sourceRepository(t, "acme", "widgets")
			ledgerPath := test.ledgerPath(t, root)
			config := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", config)
			writeFile(t, filepath.Join(config, "skl", "config.json"), `{"ledger":"`+ledgerPath+`"}`)
			forge := newForgeServer(t)
			before := ledgerSnapshot(t, root)
			outcome := newLedgerApp(t, forge).accept(t, root, writeProposal(t, "", singleSlice("private-work")))
			if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "shares Git storage") || !strings.Contains(outcome.Repair, "separate private ledger") {
				t.Fatalf("overlap not refused with a repair: %s", mustJSON(t, outcome))
			}
			if ledgerSnapshot(t, root) != before || forge.createdCount() != 0 {
				t.Fatalf("overlap mutated or published source records:\n%s", ledgerSnapshot(t, root))
			}
			if _, err := os.Stat(filepath.Join(root, "projects")); !os.IsNotExist(err) {
				t.Fatalf("private records entered the source checkout: %v", err)
			}
		})
	}
}

func TestConcurrentAcceptanceCannotOverwriteFrozenContract(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	firstSpec := singleSlice("concurrent-contract")
	secondSpec := singleSlice("concurrent-contract")
	secondSpec.slices[0].files["intent.md"] = "# Different accepted intent\n"
	directories := []string{writeProposal(t, "", firstSpec), writeProposal(t, "", secondSpec)}
	outcomes := runConcurrentAcceptanceHelpers(t, root, directories)
	accepted, refused := 0, 0
	for _, outcome := range outcomes {
		switch outcome.Status {
		case "accepted":
			accepted++
		case "fix_required":
			if !strings.Contains(outcome.Reason, "different content") {
				t.Fatalf("concurrent loser had the wrong refusal: %s", mustJSON(t, outcome))
			}
			refused++
		default:
			t.Fatalf("unexpected concurrent outcome: %s", mustJSON(t, outcome))
		}
	}
	if accepted != 1 || refused != 1 {
		t.Fatalf("concurrent acceptance outcomes: accepted=%d refused=%d", accepted, refused)
	}
	commits := strings.Fields(runGitOutput(t, fixture.clone, "log", "--format=%H", "--grep=^accept widgets/concurrent-contract$"))
	if len(commits) != 1 {
		t.Fatalf("concurrent acceptance created %d acceptance commits", len(commits))
	}
}

func runConcurrentAcceptanceHelpers(t *testing.T, root string, directories []string) []ledgerOutcome {
	t.Helper()
	results := make([]string, len(directories))
	commands := make([]*exec.Cmd, len(directories))
	logs := make([]bytes.Buffer, len(directories))
	for index := range directories {
		results[index] = filepath.Join(t.TempDir(), fmt.Sprintf("result-%d.json", index))
		command := exec.Command(os.Args[0], "-test.run=^TestConcurrentAcceptanceHelper$")
		command.Env = append(os.Environ(),
			"SKL_CONCURRENT_ACCEPT_HELPER=1",
			"SKL_CONCURRENT_ACCEPT_REPO="+root,
			"SKL_CONCURRENT_ACCEPT_PROPOSAL="+directories[index],
			"SKL_CONCURRENT_ACCEPT_RESULT="+results[index],
		)
		command.Stdout = &logs[index]
		command.Stderr = &logs[index]
		commands[index] = command
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for index, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("concurrent helper: %v\n%s", err, logs[index].String())
		}
	}
	outcomes := make([]ledgerOutcome, len(results))
	for index, path := range results {
		if err := json.Unmarshal([]byte(readFile(t, path)), &outcomes[index]); err != nil {
			t.Fatalf("decode helper outcome: %v", err)
		}
	}
	return outcomes
}

func TestConcurrentAcceptanceHelper(t *testing.T) {
	if os.Getenv("SKL_CONCURRENT_ACCEPT_HELPER") != "1" {
		return
	}
	var output bytes.Buffer
	factory := func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend("http://127.0.0.1:1", "secret", http.DefaultClient)
		backend.BindRepository(repository)
		return backend, nil
	}
	app := newApp(factory, bytes.NewReader(nil), &output, &output)
	arguments := []string{
		"skl", "ledger", "accept",
		"--repo", os.Getenv("SKL_CONCURRENT_ACCEPT_REPO"),
		"--proposal-dir", os.Getenv("SKL_CONCURRENT_ACCEPT_PROPOSAL"),
		"--format", "json",
	}
	if err := app.Run(arguments); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("SKL_CONCURRENT_ACCEPT_RESULT"), output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationBookkeepingPreservesConcurrentStagedWork(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	staged := false
	forge.before = func(method, path string) {
		if staged || method != http.MethodPost || path != "/repos/acme/widgets/issues" {
			return
		}
		staged = true
		if err := os.WriteFile(filepath.Join(fixture.clone, "human-private.txt"), []byte("human bytes\n"), 0o644); err != nil {
			panic(err)
		}
		if err := exec.Command("git", "-C", fixture.clone, "add", "human-private.txt").Run(); err != nil {
			panic(err)
		}
	}
	directory := writeProposal(t, "", singleSlice("concurrent-bookkeeping"))
	outcome := cli.accept(t, root, directory, issueFile(t, "foundation", "body\n"))
	if outcome.Status != "accepted" || outcome.Acceptance.BookkeepingStatus != nil {
		t.Fatalf("safe path-limited bookkeeping did not complete: %s", mustJSON(t, outcome))
	}
	if exec.Command("git", "-C", fixture.clone, "cat-file", "-e", "HEAD:human-private.txt").Run() == nil {
		t.Fatal("unrelated staged work was absorbed into a ledger commit")
	}
	if status := runGitOutput(t, fixture.clone, "status", "--porcelain"); !strings.Contains(status, "human-private.txt") {
		t.Fatalf("unrelated staged work was not preserved: %s", status)
	}
	if outcome.Acceptance.Slices[0].Issue == nil || forge.createdCount() != 1 {
		t.Fatalf("completed forge attachment was lost: %s", mustJSON(t, outcome))
	}
	state := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/concurrent-bookkeeping/foundation/state.json")
	if !strings.Contains(state, `"number": 101`) {
		t.Fatalf("successful attachment was not durable while unrelated work stayed staged: %s", state)
	}

	forge.before = nil
	runGit(t, fixture.clone, "reset", "--", "human-private.txt")
	if err := os.Remove(filepath.Join(fixture.clone, "human-private.txt")); err != nil {
		t.Fatal(err)
	}
	repeat := cli.accept(t, root, directory, issueFile(t, "foundation", "body\n"))
	if repeat.Status != "existing" || repeat.Acceptance.Slices[0].Issue == nil || forge.createdCount() != 1 {
		t.Fatalf("repair retry failed to adopt the completed attachment without duplication: %s", mustJSON(t, repeat))
	}
}

func TestLedgerShowUsesOneCommittedRecord(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	if outcome := cli.accept(t, root, writeProposal(t, "", singleSlice("committed-readback"))); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	directory := filepath.Join(fixture.clone, "projects", "widgets", "proposals", "committed-readback", "foundation")
	if err := os.Remove(filepath.Join(directory, "intent.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(directory, "state.json"), `{"state":"ready_for_implementation","title":"Add foundation","branch":"uncommitted-branch"}`)
	outcome, _ := cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--item", "committed-readback/foundation", "--format", "json"})
	if outcome.Status != "shown" || outcome.Readback.Branch != "foundation" || len(outcome.Readback.Documents) != 2 {
		t.Fatalf("readback mixed working-tree membership or state with HEAD: %s", mustJSON(t, outcome))
	}
	if !strings.Contains(outcome.Readback.Documents[1].Contents+outcome.Readback.Documents[0].Contents, "Manual verification") {
		t.Fatalf("committed human obligations disappeared: %s", mustJSON(t, outcome.Readback.Documents))
	}
}

func TestLedgerPushUsesCurrentBranchForTrackedDestination(t *testing.T) {
	fixture := newLedgerFixture(t)
	runGit(t, fixture.clone, "switch", "-q", "-c", "local-ledger", "--track", "origin/main")
	root := sourceRepository(t, "acme", "widgets")
	outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("tracked-destination")))
	if outcome.Status != "accepted" || outcome.Acceptance.Slices[0].PushStatus.Status != ledger.PushPushed {
		t.Fatalf("tracked destination was not pushed honestly: %s", mustJSON(t, outcome))
	}
	local := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))
	remote := strings.TrimSpace(runGitOutput(t, fixture.upstream, "rev-parse", "refs/heads/main"))
	if local != remote {
		t.Fatalf("reported push omitted accepted revision: local %s remote %s", local, remote)
	}
}

func TestLaterPublicationCompletesCurrentParentGrouping(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	creates := 0
	forge.fail = func(method, path string) int {
		if method == http.MethodPost && path == "/repos/acme/widgets/issues" {
			creates++
			if creates == 2 { // the second child
				return http.StatusUnprocessableEntity
			}
		}
		if method == http.MethodPost && strings.HasSuffix(path, "/sub_issues") {
			return http.StatusServiceUnavailable
		}
		return 0
	}
	directory := writeProposal(t, "", dualSlice("grouping-retry"))
	cli := newLedgerApp(t, forge)
	first := cli.accept(t, root, directory,
		"--issue=foundation="+writeTemp(t, t, "foundation body\n"),
		"--issue=feature="+writeTemp(t, t, "feature body\n"),
		"--parent-body="+writeTemp(t, t, "parent body\n"))
	if first.Status != "accepted" || first.Acceptance.ParentIssue == nil {
		t.Fatalf("parent was not established with one child: %s", mustJSON(t, first))
	}
	var attached, missing *ledger.SliceAcceptance
	for index := range first.Acceptance.Slices {
		if first.Acceptance.Slices[index].Issue != nil {
			attached = &first.Acceptance.Slices[index]
		} else {
			missing = &first.Acceptance.Slices[index]
		}
	}
	if attached == nil || missing == nil || attached.GroupingStatus == nil || attached.GroupingStatus.Status != ledger.IssueFailed {
		t.Fatalf("partial publication was not reported: %s", mustJSON(t, first))
	}
	for _, name := range []string{attached.Name, missing.Name} {
		if state := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/grouping-retry/"+name+"/state.json"); strings.Contains(state, `"grouping"`) || strings.Contains(state, `"publication"`) {
			t.Fatalf("grouping failure left a persisted retry record: %s", state)
		}
	}
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "ledger", "publish", "--repo", root, "--proposal", "grouping-retry"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cli.out.String(), "Parent grouping: failed") {
		t.Fatalf("Markdown hid the grouping failure: %s", cli.out.String())
	}

	// The missing child is attached later; the current parent grouping is
	// applied without recreating or replaying earlier objects.
	forge.mu.Lock()
	forge.fail = nil
	forge.mu.Unlock()
	parent := first.Acceptance.ParentIssue.Number
	repeat, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "grouping-retry", "--format", "json",
		"--issue=" + missing.Name + "=" + writeTemp(t, t, "fresh child body\n")})
	if repeat.Publication.ParentIssue == nil || repeat.Publication.ParentIssue.Number != parent || forge.createdCount() != 3 {
		t.Fatalf("later publication did not reuse the parent and create only the missing child: %s (%d creates)", mustJSON(t, repeat), forge.createdCount())
	}
	if updates := forge.updatedIssues(); len(updates) != 0 {
		t.Fatalf("publication without prose updated established issues: %v", updates)
	}
	forge.mu.Lock()
	grouped := append([]int(nil), forge.children[parent]...)
	forge.mu.Unlock()
	if len(grouped) != 2 {
		t.Fatalf("current grouping has %v, want both children under #%d", grouped, parent)
	}
	for _, slice := range repeat.Publication.Slices {
		if slice.Issue == nil || slice.GroupingStatus != nil {
			t.Fatalf("current grouping stayed incomplete: %s", mustJSON(t, repeat))
		}
	}
}

func TestFreshPublicationDoesNotAdoptPreexistingMatch(t *testing.T) {
	newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	forge.list = append(forge.list, map[string]any{
		"id": 1777, "number": 777, "state": "open", "title": "Add foundation", "body": "same body\n",
	})
	outcome := newLedgerApp(t, forge).accept(t, root, writeProposal(t, "", singleSlice("fresh-publication")), issueFile(t, "foundation", "same body\n"))
	issue := outcome.Acceptance.Slices[0].Issue
	if issue == nil || issue.Number == 777 || forge.createdCount() != 1 {
		t.Fatalf("fresh publication adopted a pre-existing forge record: %s", mustJSON(t, outcome))
	}
}

func TestPushRaceRequiresReconciliation(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	competitor := filepath.Join(t.TempDir(), "competitor")
	runGit(t, t.TempDir(), "clone", "-q", fixture.upstream, competitor)
	runGit(t, competitor, "config", "user.name", "Other")
	runGit(t, competitor, "config", "user.email", "other@example.com")
	writeFile(t, filepath.Join(competitor, "competing.txt"), "history\n")
	runGit(t, competitor, "add", "competing.txt")
	runGit(t, competitor, "commit", "-q", "-m", "competing history")
	competitorHead := strings.TrimSpace(runGitOutput(t, competitor, "rev-parse", "HEAD"))
	hook := filepath.Join(fixture.clone, ".git", "hooks", "pre-push")
	writeFile(t, hook, fmt.Sprintf("#!/bin/sh\nrm -f \"$0\"\ngit -C %q push -q origin main\n", competitor))
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}
	outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("push-race")))
	if outcome.Acceptance.Slices[0].PushStatus.Status != ledger.PushReconciliation {
		t.Fatalf("push race was misclassified: %s", mustJSON(t, outcome))
	}
	if remote := strings.TrimSpace(runGitOutput(t, fixture.upstream, "rev-parse", "refs/heads/main")); remote != competitorHead {
		t.Fatalf("competing remote history was not preserved: got %s want %s", remote, competitorHead)
	}
	state := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/push-race/foundation/state.json")
	if !strings.Contains(state, ledger.PushReconciliation) {
		t.Fatalf("reconciliation requirement was not durable: %s", state)
	}
}

func TestSuccessfulPushDescendantStillContainsAcceptedRevision(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	hook := filepath.Join(fixture.upstream, "hooks", "post-receive")
	script := fmt.Sprintf(`#!/bin/sh
read old new ref
rm -f "$0"
tree=$(git --git-dir=%q rev-parse "$new^{tree}")
competing=$(printf 'post-push competition\n' | GIT_AUTHOR_NAME=Other GIT_AUTHOR_EMAIL=other@example.com GIT_COMMITTER_NAME=Other GIT_COMMITTER_EMAIL=other@example.com git --git-dir=%q commit-tree "$tree" -p "$new")
git --git-dir=%q update-ref "$ref" "$competing" "$new"
`, fixture.upstream, fixture.upstream, fixture.upstream)
	writeFile(t, hook, script)
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}
	outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("post-push-race")), issueFile(t, "foundation", "body\n"))
	push := outcome.Acceptance.Slices[0].PushStatus
	if push.Status != ledger.PushPushed {
		t.Fatalf("remote descendant containing the accepted revision was not counted as replicated: %s", mustJSON(t, outcome))
	}
	if !strings.Contains(push.Detail, ledger.PushReconciliation) {
		t.Fatalf("the later competing bookkeeping push was not distinguished: %s", mustJSON(t, outcome))
	}
	state := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/post-push-race/foundation/state.json")
	if !strings.Contains(state, ledger.PushReconciliation) {
		t.Fatalf("bookkeeping reconciliation was not durable: %s", state)
	}
}

func TestBookkeepingPushRaceIsDurablyReconciliationRequired(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	competitor := filepath.Join(t.TempDir(), "competitor")
	runGit(t, t.TempDir(), "clone", "-q", fixture.upstream, competitor)
	runGit(t, competitor, "config", "user.name", "Other")
	runGit(t, competitor, "config", "user.email", "other@example.com")
	counter := filepath.Join(t.TempDir(), "first-push-complete")
	competingFile := filepath.Join(competitor, "bookkeeping-race.txt")
	hook := filepath.Join(fixture.clone, ".git", "hooks", "pre-push")
	script := fmt.Sprintf(`#!/bin/sh
if [ ! -f %q ]; then
  : > %q
  exit 0
fi
git -C %q fetch -q origin main
git -C %q reset -q --hard origin/main
printf 'competing bookkeeping history\n' > %q
git -C %q add bookkeeping-race.txt
git -C %q commit -q -m 'competing bookkeeping history'
git -C %q push -q origin main
rm -f "$0"
`, counter, counter, competitor, competitor, competingFile, competitor, competitor, competitor)
	writeFile(t, hook, script)
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}
	outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("bookkeeping-race")), issueFile(t, "foundation", "body\n"))
	push := outcome.Acceptance.Slices[0].PushStatus
	if push == nil || push.Status != ledger.PushPushed || !strings.Contains(push.Detail, ledger.PushReconciliation) {
		t.Fatalf("bookkeeping race was not reported distinctly from accepted-record replication: %s", mustJSON(t, outcome))
	}
	state := readLedgerFile(t, fixture.clone, "projects/widgets/proposals/bookkeeping-race/foundation/state.json")
	if !strings.Contains(state, `"status": "reconciliation_required"`) {
		t.Fatalf("bookkeeping reconciliation was not durable: %s", state)
	}
}

func TestMalformedDocumentsAndGitInvalidBranchAreRefusedWhole(t *testing.T) {
	t.Run("configuration trailing value", func(t *testing.T) {
		fixture := newLedgerFixture(t)
		root := sourceRepository(t, "acme", "widgets")
		fixture.misconfigure(t, `{"ledger":"`+fixture.clone+`"} trailing`)
		before := ledgerSnapshot(t, fixture.clone)
		outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("bad-config")))
		if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "malformed") || ledgerSnapshot(t, fixture.clone) != before {
			t.Fatalf("trailing configuration was accepted: %s", mustJSON(t, outcome))
		}
	})
	t.Run("declaration trailing value", func(t *testing.T) {
		fixture := newLedgerFixture(t)
		root := sourceRepository(t, "acme", "widgets")
		directory := writeProposal(t, "", singleSlice("bad-declaration"))
		path := filepath.Join(directory, "proposal.json")
		writeFile(t, path, readFileString(t, path)+` {"extra":true}`)
		before := ledgerSnapshot(t, fixture.clone)
		outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, directory)
		if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "malformed") || ledgerSnapshot(t, fixture.clone) != before {
			t.Fatalf("trailing declaration was accepted: %s", mustJSON(t, outcome))
		}
	})
	t.Run("Git-invalid branch", func(t *testing.T) {
		fixture := newLedgerFixture(t)
		root := sourceRepository(t, "acme", "widgets")
		spec := singleSlice("bad-branch")
		spec.slices[0].branch = "bad."
		before := ledgerSnapshot(t, fixture.clone)
		outcome := newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", spec))
		if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "invalid planned branch") || ledgerSnapshot(t, fixture.clone) != before {
			t.Fatalf("Git-invalid branch was accepted: %s", mustJSON(t, outcome))
		}
	})
}

func TestProposePublishRefusedForAdoptedProjects(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := proposalRepository(t)
	prepareSlice(t, root, "legacy-publish")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	if outcome := cli.accept(t, root, writeProposal(t, "", singleSlice("adopted-publish"))); outcome.Status != "accepted" {
		t.Fatalf("adoption acceptance failed: %s", mustJSON(t, outcome))
	}
	if _, err := os.Stat(filepath.Join(fixture.clone, "projects", "widgets", "proposals", "adopted-publish")); err != nil {
		t.Fatalf("adoption not recorded: %v", err)
	}
	before := ledgerSnapshot(t, root)
	created := forge.createdCount()
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "propose", "publish", "--repo", root, "--target", "main", "--slice", proposalSliceFlag(t, "legacy-publish")}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(cli.out.String(), "Status: unsupported") || !strings.Contains(cli.out.String(), "propose publish") {
		t.Fatalf("legacy publication was not gated: %s", cli.out.String())
	}
	if ledgerSnapshot(t, root) != before || forge.createdCount() != created {
		t.Fatalf("gated legacy publication mutated source or forge state")
	}
}

// requireLedgerUnlocked fails when the ledger mutation lock is held, which
// would mean forge I/O happens inside a local mutation.
func requireLedgerUnlocked(t *testing.T, clone string) {
	t.Helper()
	lock, err := os.OpenFile(filepath.Join(clone, ".git", "skl-ledger.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Errorf("open the ledger mutation lock: %v", err)
		return
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Errorf("forge I/O ran while the ledger mutation lock was held: %v", err)
		return
	}
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
}

// commitLedgerRecord overwrites one ledger record and commits it as another
// local writer would.
func commitLedgerRecord(t *testing.T, clone, path, contents string) {
	t.Helper()
	writeFile(t, filepath.Join(clone, path), contents)
	runGit(t, clone, "add", path)
	runGit(t, clone, "commit", "-q", "-m", "local writer "+path)
}

func decodeRecord(t *testing.T, contents string) map[string]any {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal([]byte(contents), &record); err != nil {
		t.Fatalf("decode record %q: %v", contents, err)
	}
	return record
}

func TestExplicitPublicationAfterInterruptedAttemptAndLocalProgress(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	if outcome := cli.accept(t, root, writeProposal(t, "", dualSlice("interrupted-work"))); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	// An earlier publisher exited after reserving issue and parent creation
	// and before recording any attachment.
	proposalPath := "projects/widgets/proposals/interrupted-work/"
	for _, slice := range []string{"feature", "foundation"} {
		record := decodeRecord(t, readLedgerFile(t, fixture.clone, proposalPath+slice+"/state.json"))
		record["publication"] = map[string]any{
			"issue":    map[string]any{"status": "reserved", "detail": "issue creation is reserved before network publication"},
			"grouping": map[string]any{"status": "pending", "detail": "grouping under the parent issue failed"},
		}
		commitLedgerRecord(t, fixture.clone, proposalPath+slice+"/state.json", mustJSON(t, record)+"\n")
	}
	meta := decodeRecord(t, readLedgerFile(t, fixture.clone, proposalPath+"proposal.json"))
	meta["parent_publication"] = map[string]any{"status": "reserved", "detail": "parent issue creation is reserved before network publication"}
	commitLedgerRecord(t, fixture.clone, proposalPath+"proposal.json", mustJSON(t, meta)+"\n")

	// Local work advances without waiting for publication.
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "implement", "next", "--repo", root, "--format", "json"}); err != nil || !strings.Contains(cli.out.String(), "work_available") {
		t.Fatalf("local work did not advance: %v\n%s", err, cli.out.String())
	}
	before := map[string]map[string]any{}
	for _, slice := range []string{"feature", "foundation"} {
		before[slice] = decodeRecord(t, readLedgerFile(t, fixture.clone, proposalPath+slice+"/state.json"))
	}
	head := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))

	requests := 0
	forge.before = func(string, string) {
		requests++
		requireLedgerUnlocked(t, fixture.clone)
	}
	publication, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "interrupted-work", "--format", "json",
		"--issue=foundation=" + writeTemp(t, t, "current foundation prose\n"),
		"--issue=feature=" + writeTemp(t, t, "current feature prose\n"),
		"--parent-body=" + writeTemp(t, t, "current parent prose\n")})
	if publication.Status != "attempted" || publication.Publication.ParentIssue == nil {
		t.Fatalf("old reservations blocked current publication: %s", mustJSON(t, publication))
	}
	for _, slice := range publication.Publication.Slices {
		if slice.Issue == nil || slice.IssueStatus.Status != ledger.IssueCreated || slice.GroupingStatus != nil {
			t.Fatalf("current presentation was not published: %s", mustJSON(t, publication))
		}
	}
	if forge.createdCount() != 3 {
		t.Fatalf("publication created %d issues, want the current children and parent", forge.createdCount())
	}
	if requests == 0 {
		t.Fatal("no forge request probed the ledger mutation lock")
	}

	// Publication changed only established identities: Claims, lifecycle,
	// Contracts, and reports are untouched, and no attempt tracking remains.
	changed := strings.Fields(runGitOutput(t, fixture.clone, "diff", "--name-only", head, "HEAD"))
	for _, path := range changed {
		if !strings.HasSuffix(path, "/state.json") && !strings.HasSuffix(path, "/proposal.json") {
			t.Fatalf("publication changed %s", path)
		}
	}
	for _, slice := range []string{"feature", "foundation"} {
		after := decodeRecord(t, readLedgerFile(t, fixture.clone, proposalPath+slice+"/state.json"))
		if _, found := after["issue"]; !found {
			t.Fatalf("%s attachment was not recorded: %v", slice, after)
		}
		if _, found := after["publication"]; found {
			t.Fatalf("%s kept publication tracking: %v", slice, after)
		}
		delete(after, "issue")
		delete(before[slice], "publication")
		if mustJSON(t, after) != mustJSON(t, before[slice]) {
			t.Fatalf("publication changed local state of %s:\nbefore %s\nafter  %s", slice, mustJSON(t, before[slice]), mustJSON(t, after))
		}
	}
	proposal := readLedgerFile(t, fixture.clone, proposalPath+"proposal.json")
	if !strings.Contains(proposal, `"parent_issue"`) || strings.Contains(proposal, `"parent_publication"`) {
		t.Fatalf("parent record kept attempt tracking or lost its attachment: %s", proposal)
	}
}

func TestPublicationReportsSupersededAndConflictingAttachments(t *testing.T) {
	t.Run("superseded selection", func(t *testing.T) {
		fixture := newLedgerFixture(t)
		root := sourceRepository(t, "acme", "widgets")
		forge := newForgeServer(t)
		cli := newLedgerApp(t, forge)
		cli.accept(t, root, writeProposal(t, "", dualSlice("superseded-work")))
		featurePath := "projects/widgets/proposals/superseded-work/foundation/state.json"
		// While the first slice is being created, another local writer
		// establishes the second slice's issue.
		forge.before = func(method, path string) {
			if method != http.MethodPost || path != "/repos/acme/widgets/issues" || forge.before == nil {
				return
			}
			forge.before = nil
			record := decodeRecord(t, readLedgerFile(t, fixture.clone, featurePath))
			record["issue"] = map[string]any{"repository": "acme/widgets", "number": 900}
			commitLedgerRecord(t, fixture.clone, featurePath, mustJSON(t, record)+"\n")
		}
		outcome, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "superseded-work", "--format", "json",
			"--issue=feature=" + writeTemp(t, t, "feature prose\n"),
			"--issue=foundation=" + writeTemp(t, t, "foundation prose\n")})
		if forge.createdCount() != 1 || len(forge.updatedIssues()) != 0 {
			t.Fatalf("a mutation was sent for a superseded selection: %d creates, updates %v", forge.createdCount(), forge.updatedIssues())
		}
		var superseded *ledger.SliceAcceptance
		for index := range outcome.Publication.Slices {
			if outcome.Publication.Slices[index].Name == "foundation" {
				superseded = &outcome.Publication.Slices[index]
			}
		}
		if superseded == nil || superseded.IssueStatus == nil || superseded.IssueStatus.Status != ledger.IssueSuperseded || superseded.Issue == nil || superseded.Issue.Number != 900 {
			t.Fatalf("superseded selection was not reported: %s", mustJSON(t, outcome))
		}
		if state := readLedgerFile(t, fixture.clone, featurePath); mustJSON(t, decodeRecord(t, state)["issue"]) != `{"number":900,"repository":"acme/widgets"}` {
			t.Fatalf("the newer local attachment was overwritten: %s", state)
		}
	})

	t.Run("conflicting attachment", func(t *testing.T) {
		fixture := newLedgerFixture(t)
		root := sourceRepository(t, "acme", "widgets")
		forge := newForgeServer(t)
		cli := newLedgerApp(t, forge)
		cli.accept(t, root, writeProposal(t, "", singleSlice("conflicting-work")))
		statePath := "projects/widgets/proposals/conflicting-work/foundation/state.json"
		forge.before = func(method, path string) {
			if method != http.MethodPost || path != "/repos/acme/widgets/issues" {
				return
			}
			record := decodeRecord(t, readLedgerFile(t, fixture.clone, statePath))
			record["issue"] = map[string]any{"repository": "acme/widgets", "number": 900}
			commitLedgerRecord(t, fixture.clone, statePath, mustJSON(t, record)+"\n")
		}
		outcome, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "conflicting-work", "--format", "json",
			"--issue=foundation=" + writeTemp(t, t, "foundation prose\n")})
		slice := outcome.Publication.Slices[0]
		if slice.IssueStatus == nil || slice.IssueStatus.Status != ledger.IssueConflict || slice.Issue == nil || slice.Issue.Number != 900 || !strings.Contains(slice.IssueStatus.Detail, "#101") {
			t.Fatalf("conflicting attachment was not reported: %s", mustJSON(t, slice))
		}
		if state := readLedgerFile(t, fixture.clone, statePath); mustJSON(t, decodeRecord(t, state)["issue"]) != `{"number":900,"repository":"acme/widgets"}` {
			t.Fatalf("established attachment was replaced: %s", state)
		}
	})
}

// publishedSlice returns the named slice outcome of a publication.
func publishedSlice(t *testing.T, outcome *ledger.Acceptance, name string) *ledger.SliceAcceptance {
	t.Helper()
	for index := range outcome.Slices {
		if outcome.Slices[index].Name == name {
			return &outcome.Slices[index]
		}
	}
	t.Fatalf("publication lacks slice %s: %s", name, mustJSON(t, outcome))
	return nil
}

func TestGroupingRevalidatesSelectedRecords(t *testing.T) {
	const proposalPath = "projects/widgets/proposals/group-check/"
	clearIssue := func(slice string, issue any) func(t *testing.T, clone string) string {
		return func(t *testing.T, clone string) string {
			path := proposalPath + slice + "/state.json"
			record := decodeRecord(t, readLedgerFile(t, clone, path))
			if issue == nil {
				delete(record, "issue")
			} else {
				record["issue"] = issue
			}
			commitLedgerRecord(t, clone, path, mustJSON(t, record)+"\n")
			return path
		}
	}
	for _, tc := range []struct {
		name string
		// change is committed by another local writer once the parent
		// grouping has been observed and before any grouping write; it
		// returns the record it changed.
		change func(t *testing.T, clone string) string
		// stopped lists the slices whose grouping write must not be sent,
		// each reported with status; every other child is grouped.
		stopped []string
		status  string
	}{
		{"superseded child", clearIssue("feature", map[string]any{"repository": "acme/widgets", "number": 900}), []string{"feature"}, ledger.IssueSuperseded},
		{"removed child", clearIssue("feature", nil), []string{"feature"}, ledger.IssueSuperseded},
		{"removed parent", func(t *testing.T, clone string) string {
			record := decodeRecord(t, readLedgerFile(t, clone, proposalPath+"proposal.json"))
			delete(record, "parent_issue")
			commitLedgerRecord(t, clone, proposalPath+"proposal.json", mustJSON(t, record)+"\n")
			return proposalPath + "proposal.json"
		}, []string{"feature", "foundation"}, ledger.IssueSuperseded},
		{"unreadable parent", func(t *testing.T, clone string) string {
			commitLedgerRecord(t, clone, proposalPath+"proposal.json", "{\n")
			return proposalPath + "proposal.json"
		}, []string{"feature", "foundation"}, ledger.IssueFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			root := sourceRepository(t, "acme", "widgets")
			forge := newForgeServer(t)
			cli := newLedgerApp(t, forge)
			accepted := cli.accept(t, root, writeProposal(t, "", dualSlice("group-check")),
				issueFile(t, "feature", "feature\n"), issueFile(t, "foundation", "foundation\n"), "--parent-body="+writeTemp(t, t, "parent\n"))
			if accepted.Acceptance == nil || accepted.Acceptance.ParentIssue == nil {
				t.Fatalf("setup acceptance did not publish a parent: %s", mustJSON(t, accepted))
			}
			parent := accepted.Acceptance.ParentIssue.Number
			var wantGrouped []int
			for _, slice := range accepted.Acceptance.Slices {
				if !slices.Contains(tc.stopped, slice.Name) {
					wantGrouped = append(wantGrouped, slice.Issue.Number)
				}
			}
			var changedPath, changedRecord string
			forge.mu.Lock()
			forge.children[parent] = nil
			forge.before = func(method, path string) {
				if changedPath == "" && method == http.MethodGet && strings.HasSuffix(path, "/sub_issues") {
					changedPath = tc.change(t, fixture.clone)
					changedRecord = readLedgerFile(t, fixture.clone, changedPath)
				}
			}
			forge.mu.Unlock()

			outcome, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "group-check", "--format", "json"})
			forge.mu.Lock()
			grouped := append([]int(nil), forge.children[parent]...)
			forge.mu.Unlock()
			if !slices.Equal(grouped, wantGrouped) {
				t.Fatalf("grouped %v, want %v", grouped, wantGrouped)
			}
			for _, name := range tc.stopped {
				if slice := publishedSlice(t, outcome.Publication, name); slice.GroupingStatus == nil || slice.GroupingStatus.Status != tc.status {
					t.Fatalf("stopped grouping of %s was not reported as %s: %s", name, tc.status, mustJSON(t, outcome))
				}
			}
			// The concurrent writer's record is kept: bookkeeping neither
			// restores a removed attachment nor replaces a newer one.
			if record := readLedgerFile(t, fixture.clone, changedPath); record != changedRecord {
				t.Fatalf("publication rewrote the concurrently changed %s:\n%s\nwant\n%s", changedPath, record, changedRecord)
			}
		})
	}

	t.Run("new child superseded before grouping", func(t *testing.T) {
		fixture := newLedgerFixture(t)
		root := sourceRepository(t, "acme", "widgets")
		forge := newForgeServer(t)
		cli := newLedgerApp(t, forge)
		accepted := cli.accept(t, root, writeProposal(t, "", dualSlice("new-child-check")),
			issueFile(t, "foundation", "foundation\n"), "--parent-body="+writeTemp(t, t, "parent\n"))
		if accepted.Acceptance == nil || accepted.Acceptance.ParentIssue == nil {
			t.Fatalf("setup acceptance did not publish a parent: %s", mustJSON(t, accepted))
		}
		parent := accepted.Acceptance.ParentIssue.Number
		statePath := "projects/widgets/proposals/new-child-check/feature/state.json"
		// While this invocation creates the missing child, another local
		// writer establishes a different one.
		forge.mu.Lock()
		forge.before = func(method, path string) {
			if method != http.MethodPost || path != "/repos/acme/widgets/issues" {
				return
			}
			record := decodeRecord(t, readLedgerFile(t, fixture.clone, statePath))
			record["issue"] = map[string]any{"repository": "acme/widgets", "number": 900}
			commitLedgerRecord(t, fixture.clone, statePath, mustJSON(t, record)+"\n")
		}
		forge.mu.Unlock()
		outcome, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "new-child-check", "--format", "json", issueFile(t, "feature", "feature\n")})
		forge.mu.Lock()
		grouped := append([]int(nil), forge.children[parent]...)
		forge.mu.Unlock()
		if len(grouped) != 1 || grouped[0] != publishedSlice(t, outcome.Publication, "foundation").Issue.Number {
			t.Fatalf("grouped %v; the new child must not be grouped after a different attachment was recorded", grouped)
		}
		feature := publishedSlice(t, outcome.Publication, "feature")
		if feature.GroupingStatus == nil || feature.GroupingStatus.Status != ledger.IssueSuperseded || feature.IssueStatus == nil || feature.IssueStatus.Status != ledger.IssueConflict || feature.Issue.Number != 900 {
			t.Fatalf("new child supersession was not reported: %s", mustJSON(t, feature))
		}
		if state := readLedgerFile(t, fixture.clone, statePath); mustJSON(t, decodeRecord(t, state)["issue"]) != `{"number":900,"repository":"acme/widgets"}` {
			t.Fatalf("the concurrently recorded attachment was replaced: %s", state)
		}
	})
}

func TestUpdateRetryStopsAfterSupersession(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	accepted := cli.accept(t, root, writeProposal(t, "", singleSlice("retry-check")), issueFile(t, "foundation", "original\n"))
	if accepted.Acceptance == nil || accepted.Acceptance.Slices[0].Issue == nil {
		t.Fatalf("setup acceptance did not attach an issue: %s", mustJSON(t, accepted))
	}
	statePath := "projects/widgets/proposals/retry-check/foundation/state.json"
	// The first update fails transiently, and another local writer records a
	// different attachment before the adapter would retry it.
	patches := 0
	forge.mu.Lock()
	forge.fail = func(method, path string) int {
		if method != http.MethodPatch {
			return 0
		}
		if patches++; patches > 1 {
			return 0
		}
		record := decodeRecord(t, readLedgerFile(t, fixture.clone, statePath))
		record["issue"] = map[string]any{"repository": "acme/widgets", "number": 900}
		commitLedgerRecord(t, fixture.clone, statePath, mustJSON(t, record)+"\n")
		return http.StatusServiceUnavailable
	}
	forge.mu.Unlock()
	outcome, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "retry-check", "--format", "json", issueFile(t, "foundation", "current\n")})
	forge.mu.Lock()
	sent := patches
	forge.mu.Unlock()
	if sent != 1 || len(forge.updatedIssues()) != 0 {
		t.Fatalf("sent %d updates although the selection was superseded after the first; updated %v", sent, forge.updatedIssues())
	}
	slice := outcome.Publication.Slices[0]
	if slice.IssueStatus == nil || slice.IssueStatus.Status != ledger.IssueSuperseded || slice.Issue == nil || slice.Issue.Number != 900 {
		t.Fatalf("stopped update was not reported as superseded: %s", mustJSON(t, slice))
	}
	if state := readLedgerFile(t, fixture.clone, statePath); mustJSON(t, decodeRecord(t, state)["issue"]) != `{"number":900,"repository":"acme/widgets"}` {
		t.Fatalf("the newer local attachment was overwritten: %s", state)
	}
}

func TestPublicationRefusesAttachmentsOfAnotherRepository(t *testing.T) {
	foreignIssue := map[string]any{"repository": "other/repository", "number": 777}
	t.Run("issue and parent", func(t *testing.T) {
		fixture := newLedgerFixture(t)
		root := sourceRepository(t, "acme", "widgets")
		forge := newForgeServer(t)
		cli := newLedgerApp(t, forge)
		cli.accept(t, root, writeProposal(t, "", dualSlice("identity-check")))
		proposalPath := "projects/widgets/proposals/identity-check/"
		record := decodeRecord(t, readLedgerFile(t, fixture.clone, proposalPath+"foundation/state.json"))
		record["issue"] = foreignIssue
		commitLedgerRecord(t, fixture.clone, proposalPath+"foundation/state.json", mustJSON(t, record)+"\n")
		meta := decodeRecord(t, readLedgerFile(t, fixture.clone, proposalPath+"proposal.json"))
		meta["parent_issue"] = map[string]any{"repository": "other/repository", "number": 778}
		commitLedgerRecord(t, fixture.clone, proposalPath+"proposal.json", mustJSON(t, meta)+"\n")
		// Unrelated same-numbered issues exist in the selected repository.
		forge.mu.Lock()
		for _, number := range []int{777, 778} {
			forge.issues = append(forge.issues, map[string]any{"id": number + 1000, "number": number, "title": "Unrelated", "body": "Do not overwrite"})
		}
		forge.list = append(forge.list, forge.issues...)
		forge.mu.Unlock()
		head := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))

		outcome, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "identity-check", "--format", "json",
			issueFile(t, "foundation", "current\n"), "--parent-body=" + writeTemp(t, t, "parent\n")})
		if len(forge.updatedIssues()) != 0 {
			t.Fatalf("publication mutated a same-numbered issue: updated %v", forge.updatedIssues())
		}
		if forge.createdCount() != 2 {
			t.Fatalf("publication created %d issues, want only the two seeded unrelated ones", forge.createdCount())
		}
		for _, body := range forge.receivedBodies() {
			if strings.Contains(body, "sub_issue_id") {
				t.Fatalf("grouping was sent under a parent of another repository: %s", body)
			}
		}
		foundation := publishedSlice(t, outcome.Publication, "foundation")
		if foundation.IssueStatus == nil || foundation.IssueStatus.Status != ledger.IssueConflict || !strings.Contains(foundation.IssueStatus.Detail, "other/repository#777") || mustJSON(t, foundation.Issue) != `{"repository":"other/repository","number":777}` {
			t.Fatalf("foreign issue attachment was not refused: %s", mustJSON(t, foundation))
		}
		if foundation.GroupingStatus == nil || foundation.GroupingStatus.Status != ledger.IssueConflict || outcome.Publication.ParentNote == nil || outcome.Publication.ParentNote.Status != ledger.IssueConflict {
			t.Fatalf("foreign parent attachment was not refused: %s", mustJSON(t, outcome))
		}
		if changed := runGitOutput(t, fixture.clone, "diff", head, "HEAD", "--", proposalPath+"foundation/state.json", proposalPath+"proposal.json"); strings.Contains(changed, "other/repository") {
			t.Fatalf("the recorded associations changed:\n%s", changed)
		}
	})

	t.Run("grouped child", func(t *testing.T) {
		fixture := newLedgerFixture(t)
		root := sourceRepository(t, "acme", "widgets")
		forge := newForgeServer(t)
		cli := newLedgerApp(t, forge)
		accepted := cli.accept(t, root, writeProposal(t, "", dualSlice("child-identity")),
			issueFile(t, "feature", "feature\n"), issueFile(t, "foundation", "foundation\n"), "--parent-body="+writeTemp(t, t, "parent\n"))
		parent := accepted.Acceptance.ParentIssue.Number
		statePath := "projects/widgets/proposals/child-identity/foundation/state.json"
		record := decodeRecord(t, readLedgerFile(t, fixture.clone, statePath))
		record["issue"] = foreignIssue
		commitLedgerRecord(t, fixture.clone, statePath, mustJSON(t, record)+"\n")
		forge.mu.Lock()
		forge.children[parent] = nil
		forge.mu.Unlock()

		outcome, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--proposal", "child-identity", "--format", "json"})
		forge.mu.Lock()
		grouped := append([]int(nil), forge.children[parent]...)
		forge.mu.Unlock()
		if feature := publishedSlice(t, outcome.Publication, "feature").Issue.Number; !slices.Equal(grouped, []int{feature}) {
			t.Fatalf("grouped %v; want only the in-repository child #%d", grouped, feature)
		}
		if foundation := publishedSlice(t, outcome.Publication, "foundation"); foundation.GroupingStatus == nil || foundation.GroupingStatus.Status != ledger.IssueConflict {
			t.Fatalf("foreign child grouping was not refused: %s", mustJSON(t, foundation))
		}
	})
}

func TestMissingProseGuidesFreshAuthoring(t *testing.T) {
	fixture := newLedgerFixture(t)
	// A fork checkout selects the upstream Project explicitly; every pointer
	// must keep that selection rather than fall back to origin.
	root := sourceRepository(t, "acme", "widgets")
	runGit(t, root, "remote", "rename", "origin", "upstream")
	runGit(t, root, "remote", "add", "origin", "git@github.com:other/widgets.git")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	outcome := cli.accept(t, root, writeProposal(t, "", dualSlice("lost-prose")), "--remote", "upstream")
	if outcome.Status != "accepted" || forge.createdCount() != 0 {
		t.Fatalf("acceptance without prose published or failed: %s", mustJSON(t, outcome))
	}
	for _, slice := range outcome.Acceptance.Slices {
		if slice.IssueStatus == nil || slice.IssueStatus.Status != ledger.IssueMissingInput {
			t.Fatalf("missing prose was not reported: %s", mustJSON(t, outcome))
		}
	}
	root = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "--show-toplevel"))
	selection := "--repo '" + root + "' --remote 'upstream'"
	want := &proseAuthoring{
		Readback: []string{
			"skl ledger show " + selection + " --item 'lost-prose/feature'",
			"skl ledger show " + selection + " --item 'lost-prose/foundation'",
		},
		Guidance:     "skl skill --resource reference/issue-publication.md --input 'proposal=lost-prose' --input 'repo=" + root + "' --input 'remote=upstream' propose",
		Continuation: "skl ledger publish " + selection + " --proposal 'lost-prose' --issue 'feature='<body-file> --issue 'foundation='<body-file> --parent-body <parent-body-file>",
	}
	if mustJSON(t, outcome.Authoring) != mustJSON(t, want) {
		t.Fatalf("authoring pointers = %s\nwant %s", mustJSON(t, outcome.Authoring), mustJSON(t, want))
	}
	// The readback pointer, run with exactly its bound arguments, reads the
	// selected upstream Project.
	readback, _ := cli.run(t, []string{"skl", "ledger", "show", "--repo", root, "--remote", "upstream", "--item", "lost-prose/feature", "--format", "json"})
	if readback.Status != "shown" {
		t.Fatalf("authoring readback cannot read the selected Project: %s", mustJSON(t, readback))
	}
	// The guidance renders with the continuation arguments bound.
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "skill", "--resource", "reference/issue-publication.md", "--input", "proposal=lost-prose", "--input", "repo=" + root, "--input", "remote=upstream", "propose"}); err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{
		"skl ledger show " + selection + " --item 'lost-prose/<slice>'",
		"skl ledger publish " + selection + " --proposal 'lost-prose'",
	} {
		if !strings.Contains(cli.out.String(), wanted) {
			t.Fatalf("authoring guidance lacks %q:\n%s", wanted, cli.out.String())
		}
	}
	cli.out.Reset()
	if err := cli.app.Run([]string{"skl", "ledger", "publish", "--repo", root, "--remote", "upstream", "--proposal", "lost-prose"}); err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"Issue: missing_input", "  Guidance: " + want.Guidance, "  Then publish: " + want.Continuation} {
		if !strings.Contains(cli.out.String(), wanted) {
			t.Fatalf("Markdown lacks %q:\n%s", wanted, cli.out.String())
		}
	}

	// Freshly authored prose is published verbatim: no accepted document,
	// report, or other private evidence is appended, and nothing is stored.
	prose := map[string]string{"feature": "Fresh feature prose.\n", "foundation": "Fresh foundation prose.\n", "parent": "Fresh parent prose.\n"}
	published, _ := cli.run(t, []string{"skl", "ledger", "publish", "--repo", root, "--remote", "upstream", "--proposal", "lost-prose", "--format", "json",
		"--issue=feature=" + writeTemp(t, t, prose["feature"]),
		"--issue=foundation=" + writeTemp(t, t, prose["foundation"]),
		"--parent-body=" + writeTemp(t, t, prose["parent"])})
	if published.Authoring != nil || forge.createdCount() != 3 {
		t.Fatalf("supplied prose was not published: %s", mustJSON(t, published))
	}
	forge.mu.Lock()
	for _, issue := range forge.issues {
		body := issue["body"].(string)
		if body != prose["feature"] && body != prose["foundation"] && body != prose["parent"] {
			t.Errorf("forge received altered prose: %q", body)
		}
	}
	forge.mu.Unlock()
	for _, path := range ledgerPaths(t, fixture.clone) {
		if contents := readLedgerFile(t, fixture.clone, path); strings.Contains(contents, "Fresh ") {
			t.Fatalf("ledger persisted public prose in %s", path)
		}
	}
}

func TestExplicitPublicationRefusesUnselectableInputs(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	forge := newForgeServer(t)
	cli := newLedgerApp(t, forge)
	cli.accept(t, root, writeProposal(t, "", singleSlice("selected-work")))
	head := strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD"))
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--proposal", "unknown-work"}, "no accepted proposal unknown-work"},
		{[]string{"--proposal", "selected-work", "--issue=other=" + writeTemp(t, t, "prose\n")}, "not a slice of proposal selected-work"},
		{[]string{"--proposal", "selected-work", "--parent-body=" + writeTemp(t, t, "prose\n")}, "single-slice proposals take no parent body"},
	} {
		outcome, _ := cli.run(t, append([]string{"skl", "ledger", "publish", "--repo", root, "--format", "json"}, tc.args...))
		if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, tc.want) {
			t.Fatalf("publish %v = %s, want refusal %q", tc.args, mustJSON(t, outcome), tc.want)
		}
	}
	if forge.createdCount() != 0 || strings.TrimSpace(runGitOutput(t, fixture.clone, "rev-parse", "HEAD")) != head {
		t.Fatal("refused publication touched the forge or the ledger")
	}
}
