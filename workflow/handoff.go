package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const implementationDecisionPrefix = "<!-- skl.implement.decision/v1 -->\n"

func OpaqueImplementationDecision(body string) string { return implementationDecisionPrefix + body }

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
		return outcome, err
	}
	var item ImplementationItem
	for _, candidate := range items {
		if candidate.ID == id {
			if item.ID != "" {
				return outcome, Refuse("ambiguous Work Item identity; repair duplicate attachments")
			}
			item = candidate
		}
	}
	if item.ID == "" {
		return outcome, Refuse("Work Item unavailable; inspect its stable identity")
	}

	resultPath := bodyPath
	if resultPath == "" {
		resultPath = decisionPath
	}
	if err := validateResultDirectory(resultPath); err != nil {
		return outcome, err
	}
	if decisionPath != "" && bodyPath != "" {
		if filepath.Dir(decisionPath) != filepath.Dir(bodyPath) {
			return outcome, errors.New("decision and Submission must share one private operation directory")
		}
		if err := validateResultDirectory(decisionPath); err != nil {
			return outcome, err
		}
	}
	var body, decision []byte
	if bodyPath != "" {
		body, err = os.ReadFile(bodyPath)
		if err != nil {
			return outcome, err
		}
	}
	if decisionPath != "" {
		decision, err = os.ReadFile(decisionPath)
		if err != nil {
			return outcome, err
		}
	}

	head, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
	if err != nil {
		return outcome, Refuse("local branch unavailable; restore its conventional worktree")
	}
	guard := func() error {
		local, e := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
		if e != nil || local != head {
			return Refuse("local head changed during handoff; commit and push a fixed head, then retry")
		}
		if bodyPath != "" {
			pushed, e := backend.ImplementationHead(ctx, item.Branch)
			if e != nil {
				return e
			}
			if pushed != head {
				return Refuse("remote head changed or local and remote heads differ; push a fixed head and retry")
			}
		}
		return nil
	}
	if err := guard(); err != nil {
		return outcome, err
	}

	if implementationHandoffMatches(item, target, head, string(body), bodyPath != "", string(decision), decisionPath != "") {
		outcome = ImplementationOutcome{Status: string(target), Item: &item}
		cleanupImplementationResult(&outcome, resultPath)
		return outcome, nil
	}
	if item.Claimed && item.State == target && !implementationSourceClaim(item) {
		return outcome, Refuse("claimed destination may belong to a later worker; inspect it without releasing or replacing its Claim")
	}
	if item.Problem != "" && !(item.Problem == "contradictory lifecycle projections" && implementationSourceClaim(item)) {
		return outcome, Refuse("Workflow State contradicts handoff: " + item.Problem + "; inspect projections before retrying")
	}
	from := item.State
	if item.Source != nil && item.Source.Claimed && slices.Contains(item.Source.States, Ready) {
		from = Ready
	} else if item.Submission != nil && item.Submission.Lifecycle != nil && item.Submission.Lifecycle.Claimed && slices.Contains(item.Submission.Lifecycle.States, Rework) {
		from = Rework
	}
	if (from != Ready && from != Rework) || !item.Claimed {
		return outcome, Refuse("Workflow State cannot prove this command owns a claimed Ready or Rework source; inspect before retrying")
	}
	if from == Rework && item.Submission == nil {
		return outcome, Refuse("Rework has no existing Submission; repair its attachment before resubmitting")
	}
	if from == Ready && bodyPath != "" && item.Submission != nil && item.Submission.Head == head && !implementationBodyMatches(item.ID, item.Submission.Body, string(body)) {
		return outcome, Refuse("published Submission differs from the supplied Result Document; restore the original body before retrying")
	}
	if from == Rework && bodyPath != "" && item.Submission != nil && item.Submission.Lifecycle != nil && slices.Contains(item.Submission.Lifecycle.States, target) && !implementationBodyMatches(item.ID, item.Submission.Body, string(body)) {
		return outcome, Refuse("published Rework Submission differs from the supplied Result Document; restore the original body before retrying")
	}
	if decisionPath != "" && implementationDecisionConflicts(item, string(decision)) {
		return outcome, Refuse("published decision differs from the supplied Result Document; restore the original decision before retrying")
	}
	if target == AwaitingReview && from == Rework && item.Submission != nil && implementationBodyMatches(item.ID, item.Submission.Body, string(body)) {
		return outcome, Refuse("unchanged Rework Submission cannot distinguish a stale command from a new handoff; inspect and supply the current round's Result Document")
	}

	policy := RequireRetiredArtifacts
	if target == NeedsHuman {
		policy = PreserveIncompleteArtifacts
	}
	history, err := InspectLedger(root, head, item.Branch, endpoints, policy)
	if err != nil {
		return outcome, err
	}
	if target == AwaitingReview {
		if from == Ready && item.TargetSnapshot == "" || item.TargetSnapshot != "" && gitOK(root, "merge-base", "--is-ancestor", item.TargetSnapshot, head) != nil {
			return outcome, Refuse("Target Snapshot is absent; merge the pinned snapshot, commit and push before retrying")
		}
		if history.Phase != "retired" || len(history.Violations) > 0 {
			return outcome, Refuse(fmt.Sprint(history.Violations) + "; complete permitted ticks, commit Completion, then delete the entire ledger in a child commit and push")
		}
	} else if len(history.endpointIdentityViolations) != 0 || len(history.acceptedBaselineViolations) != 0 {
		return outcome, Refuse(fmt.Sprint(append(history.endpointIdentityViolations, history.acceptedBaselineViolations...)) + "; repair endpoint identity or the accepted baseline before pausing")
	} else if bodyPath == "" {
		changed, e := git(root, "diff", "--name-only", history.Baseline, head, "--", ".", ":(exclude).changes/"+item.Branch)
		if e != nil {
			return outcome, e
		}
		if changed != "" || item.Submission != nil {
			return outcome, Refuse("implementation work needs preservation; push and supply --body for a draft Submission")
		}
	}

	if bodyPath != "" {
		base := item.TargetBranch
		if item.Submission != nil && item.Submission.Base != "" {
			base = item.Submission.Base
		}
		if base == "" {
			base, err = backend.ImplementationTarget(ctx)
			if err != nil {
				return outcome, err
			}
		}
		submission := Submission{Head: head, Base: base, Body: string(body), Draft: target == NeedsHuman}
		if item.Submission != nil {
			submission.ID = item.Submission.ID
		}
		submission, err = backend.PublishImplementation(ctx, item, submission)
		if err != nil {
			return outcome, err
		}
		if submission.Head != head || submission.Draft != (target == NeedsHuman) {
			return outcome, Refuse("Submission head changed or draft state contradicts handoff; inspect and retry")
		}
		item.Submission = &submission
	}
	if err := guard(); err != nil {
		return outcome, err
	}
	var writeErr error
	if target == AwaitingReview {
		writeErr = backend.AwaitImplementationReview(ctx, item, guard)
	} else {
		writeErr = backend.PauseImplementation(ctx, item, string(decision), guard)
	}
	if err := guard(); err != nil {
		return outcome, err
	}
	observed, observeErr := loadImplementation(ctx, backend)
	if observeErr != nil {
		return outcome, observeErr
	}
	for _, current := range observed {
		if current.ID == id && implementationHandoffMatches(current, target, head, string(body), bodyPath != "", string(decision), decisionPath != "") {
			outcome = ImplementationOutcome{Status: string(target), Item: &current}
			cleanupImplementationResult(&outcome, resultPath)
			return outcome, nil
		}
	}
	if writeErr != nil {
		return outcome, writeErr
	}
	return outcome, Refuse("handoff observations cannot prove completion; retain Result Documents and inspect before retrying")
}

