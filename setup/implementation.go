package setup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

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

func (b *GitHubBackend) ClaimImplementation(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem) error {
	itemNumber, submissionNumber, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	items, err := b.ImplementationItems(ctx, repository)
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
		if item.Submission != nil && item.Submission.PreviousReviewedHead != "" {
			if err := b.publishImplementationMetadata(ctx, repository, itemNumber, implementationMetadata{ReviewedHead: item.Submission.PreviousReviewedHead, ReviewRoundHead: item.Submission.Head}); err != nil {
				return err
			}
		}
		if item.State == workflow.AwaitingReview {
			if item.Submission == nil || current.Submission == nil || current.Submission.Head != item.Submission.ReviewedHead || current.Submission.ID != item.Submission.ID || current.Claimed && current.Submission.ReviewedHead != "" && current.Submission.ReviewedHead != item.Submission.ReviewedHead {
				return workflow.Refuse("Submission changed before Watchdog Claim; restore the fixed head")
			}
			if err := b.publishImplementationMetadata(ctx, repository, itemNumber, implementationMetadata{WatchdogHead: item.Submission.ReviewedHead}); err != nil {
				return err
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
	return b.implementationComment(ctx, repository, number, "<!-- skl.implement/v1\n"+string(payload)+"\n-->", true)
}

func (b *GitHubBackend) implementationComment(ctx context.Context, repository github.RepositoryID, number int, body string, metadata bool) error {
	published := func(comments []skilldist.ReviewComment) bool {
		latest := ""
		for _, comment := range comments {
			if metadata {
				if strings.HasPrefix(comment.Body, "<!-- skl.implement/v1\n") && trustedMetadata(comment) {
					latest = comment.Body
				}
			} else if comment.Body == body {
				return true
			}
		}
		return metadata && latest == body
	}
	stream := fmt.Sprintf("/issues/%d/comments", number)
	comments, err := b.implementationComments(ctx, repository, stream)
	if err != nil {
		return err
	}
	if published(comments) {
		return nil
	}
	writeErr := b.request(ctx, http.MethodPost, b.repositoryPath(repository)+stream, map[string]string{"body": body}, nil)
	comments, err = b.implementationComments(ctx, repository, stream)
	if err != nil {
		return err
	}
	if published(comments) {
		return nil
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

func (b *GitHubBackend) PublishImplementation(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem, wanted workflow.Submission) (workflow.Submission, error) {
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

func (b *GitHubBackend) AwaitImplementationReview(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem, guard func() error) (err error) {
	itemNumber, submissionNumber, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			number := itemNumber
			if item.State == workflow.Rework && item.Submission != nil {
				number = submissionNumber
			}
			err = errors.Join(err, b.implementationLabelMutation(ctx, repository, number, []string{"wip"}, nil, nil))
		}
	}()
	if item.Submission == nil {
		return workflow.Refuse("review requires a durable Submission; publish it before retrying")
	}
	var issue githubIssue
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d", submissionNumber), nil, &issue); err != nil {
		return err
	}
	state, _, problem := implementationLabels(issue)
	// Adding review is the first durable step; a retry can see both review and rework.
	if state == workflow.NeedsHuman || state == workflow.ReadyForMerge || state == workflow.Ready || problem != "" && state != workflow.Rework && state != workflow.AwaitingReview {
		return workflow.Refuse("Submission lifecycle contradicts review handoff; repair its projections")
	}
	if err := b.implementationLabelMutation(ctx, repository, submissionNumber, []string{"review"}, []string{"rework", "wip", "sync"}, guard); err != nil {
		return err
	}
	return b.implementationLabelMutation(ctx, repository, itemNumber, nil, []string{"ready", "wip"}, guard)
}

func (b *GitHubBackend) PauseImplementation(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem, decision string, guard func() error) (err error) {
	itemNumber, submissionNumber, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			number := itemNumber
			if item.State == workflow.Rework && item.Submission != nil {
				number = submissionNumber
			}
			err = errors.Join(err, b.implementationLabelMutation(ctx, repository, number, []string{"wip"}, nil, nil))
		}
	}()
	if err := guard(); err != nil {
		return err
	}
	if err := b.publishImplementationMetadata(ctx, repository, itemNumber, implementationMetadata{ResumeState: item.State}); err != nil {
		return err
	}
	number := itemNumber
	if item.Submission != nil {
		number = submissionNumber
	}
	if err := b.implementationComment(ctx, repository, number, decision, false); err != nil {
		return err
	}
	if item.Submission != nil {
		if err := b.implementationLabelMutation(ctx, repository, number, []string{"needs-human"}, []string{"review", "rework", "wip"}, guard); err != nil {
			return err
		}
	}
	return b.implementationLabelMutation(ctx, repository, itemNumber, []string{"needs-human"}, []string{"ready", "wip"}, guard)
}

