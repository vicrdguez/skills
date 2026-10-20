package browse_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/vicrdguez/skills/ledger"
)

// paneText is the text inside one bordered pane, column from the left
// border inward, with all whitespace removed so wrapping at another width
// reads the same.
func paneText(view string, title string) string {
	var text strings.Builder
	inside := -1
	for _, line := range strings.Split(view, "\n") {
		runes := []rune(line)
		if at := strings.Index(line, "╭─ "+title+" "); at >= 0 {
			inside = len([]rune(line[:at]))
			continue
		}
		if inside < 0 || inside >= len(runes) {
			continue
		}
		if runes[inside] == '╰' {
			break
		}
		rest := string(runes[inside+1:])
		if end := strings.Index(rest, "│"); end >= 0 {
			rest = rest[:end]
		}
		text.WriteString(strings.Join(strings.Fields(rest), ""))
	}
	return text.String()
}

func TestReadyForMergeBlockerIsStyledUnsatisfiedAndFollowable(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	s := start(t, "gadgets", func(root string) {
		write(t, root, "projects/gadgets/proposals/tools/saw/state.json", `{"state": "ready_for_merge", "title": "Saw"}`)
		write(t, root, "projects/gadgets/proposals/tools/saw/intent.md", "# Intent\n")
		write(t, root, "projects/gadgets/proposals/tools/saw/behavior.md", "# Behavior\n")
		write(t, root, "projects/gadgets/proposals/tools/hammer/state.json", `{"state": "rework", "title": "Hammer", "branch": "hammer", "dependencies": ["proposals/tools/saw"]}`)
	})
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("enter", "enter")
	s.shows("skl browse › Projects › gadgets › tools › hammer",
		"> Depends on  tools/saw — Saw  ● Ready for Merge · unsatisfied until Merged")
	if got := s.styleOf("tools/saw", "● Ready for Merge"); got != "1;32;7" {
		t.Errorf("selected Ready for Merge blocker style = %q, want bold green reversed", got)
	}

	s.press("enter")
	s.shows("skl browse › Projects › gadgets › tools › saw", "Followed from tools/hammer")
	if got := s.styleOf("tools/hammer", "↻ Rework"); got != "35;7" {
		t.Errorf("selected Rework dependent style = %q, want magenta reversed", got)
	}
	s.press("esc")
	s.shows("skl browse › Projects › gadgets › tools › hammer", "> Depends on  tools/saw")
}

func TestSelectedSliceShowsLatestReportOutcomes(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	hammer := "projects/gadgets/proposals/tools/hammer"
	s := start(t, "gadgets", func(root string) {
		reference := func(path string) ledger.Reference {
			return ledger.Reference{Commit: strings.Repeat("d", 40), Path: path}
		}
		implement := reference(hammer + "/implement-report.md")
		report, err := ledger.FormatReport(ledger.WatchdogPhase, ledger.Report{
			Schema: 1, Outcome: "rework", Round: 2,
			Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40), Reviewed: strings.Repeat("a", 40)},
			Ledger: ledger.ReportInputs{
				Claim: reference(hammer + "/state.json"), Contract: []ledger.Reference{reference(hammer + "/intent.md")},
				Implement: &implement,
			},
		}, "## Review\n")
		if err != nil {
			t.Fatal(err)
		}
		write(t, root, hammer+"/watchdog-report.md", string(report))
	})
	s.send(tea.WindowSizeMsg{Width: 140, Height: 60})
	s.press("enter")
	s.row("> ", "hammer", "↻ Rework")
	s.fact("Watchdog", "rework · round 2")
	s.fact("Implementation", "not yet available")
	if got := s.styleOf("Watchdog", "rework"); got != "35" {
		t.Errorf("rework outcome style = %q, want magenta", got)
	}

	s.press("s", "down", "enter", "enter", "down")
	s.row("> ", "cancel", "◐ Awaiting Review")
	s.fact("Implementation", "awaiting_review")
	if got := s.styleOf("Implementation", "awaiting_review"); got != "33" {
		t.Errorf("awaiting_review outcome style = %q, want yellow", got)
	}
	s.fact("Watchdog", "! watchdog report metadata is unreadable")
	for _, outcome := range []string{"pass", "rework", "needs_human", "awaiting_review"} {
		s.noFact("Watchdog", outcome)
	}
	// Outcomes are facts of the selected Slice only, never of list rows.
	if row := s.row("cancel", "◐ Awaiting Review", "▸ watchdog"); strings.Contains(row, "awaiting_review") {
		t.Fatalf("a list row shows a report outcome: %q", row)
	}
	s.press("enter")
	s.fact("Implementation", "awaiting_review")
	s.fact("Watchdog", "! watchdog report metadata is unreadable")
}

func TestSliceFactsKeepTargetCompletionDecisionAndPendingPush(t *testing.T) {
	t.Parallel()
	s := start(t, "gadgets", func(root string) {
		write(t, root, "projects/gadgets/proposals/tools/hammer/state.json", `{"state": "merged", "title": "Hammer", "branch": "hammer",
			"integration_target": {"repository": "acme/gadgets", "branch": "main"},
			"completion": {"submission": {"repository": "acme/gadgets", "number": 7}, "target": {"repository": "acme/gadgets", "branch": "main"},
				"merge_commit": "`+strings.Repeat("e", 40)+`", "source_head": "`+strings.Repeat("f", 40)+`"},
			"decision": true, "publication": {"push": {"status": "pending", "detail": "ledger push awaits the remote"}}}`)
	})
	s.send(tea.WindowSizeMsg{Width: 200, Height: 60})
	s.press("enter")
	for _, view := range []string{"preview", "detail"} {
		s.fact("Target", "acme/gadgets main")
		s.fact("Completion", "acme/gadgets#7 into main, merge commit "+strings.Repeat("e", 40))
		s.shows("source head " + strings.Repeat("f", 40))
		s.fact("Decision", "active Human Decision")
		s.fact("Pending push", "pending ledger push awaits the remote")
		if view == "preview" {
			s.shows("d proposal documents")
			s.press("enter")
		}
	}
}
