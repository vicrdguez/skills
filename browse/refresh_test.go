package browse_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func commitFixture(t *testing.T, root, name string) {
	t.Helper()
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", name)
}

func changeFixture(t *testing.T, root, relative, before, after string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), before) {
		t.Fatalf("fixture %s lacks %q", relative, before)
	}
	write(t, root, relative, strings.Replace(string(content), before, after, 1))
}

func TestRefreshKeepsFilteredReportAndReadingPositionWhileFactsAdvance(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 100, Height: 17})
	s.press("/", "cancel", "enter", "f")
	s.press(downs(10)...)
	s.press("enter", "g", "enter", "d", "down", "down", "enter")
	s.send(tea.KeyMsg{Type: tea.KeyPgDown})
	s.shows("scrolled ")
	old := s.model.View()

	cancel := "projects/widgets/proposals/orders/cancel/"
	changeFixture(t, s.root, cancel+"implement-report.md", "The recorded implementation evidence is readable.", "New implementation evidence is readable.")
	changeFixture(t, s.root, cancel+"state.json", `"state": "awaiting_review"`, `"state": "ready_for_merge"`)
	changeFixture(t, s.root, "projects/widgets/proposals/orders/refund/state.json", `"state": "ready_for_implementation"`, `"state": "merged"`)
	commitFixture(t, s.root, "advance report and other slice")
	s.press("R")
	s.shows("Newer document available", "HISTORICAL ", "scrolled ")
	s.hides("New implementation evidence is readable")
	if before, after := strings.Split(old, "\n"), strings.Split(s.model.View(), "\n"); strings.Join(before[1:8], "\n") != strings.Join(after[1:8], "\n") {
		t.Fatalf("refresh replaced document text or reading position:\nbefore:\n%s\nafter:\n%s", old, s.model.View())
	}
	s.press("esc")
	s.shows("Latest implementation report")
	s.press("enter")
	s.send(tea.WindowSizeMsg{Width: 100, Height: 50})
	s.shows("New implementation evidence is readable")
	s.hides("Newer document available")
	s.press("esc", "esc")
	s.shows("Lifecycle: Ready for Merge")
	s.press("esc")
	s.shows(`name contains "cancel"`, "watchdog claim", "grouped by lifecycle", "Ready for Merge", "orders/cancel")
}

func TestAutomaticRefreshIgnoresUncommittedEditsAndLateOldResults(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	s.press("enter", "down", "enter")
	initial := gitOutput(t, s.root, "rev-parse", "HEAD")[:12]
	path := "projects/widgets/proposals/orders/cancel/state.json"
	changeFixture(t, s.root, path, `"state": "awaiting_review"`, `"state": "rework"`)
	s.press("R")
	s.shows("Lifecycle: Awaiting Review", initial)

	// A first request completes against the old commit but is delivered last.
	model, oldCmd := s.model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	s.model = model
	oldResult := oldCmd()
	commitFixture(t, s.root, "record rework")
	auto := s.model.Init()
	if auto == nil {
		t.Fatal("browser did not schedule automatic local checks")
	}
	model, next := s.model.Update(auto())
	s.model = model
	batch, ok := next().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("automatic check did not schedule a refresh and another check")
	}
	s.send(batch[1]())
	s.shows("Lifecycle: Rework", gitOutput(t, s.root, "rev-parse", "HEAD")[:12])
	s.send(oldResult)
	s.shows("Lifecycle: Rework")
	s.hides("NOT REFRESHED")
}

func TestFailedRefreshStaysLoudUntilARefreshSucceeds(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 14}, {Width: 140, Height: 35}} {
		s := start(t, "widgets")
		s.send(size)
		s.press("enter", "down", "enter")
		s.shows("Slice: orders/cancel")
		revision := gitOutput(t, s.root, "rev-parse", "HEAD")[:12]
		missing := s.root + "-temporarily-missing"
		if err := os.Rename(s.root, missing); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Rename(missing, s.root) })
		s.press("R")
		if header := s.header(); !strings.HasSuffix(header, "NOT REFRESHED "+revision) {
			t.Fatalf("%d-column header %q, want NOT REFRESHED %s", size.Width, header, revision)
		}
		s.shows("Slice: orders/cancel", "! Refresh failed; displayed facts are")
		s.fitsIn(size)
		// Listing every binding keeps the marker and the reason in view.
		s.press("?")
		if header := s.header(); !strings.HasSuffix(header, "NOT REFRESHED "+revision) {
			t.Fatalf("%d-column header %q with every binding listed, want NOT REFRESHED %s", size.Width, header, revision)
		}
		s.shows("! Refresh failed; displayed facts are", "↑/k up")
		s.fitsIn(size)
		s.press("?")
		if err := os.Rename(missing, s.root); err != nil {
			t.Fatal(err)
		}
		s.press("R")
		if header := s.header(); !strings.HasSuffix(header, " "+revision) {
			t.Fatalf("%d-column header %q after recovery, want %s", size.Width, header, revision)
		}
		s.shows("Slice: orders/cancel")
		s.hides("NOT REFRESHED", "Refresh failed")
	}
}

func TestRefreshDoesNotSilentlySelectAnotherResultWhenSelectedSliceDisappears(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 35})
	s.press("/", "orders", "enter", "down")
	s.shows("orders/cancel", "orders/refund")
	if err := os.RemoveAll(filepath.Join(s.root, "projects/widgets/proposals/orders/cancel")); err != nil {
		t.Fatal(err)
	}
	commitFixture(t, s.root, "remove cancellation")
	s.press("R")
	s.shows("Selected widgets/orders/cancel", "no longer available", "orders/refund")
	s.press("enter")
	s.shows("no longer available")
	s.hides("Slice: orders/refund")
	s.press("down", "enter")
	s.shows("Slice: orders/refund")
}
