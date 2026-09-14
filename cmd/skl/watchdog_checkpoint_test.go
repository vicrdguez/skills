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
	"github.com/vicrdguez/skills/workflow"
)

type reviewForge struct {
	mu                    sync.Mutex
	head                  string
	branch                string
	remoteHead            string
	pullHead              string
	body                  string
	draft                 bool
	noPull                bool
	noOther               bool
	labels                []string
	sourceLabels          []string
	summaries             []map[string]any
	issueComments         []map[string]any
	inlines               []map[string]any
	sourceComments        []map[string]any
	timeline              []map[string]any
	sourceTimeline        []map[string]any
	failSummary           bool
	failHandoff           bool
	failInline            bool
	failInlinePost        bool
	failBody              bool
	failBodyPost          bool
	failReadback          bool
	duplicateRead         bool
	reviewReads           int
	failPostRead          bool
	failDelete            string
	loseDelete            string
	failItemsRead         bool
	failFinalRead         bool
	failPullRead          bool
	failPullCreate        bool
	failSourceLabel       bool
	failSourceComment     bool
	failSourceCommentRead bool
	denyCleanup           string
	mergeable             bool
	checkpointPath        string
	atWipRelease          string
	denyRename            string
	renameDenied          bool
	afterMutation         func()
	clock                 int
	writes                int
	pullCreations         int
}

func (f *reviewForge) timestamp() string {
	f.clock++
	return time.Date(2026, 1, 1, 0, 0, f.clock, 0, time.UTC).Format(time.RFC3339Nano)
}

func (f *reviewForge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && f.afterMutation != nil {
		defer f.afterMutation()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodGet {
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
		}
		return result
	}
	pull := func() map[string]any {
		head := f.head
		if f.pullHead != "" {
			head = f.pullHead
		}
		result := issue(11, branch, f.labels, true)
		result["body"], result["draft"], result["merged"], result["mergeable"] = f.body, f.draft, false, f.mergeable
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
			issues = append(issues, issue(8, "other", []string{"ready"}, false))
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
		f.draft, _ = value["draft"].(bool)
		f.noPull = false
		f.pullCreations++
		if f.failPullCreate {
			f.failPullCreate = false
			http.Error(w, "creation response lost", http.StatusInternalServerError)
			return
		}
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
		if f.failSourceCommentRead {
			f.failSourceCommentRead = false
			http.Error(w, "comment readback unavailable", http.StatusInternalServerError)
			return
		}
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
		if value["event"] != "COMMENT" {
			http.Error(w, "pull request authors cannot approve or request changes on their own submissions", http.StatusUnprocessableEntity)
			return
		}
		value["state"] = "COMMENTED"
		value["submitted_at"] = f.timestamp()
		f.summaries = append(f.summaries, value)
		if f.failPostRead {
			f.failPostRead = false
			f.failReadback = true
		}
	case r.Method == http.MethodPost && path == "/issues/7/comments":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		value["author_association"] = "OWNER"
		value["created_at"] = f.timestamp()
		f.sourceComments = append(f.sourceComments, value)
		if f.failSourceComment {
			f.failSourceComment = false
			f.failSourceCommentRead = true
			http.Error(w, "decision response lost", http.StatusInternalServerError)
		}
	case r.Method == http.MethodPost && path == "/pulls/11/comments":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		if f.failInlinePost {
			f.failInlinePost = false
			http.Error(w, "inline unavailable", http.StatusInternalServerError)
			return
		}
		value["created_at"] = f.timestamp()
		f.inlines = append(f.inlines, value)
		if f.failInline {
			f.failInline = false
			http.Error(w, "inline response lost", http.StatusInternalServerError)
		}
	case r.Method == http.MethodPost && path == "/issues/11/comments":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
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
		if f.failBody {
			f.failBody = false
			http.Error(w, "body response lost", http.StatusInternalServerError)
		}
	case r.Method == http.MethodPost && path == "/graphql":
		var value map[string]any
		_ = json.NewDecoder(r.Body).Decode(&value)
		query, _ := value["query"].(string)
		f.draft = strings.Contains(query, "convertPullRequestToDraft")
	case r.Method == http.MethodGet && (path == "/issues/7" || path == "/issues/11"):
		if path == "/issues/7" {
			write(issue(7, branch, f.sourceLabels, false))
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
		if label == "wip" && f.denyCleanup != "" {
			_ = os.Chmod(f.denyCleanup, 0500)
		}
	default:
		http.Error(w, fmt.Sprintf("unexpected %s %s", r.Method, path), http.StatusNotFound)
	}
}

func TestImplementationHandoffProtectsDestinationThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.labels = []string{"rework", "wip"}
	start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
	if start.Status != "work_available" || start.Packet == nil || start.Packet.Facts.Implementation == nil {
		t.Fatalf("resume: %#v", start)
	}
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(body, []byte("reworked"), 0600); err != nil {
		t.Fatal(err)
	}

	active := false
	checks := 0
	claimedAfterRelease := false
	f.forge.afterMutation = func() {
		if active {
			return
		}
		active = true
		defer func() { active = false }()
		checks++
		unprotectedReview := slices.Contains(f.forge.labels, "review") && !slices.Contains(f.forge.labels, "wip")
		got := f.run(t, f.root, "watchdog", "next")
		if unprotectedReview {
			if got.Status != "work_available" {
				t.Fatalf("released review was not claimable: %#v labels=%v", got, f.forge.labels)
			}
			claimedAfterRelease = true
			return
		}
		if got.Status != "no_work" {
			t.Fatalf("destination became claimable before release: %#v labels=%v", got, f.forge.labels)
		}
		if slices.Contains(f.forge.labels, "review") && (f.forge.body != "reworked\n\nCloses #7\n" || f.forge.head != f.head) {
			t.Errorf("protected destination lacked exact evidence: body=%q head=%q want=%q", f.forge.body, f.forge.head, f.head)
		}
	}

	_, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if err == nil || checks == 0 || !claimedAfterRelease || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
		t.Fatalf("handoff did not preserve the later Watchdog Claim: err=%v checks=%d claimed=%t labels=%v", err, checks, claimedAfterRelease, f.forge.labels)
	}
}

func TestFirstImplementationHandoffProtectsDestinationThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.noPull = true
	f.forge.labels = nil
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
	start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(body, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	active, releaseObserved, claimed := false, false, false
	f.forge.afterMutation = func() {
		if active {
			return
		}
		active = true
		defer func() { active = false }()
		unprotectedReview := slices.Contains(f.forge.labels, "review") && !slices.Contains(f.forge.labels, "wip")
		got := f.run(t, f.root, "watchdog", "next")
		if unprotectedReview {
			if len(f.forge.sourceLabels) != 0 {
				t.Errorf("source cleanup incomplete at release: %v", f.forge.sourceLabels)
			}
			releaseObserved = true
			claimed = got.Status == "work_available" && got.Item != nil && got.Item.Number == 7
			if !claimed {
				t.Fatalf("released first Submission was not claimable: %#v source=%v destination=%v", got, f.forge.sourceLabels, f.forge.labels)
			}
		} else if got.Status != "no_work" {
			t.Fatalf("first Submission became claimable before release: %#v labels=%v", got, f.forge.labels)
		}
	}
	_, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if !releaseObserved || !claimed || err == nil || !strings.Contains(err.Error(), "label mutation not observed") || len(f.forge.sourceLabels) != 0 || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
		t.Fatalf("first handoff lost release ordering: err=%v released=%t source=%v destination=%v", err, releaseObserved, f.forge.sourceLabels, f.forge.labels)
	}
}