func (b *GitHubBackend) RecordImplementationTransition(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem, transition workflow.ImplementationTransition) error {
	number, err := githubIssueNumber(item.ID)
	if err != nil {
		return err
	}
	return b.publishImplementationMetadata(ctx, repository, number, implementationMetadata{Transition: &transition})
}

func (b *GitHubBackend) RetainImplementationClaim(ctx context.Context, repository github.RepositoryID, item workflow.ImplementationItem) error {
	number, submissionNumber, err := githubImplementationNumbers(item)
	if err != nil {
		return err
	}
	if item.State == workflow.Rework && item.Submission != nil {
		number = submissionNumber
	}
	return b.implementationLabelMutation(ctx, repository, number, []string{"wip"}, nil, nil)
}

func trustedMetadata(comment skilldist.ReviewComment) bool {
	return slices.Contains([]string{"OWNER", "MEMBER", "COLLABORATOR"}, comment.Association)
}

// implementationEvidence excludes decision occurrences during their pending handoff.
// Completion closes every decision window for that handoff's directory, including
// superseded prose. An identical later engine receipt is independent evidence,
// while earlier opaque occurrences stay excluded.
func implementationEvidence(comments []skilldist.ReviewComment) []skilldist.ReviewComment {
	var evidence []skilldist.ReviewComment
	decisions := make(map[string]map[string]bool)
	for _, comment := range comments {
		if !trustedMetadata(comment) || len(decisions[fmt.Sprintf("%x", sha256.Sum256([]byte(comment.Body)))]) > 0 {
			continue
		}
		evidence = append(evidence, comment)
		body, ok := strings.CutPrefix(comment.Body, "<!-- skl.implement/v1\n")
		if !ok || !strings.HasSuffix(body, "\n-->") {
			continue
		}
		var metadata implementationMetadata
		if json.Unmarshal([]byte(strings.TrimSuffix(body, "\n-->")), &metadata) == nil && metadata.Transition != nil {
			transition := metadata.Transition
			if transition.Completed {
				for digest, directories := range decisions {
					delete(directories, transition.Directory)
					if len(directories) == 0 {
						delete(decisions, digest)
					}
				}
			} else {
				if decisions[transition.DecisionDigest] == nil {
					decisions[transition.DecisionDigest] = make(map[string]bool)
				}
				decisions[transition.DecisionDigest][transition.Directory] = true
			}
		}
	}
	return evidence
}

type implementationMetadata struct {
	SynchronizationTarget string                             `json:"synchronization_target,omitempty"`
	WatchdogHead          string                             `json:"watchdog_head,omitempty"`
	Transition            *workflow.ImplementationTransition `json:"transition,omitempty"`
	TargetSnapshot        string                             `json:"target_snapshot,omitempty"`
	TargetBranch          string                             `json:"target_branch,omitempty"`
	ReviewedHead          string                             `json:"reviewed_head,omitempty"`
	ReviewRoundHead       string                             `json:"review_round_head,omitempty"`
	ResumeState           workflow.State                     `json:"resume_state,omitempty"`
	Round                 *workflow.DispatchRound            `json:"round,omitempty"`
}

func validDispatchRound(round workflow.DispatchRound) bool {
	return round.ID != "" && round.Item != "" && (round.Lane == workflow.ImplementLane || round.Lane == workflow.WatchdogLane) && round.Directory != "" && round.Obligation != ""
}

func (b *GitHubBackend) RecordDispatchRound(ctx context.Context, repository github.RepositoryID, round workflow.DispatchRound) error {
	if !validDispatchRound(round) {
		return workflow.Refuse("dispatch evidence has invalid or contradictory bindings")
	}
	number, err := githubIssueNumber(round.Item)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(implementationMetadata{Round: &round})
	if err != nil {
		return err
	}
	body := "<!-- skl.implement/v1\n" + string(payload) + "\n-->"
	published := func(comments []skilldist.ReviewComment) bool {
		for _, comment := range implementationEvidence(comments) {
			if comment.Body == body && trustedMetadata(comment) {
				return true
			}
		}
		return false
	}
	stream := fmt.Sprintf("/issues/%d/comments", number)
	comments, err := b.implementationComments(ctx, repository, stream)
	if err != nil || published(comments) {
		return err
	}
	writeErr := b.request(ctx, http.MethodPost, b.repositoryPath(repository)+stream, map[string]string{"body": body}, nil)
	comments, err = b.implementationComments(ctx, repository, stream)
	if err != nil {
		return err
	}
	if published(comments) {
		return nil
	}
	if writeErr != nil {
		return writeErr
	}
	return errors.New("dispatch evidence publication not observed; explicitly inspect the retained Claim")
}

