package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

func boundGitHubBackend(backend *GitHubBackend) *GitHubBackend {
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	return backend
}

func TestGitHubImplementationRejectsInvalidOwningLinks(t *testing.T) {
	cases := map[string]string{
		"no explicit owning issue":     "unowned body",
		"multiple conflicting owners":  "opening\n\nCloses #7\nCloses #8\n",
		"outside the repository":       "opening\n\nCloses acme/other#7\n",
		"conflicting trailing footers": "opening\n\nCloses #7\n\nCloses #8\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					http.Error(w, "unexpected mutation", 500)
					return
				}
				switch {
				case r.URL.Path == "/repos/acme/widgets/pulls/11":
					json.NewEncoder(w).Encode(map[string]any{"number": 11, "state": "open", "body": body, "head": map[string]any{"ref": "widget", "sha": "fixed", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
			item, err := b.SelectedImplementation(context.Background(), workflow.QueueCandidate{SubmissionID: "11", Number: 11})
			if err != nil || item.Problem == "" {
				t.Fatalf("invalid ownership accepted: %#v %v", item, err)
			}
			if writes != 0 {
				t.Fatalf("invalid ownership caused %d mutations", writes)
			}
		})
	}
}

func TestGitHubImplementationReportsMultipleActiveOwner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[{"number":11,"repository":{"nameWithOwner":"acme/widgets"}},{"number":12,"repository":{"nameWithOwner":"acme/widgets"}}]}}}}}`)
		case r.URL.Path == "/repos/acme/widgets/pulls/11":
			json.NewEncoder(w).Encode(map[string]any{"number": 11, "state": "open", "body": "opening\n\nCloses #7\n", "head": map[string]any{"ref": "widget", "sha": "fixed", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}})
		case r.URL.Path == "/repos/acme/widgets/issues/7":
			json.NewEncoder(w).Encode(map[string]any{"number": 7, "state": "open", "labels": []map[string]string{{"name": "rework"}}})
		case strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/reviews"):
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
	item, err := b.SelectedImplementation(context.Background(), workflow.QueueCandidate{SubmissionID: "11", Number: 11})
	if err != nil || !strings.Contains(item.Problem, "another active Submission") {
		t.Fatalf("competing owner accepted: %#v %v", item, err)
	}
}

func TestGitHubImplementationRejectsReassignedSourceBeforePublication(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
			http.Error(w, "unexpected mutation", 500)
			return
		}
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			fmt.Fprint(w, `[{"number":7,"title":"renamed","state":"open"},{"number":8,"title":"widget","state":"open","labels":[{"name":"ready"}]}]`)
		case "/repos/acme/widgets/pulls":
			fmt.Fprint(w, `[{"number":11,"state":"open","body":"original","head":{"ref":"widget","sha":"fixed","repo":{"full_name":"acme/widgets"}},"base":{"ref":"main"}}]`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
	_, err := b.PublishImplementation(context.Background(), workflow.ImplementationItem{ID: "7", Branch: "widget"}, workflow.Submission{ID: "11", Head: "fixed", Base: "main", Body: "replacement"})
	if err == nil || writes != 0 {
		t.Fatalf("reassigned source allowed publication: %v, writes=%d", err, writes)
	}
}

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
				fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","body":"Branch: `+"`widget`"+`\n\nBlocked by: #2, #3","created_at":"2020-01-01T00:00:00Z","labels":[{"name":"ready"}]}]`)
			}
		case "/repos/acme/widgets/pulls":
			fmt.Fprint(w, `[]`)
		case "/repos/acme/widgets/issues/7/dependencies/blocked_by":
			fmt.Fprint(w, `[{"number":2}]`)
		case "/repos/acme/widgets/issues/7/comments":
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	backend := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
	items, err := backend.ImplementationItems(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != "7" || items[0].Order != 7 || !slices.Equal(items[0].Blockers, []workflow.WorkItemID{"2", "3"}) || items[0].State != workflow.Ready || items[0].Branch != "widget" || items[0].CreatedAt != "2020-01-01T00:00:00Z" {
		t.Fatalf("items = %#v, %v", items, err)
	}
}