func implementationSourceClaim(item ImplementationItem) bool {
	return item.Source != nil && item.Source.Claimed && slices.Contains(item.Source.States, Ready) || item.Submission != nil && item.Submission.Lifecycle != nil && item.Submission.Lifecycle.Claimed && slices.Contains(item.Submission.Lifecycle.States, Rework)
}

func implementationHandoffMatches(item ImplementationItem, target State, head, body string, bodySupplied bool, decision string, decisionSupplied bool) bool {
	if item.Problem != "" || item.State != target || item.Claimed {
		return false
	}
	if bodySupplied && (item.Submission == nil || item.Submission.Head != head || item.Submission.Draft != (target == NeedsHuman) || !implementationBodyMatches(item.ID, item.Submission.Body, body)) {
		return false
	}
	if !decisionSupplied {
		return true
	}
	wanted := OpaqueImplementationDecision(decision)
	for _, comment := range item.Feedback {
		if comment.Body == wanted {
			return true
		}
	}
	if item.Submission != nil {
		for _, comment := range item.Submission.Comments {
			if comment.Body == wanted {
				return true
			}
		}
	}
	return false
}

func implementationDecisionConflicts(item ImplementationItem, decision string) bool {
	wanted := OpaqueImplementationDecision(decision)
	for _, comment := range item.Feedback {
		if strings.HasPrefix(comment.Body, implementationDecisionPrefix) {
			if comment.Body != wanted {
				return true
			}
		}
	}
	if item.Submission != nil {
		for _, comment := range item.Submission.Comments {
			if strings.HasPrefix(comment.Body, implementationDecisionPrefix) {
				if comment.Body != wanted {
					return true
				}
			}
		}
	}
	return false
}

func implementationBodyMatches(id WorkItemID, actual, supplied string) bool {
	if _, err := strconv.Atoi(string(id)); err != nil {
		return actual == supplied
	}
	footer := "\n\nCloses #" + string(id) + "\n"
	if strings.HasSuffix(supplied, footer) {
		return actual == supplied
	}
	return actual == supplied+footer
}

func cleanupImplementationResult(outcome *ImplementationOutcome, resultPath string) {
	if err := removeResultDirectory(resultPath); err != nil {
		outcome.Reason = "handoff completed but private Result Document directory cleanup failed: " + err.Error()
	}
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
