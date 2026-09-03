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

	items, err := backend.FindWorkItems(context.Background(), workflow.RepositoryID{Owner: "acme", Name: "widgets"}, []string{"slice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Ready || items[0].Merged || items[0].Parent != 10 || !slices.Equal(items[0].Blockers, []int{16}) {
		t.Fatalf("items = %#v", items)
	}
}

func TestGitHubBackendReportsOnlyCommitClosedWorkflowItemsAsMerged(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/repos/acme/widgets/issues":
			return jsonResponse(http.StatusOK, `[
				{"id":501,"number":17,"title":"merged","state":"closed","labels":[{"name":"done"}]},
				{"id":502,"number":18,"title":"manual","state":"closed","labels":[{"name":"done"}]},
				{"id":503,"number":19,"title":"unrelated","state":"closed","labels":[]}
			]`), nil
		case "/repos/acme/widgets/issues/17/timeline":
			return jsonResponse(http.StatusOK, `[{"event":"closed","commit_id":"abc123"}]`), nil
		case "/repos/acme/widgets/issues/18/timeline":
			return jsonResponse(http.StatusOK, `[{"event":"closed","commit_id":null}]`), nil
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
	if patchedBody != "opaque body\n\nBlocked by #1\n" {
		t.Fatalf("body = %q", patchedBody)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
}
