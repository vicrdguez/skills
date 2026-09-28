package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vicrdguez/skills/github"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestGitHubBackendMapsRepositoryAndLabels(t *testing.T) {
	mutations := 0
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
			mutations++
			var label Label
			_ = json.NewDecoder(request.Body).Decode(&label)
			label.Name = strings.TrimPrefix(request.URL.Path, "/repos/acme/widgets/labels/")
			labels[label.Name] = label
			response.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPost && request.URL.Path == "/repos/acme/widgets/labels":
			mutations++
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
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})

	branch, err := backend.Validate(context.Background())
	if err != nil || branch != "trunk" {
		t.Fatalf("Validate() = %q, %v", branch, err)
	}
	if err := backend.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	firstMutations := mutations
	if err := backend.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if firstMutations == 0 || mutations != firstMutations {
		t.Fatalf("Prepare mutations = %d then %d", firstMutations, mutations)
	}
	for _, want := range WorkflowLabels {
		if got := labels[want.Name]; got != want {
			t.Fatalf("label %q = %#v", want.Name, got)
		}
	}
	if got := labels["custom"]; got != (Label{Name: "custom", Color: "123456", Description: "unrelated"}) {
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
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	if _, err := backend.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if resolved != 1 {
		t.Fatalf("authentication resolved %d times", resolved)
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

	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	items, err := backend.ListMergedWorkItems(context.Background())
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
			return jsonResponse(http.StatusOK, `[{"body":"Closes #17\n","merged_at":"2026-09-02T16:57:13Z","merge_commit_sha":"abc123","head":{"ref":"merged","sha":"accepted","repo":{"full_name":"acme/widgets"}}}]`), nil
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
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	items, err := backend.ListMergedWorkItems(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != "17" || !items[0].Merged || items[0].AcceptedHead != "accepted" {
		t.Fatalf("Merged lifecycle = %#v, %v", items, err)
	}
}

func TestGitHubBackendIdentifiesAcceptedSubmissionHead(t *testing.T) {
	accepted := `{"body":"Closes #17\n","merged_at":"2026-09-07T10:00:00Z","merge_commit_sha":"squash","head":{"ref":"slice","sha":"accepted","repo":{"full_name":"acme/widgets"}}}`
	for _, test := range []struct{ name, pulls, want string }{
		{"accepted squash", "[" + accepted + "]", "accepted"},
		{"unknown", `[]`, ""},
		{"ambiguous", "[" + accepted + "," + accepted + "]", ""},
		{"unmerged", `[{"body":"Closes #17\n","merge_commit_sha":"squash","head":{"ref":"slice","sha":"unaccepted","repo":{"full_name":"acme/widgets"}}}]`, ""},
		{"renamed branch", `[{"body":"Closes #17\n","merged_at":"2026-09-07T10:00:00Z","merge_commit_sha":"squash","head":{"ref":"other","sha":"unaccepted","repo":{"full_name":"acme/widgets"}}}]`, "unaccepted"},
		{"other merge", `[{"body":"Closes #17\n","merged_at":"2026-09-07T10:00:00Z","merge_commit_sha":"other","head":{"ref":"slice","sha":"unaccepted","repo":{"full_name":"acme/widgets"}}}]`, ""},
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
			backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			items, err := backend.ListMergedWorkItems(context.Background())
			if err != nil || len(items) != 1 || items[0].AcceptedHead != test.want {
				t.Fatalf("accepted Submission = %#v, %v; want %q", items, err, test.want)
			}
		})
	}
}

