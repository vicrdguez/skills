package setup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

func queueLabel(queue workflow.QueueKind) string {
	return string(queue)
}

// QueuePage observes one page of an open, queue-labelled selection stream.
func (b *GitHubBackend) QueuePage(ctx context.Context, queue workflow.QueueKind, cursor string) (workflow.QueuePage, error) {
	if err := b.requireRepository(); err != nil {
		return workflow.QueuePage{}, err
	}
	if queue == workflow.ReadyQueue {
		return b.readyQueuePage(ctx, cursor)
	}
	return b.submissionQueuePage(ctx, queue, cursor)
}

func (b *GitHubBackend) readyQueuePage(ctx context.Context, cursor string) (workflow.QueuePage, error) {
	page, err := strconv.Atoi(cursor)
	if err != nil || page < 1 {
		page = 1
	}
	var issues []githubIssue
	path := fmt.Sprintf("%s/issues?state=open&labels=ready&sort=created&direction=asc&filter=all&per_page=100&page=%d", b.repositoryPath(b.repository), page)
	if err := b.request(ctx, http.MethodGet, path, nil, &issues); err != nil {
		return workflow.QueuePage{}, err
	}
	result := workflow.QueuePage{}
	for _, issue := range issues {
		if len(issue.PullRequest) != 0 || issue.SubIssuesSummary.Total > 0 {
			continue
		}
		claimed, paused, ready := false, false, false
		for _, label := range issue.Labels {
			claimed = claimed || label.Name == "wip"
			paused = paused || label.Name == "needs-human"
			ready = ready || label.Name == "ready"
		}
		if paused || !ready {
			continue
		}
		result.Candidates = append(result.Candidates, workflow.QueueCandidate{ID: workflow.WorkItemID(strconv.Itoa(issue.Number)), Number: issue.Number, CreatedAt: issue.CreatedAt, Claimed: claimed})
		b.issueIDs[issue.Number] = issue.ID
		b.issueBodies[issue.Number] = issue.Body
	}
	if len(issues) == 100 {
		result.Next = strconv.Itoa(page + 1)
	}
	return result, nil
}