func TestGitHubLifecycleRejectsInvalidIdentitiesBeforeTransport(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected transport", http.StatusBadRequest)
	}))
	defer server.Close()
	b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
	ctx := context.Background()
	guard := func() error { return nil }
	operations := map[string]func(workflow.ImplementationItem) error{
		"claim": func(item workflow.ImplementationItem) error {
			candidate := workflow.QueueCandidate{ID: item.ID}
			if item.Submission != nil {
				candidate.SubmissionID = item.Submission.ID
			}
			_, err := b.ClaimSelected(ctx, candidate, item)
			return err
		},
		"publish": func(item workflow.ImplementationItem) error {
			_, err := b.PublishImplementation(ctx, item, *item.Submission)
			return err
		},
		"await": func(item workflow.ImplementationItem) error {
			return b.AwaitImplementationReview(ctx, item, guard)
		},
		"pause": func(item workflow.ImplementationItem) error {
			return b.PauseImplementation(ctx, item, "opaque decision", guard)
		},
		"complete-review": func(item workflow.ImplementationItem) error {
			return b.CompleteReview(ctx, item, workflow.Rework, guard)
		},
		"close-coordination": func(item workflow.ImplementationItem) error {
			return b.CloseCoordination(ctx, item.ID)
		},
		"review-submission": func(item workflow.ImplementationItem) error {
			_, err := b.ReviewSubmission(ctx, item.Submission.ID)
			return err
		},
		"publish-review": func(item workflow.ImplementationItem) error {
			return b.PublishReview(ctx, item, nil, guard)
		},
	}
	for _, id := range []string{"", "0", "-1", "07", "7/labels", "issue:7", "999999999999999999999999"} {
		for _, identity := range []string{"item", "submission"} {
			for name, operation := range operations {
				if identity == "item" && (name == "review-submission" || name == "publish-review") || identity == "submission" && (name == "close-coordination" || name == "claim" || name == "publish" && id == "") {
					continue
				}
				t.Run(name+"/"+identity+"/"+id, func(t *testing.T) {
					item := workflow.ImplementationItem{ID: "7", State: workflow.Rework, Submission: &workflow.Submission{ID: "11"}}
					if identity == "item" {
						item.ID = workflow.WorkItemID(id)
					} else {
						item.Submission.ID = workflow.SubmissionID(id)
					}
					run := operation
					if name == "claim" {
						run = func(item workflow.ImplementationItem) error {
							candidate := workflow.QueueCandidate{ID: item.ID}
							if identity == "submission" {
								candidate = workflow.QueueCandidate{SubmissionID: item.Submission.ID}
							}
							_, err := b.ClaimSelected(ctx, candidate, item)
							return err
						}
					}
					if err := run(item); err == nil || !strings.Contains(err.Error(), "invalid GitHub issue identity") {
						t.Fatalf("invalid identity accepted: %v", err)
					}
				})
			}
		}
	}
	if requests != 0 {
		t.Fatalf("invalid identities reached transport: %d requests", requests)
	}
}

