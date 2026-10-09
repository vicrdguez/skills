package browse_test

// Interaction tests drive the browser through key and resize messages and
// observe its rendered view and external-browser requests over a real
// committed ledger fixture.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/vicrdguez/skills/browse"
	"github.com/vicrdguez/skills/ledger"
)

func write(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// fixtureLedger commits two Projects: widgets with an attached, watchdog
// claimed Slice that depends on a Slice of an archived Proposal and blocks a
// sibling, and gadgets with one Slice.
func fixtureLedger(t *testing.T, extras ...func(string)) *ledger.Snapshot {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.name", "Ledger")
	git(t, root, "config", "user.email", "ledger@example.com")
	write(t, root, "projects/widgets/project.json", `{"repository": "acme/widgets"}`)
	write(t, root, "projects/widgets/proposals/orders/proposal.json", `{"accepted": "2024-01-01T00:00:00Z", "parent_title": "Order cancellation", "parent_issue": {"repository": "acme/widgets", "number": 10}}`)
	write(t, root, "projects/widgets/proposals/orders/proposal.md", "# Order cancellation\n\nProposal description stays readable.\n")
	cancelPath := "projects/widgets/proposals/orders/cancel"
	write(t, root, cancelPath+"/intent.md", "# Intent\n\nCancellation intent.\n")
	write(t, root, cancelPath+"/behavior.md", "# Behavior\n\nCancellation behavior.\n")
	write(t, root, "projects/widgets/proposals/orders/refund/intent.md", "# Intent\n")
	write(t, root, "projects/widgets/proposals/orders/refund/behavior.md", "# Behavior\n")
	write(t, root, "projects/widgets/proposals/orders/broken/intent.md", "# Intent\n")
	write(t, root, "projects/widgets/proposals/orders/broken/behavior.md", "# Behavior\n")
	write(t, root, cancelPath+"/state.json", `{"state": "ready_for_implementation", "title": "Cancel orders", "branch": "feat/cancel",
		"issue": {"repository": "acme/widgets", "number": 11}, "submission": {"repository": "acme/widgets", "number": 12},
		"dependencies": ["proposals/legacy/old"], "claim": {"phase": "watchdog", "basis": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "inputs": {"contract": []}}}`)
	write(t, root, "projects/widgets/proposals/orders/refund/state.json", `{"state": "ready_for_implementation", "title": "Refund orders", "branch": "feat/refund",
		"dependencies": ["proposals/orders/cancel", "proposals/gone/missing"]}`)
	write(t, root, "projects/widgets/proposals/orders/broken/state.json", `{broken`)
	write(t, root, "projects/widgets/archive/legacy/proposal.json", `{"accepted": "2023-01-01T00:00:00Z"}`)
	write(t, root, "projects/widgets/archive/legacy/proposal.md", "# Legacy proposal\n\nArchived description.\n")
	write(t, root, "projects/widgets/archive/legacy/old/state.json", `{"state": "merged", "title": "Old work", "branch": "old", "completion": {"target": {"repository": "acme/widgets", "branch": "main"}}}`)
	write(t, root, "projects/widgets/archive/legacy/old/intent.md", "# Intent\n")
	write(t, root, "projects/widgets/archive/legacy/old/behavior.md", "# Behavior\n")
	write(t, root, "projects/gadgets/project.json", `{"repository": "acme/gadgets"}`)
	write(t, root, "projects/gadgets/proposals/tools/proposal.json", `{"accepted": "2024-01-01T00:00:00Z"}`)
	write(t, root, "projects/gadgets/proposals/tools/proposal.md", "# Tools proposal\n")
	write(t, root, "projects/gadgets/proposals/tools/hammer/state.json", `{"state": "rework", "title": "Hammer", "branch": "hammer"}`)
	write(t, root, "projects/gadgets/proposals/tools/hammer/intent.md", "# Intent\n")
	write(t, root, "projects/gadgets/proposals/tools/hammer/behavior.md", "# Behavior\n")
	for _, extra := range extras {
		extra(root)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "accepted records")
	base := gitOutput(t, root, "rev-parse", "HEAD")
	claimRef := ledger.Reference{Commit: base, Path: cancelPath + "/state.json"}
	contractRef := []ledger.Reference{{Commit: base, Path: cancelPath + "/intent.md"}, {Commit: base, Path: cancelPath + "/behavior.md"}}
	write(t, root, cancelPath+"/state.json", `{"state": "awaiting_review", "title": "Cancel orders", "branch": "feat/cancel",
		"issue": {"repository": "acme/widgets", "number": 11}, "submission": {"repository": "acme/widgets", "number": 12}, "decision": true,
		"dependencies": ["proposals/legacy/old"], "claim": {"phase": "watchdog", "basis": "3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c", "inputs": {"contract": [{"commit": "`+base+`", "path": "`+cancelPath+`/intent.md"}]}}}`)
	implementation, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
		Schema: 1, Outcome: "awaiting_review",
		Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40)},
		Ledger: ledger.ReportInputs{
			Claim: claimRef, Contract: contractRef,
			Watchdog: &ledger.Reference{Commit: strings.Repeat("c", 40), Path: cancelPath + "/watchdog-report.md"},
		},
	}, "## Implementation evidence\n\nThe recorded implementation evidence is readable.\n\n"+strings.Repeat("Further evidence.\n\n", 30))
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, cancelPath+"/implement-report.md", string(implementation))
	write(t, root, cancelPath+"/watchdog-report.md", "---\nschema: unsupported\nunknown_field: true\n---\n\n## Watchdog bytes remain readable\n\nMalformed metadata does not hide this body.\n")
	decision, err := ledger.FormatDecision(ledger.DecisionRecord{
		Schema: 1, Project: "widgets", Item: "orders/cancel", AnsweredRequest: claimRef, Route: "implement",
	}, "The human direction remains available as Markdown.\n")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, cancelPath+"/decision.md", string(decision))
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "phase evidence")
	store, err := ledger.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

type session struct {
	t      *testing.T
	model  tea.Model
	root   string
	opened []string
}

func start(t *testing.T, project string, extras ...func(string)) *session {
	s := &session{t: t}
	extras = append(extras, func(root string) { s.root = root })
	s.model = browse.New(fixtureLedger(t, extras...), browse.Options{Project: project, Open: func(url string) error {
		s.opened = append(s.opened, url)
		return nil
	}})
	return s
}

