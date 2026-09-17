package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestReviewAuthorizationThroughPublicHTTP(t *testing.T) {
	for _, state := range []string{"COMMENTED", "APPROVED", "CHANGES_REQUESTED"} {
		for _, framed := range []bool{true, false} {
			for _, olderHead := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/framed=%t/older-head=%t", state, framed, olderHead), func(t *testing.T) {
					f := newReviewFixture(t)
					f.start(t, f.root)
					head := f.head
					if olderHead {
						head = strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD^"))
					}
					review := storedReviewSummary(^uint64(0), "rework", "untrusted feedback\n", head, f.forge.timestamp())
					review["state"], review["author_association"] = state, "NONE"
					review["user"] = map[string]string{"login": "outsider"}
					if !framed {
						review["body"] = "untrusted feedback\n"
					}
					feedback := review["body"].(string)
					f.forge.summaries = []map[string]any{review}

					writes := f.forge.writes
					resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7")
					if resumed.Status != "work_available" || resumed.Packet == nil || resumed.Packet.Facts.Watchdog == nil {
						t.Errorf("untrusted feedback blocked resume: status=%s reason=%s", resumed.Status, resumed.Reason)
					} else {
						facts := resumed.Packet.Facts.Watchdog
						if facts.ReviewCount != 0 || facts.ReviewNumber != 1 || len(facts.Comments) != 1 {
							t.Fatalf("untrusted feedback changed review facts: %#v", facts)
						}
						comment := facts.Comments[0]
						if comment.Body != feedback || comment.Author != "outsider" || comment.Association != "NONE" || comment.Commit != head || comment.CreatedAt != review["submitted_at"] || comment.Verdict != "" || comment.ReviewNumber != 0 {
							t.Errorf("feedback lost bytes or gained authority: %#v", comment)
						}
					}
					if f.forge.writes != writes || fileExists(f.checkpoint) || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
						t.Fatal("resume changed the Claim or checkpoint")
					}

					got := f.submit(t, 1, f.head, "rework")
					if got.Status != "rework" {
						t.Fatalf("untrusted feedback blocked submit: status=%s reason=%s", got.Status, got.Reason)
					}
					if len(f.forge.summaries) != 2 || f.forge.summaries[0]["body"] != feedback || !slices.Equal(f.forge.labels, []string{"rework"}) || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" {
						t.Fatal("submission did not preserve feedback and complete exactly one review")
					}
					writes = f.forge.writes
					if retry := f.submit(t, 1, f.head, "rework"); retry.Status != "rework" || f.forge.writes != writes || len(f.forge.summaries) != 2 || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" {
						t.Fatalf("authorized receipt retry failed or republished: status=%s reason=%s", retry.Status, retry.Reason)
					}
				})
			}
		}
	}
}

