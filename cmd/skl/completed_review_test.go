package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestCompletedWatchdogHandoffRequiresFinalProjectionsThroughPublicHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, verdict, stale string
		conflicting          bool
	}{
		{"rework source pause", "rework", "source", false},
		{"pass source pause", "pass", "source", false},
		{"rework stale synchronization", "rework", "sync", false},
		{"pass stale synchronization", "pass", "sync", false},
		{"pause stale synchronization", "needs-human", "sync", false},
		{"complete rework", "rework", "", false},
		{"complete pass", "pass", "", false},
		{"complete pause", "needs-human", "", false},
		{"complete conflicting pass", "pass", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.mergeable = !tc.conflicting
			start := f.start(t, f.root)
			first := f.submit(t, 1, f.head, tc.verdict)
			if first.Status != "rework" && first.Status != "ready_for_merge" && first.Status != "needs_human" {
				t.Fatalf("first completion: %#v", first)
			}
			if tc.stale == "source" {
				f.forge.sourceLabels = []string{"needs-human"}
			} else if tc.stale == "sync" {
				f.forge.labels = append(f.forge.labels, "sync")
			}
			directory := start.Packet.Facts.Watchdog.ResultDirectory
			summary, body := filepath.Join(directory, "summary.md"), filepath.Join(directory, "submission.md")
			if err := os.WriteFile(summary, []byte("round 1"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"watchdog", "submit", "--item", "7", "--review-number", "1", "--reviewed-head", f.head, "--verdict", tc.verdict, "--summary", summary}
			if tc.verdict == "pass" {
				if err := os.WriteFile(body, []byte("final"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--body", body)
			}
			before, writes := f.evidenceSnapshot(t), f.forge.writes
			got := f.run(t, f.worktree, args...)
			if tc.stale != "" {
				if got.Status != "fix_required" || got.Reason == "" || readFile(t, summary) != "round 1" || tc.verdict == "pass" && readFile(t, body) != "final" {
					t.Fatalf("incomplete destination accepted or documents lost: %#v", got)
				}
			} else if got.Status != first.Status || fileExists(directory) {
				t.Fatalf("complete unclaimed destination not recognized: %#v", got)
			}
			if f.forge.writes != writes || !reflect.DeepEqual(before, f.evidenceSnapshot(t)) {
				t.Fatal("completed retry mutated backend evidence, labels, or checkpoint")
			}
		})
	}
}

func TestWatchdogRemovesObsoleteSynchronizationBeforeReleaseThroughPublicHTTP(t *testing.T) {
	for _, verdict := range []string{"rework", "pass", "needs-human"} {
		t.Run(verdict, func(t *testing.T) {
			f := newReviewFixture(t)
			f.forge.noOther = true
			f.forge.labels = []string{"review", "sync"}
			start := f.start(t, f.root)
			directory := start.Packet.Facts.Watchdog.ResultDirectory
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
			f.forge.rejectMutation = "DELETE /issues/11/labels/sync"
			if _, err := f.runResult(f.worktree, args...); err == nil {
				t.Fatal("obsolete synchronization cleanup failure was not reported")
			}
			f.assertNoMutationAfter(t, "DELETE /issues/11/labels/sync")
			f.forge.afterMutation = nil
			if !slices.Contains(f.forge.labels, "wip") || !slices.Contains(f.forge.labels, "sync") || readFile(t, summary) != "verdict" || len(f.forge.summaries) != 1 {
				t.Fatal("cleanup failure lost protection or accepted evidence")
			}
		})
	}
}
