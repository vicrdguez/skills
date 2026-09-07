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

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

type githubPull struct {
	githubIssue
	NodeID   string `json:"node_id"`
	Draft    bool   `json:"draft"`
	MergedAt string `json:"merged_at"`
	Head     struct {
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

func (b *GitHubBackend) ClaimImplementation(ctx context.Context, repository workflow.RepositoryID, item workflow.ImplementationItem) error {
	items, err := b.ImplementationItems(ctx, repository)
	if err != nil {
		return err
	}
	for _, current := range items {
		if current.Number != item.Number {
			continue
		}
		if current.Problem != "" || current.State != item.State || current.Branch != item.Branch {
			return workflow.Refuse("implementation state changed before Claim; inspect projections and explicitly resume")
		}
		if item.TargetSnapshot != "" {
			if current.TargetSnapshot != "" && current.TargetSnapshot != item.TargetSnapshot {
				return workflow.Refuse("Target Snapshot contradicts recorded obligation; use the original pinned commit")
			}
			if err := b.publishImplementationMetadata(ctx, repository, item.Number, implementationMetadata{TargetSnapshot: item.TargetSnapshot, TargetBranch: item.TargetBranch}); err != nil {
				return err
			}
		}
		if item.Submission != nil && item.Submission.PreviousReviewedHead != "" {
			if err := b.publishImplementationMetadata(ctx, repository, item.Number, implementationMetadata{ReviewedHead: item.Submission.PreviousReviewedHead, ReviewRoundHead: item.Submission.Head}); err != nil {
				return err
			}
		}
		if current.Claimed {
			return nil
		}
		number := item.Number
		if item.State == workflow.Rework {
			if item.Submission == nil {
				return workflow.Refuse("Rework has no Submission; repair its attachment")
			}
			number = item.Submission.Number
		}
		return b.implementationLabelMutation(ctx, repository, number, []string{"wip"}, nil, nil)
	}
	return workflow.Refuse("Work Item disappeared before Claim; inspect its stable identity")
}

func (b *GitHubBackend) publishImplementationMetadata(ctx context.Context, repository workflow.RepositoryID, number int, metadata implementationMetadata) error {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return b.implementationComment(ctx, repository, number, "<!-- skl.implement/v1\n"+string(payload)+"\n-->", true)
}

func (b *GitHubBackend) implementationComment(ctx context.Context, repository workflow.RepositoryID, number int, body string, metadata bool) error {
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

func (b *GitHubBackend) implementationLabelMutation(ctx context.Context, repository workflow.RepositoryID, number int, add, remove []string, guard func() error) error {
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

func (b *GitHubBackend) PublishImplementation(ctx context.Context, repository workflow.RepositoryID, item workflow.ImplementationItem, wanted workflow.Submission) (workflow.Submission, error) {
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
	if len(matches) > 1 || len(matches) == 1 && (matches[0].State == "closed" || wanted.Number != 0 && matches[0].Number != wanted.Number) {
		return workflow.Submission{}, workflow.Refuse("ambiguous or closed existing Submission; repair the attachment")
	}
	if len(matches) == 0 && wanted.Number != 0 {
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
	wanted.Number = observed.Number
	return wanted, nil
}

func (b *GitHubBackend) AwaitImplementationReview(ctx context.Context, repository workflow.RepositoryID, item workflow.ImplementationItem, guard func() error) (err error) {
	defer func() {
		if err != nil {
			number := item.Number
			if item.State == workflow.Rework && item.Submission != nil {
				number = item.Submission.Number
			}
			err = errors.Join(err, b.implementationLabelMutation(ctx, repository, number, []string{"wip"}, nil, nil))
		}
	}()
	if item.Submission == nil {
		return workflow.Refuse("review requires a durable Submission; publish it before retrying")
	}
	var issue githubIssue
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/issues/%d", item.Submission.Number), nil, &issue); err != nil {
		return err
	}
	state, _, problem := implementationLabels(issue)
	// Adding review is the first durable step; a retry can see both review and rework.
	if state == workflow.NeedsHuman || state == workflow.ReadyForMerge || state == workflow.Ready || problem != "" && state != workflow.Rework && state != workflow.AwaitingReview {
		return workflow.Refuse("Submission lifecycle contradicts review handoff; repair its projections")
	}
	if err := b.implementationLabelMutation(ctx, repository, item.Submission.Number, []string{"review"}, []string{"rework", "wip"}, guard); err != nil {
		return err
	}
	return b.implementationLabelMutation(ctx, repository, item.Number, nil, []string{"ready", "wip"}, guard)
}

func (b *GitHubBackend) PauseImplementation(ctx context.Context, repository workflow.RepositoryID, item workflow.ImplementationItem, decision string, guard func() error) (err error) {
	defer func() {
		if err != nil {
			number := item.Number
			if item.State == workflow.Rework && item.Submission != nil {
				number = item.Submission.Number
			}
			err = errors.Join(err, b.implementationLabelMutation(ctx, repository, number, []string{"wip"}, nil, nil))
		}
	}()
	if err := guard(); err != nil {
		return err
	}
	if err := b.publishImplementationMetadata(ctx, repository, item.Number, implementationMetadata{ResumeState: item.State}); err != nil {
		return err
	}
	number := item.Number
	if item.Submission != nil {
		number = item.Submission.Number
	}
	if err := b.implementationComment(ctx, repository, number, decision, false); err != nil {
		return err
	}
	if item.Submission != nil {
		if err := b.implementationLabelMutation(ctx, repository, number, []string{"needs-human"}, []string{"review", "rework", "wip"}, guard); err != nil {
			return err
		}
	}
	return b.implementationLabelMutation(ctx, repository, item.Number, []string{"needs-human"}, []string{"ready", "wip"}, guard)
}

func (b *GitHubBackend) RecordImplementationTransition(ctx context.Context, repository workflow.RepositoryID, item workflow.ImplementationItem, transition workflow.ImplementationTransition) error {
	return b.publishImplementationMetadata(ctx, repository, item.Number, implementationMetadata{Transition: &transition})
}

func (b *GitHubBackend) RetainImplementationClaim(ctx context.Context, repository workflow.RepositoryID, item workflow.ImplementationItem) error {
	number := item.Number
	if item.State == workflow.Rework && item.Submission != nil {
		number = item.Submission.Number
	}
	return b.implementationLabelMutation(ctx, repository, number, []string{"wip"}, nil, nil)
}

func trustedMetadata(comment skilldist.ReviewComment) bool {
	return slices.Contains([]string{"OWNER", "MEMBER", "COLLABORATOR"}, comment.Association)
}

type implementationMetadata struct {
	Transition      *workflow.ImplementationTransition `json:"transition,omitempty"`
	TargetSnapshot  string                             `json:"target_snapshot,omitempty"`
	TargetBranch    string                             `json:"target_branch,omitempty"`
	ReviewedHead    string                             `json:"reviewed_head,omitempty"`
	ReviewRoundHead string                             `json:"review_round_head,omitempty"`
	ResumeState     workflow.State                     `json:"resume_state,omitempty"`
}

func (b *GitHubBackend) ImplementationItems(ctx context.Context, repository workflow.RepositoryID) ([]workflow.ImplementationItem, error) {
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
		item := workflow.ImplementationItem{Number: issue.Number, Branch: issue.Title, CreatedAt: issue.CreatedAt, State: state, Claimed: claimed, Problem: problem}
		if len(matches) > 1 {
			item.Problem = "multiple Submissions share the conventional branch"
		}
		if len(matches) == 1 {
			pull := matches[0]
			prState, prClaimed, prProblem := implementationLabels(pull.githubIssue)
			item.Submission = &workflow.Submission{Number: pull.Number, Head: pull.Head.SHA, Base: pull.Base.Ref, Draft: pull.Draft, Body: pull.Body, State: prState, Claimed: prClaimed}
			item.Claimed = item.Claimed || prClaimed
			if prProblem != "" {
				item.Problem = prProblem
			}
			if state == workflow.NeedsHuman || prState == workflow.NeedsHuman {
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
			if item.State == workflow.Rework {
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
		if issue.State == "closed" && item.State != workflow.Merged && item.State != workflow.ReadyForMerge {
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
					item.Blockers = append(item.Blockers, blocker.Number)
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
						} else if !slices.Contains(item.Blockers, number) {
							item.Blockers = append(item.Blockers, number)
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
				if metadata.ReviewedHead != "" && item.Submission != nil && metadata.ReviewRoundHead == item.Submission.Head {
					item.Submission.PreviousReviewedHead = metadata.ReviewedHead
				}
				if metadata.ResumeState != "" {
					item.ResumeState = metadata.ResumeState
				}
				if metadata.Transition != nil {
					item.Transition = metadata.Transition
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
		items = append(items, item)
	}
	return items, nil
}

func (b *GitHubBackend) ImplementationTarget(ctx context.Context, repository workflow.RepositoryID) (string, error) {
	return b.Validate(ctx, repository)
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

func (b *GitHubBackend) implementationComments(ctx context.Context, repository workflow.RepositoryID, stream string) ([]skilldist.ReviewComment, error) {
	var comments []skilldist.ReviewComment
	for page := 1; ; page++ {
		var batch []struct {
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
			comments = append(comments, skilldist.ReviewComment{Body: comment.Body, Author: comment.User.Login, Association: comment.Association, Commit: comment.Commit, Path: comment.Path, CreatedAt: comment.CreatedAt})
		}
		if len(batch) < 100 {
			return comments, nil
		}
	}
}

func (b *GitHubBackend) ImplementationHead(ctx context.Context, repository workflow.RepositoryID, branch string) (string, error) {
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
