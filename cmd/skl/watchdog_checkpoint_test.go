package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"slices"

	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

type reviewForge struct {
	mu                sync.Mutex
	head              string
	branch            string
	remoteHead        string
	pullHead          string
	body              string
	bodyEditedAt      string
	draft             bool
	noPull            bool
	noOther           bool
	labels            []string
	sourceLabels      []string
	otherLabels       []string
	summaries         []map[string]any
	issueComments     []map[string]any
	inlines           []map[string]any
	sourceComments    []map[string]any
	otherComments     []map[string]any
	timeline          []map[string]any
	sourceTimeline    []map[string]any
	otherTimeline     []map[string]any
	failSummary       bool
	failHandoff       bool
	failInline        bool
	failInlinePost    bool
	failBody          bool
	failBodyPost      bool
	failReadback      bool
	duplicateRead     bool
	reviewReads       int
	failPostRead      bool
	failDelete        string
	loseDelete        string
	failItemsRead     bool
	failFinalRead     bool
	failPullRead      bool
	failSourceLabel   bool
	cleanupEntry      string
	cleanupCheckpoint string
	mergeable         bool
	checkpointPath    string
	atWipRelease      string
	denyRename        string
	renameDenied      bool
	afterMutation     func()
	afterRead         func(string)
	rejectMutation    string
	loseResponse      string
	readsUnavailable  bool
	failedReads       []string
	requests          []string
	acceptedMutations []string
	clock             int
	writes            int
	pullCreations     int
}

func (f *reviewForge) timestamp() string {
	f.clock++
	return time.Date(2026, 1, 1, 0, 0, f.clock, 0, time.UTC).Format(time.RFC3339Nano)
}

// submissionBody renders the observed Submission body: the engine always
// establishes its explicit owning reference at publication, while stored prose
// stays separate so refusal probes can still distinguish it from the footer.
func (f *reviewForge) submissionBody() string {
	body := strings.TrimRight(f.body, "\n")
	if !strings.HasSuffix(body, "Closes #7") {
		body += "\n\nCloses #7"
	}
	return body + "\n"
}

func (f *reviewForge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
	request := r.Method + " " + path
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	var graphql struct{ Query string }
	_ = json.Unmarshal(body, &graphql)
	readOnly := r.Method == http.MethodGet || path == "/graphql" && strings.HasPrefix(graphql.Query, "query")
	if readOnly && r.Method == http.MethodPost {
		request += " query"
	}
	f.mu.Lock()
	f.requests = append(f.requests, request)
	unavailable := f.readsUnavailable && readOnly
	if unavailable {
		f.failedReads = append(f.failedReads, request)
	}
	rejected := request == f.rejectMutation
	if rejected {
		f.rejectMutation = ""
	}
	f.mu.Unlock()
	if unavailable || rejected {
		http.Error(w, "injected request unavailable", http.StatusInternalServerError)
		return
	}
	response := httptest.NewRecorder()
	f.serveHTTP(response, r)
	f.mu.Lock()
	if !readOnly && response.Code < 400 {
		f.acceptedMutations = append(f.acceptedMutations, request+" "+string(body))
		if request == f.loseResponse {
			f.loseResponse, f.readsUnavailable = "", true
			response = httptest.NewRecorder()
			http.Error(response, "accepted mutation response lost", http.StatusInternalServerError)
		}
	}
	f.mu.Unlock()

	if !readOnly && f.afterMutation != nil {
		f.afterMutation()
	} else if readOnly && response.Code < 400 && f.afterRead != nil {
		f.afterRead(path)
	}
	for key, values := range response.Header() {
		w.Header()[key] = values
	}
	w.WriteHeader(response.Code)
	_, _ = w.Write(response.Body.Bytes())
}

