package browse_test

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRefreshUpdatesCurrentClaimReferences(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 35})
	s.press("enter", "down", "enter", "r")
	s.shows("References (1)", "current Claim state")
	path := "projects/widgets/proposals/orders/cancel/state.json"
	content, err := os.ReadFile(s.root + "/" + path)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(content), `, "claim":`)
	if start < 0 {
		t.Fatal("fixture has no claim")
	}
	// Remove the Claim without changing the selected Slice identity.
	write(t, s.root, path, string(content[:start])+`}`)
	commitFixture(t, s.root, "end claim")
	s.press("R")
	s.shows("References (0)")
	s.hides("current Claim state")
	s.press("esc")
	s.shows("Claim: none")
}
