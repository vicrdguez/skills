package browse_test

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUnrelatedCommitDoesNotClaimAReportHasAChangedVersion(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 45})
	s.press("enter", "down", "enter", "d", "down", "down", "enter")
	changeFixture(t, s.root, "projects/widgets/proposals/orders/refund/state.json", `"state": "ready_for_implementation"`, `"state": "merged"`)
	commitFixture(t, s.root, "advance another slice")
	s.press("R")
	s.shows("HISTORICAL ", "The recorded implementation evidence is readable")
	s.hides("Newer document available")
	s.press("esc", "esc")
	s.shows("Blocks      orders/refund", "Merged")
}

func TestOpenedDocumentSurvivesRemovalWithoutPresentingOldDocumentsAsCurrent(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 45})
	s.press("enter", "down", "enter", "d", "down", "down", "enter")
	if err := os.RemoveAll(filepath.Join(s.root, "projects/widgets/proposals/orders/cancel")); err != nil {
		t.Fatal(err)
	}
	commitFixture(t, s.root, "remove slice")
	s.press("R")
	s.shows("The recorded implementation evidence is readable", "Selected entity is no longer available")
	s.press("esc")
	s.shows("Available documents (0)", "no longer available")
	s.hides("Latest implementation report")
}
