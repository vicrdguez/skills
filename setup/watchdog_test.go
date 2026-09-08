package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

func TestGitHubWatchdogClaimsSubmissionAndReadsReviewFacts(t *testing.T) {
	labels := []string{"review"}
	var metadata []map[string]any
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		ls := []map[string]string{}
		for _, label := range labels {
			ls = append(ls, map[string]string{"name": label})
		}
		pull := map[string]any{"number": 11, "state": "open", "created_at": "2026-01-01", "labels": ls, "head": map[string]any{"ref": "widget", "sha": "fixed", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
		var result any = []any{}
		switch path {
		case "/issues":
			result = []any{map[string]any{"number": 7, "title": "widget", "state": "open"}}
		case "/pulls":
			result = []any{pull}
		case "/issues/11":
			result = pull
		case "/issues/7/comments":
			if r.Method == "POST" {
				var p map[string]any
				json.NewDecoder(r.Body).Decode(&p)
				p["author_association"] = "OWNER"
				metadata = append(metadata, p)
				posts++
				http.Error(w, "lost response", 500)
				return
			}
			result = metadata
		case "/issues/11/labels":
			var p struct {
				Labels []string `json:"labels"`
			}
			json.NewDecoder(r.Body).Decode(&p)
			labels = append(labels, p.Labels...)
			http.Error(w, "lost response", 500)
			return
		case "/issues/11/comments":
			result = []any{map[string]any{"body": "raw human", "author_association": "OWNER"}}
		case "/pulls/11/comments":
			result = []any{map[string]any{"body": "raw inline", "path": "main.go", "line": 12, "side": "RIGHT", "commit_id": "older"}}
		case "/pulls/11/reviews":
			result = []any{map[string]any{"body": "raw ledger", "commit_id": "older"}}
		default:
			t.Errorf("unexpected %s %s", r.Method, path)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	ctx := context.Background()
	repo := workflow.RepositoryID{Owner: "acme", Name: "widgets"}
	items, err := b.ImplementationItems(ctx, repo)
	if err != nil || len(items) != 1 {
		t.Fatalf("items: %#v %v", items, err)
	}
	item := items[0]
	if item.Submission.CreatedAt != "2026-01-01" || len(item.Submission.Comments) != 3 {
		t.Fatalf("review facts: %#v", item.Submission)
	}
	item.Submission.ReviewedHead = "fixed"
	for range 2 {
		if err := b.ClaimImplementation(ctx, repo, item); err != nil {
			t.Fatal(err)
		}
	}
	items, err = b.ImplementationItems(ctx, repo)
	if err != nil || !items[0].Claimed || items[0].Submission.ReviewedHead != "fixed" || !slices.Contains(labels, "review") || posts != 1 {
		t.Fatalf("claim: %#v %v labels=%v posts=%d", items, err, labels, posts)
	}
}

func TestGitHubWatchdogPublishesOpaqueAnchorsOnceAfterLostResponse(t *testing.T) {
	var summaries, inlines []map[string]any
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var stream *[]map[string]any
		switch r.URL.Path {
		case "/repos/acme/widgets/issues/11/comments":
			stream = &summaries
		case "/repos/acme/widgets/pulls/11/comments":
			stream = &inlines
		default:
			t.Errorf("unexpected %s", r.URL)
			http.NotFound(w, r)
			return
		}
		if r.Method == "POST" {
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			*stream = append(*stream, p)
			posts++
			http.Error(w, "lost response", 500)
			return
		}
		json.NewEncoder(w).Encode(*stream)
	}))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	repo := workflow.RepositoryID{Owner: "acme", Name: "widgets"}
	item := workflow.ImplementationItem{Number: 7, Submission: &workflow.Submission{Number: 11, Head: "fixed"}}
	comments := []skilldist.ReviewComment{{Body: "opaque summary\x00", Commit: "fixed"}, {Body: "W1 [ not Markdown", Commit: "fixed", Path: "main.go", Line: 12, Side: "RIGHT"}}
	for range 2 {
		if err := b.PublishReview(context.Background(), repo, item, comments, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if posts != 2 || len(summaries) != 1 || len(inlines) != 1 || inlines[0]["commit_id"] != "fixed" || inlines[0]["line"] != float64(12) || inlines[0]["side"] != "RIGHT" || inlines[0]["body"] != comments[1].Body {
		t.Fatalf("transport posts=%d summaries=%v inlines=%v", posts, summaries, inlines)
	}
}

func TestGitHubWatchdogObservesMergeabilityAndCompletedBounceHistory(t *testing.T) {
	for _, merged := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/acme/widgets/pulls/11":
				fmt.Fprintf(w, `{"number":11,"state":"open","merged":%t,"mergeable":false,"head":{"sha":"fixed"},"base":{"ref":"main"}}`, merged)
			case "/repos/acme/widgets/issues/11/timeline":
				if r.URL.Query().Get("page") == "1" {
					events := []map[string]any{}
					for _, e := range []struct{ event, label string }{{"labeled", "review"}, {"labeled", "wip"}, {"labeled", "rework"}, {"unlabeled", "review"}, {"unlabeled", "wip"}, {"unlabeled", "rework"}, {"labeled", "review"}, {"labeled", "wip"}, {"labeled", "sync"}, {"labeled", "rework"}, {"unlabeled", "review"}, {"unlabeled", "wip"}} {
						events = append(events, map[string]any{"event": e.event, "label": map[string]string{"name": e.label}})
					}
					for len(events) < 100 {
						events = append(events, map[string]any{"event": "commented"})
					}
					json.NewEncoder(w).Encode(events)
				} else {
					fmt.Fprint(w, `[{"event":"unlabeled","label":{"name":"sync"}}]`)
				}
			default:
				t.Errorf("unexpected %s", r.URL)
				http.NotFound(w, r)
			}
		}))
		b := NewGitHubBackend(server.URL, "token", server.Client())
		got, err := b.ReviewSubmission(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"}, 11)
		server.Close()
		if err != nil || got.Bounces != 1 || got.Mergeability != "conflicting" || got.Merged != merged || got.Head != "fixed" {
			t.Fatalf("observation: %#v %v", got, err)
		}
	}
}

