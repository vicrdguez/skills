package main

// Controlled GitHub-shaped HTTP fixture for the public publication CLI tests.
// It serves exactly the recovery surfaces the real adapter uses: descriptive
// issues, parent grouping, pull requests, reviewed diffs, inline comments, and
// the native readiness GraphQL mutation. It is deliberately not a fake forge
// interface: every observable request, effect, and count is real HTTP against
// the production setup.GitHubBackend.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

type publicationIssue struct {
	Number      int
	ID          int64
	Title       string
	Body        string
	State       string
	PullRequest bool
}

type publicationPull struct {
	Number   int
	NodeID   string
	Title    string
	Body     string
	State    string
	Branch   string
	Base     string
	Head     string
	Owner    string
	Draft    bool
	Merged   bool
	MergedAt string
}

type publicationReviewFile struct {
	Filename string
	Status   string
	Patch    string
}

type publicationReviewComment struct {
	Body           string
	Commit         string
	Path           string
	Side           string
	Line           int
	OriginalCommit string
	OriginalLine   int
	Outdated       bool
}

// publicationForge is one bounded controlled forge. Fault injection is
// expressed through three hooks: fail rejects a request with an explicit
// status before its effect, drop applies the effect and then loses the
// response, and before runs before any decision so a test can hold a request
// in flight.
type publicationForge struct {
	t      *testing.T
	server *httptest.Server

	mu         sync.Mutex
	issues     map[int]*publicationIssue
	issueOrder []int
	nextIssue  int
	pulls      map[int]*publicationPull
	pullOrder  []int
	nextPull   int
	links      map[int][]int
	files      []publicationReviewFile
	comments   []publicationReviewComment

	// branchRemote, when set, makes every pull head follow that bare
	// repository's branch ref, exactly as GitHub moves a pull head on push.
	branchRemote string

	requests       []string
	issueCreates   []map[string]any
	issuePatches   []map[string]any
	pullCreates    []map[string]any
	pullPatches    []map[string]any
	commentCreates []map[string]any

	before func(method, path string)
	fail   func(method, path string) int
	drop   func(method, path string) bool
	// afterMutation runs once a mutation took effect and before its response,
	// so a test can hold a real effect in flight.
	afterMutation func(method, path string)
}

func newPublicationForge(t *testing.T) *publicationForge {
	t.Helper()
	forge := &publicationForge{
		t: t, issues: map[int]*publicationIssue{}, pulls: map[int]*publicationPull{},
		links: map[int][]int{}, nextIssue: 100, nextPull: 10,
	}
	forge.server = httptest.NewServer(forge)
	t.Cleanup(forge.server.Close)
	return forge
}

func (f *publicationForge) addIssue(title, body string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addIssueLocked(title, body)
}

func (f *publicationForge) addIssueLocked(title, body string) int {
	f.nextIssue++
	issue := &publicationIssue{Number: f.nextIssue, ID: int64(f.nextIssue) * 1000, Title: title, Body: body, State: "open"}
	f.issues[issue.Number] = issue
	f.issueOrder = append(f.issueOrder, issue.Number)
	return issue.Number
}

func (f *publicationForge) addPull(pull publicationPull) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.putPullLocked(pull)
}

func (f *publicationForge) putPullLocked(pull publicationPull) int {
	if pull.Number == 0 {
		f.nextPull++
		pull.Number = f.nextPull
	}
	if pull.NodeID == "" {
		pull.NodeID = fmt.Sprintf("PR_node_%d", pull.Number)
	}
	if pull.State == "" {
		pull.State = "open"
	}
	if pull.Owner == "" {
		pull.Owner = "acme/widgets"
	}
	if pull.Base == "" {
		pull.Base = "main"
	}
	f.pulls[pull.Number] = &pull
	f.pullOrder = append(f.pullOrder, pull.Number)
	return pull.Number
}

func (f *publicationForge) issue(number int) publicationIssue {
	f.mu.Lock()
	defer f.mu.Unlock()
	if issue := f.issues[number]; issue != nil {
		return *issue
	}
	return publicationIssue{}
}

