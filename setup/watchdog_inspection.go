package setup

import (
	"fmt"
	"strings"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

type WatchdogInspectionOutput struct {
	workflow.ReviewInspection
	Item         *implementationItemOutput `json:"item,omitempty"`
	Instructions string                    `json:"instructions"`
}

func PresentWatchdogInspection(root string, result workflow.ReviewInspection) WatchdogInspectionOutput {
	output := WatchdogInspectionOutput{ReviewInspection: result}
	if number, err := githubIssueNumber(result.Item); err == nil {
		output.Item = &implementationItemOutput{Number: number, Branch: result.Branch}
	}
	var body strings.Builder
	if result.Status != "inspected" {
		fmt.Fprintf(&body, "Status: %s\n%s\n\nFixed invocation: Work Item #%s, Submission #%s, remote %s, reviewed head %s, review number %d, Result Documents %s.\n", result.Status, result.Reason, result.Item, result.Submission, skilldist.ShellQuote(result.Remote), skilldist.ShellQuote(result.ReviewedHead), result.ReviewNumber, skilldist.ShellQuote(result.ResultDirectory))
		fmt.Fprintf(&body, "After repairing the reported precondition, retry this same inspection: `%s`. Stop if the fixed identity cannot be restored.\n", watchdogInspectionCommand(root, result))
		for _, violation := range result.Ledger.Violations {
			fmt.Fprintf(&body, "- %s\n", violation)
		}
		output.Instructions = body.String()
		return output
	}
	q := skilldist.ShellQuote
	fmt.Fprintf(&body, "# Watchdog Inspection Continuation\n\nWork Item #%s, Submission #%s, remote %s, review number %d. The original reviewed head remains `%s`. This is a read-only continuation of the same Claim; it neither selects work nor publishes a verdict. Preserve Result Documents in %s.\n\n", result.Item, result.Submission, q(result.Remote), result.ReviewNumber, result.ReviewedHead, q(result.ResultDirectory))
	if result.SuppliedBaseline != "" || result.SuppliedCompletion != "" {
		fmt.Fprintf(&body, "The fixed inspection used the supplied artifact endpoint overrides in `%s`.\n\n", watchdogInspectionCommand(root, result))
	}
	fmt.Fprintf(&body, "Artifact Baseline: `%s`\nArtifact Completion: `%s`\nLedger phase: %s. No endpoint violations were found.\n\n", result.Ledger.Baseline, result.Ledger.Completion, result.Ledger.Phase)
	fmt.Fprintf(&body, "Read the complete historical contract at these exact endpoints:\n\n")
	for _, file := range result.Ledger.Files {
		fmt.Fprintf(&body, "- `git -C %s show %s`\n", q(root), q(result.Ledger.Baseline+":"+file))
		fmt.Fprintf(&body, "- `git -C %s show %s`\n", q(root), q(result.Ledger.Completion+":"+file))
	}
	fmt.Fprintf(&body, "\nCompare with `git -C %s diff %s`. ", q(root), q(result.Comparison))
	if result.FallbackReason != "" {
		fmt.Fprintf(&body, "This is the full PR comparison: %s\n", result.FallbackReason)
	} else if result.PreviousHead != "" {
		fmt.Fprintf(&body, "This is the incremental comparison, including a valid empty range when both heads match. Preserve prior finding identities and human dispositions.\n")
	} else {
		fmt.Fprintf(&body, "This is the first full PR comparison. Preserve any supplied prior finding identities even if the Review Checkpoint was lost.\n")
	}
	fmt.Fprintf(&body, "\nContinue the fixed-head Watchdog procedure. Read the Submission's integrated target SHA as Verification evidence, confirm it is reachable from the reviewed head, and review merge and conflict-resolution effects. Do not treat unrelated target additions as scope creep or fetch a newer target merely because it moved after the recorded cutoff.\n")
	output.Instructions = body.String()
	return output
}

func watchdogInspectionCommand(root string, result workflow.ReviewInspection) string {
	q := skilldist.ShellQuote
	command := fmt.Sprintf("skl watchdog inspect --repo %s --remote %s --item %s --submission %s --base %s --submission-body-sha256 %s --review-number %d --reviewed-head %s --result-directory %s", q(root), q(result.Remote), result.Item, result.Submission, q(result.SubmissionBase), q(result.BodySHA256), result.ReviewNumber, q(result.ReviewedHead), q(result.ResultDirectory))
	if result.PreviousHead != "" {
		command += " --previous-reviewed-head " + q(result.PreviousHead)
	}
	if result.SuppliedBaseline != "" {
		command += " --artifact-baseline " + q(result.SuppliedBaseline)
	}
	if result.SuppliedCompletion != "" {
		command += " --artifact-completion " + q(result.SuppliedCompletion)
	}
	return command
}
