package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

// selectionGraphQL answers the candidate-queue and owning-link queries from the
// same REST fixture records. It reports whether it handled the request.
func selectionGraphQL(w http.ResponseWriter, r *http.Request, pull map[string]any, present bool) bool {
	if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
		return false
	}
	var request struct {
		Query string `json:"query"`
	}
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(payload))
	if err := json.Unmarshal(payload, &request); err != nil {
		return false
	}
	data := func(value map[string]any) { _ = json.NewEncoder(w).Encode(map[string]any{"data": value}) }
	switch {
	case strings.Contains(request.Query, "pullRequests("):
		nodes := []any{}
		if present {
			head, _ := pull["head"].(map[string]any)
			labels := []any{}
			if raw, ok := pull["labels"].([]map[string]string); ok {
				for _, label := range raw {
					labels = append(labels, map[string]any{"name": label["name"]})
				}
			}
			created, _ := pull["created_at"].(string)
			draft, _ := pull["draft"].(bool)
			nodes = append(nodes, map[string]any{"number": pull["number"], "createdAt": created, "headRefName": head["ref"], "headRefOid": head["sha"], "isDraft": draft, "labels": map[string]any{"nodes": labels}})
		}
		data(map[string]any{"repository": map[string]any{"pullRequests": map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}})
		return true
	case strings.Contains(request.Query, "closedByPullRequestsReferences"):
		nodes := []any{}
		if present {
			nodes = append(nodes, map[string]any{"number": pull["number"], "merged": false, "mergedAt": "", "state": "OPEN", "repository": map[string]string{"nameWithOwner": "acme/widgets"}})
		}
		data(map[string]any{"repository": map[string]any{"issue": map[string]any{"closedByPullRequestsReferences": map[string]any{"nodes": nodes}}}})
		return true
	}
	return false
}

func TestStatusRefusesRetargetedPartialHandoffThroughGitHub(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	sourceLabels := []string{"ready", "wip"}
	pullLabels := []string{"review"}
	metadata := []map[string]any{{"body": fmt.Sprintf("<!-- skl.implement/v1\n{\"watchdog_head\":%q}\n-->", head), "author_association": "OWNER"}}
	writes := 0
	unwanted := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		toLabels := func(values []string) []map[string]string {
			labels := make([]map[string]string, 0, len(values))
			for _, value := range values {
				labels = append(labels, map[string]string{"name": value})
			}
			return labels
		}
		source := map[string]any{"number": 7, "title": "widget", "state": "open", "labels": toLabels(sourceLabels), "body": "Branch: `widget`\n"}
		pull := map[string]any{"number": 11, "state": "open", "body": "existing\n\nCloses #7\n", "labels": toLabels(pullLabels), "head": map[string]any{"ref": "widget", "sha": head, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "release"}}
		if selectionGraphQL(w, r, pull, true) {
			return
		}
		if r.Method != http.MethodGet {
			writes++
		}
		if strings.HasPrefix(path, "/issues/7/labels") || strings.HasPrefix(path, "/issues/11/labels") {
			target, prefix := &sourceLabels, "/issues/7/labels/"
			if strings.HasPrefix(path, "/issues/11/") {
				target, prefix = &pullLabels, "/issues/11/labels/"
			}
			switch r.Method {
			case http.MethodPost:
				var payload struct{ Labels []string }
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				for _, label := range payload.Labels {
					if !slices.Contains(*target, label) {
						*target = append(*target, label)
					}
				}
			case http.MethodDelete:
				label := strings.TrimPrefix(path, prefix)
				*target = slices.DeleteFunc(*target, func(value string) bool { return value == label })
			}
			json.NewEncoder(w).Encode([]any{})
			return
		}
		var result any = []any{}
		switch path {
		case "/issues":
			result = []any{source}
		case "/pulls":
			result = []any{pull}
		case "/issues/7":
			result = source
		case "/issues/7/comments":
		case "/issues/7/timeline":
			result = []any{map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
			result = metadata
		case "/issues/7/dependencies/blocked_by", "/issues/11/comments", "/pulls/11/comments", "/pulls/11/reviews", "/issues/11/timeline":
		case "/pulls/11", "/issues/11":
			result = pull
		default:
			unwanted = r.Method + " " + path
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
		backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "status", "--repo", root}); err != nil {
		t.Fatal(err)
	}
	var result workflow.ImplementationOutcome
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "fix_required" || !strings.Contains(result.Reason, "release") || !strings.Contains(result.Reason, "main") || writes != 0 || !slices.Contains(sourceLabels, "wip") || len(pullLabels) != 1 || pullLabels[0] != "review" || unwanted != "" {
		t.Fatalf("retargeted partial handoff: %#v writes=%d source=%v pull=%v unwanted=%q", result, writes, sourceLabels, pullLabels, unwanted)
	}
}
