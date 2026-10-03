package browse

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/vicrdguez/skills/ledger"
)

// span is text drawn in one style, such as one column of a list row.
type span struct {
	text  string
	style lipgloss.Style
}

func (s span) render() string {
	return s.style.Render(s.text)
}

// lifecycleLooks is the status vocabulary: how the browser shows a Slice's
// lifecycle wherever it names one. Colours are the terminal's own ANSI
// colours, so they follow its theme. Weight carries urgency: bold for what
// waits on the human, normal for in-flight work, and dim for finished work.
// Every lifecycle keeps its own glyph and label, so none relies on colour.
var lifecycleLooks = map[string]struct {
	glyph string
	style lipgloss.Style
}{
	ledger.NeedsHuman:             {"◆", lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)},
	ledger.ReadyForMerge:          {"●", lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)},
	ledger.ReadyForImplementation: {"○", lipgloss.NewStyle().Foreground(lipgloss.Color("4"))},
	ledger.AwaitingReview:         {"◐", lipgloss.NewStyle().Foreground(lipgloss.Color("3"))},
	ledger.Rework:                 {"↻", lipgloss.NewStyle().Foreground(lipgloss.Color("5"))},
	ledger.Merged:                 {"✓", lipgloss.NewStyle().Faint(true)},
	ledger.Superseded:             {"⊘", lipgloss.NewStyle().Faint(true)},
}

// claimStyle is the Claim's own channel: a colour no lifecycle uses.
var claimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))

// lifecycleSpan names a recorded lifecycle by its glyph and label. A
// lifecycle outside the vocabulary is marked like an unknown one.
func lifecycleSpan(state string) span {
	look, ok := lifecycleLooks[state]
	if !ok {
		return span{"! " + lifecycleLabel(state), warningStyle}
	}
	return span{look.glyph + " " + lifecycleLabel(state), look.style}
}

// unknownLifecycleSpan names a lifecycle that cannot be read. It never takes
// a lifecycle's glyph.
func unknownLifecycleSpan() span {
	return span{"! lifecycle unknown", warningStyle}
}

// claimSpan marks the phase a Claim reserves; an unclaimed Slice has no
// marker. The marker is static because a Claim is a reservation, not a
// running worker.
func claimSpan(phase string) span {
	if phase == "" {
		return span{}
	}
	return span{"▸ " + phase, claimStyle}
}

// unknownClaimSpan names a Claim that cannot be read, so it never reads as
// unclaimed.
func unknownClaimSpan() span {
	return span{"claim unknown", warningStyle}
}