func TestImplementationPausePublishesBeforeReleaseThroughPublicHTTP(t *testing.T) {
	for _, mode := range []string{"issue-only", "new-draft", "rework-draft"} {
		t.Run(mode, func(t *testing.T) {
			f := newReviewFixture(t)
			if mode == "rework-draft" {
				f.forge.labels = []string{"rework", "wip"}
			} else {
				f.forge.noPull = true
				f.forge.sourceLabels = []string{"ready", "wip"}
				f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
			}
			start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
			directory := start.Packet.Facts.Implementation.ResultDirectory
			decision := filepath.Join(directory, "decision.md")
			if err := os.WriteFile(decision, []byte("pause\n"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision}
			if mode != "issue-only" {
				body := filepath.Join(directory, "submission.md")
				if err := os.WriteFile(body, []byte("draft"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--body", body)
			}
			active := false
			f.forge.afterMutation = func() {
				if active {
					return
				}
				active = true
				defer func() { active = false }()
				got := f.run(t, f.root, "watchdog", "next")
				if got.Status == "work_available" && got.Item != nil && got.Item.Number == 7 {
					t.Fatalf("paused destination became claimable: %#v", got)
				}
				paused := slices.Contains(f.forge.labels, "needs-human") && !slices.Contains(f.forge.labels, "wip")
				if paused && !slices.Equal(f.forge.sourceLabels, []string{"needs-human"}) {
					t.Errorf("pause released before source cleanup: source=%v destination=%v", f.forge.sourceLabels, f.forge.labels)
				}
			}
			got := f.run(t, f.worktree, args...)
			if got.Status != "needs_human" || got.Item.Claimed || !slices.Equal(f.forge.sourceLabels, []string{"needs-human"}) || mode != "issue-only" && (!slices.Equal(f.forge.labels, []string{"needs-human"}) || !f.forge.draft) {
				t.Fatalf("pause handoff incomplete: %#v source=%v destination=%v", got, f.forge.sourceLabels, f.forge.labels)
			}
		})
	}
}

func TestWatchdogVerdictsReleaseLastThroughPublicHTTP(t *testing.T) {
	for _, mode := range []string{"rework", "pass", "needs-human", "conflicting-pass", "review-limit"} {
		t.Run(mode, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther = true
			verdict, number, want := mode, "1", map[string]string{"rework": "rework", "pass": "ready_for_merge", "needs-human": "needs_human", "conflicting-pass": "rework", "review-limit": "needs_human"}[mode]
			if mode == "conflicting-pass" {
				verdict, f.forge.mergeable = "pass", false
			}
			if mode == "review-limit" {
				verdict, number = "rework", "2"
				if err := os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			start := f.start(t, f.root)
			directory := start.Packet.Facts.Watchdog.ResultDirectory
			summary := filepath.Join(directory, "summary.md")
			if err := os.WriteFile(summary, []byte("verdict\n"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", number, "--reviewed-head", f.head, "--verdict", verdict, "--summary", summary}
			if verdict == "pass" {
				body := filepath.Join(directory, "submission.md")
				if err := os.WriteFile(body, []byte("final\n"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--body", body)
			}
			active, released, claimedRework := false, false, false
			f.forge.afterMutation = func() {
				if active {
					return
				}
				active = true
				defer func() { active = false }()
				targetVisible := slices.Contains(f.forge.labels, "rework") || slices.Contains(f.forge.labels, "done") || slices.Contains(f.forge.labels, "needs-human")
				unprotected := targetVisible && !slices.Contains(f.forge.labels, "wip")
				got := f.run(t, f.root, "implement", "next")
				if unprotected {
					if len(f.forge.summaries) != 1 {
						t.Errorf("verdict released without published evidence: summaries=%d labels=%v", len(f.forge.summaries), f.forge.labels)
					}
					released = true
					claimedRework = got.Status == "work_available" && got.Item != nil && got.Item.Number == 7
					if want != "rework" && got.Status != "no_work" {
						t.Fatalf("implementation accepted released %s verdict: %#v labels=%v", mode, got, f.forge.labels)
					}
					return
				}
				if got.Status != "no_work" {
					t.Fatalf("implementation accepted verdict before release: %#v labels=%v", got, f.forge.labels)
				}
			}
			got, err := f.runResult(f.worktree, args...)
			reworkTarget := want == "rework"
			reworkLabels := slices.Contains(f.forge.labels, "rework") && slices.Contains(f.forge.labels, "wip")
			if !released || reworkTarget && (!claimedRework || err == nil || !reworkLabels) || !reworkTarget && (err != nil || got.Status != want) {
				t.Fatalf("verdict release ordering: mode=%s got=%#v err=%v released=%t claimed=%t labels=%v", mode, got, err, released, claimedRework, f.forge.labels)
			}
		})
	}
}

func TestImplementationRetryRefusesChangedPublishedEvidenceThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.labels = nil
	f.forge.body = "original\n\nCloses #7\n"
	f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
	directory := newImplementationResultDirectory(t)
	body := filepath.Join(directory, "submission.md")
	if err := os.WriteFile(body, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if got.Status != "fix_required" || !strings.Contains(got.Reason, "published Submission differs") || f.forge.body != "original\n\nCloses #7\n" || !slices.Equal(f.forge.sourceLabels, []string{"ready", "wip"}) {
		t.Fatalf("changed recovery evidence mutated handoff: %#v body=%q labels=%v", got, f.forge.body, f.forge.sourceLabels)
	}

	f = newReviewFixture(t)
	f.forge.labels = []string{"rework", "review", "wip"}
	f.forge.body = "accepted\n\nCloses #7\n"
	directory = newImplementationResultDirectory(t)
	body = filepath.Join(directory, "submission.md")
	if err := os.WriteFile(body, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got = f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if got.Status != "fix_required" || !strings.Contains(got.Reason, "contradictory lifecycle projections") || f.forge.body != "accepted\n\nCloses #7\n" || !slices.Equal(f.forge.labels, []string{"rework", "review", "wip"}) {
		t.Fatalf("changed Rework recovery evidence mutated handoff: %#v body=%q labels=%v", got, f.forge.body, f.forge.labels)
	}

	f = newReviewFixture(t)
	f.forge.noPull = true
	f.forge.labels = nil
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.sourceComments = []map[string]any{
		{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"},
		{"author_association": "OWNER", "body": workflow.OpaqueImplementationDecision("original\n")},
	}
	directory = newImplementationResultDirectory(t)
	decision := filepath.Join(directory, "decision.md")
	if err := os.WriteFile(decision, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got = f.run(t, f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision)
	if got.Status != "fix_required" || !strings.Contains(got.Reason, "published decision differs") || len(f.forge.sourceComments) != 2 || !slices.Equal(f.forge.sourceLabels, []string{"ready", "wip"}) {
		t.Fatalf("changed decision mutated handoff: %#v comments=%v labels=%v", got, f.forge.sourceComments, f.forge.sourceLabels)
	}
}

func TestImplementationCompletedHandoffVerifiesEmptyResultDocuments(t *testing.T) {
	for _, decisionVisible := range []bool{false, true} {
		t.Run(fmt.Sprintf("decision-visible=%t", decisionVisible), func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.sourceLabels = []string{"needs-human"}
			f.forge.labels = []string{"needs-human"}
			f.forge.body = "\n\nCloses #7\n"
			f.forge.draft = true
			if decisionVisible {
				f.forge.issueComments = []map[string]any{{"body": workflow.OpaqueImplementationDecision("")}}
			}
			directory := newImplementationResultDirectory(t)
			body, decision := filepath.Join(directory, "submission.md"), filepath.Join(directory, "decision.md")
			for _, name := range []string{body, decision} {
				if err := os.WriteFile(name, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			got := f.run(t, f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision, "--body", body)
			if decisionVisible && got.Status != "needs_human" || !decisionVisible && got.Status != "fix_required" {
				t.Fatalf("empty evidence verification: %#v", got)
			}
		})
	}
}

func TestImplementationContradictoryLifecycleRefusesWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		labels []string
		target string
	}{
		{"review-then-rework submit", []string{"review", "rework", "wip"}, "submit"},
		{"review-then-rework pause", []string{"review", "rework", "wip"}, "needs-human"},
		{"rework-then-done submit", []string{"rework", "done", "wip"}, "submit"},
		{"rework-then-done pause", []string{"rework", "done", "wip"}, "needs-human"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.labels = slices.Clone(tc.labels)
			f.forge.body = "accepted\n\nCloses #7\n"
			directory := newImplementationResultDirectory(t)
			body, decision := filepath.Join(directory, "submission.md"), filepath.Join(directory, "decision.md")
			if err := os.WriteFile(body, []byte("replacement\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(decision, []byte("hold\n"), 0600); err != nil {
				t.Fatal(err)
			}
			writes, sourceComments, issueComments := f.forge.writes, len(f.forge.sourceComments), len(f.forge.issueComments)
			args := []string{"implement", "submit", "--item", "7", "--body", body}
			if tc.target == "needs-human" {
				args = []string{"implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision, "--body", body}
			}
			got := f.run(t, f.worktree, args...)
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "contradictory lifecycle projections") {
				t.Fatalf("contradictory observation was not refused: %#v", got)
			}
			if f.forge.body != "accepted\n\nCloses #7\n" || f.forge.draft || !slices.Equal(f.forge.labels, tc.labels) || f.forge.writes != writes || len(f.forge.sourceComments) != sourceComments || len(f.forge.issueComments) != issueComments || !fileExists(body) || !fileExists(decision) {
				t.Fatalf("refusal mutated the ambiguous handoff: body=%q draft=%t labels=%v writes=%d", f.forge.body, f.forge.draft, f.forge.labels, f.forge.writes)
			}
			if !slices.Equal(f.forge.sourceLabels, []string(nil)) {
				t.Fatalf("refusal mutated source labels: %v", f.forge.sourceLabels)
			}
		})
	}
}

func TestReworkSubmitRefusesChangedBodyAfterAcceptedWriteThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.labels = []string{"rework", "wip"}
	directory := newImplementationResultDirectory(t)
	body := filepath.Join(directory, "submission.md")
	if err := os.WriteFile(body, []byte("first accepted body"), 0600); err != nil {
		t.Fatal(err)
	}
	f.forge.failBody, f.forge.failPullRead = true, true
	if _, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body); err == nil {
		t.Fatal("accepted body write with lost response and failed readback was not interrupted")
	}
	if f.forge.body != "first accepted body\n\nCloses #7\n" || !slices.Equal(f.forge.labels, []string{"rework", "wip"}) || f.forge.draft {
		t.Fatalf("interruption did not leave accepted evidence: body=%q labels=%v draft=%t", f.forge.body, f.forge.labels, f.forge.draft)
	}
	writes := f.forge.writes
	if err := os.WriteFile(body, []byte("changed retry body"), 0600); err != nil {
		t.Fatal(err)
	}
	got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if got.Status != "fix_required" || !strings.Contains(got.Reason, "published Rework Submission differs") {
		t.Fatalf("changed retry overwrote accepted evidence: %#v", got)
	}
	if f.forge.body != "first accepted body\n\nCloses #7\n" || !slices.Equal(f.forge.labels, []string{"rework", "wip"}) || f.forge.draft || f.forge.writes != writes || !fileExists(body) {
		t.Fatalf("changed retry mutated the handoff: body=%q labels=%v writes=%d", f.forge.body, f.forge.labels, f.forge.writes)
	}
}

func TestReworkSubmitRetriesExactAcceptedBodyThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.labels = []string{"rework", "wip"}
	directory := newImplementationResultDirectory(t)
	body := filepath.Join(directory, "submission.md")
	if err := os.WriteFile(body, []byte("first accepted body"), 0600); err != nil {
		t.Fatal(err)
	}
	f.forge.failBody, f.forge.failPullRead = true, true
	if _, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body); err == nil {
		t.Fatal("accepted body write with lost response and failed readback was not interrupted")
	}
	got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if got.Status != "awaiting_review" || f.forge.body != "first accepted body\n\nCloses #7\n" || !slices.Equal(f.forge.labels, []string{"review"}) {
		t.Fatalf("exact accepted-body retry did not finish the handoff: %#v body=%q labels=%v", got, f.forge.body, f.forge.labels)
	}
}

func TestReworkSubmitAcceptsProvenNewSourceStageUpdateThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	reviewed := f.head
	runGit(t, f.worktree, "commit", "--allow-empty", "-m", "new rework work")
	f.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
	f.forge.head = f.head
	f.forge.labels = []string{"rework", "wip"}
	f.forge.body = "previous round\n\nCloses #7\n"
	f.forge.summaries = []map[string]any{storedReviewSummary(1, "rework", "round 1", reviewed, "2026-01-01T00:00:02Z")}
	directory := newImplementationResultDirectory(t)
	body := filepath.Join(directory, "submission.md")
	if err := os.WriteFile(body, []byte("legitimate update"), 0600); err != nil {
		t.Fatal(err)
	}
	got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if got.Status != "awaiting_review" || f.forge.body != "legitimate update\n\nCloses #7\n" || !slices.Equal(f.forge.labels, []string{"review"}) {
		t.Fatalf("proven new Rework update was not published: %#v body=%q labels=%v", got, f.forge.body, f.forge.labels)
	}
}

func TestReworkSubmitRefusesStaleSameHeadCommandThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.labels = []string{"rework", "wip"}
	f.forge.body = "accepted\n\nCloses #7\n"
	f.forge.summaries = []map[string]any{storedReviewSummary(1, "rework", "round 1", f.head, "2026-01-01T00:00:02Z")}
	directory := newImplementationResultDirectory(t)
	body := filepath.Join(directory, "submission.md")
	if err := os.WriteFile(body, []byte("accepted"), 0600); err != nil {
		t.Fatal(err)
	}
	writes := f.forge.writes
	got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if got.Status != "fix_required" || !strings.Contains(got.Reason, "completed review at this head") {
		t.Fatalf("stale same-head command was accepted: %#v", got)
	}
	if f.forge.writes != writes || !slices.Equal(f.forge.labels, []string{"rework", "wip"}) || f.forge.body != "accepted\n\nCloses #7\n" || !fileExists(body) {
		t.Fatalf("stale refusal mutated the handoff: labels=%v writes=%d", f.forge.labels, f.forge.writes)
	}
}

func TestImplementationCompletedHandoffRequiresFinalSourceCleanup(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sourceLabels []string
		labels       []string
	}{
		{"stale source pause", []string{"needs-human"}, []string{"review"}},
		{"stale source pause and sync", []string{"needs-human"}, []string{"review", "sync"}},
		{"stale source ready", []string{"ready"}, []string{"review"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.sourceLabels = slices.Clone(tc.sourceLabels)
			f.forge.labels = slices.Clone(tc.labels)
			f.forge.body = "\n\nCloses #7\n"
			directory := newImplementationResultDirectory(t)
			body := filepath.Join(directory, "submission.md")
			if err := os.WriteFile(body, nil, 0600); err != nil {
				t.Fatal(err)
			}
			writes := f.forge.writes
			got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "cannot prove") {
				t.Fatalf("incomplete unclaimed destination was accepted: %#v", got)
			}
			if f.forge.writes != writes || f.forge.body != "\n\nCloses #7\n" || !slices.Equal(f.forge.labels, tc.labels) || !slices.Equal(f.forge.sourceLabels, tc.sourceLabels) || !fileExists(body) {
				t.Fatalf("refusal mutated incomplete handoff: labels=%v source=%v writes=%d", f.forge.labels, f.forge.sourceLabels, f.forge.writes)
			}
		})
	}
	t.Run("completed unclaimed destination", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.labels = []string{"review"}
		f.forge.body = "\n\nCloses #7\n"
		directory := newImplementationResultDirectory(t)
		body := filepath.Join(directory, "submission.md")
		if err := os.WriteFile(body, nil, 0600); err != nil {
			t.Fatal(err)
		}
		writes := f.forge.writes
		got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
		if got.Status != "awaiting_review" || f.forge.writes != writes || fileExists(body) {
			t.Fatalf("verified completed handoff was not a no-mutation success: %#v writes=%d", got, f.forge.writes)
		}
	})
}