func (f *publicationForge) pull(number int) publicationPull {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		return *pull
	}
	return publicationPull{}
}

func (f *publicationForge) link(parent, child int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links[parent] = append(f.links[parent], child)
}

func (f *publicationForge) issueNumberByTitle(title string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, number := range f.issueOrder {
		if f.issues[number].Title == title {
			return number
		}
	}
	return 0
}

func (f *publicationForge) issueCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.issueOrder)
}

func (f *publicationForge) pullCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pullOrder)
}

func (f *publicationForge) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, request := range f.requests {
		if request == method+" "+path {
			total++
		}
	}
	return total
}

func (f *publicationForge) recordedRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *publicationForge) commentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.comments)
}

func (f *publicationForge) client() *http.Client { return f.server.Client() }

func (f *publicationForge) setBefore(hook func(method, path string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.before = hook
}

func (f *publicationForge) setFail(hook func(method, path string) int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = hook
}

func (f *publicationForge) setDrop(hook func(method, path string) bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.drop = hook
}

func (f *publicationForge) setAfterMutation(hook func(method, path string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.afterMutation = hook
}

// mutate invokes the after-mutation hook. Callers hold f.mu; the hook must not
// touch the fixture, so a test can hold one real effect in flight.
func (f *publicationForge) mutate(method, path string) {
	if f.afterMutation != nil {
		f.afterMutation(method, path)
	}
}

// Patch helpers let a test model an attachment that changed externally.
func (f *publicationForge) setPullBody(number int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		pull.Body = body
	}
}

func (f *publicationForge) setPullDraft(number int, draft bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		pull.Draft = draft
	}
}

func (f *publicationForge) setPullBase(number int, base string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		pull.Base = base
	}
}

func (f *publicationForge) setPullState(number int, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		pull.State = state
	}
}

func (f *publicationForge) setPullHead(number int, head string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		pull.Head = head
	}
}

func (f *publicationForge) setPullBranch(number int, branch string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		pull.Branch = branch
	}
}

func (f *publicationForge) setPullOwner(number int, owner string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pull := f.pulls[number]; pull != nil {
		pull.Owner = owner
	}
}

func (f *publicationForge) setPullFiles(files ...publicationReviewFile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files = files
}

