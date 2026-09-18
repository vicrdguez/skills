package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

// The same CLI/adapter seam as selectionRun, with malformed wire observations.
func selectionReworkRun(t *testing.T, root string, forge *candidateForge, change func(*http.Request, *http.Response, []byte) []byte, args ...string) (setup.ImplementationOutput, error) {
	t.Helper()
	server := httptest.NewServer(forge)
	defer server.Close()
	client := server.Client()
	previous := client.Transport
	client.Transport = httpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		response, err := previous.RoundTrip(r)
		if err != nil {
			return response, err
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		body = change(r, response, body)
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		return response, nil
	})
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(server.URL, "token", client)
		backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	command := structuredStageCommand(args...)
	err := app.Run(append(command, "--repo", root))
	if err != nil {
		return setup.ImplementationOutput{}, err
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode %s: %v", &output, err)
	}
	if result.Packet != nil {
		t.Cleanup(func() {
			if result.Packet.Facts.Implementation != nil {
				_ = os.RemoveAll(result.Packet.Facts.Implementation.ResultDirectory)
			}
			if result.Packet.Facts.Watchdog != nil {
				_ = os.RemoveAll(result.Packet.Facts.Watchdog.ResultDirectory)
			}
		})
	}
	return result, nil
}

func TestRA1FinalPrewriteObservationCannotAdoptClaimOrLifecycleDrift(t *testing.T) {
	for _, queue := range []string{"ready", "rework", "review"} {
		for _, drift := range []string{"claimed", "paused", "closed", "body"} {
			t.Run(queue+"/"+drift, func(t *testing.T) {
				root := selectionRepository(t)
				forge := newCandidateForge()
				forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `widget`\n")
				number, lane := 30, "implement"
				if queue == "ready" {
					number = 7
					forge.addLabel(7, "ready")
					forge.readyPages = [][]int{{7}}
				} else {
					forge.addPull(30, "2021-01-01T00:00:00Z", "Closes #7\n", "widget", strings.Repeat("a", 40), queue)
					forge.reworkPages, forge.reviewPages = [][]int{{30}}, [][]int{{30}}
					if queue == "review" {
						lane = "watchdog"
					}
				}
				reads, changed := 0, false
				got, err := selectionReworkRun(t, root, forge, func(r *http.Request, _ *http.Response, body []byte) []byte {
					if r.Method != http.MethodGet || r.URL.Path != fmt.Sprintf("/repos/acme/widgets/issues/%d", number) {
						return body
					}
					reads++
					// Ready also reads Dependencies, selected context, and preflight.
					if changed || queue == "ready" && reads != 4 {
						return body
					}
					record := forge.pulls[number]
					if queue == "ready" {
						record = forge.issues[number]
					}
					switch drift {
					case "claimed":
						forge.addLabel(number, "wip")
					case "paused":
						record["labels"] = []map[string]string{{"name": "needs-human"}}
					case "closed":
						record["state"] = "closed"
					case "body":
						record["body"] = "changed attachment"
					}
					changed = true
					body, _ = json.Marshal(record)
					return body
				}, lane, "next")
				if !changed || got.Packet != nil || err == nil && got.Status != "fix_required" {
					t.Fatalf("final pre-write drift returned work: %#v, %v changed=%t", got, err, changed)
				}
				if forge.matching(func(request string) bool { return strings.Contains(request, "/labels") }) != 0 || forge.matching(func(request string) bool { return request == "graphql:queue" }) != 1 {
					t.Fatalf("incompatible observation was mutated or replaced: %v", forge.seen())
				}
			})
		}
	}
}