func TestImplementationPausePublishesDistinctDecisionAfterRequeueThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.labels = []string{"rework", "wip"}
	firstDirectory := newImplementationResultDirectory(t)
	body, decision := filepath.Join(firstDirectory, "submission.md"), filepath.Join(firstDirectory, "decision.md")
	if err := os.WriteFile(body, []byte("draft"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decision, []byte("first decision\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first := f.run(t, f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision, "--body", body)
	if first.Status != "needs_human" || !f.forge.draft || !slices.Equal(f.forge.labels, []string{"needs-human"}) || len(f.forge.issueComments) != 1 {
		t.Fatalf("first pause: %#v labels=%v draft=%t", first, f.forge.labels, f.forge.draft)
	}
	// Explicit human requeue opens a new implementation stage.
	f.forge.labels = []string{"rework"}
	f.forge.sourceLabels = nil
	resumed := f.run(t, f.worktree, "implement", "next")
	if resumed.Status != "work_available" || resumed.Packet.Facts.Implementation.Branch != "widget" {
		t.Fatalf("requeued stage was not claimable: %#v", resumed)
	}
	runGit(t, f.worktree, "commit", "--allow-empty", "-m", "requeued work")
	f.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
	f.forge.head = f.head
	secondDirectory := newImplementationResultDirectory(t)
	secondBody, secondDecision := filepath.Join(secondDirectory, "submission.md"), filepath.Join(secondDirectory, "decision.md")
	if err := os.WriteFile(secondBody, []byte("draft"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondDecision, []byte("second decision\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second := f.run(t, f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", secondDecision, "--body", secondBody)
	if second.Status != "needs_human" || !f.forge.draft || !slices.Equal(f.forge.labels, []string{"needs-human"}) || len(f.forge.issueComments) != 2 {
		t.Fatalf("second pause: %#v labels=%v comments=%d", second, f.forge.labels, len(f.forge.issueComments))
	}
	if f.forge.issueComments[0]["body"] != workflow.OpaqueImplementationDecision("first decision\n") || f.forge.issueComments[1]["body"] != workflow.OpaqueImplementationDecision("second decision\n") {
		t.Fatalf("historical decision was replaced: %#v", f.forge.issueComments)
	}
}

func TestImplementationCleanupFailuresKeepDestinationProtectedThroughPublicHTTP(t *testing.T) {
	t.Run("first submit source cleanup", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.noPull = true
		f.forge.labels = nil
		f.forge.sourceLabels = []string{"ready", "wip"}
		f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
		start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
		body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
		if err := os.WriteFile(body, []byte("first"), 0600); err != nil {
			t.Fatal(err)
		}
		f.forge.failDelete = "ready"
		f.forge.afterMutation = func() {
			if slices.Contains(f.forge.labels, "review") && !slices.Contains(f.forge.labels, "wip") {
				t.Errorf("destination exposed without protection during source cleanup: labels=%v", f.forge.labels)
			}
		}
		if _, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body); err == nil {
			t.Fatal("source cleanup failure was not reported")
		}
		if f.forge.body != "first\n\nCloses #7\n" || !slices.Contains(f.forge.labels, "wip") || !slices.Contains(f.forge.labels, "review") || !fileExists(body) {
			t.Fatalf("evidence or protection lost: body=%q labels=%v", f.forge.body, f.forge.labels)
		}
		if got := f.run(t, f.root, "watchdog", "next"); got.Status != "no_work" {
			t.Fatalf("failed cleanup exposed the destination: %#v labels=%v", got, f.forge.labels)
		}
	})
	t.Run("rework submit label cleanup", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.labels = []string{"rework", "wip"}
		directory := newImplementationResultDirectory(t)
		body := filepath.Join(directory, "submission.md")
		if err := os.WriteFile(body, []byte("rework"), 0600); err != nil {
			t.Fatal(err)
		}
		f.forge.failDelete = "rework"
		if _, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body); err == nil {
			t.Fatal("Rework label cleanup failure was not reported")
		}
		if f.forge.body != "rework\n\nCloses #7\n" || !slices.Contains(f.forge.labels, "review") || !slices.Contains(f.forge.labels, "wip") || !fileExists(body) {
			t.Fatalf("Rework evidence or protection lost: body=%q labels=%v", f.forge.body, f.forge.labels)
		}
		if got := f.run(t, f.root, "watchdog", "next"); got.Status != "no_work" {
			t.Fatalf("failed Rework cleanup exposed the destination: %#v labels=%v", got, f.forge.labels)
		}
	})
	t.Run("sync label cleanup", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.labels = []string{"rework", "sync", "wip"}
		directory := newImplementationResultDirectory(t)
		body := filepath.Join(directory, "submission.md")
		if err := os.WriteFile(body, []byte("rework"), 0600); err != nil {
			t.Fatal(err)
		}
		f.forge.failDelete = "sync"
		if _, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body); err == nil {
			t.Fatal("sync label cleanup failure was not reported")
		}
		if f.forge.body != "rework\n\nCloses #7\n" || !slices.Contains(f.forge.labels, "review") || !slices.Contains(f.forge.labels, "wip") || !fileExists(body) {
			t.Fatalf("sync evidence or protection lost: body=%q labels=%v", f.forge.body, f.forge.labels)
		}
		if got := f.run(t, f.root, "watchdog", "next"); got.Status != "no_work" {
			t.Fatalf("failed sync cleanup exposed the destination: %#v labels=%v", got, f.forge.labels)
		}
	})
	t.Run("first submit wip cleanup", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.noPull = true
		f.forge.labels = nil
		f.forge.sourceLabels = []string{"ready", "wip"}
		f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
		start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
		body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
		if err := os.WriteFile(body, []byte("first"), 0600); err != nil {
			t.Fatal(err)
		}
		f.forge.failDelete = "wip"
		if _, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body); err == nil {
			t.Fatal("source wip cleanup failure was not reported")
		}
		if f.forge.body != "first\n\nCloses #7\n" || !slices.Contains(f.forge.labels, "review") || !slices.Contains(f.forge.labels, "wip") || !fileExists(body) {
			t.Fatalf("first submit evidence or protection lost: body=%q labels=%v", f.forge.body, f.forge.labels)
		}
		if got := f.run(t, f.root, "watchdog", "next"); got.Status != "no_work" {
			t.Fatalf("failed wip cleanup exposed the destination: %#v labels=%v", got, f.forge.labels)
		}
	})
	t.Run("pause label cleanup", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.labels = []string{"rework", "wip"}
		directory := newImplementationResultDirectory(t)
		body, decision := filepath.Join(directory, "submission.md"), filepath.Join(directory, "decision.md")
		if err := os.WriteFile(body, []byte("draft"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(decision, []byte("hold\n"), 0600); err != nil {
			t.Fatal(err)
		}
		f.forge.failDelete = "rework"
		if _, err := f.runResult(f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision, "--body", body); err == nil {
			t.Fatal("pause label cleanup failure was not reported")
		}
		if !slices.Contains(f.forge.labels, "needs-human") || !slices.Contains(f.forge.labels, "wip") || !f.forge.draft || !fileExists(body) || !fileExists(decision) {
			t.Fatalf("pause evidence or protection lost: labels=%v draft=%t", f.forge.labels, f.forge.draft)
		}
		if got := f.run(t, f.root, "watchdog", "next"); got.Status != "no_work" {
			t.Fatalf("failed pause cleanup exposed the destination: %#v labels=%v", got, f.forge.labels)
		}
	})
}

