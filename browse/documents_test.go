package browse_test

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vicrdguez/skills/browse"
)

func TestDocumentsRenderInTheStyleChosenAtStartup(t *testing.T) {
	snapshot := fixtureLedger(t)
	render := func(dark bool) string {
		s := &session{t: t, model: browse.New(snapshot, browse.Options{Project: "widgets", DarkBackground: dark})}
		s.send(tea.WindowSizeMsg{Width: 100, Height: 40})
		s.press("enter", "down", "enter", "d", "down", "down", "enter")
		s.shows("The recorded implementation evidence is readable")
		opened := s.model.View()
		s.press("?", "?")
		s.send(tea.WindowSizeMsg{Width: 70, Height: 30})
		s.send(tea.WindowSizeMsg{Width: 100, Height: 40})
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

func TestDocumentListLoadsOnlyTheDocumentSelectedForReading(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 45})
	s.press("enter", "d")
	s.shows("Available documents (10)", "> Proposal description", "cancel — intent.md — accepted contract",
		"cancel — Latest implementation report", "refund — behavior.md — accepted contract")

	// Once the list is open, drop the prose objects: a list that had loaded
	// them would still show their text.
	proposal := "projects/widgets/proposals/orders/"
	for _, path := range []string{"proposal.md", "cancel/intent.md", "cancel/behavior.md", "refund/intent.md", "refund/behavior.md"} {
		blob := gitOutput(t, s.root, "rev-parse", "HEAD:"+proposal+path)
		if err := os.Remove(filepath.Join(s.root, ".git", "objects", blob[:2], blob[2:])); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	s.press(downs(5)...)
	s.press("enter")
	s.shows("Latest implementation report", "The recorded implementation evidence is readable")
	s.press("esc", "k", "k", "k", "k", "k", "enter")
	s.shows("Available documents (10)", "Exact ledger reference unavailable", "substitute was opened")
	s.hides("Proposal description stays readable")
}

func TestArchivedDocumentIsNewerOnlyWhenItsBytesChanged(t *testing.T) {
	var root string
	fixtureLedger(t, func(directory string) {
		root = directory
		write(t, directory, "projects/widgets/proposals/orders/cancel/implement-report.md", "Earlier implementation reasoning.\n")
	})
	git(t, root, "mv", "projects/widgets/proposals/orders", "projects/widgets/archive/orders")
	git(t, root, "commit", "-q", "-m", "archive orders without changing its records")
	s := &session{t: t, model: browse.New(snapshotAfterChange(t, root), browse.Options{Project: "widgets"})}
	s.send(tea.WindowSizeMsg{Width: 120, Height: 45})
	s.press("a", "down", "enter", "down", "enter", "d", "down", "down", "enter")
	s.shows("current document", "The recorded implementation evidence is readable")
	s.press("v", "enter")
	s.shows("HISTORICAL", "projects/widgets/proposals/orders/cancel/implement-report.md", "The recorded implementation evidence is readable")
	s.hides("newer document available", "Newer document available")
	s.press("esc", "v", "down", "enter")
	s.shows("HISTORICAL", "Earlier implementation reasoning", "Newer document available")
}
