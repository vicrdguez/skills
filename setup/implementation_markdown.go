package setup

import (
	"strings"
)

// ImplementationMarkdown renders one Implement outcome as its default Markdown
// transport. An instruction outcome is the complete Execution Skill itself; the
// other outcomes are short reports that name the established status and the
// recovery available to the user. No representation authorizes success.
func ImplementationMarkdown(output ImplementationOutput) string {
	if packet := output.Packet; packet != nil {
		return strings.TrimRight(packet.Instructions, "\n") + "\n"
	}
	var report strings.Builder
	switch output.Status {
	case "no_work":
		report.WriteString("Status: no_work\n" +
			"The immediate queue observation found no eligible Work Item. This is not global completion: work may still appear later, so retry when it does.\n")
	case "idle_timeout":
		report.WriteString("Status: idle_timeout\n" +
			"The local bounded wait ended without claimable work. This is not global completion, so retry when work appears.\n")
	case "fix_required":
		report.WriteString("Status: fix_required\nThe operation was refused and no Claim was taken for this invocation.\n")
		if output.Reason != "" {
			report.WriteString("\n" + output.Reason + "\n")
		}
	case "awaiting_review":
		report.WriteString("Status: awaiting_review\nThe Submission was verified and published; an independent Watchdog review is the next step.\n")
		if output.Reason != "" {
			report.WriteString("\n" + output.Reason + "\n")
		}
	case "needs_human":
		report.WriteString("Status: needs_human\nA human decision is required; the Work Item is paused and not approved, merged, or requeued.\n")
		if output.Reason != "" {
			report.WriteString("\n" + output.Reason + "\n")
		}
	default:
		report.WriteString("Status: " + output.Status + "\n")
		if output.Reason != "" {
			report.WriteString("\n" + output.Reason + "\n")
		}
	}
	return report.String()
}
