package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		issue := map[string]any{"number": 7, "title": "widget", "state": "open", "labels": issueLabels}
		pullLabelObjects := make([]map[string]string, 0, len(prLabels))
		for _, label := range prLabels {
			pullLabelObjects = append(pullLabelObjects, map[string]string{"name": label})
		}
		pull := map[string]any{"number": 11, "state": "open", "body": prBody, "labels": pullLabelObjects, "head": map[string]any{"ref": "widget", "sha": branchHead, "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": postedBase}}
		var result any = []any{}
		switch {
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
				metadata = append(metadata, comment)
			}
			result = metadata
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
			return setup.NewGitHubBackend(server.URL, "token", server.Client()), nil
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
	if exec.Command("git", "-C", root, "cat-file", "-e", mainHead+"^{commit}").Run() == nil || exec.Command("git", "-C", root, "cat-file", "-e", obsoleteTarget+"^{commit}").Run() == nil || first.Status != "work_available" || first.Packet == nil || first.Packet.Facts.Implementation.ArtifactBaseline != baseline || unwanted != "" {
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
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	branchHead = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	submitted := run("submit", "--item", "7", "--body", result)
	if submitted.Status != "awaiting_review" || postedBase != "main" || prBody != "candidate\n\nCloses #7\n" || !pullExists || !slices.Contains(prLabels, "review") || slices.Contains(labels, "ready") || unwanted != "" {
		t.Fatalf("target-free submission: %#v base=%q labels=%v/%v unwanted=%q", submitted, postedBase, labels, prLabels, unwanted)
	}
}

func TestStaleSynchronizationReworkUsesOrdinaryCLIFlow(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	runGit(t, root, "rm", "-r", ".changes/widget")
	runGit(t, root, "commit", "-m", "retire")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	labels := []string{"rework", "sync"}
	body := "existing"
	metadata := []map[string]any{{"body": fmt.Sprintf("<!-- skl.implement/v1\n{\"reviewed_head\":%q,\"review_round_head\":%q,\"target_snapshot\":\"missing\",\"target_branch\":\"release\",\"synchronization_target\":\"conflicting\"}\n-->", head, head), "author_association": "OWNER"}}
	reviewComments := []map[string]any{{"body": "retained review feedback", "author_association": "OWNER"}}
	events := []map[string]any{
		{"event": "labeled", "label": map[string]string{"name": "review"}},
		{"event": "labeled", "label": map[string]string{"name": "wip"}},
		{"event": "labeled", "label": map[string]string{"name": "rework"}},
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
	if start.Status != "work_available" || start.Packet == nil || start.Packet.Facts.Implementation.PreviousReviewedHead != head || len(start.Packet.Facts.Implementation.Comments) != 1 || start.Packet.Facts.Implementation.Comments[0].Body != "retained review feedback" || !slices.Contains(labels, "sync") {
		t.Fatalf("start lost review evidence or sync: %#v labels=%v", start, labels)
	}
	counted, err := setup.NewGitHubBackend(server.URL, "token", server.Client()).ReviewSubmission(context.Background(), github.RepositoryID{Owner: "acme", Name: "widgets"}, "11")
	if err != nil || counted.Bounces != 1 {
		t.Fatalf("retained review count = %d, %v", counted.Bounces, err)
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
	counted, err = setup.NewGitHubBackend(server.URL, "token", server.Client()).ReviewSubmission(context.Background(), github.RepositoryID{Owner: "acme", Name: "widgets"}, "11")
	if err != nil || counted.Bounces != 1 {
		t.Fatalf("review count changed after handoff = %d, %v", counted.Bounces, err)
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
			runGit(t, root, "rm", "-r", ".changes/widget")
			runGit(t, root, "commit", "-m", "retire")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			labels := []string{"review", "wip"}
			prBody := "audit"
			mergeability := tc.initial
			failRelease := tc.recover != ""
			metadata := []map[string]any{{"body": fmt.Sprintf("<!-- skl.implement/v1\n{\"watchdog_head\":%q}\n-->", head), "author_association": "OWNER"}}
			var summaries, events []map[string]any
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
				case path == "/pulls/11/comments", path == "/pulls/11/reviews":
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
					return setup.NewGitHubBackend(server.URL, "token", server.Client()), nil
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
			command := []string{"watchdog", "submit", "--item", "7", "--reviewed-head", head, "--verdict", "pass", "--summary", summary, "--body", finalBody}
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
			runGit(t, root, "rm", "-r", ".changes/widget")
			runGit(t, root, "commit", "-m", "retire")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
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
				pull := map[string]any{"number": 11, "state": "open", "body": "existing", "labels": pullLabels, "head": map[string]any{"sha": head, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "release"}}
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
				return setup.NewGitHubBackend(server.URL, "token", server.Client()), nil
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
				args = append(args, "--item", "7", "--reviewed-head", head, "--verdict", "pass", "--summary", summary, "--body", body)
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
	prBody := "existing"
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
		source := map[string]any{"number": 7, "title": "widget", "state": "open", "labels": toLabels(sourceLabels)}
		pull := map[string]any{"number": 11, "node_id": "PR_node", "state": "open", "draft": draft, "body": prBody, "labels": toLabels(prLabels), "head": map[string]any{"sha": head, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}}, "base": map[string]string{"ref": "main"}}
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
				sourceComments = append(sourceComments, comment)
			}
			result = sourceComments
		case path == "/issues/11/comments":
			if r.Method == http.MethodPost {
				var comment map[string]any
				json.NewDecoder(r.Body).Decode(&comment)
				comment["author_association"] = "OWNER"
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
			draft = true
			result = map[string]any{"data": map[string]any{}}
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
		return setup.NewGitHubBackend(server.URL, "token", server.Client()), nil
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
