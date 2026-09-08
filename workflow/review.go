package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	skilldist "github.com/vicrdguez/skills"
)

type ReviewBackend interface {
	ImplementationBackend
	ReviewSubmission(context.Context, RepositoryID, int) (Submission, error)
	PublishReview(context.Context, RepositoryID, ImplementationItem, []skilldist.ReviewComment, func() error) error
	CompleteReview(context.Context, RepositoryID, ImplementationItem, State, func() error) error
}

func SubmitWatchdog(ctx context.Context, root, remote string, number int, reviewed, head, verdict, summaryPath, findingsPath, bodyPath string, backend ReviewBackend) (ImplementationOutcome, error) {
	if head == "" {
		head = reviewed
	}
	if number <= 0 || reviewed == "" || verdict != "rework" && verdict != "pass" && verdict != "needs-human" || summaryPath == "" || verdict == "pass" && bodyPath == "" {
		return ImplementationOutcome{}, fmt.Errorf("submit requires --item, --reviewed-head, --verdict rework|pass|needs-human and --summary; pass also requires --body")
	}
	remote, err := ResolveGitHubRemote(root, remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, items, err := loadImplementation(ctx, root, remote, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	var item ImplementationItem
	for _, candidate := range items {
		if candidate.Number == number {
			if item.Number != 0 {
				return ImplementationOutcome{}, Refuse("ambiguous Work Item")
			}
			item = candidate
		}
	}
	if item.Problem != "" || item.Submission == nil || item.State != AwaitingReview || !item.Claimed || item.Submission.ReviewedHead != reviewed {
		return ImplementationOutcome{}, Refuse("verdict requires the fixed Awaiting Review Claim")
	}
	if head != reviewed && (verdict != "pass" || gitOK(root, "merge-base", "--is-ancestor", reviewed, head) != nil) {
		return ImplementationOutcome{}, Refuse("post-marker head must descend from the fixed reviewed head on pass")
	}
	guard := func() error {
		local, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
		if err != nil || local != head {
			return Refuse("local reviewed head changed; restore the fixed head")
		}
		remote, err := backend.ImplementationHead(ctx, repository, item.Branch)
		if err != nil {
			return err
		}
		if remote != head {
			return Refuse("remote reviewed head changed; push the fixed head")
		}
		submission, err := backend.ReviewSubmission(ctx, repository, item.Submission.Number)
		if err != nil {
			return err
		}
		if submission.Head != head {
			return Refuse("Submission head changed during verdict")
		}
		return nil
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	history, err := InspectLedger(root, head, item.Branch)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if history.Phase != "retired" || len(history.Violations) > 0 {
		return ImplementationOutcome{}, Refuse("restore valid retired ledger history")
	}
	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	comments := []skilldist.ReviewComment{{Body: string(summary), Commit: head}}
	if findingsPath != "" {
		data, err := os.ReadFile(findingsPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		var anchors []struct {
			Path     string `json:"path"`
			Line     int    `json:"line"`
			Side     string `json:"side"`
			BodyFile string `json:"body_file"`
		}
		if err := json.Unmarshal(data, &anchors); err != nil {
			return ImplementationOutcome{}, err
		}
		for _, a := range anchors {
			if a.Path == "" || path.IsAbs(a.Path) || path.Clean(a.Path) != a.Path || strings.HasPrefix(a.Path, "../") || a.Line <= 0 || a.Side != "LEFT" && a.Side != "RIGHT" {
				return ImplementationOutcome{}, fmt.Errorf("invalid structured inline anchor")
			}
			body, err := os.ReadFile(a.BodyFile)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			comments = append(comments, skilldist.ReviewComment{Body: string(body), Commit: head, Path: a.Path, Line: a.Line, Side: a.Side})
		}
	}
	submission, err := backend.ReviewSubmission(ctx, repository, item.Submission.Number)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	target := Rework
	if submission.Bounces > 0 {
		target = NeedsHuman
		item.ResumeState = Rework
	}
	if verdict == "needs-human" {
		target = NeedsHuman
		item.ResumeState = AwaitingReview
	}
	if verdict == "pass" {
		if submission.Mergeability != "mergeable" && submission.Mergeability != "conflicting" {
			return ImplementationOutcome{}, Refuse("mergeability unavailable; wait for backend evaluation and retry")
		}
		target = ReadyForMerge
		if submission.Mergeability == "conflicting" {
			target = Rework
			item.Synchronization = true
			item.TargetBranch = submission.Base
			item.TargetSnapshot, err = backend.ImplementationHead(ctx, repository, submission.Base)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			if item.TargetSnapshot == "" {
				return ImplementationOutcome{}, Refuse("current target unavailable; restore it and retry")
			}
		}
		body, err := os.ReadFile(bodyPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		wanted := *item.Submission
		wanted.Body = string(body)
		footer := fmt.Sprintf("\n\nCloses #%d\n", number)
		if !strings.HasSuffix(wanted.Body, footer) {
			wanted.Body += footer
		}
		published, err := backend.PublishImplementation(ctx, repository, item, wanted)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if published.Head != head {
			return ImplementationOutcome{}, Refuse("Submission head changed during final body publication")
		}
	}
	if err := backend.PublishReview(ctx, repository, item, comments, guard); err != nil {
		return ImplementationOutcome{}, err
	}
	if err := backend.CompleteReview(ctx, repository, item, target, guard); err != nil {
		return ImplementationOutcome{}, err
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	observed, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for _, current := range observed {
		if current.Number == number && current.Problem == "" && current.State == target && !current.Claimed {
			return ImplementationOutcome{Status: string(target), Item: &current, Head: head}, nil
		}
	}
	return ImplementationOutcome{}, Refuse("review handoff incomplete; retry the same verdict and Result Documents")
}
