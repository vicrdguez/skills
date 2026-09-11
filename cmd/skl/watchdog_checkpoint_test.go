package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

type reviewForge struct {
	mu             sync.Mutex
	head           string
	body           string
	labels         []string
	summaries      []map[string]any
	inlines        []map[string]any
	sourceComments []map[string]any
	timeline       []map[string]any
	failSummary    bool
	failAfterWrite bool
	failHandoff    bool
	denyCleanup    string
}

func (f *reviewForge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
	write := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	issue := func(number int, title string, labels []string, pull bool) map[string]any {
		result := map[string]any{"id": number, "number": number, "title": title, "body": "", "state": "open", "created_at": "2026", "labels": labelObjects(labels), "sub_issues_summary": map[string]int{"total": 0}}
		if pull {
			result["pull_request"] = map[string]string{"url": "pull"}
		}
		return result
	}
	pull := func() map[string]any {
		result := issue(11, "widget", f.labels, true)
		result["body"], result["draft"], result["merged"], result["mergeable"] = f.body, false, false, true
		result["head"] = map[string]any{"ref": "widget", "sha": f.head, "repo": map[string]string{"full_name": "acme/widgets"}}
		result["base"] = map[string]string{"ref": "main"}
		return result
	}
	switch {
	case r.Method == http.MethodGet && path == "/issues":
		write([]any{issue(7, "widget", nil, false), issue(11, "widget", f.labels, true)})
	case r.Method == http.MethodGet && path == "/pulls":
		write([]any{pull()})
	case r.Method == http.MethodGet && path == "/pulls/11":
		write(pull())
	case r.Method == http.MethodGet && path == "/git/ref/heads/widget":
		write(map[string]any{"object": map[string]string{"sha": f.head}})
	case r.Method == http.MethodGet && path == "/git/ref/heads/main":
		write(map[string]any{"object": map[string]string{"sha": f.head}})
	case r.Method == http.MethodGet && path == "/issues/11/timeline":
		write(f.timeline)
	case r.Method == http.MethodGet && path == "/pulls/11/reviews":
		write([]any{})
	case r.Method == http.MethodGet && path == "/issues/7/comments":
		write(f.sourceComments)
	case r.Method == http.MethodGet && path == "/issues/11/comments":
		write(f.summaries)
	case r.Method == http.MethodGet && path == "/pulls/11/comments":
		write(f.inlines)
	case r.Method == http.MethodPost && path == "/issues/11/comments":
		if f.failSummary {
			http.Error(w, "summary unavailable", http.StatusInternalServerError)
			return
		}
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		f.summaries = append(f.summaries, value)
		if f.failAfterWrite {
			f.failAfterWrite = false
			http.Error(w, "response lost", http.StatusInternalServerError)
		}
	case r.Method == http.MethodPost && path == "/issues/7/comments":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		value["author_association"] = "OWNER"
		f.sourceComments = append(f.sourceComments, value)
	case r.Method == http.MethodPost && path == "/pulls/11/comments":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		f.inlines = append(f.inlines, value)
	case r.Method == http.MethodPatch && path == "/pulls/11":
		var value map[string]string
		_ = json.NewDecoder(r.Body).Decode(&value)
		f.body = value["body"]
	case r.Method == http.MethodGet && (path == "/issues/7" || path == "/issues/11"):
		if path == "/issues/7" {
			write(issue(7, "widget", nil, false))
		} else {
			write(issue(11, "widget", f.labels, true))
		}
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/labels"):
		if f.failHandoff && slices.Contains(f.labels, "review") {
			f.failHandoff = false
			http.Error(w, "handoff unavailable", http.StatusInternalServerError)
			return
		}
		var value struct {
			Labels []string `json:"labels"`
		}
		_ = json.NewDecoder(r.Body).Decode(&value)
		for _, label := range value.Labels {
			if !slices.Contains(f.labels, label) {
				f.labels = append(f.labels, label)
			}
		}
	case r.Method == http.MethodDelete && strings.Contains(path, "/labels/"):
		label := path[strings.LastIndex(path, "/")+1:]
		f.labels = slices.DeleteFunc(f.labels, func(current string) bool { return current == label })
		if label == "wip" && f.denyCleanup != "" {
			_ = os.Chmod(f.denyCleanup, 0500)
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
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "switch", "main")
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "worktree", "add", worktree, "widget")
	gitDir := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "--absolute-git-dir"))
	forge := &reviewForge{head: head, labels: []string{"review"}}
	server := httptest.NewServer(forge)
	t.Cleanup(server.Close)
	return &reviewFixture{root: root, worktree: worktree, head: head, checkpoint: filepath.Join(gitDir, ".watchdog"), forge: forge, server: server}
}