func TestGitHubImplementationReconcilesMutationTimeouts(t *testing.T) {
	labels := map[int][]string{7: {"ready", "external"}}
	comments := map[int][]map[string]any{7: {{"author_association": "NONE", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"snapshot\"}\n-->"}}}
	timeline := map[int][]map[string]any{}
	clock := 0
	timestamp := func() string {
		clock++
		return fmt.Sprintf("2026-01-01T00:00:%02dZ", clock)
	}
	var pulls []map[string]any
	creations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		issue := func(number int) map[string]any {
			ls := []map[string]string{}
			for _, name := range labels[number] {
				ls = append(ls, map[string]string{"name": name})
			}
			return map[string]any{"number": number, "title": "widget", "state": "open", "body": "Branch: `widget`\n", "labels": ls}
		}
		var result any = map[string]any{}
		switch {
		case path == "/issues":
			result = []any{issue(7)}
		case path == "/pulls" && r.Method == http.MethodGet:
			for _, pull := range pulls {
				pull["labels"] = issue(11)["labels"]
			}
			result = pulls
		case path == "/pulls" && r.Method == http.MethodPost:
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			creations++
			pulls = append(pulls, map[string]any{"number": 11, "node_id": "PR_11", "title": "widget", "state": "open", "body": payload["body"], "draft": payload["draft"], "head": map[string]any{"ref": "widget", "sha": "fixed", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}})
			http.Error(w, "response lost after creation", 500)
			return
		case path == "/pulls/11" && r.Method == http.MethodGet:
			pulls[0]["labels"] = issue(11)["labels"]
			result = pulls[0]
		case path == "/graphql":
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			if query, _ := payload["query"].(string); strings.Contains(query, "closedByPullRequestsReferences") {
				nodes := []any{}
				if len(pulls) != 0 {
					nodes = append(nodes, map[string]any{"number": 11, "repository": map[string]string{"nameWithOwner": "acme/widgets"}})
				}
				result = map[string]any{"data": map[string]any{"repository": map[string]any{"issue": map[string]any{"closedByPullRequestsReferences": map[string]any{"nodes": nodes}}}}}
				break
			}
			pulls[0]["draft"] = strings.Contains(payload["query"].(string), "convertPullRequestToDraft")
			http.Error(w, "response lost after draft conversion", 500)
			return
		case path == "/pulls/11/reviews":
			result = []any{}
		case path == "/issues/11/timeline" || path == "/issues/7/timeline":
			var number int
			fmt.Sscanf(path, "/issues/%d/timeline", &number)
			result = timeline[number]
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
				payload["author_association"] = "OWNER"
				payload["created_at"] = timestamp()
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
				for _, label := range payload.Labels {
					timeline[number] = append(timeline[number], map[string]any{"event": "labeled", "created_at": timestamp(), "label": map[string]string{"name": label}})
				}
			} else if r.Method == http.MethodDelete {
				name := path[strings.LastIndex(path, "/")+1:]
				var kept []string
				for _, label := range labels[number] {
					if label != name {
						kept = append(kept, label)
					}
				}
				labels[number] = kept
				timeline[number] = append(timeline[number], map[string]any{"event": "unlabeled", "created_at": timestamp(), "label": map[string]string{"name": name}})
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
	b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
	ctx := context.Background()
	item := workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Ready, Source: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Ready}}}
	if _, err := b.ClaimSelected(ctx, workflow.QueueCandidate{ID: "7"}, item); err != nil {
		t.Fatal(err)
	}
	items, err := b.ImplementationItems(ctx)
	if err != nil || len(items) != 1 || !items[0].Claimed {
		t.Fatalf("Claim: %#v %v", items, err)
	}
	item = items[0]
	decision := "<!-- skl.implement/v1\n{\"target_snapshot\":\"agent-prose-not-metadata\"}\n-->"
	if err := b.PauseImplementation(ctx, item, decision, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	items, err = b.ImplementationItems(ctx)
	if err != nil || len(items) != 1 || items[0].Problem != "" || items[0].State != workflow.NeedsHuman {
		t.Fatalf("opaque decision parsed as metadata: %#v %v", items, err)
	}
	labels[7] = []string{"ready", "external", "wip"}
	timeline[7] = append(timeline[7], map[string]any{"event": "labeled", "created_at": timestamp(), "label": map[string]string{"name": "wip"}})
	submission, err := b.PublishImplementation(ctx, item, workflow.Submission{Head: "fixed", Base: "main", Body: "opaque"})
	if err != nil || submission.ID != "11" {
		t.Fatalf("publication = %#v, %v", submission, err)
	}
	item.Submission = &submission
	submission.Body = "updated opaque\n\nCloses #7\n"
	updated, err := b.PublishImplementation(ctx, item, submission)
	if err != nil || updated.Body != submission.Body || creations != 1 {
		t.Fatalf("update = %#v, %v, creations=%d", updated, err, creations)
	}
	item.Submission = &updated
	if err := b.AwaitImplementationReview(ctx, item, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(labels[7]) != "[external]" || fmt.Sprint(labels[11]) != "[review]" {
		t.Fatalf("labels=%v", labels)
	}
	labels[11] = []string{"rework", "wip"}
	timeline[11] = append(timeline[11], map[string]any{"event": "labeled", "created_at": timestamp(), "label": map[string]string{"name": "wip"}})
	item.State = workflow.Rework
	item.Claimed = true
	labels[7] = []string{"external"}
	updated.Draft = true
	draft, err := b.PublishImplementation(ctx, item, updated)
	if err != nil || !draft.Draft {
		t.Fatalf("draft = %#v %v", draft, err)
	}
	item.Submission = &draft
	interrupted := false
	guard := func() error {
		if !interrupted && slices.Contains(labels[11], "needs-human") && slices.Contains(labels[11], "rework") {
			interrupted = true
			return errors.New("interrupted before final projection")
		}
		return nil
	}
	if err := b.PauseImplementation(ctx, item, "opaque decision", guard); err == nil {
		t.Fatal("fixture did not interrupt pause")
	}
	items, err = b.ImplementationItems(ctx)
	if err != nil || len(items) != 1 || items[0].Problem == "" || !items[0].Claimed {
		t.Fatalf("interrupted pause = %#v %v", items, err)
	}
	if err := b.PauseImplementation(ctx, item, "opaque decision", guard); err != nil {
		t.Fatal(err)
	}
	items, err = b.ImplementationItems(ctx)
	if err != nil || items[0].State != workflow.NeedsHuman || items[0].Claimed || !items[0].Submission.Draft {
		t.Fatalf("completed pause = %#v %v", items, err)
	}
}

func TestGitHubImplementationCompletesRequeuedSubmissionHandoff(t *testing.T) {
	for _, tt := range []struct {
		name        string
		interrupted bool
	}{
		{name: "accepted human requeue"},
		{name: "retry after interrupted publication", interrupted: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			labels := map[int][]string{7: {"needs-human"}, 11: {"rework", "wip"}}
			interrupt := tt.interrupted
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
				issue := func(number int) map[string]any {
					ls := []map[string]string{}
					for _, name := range labels[number] {
						ls = append(ls, map[string]string{"name": name})
					}
					return map[string]any{"number": number, "state": "open", "labels": ls}
				}
				switch {
				case path == "/graphql":
					fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[{"number":11,"repository":{"nameWithOwner":"acme/widgets"}}]}}}}}`)
					return
				case path == "/issues" && r.Method == http.MethodGet:
					issue := issue(7)
					issue["title"] = "widget"
					json.NewEncoder(w).Encode([]any{issue})
					return
				case path == "/pulls" && r.Method == http.MethodGet:
					pull := issue(11)
					pull["body"] = "original\n\nCloses #7\n"
					pull["head"] = map[string]any{"ref": "widget", "sha": "fixed", "repo": map[string]string{"full_name": "acme/widgets"}}
					pull["base"] = map[string]string{"ref": "main"}
					json.NewEncoder(w).Encode([]any{pull})
					return
				case path == "/issues/7" && r.Method == http.MethodGet:
					json.NewEncoder(w).Encode(issue(7))
					return
				case path == "/issues/11" && r.Method == http.MethodGet:
					json.NewEncoder(w).Encode(issue(11))
					return
				case path == "/issues/7/comments" && r.Method == http.MethodGet:
					fmt.Fprint(w, `[{"author_association":"OWNER","body":"<!-- skl.implement/v1\n{\"transition\":{\"from\":\"rework\",\"target\":\"awaiting_review\",\"head\":\"fixed\",\"directory\":\"original-operation\"}}\n-->"}]`)
					return
				}
				var number int
				var name string
				switch {
				case r.Method == http.MethodPost && strings.HasSuffix(path, "/labels"):
					fmt.Sscanf(path, "/issues/%d/labels", &number)
					var payload struct {
						Labels []string `json:"labels"`
					}
					json.NewDecoder(r.Body).Decode(&payload)
					labels[number] = append(labels[number], payload.Labels...)
				case r.Method == http.MethodDelete && strings.Contains(path, "/labels/"):
					fmt.Sscanf(path, "/issues/%d/labels/%s", &number, &name)
					if interrupt && number == 7 && name == "needs-human" {
						interrupt = false
						http.Error(w, "lost response", http.StatusInternalServerError)
						return
					}
					labels[number] = slices.DeleteFunc(labels[number], func(label string) bool { return label == name })
				default:
					json.NewEncoder(w).Encode([]any{})
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
			item := workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Rework,
				Submission: &workflow.Submission{ID: "11", Head: "fixed", Lifecycle: &workflow.LifecycleObservation{Open: true, States: []workflow.State{workflow.Rework}, Claimed: true}}}
			ctx := context.Background()
			if tt.interrupted {
				if err := b.AwaitImplementationReview(ctx, item, func() error { return nil }); err == nil {
					t.Fatal("fixture did not interrupt the source cleanup")
				}
			}
			if err := b.AwaitImplementationReview(ctx, item, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			items, err := b.ImplementationItems(ctx)
			if err != nil || len(items) != 1 || items[0].Problem != "" || items[0].State != workflow.AwaitingReview || items[0].Claimed {
				t.Fatalf("requeued handoff: %#v %v", items, err)
			}
			if !slices.Equal(labels[11], []string{"review"}) || slices.Contains(labels[7], "needs-human") {
				t.Fatalf("labels after requeued handoff: %v", labels)
			}
		})
	}
}

func TestGitHubImplementationRejectsForeignAttachmentsAndIgnoresObsoleteTargetMetadata(t *testing.T) {
	for _, kind := range []string{"fork", "untrusted metadata", "conflicting metadata"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/acme/widgets/issues":
					fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","labels":[{"name":"ready"},{"name":"wip"}]}]`)
				case "/repos/acme/widgets/pulls":
					if kind == "fork" {
						fmt.Fprint(w, `[{"number":11,"state":"closed","merged_at":"2020","head":{"ref":"widget","repo":{"full_name":"outsider/widgets"}}}]`)
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/repos/acme/widgets/issues/7/dependencies/blocked_by":
					fmt.Fprint(w, `[]`)
				case "/repos/acme/widgets/issues/7/timeline":
					fmt.Fprint(w, `[]`)
				case "/repos/acme/widgets/issues/7/comments":
					comments := []map[string]string{}
					if kind != "fork" {
						comments = append(comments, map[string]string{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"original\"}\n-->"})
						association := "NONE"
						if kind == "conflicting metadata" {
							association = "OWNER"
						}
						comments = append(comments, map[string]string{"author_association": association, "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"replacement\"}\n-->"})
					}
					json.NewEncoder(w).Encode(comments)
				default:
					t.Errorf("unexpected %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
			items, err := b.ImplementationItems(context.Background())
			if err != nil || len(items) != 1 {
				t.Fatalf("items = %#v %v", items, err)
			}
			item := items[0]
			if kind == "fork" && (item.Submission != nil || item.State == workflow.Merged) || kind != "fork" && item.Problem != "" {
				t.Fatalf("unsafe adoption: %#v", item)
			}
		})
	}
}