// press sends keys and runs any command they return, as the program would.
func (s *session) press(keys ...string) {
	s.t.Helper()
	for _, name := range keys {
		var msg tea.KeyMsg
		switch name {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
		}
		s.send(msg)
	}
}

func (s *session) send(msg tea.Msg) {
	var command tea.Cmd
	s.model, command = s.model.Update(msg)
	if command != nil {
		s.model, _ = s.model.Update(command())
	}
}

// text is the view as the human reads it, without terminal styling.
func (s *session) text() string {
	return ansi.Strip(s.model.View())
}

func (s *session) shows(fragments ...string) {
	s.t.Helper()
	view := s.text()
	for _, fragment := range fragments {
		if !strings.Contains(view, fragment) {
			s.t.Fatalf("view lacks %q:\n%s", fragment, view)
		}
	}
}

// row returns the first view line showing every fragment in order, failing
// when no line does.
func (s *session) row(fragments ...string) string {
	s.t.Helper()
	view := s.model.View()
	for _, line := range strings.Split(view, "\n") {
		rest, found := line, true
		for _, fragment := range fragments {
			_, after, ok := strings.Cut(rest, fragment)
			rest, found = after, found && ok
		}
		if found {
			return line
		}
	}
	s.t.Fatalf("no view line shows %q in order:\n%s", fragments, view)
	return ""
}

// fact fails unless one view line shows label, then the label column's
// padding, then value.
func (s *session) fact(label, value string) {
	s.t.Helper()
	if !s.hasFact(label, value) {
		s.t.Fatalf("view lacks fact %s %q:\n%s", label, value, s.text())
	}
}

func (s *session) noFact(label, value string) {
	s.t.Helper()
	if s.hasFact(label, value) {
		s.t.Fatalf("view unexpectedly shows fact %s %q:\n%s", label, value, s.text())
	}
}

func (s *session) hasFact(label, value string) bool {
	for _, line := range strings.Split(s.text(), "\n") {
		for rest := line; ; {
			_, after, found := strings.Cut(rest, label)
			if !found {
				break
			}
			if strings.HasPrefix(after, "  ") && strings.Contains(after, value) {
				return true
			}
			rest = after
		}
	}
	return false
}

func (s *session) hides(fragments ...string) {
	s.t.Helper()
	view := s.text()
	for _, fragment := range fragments {
		if strings.Contains(view, fragment) {
			s.t.Fatalf("view unexpectedly shows %q:\n%s", fragment, view)
		}
	}
}

// header and footer are the first and last lines of the view.
func (s *session) header() string {
	lines := strings.Split(s.model.View(), "\n")
	return lines[0]
}

func (s *session) footer() string {
	lines := strings.Split(s.model.View(), "\n")
	return lines[len(lines)-1]
}

func (s *session) footerLists(helps ...string) {
	s.t.Helper()
	footer := s.footer()
	for _, help := range helps {
		if !strings.Contains(footer, help) {
			s.t.Fatalf("footer lacks %q: %q", help, footer)
		}
	}
}

func (s *session) footerOmits(helps ...string) {
	s.t.Helper()
	footer := s.footer()
	for _, help := range helps {
		if strings.Contains(footer, help) {
			s.t.Fatalf("footer lists %q: %q", help, footer)
		}
	}
}

func TestBrowserNavigatesProjectsProposalsAndSliceFacts(t *testing.T) {
	s := start(t, "")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.shows("Projects (2)")
	s.row("> ", "gadgets", "acme/gadgets", "1 slice")
	s.row("! widgets", "acme/widgets")

	s.press("down", "enter")
	s.shows("Project widgets (acme/widgets)", "Proposals (1)", "! Incomplete")
	s.row("! orders", "0 of 3 Merged · 1 unknown")
	s.hides("legacy")

	s.press("enter")
	s.shows("Slices (3)", "Order cancellation")
	s.row("cancel", "◐ Awaiting Review", "▸ watchdog")
	s.row("! broken", "! lifecycle unknown", "claim unknown")
	if refund := s.row("refund", "○ Ready for Implementation"); strings.Contains(refund, "▸") {
		t.Fatalf("an unclaimed Slice shows a Claim marker: %q", refund)
	}

	s.press("down", "enter")
	s.fact("Lifecycle", "◐ Awaiting Review")
	s.fact("Claim", "▸ watchdog reservation")
	s.fact("Claim", "▸ watchdog reservation at ledger basis 3c3c")
	s.shows("running worker")
	s.fact("Branch", "feat/cancel")
	s.fact("Issue", "acme/widgets#11")
	s.fact("Pull request", "acme/widgets#12")
	s.hides("running worker status")

	s.press("esc", "k", "enter")
	s.shows("state.json is unreadable")
	s.fact("Lifecycle", "lifecycle unknown")
	s.fact("Claim", "claim unknown")

	s.press("s")
	s.shows("Projects (2)")
	s.row("> ! widgets", "acme/widgets", "3 slices", "1 claimed")
	s.press("k", "enter", "enter", "enter")
	s.shows("Slice  tools/hammer")
	s.fact("Lifecycle", "Rework")
	if len(s.opened) != 0 {
		t.Fatalf("navigation opened links: %v", s.opened)
	}
}

func TestBrowserStartsAtExplicitProjectAndSwitches(t *testing.T) {
	s := start(t, "gadgets")
	s.shows("skl browse › Projects › gadgets", "tools")
	s.press("s", "down", "enter")
	s.shows("skl browse › Projects › widgets", "orders")
}

func TestBrowserHeaderIsOneLineAndMentionsArchivesOnlyWhenShown(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	revision := gitOutput(t, s.root, "rev-parse", "HEAD")[:12]
	lines := strings.Split(s.model.View(), "\n")
	if !strings.HasPrefix(lines[0], "skl browse › Projects › widgets ") || !strings.HasSuffix(lines[0], " "+revision) ||
		strings.Contains(lines[0], "archived") {
		t.Fatalf("header %q, want the breadcrumb, then %s, and no archives", lines[0], revision)
	}
	if strings.HasPrefix(lines[1], "skl browse") || strings.Contains(lines[1], revision) {
		t.Fatalf("header continues on a second line: %q", lines[1])
	}
	s.shows("Proposals (1)")
	s.row("╭─ Proposals (1)")
	s.row("╭─ Proposal")
	s.row("╰", "1/1", "╯")
	s.footerLists("? help")
	s.hides("legacy")

	s.press("a")
	if header := s.header(); !strings.HasPrefix(header, "skl browse › Projects › widgets ") ||
		!strings.HasSuffix(header, "archived shown · "+revision) {
		t.Fatalf("header %q, want the breadcrumb, then archived shown and %s", header, revision)
	}
	s.row("legacy [archived]", "1 of 1 Merged · fully delivered")
	s.row("╭─ Proposals (2)")
	s.row("╰", "1/2", "╯")
}

