package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func (f *reviewFixture) getJSON(t *testing.T, path string) any {
	t.Helper()
	response, err := f.server.Client().Get(f.server.URL + "/repos/acme/widgets" + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", path, response.Status)
	}
	var value any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// Snapshot the actual records, not the engine's normalized lifecycle or counts.
func (f *reviewFixture) evidenceSnapshot(t *testing.T) map[string]any {
	t.Helper()
	snapshot := map[string]any{"checkpoint": checkpointSnapshot(f.checkpoint)}
	paths := []string{"/issues", "/pulls", "/issues/7", "/issues/8", "/issues/7/comments", "/issues/8/comments", "/issues/7/timeline", "/issues/8/timeline", "/git/ref/heads/widget"}
	if !f.forge.noPull {
		paths = append(paths, "/issues/11", "/pulls/11", "/issues/11/comments", "/pulls/11/comments", "/pulls/11/reviews", "/issues/11/timeline")
	}
	for _, path := range paths {
		snapshot[path] = f.getJSON(t, path)
	}
	return snapshot
}

func recordHasLabel(record any, label string) bool {
	for _, value := range record.(map[string]any)["labels"].([]any) {
		if value.(map[string]any)["name"] == label {
			return true
		}
	}
	return false
}

func (f *reviewFixture) assertSelectedClaim(t *testing.T) {
	t.Helper()
	data, err := f.runJSON(f.root, "status")
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Status string `json:"status"`
		Items  []struct {
			Number     int  `json:"number"`
			Claimed    bool `json:"claimed"`
			Submission *struct {
				Number  int  `json:"number"`
				Claimed bool `json:"claimed"`
			} `json:"submission"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	selected, other := false, false
	for _, item := range status.Items {
		if item.Number == 7 {
			selected = item.Claimed && item.Submission != nil && item.Submission.Number == 11 && item.Submission.Claimed
		}
		if item.Number == 8 {
			other = !item.Claimed
		}
	}
	if status.Status != "observed" || !selected || !other {
		t.Fatalf("status dropped the selected Claim or other item: %s", data)
	}
}

func (f *reviewFixture) assertProtectedCheckpoint(t *testing.T, lane string) {
	t.Helper()
	pull := f.getJSON(t, "/pulls/11")
	if pull.(map[string]any)["state"] != "open" {
		t.Fatal("Submission closed during cleanup")
	}
	if !recordHasLabel(pull, "wip") && (recordHasLabel(pull, "review") || recordHasLabel(pull, "rework") || recordHasLabel(pull, "done") || recordHasLabel(pull, "needs-human")) {
		t.Errorf("GET-visible destination exposed before cleanup: %#v", pull)
	}
	if source := f.getJSON(t, "/issues/7").(map[string]any); source["state"] != "open" {
		t.Errorf("source issue closed during cleanup: %#v", source)
	}
	if got := f.run(t, f.root, lane, "next"); got.Status != "no_work" {
		t.Errorf("%s claimed before cleanup: %#v", lane, got)
	}
}

func (f *reviewFixture) assertNoMutationAfter(t *testing.T, request string) {
	t.Helper()
	index := slices.Index(f.forge.requests, request)
	if index < 0 {
		t.Fatalf("failure boundary not reached: %s", request)
	}
	for _, later := range f.forge.requests[index+1:] {
		if !strings.HasPrefix(later, "GET ") && later != "POST /graphql query" {
			t.Fatalf("mutation after %s: %s", request, later)
		}
	}
}

func (f *reviewFixture) interleaveProtectedCleanup(t *testing.T, lane, body, decision, verdict string) {
	t.Helper()
	active, checks := false, 0
	f.forge.afterMutation = func() {
		if active {
			return
		}
		active = true
		defer func() { active = false }()
		checks++
		f.assertProtectedCheckpoint(t, lane)
		pull := f.getJSON(t, "/pulls/11").(map[string]any)
		target := recordHasLabel(pull, "needs-human") || lane == "watchdog" && recordHasLabel(pull, "review") || lane == "implement" && (recordHasLabel(pull, "rework") || recordHasLabel(pull, "done"))
		if !target {
			return
		}
		if body != "" && (pull["body"] != body || pull["head"].(map[string]any)["sha"] != f.head || pull["base"].(map[string]any)["ref"] != "main") {
			t.Errorf("target lacks fixed body/head/base: %#v", pull)
		}
		if decision != "" {
			comments := f.getJSON(t, "/issues/11/comments").([]any)
			if len(comments) != 1 || comments[0].(map[string]any)["body"] != "<!-- skl.implement.decision/v1 -->\n"+decision || pull["draft"] != true {
				t.Errorf("pause target lacks exact draft/decision: %#v %#v", pull, comments)
			}
		}
		if verdict != "" {
			reviews := f.getJSON(t, "/pulls/11/reviews").([]any)
			wanted := "<!-- skl.watchdog.review/v1\n{\"review_number\":1,\"verdict\":\"" + verdict + "\"}\n-->\nverdict"
			if len(reviews) != 1 || reviews[0].(map[string]any)["body"] != wanted || reviews[0].(map[string]any)["commit_id"] != f.head {
				t.Errorf("target lacks exact review receipt: %#v", reviews)
			}
		}
	}
	t.Cleanup(func() {
		if checks == 0 {
			t.Error("no mutation interleaving exercised")
		}
	})
}

func TestHandoffSourceCleanupFailureRowsThroughPublicHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, operation, failure string
		ready, sync              bool
	}{
		{"submit source pause", "submit", "DELETE /issues/7/labels/needs-human", false, false},
		{"pause source projection", "needs-human", "POST /issues/7/labels", true, false},
		{"pause source ready", "needs-human", "DELETE /issues/7/labels/ready", true, false},
		{"pause source Claim", "needs-human", "DELETE /issues/7/labels/wip", true, false},
		{"pause synchronization", "needs-human", "DELETE /issues/11/labels/sync", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther = true
			f.forge.labels, f.forge.sourceLabels = []string{"rework", "wip", "custom-pr"}, []string{"needs-human", "custom-source"}
			if tc.ready {
				f.forge.noPull, f.forge.labels = true, nil
				f.forge.sourceLabels = []string{"ready", "wip", "custom-source"}
				f.forge.sourceTimeline = []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
			} else {
				f.forge.timeline = []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
			}
			if tc.sync {
				f.forge.labels = append(f.forge.labels, "sync")
			}
			f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
			directory := newImplementationResultDirectory(t)
			body, decision := filepath.Join(directory, "submission.md"), filepath.Join(directory, "decision.md")
			if err := os.WriteFile(body, []byte("publication"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"implement", tc.operation, "--item", "7", "--body", body}
			decisionBytes := ""
			if tc.operation == "needs-human" {
				decisionBytes = "hold\n"
				if err := os.WriteFile(decision, []byte(decisionBytes), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--reason", "mandatory_rule", "--decision", decision)
			}
			f.interleaveProtectedCleanup(t, "watchdog", "publication\n\nCloses #7\n", decisionBytes, "")
			f.forge.rejectMutation = tc.failure
			if _, err := f.runResult(f.worktree, args...); err == nil {
				t.Fatal("source cleanup failure not reported")
			}
			f.assertNoMutationAfter(t, tc.failure)
			f.forge.afterMutation = nil
			if f.forge.rejectMutation != "" || !slices.Contains(f.forge.labels, "wip") || f.forge.body != "publication\n\nCloses #7\n" || readFile(t, body) != "publication" || !slices.Contains(f.forge.sourceLabels, "custom-source") || !tc.ready && !slices.Contains(f.forge.labels, "custom-pr") {
				t.Fatalf("failure did not preserve evidence/protection/unrelated labels: %v %v", f.forge.labels, f.forge.sourceLabels)
			}
			if decisionBytes != "" && readFile(t, decision) != decisionBytes {
				t.Fatal("decision changed")
			}
			f.assertProtectedCheckpoint(t, "watchdog")
		})
	}
	for _, verdict := range []string{"rework", "pass", "needs-human"} {
		t.Run("watchdog source pause/"+verdict, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther = true
			f.forge.sourceLabels = []string{"needs-human", "custom-source"}
			f.start(t, f.root)
			directory := t.TempDir()
			summary, body := filepath.Join(directory, "summary.md"), filepath.Join(directory, "submission.md")
			if err := os.WriteFile(summary, []byte("verdict"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", verdict, "--summary", summary}
			wantedBody := ""
			if verdict == "pass" {
				if err := os.WriteFile(body, []byte("final"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--body", body)
				wantedBody = "final\n\nCloses #7\n"
			}
			f.interleaveProtectedCleanup(t, "implement", wantedBody, "", verdict)
			failure := "DELETE /issues/7/labels/needs-human"
			if verdict == "needs-human" {
				// Needs Human retains the source pause; only its old review label is obsolete.
				failure = "DELETE /issues/11/labels/review"
			}
			f.forge.rejectMutation = failure
			if _, err := f.runResult(f.worktree, args...); err == nil {
				t.Fatal("cleanup failure not reported")
			}
			f.assertNoMutationAfter(t, failure)
			f.forge.afterMutation = nil
			if f.forge.rejectMutation != "" || readFile(t, summary) != "verdict" || !slices.Equal(f.forge.sourceLabels, []string{"needs-human", "custom-source"}) || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" {
				t.Fatalf("source/receipt/evidence changed after refusal: %v", f.forge.sourceLabels)
			}
			f.assertProtectedCheckpoint(t, "implement")
		})
	}
}

func TestUncertainDestinationReleasePreservesLaterClaimThroughPublicHTTP(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		for _, operation := range []string{"retry", "resume", "status"} {
			t.Run(lane+"/"+operation, func(t *testing.T) {
				f := newReviewFixture(t)
				var args []string
				var document string
				documents := map[string]string{}
				consumer := "implement"
				if lane == "implement" {
					consumer = "watchdog"
					f.forge.noPull, f.forge.labels = true, nil
					f.forge.sourceLabels = []string{"ready", "wip"}
					f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"}}
					start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
					document = filepath.Join(start.Packet.Facts.Implementation.ResultDirectory, "submission.md")
					args = []string{"implement", "submit", "--item", "7", "--body", document}
				} else {
					start := f.start(t, f.root)
					document = filepath.Join(start.Packet.Facts.Watchdog.ResultDirectory, "summary.md")
					args = []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", document}
					inline, findings := filepath.Join(filepath.Dir(document), "inline.md"), filepath.Join(filepath.Dir(document), "findings.json")
					documents[inline] = "exact finding"
					documents[findings] = fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, inline)
					args = append(args, "--findings", findings)
				}
				documents[document] = "exact handoff"
				for path, body := range documents {
					if err := os.WriteFile(path, []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
				}
				f.forge.loseResponse = "DELETE /issues/11/labels/wip"
				if got, err := f.runResult(f.worktree, args...); err == nil {
					t.Fatalf("uncertain destination release succeeded: %#v", got)
				}
				if f.forge.loseResponse != "" || !f.forge.readsUnavailable || !slices.Contains(f.forge.failedReads, "GET /issues/11") || slices.Contains(f.forge.labels, "wip") || len(f.forge.sourceLabels) != 0 || readFile(t, document) != "exact handoff" {
					t.Fatalf("not an accepted final destination release with all reads down: labels=%v source=%v failed=%v", f.forge.labels, f.forge.sourceLabels, f.forge.failedReads)
				}
				mutations := f.forge.acceptedMutations
				if len(mutations) == 0 || mutations[len(mutations)-1] != "DELETE /issues/11/labels/wip " {
					t.Fatalf("release was not last: %v", mutations)
				}
				f.forge.readsUnavailable = false
				pull := f.getJSON(t, "/pulls/11").(map[string]any)
				if pull["head"].(map[string]any)["sha"] != f.head || pull["base"].(map[string]any)["ref"] != "main" || pull["draft"] != false {
					t.Fatalf("release lost fixed PR evidence: %#v", pull)
				}
				if lane == "implement" {
					if pull["body"] != "exact handoff\n\nCloses #7\n" {
						t.Fatalf("release lost exact body: %#v", pull)
					}
				} else {
					reviews := f.getJSON(t, "/pulls/11/reviews").([]any)
					inlines := f.getJSON(t, "/pulls/11/comments").([]any)
					if len(reviews) != 1 || reviews[0].(map[string]any)["body"] != "<!-- skl.watchdog.review/v1\n{\"review_number\":1,\"verdict\":\"rework\"}\n-->\nexact handoff" || reviews[0].(map[string]any)["commit_id"] != f.head || len(inlines) != 1 {
						t.Fatalf("release lost review evidence: %#v %#v", reviews, inlines)
					}
					inline := inlines[0].(map[string]any)
					if inline["body"] != "exact finding" || inline["commit_id"] != f.head || inline["path"] != "README.md" || inline["line"] != float64(1) || inline["side"] != "RIGHT" {
						t.Fatalf("release lost exact finding anchor: %#v", inline)
					}
				}
				claimed := f.run(t, f.root, consumer, "next")
				if claimed.Status != "work_available" || claimed.Item == nil || claimed.Item.Number != 7 || !claimed.Item.Claimed || !slices.Contains(f.forge.labels, "wip") {
					t.Fatalf("other lane did not claim selected item: %#v", claimed)
				}
				before := f.evidenceSnapshot(t)
				writes, accepted := f.forge.writes, len(f.forge.acceptedMutations)
				switch operation {
				case "retry":
					got := f.run(t, f.worktree, args...)
					if got.Status != "fix_required" || got.Packet != nil || got.Reason == "" {
						t.Fatalf("old retry took later Claim: %#v", got)
					}
				case "resume":
					got := f.run(t, f.root, lane, "resume", "--item", "7")
					if got.Status != "fix_required" || got.Packet != nil || got.Reason == "" {
						t.Fatalf("old lane resumed later Claim: %#v", got)
					}
				case "status":
					f.assertSelectedClaim(t)
				}
				if f.forge.writes != writes || len(f.forge.acceptedMutations) != accepted || !reflect.DeepEqual(before, f.evidenceSnapshot(t)) || readFile(t, document) != "exact handoff" {
					t.Fatalf("%s mutated later Claim, complete evidence, checkpoint, or documents: %v", operation, f.forge.acceptedMutations[accepted:])
				}
				for path, body := range documents {
					if readFile(t, path) != body {
						t.Fatalf("retained Result Document changed: %s", path)
					}
				}
			})
		}
	}
}

func TestCleanupAfterVerificationNeverMutatesHandoffThroughPublicHTTP(t *testing.T) {
	for _, mode := range []string{"submit", "pause", "rework", "pass", "needs-human"} {
		t.Run(mode, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther = true
			var directory string
			var args []string
			consumer, want := "implement", mode
			if mode == "submit" || mode == "pause" {
				consumer = "watchdog"
				f.forge.labels = []string{"rework", "wip"}
				f.forge.timeline = []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:01Z", "label": map[string]string{"name": "wip"}}}
				directory = newImplementationResultDirectory(t)
				body := filepath.Join(directory, "submission.md")
				if err := os.WriteFile(body, []byte("final"), 0600); err != nil {
					t.Fatal(err)
				}
				args = []string{"implement", "submit", "--item", "7", "--body", body}
				want = "awaiting_review"
				if mode == "pause" {
					decision := filepath.Join(directory, "decision.md")
					if err := os.WriteFile(decision, []byte("hold"), 0600); err != nil {
						t.Fatal(err)
					}
					args = []string{"implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--body", body, "--decision", decision}
					want = "needs_human"
				}
			} else {
				start := f.start(t, f.root)
				directory = start.Packet.Facts.Watchdog.ResultDirectory
				summary := filepath.Join(directory, "summary.md")
				if err := os.WriteFile(summary, []byte("verdict"), 0600); err != nil {
					t.Fatal(err)
				}
				args = []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", mode, "--summary", summary}
				if mode == "needs-human" {
					want = "needs_human"
				}
				if mode == "pass" {
					want = "ready_for_merge"
					body := filepath.Join(directory, "submission.md")
					if err := os.WriteFile(body, []byte("final"), 0600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--body", body)
				}
			}
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("outside stays"), 0600); err != nil {
				t.Fatal(err)
			}
			unexpected := filepath.Join(directory, "unexpected")
			documents := map[string]string{}
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				documents[entry.Name()] = readFile(t, filepath.Join(directory, entry.Name()))
			}
			verified, writes, accepted := false, 0, 0
			var evidence map[string]any
			f.forge.afterRead = func(path string) {
				// Last selected-item GET in final observation: the response already
				// contains the complete unclaimed state, before the consumer starts.
				if path != "/issues/7/comments" || verified || slices.Contains(f.forge.labels, "wip") || slices.Contains(f.forge.sourceLabels, "wip") {
					return
				}
				verified = true
				if err := os.Symlink(outside, unexpected); err != nil {
					t.Error(err)
					return
				}
				beforeClaim := len(f.forge.acceptedMutations)
				got := f.run(t, f.root, consumer, "next")
				claimable := mode == "submit" || mode == "rework"
				if claimable && (got.Status != "work_available" || got.Item == nil || got.Item.Number != 7 || !got.Item.Claimed) || !claimable && got.Status != "no_work" {
					t.Errorf("immediate consumer: %#v", got)
				}
				if !claimable && len(f.forge.acceptedMutations) != beforeClaim {
					t.Error("nonclaimable consumer wrote state")
				}
				writes, accepted = f.forge.writes, len(f.forge.acceptedMutations)
				evidence = f.evidenceSnapshot(t)
			}
			got := f.run(t, f.worktree, args...)
			f.forge.afterRead = nil
			if !verified || got.Status != want || !strings.Contains(got.Reason, "cleanup") || !strings.Contains(got.Reason, "unexpected") || got.Packet != nil {
				t.Fatalf("cleanup did not retain verified success: %#v verified=%t", got, verified)
			}
			if f.forge.writes != writes || len(f.forge.acceptedMutations) != accepted {
				t.Fatalf("producer mutated after verification: %v", f.forge.acceptedMutations[accepted:])
			}
			// Successful pass may remove only its private checkpoint after the snapshot.
			after := f.evidenceSnapshot(t)
			if mode == "pass" {
				delete(evidence, "checkpoint")
				delete(after, "checkpoint")
			}
			if !reflect.DeepEqual(evidence, after) || readFile(t, outside) != "outside stays" {
				t.Fatal("post-verification cleanup changed evidence or outside file")
			}
			if target, err := os.Readlink(unexpected); err != nil || target != outside {
				t.Fatalf("unexpected symlink was removed or replaced: %q %v", target, err)
			}
			for name, body := range documents {
				if readFile(t, filepath.Join(directory, name)) != body {
					t.Fatalf("cleanup changed Result Document: %s", name)
				}
			}
		})
	}
}

func TestRetiredMetadataDoesNotAuthorizeHandoffsThroughPublicHTTP(t *testing.T) {
	for _, retired := range []struct{ name, fields string }{
		{"absent", ""},
		{"stale", `,"transition":{"from":"review","target":"done","head":"old","directory":"/retired","completed":true},"resume_state":"review"`},
		{"malformed fields", `,"transition":[false,17],"resume_state":{"invalid":true}`},
	} {
		for _, operation := range []string{"normal", "partial", "completed", "resume"} {
			t.Run(retired.name+"/"+operation, func(t *testing.T) {
				f := newReviewFixture(t)
				f.forge.noPull, f.forge.labels = true, nil
				f.forge.sourceLabels = []string{"ready", "wip"}
				metadata := fmt.Sprintf("<!-- skl.implement/v1\n{\"target_snapshot\":%q,\"target_branch\":\"main\"%s}\n-->", f.head, retired.fields)
				f.forge.sourceComments = []map[string]any{{"author_association": "OWNER", "body": metadata}}
				start := f.run(t, f.worktree, "implement", "resume", "--item", "7")
				if start.Status != "work_available" || start.Packet.Facts.Implementation.TargetSnapshot != f.head || start.Packet.Facts.Implementation.WorkItem != 7 {
					t.Fatalf("retired metadata altered resume/pin: %#v", start)
				}
				directory := start.Packet.Facts.Implementation.ResultDirectory
				body := filepath.Join(directory, "submission.md")
				if err := os.WriteFile(body, []byte("exact"), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"implement", "submit", "--item", "7", "--body", body}
				if operation == "resume" {
					before, writes := f.evidenceSnapshot(t), len(f.forge.acceptedMutations)
					got := f.run(t, f.root, "implement", "resume", "--item", "7")
					if got.Status != "work_available" || got.Packet.Facts.Implementation.TargetSnapshot != f.head || writes != len(f.forge.acceptedMutations) || !reflect.DeepEqual(before, f.evidenceSnapshot(t)) {
						t.Fatalf("resume used retired fields: %#v", got)
					}
				} else {
					if operation == "partial" {
						f.forge.loseResponse = "POST /pulls"
						if _, err := f.runResult(f.worktree, args...); err == nil || !f.forge.readsUnavailable {
							t.Fatal("partial publication not interrupted")
						}
						f.forge.readsUnavailable = false
					}
					if got := f.run(t, f.worktree, args...); got.Status != "awaiting_review" {
						t.Fatalf("publication used retired fields: %#v", got)
					}
					if operation == "completed" {
						directory = newImplementationResultDirectory(t)
						body = filepath.Join(directory, "submission.md")
						if err := os.WriteFile(body, []byte("exact"), 0600); err != nil {
							t.Fatal(err)
						}
						before, writes := f.evidenceSnapshot(t), len(f.forge.acceptedMutations)
						got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
						if got.Status != "awaiting_review" || writes != len(f.forge.acceptedMutations) || !reflect.DeepEqual(before, f.evidenceSnapshot(t)) || fileExists(body) {
							t.Fatalf("completed verification used retired fields: %#v", got)
						}
					}
					if f.forge.pullCreations != 1 || f.forge.body != "exact\n\nCloses #7\n" || !slices.Equal(f.forge.labels, []string{"review"}) || len(f.forge.sourceLabels) != 0 {
						t.Fatal("retired metadata changed final publication")
					}
				}
				for _, mutation := range f.forge.acceptedMutations {
					for _, forbidden := range []string{"transition", "resume_state", "body_digest", "decision_digest", "operation_id"} {
						if strings.Contains(mutation, forbidden) {
							t.Fatalf("new retired authority in HTTP payload: %s", mutation)
						}
					}
				}
				if f.forge.sourceComments[0]["body"] != metadata || !slices.Equal(f.forge.otherLabels, []string{"ready"}) {
					t.Fatal("historical pin or other item changed")
				}
				if len(f.forge.issueComments) != 0 || len(f.forge.inlines) != 0 || len(f.forge.summaries) != 0 {
					t.Fatal("publication wrote an unexpected comment record")
				}
				for _, comment := range f.forge.sourceComments[1:] {
					if comment["body"] != fmt.Sprintf("<!-- skl.implement/v1\n{\"target_snapshot\":%q,\"target_branch\":\"main\"}\n-->", f.head) {
						t.Fatalf("legitimate pin changed: %#v", comment)
					}
				}
				if fileExists(directory) {
					entries, err := os.ReadDir(directory)
					if err != nil {
						t.Fatal(err)
					}
					for _, entry := range entries {
						if entry.Name() != ".skl-result" && entry.Name() != "submission.md" {
							t.Fatalf("unexpected local operation record: %s", entry.Name())
						}
					}
				}
			})
		}
	}
}
