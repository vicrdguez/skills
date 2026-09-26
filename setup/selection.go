package setup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
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

// declaredBranch reads the explicit proposal attachment fact from a Work Item
// body. Renaming titles never changes it.
func declaredBranch(body string) (string, string) {
	branch, problem := "", ""
	for _, line := range strings.Split(body, "\n") {
		rest, ok := strings.CutPrefix(line, "Branch: `")
		if !ok {
			continue
		}
		value, ok := strings.CutSuffix(strings.TrimSpace(rest), "`")
		if !ok || value == "" {
			problem = "invalid branch attachment; repair the Work Item body"
			continue
		}
		if branch != "" && branch != value {
			return "", "ambiguous branch attachment; repair the Work Item body"
		}
		branch = value
	}
	return branch, problem
}

func (b *GitHubBackend) pullRecord(ctx context.Context, number int) (githubPull, error) {
	var pull githubPull
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(b.repository)+fmt.Sprintf("/pulls/%d", number), nil, &pull); err != nil {
		return githubPull{}, err
	}
	return pull, nil
}

type closingReference struct {
	Number     int    `json:"number"`
	Merged     bool   `json:"merged"`
	MergedAt   string `json:"mergedAt"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

// closingReferences observes GitHub's explicit issue/PR references for one
// issue, following every continuation page.
func (b *GitHubBackend) closingReferences(ctx context.Context, issueNumber int, includeClosed bool) ([]closingReference, error) {
	closedPrs := "false"
	if includeClosed {
		closedPrs = "true"
	}
	var references []closingReference
	after := ""
	seen := make(map[string]bool)
	for {
		var response struct {
			Data struct {
				Repository struct {
					Issue *struct {
						ClosedByPullRequestsReferences struct {
							Nodes    []closingReference `json:"nodes"`
							PageInfo struct {
								HasNextPage bool   `json:"hasNextPage"`
								EndCursor   string `json:"endCursor"`
							} `json:"pageInfo"`
						} `json:"closedByPullRequestsReferences"`
					} `json:"issue"`
				} `json:"repository"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		query := "query($owner:String!,$name:String!,$number:Int!,$after:String){repository(owner:$owner,name:$name){issue(number:$number){closedByPullRequestsReferences(first:100,includeClosedPrs:" + closedPrs + ",after:$after){nodes{number merged mergedAt repository{nameWithOwner}}pageInfo{hasNextPage endCursor}}}}}"
		variables := map[string]any{"owner": b.repository.Owner, "name": b.repository.Name, "number": issueNumber}
		if after != "" {
			variables["after"] = after
		}
		if err := b.request(ctx, http.MethodPost, "/graphql", map[string]any{"query": query, "variables": variables}, &response); err != nil {
			return nil, err
		}
		if len(response.Errors) != 0 {
			return nil, errors.New(response.Errors[0].Message)
		}
		if response.Data.Repository.Issue == nil {
			return nil, workflow.Refuse("owning-link relationship for #" + strconv.Itoa(issueNumber) + " is unavailable; inspect the Work Item before retrying")
		}
		page := response.Data.Repository.Issue.ClosedByPullRequestsReferences
		for _, reference := range page.Nodes {
			if !strings.EqualFold(reference.Repository.NameWithOwner, b.repository.Owner+"/"+b.repository.Name) {
				return nil, workflow.Refuse("owning-link relationship for #" + strconv.Itoa(issueNumber) + " is outside the supported repository attachment; inspect and repair its association")
			}
		}
		references = append(references, page.Nodes...)
		if !page.PageInfo.HasNextPage {
			return references, nil
		}
		if page.PageInfo.EndCursor == "" || seen[page.PageInfo.EndCursor] {
			return nil, errors.New("owning-link pagination cursor is missing or repeated; required observation is incomplete")
		}
		seen[page.PageInfo.EndCursor] = true
		after = page.PageInfo.EndCursor
	}
}

// verifyOwningAssociation requires exactly this Submission, or no association
// when submissionNumber is zero before first publication.
func (b *GitHubBackend) verifyOwningAssociation(ctx context.Context, issueNumber, submissionNumber int) error {
	references, err := b.closingReferences(ctx, issueNumber, false)
	if err != nil {
		return err
	}
	present := submissionNumber == 0
	for _, reference := range references {
		if reference.Number != submissionNumber {
			return workflow.Refuse("another active Submission already owns Work Item #" + strconv.Itoa(issueNumber) + "; retain the Claim and inspect its association before retrying")
		}
		present = true
	}
	if !present {
		return workflow.Refuse("owning-link relationship for #" + strconv.Itoa(issueNumber) + " does not observe Submission #" + strconv.Itoa(submissionNumber) + "; inspect the association before retrying")
	}
	return nil
}

// dependencyReferences observes one Ready Work Item's referenced blockers from
// its native relationships and the supported explicit declaration. A non-empty
// problem reports an invalid legacy projection; inaccessible native relationships
// cannot establish the complete set of blockers.
func (b *GitHubBackend) dependencyReferences(ctx context.Context, number int, body string) ([]int, string, error) {
	var blockers []int
	for page := 1; ; page++ {
		var batch []githubIssue
		path := b.repositoryPath(b.repository) + fmt.Sprintf("/issues/%d/dependencies/blocked_by?per_page=100&page=%d", number, page)
		if err := b.request(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return nil, "", err
		}
		for _, blocker := range batch {
			if !slices.Contains(blockers, blocker.Number) {
				blockers = append(blockers, blocker.Number)
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	// Adopt the former workflow's explicit dependency projection only.
	for _, line := range strings.Split(body, "\n") {
		rest, ok := strings.CutPrefix(line, "Blocked by: ")
		if !ok {
			continue
		}
		for _, value := range strings.Split(rest, ",") {
			blocker, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(value), "#"))
			if err != nil || blocker <= 0 {
				return nil, "invalid legacy Dependency projection", nil
			}
			if !slices.Contains(blockers, blocker) {
				blockers = append(blockers, blocker)
			}
		}
	}
	return blockers, "", nil
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

func firstProblem(current, next string) string {
	if current != "" {
		return current
	}
	return next
}