func TestBrowserReadsReportsFollowsExactHistoricalReferencesAndPreservesScroll(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 120})
	s.press("enter", "down", "enter", "d", "down", "down", "enter")
	s.row("╭─ ", "Awaiting Review", "╭─ ▶ cancel/implement-report.md · current · awaiting_review")
	s.shows("Implementation report", "The recorded implementation evidence is readable")

	s.send(tea.WindowSizeMsg{Width: 88, Height: 16})
	s.send(tea.KeyMsg{Type: tea.KeyPgDown})
	if !strings.Contains(s.model.View(), "scrolled ") || strings.Contains(s.model.View(), "scrolled 0%") {
		t.Fatalf("document did not scroll:\n%s", s.model.View())
	}
	s.send(tea.WindowSizeMsg{Width: 90, Height: 17})
	if !strings.Contains(s.model.View(), "scrolled ") || strings.Contains(s.model.View(), "scrolled 0%") {
		t.Fatalf("resize reset document scroll:\n%s", s.model.View())
	}

	s.press("r", "down", "down", "down", "enter")
	s.shows("Exact ledger reference unavailable", strings.Repeat("c", 40), "watchdog-report.md", "no substitute was opened")
	s.hides("Watchdog bytes remain readable")
	s.press("esc", "r", "enter")
	s.row("cancel/state.json · HISTORICAL", "▸ watchdog claim")
	s.press(repeat("pgdown", 10)...)
	s.shows("ready_for_implementation")
	s.press("esc", "esc")
	s.shows("cancel/implement-report.md · current", "scrolled ")
	s.hides("scrolled 0%")

	// The latest malformed report remains selectable and readable, while its
	// schema failure is visible and never supplies invented metadata.
	s.press("esc")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 120})
	s.press("down")
	s.row("watchdog-report.md · current · ! metadata unreadable ─")
	s.shows("Metadata diagnostic", "watchdog report metadata is", "unreadable", "Watchdog bytes remain readable", "◐ Awaiting Review · ▸ watchdog")
	if len(s.opened) != 0 {
		t.Fatalf("document navigation opened external URLs: %v", s.opened)
	}
}

func TestBrowserKeepsHistoricalDocumentIdentityAndScrollInView(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 80, Height: 10})
	s.press("enter", "down", "enter", "a", "d", "down", "down", "enter", "r", "enter", "pgdown")
	reference := gitOutput(t, s.root, "rev-parse", "HEAD~1")[:12]
	// The pane's title carries the identity; the header repeats neither it
	// nor the scroll position.
	s.row("╭─ ▶ ", "HISTORICAL "+reference)
	if header := s.header(); strings.Contains(header, "HIST") || strings.Contains(header, "scrolled") {
		t.Fatalf("historical document header %q repeats the identity or scroll position", header)
	}
	s.row("╰", "scrolled ")
	s.hides("scrolled 0%")

	// Narrower terminals shorten the title's identity and the header's
	// indicators, keeping each of them.
	reference = reference[:7]
	revision := gitOutput(t, s.root, "rev-parse", "HEAD")[:7]
	percent := regexp.MustCompile(`\b[1-9][0-9]*%`)
	for _, width := range []int{40, 34} {
		size := tea.WindowSizeMsg{Width: width, Height: 14}
		s.send(size)
		s.row("╭─ ▶ ", "HIST "+reference)
		header := s.header()
		if strings.Contains(header, "HIST") || percent.MatchString(header) || !strings.Contains(header, "archived") || !strings.Contains(header, revision) {
			t.Fatalf("%d-column historical document header %q, want archived and ledger %s without the identity or scroll",
				width, header, revision)
		}
		s.row("╰", "scrolled ")
		s.within(size)
	}
}

func TestBrowserSelectsEarlierReportVersionAndReturnsToCurrentContext(t *testing.T) {
	s := start(t, "widgets", func(root string) {
		write(t, root, "projects/widgets/proposals/orders/cancel/implement-report.md", "---\nschema: 88\n---\nOlder recorded reasoning.\n")
	})
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "down", "enter", "d", "down", "down")
	s.shows("cancel/implement-report.md · current")
	s.row("╭─ ", "◐ Awaiting Review")
	s.row("> ", "Implementation report", "awaiting_review")
	s.row("! ", "└", "bad metadata")
	s.press("down")
	s.row("╭─ ", "◐ Awaiting Review", "╭─ ", "HISTORICAL", "! unreadable")
	s.shows("Older recorded reasoning", "Metadata diagnostic")
	s.send(tea.WindowSizeMsg{Width: 84, Height: 19})
	if header, revision := s.header(), gitOutput(t, s.root, "rev-parse", "HEAD")[:12]; strings.Contains(header, "HIST") ||
		!strings.HasSuffix(header, revision) {
		t.Fatalf("resized header %q, want ledger %s without the document's identity", header, revision)
	}
	s.row("> ", "└")
	s.press("up")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.row("╭─ ", "◐ Awaiting Review", "╭─ cancel/implement-report.md · current · awaiting_review")
	if len(s.opened) != 0 {
		t.Fatalf("version navigation opened external URLs: %v", s.opened)
	}
}

func TestBrowserFindsReportHistoryWhenLatestReportWasRemoved(t *testing.T) {
	var root string
	fixtureLedger(t, func(directory string) {
		root = directory
		write(t, directory, "projects/widgets/proposals/orders/cancel/implement-report.md", "Older implementation reasoning.\n")
	})
	if err := os.Remove(filepath.Join(root, "projects/widgets/proposals/orders/cancel/implement-report.md")); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "remove latest report")
	s := &session{t: t, model: browse.New(snapshotAfterChange(t, root), browse.Options{Project: "widgets"})}
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "down", "enter", "d", "down", "down")
	s.row("> ", "Implementation report", "no latest report")
	s.hides("not yet available")
	s.row("└", "awaiting_review")
	s.row("! ", "└", "bad metadata")
	s.press("down")
	s.row("╭─ ", "◐ Awaiting Review", "╭─ ", "HISTORICAL")
	s.shows("The recorded implementation evidence is readable")
	s.press("down")
	s.shows("HISTORICAL", "Older implementation reasoning")
}