// headOfLocked resolves one pull's observable head. A follow site reads the
// real bare remote so a pushed source revision is reflected without any test
// sleight of hand.
func (f *publicationForge) headOfLocked(pull *publicationPull) string {
	if f.branchRemote == "" {
		return pull.Head
	}
	output, err := exec.Command("git", "-C", f.branchRemote, "rev-parse", "--verify", "refs/heads/"+pull.Branch).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (f *publicationForge) issueRecordLocked(issue *publicationIssue) map[string]any {
	record := map[string]any{
		"number": issue.Number, "id": issue.ID, "title": issue.Title, "body": issue.Body,
		"state": issue.State, "labels": []any{}, "created_at": "2024-01-01T00:00:00Z",
	}
	if issue.PullRequest {
		record["pull_request"] = map[string]any{"url": "https://example.invalid/pull"}
	}
	return record
}

func (f *publicationForge) pullRecordLocked(pull *publicationPull) map[string]any {
	owner := pull.Owner
	if owner == "" {
		owner = "acme/widgets"
	}
	return map[string]any{
		"number": pull.Number, "id": pull.Number * 1000, "node_id": pull.NodeID,
		"state": pull.State, "title": pull.Title, "body": pull.Body, "draft": pull.Draft,
		"merged": pull.Merged, "merged_at": pull.MergedAt,
		"created_at": "2024-01-01T00:00:00Z", "author_association": "MEMBER",
		"user":   map[string]string{"login": "author"},
		"head":   map[string]any{"ref": pull.Branch, "sha": f.headOfLocked(pull), "repo": map[string]string{"full_name": owner}},
		"base":   map[string]string{"ref": pull.Base},
		"labels": []any{},
	}
}

func (f *publicationForge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	before, fail := f.before, f.fail
	f.mu.Unlock()
	if before != nil {
		before(r.Method, r.URL.Path)
	}
	if fail != nil {
		if status := fail(r.Method, r.URL.Path); status != 0 {
			http.Error(w, "forced failure", status)
			return
		}
	}
	switch {
	case r.URL.Path == "/graphql":
		f.serveGraphQL(w, r)
	case r.URL.Path == "/repos/acme/widgets/issues" && r.Method == http.MethodGet:
		f.serveIssueList(w)
	case r.URL.Path == "/repos/acme/widgets/issues" && r.Method == http.MethodPost:
		f.serveIssueCreate(w, r)
	case strings.HasPrefix(r.URL.Path, "/repos/acme/widgets/issues/"):
		f.serveIssue(w, r)
	case r.URL.Path == "/repos/acme/widgets/pulls" && r.Method == http.MethodGet:
		f.servePullList(w, r)
	case r.URL.Path == "/repos/acme/widgets/pulls" && r.Method == http.MethodPost:
		f.servePullCreate(w, r)
	case strings.HasPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"):
		f.servePull(w, r)
	default:
		f.t.Errorf("unexpected forge request %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	}
}

func (f *publicationForge) serveIssueList(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	values := []map[string]any{}
	for _, number := range f.issueOrder {
		values = append(values, f.issueRecordLocked(f.issues[number]))
	}
	_ = json.NewEncoder(w).Encode(values)
}

func (f *publicationForge) serveIssueCreate(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	title, _ := payload["title"].(string)
	body, _ := payload["body"].(string)
	f.mu.Lock()
	f.issueCreates = append(f.issueCreates, payload)
	number := f.addIssueLocked(title, body)
	record := f.issueRecordLocked(f.issues[number])
	drop := f.drop != nil && f.drop(http.MethodPost, r.URL.Path)
	f.mu.Unlock()
	if drop {
		loseResponse(w)
		return
	}
	_ = json.NewEncoder(w).Encode(record)
}

func (f *publicationForge) serveIssue(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets/issues/"), "/")
	number, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		issue, found := f.issues[number]
		if !found {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(f.issueRecordLocked(issue))
	case len(parts) == 1 && r.Method == http.MethodPatch:
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		f.issuePatches = append(f.issuePatches, payload)
		issue, found := f.issues[number]
		if !found {
			http.NotFound(w, r)
			return
		}
		if body, ok := payload["body"].(string); ok {
			issue.Body = body
		}
		f.mutate(http.MethodPatch, r.URL.Path)
		_ = json.NewEncoder(w).Encode(f.issueRecordLocked(issue))
	case len(parts) == 2 && parts[1] == "sub_issues" && r.Method == http.MethodGet:
		children := []map[string]any{}
		for _, child := range f.links[number] {
			if issue := f.issues[child]; issue != nil {
				children = append(children, map[string]any{"number": issue.Number, "id": issue.ID})
			}
		}
		_ = json.NewEncoder(w).Encode(children)
	case len(parts) == 2 && parts[1] == "sub_issues" && r.Method == http.MethodPost:
		var payload struct {
			SubIssueID int64 `json:"sub_issue_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		child := 0
		for _, candidate := range f.issues {
			if candidate.ID == payload.SubIssueID {
				child = candidate.Number
			}
		}
		if child == 0 {
			http.Error(w, "unknown sub issue", http.StatusUnprocessableEntity)
			return
		}
		if !containsNumber(f.links[number], child) {
			f.links[number] = append(f.links[number], child)
		}
		fmt.Fprint(w, `{}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *publicationForge) servePullList(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := r.URL.Query().Get("state")
	base := r.URL.Query().Get("base")
	head := r.URL.Query().Get("head")
	values := []map[string]any{}
	for _, number := range f.pullOrder {
		pull := f.pulls[number]
		if state != "" && state != "all" && pull.State != state {
			continue
		}
		if base != "" && pull.Base != base {
			continue
		}
		if head != "" {
			owner := pull.Owner
			if owner == "" {
				owner = "acme/widgets"
			}
			owner = strings.SplitN(owner, "/", 2)[0]
			if head != owner+":"+pull.Branch {
				continue
			}
		}
		values = append(values, f.pullRecordLocked(pull))
	}
	_ = json.NewEncoder(w).Encode(values)
}

func (f *publicationForge) servePullCreate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Title string `json:"title"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Body  string `json:"body"`
		Draft bool   `json:"draft"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	f.mu.Lock()
	f.pullCreates = append(f.pullCreates, map[string]any{
		"title": payload.Title, "head": payload.Head, "base": payload.Base, "body": payload.Body, "draft": payload.Draft,
	})
	number := f.putPullLocked(publicationPull{Title: payload.Title, Body: payload.Body, Branch: payload.Head, Base: payload.Base, Draft: payload.Draft})
	record := f.pullRecordLocked(f.pulls[number])
	drop := f.drop != nil && f.drop(http.MethodPost, r.URL.Path)
	f.mu.Unlock()
	if drop {
		loseResponse(w)
		return
	}
	_ = json.NewEncoder(w).Encode(record)
}

func (f *publicationForge) servePull(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"), "/")
	number, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		pull, found := f.pulls[number]
		if !found {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(f.pullRecordLocked(pull))
	case len(parts) == 1 && r.Method == http.MethodPatch:
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		f.pullPatches = append(f.pullPatches, payload)
		pull, found := f.pulls[number]
		if !found {
			http.NotFound(w, r)
			return
		}
		if body, ok := payload["body"].(string); ok {
			pull.Body = body
		}
		f.mutate(http.MethodPatch, r.URL.Path)
		_ = json.NewEncoder(w).Encode(f.pullRecordLocked(pull))
	case len(parts) == 2 && parts[1] == "files" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(f.files)
	case len(parts) == 2 && parts[1] == "comments" && r.Method == http.MethodGet:
		values := []map[string]any{}
		for _, comment := range f.comments {
			var line any = comment.Line
			if comment.Outdated {
				line = nil
			}
			values = append(values, map[string]any{
				"body": comment.Body, "commit_id": comment.Commit, "path": comment.Path,
				"line": line, "side": comment.Side, "original_commit_id": comment.OriginalCommit,
				"original_line": comment.OriginalLine, "author_association": "MEMBER",
				"created_at": "2024-01-01T00:00:00Z", "user": map[string]string{"login": "reviewer"},
			})
		}
		_ = json.NewEncoder(w).Encode(values)
	case len(parts) == 2 && parts[1] == "comments" && r.Method == http.MethodPost:
		var payload struct {
			Body   string `json:"body"`
			Commit string `json:"commit_id"`
			Path   string `json:"path"`
			Line   int    `json:"line"`
			Side   string `json:"side"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		f.commentCreates = append(f.commentCreates, map[string]any{
			"body": payload.Body, "commit_id": payload.Commit, "path": payload.Path, "line": payload.Line, "side": payload.Side,
		})
		f.comments = append(f.comments, publicationReviewComment{Body: payload.Body, Commit: payload.Commit, Path: payload.Path, Line: payload.Line, Side: payload.Side})
		f.mutate(http.MethodPost, r.URL.Path)
		drop := f.drop != nil && f.drop(http.MethodPost, r.URL.Path)
		if drop {
			loseResponse(w)
			return
		}
		fmt.Fprint(w, `{}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *publicationForge) serveGraphQL(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	f.mu.Lock()
	if strings.Contains(payload.Query, "markPullRequestReadyForReview") {
		for _, pull := range f.pulls {
			if pull.NodeID == payload.Variables["id"] {
				pull.Draft = false
			}
		}
	}
	if strings.Contains(payload.Query, "convertPullRequestToDraft") {
		for _, pull := range f.pulls {
			if pull.NodeID == payload.Variables["id"] {
				pull.Draft = true
			}
		}
	}
	drop := f.drop != nil && f.drop(http.MethodPost, r.URL.Path)
	f.mutate(http.MethodPost, r.URL.Path)
	f.mu.Unlock()
	if drop {
		loseResponse(w)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
}

// loseResponse models a mutation whose response was lost after it took effect:
// the server closes the connection without a usable reply.
func loseResponse(w http.ResponseWriter) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	connection, _, err := hijacker.Hijack()
	if err == nil {
		_ = connection.Close()
	}
}

// gitOutputString reads one Git command's output without failing the test, so
// a refusal can be observed at a real remote ref.
func gitOutputString(directory string, args ...string) string {
	output, _ := exec.Command("git", append([]string{"-C", directory}, args...)...).Output()
	return strings.TrimSpace(string(output))
}

// publicationBackendFactory binds the production GitHub adapter to one
// controlled forge server.
func publicationBackendFactory(forge *publicationForge) backendFactory {
	return func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(forge.server.URL, "secret", forge.client())
		backend.BindRepository(repository)
		return backend, nil
	}
}

// newPublicationApp binds the public skl CLI to this controlled forge.
func newPublicationApp(t *testing.T, forge *publicationForge) ledgerCLI {
	t.Helper()
	var output bytes.Buffer
	return ledgerCLI{app: newApp(publicationBackendFactory(forge), bytes.NewReader(nil), &output, &output), out: &output}
}

// publicationJSON runs one publication invocation and decodes its envelope.
func (c ledgerCLI) publicationJSON(t *testing.T, args ...string) (publicationOutput, error) {
	t.Helper()
	c.out.Reset()
	err := c.app.Run(args)
	text := c.out.String()
	if err != nil {
		return publicationOutput{}, err
	}
	var out publicationOutput
	if decodeErr := json.Unmarshal([]byte(text), &out); decodeErr != nil {
		t.Fatalf("decode publication output %q: %v", text, decodeErr)
	}
	return out, nil
}

// publicationRun runs one publication invocation and returns its raw output.
func (c ledgerCLI) publicationRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c.out.Reset()
	err := c.app.Run(args)
	return c.out.String(), err
}

// publicationSourceRepo creates an isolated source checkout whose GitHub-shaped
// fetch identity resolves to acme/widgets while every SSH transport is served
// locally from a bare repository. No test ever reaches the network, and
// `git remote get-url origin` keeps returning the GitHub identity the engine
// resolves.
func publicationSourceRepo(t *testing.T) (root, target, remote string) {
	t.Helper()
	root = sourceRepository(t, "acme", "widgets")
	target = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "update-ref", "refs/remotes/origin/main", target)
	remote = filepath.Join(t.TempDir(), "remote.git")
	runGit(t, t.TempDir(), "init", "-q", "--bare", "-b", "main", remote)
	shim := filepath.Join(t.TempDir(), "github-ssh.sh")
	writeFile(t, shim, "#!/bin/sh\nhost=\"$1\"; shift\ncmd=${1%% *}\nexec sh -c \"$cmd \\\"$SKL_BARE\\\"\"\n")
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SKL_BARE", remote)
	t.Setenv("GIT_SSH_COMMAND", shim)
	return root, target, remote
}

// publicationProposal writes one proposal and returns its intake directory plus
// its descriptive body files, so the CLI test drives acceptance through the
// public --issue/--parent-body inputs rather than an internal API.
func publicationProposal(t *testing.T, spec proposalSpec, bodies map[string]string) (string, []string, string) {
	t.Helper()
	directory := writeProposal(t, "", spec)
	var flags []string
	for _, slice := range spec.slices {
		body, ok := bodies[slice.name]
		if !ok {
			continue
		}
		path := filepath.Join(t.TempDir(), slice.name+".md")
		writeFile(t, path, body)
		flags = append(flags, "--issue="+slice.name+"="+path)
	}
	parentPath := ""
	if spec.parentTitle != "" {
		if body, ok := bodies["parent"]; ok {
			parentPath = filepath.Join(t.TempDir(), "parent.md")
			writeFile(t, parentPath, body)
		}
	}
	return directory, flags, parentPath
}
