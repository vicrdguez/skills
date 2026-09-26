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

// LifecycleLabel names one recorded lifecycle.
func LifecycleLabel(state string) string {
	for _, lifecycle := range lifecycleOrder {
		if lifecycle.state == state {
			return lifecycle.label
		}
	}
	return "unsupported lifecycle " + strconv.Quote(state)
}

// TallyText summarizes a scope's Slices by lifecycle, Claims, and unknowns.
func TallyText(tally ledger.Tally) string {
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

// DeliveryText states a Proposal's delivery without equating archival,
// retirement, or supersession with full delivery.
func DeliveryText(proposal ledger.ProposalSummary) string {
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
		"Slices: " + TallyText(project.Tally),
	}
	return append(lines, diagnosticLines(project.Diagnostics)...)
}

// ProposalLines are the labeled facts of one Proposal summary.
func ProposalLines(proposal ledger.ProposalSummary) []string {
	var lines []string
	if proposal.ParentTitle != "" {
		lines = append(lines, "Title: "+proposal.ParentTitle)
	}
	location := "active"
	if proposal.Archived {
		location = "archived"
	}
	if proposal.Retired {
		location += ", retired"
	}
	lines = append(lines,
		"Accepted: "+orUnknown(proposal.Accepted),
		"Location: "+location,
		"Delivery: "+DeliveryText(proposal),
		"Slices: "+TallyText(proposal.Tally),
		"Parent issue: "+attachmentText(proposal.ParentIssue),
	)
	return append(lines, diagnosticLines(proposal.Diagnostics)...)
}

// SliceSummaryLines are the labeled facts of one Proposal member.
func SliceSummaryLines(slice ledger.SliceSummary) []string {
	lines := []string{"Slice: " + slice.Item}
	if slice.Readable {
		lines = append(lines, "Title: "+slice.Title, "Lifecycle: "+LifecycleLabel(slice.Lifecycle), "Claim: "+claimText(slice.ClaimPhase, ""))
	} else {
		lines = append(lines, "Lifecycle: unknown", "Claim: unknown")
	}
	return append(lines, diagnosticLines(slice.Diagnostics)...)
}

// SliceLines are the labeled facts of one Slice detail.
func SliceLines(slice *ledger.SliceDetail) []string {
	lines := []string{"Slice: " + slice.Item}
	if slice.Readable {
		lines = append(lines, "Title: "+slice.Title)
	}
	location := "active proposal"
	if slice.Archived {
		location = "archived proposal"
	}
	if slice.ProposalRetired {
		location += ", retired"
	}
	lines = append(lines, "Project: "+slice.Project+" ("+orUnknown(slice.Repository)+")", "Location: "+location)
	if !slice.Readable {
		lines = append(lines, "Lifecycle: unknown", "Claim: unknown")
	} else {
		lines = append(lines, "Lifecycle: "+LifecycleLabel(slice.Lifecycle))
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
			switch {
			case dependency.Problem != "":
				lines = append(lines, "Depends on: "+dependency.Item+" (lifecycle unknown: "+dependency.Problem+")")
			default:
				lines = append(lines, "Depends on: "+dependency.Item+" ("+LifecycleLabel(dependency.Lifecycle)+")")
			}
		}
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
	return append(lines, diagnosticLines(slice.Diagnostics)...)
}

// IssueURL and PullRequestURL construct GitHub links from a recorded
// attachment identity. They return false when the identity cannot name one.
func IssueURL(attachment *ledger.ForgeAttachment) (string, bool) {
	return attachmentURL(attachment, "issues")
}

func PullRequestURL(attachment *ledger.ForgeAttachment) (string, bool) {
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
	lines := []string{"Incomplete: " + plural(len(diagnostics), "unreadable or unsupported record") + "; the facts above omit them"}
	for _, diagnostic := range diagnostics {
		lines = append(lines, "! "+diagnostic.Scope+" "+diagnostic.Subject+": "+diagnostic.Problem)
	}
	return lines
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
