package browse

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vicrdguez/skills/ledger"
)

// relation is one followable relationship of a Slice: the recorded Slice it
// opens.
type relation struct {
	item     string
	archived bool
}

// sliceRelations lists a Slice's followable relationships in the order its
// facts show them: recorded Dependencies, then the Slices it blocks.
func sliceRelations(slice *ledger.SliceDetail) []relation {
	var relations []relation
	if slice.Readable {
		for _, dependency := range slice.Dependencies {
			if dependency.Recorded {
				relations = append(relations, relation{dependency.Item, dependency.Archived})
			}
		}
	}
	for _, blocked := range slice.Blocks.Slices {
		relations = append(relations, relation{blocked.Item, blocked.Archived})
	}
	return relations
}

// phaseReport is what a Slice's latest report of one phase records: its
// metadata, or why it has none.
type phaseReport struct {
	label   string
	report  *ledger.Report
	absent  bool
	problem string
}

// latestReports reads the latest implementation and watchdog reports from a
// Slice's document list. A report that exists but cannot be interpreted
// keeps its diagnostic and never an outcome.
func latestReports(list *ledger.DocumentList, err error) []phaseReport {
	reports := []phaseReport{{label: "Implementation"}, {label: "Watchdog"}}
	kinds := []ledger.DocumentKind{ledger.ImplementReportDocumentKind, ledger.WatchdogReportDocumentKind}
	for index, kind := range kinds {
		report := &reports[index]
		if err != nil {
			report.problem = "unknown: " + err.Error()
			continue
		}
		report.problem = "unknown: availability not recorded"
		for _, availability := range list.Availability {
			if availability.Slice != list.Slice || availability.Kind != kind {
				continue
			}
			switch availability.Status {
			case ledger.DocumentAbsent:
				report.absent, report.problem = true, ""
			case ledger.DocumentAvailable:
				report.problem = "unreadable metadata"
				for _, entry := range list.Documents {
					if entry.Kind != kind {
						continue
					}
					report.report, report.problem = entry.Report, problemText(entry.Diagnostics)
				}
			default:
				report.problem = "unreadable"
				if availability.Reference != nil {
					for _, diagnostic := range list.Diagnostics {
						if strings.Contains(diagnostic.Problem, availability.Reference.Path) {
							report.problem = diagnostic.Problem
						}
					}
				}
			}
		}
	}
	return reports
}

func problemText(diagnostics []ledger.Diagnostic) string {
	var problems []string
	for _, diagnostic := range diagnostics {
		problems = append(problems, diagnostic.Problem)
	}
	return strings.Join(problems, "; ")
}

// fact is one labeled line of a Slice's facts. Relation is the index of the
// followable relationship it shows, or -1.
type fact struct {
	label    string
	value    []span
	relation int
}

// factSection is one titled group of facts; the identity group has no title.
type factSection struct {
	title string
	facts []fact
}

func plainFact(label, value string) fact {
	return fact{label: label, value: []span{{text: value}}, relation: -1}
}

func styledFact(label string, value ...span) fact {
	return fact{label: label, value: value, relation: -1}
}

func warningFact(label, value string) fact {
	return styledFact(label, span{"! " + value, warningStyle})
}

