package main

import (
	"encoding/json"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

func TestB13CarryExplicitEndpointsThroughGeneratedCommands(t *testing.T) {
	markerless := func(t *testing.T, retired bool) (string, string, string, string) {
		t.Helper()
		root := proposalRepository(t)
		runGit(t, root, "switch", "-c", "widget", "main")
		writeLedger(t, root, "widget", true)
		runGit(t, root, "add", ".changes/widget")
		runGit(t, root, "commit", "-m", "legacy baseline")
		baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		if !retired {
			return root, baseline, "", baseline
		}
		runGit(t, root, "commit", "--allow-empty", "-m", "legacy completion")
		completion := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
		runGit(t, root, "rm", "-r", ".changes/widget")
		runGit(t, root, "commit", "-m", "legacy retirement")
		return root, baseline, completion, strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	}
	for _, test := range []struct {
		name string
		run  func(*testing.T) *skilldist.Packet
	}{
		{"baseline-only implementation start", func(t *testing.T) *skilldist.Packet {
			root, baseline, _, _ := markerless(t, false)
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}
			got := implementCLI(t, root, b, "next", "--artifact-baseline", baseline)
			if got.Packet == nil {
				t.Fatalf("start = %#v", got)
			}
			return got.Packet
		}},
		{"markerless implementation Rework", func(t *testing.T) *skilldist.Packet {
			root, baseline, completion, head := markerless(t, true)
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Rework, Submission: &workflow.Submission{ID: "11", Head: head, PreviousReviewedHead: head}}}}
			got := implementCLI(t, root, b, "next", "--artifact-baseline", baseline, "--artifact-completion", completion)
			if got.Packet == nil {
				t.Fatalf("rework = %#v", got)
			}
			return got.Packet
		}},
		{"markerless Watchdog resume", func(t *testing.T) *skilldist.Packet {
			root, baseline, completion, head := markerless(t, true)
			b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.AwaitingReview, Claimed: true, Submission: &workflow.Submission{ID: "11", Head: head, ReviewedHead: head}}}}
			got := watchdogCLI(t, root, b, "resume", "--item", "7", "--artifact-baseline", baseline, "--artifact-completion", completion)
			if got.Packet == nil {
				t.Fatalf("watchdog = %#v", got)
			}
			return got.Packet
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet := test.run(t)
			encoded, err := json.Marshal(packet.Facts)
			if err != nil {
				t.Fatal(err)
			}
			var baseline, completion string
			if packet.Facts.Implementation != nil {
				facts := packet.Facts.Implementation
				baseline, completion = facts.SuppliedArtifactBaseline, facts.SuppliedArtifactCompletion
				for name, command := range map[string]string{"resume": facts.ResumeCommand, "inspect": facts.InspectCommand, "submit": facts.SubmitCommand, "needs-human": facts.NeedsHumanCommand} {
					if !strings.Contains(command, "--artifact-baseline "+baseline) || completion != "" && !strings.Contains(command, "--artifact-completion "+completion) {
						t.Errorf("%s command lost endpoints: %s", name, command)
					}
				}
				if !strings.Contains(packet.Instructions, facts.InspectCommand) || !strings.Contains(packet.Instructions, facts.SubmitCommand) || !strings.Contains(packet.Instructions, facts.NeedsHumanCommand) {
					t.Error("Audit, Needs Human, or handoff guidance lost exact endpoint commands")
				}
			} else {
				facts := packet.Facts.Watchdog
				baseline, completion = facts.SuppliedArtifactBaseline, facts.SuppliedArtifactCompletion
				for name, command := range map[string]string{"resume": facts.ResumeCommand, "submit": facts.SubmitCommand} {
					if !strings.Contains(command, "--artifact-baseline "+baseline) || !strings.Contains(command, "--artifact-completion "+completion) {
						t.Errorf("%s command lost endpoints: %s", name, command)
					}
				}
			}
			if baseline == "" || !strings.Contains(string(encoded), baseline) || completion != "" && !strings.Contains(string(encoded), completion) {
				t.Fatalf("transient endpoint facts missing: %s", encoded)
			}
		})
	}

	root := proposalRepository(t)
	prepareSlice(t, root, "marked")
	b := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "marked", State: workflow.Ready}}, remoteHeads: map[string]string{"main": strings.TrimSpace(runGitOutput(t, root, "rev-parse", "main"))}}
	got := implementCLI(t, root, b, "next")
	if got.Packet == nil || strings.Contains(got.Packet.Facts.Implementation.ResumeCommand+got.Packet.Facts.Implementation.InspectCommand+got.Packet.Facts.Implementation.SubmitCommand+got.Packet.Facts.Implementation.NeedsHumanCommand, "--artifact-") {
		t.Fatalf("marked work received synthetic flags: %#v", got)
	}
}
