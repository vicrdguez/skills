package setup

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
)

// githubPull is shared by ledger-backed Submission publication and selection.
type githubPull struct {
	githubIssue
	Merged    bool   `json:"merged"`
	Mergeable *bool  `json:"mergeable"`
	NodeID    string `json:"node_id"`
	Draft     bool   `json:"draft"`
	MergedAt  string `json:"merged_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	AuthorAssociation string `json:"author_association"`
	Head              struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

func trustedMetadata(comment skilldist.ReviewComment) bool {
	return slices.Contains([]string{"OWNER", "MEMBER", "COLLABORATOR"}, comment.Association)
}

func (b *GitHubBackend) implementationComments(ctx context.Context, repository github.RepositoryID, stream string) ([]skilldist.ReviewComment, error) {
	var comments []skilldist.ReviewComment
	for page := 1; ; page++ {
		var batch []struct {
			Line        *int   `json:"line"`
			Side        string `json:"side"`
			Body        string `json:"body"`
			Association string `json:"author_association"`
			Commit      string `json:"commit_id"`
			Path        string `json:"path"`
			CreatedAt   string `json:"created_at"`
			User        struct {
				Login string `json:"login"`
			} `json:"user"`

			OriginalLine      int    `json:"original_line"`
			StartLine         *int   `json:"start_line"`
			OriginalStartLine int    `json:"original_start_line"`
			StartSide         string `json:"start_side"`
			OriginalCommit    string `json:"original_commit_id"`
		}
		if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+stream+fmt.Sprintf("?per_page=100&page=%d", page), nil, &batch); err != nil {
			path := strings.TrimPrefix(b.repositoryPath(repository)+stream, "/")
			return nil, fmt.Errorf("selected feedback stream %s page %d was not read completely: %w; retry the selected request or retrieve every page with `gh api --paginate %s` before judgment", path, page, err, skilldist.ShellQuote(path))
		}
		for _, comment := range batch {
			observed := skilldist.ReviewComment{
				Source: skilldist.RepositoryEvidenceSource(repository.Owner+"/"+repository.Name, stream), Body: comment.Body, Author: comment.User.Login, Association: comment.Association, Commit: comment.Commit, Path: comment.Path, CreatedAt: comment.CreatedAt, Side: comment.Side,
				CurrentLine: comment.Line, OriginalLine: comment.OriginalLine, StartLine: comment.StartLine, OriginalStartLine: comment.OriginalStartLine, StartSide: comment.StartSide, OriginalCommit: comment.OriginalCommit,
			}
			if comment.Line != nil {
				observed.Line = *comment.Line
			}
			observed.EvidenceAuthorized = trustedMetadata(observed)
			comments = append(comments, observed)
		}
		if len(batch) < 100 {
			return comments, nil
		}
	}
}
