package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestW5PacketUsesSubmissionIdentityNotFeedback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rework   bool
		feedback bool
	}{
		{"ready with feedback", false, true},
		{"rework with feedback", true, true},
		{"rework without feedback", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `widget`\n")
			if tc.rework {
				forge.addPull(30, "2021-01-01T00:00:00Z", "review\n\nCloses #7\n", "widget", strings.Repeat("a", 40), "rework")
				forge.reworkPages = [][]int{{30}}
			} else {
				forge.addLabel(7, "ready")
				forge.readyPages = [][]int{{7}}
			}
			if tc.feedback {
				forge.comments["/issues/7/comments"] = []map[string]any{{"body": "human context, not a lifecycle signal", "author_association": "OWNER", "user": map[string]string{"login": "maintainer"}}}
			}
			got, err := selectionRun(t, root, forge, "implement", "next")
			if err != nil || got.Status != "work_available" || got.Packet == nil {
				t.Fatalf("start = %#v, %v", got, err)
			}
			facts := got.Packet.Facts.Implementation
			if facts.WorkItem != 7 || (facts.Submission != 0) != tc.rework {
				t.Fatalf("packet identity = %#v", facts)
			}
			if tc.feedback && (len(facts.Comments) != 1 || facts.Comments[0].Body != "human context, not a lifecycle signal" || facts.Comments[0].Author != "maintainer" || facts.Comments[0].Association != "OWNER") {
				t.Fatalf("source feedback lost: %#v", facts.Comments)
			}
			for _, guidance := range []string{"Finding-driven Rework", "Keep the ledger retired"} {
				if strings.Contains(got.Packet.Instructions, guidance) != tc.rework {
					t.Errorf("%q guidance present = %v, want %v", guidance, !tc.rework, tc.rework)
				}
			}
		})
	}
}

func TestW11PacketPreservesOutdatedAndMultilineAnchors(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		t.Run(lane, func(t *testing.T) {
			root := selectionRepository(t)
			forge := newCandidateForge()
			forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `widget`\n")
			label := "rework"
			if lane == "watchdog" {
				label = "review"
				forge.reviewPages = [][]int{{30}}
			} else {
				forge.reworkPages = [][]int{{30}}
			}
			forge.addPull(30, "2021-01-01T00:00:00Z", "review\n\nCloses #7\n", "widget", strings.Repeat("a", 40), label)
			forge.comments["/pulls/30/comments"] = []map[string]any{
				{"body": "current multiline\nraw evidence", "path": "main.go", "line": 42, "original_line": 35, "start_line": 40, "original_start_line": 33, "side": "LEFT", "start_side": "LEFT", "commit_id": strings.Repeat("a", 40), "original_commit_id": strings.Repeat("b", 40), "created_at": "2021-01-01T00:00:00Z", "author_association": "OWNER", "user": map[string]string{"login": "maintainer"}},
				{"body": "outdated multiline\nraw evidence", "path": "old.go", "line": nil, "original_line": 19, "start_line": nil, "original_start_line": 16, "side": "RIGHT", "start_side": "RIGHT", "commit_id": strings.Repeat("c", 40), "original_commit_id": strings.Repeat("d", 40), "created_at": "2020-01-01T00:00:00Z", "author_association": "COLLABORATOR", "user": map[string]string{"login": "reviewer"}},
			}
			got, err := selectionRun(t, root, forge, lane, "next")
			if err != nil || got.Status != "work_available" || got.Packet == nil {
				t.Fatalf("start = %#v, %v", got, err)
			}
			var encoded []byte
			if f := got.Packet.Facts.Implementation; f != nil {
				encoded, err = json.Marshal(f.Comments)
			} else {
				encoded, err = json.Marshal(got.Packet.Facts.Watchdog.Comments)
			}
			if err != nil {
				t.Fatal(err)
			}
			var observations []map[string]any
			if err := json.Unmarshal(encoded, &observations); err != nil {
				t.Fatal(err)
			}
			want := []map[string]any{
				{"body": "current multiline\nraw evidence", "path": "main.go", "line": float64(42), "current_line": float64(42), "original_line": float64(35), "start_line": float64(40), "original_start_line": float64(33), "side": "LEFT", "start_side": "LEFT", "commit": strings.Repeat("a", 40), "original_commit": strings.Repeat("b", 40), "created_at": "2021-01-01T00:00:00Z", "association": "OWNER", "author": "maintainer"},
				{"body": "outdated multiline\nraw evidence", "path": "old.go", "current_line": nil, "original_line": float64(19), "start_line": nil, "original_start_line": float64(16), "side": "RIGHT", "start_side": "RIGHT", "commit": strings.Repeat("c", 40), "original_commit": strings.Repeat("d", 40), "created_at": "2020-01-01T00:00:00Z", "association": "COLLABORATOR", "author": "reviewer"},
			}
			if len(observations) != len(want) {
				t.Fatalf("inline observations = %s", encoded)
			}
			for i, fields := range want {
				for key, value := range fields {
					if observed, ok := observations[i][key]; !ok || !reflect.DeepEqual(observed, value) {
						t.Errorf("comment %d %s = %#v (present=%v), want %#v", i, key, observed, ok, value)
					}
				}
			}
			if line := observations[1]["line"]; line != nil && line != float64(0) {
				t.Fatalf("outdated original anchor became a live publication anchor: %#v", line)
			}
		})
	}
}