func TestW2ClaimReadbackRejectsAttachmentDrift(t *testing.T) {
	for _, change := range []string{"owner", "head", "branch", "base", "draft", "body", "ready branch"} {
		t.Run(change, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
			forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "rework")
			forge.reworkPages = [][]int{{30}}
			if change == "ready branch" {
				delete(forge.pulls, 30)
				forge.reworkPages = nil
				forge.readyPages = [][]int{{7}}
				forge.addLabel(7, "ready")
			}
			changed := false
			got, err := selectionReworkRun(t, root, forge, func(r *http.Request, _ *http.Response, body []byte) []byte {
				if r.Method != http.MethodGet || !(strings.HasSuffix(r.URL.Path, "/pulls/30") && hasLabel(forge.pulls[30], "wip") || change == "ready branch" && strings.HasSuffix(r.URL.Path, "/issues/7") && hasLabel(forge.issues[7], "wip")) {
					return body
				}
				var record map[string]any
				if err := json.Unmarshal(body, &record); err != nil {
					t.Fatal(err)
				}
				switch change {
				case "owner":
					record["body"] = "review\n\nCloses #8\n"
				case "head":
					record["head"].(map[string]any)["sha"] = strings.Repeat("b", 40)
				case "branch":
					record["head"].(map[string]any)["ref"] = "other-branch"
				case "base":
					record["base"].(map[string]any)["ref"] = "release"
				case "draft":
					record["draft"] = true
				case "body":
					record["body"] = "changed review\n\nCloses #7\n"
				case "ready branch":
					record["body"] = "Branch: `other-branch`\n"
				}
				changed = true
				body, _ = json.Marshal(record)
				return body
			}, "implement", "next")
			if !changed || err != nil || got.Status != "fix_required" || got.Packet != nil || !strings.Contains(got.Reason, "explicitly resume") {
				t.Fatalf("drift returned a stale handoff: %#v, %v (changed=%v)", got, err, changed)
			}
			if !hasLabel(forge.pulls[30], "wip") && !hasLabel(forge.issues[7], "wip") {
				t.Fatal("readback drift released the Claim")
			}
			if forge.matching(func(request string) bool { return request == "graphql:queue" }) != 1 || forge.matching(func(request string) bool { return strings.HasPrefix(request, "DELETE ") }) != 0 {
				t.Fatalf("readback drift replaced selection or released a Claim: %v", forge.seen())
			}
		})
	}
}

func TestW3ResumeRequiresSuppliedIdentity(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		t.Run(lane, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			label := "rework"
			if lane == "watchdog" {
				label = "review"
			}
			forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
			forge.addIssue(8, "2019-01-01T00:00:00Z", "Branch: `slice-eight`\n")
			forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #8\n", "slice-eight", strings.Repeat("a", 40), label, "wip")
			forge.owners[7] = []int{30}
			got, err := selectionRun(t, root, forge, lane, "resume", "--item", "7")
			if err != nil || got.Status != "fix_required" || got.Packet != nil || !strings.Contains(got.Reason, "supplied Work Item #7") {
				t.Fatalf("resume redirected the supplied identity: %#v, %v", got, err)
			}
			if forge.matching(func(request string) bool { return request == "graphql:queue" || strings.Contains(request, "/labels") }) != 0 {
				t.Fatalf("resume rediscovered or mutated work: %v", forge.seen())
			}
		})
	}
}

func TestW4SelectionRejectsForeignHeadRepository(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		t.Run(lane, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			label := "rework"
			if lane == "watchdog" {
				label = "review"
			}
			forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
			forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), label)
			forge.pulls[30]["head"].(map[string]any)["repo"] = map[string]string{"full_name": "outsider/widgets"}
			forge.reworkPages, forge.reviewPages = [][]int{{30}}, [][]int{{30}}
			got, err := selectionRun(t, root, forge, lane, "next")
			if err != nil || got.Status != "fix_required" || got.Packet != nil || !strings.Contains(got.Reason, "repository") {
				t.Fatalf("fork accepted: %#v, %v", got, err)
			}
			if forge.matching(func(request string) bool { return strings.Contains(request, "/labels") }) != 0 {
				t.Fatalf("foreign attachment claimed: %v", forge.seen())
			}
		})
	}
}

