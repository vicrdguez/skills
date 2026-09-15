package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

type githubPull struct {
	githubIssue
	Merged    bool   `json:"merged"`
	Mergeable *bool  `json:"mergeable"`
	NodeID    string `json:"node_id"`
	Draft     bool   `json:"draft"`
	MergedAt  string `json:"merged_at"`
	Head      struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func githubImplementationNumbers(item workflow.ImplementationItem) (int, int, error) {
	number, err := githubIssueNumber(item.ID)
	if err != nil {
		return 0, 0, err
	}
	var submissionNumber int
	if item.Submission != nil {
		submissionNumber, err = githubIssueNumber(workflow.WorkItemID(item.Submission.ID))
		if err != nil {
			return 0, 0, err
		}
	}
	return number, submissionNumber, nil
}

func (b *GitHubBackend) ClaimImplementation(ctx context.Context, item workflow.ImplementationItem) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	repository := b.repository
	itemNumber, submissionNumber, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	items, err := b.ImplementationItems(ctx)
	if err != nil {
		return err
	}
	for _, current := range items {
		if current.ID != item.ID {
			continue
		}
		if current.Problem != "" || current.State != item.State || current.Branch != item.Branch {
			return workflow.Refuse("implementation state changed before Claim; inspect projections and explicitly resume")
		}
		if item.TargetSnapshot != "" {
			if current.TargetSnapshot != "" && current.TargetSnapshot != item.TargetSnapshot {
				return workflow.Refuse("Target Snapshot contradicts recorded obligation; use the original pinned commit")
			}
			if err := b.publishImplementationMetadata(ctx, repository, itemNumber, implementationMetadata{TargetSnapshot: item.TargetSnapshot, TargetBranch: item.TargetBranch}); err != nil {
				return err
			}
		}
		if item.State == workflow.AwaitingReview {
			if item.Submission == nil || current.Submission == nil || current.Submission.Head != item.Submission.Head || current.Submission.ID != item.Submission.ID {
				return workflow.Refuse("Submission changed before Watchdog Claim; retry with the current head")
			}
		}
		if current.Claimed {
			return nil
		}
		number := itemNumber
		if item.State == workflow.Rework || item.State == workflow.AwaitingReview {
			if item.Submission == nil {
				return workflow.Refuse("Rework has no Submission; repair its attachment")
			}
			number = submissionNumber
		}
		return b.implementationLabelMutation(ctx, repository, number, []string{"wip"}, nil, nil)
	}
	return workflow.Refuse("Work Item disappeared before Claim; inspect its stable identity")
}

func (b *GitHubBackend) publishImplementationMetadata(ctx context.Context, repository github.RepositoryID, number int, metadata implementationMetadata) error {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return b.implementationComment(ctx, repository, number, "<!-- skl.implement/v1\n"+string(payload)+"\n-->", true, "")
}

func (b *GitHubBackend) implementationComment(ctx context.Context, repository github.RepositoryID, number int, body string, metadata bool, claimAcquiredAt string) error {
	var claim time.Time
	if claimAcquiredAt != "" {
		var err error
		claim, err = time.Parse(time.RFC3339Nano, claimAcquiredAt)
		if err != nil {
			return workflow.Refuse("decision Claim timing is unavailable; inspect before retrying")
		}
	}
	published := func(comments []skilldist.ReviewComment) (bool, error) {
		latest := ""
		found := false
		for _, comment := range comments {
			if !comment.EvidenceAuthorized || comment.Path != "" {
				continue
			}
			if metadata {
				if strings.HasPrefix(comment.Body, "<!-- skl.implement/v1\n") {
					latest = comment.Body
				}
				continue
			}
			if claimAcquiredAt != "" {
				if !strings.HasPrefix(comment.Body, workflow.OpaqueImplementationDecision("")) {
					continue
				}
				created, err := time.Parse(time.RFC3339Nano, comment.CreatedAt)
				if err != nil || created.Equal(claim) {
					return false, workflow.Refuse("decision receipt ordering is unknown or equal to the Claim; inspect before retrying")
				}
				if created.Before(claim) {
					continue
				}
				if comment.Body != body {
					return false, workflow.Refuse("current decision differs from the supplied Result Document; restore the original decision before retrying")
				}
			}
			found = found || comment.Body == body
		}
		return metadata && latest == body || !metadata && found, nil
	}
	stream := fmt.Sprintf("/issues/%d/comments", number)
	comments, err := b.implementationComments(ctx, repository, stream)
	if err != nil {
		return err
	}
	if found, err := published(comments); err != nil || found {
		return err
	}
	writeErr := b.request(ctx, http.MethodPost, b.repositoryPath(repository)+stream, map[string]string{"body": body}, nil)
	comments, err = b.implementationComments(ctx, repository, stream)
	if err != nil {
		return err
	}
	if found, err := published(comments); err != nil || found {
		return err
	}
	if writeErr != nil {
		return writeErr
	}
	return errors.New("comment publication not observed; retry the same operation")
}

