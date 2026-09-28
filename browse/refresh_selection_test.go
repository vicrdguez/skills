package browse_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRefreshDoesNotReplaceAnOpenedFilteredSliceWhenItStopsMatching(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 140, Height: 35})
	s.press("f", "down", "down", "enter", "enter")
	s.shows("Slice: orders/cancel", "Lifecycle: Awaiting Review")
	changeFixture(t, s.root, "projects/widgets/proposals/orders/cancel/state.json", `"state": "awaiting_review"`, `"state": "rework"`)
	commitFixture(t, s.root, "change selected slice lifecycle")
	s.press("R")
	s.shows("Slice: orders/cancel", "Lifecycle: Rework")
	s.press("esc")
	s.shows("Finding: Awaiting Review", "no longer available")
	s.press("enter")
	s.hides("Slice: orders/broken")
}
