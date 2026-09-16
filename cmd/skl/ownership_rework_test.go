package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func ownershipHTTP(t *testing.T, f *reviewFixture, change func(*http.Request, []byte, []byte) []byte) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(request))
		response := httptest.NewRecorder()
		f.forge.ServeHTTP(response, r)
		body := change(r, request, response.Body.Bytes())
		for key, values := range response.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	f.server = server
}

func ownershipResponse(numbers ...int) []byte {
	nodes := []any{}
	for _, number := range numbers {
		nodes = append(nodes, map[string]any{"number": number, "repository": map[string]string{"nameWithOwner": "acme/widgets"}})
	}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"issue": map[string]any{"closedByPullRequestsReferences": map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}}}})
	return body
}

func TestRA2StatusPreservesNativeOwnershipProblems(t *testing.T) {
	for _, defect := range []string{"missing native", "competing native", "native without footer", "pending review conflict"} {
		t.Run(defect, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther = true
			f.forge.labels = []string{"review", "wip"}
			f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:02Z", "label": map[string]string{"name": "wip"}})
			if defect == "pending review conflict" {
				f.forge.labels = append(f.forge.labels, "done")
				f.forge.summaries = []map[string]any{storedReviewSummary(1, "pass", "round 1", f.head, "2026-01-01T00:00:03Z")}
			}
			if defect == "native without footer" {
				f.forge.noPull = true
				f.forge.sourceLabels = []string{"ready", "wip"}
			}
			observed := false
			ownershipHTTP(t, f, func(_ *http.Request, _ []byte, body []byte) []byte {
				if bytes.Contains(body, []byte("closedByPullRequestsReferences")) {
					observed = true
					if defect == "missing native" {
						return ownershipResponse()
					}
					if defect == "native without footer" {
						return ownershipResponse(12)
					}
					return ownershipResponse(11, 12)
				}
				return body
			})
			output, err := f.runJSON(f.root, "status")
			var got setup.StatusOutput
			if err != nil || json.Unmarshal(output, &got) != nil || !observed || len(got.Items) != 1 || got.Items[0].Problem == "" || !strings.Contains(got.Items[0].Problem, "association") || got.Items[0].State != workflow.NeedsHuman {
				t.Fatalf("status certified or lost conflicting ownership: %s, %v observed=%t", output, err, observed)
			}
			if !got.Items[0].Claimed || f.forge.writes != 0 {
				t.Fatalf("status released or mutated a conflicting Claim: %s writes=%d", output, f.forge.writes)
			}
		})
	}
}

func TestRA2StatusRetainsClosedSubmissionHistory(t *testing.T) {
	for _, merged := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "merged"}[merged], func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther, f.forge.labels = true, nil
			closedReferences := false
			ownershipHTTP(t, f, func(r *http.Request, request, body []byte) []byte {
				if bytes.Contains(body, []byte("closedByPullRequestsReferences")) {
					if bytes.Contains(request, []byte("includeClosedPrs:true")) {
						closedReferences = true
						return ownershipResponse(11, 12)
					}
					return ownershipResponse()
				}
				if r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/issues") || strings.HasSuffix(r.URL.Path, "/pulls")) {
					var records []map[string]any
					_ = json.Unmarshal(body, &records)
					for _, record := range records {
						record["state"] = "closed"
						if record["head"] != nil && merged {
							record["merged_at"] = "2026-01-01T00:00:03Z"
						}
					}
					if strings.HasSuffix(r.URL.Path, "/pulls") {
						records = append(records, map[string]any{"number": 12, "state": "closed", "body": "historical native reference without an owning footer"})
					}
					body, _ = json.Marshal(records)
				}
				return body
			})
			output, err := f.runJSON(f.root, "status")
			var got setup.StatusOutput
			want := workflow.Superseded
			if merged {
				want = workflow.Merged
			}
			if err != nil || json.Unmarshal(output, &got) != nil || !closedReferences || len(got.Items) != 1 || got.Items[0].Problem != "" || got.Items[0].State != want || got.Items[0].Submission == nil || got.Items[0].Submission.Number != 11 {
				t.Fatalf("legitimate terminal attachment lost: %s, %v closedReferences=%t", output, err, closedReferences)
			}
		})
	}
}

func TestRA2WatchdogRefusesNativeOwnershipConflict(t *testing.T) {
	for _, verdict := range []string{"rework", "needs-human", "pass"} {
		for _, phase := range []string{"before submit", "after summary", "target visible", "before release", "footer drift"} {
			t.Run(verdict+"/"+phase, func(t *testing.T) {
				f := newReviewFixture(t)
				f.forge.noOther = true
				start := f.start(t, f.root)
				if start.Packet == nil {
					t.Fatalf("start: %#v", start)
				}
				dir := start.Packet.Facts.Watchdog.ResultDirectory
				summary := filepath.Join(dir, "summary.md")
				if err := os.WriteFile(summary, []byte("round 1"), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", verdict, "--summary", summary}
				if phase == "after summary" {
					findings := filepath.Join(dir, "findings.json")
					anchors, _ := json.Marshal([]map[string]any{{"path": "main.go", "line": 1, "side": "RIGHT", "body_file": summary}})
					if err := os.WriteFile(findings, anchors, 0600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--findings", findings)
				}
				if verdict == "pass" {
					body := filepath.Join(dir, "submission.md")
					if err := os.WriteFile(body, []byte("final"), 0600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--body", body)
				}
				conflict := phase == "before submit"
				target := map[string]string{"rework": "rework", "needs-human": "needs-human", "pass": "done"}[verdict]
				f.forge.afterMutation = func() {
					if phase == "after summary" && len(f.forge.summaries) != 0 || (phase == "target visible" || phase == "footer drift") && slices.Contains(f.forge.labels, target) || phase == "before release" && !slices.Contains(f.forge.labels, "review") {
						conflict = true
					}
				}
				observed := false
				ownershipHTTP(t, f, func(r *http.Request, _ []byte, body []byte) []byte {
					if conflict && phase == "footer drift" && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/11") {
						var pull map[string]any
						_ = json.Unmarshal(body, &pull)
						pull["body"] = "Closes #8"
						body, _ = json.Marshal(pull)
						observed = true
					}
					if conflict && phase != "footer drift" && bytes.Contains(body, []byte("closedByPullRequestsReferences")) {
						observed = true
						return ownershipResponse(11, 12)
					}
					return body
				})
				writes := f.forge.writes
				got, err := f.runResult(f.worktree, args...)
				if err != nil || got.Status != "fix_required" || !observed || !slices.Contains(f.forge.labels, "wip") || !fileExists(summary) {
					t.Fatalf("conflicting ownership completed or released review: %#v, %v observed=%t labels=%v", got, err, observed, f.forge.labels)
				}
				if phase == "before submit" && f.forge.writes != writes || phase == "after summary" && (len(f.forge.summaries) != 1 || len(f.forge.inlines) != 0) {
					t.Fatalf("unexpected publication through conflict: writes=%d/%d summaries=%d inlines=%d", f.forge.writes, writes, len(f.forge.summaries), len(f.forge.inlines))
				}
			})
		}
	}
}