func (f *reviewForge) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodGet && r.URL.Path != "/graphql" {
		f.writes++
	}
	path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
	branch := f.branch
	if branch == "" {
		branch = "widget"
	}
	write := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	issue := func(number int, title string, labels []string, pull bool) map[string]any {
		result := map[string]any{"id": number, "number": number, "title": title, "body": "", "state": "open", "created_at": "2026", "labels": labelObjects(labels), "sub_issues_summary": map[string]int{"total": 0}}
		if pull {
			result["pull_request"] = map[string]string{"url": "pull"}
			result["body"] = f.submissionBody()
		} else {
			result["body"] = "Branch: `" + title + "`\n"
		}
		return result
	}
	pull := func() map[string]any {
		head := f.head
		if f.pullHead != "" {
			head = f.pullHead
		}
		result := issue(11, branch, f.labels, true)
		result["body"], result["draft"], result["merged"], result["mergeable"] = f.submissionBody(), f.draft, false, f.mergeable
		result["node_id"] = "PR_11"
		result["head"] = map[string]any{"ref": branch, "sha": head, "repo": map[string]string{"full_name": "acme/widgets"}}
		result["base"] = map[string]string{"ref": "main"}
		return result
	}
	switch {
	case r.Method == http.MethodGet && path == "/issues":
		if f.failItemsRead {
			f.failItemsRead = false
			http.Error(w, "readback unavailable", http.StatusInternalServerError)
			return
		}
		issues := []any{issue(7, branch, f.sourceLabels, false)}
		if !f.noOther {
			issues = append(issues, issue(8, "other", f.otherLabels, false))
		}
		if !f.noPull {
			issues = append(issues, issue(11, branch, f.labels, true))
		}
		write(issues)
	case r.Method == http.MethodGet && path == "/pulls":
		if f.noPull {
			write([]any{})
		} else {
			write([]any{pull()})
		}
	case r.Method == http.MethodPost && path == "/pulls":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		f.body, _ = value["body"].(string)
		f.bodyEditedAt = f.timestamp()
		f.draft, _ = value["draft"].(bool)
		f.noPull = false
		f.pullCreations++
		write(pull())
	case r.Method == http.MethodGet && path == "/pulls/11":
		if f.failPullRead {
			f.failPullRead = false
			http.Error(w, "pull readback unavailable", http.StatusInternalServerError)
			return
		}
		write(pull())
	case r.Method == http.MethodGet && path == "/git/ref/heads/widget":
		if f.denyRename != "" {
			entries, _ := os.ReadDir(f.denyRename)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".watchdog-") {
					_ = os.Chmod(f.denyRename, 0500)
					f.denyRename, f.renameDenied = "", true
					break
				}
			}
		}
		head := f.head
		if f.remoteHead != "" {
			head = f.remoteHead
		}
		write(map[string]any{"object": map[string]string{"sha": head}})
	case r.Method == http.MethodGet && path == "/git/ref/heads/main":
		write(map[string]any{"object": map[string]string{"sha": f.head}})
	case r.Method == http.MethodGet && path == "/issues/11/timeline":
		write(f.timeline)
	case r.Method == http.MethodGet && path == "/issues/7/timeline":
		write(f.sourceTimeline)
	case r.Method == http.MethodGet && path == "/issues/8/timeline":
		write(f.otherTimeline)
	case r.Method == http.MethodGet && (path == "/issues/7/dependencies/blocked_by" || path == "/issues/8/dependencies/blocked_by"):
		write([]any{})
	case r.Method == http.MethodGet && path == "/pulls/11/reviews":
		f.reviewReads++
		if f.duplicateRead && f.reviewReads == 2 {
			f.summaries = []map[string]any{storedReviewSummary(1, "rework", "round 1", f.head, f.timestamp()), storedReviewSummary(1, "rework", "round 1", f.head, f.timestamp())}
		}
		if f.failReadback {
			f.failReadback = false
			http.Error(w, "review readback unavailable", http.StatusInternalServerError)
			return
		}
		write(f.summaries)
	case r.Method == http.MethodGet && path == "/issues/7/comments":
		write(f.sourceComments)
	case r.Method == http.MethodGet && path == "/issues/8/comments":
		write(f.otherComments)
	case r.Method == http.MethodGet && path == "/issues/11/comments":
		write(f.issueComments)
	case r.Method == http.MethodGet && path == "/pulls/11/comments":
		write(f.inlines)
	case r.Method == http.MethodPost && path == "/pulls/11/reviews":
		if f.failSummary {
			http.Error(w, "summary unavailable", http.StatusInternalServerError)
			return
		}
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		if value["event"] != "COMMENT" {
			http.Error(w, "pull request authors cannot approve or request changes on their own submissions", http.StatusUnprocessableEntity)
			return
		}
		value["state"] = "COMMENTED"
		value["author_association"] = "OWNER"
		value["submitted_at"] = f.timestamp()
		f.summaries = append(f.summaries, value)
		if f.failPostRead {
			f.failPostRead = false
			f.failReadback = true
		}
	case r.Method == http.MethodPost && (path == "/issues/7/comments" || path == "/issues/8/comments"):
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		value["author_association"] = "OWNER"
		value["created_at"] = f.timestamp()
		if path == "/issues/7/comments" {
			f.sourceComments = append(f.sourceComments, value)
		} else {
			f.otherComments = append(f.otherComments, value)
		}
	case r.Method == http.MethodPost && path == "/pulls/11/comments":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		if f.failInlinePost {
			f.failInlinePost = false
			http.Error(w, "inline unavailable", http.StatusInternalServerError)
			return
		}
		value["author_association"] = "OWNER"
		value["created_at"] = f.timestamp()
		f.inlines = append(f.inlines, value)
		if f.failInline {
			f.failInline = false
			http.Error(w, "inline response lost", http.StatusInternalServerError)
		}
	case r.Method == http.MethodPost && path == "/issues/11/comments":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		value["author_association"] = "OWNER"
		value["created_at"] = f.timestamp()
		f.issueComments = append(f.issueComments, value)
	case r.Method == http.MethodPatch && path == "/pulls/11":
		var value map[string]string
		_ = json.NewDecoder(r.Body).Decode(&value)
		if f.failBodyPost {
			f.failBodyPost = false
			http.Error(w, "body unavailable", http.StatusInternalServerError)
			return
		}
		f.body = value["body"]
		f.bodyEditedAt = f.timestamp()
		if f.failBody {
			f.failBody = false
			http.Error(w, "body response lost", http.StatusInternalServerError)
		}
	case r.Method == http.MethodPost && path == "/graphql":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		query, _ := value["query"].(string)
		if strings.HasPrefix(query, "query") {
			if strings.Contains(query, "pullRequests(") {
				nodes := []any{}
				if !f.noPull {
					head := f.head
					if f.pullHead != "" {
						head = f.pullHead
					}
					nodes = append(nodes, map[string]any{"number": 11, "createdAt": "2026-01-01T00:00:00Z", "headRefName": branch, "headRefOid": head, "isDraft": f.draft, "labels": map[string]any{"nodes": labelObjects(f.labels)}})
				}
				write(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequests": map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}}})
				return
			}
			if strings.Contains(query, "closedByPullRequestsReferences") {
				nodes := []any{}
				variables, _ := value["variables"].(map[string]any)
				if !f.noPull && variables["number"] == float64(7) {
					nodes = append(nodes, map[string]any{"number": 11, "merged": false, "mergedAt": "", "state": "OPEN", "repository": map[string]string{"nameWithOwner": "acme/widgets"}})
				}
				write(map[string]any{"data": map[string]any{"repository": map[string]any{"issue": map[string]any{"state": "OPEN", "closedByPullRequestsReferences": map[string]any{"nodes": nodes}}}}})
				return
			}
			var edited any
			if f.bodyEditedAt != "" {
				edited = f.bodyEditedAt
			}
			write(map[string]any{"data": map[string]any{"node": map[string]any{"body": f.submissionBody(), "createdAt": "2026-01-01T00:00:00Z", "lastEditedAt": edited}}})
			return
		}
		f.writes++
		f.draft = strings.Contains(query, "convertPullRequestToDraft")
	case r.Method == http.MethodGet && (path == "/issues/7" || path == "/issues/8" || path == "/issues/11"):
		if path == "/issues/7" {
			write(issue(7, branch, f.sourceLabels, false))
		} else if path == "/issues/8" {
			write(issue(8, "other", f.otherLabels, false))
		} else {
			write(issue(11, branch, f.labels, true))
		}
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/labels"):
		if f.failSourceLabel && strings.HasPrefix(path, "/issues/7/") {
			f.failSourceLabel = false
			http.Error(w, "source label projection unavailable", http.StatusInternalServerError)
			return
		}
		if f.failHandoff && slices.Contains(f.labels, "review") {
			f.failHandoff = false
			http.Error(w, "handoff unavailable", http.StatusInternalServerError)
			return
		}
		var value struct {
			Labels []string `json:"labels"`
		}
		_ = json.NewDecoder(r.Body).Decode(&value)
		labels := &f.labels
		events := &f.timeline
		if strings.HasPrefix(path, "/issues/7/") {
			labels, events = &f.sourceLabels, &f.sourceTimeline
		} else if strings.HasPrefix(path, "/issues/8/") {
			labels, events = &f.otherLabels, &f.otherTimeline
		}
		for _, label := range value.Labels {
			if !slices.Contains(*labels, label) {
				*labels = append(*labels, label)
				*events = append(*events, map[string]any{"event": "labeled", "created_at": f.timestamp(), "label": map[string]string{"name": label}})
			}
		}
	case r.Method == http.MethodDelete && strings.Contains(path, "/labels/"):
		label := path[strings.LastIndex(path, "/")+1:]
		if label == "wip" && f.checkpointPath != "" {
			f.atWipRelease = checkpointSnapshot(f.checkpointPath)
		}
		if label == f.failDelete {
			f.failDelete = ""
			http.Error(w, "label deletion unavailable", http.StatusInternalServerError)
			return
		}
		labels := &f.labels
		events := &f.timeline
		if strings.HasPrefix(path, "/issues/7/") {
			labels, events = &f.sourceLabels, &f.sourceTimeline
		} else if strings.HasPrefix(path, "/issues/8/") {
			labels, events = &f.otherLabels, &f.otherTimeline
		}
		*labels = slices.DeleteFunc(*labels, func(current string) bool { return current == label })
		*events = append(*events, map[string]any{"event": "unlabeled", "created_at": f.timestamp(), "label": map[string]string{"name": label}})
		if label == "wip" && f.failFinalRead {
			f.failItemsRead = true
		}
		if label == f.loseDelete {
			f.loseDelete = ""
			http.Error(w, "label deletion response lost", http.StatusInternalServerError)
			return
		}
		if path == "/issues/11/labels/wip" && f.cleanupEntry != "" {
			_ = os.WriteFile(f.cleanupEntry, []byte("keep"), 0600)
		}
		if path == "/issues/11/labels/wip" && f.cleanupCheckpoint != "" {
			_ = os.Rename(f.checkpointPath, f.cleanupCheckpoint)
			_ = os.Mkdir(f.checkpointPath, 0700)
			_ = os.WriteFile(filepath.Join(f.checkpointPath, "keep"), []byte("keep"), 0600)
		}
	default:
		http.Error(w, fmt.Sprintf("unexpected %s %s", r.Method, path), http.StatusNotFound)
	}
}