func TestFirstImplementationSubmitRecoversCreatedPullWithoutDuplicateThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.noPull = true
	f.forge.labels = nil
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
	start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(body, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	f.forge.failPullCreate, f.forge.failPullRead = true, true
	if _, err := f.runResult(f.worktree, "implement", "submit", "--item", "7", "--body", body); err == nil {
		t.Fatal("lost creation response was not interrupted")
	}
	if f.forge.pullCreations != 1 || f.forge.body != "first\n\nCloses #7\n" || len(f.forge.labels) != 0 {
		t.Fatalf("partial first publication state: creations=%d body=%q labels=%v", f.forge.pullCreations, f.forge.body, f.forge.labels)
	}
	got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
	if got.Status != "awaiting_review" || f.forge.pullCreations != 1 || f.forge.body != "first\n\nCloses #7\n" || !slices.Equal(f.forge.labels, []string{"review"}) {
		t.Fatalf("fresh retry duplicated or lost publication: %#v creations=%d body=%q labels=%v", got, f.forge.pullCreations, f.forge.body, f.forge.labels)
	}
}

func TestWatchdogReleaseRecoveryNeverReleasesLaterClaimThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.noOther = true
	f.start(t, f.root)
	f.forge.loseDelete, f.forge.failFinalRead = "wip", true
	directory := t.TempDir()
	summary := filepath.Join(directory, "summary.md")
	if err := os.WriteFile(summary, []byte("round 1"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary}
	if _, err := f.runResult(f.worktree, args...); err == nil {
		t.Fatal("lost release response was not interrupted")
	}
	if !slices.Contains(f.forge.labels, "rework") || slices.Contains(f.forge.labels, "wip") {
		t.Fatalf("lost release did not finish: %v", f.forge.labels)
	}
	claimed := f.run(t, f.root, "implement", "next")
	if claimed.Status != "work_available" || !slices.Contains(f.forge.labels, "wip") {
		t.Fatalf("later implementation Claim not acquired: %#v labels=%v", claimed, f.forge.labels)
	}
	labels, summaries, checkpoint := slices.Clone(f.forge.labels), len(f.forge.summaries), checkpointSnapshot(f.checkpoint)
	if got := f.run(t, f.root, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary); got.Status != "fix_required" {
		t.Fatalf("old command consumed the later Claim: %#v", got)
	}
	if !slices.Equal(f.forge.labels, labels) || len(f.forge.summaries) != summaries || checkpointSnapshot(f.checkpoint) != checkpoint {
		t.Fatalf("old command mutated the later Claim: labels=%v", f.forge.labels)
	}
	if resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7"); resumed.Status != "fix_required" {
		t.Fatalf("old lane resumed the later Claim: %#v", resumed)
	}
	if status := f.run(t, f.root, "status"); status.Status != "observed" {
		t.Fatalf("status did not observe the later Claim: %#v", status)
	}
	if !slices.Equal(f.forge.labels, labels) || checkpointSnapshot(f.checkpoint) != checkpoint {
		t.Fatalf("resume or status released the later Claim: labels=%v", f.forge.labels)
	}
}