func TestW4ClosingReferencesRetainRepositoryIdentity(t *testing.T) {
	for _, purpose := range []string{"resume", "dependency"} {
		t.Run(purpose, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
			forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "rework", "wip")
			args := []string{"implement", "resume", "--item", "7"}
			if purpose == "dependency" {
				delete(forge.pulls, 30)
				forge.addLabel(7, "ready")
				forge.readyPages = [][]int{{7}}
				forge.dependencies[7] = []map[string]any{{"number": 9}}
				forge.evidence[9] = []map[string]any{{"number": 30, "merged": true}}
				args = []string{"implement", "next"}
			}
			changed := false
			got, err := selectionReworkRun(t, root, forge, func(r *http.Request, _ *http.Response, body []byte) []byte {
				if r.URL.Path != "/graphql" || !bytes.Contains(body, []byte("closedByPullRequestsReferences")) {
					return body
				}
				var record map[string]any
				_ = json.Unmarshal(body, &record)
				connection := record["data"].(map[string]any)["repository"].(map[string]any)["issue"].(map[string]any)["closedByPullRequestsReferences"].(map[string]any)
				for _, raw := range connection["nodes"].([]any) {
					raw.(map[string]any)["repository"] = map[string]string{"nameWithOwner": "outsider/widgets"}
					changed = true
				}
				body, _ = json.Marshal(record)
				return body
			}, args...)
			if !changed || err != nil || got.Status != "fix_required" || got.Packet != nil || !strings.Contains(got.Reason, "repository") {
				t.Fatalf("foreign reference became local identity: %#v, %v (changed=%v)", got, err, changed)
			}
			if forge.matching(func(request string) bool {
				return strings.Contains(request, "/pulls/30") || strings.Contains(request, "/labels")
			}) != 0 {
				t.Fatalf("foreign reference used as a local PR or claimed: %v", forge.seen())
			}
		})
	}
}

