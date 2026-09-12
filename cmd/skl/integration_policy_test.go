package main

import (
	"bytes"
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
)

func TestStaleSynchronizationReworkUsesOrdinaryCLIFlow(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	labels := []string{"rework", "sync"}
	body := "existing"
	metadata := []map[string]any{{"body": fmt.Sprintf("<!-- skl.implement/v1\n{\"reviewed_head\":%q,\"review_round_head\":%q}\n-->", head, head), "author_association": "OWNER"}}
	var events []map[string]any
	unwanted := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
		pullLabels := make([]map[string]string, 0, len(labels))
		for _, label := range labels {
			pullLabels = append(pullLabels, map[string]string{"name": label})
		}
		pull := map[string]any{"number": 11, "state": "open", "body": body, "labels": pullLabels, "head": map[string]any{"sha": head, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
		var result any = []any{}
		switch {
		case path == "/issues":
			result = []any{map[string]any{"number": 7, "title": "widget", "state": "open"}}
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
		case path == "/issues/11/comments", path == "/pulls/11/comments", path == "/pulls/11/reviews":
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
					events = append(events, map[string]any{"event": "labeled", "label": map[string]string{"name": label}})
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
			return setup.NewGitHubBackend(server.URL, "token", server.Client()), nil
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
	if start.Status != "work_available" || start.Packet == nil || start.Packet.Facts.Implementation.PreviousReviewedHead != head || !slices.Contains(labels, "sync") {
		t.Fatalf("start lost review evidence or sync: %#v labels=%v", start, labels)
	}
	resume := run("implement", "resume", "--item", "7")
	if resume.Status != "work_available" || resume.Packet.Facts.Implementation.PreviousReviewedHead != head || !slices.Contains(labels, "sync") {
		t.Fatalf("resume lost review evidence or sync: %#v labels=%v", resume, labels)
	}
	resultDir := resume.Packet.Facts.Implementation.ResultDirectory
	result := filepath.Join(resultDir, "submission.md")
	if err := os.WriteFile(result, []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	completed := run("implement", "submit", "--item", "7", "--body", result)
	if completed.Status != "awaiting_review" || slices.Contains(labels, "sync") || !slices.Contains(labels, "review") || unwanted != "" {
		t.Fatalf("ordinary handoff: %#v labels=%v unwanted=%q", completed, labels, unwanted)
	}
}
