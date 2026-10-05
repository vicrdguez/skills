package browse_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/vicrdguez/skills/browse"
	"github.com/vicrdguez/skills/ledger"
)

const cancelPath = "projects/widgets/proposals/orders/cancel/"

// review commits a watchdog report of the cancel Slice that consumed its
// intent at the fixture's first commit, returning the recording commit.
func review(t *testing.T, root, outcome string, round uint64, body string) string {
	t.Helper()
	base := gitOutput(t, root, "rev-list", "--max-parents=0", "HEAD")
	report, err := ledger.FormatReport(ledger.WatchdogPhase, ledger.Report{
		Schema: 1, Outcome: outcome, Round: round,
		Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40), Reviewed: strings.Repeat("a", 40)},
		Ledger: ledger.ReportInputs{
			Claim:     ledger.Reference{Commit: base, Path: cancelPath + "state.json"},
			Contract:  []ledger.Reference{{Commit: base, Path: cancelPath + "intent.md"}},
			Implement: &ledger.Reference{Commit: base, Path: cancelPath + "implement-report.md"},
		},
	}, body)
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, cancelPath+"watchdog-report.md", string(report))
	commitFixture(t, root, fmt.Sprintf("review round %d", round))
	return gitOutput(t, root, "rev-parse", "HEAD")
}

// reviewed starts at widgets after the cancel Slice's watchdog report was
// recorded at rework round 1, rework round 2 and pass round 3, returning the
// session and the three recording commits.
func reviewed(t *testing.T) (*session, []string) {
	s := start(t, "widgets")
	var commits []string
	for round, outcome := range []string{"rework", "rework", "pass"} {
		commits = append(commits, review(t, s.root, outcome, uint64(round+1), fmt.Sprintf("Review %d of cancellation.\n", round+1)))
	}
	s.model = browse.New(snapshotAfterChange(t, s.root), browse.Options{Project: "widgets"})
	return s, commits
}

func TestReaderShowsTheNavigatorBesideTheSelectedDocument(t *testing.T) {
	s, _ := reviewed(t)
	size := tea.WindowSizeMsg{Width: 140, Height: 40}
	s.send(size)
	s.press("enter", "down", "enter", "d")
	s.within(size)
	s.row("╭─ ▶ Documents", "╭─ cancel/intent.md · current")
	s.row("> ", "intent.md")
	s.row("Implementation report", "awaiting_review")
	s.row("Watchdog report", "pass")
	s.row("Human Decision", "route implement")

	s.press("down", "down", "down", "tab", "pgdown")
	s.row("╰", "scrolled ", "╯")
	if facts := strings.Split(s.text(), "\n")[0]; strings.Contains(facts, "scrolled") {
		t.Fatalf("the header repeats the scroll position: %q", facts)
	}
	s.shows("esc/← navigator/back", "d diagnostics", "r references", "? help")
	s.press("?")
	s.shows("tab/→ focus document", "pgup page up", "pgdown page down", "v show/hide versions")
}

func TestReaderNestsEarlierReportVersionsUnderTheLatestByOutcomeAndRound(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	s, commits := reviewed(t)
	s.send(tea.WindowSizeMsg{Width: 140, Height: 80})
	s.press("enter", "down", "enter", "d")

	latest := s.lineIndex("Watchdog report")
	s.row("Watchdog report", "pass", "· round 3")
	second, first := s.lineIndex("└ "+commits[1][:7]), s.lineIndex("└ "+commits[0][:7])
	if !(latest < second && second < first) {
		t.Fatalf("versions are not nested newest first under the latest (lines %d, %d, %d):\n%s", latest, second, first, s.text())
	}
	s.row("└ "+commits[1][:7], "rework", "· round 2")
	s.row("└ "+commits[0][:7], "rework", "· round 1")
	s.hides("└ " + commits[2][:7])

	// Outcomes keep their text in the lifecycle colours.
	for _, outcome := range []struct{ row, fragment, want string }{
		{"Watchdog report", "pass", "1;32"},
		{"└ " + commits[1][:7], "rework", "35"},
		{"Implementation report", "awaiting_review", "33"},
	} {
		if got := s.styleOf(outcome.row, outcome.fragment); got != outcome.want {
			t.Errorf("%s %s style = %q, want %q", outcome.row, outcome.fragment, got, outcome.want)
		}
	}

	s.press(downs(4)...)
	s.shows("cancel/watchdog-report.md · HISTORICAL "+commits[1][:12], "Review 2 of cancellation", "Outcome: rework")
	s.press("down")
	s.shows("cancel/watchdog-report.md · HISTORICAL "+commits[0][:12], "Review 1 of cancellation")
}