func (b *GitHubBackend) DispatchRounds(ctx context.Context, repository github.RepositoryID, item workflow.WorkItemID) ([]workflow.DispatchRound, error) {
	number, err := githubIssueNumber(item)
	if err != nil {
		return nil, err
	}
	comments, err := b.implementationComments(ctx, repository, fmt.Sprintf("/issues/%d/comments", number))
	if err != nil {
		return nil, err
	}
	return dispatchRoundsFromComments(comments, item)
}

func dispatchRoundsFromComments(comments []skilldist.ReviewComment, item workflow.WorkItemID) ([]workflow.DispatchRound, error) {
	positions := make(map[string]int)
	var rounds []workflow.DispatchRound
	for _, comment := range implementationEvidence(comments) {
		body, ok := strings.CutPrefix(comment.Body, "<!-- skl.implement/v1\n")
		if !ok {
			continue
		}
		if !strings.HasSuffix(body, "\n-->") {
			return nil, workflow.Refuse("invalid trusted dispatch metadata")
		}
		var metadata implementationMetadata
		if err := json.Unmarshal([]byte(strings.TrimSuffix(body, "\n-->")), &metadata); err != nil {
			return nil, workflow.Refuse("invalid trusted dispatch metadata")
		}
		if metadata.Round == nil {
			continue
		}
		round := *metadata.Round
		if !validDispatchRound(round) || round.Item != item {
			return nil, workflow.Refuse("dispatch evidence has invalid or contradictory bindings")
		}
		index, exists := positions[round.ID]
		if !exists {
			positions[round.ID] = len(rounds)
			rounds = append(rounds, round)
			continue
		}
		previous := rounds[index]
		sameBinding := previous.ID == round.ID && previous.Lane == round.Lane && previous.Item == round.Item && previous.Submission == round.Submission && previous.Obligation == round.Obligation && previous.Directory == round.Directory && previous.Synchronization == round.Synchronization
		if !sameBinding || previous.Outcome != "" && previous != round || previous.Outcome == "" && round.Outcome == "" && previous != round {
			return nil, workflow.Refuse("conflicting trusted dispatch evidence")
		}
		if round.Outcome != "" {
			rounds[index] = round
		}
	}
	return rounds, nil
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

func (b *GitHubBackend) ImplementationItems(ctx context.Context, repository github.RepositoryID) ([]workflow.ImplementationItem, error) {
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
		item := workflow.ImplementationItem{ID: workflow.WorkItemID(strconv.Itoa(issue.Number)), Order: issue.Number, ClosingReference: fmt.Sprintf("Closes #%d", issue.Number), Branch: issue.Title, CreatedAt: issue.CreatedAt, State: state, Claimed: claimed, Problem: problem}
		if len(matches) > 1 {
			item.Problem = "multiple Submissions share the conventional branch"
		}
		if len(matches) == 1 {
			pull := matches[0]
			prState, prClaimed, prProblem := implementationLabels(pull.githubIssue)
			item.Submission = &workflow.Submission{ID: workflow.SubmissionID(strconv.Itoa(pull.Number)), Head: pull.Head.SHA, Base: pull.Base.Ref, Draft: pull.Draft, Body: pull.Body, State: prState, Claimed: prClaimed, CreatedAt: pull.CreatedAt}
			for _, label := range pull.Labels {
				item.Synchronization = item.Synchronization || label.Name == "sync"
			}
			item.Claimed = item.Claimed || prClaimed
			if prProblem != "" {
				item.Problem = prProblem
			}
			if state == workflow.NeedsHuman && (prState == workflow.Rework || prState == workflow.AwaitingReview) && prProblem == "" {
				item.State = prState
			} else if state == workflow.NeedsHuman || prState == workflow.NeedsHuman {
				item.State = workflow.NeedsHuman
			} else if prState != "" {
				if state == workflow.Ready && prState != workflow.AwaitingReview {
					item.Problem = "source Ready contradicts Submission lifecycle"
				} else if state != workflow.Ready {
					item.State = prState
				}
			}
			if pull.MergedAt != "" {
				item.State = workflow.Merged
			} else if pull.State == "closed" {
				item.State = workflow.Superseded
			}
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
						item.Submission.Comments = append(item.Submission.Comments, skilldist.ReviewComment{Body: review.Body, Author: review.User.Login, Association: review.Association, Commit: review.Commit, CreatedAt: review.SubmittedAt})
					}
					if len(reviews) < 100 {
						break
					}
				}
			}
		}
		if issue.State == "closed" && item.State != workflow.Merged && item.State != workflow.ReadyForMerge && item.State != workflow.Superseded {
			item.Problem = "source issue is closed without a merged Submission"
		}
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
			if item.Submission != nil && !strings.HasPrefix(comment.Body, "<!-- skl.implement/v1\n") {
				item.Submission.Comments = append(item.Submission.Comments, comment)
			}
		}
		for _, comment := range implementationEvidence(comments) {
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
				if metadata.ReviewedHead != "" && item.Submission != nil && metadata.ReviewRoundHead == item.Submission.Head {
					item.Submission.PreviousReviewedHead = metadata.ReviewedHead
				}
				if metadata.ResumeState != "" {
					item.ResumeState = metadata.ResumeState
				}
				if metadata.WatchdogHead != "" && item.Submission != nil {
					item.Submission.ReviewedHead = metadata.WatchdogHead
				}
				if metadata.Transition != nil {
					item.Transition = metadata.Transition
				}
			}
		}
		// A Rework push changes the Submission head, not the active round's
		// fixed reviewed obligation. Legacy metadata remains head-scoped.
		rounds, roundErr := dispatchRoundsFromComments(comments, item.ID)
		if roundErr != nil {
			item.Problem = roundErr.Error()
		}
		for _, round := range rounds {
			if round.Lane == workflow.ImplementLane && round.Outcome == "" && !round.Synchronization && round.Submission != "" && item.Submission != nil && round.Submission == item.Submission.ID {
				if item.Submission.PreviousReviewedHead != "" && item.Submission.PreviousReviewedHead != round.Obligation {
					item.Problem = "active dispatch contradicts the reviewed obligation"
				} else {
					item.Submission.PreviousReviewedHead = round.Obligation
				}
			}
		}
		if transition := item.Transition; transition != nil && !transition.Completed {
			allowed := func(record githubIssue, states []workflow.State) bool {
				for _, label := range record.Labels {
					single := githubIssue{Labels: []struct {
						Name string `json:"name"`
					}{label}}
					state, _, _ := implementationLabels(single)
					if state != "" && !slices.Contains(states, state) {
						return false
					}
				}
				return true
			}
			sourceStates := []workflow.State{}
			prStates := []workflow.State{}
			if transition.From == workflow.Ready {
				sourceStates = append(sourceStates, workflow.Ready)
			} else if transition.From == workflow.Rework {
				prStates = append(prStates, workflow.Rework)
			}
			prStates = append(prStates, transition.Target)
			if transition.Target == workflow.NeedsHuman {
				sourceStates = append(sourceStates, workflow.NeedsHuman)
			}
			valid := issue.State == "open" && allowed(issue, sourceStates) && (len(matches) == 0 || len(matches) == 1 && matches[0].State == "open" && allowed(matches[0].githubIssue, prStates))
			if !valid {
				item.Problem = "projections contradict the pending implementation transition"
			}
			if valid && (item.Problem == "" || item.Problem == "contradictory lifecycle projections" || item.Problem == "source Ready contradicts Submission lifecycle") {
				item.Problem = ""
				sourceState, sourceClaimed, sourceProblem := implementationLabels(issue)
				final := !sourceClaimed && sourceProblem == ""
				if transition.Target == workflow.AwaitingReview {
					final = final && sourceState == "" && item.Submission != nil && item.Submission.State == workflow.AwaitingReview && !item.Submission.Claimed
				} else {
					final = final && sourceState == workflow.NeedsHuman && (item.Submission == nil || item.Submission.State == workflow.NeedsHuman && !item.Submission.Claimed)
				}
				item.State = transition.From
				if final {
					item.State = transition.Target
				} else {
					item.ResumeState = transition.From
				}
			}
		}
		if owners[item.Branch] != issue.Number {
			item.Problem = "multiple source issues own the conventional branch"
		}
		if len(matches) == 1 && (item.Problem == "contradictory lifecycle projections" || item.Problem == "" && item.Claimed && (item.State == workflow.ReadyForMerge || item.State == workflow.NeedsHuman || item.State == workflow.Rework)) && (item.Transition == nil || item.Transition.Completed) {
			observation, err := b.ReviewSubmission(ctx, repository, item.Submission.ID)
			if err != nil {
				return nil, err
			}
			if observation.PendingReview != "" && observation.Head == item.Submission.Head && problem == "" {
				item.Problem = ""
				item.State = observation.PendingReview
				item.Submission.PendingReview = observation.PendingReview
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func (b *GitHubBackend) ImplementationTarget(ctx context.Context, repository github.RepositoryID) (string, error) {
	return b.validate(ctx, repository)
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
			comments = append(comments, skilldist.ReviewComment{Body: comment.Body, Author: comment.User.Login, Association: comment.Association, Commit: comment.Commit, Path: comment.Path, CreatedAt: comment.CreatedAt, Line: comment.Line, Side: comment.Side})
		}
		if len(batch) < 100 {
			return comments, nil
		}
	}
}

func (b *GitHubBackend) ImplementationHead(ctx context.Context, repository github.RepositoryID, branch string) (string, error) {
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
