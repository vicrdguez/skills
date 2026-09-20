package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

func TestCleanupRequiresMergedOwningSubmissionThroughGitHub(t *testing.T) {
	for _, test := range []struct {
		name, body, headRepository string
		merged                     bool
		remove                     bool
	}{
		{"ordinary mention by another owner", "Related to #17.\n\nCloses #99\n", "acme/widgets", true, false},
		{"ordinary mention without owner", "Related to #17.\n", "acme/widgets", true, false},
		{"conflicting owners", "Closes #17\nCloses #99\n", "acme/widgets", true, false},
		{"foreign owner", "Closes other/repository#17\n", "acme/widgets", true, false},
		{"unmerged owner", "Closes #17\n", "acme/widgets", false, false},
		{"merged owner with renamed titles", "Result\n\nCloses #17\n", "Acme/Widgets", true, true},
		{"merged fork owner", "Closes #17\n", "contributor/widgets", true, false},
		{"unknown head repository", "Closes #17\n", "", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := proposalRepository(t)
			path := filepath.Join(root, ".worktrees", "submission-branch")
			runGit(t, root, "worktree", "add", path, "-b", "submission-branch", "main")
			head := strings.TrimSpace(runGitOutput(t, path, "rev-parse", "HEAD"))
			if status := runGitOutput(t, path, "status", "--porcelain"); status != "" {
				t.Fatalf("fixture worktree is not clean: %q", status)
			}
			client := &http.Client{Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet {
					t.Fatalf("cleanup mutated GitHub: %s %s", request.Method, request.URL)
				}
				var body string
				switch request.URL.Path {
				case "/repos/acme/widgets/issues":
					body = `[{"number":17,"title":"Renamed Work Item","state":"closed","labels":[]}]`
				case "/repos/acme/widgets/issues/17/timeline":
					body = `[{"event":"labeled","label":{"name":"ready"}},{"event":"unlabeled","label":{"name":"ready"}},{"event":"closed","commit_id":null},{"event":"referenced","commit_id":"squash"}]`
				case "/repos/acme/widgets/commits/squash/pulls":
					mergedAt := ""
					if test.merged {
						mergedAt = "2026-09-16T10:00:00Z"
					}
					body = fmt.Sprintf(`[{"number":21,"title":"Renamed Submission","body":%q,"state":"closed","merged_at":%q,"merge_commit_sha":"squash","head":{"ref":"submission-branch","sha":%q,"repo":{"full_name":%q}}}]`, test.body, mergedAt, head, test.headRepository)
				default:
					t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			var output bytes.Buffer
			app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
				backend := setup.NewGitHubBackend("https://api.github.test", "secret", client)
				backend.BindRepository(repository)
				return backend, nil
			}, bytes.NewReader(nil), &output, &output)
			if err := app.Run([]string{"skl", "propose", "cleanup", "--repo", root}); err != nil {
				t.Fatal(err)
			}
			if test.remove {
				if _, err := os.Stat(path); !os.IsNotExist(err) || gitRefExists(root, "refs/heads/submission-branch") {
					t.Fatalf("owning merged Submission was not cleaned up: worktree error=%v report=%q", err, output.String())
				}
				if output.String() != "removed submission-branch\n" {
					t.Fatalf("cleanup report = %q", output.String())
				}
				return
			}
			if _, err := os.Stat(path); err != nil || !gitRefExists(root, "refs/heads/submission-branch") {
				t.Fatalf("cleanup deleted a branch without owning merged Submission evidence: worktree error=%v report=%q", err, output.String())
			}
			if got := strings.TrimSpace(runGitOutput(t, path, "rev-parse", "HEAD")); got != head || strings.Contains(output.String(), "removed") {
				t.Fatalf("cleanup changed unrelated local work: head=%q report=%q", got, output.String())
			}
		})
	}
}
