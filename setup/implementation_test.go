package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/workflow"
)

func TestGitHubImplementationNormalizesPaginatedWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			if r.URL.Query().Get("page") == "1" {
				fmt.Fprint(w, "[")
				for i := 0; i < 100; i++ {
					if i > 0 {
						fmt.Fprint(w, ",")
					}
					fmt.Fprintf(w, `{"number":%d,"title":"unrelated","state":"open"}`, 100+i)
				}
				fmt.Fprint(w, "]")
			} else {
				fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","created_at":"2020-01-01T00:00:00Z","labels":[{"name":"ready"}]}]`)
			}
		case "/repos/acme/widgets/pulls":
			fmt.Fprint(w, `[]`)
		case "/repos/acme/widgets/issues/7/dependencies/blocked_by":
			fmt.Fprint(w, `[]`)
		case "/repos/acme/widgets/issues/7/comments":
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	backend := NewGitHubBackend(server.URL, "token", server.Client())
	items, err := backend.ImplementationItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"})
	if err != nil || len(items) != 1 || items[0].Number != 7 || items[0].State != workflow.Ready || items[0].Branch != "widget" || items[0].CreatedAt != "2020-01-01T00:00:00Z" {
		t.Fatalf("items = %#v, %v", items, err)
	}
}

func TestGitHubImplementationReconcilesMutationTimeouts(t *testing.T) {
	labels := map[int][]string{7: {"ready", "external"}}
	comments := map[int][]map[string]any{}
	var pulls []map[string]any
	creations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		issue := func(number int) map[string]any {
			ls := []map[string]string{}
			for _, name := range labels[number] {
				ls = append(ls, map[string]string{"name": name})
			}
			return map[string]any{"number": number, "title": "widget", "state": "open", "labels": ls}
		}
		var result any = map[string]any{}
		switch {
		case path == "/issues":
			result = []any{issue(7)}
		case path == "/pulls" && r.Method == http.MethodGet:
			result = pulls
		case path == "/pulls" && r.Method == http.MethodPost:
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			creations++
			pulls = append(pulls, map[string]any{"number": 11, "node_id": "PR_11", "title": "widget", "state": "open", "body": payload["body"], "draft": payload["draft"], "head": map[string]string{"ref": "widget", "sha": "fixed"}, "base": map[string]string{"ref": "main"}})
			http.Error(w, "response lost after creation", 500)
			return
		case path == "/pulls/11" && r.Method == http.MethodGet:
			result = pulls[0]
		case path == "/pulls/11" && r.Method == http.MethodPatch:
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			for key, value := range payload {
				if key == "base" {
					value = map[string]any{"ref": value}
				}
				pulls[0][key] = value
			}
			http.Error(w, "response lost after update", 500)
			return
		case path == "/issues/7/dependencies/blocked_by":
			result = []any{}
		case strings.HasSuffix(path, "/comments"):
			var number int
			fmt.Sscanf(path, "/issues/%d/comments", &number)
			if r.Method == http.MethodPost {
				var payload map[string]any
				json.NewDecoder(r.Body).Decode(&payload)
				comments[number] = append(comments[number], payload)
				http.Error(w, "response lost after comment", 500)
				return
			}
			result = comments[number]
		case strings.Contains(path, "/labels"):
			var number int
			fmt.Sscanf(path, "/issues/%d/labels", &number)
			if r.Method == http.MethodPost {
				var payload struct {
					Labels []string `json:"labels"`
				}
				json.NewDecoder(r.Body).Decode(&payload)
				labels[number] = append(labels[number], payload.Labels...)
			} else if r.Method == http.MethodDelete {
				name := path[strings.LastIndex(path, "/")+1:]
				var kept []string
				for _, label := range labels[number] {
					if label != name {
						kept = append(kept, label)
					}
				}
				labels[number] = kept
			}
			http.Error(w, "response lost after label mutation", 500)
			return
		case path == "/issues/7":
			result = issue(7)
		case path == "/issues/11":
			result = issue(11)
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
	item := workflow.ImplementationItem{Number: 7, Branch: "widget", State: workflow.Ready, TargetSnapshot: "snapshot"}
	if err := b.ClaimImplementation(ctx, repo, item); err != nil {
		t.Fatal(err)
	}
	items, err := b.ImplementationItems(ctx, repo)
	if err != nil || len(items) != 1 || !items[0].Claimed || items[0].TargetSnapshot != "snapshot" {
		t.Fatalf("Claim: %#v %v", items, err)
	}
	item = items[0]
	submission, err := b.PublishImplementation(ctx, repo, item, workflow.Submission{Head: "fixed", Base: "main", Body: "opaque\n\nCloses #7\n"})
	if err != nil {
		t.Fatal(err)
	}
	item.Submission = &submission
	submission.Body = "updated opaque\n\nCloses #7\n"
	updated, err := b.PublishImplementation(ctx, repo, item, submission)
	if err != nil || updated.Body != submission.Body || creations != 1 {
		t.Fatalf("update = %#v, %v, creations=%d", updated, err, creations)
	}
	item.Submission = &updated
	if err := b.AwaitImplementationReview(ctx, repo, item); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(labels[7]) != "[external]" || fmt.Sprint(labels[11]) != "[review]" {
		t.Fatalf("labels=%v", labels)
	}
}
