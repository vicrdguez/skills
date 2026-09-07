package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/workflow"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestGitHubBackendMapsRepositoryAndLabels(t *testing.T) {
	labels := map[string]Label{
		"ready":  {Name: "ready", Color: "ffffff", Description: "stale"},
		"custom": {Name: "custom", Color: "123456", Description: "unrelated"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/widgets":
			_ = json.NewEncoder(response).Encode(map[string]string{"default_branch": "trunk"})
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/widgets/labels":
			values := make([]Label, 0, len(labels))
			for _, label := range labels {
				values = append(values, label)
			}
			_ = json.NewEncoder(response).Encode(values)
		case request.Method == http.MethodPatch:
			var label Label
			_ = json.NewDecoder(request.Body).Decode(&label)
			label.Name = strings.TrimPrefix(request.URL.Path, "/repos/acme/widgets/labels/")
			labels[label.Name] = label
			response.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/widgets/labels":
			var label Label
			_ = json.NewDecoder(request.Body).Decode(&label)
			labels[label.Name] = label
			response.WriteHeader(http.StatusCreated)
		default:
			http.Error(response, "unexpected request", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	backend := NewGitHubBackend(server.URL, "secret", server.Client())
	repository := RepositoryID{Owner: "acme", Name: "widgets"}

	branch, err := backend.Validate(context.Background(), repository)
	if err != nil || branch != "trunk" {
		t.Fatalf("Validate() = %q, %v", branch, err)
	}
	if err := backend.EnsureLabels(context.Background(), repository, WorkflowLabels); err != nil {
		t.Fatal(err)
	}
	for _, want := range WorkflowLabels {
		if got := labels[want.Name]; got != want {
			t.Fatalf("label %q = %#v", want.Name, got)
		}
	}
	if got := labels["custom"]; got.Description != "unrelated" {
		t.Fatalf("unrelated label changed: %#v", got)
	}
}

func TestGitHubTokenChain(t *testing.T) {
	tests := []struct {
		name        string
		environment map[string]string
		ghToken     string
		want        string
	}{
		{name: "GH_TOKEN first", environment: map[string]string{"GH_TOKEN": "gh", "GITHUB_TOKEN": "github"}, ghToken: "cli", want: "gh"},
		{name: "GITHUB_TOKEN second", environment: map[string]string{"GITHUB_TOKEN": "github"}, ghToken: "cli", want: "github"},
		{name: "gh auth fallback", environment: map[string]string{}, ghToken: "cli", want: "cli"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			getenv := func(name string) string { return test.environment[name] }
			got, err := resolveGitHubToken(getenv, func() (string, error) { return test.ghToken, nil })
			if err != nil || got != test.want {
				t.Fatalf("resolveGitHubToken() = %q, %v", got, err)
			}
		})
	}
}

func TestGitHubBackendDefersAuthenticationUntilValidation(t *testing.T) {
	resolved := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); got != "Bearer delayed" {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"default_branch":"main"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	backend := newGitHubBackend("https://api.github.test", client, func() (string, error) {
		resolved++
		return "delayed", nil
	})
	if resolved != 0 {
		t.Fatal("authentication resolved during backend construction")
	}
	if _, err := backend.Validate(context.Background(), RepositoryID{Owner: "acme", Name: "widgets"}); err != nil {
		t.Fatal(err)
	}
	if resolved != 1 {
		t.Fatalf("authentication resolved %d times", resolved)
	}
}

func TestGitHubBackendPublishesSuppliedMarkdownWithoutInterpretation(t *testing.T) {
	wantBody := "---\nBlocked by: #not-metadata\n[broken markdown\n"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/repos/acme/widgets/issues" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["title"] != "opaque-slice" || payload["body"] != wantBody {
			t.Fatalf("payload = %#v", payload)
		}
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(bytes.NewBufferString(`{"id":501,"number":17,"title":"opaque-slice","body":"---\nBlocked by: #not-metadata\n[broken markdown\n"}`)), Header: make(http.Header)}, nil
	})}
	backend := NewGitHubBackend("https://api.github.test", "secret", client)

	item, err := backend.CreateWorkItem(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"}, workflow.WorkItem{Title: "opaque-slice", Body: wantBody})
	if err != nil {
		t.Fatal(err)
	}
	if item.Number != 17 || item.Title != "opaque-slice" || item.Body != wantBody {
		t.Fatalf("item = %#v", item)
	}
}

