package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func TestWatchdogHumanRequeuePreservesFirstFailureBounce(t *testing.T) {
	root := proposalRepository(t)
	prepareSlice(t, root, "widget")
	completeAndRetireSlice(t, root, "widget")
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	pause := []string{"+review", "+wip", "+needs-human", "-review", "-wip", "-needs-human", "+rework", "+wip", "+review", "-rework", "-wip", "+wip"}
	bounce := []string{"+review", "+wip", "+rework", "-review", "-wip", "-rework"}
	for _, tc := range []struct {
		name    string
		history []string
		bounces int
		status  string
	}{
		{"human requeue", pause, 0, "rework"},
		{"ordinary first failure", []string{"+review", "+wip"}, 0, "rework"},
		{"second failure", append(append([]string{}, bounce...), "+review", "+wip"), 1, "needs_human"},
		{"human requeue after real bounce", append(append([]string{}, bounce...), pause...), 1, "needs_human"},
		{"synchronization rework", []string{"+review", "+wip", "+sync", "+rework", "-review", "-wip", "+wip", "+review", "-rework", "-sync", "-wip", "+wip"}, 0, "rework"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/acme/widgets/pulls/11":
					fmt.Fprintf(w, `{"number":11,"state":"open","labels":[{"name":"review"},{"name":"wip"}],"head":{"sha":%q},"base":{"ref":"main"}}`, head)
				case "/repos/acme/widgets/issues/11/timeline":
					events := []map[string]any{}
					for _, event := range tc.history {
						kind := "labeled"
						if event[0] == '-' {
							kind = "unlabeled"
						}
						events = append(events, map[string]any{"event": kind, "label": map[string]string{"name": event[1:]}})
					}
					json.NewEncoder(w).Encode(events)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			adapter := setup.NewGitHubBackend(server.URL, "token", server.Client())
			submission, err := adapter.ReviewSubmission(context.Background(), github.RepositoryID{Owner: "acme", Name: "widgets"}, "11")
			if err != nil {
				t.Fatal(err)
			}
			if submission.Bounces != tc.bounces {
				t.Errorf("timeline bounces = %d, want %d", submission.Bounces, tc.bounces)
			}
			submission.ReviewedHead = head
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: submission.State, Claimed: submission.Claimed, Submission: &submission}}, remoteHeads: map[string]string{"widget": head}}
			summary := filepath.Join(t.TempDir(), "summary.md")
			if err := os.WriteFile(summary, []byte("first actual finding-driven failure"), 0600); err != nil {
				t.Fatal(err)
			}
			got := watchdogCLI(t, root, b, "submit", "--item", "7", "--reviewed-head", head, "--verdict", "rework", "--summary", summary)
			if got.Status != tc.status || got.Item == nil || got.Item.Claimed || got.Item.Submission.Bounces != 1 {
				t.Fatalf("first failure after human requeue: %#v; Submission: %#v", got, submission)
			}
		})
	}
}
