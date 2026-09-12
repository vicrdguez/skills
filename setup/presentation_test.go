package setup

import (
	"encoding/json"
	"testing"

	"github.com/vicrdguez/skills/workflow"
)

func TestGitHubLifecyclePresentationKeepsNativeJSON(t *testing.T) {
	output, err := PresentImplementation(workflow.ImplementationOutcome{
		Status: "inspected",
		Item: &workflow.ImplementationItem{
			ID: "7", Order: 7, Blockers: []workflow.WorkItemID{"2"},
			Submission: &workflow.Submission{ID: "11"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	item := decoded["item"].(map[string]any)
	submission := item["Submission"].(map[string]any)
	if item["Number"] != float64(7) || submission["Number"] != float64(11) || item["Blockers"].([]any)[0] != float64(2) {
		t.Fatalf("native identities changed: %s", payload)
	}
	for _, key := range []string{"ID", "Order"} {
		if _, found := item[key]; found {
			t.Errorf("internal %s leaked into item: %s", key, payload)
		}
	}
	if _, found := submission["ID"]; found {
		t.Errorf("internal ID leaked into submission: %s", payload)
	}
}