func snapshotAfterChange(t *testing.T, root string) *ledger.Snapshot {
	t.Helper()
	store, err := ledger.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestBrowserKeepsArchivedReportIdentityAfterOpeningAnEarlierVersion(t *testing.T) {
	var root string
	fixtureLedger(t, func(directory string) {
		root = directory
		write(t, directory, "projects/widgets/proposals/orders/cancel/implement-report.md", "Earlier implementation reasoning.\n")
	})
	git(t, root, "mv", "projects/widgets/proposals/orders", "projects/widgets/archive/orders")
	git(t, root, "commit", "-q", "-m", "archive orders")
	write(t, root, "projects/widgets/proposals/orders/proposal.json", `{"accepted":"2025-01-01T00:00:00Z"}`)
	write(t, root, "projects/widgets/proposals/orders/cancel/state.json", `{"state":"ready_for_implementation","title":"New cancellation","branch":"new-cancel"}`)
	write(t, root, "projects/widgets/proposals/orders/cancel/intent.md", "# New cancellation intent\n")
	write(t, root, "projects/widgets/proposals/orders/cancel/behavior.md", "# New cancellation behavior\n")
	write(t, root, "projects/widgets/proposals/orders/cancel/implement-report.md", "Active replacement report.\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "active replacement")
	s := &session{t: t, model: browse.New(snapshotAfterChange(t, root), browse.Options{Project: "widgets"})}
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("a")
	s.shows("Proposals (3)", "orders [archived]")
	s.press("down", "down", "enter", "down", "enter", "d", "down", "down")
	s.row("> ", "Implementation report", "awaiting_review")
	s.row("! ", "└", "bad metadata")
	s.hides("Active replacement report", "versions unavailable", "history incomplete")
	s.press("down")
	s.shows("Earlier implementation reasoning", "HISTORICAL")
	s.hides("Active replacement report")
}

func TestBrowserBrowsesActiveReportInProjectNamedArchive(t *testing.T) {
	var root string
	fixtureLedger(t, func(directory string) { root = directory })
	git(t, root, "mv", "projects/widgets", "projects/archive")
	git(t, root, "commit", "-q", "-m", "rename project archive")
	s := &session{t: t, model: browse.New(snapshotAfterChange(t, root), browse.Options{Project: "archive"})}
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down")
	s.row("> ", "Implementation report", "awaiting_review")
	s.hides("versions unavailable", "history incomplete", "└")
}

func TestBrowserFollowsAnArchivedReportReferenceWithoutUsingTheCurrentSliceLocation(t *testing.T) {
	var root string
	fixtureLedger(t, func(directory string) {
		root = directory
		write(t, directory, "projects/widgets/archive/legacy/old/watchdog-report.md", "Earlier archived watchdog report.\n")
		write(t, directory, "projects/widgets/proposals/legacy/proposal.json", `{"accepted": "2024-01-01T00:00:00Z"}`)
		write(t, directory, "projects/widgets/proposals/legacy/proposal.md", "Active replacement proposal.\n")
		write(t, directory, "projects/widgets/proposals/legacy/old/state.json", `{"state": "merged", "title": "Replacement", "branch": "replacement"}`)
		write(t, directory, "projects/widgets/proposals/legacy/old/watchdog-report.md", "Unrelated active replacement report.\n")
	})
	previous := gitOutput(t, root, "rev-parse", "HEAD")
	watchdogPath := "projects/widgets/archive/legacy/old/watchdog-report.md"
	recorded := gitOutput(t, root, "log", "-1", "--format=%H", "--", watchdogPath)
	currentPath := "projects/widgets/proposals/orders/cancel"
	report, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
		Schema: 1, Outcome: "awaiting_review",
		Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40)},
		Ledger: ledger.ReportInputs{
			Claim:    ledger.Reference{Commit: previous, Path: currentPath + "/state.json"},
			Contract: []ledger.Reference{{Commit: previous, Path: currentPath + "/intent.md"}},
			Watchdog: &ledger.Reference{Commit: previous, Path: watchdogPath},
		},
	}, "Current implementation reasoning.\n")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, currentPath+"/implement-report.md", string(report))
	write(t, root, watchdogPath, "Later archived watchdog report.\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "later archived report and reference")
	s := &session{t: t, model: browse.New(snapshotAfterChange(t, root), browse.Options{Project: "widgets"})}
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("down", "enter", "down", "enter", "d", "down", "down", "enter", "r", "down", "down", "enter")
	// The followed report keeps its archived incarnation: it is named and
	// compared at its own location, not the current Slice's.
	s.row("╭─ ", "◐ Awaiting Review", "╭─ ▶ old/watchdog-report.md · HIST", previous[:7])
	s.shows("Earlier archived watchdog report", "Newer document available")
	s.hides("Later archived watchdog report")
	followed := s.model.View()
	s.press("v")
	s.shows("Followed report versions", recorded[:7])
	s.hides("Unrelated active replacement report")
	// The cursor selects the latest version, then the earlier one, in the
	// reader itself; the unrelated implementation group stays expanded.
	s.press("enter")
	s.shows("Later archived watchdog report", "old/watchdog-report.md")
	s.press("esc", "down", "enter")
	s.row("╭─ ", "watchdog-report.md · HIST", recorded[:7])
	s.shows("Earlier archived watchdog report")
	s.hides("Later archived watchdog report")
	s.press("esc", "v")
	if after := s.model.View(); after != followed {
		t.Fatalf("closing followed versions did not restore the followed document:\nbefore:\n%s\nafter:\n%s", followed, after)
	}
	s.press("esc")
	s.shows("References (3)", "watchdog")
	s.press("esc")
	s.shows("Current implementation reasoning", "cancel/implement-report.md · current")
	s.row("> ", "Implementation report", "awaiting_review")
}

func TestBrowserReturnsFromNestedVersionsToTheDocumentList(t *testing.T) {
	var root string
	fixtureLedger(t, func(directory string) { root = directory })
	if err := os.Remove(filepath.Join(root, "projects/widgets/proposals/orders/cancel/implement-report.md")); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "remove latest report")
	s := &session{t: t, model: browse.New(snapshotAfterChange(t, root), browse.Options{Project: "widgets"})}
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "tab")
	s.row("╭─ ▶ implement-report.md · HISTORICAL")
	s.shows("The recorded implementation evidence is readable")
	s.press("esc")
	s.row("╭─ ▶ ", "◐ Awaiting Review", "╭─ ", "implement-report.md · HISTORICAL")
	s.row("> ", "└", "awaiting_review")
	s.press("esc")
	s.shows("Slice  orders/cancel")
	s.hides("Cannot discover report versions")
}

