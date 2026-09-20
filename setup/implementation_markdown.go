package setup

import (
	"fmt"
	"strings"
)

func implementationGuidance(output ImplementationOutput) *ImplementationGuidance {
	identity := "The invocation"
	claimed := false
	var submission *submissionOutput
	if output.Item != nil {
		identity = fmt.Sprintf("Work Item #%d", output.Item.Number)
		claimed, submission = output.Item.Claimed, output.Item.Submission
	}
	switch output.Status {
	case "no_work":
		return &ImplementationGuidance{Claim: "not_acquired", Explanation: "The immediate queue observation found no eligible work; nothing was selected or claimed.", NextStep: "Retry `skl implement next` later when work may have appeared; this is not global completion."}
	case "idle_timeout":
		return &ImplementationGuidance{Claim: "not_acquired", Explanation: "The local bounded wait ended without claimable work; nothing was selected or claimed.", NextStep: "Retry `skl implement next` when work may have appeared; this is not global completion."}
	case "fix_required":
		guidance := &ImplementationGuidance{Explanation: identity + " was refused, and this outcome made no successful transition."}
		switch {
		case output.ClaimAcquisitionUncertain:
			guidance.Claim = "unknown"
			guidance.Explanation += " The contradictory read-back does not establish whether Claim acquisition succeeded."
			guidance.Recovery = fmt.Sprintf("Inspect Work Item #%d and explicitly run `skl implement resume --item %d` for that same identity; do not run `skl implement next` or assume the Claim was released.", output.Item.Number, output.Item.Number)
		case output.Item == nil:
			guidance.Claim = "unknown"
			guidance.Recovery = "Inspect the Work Item before retrying; do not run `skl implement next` blindly or assume any reservation was released."
		case claimed:
			guidance.Claim = "retained"
			guidance.Recovery = "Inspect the Work Item and explicitly resume the same identity rather than selecting replacement work."
		default:
			guidance.Claim = "not_acquired"
			guidance.Recovery = "Repair the reported refusal and retry the same operation; this outcome released no Claim."
		}
		return guidance
	case "awaiting_review":
		claim := "released"
		if claimed {
			claim = "retained"
		}
		explanation := identity + " was published and verified after the handoff check."
		if submission != nil {
			explanation += fmt.Sprintf(" Submission #%d holds the reviewed head.", submission.Number)
		}
		return &ImplementationGuidance{Claim: claim, Explanation: explanation, NextStep: "The next step is independent Watchdog review; do not resubmit, roll back, approve, or merge from this outcome."}
	case "needs_human":
		claim := "released"
		if claimed {
			claim = "retained"
		}
		preserved := "Preserved work: none. The decision was published without inventing a Submission."
		if submission != nil {
			preserved = fmt.Sprintf("Preserved work: Submission #%d stays attached to this Work Item.%s", submission.Number, draftNote(submission.Draft))
		}
		return &ImplementationGuidance{Claim: claim, Explanation: preserved, NextStep: "A human decision is required; this does not approve, merge, automatically requeue, complete, or retire the change."}
	default:
		return nil
	}
}

// ImplementationMarkdown renders one Implement outcome as its default Markdown
// transport. An instruction outcome is the complete Execution Skill itself; the
// other outcomes are short reports that name the established status, the
// observed Claim certainty, and the recovery available to the user. No
// representation authorizes success or a Claim release.
func ImplementationMarkdown(output ImplementationOutput) string {
	if packet := output.Packet; packet != nil {
		return strings.TrimRight(packet.Instructions, "\n") + "\n"
	}
	var report strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&report, format+"\n", args...) }
	identity := "The invocation"
	claimed := false
	var submission *submissionOutput
	if output.Item != nil {
		identity = fmt.Sprintf("Work Item #%d", output.Item.Number)
		claimed, submission = output.Item.Claimed, output.Item.Submission
	}
	line("Status: %s", output.Status)
	if output.Reason != "" {
		line("%s", output.Reason)
	}
	switch output.Status {
	case "no_work":
		line("The immediate queue observation found no eligible work; nothing was selected or claimed. This is not global completion: work may still appear, so run `skl implement next` again later.")
	case "idle_timeout":
		line("The local bounded wait ended without claimable work; nothing was selected or claimed. This is not global completion, so retry `skl implement next` when work may have appeared.")
	case "fix_required":
		line("%s was refused, and this outcome made no successful transition.", identity)
		switch {
		case output.ClaimAcquisitionUncertain:
			line("The contradictory read-back does not establish whether Claim acquisition succeeded. Inspect Work Item #%d and explicitly run `skl implement resume --item %d` for that same identity; do not run `skl implement next` or assume the Claim was released.", output.Item.Number, output.Item.Number)
		case output.Item == nil:
			line("This outcome does not establish whether the Work Item holds a Claim. Inspect it before retrying; do not run `skl implement next` blindly and do not assume any reservation was released.")
		case claimed:
			line("An existing Claim is retained; nothing in this outcome released it. Inspect the Work Item and resume the same identity explicitly rather than selecting replacement work.")
		default:
			line("No Claim is recorded for this Work Item, so this refusal released nothing.")
		}
	case "awaiting_review":
		line("%s was published and verified after the handoff check.", identity)
		if submission != nil {
			line("Submission #%d holds the reviewed head.", submission.Number)
		}
		if claimed {
			line("The Claim is still recorded; inspect it before assuming the handoff finished.")
		} else {
			line("The Claim was released by the verified handoff.")
		}
		line("The next step is independent Watchdog review; this outcome neither approves nor merges the change, and it is not a reason to resubmit or roll back published work.")
	case "needs_human":
		line("%s is paused until a human decides.", identity)
		if submission != nil {
			line("Preserved work: Submission #%d stays attached to this Work Item.%s", submission.Number, draftNote(submission.Draft))
		} else {
			line("Preserved work: none. The decision was published without inventing a Submission.")
		}
		if claimed {
			line("The Claim is still recorded; inspect it before assuming the pause finished.")
		} else {
			line("The verified pause released the Claim.")
		}
		line("A human decision is required. This does not approve, merge, or automatically requeue the change, and incomplete artifacts were not completed or retired to allow it.")
	default:
		line("This outcome implies no workflow transition.")
	}
	return report.String()
}

func draftNote(draft bool) string {
	if draft {
		return " It is a draft, and further progress updates this same draft rather than creating another."
	}
	return ""
}