func (b *GitHubBackend) submissionQueuePage(ctx context.Context, queue workflow.QueueKind, cursor string) (workflow.QueuePage, error) {
	var response struct {
		Data struct {
			Repository struct {
				PullRequests struct {
					Nodes []struct {
						CreatedAt string `json:"createdAt"`
						Number    int    `json:"number"`
						Labels    struct {
							Nodes []struct {
								Name string `json:"name"`
							} `json:"nodes"`
						} `json:"labels"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"pullRequests"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	query := "query($owner:String!,$name:String!,$label:[String!],$after:String){repository(owner:$owner,name:$name){pullRequests(states:OPEN,labels:$label,first:100,after:$after,orderBy:{field:CREATED_AT,direction:ASC}){nodes{number createdAt labels(first:100){nodes{name}}}pageInfo{hasNextPage endCursor}}}}"
	variables := map[string]any{"owner": b.repository.Owner, "name": b.repository.Name, "label": []string{queueLabel(queue)}}
	if cursor != "" {
		variables["after"] = cursor
	}
	if err := b.request(ctx, http.MethodPost, "/graphql", map[string]any{"query": query, "variables": variables}, &response); err != nil {
		return workflow.QueuePage{}, err
	}
	if len(response.Errors) != 0 {
		return workflow.QueuePage{}, errors.New(response.Errors[0].Message)
	}
	pulls := response.Data.Repository.PullRequests
	if pulls.PageInfo.HasNextPage && (pulls.PageInfo.EndCursor == "" || pulls.PageInfo.EndCursor == cursor) {
		return workflow.QueuePage{}, errors.New("queue pagination cursor is missing or did not advance; required observation is incomplete")
	}
	result := workflow.QueuePage{}
	for _, pull := range pulls.Nodes {
		if len(pull.Labels.Nodes) == 100 {
			return workflow.QueuePage{}, workflow.Refuse("label observation is truncated on Submission #" + strconv.Itoa(pull.Number) + "; repair the record before retrying")
		}
		claimed, paused, queued := false, false, false
		for _, label := range pull.Labels.Nodes {
			claimed = claimed || label.Name == "wip"
			paused = paused || label.Name == "needs-human"
			queued = queued || label.Name == queueLabel(queue)
		}
		if paused || !queued {
			continue
		}
		candidate := workflow.QueueCandidate{SubmissionID: workflow.SubmissionID(strconv.Itoa(pull.Number)), Number: pull.Number, CreatedAt: pull.CreatedAt, Claimed: claimed}
		result.Candidates = append(result.Candidates, candidate)
	}
	if pulls.PageInfo.HasNextPage {
		result.Next = pulls.PageInfo.EndCursor
	}
	return result, nil
}

// SelectedImplementation hydrates exactly the selected candidate: its source
// record, its explicit Submission attachment, and only its feedback streams.
func (b *GitHubBackend) SelectedImplementation(ctx context.Context, candidate workflow.QueueCandidate) (workflow.ImplementationItem, error) {
	if err := b.requireRepository(); err != nil {
		return workflow.ImplementationItem{}, err
	}
	if candidate.SubmissionID != "" {
		number, err := githubIssueNumber(workflow.WorkItemID(candidate.SubmissionID))
		if err != nil {
			return workflow.ImplementationItem{}, err
		}
		return b.selectedSubmission(ctx, number)
	}
	return b.selectedReady(ctx, candidate.ID)
}

// ResumedImplementation observes exactly one explicitly identified Work Item
// without enumerating any queue.
func (b *GitHubBackend) ResumedImplementation(ctx context.Context, id workflow.WorkItemID, branch string) (workflow.ImplementationItem, error) {
	if err := b.requireRepository(); err != nil {
		return workflow.ImplementationItem{}, err
	}
	if id != "" {
		number, err := githubIssueNumber(id)
		if err != nil {
			return workflow.ImplementationItem{}, workflow.Refuse("explicit Work Item identity is invalid; resume with its stable numeric --item")
		}
		owners, err := b.activeOwners(ctx, number)
		if err != nil {
			return workflow.ImplementationItem{}, err
		}
		if len(owners) > 1 {
			return workflow.ImplementationItem{}, workflow.Refuse("multiple active Submissions own Work Item #" + strconv.Itoa(number) + "; repair the attachment before resuming")
		}
		if len(owners) == 1 {
			item, err := b.selectedSubmission(ctx, owners[0])
			if err != nil {
				return workflow.ImplementationItem{}, err
			}
			if item.ID != id {
				return workflow.ImplementationItem{}, workflow.Refuse("Submission ownership contradicts supplied Work Item #" + strconv.Itoa(number) + "; repair its explicit association before resuming")
			}
			return item, nil
		}
		return b.selectedReady(ctx, id)
	}
	if branch == "" {
		return workflow.ImplementationItem{}, workflow.Refuse("resume requires an explicit --item or an unambiguous conventional worktree branch")
	}
	var pulls []githubPull
	for page := 1; ; page++ {
		var batch []githubPull
		path := b.repositoryPath(b.repository) + "/pulls?state=open&head=" + url.QueryEscape(b.repository.Owner+":"+branch) + fmt.Sprintf("&per_page=100&page=%d", page)
		if err := b.request(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return workflow.ImplementationItem{}, err
		}
		pulls = append(pulls, batch...)
		if len(batch) < 100 {
			break
		}
	}
	var matched []githubPull
	for _, pull := range pulls {
		if pull.Head.Ref == branch && strings.EqualFold(pull.Head.Repo.FullName, b.repository.Owner+"/"+b.repository.Name) {
			matched = append(matched, pull)
		}
	}
	if len(matched) != 1 {
		return workflow.ImplementationItem{}, workflow.Refuse("worktree branch does not identify exactly one active Submission; resume with --item after repairing attachments")
	}
	return b.selectedSubmission(ctx, matched[0].Number)
}

func (b *GitHubBackend) selectedSubmission(ctx context.Context, number int) (workflow.ImplementationItem, error) {
	pull, err := b.pullRecord(ctx, number)
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	if pull.State != "open" {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Submission #" + strconv.Itoa(number) + " is not open; repair its attachment")
	}
	if !strings.EqualFold(pull.Head.Repo.FullName, b.repository.Owner+"/"+b.repository.Name) {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Submission head repository is outside the supported repository attachment; inspect and repair its association")
	}
	item, problem, err := b.submissionItem(ctx, pull)
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	item.Problem = firstProblem(item.Problem, problem)
	return item, nil
}

func (b *GitHubBackend) selectedReady(ctx context.Context, id workflow.WorkItemID) (workflow.ImplementationItem, error) {
	number, err := githubIssueNumber(id)
	if err != nil {
		return workflow.ImplementationItem{}, workflow.Refuse("explicit Work Item identity is invalid; resume with its stable numeric --item")
	}
	issue, err := b.issueRecord(ctx, number)
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	branch, problem := declaredBranch(issue.Body)
	item := workflow.ImplementationItem{ID: workflow.WorkItemID(strconv.Itoa(issue.Number)), Order: issue.Number, Branch: branch, CreatedAt: issue.CreatedAt, Source: implementationLifecycle(issue)}
	if branch == "" && problem == "" {
		problem = "no explicit branch attachment; repair the Work Item body"
	}
	owners, err := b.activeOwners(ctx, issue.Number)
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	if len(owners) != 0 {
		problem = "another active Submission already owns the Work Item; inspect its attachment instead of reassigning it"
	}
	stream := fmt.Sprintf("/issues/%d/comments", issue.Number)
	comments, err := b.implementationComments(ctx, b.repository, stream)
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	item.EvidenceSources = append(item.EvidenceSources, fmt.Sprintf("repos/%s/%s%s", b.repository.Owner, b.repository.Name, stream))
	for _, comment := range comments {
		if strings.HasPrefix(comment.Body, "<!-- skl.implement/v1\n") {
			continue
		}
		item.Feedback = append(item.Feedback, comment)
	}
	item = workflow.ReconcileImplementation(item)
	item.Problem = firstProblem(item.Problem, problem)
	return item, nil
}

func (b *GitHubBackend) submissionItem(ctx context.Context, pull githubPull) (workflow.ImplementationItem, string, error) {
	owner, problem := submissionOwner(pull.Body)
	if problem == "no explicit owning issue" {
		problem = "no explicit owning issue; repair the Submission attachment"
	}
	if owner == 0 {
		return workflow.ImplementationItem{Submission: &workflow.Submission{ID: workflow.SubmissionID(strconv.Itoa(pull.Number)), Head: pull.Head.SHA, Base: pull.Base.Ref, Draft: pull.Draft, Body: pull.Body, CreatedAt: pull.CreatedAt}}, problem, nil
	}
	issue, err := b.issueRecord(ctx, owner)
	if err != nil {
		return workflow.ImplementationItem{}, "", err
	}
	owners, err := b.activeOwners(ctx, owner)
	if err != nil {
		return workflow.ImplementationItem{}, "", err
	}
	for _, active := range owners {
		if active != pull.Number {
			problem = firstProblem(problem, "another active Submission already owns the Work Item; inspect its attachment instead of reassigning it")
		}
	}
	item := workflow.ImplementationItem{
		ID: workflow.WorkItemID(strconv.Itoa(issue.Number)), Order: issue.Number, Branch: pull.Head.Ref, CreatedAt: issue.CreatedAt,
		Source: implementationLifecycle(issue),
		Submission: &workflow.Submission{
			ID: workflow.SubmissionID(strconv.Itoa(pull.Number)), Head: pull.Head.SHA, Base: pull.Base.Ref, Draft: pull.Draft, Body: pull.Body,
			Author: pull.User.Login, Association: pull.AuthorAssociation,
			CreatedAt: pull.CreatedAt, Lifecycle: implementationLifecycle(pull.githubIssue),
		},
	}
	item.Submission.Lifecycle.Merged = pull.MergedAt != ""
	for _, label := range pull.Labels {
		item.Synchronization = item.Synchronization || label.Name == "sync"
	}
	item = workflow.ReconcileImplementation(item)
	comments, err := b.implementationComments(ctx, b.repository, fmt.Sprintf("/issues/%d/comments", owner))
	if err != nil {
		return workflow.ImplementationItem{}, "", err
	}
	for _, comment := range comments {
		if strings.HasPrefix(comment.Body, "<!-- skl.implement/v1\n") {
			continue
		}
		item.Feedback = append(item.Feedback, comment)
		item.Submission.Comments = append(item.Submission.Comments, comment)
	}
	prefix := fmt.Sprintf("repos/%s/%s", b.repository.Owner, b.repository.Name)
	item.Submission.EvidenceSources = []string{
		fmt.Sprintf("%s/pulls/%d", prefix, pull.Number),
		fmt.Sprintf("%s/issues/%d/comments", prefix, owner),
	}
	for _, stream := range []string{fmt.Sprintf("/issues/%d/comments", pull.Number), fmt.Sprintf("/pulls/%d/comments", pull.Number)} {
		comments, err := b.implementationComments(ctx, b.repository, stream)
		if err != nil {
			return workflow.ImplementationItem{}, "", err
		}
		item.Submission.EvidenceSources = append(item.Submission.EvidenceSources, prefix+stream)
		item.Submission.Comments = append(item.Submission.Comments, comments...)
	}
	reviews, err := b.implementationReviews(ctx, pull.Number)
	if err != nil {
		return workflow.ImplementationItem{}, "", err
	}
	item.Submission.EvidenceSources = append(item.Submission.EvidenceSources, fmt.Sprintf("%s/pulls/%d/reviews", prefix, pull.Number))
	item.Submission.Comments = append(item.Submission.Comments, reviews...)
	return workflow.ReconcileImplementation(item), problem, nil
}

func (b *GitHubBackend) pullRecord(ctx context.Context, number int) (githubPull, error) {
	var pull githubPull
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(b.repository)+fmt.Sprintf("/pulls/%d", number), nil, &pull); err != nil {
		return githubPull{}, err
	}
	return pull, nil
}

func (b *GitHubBackend) issueRecord(ctx context.Context, number int) (githubIssue, error) {
	var issue githubIssue
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(b.repository)+fmt.Sprintf("/issues/%d", number), nil, &issue); err != nil {
		return githubIssue{}, err
	}
	b.issueIDs[number] = issue.ID
	b.issueBodies[number] = issue.Body
	return issue, nil
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

// activeOwners observes GitHub's explicit open-PR references for one issue.
func (b *GitHubBackend) activeOwners(ctx context.Context, issueNumber int) ([]int, error) {
	references, err := b.closingReferences(ctx, issueNumber, false)
	if err != nil {
		return nil, err
	}
	var owners []int
	for _, reference := range references {
		owners = append(owners, reference.Number)
	}
	return owners, nil
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

// ImplementationDependencies observes only the tentative Ready candidate's
// referenced blockers and their explicit merged Submission evidence.
func (b *GitHubBackend) ImplementationDependencies(ctx context.Context, id workflow.WorkItemID) ([]workflow.BlockerObservation, error) {
	if err := b.requireRepository(); err != nil {
		return nil, err
	}
	number, err := githubIssueNumber(id)
	if err != nil {
		return nil, workflow.Refuse("invalid Ready Work Item identity; repair its attachment")
	}
	issue, err := b.issueRecord(ctx, number)
	if err != nil {
		return nil, err
	}
	blockers, problem, err := b.dependencyReferences(ctx, number, issue.Body)
	if err != nil {
		return nil, err
	}
	if problem != "" {
		return nil, workflow.Refuse(problem + "; repair the Work Item body")
	}
	var result []workflow.BlockerObservation
	for _, blocker := range blockers {
		observation, err := b.blockerObservation(ctx, blocker)
		if err != nil {
			return nil, err
		}
		result = append(result, observation)
	}
	return result, nil
}

func (b *GitHubBackend) blockerObservation(ctx context.Context, number int) (workflow.BlockerObservation, error) {
	references, err := b.closingReferences(ctx, number, true)
	if err != nil {
		return workflow.BlockerObservation{}, err
	}
	merged := false
	for _, reference := range references {
		merged = merged || reference.Merged || reference.MergedAt != ""
	}
	return workflow.BlockerObservation{ID: workflow.WorkItemID(strconv.Itoa(number)), Merged: merged}, nil
}

// claimWriteFailure reports a Claim write that could not be verified through
// the selected record alone, never inventing a packet for an unverified Claim.
func claimWriteFailure(writeErr, observeErr error, observedClaimed bool, subject string) error {
	var refusal *workflow.InvariantError
	if errors.As(writeErr, &refusal) {
		return writeErr
	}
	if observeErr == nil && observedClaimed {
		return workflow.Refuse("Claim response was uncertain and a later Claim is now observed; inspect whether it belongs to this handoff before resuming")
	}
	return errors.New(writeErr.Error() + "; inspect the selected " + subject + " and explicitly resume instead of retrying next")
}

// Acquisition cannot use an idempotent label update: an existing Claim belongs
// to an earlier acquisition, not the worker currently requesting next.
func (b *GitHubBackend) acquireSelectedClaim(ctx context.Context, before githubIssue) error {
	observed, err := b.issueRecord(ctx, before.Number)
	if err != nil {
		return err
	}
	expected, _, _ := implementationLabels(before)
	state, claimed, problem := implementationLabels(observed)
	if problem != "" || observed.Number != before.Number || observed.State != "open" || state != expected || claimed || observed.Body != before.Body {
		return workflow.Refuse("selected record changed before the Claim write; inspect it and explicitly resume instead of retrying next")
	}
	return b.request(ctx, http.MethodPost, b.repositoryPath(b.repository)+fmt.Sprintf("/issues/%d/labels", before.Number), map[string][]string{"labels": {"wip"}}, nil)
}

// ClaimSelected adds the queue record's additive `wip` Claim after refreshing
// only the selected records, then verifies the Claim through that same readback.
func (b *GitHubBackend) ClaimSelected(ctx context.Context, candidate workflow.QueueCandidate, item workflow.ImplementationItem) (workflow.ImplementationItem, error) {
	if err := b.requireRepository(); err != nil {
		return workflow.ImplementationItem{}, err
	}
	if candidate.SubmissionID != "" {
		return b.claimSubmission(ctx, candidate, item)
	}
	return b.claimReady(ctx, candidate, item)
}

func (b *GitHubBackend) claimSubmission(ctx context.Context, candidate workflow.QueueCandidate, item workflow.ImplementationItem) (workflow.ImplementationItem, error) {
	number, err := githubIssueNumber(workflow.WorkItemID(candidate.SubmissionID))
	if err != nil {
		return workflow.ImplementationItem{}, workflow.Refuse(err.Error())
	}
	before, err := b.pullRecord(ctx, number)
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	owner, problem := submissionOwner(before.Body)
	if problem != "" || owner != mustNumber(item.ID) {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Submission ownership changed before acquisition; inspect it and explicitly resume or retry")
	}
	state, claimed, problem := implementationLabels(before.githubIssue)
	if problem != "" || before.Number != number || before.State != "open" || !strings.EqualFold(before.Head.Repo.FullName, b.repository.Owner+"/"+b.repository.Name) || before.Head.SHA != item.Submission.Head || before.Head.Ref != item.Branch || before.Base.Ref != item.Submission.Base || before.Draft != item.Submission.Draft || before.Body != item.Submission.Body {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Submission changed before acquisition; inspect it and explicitly resume or retry")
	}
	if state != workflow.Rework && state != workflow.AwaitingReview {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Submission is no longer an eligible queue record; inspect it before retrying")
	}
	if claimed {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Submission already carries a Claim; resume with --item if it is yours, otherwise inspect it")
	}
	if err := b.acquireSelectedClaim(ctx, before.githubIssue); err != nil {
		observed, observeErr := b.pullRecord(ctx, number)
		var observedClaimed bool
		if observeErr == nil {
			_, observedClaimed, _ = implementationLabels(observed.githubIssue)
		}
		return workflow.ImplementationItem{}, claimWriteFailure(err, observeErr, observedClaimed, "Submission")
	}
	observed, err := b.pullRecord(ctx, number)
	if err != nil {
		return workflow.ImplementationItem{}, errors.New(err.Error() + "; Claim verification was interrupted; inspect the selected Submission and explicitly resume instead of retrying next")
	}
	if _, observedClaimed, observedProblem := implementationLabels(observed.githubIssue); observedProblem != "" || !observedClaimed {
		return workflow.ImplementationItem{}, workflow.Refuse("Claim was not observed on the selected Submission; inspect it before retrying")
	}
	if observedOwner, problem := submissionOwner(observed.Body); problem != "" || observedOwner != owner || observed.Number != number || observed.Head.SHA != before.Head.SHA || observed.Head.Ref != before.Head.Ref || !strings.EqualFold(observed.Head.Repo.FullName, before.Head.Repo.FullName) || observed.Base.Ref != before.Base.Ref || observed.Draft != before.Draft || observed.Body != before.Body {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Submission identity or revision changed during Claim acquisition; Claim preserved; inspect it and explicitly resume instead of retrying next")
	}
	source, err := b.issueRecord(ctx, owner)
	if err != nil {
		return workflow.ImplementationItem{}, errors.New(err.Error() + "; Claim verification was interrupted; inspect the selected Work Item and explicitly resume instead of retrying next")
	}
	if source.Number != owner {
		return workflow.ImplementationItem{}, workflow.Refuse("selected source identity changed during Claim acquisition; Claim preserved; inspect it and explicitly resume instead of retrying next")
	}
	item.Source = implementationLifecycle(source)
	item.Submission.Lifecycle = implementationLifecycle(observed.githubIssue)
	item.Submission.Lifecycle.Merged = observed.MergedAt != ""
	item = workflow.ReconcileImplementation(item)
	if item.Problem != "" || !item.Claimed {
		return workflow.ImplementationItem{}, workflow.Refuse("Claim read-back contradicts the selected Submission; inspect it before retrying")
	}
	return item, nil
}

func (b *GitHubBackend) claimReady(ctx context.Context, candidate workflow.QueueCandidate, item workflow.ImplementationItem) (workflow.ImplementationItem, error) {
	number, err := githubIssueNumber(item.ID)
	if err != nil {
		return workflow.ImplementationItem{}, workflow.Refuse(err.Error())
	}
	before, err := b.issueRecord(ctx, number)
	if err != nil {
		return workflow.ImplementationItem{}, err
	}
	state, claimed, problem := implementationLabels(before)
	if problem != "" || before.State != "open" || state != workflow.Ready {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Work Item changed before acquisition; inspect it and explicitly resume or retry")
	}
	if claimed {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Work Item already carries a Claim; resume with --item if it is yours, otherwise inspect it")
	}
	if branch, branchProblem := declaredBranch(before.Body); branchProblem != "" || branch == "" || branch != item.Branch {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Work Item branch attachment changed before acquisition; inspect it before retrying")
	}
	if err := b.acquireSelectedClaim(ctx, before); err != nil {
		observed, observeErr := b.issueRecord(ctx, number)
		var observedClaimed bool
		if observeErr == nil {
			_, observedClaimed, _ = implementationLabels(observed)
		}
		return workflow.ImplementationItem{}, claimWriteFailure(err, observeErr, observedClaimed, "Work Item")
	}
	observed, err := b.issueRecord(ctx, number)
	if err != nil {
		return workflow.ImplementationItem{}, errors.New(err.Error() + "; Claim verification was interrupted; inspect the selected Work Item and explicitly resume instead of retrying next")
	}
	if _, observedClaimed, observedProblem := implementationLabels(observed); observedProblem != "" || !observedClaimed {
		return workflow.ImplementationItem{}, workflow.Refuse("Claim was not observed on the selected Work Item; inspect it before retrying")
	}
	if branch, problem := declaredBranch(observed.Body); problem != "" || branch != item.Branch || observed.Number != number {
		return workflow.ImplementationItem{}, workflow.Refuse("selected Work Item branch attachment or identity changed during Claim acquisition; Claim preserved; inspect it and explicitly resume instead of retrying next")
	}
	item.Source = implementationLifecycle(observed)
	item = workflow.ReconcileImplementation(item)
	if item.Problem != "" || !item.Claimed {
		return workflow.ImplementationItem{}, workflow.Refuse("Claim read-back contradicts the selected Work Item; inspect it before retrying")
	}
	return item, nil
}

func mustNumber(id workflow.WorkItemID) int {
	number, _ := githubIssueNumber(id)
	return number
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
			return nil, err
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
			comments = append(comments, skilldist.ReviewComment{Body: body, Author: review.User.Login, Association: review.Association, Commit: review.Commit, FinalHead: finalHead, CreatedAt: review.SubmittedAt, Verdict: verdict, ReviewNumber: reviewNumber,
				Source: fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", b.repository.Owner, b.repository.Name, number)})
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
