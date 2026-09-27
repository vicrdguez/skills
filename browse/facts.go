// Package browse is the interactive ledger browser behind `skl browse`. It
// presents the committed facts answered by one ledger.Snapshot as workflow
// entities, and it formats those facts for the read-only query commands too.
package browse

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vicrdguez/skills/ledger"
)

// lifecycleOrder lists the canonical lifecycles in workflow order with their
// labels.
var lifecycleOrder = []struct{ state, label string }{
	{ledger.ReadyForImplementation, "Ready for Implementation"},
	{ledger.AwaitingReview, "Awaiting Review"},
	{ledger.Rework, "Rework"},
	{ledger.NeedsHuman, "Needs Human"},
	{ledger.ReadyForMerge, "Ready for Merge"},
	{ledger.Merged, "Merged"},
	{ledger.Superseded, "Superseded"},
}

// lifecycleLabel names one recorded lifecycle.
func lifecycleLabel(state string) string {
	for _, lifecycle := range lifecycleOrder {
		if lifecycle.state == state {
			return lifecycle.label
		}
	}
	return "unsupported lifecycle " + strconv.Quote(state)
}

// tallyText summarizes a scope's Slices by lifecycle, Claims, and unknowns.
func tallyText(tally ledger.Tally) string {
	parts := []string{plural(tally.Slices, "slice")}
	var lifecycles []string
	for _, lifecycle := range lifecycleOrder {
		if count := tally.Lifecycles[lifecycle.state]; count > 0 {
			lifecycles = append(lifecycles, fmt.Sprintf("%d %s", count, lifecycle.label))
		}
	}
	if len(lifecycles) > 0 {
		parts[0] += " (" + strings.Join(lifecycles, ", ") + ")"
	}
	if tally.Claimed > 0 {
		parts = append(parts, fmt.Sprintf("%d claimed", tally.Claimed))
	}
	if tally.Unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d unknown", tally.Unknown))
	}
	return strings.Join(parts, " · ")
}

// deliveryText states a Proposal's delivery without equating archival,
// retirement, or supersession with full delivery.
func deliveryText(proposal ledger.ProposalSummary) string {
	merged := proposal.Lifecycles[ledger.Merged]
	switch {
	case proposal.FullyDelivered:
		return fmt.Sprintf("fully delivered (%d of %d Merged)", merged, proposal.Slices)
	case proposal.Unknown > 0:
		return fmt.Sprintf("unknown: %d of %d Merged, %d with unknown lifecycle", merged, proposal.Slices, proposal.Unknown)
	default:
		return fmt.Sprintf("not fully delivered (%d of %d Merged)", merged, proposal.Slices)
	}
}

// ProjectLines are the labeled facts of one Project summary.
func ProjectLines(project ledger.ProjectSummary) []string {
	lines := []string{
		"Repository: " + orUnknown(project.Repository),
		fmt.Sprintf("Proposals: %d active, %d archived", project.Proposals, project.ArchivedProposals),
		"Slices: " + tallyText(project.Tally),
	}
	return append(lines, diagnosticLines(project.Diagnostics)...)
}

// ProposalLines are the labeled facts of one Proposal summary.
func ProposalLines(proposal ledger.ProposalSummary) []string {
	var lines []string
	if proposal.ParentTitle != "" {
		lines = append(lines, "Title: "+proposal.ParentTitle)
	}
	lines = append(lines,
		"Accepted: "+orUnknown(proposal.Accepted),
		"Location: "+locationText(proposal.Archived, proposal.Retired),
		"Delivery: "+deliveryText(proposal),
		"Slices: "+tallyText(proposal.Tally),
		"Parent issue: "+attachmentText(proposal.ParentIssue),
	)
	return append(lines, diagnosticLines(proposal.Diagnostics)...)
}

// SliceSummaryLines are the labeled facts of one Proposal member.
func SliceSummaryLines(slice ledger.SliceSummary) []string {
	lines := []string{"Slice: " + slice.Item}
	if slice.Readable {
		lines = append(lines, "Title: "+slice.Title, "Lifecycle: "+lifecycleLabel(slice.Lifecycle), "Claim: "+claimText(slice.ClaimPhase, ""))
	} else {
		lines = append(lines, "Lifecycle: unknown", "Claim: unknown")
	}
	return append(lines, diagnosticLines(slice.Diagnostics)...)
}

// SliceLines are the labeled facts of one Slice detail.
func SliceLines(slice *ledger.SliceDetail) []string {
	lines, _ := sliceLines(slice)
	return lines
}

// relation is one followable relationship line of a Slice detail: its line
// index and the recorded Slice it opens.
type relation struct {
	line int
	item string
}

