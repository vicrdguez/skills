package workflow

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// ReviewInspection is a read-only continuation of one fixed Watchdog invocation.
// Startup cannot establish these facts without the selected project's objects.
type ReviewInspection struct {
	Status             string        `json:"status"`
	Reason             string        `json:"reason,omitempty"`
	Item               WorkItemID    `json:"work_item"`
	Submission         SubmissionID  `json:"submission"`
	Remote             string        `json:"remote"`
	SubmissionBase     string        `json:"submission_base"`
	BodySHA256         string        `json:"submission_body_sha256"`
	Branch             string        `json:"branch"`
	ReviewedHead       string        `json:"reviewed_head"`
	Head               string        `json:"head"`
	ReviewNumber       uint64        `json:"review_number"`
	PreviousHead       string        `json:"previous_reviewed_head,omitempty"`
	ResultDirectory    string        `json:"result_directory"`
	SuppliedBaseline   string        `json:"supplied_artifact_baseline,omitempty"`
	SuppliedCompletion string        `json:"supplied_artifact_completion,omitempty"`
	Ledger             LedgerHistory `json:"ledger"`
	Comparison         string        `json:"comparison,omitempty"`
	FallbackReason     string        `json:"fallback_reason,omitempty"`
}

func InspectWatchdog(ctx context.Context, root, remote string, id WorkItemID, submission SubmissionID, base, bodySHA256, reviewed string, number uint64, previous, resultDirectory string, endpoints ArtifactEndpoints, backend ImplementationBackend) (ReviewInspection, error) {
	result := ReviewInspection{Status: "fix_required", Item: id, Submission: submission, Remote: remote, SubmissionBase: base, BodySHA256: bodySHA256, ReviewedHead: reviewed, Head: reviewed, ReviewNumber: number, PreviousHead: previous, ResultDirectory: resultDirectory, SuppliedBaseline: endpoints.Baseline, SuppliedCompletion: endpoints.Completion}
	if id == "" || submission == "" || base == "" || len(bodySHA256) != 64 || reviewed == "" || number == 0 || resultDirectory == "" {
		result.Reason = "inspect requires the fixed --item, --submission, --base, --submission-body-sha256, --reviewed-head, --review-number, and --result-directory from this invocation"
		return result, nil
	}
	marker, err := os.ReadFile(filepath.Join(resultDirectory, ".skl-result"))
	if err != nil || string(marker) != "skl.watchdog/v1\n" {
		result.Reason = "private Watchdog Result Document directory is unavailable or does not belong to this invocation; preserve the original path and inspect it"
		return result, nil
	}
	selection, ok := backend.(SelectionBackend)
	if !ok {
		return result, fmt.Errorf("workflow backend does not support selected Work Item inspection")
	}
	item, err := selection.ResumedImplementation(ctx, id, "")
	if err != nil {
		return result, fmt.Errorf("selected Work Item observation failed during fixed review inspection: %w", err)
	}
	if item.ID != id || item.Problem != "" || item.State != AwaitingReview || !item.Claimed || item.Submission == nil || item.Submission.ID != submission || item.Submission.Head != reviewed || item.Submission.Base != base || item.Submission.Draft || fmt.Sprintf("%x", sha256.Sum256([]byte(item.Submission.Body))) != bodySHA256 {
		result.Reason = "selected Work Item, Submission attachment, Claim, or original reviewed head changed; inspect the fixed invocation and stop until repaired"
		return result, nil
	}
	result.Branch = item.Branch
	checkpoint, err := loadReviewCheckpoint(root, item.Branch)
	if err != nil {
		return result, err
	}
	checkpointChanged := checkpoint.Count == ^uint64(0) || checkpoint.Count+1 != number
	if checkpoint.Count == 0 {
		// A zero-count checkpoint still records the fixed reviewed head. It is
		// not a previous completed review, so startup intentionally leaves
		// PreviousReviewedHead empty and the first inspection is full.
		checkpointChanged = checkpointChanged || previous != "" || checkpoint.Head != "" && checkpoint.Head != reviewed
	} else {
		checkpointChanged = checkpointChanged || checkpoint.Head != previous
	}
	if checkpointChanged {
		result.Reason = "completed-review checkpoint changed from this invocation's round or previous reference; preserve the original Result Documents and stop"
		return result, nil
	}
	local, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
	if err != nil || local != reviewed {
		result.Reason = "local branch does not name the fixed reviewed head; fetch or repair the selected branch and retry this inspection"
		return result, nil
	}
	ledger, err := InspectLedger(root, reviewed, item.Branch, endpoints, RequireRetiredArtifacts)
	if err != nil {
		return result, err
	}
	result.Ledger = ledger
	if len(ledger.Violations) != 0 {
		result.Reason = fmt.Sprintf("artifact endpoint inspection found %d violation(s); repair the exact evidence and retry this inspection", len(ledger.Violations))
		return result, nil
	}
	comparisonBase, err := git(root, "merge-base", "main", reviewed)
	if err != nil {
		result.Reason = "main comparison base is unavailable; fetch main and retry this inspection"
		return result, nil
	}
	result.Comparison = comparisonBase + "..." + reviewed
	if number > 1 && previous != "" {
		if gitOK(root, "cat-file", "-e", previous+"^{commit}") == nil && gitOK(root, "merge-base", "--is-ancestor", previous, reviewed) == nil {
			result.Comparison = previous + "..." + reviewed
		} else {
			result.FallbackReason = "previous completed review revision is unavailable or is not an ancestor of the fixed head; use the full PR comparison while retaining finding history"
		}
	}
	result.Status = "inspected"
	result.Reason = ""
	return result, nil
}