func TestGitHubImplementationUsesMainAndNeverRetargetsExistingSubmission(t *testing.T) {
	for _, existingBase := range []string{"", "release"} {
		t.Run(existingBase, func(t *testing.T) {
			writes := 0
			postedBase := ""
			handlerErr := ""
			pull := map[string]any{"number": 11, "state": "open", "body": "wanted", "head": map[string]any{"ref": "widget", "sha": "fixed", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": existingBase}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet && r.URL.Path != "/graphql" {
					writes++
				}
				switch r.URL.Path {
				case "/repos/acme/widgets/issues":
					fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","labels":[{"name":"ready"}]}]`)
				case "/repos/acme/widgets/pulls":
					if r.Method == http.MethodPost {
						var payload map[string]any
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							handlerErr = err.Error()
							http.Error(w, handlerErr, http.StatusBadRequest)
							return
						}
						postedBase, _ = payload["base"].(string)
						pull["base"] = map[string]string{"ref": postedBase}
						pull["body"] = payload["body"]
						json.NewEncoder(w).Encode(pull)
					} else if existingBase == "" {
						fmt.Fprint(w, `[]`)
					} else {
						json.NewEncoder(w).Encode([]any{pull})
					}
				case "/repos/acme/widgets/pulls/11":
					json.NewEncoder(w).Encode(pull)
				case "/graphql":
					if existingBase == "" && postedBase == "" {
						fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[]}}}}}`)
					} else {
						fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[{"number":11,"repository":{"nameWithOwner":"acme/widgets"}}]}}}}}`)
					}
				default:
					handlerErr = fmt.Sprintf("unexpected %s %s", r.Method, r.URL)
					http.Error(w, handlerErr, http.StatusNotFound)
				}
			}))
			defer server.Close()
			b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
			wanted := workflow.Submission{Head: "fixed", Base: "release", Body: "wanted"}
			if existingBase != "" {
				wanted.ID = "11"
			}
			got, err := b.PublishImplementation(context.Background(), workflow.ImplementationItem{ID: "7", Branch: "widget"}, wanted)
			if existingBase == "" {
				if err != nil || handlerErr != "" || got.Base != "main" || postedBase != "main" || writes != 1 {
					t.Fatalf("creation = %#v, %v, base=%q writes=%d", got, err, postedBase, writes)
				}
			} else if handlerErr != "" || err == nil || !strings.Contains(err.Error(), "repair its base to main") || writes != 0 {
				t.Fatalf("non-main update = %#v, %v, writes=%d", got, err, writes)
			}
		})
	}
}

