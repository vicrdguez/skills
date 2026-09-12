package workflow

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func SubmitImplementation(ctx context.Context, root, remote string, id WorkItemID, bodyPath string, endpoints ArtifactEndpoints, backend ImplementationBackend) (ImplementationOutcome, error) {
	if id == "" || bodyPath == "" {
		return ImplementationOutcome{}, errors.New("submit requires --item and --body")
	}
	return handoffImplementation(ctx, root, remote, id, AwaitingReview, "", bodyPath, endpoints, backend)
}

func PauseImplementation(ctx context.Context, root, remote string, id WorkItemID, reason, decisionPath, bodyPath string, endpoints ArtifactEndpoints, backend ImplementationBackend) (ImplementationOutcome, error) {
	if id == "" || decisionPath == "" || !slices.Contains([]string{"contradictory_artifacts", "mandatory_rule", "frozen_interface", "disputed_blocker", "bounce_cap"}, reason) {
		return ImplementationOutcome{}, errors.New("Needs Human requires --item, --decision and a permitted --reason")
	}
	return handoffImplementation(ctx, root, remote, id, NeedsHuman, decisionPath, bodyPath, endpoints, backend)
}

func handoffImplementation(ctx context.Context, root, remote string, id WorkItemID, target State, decisionPath, bodyPath string, endpoints ArtifactEndpoints, backend ImplementationBackend) (outcome ImplementationOutcome, err error) {
	items, err := loadImplementation(ctx, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	var item ImplementationItem
	for _, candidate := range items {
		if candidate.ID == id {
			if item.ID != "" {
				return ImplementationOutcome{}, Refuse("ambiguous Work Item identity; repair duplicate attachments")
			}
			item = candidate
		}
	}
	if item.ID == "" || item.Problem != "" {
		return ImplementationOutcome{}, Refuse("Workflow State contradicts handoff: " + item.Problem + "; repair projections before retrying")
	}
	resultPath := bodyPath
	if resultPath == "" {
		resultPath = decisionPath
	}
	directory := filepath.Base(filepath.Dir(resultPath))
	head, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
	if err != nil {
		return ImplementationOutcome{}, Refuse("local branch unavailable; restore its conventional worktree")
	}
	guard := func() error {
		local, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
		if err != nil || local != head {
			return Refuse("local head changed during handoff; commit and push a fixed head, then retry")
		}
		if bodyPath != "" {
			remote, err := backend.ImplementationHead(ctx, item.Branch)
			if err != nil {
				return err
			}
			if remote != head {
				return Refuse("remote head changed or local and remote heads differ; push a fixed head and retry")
			}
		}
		return nil
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	prior := item.Transition
	if prior != nil && prior.Completed && prior.Target == target && prior.Directory == directory && prior.Head == head && item.State == target && !item.Claimed {
		if _, err := os.Lstat(filepath.Dir(resultPath)); err == nil {
			if err := removeResultDirectory(resultPath); err != nil {
				return ImplementationOutcome{}, err
			}
		} else if !os.IsNotExist(err) {
			return ImplementationOutcome{}, err
		}
		return ImplementationOutcome{Status: string(target), Item: &item}, nil
	}
	from := item.State
	if prior != nil && !prior.Completed {
		from = prior.From
	}
	if from != Ready && from != Rework || !item.Claimed && (prior == nil || prior.Completed) {
		return ImplementationOutcome{}, Refuse("Workflow State contradicts submission; repair the claimed Ready or Rework projections and retry")
	}
	if from == Rework && item.Submission == nil {
		return ImplementationOutcome{}, Refuse("Rework has no existing Submission; repair its attachment before resubmitting")
	}
	if err := validateResultDirectory(resultPath); err != nil {
		return ImplementationOutcome{}, err
	}
	if decisionPath != "" && bodyPath != "" {
		if filepath.Dir(decisionPath) != filepath.Dir(bodyPath) {
			return ImplementationOutcome{}, errors.New("decision and Submission must share one private operation directory")
		}
		if err := validateResultDirectory(decisionPath); err != nil {
			return ImplementationOutcome{}, err
		}
	}
	var body, decision []byte
	if bodyPath != "" {
		body, err = os.ReadFile(bodyPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
	}
	if decisionPath != "" {
		decision, err = os.ReadFile(decisionPath)
		if err != nil {
			return ImplementationOutcome{}, err
		}
	}
	policy := RequireRetiredArtifacts
	if target == NeedsHuman {
		policy = PreserveIncompleteArtifacts
	}
	history, err := InspectLedger(root, head, item.Branch, endpoints, policy)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if target == AwaitingReview {
		if from == Ready && item.TargetSnapshot == "" || item.TargetSnapshot != "" && gitOK(root, "merge-base", "--is-ancestor", item.TargetSnapshot, head) != nil {
			return ImplementationOutcome{}, Refuse("Target Snapshot is absent; merge the pinned snapshot, commit and push before retrying")
		}
		if history.Phase != "retired" || len(history.Violations) > 0 {
			return ImplementationOutcome{}, Refuse(fmt.Sprint(history.Violations) + "; complete permitted ticks, commit Completion, then delete the entire ledger in a child commit and push")
		}
	} else if len(history.endpointIdentityViolations) != 0 || len(history.acceptedBaselineViolations) != 0 {
		return ImplementationOutcome{}, Refuse(fmt.Sprint(append(history.endpointIdentityViolations, history.acceptedBaselineViolations...)) + "; repair endpoint identity or the accepted baseline before pausing")
	} else if bodyPath == "" {
		changed, err := git(root, "diff", "--name-only", history.Baseline, head, "--", ".", ":(exclude).changes/"+item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if changed != "" || item.Submission != nil {
			return ImplementationOutcome{}, Refuse("implementation work needs preservation; push and supply --body for a draft Submission")
		}
	}
	transition := ImplementationTransition{From: from, Target: target, Head: head, Directory: directory, BodyDigest: fmt.Sprintf("%x", sha256.Sum256(body)), DecisionDigest: fmt.Sprintf("%x", sha256.Sum256(decision))}
	if prior != nil && !prior.Completed && *prior != transition && (item.State != from || !item.Claimed) {
		return ImplementationOutcome{}, Refuse("pending handoff differs from supplied intent or head; restore its fixed Result Documents and resume")
	}
	item.State = from
	if err := backend.RecordImplementationTransition(ctx, item, transition); err != nil {
		return ImplementationOutcome{}, err
	}
	item.Transition = &transition
	defer func() {
		if err != nil {
			transition.Completed = false
			err = errors.Join(err, backend.RecordImplementationTransition(ctx, item, transition), backend.RetainImplementationClaim(ctx, item))
		}
	}()
	if bodyPath != "" {
		base := item.TargetBranch
		if item.Submission != nil && item.Submission.Base != "" {
			base = item.Submission.Base
		}
		if base == "" {
			base, err = backend.ImplementationTarget(ctx)
			if err != nil {
				return ImplementationOutcome{}, err
			}
		}
		submission := Submission{Head: head, Base: base, Body: string(body), Draft: target == NeedsHuman}
		if item.Submission != nil {
			submission.ID = item.Submission.ID
		}
		submission, err = backend.PublishImplementation(ctx, item, submission)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if submission.Head != head || submission.Draft != (target == NeedsHuman) {
			return ImplementationOutcome{}, Refuse("Submission head changed or draft state contradicts handoff; inspect and retry")
		}
		item.Submission = &submission
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	writeErr := projectImplementation(ctx, backend, item, target, string(decision), guard)
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	observed, err := loadImplementation(ctx, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if err := guard(); err != nil {
		return ImplementationOutcome{}, err
	}
	for _, current := range observed {
		if current.ID != id {
			continue
		}
		if current.Problem == "" && current.State == target && !current.Claimed && (bodyPath == "" || current.Submission != nil && current.Submission.Head == head && current.Submission.Draft == (target == NeedsHuman)) {
			transition.Completed = true
			if err := backend.RecordImplementationTransition(ctx, current, transition); err != nil {
				return ImplementationOutcome{}, err
			}
			if err := guard(); err != nil {
				return ImplementationOutcome{}, err
			}
			current.Transition = &transition
			if err := removeResultDirectory(resultPath); err != nil {
				return ImplementationOutcome{}, err
			}
			return ImplementationOutcome{Status: string(target), Item: &current}, nil
		}
	}
	if writeErr != nil {
		return ImplementationOutcome{}, writeErr
	}
	return ImplementationOutcome{}, Refuse("handoff projection is incomplete; retry the same semantic command with retained Result Documents")
}

func projectImplementation(ctx context.Context, backend ImplementationBackend, item ImplementationItem, target State, decision string, guard func() error) error {
	var err error
	if target == AwaitingReview {
		err = backend.AwaitImplementationReview(ctx, item, guard)
	} else {
		err = backend.PauseImplementation(ctx, item, decision, guard)
	}
	if err != nil {
		// Restore the Claim before read-back can mistake a failed write for completion.
		err = errors.Join(err, backend.RetainImplementationClaim(ctx, item))
	}
	return err
}

func removeResultDirectory(bodyPath string) error {
	if err := validateResultDirectory(bodyPath); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Dir(bodyPath))
}

func validateResultDirectory(bodyPath string) error {
	directory := filepath.Dir(bodyPath)
	if !filepath.IsAbs(bodyPath) || !slices.Contains([]string{"submission.md", "decision.md"}, filepath.Base(bodyPath)) {
		return errors.New("use the absolute Result Document path from the packet")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("Result Document directory must be private and not a symlink")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(directory))
	temporary, tempErr := filepath.EvalSymlinks(os.TempDir())
	if err != nil || tempErr != nil || parent != temporary {
		return errors.New("Result Document directory must be engine-created outside the repository")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !slices.Contains([]string{".skl-result", "submission.md", "decision.md"}, entry.Name()) || !entry.Type().IsRegular() {
			return errors.New("private Result Document directory contains unexpected files or symlinks")
		}
	}
	marker, err := os.ReadFile(filepath.Join(directory, ".skl-result"))
	if err != nil || string(marker) != "skl.implement/v1\n" || !strings.HasPrefix(filepath.Base(directory), "skl-implement-") {
		return errors.New("Result Document must be in an engine-created private operation directory")
	}
	return nil
}
