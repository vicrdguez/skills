package setup

import (
	"fmt"
	"strings"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

type WatchdogInspectionOutput struct {
	workflow.ReviewInspection
	Instructions string `json:"instructions"`
}

func PresentWatchdogInspection(root string, result workflow.ReviewInspection) WatchdogInspectionOutput {
	output := WatchdogInspectionOutput{ReviewInspection: result}
	var body strings.Builder
	if result.Status != "inspected" {
		fmt.Fprintf(&body, "Status: %s\n%s\n", result.Status, result.Reason)
		for _, violation := range result.Ledger.Violations {
			fmt.Fprintf(&body, "- %s\n", violation)
		}
		output.Instructions = body.String()
		return output
	}
	q := skilldist.ShellQuote
	fmt.Fprintf(&body, "# Watchdog Inspection Continuation\n\nWork Item #%s, Submission #%s, review number %d. The original reviewed head remains `%s`. This is a read-only continuation of the same Claim; it neither selects work nor publishes a verdict. Preserve Result Documents in %s.\n\n", result.Item, result.Submission, result.ReviewNumber, result.ReviewedHead, q(result.ResultDirectory))
	fmt.Fprintf(&body, "Artifact Baseline: `%s`\nArtifact Completion: `%s`\nLedger phase: %s. No endpoint violations were found.\n\n", result.Ledger.Baseline, result.Ledger.Completion, result.Ledger.Phase)
	fmt.Fprintf(&body, "Read the complete historical contract at these exact endpoints:\n\n")
	for _, file := range []string{"intent.md", "behavior.md", "plan.md", "tasks.md"} {
		fmt.Fprintf(&body, "- `git -C %s show %s`\n", q(root), q(result.Ledger.Baseline+":.changes/"+result.Branch+"/"+file))
		fmt.Fprintf(&body, "- `git -C %s show %s`\n", q(root), q(result.Ledger.Completion+":.changes/"+result.Branch+"/"+file))
	}
	fmt.Fprintf(&body, "\nCompare with `git -C %s diff %s`. ", q(root), q(result.Comparison))
	if result.FallbackReason != "" {
		fmt.Fprintf(&body, "This is the full PR comparison: %s\n", result.FallbackReason)
	} else if result.PreviousHead != "" {
		fmt.Fprintf(&body, "This is the incremental comparison, including a valid empty range when both heads match. Preserve prior finding identities and human dispositions.\n")
	} else {
		fmt.Fprintf(&body, "This is the first full PR comparison. Preserve any supplied prior finding identities even if the Review Checkpoint was lost.\n")
	}
	output.Instructions = body.String()
	return output
}