func TestGitHubBackendMapsNativeProposalRelationships(t *testing.T) {
	var relationships []string
	created := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost && request.URL.Path == "/repos/acme/widgets/issues" {
			created++
			return jsonResponse(http.StatusCreated, fmt.Sprintf(`{"id":%d,"number":%d}`, 1000+created, 100+created)), nil
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		relationships = append(relationships, fmt.Sprintf("%s %s %#v", request.Method, request.URL.Path, payload))
		return jsonResponse(http.StatusCreated, `{}`), nil
	})}
	backend := NewGitHubBackend("https://api.github.test", "secret", client)
	repository := workflow.RepositoryID{Owner: "acme", Name: "widgets"}
	parent, err := backend.CreateCoordinationItem(context.Background(), repository, workflow.CoordinationItem{Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := backend.CreateWorkItem(context.Background(), repository, workflow.WorkItem{Title: "blocker"})
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := backend.CreateWorkItem(context.Background(), repository, workflow.WorkItem{Title: "dependent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.AddChild(context.Background(), repository, parent.Number, dependent.Number); err != nil {
		t.Fatal(err)
	}
	if err := backend.AddDependency(context.Background(), repository, dependent.Number, blocker.Number); err != nil {
		t.Fatal(err)
	}
	if err := backend.SetReady(context.Background(), repository, dependent.Number); err != nil {
		t.Fatal(err)
	}

	want := []string{
		`POST /repos/acme/widgets/issues/101/sub_issues map[string]interface {}{"sub_issue_id":1003}`,
		`POST /repos/acme/widgets/issues/103/dependencies/blocked_by map[string]interface {}{"issue_id":1002}`,
		`POST /repos/acme/widgets/issues/103/labels map[string]interface {}{"labels":[]interface {}{"ready"}}`,
	}
	if !slices.Equal(relationships, want) {
		t.Fatalf("relationships = %#v", relationships)
	}
}

func TestGitHubBackendNormalizesProposalState(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/repos/acme/widgets/issues":
			return jsonResponse(http.StatusOK, `[{"id":501,"number":17,"title":"slice","body":"body","state":"closed","labels":[{"name":"ready"}]}]`), nil
		case "/repos/acme/widgets/issues/17/parent":
			return jsonResponse(http.StatusOK, `{"id":502,"number":10}`), nil
		case "/repos/acme/widgets/issues/17/dependencies/blocked_by":
			return jsonResponse(http.StatusOK, `[{"id":503,"number":16}]`), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL.Path)
			return nil, nil
		}
	})}
	backend := NewGitHubBackend("https://api.github.test", "secret", client)

	items, err := backend.FindWorkItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"}, []workflow.WorkItem{{Title: "slice"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Ready || items[0].Merged || items[0].Parent != 10 || !slices.Equal(items[0].Blockers, []int{16}) {
		t.Fatalf("items = %#v", items)
	}
}

func TestGitHubBackendReconcilesDeclaredFallback(t *testing.T) {
	for _, test := range []struct {
		name, supplied, persisted string
		blockers                  []int
	}{
		{"declared fallback", "opaque\n", "opaque\n\nBlocked by: #1\n", []int{1}},
		{"authored lookalike", "opaque\n\nBlocked by: #1\n", "opaque\n\nBlocked by: #1\n", nil},
		{"conflicting prose", "opaque\n", "changed\n\nBlocked by: #1\n", nil},
		{"undeclared suffix", "opaque\n", "opaque\n\nBlocked by: #99\n", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet {
					t.Fatalf("normalization mutated backend: %s", request.Method)
				}
				switch {
				case request.URL.Path == "/repos/acme/widgets/issues":
					return jsonResponse(http.StatusOK, fmt.Sprintf(`[{"number":1,"title":"base","body":"base"},{"number":2,"title":"dependent","body":%q}]`, test.persisted)), nil
				case strings.HasSuffix(request.URL.Path, "/parent"), strings.HasSuffix(request.URL.Path, "/dependencies/blocked_by"):
					return jsonResponse(http.StatusNotFound, `{}`), nil
				default:
					t.Fatalf("unexpected request: %s", request.URL)
					return nil, nil
				}
			})}
			backend := NewGitHubBackend("https://api.github.test", "secret", client)
			items, err := backend.FindWorkItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"},
				[]workflow.WorkItem{{Title: "base", Body: "base"}, {Title: "dependent", Body: test.supplied}},
				[]workflow.Dependency{{Dependent: "dependent", Blocker: "base"}})
			wantBody := test.persisted
			if len(test.blockers) > 0 {
				wantBody = test.supplied
			}
			if err != nil || len(items) != 2 || items[1].Body != wantBody || !slices.Equal(items[1].Blockers, test.blockers) {
				t.Fatalf("normalized records = %#v, %v; want body %q blockers %v", items, err, wantBody, test.blockers)
			}
		})
	}
}