// sliceSections groups a Slice's facts into identity, status,
// relationships, links and documents, then any diagnostics.
func sliceSections(slice *ledger.SliceDetail, reports []phaseReport) []factSection {
	identity := factSection{facts: []fact{plainFact("Slice", slice.Item)}}
	if slice.Readable && slice.Title != "" {
		identity.facts = append(identity.facts, plainFact("Title", slice.Title))
	}

	status := factSection{title: "Status"}
	if slice.Readable {
		status.facts = append(status.facts, styledFact("Lifecycle", lifecycleSpan(slice.Lifecycle)))
		if slice.Claim == nil {
			status.facts = append(status.facts, plainFact("Claim", "none"))
		} else {
			status.facts = append(status.facts,
				styledFact("Claim", claimSpan(slice.Claim.Phase), span{text: strings.TrimPrefix(claimText(slice.Claim.Phase, slice.Claim.Basis), slice.Claim.Phase)}),
				plainFact("Claim state", slice.Claim.Reference.Commit+":"+slice.Claim.Reference.Path))
		}
		status.facts = append(status.facts, plainFact("Branch", orUnknown(slice.Branch)))
	} else {
		status.facts = append(status.facts, styledFact("Lifecycle", unknownLifecycleSpan()), styledFact("Claim", unknownClaimSpan()))
	}
	status.facts = append(status.facts, plainFact("Location", locationText(slice.Archived, slice.ProposalRetired)+" proposal"))
	if slice.Readable {
		if slice.ActiveDecision {
			status.facts = append(status.facts, plainFact("Decision", "active Human Decision"))
		}
		if slice.Pending != nil && slice.Pending.Push != nil {
			status.facts = append(status.facts, plainFact("Pending push", slice.Pending.Push.Status+" "+slice.Pending.Push.Detail))
		}
		if completion := slice.Completion; completion != nil {
			text := attachmentText(&completion.Submission) + " into " + completion.Target.Branch
			if completion.MergeCommit != "" {
				text += ", merge commit " + completion.MergeCommit
			}
			if completion.SourceHead != "" {
				text += ", source head " + completion.SourceHead
			}
			status.facts = append(status.facts, plainFact("Completion", text))
		}
	}

	relationships := factSection{title: "Relationships"}
	followable := 0
	switch {
	case !slice.Readable:
		relationships.facts = append(relationships.facts, warningFact("Depends on", "unknown"))
	case len(slice.Dependencies) == 0:
		relationships.facts = append(relationships.facts, plainFact("Depends on", "none"))
	}
	if slice.Readable {
		for _, dependency := range slice.Dependencies {
			satisfaction := "unsatisfied until Merged"
			if dependency.Satisfied {
				satisfaction = "satisfied"
			}
			row := fact{label: "Depends on", value: relatedSpans(dependency.RelatedSlice, satisfaction), relation: -1}
			if dependency.Recorded {
				row.relation, followable = followable, followable+1
			}
			relationships.facts = append(relationships.facts, row)
		}
	}
	for _, blocked := range slice.Blocks.Slices {
		relationships.facts = append(relationships.facts, fact{label: "Blocks", value: relatedSpans(blocked, ""), relation: followable})
		followable++
	}
	switch {
	case slice.Blocks.Incomplete:
		relationships.facts = append(relationships.facts, warningFact("Blocks", "incomplete: "+incompleteText(slice.Blocks.Diagnostics)+" may also depend on this Slice"))
		for _, diagnostic := range slice.Blocks.Diagnostics {
			relationships.facts = append(relationships.facts, styledFact("", span{DiagnosticText(diagnostic), warningStyle}))
		}
	case len(slice.Blocks.Slices) == 0:
		relationships.facts = append(relationships.facts, plainFact("Blocks", "none"))
	}

	links := factSection{title: "Links", facts: []fact{plainFact("Project", slice.Project+" ("+orUnknown(slice.Repository)+")")}}
	if slice.Readable {
		links.facts = append(links.facts, plainFact("Issue", attachmentText(slice.Issue)), plainFact("Pull request", attachmentText(slice.Submission)))
	}
	links.facts = append(links.facts, plainFact("Parent issue", attachmentText(slice.ParentIssue)))
	if slice.Readable && slice.Target != nil {
		links.facts = append(links.facts, plainFact("Target", slice.Target.Repository+" "+slice.Target.Branch))
	}

	documents := factSection{title: "Documents", facts: []fact{plainFact("Contract", listText(slice.Documents))}}
	for _, report := range reports {
		documents.facts = append(documents.facts, reportFact(report))
	}

	sections := []factSection{identity, status, relationships, links, documents}
	if len(slice.Diagnostics) > 0 {
		diagnostics := factSection{title: "Diagnostics"}
		for _, line := range diagnosticLines(slice.Diagnostics) {
			style := lipgloss.NewStyle()
			if strings.HasPrefix(line, "! ") {
				style = warningStyle
			}
			diagnostics.facts = append(diagnostics.facts, styledFact("", span{line, style}))
		}
		sections = append(sections, diagnostics)
	}
	return sections
}