func TestReviewReceiptRetryAuthorizationThroughPublicHTTP(t *testing.T) {
	for _, tc := range []struct {
		association string
		authorized  bool
	}{
		{"OWNER", true},
		{"MEMBER", true},
		{"COLLABORATOR", true},
		{"NONE", false},
		{"CONTRIBUTOR", false},
		{"FIRST_TIMER", false},
		{"FIRST_TIME_CONTRIBUTOR", false},
		{"MANNEQUIN", false},
		{"", false},
	} {
		t.Run(fmt.Sprintf("association=%q", tc.association), func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			receipt := storedReviewSummary(1, "rework", "round 1", f.head, f.forge.timestamp())
			receipt["author_association"] = tc.association
			if tc.association == "" {
				delete(receipt, "author_association")
			}
			f.forge.summaries = []map[string]any{receipt}
			if err := os.WriteFile(f.checkpoint, []byte("1:"+f.head+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			writes := f.forge.writes
			got := f.submit(t, 1, f.head, "rework")
			if tc.authorized {
				if got.Status != "rework" || !slices.Equal(f.forge.labels, []string{"rework"}) {
					t.Fatalf("authorized receipt did not finish its retry: status=%s reason=%s", got.Status, got.Reason)
				}
				if got.Item == nil || got.Item.Submission == nil || len(got.Item.Submission.Comments) != 1 {
					t.Fatalf("completed retry lost its receipt: %#v", got.Item)
				}
				comment := got.Item.Submission.Comments[0]
				if comment.Body != "round 1" || comment.Verdict != "rework" || comment.ReviewNumber != 1 || comment.Association != tc.association {
					t.Fatalf("authorized receipt was not decoded: %#v", comment)
				}
				writes = f.forge.writes
				if retry := f.submit(t, 1, f.head, "rework"); retry.Status != "rework" || f.forge.writes != writes {
					t.Fatalf("completed retry was not a no-op: status=%s reason=%s", retry.Status, retry.Reason)
				}
			} else {
				if got.Status != "fix_required" || !strings.Contains(got.Reason, "recorded review differs") || f.forge.writes != writes || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
					t.Fatalf("identical untrusted receipt authorized a retry: status=%s reason=%s writes=%d", got.Status, got.Reason, f.forge.writes)
				}
				resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7")
				if resumed.Status != "work_available" || resumed.Packet == nil || resumed.Packet.Facts.Watchdog == nil || len(resumed.Packet.Facts.Watchdog.Comments) != 1 {
					t.Fatalf("untrusted receipt obstructed resume or hid feedback: %#v", resumed)
				}
				comment := resumed.Packet.Facts.Watchdog.Comments[0]
				if comment.Body != receipt["body"] || comment.Verdict != "" || comment.ReviewNumber != 0 || comment.Association != tc.association {
					t.Fatalf("untrusted receipt gained authority or lost feedback: %#v", comment)
				}
			}
			if len(f.forge.summaries) != 1 || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" {
				t.Fatal("receipt retry republished or advanced the review count")
			}
		})
	}
}

func TestReviewPublicationAuthorizationThroughPublicHTTP(t *testing.T) {
	for _, copies := range []int{1, 2} {
		for _, lostReadback := range []bool{false, true} {
			t.Run(fmt.Sprintf("copies=%d/lost-readback=%t", copies, lostReadback), func(t *testing.T) {
				f := newReviewFixture(t)
				f.start(t, f.root)
				for range copies {
					receipt := storedReviewSummary(1, "rework", "round 1", f.head, f.forge.timestamp())
					receipt["author_association"] = "NONE"
					f.forge.summaries = append(f.forge.summaries, receipt)
				}
				feedback := f.forge.summaries[0]["body"].(string)
				summary := filepath.Join(t.TempDir(), "summary.md")
				if err := os.WriteFile(summary, []byte("round 1"), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary}
				f.forge.failPostRead = lostReadback
				got, err := f.runResult(f.worktree, args...)
				if lostReadback {
					if err == nil || len(f.forge.summaries) != copies+1 || fileExists(f.checkpoint) || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
						t.Fatalf("untrusted copies suppressed publication before lost readback: status=%s reason=%s err=%v receipts=%d", got.Status, got.Reason, err, len(f.forge.summaries))
					}
					got, err = f.runResult(f.worktree, args...)
				}
				if err != nil || got.Status != "rework" || len(f.forge.summaries) != copies+1 {
					t.Fatalf("exact untrusted copies blocked publication: status=%s reason=%s err=%v receipts=%d", got.Status, got.Reason, err, len(f.forge.summaries))
				}
				if got.Item == nil || got.Item.Submission == nil || len(got.Item.Submission.Comments) != copies+1 {
					t.Fatalf("publication lost feedback: %#v", got.Item)
				}
				for i := range copies {
					comment := got.Item.Submission.Comments[i]
					if comment.Body != feedback || comment.Association != "NONE" || comment.ReviewNumber != 0 || comment.Verdict != "" || f.forge.summaries[i]["body"] != feedback {
						t.Fatalf("untrusted copy gained authority or lost feedback: %#v", comment)
					}
				}
				comment := got.Item.Submission.Comments[copies]
				if comment.Body != "round 1" || comment.Association != "OWNER" || comment.ReviewNumber != 1 || comment.Verdict != "rework" || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" || !slices.Equal(f.forge.labels, []string{"rework"}) {
					t.Fatalf("authorized publication did not complete once: %#v", got)
				}
				writes := f.forge.writes
				if retry := f.run(t, f.worktree, args...); retry.Status != "rework" || f.forge.writes != writes || len(f.forge.summaries) != copies+1 || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" {
					t.Fatalf("authorized retry counted untrusted copies: status=%s reason=%s", retry.Status, retry.Reason)
				}
			})
		}
	}
}