func TestW6InaccessibleDependencyPagesStopSelection(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		for _, failedPage := range []string{"1", "2"} {
			t.Run(fmt.Sprintf("%d/page%s", status, failedPage), func(t *testing.T) {
				root := selectionRepository(t)
				forge := newCandidateForge()
				forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n", "ready")
				forge.addIssue(8, "2020-01-01T00:00:00Z", "Branch: `slice-eight`\n", "ready")
				forge.readyPages = [][]int{{7, 8}}
				if failedPage == "2" {
					for range 100 {
						forge.dependencies[7] = append(forge.dependencies[7], map[string]any{"number": 9})
					}
					forge.evidence[9] = []map[string]any{{"number": 90, "merged": true, "repository": map[string]string{"nameWithOwner": "acme/widgets"}}}
				}
				var pages []string
				got, err := selectionReworkRun(t, root, forge, func(r *http.Request, response *http.Response, body []byte) []byte {
					if !strings.HasSuffix(r.URL.Path, "/issues/7/dependencies/blocked_by") {
						return body
					}
					page := r.URL.Query().Get("page")
					pages = append(pages, page)
					if page == failedPage {
						response.StatusCode = status
						return []byte(`{"message":"required Dependency page inaccessible"}`)
					}
					if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"number":9`)) {
						t.Fatalf("first Dependency page was not successful: %s", body)
					}
					return body
				}, "implement", "next")
				if err == nil && got.Status != "fix_required" || got.Packet != nil {
					t.Fatalf("inaccessible Dependency became absence: %#v, %v", got, err)
				}
				if strings.Join(pages, ",") != map[string]string{"1": "1", "2": "1,2"}[failedPage] {
					t.Fatalf("required page stream not observed: %v", pages)
				}
				if forge.matching(func(request string) bool {
					return strings.Contains(request, "/labels") || strings.Contains(request, "/issues/8/")
				}) != 0 {
					t.Fatalf("failure selected replacement work: %v", forge.seen())
				}
			})
		}
	}
}

func TestW9TruncatedLabelsCannotProveQueueIneligibility(t *testing.T) {
	for _, visible := range []string{"unrelated", "wip", "needs-human"} {
		t.Run(visible, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			labels := []string{visible}
			for len(labels) < 100 {
				labels = append(labels, fmt.Sprintf("external-%d", len(labels)))
			}
			forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), labels...)
			forge.reworkPages = [][]int{{30}}
			got, err := selectionRun(t, root, forge, "implement", "next")
			if err != nil || got.Status != "fix_required" || got.Packet != nil || !strings.Contains(got.Reason, "truncated") {
				t.Fatalf("truncated labels became no work: %#v, %v", got, err)
			}
			if forge.matching(func(request string) bool {
				return strings.Contains(request, "/labels") || strings.Contains(request, "labels=ready")
			}) != 0 {
				t.Fatalf("incomplete observation advanced selection: %v", forge.seen())
			}
		})
	}
}

func TestW10RequiredGraphQLContinuationsMustProgress(t *testing.T) {
	for _, stream := range []string{"queue", "owners"} {
		for name, cursors := range map[string][]string{"empty": {""}, "repeated": {"1", "1"}, "cycle": {"1", "2", "1"}, "failed continuation": {"1", "failure"}} {
			t.Run(stream+"/"+name, func(t *testing.T) {
				root := selectionRepository(t)
				forge := newCandidateForge()
				forge.addIssue(7, "2019-01-01T00:00:00Z", "Branch: `slice-seven`\n")
				forge.addPull(30, "2020-01-01T00:00:00Z", "review\n\nCloses #7\n", "slice-seven", strings.Repeat("a", 40), "rework", "wip")
				forge.reworkPages = [][]int{{30}}
				args := []string{"implement", "next"}
				connectionName := "pullRequests"
				if stream == "owners" {
					args = []string{"implement", "resume", "--item", "7"}
					connectionName = "closedByPullRequestsReferences"
				}
				var afters []string
				got, err := selectionReworkRun(t, root, forge, func(r *http.Request, response *http.Response, body []byte) []byte {
					if r.URL.Path != "/graphql" || !bytes.Contains(body, []byte(connectionName)) {
						return body
					}
					requestBody, _ := r.GetBody()
					var request struct{ Variables struct{ After string } }
					_ = json.NewDecoder(requestBody).Decode(&request)
					requestBody.Close()
					afters = append(afters, request.Variables.After)
					page := len(afters) - 1
					if page < len(cursors) && cursors[page] == "failure" {
						response.StatusCode = http.StatusNotFound
						return []byte(`{"message":"required continuation unavailable"}`)
					}
					var record map[string]any
					_ = json.Unmarshal(body, &record)
					container := record["data"].(map[string]any)["repository"].(map[string]any)
					if stream == "owners" {
						container = container["issue"].(map[string]any)
					}
					connection := container[connectionName].(map[string]any)
					if stream == "owners" {
						for _, raw := range connection["nodes"].([]any) {
							raw.(map[string]any)["repository"] = map[string]string{"nameWithOwner": "acme/widgets"}
						}
					}
					if page == 0 && (response.StatusCode != http.StatusOK || len(connection["nodes"].([]any)) == 0) {
						t.Fatal("first required page was not successful")
					}
					if page < len(cursors) {
						connection["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": cursors[page]}
					} else {
						// Bound a broken implementation without hiding its extra request.
						connection["pageInfo"] = map[string]any{"hasNextPage": false, "endCursor": ""}
					}
					body, _ = json.Marshal(record)
					return body
				}, args...)
				reason := got.Reason
				if err != nil {
					reason = err.Error()
				}
				want := "cursor"
				if name == "failed continuation" {
					want = "required continuation unavailable"
				}
				if !strings.Contains(reason, want) || got.Packet != nil || got.Status == "no_work" {
					t.Fatalf("incomplete GraphQL stream accepted: %#v, %v, after=%v", got, err, afters)
				}
				if len(afters) != len(cursors) || strings.Join(afters[1:], ",") != strings.Join(cursors[:len(cursors)-1], ",") {
					t.Fatalf("required continuation not followed or cursor retried: %v", afters)
				}
				if forge.matching(func(request string) bool { return strings.Contains(request, "/labels") }) != 0 {
					t.Fatalf("incomplete observation claimed work: %v", forge.seen())
				}
			})
		}
	}
}

func TestW7PublicationRequiresExclusiveNativeOwner(t *testing.T) {
	for _, phase := range []string{"before create", "before update", "after create", "handoff starts", "source cleanup", "final release", "pause handoff starts"} {
		t.Run(phase, func(t *testing.T) {
			root := selectionRepository(t)
			prepareSlice(t, root, "widget")
			completeAndRetireSlice(t, root, "widget")
			forge := newCandidateForge()
			forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `widget`\n", "ready")
			forge.readyPages = [][]int{{7}}
			forge.heads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			start, err := selectionRun(t, root, forge, "implement", "next")
			if err != nil || start.Packet == nil {
				t.Fatalf("start: %#v, %v", start, err)
			}
			bodyPath := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
			if err := os.WriteFile(bodyPath, []byte("candidate"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"implement", "submit", "--item", "7", "--body", bodyPath}
			if phase == "pause handoff starts" {
				decisionPath := filepath.Join(filepath.Dir(bodyPath), "decision.md")
				if err := os.WriteFile(decisionPath, []byte("needs a decision"), 0600); err != nil {
					t.Fatal(err)
				}
				args = []string{"implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decisionPath, "--body", bodyPath}
			}
			if phase == "before update" {
				forge.removeLabel(7, "ready")
				forge.removeLabel(7, "wip")
				forge.addPull(11, "2020-01-01T00:00:00Z", "older body\n\nCloses #7\n", "widget", forge.heads["widget"], "rework")
				forge.addLabel(11, "wip")
			}
			injected := strings.HasPrefix(phase, "before")
			if injected {
				forge.owners[7] = []int{99}
			}
			requestStart := len(forge.seen())
			got, err := selectionReworkRun(t, root, forge, func(r *http.Request, response *http.Response, body []byte) []byte {
				if phase == "pause handoff starts" && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls") {
					forge.pulls[11]["draft"] = true
					forge.pulls[11]["labels"] = []map[string]string{}
					body, _ = json.Marshal(forge.pulls[11])
				}
				if phase == "pause handoff starts" && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/11/comments") {
					requestBody, _ := r.GetBody()
					var comment map[string]any
					_ = json.NewDecoder(requestBody).Decode(&comment)
					requestBody.Close()
					comment["author_association"], comment["created_at"] = "OWNER", "2026-01-01T00:00:03Z"
					forge.comments["/issues/11/comments"] = append(forge.comments["/issues/11/comments"], comment)
					response.StatusCode = http.StatusCreated
					body, _ = json.Marshal(comment)
				}
				if r.URL.Path == "/graphql" && bytes.Contains(body, []byte("closedByPullRequestsReferences")) {
					var record map[string]any
					_ = json.Unmarshal(body, &record)
					connection := record["data"].(map[string]any)["repository"].(map[string]any)["issue"].(map[string]any)["closedByPullRequestsReferences"].(map[string]any)
					for _, raw := range connection["nodes"].([]any) {
						raw.(map[string]any)["repository"] = map[string]string{"nameWithOwner": "acme/widgets"}
					}
					body, _ = json.Marshal(record)
					if strings.HasSuffix(phase, "handoff starts") && forge.pulls[11] != nil && !injected {
						forge.owners[7], injected = []int{99}, true
					}
				}
				if !injected && (phase == "after create" && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls") || phase == "source cleanup" && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/11/labels") || phase == "final release" && r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/issues/7/labels/wip")) {
					forge.owners[7], injected = []int{99}, true
				}
				return body
			}, args...)
			if !injected || err != nil || got.Status != "fix_required" || !strings.Contains(got.Reason, "active Submission") {
				t.Fatalf("competing native owner accepted: %#v, %v (injected=%v)", got, err, injected)
			}
			if !hasLabel(forge.issues[7], "wip") && !hasLabel(forge.pulls[11], "wip") {
				t.Fatal("ownership refusal released the Claim")
			}
			if _, err := os.Stat(bodyPath); err != nil {
				t.Fatalf("ownership refusal discarded Result Document: %v", err)
			}
			if strings.HasPrefix(phase, "before") {
				for _, request := range forge.seen()[requestStart:] {
					if strings.HasPrefix(request, "PATCH ") || strings.HasPrefix(request, "DELETE ") || strings.HasPrefix(request, "POST ") && !strings.HasSuffix(request, " /graphql") {
						t.Fatalf("prepublication conflict was mutated: %s", request)
					}
				}
			}
		})
	}
}

func TestW15ImplementationHandoffRefusesLateFooterDrift(t *testing.T) {
	for _, command := range []string{"submit", "needs-human"} {
		for _, phase := range []string{"after publication", "source cleanup", "final release", "unchanged"} {
			t.Run(command+"/"+phase, func(t *testing.T) {
				root := selectionRepository(t)
				prepareSlice(t, root, "widget")
				completeAndRetireSlice(t, root, "widget")
				forge := newCandidateForge()
				forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `widget`\n", "ready")
				forge.readyPages = [][]int{{7}}
				forge.heads["widget"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
				start, err := selectionRun(t, root, forge, "implement", "next")
				if err != nil || start.Packet == nil {
					t.Fatalf("start: %#v, %v", start, err)
				}
				bodyPath := filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
				if err := os.WriteFile(bodyPath, []byte("candidate"), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"implement", command, "--item", "7", "--body", bodyPath}
				documents := []string{bodyPath}
				if command == "needs-human" {
					decisionPath := filepath.Join(filepath.Dir(bodyPath), "decision.md")
					if err := os.WriteFile(decisionPath, []byte("needs a decision"), 0600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--reason", "mandatory_rule", "--decision", decisionPath)
					documents = append(documents, decisionPath)
				}
				injected, observed := false, false
				requestStart := 0
				var sourceClaim, submissionClaim bool
				got, err := selectionReworkRun(t, root, forge, func(r *http.Request, response *http.Response, body []byte) []byte {
					if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls") {
						forge.pulls[11]["draft"] = command == "needs-human"
						forge.pulls[11]["labels"] = []map[string]string{}
						// Keep the original native link independent of later footer edits.
						forge.owners[7] = []int{11}
						forge.evidence[7] = []map[string]any{{"number": 11}}
						body, _ = json.Marshal(forge.pulls[11])
					}
					if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/11/comments") {
						requestBody, _ := r.GetBody()
						var comment map[string]any
						_ = json.NewDecoder(requestBody).Decode(&comment)
						requestBody.Close()
						comment["author_association"], comment["created_at"] = "OWNER", "2026-01-01T00:00:03Z"
						forge.comments["/issues/11/comments"] = append(forge.comments["/issues/11/comments"], comment)
						response.StatusCode = http.StatusCreated
						body, _ = json.Marshal(comment)
					}
					if injected && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/11") && bytes.Contains(body, []byte("Closes #8")) {
						observed = true
					}
					if !injected && forge.pulls[11] != nil && (phase == "after publication" && bytes.Contains(body, []byte("closedByPullRequestsReferences")) || phase == "source cleanup" && r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/issues/7/labels/ready") || phase == "final release" && r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/issues/7/labels/wip")) {
						forge.pulls[11]["body"] = "candidate\n\nCloses #8\n"
						injected, requestStart = true, len(forge.seen())
						sourceClaim, submissionClaim = hasLabel(forge.issues[7], "wip"), hasLabel(forge.pulls[11], "wip")
					}
					return body
				}, args...)
				if phase == "unchanged" {
					want := "awaiting_review"
					if command == "needs-human" {
						want = "needs_human"
					}
					if err != nil || got.Status != want || hasLabel(forge.issues[7], "wip") || hasLabel(forge.pulls[11], "wip") {
						t.Fatalf("unchanged ownership did not complete: %#v, %v", got, err)
					}
					return
				}
				if !injected || !observed || err != nil || got.Status != "fix_required" || !strings.Contains(got.Reason, "association") {
					t.Errorf("footer drift not refused: %#v, %v injected=%t observed=%t", got, err, injected, observed)
				}
				for _, request := range forge.seen()[requestStart:] {
					if strings.HasPrefix(request, "POST ") || strings.HasPrefix(request, "PATCH ") || strings.HasPrefix(request, "DELETE ") || request == "graphql:mutation" {
						t.Errorf("mutation after footer drift: %s", request)
					}
				}
				if !sourceClaim && !submissionClaim || hasLabel(forge.issues[7], "wip") != sourceClaim || hasLabel(forge.pulls[11], "wip") != submissionClaim {
					t.Error("ownership refusal changed or released the retained Claims")
				}
				if len(forge.owners[7]) != 1 || forge.owners[7][0] != 11 || forge.pulls[11]["body"] != "candidate\n\nCloses #8\n" {
					t.Error("handoff repaired or reassigned the conflicting ownership")
				}
				for _, path := range documents {
					if _, err := os.Stat(path); err != nil {
						t.Fatalf("ownership refusal discarded Result Document: %v", err)
					}
				}
			})
		}
	}
}