func TestGitHubBackendReportsOnlyCommitClosedWorkflowItemsAsMerged(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/repos/acme/widgets/issues":
			return jsonResponse(http.StatusOK, `[
				{"id":501,"number":17,"title":"merged","state":"closed","labels":[]},
				{"id":502,"number":18,"title":"manual","state":"closed","labels":[{"name":"done"}]},
				{"id":503,"number":19,"title":"unrelated","state":"closed","labels":[]}
			]`), nil
		case "/repos/acme/widgets/issues/17/timeline":
			return jsonResponse(http.StatusOK, `[{"event":"labeled","label":{"name":"ready"}},{"event":"labeled","label":{"name":"wip"}},{"event":"unlabeled","label":{"name":"ready"}},{"event":"unlabeled","label":{"name":"wip"}},{"event":"closed","commit_id":"abc123"}]`), nil
		case "/repos/acme/widgets/issues/18/timeline":
			return jsonResponse(http.StatusOK, `[{"event":"closed","commit_id":null}]`), nil
		case "/repos/acme/widgets/issues/19/timeline":
			return jsonResponse(http.StatusOK, `[{"event":"closed","commit_id":"def456"}]`), nil
		case "/repos/acme/widgets/commits/abc123/pulls":
			return jsonResponse(http.StatusOK, `[]`), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL.Path)
			return nil, nil
		}
	})}
	backend := NewGitHubBackend("https://api.github.test", "secret", client)

	items, err := backend.ListMergedWorkItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "merged" || !items[0].Merged {
		t.Fatalf("items = %#v", items)
	}
}

func TestGitHubBackendReadsMergedLifecycleAcrossTimelinePages(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/repos/acme/widgets/issues":
			return jsonResponse(http.StatusOK, `[{"number":17,"title":"merged","state":"closed","labels":[]}]`), nil
		case "/repos/acme/widgets/commits/abc123/pulls":
			return jsonResponse(http.StatusOK, `[{"merged_at":"2026-09-02T16:57:13Z","merge_commit_sha":"abc123","head":{"ref":"merged","sha":"accepted"}}]`), nil
		case "/repos/acme/widgets/issues/17/timeline":
			if request.URL.Query().Get("page") == "2" {
				return jsonResponse(http.StatusOK, `[{"event":"referenced","commit_id":"abc123"}]`), nil
			}
			return jsonResponse(http.StatusOK, `[{"event":"labeled","label":{"name":"ready"}}`+strings.Repeat(`,{"event":"commented"}`, 97)+`,{"event":"unlabeled","label":{"name":"ready"}},{"event":"closed","commit_id":null}]`), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL)
			return nil, nil
		}
	})}
	backend := NewGitHubBackend("https://api.github.test", "secret", client)
	items, err := backend.ListMergedWorkItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"})
	if err != nil || len(items) != 1 || items[0].Number != 17 || !items[0].Merged || items[0].AcceptedHead != "accepted" {
		t.Fatalf("Merged lifecycle = %#v, %v", items, err)
	}
}

func TestGitHubBackendIdentifiesAcceptedSubmissionHead(t *testing.T) {
	accepted := `{"merged_at":"2026-09-07T10:00:00Z","merge_commit_sha":"squash","head":{"ref":"slice","sha":"accepted"}}`
	for _, test := range []struct{ name, pulls, want string }{
		{"accepted squash", "[" + accepted + "]", "accepted"},
		{"unknown", `[]`, ""},
		{"ambiguous", "[" + accepted + "," + accepted + "]", ""},
		{"unmerged", `[{"merge_commit_sha":"squash","head":{"ref":"slice","sha":"unaccepted"}}]`, ""},
		{"other branch", `[{"merged_at":"2026-09-07T10:00:00Z","merge_commit_sha":"squash","head":{"ref":"other","sha":"unaccepted"}}]`, ""},
		{"other merge", `[{"merged_at":"2026-09-07T10:00:00Z","merge_commit_sha":"other","head":{"ref":"slice","sha":"unaccepted"}}]`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/repos/acme/widgets/issues":
					return jsonResponse(http.StatusOK, `[{"number":17,"title":"slice","state":"closed","labels":[{"name":"ready"}]}]`), nil
				case "/repos/acme/widgets/issues/17/timeline":
					return jsonResponse(http.StatusOK, `[{"event":"closed","commit_id":"squash"}]`), nil
				case "/repos/acme/widgets/commits/squash/pulls":
					return jsonResponse(http.StatusOK, test.pulls), nil
				default:
					t.Fatalf("unexpected request: %s", request.URL)
					return nil, nil
				}
			})}
			backend := NewGitHubBackend("https://api.github.test", "secret", client)
			items, err := backend.ListMergedWorkItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"})
			if err != nil || len(items) != 1 || items[0].AcceptedHead != test.want {
				t.Fatalf("accepted Submission = %#v, %v; want %q", items, err, test.want)
			}
		})
	}
}