func (f *reviewFixture) run(t *testing.T, caller string, args ...string) setup.ImplementationOutput {
	t.Helper()
	result, err := f.runResult(caller, args...)
	if err != nil {
		t.Fatal(err)
	}
	if result.Packet != nil && result.Packet.Facts.Watchdog != nil {
		t.Cleanup(func() { _ = os.RemoveAll(result.Packet.Facts.Watchdog.ResultDirectory) })
	}
	if result.Packet != nil && result.Packet.Facts.Implementation != nil {
		t.Cleanup(func() { _ = os.RemoveAll(result.Packet.Facts.Implementation.ResultDirectory) })
	}
	return result
}

func (f *reviewFixture) runResult(caller string, args ...string) (setup.ImplementationOutput, error) {
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		return setup.NewGitHubBackend(f.server.URL, "token", f.server.Client()), nil
	}, bytes.NewReader(nil), &output, &output)
	command := append([]string{"skl"}, args...)
	command = append(command, "--repo", caller)
	if err := app.Run(command); err != nil {
		return setup.ImplementationOutput{}, fmt.Errorf("%v: %w: %s", command, err, &output)
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return result, fmt.Errorf("decode %s: %w", &output, err)
	}
	return result, nil
}

func (f *reviewFixture) start(t *testing.T, caller string) setup.ImplementationOutput {
	return f.run(t, caller, "watchdog", "next")
}

