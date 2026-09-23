package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

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

func TestRA2StatusDiscoversUnlabeledNativeOwnerWithInvalidFooter(t *testing.T) {
	for _, footer := range []string{"missing", "wrong issue"} {
		t.Run(footer, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.sourceLabels, f.forge.otherLabels = nil, nil
			f.forge.labels = []string{"review", "wip"}
			f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": "2026-01-01T00:00:02Z", "label": map[string]string{"name": "wip"}})
			invalidBody := "audit without an owning footer"
			if footer == "wrong issue" {
				invalidBody = "audit\n\nCloses #8\n"
			}
			reads := map[int]int{}
			ownershipHTTP(t, f, func(r *http.Request, request, body []byte) []byte {
				if bytes.Contains(body, []byte("closedByPullRequestsReferences")) {
					var query struct{ Variables struct{ Number int } }
					_ = json.Unmarshal(request, &query)
					reads[query.Variables.Number]++
				}
				if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls") {
					var pulls []map[string]any
					_ = json.Unmarshal(body, &pulls)
					pulls[0]["body"] = invalidBody
					body, _ = json.Marshal(pulls)
				} else if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/11") {
					var pull map[string]any
					_ = json.Unmarshal(body, &pull)
					pull["body"] = invalidBody
					body, _ = json.Marshal(pull)
				}
				return body
			})
			output, err := f.runJSON(f.root, "status")
			var got setup.StatusOutput
			if err != nil || json.Unmarshal(output, &got) != nil {
				t.Fatalf("status: %s, %v", output, err)
			}
			found := false
			for _, item := range got.Items {
				if item.Number == 7 {
					found = true
					if item.State != workflow.NeedsHuman || !strings.Contains(item.Problem, "association") {
						t.Fatalf("native owner lacks an actionable ownership problem: %s", output)
					}
				}
				if item.Number == 8 && item.Submission != nil && item.Problem == "" {
					t.Fatalf("footer redirected the native owning association: %s", output)
				}
			}
			if !found {
				t.Fatalf("status omitted unlabeled native owner #7: %s (native reads=%v)", output, reads)
			}
			if reads[7] != 1 || f.forge.writes != 0 || !slices.Contains(f.forge.labels, "wip") {
				t.Fatalf("discovery repeated native reads or mutated the Claim: reads=%v writes=%d labels=%v", reads, f.forge.writes, f.forge.labels)
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

func TestW16StatusIgnoresOrdinaryNativeLinkedHistory(t *testing.T) {
	for _, state := range []string{"open", "closed"} {
		t.Run(state, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			forge.addIssue(7, "2020-01-01T00:00:00Z", "An ordinary repository issue.")
			forge.issues[7]["state"] = state
			forge.addPull(11, "2020-01-01T00:00:00Z", "Fixes #7", "ordinary", strings.Repeat("a", 40))
			forge.pulls[11]["state"] = state
			forge.evidence[7] = []map[string]any{{"number": 11}}
			if state == "closed" {
				forge.pulls[11]["merged_at"] = "2020-01-02T00:00:00Z"
			}

			forge.addIssue(8, "2020-01-01T00:00:00Z", "Branch: `widget`")
			forge.addPull(12, "2020-01-01T00:00:00Z", "damaged owning footer", "widget", strings.Repeat("b", 40), "review")
			forge.evidence[8] = []map[string]any{{"number": 12}}
			got := selectionStatusRun(t, root, forge)
			if len(got.Items) != 1 || got.Items[0].Number != 8 || got.Items[0].State != workflow.NeedsHuman || !strings.Contains(got.Items[0].Problem, "association") {
				t.Fatalf("ordinary history adopted or broken Workflow source lost: %#v", got)
			}
			for _, request := range forge.seen() {
				if strings.HasPrefix(request, "POST ") || strings.HasPrefix(request, "PATCH ") || strings.HasPrefix(request, "DELETE ") || request == "graphql:mutation" {
					t.Fatalf("status mutated native-linked history: %s", request)
				}
			}
		})
	}
}