func TestGitHubBackendRecognizesCloseBeforeMerge(t *testing.T) {
	accepted := `{"merged_at":"2026-09-02T16:57:13Z","merge_commit_sha":"squash","head":{"ref":"slice","sha":"accepted"}}`
	for _, test := range []struct {
		name, pulls, wantHead string
		wantItems             int
	}{
		{"merged after handoff", "[" + accepted + "]", "accepted", 1},
		{"merely closed", `[]`, "", 0},
		{"unmerged submission", `[{"merge_commit_sha":"squash","head":{"ref":"slice","sha":"accepted"}}]`, "", 0},
		{"unrelated submission", `[{"merged_at":"2026-09-02T16:57:13Z","merge_commit_sha":"squash","head":{"ref":"other","sha":"accepted"}}]`, "", 0},
		{"ambiguous submissions", "[" + accepted + "," + accepted + "]", "", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet {
					t.Fatalf("cleanup discovery mutated GitHub: %s", request.Method)
				}
				switch request.URL.Path {
				case "/repos/acme/widgets/issues":
					return jsonResponse(http.StatusOK, `[{"number":17,"title":"slice","state":"closed","labels":[]}]`), nil
				case "/repos/acme/widgets/issues/17/timeline":
					return jsonResponse(http.StatusOK, `[
						{"event":"labeled","label":{"name":"ready"}},
						{"event":"labeled","label":{"name":"wip"}},
						{"event":"referenced","commit_id":"implementation"},
						{"event":"cross-referenced","source":{"issue":{"number":9,"pull_request":{}}}},
						{"event":"unlabeled","label":{"name":"ready"}},
						{"event":"unlabeled","label":{"name":"wip"}},
						{"event":"closed","commit_id":null,"created_at":"2026-09-02T16:44:42Z"},
						{"event":"referenced","commit_id":"squash","created_at":"2026-09-02T16:57:14Z"},
						{"event":"referenced","commit_id":"squash","created_at":"2026-09-02T16:57:15Z"}
					]`), nil
				case "/repos/acme/widgets/commits/squash/pulls":
					return jsonResponse(http.StatusOK, test.pulls), nil
				default:
					t.Fatalf("unexpected request: %s", request.URL)
					return nil, nil
				}
			})}
			backend := NewGitHubBackend("https://api.github.test", "secret", client)
			items, err := backend.ListMergedWorkItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"})
			if err != nil || len(items) != test.wantItems {
				t.Fatalf("Merged Work Items = %#v, %v; want %d", items, err, test.wantItems)
			}
			if len(items) == 1 && (!items[0].Merged || items[0].Number != 17 || items[0].Branch != "slice" || items[0].AcceptedHead != test.wantHead) {
				t.Fatalf("accepted Submission = %#v; want head %q", items[0], test.wantHead)
			}
		})
	}
}

func TestGitHubBackendFallsBackWhenNativeDependenciesAreUnavailable(t *testing.T) {
	var patchedBody string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/dependencies/blocked_by") {
			return jsonResponse(http.StatusNotFound, `{"message":"not available"}`), nil
		}
		if request.Method == http.MethodPatch && request.URL.Path == "/repos/acme/widgets/issues/2" {
			var payload map[string]string
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			patchedBody = payload["body"]
			return jsonResponse(http.StatusOK, `{}`), nil
		}
		t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		return nil, nil
	})}
	backend := NewGitHubBackend("https://api.github.test", "secret", client)
	backend.issueIDs[1] = 501
	backend.issueBodies[2] = "opaque body\n"

	if err := backend.AddDependency(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"}, 2, 1); err != nil {
		t.Fatal(err)
	}
	if patchedBody != "opaque body\n\nBlocked by: #1\n" {
		t.Fatalf("body = %q", patchedBody)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
}