func TestBrowserFollowsDecisionAndClaimReferencesWithoutChangingSliceContext(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 120})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "down", "enter")
	s.row("╭─ ▶ cancel/decision.md · current · route implement")
	s.shows("Human Decision", "The human direction remains available as Markdown")
	s.press("r", "enter")
	s.row("╭─ ", "◐ Awaiting Review", "▸ watchdog", "╭─ ", "HISTORICAL")
	s.shows("ready_for_implementation")
	s.press("esc", "esc", "esc", "esc")
	s.shows("Slice  orders/cancel", "Claim state  ")
	s.fact("Lifecycle", "Awaiting Review")
	s.press("r", "enter")
	s.row("╭─ ", "◐ Awaiting Review", "╭─ ▶ cancel/state.json · current · ▸ watchdog claim")
	s.press("esc", "esc")
	s.shows("Slice  orders/cancel", "Depends on  legacy/old [archived] — Old work  ✓ Merged · satisfied")
	s.fact("Lifecycle", "Awaiting Review")
}

func TestBrowserFollowsClaimInputsWithoutRewindingCurrentSlice(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "down", "enter", "r", "enter")
	s.row("╭─ ", "◐ Awaiting Review", "╭─ ▶ cancel/state.json · current")
	s.press("m")
	s.shows("Claim input references", "contract", "Claim basis")
	s.press("m", "r")
	s.shows("References (1)", "contract", "intent.md")
	s.press("enter")
	s.row("╭─ ", "◐ Awaiting Review", "▸ watchdog", "╭─ ", "HISTORICAL")
	s.shows("Cancellation intent")
	s.press("esc")
	s.shows("References (1)", "contract")
	s.press("esc", "esc")
	s.shows("References (1)", "current Claim state")
	s.press("esc")
	s.shows("Slice  orders/cancel")
	s.fact("Lifecycle", "Awaiting Review")
	if len(s.opened) != 0 {
		t.Fatalf("following Claim inputs opened external links: %v", s.opened)
	}
}

func TestBrowserReturnsFromNestedEmptyReferencesToTheOriginatingClaimReference(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 100, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down", "enter", "r", "enter")
	s.row("╭─ ▶ ", "state.json · HIST")

	// The earlier state has an empty Claim input set. Returning from its
	// reference screen must restore the report reference that opened it.
	s.press("r")
	s.shows("References (0)", "No structured exact ledger references")
	s.press("esc", "esc")
	s.shows("References (4)", "claim", "state.json")
	s.press("esc")
	s.row("╭─ ▶ ", "implement-report.md · current")
	s.press("r")
	s.shows("References (4)", "claim", "state.json")
}

func TestBrowserKeepsHealthyDocumentsVisibleAndDiagnosticsScrollable(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 14}, {Width: 140, Height: 30}} {
		s := start(t, "widgets", func(root string) {
			for i := 0; i < 35; i++ {
				write(t, root, fmt.Sprintf("projects/widgets/proposals/orders/damaged%02d/state.json", i), `{`)
			}
		})
		s.send(size)
		s.press("enter", "d")
		s.within(size)
		s.row("╭─ ▶ ", "orders")
		s.shows("diagnostics · d")
		s.row("> ", "Proposal description")
		s.send(tea.WindowSizeMsg{Width: 140, Height: 30})

		s.press("d")
		s.shows("diagnostics", "damaged00")
		for i := 0; i < 100; i++ {
			s.press("pgdown")
		}
		s.shows("damaged34")
		s.press("esc")
		s.row("> ", "Proposal description")
		s.press("enter")
		s.shows("Proposal description · current", "Order cancellation", "Proposal description stays readable")
	}
}

func TestBrowserRestoresSliceReadingPositionAfterDocumentDiagnostics(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 80, Height: 14})
	s.press("enter", "down", "enter", "tab")
	before := s.model.View()
	s.press("d", "d")
	diagnostics := s.model.View()
	s.press("g", "w", "f", "/", "a")
	if s.model.View() != diagnostics {
		t.Fatal("finding keys changed the document diagnostics context")
	}
	s.press("pgdown", "esc", "esc")
	if after := s.model.View(); after != before {
		t.Fatalf("document diagnostics changed the Slice reading position:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestBrowserFooterListsOnlyKeysThatActOnTheScreen(t *testing.T) {
	// Wide enough that the footer lists every key that acts, untrimmed.
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 400, Height: 40})
	s.press("enter", "down", "enter")
	s.shows("Depends on  legacy/old")
	s.footerLists("enter follow relation", "tab next relation", "esc back", "d documents", "r claim reference",
		"i open issue", "p open PR", "f find by lifecycle or claim", "/ search names", "a toggle archived", "? help")
	s.footerOmits("toggle grouping", "toggle scope", "report versions", "diagnostics")

	s.press("/", "cancel", "enter")
	s.footerLists("enter open", "g toggle grouping", "w toggle scope", "d result diagnostics", "? help")
	s.footerOmits("d documents", "relation", "references", "open issue", "open PR")
	// The prompt stays above the footer; typing takes every other key, `?`
	// included, as text.
	s.press("/")
	if lines := strings.Split(s.model.View(), "\n"); !strings.Contains(lines[len(lines)-2], "Search names: █") {
		t.Fatalf("the name-search prompt is not above the footer:\n%s", s.model.View())
	}
	s.footerLists("enter apply • esc cancel")
	s.footerOmits("? help", "toggle grouping")
	s.press("?")
	s.shows("Search names: ?█")
	s.footerOmits("? help")
	s.press("esc")
	s.footerLists("? help")
	s.press("/", "cancel", "enter")
	s.footerLists("? help")

	s = start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 400, Height: 40})
	s.footerLists("enter open", "a toggle archived", "s switch project", "? help")
	s.footerOmits("documents", "open issue", "open PR", "toggle grouping")
	s.press("enter", "d")
	s.footerLists("enter read selected", "tab/→ focus document", "d diagnostics", "esc/← navigator/back", "? help")
	s.footerOmits("d documents", "result diagnostics", "search names", "toggle archived")
}

