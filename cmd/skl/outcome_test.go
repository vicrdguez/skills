package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

func TestHandoffOutcomeRendersIdenticallyEveryTime(t *testing.T) {
	result := &ledger.DeliveryResult{
		Status: ledger.AwaitingReview, Item: "widget/foundation",
		Report:      ledger.Reference{Commit: strings.Repeat("a", 40), Path: "projects/widgets/proposals/widget/foundation/implement-report.md"},
		Replication: &ledger.PublicationNote{Status: ledger.PushPending, Detail: "remote unavailable"},
		Publication: &ledger.PublicationNote{Status: ledger.IssuePending, Detail: "forge unavailable"},
	}
	out := handoffOutput(ledger.ImplementPhase, setup.RepositoryContext{Root: "/work/widgets", Remote: "origin"}, result)
	var first string
	for range 50 {
		var rendered bytes.Buffer
		if err := renderDelivery(&rendered, formatMarkdown, ledger.ImplementPhase, "submit", out); err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = rendered.String()
			replication, publication := strings.Index(first, "Ledger replication: pending"), strings.Index(first, "Public presentation: pending")
			if replication < 0 || publication < replication {
				t.Fatalf("ledger replication must precede public presentation:\n%s", first)
			}
		} else if rendered.String() != first {
			t.Fatalf("handoff outcome changed between renders:\n%s\n---\n%s", first, rendered.String())
		}
	}
}