func (b *GitHubBackend) implementationLabelMutation(ctx context.Context, repository github.RepositoryID, number int, add, remove []string, guard func() error) error {
	if guard == nil {
		guard = func() error { return nil }
	}
	path := b.repositoryPath(repository) + fmt.Sprintf("/issues/%d", number)
	read := func() (githubIssue, error) {
		var issue githubIssue
		err := b.request(ctx, http.MethodGet, path, nil, &issue)
		return issue, err
	}
	for _, label := range append(slices.Clone(add), remove...) {
		if err := guard(); err != nil {
			return err
		}
		issue, err := read()
		if err != nil {
			return err
		}
		present := false
		for _, current := range issue.Labels {
			present = present || current.Name == label
		}
		wanted := slices.Contains(add, label)
		if present == wanted {
			continue
		}
		var writeErr error
		if wanted {
			writeErr = b.request(ctx, http.MethodPost, path+"/labels", map[string][]string{"labels": {label}}, nil)
		} else {
			writeErr = b.request(ctx, http.MethodDelete, path+"/labels/"+url.PathEscape(label), nil, nil)
		}
		issue, err = read()
		if err != nil {
			return err
		}
		present = false
		for _, current := range issue.Labels {
			present = present || current.Name == label
		}
		if present != wanted {
			if writeErr != nil {
				return writeErr
			}
			return errors.New("label mutation not observed; retry the same operation")
		}
		if err := guard(); err != nil {
			return err
		}
	}
	return nil
}

func (b *GitHubBackend) PublishImplementation(ctx context.Context, item workflow.ImplementationItem, wanted workflow.Submission) (workflow.Submission, error) {
	if err := b.requireRepository(); err != nil {
		return workflow.Submission{}, err
	}
	repository := b.repository
	itemNumber, err := githubIssueNumber(item.ID)
	if err != nil {
		return workflow.Submission{}, err
	}
	var submissionNumber int
	if wanted.ID != "" {
		submissionNumber, err = githubIssueNumber(workflow.WorkItemID(wanted.ID))
		if err != nil {
			return workflow.Submission{}, err
		}
	}
	wanted.Body = withClosingReference(wanted.Body, itemNumber)
	issues, err := b.listIssues(ctx, repository)
	if err != nil {
		return workflow.Submission{}, err
	}
	owners := implementationBranchOwners(issues)
	if owners[item.Branch] != itemNumber {
		return workflow.Submission{}, workflow.Refuse("missing, changed or multiple source issues own the conventional branch; repair attachments before publication")
	}
	var matches []githubPull
	for page := 1; ; page++ {
		var pulls []githubPull
		path := b.repositoryPath(repository) + "/pulls?state=all&head=" + url.QueryEscape(repository.Owner+":"+item.Branch) + fmt.Sprintf("&per_page=100&page=%d", page)
		if err := b.request(ctx, http.MethodGet, path, nil, &pulls); err != nil {
			return workflow.Submission{}, err
		}
		for _, pull := range pulls {
			if pull.Head.Ref == item.Branch && strings.EqualFold(pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name) {
				matches = append(matches, pull)
			}
		}
		if len(pulls) < 100 {
			break
		}
	}
	if len(matches) > 1 || len(matches) == 1 && (matches[0].State == "closed" || wanted.ID != "" && matches[0].Number != submissionNumber) {
		return workflow.Submission{}, workflow.Refuse("ambiguous or closed existing Submission; repair the attachment")
	}
	if len(matches) == 0 && wanted.ID != "" {
		return workflow.Submission{}, workflow.Refuse("existing Submission disappeared; repair its attachment")
	}
	var pull githubPull
	var writeErr error
	if len(matches) == 0 {
		writeErr = b.request(ctx, http.MethodPost, b.repositoryPath(repository)+"/pulls", map[string]any{"title": item.Branch, "head": item.Branch, "base": wanted.Base, "body": wanted.Body, "draft": wanted.Draft}, &pull)
		if writeErr != nil {
			// Observe an ambiguous create before considering another write.
			var observed []githubPull
			if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+"/pulls?state=all&head="+url.QueryEscape(repository.Owner+":"+item.Branch)+"&per_page=100&page=1", nil, &observed); err != nil {
				return workflow.Submission{}, err
			}
			if len(observed) != 1 {
				return workflow.Submission{}, writeErr
			}
			pull = observed[0]
		}
	} else {
		pull = matches[0]
	}
	if pull.Head.Ref != item.Branch || !strings.EqualFold(pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name) || pull.Head.SHA != wanted.Head || pull.State == "closed" {
		return workflow.Submission{}, workflow.Refuse("Submission head or state changed during publication; inspect and retry at a pushed fixed head")
	}
	if pull.Body != wanted.Body || pull.Base.Ref != wanted.Base {
		writeErr = b.request(ctx, http.MethodPatch, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d", pull.Number), map[string]string{"body": wanted.Body, "base": wanted.Base}, nil)
	}
	if pull.Draft != wanted.Draft {
		mutation := "markPullRequestReadyForReview"
		if wanted.Draft {
			mutation = "convertPullRequestToDraft"
		}
		var response struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		writeErr = b.request(ctx, http.MethodPost, "/graphql", map[string]any{"query": "mutation($id:ID!){" + mutation + "(input:{pullRequestId:$id}){pullRequest{id}}}", "variables": map[string]string{"id": pull.NodeID}}, &response)
		if writeErr == nil && len(response.Errors) > 0 {
			writeErr = errors.New(response.Errors[0].Message)
		}
	}
	var observed githubPull
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d", pull.Number), nil, &observed); err != nil {
		return workflow.Submission{}, err
	}
	if observed.Head.Ref != item.Branch || observed.Head.SHA != wanted.Head || observed.Body != wanted.Body || observed.Base.Ref != wanted.Base || observed.Draft != wanted.Draft || observed.State != "open" {
		if writeErr != nil {
			return workflow.Submission{}, writeErr
		}
		return workflow.Submission{}, workflow.Refuse("Submission publication not observed at the fixed head; inspect and retry the same handoff")
	}
	wanted.ID = workflow.SubmissionID(strconv.Itoa(observed.Number))
	return wanted, nil
}

