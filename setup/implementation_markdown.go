package setup

import (
	"fmt"
	"strings"
)

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