func TestGitHubImplementationPreservesPendingLifecycleObservations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("observation mutated backend: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected mutation", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			fmt.Fprint(w, `[{"number":7,"title":"widget","state":"open","labels":[{"name":"ready"},{"name":"needs-human"},{"name":"wip"},{"name":"external"}]}]`)
		case "/repos/acme/widgets/pulls":
			fmt.Fprint(w, `[{"number":11,"state":"open","body":"original\n\nCloses #7\n","labels":[{"name":"needs-human"}],"head":{"ref":"widget","sha":"fixed","repo":{"full_name":"acme/widgets"}},"base":{"ref":"main"}}]`)
		case "/repos/acme/widgets/pulls/11":
			fmt.Fprint(w, `{"number":11,"state":"open","labels":[{"name":"needs-human"}],"head":{"ref":"widget","sha":"fixed","repo":{"full_name":"acme/widgets"}},"base":{"ref":"main"}}`)
		case "/repos/acme/widgets/issues/7/comments":
			fmt.Fprint(w, `[{"author_association":"OWNER","body":"<!-- skl.implement/v1\n{\"target_snapshot\":\"pinned\",\"transition\":{\"from\":\"ready_for_implementation\",\"target\":\"needs_human\",\"head\":\"fixed\",\"directory\":\"original-operation\"}}\n-->"}]`)
		default:
			fmt.Fprint(w, `[]`)
		}
	}))
	defer server.Close()
	b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
	items, err := b.ImplementationItems(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("observations: %#v, %v", items, err)
	}
	item := items[0]
	if item.Source == nil || !item.Source.Open || !item.Source.Claimed || !slices.Equal(item.Source.States, []workflow.State{workflow.Ready, workflow.NeedsHuman}) || item.Submission.Lifecycle == nil || !slices.Equal(item.Submission.Lifecycle.States, []workflow.State{workflow.NeedsHuman}) {
		t.Fatalf("lost normalized partial progress: %#v, %#v", item, item.Submission)
	}
	if item.Problem != "contradictory lifecycle projections" || !item.Claimed {
		t.Fatalf("retired transition metadata became authoritative: %#v", item)
	}
}