func (b *GitHubBackend) AwaitImplementationReview(ctx context.Context, item workflow.ImplementationItem, guard func() error) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	repository := b.repository
	itemNumber, submissionNumber, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	if item.Submission == nil {
		return workflow.PermitImplementationReview(nil)
	}
	var issue githubIssue
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d", submissionNumber), nil, &issue); err != nil {
		return err
	}
	if err := workflow.PermitImplementationReview(implementationLifecycle(issue)); err != nil {
		return err
	}
	if err := b.implementationLabelMutation(ctx, repository, submissionNumber, []string{"wip", "review"}, []string{"rework", "sync"}, guard); err != nil {
		return err
	}
	if err := b.implementationLabelMutation(ctx, repository, itemNumber, nil, []string{"ready", "needs-human", "wip"}, guard); err != nil {
		return err
	}
	return b.implementationLabelMutation(ctx, repository, submissionNumber, nil, []string{"wip"}, guard)
}

func (b *GitHubBackend) PauseImplementation(ctx context.Context, item workflow.ImplementationItem, decision string, guard func() error) error {
	if err := b.requireRepository(); err != nil {
		return err
	}
	repository := b.repository
	itemNumber, submissionNumber, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	number := itemNumber
	if item.Submission != nil {
		number = submissionNumber
	}
	claimNumber := itemNumber
	if item.State == workflow.Rework {
		claimNumber = submissionNumber
	}
	claimAcquiredAt, err := b.issueClaimAcquiredAt(ctx, repository, claimNumber)
	if err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, claimAcquiredAt); err != nil {
		return workflow.Refuse("decision publication requires known source Claim timing; inspect before retrying")
	}
	if err := b.implementationComment(ctx, repository, number, workflow.OpaqueImplementationDecision(decision), false, claimAcquiredAt); err != nil {
		return err
	}
	if item.Submission != nil {
		if err := b.implementationLabelMutation(ctx, repository, number, []string{"wip", "needs-human"}, []string{"review", "rework", "sync"}, guard); err != nil {
			return err
		}
	}
	if err := b.implementationLabelMutation(ctx, repository, itemNumber, []string{"needs-human"}, []string{"ready", "wip"}, guard); err != nil {
		return err
	}
	if item.Submission != nil {
		return b.implementationLabelMutation(ctx, repository, submissionNumber, nil, []string{"wip"}, guard)
	}
	return nil
}

