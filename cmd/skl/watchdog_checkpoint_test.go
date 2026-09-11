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
	"time"

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
	failInline     bool
	failBody       bool
	failReadback   bool
	failPostRead   bool
	failDelete     string
	loseDelete     string
	failItemsRead  bool
	failFinalRead  bool
	denyCleanup    string
	mergeable      bool
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
		result["body"], result["draft"], result["merged"], result["mergeable"] = f.body, false, false, f.mergeable
		result["head"] = map[string]any{"ref": "widget", "sha": f.head, "repo": map[string]string{"full_name": "acme/widgets"}}
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
		write([]any{issue(7, "widget", nil, false), issue(8, "other", []string{"ready"}, false), issue(11, "widget", f.labels, true)})
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
		if f.failReadback {
			f.failReadback = false
			http.Error(w, "review readback unavailable", http.StatusInternalServerError)
			return
		}
		write(f.summaries)
	case r.Method == http.MethodGet && path == "/issues/7/comments":
		write(f.sourceComments)
	case r.Method == http.MethodGet && path == "/issues/8/comments":
		write([]any{})
	case r.Method == http.MethodGet && path == "/issues/11/comments":
		write(f.summaries)
	case r.Method == http.MethodGet && path == "/pulls/11/comments":
		write(f.inlines)
	case r.Method == http.MethodPost && path == "/pulls/11/reviews":
		if f.failSummary {
			http.Error(w, "summary unavailable", http.StatusInternalServerError)
			return
		}
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		value["state"] = map[string]string{"REQUEST_CHANGES": "CHANGES_REQUESTED", "APPROVE": "APPROVED", "COMMENT": "COMMENTED"}[value["event"].(string)]
		f.summaries = append(f.summaries, value)
		if f.failPostRead {
			f.failPostRead = false
			f.failReadback = true
		}
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
		if f.failInline {
			f.failInline = false
			http.Error(w, "inline response lost", http.StatusInternalServerError)
		}
	case r.Method == http.MethodPost && path == "/issues/11/comments":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		f.summaries = append(f.summaries, value)
	case r.Method == http.MethodPatch && path == "/pulls/11":
		var value map[string]string
		_ = json.NewDecoder(r.Body).Decode(&value)
		f.body = value["body"]
		if f.failBody {
			f.failBody = false
			http.Error(w, "body response lost", http.StatusInternalServerError)
		}
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
				f.timeline = append(f.timeline, map[string]any{"event": "labeled", "label": map[string]string{"name": label}})
			}
		}
	case r.Method == http.MethodDelete && strings.Contains(path, "/labels/"):
		label := path[strings.LastIndex(path, "/")+1:]
		if label == f.failDelete {
			f.failDelete = ""
			http.Error(w, "label deletion unavailable", http.StatusInternalServerError)
			return
		}
		f.labels = slices.DeleteFunc(f.labels, func(current string) bool { return current == label })
		f.timeline = append(f.timeline, map[string]any{"event": "unlabeled", "label": map[string]string{"name": label}})
		if label == "wip" && f.failFinalRead {
			f.failItemsRead = true
		}
		if label == f.loseDelete {
			f.loseDelete = ""
			http.Error(w, "label deletion response lost", http.StatusInternalServerError)
			return
		}
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
	forge := &reviewForge{head: head, labels: []string{"review"}, mergeable: true}
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
				other := filepath.Join(f.root, ".worktrees", "other")
				runGit(t, f.root, "worktree", "add", "-b", "other", other, "main")
				otherGitDir := strings.TrimSpace(runGitOutput(t, other, "rev-parse", "--absolute-git-dir"))
				otherCheckpoint := filepath.Join(otherGitDir, ".watchdog")
				mainHead := strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "main"))
				if err := os.WriteFile(otherCheckpoint, []byte("9:"+mainHead+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				gitFileBefore := readFile(t, filepath.Join(f.worktree, ".git"))
				location := map[string]string{"primary": f.root, "selected": f.worktree}[caller]
				if caller == "other" {
					location = other
				}
				start := f.start(t, location)
				if start.Packet.Facts.Watchdog.ReviewNumber != 1 || strings.Contains(start.Packet.Instructions, ".watchdog") {
					t.Fatalf("encapsulation: %#v", start.Packet.Facts.Watchdog)
				}
				f.submit(t, 1, f.head, "rework")
				if got := strings.TrimSpace(readFile(t, f.checkpoint)); got != "1:"+f.head {
					t.Fatalf("checkpoint = %q", got)
				}
				if got := readFile(t, otherCheckpoint); got != "9:"+mainHead+"\n" {
					t.Fatalf("other Work Item checkpoint changed: %q", got)
				}
				if readFile(t, filepath.Join(f.worktree, ".git")) != gitFileBefore {
					t.Fatal("linked worktree .git file was used as storage")
				}
				common := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "--git-common-dir"))
				if !filepath.IsAbs(common) {
					common = filepath.Join(f.worktree, common)
				}
				if fileExists(filepath.Join(common, ".watchdog")) {
					t.Fatal("common Git directory used as checkpoint registry")
				}
			})
		}
	})

	t.Run("B2 count and scope facts", func(t *testing.T) {
		for _, tc := range []struct {
			name          string
			count, number uint64
			scope, head   string
		}{
			{"absent", 0, 1, "full", "absent"},
			{"zero at current head", 0, 1, "full", "current"},
			{"one at ancestor", 1, 2, "incremental", "ancestor"},
			{"one at current head", 1, 2, "incremental", "current"},
			{"unavailable prior head", 2, 3, "full", strings.Repeat("f", 40)},
			{"available nonancestor", 2, 3, "full", "nonancestor"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newReviewFixture(t)
				if tc.head != "absent" {
					head := tc.head
					if head == "current" {
						head = f.head
					} else if head == "ancestor" {
						head = strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "main"))
					} else if head == "nonancestor" {
						tree := strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "main^{tree}"))
						head = strings.TrimSpace(runGitOutput(t, f.root, "commit-tree", tree, "-m", "unrelated"))
					}
					if err := os.WriteFile(f.checkpoint, []byte(strconv.FormatUint(tc.count, 10)+":"+head+"\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				facts := f.start(t, f.root).Packet.Facts.Watchdog
				if facts.ReviewCount != tc.count || facts.ReviewNumber != tc.number || string(facts.ReviewScope) != tc.scope {
					t.Fatalf("facts = %#v", facts)
				}
			})
		}
	})

	t.Run("B3 resume refreshes PR head", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.sourceComments = append(f.forge.sourceComments, map[string]any{
			"body":               "<!-- watchdog-checkpoint review-count=99 reviewed-head=" + strings.Repeat("a", 40) + " -->",
			"author_association": "OWNER",
		})
		first := f.start(t, f.root).Packet.Facts.Watchdog
		if first.ReviewCount != 0 || fileExists(f.checkpoint) {
			t.Fatalf("stale forge metadata affected local checkpoint: %#v", first)
		}
		runGit(t, f.worktree, "commit", "--allow-empty", "-m", "move")
		f.forge.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
		resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7").Packet.Facts.Watchdog
		if resumed.ReviewedHead == first.ReviewedHead || resumed.ReviewNumber != first.ReviewNumber || !strings.Contains(resumed.SubmitCommand, "--reviewed-head "+f.forge.head) {
			t.Fatalf("resume facts = %#v", resumed)
		}
		if first.ReviewedHead == f.forge.head || len(f.forge.sourceComments) != 1 || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
			t.Fatal("resume mutated prior packet, source metadata, or unrelated Workflow State")
		}
	})

	t.Run("B4 invalid checkpoint and submit inputs refuse", func(t *testing.T) {
		for _, invalid := range []string{"", "\n", " 1:" + strings.Repeat("a", 40) + "\n", "-1:" + strings.Repeat("a", 40), "+1:" + strings.Repeat("a", 40), "1:abc", "1:" + strings.Repeat("a", 39), "1:" + strings.Repeat("a", 64), "1:" + strings.Repeat("a", 40) + ":x", "1:" + strings.Repeat("a", 40) + "\nextra"} {
			t.Run(strconv.Quote(invalid), func(t *testing.T) {
				f := newReviewFixture(t)
				if err := os.WriteFile(f.checkpoint, []byte(invalid), 0600); err != nil {
					t.Fatal(err)
				}
				got := f.start(t, f.root)
				if got.Status != "fix_required" || !strings.Contains(got.Reason, "Review Checkpoint") || len(f.forge.summaries) != 0 || slices.Contains(f.forge.labels, "wip") {
					t.Fatalf("accepted %q: %#v", invalid, got)
				}
			})
		}
		t.Run("unreadable checkpoint", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0000)
			defer os.Chmod(f.checkpoint, 0600)
			if got := f.start(t, f.root); got.Status != "fix_required" || slices.Contains(f.forge.labels, "wip") {
				t.Fatalf("unreadable checkpoint accepted: %#v", got)
			}
		})
		t.Run("selected worktree cannot be resolved", func(t *testing.T) {
			f := newReviewFixture(t)
			runGit(t, f.root, "worktree", "remove", "--force", f.worktree)
			if got := f.start(t, f.root); got.Status != "fix_required" || slices.Contains(f.forge.labels, "wip") {
				t.Fatalf("missing selected worktree accepted: %#v", got)
			}
		})
		for _, number := range []string{"0", "-1", "18446744073709551616"} {
			t.Run("submit number "+number, func(t *testing.T) {
				f := newReviewFixture(t)
				dir := t.TempDir()
				summary := filepath.Join(dir, "summary.md")
				_ = os.WriteFile(summary, []byte("summary"), 0600)
				_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", number, "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
				if err == nil || len(f.forge.summaries) != 0 || slices.Contains(f.forge.labels, "wip") {
					t.Fatalf("accepted review number %q: %v", number, err)
				}
			})
		}
		for _, head := range []string{"", "abc", strings.Repeat("a", 39), strings.Repeat("a", 64)} {
			t.Run("submit head "+strconv.Quote(head), func(t *testing.T) {
				f := newReviewFixture(t)
				dir := t.TempDir()
				summary := filepath.Join(dir, "summary.md")
				_ = os.WriteFile(summary, []byte("summary"), 0600)
				args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--verdict", "rework", "--summary", summary}
				if head != "" {
					args = append(args, "--reviewed-head", head)
				}
				got, err := f.runResult(f.worktree, args...)
				refused := err != nil || got.Status == "fix_required"
				if !refused || len(f.forge.summaries) != 0 {
					t.Fatalf("accepted reviewed head %q: %#v %v", head, got, err)
				}
			})
		}
		t.Run("submit refuses corrupted checkpoint", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("corrupt\n"), 0600)
			got := f.submit(t, 1, f.head, "rework")
			if got.Status != "fix_required" || len(f.forge.summaries) != 0 || slices.Contains(f.forge.labels, "wip") {
				t.Fatalf("corrupted checkpoint accepted: %#v", got)
			}
		})
	})

	t.Run("B5 completed verdict count policy", func(t *testing.T) {
		for _, tc := range []struct {
			before, number  uint64
			verdict, status string
		}{
			{0, 1, "rework", "rework"}, {1, 2, "rework", "needs_human"}, {2, 3, "rework", "needs_human"},
			{0, 1, "needs-human", "needs_human"}, {1, 2, "needs-human", "needs_human"}, {2, 3, "needs-human", "needs_human"},
			{0, 1, "pass", "ready_for_merge"}, {1, 2, "pass", "ready_for_merge"}, {2, 3, "pass", "ready_for_merge"},
		} {
			f := newReviewFixture(t)
			if tc.before > 0 {
				_ = os.WriteFile(f.checkpoint, []byte(fmt.Sprintf("%d:%s\n", tc.before, f.head)), 0600)
			}
			f.start(t, f.root)
			got := f.submit(t, tc.number, f.head, tc.verdict)
			if got.Status != tc.status {
				t.Fatalf("%+v: %#v", tc, got)
			}
			if tc.verdict == "pass" {
				if fileExists(f.checkpoint) {
					t.Fatalf("pass retained checkpoint: %+v", tc)
				}
			} else if strings.TrimSpace(readFile(t, f.checkpoint)) != fmt.Sprintf("%d:%s", tc.number, f.head) {
				t.Fatalf("completed verdict did not record exact count: %+v", tc)
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
		f.forge.labels = []string{"review"} // Explicit human requeue.
		if next := f.start(t, f.root).Packet.Facts.Watchdog; next.ReviewNumber != 2 || next.ReviewCount != 1 {
			t.Fatalf("requeue did not start a new round: %#v", next)
		}
		second := f.submit(t, 2, f.head, "rework")
		if first.Status != "needs_human" || second.Status != "needs_human" || strings.TrimSpace(readFile(t, f.checkpoint)) != "2:"+f.head {
			t.Fatalf("same SHA rounds: %#v %#v", first, second)
		}
		if retry := f.submit(t, 2, f.head, "rework"); retry.Status != "needs_human" || len(f.forge.summaries) != comments+1 {
			t.Fatalf("second exact retry duplicated evidence: %#v", retry)
		}
	})

	t.Run("B7 evidence precedes checkpoint and retries exactly", func(t *testing.T) {
		t.Run("summary write unapplied", func(t *testing.T) {
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
		t.Run("summary readback unavailable", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.forge.failPostRead = true
			dir := t.TempDir()
			summary := filepath.Join(dir, "summary.md")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
			if err == nil || fileExists(f.checkpoint) || len(f.forge.summaries) != 1 {
				t.Fatalf("readback failure ordering: %v", err)
			}
			f.submit(t, 1, f.head, "rework")
			if len(f.forge.summaries) != 1 {
				t.Fatal("readback repair duplicated summary")
			}
		})
		t.Run("inline response lost", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.forge.failInline = true
			dir := t.TempDir()
			summary, body, findings := filepath.Join(dir, "summary.md"), filepath.Join(dir, "inline.md"), filepath.Join(dir, "findings.json")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			_ = os.WriteFile(body, []byte("finding"), 0600)
			_ = os.WriteFile(findings, []byte(fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, body)), 0600)
			got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary, "--findings", findings)
			if got.Status != "rework" || len(f.forge.inlines) != 1 || len(f.forge.summaries) != 1 {
				t.Fatalf("inline response recovery: %#v", got)
			}
		})
		t.Run("body response lost", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.forge.failBody = true
			got := f.submit(t, 1, f.head, "pass")
			if got.Status != "ready_for_merge" || f.forge.body != "final\n\nCloses #7\n" || len(f.forge.summaries) != 1 {
				t.Fatalf("body response recovery: %#v body=%q", got, f.forge.body)
			}
		})
	})

	t.Run("B8 checkpoint replacement is atomic", func(t *testing.T) {
		t.Run("temporary creation failure", func(t *testing.T) {
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
			if retry := f.submit(t, 2, f.head, "rework"); retry.Status != "needs_human" || len(f.forge.summaries) != 1 {
				t.Fatalf("repaired fixed-number retry: %#v", retry)
			}
		})
		t.Run("rename failure", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.start(t, f.root)
			gitDir := filepath.Dir(f.checkpoint)
			changed := make(chan struct{})
			stop := make(chan struct{})
			go func() {
				for {
					entries, _ := os.ReadDir(gitDir)
					for _, entry := range entries {
						if strings.HasPrefix(entry.Name(), ".watchdog-") {
							_ = os.Chmod(gitDir, 0500)
							close(changed)
							return
						}
					}
					select {
					case <-stop:
						return
					default:
						time.Sleep(50 * time.Microsecond)
					}
				}
			}()
			got := f.submit(t, 2, f.head, "rework")
			close(stop)
			select {
			case <-changed:
			default:
				t.Fatal("did not intercept atomic replacement before rename")
			}
			_ = os.Chmod(gitDir, 0700)
			if got.Status != "fix_required" || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head || !slices.Contains(f.forge.labels, "wip") {
				t.Fatalf("rename failure damaged old checkpoint: %#v", got)
			}
			if retry := f.submit(t, 2, f.head, "rework"); retry.Status != "needs_human" || len(f.forge.summaries) != 1 {
				t.Fatalf("rename repair retry: %#v", retry)
			}
		})
	})

	t.Run("B9 fixed-number retry after checkpoint replacement", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			configure func(*reviewForge)
		}{
			{"after checkpoint before destination publication", func(f *reviewForge) { f.failHandoff = true }},
			{"after destination before source cleanup", func(f *reviewForge) { f.failDelete = "review" }},
			{"during claim release", func(f *reviewForge) { f.loseDelete, f.failFinalRead = "wip", true }},
			{"after verified nonterminal state before caller success", func(f *reviewForge) { f.failFinalRead = true }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newReviewFixture(t)
				f.start(t, f.root)
				tc.configure(f.forge)
				dir := t.TempDir()
				summary := filepath.Join(dir, "summary.md")
				_ = os.WriteFile(summary, []byte("round 1"), 0600)
				_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
				if err == nil || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head {
					t.Fatalf("checkpoint ordering: %v", err)
				}
				if retry := f.submit(t, 1, f.head, "rework"); retry.Status != "rework" || len(f.forge.summaries) != 1 || !slices.Equal(f.forge.labels, []string{"rework"}) {
					t.Fatalf("fixed retry: %#v labels=%v", retry, f.forge.labels)
				}
			})
		}
	})

	t.Run("B10 ambiguous commands stop", func(t *testing.T) {
		t.Run("recorded rework cannot be replayed as pass", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.forge.failHandoff = true
			dir := t.TempDir()
			summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "body.md")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			_ = os.WriteFile(body, []byte("incompatible pass"), 0600)
			_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
			if err == nil || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head {
				t.Fatalf("failed to arrange interrupted rework: %v", err)
			}
			got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--body", body)
			if got.Status != "fix_required" || f.forge.body != "" || slices.Contains(f.forge.labels, "done") {
				t.Fatalf("incompatible retry mutated publication: %#v body=%q labels=%v", got, f.forge.body, f.forge.labels)
			}
		})
		for _, tc := range []struct {
			name, state string
		}{
			{"checkpoint without durable evidence", ""},
			{"checkpoint with incompatible pass evidence", "APPROVED"},
			{"checkpoint with needs-human evidence", "COMMENTED"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newReviewFixture(t)
				_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
				f.forge.labels = []string{"review", "wip"}
				if tc.state != "" {
					f.forge.summaries = append(f.forge.summaries, map[string]any{"body": "round 1", "commit_id": f.head, "state": tc.state})
				}
				got := f.submit(t, 1, f.head, "rework")
				expectedSummaries := 0
				if tc.state != "" {
					expectedSummaries = 1
				}
				if got.Status != "fix_required" || !slices.Equal(f.forge.labels, []string{"review", "wip"}) || len(f.forge.summaries) != expectedSummaries {
					t.Fatalf("ambiguous retry mutated state: %#v", got)
				}
			})
		}
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
		t.Run("pass with pushed descending final head", func(t *testing.T) {
			f := newReviewFixture(t)
			packet := f.start(t, f.root).Packet
			if !strings.Contains(packet.Instructions, "Debt Marker") || strings.Contains(packet.Instructions, "Full Gate after") {
				t.Fatalf("post-marker guidance: %q", packet.Instructions)
			}
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
		for _, tc := range []struct {
			name, verdict string
			arrange       func(*testing.T, *reviewFixture) string
			withFinal     bool
		}{
			{"final head not pushed", "pass", func(t *testing.T, f *reviewFixture) string {
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "local only")
				return strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
			}, true},
			{"final head not descendant", "pass", func(t *testing.T, f *reviewFixture) string {
				tree := strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "main^{tree}"))
				final := strings.TrimSpace(runGitOutput(t, f.root, "commit-tree", tree, "-m", "unrelated final"))
				runGit(t, f.worktree, "reset", "--hard", final)
				f.forge.head = final
				return final
			}, true},
			{"rework with different final head", "rework", func(t *testing.T, f *reviewFixture) string {
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "different")
				final := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
				f.forge.head = final
				return final
			}, true},
			{"needs-human with different final head", "needs-human", func(t *testing.T, f *reviewFixture) string {
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "different")
				final := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
				f.forge.head = final
				return final
			}, true},
			{"PR moved without final-head override", "pass", func(t *testing.T, f *reviewFixture) string {
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "moved")
				final := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
				f.forge.head = final
				return final
			}, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newReviewFixture(t)
				f.start(t, f.root)
				reviewed := f.head
				final := tc.arrange(t, f)
				dir := t.TempDir()
				summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "body.md")
				_ = os.WriteFile(summary, []byte("summary"), 0600)
				_ = os.WriteFile(body, []byte("body"), 0600)
				args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", reviewed, "--verdict", tc.verdict, "--summary", summary}
				if tc.withFinal {
					args = append(args, "--head", final)
				}
				if tc.verdict == "pass" {
					args = append(args, "--body", body)
				}
				got := f.run(t, f.worktree, args...)
				if got.Status != "fix_required" || fileExists(f.checkpoint) || len(f.forge.summaries) != 0 || !slices.Contains(f.forge.labels, "wip") {
					t.Fatalf("unsafe final head accepted: %#v", got)
				}
			})
		}
	})

	t.Run("B12 implementation and Audit need no previous review cache", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.labels = []string{"rework"}
		got := f.run(t, f.worktree, "implement", "next")
		if got.Status != "work_available" || strings.Contains(got.Packet.Instructions, "previous-reviewed-head") || strings.Contains(got.Packet.Instructions, "--reviewed-head") {
			t.Fatalf("rework packet: %#v", got)
		}
		resumed := f.run(t, f.root, "implement", "resume", "--item", "7")
		for _, instructions := range []string{got.Packet.Instructions, resumed.Packet.Instructions} {
			if strings.Contains(instructions, "previous-reviewed-head") || strings.Contains(instructions, "cache repair") || strings.Contains(instructions, "required previous-review") || !strings.Contains(instructions, "Audit") {
				t.Fatalf("implementation/Audit fallback instructions: %q", instructions)
			}
		}
	})

	t.Run("B13 nonterminal work retains checkpoint", func(t *testing.T) {
		assertRetained := func(t *testing.T, f *reviewFixture) {
			t.Helper()
			if strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head {
				t.Fatal("nonterminal operation removed checkpoint")
			}
		}
		t.Run("Needs Human pause and explicit requeue", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.submit(t, 1, f.head, "needs-human")
			f.forge.labels = []string{"review"}
			facts := f.start(t, f.root).Packet.Facts.Watchdog
			assertRetained(t, f)
			if facts.ReviewNumber != 2 || facts.ReviewCount != 1 {
				t.Fatalf("requeue lost completed history: %#v", facts)
			}
		})
		t.Run("finding implementation and resubmission", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"rework"}
			start := f.run(t, f.worktree, "implement", "next")
			body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			_ = os.WriteFile(body, []byte("resubmission"), 0600)
			if got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body); got.Status != "awaiting_review" {
				t.Fatalf("resubmission: %#v", got)
			}
			assertRetained(t, f)
		})
		t.Run("Implementation Ledger retirement", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			if fileExists(filepath.Join(f.worktree, ".changes", "widget")) {
				t.Fatal("fixture did not retire Implementation Ledger")
			}
			assertRetained(t, f)
		})
		t.Run("ordinary git clean", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			_ = os.WriteFile(filepath.Join(f.worktree, "untracked"), []byte("x"), 0600)
			runGit(t, f.worktree, "clean", "-fd")
			assertRetained(t, f)
		})
	})

	t.Run("B14 only verified done deletes checkpoint", func(t *testing.T) {
		f := newReviewFixture(t)
		f.start(t, f.root)
		f.forge.failHandoff = true
		dir := t.TempDir()
		summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "body.md")
		_ = os.WriteFile(summary, []byte("pass"), 0600)
		_ = os.WriteFile(body, []byte("final"), 0600)
		_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--body", body)
		if err == nil || !fileExists(f.checkpoint) || !slices.Contains(f.forge.labels, "wip") {
			t.Fatalf("unverified done removed retry context: %v", err)
		}

		f = newReviewFixture(t)
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

		f = newReviewFixture(t)
		start := f.start(t, f.root)
		resultDir := start.Packet.Facts.Watchdog.ResultDirectory
		summary, body = filepath.Join(resultDir, "summary.md"), filepath.Join(resultDir, "submission.md")
		_ = os.WriteFile(summary, []byte("pass"), 0600)
		_ = os.WriteFile(body, []byte("final"), 0600)
		_ = os.Mkdir(filepath.Join(resultDir, "unexpected"), 0700)
		got = f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--body", body)
		if got.Status != "ready_for_merge" || fileExists(f.checkpoint) || !strings.Contains(got.Reason, "unexpected files") || len(f.forge.summaries) != 1 {
			t.Fatalf("Result Document cleanup warning: %#v", got)
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
		f = newReviewFixture(t)
		f.forge.timeline = []map[string]any{{"event": "labeled", "label": map[string]string{"name": "rework"}}}
		f.forge.mergeable = false
		_ = os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600)
		f.start(t, f.root)
		if got := f.submit(t, 3, f.head, "pass"); got.Status != "rework" || strings.TrimSpace(readFile(t, f.checkpoint)) != "3:"+f.head {
			t.Fatalf("independent conflict diversion consumed review history: %#v", got)
		}
		before := readFile(t, f.checkpoint)
		if got := f.run(t, f.root, "status"); got.Status != "observed" || readFile(t, f.checkpoint) != before {
			t.Fatalf("status rewrote conflict-diverted checkpoint: %#v", got)
		}

		f = newReviewFixture(t)
		_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
		f.forge.labels = []string{"review", "wip"}
		if got := f.submit(t, 1, f.head, "rework"); got.Status != "fix_required" || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
			t.Fatalf("caller invented completion without evidence: %#v", got)
		}

		f = newReviewFixture(t)
		f.forge.labels = []string{"review", "rework", "wip"}
		f.forge.timeline = []map[string]any{{"event": "labeled", "label": map[string]string{"name": "review"}}, {"event": "labeled", "label": map[string]string{"name": "rework"}}}
		_ = os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600)
		before = readFile(t, f.checkpoint)
		status := f.run(t, f.root, "status")
		if status.Status != "observed" || readFile(t, f.checkpoint) != before {
			t.Fatalf("status invented completion: %#v", status)
		}
	})

	t.Run("B16 recreated worktree starts fresh", func(t *testing.T) {
		f := newReviewFixture(t)
		_ = os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600)
		f.forge.timeline = []map[string]any{{"event": "labeled", "label": map[string]string{"name": "rework"}}}
		f.forge.summaries = []map[string]any{{"body": "old completed review", "commit_id": f.head, "state": "CHANGES_REQUESTED"}}
		f.forge.sourceComments = []map[string]any{{"body": "<!-- watchdog-checkpoint review-count=2 reviewed-head=" + f.head + " -->", "author_association": "OWNER"}}
		runGit(t, f.root, "worktree", "remove", "--force", f.worktree)
		runGit(t, f.root, "worktree", "add", f.worktree, "widget")
		facts := f.start(t, f.root).Packet.Facts.Watchdog
		if facts.ReviewCount != 0 || facts.ReviewNumber != 1 || facts.ReviewScope != "full" {
			t.Fatalf("recreated facts: %#v", facts)
		}
		if got := f.submit(t, 1, f.head, "rework"); got.Status != "rework" || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head || len(f.forge.summaries) != 2 {
			t.Fatalf("fresh retained-worktree budget did not complete round 1: %#v", got)
		}
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