func TestGitHubImplementationStatusRefusesPartialHandoff(t *testing.T) {
	labels := []string{"ready"}
	interrupt := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		ls := []map[string]string{}
		for _, label := range labels {
			ls = append(ls, map[string]string{"name": label})
		}
		source := map[string]any{"number": 7, "title": "widget", "state": "open", "body": "Branch: `widget`\n", "labels": ls}
		pull := map[string]any{"number": 11, "state": "open", "body": "candidate\n\nCloses #7\n", "labels": []map[string]string{{"name": "review"}}, "head": map[string]any{"ref": "widget", "sha": "fixed", "repo": map[string]string{"full_name": "acme/widgets"}}}
		var result any = []any{}
		switch {
		case path == "/issues" && r.Method == http.MethodGet:
			result = []any{source}
		case path == "/pulls" && r.Method == http.MethodGet:
			result = []any{pull}
		case path == "/issues/11" && r.Method == http.MethodGet:
			result = pull
		case path == "/issues/7" && r.Method == http.MethodGet:
			result = source
		case path == "/issues/7/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			json.NewDecoder(r.Body).Decode(&payload)
			labels = append(labels, payload.Labels...)
		case strings.HasPrefix(path, "/issues/7/labels/") && r.Method == http.MethodDelete:
			if interrupt {
				http.Error(w, "interrupted source cleanup", http.StatusBadRequest)
				return
			}
			label := strings.TrimPrefix(path, "/issues/7/labels/")
			labels = slices.DeleteFunc(labels, func(value string) bool { return value == label })
		case r.Method == http.MethodGet && (strings.HasSuffix(path, "/comments") || strings.HasSuffix(path, "/reviews") || strings.HasSuffix(path, "/dependencies/blocked_by")):
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	b := boundGitHubBackend(NewGitHubBackend(server.URL, "token", server.Client()))
	_, err := workflow.ObserveStatus(context.Background(), b)
	if err == nil || !strings.Contains(err.Error(), "original Result Document") || !slices.Equal(labels, []string{"ready"}) {
		t.Fatalf("status mutated an ambiguous partial handoff: %v, labels=%v", err, labels)
	}
}
