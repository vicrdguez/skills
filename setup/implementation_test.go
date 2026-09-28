package setup

import (
	"testing"

	"github.com/vicrdguez/skills/github"
)

func boundGitHubBackend(backend *GitHubBackend) *GitHubBackend {
	backend.BindRepository(github.RepositoryID{Owner: "acme", Name: "widgets"})
	return backend
}

func TestSubmissionOwnerAcceptsOnlyOneLocalClosingFooter(t *testing.T) {
	for body, want := range map[string]string{
		"opening\n\nCloses #7\n":              "",
		"unowned body":                        "no explicit owning issue",
		"opening\n\nCloses #7\nCloses #8\n":   "multiple conflicting owning issues",
		"opening\n\nCloses acme/other#7\n":    "ownership reference outside the supported repository attachment",
		"opening\n\nCloses #7\n\nCloses #8\n": "multiple conflicting owning issues",
	} {
		owner, problem := submissionOwner(body)
		if problem != want || want == "" && owner != 7 || want != "" && owner != 0 {
			t.Errorf("submissionOwner(%q) = %d, %q; want problem %q", body, owner, problem, want)
		}
	}
}
