package browse_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vicrdguez/skills/browse"
	"github.com/vicrdguez/skills/ledger"
)

func TestReaderFollowsActiveReportVersionsAcrossProjectsAndReturnsThroughNestedReferences(t *testing.T) {
	for _, width := range []int{40, 140} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			s := start(t, "widgets", func(root string) {
				write(t, root, "projects/gadgets/proposals/tools/hammer/implement-report.md", "Earlier hammer implementation.\n")
			})
			base := gitOutput(t, s.root, "rev-parse", "HEAD")
			path := "projects/gadgets/proposals/tools/hammer/implement-report.md"
			earlier := gitOutput(t, s.root, "log", "-1", "--format=%H", "--", path)
			inputs := ledger.ReportInputs{
				Claim:    ledger.Reference{Commit: base, Path: "projects/gadgets/proposals/tools/hammer/state.json"},
				Contract: []ledger.Reference{{Commit: base, Path: cancelPath + "intent.md"}},
			}
			report, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
				Schema: 1, Outcome: "awaiting_review", Ledger: inputs,
				Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40)},
			}, "Later hammer implementation.\n")
			if err != nil {
				t.Fatal(err)
			}
			write(t, s.root, path, string(report))
			origin, err := ledger.FormatReport(ledger.WatchdogPhase, ledger.Report{
				Schema: 1, Outcome: "pass", Round: 1,
				Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40), Reviewed: strings.Repeat("a", 40)},
				Ledger: ledger.ReportInputs{
					Claim:     ledger.Reference{Commit: base, Path: cancelPath + "state.json"},
					Contract:  []ledger.Reference{{Commit: base, Path: cancelPath + "intent.md"}},
					Implement: &ledger.Reference{Commit: base, Path: path},
				},
			}, "Originating cancellation review.\n")
			if err != nil {
				t.Fatal(err)
			}
			write(t, s.root, cancelPath+"watchdog-report.md", string(origin))
			commitFixture(t, s.root, "reference another project's implementation")
			s.model = browse.New(snapshotAfterChange(t, s.root), browse.Options{Project: "widgets"})
			size := tea.WindowSizeMsg{Width: width, Height: 80}
			s.send(size)
			s.press("enter", "down", "enter", "d", "down", "down", "down", "enter", "r", "down", "down", "enter")
			s.shows("Earlier hammer implementation")
			s.press("v")
			s.shows("Followed report versions", earlier[:7])
			s.within(size)
			s.press("enter")
			s.shows("Later hammer implementation")
			if width >= 100 {
				// The navigator keeps the cancel Slice's current facts.
				s.row("╭─ ", "Awaiting Review", "╭─ ")
			}
			version := s.model.View()
			s.press("r", "down", "enter")
			s.shows("Cancellation intent")
			s.press("esc", "esc")
			if after := s.model.View(); after != version {
				t.Fatal("nested follow did not restore the version and navigator")
			}
			s.press("esc", "down", "enter")
			s.shows("Earlier hammer implementation")
			s.hides("Later hammer implementation")
			// Refresh advances the version entries but does not replace the
			// selected historical document or rewind the cancellation facts.
			write(t, s.root, path, strings.Replace(string(report), "Later hammer implementation", "Newest hammer implementation", 1))
			commitFixture(t, s.root, "another hammer implementation")
			newest := gitOutput(t, s.root, "rev-parse", "HEAD")
			s.press("R")
			s.shows("Earlier hammer implementation", "Newer document available")
			s.hides("Newest hammer implementation")
			s.press("esc")
			s.shows(newest[:7])
			s.press("esc")
			s.shows("Earlier hammer implementation")
			s.press("esc")
			s.shows("References (3)", "implement")
			s.press("esc")
			s.shows("Originating cancellation review")
			s.hides("Followed report versions")
			s.within(size)
		})
	}
}
