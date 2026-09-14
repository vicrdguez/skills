package skills

import (
	"strings"
	"testing"
)

func TestPacketsUseIntegrationReferences(t *testing.T) {
	packet, err := BuildPacket("implement", InvocationFacts{Implementation: &ImplementationFacts{WorkItemReference: "ticket-7"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packet.Instructions, "Work Item: ticket-7") || strings.Contains(packet.Instructions, "Work Item: #0") {
		t.Fatalf("implementation reference rendered by Catalog:\n%s", packet.Instructions)
	}

	packet, err = BuildPacket("watchdog", InvocationFacts{Watchdog: &WatchdogFacts{WorkItemReference: "ticket-7", SubmissionReference: "change-11"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packet.Instructions, "Work Item: ticket-7\nSubmission: change-11") {
		t.Fatalf("review references rendered by Catalog:\n%s", packet.Instructions)
	}
}
