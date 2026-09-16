package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func prepareReviewWorktree(t *testing.T, root, branch string) {
	t.Helper()
	runGit(t, root, "switch", "main")
	worktree := filepath.Join(root, ".worktrees", branch)
	if _, err := os.Stat(worktree); os.IsNotExist(err) {
		runGit(t, root, "worktree", "add", worktree, branch)
	}
}

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
			nodes = append(nodes, map[string]any{"number": pull["number"], "merged": false, "mergedAt": "", "state": "OPEN"})
		}
		data(map[string]any{"repository": map[string]any{"issue": map[string]any{"closedByPullRequestsReferences": map[string]any{"nodes": nodes}}}})
		return true
	}
	return false
}

func TestReadyStartAndResumeDoNotObserveIntegrationTargetThroughGitHub(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	labels := []string{"ready"}
	branchHead := baseline
	mainHead := strings.Repeat("a", 40)
	obsoleteTarget := strings.Repeat("c", 40)
	pullExists := false
	postedBase := ""
	prBody := ""
	var prLabels []string
	metadata := []map[string]any{{"body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + obsoleteTarget + "\",\"target_branch\":\"release\",\"synchronization_target\":\"obsolete\"}\n-->", "author_association": "OWNER"}}
	unwanted := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		issueLabels := make([]map[string]string, 0, len(labels))
		for _, label := range labels {
			issueLabels = append(issueLabels, map[string]string{"name": label})
		}
		issue := map[string]any{"number": 7, "title": "widget", "state": "open", "labels": issueLabels, "body": "Branch: `widget`\n"}
		pullLabelObjects := make([]map[string]string, 0, len(prLabels))
		for _, label := range prLabels {
			pullLabelObjects = append(pullLabelObjects, map[string]string{"name": label})
		}
		pull := map[string]any{"number": 11, "state": "open", "body": prBody, "labels": pullLabelObjects, "head": map[string]any{"ref": "widget", "sha": branchHead, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": postedBase}}
		var result any = []any{}
		switch {
		case path == "/graphql":
			var payload struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.Contains(payload.Query, "pullRequests(") {
				result = map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequests": map[string]any{"nodes": []any{}, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}}}
			} else {
				result = map[string]any{"data": map[string]any{"repository": map[string]any{"issue": map[string]any{"closedByPullRequestsReferences": map[string]any{"nodes": []any{}}}}}}
			}
		case path == "/issues":
			result = []any{issue}
		case path == "/pulls":
			if r.Method == http.MethodPost {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				postedBase, _ = payload["base"].(string)
				prBody, _ = payload["body"].(string)
				pullExists = true
				pull["base"] = map[string]string{"ref": postedBase}
				pull["body"] = prBody
				result = pull
			} else if pullExists {
				result = []any{pull}
			}
		case path == "/issues/7/comments":
			if r.Method == http.MethodPost {
				var comment map[string]any
				if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				comment["author_association"] = "OWNER"
				comment["created_at"] = "2026-01-01T00:00:02Z"
				metadata = append(metadata, comment)
			}
			result = metadata
		case path == "/issues/7/timeline":
			result = []any{map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
		case path == "/issues/11/timeline":
			result = []any{map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
		case path == "/issues/7/dependencies/blocked_by", path == "/issues/11/comments", path == "/pulls/11/comments", path == "/pulls/11/reviews":
		case path == "/issues/7":
			result = issue
		case path == "/issues/7/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for _, label := range payload.Labels {
				if !slices.Contains(labels, label) {
					labels = append(labels, label)
				}
			}
		case path == "/git/ref/heads/widget":
			result = map[string]any{"object": map[string]string{"sha": branchHead}}
		case path == "/pulls/11", path == "/issues/11":
			result = pull
		case path == "/issues/11/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for _, label := range payload.Labels {
				if !slices.Contains(prLabels, label) {
					prLabels = append(prLabels, label)
				}
			}
		case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
			label := strings.TrimPrefix(path, "/issues/11/labels/")
			prLabels = slices.DeleteFunc(prLabels, func(value string) bool { return value == label })
		case strings.HasPrefix(path, "/issues/7/labels/") && r.Method == http.MethodDelete:
			label := strings.TrimPrefix(path, "/issues/7/labels/")
			labels = slices.DeleteFunc(labels, func(value string) bool { return value == label })
		case path == "/git/ref/heads/main" || strings.Contains(path, "target"):
			unwanted = r.Method + " " + path
			http.Error(w, "integration target unavailable", http.StatusInternalServerError)
			return
		default:
			unwanted = r.Method + " " + path
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	run := func(args ...string) setup.ImplementationOutput {
		t.Helper()
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) {
			backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
			backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			return backend, nil
		}, bytes.NewReader(nil), &output, &output)
		command := append([]string{"skl", "implement"}, args...)
		command = append(command, "--repo", root)
		if err := app.Run(command); err != nil {
			t.Fatalf("%v: %v\n%s", command, err, &output)
		}
		var result setup.ImplementationOutput
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Packet != nil {
			t.Cleanup(func() { os.RemoveAll(result.Packet.Facts.Implementation.ResultDirectory) })
		}
		return result
	}
	first := run("next")
	if exec.Command("git", "-C", root, "cat-file", "-e", mainHead+"^{commit}").Run() == nil || exec.Command("git", "-C", root, "cat-file", "-e", obsoleteTarget+"^{commit}").Run() == nil || first.Status != "work_available" || first.Packet == nil || first.Packet.Facts.Implementation.InspectCommand == "" || unwanted != "" {
		t.Fatalf("target-free start: %#v unwanted=%q", first, unwanted)
	}
	if err := os.WriteFile(filepath.Join(root, "progress.txt"), []byte("preserved\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "progress.txt")
	runGit(t, root, "commit", "-m", "implementation progress")
	mainHead = strings.Repeat("b", 40)
	second := run("resume", "--item", "7")
	if exec.Command("git", "-C", root, "cat-file", "-e", mainHead+"^{commit}").Run() == nil || second.Status != "work_available" || unwanted != "" || readFile(t, filepath.Join(root, "progress.txt")) != "preserved\n" {
		t.Fatalf("target-free resume: %#v unwanted=%q", second, unwanted)
	}
	result := filepath.Join(second.Packet.Facts.Implementation.ResultDirectory, "submission.md")
	if err := os.WriteFile(result, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	completeAndRetireSlice(t, root, "widget")
	branchHead = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	submitted := run("submit", "--item", "7", "--body", result)
	if submitted.Status != "awaiting_review" || postedBase != "main" || prBody != "candidate\n\nCloses #7\n" || !pullExists || !slices.Contains(prLabels, "review") || slices.Contains(labels, "ready") || unwanted != "" {
		t.Fatalf("target-free submission: %#v base=%q labels=%v/%v unwanted=%q", submitted, postedBase, labels, prLabels, unwanted)
	}
}

func TestStaleSynchronizationReworkUsesOrdinaryCLIFlow(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	prepareReviewWorktree(t, root, "widget")
	worktreeGitDir := strings.TrimSpace(runGitOutput(t, filepath.Join(root, ".worktrees", "widget"), "rev-parse", "--absolute-git-dir"))
	checkpoint := filepath.Join(worktreeGitDir, ".watchdog")
	if err := os.WriteFile(checkpoint, []byte("1:"+head+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	labels := []string{"rework", "sync"}
	body := "existing\n\nCloses #7\n"
	metadata := []map[string]any{{"body": fmt.Sprintf("<!-- skl.implement/v1\n{\"reviewed_head\":%q,\"review_round_head\":%q,\"target_snapshot\":\"missing\",\"target_branch\":\"release\",\"synchronization_target\":\"conflicting\"}\n-->", head, head), "author_association": "OWNER"}}
	reviewComments := []map[string]any{{"body": "retained review feedback", "author_association": "OWNER"}}
	events := []map[string]any{
		{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "review"}},
		{"event": "labeled", "created_at": "2026-01-01T00:00:02Z", "label": map[string]string{"name": "wip"}},
		{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "rework"}},
		{"event": "unlabeled", "label": map[string]string{"name": "review"}},
		{"event": "unlabeled", "label": map[string]string{"name": "wip"}},
	}
	unwanted := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		pullLabels := make([]map[string]string, 0, len(labels))
		for _, label := range labels {
			pullLabels = append(pullLabels, map[string]string{"name": label})
		}
		pull := map[string]any{"number": 11, "node_id": "PR_11", "state": "open", "created_at": "2026-01-01T00:00:00Z", "body": body, "labels": pullLabels, "head": map[string]any{"sha": head, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
		if selectionGraphQL(w, r, pull, true) {
			return
		}
		var result any = []any{}
		switch {
		case path == "/issues":
			result = []any{map[string]any{"number": 7, "title": "widget", "state": "open", "body": "Branch: `widget`\n"}}
		case path == "/pulls":
			result = []any{pull}
		case path == "/issues/7/comments":
			if r.Method == http.MethodPost {
				var comment map[string]any
				if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				comment["author_association"] = "OWNER"
				metadata = append(metadata, comment)
			}
			result = metadata
		case path == "/issues/7":
			result = map[string]any{"number": 7, "title": "widget", "state": "open"}
		case path == "/issues/11/comments":
			result = reviewComments
		case path == "/pulls/11/comments", path == "/pulls/11/reviews", path == "/issues/7/dependencies/blocked_by":
		case path == "/issues/7/dependencies/blocked_by":
		case path == "/pulls/11" && r.Method == http.MethodPatch:
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if _, exists := payload["base"]; exists {
				unwanted = "retargeted Submission"
			}
			body = payload["body"]
		case path == "/pulls/11", path == "/issues/11":
			result = pull
		case path == "/issues/11/timeline":
			result = events
		case path == "/graphql":
			var request struct {
				Query string `json:"query"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			if strings.Contains(request.Query, "lastEditedAt") {
				result = map[string]any{"data": map[string]any{"node": map[string]any{"body": body, "createdAt": "2026-01-01T00:00:00Z", "lastEditedAt": ""}}}
				break
			}
			result = map[string]any{"data": map[string]any{}}
		case path == "/git/ref/heads/widget":
			result = map[string]any{"object": map[string]string{"sha": head}}
		case path == "/issues/11/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for _, label := range payload.Labels {
				if !slices.Contains(labels, label) {
					labels = append(labels, label)
					events = append(events, map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:03Z", "label": map[string]string{"name": label}})
				}
			}
		case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
			label := strings.TrimPrefix(path, "/issues/11/labels/")
			labels = slices.DeleteFunc(labels, func(value string) bool { return value == label })
			events = append(events, map[string]any{"event": "unlabeled", "label": map[string]string{"name": label}})
		case strings.Contains(path, "target"), path == "/git/ref/heads/main":
			unwanted = r.Method + " " + path
			http.Error(w, "target lookup forbidden", http.StatusInternalServerError)
			return
		default:
			unwanted = r.Method + " " + path
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	run := func(args ...string) setup.ImplementationOutput {
		t.Helper()
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) {
			backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
			backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			return backend, nil
		}, bytes.NewReader(nil), &output, &output)
		command := append([]string{"skl"}, args...)
		command = append(command, "--repo", root)
		if err := app.Run(command); err != nil {
			t.Fatalf("%v: %v\n%s", command, err, &output)
		}
		var result setup.ImplementationOutput
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Packet != nil && result.Packet.Facts.Implementation != nil {
			t.Cleanup(func() { os.RemoveAll(result.Packet.Facts.Implementation.ResultDirectory) })
		}
		return result
	}

	status := run("status")
	if status.Status != "observed" || status.Item != nil || !slices.Contains(labels, "sync") {
		t.Fatalf("status changed stale sync: %#v labels=%v", status, labels)
	}
	start := run("implement", "next")
	if start.Status != "work_available" || start.Packet == nil || len(start.Packet.Facts.Implementation.Comments) != 1 || start.Packet.Facts.Implementation.Comments[0].Body != "retained review feedback" || !slices.Contains(labels, "sync") || readFile(t, checkpoint) != "1:"+head+"\n" {
		t.Fatalf("start lost review evidence or sync: %#v labels=%v", start, labels)
	}
	resume := run("implement", "resume", "--item", "7")
	if resume.Status != "work_available" || !slices.Contains(labels, "sync") || readFile(t, checkpoint) != "1:"+head+"\n" {
		t.Fatalf("resume lost review evidence or sync: %#v labels=%v", resume, labels)
	}
	resultDir := resume.Packet.Facts.Implementation.ResultDirectory
	result := filepath.Join(resultDir, "submission.md")
	if err := os.WriteFile(result, []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	completed := run("implement", "submit", "--item", "7", "--body", result)
	if completed.Status != "awaiting_review" || slices.Contains(labels, "sync") || !slices.Contains(labels, "review") || unwanted != "" || readFile(t, checkpoint) != "1:"+head+"\n" {
		t.Fatalf("ordinary handoff: %#v labels=%v unwanted=%q", completed, labels, unwanted)
	}
}

func TestWatchdogPassRetryAndStatusIgnoreMergeabilityThroughGitHub(t *testing.T) {
	for _, tc := range []struct {
		name, initial, recover string
	}{
		{"mergeable", "mergeable", ""},
		{"conflicting", "conflicting", ""},
		{"unknown", "unknown", ""},
		{"partial retry", "conflicting", "retry"},
		{"partial status", "unknown", "status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			prepareReviewWorktree(t, root, "widget")
			labels := []string{"review", "wip"}
			prBody := "audit\n\nCloses #7\n"
			mergeability := tc.initial
			failRelease := tc.recover != ""
			metadata := []map[string]any{{"body": fmt.Sprintf("<!-- skl.implement/v1\n{\"watchdog_head\":%q}\n-->", head), "author_association": "OWNER"}}
			var summaries []map[string]any
			events := []map[string]any{}
			for _, label := range labels {
				events = append(events, map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": label}})
			}
			writes := 0
			unwanted := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
				pullLabels := make([]map[string]string, 0, len(labels))
				for _, label := range labels {
					pullLabels = append(pullLabels, map[string]string{"name": label})
				}
				pull := map[string]any{"number": 11, "state": "open", "body": prBody, "labels": pullLabels, "head": map[string]any{"sha": head, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
				if mergeability != "unknown" {
					pull["mergeable"] = mergeability == "mergeable"
				}
				if r.Method != http.MethodGet {
					writes++
				}
				if selectionGraphQL(w, r, pull, true) {
					return
				}
				var result any = []any{}
				switch {
				case path == "/issues":
					result = []any{map[string]any{"number": 7, "title": "widget", "state": "open", "body": "Branch: `widget`\n"}}
				case path == "/pulls":
					result = []any{pull}
				case path == "/issues/7/comments":
					result = metadata
				case path == "/issues/11/comments":
					if r.Method == http.MethodPost {
						var comment map[string]any
						if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						comment["author_association"] = "OWNER"
						summaries = append(summaries, comment)
					}
					result = summaries
				case path == "/pulls/11/reviews":
					if r.Method == http.MethodPost {
						var review map[string]any
						if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						review["author_association"] = "OWNER"
						review["state"] = "COMMENTED"
						review["submitted_at"] = "2026-01-01T00:00:02Z"
						summaries = append(summaries, review)
					}
					result = summaries
				case path == "/pulls/11/comments":
				case path == "/pulls/11" && r.Method == http.MethodPatch:
					var payload map[string]string
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if _, exists := payload["base"]; exists {
						unwanted = "retargeted Submission"
					}
					prBody = payload["body"]
				case path == "/pulls/11", path == "/issues/11":
					result = pull
				case path == "/issues/11/timeline":
					result = events
				case path == "/git/ref/heads/widget":
					result = map[string]any{"object": map[string]string{"sha": head}}
				case path == "/issues/7":
					result = map[string]any{"number": 7, "title": "widget", "state": "open"}
				case path == "/issues/11/labels" && r.Method == http.MethodPost:
					var payload struct{ Labels []string }
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					for _, label := range payload.Labels {
						if !slices.Contains(labels, label) {
							labels = append(labels, label)
							events = append(events, map[string]any{"event": "labeled", "label": map[string]string{"name": label}})
						}
					}
				case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
					label := strings.TrimPrefix(path, "/issues/11/labels/")
					if label == "wip" && failRelease {
						http.Error(w, "injected Claim release failure", http.StatusInternalServerError)
						return
					}
					labels = slices.DeleteFunc(labels, func(value string) bool { return value == label })
					events = append(events, map[string]any{"event": "unlabeled", "label": map[string]string{"name": label}})
				case path == "/git/ref/heads/main" || strings.Contains(path, "target"):
					unwanted = r.Method + " " + path
					http.Error(w, "integration target unavailable", http.StatusInternalServerError)
					return
				default:
					unwanted = r.Method + " " + path
					http.Error(w, "unexpected request", http.StatusNotFound)
					return
				}
				json.NewEncoder(w).Encode(result)
			}))
			defer server.Close()
			run := func(args ...string) ([]byte, error) {
				t.Helper()
				var output bytes.Buffer
				app := newApp(func(github.RepositoryID) (setup.Backend, error) {
					backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
					backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
					return backend, nil
				}, bytes.NewReader(nil), &output, &output)
				command := append([]string{"skl"}, args...)
				command = append(command, "--repo", root)
				err := app.Run(command)
				return slices.Clone(output.Bytes()), err
			}
			mustRun := func(args ...string) []byte {
				t.Helper()
				output, err := run(args...)
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, output)
				}
				return output
			}
			dir := t.TempDir()
			summary, finalBody := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md")
			if err := os.WriteFile(summary, []byte("review summary"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(finalBody, []byte("final body"), 0600); err != nil {
				t.Fatal(err)
			}
			command := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", head, "--verdict", "pass", "--summary", summary, "--body", finalBody}
			if tc.recover != "" {
				if _, err := run(command...); err == nil || !slices.Contains(labels, "done") || !slices.Contains(labels, "wip") {
					t.Fatalf("partial pass was not retained: %v labels=%v", err, labels)
				}
				failRelease = false
				var recovered setup.ImplementationOutput
				if tc.recover == "retry" {
					if err := json.Unmarshal(mustRun(command...), &recovered); err != nil || recovered.Status != "ready_for_merge" {
						t.Fatalf("partial retry: %#v, %v", recovered, err)
					}
				} else {
					var status setup.StatusOutput
					if err := json.Unmarshal(mustRun("status"), &status); err != nil || status.Status != "observed" || len(status.Items) != 1 || status.Items[0].State != workflow.ReadyForMerge || status.Items[0].Claimed {
						t.Fatalf("partial status: %#v, %v", status, err)
					}
				}
				if len(summaries) != 1 || unwanted != "" {
					t.Fatalf("partial recovery duplicated evidence or used target: summaries=%d unwanted=%q", len(summaries), unwanted)
				}
				return
			}
			var pass setup.ImplementationOutput
			if err := json.Unmarshal(mustRun(command...), &pass); err != nil || pass.Status != "ready_for_merge" || pass.Item == nil || pass.Item.Claimed {
				t.Fatalf("%s pass: %#v, %v", tc.initial, pass, err)
			}
			afterPass := writes
			mergeability = "unknown"
			var retry setup.ImplementationOutput
			if err := json.Unmarshal(mustRun(command...), &retry); err != nil || retry.Status != "ready_for_merge" || writes != afterPass || len(summaries) != 1 {
				t.Fatalf("unknown retry: %#v, %v writes=%d/%d summaries=%d", retry, err, writes, afterPass, len(summaries))
			}
			var status setup.StatusOutput
			if err := json.Unmarshal(mustRun("status"), &status); err != nil || status.Status != "observed" || len(status.Items) != 1 || status.Items[0].State != workflow.ReadyForMerge || writes != afterPass || unwanted != "" {
				t.Fatalf("unknown status: %#v, %v writes=%d/%d unwanted=%q", status, err, writes, afterPass, unwanted)
			}
		})
	}
}

func TestNonMainSubmissionRefusesPublicHandoffsThroughGitHub(t *testing.T) {
	for _, operation := range []string{"implement submit", "implement needs-human", "watchdog submit", "status"} {
		t.Run(operation, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			prepareReviewWorktree(t, root, "widget")
			labels := []string{"rework", "wip"}
			metadataBody := fmt.Sprintf("<!-- skl.implement/v1\n{\"reviewed_head\":%q,\"review_round_head\":%q}\n-->", head, head)
			if operation == "watchdog submit" {
				labels = []string{"review", "wip"}
				metadataBody = fmt.Sprintf("<!-- skl.implement/v1\n{\"watchdog_head\":%q}\n-->", head)
			} else if operation == "status" {
				labels = []string{"done", "wip"}
				metadataBody = ""
			}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
				if r.Method != http.MethodGet {
					writes++
				}
				pullLabels := make([]map[string]string, 0, len(labels))
				for _, label := range labels {
					pullLabels = append(pullLabels, map[string]string{"name": label})
				}
				pull := map[string]any{"number": 11, "state": "open", "body": "existing\n\nCloses #7\n", "labels": pullLabels, "head": map[string]any{"sha": head, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "release"}}
				if selectionGraphQL(w, r, pull, true) {
					return
				}
				var result any = []any{}
				switch path {
				case "/issues":
					result = []any{map[string]any{"number": 7, "title": "widget", "state": "open"}}
				case "/pulls":
					result = []any{pull}
				case "/pulls/11", "/issues/11":
					result = pull
				case "/issues/7/comments":
					if metadataBody != "" {
						result = []any{map[string]any{"body": metadataBody, "author_association": "OWNER"}}
					}
				case "/issues/11/comments", "/pulls/11/comments", "/pulls/11/reviews", "/issues/11/timeline":
				default:
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
			dir := t.TempDir()
			summary, body, decision := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md"), filepath.Join(dir, "decision.md")
			for _, file := range []string{summary, body, decision} {
				if err := os.WriteFile(file, []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			args := strings.Fields(operation)
			switch operation {
			case "implement submit":
				args = append(args, "--item", "7", "--body", body)
			case "implement needs-human":
				args = append(args, "--item", "7", "--body", body, "--decision", decision, "--reason", "mandatory_rule")
			case "watchdog submit":
				args = append(args, "--item", "7", "--review-number", "1", "--reviewed-head", head, "--verdict", "pass", "--summary", summary, "--body", body)
			}
			args = append([]string{"skl"}, args...)
			args = append(args, "--repo", root)
			if err := app.Run(args); err != nil {
				t.Fatal(err)
			}
			var result setup.ImplementationOutput
			if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Status != "fix_required" || !strings.Contains(result.Reason, "release") || !strings.Contains(result.Reason, "main") || writes != 0 {
				t.Fatalf("%s refusal: %#v, %v writes=%d", operation, result, err, writes)
			}
		})
	}
}

func TestNeedsHumanPreservesDraftMainSubmissionThroughGitHub(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	if err := os.WriteFile(filepath.Join(root, "progress.txt"), []byte("work"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "progress.txt")
	runGit(t, root, "commit", "-m", "implementation progress")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	sourceLabels := []string{"ready", "wip"}
	var prLabels []string
	prBody := "existing\n\nCloses #7\n"
	draft := false
	var sourceComments, prComments []map[string]any
	unwanted := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		toLabels := func(labels []string) []map[string]string {
			result := make([]map[string]string, 0, len(labels))
			for _, label := range labels {
				result = append(result, map[string]string{"name": label})
			}
			return result
		}
		source := map[string]any{"number": 7, "title": "widget", "state": "open", "labels": toLabels(sourceLabels), "body": "Branch: `widget`\n"}
		pull := map[string]any{"number": 11, "node_id": "PR_node", "state": "open", "draft": draft, "body": prBody, "labels": toLabels(prLabels), "head": map[string]any{"sha": head, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
		if selectionGraphQL(w, r, pull, true) {
			return
		}
		var result any = []any{}
		switch {
		case path == "/issues":
			result = []any{source}
		case path == "/pulls":
			result = []any{pull}
		case path == "/issues/7/comments":
			if r.Method == http.MethodPost {
				var comment map[string]any
				json.NewDecoder(r.Body).Decode(&comment)
				comment["author_association"] = "OWNER"
				comment["created_at"] = "2026-01-01T00:00:02Z"
				sourceComments = append(sourceComments, comment)
			}
			result = sourceComments
		case path == "/issues/7/timeline":
			result = []any{map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
		case path == "/issues/11/timeline":
			result = []any{map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
		case path == "/issues/11/comments":
			if r.Method == http.MethodPost {
				var comment map[string]any
				json.NewDecoder(r.Body).Decode(&comment)
				comment["author_association"] = "OWNER"
				comment["created_at"] = "2026-01-01T00:00:02Z"
				prComments = append(prComments, comment)
			}
			result = prComments
		case path == "/pulls/11/comments", path == "/pulls/11/reviews", path == "/issues/7/dependencies/blocked_by":
		case path == "/pulls/11" && r.Method == http.MethodPatch:
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)
			if _, exists := payload["base"]; exists {
				unwanted = "retargeted Submission"
			}
			prBody = payload["body"]
		case path == "/pulls/11", path == "/issues/11":
			result = pull
		case path == "/git/ref/heads/widget":
			result = map[string]any{"object": map[string]string{"sha": head}}
		case path == "/issues/7":
			result = source
		case path == "/issues/7/labels" && r.Method == http.MethodPost, path == "/issues/11/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			json.NewDecoder(r.Body).Decode(&payload)
			target := &sourceLabels
			if path == "/issues/11/labels" {
				target = &prLabels
			}
			for _, label := range payload.Labels {
				if !slices.Contains(*target, label) {
					*target = append(*target, label)
				}
			}
		case strings.HasPrefix(path, "/issues/7/labels/") && r.Method == http.MethodDelete, strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
			target := &sourceLabels
			prefix := "/issues/7/labels/"
			if strings.HasPrefix(path, "/issues/11/") {
				target, prefix = &prLabels, "/issues/11/labels/"
			}
			label := strings.TrimPrefix(path, prefix)
			*target = slices.DeleteFunc(*target, func(value string) bool { return value == label })
		case path == "/graphql":
			var payload struct{ Query string }
			json.NewDecoder(r.Body).Decode(&payload)
			if strings.HasPrefix(payload.Query, "query") {
				// A pre-existing draft body carries native content-edit evidence
				// older than the source Claim, so the pause may refresh it.
				result = map[string]any{"data": map[string]any{"node": map[string]any{"body": prBody, "createdAt": "2026-01-01T00:00:00Z", "lastEditedAt": "2026-01-01T00:00:00Z"}}}
			} else {
				draft = true
				result = map[string]any{"data": map[string]any{}}
			}
		case path == "/git/ref/heads/main" || strings.Contains(path, "target"):
			unwanted = r.Method + " " + path
			http.Error(w, "target lookup forbidden", http.StatusInternalServerError)
			return
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
	if err := app.Run([]string{"skl", "implement", "resume", "--repo", root, "--item", "7"}); err != nil {
		t.Fatal(err)
	}
	var start setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &start); err != nil || start.Packet == nil {
		t.Fatalf("resume: %#v, %v", start, err)
	}
	dir := start.Packet.Facts.Implementation.ResultDirectory
	body, decision := filepath.Join(dir, "submission.md"), filepath.Join(dir, "decision.md")
	if err := os.WriteFile(body, []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decision, []byte("human decision"), 0600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := app.Run([]string{"skl", "implement", "needs-human", "--repo", root, "--item", "7", "--body", body, "--decision", decision, "--reason", "mandatory_rule"}); err != nil {
		t.Fatal(err)
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Status != "needs_human" || result.Item == nil || result.Item.Submission == nil || !result.Item.Submission.Draft || result.Item.Submission.Base != "main" || result.Item.Claimed || unwanted != "" {
		t.Fatalf("draft preservation: %#v, %v unwanted=%q", result, err, unwanted)
	}
}

func TestSubmitRefusesLateRetargetThroughGitHub(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	sourceLabels := []string{"ready"}
	var pullLabels []string
	branchHead := baseline
	base := ""
	prBody := ""
	createdBody := ""
	prExists := false
	completed := 0
	metadata := []map[string]any{}
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
		pull := map[string]any{"number": 11, "state": "open", "body": prBody, "labels": toLabels(pullLabels), "head": map[string]any{"ref": "widget", "sha": branchHead, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": base}}
		if selectionGraphQL(w, r, pull, prExists) {
			return
		}
		var result any = []any{}
		switch {
		case path == "/issues":
			result = []any{source}
		case path == "/pulls":
			if r.Method == http.MethodPost {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				base, _ = payload["base"].(string)
				prBody, _ = payload["body"].(string)
				createdBody = prBody
				prExists = true
				pull["base"] = map[string]string{"ref": base}
				pull["body"] = prBody
				result = pull
			} else if prExists {
				result = []any{pull}
			}
		case path == "/issues/7/comments":
			if r.Method == http.MethodPost {
				var comment map[string]any
				if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				comment["author_association"] = "OWNER"
				comment["created_at"] = "2026-01-01T00:00:02Z"
				if body, _ := comment["body"].(string); strings.Contains(body, `"completed":true`) {
					completed++
				}
				metadata = append(metadata, comment)
			}
			result = metadata
		case path == "/issues/7/timeline":
			result = []any{map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
		case path == "/issues/7/dependencies/blocked_by", path == "/issues/11/comments", path == "/pulls/11/comments", path == "/pulls/11/reviews", path == "/issues/11/timeline":
		case path == "/issues/7":
			result = source
		case path == "/issues/7/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for _, label := range payload.Labels {
				if !slices.Contains(sourceLabels, label) {
					sourceLabels = append(sourceLabels, label)
				}
			}
		case strings.HasPrefix(path, "/issues/7/labels/") && r.Method == http.MethodDelete:
			label := strings.TrimPrefix(path, "/issues/7/labels/")
			sourceLabels = slices.DeleteFunc(sourceLabels, func(value string) bool { return value == label })
		case path == "/git/ref/heads/widget":
			if prExists {
				// A human retargets the published Submission before its lifecycle labels move.
				base = "release"
			}
			result = map[string]any{"object": map[string]string{"sha": branchHead}}
		case path == "/pulls/11", path == "/issues/11":
			result = pull
		case path == "/issues/11/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for _, label := range payload.Labels {
				if !slices.Contains(pullLabels, label) {
					pullLabels = append(pullLabels, label)
				}
			}
		case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
			label := strings.TrimPrefix(path, "/issues/11/labels/")
			pullLabels = slices.DeleteFunc(pullLabels, func(value string) bool { return value == label })
		case path == "/graphql":
			result = map[string]any{"data": map[string]any{}}
		case path == "/git/ref/heads/main" || strings.Contains(path, "target"):
			unwanted = r.Method + " " + path
			http.Error(w, "integration target unavailable", http.StatusInternalServerError)
			return
		default:
			unwanted = r.Method + " " + path
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	run := func(args ...string) ([]byte, error) {
		t.Helper()
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) {
			backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
			backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			return backend, nil
		}, bytes.NewReader(nil), &output, &output)
		command := append([]string{"skl"}, args...)
		command = append(command, "--repo", root)
		err := app.Run(command)
		return slices.Clone(output.Bytes()), err
	}
	mustRun := func(args ...string) []byte {
		t.Helper()
		output, err := run(args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
		return output
	}
	var start setup.ImplementationOutput
	if err := json.Unmarshal(mustRun("implement", "next"), &start); err != nil || start.Packet == nil {
		t.Fatalf("start: %#v, %v", start, err)
	}
	directory := start.Packet.Facts.Implementation.ResultDirectory
	t.Cleanup(func() { os.RemoveAll(directory) })
	body := filepath.Join(directory, "submission.md")
	if err := os.WriteFile(body, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	completeAndRetireSlice(t, root, "widget")
	branchHead = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	var result setup.ImplementationOutput
	if err := json.Unmarshal(mustRun("implement", "submit", "--item", "7", "--body", body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "fix_required" || !strings.Contains(result.Reason, "release") || !strings.Contains(result.Reason, "main") || completed != 0 || slices.Contains(pullLabels, "review") || !slices.Contains(sourceLabels, "wip") || base != "release" || createdBody == "" || prBody != createdBody || unwanted != "" || readFile(t, body) != "candidate" {
		t.Fatalf("late retarget: %#v completed=%d source=%v pull=%v base=%q body=%q/%q unwanted=%q", result, completed, sourceLabels, pullLabels, base, createdBody, prBody, unwanted)
	}
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
		if r.Method != http.MethodGet {
			writes++
		}
		toLabels := func(values []string) []map[string]string {
			labels := make([]map[string]string, 0, len(values))
			for _, value := range values {
				labels = append(labels, map[string]string{"name": value})
			}
			return labels
		}
		source := map[string]any{"number": 7, "title": "widget", "state": "open", "labels": toLabels(sourceLabels), "body": "Branch: `widget`\n"}
		pull := map[string]any{"number": 11, "state": "open", "body": "existing\n\nCloses #7\n", "labels": toLabels(pullLabels), "head": map[string]any{"ref": "widget", "sha": head, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "release"}}
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
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "fix_required" || !strings.Contains(result.Reason, "release") || !strings.Contains(result.Reason, "main") || writes != 0 || !slices.Contains(sourceLabels, "wip") || len(pullLabels) != 1 || pullLabels[0] != "review" || unwanted != "" {
		t.Fatalf("retargeted partial handoff: %#v writes=%d source=%v pull=%v unwanted=%q", result, writes, sourceLabels, pullLabels, unwanted)
	}
}

func TestSubmitRefusesRecoveredNonMainSubmissionThroughGitHub(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	sourceLabels := []string{"ready"}
	pullLabels := []string{"review", "wip"}
	branchHead := baseline
	prBody := "human body"
	draft := true
	created := false
	patched := false
	draftMutations := 0
	unwanted := ""
	metadata := []map[string]any{}
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
		pull := map[string]any{"number": 11, "node_id": "PR_node", "state": "open", "draft": draft, "body": prBody, "labels": toLabels(pullLabels), "head": map[string]any{"ref": "widget", "sha": branchHead, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "release"}}
		if selectionGraphQL(w, r, pull, created) {
			return
		}
		var result any = []any{}
		switch {
		case path == "/issues":
			result = []any{source}
		case path == "/pulls":
			switch {
			case r.Method == http.MethodPost:
				// The create lands a fixed-head PR but its response is lost.
				created = true
				http.Error(w, "ambiguous create response", http.StatusInternalServerError)
				return
			case r.URL.Query().Get("head") != "" && !created:
			case created:
				result = []any{pull}
			}
		case path == "/issues/7/dependencies/blocked_by", path == "/issues/11/comments", path == "/pulls/11/comments", path == "/pulls/11/reviews", path == "/issues/11/timeline":
		case path == "/issues/7":
			result = source
		case path == "/issues/7/comments":
			if r.Method == http.MethodPost {
				var comment map[string]any
				if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				comment["author_association"] = "OWNER"
				comment["created_at"] = "2026-01-01T00:00:02Z"
				metadata = append(metadata, comment)
			}
			result = metadata
		case path == "/issues/7/timeline":
			result = []any{map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
		case path == "/issues/7/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for _, label := range payload.Labels {
				if !slices.Contains(sourceLabels, label) {
					sourceLabels = append(sourceLabels, label)
				}
			}
		case strings.HasPrefix(path, "/issues/7/labels/") && r.Method == http.MethodDelete:
			label := strings.TrimPrefix(path, "/issues/7/labels/")
			sourceLabels = slices.DeleteFunc(sourceLabels, func(value string) bool { return value == label })
		case path == "/git/ref/heads/widget":
			result = map[string]any{"object": map[string]string{"sha": branchHead}}
		case path == "/pulls/11" && r.Method == http.MethodPatch:
			patched = true
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			prBody = payload["body"]
		case path == "/pulls/11", path == "/issues/11":
			result = pull
		case path == "/issues/11/labels" && r.Method == http.MethodPost:
			var payload struct{ Labels []string }
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			for _, label := range payload.Labels {
				if !slices.Contains(pullLabels, label) {
					pullLabels = append(pullLabels, label)
				}
			}
		case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
			label := strings.TrimPrefix(path, "/issues/11/labels/")
			pullLabels = slices.DeleteFunc(pullLabels, func(value string) bool { return value == label })
		case path == "/graphql":
			draftMutations++
			draft = false
			result = map[string]any{"data": map[string]any{}}
		case path == "/git/ref/heads/main" || strings.Contains(path, "target"):
			unwanted = r.Method + " " + path
			http.Error(w, "integration target unavailable", http.StatusInternalServerError)
			return
		default:
			unwanted = r.Method + " " + path
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	run := func(args ...string) ([]byte, error) {
		t.Helper()
		var output bytes.Buffer
		app := newApp(func(github.RepositoryID) (setup.Backend, error) {
			backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
			backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			return backend, nil
		}, bytes.NewReader(nil), &output, &output)
		command := append([]string{"skl"}, args...)
		command = append(command, "--repo", root)
		err := app.Run(command)
		return slices.Clone(output.Bytes()), err
	}
	mustRun := func(args ...string) []byte {
		t.Helper()
		output, err := run(args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
		return output
	}
	var start setup.ImplementationOutput
	if err := json.Unmarshal(mustRun("implement", "next"), &start); err != nil || start.Packet == nil {
		t.Fatalf("start: %#v, %v", start, err)
	}
	directory := start.Packet.Facts.Implementation.ResultDirectory
	t.Cleanup(func() { os.RemoveAll(directory) })
	body := filepath.Join(directory, "submission.md")
	if err := os.WriteFile(body, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	completeAndRetireSlice(t, root, "widget")
	branchHead = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	var result setup.ImplementationOutput
	if err := json.Unmarshal(mustRun("implement", "submit", "--item", "7", "--body", body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "fix_required" || !strings.Contains(result.Reason, "release") || !strings.Contains(result.Reason, "main") || patched || draftMutations != 0 || prBody != "human body" || !draft || !slices.Contains(pullLabels, "wip") || !slices.Contains(sourceLabels, "wip") || unwanted != "" {
		t.Fatalf("recovered non-main Submission: %#v patched=%t draft=%d body=%q/%t pull=%v source=%v unwanted=%q", result, patched, draftMutations, prBody, draft, pullLabels, sourceLabels, unwanted)
	}
}

func TestWatchdogRefusesInvalidReviewedEvidenceThroughGitHub(t *testing.T) {
	for _, mergeability := range []string{"conflicting", "unknown"} {
		for _, tc := range []struct {
			evidence          string
			labels            []string
			differentReviewed bool
			divergentHead     bool
			moveHead          bool
			claimAbsent       bool
			reason            string
		}{
			{evidence: "PR head moves during verdict publication", labels: []string{"review", "wip"}, moveHead: true, reason: "Submission head changed"},
			{evidence: "supplied reviewed head differs from claimed revision", labels: []string{"review", "wip"}, differentReviewed: true, reason: "local reviewed head changed"},
			{evidence: "pushed final head does not descend from reviewed head", labels: []string{"review", "wip"}, divergentHead: true, reason: "descend"},
			{evidence: "required review Claim is absent", labels: []string{"review"}, claimAbsent: true, reason: "selected review Claim"},
		} {
			t.Run(mergeability+"/"+tc.evidence, func(t *testing.T) {
				root := proposalRepository(t)
				baseline := prepareSlice(t, root, "widget")
				completeAndRetireSlice(t, root, "widget")
				reviewed := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				head := reviewed
				if tc.divergentHead {
					runGit(t, root, "switch", "-C", "widget", baseline)
					runGit(t, root, "commit", "--allow-empty", "-m", "[completion] widget divergent")
					runGit(t, root, "rm", "-r", ".changes/widget")
					runGit(t, root, "commit", "-m", "divergent retirement")
					head = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				}
				prepareReviewWorktree(t, root, "widget")
				supplied := reviewed
				if tc.differentReviewed {
					supplied = baseline
				}
				labels := slices.Clone(tc.labels)
				pullHead := head
				prBody := "existing\n\nCloses #7\n"
				moved := false
				writes := 0
				metadata := []map[string]any{{"body": fmt.Sprintf("<!-- skl.implement/v1\n{\"watchdog_head\":%q}\n-->", reviewed), "author_association": "OWNER"}}
				var summaries []map[string]any
				events := []map[string]any{}
				for _, label := range labels {
					events = append(events, map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": label}})
				}
				unwanted := ""
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
					if r.Method != http.MethodGet {
						writes++
					}
					if tc.moveHead && path == "/pulls/11/reviews" && r.Method == http.MethodPost && !moved {
						moved = true
						pullHead = strings.Repeat("e", 40)
					}
					toLabels := func() []map[string]string {
						pullLabels := make([]map[string]string, 0, len(labels))
						for _, label := range labels {
							pullLabels = append(pullLabels, map[string]string{"name": label})
						}
						return pullLabels
					}
					pull := map[string]any{"number": 11, "state": "open", "body": prBody, "labels": toLabels(), "head": map[string]any{"sha": pullHead, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
					if mergeability != "unknown" {
						pull["mergeable"] = mergeability == "mergeable"
					}
					var result any = []any{}
					switch {
					case path == "/issues":
						result = []any{map[string]any{"number": 7, "title": "widget", "state": "open"}}
					case path == "/pulls":
						result = []any{pull}
					case path == "/issues/7/comments":
						result = metadata
					case path == "/issues/11/comments":
						if r.Method == http.MethodPost {
							var comment map[string]any
							if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
								http.Error(w, err.Error(), http.StatusBadRequest)
								return
							}
							comment["author_association"] = "OWNER"
							summaries = append(summaries, comment)
						}
						result = summaries
					case path == "/pulls/11" && r.Method == http.MethodPatch:
						var payload map[string]string
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						if _, exists := payload["base"]; exists {
							unwanted = "retargeted Submission"
						}
						prBody = payload["body"]
					case path == "/pulls/11", path == "/issues/11":
						result = pull
					case path == "/issues/11/timeline":
						result = events
					case path == "/git/ref/heads/widget":
						result = map[string]any{"object": map[string]string{"sha": head}}
					case path == "/issues/7":
						result = map[string]any{"number": 7, "title": "widget", "state": "open"}
					case path == "/issues/11/labels" && r.Method == http.MethodPost:
						var payload struct{ Labels []string }
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						for _, label := range payload.Labels {
							if !slices.Contains(labels, label) {
								labels = append(labels, label)
								events = append(events, map[string]any{"event": "labeled", "label": map[string]string{"name": label}})
							}
						}
					case strings.HasPrefix(path, "/issues/11/labels/") && r.Method == http.MethodDelete:
						label := strings.TrimPrefix(path, "/issues/11/labels/")
						labels = slices.DeleteFunc(labels, func(value string) bool { return value == label })
						events = append(events, map[string]any{"event": "unlabeled", "label": map[string]string{"name": label}})
					case path == "/graphql":
						result = map[string]any{"data": map[string]any{}}
					case path == "/git/ref/heads/main" || strings.Contains(path, "target"):
						unwanted = r.Method + " " + path
						http.Error(w, "integration target unavailable", http.StatusInternalServerError)
						return
					case path == "/pulls/11/reviews":
						if r.Method == http.MethodPost {
							var review map[string]any
							if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
								http.Error(w, err.Error(), http.StatusBadRequest)
								return
							}
							review["author_association"] = "OWNER"
							review["state"] = "COMMENTED"
							review["submitted_at"] = "2026-01-01T00:00:02Z"
							summaries = append(summaries, review)
						}
						result = summaries
					case path == "/pulls/11/comments", path == "/issues/7/dependencies/blocked_by":
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
				dir := t.TempDir()
				summary, body := filepath.Join(dir, "summary.md"), filepath.Join(dir, "body.md")
				for path, content := range map[string]string{summary: "review summary", body: "final body"} {
					if err := os.WriteFile(path, []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
				}
				command := []string{"skl", "watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", supplied, "--verdict", "pass", "--summary", summary, "--body", body}
				if tc.divergentHead {
					command = append(command, "--head", head)
				}
				command = append(command, "--repo", root)
				if err := app.Run(command); err != nil {
					t.Fatal(err)
				}
				var result setup.ImplementationOutput
				if err := json.Unmarshal(output.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Status != "fix_required" || !strings.Contains(result.Reason, tc.reason) || slices.Contains(labels, "done") || !slices.Contains(labels, "review") || len(summaries) > 1 || unwanted != "" {
					t.Fatalf("%s: %#v labels=%v summaries=%d unwanted=%q", tc.evidence, result, labels, len(summaries), unwanted)
				}
				if readFile(t, summary) != "review summary" || readFile(t, body) != "final body" {
					t.Fatalf("%s discarded repair documents", tc.evidence)
				}
				switch {
				case tc.moveHead:
					if len(summaries) != 1 || !slices.Contains(labels, "wip") {
						t.Fatalf("%s retained evidence: summaries=%d labels=%v", tc.evidence, len(summaries), labels)
					}
				case tc.claimAbsent:
					if slices.Contains(labels, "wip") || writes != 0 {
						t.Fatalf("%s fabricated a review Claim: labels=%v writes=%d", tc.evidence, labels, writes)
					}
				default:
					if !slices.Contains(labels, "wip") || writes != 0 {
						t.Fatalf("%s released the reviewed Claim or mutated the Submission: labels=%v writes=%d", tc.evidence, labels, writes)
					}
				}
			})
		}
	}
}