func TestReaderExpandsAProposalsReportVersionsOnRequest(t *testing.T) {
	s, commits := reviewed(t)
	s.send(tea.WindowSizeMsg{Width: 180, Height: 60})
	s.press("enter", "d")
	s.row("Watchdog report", "pass", "[v]")
	s.hides("└")

	s.press(downs(9)...)
	s.row("> ", "Watchdog report", "pass")
	s.press("v")
	s.row("└ "+commits[1][:7], "rework", "· round 2")
	s.row("└ "+commits[0][:7], "rework", "· round 1")
	s.press("down")
	s.shows("Review 2 of cancellation")
	s.press("v")
	s.row("> ", "Watchdog report", "pass", "3 earlier [v]")
	s.hides("└")
	s.shows("Review 3 of cancellation")
}

func TestReaderCursorSelectsAndFocusMovesBetweenPanes(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 14})
	s.press("enter", "down", "enter", "tab")
	s.shows("scrolled ")
	before := s.model.View()

	s.press("d", "down", "down", "down")
	s.shows("cancel/watchdog-report.md · current")
	s.row("> ", "Watchdog report")
	s.press("tab", "pgdown")
	s.shows("▶ cancel/watchdog-report.md", "scrolled ")
	s.row("> ", "Watchdog report")
	s.press("j")
	s.row("> ", "Watchdog report")
	s.press("esc")
	s.shows("▶ Documents", "cancel/watchdog-report.md")
	s.row("> ", "Watchdog report")
	s.press("esc")
	if after := s.model.View(); after != before {
		t.Fatalf("leaving the reader did not restore the Slice detail:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestReaderShowsOnePaneAtATimeWhenNarrow(t *testing.T) {
	s := start(t, "widgets")
	wide, narrow := tea.WindowSizeMsg{Width: 140, Height: 30}, tea.WindowSizeMsg{Width: 40, Height: 30}
	s.send(wide)
	s.press("enter", "down", "enter", "d", "down", "down", "enter", "pgdown")
	s.shows("scrolled ")
	before := s.model.View()

	s.send(narrow)
	s.within(narrow)
	s.shows("▶ cancel/implement-report.md", "scrolled ")
	s.hides("Documents")
	s.send(wide)
	if after := s.model.View(); after != before {
		t.Fatalf("resizing changed the selected entry or scroll position:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	s.send(narrow)
	s.press("esc")
	s.within(narrow)
	s.shows("▶ Documents")
	s.row("> ", "Implementation")
	s.hides("cancel/implement-report.md")
	s.press("enter")
	s.shows("▶ cancel/implement-report.md")
}

func TestReaderPinsTheDisplayedDocumentWhenANewReviewLands(t *testing.T) {
	s := start(t, "widgets")
	first := review(t, s.root, "rework", 1, "Review 1 of cancellation.\n")
	s.model = browse.New(snapshotAfterChange(t, s.root), browse.Options{Project: "widgets"})
	// Narrow enough that the focused document is shown alone.
	s.send(tea.WindowSizeMsg{Width: 99, Height: 20})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "enter", "pgdown")
	s.shows("cancel/watchdog-report.md · current", "scrolled ")
	before := strings.Split(s.model.View(), "\n")

	second := review(t, s.root, "pass", 2, "Review 2 of cancellation.\n")
	s.press("R")
	s.shows("Newer document available", "cancel/watchdog-report.md · HISTORICAL "+first[:12], "scrolled ")
	// Below the pane's title, which now marks the document historical.
	if after := strings.Split(s.model.View(), "\n"); strings.Join(before[3:12], "\n") != strings.Join(after[3:12], "\n") {
		t.Fatalf("refresh replaced the displayed text or scroll position:\nbefore:\n%s\nafter:\n%s", strings.Join(before, "\n"), strings.Join(after, "\n"))
	}
	s.press("esc")
	s.row("> ", "Watchdog report", "pass", "· round 2")
	s.row("└ "+first[:7], "rework", "· round 1")

	// Leaving the pinned reader restores current sectioned facts, not the
	// outcome of the historical report that remained displayed on refresh.
	s.press("esc")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 80})
	s.fact("Watchdog", "pass · round 2")
	s.fact("Lifecycle", "Awaiting Review")
	s.press("esc")
	s.fact("Watchdog", "pass · round 2")
	s.fact("Lifecycle", "Awaiting Review")

	s.press("enter", "d", "down", "down", "down", "enter")
	s.press(repeat("pgdown", 10)...)
	s.shows("cancel/watchdog-report.md · current", "Review 2 of cancellation")
	s.hides("Newer document available", "HISTORICAL "+second[:12])
}