// sliceLines are SliceLines with the followable relationship lines in order.
func sliceLines(slice *ledger.SliceDetail) ([]string, []relation) {
	var relations []relation
	lines := []string{"Slice: " + slice.Item}
	if slice.Readable {
		lines = append(lines, "Title: "+slice.Title)
	}
	lines = append(lines, "Project: "+slice.Project+" ("+orUnknown(slice.Repository)+")", "Location: "+locationText(slice.Archived, slice.ProposalRetired)+" proposal")
	if !slice.Readable {
		lines = append(lines, "Lifecycle: unknown", "Claim: unknown", "Dependencies: unknown")
	} else {
		lines = append(lines, "Lifecycle: "+lifecycleLabel(slice.Lifecycle))
		if slice.Claim == nil {
			lines = append(lines, "Claim: none")
		} else {
			lines = append(lines, "Claim: "+claimText(slice.Claim.Phase, slice.Claim.Basis))
		}
		lines = append(lines, "Branch: "+orUnknown(slice.Branch))
		if len(slice.Dependencies) == 0 {
			lines = append(lines, "Dependencies: none")
		}
		for _, dependency := range slice.Dependencies {
			satisfaction := "unsatisfied until Merged"
			if dependency.Satisfied {
				satisfaction = "satisfied"
			}
			if dependency.Recorded {
				relations = append(relations, relation{len(lines), dependency.Item})
			}
			lines = append(lines, "Depends on: "+relationText(dependency.RelatedSlice, satisfaction))
		}
	}
	for _, blocked := range slice.Blocks.Slices {
		relations = append(relations, relation{len(lines), blocked.Item})
		lines = append(lines, "Blocks: "+relationText(blocked, ""))
	}
	switch {
	case slice.Blocks.Incomplete:
		lines = append(lines, "Blocks incomplete: "+incompleteText(slice.Blocks.Diagnostics)+" may also depend on this Slice")
		for _, diagnostic := range slice.Blocks.Diagnostics {
			lines = append(lines, DiagnosticText(diagnostic))
		}
	case len(slice.Blocks.Slices) == 0:
		lines = append(lines, "Blocks: none")
	}
	if slice.Readable {
		lines = append(lines, "Issue: "+attachmentText(slice.Issue), "Pull request: "+attachmentText(slice.Submission))
		if slice.Target != nil {
			lines = append(lines, "Integration target: "+slice.Target.Repository+" "+slice.Target.Branch)
		}
		if completion := slice.Completion; completion != nil {
			text := "Completion: " + attachmentText(&completion.Submission) + " into " + completion.Target.Branch
			if completion.MergeCommit != "" {
				text += ", merge commit " + completion.MergeCommit
			}
			if completion.SourceHead != "" {
				text += ", source head " + completion.SourceHead
			}
			lines = append(lines, text)
		}
		if slice.ActiveDecision {
			lines = append(lines, "Human decision: active")
		}
		if slice.Pending != nil && slice.Pending.Push != nil {
			lines = append(lines, "Pending push: "+slice.Pending.Push.Status+" "+slice.Pending.Push.Detail)
		}
	}
	lines = append(lines, "Parent issue: "+attachmentText(slice.ParentIssue), "Contract documents: "+listText(slice.Documents), "Current reports: "+listText(slice.Reports))
	return append(lines, diagnosticLines(slice.Diagnostics)...), relations
}

// relationText names a related Slice with its recorded state. Qualifier
// follows a known lifecycle; an unknown one is never read as satisfied.
func relationText(slice ledger.RelatedSlice, qualifier string) string {
	text := slice.Item
	if slice.Archived {
		text += " [archived]"
	}
	if slice.Title != "" {
		text += " — " + slice.Title
	}
	var state string
	switch {
	case !slice.Recorded:
		state = "unresolved: " + slice.Problem
	case slice.Problem != "":
		state = "lifecycle unknown: " + slice.Problem
	default:
		state = lifecycleLabel(slice.Lifecycle)
	}
	if qualifier != "" {
		state += "; " + qualifier
	}
	return text + " (" + state + ")"
}

// issueURL and pullRequestURL construct GitHub links from a recorded
// attachment identity. They return false when the identity cannot name one.
func issueURL(attachment *ledger.ForgeAttachment) (string, bool) {
	return attachmentURL(attachment, "issues")
}

func pullRequestURL(attachment *ledger.ForgeAttachment) (string, bool) {
	return attachmentURL(attachment, "pull")
}

func attachmentURL(attachment *ledger.ForgeAttachment, kind string) (string, bool) {
	if attachment == nil || attachment.Number <= 0 {
		return "", false
	}
	owner, name, found := strings.Cut(attachment.Repository, "/")
	if !found || owner == "" || name == "" || strings.ContainsAny(name, "/ ") || strings.ContainsAny(owner, " ") {
		return "", false
	}
	return fmt.Sprintf("https://github.com/%s/%s/%s/%d", owner, name, kind, attachment.Number), true
}

func claimText(phase, basis string) string {
	if phase == "" {
		return "none"
	}
	text := phase + " reservation"
	if basis != "" {
		text += " at ledger basis " + basis
	}
	return text + "; a reservation, not a running worker"
}

func attachmentText(attachment *ledger.ForgeAttachment) string {
	if attachment == nil {
		return "none recorded"
	}
	return fmt.Sprintf("%s#%d", attachment.Repository, attachment.Number)
}

func diagnosticLines(diagnostics []ledger.Diagnostic) []string {
	if len(diagnostics) == 0 {
		return nil
	}
	lines := []string{"Incomplete: " + incompleteText(diagnostics) + "; the facts above omit them"}
	for _, diagnostic := range diagnostics {
		lines = append(lines, DiagnosticText(diagnostic))
	}
	return lines
}

// DiagnosticText is the marked, labeled line of one diagnostic.
func DiagnosticText(diagnostic ledger.Diagnostic) string {
	return "! " + diagnostic.Scope + " " + diagnostic.Subject + ": " + diagnostic.Problem
}

func incompleteText(diagnostics []ledger.Diagnostic) string {
	return plural(len(diagnostics), "unreadable or unsupported record")
}

// locationText names where a Proposal is recorded and whether it was
// retired; neither states delivery.
func locationText(archived, retired bool) string {
	location := "active"
	if archived {
		location = "archived"
	}
	if retired {
		location += ", retired"
	}
	return location
}

func listText(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