func trustedMetadata(comment skilldist.ReviewComment) bool {
	return slices.Contains([]string{"OWNER", "MEMBER", "COLLABORATOR"}, comment.Association)
}

type implementationMetadata struct {
	SynchronizationTarget string `json:"synchronization_target,omitempty"`
	TargetSnapshot        string `json:"target_snapshot,omitempty"`
	TargetBranch          string `json:"target_branch,omitempty"`
}

func implementationBranchOwners(issues []githubIssue) map[string]int {
	owners := make(map[string]int)
	for _, issue := range issues {
		if len(issue.PullRequest) == 0 && issue.SubIssuesSummary.Total == 0 {
			if _, exists := owners[issue.Title]; exists {
				owners[issue.Title] = 0 // Duplicate ownership has no unambiguous Work Item identity.
			} else {
				owners[issue.Title] = issue.Number
			}
		}
	}
	return owners
}

func (b *GitHubBackend) ImplementationItems(ctx context.Context) ([]workflow.ImplementationItem, error) {
	if err := b.requireRepository(); err != nil {
		return nil, err
	}
	repository := b.repository
	issues, err := b.listIssues(ctx, repository)
	if err != nil {
		return nil, err
	}
	var pulls []githubPull
	for page := 1; ; page++ {
		var batch []githubPull
		if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/pulls?state=all&per_page=100&page=%d", page), nil, &batch); err != nil {
			return nil, err
		}
		pulls = append(pulls, batch...)
		if len(batch) < 100 {
			break
		}
	}
	owners := implementationBranchOwners(issues)
	var items []workflow.ImplementationItem
	for _, issue := range issues {
		if len(issue.PullRequest) != 0 || issue.SubIssuesSummary.Total > 0 {
			continue
		}
		var matches []githubPull
		for _, pull := range pulls {
			if pull.Head.Ref == issue.Title && strings.EqualFold(pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name) {
				matches = append(matches, pull)
			}
		}
		if !hasWorkflowLabel(issue) && len(matches) == 0 {
			continue
		}
		state, claimed, problem := implementationLabels(issue)
		item := workflow.ImplementationItem{ID: workflow.WorkItemID(strconv.Itoa(issue.Number)), Order: issue.Number, Branch: issue.Title, CreatedAt: issue.CreatedAt, State: state, Claimed: claimed, Problem: problem}
		item.Source = implementationLifecycle(issue)
		if claimed {
			item.SourceClaimAcquiredAt, err = b.issueClaimAcquiredAt(ctx, repository, issue.Number)
			if err != nil {
				return nil, err
			}
		}
		if len(matches) > 1 {
			item.Problem = "multiple Submissions share the conventional branch"
		}
		if len(matches) == 1 {
			pull := matches[0]
			prState, prClaimed, _ := implementationLabels(pull.githubIssue)
			item.Submission = &workflow.Submission{ID: workflow.SubmissionID(strconv.Itoa(pull.Number)), Head: pull.Head.SHA, Base: pull.Base.Ref, Draft: pull.Draft, Body: pull.Body, State: prState, Claimed: prClaimed, CreatedAt: pull.CreatedAt}
			item.Submission.Lifecycle = implementationLifecycle(pull.githubIssue)
			item.Submission.Lifecycle.Merged = pull.MergedAt != ""
			if prClaimed {
				item.Submission.ClaimAcquiredAt, err = b.issueClaimAcquiredAt(ctx, repository, pull.Number)
				if err != nil {
					return nil, err
				}
				if prState == workflow.Rework && pull.Body != "" && pull.NodeID != "" {
					item.Submission.BodyUpdatedAt, err = b.implementationBodyUpdatedAt(ctx, pull)
					if err != nil {
						return nil, err
					}
				}
			}
			for _, label := range pull.Labels {
				item.Synchronization = item.Synchronization || label.Name == "sync"
			}
			item = workflow.ReconcileImplementation(item)
			if item.State == workflow.Rework || item.State == workflow.AwaitingReview || item.State == workflow.NeedsHuman || item.State == workflow.ReadyForMerge {
				for _, stream := range []string{fmt.Sprintf("/issues/%d/comments", pull.Number), fmt.Sprintf("/pulls/%d/comments", pull.Number)} {
					comments, err := b.implementationComments(ctx, repository, stream)
					if err != nil {
						return nil, err
					}
					item.Submission.Comments = append(item.Submission.Comments, comments...)
				}
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
					if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d/reviews?per_page=100&page=%d", pull.Number, page), nil, &reviews); err != nil {
						return nil, err
					}
					for _, review := range reviews {
						verdict := map[string]string{"CHANGES_REQUESTED": "rework", "APPROVED": "pass", "COMMENTED": "needs-human"}[review.State]
						body := review.Body
						reviewNumber := uint64(0)
						if !trustedMetadata(skilldist.ReviewComment{Association: review.Association}) {
							verdict = ""
						} else if strings.HasPrefix(body, reviewSummaryPrefix) {
							verdict = ""
							if metadata, summary, ok := parseReviewSummary(body); ok && review.State == "COMMENTED" {
								body, verdict, reviewNumber = summary, metadata.Verdict, metadata.ReviewNumber
							}
						}
						item.Submission.Comments = append(item.Submission.Comments, skilldist.ReviewComment{Body: body, Author: review.User.Login, Association: review.Association, Commit: review.Commit, CreatedAt: review.SubmittedAt, Verdict: verdict, ReviewNumber: reviewNumber})
					}
					if len(reviews) < 100 {
						break
					}
				}
			}
		}
		item = workflow.ReconcileImplementation(item)
		if item.State == workflow.Ready {
			for page := 1; ; page++ {
				var blockers []githubIssue
				status, err := b.requestStatus(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d/dependencies/blocked_by?per_page=100&page=%d", issue.Number, page), nil, &blockers)
				if err != nil && status != http.StatusNotFound && status != http.StatusGone {
					return nil, err
				}
				for _, blocker := range blockers {
					item.Blockers = append(item.Blockers, workflow.WorkItemID(strconv.Itoa(blocker.Number)))
				}
				if len(blockers) < 100 {
					break
				}
			}
			// Adopt the former workflow's explicit dependency projection only.
			for _, line := range strings.Split(issue.Body, "\n") {
				if rest, ok := strings.CutPrefix(line, "Blocked by: "); ok {
					for _, value := range strings.Split(rest, ",") {
						number, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(value), "#"))
						if err != nil || number <= 0 {
							item.Problem = "invalid legacy Dependency projection"
						} else if id := workflow.WorkItemID(strconv.Itoa(number)); !slices.Contains(item.Blockers, id) {
							item.Blockers = append(item.Blockers, id)
						}
					}
				}
			}
		}
		comments, err := b.implementationComments(ctx, repository, fmt.Sprintf("/issues/%d/comments", issue.Number))
		if err != nil {
			return nil, err
		}
		for _, comment := range comments {
			if !strings.HasPrefix(comment.Body, "<!-- skl.implement/v1\n") {
				item.Feedback = append(item.Feedback, comment)
			}
			if item.Submission != nil && !strings.HasPrefix(comment.Body, "<!-- skl.implement/v1\n") {
				item.Submission.Comments = append(item.Submission.Comments, comment)
			}
			if body, ok := strings.CutPrefix(comment.Body, "<!-- skl.implement/v1\n"); ok && strings.HasSuffix(body, "\n-->") {
				if !trustedMetadata(comment) {
					continue
				}
				var metadata implementationMetadata
				if err := json.Unmarshal([]byte(strings.TrimSuffix(body, "\n-->")), &metadata); err != nil {
					item.Problem = "invalid implementation operation metadata"
					continue
				}
				if metadata.TargetSnapshot != "" {
					if item.TargetSnapshot != "" && item.TargetSnapshot != metadata.TargetSnapshot {
						item.Problem = "conflicting Target Snapshot metadata"
						continue
					}
					item.TargetSnapshot = metadata.TargetSnapshot
				}
				if metadata.TargetBranch != "" {
					item.TargetBranch = metadata.TargetBranch
				}
				if metadata.SynchronizationTarget != "" {
					item.TargetSnapshot = metadata.SynchronizationTarget
				}
			}
		}
		item = workflow.ReconcileImplementation(item)
		if owners[item.Branch] != issue.Number {
			item.Problem = "multiple source issues own the conventional branch"
		}
		items = append(items, item)
	}
	return items, nil
}

