package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ReviewInspection is a read-only continuation of one fixed Watchdog invocation.
// Startup cannot establish these facts without the selected project's objects.
type ReviewInspection struct {
	Status          string        `json:"status"`
	Reason          string        `json:"reason,omitempty"`
	Item            WorkItemID    `json:"item"`
	Submission      SubmissionID  `json:"submission"`
	Remote          string        `json:"remote"`
	Branch          string        `json:"branch"`
	ReviewedHead    string        `json:"reviewed_head"`
	ReviewNumber    uint64        `json:"review_number"`
	PreviousHead    string        `json:"previous_reviewed_head,omitempty"`
	ResultDirectory string        `json:"result_directory"`
	Ledger          LedgerHistory `json:"ledger"`
	Comparison      string        `json:"comparison,omitempty"`
	FallbackReason  string        `json:"fallback_reason,omitempty"`
}

func InspectWatchdog(ctx context.Context, root, remote string, id WorkItemID, submission SubmissionID, reviewed string, number uint64, previous, resultDirectory string, endpoints ArtifactEndpoints, backend ImplementationBackend) (ReviewInspection, error) {
	result := ReviewInspection{Status: "fix_required", Item: id, Submission: submission, Remote: remote, ReviewedHead: reviewed, ReviewNumber: number, PreviousHead: previous, ResultDirectory: resultDirectory}
	if id == "" || submission == "" || reviewed == "" || number == 0 || resultDirectory == "" {
		result.Reason = "inspect requires the fixed --item, --submission, --reviewed-head, --review-number, and --result-directory from this invocation"
		return result, nil
	}
	marker, err := os.ReadFile(filepath.Join(resultDirectory, ".skl-result"))
	if err != nil || string(marker) != "skl.watchdog/v1\n" {
		result.Reason = "private Watchdog Result Document directory is unavailable or does not belong to this invocation; preserve the original path and inspect it"
		return result, nil
	}
	items, err := loadImplementation(ctx, backend)
	if err != nil {
		return result, err
	}
	var item *ImplementationItem
	for i := range items {
		if items[i].ID == id {
			if item != nil {
				result.Reason = "ambiguous Work Item observation; repair backend projections and retry the same inspection"
				return result, nil
			}
			item = &items[i]
		}
	}
	if item == nil || item.Problem != "" || item.State != AwaitingReview || !item.Claimed || item.Submission == nil || item.Submission.ID != submission || item.Submission.Head != reviewed {
		result.Reason = "selected Work Item, Submission attachment, Claim, or original reviewed head changed; inspect the fixed invocation and stop until repaired"
		return result, nil
	}
	result.Branch = item.Branch
	checkpoint, err := loadReviewCheckpoint(root, item.Branch)
	if err != nil {
		return result, err
	}
	if checkpoint.Count == ^uint64(0) || checkpoint.Count+1 != number || checkpoint.Head != previous {
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
	base, err := git(root, "merge-base", "main", reviewed)
	if err != nil {
		result.Reason = "main comparison base is unavailable; fetch main and retry this inspection"
		return result, nil
	}
	result.Comparison = base + "..." + reviewed
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