// relatedSpans name a related Slice and its recorded lifecycle in the
// status vocabulary. Qualifier follows the lifecycle; an unresolved or
// unreadable Slice is marked and never reads as satisfied.
func relatedSpans(slice ledger.RelatedSlice, qualifier string) []span {
	text := slice.Item
	if slice.Archived {
		text += " [archived]"
	}
	if slice.Title != "" {
		text += " — " + slice.Title
	}
	var state span
	switch {
	case !slice.Recorded:
		state = span{"! unresolved: " + slice.Problem, warningStyle}
	case slice.Problem != "":
		state = span{"! lifecycle unknown: " + slice.Problem, warningStyle}
	default:
		state = lifecycleSpan(slice.Lifecycle)
	}
	spans := []span{{text: text + "  "}, state}
	if qualifier != "" {
		spans = append(spans, span{text: " · " + qualifier})
	}
	return spans
}

// reportFact is one phase's latest report outcome as the report recorded
// it, with its round when recorded.
func reportFact(report phaseReport) fact {
	switch {
	case report.absent:
		return plainFact(report.label, "not yet available")
	case report.report == nil:
		return warningFact(report.label, strings.Join(strings.Fields(report.problem), " "))
	}
	value := []span{outcomeSpan(report.report.Outcome)}
	if report.report.Round != 0 {
		value = append(value, span{text: fmt.Sprintf(" · round %d", report.report.Round)})
	}
	if report.problem != "" {
		value = append(value, span{" · ! " + strings.Join(strings.Fields(report.problem), " "), warningStyle})
	}
	return styledFact(report.label, value...)
}

// factIndent is how far a fact's label sits inside its section; a selected
// relationship marks the indent with "> ".
const factIndent = 2

// factGap separates a fact's label column from its value.
const factGap = 2

// narrowValue is the narrowest value column kept beside the labels; a
// narrower pane puts each value beneath its label.
const narrowValue = 16

// renderSliceFacts lays the sections out within width, values aligned beside
// their section's labels and wrapped beneath themselves. Selected is the
// highlighted relationship, or -1. It returns the lines and, for each
// followable relationship, the line where it starts.
func renderSliceFacts(sections []factSection, width, selected int) ([]string, []int) {
	width = max(width, 1)
	var lines []string
	var starts []int
	for _, section := range sections {
		if len(section.facts) == 0 {
			continue
		}
		if section.title != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, truncate(titleStyle.Render(section.title), width))
		}
		labels := 0
		for _, fact := range section.facts {
			labels = max(labels, lipgloss.Width(fact.label))
		}
		for _, fact := range section.facts {
			chosen := fact.relation >= 0 && fact.relation == selected
			if fact.relation >= 0 {
				starts = append(starts, len(lines))
			}
			lines = append(lines, fact.render(labels, width, chosen)...)
		}
	}
	return lines, starts
}

// render draws one fact: its label padded to labels cells, then its value
// wrapped beneath itself, or beneath the label when the pane is too narrow.
func (f fact) render(labels, width int, selected bool) []string {
	style := func(base lipgloss.Style) lipgloss.Style {
		if selected {
			return base.Reverse(true)
		}
		return base
	}
	lead := strings.Repeat(" ", factIndent)
	if selected {
		lead = relationStyle.Render(">") + " "
	}
	var value string
	for _, part := range f.value {
		value += style(part.style).Render(part.text)
	}
	label := style(mutedStyle).Render(f.label + strings.Repeat(" ", labels-lipgloss.Width(f.label)))
	column := factIndent + labels + factGap
	if f.label == "" {
		column = factIndent
		label = ""
	} else {
		label += strings.Repeat(" ", factGap)
	}
	if width-column < narrowValue && f.label != "" {
		under := factIndent * 2
		lines := []string{truncate(lead+label, width)}
		for _, line := range strings.Split(wrap(value, max(width-under, 1)), "\n") {
			lines = append(lines, strings.Repeat(" ", under)+strings.TrimRight(line, " "))
		}
		return lines
	}
	wrapped := strings.Split(wrap(value, max(width-column, 1)), "\n")
	lines := []string{lead + label + strings.TrimRight(wrapped[0], " ")}
	for _, line := range wrapped[1:] {
		lines = append(lines, strings.Repeat(" ", column)+strings.TrimRight(line, " "))
	}
	return lines
}
