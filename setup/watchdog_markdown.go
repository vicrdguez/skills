package setup

import "fmt"

// WatchdogOutcomeMarkdown renders an already-verified engine result. It never
// repeats the operation or treats Result Document prose as a verdict.
func WatchdogOutcomeMarkdown(output ImplementationOutput) string {
	message := ""
	switch output.Status {
	case "ready_for_merge":
		message = "Review publication and Claim release were verified. The change awaits human integration and merge; the source Work Item remains open until merge."
	case "rework":
		message = "Review findings were published and the Claim was released. The Work Item is ready for finding-driven implementation rework."
	case "needs_human":
		message = "Review publication and Claim release were verified. A human must decide the next Workflow transition."
	case "no_work":
		message = "No eligible review Work Item was found. Stop this one-item invocation; another item may appear later."
	case "idle_timeout":
		message = "The bounded wait ended without claimable review work. Stop this one-item invocation and retry later if work may have appeared."
	case "fix_required":
		message = "The review did not complete. Repair the reported precondition before continuing this invocation. Preserve any selected Claim and Result Documents while their state is unresolved."
	default:
		message = "Inspect this outcome before continuing the selected Work Item."
	}
	if output.Reason != "" {
		return fmt.Sprintf("Status: %s\n\n%s\n\n%s\n", output.Status, message, output.Reason)
	}
	return fmt.Sprintf("Status: %s\n\n%s\n", output.Status, message)
}