func TestBrowserNarrowFooterKeepsHelpAndHelpShowsEveryBinding(t *testing.T) {
	size := tea.WindowSizeMsg{Width: 40, Height: 14}
	s := start(t, "widgets")
	s.send(size)
	s.press("/", "cancel", "enter")
	if footer := s.footer(); lipgloss.Width(footer) > size.Width || !strings.HasSuffix(footer, "? help") {
		t.Fatalf("narrow footer %q does not fit or omits help", footer)
	}
	s.press("?")
	s.shows("↑/k up", "↓/j down", "pgup page up", "pgdown page down", "enter open", "esc back",
		"tab next relation", "shift+tab previous relation", "s switch project", "a toggle archived",
		"/ search names", "f find by lifecycle or claim", "g toggle grouping", "w toggle scope",
		"d diagnostics", "d documents", "r references", "m details", "v show/hide versions", "i open issue", "p open PR",
		"R refresh ledger", "? help", "q quit")
	s.within(size)
}

func TestBrowserKeyListYieldsTheBodyToAStatusNotice(t *testing.T) {
	size := tea.WindowSizeMsg{Width: 40, Height: 15}
	s := start(t, "widgets")
	s.send(size)
	s.press("a", "?")
	s.shows("Archived proposals shown", "↑/k up", "R refresh ledger", "q quit")
	s.hides("more keys fit a larger terminal")
	if header := s.header(); !strings.Contains(header, "archived shown") {
		t.Fatalf("first line %q with every binding listed, want the header saying archived shown", header)
	}
	s.within(size)
}

func TestBrowserPreservesRelationScrollAndFindingContextAcrossNestedReferences(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 80, Height: 14})
	s.press("enter", "down", "enter", "tab")
	s.shows("> Blocks      orders/refund")
	s.press("r", "enter", "r", "esc", "esc", "esc")
	s.shows("> Blocks      orders/refund", "scrolled ")

	s = start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 100, Height: 20})
	s.press("/", "cancel", "enter", "enter")
	s.shows("Slice  orders/cancel")
	s.press("r", "enter", "r", "esc", "esc", "esc")
	s.shows("Slice  orders/cancel")
	s.press("esc")
	s.shows(`Finding: any lifecycle · any claim · name contains "cancel"`, "orders/cancel")
}

func TestBrowserUsesOptionalDocumentAvailabilityWithoutInventingAbsence(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 60})
	s.press(append([]string{"enter", "down", "enter", "d"}, downs(3)...)...)
	s.row("> ! ", "Watchdog report", "bad metadata")
	s.shows("watchdog report metadata is", "unreadable", "Watchdog bytes remain readable")
	s.hides("not yet available", "recorded but unavailable")

	s = start(t, "widgets", func(root string) {
		write(t, root, "projects/widgets/proposals/orders/nostate/intent.md", "# Intent\n")
		write(t, root, "projects/widgets/proposals/orders/nostate/behavior.md", "# Behavior\n")
	})
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("enter", "down", "down", "enter", "d")
	s.row("! ", "Human Decision", "membership unknown")
	s.press(downs(4)...)
	s.shows("Whether a Human Decision is active cannot be determined")
}

func TestBrowserReadsArchivedDocumentsAndNamesAbsentOptionalRecords(t *testing.T) {
	s := start(t, "widgets")
	s.press("a", "down", "enter", "d")
	s.row("╭─ ▶ Proposal legacy · archived")
	s.press("enter")
	s.shows("Archived description", "Legacy proposal")

	s = start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("enter", "down", "down", "enter", "d")
	s.row("Implementation report", "not yet available")
	s.row("Watchdog report", "not yet available")
	s.hides("Human Decision", "Metadata diagnostic", "malformed record", "no latest report")
	s.press("down", "down")
	s.shows("has been recorded yet")
}

func TestBrowserOpensRecordedAttachmentsOnlyOnExplicitAction(t *testing.T) {
	s := start(t, "widgets")
	s.press("enter")
	s.press("i")
	s.press("down", "enter")
	if len(s.opened) != 1 || s.opened[0] != "https://github.com/acme/widgets/issues/10" {
		t.Fatalf("proposal parent issue opened %v", s.opened)
	}
	s.press("i", "p")
	want := []string{"https://github.com/acme/widgets/issues/10", "https://github.com/acme/widgets/issues/11", "https://github.com/acme/widgets/pull/12"}
	if strings.Join(s.opened, " ") != strings.Join(want, " ") {
		t.Fatalf("opened %v, want %v", s.opened, want)
	}
	s.shows("Requested https://github.com/acme/widgets/pull/12 in the external browser")

	s.press("esc", "down", "enter", "p")
	if len(s.opened) != 3 {
		t.Fatalf("a Slice without a recorded pull request opened %v", s.opened)
	}
	s.shows("No recorded pull request attachment to open")
}

// fits presses each key sequence in turn at both terminal sizes and fails
// when a view overflows the terminal.
func fits(t *testing.T, sequences [][]string) []*session {
	t.Helper()
	var sessions []*session
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 14}, {Width: 140, Height: 30}} {
		s := start(t, "widgets")
		s.send(size)
		for _, keys := range sequences {
			s.press(keys...)
			s.within(size)
		}
		sessions = append(sessions, s)
	}
	return sessions
}

// within fails when the view overflows a terminal of size.
func (s *session) within(size tea.WindowSizeMsg) {
	s.t.Helper()
	view := s.model.View()
	if height := lipgloss.Height(view); height > size.Height {
		s.t.Fatalf("%dx%d view is %d lines tall:\n%s", size.Width, size.Height, height, view)
	}
	for _, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > size.Width {
			s.t.Fatalf("%dx%d view line is %d wide: %q", size.Width, size.Height, width, line)
		}
	}
}

func TestBrowserFitsNarrowAndWideTerminals(t *testing.T) {
	for _, s := range fits(t, [][]string{nil, {"enter"}, {"down", "enter"}}) {
		s.shows("Slice  orders/cancel")
	}
}

func downs(count int) []string {
	return repeat("down", count)
}

func repeat(key string, count int) []string {
	return strings.Fields(strings.Repeat(key+" ", count))
}