func TestGitHubBackendRecognizesCloseBeforeMerge(t *testing.T) {
	accepted := `{"body":"Closes #17\n","merged_at":"2026-09-02T16:57:13Z","merge_commit_sha":"squash","head":{"ref":"slice","sha":"accepted","repo":{"full_name":"acme/widgets"}}}`
	for _, test := range []struct {
		name, pulls, wantHead, wantBranch string
		wantItems                         int
	}{
		{"merged after handoff", "[" + accepted + "]", "accepted", "slice", 1},
		{"merely closed", `[]`, "", "", 0},
		{"unmerged submission", `[{"body":"Closes #17\n","merge_commit_sha":"squash","head":{"ref":"slice","sha":"accepted","repo":{"full_name":"acme/widgets"}}}]`, "", "", 0},
		{"renamed submission", `[{"body":"Closes #17\n","merged_at":"2026-09-02T16:57:13Z","merge_commit_sha":"squash","head":{"ref":"other","sha":"accepted","repo":{"full_name":"acme/widgets"}}}]`, "accepted", "other", 1},
		{"ambiguous submissions", "[" + accepted + "," + accepted + "]", "", "", 1},
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
			backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			items, err := backend.ListMergedWorkItems(context.Background())
			if err != nil || len(items) != test.wantItems {
				t.Fatalf("Merged Work Items = %#v, %v; want %d", items, err, test.wantItems)
			}
			if len(items) == 1 && (!items[0].Merged || items[0].ID != "17" || items[0].Branch != test.wantBranch || items[0].AcceptedHead != test.wantHead) {
				t.Fatalf("accepted Submission = %#v; want head %q branch %q", items[0], test.wantHead, test.wantBranch)
			}
		})
	}
}

func TestGitHubBackendRefusesUnboundOperations(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("unbound operation requested %s", request.URL)
		return nil, nil
	})}
	backend := NewGitHubBackend("https://api.github.test", "secret", client)
	ctx := context.Background()
	operations := map[string]func() error{
		"Validate":            func() error { _, err := backend.Validate(ctx); return err },
		"Prepare":             func() error { return backend.Prepare(ctx) },
		"ListMergedWorkItems": func() error { _, err := backend.ListMergedWorkItems(ctx); return err },
		"CreateIssue":         func() error { _, err := backend.CreateIssue(ctx, "slice", "body"); return err },
		"ListChildren":        func() error { _, err := backend.ListChildren(ctx, 1); return err },
		"AttachChild":         func() error { return backend.AttachChild(ctx, 1, 2) },
	}
	for _, repository := range []github.RepositoryID{{}, {Owner: "acme"}, {Name: "widgets"}} {
		backend.BindRepository(repository)
		for name, operation := range operations {
			if err := operation(); err == nil || !strings.Contains(err.Error(), "not bound") {
				t.Errorf("%s with %#v = %v; want unbound refusal", name, repository, err)
			}
		}
	}
}

