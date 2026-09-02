package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
