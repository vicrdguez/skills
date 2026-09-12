package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	skilldist "github.com/vicrdguez/skills"
)

type ReviewBackend interface {
	ImplementationBackend
	ReviewSubmission(context.Context, SubmissionID) (Submission, error)
	// SubmissionBodyMatches compares an observation with the body publication would produce.
	SubmissionBodyMatches(id WorkItemID, actual, supplied string) (bool, error)
	PublishReview(context.Context, ImplementationItem, []skilldist.ReviewComment, func() error) error
	CompleteReview(context.Context, ImplementationItem, State, func() error) error
}

func SubmitWatchdog(ctx context.Context, root, remote string, id WorkItemID, reviewed, head, verdict, summaryPath, findingsPath, bodyPath string, endpoints ArtifactEndpoints, backend ReviewBackend) (outcome ImplementationOutcome, err error) {
	defer func() {
		if err != nil || outcome.Item == nil || outcome.Item.Claimed {
			return
		}
		dir := filepath.Dir(summaryPath)
		if !filepath.IsAbs(summaryPath) || !strings.HasPrefix(filepath.Base(dir), "skl-watchdog-") {
			return
		}
		parent, e := filepath.EvalSymlinks(filepath.Dir(dir))
		temp, te := filepath.EvalSymlinks(os.TempDir())
		if e != nil || te != nil || parent != temp {
			return
		}
		info, e := os.Lstat(dir)
		if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return
		}
		marker, e := os.ReadFile(filepath.Join(dir, ".skl-result"))
		if e != nil || string(marker) != "skl.watchdog/v1\n" {
			return
		}
		entries, e := os.ReadDir(dir)
		if e != nil {
			err = e
			return
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() || entry.Name() != ".skl-result" && filepath.Ext(entry.Name()) != ".md" && filepath.Ext(entry.Name()) != ".json" {
				err = fmt.Errorf("handoff completed but private directory has unexpected files; preserve it for explicit cleanup")
				return
			}
		}
		err = os.RemoveAll(dir)
	}()
	if head == "" {
		head = reviewed
	}
	if id == "" || reviewed == "" || verdict != "rework" && verdict != "pass" && verdict != "needs-human" || summaryPath == "" || verdict == "pass" && bodyPath == "" {
		return ImplementationOutcome{}, fmt.Errorf("submit requires --item, --reviewed-head, --verdict rework|pass|needs-human and --summary; pass also requires --body")
	}
	items, err := loadImplementation(ctx, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	var item ImplementationItem
	for _, candidate := range items {
		if candidate.ID == id {
			if item.ID != "" {
				return ImplementationOutcome{}, Refuse("ambiguous Work Item")
			}
			item = candidate
		}
	}
	if item.Problem != "" || item.Submission == nil || item.State == AwaitingReview && !item.Claimed || item.Submission.ReviewedHead != reviewed {
		return ImplementationOutcome{}, Refuse("verdict requires the fixed Awaiting Review Claim")
	}
	if head != reviewed && (verdict != "pass" || gitOK(root, "merge-base", "--is-ancestor", reviewed, head) != nil) {
		return ImplementationOutcome{}, Refuse("post-marker head must descend from the fixed reviewed head on pass")
	}
	requireMergeable := false
	guard := func() error {
		local, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
		if err != nil || local != head {
			return Refuse("local reviewed head changed; restore the fixed head")
		}
		remote, err := backend.ImplementationHead(ctx, item.Branch)
		if err != nil {
			return err
		}
		if remote != head {
			return Refuse("remote reviewed head changed; push the fixed head")
		}
		submission, err := backend.ReviewSubmission(ctx, item.Submission.ID)
		if err != nil {
			return err
		}
		if submission.Head != head || submission.Merged || submission.Draft {
			return Refuse("Submission head changed during verdict")
		}
		if requireMergeable && submission.Mergeability != "mergeable" {
			return Refuse("mergeability changed during verdict; retry to observe the current target")
		}
		return nil
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	history, err := InspectLedger(root, reviewed, item.Branch, endpoints, RequireRetiredArtifacts)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if history.Phase != "retired" || len(history.Violations) > 0 {
		return ImplementationOutcome{}, Refuse(fmt.Sprint(history.Violations) + "; restore valid retired ledger history at reviewed head " + reviewed)
	}
	if head != reviewed {
		history, err = InspectLedger(root, head, item.Branch, endpoints, RequireRetiredArtifacts)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if history.Phase != "retired" || len(history.Violations) > 0 {
			return ImplementationOutcome{}, Refuse(fmt.Sprint(history.Violations) + "; keep the ledger retired at final head " + head)
		}
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
	submission, err := backend.ReviewSubmission(ctx, item.Submission.ID)
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
		if item.State == Rework && item.Synchronization {
			target = Rework
		} else if submission.Mergeability == "conflicting" && (item.State == AwaitingReview || item.State == ReadyForMerge) {
			target = Rework
			item.Synchronization = true
			item.TargetBranch = submission.Base
			item.TargetSnapshot, err = backend.ImplementationHead(ctx, submission.Base)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			if item.TargetSnapshot == "" {
				return ImplementationOutcome{}, Refuse("current target unavailable; restore it and retry")
			}
		}
		requireMergeable = target == ReadyForMerge
	}
	if item.State != AwaitingReview {
		compatible := verdict == "rework" && (item.State == Rework || item.State == NeedsHuman) || verdict == "needs-human" && item.State == NeedsHuman || verdict == "pass" && (item.State == ReadyForMerge || item.State == Rework && item.Synchronization)
		for _, wanted := range comments {
			found := false
			for _, existing := range item.Submission.Comments {
				if wanted.Body == existing.Body && wanted.Path == existing.Path && (wanted.Path == "" || wanted.Commit == existing.Commit && wanted.Line == existing.Line && wanted.Side == existing.Side) {
					found = true
					break
				}
			}
			compatible = compatible && found
		}
		if verdict == "pass" {
			body, err := os.ReadFile(bodyPath)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			matches, err := backend.SubmissionBodyMatches(item.ID, item.Submission.Body, string(body))
			if err != nil {
				return ImplementationOutcome{}, err
			}
			compatible = compatible && matches
		}
		if !compatible {
			return ImplementationOutcome{}, Refuse("completed or partial review differs from supplied verdict; restore its exact Result Documents")
		}
		if verdict != "pass" {
			target = item.State
		}
		if item.Claimed || target != item.State {
			if err := backend.CompleteReview(ctx, item, target, guard); err != nil {
				return ImplementationOutcome{}, err
			}
			current, err := backend.ImplementationItems(ctx)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			for _, c := range current {
				if c.ID == item.ID && c.Problem == "" && !c.Claimed && c.State == target {
					return ImplementationOutcome{Status: string(c.State), Item: &c, Head: head}, guard()
				}
			}
			return ImplementationOutcome{}, Refuse("review handoff still incomplete; retain Claim and retry")
		}
		return ImplementationOutcome{Status: string(item.State), Item: &item, Head: head}, guard()
	}
	if verdict == "pass" {
		body, err := os.ReadFile(bodyPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		wanted := *item.Submission
		wanted.Body = string(body)
		published, err := backend.PublishImplementation(ctx, item, wanted)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if published.Head != head {
			return ImplementationOutcome{}, Refuse("Submission head changed during final body publication")
		}
	}
	if err := backend.PublishReview(ctx, item, comments, guard); err != nil {
		return ImplementationOutcome{}, err
	}
	if err := backend.CompleteReview(ctx, item, target, guard); err != nil {
		return ImplementationOutcome{}, err
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	observed, err := backend.ImplementationItems(ctx)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for _, current := range observed {
		if current.ID == id && current.Problem == "" && current.State == target && !current.Claimed {
			return ImplementationOutcome{Status: string(target), Item: &current, Head: head}, nil
		}
	}
	return ImplementationOutcome{}, Refuse("review handoff incomplete; retry the same verdict and Result Documents")
}