func TestImplementationCleanupWarningsThroughPublicHTTP(t *testing.T) {
	t.Run("submit", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.noPull = true
		f.forge.labels = nil
		f.forge.sourceLabels = []string{"ready", "wip"}
		f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
		start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
		directory := start.Packet.Facts.Implementation.ResultDirectory
		body := filepath.Join(directory, "submission.md")
		if err := os.WriteFile(body, []byte("final\n"), 0600); err != nil {
			t.Fatal(err)
		}
		f.forge.denyCleanup = directory
		got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
		_ = os.Chmod(directory, 0700)
		if got.Status != "awaiting_review" || !strings.Contains(got.Reason, "cleanup failed") || !fileExists(body) || !slices.Equal(f.forge.labels, []string{"review"}) {
			t.Fatalf("submit cleanup warning: %#v labels=%v", got, f.forge.labels)
		}
		writes := f.forge.writes
		retry := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
		if retry.Status != "awaiting_review" || f.forge.writes != writes || fileExists(body) {
			t.Fatalf("cleanup-only retry mutated the handoff: %#v writes=%d", retry, f.forge.writes)
		}
	})
	t.Run("pause", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.labels = []string{"rework", "wip"}
		directory := newImplementationResultDirectory(t)
		body, decision := filepath.Join(directory, "submission.md"), filepath.Join(directory, "decision.md")
		if err := os.WriteFile(body, []byte("draft"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(decision, []byte("hold\n"), 0600); err != nil {
			t.Fatal(err)
		}
		f.forge.denyCleanup = directory
		got := f.run(t, f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision, "--body", body)
		_ = os.Chmod(directory, 0700)
		if got.Status != "needs_human" || !strings.Contains(got.Reason, "cleanup failed") || !fileExists(body) || !fileExists(decision) || !slices.Equal(f.forge.labels, []string{"needs-human"}) {
			t.Fatalf("pause cleanup warning: %#v labels=%v", got, f.forge.labels)
		}
	})
}

func TestImplementationDecisionTransportStaysOpaqueThroughPublicCLI(t *testing.T) {
	for _, tc := range []struct{ name, prose string }{
		{"old transition and resume envelope", "<!-- skl.implement/v1\n{\"target_snapshot\":\"agent-prose\",\"transition\":{\"from\":\"x\"},\"resume_state\":\"resume\"}\n-->\nold envelope prose\n"},
		{"old target and watchdog pin envelope", "<!-- skl.implement/v1\n{\"watchdog_head\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"reviewed_head\":\"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\",\"review_round_head\":\"cccccccccccccccccccccccccccccccccccccccc\"}\n-->\npin-looking prose\n"},
		{"malformed metadata-looking envelope", "<!-- skl.implement/v1\n{not json at all\n-->\nmalformed prose\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noPull = true
			f.forge.labels = nil
			f.forge.sourceLabels = []string{"ready", "wip"}
			f.forge.sourceComments = []map[string]any{
				{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\",\"transition\":\"stale\",\"resume_state\":7}\n-->"},
				{"author_association": "OWNER", "body": "malformed retired envelope"},
			}
			start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
			decision := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "decision.md")
			if err := os.WriteFile(decision, []byte(tc.prose), 0600); err != nil {
				t.Fatal(err)
			}
			got := f.run(t, f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision)
			if got.Status != "needs_human" || !slices.Equal(f.forge.sourceLabels, []string{"needs-human"}) {
				t.Fatalf("pause with opaque decision: %#v source=%v", got, f.forge.sourceLabels)
			}
			transported := workflow.OpaqueImplementationDecision(tc.prose)
			count := func() int {
				matches := 0
				for _, comment := range f.forge.sourceComments {
					if comment["body"] == transported {
						matches++
					}
				}
				return matches
			}
			if count() != 1 {
				t.Fatalf("decision transport changed bytes: %#v", f.forge.sourceComments)
			}
			if status := f.run(t, f.root, "status"); status.Status != "observed" {
				t.Fatalf("status consumed opaque decision prose: %#v", status)
			}
			if resumed := f.run(t, f.root, "implement", "resume", "--item", "7"); resumed.Status != "fix_required" {
				t.Fatalf("resume consumed opaque decision prose: %#v", resumed)
			}
			if count() != 1 {
				t.Fatalf("status or resume rewrote the opaque decision: %#v", f.forge.sourceComments)
			}
		})
	}
}

func TestImplementationReadyResumePreservesWorktreeThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
	uncommitted := filepath.Join(f.worktree, "uncommitted.txt")
	if err := os.WriteFile(uncommitted, []byte("work in progress"), 0600); err != nil {
		t.Fatal(err)
	}
	start := f.run(t, f.worktree, "implement", "resume")
	if start.Status != "work_available" || start.Packet.Facts.Implementation.Branch != "widget" || start.Packet.Facts.Implementation.WorkItem != 7 || !strings.Contains(start.Packet.Facts.Implementation.Worktree, "widget") {
		t.Fatalf("resume packet: %#v", start.Packet.Facts.Implementation)
	}
	if readFile(t, uncommitted) != "work in progress" || !slices.Equal(f.forge.sourceLabels, []string{"ready", "wip"}) {
		t.Fatalf("resume did not preserve the selected Work Item: source=%v", f.forge.sourceLabels)
	}
	explicit := f.run(t, f.root, "implement", "resume", "--item", "7")
	if explicit.Status != "work_available" || explicit.Packet.Facts.Implementation.Branch != "widget" {
		t.Fatalf("explicit resume selected another Work Item: %#v", explicit)
	}
}