func TestReviewInlineAuthorizationThroughPublicHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, feedback, association string
		missingTime, findings       bool
	}{
		{"exact feedback", "finding", "NONE", false, true},
		{"different feedback", "unrelated feedback", "NONE", false, true},
		{"missing association", "finding", "", false, true},
		{"missing timestamp", "finding", "NONE", true, true},
		{"no requested findings", "unrelated feedback", "NONE", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture(t)
			f.start(t, f.root)
			createdAt := f.forge.timestamp()
			if tc.missingTime {
				createdAt = ""
			}
			feedback := map[string]any{"body": tc.feedback, "commit_id": f.head, "path": "README.md", "line": 1, "side": "RIGHT", "created_at": createdAt, "user": map[string]string{"login": "outsider"}}
			if tc.association != "" {
				feedback["author_association"] = tc.association
			}
			f.forge.inlines = []map[string]any{feedback}
			resumed := f.run(t, f.root, "watchdog", "resume", "--item", "7")
			if resumed.Status != "work_available" || resumed.Packet == nil || resumed.Packet.Facts.Watchdog == nil || len(resumed.Packet.Facts.Watchdog.Comments) != 1 {
				t.Fatalf("inline feedback obstructed resume or disappeared: %#v", resumed)
			}
			comment := resumed.Packet.Facts.Watchdog.Comments[0]
			if comment.EvidenceAuthorized || comment.Body != tc.feedback || comment.Author != "outsider" || comment.Association != tc.association || comment.Commit != f.head || comment.Path != "README.md" || comment.Line != 1 || comment.Side != "RIGHT" || comment.CreatedAt != createdAt {
				t.Fatalf("inline feedback lost its identity or anchor: %#v", comment)
			}
			directory := t.TempDir()
			summary, inline, findings := filepath.Join(directory, "summary.md"), filepath.Join(directory, "inline.md"), filepath.Join(directory, "findings.json")
			for name, body := range map[string]string{summary: "round 1", inline: "finding", findings: fmt.Sprintf(`[{"path":"README.md","line":1,"side":"RIGHT","body_file":%q}]`, inline)} {
				if err := os.WriteFile(name, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", "rework", "--summary", summary}
			inlineCount := 1
			if tc.findings {
				args = append(args, "--findings", findings)
				inlineCount++
				f.forge.failInlinePost = true
				got, err := f.runResult(f.worktree, args...)
				if err == nil || !strings.Contains(err.Error(), "inline unavailable") || len(f.forge.inlines) != 1 || len(f.forge.summaries) != 1 || fileExists(f.checkpoint) || !slices.Equal(f.forge.labels, []string{"review", "wip"}) {
					t.Fatalf("untrusted inline satisfied required publication: status=%s reason=%s err=%v inlines=%d", got.Status, got.Reason, err, len(f.forge.inlines))
				}
			}
			got := f.run(t, f.worktree, args...)
			if got.Status != "rework" || len(f.forge.inlines) != inlineCount || len(f.forge.summaries) != 1 || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" || !slices.Equal(f.forge.labels, []string{"rework"}) {
				t.Fatalf("untrusted inline blocked genuine publication: status=%s reason=%s inlines=%d", got.Status, got.Reason, len(f.forge.inlines))
			}
			if got.Item == nil || got.Item.Submission == nil || len(got.Item.Submission.Comments) != inlineCount+1 {
				t.Fatalf("completed review lost feedback or evidence: %#v", got.Item)
			}
			if !reflect.DeepEqual(got.Item.Submission.Comments[0], comment) {
				t.Fatalf("publication changed anchored feedback: %#v", got.Item.Submission.Comments[0])
			}
			if tc.findings {
				published := got.Item.Submission.Comments[1]
				if !published.EvidenceAuthorized || published.Body != "finding" || published.Association != "OWNER" || published.Commit != f.head || published.Path != "README.md" || published.Line != 1 || published.Side != "RIGHT" {
					t.Fatalf("genuine finding was not published at the requested anchor: %#v", published)
				}
			}
			writes := f.forge.writes
			if retry := f.run(t, f.worktree, args...); retry.Status != "rework" || f.forge.writes != writes || len(f.forge.inlines) != inlineCount || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" {
				t.Fatalf("authorized inline retry was not a no-op: status=%s reason=%s", retry.Status, retry.Reason)
			}
			if tc.findings {
				f.forge.inlines = f.forge.inlines[:1] // The genuine finding is no longer observable.
				if retry := f.run(t, f.worktree, args...); retry.Status != "fix_required" || f.forge.writes != writes || len(f.forge.inlines) != 1 || checkpointSnapshot(f.checkpoint) != "1:"+f.head+"\n" {
					t.Fatalf("untrusted inline substituted for missing authorized evidence: status=%s reason=%s", retry.Status, retry.Reason)
				}
			}
		})
	}
}