func (f *reviewFixture) submit(t *testing.T, number uint64, head, verdict string) setup.ImplementationOutput {
	t.Helper()
	dir := t.TempDir()
	summary := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(summary, []byte("round "+strconv.FormatUint(number, 10)), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"watchdog", "submit", "--item", "7", "--review-number", strconv.FormatUint(number, 10), "--reviewed-head", head, "--verdict", verdict, "--summary", summary}
	if verdict == "pass" {
		body := filepath.Join(dir, "submission.md")
		if err := os.WriteFile(body, []byte("final"), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--body", body)
	}
	return f.run(t, f.worktree, args...)
}

func TestWatchdogReviewCheckpoints(t *testing.T) {
	t.Run("B1 selected linked worktree owns checkpoint", func(t *testing.T) {
		for _, caller := range []string{"primary", "selected", "other"} {
			t.Run(caller, func(t *testing.T) {
				f := newReviewFixture(t)
				location := map[string]string{"primary": f.root, "selected": f.worktree}[caller]
				if caller == "other" {
					location = filepath.Join(f.root, ".worktrees", "other")
					runGit(t, f.root, "worktree", "add", "-b", "other", location, "main")
				}
				start := f.start(t, location)
				if start.Packet.Facts.Watchdog.ReviewNumber != 1 || strings.Contains(start.Packet.Instructions, ".watchdog") {
					t.Fatalf("encapsulation: %#v", start.Packet.Facts.Watchdog)
				}
				f.submit(t, 1, f.head, "rework")
				if got := strings.TrimSpace(readFile(t, f.checkpoint)); got != "1:"+f.head {
					t.Fatalf("checkpoint = %q", got)
				}
			})
		}
	})

	t.Run("B2 count and scope facts", func(t *testing.T) {
		for _, tc := range []struct {
			checkpoint    string
			count, number uint64
			scope         string
		}{
			{"", 0, 1, "full"}, {"0:", 0, 1, "full"}, {"1:", 1, 2, "incremental"}, {"2:" + strings.Repeat("f", 40), 2, 3, "full"},
		} {
			f := newReviewFixture(t)
			if tc.checkpoint != "" {
				head := tc.checkpoint
				if strings.HasSuffix(head, ":") {
					head += f.head
				}
				if err := os.WriteFile(f.checkpoint, []byte(head+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			facts := f.start(t, f.root).Packet.Facts.Watchdog
			if facts.ReviewCount != tc.count || facts.ReviewNumber != tc.number || facts.ReviewScope != tc.scope {
				t.Fatalf("facts = %#v", facts)
			}
		}
	})

	t.Run("B3 resume refreshes PR head", func(t *testing.T) {
		f := newReviewFixture(t)
		first := f.start(t, f.root).Packet.Facts.Watchdog
		runGit(t, f.worktree, "commit", "--allow-empty", "-m", "move")
		f.forge.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
		resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7").Packet.Facts.Watchdog
		if resumed.ReviewedHead == first.ReviewedHead || resumed.ReviewNumber != first.ReviewNumber || !strings.Contains(resumed.SubmitCommand, "--reviewed-head "+f.forge.head) {
			t.Fatalf("resume facts = %#v", resumed)
		}
	})

	t.Run("B4 invalid checkpoint and submit inputs refuse", func(t *testing.T) {
		for _, invalid := range []string{"", "-1:" + strings.Repeat("a", 40), "1:abc", "1:" + strings.Repeat("a", 39), "1:" + strings.Repeat("a", 40) + ":x"} {
			f := newReviewFixture(t)
			if err := os.WriteFile(f.checkpoint, []byte(invalid), 0600); err != nil {
				t.Fatal(err)
			}
			got := f.start(t, f.root)
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "Review Checkpoint") {
				t.Fatalf("accepted %q: %#v", invalid, got)
			}
		}
		f := newReviewFixture(t)
		_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "0", "--reviewed-head", f.head, "--verdict", "rework", "--summary", filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.Contains(err.Error(), "positive --review-number") {
			t.Fatalf("accepted zero round: %v", err)
		}
	})

	t.Run("B5 completed verdict count policy", func(t *testing.T) {
		for _, tc := range []struct {
			before, number  uint64
			verdict, status string
		}{{0, 1, "rework", "rework"}, {1, 2, "rework", "needs_human"}, {2, 3, "rework", "needs_human"}, {0, 1, "needs-human", "needs_human"}, {2, 3, "pass", "ready_for_merge"}} {
			f := newReviewFixture(t)
			if tc.before > 0 {
				_ = os.WriteFile(f.checkpoint, []byte(fmt.Sprintf("%d:%s\n", tc.before, f.head)), 0600)
			}
			f.start(t, f.root)
			got := f.submit(t, tc.number, f.head, tc.verdict)
			if got.Status != tc.status {
				t.Fatalf("%+v: %#v", tc, got)
			}
		}
	})

	t.Run("B6 same SHA new round differs from retry", func(t *testing.T) {
		f := newReviewFixture(t)
		f.start(t, f.root)
		first := f.submit(t, 1, f.head, "needs-human")
		comments := len(f.forge.summaries)
		if retry := f.submit(t, 1, f.head, "needs-human"); retry.Status != "needs_human" || len(f.forge.summaries) != comments {
			t.Fatalf("retry: %#v", retry)
		}
		f.forge.labels = []string{"review", "wip"}
		second := f.submit(t, 2, f.head, "rework")
		if first.Status != "needs_human" || second.Status != "needs_human" || strings.TrimSpace(readFile(t, f.checkpoint)) != "2:"+f.head {
			t.Fatalf("same SHA rounds: %#v %#v", first, second)
		}
	})

	t.Run("B7 evidence precedes checkpoint and retries exactly", func(t *testing.T) {
		f := newReviewFixture(t)
		f.start(t, f.root)
		f.forge.failSummary = true
		dir := t.TempDir()
		summary := filepath.Join(dir, "summary.md")
		_ = os.WriteFile(summary, []byte("round 1"), 0600)
		_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
		if err == nil || fileExists(f.checkpoint) || !slices.Contains(f.forge.labels, "wip") {
			t.Fatalf("failed evidence: %v", err)
		}
		f.forge.failSummary = false
		f.submit(t, 1, f.head, "rework")
		if len(f.forge.summaries) != 1 || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head {
			t.Fatal("evidence retry duplicated or did not checkpoint")
		}
	})

	t.Run("B8 checkpoint replacement is atomic", func(t *testing.T) {
		f := newReviewFixture(t)
		_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
		f.start(t, f.root)
		gitDir := filepath.Dir(f.checkpoint)
		if err := os.Chmod(gitDir, 0500); err != nil {
			t.Fatal(err)
		}
		got := f.submit(t, 2, f.head, "rework")
		_ = os.Chmod(gitDir, 0700)
		if got.Status != "fix_required" || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head || !slices.Contains(f.forge.labels, "wip") {
			t.Fatalf("atomic failure: %#v", got)
		}
	})

	t.Run("B9 fixed-number retry after checkpoint replacement", func(t *testing.T) {
		f := newReviewFixture(t)
		f.start(t, f.root)
		f.forge.failHandoff = true
		dir := t.TempDir()
		summary := filepath.Join(dir, "summary.md")
		_ = os.WriteFile(summary, []byte("round 1"), 0600)
		_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
		if err == nil || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head {
			t.Fatalf("checkpoint ordering: %v", err)
		}
		if retry := f.submit(t, 1, f.head, "rework"); retry.Status != "rework" || len(f.forge.summaries) != 1 {
			t.Fatalf("fixed retry: %#v", retry)
		}
	})

	t.Run("B10 ambiguous commands stop", func(t *testing.T) {
		f := newReviewFixture(t)
		_ = os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600)
		f.start(t, f.root)
		for _, number := range []uint64{1, 4} {
			if got := f.submit(t, number, f.head, "rework"); got.Status != "fix_required" {
				t.Fatalf("accepted round %d: %#v", number, got)
			}
		}
	})

	t.Run("B11 reviewed head remains distinct from final head", func(t *testing.T) {
		f := newReviewFixture(t)
		f.start(t, f.root)
		reviewed := f.head
		runGit(t, f.worktree, "commit", "--allow-empty", "-m", "debt marker")
		final := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
		f.forge.head = final
		dir := t.TempDir()
		summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "body.md")
		_ = os.WriteFile(summary, []byte("pass"), 0600)
		_ = os.WriteFile(body, []byte("final"), 0600)
		got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", reviewed, "--head", final, "--verdict", "pass", "--summary", summary, "--body", body)
		if got.Status != "ready_for_merge" || fileExists(f.checkpoint) {
			t.Fatalf("debt marker pass: %#v", got)
		}
	})

	t.Run("B12 implementation and Audit need no previous review cache", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.labels = []string{"rework"}
		got := f.run(t, f.worktree, "implement", "next")
		if got.Status != "work_available" || strings.Contains(got.Packet.Instructions, "previous-reviewed-head") || strings.Contains(got.Packet.Instructions, "--reviewed-head") {
			t.Fatalf("rework packet: %#v", got)
		}
	})

	t.Run("B13 nonterminal work retains checkpoint", func(t *testing.T) {
		f := newReviewFixture(t)
		_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
		f.forge.labels = []string{"rework"}
		f.run(t, f.worktree, "implement", "next")
		if strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head {
			t.Fatal("implementation removed checkpoint")
		}
	})

	t.Run("B14 only verified done deletes checkpoint", func(t *testing.T) {
		f := newReviewFixture(t)
		f.start(t, f.root)
		if got := f.submit(t, 1, f.head, "pass"); got.Status != "ready_for_merge" || fileExists(f.checkpoint) {
			t.Fatalf("done cleanup: %#v", got)
		}
		f = newReviewFixture(t)
		f.forge.denyCleanup = filepath.Dir(f.checkpoint)
		f.start(t, f.root)
		got := f.submit(t, 1, f.head, "pass")
		_ = os.Chmod(f.forge.denyCleanup, 0700)
		if got.Status != "ready_for_merge" || !fileExists(f.checkpoint) || !strings.Contains(got.Reason, "remove Review Checkpoint") {
			t.Fatalf("cleanup warning: %#v", got)
		}
		if retry := f.submit(t, 1, f.head, "pass"); retry.Status != "ready_for_merge" || fileExists(f.checkpoint) || len(f.forge.summaries) != 1 {
			t.Fatalf("cleanup-only retry: %#v", retry)
		}
	})

	t.Run("B15 timeline is not a count source", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.timeline = []map[string]any{{"event": "labeled", "label": map[string]string{"name": "rework"}}}
		_ = os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600)
		facts := f.start(t, f.root).Packet.Facts.Watchdog
		if facts.ReviewCount != 2 || facts.ReviewNumber != 3 {
			t.Fatalf("timeline changed count: %#v", facts)
		}
		if got := f.submit(t, 3, f.head, "pass"); got.Status != "ready_for_merge" {
			t.Fatalf("count capped pass: %#v", got)
		}
		f.forge.labels = []string{"review", "rework", "wip"}
		f.forge.timeline = []map[string]any{{"event": "labeled", "label": map[string]string{"name": "review"}}, {"event": "labeled", "label": map[string]string{"name": "rework"}}}
		status := f.run(t, f.root, "status")
		if status.Status != "fix_required" || !strings.Contains(status.Reason, "original fixed-number") {
			t.Fatalf("status invented completion: %#v", status)
		}
	})

	t.Run("B16 recreated worktree starts fresh", func(t *testing.T) {
		f := newReviewFixture(t)
		_ = os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600)
		runGit(t, f.root, "worktree", "remove", "--force", f.worktree)
		runGit(t, f.root, "worktree", "add", f.worktree, "widget")
		facts := f.start(t, f.root).Packet.Facts.Watchdog
		if facts.ReviewCount != 0 || facts.ReviewNumber != 1 || facts.ReviewScope != "full" {
			t.Fatalf("recreated facts: %#v", facts)
		}
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