func TestGitHubBackendRebindingDiscardsRepositoryCaches(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/repos/acme/widgets/issues":
			return jsonResponse(http.StatusCreated, `{"id":501,"number":1}`), nil
		case "/repos/acme/other/issues/1":
			return jsonResponse(http.StatusNotFound, `{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
			return nil, nil
		}
	})}
	backend := NewGitHubBackend("https://api.github.test", "secret", client)
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	ctx := context.Background()
	if _, err := backend.CreateIssue(ctx, "slice", "body"); err != nil {
		t.Fatal(err)
	}
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "other"})
	if len(backend.issueIDs) != 0 {
		t.Fatal("rebinding retained repository-specific caches")
	}
	if err := backend.AttachChild(ctx, 2, 1); err == nil {
		t.Fatal("relationship reused an issue ID from another repository")
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
}

func TestGitHubIssuePublicationRetriesOnlyRepeatableRequests(t *testing.T) {
	attempts := map[string]int{}
	failures := map[string]error{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		key := request.Method + " " + request.URL.Path
		attempts[key]++
		if err := failures[key]; err != nil && attempts[key] < 3 {
			return nil, err
		}
		switch key {
		case "GET /repos/acme/widgets/issues/7/sub_issues":
			if attempts[key] < 3 {
				return jsonResponse(http.StatusBadGateway, `{}`), nil
			}
			return jsonResponse(http.StatusOK, `[{"number":8}]`), nil
		case "PATCH /repos/acme/widgets/issues/8":
			var payload map[string]string
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload["title"] != "Current title" || payload["body"] != "current prose\n" {
				t.Fatalf("update payload = %v (%v)", payload, err)
			}
			return jsonResponse(http.StatusOK, `{}`), nil
		case "POST /repos/acme/widgets/issues":
			return jsonResponse(http.StatusBadGateway, `{}`), nil
		case "PATCH /repos/acme/widgets/issues/9":
			return jsonResponse(http.StatusServiceUnavailable, `{}`), nil
		}
		t.Fatalf("unexpected request %s", key)
		return nil, nil
	})}
	failures["PATCH /repos/acme/widgets/issues/8"] = fmt.Errorf("connection reset")
	backend := boundGitHubBackend(NewGitHubBackend("https://api.github.test", "secret", client))
	backend.retryDelay = 0
	ctx := context.Background()

	children, err := backend.ListChildren(ctx, 7)
	if err != nil || !slices.Equal(children, []int{8}) || attempts["GET /repos/acme/widgets/issues/7/sub_issues"] != 3 {
		t.Fatalf("read was not retried to success: %v %v %v", children, err, attempts)
	}
	if err := backend.UpdateIssue(ctx, 8, "Current title", "current prose\n", nil); err != nil || attempts["PATCH /repos/acme/widgets/issues/8"] != 3 {
		t.Fatalf("repeatable update was not retried after transport failures: %v %v", err, attempts)
	}
	if err := backend.UpdateIssue(ctx, 9, "Title", "prose\n", nil); err == nil || attempts["PATCH /repos/acme/widgets/issues/9"] != requestAttempts {
		t.Fatalf("update retries were not bounded: %v %v", err, attempts)
	}
	// A selection superseded after the first failed attempt stops the retries
	// before another update is sent, and its reason reaches the caller.
	superseded := errors.New("selection superseded")
	checks := 0
	proceed := func() error {
		if checks++; checks > 1 {
			return superseded
		}
		return nil
	}
	if err := backend.UpdateIssue(ctx, 9, "Title", "prose\n", proceed); !errors.Is(err, superseded) || attempts["PATCH /repos/acme/widgets/issues/9"] != requestAttempts+1 {
		t.Fatalf("update was retried after its selection was superseded: %v %v", err, attempts)
	}
	if _, err := backend.CreateIssue(ctx, "Title", "prose\n"); err == nil || attempts["POST /repos/acme/widgets/issues"] != 1 {
		t.Fatalf("create was retried: %v %v", err, attempts)
	}
	failures["POST /repos/acme/widgets/issues"] = fmt.Errorf("connection reset")
	_, err = backend.CreateIssue(ctx, "Title", "prose\n")
	var unknown interface{ UnknownOutcome() bool }
	if !errors.As(err, &unknown) || !unknown.UnknownOutcome() || attempts["POST /repos/acme/widgets/issues"] != 2 {
		t.Fatalf("uncertain create was not reported once as an unknown outcome: %v %v", err, attempts)
	}
}

func TestGitHubIssuePublicationBoundsUnansweredRequests(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	backend := boundGitHubBackend(NewGitHubBackend(server.URL, "secret", server.Client()))
	backend.timeout, backend.retryDelay = 50*time.Millisecond, 0
	ctx := context.Background()

	started := time.Now()
	_, err := backend.CreateIssue(ctx, "Title", "prose\n")
	var unknown interface{ UnknownOutcome() bool }
	if !errors.As(err, &unknown) || !unknown.UnknownOutcome() {
		t.Fatalf("unanswered create was not an unknown outcome: %v", err)
	}
	if err := backend.UpdateIssue(ctx, 8, "Title", "prose\n", nil); err == nil {
		t.Fatal("unanswered update reported success")
	}
	// One create plus a bounded number of update attempts, each timing out.
	if elapsed := time.Since(started); elapsed > time.Duration(1+requestAttempts)*50*time.Millisecond+2*time.Second {
		t.Fatalf("unanswered requests were not bounded: %v", elapsed)
	}
}