func TestBrowserNavigatesFromClaimFactToSlices(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("f")
	s.shows("skl browse › Find slices › Facts", "! Counted only under Any: 1 with unknown lifecycle, 1 with unknown claim")
	s.row("*", "Any lifecycle", "(3)")
	s.row("◐ Awaiting Review", "(1)")
	s.row("▸ watchdog", "(1)")
	s.row("unclaimed", "(1)")

	s.press(downs(10)...)
	s.shows("Finds: any lifecycle · watchdog claim in Project widgets")
	s.press("enter")
	s.shows("skl browse › Find slices", "Finding: any lifecycle · watchdog claim in Project widgets",
		"Proposal orders", "Undecided: unknown facts", "! 1 matching slice; 1 undecided by unknown facts; incomplete")
	s.row("orders/cancel", "◐ Awaiting Review", "▸ watchdog", "Cancel orders")
	s.row("! orders/broken", "! lifecycle unknown", "claim unknown")
	// A listed Slice's name is followed by its column gap; the preview
	// names related Slices too, followed by their titles.
	s.hides("orders/refund  ", "tools/hammer")

	s.press("enter")
	s.shows("skl browse › Find slices › widgets › orders/cancel")
	s.fact("Lifecycle", "Awaiting Review")
	s.fact("Claim", "watchdog reservation")
	s.press("esc")
	s.shows("Finding: any lifecycle · watchdog claim")
	s.press("esc")
	s.shows("skl browse › Projects › widgets", "Proposals (1)")
}

func TestBrowserCombinesNameSearchWithFactsScopeAndGrouping(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("/", "q", "u", "i", "t")
	s.shows("Search names: quit█")
	s.press("esc")
	s.hides("Search names:")

	s.press("/", "ORDERS", "enter")
	s.shows(`Finding: any lifecycle · any claim · name contains "ORDERS" in Project widgets`, "3 matching slices", "orders/refund")
	s.press("f", "down", "enter")
	s.shows(`Finding: Ready for Implementation · any claim · name contains "ORDERS"`, "! orders/broken", "1 matching slice; 1 undecided")
	s.row("orders/refund", "○ Ready for Implementation")
	s.hides("orders/cancel  ")

	s.press("/", "hammer", "enter")
	s.shows(`Finding: Ready for Implementation · any claim · name contains "hammer" in Project widgets`, "No Slice is known to match; 1 undecided by unknown facts")
	s.press("w")
	s.shows("in every Project", "No Slice is known to match")
	s.hides("tools/hammer")
	s.press("f", "k", "enter")
	s.shows("any lifecycle · any claim", "gadgets · Proposal tools", "widgets · Undecided")
	s.row("tools/hammer", "↻ Rework", "Hammer")
	s.hides("orders/refund")
	s.press("g")
	s.shows("grouped by lifecycle", "gadgets · ↻ Rework", "widgets · Undecided")
	s.press("enter")
	s.shows("skl browse › Find slices › gadgets › tools/hammer")
	s.fact("Lifecycle", "Rework")
}

func TestBrowserTellsEmptyResultsFromUnknownOnes(t *testing.T) {
	s := start(t, "gadgets")
	s.press("/", "zzz", "enter")
	s.shows("No Slice matches this selection.")
	s.hides("incomplete")

	s.press("w")
	s.shows("No Slice is known to match; 1 undecided by unknown facts; incomplete", "widgets · Undecided", "orders/broken")
	s.press("a", "/", "old", "enter")
	s.shows("archived shown")
	s.row("legacy/old [archived]", "✓ Merged", "Old work")
}

func TestBrowserFitsFindingScreens(t *testing.T) {
	for _, s := range fits(t, [][]string{{"f"}, {"enter"}, {"w", "g"}, {"/", "orders"}, {"enter"}}) {
		s.shows("Find slices", "Finding:", "> ")
	}
}

func TestBrowserReturnsToOriginalSliceAfterFindingFromDetail(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("enter", "down", "enter")
	s.shows("skl browse › Projects › widgets › orders › cancel", "Slice  orders/cancel")
	s.press("/", "refund", "enter", "enter")
	s.shows("skl browse › Find slices › widgets › orders/refund", "Slice  orders/refund")
	s.press("esc")
	s.shows("Finding:", "orders/refund")
	s.press("esc")
	s.shows("skl browse › Projects › widgets › orders › cancel", "Slice  orders/cancel")
	s.press("esc")
	s.shows("skl browse › Projects › widgets › orders", "Slices (3)")
	s.press("esc")
	s.shows("skl browse › Projects › widgets", "Proposals (1)")
}

func TestBrowserKeepsHealthyResultsVisibleWithManyMembershipDiagnostics(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 14}, {Width: 140, Height: 30}} {
		s := start(t, "widgets", func(root string) {
			for i := 0; i < 30; i++ {
				write(t, root, fmt.Sprintf("projects/widgets/proposals/orders/Bad Name %02d/state.json", i), `{}`)
			}
		})
		s.send(size)
		s.press("/", "cancel", "enter")
		s.shows("orders/cancel", "> ", "incomplete", "diagnostic")
		s.press("d")
		s.shows("Diagnostics", "Bad Name 00")
		for i := 0; i < 20; i++ {
			s.press("pgdown")
		}
		s.shows("Bad Name 29")
		s.press("esc")
		s.shows("orders/cancel", "> ")
		s.press("enter")
		s.shows("Slice  orders/cancel")
		s.fact("Lifecycle", "Awaiting Review")
	}
}

func TestBrowserOpensTheSelectedArchivedSliceWhenNamesCollide(t *testing.T) {
	s := start(t, "widgets", func(root string) {
		write(t, root, "projects/widgets/archive/orders/proposal.json", `{"accepted": "2023-01-01T00:00:00Z"}`)
		write(t, root, "projects/widgets/archive/orders/cancel/state.json", `{"state": "merged", "title": "Archived cancellation", "branch": "old-cancel"}`)
		write(t, root, "projects/widgets/archive/orders/cancel/intent.md", "# Intent\n\nArchived cancellation intent.\n")
		write(t, root, "projects/widgets/archive/orders/cancel/behavior.md", "# Behavior\n\nArchived cancellation behavior.\n")
	})
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("a", "/", "Archived cancellation", "enter")
	s.shows("Proposal orders [archived]")
	s.row("orders/cancel [archived]", "✓ Merged")
	s.press("enter")
	s.shows("Slice  orders/cancel", "Title  Archived cancellation")
	s.fact("Location", "archived proposal")
	s.fact("Lifecycle", "Merged")
	s.press("d", "enter")
	s.shows("Find slices › widgets › orders/cancel", "cancel/intent.md · current", "Archived cancellation intent")
	s.hides("Cancellation intent.")
	s.press("esc", "esc")
	s.shows("Slice  orders/cancel", "Title  Archived cancellation")
	s.fact("Lifecycle", "Merged")
	s.press("esc", "esc")
	s.shows("skl browse › Projects › widgets", "archived shown", "Proposals (3)")
	s.press("down", "down", "enter")
	s.shows("Proposal orders [archived]", "Slices (1)")
	s.press("enter")
	s.shows("Title  Archived cancellation")
	s.fact("Lifecycle", "Merged")
}

