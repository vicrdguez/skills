package setup

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	skilldist "github.com/vicrdguez/skills"
)

// submissionOwner extracts the engine-supplied closing reference from the end
// of a Submission body. Ordinary prose never assigns ownership.
func submissionOwner(body string) (int, string) {
	owner := 0
	found := false
	for _, line := range slices.Backward(strings.Split(strings.TrimRight(body, "\n"), "\n")) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rest, ok := strings.CutPrefix(line, "Closes ")
		if !ok {
			break
		}
		if !strings.HasPrefix(rest, "#") {
			return 0, "ownership reference outside the supported repository attachment"
		}
		digits := strings.TrimPrefix(rest, "#")
		number, err := strconv.Atoi(digits)
		if err != nil || number <= 0 || strconv.Itoa(number) != digits {
			return 0, "ownership reference outside the supported repository attachment"
		}
		if found {
			return 0, "multiple conflicting owning issues"
		}
		owner, found = number, true
	}
	if !found {
		return 0, "no explicit owning issue"
	}
	return owner, ""
}

func (b *GitHubBackend) implementationReviews(ctx context.Context, number int) ([]skilldist.ReviewComment, error) {
	var comments []skilldist.ReviewComment
	for page := 1; ; page++ {
		var reviews []struct {
			State       string `json:"state"`
			Commit      string `json:"commit_id"`
			Body        string `json:"body"`
			Association string `json:"author_association"`
			SubmittedAt string `json:"submitted_at"`
			User        struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if err := b.request(ctx, http.MethodGet, b.repositoryPath(b.repository)+fmt.Sprintf("/pulls/%d/reviews?per_page=100&page=%d", number, page), nil, &reviews); err != nil {
			path := strings.TrimPrefix(b.repositoryPath(b.repository)+fmt.Sprintf("/pulls/%d/reviews", number), "/")
			return nil, fmt.Errorf("selected review summary stream %s page %d was not read completely: %w; retry the selected request or retrieve every page with `gh api --paginate %s` before judgment", path, page, err, skilldist.ShellQuote(path))
		}
		for _, review := range reviews {
			verdict := map[string]string{"CHANGES_REQUESTED": "rework", "APPROVED": "pass", "COMMENTED": "needs-human"}[review.State]
			body := review.Body
			finalHead := ""
			reviewNumber := uint64(0)
			if !trustedMetadata(skilldist.ReviewComment{Association: review.Association}) {
				verdict = ""
			} else if strings.HasPrefix(body, reviewSummaryPrefix) {
				verdict = ""
				if metadata, summary, ok := parseReviewSummary(body); ok && review.State == "COMMENTED" {
					body, verdict, reviewNumber = summary, metadata.Verdict, metadata.ReviewNumber
					finalHead = metadata.FinalHead
				}
			}
			comment := skilldist.ReviewComment{
				Source:       skilldist.PullReviewsEvidenceSource(b.repository.Owner+"/"+b.repository.Name, number),
				Body:         body,
				Author:       review.User.Login,
				Association:  review.Association,
				Commit:       review.Commit,
				FinalHead:    finalHead,
				CreatedAt:    review.SubmittedAt,
				Verdict:      verdict,
				ReviewNumber: reviewNumber,
			}
			comments = append(comments, comment)
		}
		if len(reviews) < 100 {
			return comments, nil
		}
	}
}
