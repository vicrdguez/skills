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

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

func boundGitHubBackend(backend *GitHubBackend) *GitHubBackend {
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	return backend
}

func TestGitHubImplementationNormalizesPaginatedWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graphql":
			fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[]}}}}}`)
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
	}
	for _, id := range []string{"", "0", "-1", "07", "7/labels", "issue:7", "999999999999999999999999"} {
		for _, identity := range []string{"item", "submission"} {
			for name, operation := range operations {
				if identity == "item" && name == "review-submission" || identity == "submission" && name == "close-coordination" {
					continue
				}
				t.Run(name+"/"+identity+"/"+id, func(t *testing.T) {
					item := workflow.ImplementationItem{ID: "7", State: workflow.Rework, Submission: &workflow.Submission{ID: "11"}}
					if identity == "item" {
						item.ID = workflow.WorkItemID(id)
					} else {
						item.Submission.ID = workflow.SubmissionID(id)
					}
					if err := operation(item); err == nil || !strings.Contains(err.Error(), "invalid GitHub issue identity") {
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

func TestGitHubImplementationRejectsForeignAttachmentsAndIgnoresObsoleteTargetMetadata(t *testing.T) {
	for _, kind := range []string{"fork", "untrusted metadata", "conflicting metadata"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/graphql":
					fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[]}}}}}`)
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

func TestGitHubImplementationPreservesPendingLifecycleObservations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[{"number":11,"repository":{"nameWithOwner":"acme/widgets"}}]}}}}}`)
			return
		}
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
		case path == "/graphql":
			fmt.Fprint(w, `{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[{"number":11,"repository":{"nameWithOwner":"acme/widgets"}}]}}}}}`)
			return
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
