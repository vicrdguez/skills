package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

// W10: the outer decision framing is transport-only. Original Watchdog Result
// Documents must recover interrupted and completed handoffs through the concrete
// adapter and semantic command without callers altering prose.
func TestWatchdogRecoversHandoffFromOriginalSummaryDocuments(t *testing.T) {
	for _, fault := range []string{"interrupted claim release", "receipt publication"} {
		t.Run(fault, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			runGit(t, root, "switch", "main")
			runGit(t, root, "worktree", "add", filepath.Join(root, ".worktrees", "widget"), "widget")
			labels := []string{"review", "wip"}
			summaries, metadata := []map[string]any{}, []map[string]any{}
			events := []map[string]any{
				{"event": "labeled", "created_at": "2026-01-01T00:00:00Z", "label": map[string]string{"name": "review"}},
				{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}},
			}
			blockRelease, failReceipt := fault == "interrupted claim release", fault == "receipt publication"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets")
				ls := []map[string]string{}
				for _, label := range labels {
					ls = append(ls, map[string]string{"name": label})
				}
				pull := map[string]any{"number": 11, "state": "open", "labels": ls, "mergeable": true,
					"head": map[string]any{"sha": head, "ref": "widget", "repo": map[string]string{"full_name": "acme/widgets"}},
					"base": map[string]string{"ref": "main"}}
				var result any = []any{}
				switch {
				case p == "/issues":
					result = []any{map[string]any{"number": 7, "title": "widget", "state": "open"}}
				case p == "/pulls":
					result = []any{pull}
				case p == "/pulls/11" || p == "/issues/11":
					result = pull
				case p == "/issues/7/comments":
					if r.Method == http.MethodPost {
						var comment map[string]any
						if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
							t.Errorf("decode comment: %v", err)
						}
						comment["author_association"] = "OWNER"
						if failReceipt && strings.Contains(comment["body"].(string), `"outcome"`) {
							http.Error(w, "injected receipt publication failure", 500)
							return
						}
						metadata = append(metadata, comment)
					}
					result = metadata
				case p == "/issues/11/comments":
					result = []any{}
				case p == "/issues/11/timeline":
					result = events
				case p == "/issues/11/labels" && r.Method == http.MethodPost:
					var payload struct{ Labels []string }
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Errorf("decode labels: %v", err)
					}
					for _, label := range payload.Labels {
						labels = append(labels, label)
						events = append(events, map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:03Z", "label": map[string]string{"name": label}})
					}
				case strings.HasPrefix(p, "/issues/11/labels/") && r.Method == http.MethodDelete:
					label := strings.TrimPrefix(p, "/issues/11/labels/")
					if label == "wip" && blockRelease {
						http.Error(w, "injected unapplied Claim-release failure", 500)
						return
					}
					labels = slices.DeleteFunc(labels, func(v string) bool { return v == label })
					events = append(events, map[string]any{"event": "unlabeled", "created_at": "2026-01-01T00:00:04Z", "label": map[string]string{"name": label}})
				case strings.HasPrefix(p, "/git/ref/heads/"):
					result = map[string]any{"object": map[string]string{"sha": head}}
				case p == "/pulls/11/reviews":
					if r.Method == http.MethodPost {
						var review map[string]any
						if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
							t.Errorf("decode review: %v", err)
						}
						review["state"] = "COMMENTED"
						review["submitted_at"] = "2026-01-01T00:00:02Z"
						summaries = append(summaries, review)
					}
					result = summaries
				case p == "/pulls/11/comments":
					result = []any{}
				default:
					t.Errorf("unexpected %s %s", r.Method, p)
					http.NotFound(w, r)
					return
				}
				if err := json.NewEncoder(w).Encode(result); err != nil {
					t.Errorf("encode: %v", err)
				}
			}))
			defer server.Close()
			ctx, repo := t.Context(), github.RepositoryID{Owner: "acme", Name: "widgets"}
			b := setup.NewGitHubBackend(server.URL, "token", server.Client())
			b.BindRepository(repo)
			item := workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.AwaitingReview, Claimed: true,
				Submission: &workflow.Submission{ID: "11", Head: head, ReviewedHead: head}}
			if err := b.ClaimImplementation(ctx, item); err != nil {
				t.Fatal(err)
			}
			round := workflow.DispatchRound{ID: "review-round", Item: "7", Submission: "11", Lane: workflow.WatchdogLane, Obligation: head, Directory: "skl-watchdog-focused"}
			if err := b.RecordDispatchRound(ctx, round); err != nil {
				t.Fatal(err)
			}
			summary := filepath.Join(t.TempDir(), "summary.md")
			if err := os.WriteFile(summary, []byte("## W10\nOriginal document bytes.\n"), 0600); err != nil {
				t.Fatal(err)
			}
			submit := func() (setup.ImplementationOutput, error) {
				var output bytes.Buffer
				app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
					backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
					backend.BindRepository(repository)
					return backend, nil
				}, bytes.NewReader(nil), &output, &output)
				if err := app.Run([]string{"skl", "watchdog", "submit", "--repo", root, "--remote", "origin", "--item", "7", "--review-number", "1", "--reviewed-head", head, "--head", head, "--verdict", "needs-human", "--summary", summary}); err != nil {
					return setup.ImplementationOutput{}, err
				}
				var got setup.ImplementationOutput
				if err := json.Unmarshal(output.Bytes(), &got); err != nil {
					t.Fatalf("submit output: %v\n%s", err, &output)
				}
				return got, nil
			}
			if got, err := submit(); err == nil {
				t.Fatalf("interruption not injected: %+v", got)
			}
			if retained := slices.Contains(labels, "wip"); retained != (fault == "interrupted claim release") {
				t.Fatalf("interrupted Claim state: labels=%v", labels)
			}
			blockRelease, failReceipt = false, false
			got, err := submit()
			if err != nil || got.Status != "needs_human" {
				t.Fatalf("identical documents did not recover the handoff: %+v %v labels=%v", got, err, labels)
			}
			if !slices.Equal(labels, []string{"needs-human"}) || len(summaries) != 1 {
				t.Fatalf("recovery mutated projections or duplicated the summary: labels=%v summaries=%d", labels, len(summaries))
			}
			rounds, err := b.DispatchRounds(ctx, "7")
			if err != nil || len(rounds) != 1 || rounds[0].Outcome != workflow.NeedsHuman || rounds[0].Head != head || !rounds[0].Released {
				t.Fatalf("completion receipt: %+v %v", rounds, err)
			}
		})
	}
}