func labelObjects(labels []string) []map[string]string {
	result := make([]map[string]string, len(labels))
	for i, label := range labels {
		result[i] = map[string]string{"name": label}
	}
	return result
}

type reviewFixture struct {
	root, worktree, head, checkpoint string
	forge                            *reviewForge
	server                           *httptest.Server
}

func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "switch", "main")
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "worktree", "add", worktree, "widget")
	gitDir := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "--absolute-git-dir"))
	forge := &reviewForge{head: head, labels: []string{"review"}, otherLabels: []string{"ready"}, mergeable: true, clock: 1, timeline: []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "review"}}}}
	server := httptest.NewServer(forge)
	t.Cleanup(server.Close)
	checkpoint := filepath.Join(gitDir, ".watchdog")
	forge.checkpointPath = checkpoint
	return &reviewFixture{root: root, worktree: worktree, head: head, checkpoint: checkpoint, forge: forge, server: server}
}

func (f *reviewFixture) runJSON(caller string, args ...string) ([]byte, error) {
	var output bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(f.server.URL, "token", f.server.Client())
		backend.BindRepository(repository)
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	command := structuredStageCommand(args...)
	command = append(command, "--repo", caller)
	if len(args) > 0 && args[0] == "watchdog" {
		command = append(command, "--format", "json")
	}
	if err := app.Run(command); err != nil {
		return nil, fmt.Errorf("%v: %w: %s", command, err, &output)
	}
	return output.Bytes(), nil
}

func checkpointSnapshot(path string) string {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "<absent>"
	}
	if err != nil {
		return "<error: " + err.Error() + ">"
	}
	return string(data)
}

func storedReviewSummary(number uint64, verdict, body, commit, submittedAt string) map[string]any {
	finalHead := ""
	if verdict == "pass" {
		finalHead = fmt.Sprintf(",\"final_head\":%q", commit)
	}
	return map[string]any{
		"author_association": "OWNER",
		"body":               fmt.Sprintf("<!-- skl.watchdog.review/v1\n{\"review_number\":%d,\"verdict\":%q%s}\n-->\n%s", number, verdict, finalHead, body),
		"commit_id":          commit,
		"state":              "COMMENTED",
		"submitted_at":       submittedAt,
	}
}
