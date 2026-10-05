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
	s.fact("Lifecycle", "Rework")
	s.send(old)
	s.shows(gitOutput(t, s.root, "rev-parse", "HEAD")[:12])
	s.fact("Lifecycle", "Rework")
	s.noFact("Lifecycle", "Awaiting Review")
}

func TestRemovedSliceDocumentReturnsToUnavailableFactsThenHealthyParent(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "down", "enter", "d", "down", "down", "enter")
	if err := os.RemoveAll(filepath.Join(s.root, "projects/widgets/proposals/orders/cancel")); err != nil {
		t.Fatal(err)
	}
	commitFixture(t, s.root, "remove cancellation")
	s.press("R")
	s.shows("The recorded implementation evidence is readable")
	s.press("esc", "esc")
	s.shows("Unable to show this view", "no Slice")
	s.noFact("Lifecycle", "Awaiting Review")
	s.noFact("Claim", "watchdog")
	s.press("esc")
	s.shows("Slices (2)", "refund")
}

func TestRemovedProposalDocumentReturnsThroughUnavailableContext(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
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
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "down", "enter", "d", "down", "down", "down", "down", "enter")
	s.shows("Human Decision")
	if err := os.Remove(filepath.Join(s.root, "projects/widgets/proposals/orders/cancel/decision.md")); err != nil {
		t.Fatal(err)
	}
	changeFixture(t, s.root, "projects/widgets/proposals/orders/cancel/state.json", `"decision": true,`, "")
	commitFixture(t, s.root, "remove decision")
	s.press("R")
	s.press("esc")
	s.row("╰", "-/4", "╯")
	s.shows("no longer available", "cancel/decision.md · HISTORICAL")
	s.hides("Human Decision         route")
	s.press("down")
	s.shows("cancel/behavior.md · current", "Cancellation behavior")
}

func TestDocumentDiagnosticsStayDiagnosticsWhenFactsAdvance(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "down", "enter", "d", "d")
	s.shows("watchdog report metadata", "diagnostics")
	changeFixture(t, s.root, "projects/widgets/proposals/orders/refund/state.json", `"state": "ready_for_implementation"`, `"state": "merged"`)
	commitFixture(t, s.root, "change another slice")
	s.press("R")
	s.shows("watchdog report metadata", "diagnostics")
	s.hides("Slice  orders/cancel")
	s.noFact("Claim", "watchdog reservation")
}

func TestCurrentClaimReferencesStayCurrentWhenLeavingExactDocument(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("enter", "down", "enter", "r", "enter")
	s.shows("cancel/state.json · current")
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

func TestDocumentReturnKeepsMissingFilteredSliceIdentity(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("f", "down", "down", "enter", "enter", "d", "down", "down", "enter")
	s.shows("The recorded implementation evidence is readable")
	changeFixture(t, s.root, "projects/widgets/proposals/orders/cancel/state.json", `"state": "awaiting_review"`, `"state": "rework"`)
	commitFixture(t, s.root, "selected slice leaves filtered results")
	s.press("R", "R", "esc", "esc")
	s.shows("Slice  orders/cancel")
	s.fact("Lifecycle", "Rework")
	s.press("esc")
	s.shows("Finding: Awaiting Review", "no longer available")
	s.press("enter")
	s.hides("Slice  orders/broken")
}

func TestSearchAcceptsCapitalRefreshKeyAsText(t *testing.T) {
	s := start(t, "widgets")
	s.send(tea.WindowSizeMsg{Width: 120, Height: 80})
	s.press("/", "R", "efund", "enter")
	s.shows(`name contains "Refund"`, "orders/refund")
}
