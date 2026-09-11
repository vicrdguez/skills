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
	branch         string
	remoteHead     string
	pullHead       string
	body           string
	labels         []string
	summaries      []map[string]any
	issueComments  []map[string]any
	inlines        []map[string]any
	sourceComments []map[string]any
	timeline       []map[string]any
	failSummary    bool
	failAfterWrite bool
	failHandoff    bool
	failInline     bool
	failBody       bool
	failReadback   bool
	duplicateRead  bool
	reviewReads    int
	failPostRead   bool
	failDelete     string
	loseDelete     string
	failItemsRead  bool
	failFinalRead  bool
	denyCleanup    string
	mergeable      bool
	checkpointPath string
	atWipRelease   string
	clock          int
}

func (f *reviewForge) timestamp() string {
	f.clock++
	return time.Date(2026, 1, 1, 0, 0, f.clock, 0, time.UTC).Format(time.RFC3339Nano)
}

func (f *reviewForge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
		}
		return result
	}
	pull := func() map[string]any {
		head := f.head
		if f.pullHead != "" {
			head = f.pullHead
		}
		result := issue(11, branch, f.labels, true)
		result["body"], result["draft"], result["merged"], result["mergeable"] = f.body, false, false, f.mergeable
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
		write([]any{issue(7, branch, nil, false), issue(8, "other", []string{"ready"}, false), issue(11, branch, f.labels, true)})
	case r.Method == http.MethodGet && path == "/pulls":
		write([]any{pull()})
	case r.Method == http.MethodGet && path == "/pulls/11":
		write(pull())
	case r.Method == http.MethodGet && path == "/git/ref/heads/widget":
		head := f.head
		if f.remoteHead != "" {
			head = f.remoteHead
		}
		write(map[string]any{"object": map[string]string{"sha": head}})
	case r.Method == http.MethodGet && path == "/git/ref/heads/main":
		write(map[string]any{"object": map[string]string{"sha": f.head}})
	case r.Method == http.MethodGet && path == "/issues/11/timeline":
		write(f.timeline)
	case r.Method == http.MethodGet && path == "/pulls/11/reviews":
		f.reviewReads++
		if f.duplicateRead && f.reviewReads == 2 {
			f.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED", "submitted_at": f.timestamp()}, {"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED", "submitted_at": f.timestamp()}}
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
		write([]any{})
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
		value["state"] = map[string]string{"REQUEST_CHANGES": "CHANGES_REQUESTED", "APPROVE": "APPROVED", "COMMENT": "COMMENTED"}[value["event"].(string)]
		value["submitted_at"] = f.timestamp()
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
		f.issueComments = append(f.issueComments, value)
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
			write(issue(7, branch, nil, false))
		} else {
			write(issue(11, branch, f.labels, true))
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
				f.timeline = append(f.timeline, map[string]any{"event": "labeled", "created_at": f.timestamp(), "label": map[string]string{"name": label}})
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
		f.labels = slices.DeleteFunc(f.labels, func(current string) bool { return current == label })
		f.timeline = append(f.timeline, map[string]any{"event": "unlabeled", "created_at": f.timestamp(), "label": map[string]string{"name": label}})
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
	forge := &reviewForge{head: head, labels: []string{"review"}, mergeable: true, clock: 1, timeline: []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "review"}}}}
	server := httptest.NewServer(forge)
	t.Cleanup(server.Close)
	checkpoint := filepath.Join(gitDir, ".watchdog")
	forge.checkpointPath = checkpoint
	return &reviewFixture{root: root, worktree: worktree, head: head, checkpoint: checkpoint, forge: forge, server: server}
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
	return f.submitAt(t, f.worktree, number, head, verdict)
}

func (f *reviewFixture) submitAt(t *testing.T, caller string, number uint64, head, verdict string) setup.ImplementationOutput {
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
	return f.run(t, caller, args...)
}