func (b *GitHubBackend) implementationBodyUpdatedAt(ctx context.Context, pull githubPull) (string, error) {
	var response struct {
		Data struct {
			Node *struct {
				Body, CreatedAt, LastEditedAt string
			}
		}
		Errors []struct{ Message string }
	}
	query := "query($id:ID!){node(id:$id){... on PullRequest{body createdAt lastEditedAt}}}"
	if err := b.request(ctx, http.MethodPost, "/graphql", map[string]any{"query": query, "variables": map[string]string{"id": pull.NodeID}}, &response); err != nil {
		return "", err
	}
	if len(response.Errors) != 0 {
		return "", errors.New(response.Errors[0].Message)
	}
	if response.Data.Node == nil || response.Data.Node.Body != pull.Body {
		return "", workflow.Refuse("Submission content-edit evidence unavailable or changed; inspect the current body before retrying")
	}
	if response.Data.Node.LastEditedAt != "" {
		return response.Data.Node.LastEditedAt, nil
	}
	return response.Data.Node.CreatedAt, nil
}

func (b *GitHubBackend) ImplementationTarget(ctx context.Context) (string, error) {
	if err := b.requireRepository(); err != nil {
		return "", err
	}
	return b.validate(ctx, b.repository)
}

func implementationLabels(issue githubIssue) (workflow.State, bool, string) {
	var state workflow.State
	claimed := false
	problem := ""
	paused := false
	for _, label := range issue.Labels {
		var next workflow.State
		switch label.Name {
		case "ready":
			next = workflow.Ready
		case "rework":
			next = workflow.Rework
		case "review":
			next = workflow.AwaitingReview
		case "done":
			next = workflow.ReadyForMerge
		case "needs-human":
			next = workflow.NeedsHuman
			paused = true
		case "wip":
			claimed = true
		}
		if next != "" {
			if state != "" && state != next {
				problem = "contradictory lifecycle projections"
			}
			if state == "" {
				state = next
			}
		}
	}
	if paused {
		state = workflow.NeedsHuman
	}
	return state, claimed, problem
}

