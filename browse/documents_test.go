package browse_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vicrdguez/skills/browse"
	"github.com/vicrdguez/skills/ledger"
)

func TestDocumentsRenderInTheStyleChosenAtStartup(t *testing.T) {
	snapshot := fixtureLedger(t)
	render := func(dark bool) string {
		s := &session{t: t, model: browse.New(snapshot, browse.Options{Project: "widgets", DarkBackground: dark})}
		s.send(tea.WindowSizeMsg{Width: 140, Height: 120})
		s.press("enter", "down", "enter", "d", "down", "down", "enter")
		s.shows("The recorded implementation evidence is readable")
		opened := s.model.View()
		s.press("?", "?")
		s.send(tea.WindowSizeMsg{Width: 70, Height: 30})
		s.send(tea.WindowSizeMsg{Width: 140, Height: 120})
		if relaid := s.model.View(); relaid != opened {
			t.Fatalf("a later layout changed the style or text:\nopened:\n%s\nrelaid:\n%s", opened, relaid)
		}
		return opened
	}
	// Tests write to no terminal, so a style detected from the terminal
	// would render both sessions alike, without styling.
	dark, light := render(true), render(false)
	if dark == ansi.Strip(dark) || light == ansi.Strip(light) {
		t.Fatal("a startup style choice rendered unstyled Markdown")
	}
	if dark == light || ansi.Strip(dark) != ansi.Strip(light) {
		t.Fatalf("dark and light choices must render the same text in different styles:\ndark:\n%q\nlight:\n%q", dark, light)
	}
}

func TestDocumentReferencesKeepMetadataInWideAndStackedPanes(t *testing.T) {
	for _, width := range []int{140, 40} {
		s := start(t, "widgets")
		size := tea.WindowSizeMsg{Width: width, Height: 60}
		s.send(size)
		s.press("enter", "down", "enter", "d")
		s.within(size)
		s.row("╭─ ▶ Documents")
		s.row("╰", "1/5", "╯")
		s.footerLists("? help")
		revision := gitOutput(t, s.root, "rev-parse", "HEAD")[:12]
		if header := s.header(); !strings.HasSuffix(header, " "+revision) || strings.Contains(header, "archived") {
			t.Fatalf("reader header %q, want current ledger %s and no archive marker", header, revision)
		}

		s.press("down", "down", "enter", "r")
		s.within(size)
		s.shows("From cancel/implement-report.md", "Exact ledger reference:")
		s.row("╭─ References (4)")
		s.row("╭─ Reference")
		s.row("╰", "1/4", "╯")
		s.footerLists("? help")
		s.press("enter")
		s.shows("HISTORICAL")
		s.within(size)
		s.press("esc")
		s.row("╭─ References (4)")
		s.press("esc")
		s.shows("▶ cancel/implement-report.md")
		s.within(size)
	}
}

func TestDocumentListLoadsOnlyTheDocumentSelectedForReading(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "d")
	s.shows("Proposal description stays readable", "Slice cancel", "Slice refund")
	s.row("> ", "Proposal description")

	// Once the reader is open, drop the prose objects: a list that had
	// loaded them would still show their text when they are selected.
	proposal := "projects/widgets/proposals/orders/"
	for _, path := range []string{"proposal.md", "cancel/intent.md", "cancel/behavior.md"} {
		blob := gitOutput(t, s.root, "rev-parse", "HEAD:"+proposal+path)
		if err := os.Remove(filepath.Join(s.root, ".git", "objects", blob[:2], blob[2:])); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	s.press(downs(8)...)
	s.shows("cancel/implement-report.md", "The recorded implementation evidence is readable")
	s.press("k", "k")
	s.shows("Exact ledger reference unavailable", "cancel/intent.md", "no substitute was opened")
	s.hides("Cancellation intent")
	s.row("! ", "intent.md")
	s.press(append(repeat("k", 6), "enter")...)
	s.shows("Exact ledger reference unavailable", "substitute was opened")
	s.hides("Proposal description stays readable")
}

func TestOpeningCorruptProseVisiblyRefusesWithoutSubstitution(t *testing.T) {
	const intentPath = "projects/widgets/proposals/orders/cancel/intent.md"
	s := start(t, "widgets", func(root string) {
		// Keep the corrupt checksum beyond Git's header-read prefix.
		body := make([]byte, 100_000)
		random := rand.New(rand.NewSource(1))
		for i := range body {
			body[i] = byte('!' + random.Intn(90))
		}
		write(t, root, intentPath, "Corrupt cancellation intent\n"+string(body))
	})
	blob := gitOutput(t, s.root, "rev-parse", "HEAD:"+intentPath)
	objectPath := filepath.Join(s.root, ".git", "objects", blob[:2], blob[2:])
	object, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	object[len(object)-1] ^= 1
	if err := os.Chmod(objectPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, object, 0o644); err != nil {
		t.Fatal(err)
	}

	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "d")
	s.shows("Slice cancel", "intent.md")
	s.hides("Exact ledger reference unavailable")
	s.press(downs(6)...)
	s.shows("Exact ledger reference unavailable", intentPath, "substitute")
	s.hides("Corrupt cancellation intent")
	// Refusing the damaged selection leaves healthy entries readable.
	s.press("down", "down")
	s.shows("The recorded implementation evidence is readable")
	s.hides("Exact ledger reference unavailable", "Corrupt cancellation intent")
	s.row("! ", "intent.md")
}

func TestArchivedDocumentIsNewerOnlyWhenItsBytesChanged(t *testing.T) {
	var root string
	fixtureLedger(t, func(directory string) {
		root = directory
		write(t, directory, "projects/widgets/proposals/orders/cancel/implement-report.md", "Earlier implementation reasoning.\n")
	})
	// A review whose exact implementation reference names the report
	// before the archive move.
	cancel := "projects/widgets/proposals/orders/cancel/"
	commit := gitOutput(t, root, "rev-parse", "HEAD")
	review, err := ledger.FormatReport(ledger.WatchdogPhase, ledger.Report{
		Schema: 1, Outcome: "pass", Round: 1,
		Source: ledger.SourceRevisions{Head: strings.Repeat("a", 40), Target: strings.Repeat("b", 40), Reviewed: strings.Repeat("a", 40)},
		Ledger: ledger.ReportInputs{
			Claim: ledger.Reference{Commit: commit, Path: cancel + "state.json"}, Contract: []ledger.Reference{{Commit: commit, Path: cancel + "intent.md"}},
			Implement: &ledger.Reference{Commit: commit, Path: cancel + "implement-report.md"},
		},
	}, "Reviewed before archival.\n")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, cancel+"watchdog-report.md", string(review))
	commitFixture(t, root, "review")
	git(t, root, "mv", "projects/widgets/proposals/orders", "projects/widgets/archive/orders")
	git(t, root, "commit", "-q", "-m", "archive orders without changing its records")
	s := &session{t: t, model: browse.New(snapshotAfterChange(t, root), browse.Options{Project: "widgets"})}
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("a", "down", "enter", "down", "enter", "d", "down", "down")
	s.shows("cancel/implement-report.md · current", "The recorded implementation evidence is readable")
	s.press("down", "down", "r", "down", "down", "enter")
	s.shows("cancel/implement-report.md · HISTORICAL "+commit[:12], "The recorded implementation evidence is readable")
	s.hides("Newer document available")
	s.press("esc", "esc", "esc", "up")
	s.shows("HISTORICAL", "Earlier implementation reasoning", "Newer document available")
}