func TestBrowserKeepsItsContextAfterOpeningAResultElsewhere(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("enter", "/", "hammer", "enter")
	s.shows(`name contains "hammer" in Project widgets`)
	s.press("w", "enter")
	s.shows("skl browse › Find slices › gadgets › tools/hammer")
	s.fact("Lifecycle", "Rework")
	s.press("f")
	s.shows("skl browse › Find slices › Facts")
	s.press("esc")
	s.shows(`name contains "hammer" in every Project`, "tools/hammer")
	s.press("w")
	s.shows(`name contains "hammer" in Project widgets`)
	s.press("esc")
	s.shows("skl browse › Projects › widgets › orders", "Slices (3)")
}

func TestBrowserKeepsArchiveVisibilityWhenLeavingFinding(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("/", "old", "enter", "a", "enter")
	s.shows("Slice  legacy/old")
	s.fact("Location", "archived proposal")
	s.press("esc", "esc")
	s.shows("skl browse › Projects › widgets", "archived shown", "Proposals (2)", "legacy [archived]")
}

func TestBrowserFindsFromRelatedSliceAndRestoresBothHistories(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("enter", "down", "enter", "enter")
	s.shows("skl browse › Projects › widgets › legacy › old")
	s.fact("Location", "archived proposal")

	// Cancelling the fact picker must not consume the relationship history.
	s.press("f", "esc")
	s.shows("skl browse › Projects › widgets › legacy › old")
	s.press("/", "refund", "enter", "d", "esc")
	s.shows("Finding:", "orders/refund")
	s.press("enter", "enter")
	s.shows("skl browse › Find slices › widgets › orders/cancel")
	s.press("esc")
	s.shows("skl browse › Find slices › widgets › orders/refund")
	s.press("esc")
	s.shows("Finding:", "orders/refund")
	s.press("esc")
	s.shows("skl browse › Projects › widgets › legacy › old")
	s.fact("Location", "archived proposal")
	s.press("esc")
	s.shows("skl browse › Projects › widgets › orders › cancel", "> Depends on  legacy/old")
	s.press("esc")
	s.shows("Slices (3)")
	s.row("> ", "cancel", "◐ Awaiting Review")
}

func TestBrowserFollowsArchivedRelationshipsFromResultsWhenNamesCollide(t *testing.T) {
	s := start(t, "widgets", func(root string) {
		write(t, root, "projects/widgets/archive/orders/proposal.json", `{"accepted": "2023-01-01T00:00:00Z"}`)
		write(t, root, "projects/widgets/archive/orders/refund/state.json", `{"state": "merged", "title": "Archived refund", "branch": "old-refund", "dependencies": ["proposals/legacy/old"]}`)
	})
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("/", "cancel", "enter", "enter", "enter")
	s.shows("skl browse › Find slices › widgets › legacy/old", "Blocks      orders/refund [archived]")
	s.hides("archived shown")
	s.press("tab", "enter")
	s.shows("Slice  orders/refund", "Title  Archived refund")
	s.fact("Location", "archived proposal")
	s.fact("Lifecycle", "Merged")
	s.press("esc")
	s.shows("Slice  legacy/old", "> Blocks      orders/refund [archived]")
	s.press("esc")
	s.shows("skl browse › Find slices › widgets › orders/cancel")
	s.press("esc")
	s.shows("Finding:", "orders/cancel")
	s.press("esc")
	s.shows("skl browse › Projects › widgets", "Proposals (1)")
}

func TestBrowserReopensFactsFromRelatedResultWithoutKeepingItsHistory(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	s.press("/", "cancel", "enter", "enter", "enter", "f")
	s.shows("Find slices › Facts")
	s.press("esc")
	s.shows("Finding:", "orders/cancel")
	s.press("enter", "esc")
	s.shows("Finding:", "orders/cancel")
	s.press("esc")
	s.shows("skl browse › Projects › widgets", "Proposals (1)")
}

func TestBrowserFollowsRelationshipsAndRestoresContext(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("enter", "down", "enter")
	s.shows("Slice  orders/cancel",
		"> Depends on  legacy/old [archived] — Old work  ✓ Merged · satisfied",
		"  Blocks      orders/refund — Refund orders  ○ Ready for Implementation")

	s.press("enter")
	s.shows("skl browse › Projects › widgets › legacy › old", "Slice  legacy/old",
		"> Blocks      orders/cancel — Cancel orders  ◐ Awaiting Review")
	s.fact("Location", "archived proposal")
	s.fact("Lifecycle", "✓ Merged")
	s.hides("archived shown")
	s.press("d", "enter")
	s.shows("old/intent.md · current")
	s.press("esc", "esc")
	s.shows("skl browse › Projects › widgets › legacy › old", "Slice  legacy/old", "> Blocks      orders/cancel")
	s.press("a")
	s.shows("archived shown", "Slice  legacy/old")
	s.press("esc")
	s.shows("skl browse › Projects › widgets › orders › cancel", "> Depends on  legacy/old")
	s.hides("archived shown")

	s.press("tab", "enter")
	s.shows("Slice  orders/refund", "> Depends on  orders/cancel — Cancel orders  ◐ Awaiting Review · unsatisfied until Merged",
		"Depends on  gone/missing  ! unresolved: ",
		"Blocks      ! incomplete: 1 unreadable or unsupported record",
		"! slice widgets/orders/broken: state.json is unreadable")
	s.hides("Blocks      none")
	s.press("tab")
	s.shows("> Depends on  orders/cancel")
	s.press("esc")
	s.shows("Slice  orders/cancel", "> Blocks      orders/refund")

	s.press("esc")
	s.shows("Slices (3)")
	s.row("> ", "cancel", "◐ Awaiting Review")
	s.press("esc")
	s.shows("Proposals (1)")
	s.hides("archived shown", "legacy")
	if len(s.opened) != 0 {
		t.Fatalf("following relationships opened links: %v", s.opened)
	}
}
