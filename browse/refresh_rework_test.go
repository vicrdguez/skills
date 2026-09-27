package browse_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOverlappingRefreshesCannotPublishAnOlderCommittedRevision(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 35})
	s.press("enter", "down", "enter")
	firstModel, first := s.model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	s.model = firstModel
	secondModel, second := s.model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	s.model = secondModel
	old := second() // The later request reads the earlier revision first.
	changeFixture(t, s.root, "projects/widgets/proposals/orders/cancel/state.json", `"state": "awaiting_review"`, `"state": "rework"`)
	commitFixture(t, s.root, "advance selected slice")
	s.send(first())
	s.shows("Lifecycle: Rework")
	s.send(old)
	s.shows("Lifecycle: Rework", gitOutput(t, s.root, "rev-parse", "HEAD")[:12])
	s.hides("Lifecycle: Awaiting Review")
}

func TestRemovedSliceDocumentReturnsToUnavailableFactsThenHealthyParent(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down", "enter")
	if err := os.RemoveAll(filepath.Join(s.root, "projects/widgets/proposals/orders/cancel")); err != nil {
		t.Fatal(err)
	}
	commitFixture(t, s.root, "remove cancellation")
	s.press("R")
	s.shows("The recorded implementation evidence is readable")
	s.press("esc", "esc")
	s.shows("Unable to show this view", "no Slice")
	s.hides("Lifecycle: Awaiting Review", "Claim: watchdog")
	s.press("esc")
	s.shows("Slices (2)", "refund")
}

func TestRemovedProposalDocumentReturnsThroughUnavailableContext(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("enter", "d", "enter")
	if err := os.RemoveAll(filepath.Join(s.root, "projects/widgets/proposals/orders")); err != nil {
		t.Fatal(err)
	}
	commitFixture(t, s.root, "remove proposal")
	s.press("R")
	s.shows("Proposal description stays readable")
	s.press("esc", "esc")
	s.shows("Unable to show this view", "no Proposal")
	s.hides("Delivery:", "Slices (3)")
	s.press("esc")
	s.shows("Proposals (0)")
}

func TestRemovedSelectedDocumentHasAnExplicitSafeReturnList(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "down", "enter")
	s.shows("Human Decision")
	if err := os.Remove(filepath.Join(s.root, "projects/widgets/proposals/orders/cancel/decision.md")); err != nil {
		t.Fatal(err)
	}
	commitFixture(t, s.root, "remove decision")
	s.press("R")
	s.press("esc")
	s.shows("Available documents (4)", "no longer available")
	s.hides("Current Human Decision")
	s.press("down", "enter")
	s.shows("behavior.md", "Cancellation behavior")
}

func TestDocumentDiagnosticsStayDiagnosticsWhenFactsAdvance(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("enter", "down", "enter", "d", "d")
	s.shows("watchdog report metadata", "diagnostics")
	changeFixture(t, s.root, "projects/widgets/proposals/orders/refund/state.json", `"state": "ready_for_implementation"`, `"state": "merged"`)
	commitFixture(t, s.root, "change another slice")
	s.press("R")
	s.shows("watchdog report metadata", "diagnostics")
	s.hides("Slice: orders/cancel", "Claim: watchdog reservation")
}

func TestCurrentClaimReferencesStayCurrentWhenLeavingExactDocument(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("enter", "down", "enter", "r", "enter")
	s.shows("Claim state")
	path := "projects/widgets/proposals/orders/cancel/state.json"
	content, err := os.ReadFile(filepath.Join(s.root, path))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(content), `, "claim":`)
	if start < 0 {
		t.Fatal("fixture has no claim")
	}
	write(t, s.root, path, string(content[:start])+`}`)
	commitFixture(t, s.root, "end claim while reading")
	s.press("R")
	s.press("esc")
	s.shows("No current Slice Claim", "References (0)")
	s.hides("current Claim state")
}

func TestSearchAcceptsCapitalRefreshKeyAsText(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("/", "R", "efund", "enter")
	s.shows(`name contains "Refund"`, "orders/refund")
}