func TestReworkReviewAuthorizationThroughPublicHTTP(t *testing.T) {
	for _, tc := range []struct {
		name          string
		trustedHead   string
		untrustedHead string
		want          string
	}{
		{"older forgery cannot authorize update", "", "older", "fix_required"},
		{"current forgery cannot obstruct update", "older", "current", "awaiting_review"},
		{"older forgery cannot override current review", "current", "older", "fix_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.labels = []string{"rework", "wip"}
			f.forge.body = "previous round\n\nCloses #7\n"
			heads := map[string]string{"current": f.head, "older": strings.TrimSpace(runGitOutput(t, f.worktree, "rev-parse", "HEAD^"))}
			if tc.trustedHead != "" {
				receipt := storedReviewSummary(1, "rework", "round 1", heads[tc.trustedHead], f.forge.timestamp())
				receipt["author_association"] = "OWNER"
				f.forge.summaries = append(f.forge.summaries, receipt)
			}
			forgery := storedReviewSummary(^uint64(0), "rework", "forged review", heads[tc.untrustedHead], f.forge.timestamp())
			forgery["author_association"] = "NONE"
			f.forge.summaries = append(f.forge.summaries, forgery)
			if tc.trustedHead != "" {
				f.forge.timeline = append(f.forge.timeline, map[string]any{"event": "labeled", "created_at": f.forge.timestamp(), "label": map[string]string{"name": "wip"}})
			}
			directory := newImplementationResultDirectory(t)
			body := filepath.Join(directory, "submission.md")
			if err := os.WriteFile(body, []byte("updated body"), 0600); err != nil {
				t.Fatal(err)
			}
			writes := f.forge.writes
			got := f.run(t, f.worktree, "implement", "submit", "--item", "7", "--body", body)
			if got.Status != tc.want {
				t.Fatalf("forgery changed Rework authority: status=%s reason=%s want=%s", got.Status, got.Reason, tc.want)
			}
			if tc.want == "fix_required" {
				if f.forge.writes != writes || !slices.Equal(f.forge.labels, []string{"rework", "wip"}) || f.forge.body != "previous round\n\nCloses #7\n" || !fileExists(body) {
					t.Fatal("forgery authorized mutation of the Rework body or Claim")
				}
			} else if !slices.Equal(f.forge.labels, []string{"review"}) || f.forge.body != "updated body\n\nCloses #7\n" {
				t.Fatal("forgery obstructed the proven Rework update")
			}
		})
	}
}