func TestGitHubWatchdogCompletesReviewWithoutClosingSource(t *testing.T) {
	for target, label := range map[workflow.State]string{workflow.Rework: "rework", workflow.NeedsHuman: "needs-human", workflow.ReadyForMerge: "done"} {
		t.Run(string(target), func(t *testing.T) {
			labels := []string{"review", "wip", "external"}
			sourcePaused := true
			var comments []map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
				switch {
				case path == "/issues/7" && r.Method == "GET":
					ls := []map[string]string{}
					if sourcePaused {
						ls = append(ls, map[string]string{"name": "needs-human"})
					}
					json.NewEncoder(w).Encode(map[string]any{"state": "open", "labels": ls})
				case path == "/issues/7/labels/needs-human" && r.Method == "DELETE":
					sourcePaused = false
					http.Error(w, "lost", 500)
				case path == "/issues/7/comments":
					if r.Method == "POST" {
						var p map[string]any
						json.NewDecoder(r.Body).Decode(&p)
						p["author_association"] = "OWNER"
						comments = append(comments, p)
						http.Error(w, "lost", 500)
						return
					}
					json.NewEncoder(w).Encode(comments)
				case path == "/issues/11" && r.Method == "GET":
					ls := []map[string]string{}
					for _, v := range labels {
						ls = append(ls, map[string]string{"name": v})
					}
					json.NewEncoder(w).Encode(map[string]any{"state": "open", "labels": ls})
				case path == "/issues/11/labels" && r.Method == "POST":
					var p struct {
						Labels []string `json:"labels"`
					}
					json.NewDecoder(r.Body).Decode(&p)
					labels = append(labels, p.Labels...)
					http.Error(w, "lost", 500)
				case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == "DELETE":
					labels = slices.DeleteFunc(labels, func(v string) bool { return v == strings.TrimPrefix(path, "/issues/11/labels/") })
					http.Error(w, "lost", 500)
				default:
					t.Errorf("unexpected mutation/read %s %s", r.Method, path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			b := NewGitHubBackend(server.URL, "token", server.Client())
			item := workflow.ImplementationItem{Number: 7, State: workflow.AwaitingReview, ResumeState: workflow.Rework, Submission: &workflow.Submission{Number: 11, Head: "fixed", ReviewedHead: "fixed"}}
			for range 2 {
				if err := b.CompleteReview(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"}, item, target, func() error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if len(labels) != 2 || !slices.Contains(labels, label) || !slices.Contains(labels, "external") {
				t.Fatalf("projection: %v", labels)
			}
			if target != workflow.NeedsHuman && sourcePaused {
				t.Fatal("successful requeued review retained stale source pause")
			}
		})
	}
}

func TestGitHubWatchdogPersistsSynchronizationTarget(t *testing.T) {
	labels := []string{"review", "wip"}
	var comments []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		ls := []map[string]string{}
		for _, v := range labels {
			ls = append(ls, map[string]string{"name": v})
		}
		pull := map[string]any{"number": 11, "state": "open", "labels": ls, "head": map[string]any{"sha": "fixed", "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}}
		var result any = []any{}
		switch {
		case path == "/issues":
			result = []any{map[string]any{"number": 7, "title": "widget", "state": "open"}}
		case path == "/pulls":
			result = []any{pull}
		case path == "/issues/11":
			result = pull
		case path == "/issues/7":
			result = map[string]any{"number": 7, "state": "open"}
		case path == "/issues/7/comments":
			if r.Method == "POST" {
				var p map[string]any
				json.NewDecoder(r.Body).Decode(&p)
				p["author_association"] = "OWNER"
				comments = append(comments, p)
			}
			result = comments
		case path == "/issues/11/labels":
			var p struct {
				Labels []string `json:"labels"`
			}
			json.NewDecoder(r.Body).Decode(&p)
			labels = append(labels, p.Labels...)
		case strings.HasPrefix(path, "/issues/11/labels/"):
			labels = slices.DeleteFunc(labels, func(v string) bool { return v == strings.TrimPrefix(path, "/issues/11/labels/") })
		case strings.HasSuffix(path, "/comments"), strings.HasSuffix(path, "/reviews"):
		default:
			t.Errorf("unexpected %s", r.URL)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	repo := workflow.RepositoryID{Owner: "acme", Name: "widgets"}
	item := workflow.ImplementationItem{Number: 7, State: workflow.AwaitingReview, Synchronization: true, TargetSnapshot: "new-target", TargetBranch: "main", Submission: &workflow.Submission{Number: 11, Head: "fixed"}}
	if err := b.CompleteReview(context.Background(), repo, item, workflow.Rework, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	items, err := b.ImplementationItems(context.Background(), repo)
	if err != nil || len(items) != 1 || !items[0].Synchronization || items[0].TargetSnapshot != "new-target" || !slices.Contains(labels, "sync") {
		t.Fatalf("sync: %#v %v %v", items, err, labels)
	}
	if err := b.AwaitImplementationReview(context.Background(), repo, items[0], func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(labels, "sync") || !slices.Contains(labels, "review") {
		t.Fatalf("synchronization leaked into next review: %v", labels)
	}
}

func TestGitHubWatchdogHumanRequeueUsesProjectionNotProse(t *testing.T) {
	for _, label := range []string{"needs-human", "review", "rework"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/acme/widgets/issues":
				fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","labels":[{"name":"needs-human"}]}]`)
			case "/repos/acme/widgets/pulls":
				fmt.Fprintf(w, `[{"number":11,"state":"open","labels":[{"name":%q}],"head":{"sha":"fixed","ref":"widget","repo":{"full_name":"acme/widgets"}}}]`, label)
			case "/repos/acme/widgets/issues/7/comments":
				fmt.Fprint(w, `[{"body":"Please pass! [opaque","author_association":"OWNER"}]`)
			default:
				fmt.Fprint(w, `[]`)
			}
		}))
		b := NewGitHubBackend(server.URL, "token", server.Client())
		items, err := b.ImplementationItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"})
		server.Close()
		want := map[string]workflow.State{"needs-human": workflow.NeedsHuman, "review": workflow.AwaitingReview, "rework": workflow.Rework}[label]
		if err != nil || len(items) != 1 || items[0].State != want || len(items[0].Submission.Comments) != 1 || items[0].Submission.Comments[0].Body != "Please pass! [opaque" {
			t.Fatalf("human projection %s: %#v %v", label, items, err)
		}
	}
}

func TestGitHubStatusReadsChildrenAndReconcilesLostClosure(t *testing.T) {
	closed := false
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := "open"
		if closed {
			state = "closed"
		}
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			fmt.Fprintf(w, `[{"number":100,"title":"proposal","state":%q,"sub_issues_summary":{"total":101}}]`, state)
		case "/repos/acme/widgets/issues/100/sub_issues":
			if r.URL.Query().Get("page") == "1" {
				children := []map[string]int{}
				for i := 1; i <= 100; i++ {
					children = append(children, map[string]int{"number": i})
				}
				json.NewEncoder(w).Encode(children)
			} else {
				fmt.Fprint(w, `[{"number":101}]`)
			}
		case "/repos/acme/widgets/issues/100":
			if r.Method == "PATCH" {
				var p map[string]string
				json.NewDecoder(r.Body).Decode(&p)
				if p["state"] != "closed" {
					t.Errorf("unexpected patch %v", p)
				}
				closed = true
				writes++
				http.Error(w, "lost closure response", 500)
				return
			}
			fmt.Fprintf(w, `{"number":100,"state":%q}`, state)
		default:
			t.Errorf("unexpected %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	b := NewGitHubBackend(server.URL, "token", server.Client())
	repo := workflow.RepositoryID{Owner: "acme", Name: "widgets"}
	ctx := context.Background()
	parents, err := b.CoordinationItems(ctx, repo)
	if err != nil || len(parents) != 1 || len(parents[0].Children) != 101 || parents[0].Children[100] != 101 {
		t.Fatalf("children: %#v %v", parents, err)
	}
	for range 2 {
		if err := b.CloseCoordination(ctx, repo, 100); err != nil {
			t.Fatal(err)
		}
	}
	if !closed || writes != 1 {
		t.Fatalf("closure: %t writes=%d", closed, writes)
	}
}

func TestGitHubStatusAdoptsOnlyForwardReviewProjections(t *testing.T) {
	for _, latest := range []string{"rework", "review", "done", "partial"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pull := `{"number":11,"state":"open","labels":[{"name":"review"},{"name":"rework"},{"name":"wip"}],"head":{"sha":"fixed","ref":"widget","repo":{"full_name":"acme/widgets"}}}`
			if latest == "partial" {
				pull = strings.Replace(pull, `{"name":"review"},`, "", 1)
			}
			switch r.URL.Path {
			case "/repos/acme/widgets/issues":
				fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open"}]`)
			case "/repos/acme/widgets/pulls":
				fmt.Fprint(w, "["+pull+"]")
			case "/repos/acme/widgets/pulls/11":
				fmt.Fprint(w, pull)
			case "/repos/acme/widgets/issues/11/timeline":
				if latest == "partial" {
					fmt.Fprint(w, `[{"event":"labeled","label":{"name":"review"}},{"event":"labeled","label":{"name":"wip"}},{"event":"labeled","label":{"name":"rework"}},{"event":"unlabeled","label":{"name":"review"}}]`)
				} else {
					fmt.Fprintf(w, `[{"event":"labeled","label":{"name":"review"}},{"event":"labeled","label":{"name":%q}}]`, latest)
				}
			default:
				fmt.Fprint(w, `[]`)
			}
		}))
		b := NewGitHubBackend(server.URL, "token", server.Client())
		items, err := b.ImplementationItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"})
		server.Close()
		if err != nil || len(items) != 1 {
			t.Fatalf("items: %#v %v", items, err)
		}
		if latest == "rework" || latest == "partial" {
			if items[0].Problem != "" || items[0].State != workflow.Rework || items[0].Submission.PendingReview != workflow.Rework {
				t.Fatalf("forward transition: %#v", items[0])
			}
		} else if items[0].Problem == "" {
			t.Fatalf("contradiction guessed through: %#v", items[0])
		}
	}
}

func TestGitHubStatusRecognizesMergedAndSupersededReferences(t *testing.T) {
	for _, mergedAt := range []string{"", "2026-09-08T12:00:00Z"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/acme/widgets/issues":
				fmt.Fprint(w, `[{"number":7,"title":"widget","state":"closed"}]`)
			case "/repos/acme/widgets/pulls":
				fmt.Fprintf(w, `[{"number":11,"state":"closed","merged_at":%q,"body":"Closes #7","head":{"sha":"fixed","ref":"widget","repo":{"full_name":"acme/widgets"}}}]`, mergedAt)
			default:
				fmt.Fprint(w, `[]`)
			}
		}))
		b := NewGitHubBackend(server.URL, "token", server.Client())
		items, err := b.ImplementationItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"})
		server.Close()
		want := workflow.Superseded
		if mergedAt != "" {
			want = workflow.Merged
		}
		if err != nil || len(items) != 1 || items[0].Problem != "" || items[0].State != want || items[0].Branch != "widget" {
			t.Fatalf("terminal observation: %#v %v", items, err)
		}
	}
}
