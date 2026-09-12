package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func TestGitHubWatchdogBodyPassRetry(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	for _, footer := range []bool{false, true} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("footer=%t/partial=%t", footer, partial), func(t *testing.T) {
				body := "opaque\x00\r\n  final prose\n"
				publishedBody := body + "\n\nCloses #7\n"
				if footer {
					body = publishedBody
				}
				actualBody := "original"
				labels := []string{"review", "wip"}
				var comments []map[string]string
				events := []map[string]any{{"event": "labeled", "label": map[string]string{"name": "review"}}, {"event": "labeled", "label": map[string]string{"name": "wip"}}}
				writes, patches := 0, 0
				interrupt := partial
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
					if r.Method != http.MethodGet {
						writes++
					}
					ls := []map[string]string{}
					for _, label := range labels {
						ls = append(ls, map[string]string{"name": label})
					}
					source := map[string]any{"number": 7, "title": "widget", "state": "open"}
					pull := map[string]any{"number": 11, "state": "open", "body": actualBody, "mergeable": true, "labels": ls, "head": map[string]any{"ref": "widget", "sha": head, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
					var result any = []any{}
					switch {
					case path == "/issues" && r.Method == http.MethodGet:
						result = []any{source}
					case path == "/pulls" && r.Method == http.MethodGet:
						result = []any{pull}
					case path == "/pulls/11" && r.Method == http.MethodPatch:
						var payload map[string]string
						json.NewDecoder(r.Body).Decode(&payload)
						actualBody = payload["body"]
						patches++
					case (path == "/pulls/11" || path == "/issues/11") && r.Method == http.MethodGet:
						result = pull
					case path == "/issues/7" && r.Method == http.MethodGet:
						result = source
					case path == "/issues/7/comments" && r.Method == http.MethodGet:
						result = []map[string]string{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"watchdog_head\":\"" + head + "\"}\n-->"}}
					case path == "/issues/11/comments":
						if r.Method == http.MethodPost {
							var payload map[string]string
							json.NewDecoder(r.Body).Decode(&payload)
							comments = append(comments, payload)
						}
						result = comments
					case path == "/issues/11/timeline" && r.Method == http.MethodGet:
						result = events
					case path == "/issues/11/labels" && r.Method == http.MethodPost:
						var payload struct{ Labels []string }
						json.NewDecoder(r.Body).Decode(&payload)
						for _, label := range payload.Labels {
							labels = append(labels, label)
							events = append(events, map[string]any{"event": "labeled", "label": map[string]string{"name": label}})
						}
					case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
						if interrupt {
							http.Error(w, "interrupted review cleanup", http.StatusBadRequest)
							return
						}
						label := strings.TrimPrefix(path, "/issues/11/labels/")
						labels = slices.DeleteFunc(labels, func(value string) bool { return value == label })
						events = append(events, map[string]any{"event": "unlabeled", "label": map[string]string{"name": label}})
					case path == "/git/ref/heads/widget" && r.Method == http.MethodGet:
						result = map[string]any{"object": map[string]string{"sha": head}}
					case (path == "/pulls/11/comments" || path == "/pulls/11/reviews") && r.Method == http.MethodGet:
					default:
						t.Errorf("unexpected %s %s", r.Method, r.URL)
						http.NotFound(w, r)
						return
					}
					json.NewEncoder(w).Encode(result)
				}))
				defer server.Close()
				backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
				backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
				dir := t.TempDir()
				summary, bodyPath := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md")
				for path, content := range map[string]string{summary: "opaque pass summary", bodyPath: body} {
					if err := os.WriteFile(path, []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
				}
				submit := func() (workflow.ImplementationOutcome, error) {
					return workflow.SubmitWatchdog(context.Background(), root, "origin", "7", head, head, "pass", summary, "", bodyPath, workflow.ArtifactEndpoints{}, backend)
				}
				first, err := submit()
				if partial {
					if err == nil || !strings.Contains(err.Error(), "interrupted review cleanup") || !slices.Contains(labels, "done") || !slices.Contains(labels, "wip") {
						t.Fatalf("expected durable partial pass: %#v, %v, labels=%v", first, err, labels)
					}
				} else if err != nil || first.Status != "ready_for_merge" {
					t.Fatalf("first pass: %#v, %v", first, err)
				}
				interrupt = false
				if actualBody != publishedBody || patches != 1 || len(comments) != 1 {
					t.Fatalf("publication: body=%q, patches=%d, comments=%v", actualBody, patches, comments)
				}
				before := writes
				if err := os.WriteFile(bodyPath, []byte("changed "+body), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := submit(); err == nil || !strings.Contains(err.Error(), "exact Result Documents") || writes != before {
					t.Fatalf("changed Result Document accepted or mutated backend: %v, writes=%d/%d", err, writes, before)
				}
				if err := os.WriteFile(bodyPath, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				for attempt := range 2 {
					retried, err := submit()
					if err != nil || retried.Status != "ready_for_merge" || retried.Item.Claimed || retried.Item.Submission.Body != publishedBody {
						t.Fatalf("unchanged pass retry: %#v, %v", retried, err)
					}
					if actualBody != publishedBody || patches != 1 || len(comments) != 1 || !slices.Equal(labels, []string{"done"}) || (!partial || attempt > 0) && writes != before {
						t.Fatalf("retry republished or changed evidence: body=%q, patches=%d, comments=%v, labels=%v, writes=%d/%d", actualBody, patches, comments, labels, writes, before)
					}
					before = writes
				}
			})
		}
	}
}