func TestReaderFollowsAConsumedContractReferenceAndReturns(t *testing.T) {
	s := start(t, "widgets")
	review(t, s.root, "pass", 1, "Reviewed cancellation.\n\n"+strings.Repeat("Reviewed line.\n\n", 30))
	s.model = browse.New(snapshotAfterChange(t, s.root), browse.Options{Project: "widgets"})
	s.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "tab", "pgdown")
	s.shows("▶ cancel/watchdog-report.md · current", "scrolled ")
	s.hides("scrolled 0%")
	before := s.model.View()

	s.press("r")
	s.shows("References (3)", "contract")
	s.press("down", "enter")
	base := gitOutput(t, s.root, "rev-list", "--max-parents=0", "HEAD")
	s.shows("▶ cancel/intent.md · HISTORICAL "+base[:12], "Context at open")
	s.row("> ", "Watchdog report")
	s.press("esc")
	s.shows("References (3)", "contract")
	s.press("esc")
	if after := s.model.View(); after != before {
		t.Fatalf("back did not restore the report at its scroll position:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestReaderMarksAnUnreadableContractBesideReadableDocuments(t *testing.T) {
	s := start(t, "widgets")
	blob := gitOutput(t, s.root, "rev-parse", "HEAD:"+cancelPath+"behavior.md")
	if err := os.Remove(filepath.Join(s.root, ".git", "objects", blob[:2], blob[2:])); err != nil {
		t.Fatal(err)
	}
	s.model = browse.New(snapshotAfterChange(t, s.root), browse.Options{Project: "widgets"})
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("enter", "down", "enter", "d")
	s.shows("Cancellation intent", "! 2 diagnostics · d")
	s.row("! ", "behavior.md", "unreadable")
	s.press("down")
	s.shows("behavior.md is a member of this set but cannot be read")
	s.press("d")
	s.shows("cannot read document " + cancelPath + "behavior.md")
	s.press("esc")
	s.row("> ! ", "behavior.md", "unreadable")
}

func TestReaderDisclosesIncompleteLocalHistory(t *testing.T) {
	var root string
	fixtureLedger(t, func(directory string) { root = directory })
	clone := filepath.Join(t.TempDir(), "shallow")
	if output, err := exec.Command("git", "clone", "-q", "--depth", "1", "file://"+root, clone).CombinedOutput(); err != nil {
		t.Fatalf("shallow clone: %v\n%s", err, output)
	}
	s := &session{t: t, model: browse.New(snapshotAfterChange(t, clone), browse.Options{Project: "widgets"})}
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("enter", "down", "enter", "d")
	s.row("! ", "└", "history incomplete")
	s.press("down", "down", "down")
	s.shows("Local history is incomplete", "shallow")
}

func TestReaderFitsNarrowAndWideTerminals(t *testing.T) {
	for _, s := range fits(t, [][]string{{"enter", "down", "enter", "d"}, {"down", "down"}, {"enter", "pgdown"}, {"r"}, {"enter"}, {"esc", "esc"}, {"d"}}) {
		s.shows("! slice widgets/orders/cancel")
	}
}

func TestReaderLooksForAnAbsentProposalReportsHistoryOnRequest(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 180, Height: 60})
	s.press("enter", "d")
	s.press(downs(13)...)
	s.row("> ", "Implementation report", "no latest report", "[v]")
	s.shows("Press v to look for earlier versions")
	s.press("v")
	s.row("> ", "Implementation report", "not yet available")
	s.shows("has been recorded yet")
}