func TestImplementationIssueOnlyPausePublishesNewDecisionAfterRequeueThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.noPull = true
	f.forge.labels = nil
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
	start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
	first := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "decision.md")
	if err := os.WriteFile(first, []byte("first decision\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := f.run(t, f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", first); got.Status != "needs_human" || !slices.Equal(f.forge.sourceLabels, []string{"needs-human"}) {
		t.Fatalf("first issue-only pause: %#v source=%v", got, f.forge.sourceLabels)
	}
	// Explicit human requeue opens a new Ready stage.
	f.forge.sourceLabels = []string{"ready"}
	if resumed := f.run(t, f.worktree, "implement", "next"); resumed.Status != "work_available" || resumed.Packet.Facts.Implementation.WorkItem != 7 {
		t.Fatalf("requeued issue-only stage was not claimable: %#v", resumed)
	}
	runGit(t, f.worktree, "commit", "--allow-empty", "-m", "requeued issue-only work")
	f.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
	f.forge.head = f.head
	secondDirectory := newImplementationResultDirectory(t)
	second := filepath.Join(secondDirectory, "decision.md")
	if err := os.WriteFile(second, []byte("second decision\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := f.run(t, f.worktree, "implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", second)
	if got.Status != "needs_human" || !slices.Equal(f.forge.sourceLabels, []string{"needs-human"}) {
		t.Fatalf("second issue-only pause: %#v source=%v", got, f.forge.sourceLabels)
	}
	transported := []string{workflow.OpaqueImplementationDecision("first decision\n"), workflow.OpaqueImplementationDecision("second decision\n")}
	for _, wanted := range transported {
		matches := 0
		for _, comment := range f.forge.sourceComments {
			if comment["body"] == wanted {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("issue-only decision history lost or duplicated: %#v", f.forge.sourceComments)
		}
	}
}

func TestWatchdogCleanupFailuresKeepDestinationsProtectedThroughPublicHTTP(t *testing.T) {
	for _, verdict := range []string{"rework", "pass", "needs-human"} {
		t.Run(verdict, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther = true
			f.start(t, f.root)
			f.forge.failDelete = "review"
			directory := t.TempDir()
			summary := filepath.Join(directory, "summary.md")
			if err := os.WriteFile(summary, []byte("verdict"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", verdict, "--summary", summary}
			if verdict == "pass" {
				body := filepath.Join(directory, "submission.md")
				if err := os.WriteFile(body, []byte("final"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--body", body)
			}
			active := false
			f.forge.afterMutation = func() {
				if active {
					return
				}
				active = true
				defer func() { active = false }()
				if got := f.run(t, f.root, "implement", "next"); got.Status != "no_work" {
					t.Errorf("implementation accepted protected %s destination: %#v labels=%v", verdict, got, f.forge.labels)
				}
			}
			if _, err := f.runResult(f.worktree, args...); err == nil {
				t.Fatal("cleanup failure was not reported")
			}
			if len(f.forge.summaries) != 1 || !slices.Contains(f.forge.labels, "wip") || !fileExists(f.checkpoint) {
				t.Fatalf("verdict evidence, protection, or checkpoint lost: summaries=%d labels=%v", len(f.forge.summaries), f.forge.labels)
			}
			f.forge.afterMutation = nil
			wantLabel := map[string]string{"rework": "rework", "pass": "done", "needs-human": "needs-human"}[verdict]
			if got := f.run(t, f.worktree, args...); got.Status != map[string]string{"rework": "rework", "pass": "ready_for_merge", "needs-human": "needs_human"}[verdict] || !slices.Equal(f.forge.labels, []string{wantLabel}) {
				t.Fatalf("cleanup-only retry did not finish the verdict: %#v labels=%v", got, f.forge.labels)
			}
		})
	}
	t.Run("source needs human cleanup", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.noOther = true
		f.forge.sourceLabels = []string{"needs-human"}
		f.start(t, f.root)
		f.forge.failDelete = "needs-human"
		directory := t.TempDir()
		summary := filepath.Join(directory, "summary.md")
		if err := os.WriteFile(summary, []byte("verdict"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := f.runResult(f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary); err == nil {
			t.Fatal("source Needs Human cleanup failure was not reported")
		}
		if len(f.forge.summaries) != 1 || !slices.Contains(f.forge.labels, "wip") {
			t.Fatalf("source cleanup evidence or protection lost: labels=%v", f.forge.labels)
		}
		if got := f.run(t, f.root, "implement", "next"); got.Status != "no_work" {
			t.Fatalf("failed source cleanup exposed the destination: %#v labels=%v", got, f.forge.labels)
		}
	})
}

func TestImplementationPauseRetryRepublishesNothingThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.noPull = true
	f.forge.labels = nil
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
	start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
	directory := start.Packet.Facts.Implementation.ResultDirectory
	decision := filepath.Join(directory, "decision.md")
	if err := os.WriteFile(decision, []byte("hold\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.forge.failSourceLabel = true
	args := []string{"implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision}
	if _, err := f.runResult(f.worktree, args...); err == nil {
		t.Fatal("unapplied pause projection was not reported")
	}
	if len(f.forge.sourceComments) != 2 || !slices.Equal(f.forge.sourceLabels, []string{"ready", "wip"}) {
		t.Fatalf("partial pause state: comments=%d source=%v", len(f.forge.sourceComments), f.forge.sourceLabels)
	}
	got := f.run(t, f.worktree, args...)
	if got.Status != "needs_human" || len(f.forge.sourceComments) != 2 || !slices.Equal(f.forge.sourceLabels, []string{"needs-human"}) {
		t.Fatalf("fresh pause retry duplicated or lost evidence: %#v comments=%d source=%v", got, len(f.forge.sourceComments), f.forge.sourceLabels)
	}
}

func TestImplementationPauseRetryReusesAcceptedDecisionThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.noPull = true
	f.forge.labels = nil
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
	start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
	decision := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "decision.md")
	if err := os.WriteFile(decision, []byte("hold\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.forge.failSourceComment = true
	args := []string{"implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision}
	if _, err := f.runResult(f.worktree, args...); err == nil {
		t.Fatal("accepted decision with lost response and failed readback was not interrupted")
	}
	if len(f.forge.sourceComments) != 2 || !slices.Equal(f.forge.sourceLabels, []string{"ready", "wip"}) {
		t.Fatalf("partial pause state: comments=%d source=%v", len(f.forge.sourceComments), f.forge.sourceLabels)
	}
	got := f.run(t, f.worktree, args...)
	if got.Status != "needs_human" || len(f.forge.sourceComments) != 2 || !slices.Equal(f.forge.sourceLabels, []string{"needs-human"}) {
		t.Fatalf("fresh pause retry duplicated or lost accepted evidence: %#v comments=%d source=%v", got, len(f.forge.sourceComments), f.forge.sourceLabels)
	}
}

func TestImplementationReleaseRecoveryNeverReleasesLaterWatchdogClaimThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.noOther = true
	f.forge.noPull = true
	f.forge.labels = nil
	f.forge.sourceLabels = []string{"ready", "wip"}
	f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
	start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
	body := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(body, []byte("final"), 0600); err != nil {
		t.Fatal(err)
	}
	f.forge.loseDelete, f.forge.failFinalRead = "wip", true
	args := []string{"implement", "submit", "--item", "7", "--body", body}
	if got, err := f.runResult(f.worktree, args...); err == nil {
		t.Fatalf("lost release response was not interrupted: %#v", got)
	}
	if !slices.Equal(f.forge.labels, []string{"review"}) {
		t.Fatalf("released review state: %v", f.forge.labels)
	}
	claimed := f.run(t, f.root, "watchdog", "next")
	if claimed.Status != "work_available" || claimed.Packet == nil {
		t.Fatalf("later Watchdog Claim not acquired: %#v", claimed)
	}
	labels := slices.Clone(f.forge.labels)
	if got := f.run(t, f.worktree, args...); got.Status != "fix_required" {
		t.Fatalf("old implementation command consumed the later Watchdog Claim: %#v", got)
	}
	if !slices.Equal(f.forge.labels, labels) {
		t.Fatalf("old implementation command released the later Claim: %v", f.forge.labels)
	}
	if resumed := f.run(t, f.root, "implement", "resume", "--item", "7"); resumed.Status != "fix_required" {
		t.Fatalf("implementation resume consumed the later Claim: %#v", resumed)
	}
	if status := f.run(t, f.root, "status"); status.Status != "observed" {
		t.Fatalf("status did not observe the later Claim: %#v", status)
	}
	if !slices.Equal(f.forge.labels, labels) {
		t.Fatalf("resume or status released the later Claim: %v", f.forge.labels)
	}
}

func TestImplementationReworkResumePreservesWorktreeThroughPublicHTTP(t *testing.T) {
	f := newReviewFixture(t)
	f.forge.labels = []string{"rework", "wip"}
	f.forge.body = "previous\n\nCloses #7\n"
	uncommitted := filepath.Join(f.worktree, "uncommitted.txt")
	if err := os.WriteFile(uncommitted, []byte("rework in progress"), 0600); err != nil {
		t.Fatal(err)
	}
	start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
	if start.Status != "work_available" || start.Packet.Facts.Implementation.Branch != "widget" || start.Packet.Facts.Implementation.WorkItem != 7 {
		t.Fatalf("Rework resume packet: %#v", start.Packet.Facts.Implementation)
	}
	if readFile(t, uncommitted) != "rework in progress" || !slices.Equal(f.forge.labels, []string{"rework", "wip"}) {
		t.Fatalf("Rework resume did not preserve files or Claim: %v", f.forge.labels)
	}
}

func TestWatchdogCleanupWarningsThroughPublicHTTP(t *testing.T) {
	for _, verdict := range []string{"rework", "needs-human"} {
		t.Run(verdict, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther = true
			start := f.start(t, f.root)
			directory := start.Packet.Facts.Watchdog.ResultDirectory
			summary := filepath.Join(directory, "summary.md")
			if err := os.WriteFile(summary, []byte(verdict), 0600); err != nil {
				t.Fatal(err)
			}
			f.forge.denyCleanup = directory
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", verdict, "--summary", summary}
			got := f.run(t, f.worktree, args...)
			_ = os.Chmod(directory, 0700)
			if got.Status != map[string]string{"rework": "rework", "needs-human": "needs_human"}[verdict] || !strings.Contains(got.Reason, "cleanup failed") || !fileExists(summary) {
				t.Fatalf("watchdog cleanup warning: %#v", got)
			}
			writes := f.forge.writes
			retry := f.run(t, f.worktree, args...)
			if retry.Status != got.Status || f.forge.writes != writes || fileExists(summary) || len(f.forge.summaries) != 1 {
				t.Fatalf("cleanup-only retry mutated the verdict: %#v writes=%d", retry, f.forge.writes)
			}
		})
	}
	t.Run("unexpected files", func(t *testing.T) {
		f := newReviewFixture(t)
		f.forge.noOther = true
		start := f.start(t, f.root)
		directory := start.Packet.Facts.Watchdog.ResultDirectory
		summary := filepath.Join(directory, "summary.md")
		unexpected := filepath.Join(directory, "keep.txt")
		if err := os.WriteFile(summary, []byte("rework"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(unexpected, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary)
		if got.Status != "rework" || !strings.Contains(got.Reason, "unexpected files") || readFile(t, unexpected) != "keep" || !fileExists(summary) {
			t.Fatalf("unexpected-file cleanup warning: %#v", got)
		}
	})
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
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(f.server.URL, "token", f.server.Client())
		backend.BindRepository(repository)
		return backend, nil
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
		t.Run("MaxUint64 retained count refuses overflow", func(t *testing.T) {
			for _, command := range []string{"next", "resume"} {
				t.Run(command, func(t *testing.T) {
					f := newReviewFixture(t)
					checkpointBytes := []byte(strconv.FormatUint(^uint64(0), 10) + ":" + f.head + "\n")
					if err := os.WriteFile(f.checkpoint, checkpointBytes, 0600); err != nil {
						t.Fatal(err)
					}
					args := []string{"watchdog", command}
					if command == "resume" {
						f.forge.labels = []string{"review", "wip"}
						args = append(args, "--item", "7")
					}
					labels := append([]string(nil), f.forge.labels...)
					got := f.run(t, f.root, args...)
					if got.Status != "fix_required" || !strings.Contains(got.Reason, "cannot be incremented") || !bytes.Equal([]byte(readFile(t, f.checkpoint)), checkpointBytes) || !slices.Equal(f.forge.labels, labels) {
						t.Fatalf("overflow changed checkpoint or Claim through %s: %#v checkpoint=%q labels=%v", command, got, readFile(t, f.checkpoint), f.forge.labels)
					}
				})
			}
		})
	})

	t.Run("B3 resume refreshes PR head", func(t *testing.T) {
		f := newReviewFixture(t)
		prior := strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "main"))
		_ = os.WriteFile(f.checkpoint, []byte("1:"+prior+"\n"), 0600)
		stale := "<!-- skl.implement/v1\n" + `{"watchdog_head":"` + strings.Repeat("a", 40) + `","reviewed_head":"` + strings.Repeat("b", 40) + `","review_round_head":"` + strings.Repeat("c", 40) + `"}` + "\n-->"
		f.forge.sourceComments = append(f.forge.sourceComments, map[string]any{
			"body":               stale,
			"author_association": "OWNER",
		})
		before := checkpointSnapshot(f.checkpoint)
		started := f.start(t, f.root)
		first := started.Packet.Facts.Watchdog
		if first.ReviewedHead != f.head || first.ReviewCount != 1 || first.ReviewNumber != 2 || first.PreviousReviewedHead != prior || started.Packet.Facts.Watchdog.WorkItem != 7 || checkpointSnapshot(f.checkpoint) != before || len(first.Comments) != 0 {
			t.Fatalf("stale forge metadata affected local checkpoint: %#v", first)
		}
		runGit(t, f.worktree, "commit", "--allow-empty", "-m", "move")
		f.forge.head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD"))
		resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7").Packet.Facts.Watchdog
		if resumed.ReviewedHead != f.forge.head || resumed.ReviewNumber != first.ReviewNumber || resumed.ReviewCount != 1 || !strings.Contains(resumed.SubmitCommand, "--reviewed-head "+f.forge.head) || checkpointSnapshot(f.checkpoint) != before {
			t.Fatalf("resume facts = %#v", resumed)
		}
		if first.ReviewedHead == f.forge.head || len(f.forge.sourceComments) != 1 || f.forge.sourceComments[0]["body"] != stale || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
			t.Fatal("resume mutated prior packet, source metadata, or unrelated Workflow State")
		}
	})

	t.Run("B4 invalid checkpoint and submit inputs refuse", func(t *testing.T) {
		for _, invalid := range []string{"", "\n", "1:", " 1:" + strings.Repeat("a", 40) + "\n", "x:" + strings.Repeat("a", 40), "-1:" + strings.Repeat("a", 40), "+1:" + strings.Repeat("a", 40), "18446744073709551616:" + strings.Repeat("a", 40), "1:abc", "1:" + strings.Repeat("a", 39), "1:" + strings.Repeat("g", 40), "1:" + strings.Repeat("a", 64), "1:" + strings.Repeat("a", 40) + ":x", "1:" + strings.Repeat("a", 40) + "\nextra"} {
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
			f.start(t, f.root)
			_ = os.WriteFile(f.checkpoint, []byte("corrupt\n"), 0600)
			got := f.submit(t, 1, f.head, "rework")
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "corrupt Review Checkpoint") || readFile(t, f.checkpoint) != "corrupt\n" || len(f.forge.summaries) != 0 || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
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
			summary, body, findings := filepath.Join(dir, "summary.md"), filepath.Join(dir, "inline.md"), filepath.Join(dir, "findings.json")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			_ = os.WriteFile(body, []byte("finding"), 0600)
			_ = os.WriteFile(findings, []byte(fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, body)), 0600)
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary, "--findings", findings}
			_, err := f.runResult(f.worktree, args...)
			if err == nil || fileExists(f.checkpoint) || len(f.forge.summaries) != 1 || len(f.forge.inlines) != 0 {
				t.Fatalf("readback failure ordering: %v", err)
			}
			if got := f.run(t, f.worktree, args...); got.Status != "rework" || len(f.forge.summaries) != 1 || len(f.forge.inlines) != 1 {
				t.Fatalf("readback repair did not finish missing inline: %#v", got)
			}
		})
		t.Run("inline write unapplied", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.forge.failInlinePost = true
			dir := t.TempDir()
			summary, body, findings := filepath.Join(dir, "summary.md"), filepath.Join(dir, "inline.md"), filepath.Join(dir, "findings.json")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			_ = os.WriteFile(body, []byte("finding"), 0600)
			_ = os.WriteFile(findings, []byte(fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, body)), 0600)
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary, "--findings", findings}
			if _, err := f.runResult(f.worktree, args...); err == nil || fileExists(f.checkpoint) || len(f.forge.summaries) != 1 || len(f.forge.inlines) != 0 {
				t.Fatalf("unapplied inline advanced review: %v", err)
			}
			if got := f.run(t, f.worktree, args...); got.Status != "rework" || len(f.forge.summaries) != 1 || len(f.forge.inlines) != 1 {
				t.Fatalf("unapplied inline retry: %#v", got)
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
		t.Run("body write unapplied", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.forge.failBodyPost = true
			dir := t.TempDir()
			summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md")
			_ = os.WriteFile(summary, []byte("pass"), 0600)
			_ = os.WriteFile(body, []byte("final"), 0600)
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--body", body}
			if _, err := f.runResult(f.worktree, args...); err == nil || len(f.forge.summaries) != 1 || f.forge.body != "" || fileExists(f.checkpoint) || !slices.Contains(f.forge.labels, "wip") {
				t.Fatalf("unapplied body did not retain retry evidence: %v summaries=%#v", err, f.forge.summaries)
			}
			if resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7"); resumed.Status != "fix_required" || !strings.Contains(resumed.Reason, "original fixed-number") {
				t.Fatalf("resume lost partial body publication: %#v", resumed)
			}
			if got := f.run(t, f.worktree, args...); got.Status != "ready_for_merge" || len(f.forge.summaries) != 1 || f.forge.body != "final\n\nCloses #7\n" {
				t.Fatalf("unapplied body retry: %#v body=%q", got, f.forge.body)
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
			if len(f.forge.summaries) != 1 || reviewSummaryText(t, f.forge.summaries[0]) != string(summaryBytes) || f.forge.summaries[0]["commit_id"] != f.head || f.forge.summaries[0]["state"] != "COMMENTED" || f.forge.summaries[0]["event"] != "COMMENT" {
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
			f.forge.denyRename = gitDir
			t.Cleanup(func() { _ = os.Chmod(gitDir, 0700) })
			got := f.submit(t, 2, f.head, "rework")
			_ = os.Chmod(gitDir, 0700)
			if !f.forge.renameDenied || got.Status != "fix_required" || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head || !slices.Contains(f.forge.labels, "wip") {
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
					if tc.name == "after checkpoint before destination publication" {
						resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7")
						if resumed.Status != "fix_required" || !strings.Contains(resumed.Reason, "original fixed-number") {
							t.Fatalf("resume authorized another round: %#v", resumed)
						}
						checkpoint, labels, summaries := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...), len(f.forge.summaries)
						if next := f.submit(t, round+1, f.head, "rework"); next.Status != "fix_required" {
							t.Fatalf("direct submit authorized another round: %#v", next)
						}
						assertReviewUnchanged(t, f, checkpoint, labels, summaries, 0, "")
					}
					wantStatus, wantLabel := "rework", "rework"
					if round == 2 {
						wantStatus, wantLabel = "needs_human", "needs-human"
					}
					if retry := f.run(t, f.worktree, args...); retry.Status != wantStatus || len(f.forge.summaries) != int(round) || reviewSummaryText(t, f.forge.summaries[round-1]) != body || !slices.Equal(f.forge.labels, []string{wantLabel}) {
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
		t.Run("fresh round gives identical summary a distinct receipt", func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.submit(t, 1, f.head, "needs-human")
			f.forge.labels = []string{"review"}
			f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "review"}})
			if facts := f.start(t, f.root).Packet.Facts.Watchdog; facts.ReviewNumber != 2 {
				t.Fatalf("fresh review number = %d", facts.ReviewNumber)
			}
			dir := t.TempDir()
			summary := filepath.Join(dir, "summary.md")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "2", "--reviewed-head", f.head, "--verdict", "needs-human", "--summary", summary)
			if got.Status != "needs_human" || len(f.forge.summaries) != 2 || reviewSummaryText(t, f.forge.summaries[0]) != "round 1" || reviewSummaryText(t, f.forge.summaries[1]) != "round 1" || f.forge.summaries[0]["body"] == f.forge.summaries[1]["body"] {
				t.Fatalf("fresh round did not publish distinct numbered evidence: %#v summaries=%#v", got, f.forge.summaries)
			}
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
		t.Run("duplicate inline receipt stops newly published round", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.start(t, f.root)
			dir := t.TempDir()
			summary, body, findings := filepath.Join(dir, "summary.md"), filepath.Join(dir, "inline.md"), filepath.Join(dir, "findings.json")
			_ = os.WriteFile(summary, []byte("round 2"), 0600)
			_ = os.WriteFile(body, []byte("duplicate finding"), 0600)
			_ = os.WriteFile(findings, []byte(fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, body)), 0600)
			f.forge.inlines = []map[string]any{{"body": "duplicate finding", "commit_id": f.head, "path": "README.md", "line": float64(1), "side": "RIGHT", "created_at": "2026-01-01T00:00:03Z"}, {"body": "duplicate finding", "commit_id": f.head, "path": "README.md", "line": float64(1), "side": "RIGHT", "created_at": "2026-01-01T00:00:04Z"}}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "2", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary, "--findings", findings)
			if got.Status != "fix_required" || checkpointSnapshot(f.checkpoint) != checkpoint || !slices.Equal(f.forge.labels, labels) || len(f.forge.summaries) != 1 || reviewSummaryText(t, f.forge.summaries[0]) != "round 2" || len(f.forge.inlines) != 2 {
				t.Fatalf("duplicate inline advanced round: %#v checkpoint=%q labels=%v summaries=%#v", got, checkpointSnapshot(f.checkpoint), f.forge.labels, f.forge.summaries)
			}
		})
		t.Run("checkpoint SHA differs from command", func(t *testing.T) {
			f := newReviewFixture(t)
			prior := strings.TrimSpace(runGitOutput(t, f.root, "rev-parse", "main"))
			_ = os.WriteFile(f.checkpoint, []byte("1:"+prior+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			got := f.submit(t, 1, f.head, "rework")
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "different reviewed head") {
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
			f.forge.timeline = claimedReviewTimeline()
			f.forge.summaries = []map[string]any{storedReviewSummary(1, "pass", "round 1", f.head, "2026-01-01T00:00:03Z")}
			f.forge.body = "different\n\nCloses #7\n"
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			if got := f.submit(t, 1, f.head, "pass"); got.Status != "fix_required" || !strings.Contains(got.Reason, "recorded review differs") {
				t.Fatalf("final body mismatch accepted: %#v", got)
			}
			assertReviewUnchanged(t, f, checkpoint, labels, 1, 0, "different\n\nCloses #7\n")
		})
		t.Run("inline evidence mismatch", func(t *testing.T) {
			f := newReviewFixture(t)
			_ = os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600)
			f.forge.labels = []string{"review", "wip"}
			f.forge.timeline = claimedReviewTimeline()
			f.forge.summaries = []map[string]any{storedReviewSummary(1, "rework", "round 1", f.head, "2026-01-01T00:00:03Z")}
			f.forge.inlines = []map[string]any{{"body": "old", "commit_id": f.head, "path": "README.md", "line": float64(1), "side": "RIGHT", "created_at": "2026-01-01T00:00:04Z"}}
			dir := t.TempDir()
			body, findings := filepath.Join(dir, "inline.md"), filepath.Join(dir, "findings.json")
			_ = os.WriteFile(body, []byte("new"), 0600)
			_ = os.WriteFile(findings, []byte(fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, body)), 0600)
			summary := filepath.Join(dir, "summary.md")
			_ = os.WriteFile(summary, []byte("round 1"), 0600)
			checkpoint, labels := checkpointSnapshot(f.checkpoint), append([]string(nil), f.forge.labels...)
			got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary, "--findings", findings)
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "recorded review differs") {
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
			if got.Status != "fix_required" || !strings.Contains(got.Reason, "unambiguous Awaiting Review Claim") {
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
			got := f.run(t, f.worktree, "watchdog", "submit", "--item", "7", "--review-number", "2", "--reviewed-head", reviewed, "--head", final, "--verdict", "pass", "--summary", summary, "--findings", findings, "--body", body)
			if got.Status != "ready_for_merge" || fileExists(f.checkpoint) || f.forge.atWipRelease != "2:"+reviewed+"\n" || strings.Contains(f.forge.atWipRelease, final) || len(f.forge.summaries) != 1 || f.forge.summaries[0]["commit_id"] != reviewed || len(f.forge.inlines) != 1 || f.forge.inlines[0]["commit_id"] != reviewed || f.forge.body != "final\n\nCloses #7\n" {
				t.Fatalf("completed checkpoint did not record reviewed head: %#v checkpoint at release=%q", got, f.forge.atWipRelease)
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
			if status.Status != "fix_required" || !strings.Contains(status.Reason, "ambiguous claimed lifecycle") || checkpointSnapshot(f.checkpoint) != before || !slices.Equal(f.forge.labels, labels) {
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
		f.forge.summaries = []map[string]any{storedReviewSummary(1, "rework", "round 1", f.head, "2026-01-01T00:00:01Z")}
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
		dir := t.TempDir()
		summary := filepath.Join(dir, "summary.md")
		_ = os.WriteFile(summary, []byte("round 1"), 0600)
		args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary}
		f.forge.failDelete = "review"
		if _, err := f.runResult(f.worktree, args...); err == nil || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head || len(f.forge.summaries) != 2 || len(f.forge.labels) != 3 || !slices.Contains(f.forge.labels, "review") || !slices.Contains(f.forge.labels, "rework") || !slices.Contains(f.forge.labels, "wip") {
			t.Fatalf("fresh retained-worktree budget did not record interrupted round 1: %v", err)
		}
		if got := f.run(t, f.worktree, args...); got.Status != "rework" || strings.TrimSpace(readFile(t, f.checkpoint)) != "1:"+f.head || len(f.forge.summaries) != 2 {
			t.Fatalf("fresh retained-worktree retry did not complete round 1: %#v", got)
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

func reviewSummaryText(t *testing.T, summary map[string]any) string {
	t.Helper()
	_, body, ok := strings.Cut(summary["body"].(string), "\n-->\n")
	if !ok {
		t.Fatalf("summary lacks transport envelope: %#v", summary)
	}
	return body
}

func storedReviewSummary(number uint64, verdict, body, commit, submittedAt string) map[string]any {
	return map[string]any{
		"body":         fmt.Sprintf("<!-- skl.watchdog.review/v1\n{\"review_number\":%d,\"verdict\":%q}\n-->\n%s", number, verdict, body),
		"commit_id":    commit,
		"state":        "COMMENTED",
		"submitted_at": submittedAt,
	}
}

func claimedReviewTimeline() []map[string]any {
	return []map[string]any{
		{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "review"}},
		{"event": "labeled", "created_at": "2026-01-01T00:00:02Z", "label": map[string]string{"name": "wip"}},
	}
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