func TestGitHubWatchdogBodyObservation(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	for name, body := range map[string]string{
		"adopted":   "original",
		"published": "opaque\x00\r\n  prose\n\nCloses #7\n",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("observation mutated backend: %s %s", r.Method, r.URL)
					http.Error(w, "unexpected mutation", 500)
					return
				}
				pull := map[string]any{"number": 11, "state": "open", "body": body, "labels": []map[string]string{{"name": "review"}, {"name": "wip"}}, "head": map[string]any{"ref": "widget", "sha": head, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
				switch r.URL.Path {
				case "/repos/acme/widgets/issues":
					fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open"}]`)
				case "/repos/acme/widgets/pulls":
					json.NewEncoder(w).Encode([]any{pull})
				case "/repos/acme/widgets/pulls/11":
					json.NewEncoder(w).Encode(pull)
				case "/repos/acme/widgets/issues/7/comments":
					json.NewEncoder(w).Encode([]map[string]string{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"watchdog_head\":\"" + head + "\"}\n-->"}})
				case "/repos/acme/widgets/issues/11/comments", "/repos/acme/widgets/pulls/11/comments", "/repos/acme/widgets/pulls/11/reviews", "/repos/acme/widgets/issues/11/timeline":
					fmt.Fprint(w, `[]`)
				default:
					t.Errorf("unexpected %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
			backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			ctx := context.Background()
			inspected, err := workflow.InspectImplementation(ctx, root, "7", workflow.ArtifactEndpoints{}, backend)
			if err != nil || inspected.Item == nil {
				t.Fatalf("inspect: %#v, %v", inspected, err)
			}
			if inspected.Item.Submission.Body != body {
				t.Errorf("observed body = %q, want %q", inspected.Item.Submission.Body, body)
			}
			output, err := setup.PresentImplementation(inspected)
			if err != nil {
				t.Fatal(err)
			}
			status, err := setup.PresentStatus(workflow.StatusOutcome{Items: []workflow.ImplementationItem{*inspected.Item}})
			if err != nil {
				t.Fatal(err)
			}
			started, err := workflow.StartWatchdog(ctx, root, "origin", "7", workflow.ArtifactEndpoints{}, backend)
			if err != nil || started.Facts == nil {
				t.Fatalf("Watchdog resume: %#v, %v", started, err)
			}
			t.Cleanup(func() { os.RemoveAll(started.Facts.Watchdog.ResultDirectory) })
			packet, err := setup.PresentImplementation(started)
			if err != nil {
				t.Fatal(err)
			}
			for name, value := range map[string]any{"inspect": output, "status": status, "packet": packet} {
				data, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var native map[string]any
				if err := json.Unmarshal(data, &native); err != nil {
					t.Fatal(err)
				}
				item := native["item"]
				if name == "status" {
					item = native["items"].([]any)[0]
				}
				submission := item.(map[string]any)["Submission"].(map[string]any)
				if submission["Body"] != body || submission["Number"] != float64(11) {
					t.Errorf("%s native Submission = %#v, want exact body %q and number 11", name, submission, body)
				}
				if name == "packet" {
					audit := native["packet"].(map[string]any)["facts"].(map[string]any)["watchdog"].(map[string]any)["audit_body"]
					if audit != body {
						t.Errorf("audit_body = %q, want %q", audit, body)
					}
				}
			}
		})
	}
}
