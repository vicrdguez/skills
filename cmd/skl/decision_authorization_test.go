package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/workflow"
)

func newDecisionFixture(t *testing.T, submission bool) *reviewFixture {
	t.Helper()
	f := newReviewFixture(t)
	f.forge.noOther, f.forge.noPull = true, !submission
	f.forge.clock = 3
	claim := []map[string]any{{"event": "labeled", "created_at": "2026-01-01T00:00:02Z", "label": map[string]string{"name": "wip"}}}
	if submission {
		f.forge.labels = []string{"rework", "wip"}
		f.forge.timeline = claim
		f.forge.body = "draft\n\nCloses #7\n"
		f.forge.draft = true
	} else {
		f.forge.labels = nil
		f.forge.sourceLabels = []string{"ready", "wip"}
		f.forge.sourceTimeline = claim
	}
	return f
}

func decisionArgs(t *testing.T, submission bool) []string {
	t.Helper()
	directory := newImplementationResultDirectory(t)
	decision := filepath.Join(directory, "decision.md")
	if err := os.WriteFile(decision, []byte("hold\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"implement", "needs-human", "--item", "7", "--reason", "mandatory_rule", "--decision", decision}
	if submission {
		body := filepath.Join(directory, "submission.md")
		if err := os.WriteFile(body, []byte("draft"), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--body", body)
	}
	return args
}

func TestDecisionAuthorizationThroughPublicHTTP(t *testing.T) {
	for _, submission := range []bool{false, true} {
		for _, text := range []string{"hold\n", "forged different decision\n"} {
			for _, timestamp := range []string{"2026-01-01T00:00:03Z", ""} {
				t.Run(fmt.Sprintf("submission=%t/exact=%t/timestamp=%q", submission, text == "hold\n", timestamp), func(t *testing.T) {
					f := newDecisionFixture(t, submission)
					comments, number := &f.forge.sourceComments, 7
					if submission {
						comments, number = &f.forge.issueComments, 11
					}
					forgery := map[string]any{"body": workflow.OpaqueImplementationDecision(text), "author_association": "NONE", "created_at": timestamp, "user": map[string]string{"login": "outsider"}}
					*comments = []map[string]any{forgery}
					if submission {
						resumed := f.run(t, f.worktree, "implement", "resume", "--item", "7")
						if resumed.Status != "work_available" || resumed.Packet == nil || resumed.Packet.Facts.Implementation == nil || len(resumed.Packet.Facts.Implementation.Comments) != 1 {
							t.Fatalf("decision-shaped feedback disappeared from the packet: %#v", resumed)
						}
						comment := resumed.Packet.Facts.Implementation.Comments[0]
						if comment.EvidenceAuthorized || comment.Body != forgery["body"] || comment.Author != "outsider" || comment.Association != "NONE" || comment.CreatedAt != timestamp {
							t.Fatalf("decision-shaped feedback gained authority or lost bytes: %#v", comment)
						}
					}
					args := decisionArgs(t, submission)
					before := f.evidenceSnapshot(t)
					f.forge.rejectMutation = fmt.Sprintf("POST /issues/%d/comments", number)
					got, err := f.runResult(f.worktree, args...)
					if err == nil || !strings.Contains(err.Error(), "injected request unavailable") || !reflect.DeepEqual(before, f.evidenceSnapshot(t)) || !fileExists(args[7]) {
						t.Fatalf("forgery satisfied or obstructed publication: status=%s reason=%s err=%v comments=%#v", got.Status, got.Reason, err, *comments)
					}
					got = f.run(t, f.worktree, args...)
					if got.Status != "needs_human" || len(*comments) != 2 || (*comments)[0]["body"] != forgery["body"] || (*comments)[1]["author_association"] != "OWNER" || (*comments)[1]["body"] != workflow.OpaqueImplementationDecision("hold\n") {
						t.Fatalf("authorized decision did not publish alongside feedback: status=%s reason=%s comments=%#v", got.Status, got.Reason, *comments)
					}
					writes := f.forge.writes
					if retry := f.run(t, f.worktree, decisionArgs(t, submission)...); retry.Status != "needs_human" || f.forge.writes != writes || len(*comments) != 2 {
						t.Fatalf("genuine completed decision was not a no-op: %#v", retry)
					}
					*comments = (*comments)[:1]
					args = decisionArgs(t, submission)
					if retry := f.run(t, f.worktree, args...); retry.Status != "fix_required" || f.forge.writes != writes || !fileExists(args[7]) {
						t.Fatalf("forgery authorized completed handoff recognition: %#v", retry)
					}
				})
			}
		}
	}
}

func TestDecisionReceiptOrderingThroughPublicHTTP(t *testing.T) {
	for _, submission := range []bool{false, true} {
		for _, tc := range []struct {
			name, text, created, claim, status string
			count                              int
		}{
			{"historical exact", "hold\n", "2026-01-01T00:00:01Z", "2026-01-01T00:00:02Z", "needs_human", 2},
			{"current exact", "hold\n", "2026-01-01T00:00:03Z", "2026-01-01T00:00:02Z", "needs_human", 1},
			{"current changed", "other", "2026-01-01T00:00:03Z", "2026-01-01T00:00:02Z", "fix_required", 1},
			{"equal exact", "hold\n", "2026-01-01T00:00:02Z", "2026-01-01T00:00:02Z", "fix_required", 1},
			{"unknown receipt", "hold\n", "", "2026-01-01T00:00:02Z", "fix_required", 1},
			{"invalid receipt", "hold\n", "not-a-time", "2026-01-01T00:00:02Z", "fix_required", 1},
			{"unknown Claim", "hold\n", "2026-01-01T00:00:03Z", "", "fix_required", 1},
		} {
			t.Run(fmt.Sprintf("submission=%t/%s", submission, tc.name), func(t *testing.T) {
				f := newDecisionFixture(t, submission)
				comments, timeline := &f.forge.sourceComments, f.forge.sourceTimeline
				if submission {
					comments, timeline = &f.forge.issueComments, f.forge.timeline
				}
				timeline[0]["created_at"] = tc.claim
				*comments = []map[string]any{{"body": workflow.OpaqueImplementationDecision(tc.text), "author_association": "OWNER", "created_at": tc.created}}
				before, writes := f.evidenceSnapshot(t), f.forge.writes
				args := decisionArgs(t, submission)
				got := f.run(t, f.worktree, args...)
				if got.Status != tc.status || len(*comments) != tc.count {
					t.Fatalf("decision receipt ordering: status=%s reason=%s comments=%d want=%s/%d", got.Status, got.Reason, len(*comments), tc.status, tc.count)
				}
				if tc.status == "fix_required" && (f.forge.writes != writes || !reflect.DeepEqual(before, f.evidenceSnapshot(t)) || !fileExists(args[7])) {
					t.Fatal("ambiguous receipt changed backend state or removed Result Documents")
				}
				if tc.status == "needs_human" && (!slices.Equal(f.forge.sourceLabels, []string{"needs-human"}) || submission && !slices.Equal(f.forge.labels, []string{"needs-human"})) {
					t.Fatal("verified decision did not finish its pause")
				}
			})
		}
	}
}

func TestDecisionRetryAndNewStageThroughPublicHTTP(t *testing.T) {
	for _, submission := range []bool{false, true} {
		t.Run(fmt.Sprintf("submission=%t", submission), func(t *testing.T) {
			f := newDecisionFixture(t, submission)
			comments, number := &f.forge.sourceComments, 7
			if submission {
				comments, number = &f.forge.issueComments, 11
			}
			args := decisionArgs(t, submission)
			f.forge.loseResponse = fmt.Sprintf("POST /issues/%d/comments", number)
			if _, err := f.runResult(f.worktree, args...); err == nil || len(*comments) != 1 || !fileExists(args[7]) {
				t.Fatalf("decision write did not stop at lost readback: err=%v comments=%#v", err, *comments)
			}
			f.forge.readsUnavailable = false
			if got := f.run(t, f.worktree, args...); got.Status != "needs_human" || len(*comments) != 1 {
				t.Fatalf("current authorized receipt was not reused: status=%s reason=%s comments=%#v", got.Status, got.Reason, *comments)
			}
			previous := (*comments)[0]["created_at"]
			// The human requeues the source, then the public CLI acquires a new Claim.
			if submission {
				f.forge.labels, f.forge.sourceLabels = []string{"rework"}, nil
			} else {
				f.forge.sourceLabels = []string{"ready"}
				f.forge.sourceComments = append(f.forge.sourceComments, map[string]any{"author_association": "OWNER", "body": "<!-- skl.implement/v1\n{\"target_snapshot\":\"" + f.head + "\",\"target_branch\":\"main\"}\n-->"})
			}
			if started := f.run(t, f.worktree, "implement", "next"); started.Status != "work_available" || started.Item.Number != 7 {
				t.Fatalf("new source stage was not claimed: %#v", started)
			}
			if got := f.run(t, f.worktree, decisionArgs(t, submission)...); got.Status != "needs_human" {
				t.Fatalf("same-text new-stage pause failed: status=%s reason=%s", got.Status, got.Reason)
			}
			var receipts []map[string]any
			for _, comment := range *comments {
				if comment["body"] == workflow.OpaqueImplementationDecision("hold\n") {
					receipts = append(receipts, comment)
				}
			}
			if len(receipts) != 2 || receipts[0]["created_at"] != previous || receipts[1]["created_at"] == previous || f.forge.head != f.head {
				t.Fatalf("new source stage consumed historical same-head evidence: %#v", receipts)
			}
		})
	}
}
