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

// lifecycleLabels name the canonical lifecycles.
var lifecycleLabels = ledger.LifecycleLabels

// claimLabels name the selectable Claim values.
var claimLabels = map[string]string{
	ledger.ImplementPhase: "implement claim",
	ledger.WatchdogPhase:  "watchdog claim",
	ledger.ClaimNone:      "unclaimed",
}

// lifecycleLabel names one recorded lifecycle.
func lifecycleLabel(state string) string {
	if label, ok := lifecycleLabels[state]; ok {
		return label
	}
	return "unsupported lifecycle " + strconv.Quote(state)
}

// tallyText summarizes a scope's Slices by lifecycle, Claims, and unknowns.
func tallyText(tally ledger.Tally) string {
	parts := []string{plural(tally.Slices, "slice")}
	var lifecycles []string
	for _, lifecycle := range ledger.Lifecycles {
		if count := tally.Lifecycles[lifecycle]; count > 0 {
			lifecycles = append(lifecycles, fmt.Sprintf("%d %s", count, lifecycleLabels[lifecycle]))
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
		"Proposal Branch: "+proposalBranchText(proposal),
		"Integration target: "+proposalTargetText(proposal.Target),
		"Proposal Submission: "+proposalSubmissionText(proposal),
		"Location: "+locationText(proposal.Archived, proposal.Retired),
		"Delivery: "+deliveryText(proposal),
		"Slices: "+tallyText(proposal.Tally),
		"Parent issue: "+attachmentText(proposal.ParentIssue),
	)
	return append(lines, diagnosticLines(proposal.Diagnostics)...)
}

func proposalBranchText(proposal ledger.ProposalSummary) string {
	if proposal.Branch == "" || proposal.BranchState == "" {
		return orNone(proposal.Branch)
	}
	return proposal.Branch + " (" + proposal.BranchState + ")"
}

// proposalSubmissionText states the recorded Proposal Submission with the
// presentation the engine gives it: merged once completion is recorded, ready
// once every Slice is Merged, a draft before.
func proposalSubmissionText(proposal ledger.ProposalSummary) string {
	if proposal.Submission == nil {
		return "none"
	}
	text := fmt.Sprintf("%s#%d", proposal.Submission.Repository, proposal.Submission.Number)
	switch {
	case proposal.Completion != nil:
		return text + " (merged into " + proposal.Completion.Target.Branch + " at " + proposal.Completion.MergeCommit + ")"
	case proposal.Slices > 0 && proposal.Lifecycles[ledger.Merged] == proposal.Slices:
		return text + " (ready for review)"
	}
	return text + " (draft)"
}

// SliceSummaryLines are the labeled facts of one Proposal member.
func proposalTargetText(target *ledger.IntegrationTarget) string {
	if target == nil {
		return "none"
	}
	return target.Repository + " " + target.Branch
}

func SliceSummaryLines(slice ledger.SliceSummary) []string {
	lines := []string{"Slice: " + slice.Item}
	if slice.Readable {
		lines = append(lines, "Title: "+slice.Title, "Lifecycle: "+lifecycleLabel(slice.Lifecycle), "Claim: "+claimText(slice.ClaimPhase, ""))
		if slice.MergedInto != "" {
			lines = append(lines, "Merged into: "+slice.MergedInto)
		}
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
	lines = append(lines, "Project: "+slice.Project+" ("+orUnknown(slice.Repository)+")", "Location: "+locationText(slice.Archived, slice.ProposalRetired)+" proposal")
	if !slice.Readable {
		lines = append(lines, "Lifecycle: unknown", "Claim: unknown", "Dependencies: unknown")
	} else {
		lines = append(lines, "Lifecycle: "+lifecycleLabel(slice.Lifecycle))
		if slice.Claim == nil {
			lines = append(lines, "Claim: none")
		} else {
			lines = append(lines,
				"Claim: "+claimText(slice.Claim.Phase, slice.Claim.Basis),
				"Claim state reference: "+slice.Claim.Reference.Commit+":"+slice.Claim.Reference.Path,
			)
		}
		lines = append(lines, "Branch: "+orUnknown(slice.Branch))
		if len(slice.Dependencies) == 0 {
			lines = append(lines, "Dependencies: none")
		}
		for _, dependency := range slice.Dependencies {
			satisfaction := "unsatisfied until Merged into main, this Proposal Branch, or a Proposal Branch completed into main"
			if dependency.Satisfied {
				satisfaction = "satisfied"
			}
			lines = append(lines, "Depends on: "+relationText(dependency.RelatedSlice, satisfaction))
		}
	}
	for _, blocked := range slice.Blocks.Slices {
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
	lines = append(lines, "Parent issue: "+attachmentText(slice.ParentIssue), "Contract documents: "+listText(slice.Documents), "Current reports: "+listText(slice.Reports), "Press d to read Slice documents; r follows the exact Claim state reference.")
	return append(lines, diagnosticLines(slice.Diagnostics)...)
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

// SliceRow is the compact lifecycle and Claim of one labeled Slice.
func SliceRow(label string, slice ledger.SliceSummary) string {
	if !slice.Readable {
		return marked(true, label+" — lifecycle unknown · claim unknown")
	}
	claim := "unclaimed"
	if slice.ClaimPhase != "" {
		claim = slice.ClaimPhase + " claim"
	}
	return marked(len(slice.Diagnostics) > 0, label+" — "+lifecycleLabel(slice.Lifecycle)+" · "+claim)
}

func marked(incomplete bool, text string) string {
	if incomplete {
		return "! " + text
	}
	return text
}

// MatchRow is one found Slice with its identity and title.
func MatchRow(match ledger.SliceMatch) string {
	label := match.Item
	if match.Archived {
		label += " [archived]"
	}
	row := SliceRow(label, match.SliceSummary)
	if match.Title != "" {
		row += " — " + match.Title
	}
	return row
}

// GroupTitle names one group of found Slices.
func GroupTitle(group ledger.SliceGroup) string {
	switch {
	case group.UnknownLifecycle:
		return "Lifecycle unknown"
	case group.Lifecycle != "":
		return lifecycleLabel(group.Lifecycle)
	case group.Archived:
		return "Proposal " + group.Proposal + " [archived]"
	}
	return "Proposal " + group.Proposal
}

// UndecidedTitle heads the Slices whose unknown facts leave a criterion
// undecided.
const UndecidedTitle = "Undecided: unknown facts may or may not match"

// SelectionText states every criterion and the scope of one query.
func SelectionText(query ledger.SliceQuery) string {
	lifecycles := []string{}
	for _, lifecycle := range query.Lifecycles {
		lifecycles = append(lifecycles, lifecycleLabel(lifecycle))
	}
	claims := []string{}
	for _, claim := range query.Claims {
		claims = append(claims, claimLabels[claim])
	}
	parts := []string{anyOf(lifecycles, "any lifecycle"), anyOf(claims, "any claim")}
	if query.Text != "" {
		parts = append(parts, "name contains "+strconv.Quote(query.Text))
	}
	scope := "every Project"
	if query.Project != "" {
		scope = "Project " + query.Project
	}
	archived := "archived hidden"
	if query.IncludeArchived {
		archived = "archived shown"
	}
	return strings.Join(parts, " · ") + " in " + scope + " (" + archived + ") · grouped by " + query.GroupBy
}

// ResultText states how many Slices matched and whether unknown facts keep
// the result incomplete, so an empty readable result reads differently from
// one that unreadable records may hide.
func ResultText(search *ledger.SliceSearch) string {
	text := plural(search.Matched, "matching slice")
	switch {
	case search.Matched == 0 && search.Incomplete:
		text = "No Slice is known to match"
	case search.Matched == 0:
		return "No Slice matches this selection."
	}
	if search.Undecided > 0 {
		text += fmt.Sprintf("; %d undecided by unknown facts", search.Undecided)
	}
	if search.Incomplete {
		text += "; incomplete: unreadable records may hide matches"
	}
	return text
}

func anyOf(values []string, none string) string {
	if len(values) == 0 {
		return none
	}
	return strings.Join(values, " or ")
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
	return phase + claimDetail(basis)
}

// claimDetail follows a Claim's phase: its basis and what a Claim is.
func claimDetail(basis string) string {
	text := " reservation"
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

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
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
