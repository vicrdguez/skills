package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func TestGitHubWatchdogBodyPassRetry(t *testing.T) {
	for _, footer := range []bool{false, true} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("footer=%t/partial=%t", footer, partial), func(t *testing.T) {
				f := newReviewFixture(t)
				writes, patches := 0, 0
				f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet {
						writes++
					}
					if r.Method == http.MethodPatch {
						patches++
					}
					f.forge.ServeHTTP(w, r)
				})
				f.start(t, f.root)
				body := "opaque\x00\r\n  final prose\n"
				publishedBody := body + "\n\nCloses #7\n"
				if footer {
					body = publishedBody
				}
				if partial {
					f.forge.failDelete = "review"
				}
				dir := t.TempDir()
				summary, bodyPath := filepath.Join(dir, "summary.md"), filepath.Join(dir, "submission.md")
				for path, content := range map[string]string{summary: "opaque pass summary", bodyPath: body} {
					if err := os.WriteFile(path, []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
				}
				args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "pass", "--summary", summary, "--body", bodyPath}
				first, err := f.runResult(f.worktree, args...)
				if partial {
					if err == nil || !strings.Contains(err.Error(), "label deletion unavailable") || !slices.Contains(f.forge.labels, "done") || !slices.Contains(f.forge.labels, "wip") {
						t.Fatalf("expected durable partial pass: %#v, %v, labels=%v", first, err, f.forge.labels)
					}
				} else if err != nil || first.Status != "ready_for_merge" {
					t.Fatalf("first pass: %#v, %v", first, err)
				}
				if f.forge.body != publishedBody || patches != 1 || len(f.forge.summaries) != 1 {
					t.Fatalf("publication: body=%q, patches=%d, summaries=%v", f.forge.body, patches, f.forge.summaries)
				}
				before := writes
				if err := os.WriteFile(bodyPath, []byte("changed "+body), 0600); err != nil {
					t.Fatal(err)
				}
				changed := f.run(t, f.worktree, args...)
				if changed.Status != "fix_required" || !strings.Contains(changed.Reason, "Result Documents") || writes != before {
					t.Fatalf("changed Result Document accepted or mutated backend: %#v, writes=%d/%d", changed, writes, before)
				}
				if err := os.WriteFile(bodyPath, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				for attempt := range 2 {
					retried := f.run(t, f.worktree, args...)
					if retried.Status != "ready_for_merge" || retried.Item.Claimed || retried.Item.Submission.Body != publishedBody {
						t.Fatalf("unchanged pass retry: %#v", retried)
					}
					if f.forge.body != publishedBody || patches != 1 || len(f.forge.summaries) != 1 || !slices.Equal(f.forge.labels, []string{"done"}) || (!partial || attempt > 0) && writes != before {
						t.Fatalf("retry republished or changed evidence: body=%q, patches=%d, summaries=%v, labels=%v, writes=%d/%d", f.forge.body, patches, f.forge.summaries, f.forge.labels, writes, before)
					}
					before = writes
				}
			})
		}
	}
}

func TestGitHubWatchdogBodyObservation(t *testing.T) {
	for name, body := range map[string]string{
		"adopted":   "original",
		"published": "opaque\x00\r\n  prose\n\nCloses #7\n",
	} {
		t.Run(name, func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			f.forge.body = body
			f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("observation mutated backend: %s %s", r.Method, r.URL)
					http.Error(w, "unexpected mutation", 500)
					return
				}
				f.forge.ServeHTTP(w, r)
			})
			backend := setup.NewGitHubBackend(f.server.URL, "token", f.server.Client())
			backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
			ctx := context.Background()
			inspected, err := workflow.InspectImplementation(ctx, f.root, "7", workflow.ArtifactEndpoints{}, backend)
			if err != nil || inspected.Item == nil {
				t.Fatalf("inspect: %#v, %v", inspected, err)
			}
			if inspected.Item.Submission.Body != body {
				t.Errorf("observed body = %q, want %q", inspected.Item.Submission.Body, body)
			}
			output, err := setup.PresentImplementation(inspected)
			if err != nil {
				t.Fatal(err)
			}
			status, err := setup.PresentStatus(workflow.StatusOutcome{Items: []workflow.ImplementationItem{*inspected.Item}})
			if err != nil {
				t.Fatal(err)
			}
			started, err := workflow.StartWatchdog(ctx, f.root, "origin", "7", workflow.ArtifactEndpoints{}, backend)
			if err != nil || started.Facts == nil {
				t.Fatalf("Watchdog resume: %#v, %v", started, err)
			}
			t.Cleanup(func() { os.RemoveAll(started.Facts.Watchdog.ResultDirectory) })
			packet, err := setup.PresentImplementation(started)
			if err != nil {
				t.Fatal(err)
			}
			for name, value := range map[string]any{"inspect": output, "status": status, "packet": packet} {
				data, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var native map[string]any
				if err := json.Unmarshal(data, &native); err != nil {
					t.Fatal(err)
				}
				item := native["item"]
				if name == "status" {
					item = native["items"].([]any)[0]
				}
				submission := item.(map[string]any)["Submission"].(map[string]any)
				if submission["Body"] != body || submission["Number"] != float64(11) {
					t.Errorf("%s native Submission = %#v, want exact body %q and number 11", name, submission, body)
				}
				if name == "packet" {
					audit := native["packet"].(map[string]any)["facts"].(map[string]any)["watchdog"].(map[string]any)["audit_body"]
					if audit != body {
						t.Errorf("audit_body = %q, want %q", audit, body)
					}
				}
			}
		})
	}
}
