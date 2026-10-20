package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestMissingRoleFieldsStayInheritedUntilFreshRecovery(t *testing.T) {
	t.Parallel()
	f := newPrivateLedgerFixture(t)
	f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"reviewer":{"model":"initial-review"},"helper":{"thinking":"initial-helper"}}}}`, f.clone))
	source, _ := deliverySourceRepo(t)
	f.acceptDelivery(t, newForgeServer(t), source)
	cli := f.app(deliveryNoForgeApp(t))
	session := cli.dispatchJSON(t, "skl implement next --repo "+source+" --harness pi --mode team").Packet.Facts.Delivery
	if session.Reviewer.Model != "initial-review" || session.Reviewer.Thinking != "" || session.Helper.Model != "" || session.Helper.Thinking != "initial-helper" {
		t.Fatalf("missing startup fields did not inherit: %#v / %#v", session.Reviewer, session.Helper)
	}
	f.misconfigure(t, fmt.Sprintf(`{"ledger":%q,"harnesses":{"pi":{"reviewer":{"model":"later-review","thinking":"later-review-thinking"},"helper":{"model":"later-helper","thinking":"later-helper-thinking"}}}}`, f.clone))
	prepared := cli.dispatchJSON(t, session.PrepareCommand)
	inspected := cli.dispatchJSON(t, strings.ReplaceAll(prepared.Packet.Facts.Delivery.InspectCommand, "<observed-target-sha>", "HEAD"))
	for _, out := range []deliveryOutput{prepared, inspected} {
		if out.Packet.Facts.Delivery.Reviewer != nil || out.Packet.Facts.Delivery.Helper != nil || strings.Contains(out.Packet.Instructions, "later-") || strings.Contains(out.Packet.Instructions, "initial-") {
			t.Fatal("source observation supplied or persisted role defaults")
		}
	}
	fresh := cli.dispatchJSON(t, inspected.Packet.Facts.Delivery.ResumeCommand).Packet.Facts.Delivery
	if fresh.Reviewer == nil || fresh.Helper == nil || fresh.Reviewer.Model != "later-review" || fresh.Reviewer.Thinking != "later-review-thinking" || fresh.Helper.Model != "later-helper" || fresh.Helper.Thinking != "later-helper-thinking" {
		t.Fatalf("fresh recovery did not refresh all fields: %#v / %#v", fresh.Reviewer, fresh.Helper)
	}
}