func implementationLifecycle(issue githubIssue) *workflow.LifecycleObservation {
	observation := &workflow.LifecycleObservation{Open: issue.State == "open"}
	for i := range issue.Labels {
		state, claimed, _ := implementationLabels(githubIssue{Labels: issue.Labels[i : i+1]})
		if state != "" {
			observation.States = append(observation.States, state)
		}
		observation.Claimed = observation.Claimed || claimed
	}
	return observation
}

func (b *GitHubBackend) implementationComments(ctx context.Context, repository github.RepositoryID, stream string) ([]skilldist.ReviewComment, error) {
	var comments []skilldist.ReviewComment
	for page := 1; ; page++ {
		var batch []struct {
			Line        int    `json:"line"`
			Side        string `json:"side"`
			Body        string `json:"body"`
			Association string `json:"author_association"`
			Commit      string `json:"commit_id"`
			Path        string `json:"path"`
			CreatedAt   string `json:"created_at"`
			User        struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+stream+fmt.Sprintf("?per_page=100&page=%d", page), nil, &batch); err != nil {
			return nil, err
		}
		for _, comment := range batch {
			observed := skilldist.ReviewComment{Body: comment.Body, Author: comment.User.Login, Association: comment.Association, Commit: comment.Commit, Path: comment.Path, CreatedAt: comment.CreatedAt, Line: comment.Line, Side: comment.Side}
			observed.EvidenceAuthorized = trustedMetadata(observed)
			comments = append(comments, observed)
		}
		if len(batch) < 100 {
			return comments, nil
		}
	}
}

func (b *GitHubBackend) ImplementationHead(ctx context.Context, branch string) (string, error) {
	if err := b.requireRepository(); err != nil {
		return "", err
	}
	repository := b.repository
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	found, err := b.requestOptional(ctx, http.MethodGet, b.repositoryPath(repository)+"/git/ref/heads/"+url.PathEscape(branch), &ref)
	if err != nil || !found {
		return "", err
	}
	return ref.Object.SHA, nil
}

func (b *GitHubBackend) SubmissionBodyMatches(id workflow.WorkItemID, actual, supplied string) (bool, error) {
	number, err := githubIssueNumber(id)
	if err != nil {
		return false, err
	}
	return actual == withClosingReference(supplied, number), nil
}

func withClosingReference(body string, number int) string {
	footer := fmt.Sprintf("\n\nCloses #%d\n", number)
	if !strings.HasSuffix(body, footer) {
		body += footer
	}
	return body
}