func TestWatchdogReviewCheckpoints(t *testing.T) {
	t.Run("B1 selected linked worktree owns checkpoint", func(t *testing.T) {
		for _, caller := range []string{"primary", "selected", "other"} {
			t.Run(caller, func(t *testing.T) {
				f := newReviewFixture(t)
				if err := os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
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
				start := setup.ImplementationOutput{}
				if caller == "other" {
					f.start(t, f.root)
					start = f.run(t, location, "watchdog", "resume", "--item", "7")
				} else {
					start = f.start(t, location)
				}
				if start.Packet.Facts.Watchdog.ReviewCount != 2 || start.Packet.Facts.Watchdog.ReviewNumber != 3 || strings.Contains(start.Packet.Instructions, ".watchdog") {
					t.Fatalf("encapsulation: %#v", start.Packet.Facts.Watchdog)
				}
				f.submitAt(t, location, 3, f.head, "rework")
				if got := strings.TrimSpace(readFile(t, f.checkpoint)); got != "3:"+f.head {
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
				checkpointHead := ""
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
					checkpointHead = head
				}
				before := checkpointSnapshot(f.checkpoint)
				for _, started := range []setup.ImplementationOutput{f.start(t, f.root), f.run(t, f.root, "watchdog", "resume", "--item", "7")} {
					facts := started.Packet.Facts.Watchdog
					if facts.ReviewCount != tc.count || facts.ReviewNumber != tc.number || string(facts.ReviewScope) != tc.scope {
						t.Fatalf("facts = %#v", facts)
					}
					if tc.scope == "incremental" {
						comparison := "Compare `" + checkpointHead + "..." + f.head + "`"
						if facts.PreviousReviewedHead != checkpointHead || !strings.Contains(started.Packet.Instructions, comparison) {
							t.Fatalf("incremental comparison missing: %#v\n%s", facts, started.Packet.Instructions)
						}
					} else if facts.PreviousReviewedHead != "" || !strings.Contains(started.Packet.Instructions, "Review the full PR comparison") {
						t.Fatalf("full fallback missing: %#v\n%s", facts, started.Packet.Instructions)
					}
					if checkpointSnapshot(f.checkpoint) != before {
						t.Fatalf("startup mutated checkpoint: before=%q after=%q", before, checkpointSnapshot(f.checkpoint))
					}
				}
			})
		}
	})

	t.Run("B3 resume refreshes PR head", func(t *testing.T) {
		f := newReviewFixture(t)
		prior := strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "main"))
		_ = os.WriteFile(f.checkpoint, []byte("1:"+prior+"\n"), 0600)
		stale := `{"watchdog_head":"` + strings.Repeat("a", 40) + `","reviewed_head":"` + strings.Repeat("b", 40) + `","review_round_head":"` + strings.Repeat("c", 40) + `"}`
		f.forge.sourceComments = append(f.forge.sourceComments, map[string]any{
			"body":               stale,
			"author_association": "OWNER",
		})
		before := checkpointSnapshot(f.checkpoint)
		started := f.start(t, f.root)
		first := started.Packet.Facts.Watchdog
		if first.ReviewCount != 1 || first.ReviewNumber != 2 || first.PreviousReviewedHead != prior || started.Packet.Facts.Watchdog.WorkItem != 7 || checkpointSnapshot(f.checkpoint) != before {
			t.Fatalf("stale forge metadata affected local checkpoint: %#v", first)
		}
		runGit(t, f.worktree, "commit", "--allow-empty", "-m", "move")
		f.forge.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
		resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7").Packet.Facts.Watchdog
		if resumed.ReviewedHead == first.ReviewedHead || resumed.ReviewNumber != first.ReviewNumber || resumed.ReviewCount != 1 || !strings.Contains(resumed.SubmitCommand, "--reviewed-head "+f.forge.head) || checkpointSnapshot(f.checkpoint) != before {
			t.Fatalf("resume facts = %#v", resumed)
		}
		if first.ReviewedHead == f.forge.head || len(f.forge.sourceComments) != 1 || f.forge.sourceComments[0]["body"] != stale || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
			t.Fatal("resume mutated prior packet, source metadata, or unrelated Workflow State")
		}
	})

	t.Run("B4 invalid checkpoint and submit inputs refuse", func(t *testing.T) {
		for _, invalid := range []string{"", "\n", "1:", " 1:" + strings.Repeat("a", 40) + "\n", "x:" + strings.Repeat("a", 40), "-1:" + strings.Repeat("a", 40), "+1:" + strings.Repeat("a", 40), strconv.FormatUint(uint64(^uint(0)>>1)+1, 10) + ":" + strings.Repeat("a", 40), "18446744073709551616:" + strings.Repeat("a", 40), "1:abc", "1:" + strings.Repeat("a", 39), "1:" + strings.Repeat("g", 40), "1:" + strings.Repeat("a", 64), "1:" + strings.Repeat("a", 40) + ":x", "1:" + strings.Repeat("a", 40) + "\nextra"} {
			for _, command := range []string{"next", "resume"} {
				t.Run(strconv.Quote(invalid)+" "+command, func(t *testing.T) {
					f := newReviewFixture(t)
					if err := os.WriteFile(f.checkpoint, []byte(invalid), 0600); err != nil {
						t.Fatal(err)
					}
					args := []string{"watchdog", command}
					if command == "resume" {
						f.forge.labels = []string{"review", "wip"}
						args = append(args, "--item", "7")
					}
					labels := append([]string(nil), f.forge.labels...)
					got := f.run(t, f.root, args...)
					if got.Status != "fix_required" || !strings.Contains(got.Reason, "Review Checkpoint") || !strings.Contains(got.Reason, "repair") || len(f.forge.summaries) != 0 || !slices.Equal(f.forge.labels, labels) {
						t.Fatalf("accepted %q through %s: %#v", invalid, command, got)
					}
				})
			}
		}
		for _, command := range []string{"next", "resume"} {
			t.Run("unreadable checkpoint "+command, func(t *testing.T) {
				f := newReviewFixture(t)
				_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0000)
				defer os.Chmod(f.checkpoint, 0600)
				args := []string{"watchdog", command}
				if command == "resume" {
					f.forge.labels = []string{"review", "wip"}
					args = append(args, "--item", "7")
				}
				labels := append([]string(nil), f.forge.labels...)
				if got := f.run(t, f.root, args...); got.Status != "fix_required" || !strings.Contains(got.Reason, "repair access") || !slices.Equal(f.forge.labels, labels) {
					t.Fatalf("unreadable checkpoint accepted through %s: %#v", command, got)
				}
			})
			t.Run("selected worktree cannot be resolved "+command, func(t *testing.T) {
				f := newReviewFixture(t)
				if command == "resume" {
					f.forge.labels = []string{"review", "wip"}
				}
				labels := append([]string(nil), f.forge.labels...)
				runGit(t, f.root, "worktree", "remove", "--force", f.worktree)
				args := []string{"watchdog", command}
				if command == "resume" {
					args = append(args, "--item", "7")
				}
				if got := f.run(t, f.root, args...); got.Status != "fix_required" || !strings.Contains(got.Reason, "resolve private Git directory") || !slices.Equal(f.forge.labels, labels) {
					t.Fatalf("missing selected worktree accepted through %s: %#v", command, got)
				}
			})
		}
		for _, branch := range []string{"team/widget", "../outside"} {
			for _, command := range []string{"next", "resume"} {
				t.Run("invalid checkpoint branch "+branch+" "+command, func(t *testing.T) {
					f := newReviewFixture(t)
					f.forge.branch = branch
					if command == "resume" {
						f.forge.labels = []string{"review", "wip"}
					}
					labels := append([]string(nil), f.forge.labels...)
					outside := filepath.Join(f.root, "outside", ".watchdog")
					_ = os.MkdirAll(filepath.Dir(outside), 0700)
					_ = os.WriteFile(outside, []byte("sentinel"), 0600)
					args := []string{"watchdog", command}
					if command == "resume" {
						args = append(args, "--item", "7")
					}
					got := f.run(t, f.root, args...)
					if got.Status != "fix_required" || !strings.Contains(got.Reason, "invalid conventional branch") || readFile(t, outside) != "sentinel" || !slices.Equal(f.forge.labels, labels) {
						t.Fatalf("unsafe branch reached checkpoint storage through %s: %#v", command, got)
					}
				})
			}
		}
		for _, number := range []string{"0", "-1", "18446744073709551616"} {
			t.Run("submit number "+number, func(t *testing.T) {
				f := newReviewFixture(t)
				f.start(t, f.root)
				dir := t.TempDir()
				summary := filepath.Join(dir, "summary.md")
				_ = os.WriteFile(summary, []byte("summary"), 0600)
				_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", number, "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
				if err == nil || len(f.forge.summaries) != 0 || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
					t.Fatalf("accepted review number %q: %v", number, err)
				}
			})
		}
		t.Run("missing review number", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			dir := t.TempDir()
			summary := filepath.Join(dir, "summary.md")
			_ = os.WriteFile(summary, []byte("summary"), 0600)
			_, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
			if err == nil || len(f.forge.summaries) != 0 || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
				t.Fatalf("accepted missing review number: %v", err)
			}
		})
		for _, head := range []string{"", "-abc", "abc", strings.Repeat("a", 39), strings.Repeat("g", 40), strings.Repeat("a", 64)} {
			t.Run("submit head "+strconv.Quote(head), func(t *testing.T) {
				f := newReviewFixture(t)
				f.start(t, f.root)
				dir := t.TempDir()
				summary := filepath.Join(dir, "summary.md")
				_ = os.WriteFile(summary, []byte("summary"), 0600)
				args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--verdict", "rework", "--summary", summary}
				if head != "" {
					args = append(args, "--reviewed-head", head)
				}
				got, err := f.runResult(f.worktree, args...)
				refused := err != nil || got.Status == "fix_required"
				if !refused || len(f.forge.summaries) != 0 || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
					t.Fatalf("accepted reviewed head %q: %#v %v", head, got, err)
				}
			})
		}
		for _, head := range []string{"-abc", "abc", strings.Repeat("a", 39), strings.Repeat("g", 40), strings.Repeat("a", 64)} {
			t.Run("submit final head "+strconv.Quote(head), func(t *testing.T) {
				f := newReviewFixture(t)
				f.start(t, f.root)
				dir := t.TempDir()
				summary := filepath.Join(dir, "summary.md")
				_ = os.WriteFile(summary, []byte("summary"), 0600)
				got, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--head="+head, "--verdict", "rework", "--summary", summary)
				if err != nil || got.Status != "fix_required" || len(f.forge.summaries) != 0 || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
					t.Fatalf("accepted final head %q: %#v %v", head, got, err)
				}
			})
		}
		t.Run("unreadable checkpoint on submit", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0000)
			defer os.Chmod(f.checkpoint, 0600)
			got := f.submit(t, 1, f.head, "rework")
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "repair access") || len(f.forge.summaries) != 0 || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
				t.Fatalf("unreadable submit checkpoint: %#v", got)
			}
		})
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
			if f.forge.atWipRelease != fmt.Sprintf("%d:%s\n", tc.number, f.head) {
				t.Fatalf("wip release observed checkpoint %q, want N:H", f.forge.atWipRelease)
			}
			if tc.verdict == "pass" {
				if fileExists(f.checkpoint) {
					t.Fatalf("pass retained checkpoint: %+v", tc)
				}
			} else if strings.TrimSpace(readFile(t, f.checkpoint)) != fmt.Sprintf("%d:%s", tc.number, f.head) {
				t.Fatalf("completed verdict did not record exact count: %+v", tc)
			}
			if got.Status == "needs_human" {
				want := "awaiting_review"
				if tc.verdict == "rework" {
					want = "rework"
				}
				if got.Item == nil || string(got.Item.ResumeState) != want {
					t.Fatalf("Needs Human ResumeState = %#v, want %s", got.Item, want)
				}
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
		f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "review"}})
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
		t.Run("exact command and Result Documents survive handoff retry", func(t *testing.T) {
			f := newReviewFixture(t)
			start := f.start(t, f.root)
			dir := start.Packet.Facts.Watchdog.ResultDirectory
			summary, inline, findings, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "inline.md"), filepath.Join(dir, "findings.json"), filepath.Join(dir, "submission.md")
			summaryBytes, inlineBytes := []byte("exact summary\n"), []byte("exact inline\n")
			_ = os.WriteFile(summary, summaryBytes, 0600)
			_ = os.WriteFile(inline, inlineBytes, 0600)
			_ = os.WriteFile(findings, []byte(fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, inline)), 0600)
			_ = os.WriteFile(body, []byte("exact final"), 0600)
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--findings", findings, "--body", body}
			f.forge.failHandoff = true
			if _, err := f.runResult(f.worktree, args...); err == nil {
				t.Fatal("handoff interruption was not observed")
			}
			if got := f.run(t, f.worktree, args...); got.Status != "ready_for_merge" {
				t.Fatalf("exact retry: %#v", got)
			}
			if len(f.forge.summaries) != 1 || f.forge.summaries[0]["body"] != string(summaryBytes) || f.forge.summaries[0]["commit_id"] != f.head || f.forge.summaries[0]["state"] != "APPROVED" {
				t.Fatalf("summary evidence = %#v", f.forge.summaries)
			}
			if len(f.forge.inlines) != 1 || f.forge.inlines[0]["body"] != string(inlineBytes) || f.forge.inlines[0]["commit_id"] != f.head || f.forge.inlines[0]["path"] != "README.md" || f.forge.inlines[0]["line"] != float64(1) || f.forge.inlines[0]["side"] != "RIGHT" {
				t.Fatalf("inline evidence = %#v", f.forge.inlines)
			}
			if f.forge.body != "exact final\n\nCloses #7\n" {
				t.Fatalf("final body = %q", f.forge.body)
			}
		})
	})

	t.Run("B8 checkpoint replacement is atomic", func(t *testing.T) {
		t.Run("absent prior temporary creation failure", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			gitDir := filepath.Dir(f.checkpoint)
			_ = os.Chmod(gitDir, 0500)
			got := f.submit(t, 1, f.head, "rework")
			_ = os.Chmod(gitDir, 0700)
			if got.Status != "fix_required" || checkpointSnapshot(f.checkpoint) != "<absent>" || !slices.Contains(f.forge.labels, "wip") {
				t.Fatalf("absent atomic failure: %#v", got)
			}
			assertNoCheckpointTemps(t, gitDir)
			if retry := f.submit(t, 1, f.head, "rework"); retry.Status != "rework" || len(f.forge.summaries) != 1 {
				t.Fatalf("absent repaired retry: %#v", retry)
			}
			assertNoCheckpointTemps(t, gitDir)
		})
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
			assertNoCheckpointTemps(t, gitDir)
			if retry := f.submit(t, 2, f.head, "rework"); retry.Status != "needs_human" || len(f.forge.summaries) != 1 {
				t.Fatalf("repaired fixed-number retry: %#v", retry)
			}
			assertNoCheckpointTemps(t, gitDir)
		})
		t.Run("rename failure", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.start(t, f.root)
			gitDir := filepath.Dir(f.checkpoint)
			changed := make(chan struct{})
			stop := make(chan struct{})
			// No portable filesystem setup denies rename while retaining C:P and still permits temp cleanup.
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
			assertNoCheckpointTemps(t, gitDir)
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
			for _, round := range []uint64{1, 2} {
				t.Run(tc.name+" round "+strconv.FormatUint(round, 10), func(t *testing.T) {
					f := newReviewFixture(t)
					if round == 2 {
						f.start(t, f.root)
						f.submit(t, 1, f.head, "needs-human")
						f.forge.labels = []string{"review"}
						f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "review"}})
					}
					f.start(t, f.root)
					tc.configure(f.forge)
					dir := t.TempDir()
					summary := filepath.Join(dir, "summary.md")
					body := "round " + strconv.FormatUint(round, 10)
					_ = os.WriteFile(summary, []byte(body), 0600)
					args := []string{"watchdog", "submit", "--item", "7", "--review-number", strconv.FormatUint(round, 10), "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary}
					_, err := f.runResult(f.worktree, args...)
					if err == nil || strings.TrimSpace(readFile(t, f.checkpoint)) != strconv.FormatUint(round, 10)+":"+f.head {
						t.Fatalf("checkpoint ordering: %v", err)
					}
					wantStatus, wantLabel := "rework", "rework"
					if round == 2 {
						wantStatus, wantLabel = "needs_human", "needs-human"
					}
					if retry := f.run(t, f.worktree, args...); retry.Status != wantStatus || len(f.forge.summaries) != int(round) || f.forge.summaries[round-1]["body"] != body || !slices.Equal(f.forge.labels, []string{wantLabel}) {
						t.Fatalf("fixed retry: %#v labels=%v", retry, f.forge.labels)
					}
				})
			}
		}
	})

	t.Run("B10 ambiguous commands stop", func(t *testing.T) {
		t.Run("old exact command cannot consume a human-requeued Claim", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.submit(t, 1, f.head, "needs-human")
			checkpoint := readFile(t, f.checkpoint)
			summaries := len(f.forge.summaries)
			f.forge.labels = []string{"review"} // Human requeue.
			f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "review"}})
			if facts := f.start(t, f.root).Packet.Facts.Watchdog; facts.ReviewNumber != 2 {
				t.Fatalf("fresh review number = %d", facts.ReviewNumber)
			}
			labels := append([]string(nil), f.forge.labels...)
			got := f.submit(t, 1, f.head, "needs-human")
			if got.Status != "fix_required" || readFile(t, f.checkpoint) != checkpoint || len(f.forge.summaries) != summaries || !slices.Equal(f.forge.labels, labels) {
				t.Fatalf("old command consumed fresh Claim: %#v labels=%v", got, f.forge.labels)
			}
		})
		t.Run("old round 2 command cannot consume a round 3 Claim", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.submit(t, 1, f.head, "needs-human")
			f.forge.labels = []string{"review"}
			f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "review"}})
			f.start(t, f.root)
			f.submit(t, 2, f.head, "needs-human")
			checkpoint, summaries := checkpointSnapshot(f.checkpoint), len(f.forge.summaries)
			f.forge.labels = []string{"review"}
			f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "review"}})
			if facts := f.start(t, f.root).Packet.Facts.Watchdog; facts.ReviewNumber != 3 {
				t.Fatalf("fresh review number = %d", facts.ReviewNumber)
			}
			labels := append([]string(nil), f.forge.labels...)
			if got := f.submit(t, 2, f.head, "needs-human"); got.Status != "fix_required" {
				t.Fatalf("old round 2 consumed round 3 Claim: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, summaries, 0, "")
		})
		t.Run("fresh round cannot reuse identical historical receipt", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.submit(t, 1, f.head, "needs-human")
			checkpoint, summaries := checkpointSnapshot(f.checkpoint), len(f.forge.summaries)
			f.forge.labels = []string{"review"}
			f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "review"}})
			if facts := f.start(t, f.root).Packet.Facts.Watchdog; facts.ReviewNumber != 2 {
				t.Fatalf("fresh review number = %d", facts.ReviewNumber)
			}
			dir := t.TempDir()
			summary := filepath.Join(dir, "summary.md")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			labels := append([]string(nil), f.forge.labels...)
			got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "2", "--reviewed-head", f.head, "--verdict", "needs-human", "--summary", summary)
			if got.Status != "fix_required" {
				t.Fatalf("fresh round reused historical receipt: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, summaries, 0, "")
		})
		for _, tc := range []struct{ name, claim, receipt string }{
			{"missing receipt ordering", "", "2026-01-01T00:00:02Z"},
			{"equal receipt ordering", "2026-01-01T00:00:02Z", "2026-01-01T00:00:02Z"},
			{"invalid receipt ordering", "not-a-time", "2026-01-01T00:00:02Z"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newReviewFixture(t)
				_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
				f.forge.labels = []string{"review", "wip"}
				f.forge.timeline = []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "review"}}, {"event": "labeled", "created_at": tc.claim, "label": map[string]string{"name": "wip"}}}
				f.forge.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED", "submitted_at": tc.receipt}}
				checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
				if got := f.submit(t, 1, f.head, "rework"); got.Status != "fix_required" {
					t.Fatalf("ambiguous receipt ordering accepted: %#v", got)
				}
				assertReviewUnchanged(t, f, checkpoint, labels, 1, 0, "")
			})
		}
		t.Run("receipt after review but before wip Claim", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			f.forge.timeline = []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "review"}}, {"event": "labeled", "created_at": "2026-01-01T00:00:03Z", "label": map[string]string{"name": "wip"}}}
			f.forge.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED", "submitted_at": "2026-01-01T00:00:02Z"}}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if got := f.submit(t, 1, f.head, "rework"); got.Status != "fix_required" {
				t.Fatalf("pre-Claim receipt consumed round: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 1, 0, "")
		})
		t.Run("ambiguous active wip Claim", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			f.forge.timeline = []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "review"}}, {"event": "labeled", "created_at": "2026-01-01T00:00:02Z", "label": map[string]string{"name": "wip"}}, {"event": "labeled", "created_at": "2026-01-01T00:00:03Z", "label": map[string]string{"name": "wip"}}}
			f.forge.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED", "submitted_at": "2026-01-01T00:00:04Z"}}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if got := f.submit(t, 1, f.head, "rework"); got.Status != "fix_required" {
				t.Fatalf("ambiguous Claim consumed round: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 1, 0, "")
		})
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
		t.Run("checkpoint SHA differs from command", func(t *testing.T) {
			f := newReviewFixture(t)
			prior := strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "main"))
			_ = os.WriteFile(f.checkpoint, []byte("1:"+prior+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			got := f.submit(t, 1, f.head, "rework")
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "original fixed-number") {
				t.Fatalf("mismatched checkpoint SHA accepted: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 0, 0, "")
		})
		t.Run("receipt read fails", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			f.forge.failReadback = true
			dir := t.TempDir()
			summary := filepath.Join(dir, "summary.md")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if _, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary); err == nil {
				t.Fatal("receipt read failure accepted")
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 0, 0, "")
		})
		t.Run("duplicate exact receipts are ambiguous", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			f.forge.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED"}, {"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED"}}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			got := f.submit(t, 1, f.head, "rework")
			if got.Status != "fix_required" {
				t.Fatalf("duplicate exact receipts accepted: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 2, 0, "")
		})
		t.Run("duplicate receipts observed immediately before publication", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.forge.reviewReads, f.forge.duplicateRead = 0, true
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			dir := t.TempDir()
			summary := filepath.Join(dir, "summary.md")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			if _, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary); err == nil {
				t.Fatal("publication reused duplicate exact receipts")
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 2, 0, "")
		})
		t.Run("summary body mismatch", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			f.forge.summaries = []map[string]any{{"body": "different", "commit_id": f.head, "state": "CHANGES_REQUESTED"}}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if got := f.submit(t, 1, f.head, "rework"); got.Status != "fix_required" {
				t.Fatalf("summary mismatch accepted: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 1, 0, "")
		})
		t.Run("final body mismatch", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			f.forge.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "APPROVED"}}
			f.forge.body = "different\n\nCloses #7\n"
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if got := f.submit(t, 1, f.head, "pass"); got.Status != "fix_required" {
				t.Fatalf("final body mismatch accepted: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 1, 0, "different\n\nCloses #7\n")
		})
		t.Run("inline evidence mismatch", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			f.forge.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED"}}
			f.forge.inlines = []map[string]any{{"body": "old", "commit_id": f.head, "path": "README.md", "line": float64(1), "side": "RIGHT"}}
			dir := t.TempDir()
			body, findings := filepath.Join(dir, "inline.md"), filepath.Join(dir, "findings.json")
			_ = os.WriteFile(body, []byte("new"), 0600)
			_ = os.WriteFile(findings, []byte(fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, body)), 0600)
			summary := filepath.Join(dir, "summary.md")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary, "--findings", findings)
			if got.Status != "fix_required" {
				t.Fatalf("inline mismatch accepted: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 1, 1, "")
		})
		t.Run("resume lacks original partial-handoff context", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "rework", "wip"}
			f.forge.timeline = []map[string]any{{"event": "labeled", "label": map[string]string{"name": "review"}}, {"event": "labeled", "label": map[string]string{"name": "rework"}}}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			got := f.run(t, f.root, "watchdog", "resume", "--item", "7")
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "original fixed-number") {
				t.Fatalf("partial handoff resumed without command context: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 0, 0, "")
		})
		t.Run("target-only later Claim", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"rework", "wip"}
			f.forge.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED"}}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if got := f.submit(t, 1, f.head, "rework"); got.Status != "fix_required" {
				t.Fatalf("target-only Claim released: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 1, 0, "")
		})
		t.Run("protected overlap without durable ordering", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "rework", "wip"}
			f.forge.summaries = []map[string]any{{"body": "round 1", "commit_id": f.head, "state": "CHANGES_REQUESTED"}}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if got := f.submit(t, 1, f.head, "rework"); got.Status != "fix_required" {
				t.Fatalf("unordered overlap completed: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 1, 0, "")
		})
		f := newReviewFixture(t)
		_ = os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600)
		f.start(t, f.root)
		for _, number := range []uint64{1, 4} {
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if got := f.submit(t, number, f.head, "rework"); got.Status != "fix_required" {
				t.Fatalf("accepted round %d: %#v", number, got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 0, 0, "")
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
		t.Run("checkpoint records reviewed head before final-head handoff", func(t *testing.T) {
			f := newReviewFixture(t)
			reviewed := f.head
			_ = os.WriteFile(f.checkpoint, []byte("1:"+reviewed+"\n"), 0600)
			f.start(t, f.root)
			runGit(t, f.worktree, "commit", "--allow-empty", "-m", "debt marker")
			final := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
			f.forge.head = final
			dir := t.TempDir()
			summary, body, inline, findings := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md"), filepath.Join(dir, "inline.md"), filepath.Join(dir, "findings.json")
			_ = os.WriteFile(summary, []byte("pass round 2"), 0600)
			_ = os.WriteFile(body, []byte("final"), 0600)
			_ = os.WriteFile(inline, []byte("anchored at H"), 0600)
			_ = os.WriteFile(findings, []byte(fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, inline)), 0600)
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "2", "--reviewed-head", reviewed, "--head", final, "--verdict", "pass", "--summary", summary, "--findings", findings, "--body", body}
			gitDir := filepath.Dir(f.checkpoint)
			_ = os.Chmod(gitDir, 0500)
			got := f.run(t, f.worktree, args...)
			_ = os.Chmod(gitDir, 0700)
			if got.Status != "fix_required" || checkpointSnapshot(f.checkpoint) != "1:"+reviewed+"\n" || strings.Contains(checkpointSnapshot(f.checkpoint), final) || len(f.forge.summaries) != 1 || f.forge.summaries[0]["commit_id"] != reviewed || len(f.forge.inlines) != 1 || f.forge.inlines[0]["commit_id"] != reviewed || f.forge.body != "final\n\nCloses #7\n" {
				t.Fatalf("failed replacement recorded final head: %#v checkpoint=%q", got, checkpointSnapshot(f.checkpoint))
			}
			if retry := f.run(t, f.worktree, args...); retry.Status != "ready_for_merge" || fileExists(f.checkpoint) || len(f.forge.summaries) != 1 {
				t.Fatalf("reviewed-head retry cleanup: %#v", retry)
			}
		})
		for _, tc := range []struct {
			name, verdict string
			arrange       func(*testing.T, *reviewFixture) string
			withFinal     bool
		}{
			{"local head disagreement", "pass", func(t *testing.T, f *reviewFixture) string {
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "local only")
				final := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
				f.forge.head = final
				runGit(t, f.worktree, "reset", "--hard", f.head)
				return final
			}, true},
			{"remote head disagreement", "pass", func(t *testing.T, f *reviewFixture) string {
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "final")
				final := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
				f.forge.pullHead, f.forge.remoteHead = final, f.head
				return final
			}, true},
			{"PR head disagreement", "pass", func(t *testing.T, f *reviewFixture) string {
				runGit(t, f.worktree, "commit", "--allow-empty", "-m", "final")
				final := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
				f.forge.pullHead, f.forge.remoteHead = f.head, final
				return final
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
		f.forge.summaries = []map[string]any{{"body": "visible review feedback", "commit_id": f.head, "state": "CHANGES_REQUESTED"}}
		got := f.run(t, f.worktree, "implement", "next")
		if got.Status != "work_available" || len(got.Packet.Facts.Implementation.Comments) == 0 || got.Packet.Facts.Implementation.Comments[0].Body != "visible review feedback" || strings.Contains(got.Packet.Instructions, "previous-reviewed-head") || strings.Contains(got.Packet.Instructions, "--reviewed-head") {
			t.Fatalf("rework packet: %#v", got)
		}
		resumed := f.run(t, f.root, "implement", "resume", "--item", "7")
		for _, instructions := range []string{got.Packet.Instructions, resumed.Packet.Instructions} {
			required := []string{"ordinary PR comparison", "user provides a fixed point", "Two-axis review", "Standards", "Artifacts", "full suite", "documented gate", "Artifact integrity", "complete final implementation"}
			missing := slices.DeleteFunc(required, func(text string) bool { return strings.Contains(instructions, text) })
			if strings.Contains(instructions, "previous-reviewed-head") || strings.Contains(instructions, "cache repair") || strings.Contains(instructions, "required previous-review") || !strings.Contains(instructions, "## Included Skill: audit") || len(missing) != 0 {
				t.Fatalf("implementation/Audit fallback missing %v: %q", missing, instructions)
			}
		}
	})

	t.Run("B13 nonterminal work retains checkpoint", func(t *testing.T) {
		assertRetained := func(t *testing.T, f *reviewFixture, prior string) {
			t.Helper()
			before := checkpointSnapshot(f.checkpoint)
			if before != "1:"+prior+"\n" {
				t.Fatal("nonterminal operation removed checkpoint")
			}
			for _, result := range []setup.ImplementationOutput{f.start(t, f.root), f.run(t, f.root, "watchdog", "resume", "--item", "7")} {
				facts := result.Packet.Facts.Watchdog
				if facts.ReviewCount != 1 || facts.ReviewNumber != 2 || facts.ReviewScope != "incremental" || facts.PreviousReviewedHead != prior || checkpointSnapshot(f.checkpoint) != before {
					t.Fatalf("retained review facts: %#v checkpoint=%q", facts, checkpointSnapshot(f.checkpoint))
				}
			}
		}
		t.Run("Needs Human pause and explicit requeue", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.submit(t, 1, f.head, "needs-human")
			f.forge.labels = []string{"review"}
			f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "review"}})
			assertRetained(t, f, f.head)
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
			assertRetained(t, f, f.head)
		})
		t.Run("Implementation Ledger retirement", func(t *testing.T) {
			f := newReviewFixture(t)
			prior := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD^"))
			runGit(t, f.worktree, "reset", "--hard", prior)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+prior+"\n"), 0600)
			runGit(t, f.worktree, "rm", "-r", ".changes/widget")
			runGit(t, f.worktree, "commit", "-m", "retire with retained checkpoint")
			f.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
			f.forge.head = f.head
			if fileExists(filepath.Join(f.worktree, ".changes", "widget")) {
				t.Fatal("fixture did not retire Implementation Ledger")
			}
			assertRetained(t, f, prior)
		})
		t.Run("ordinary git clean", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			_ = os.WriteFile(filepath.Join(f.worktree, "untracked"), []byte("x"), 0600)
			runGit(t, f.worktree, "clean", "-fd")
			assertRetained(t, f, f.head)
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
		f.forge.denyCleanup = resultDir
		got = f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--body", body)
		_ = os.Chmod(resultDir, 0700)
		if got.Status != "ready_for_merge" || fileExists(f.checkpoint) || !fileExists(resultDir) || !strings.Contains(got.Reason, "cleanup failed") || len(f.forge.summaries) != 1 {
			t.Fatalf("Result Document cleanup warning: %#v", got)
		}
		if retry := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--body", body); retry.Status != "ready_for_merge" || len(f.forge.summaries) != 1 || fileExists(resultDir) {
			t.Fatalf("cleanup warning duplicated review: %#v", retry)
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

		for _, checkpoint := range []string{"<absent>", "2"} {
			f = newReviewFixture(t)
			f.forge.labels = []string{"review", "rework", "wip"}
			f.forge.timeline = []map[string]any{{"event": "labeled", "label": map[string]string{"name": "review"}}, {"event": "labeled", "label": map[string]string{"name": "rework"}}}
			if checkpoint != "<absent>" {
				_ = os.WriteFile(f.checkpoint, []byte(checkpoint+":"+f.head+"\n"), 0600)
			}
			before = checkpointSnapshot(f.checkpoint)
			labels := append([]string(nil), f.forge.labels...)
			status := f.run(t, f.root, "status")
			if status.Status != "fix_required" || !strings.Contains(status.Reason, "original fixed-number") || checkpointSnapshot(f.checkpoint) != before || !slices.Equal(f.forge.labels, labels) {
				t.Fatalf("status invented completion with checkpoint %q: %#v", checkpoint, status)
			}
		}

		f = newReviewFixture(t)
		f.forge.labels = []string{"done"}
		f.forge.mergeable = false
		_ = os.WriteFile(f.checkpoint, []byte("3:"+f.head+"\n"), 0600)
		before = checkpointSnapshot(f.checkpoint)
		status := f.run(t, f.root, "status")
		if status.Status != "observed" || checkpointSnapshot(f.checkpoint) != before || !slices.Contains(f.forge.labels, "sync") || !slices.Contains(f.forge.labels, "rework") || slices.Contains(f.forge.labels, "wip") {
			t.Fatalf("status lost independent conflict diversion: %#v labels=%v", status, f.forge.labels)
		}
	})

	t.Run("B16 recreated worktree starts fresh", func(t *testing.T) {
		f := newReviewFixture(t)
		_ = os.WriteFile(f.checkpoint, []byte("2:"+f.head+"\n"), 0600)
		f.forge.timeline = []map[string]any{{"event": "labeled", "label": map[string]string{"name": "rework"}}}
		f.forge.summaries = []map[string]any{{"body": "old completed review", "commit_id": f.head, "state": "CHANGES_REQUESTED"}}
		f.forge.sourceComments = []map[string]any{{"body": "<!-- watchdog-checkpoint review-count=2 reviewed-head=" + f.head + " -->", "author_association": "OWNER"}}
		oldGitDir := filepath.Dir(f.checkpoint)
		runGit(t, f.root, "worktree", "remove", "--force", f.worktree)
		runGit(t, f.root, "worktree", "prune")
		if fileExists(oldGitDir) {
			t.Fatalf("old private Git directory retained: %s", oldGitDir)
		}
		runGit(t, f.root, "worktree", "add", f.worktree, "widget")
		newGitDir := strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "--absolute-git-dir"))
		f.checkpoint = filepath.Join(newGitDir, ".watchdog")
		f.forge.checkpointPath = f.checkpoint
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

func assertNoCheckpointTemps(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".watchdog-") {
			t.Fatalf("temporary checkpoint retained: %s", entry.Name())
		}
	}
}

func assertReviewUnchanged(t *testing.T, f *reviewFixture, checkpoint string, labels []string, summaries, inlines int, body string) {
	t.Helper()
	if checkpointSnapshot(f.checkpoint) != checkpoint || !slices.Equal(f.forge.labels, labels) || len(f.forge.summaries) != summaries || len(f.forge.inlines) != inlines || f.forge.body != body {
		t.Fatalf("review state mutated: checkpoint=%q labels=%v summaries=%d inlines=%d body=%q", checkpointSnapshot(f.checkpoint), f.forge.labels, len(f.forge.summaries), len(f.forge.inlines), f.forge.body)
	}
}