func TestW8PacketPreparationInSingleBranchClone(t *testing.T) {
	for _, lane := range []string{"implement", "watchdog"} {
		t.Run(lane, func(t *testing.T) {
			remote := proposalRepository(t)
			baseline := prepareSlice(t, remote, "widget")
			head, phase := baseline, "present"
			if lane == "watchdog" {
				head, phase = completeAndRetireSlice(t, remote, "widget"), "retired"
			}
			runGit(t, remote, "switch", "main")
			parent := t.TempDir()
			root := filepath.Join(parent, "worker's clone")
			runGit(t, parent, "clone", "--single-branch", "--branch", "main", "file://"+remote, root)
			runGit(t, root, "remote", "set-url", "origin", "git@github.com:acme/widgets.git")
			if got := strings.TrimSpace(runGitOutput(t, root, "config", "--get-all", "remote.origin.fetch")); got != "+refs/heads/main:refs/remotes/origin/main" {
				t.Fatalf("not a single-branch clone: %s", got)
			}
			if exec.Command("git", "-C", root, "cat-file", "-e", head+"^{commit}").Run() == nil {
				t.Fatal("selected history already exists before preparation")
			}
			main := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
			forge := newCandidateForge()
			forge.addIssue(7, "2020-01-01T00:00:00Z", "Branch: `widget`\n")
			if lane == "implement" {
				forge.addLabel(7, "ready")
				forge.readyPages = [][]int{{7}}
			} else {
				forge.addPull(30, "2021-01-01T00:00:00Z", "review\n\nCloses #7\n", "widget", head, "review")
				forge.reviewPages = [][]int{{30}}
			}
			got, err := selectionRun(t, root, forge, lane, "next")
			if err != nil || got.Status != "work_available" || got.Packet == nil {
				t.Fatalf("start = %#v, %v", got, err)
			}
			var fetch, worktree, inspect, directory string
			if f := got.Packet.Facts.Implementation; f != nil {
				fetch, worktree, inspect, directory = f.FetchCommand, f.WorktreeCommand, f.InspectCommand, f.Worktree
			} else {
				f := got.Packet.Facts.Watchdog
				fetch, worktree, inspect, directory = f.FetchCommand, f.WorktreeCommand, f.InspectCommand, f.Worktree
			}
			// Only Git execution redirects to the local transport; the CLI still
			// observes the GitHub attachment through its real adapter.
			run := func(command string) ([]byte, error) {
				cmd := exec.Command("sh", "-c", command)
				cmd.Env = append(os.Environ(), "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=url.file://"+remote+".insteadOf", "GIT_CONFIG_VALUE_0=git@github.com:acme/widgets.git")
				return cmd.CombinedOutput()
			}
			if output, err := run(fetch); err != nil {
				t.Fatalf("packet fetch %q: %v\n%s", fetch, err, output)
			}
			if gitRefExists(root, "refs/heads/widget") {
				t.Fatal("fetch created a local branch")
			}
			if output, err := run(worktree); err != nil {
				t.Fatalf("packet worktree %q: %v\n%s", worktree, err, output)
			}
			if got := strings.TrimSpace(runGitOutput(t, directory, "rev-parse", "HEAD")); got != head {
				t.Fatalf("prepared head = %s, want %s", got, head)
			}
			// Let the shell unquote the supplied inspect command, then execute
			// those arguments through the same pinned CLI/HTTP seam as next.
			argv, err := exec.Command("sh", "-c", "printf '%s\\000' "+inspect).Output()
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Split(strings.TrimSuffix(string(argv), "\x00"), "\x00")
			repoFlag := slices.Index(args, "--repo")
			if args[0] != "skl" || repoFlag < 0 || repoFlag+1 >= len(args) {
				t.Fatalf("invalid inspect command: %q", inspect)
			}
			inspected, err := selectionRun(t, args[repoFlag+1], forge, args[1:]...)
			if err != nil || inspected.Status != "inspected" || inspected.Head != head || inspected.Ledger == nil {
				t.Fatalf("packet inspect %q = %#v, %v", inspect, inspected, err)
			}
			if inspected.Ledger.Baseline != baseline || inspected.Ledger.Phase != phase || len(inspected.Ledger.Violations) != 0 {
				t.Fatalf("prepared artifacts = %#v", inspected.Ledger)
			}
			if got := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD")); got != main {
				t.Fatalf("preparation moved the primary branch: %s", got)
			}
			runGit(t, directory, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "local progress")
			progress := strings.TrimSpace(runGitOutput(t, directory, "rev-parse", "HEAD"))
			file := filepath.Join(directory, "unfinished.txt")
			if err := os.WriteFile(file, []byte("unfinished\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := run(fetch); err != nil {
				t.Fatalf("repeat fetch: %v\n%s", err, output)
			}
			if output, err := run(worktree); err == nil {
				t.Fatalf("worktree command overwrote existing progress: %s", output)
			}
			if got := strings.TrimSpace(runGitOutput(t, directory, "rev-parse", "HEAD")); got != progress || readFile(t, file) != "unfinished\n" {
				t.Fatalf("preparation discarded local progress: %s", got)
			}
		})
	}
}
