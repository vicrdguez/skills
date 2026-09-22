package skills

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// publicationFacts is one fully bound presentation recovery invocation for the
// owning skill and presentation kind. The condition selects the narrow
// continuation; the view carries the selected committed inputs.
func publicationFacts(skill, kind, condition string) *PublicationFacts {
	commit := strings.Repeat("a", 40)
	facts := &PublicationFacts{
		Operation:       "recover",
		Kind:            kind,
		Condition:       PublicationCondition(condition),
		RepositoryRoot:  "/tmp/payments",
		Remote:          "origin",
		Item:            "add-refunds/refund",
		ResultDirectory: "/tmp/publication-result",
		RecoverCommand: "skl publication recover --repo '/tmp/payments' --remote 'origin'" +
			" --item 'add-refunds/refund' --kind '" + kind + "'" +
			" --result-directory '/tmp/publication-result' --view 'token'" +
			" --body '/tmp/publication-result/public.md'",
		ResourceCommand: "skl skill --resource reference/publication.md" +
			" --input result_directory='/tmp/publication-result' " + skill,
		ReferenceCommands: []PublicationReference{{
			Commit:  commit,
			Path:    "projects/payments/proposals/add-refunds/refund/behavior.md",
			Purpose: "accepted Contract",
			Command: "skl ledger show --repo '/tmp/payments' --commit '" + commit + "'" +
				" --path 'projects/payments/proposals/add-refunds/refund/behavior.md'",
		}},
		View: ledger.PublicationView{
			Project:    "payments",
			Repository: "vicrdguez/payments",
			Proposal:   "add-refunds",
			Item:       "add-refunds/refund",
			Kind:       kind,
			Token:      "token",
			Title:      "Recover refund presentation",
			Branch:     "add-refunds",
			State:      "Awaiting Review",
			Status:     "pending",
			Detail:     "public presentation has not been attempted",
			Source: ledger.SourceRevisions{
				Head:     strings.Repeat("b", 40),
				Target:   strings.Repeat("c", 40),
				Reviewed: strings.Repeat("d", 40),
			},
			Report:     &ledger.Reference{Commit: commit, Path: "projects/payments/proposals/add-refunds/refund/implement-report.md"},
			Contracts:  []ledger.Reference{{Commit: commit, Path: "projects/payments/proposals/add-refunds/refund/behavior.md"}},
			Attachment: &ledger.ForgeAttachment{Repository: "vicrdguez/payments", Number: 12},
		},
	}
	if condition == string(PublicationCurrentBody) || condition == string(PublicationPending) {
		facts.View.BodyPath = "/tmp/publication-result/public.md"
	}
	return facts
}

// TestPublicationPacketRecoversEachOwnerPresentation proves a publication
// packet is a narrow presentation recovery invocation: no bundled execution
// skills, no Claim, no rerun procedure, and the bound continuation with the
// exact private references.
func TestPublicationPacketRecoversEachOwnerPresentation(t *testing.T) {
	for _, testCase := range []struct{ skill, kind string }{
		{"propose", "issue"},
		{"propose", "parent"},
		{"implement", "pull"},
		{"watchdog", "pull"},
	} {
		t.Run(testCase.skill+"/"+testCase.kind, func(t *testing.T) {
			facts := publicationFacts(testCase.skill, testCase.kind, string(PublicationProseNeeded))
			packet, err := BuildPacket(testCase.skill, InvocationFacts{Publication: facts})
			if err != nil {
				t.Fatal(err)
			}
			if len(packet.IncludedSkills) != 0 {
				t.Fatalf("publication included skills = %v, want none", packet.IncludedSkills)
			}
			if !slices.Contains(packet.Resources, "reference/publication.md") {
				t.Fatalf("publication resources = %v, want the deferred authoring resource", packet.Resources)
			}
			active := packet.Instructions
			wants := []string{
				facts.Item,
				facts.RepositoryRoot,
				facts.Remote,
				facts.RecoverCommand,
				facts.ResourceCommand,
				facts.ReferenceCommands[0].Command,
				facts.ReferenceCommands[0].Commit + ":" + facts.ReferenceCommands[0].Path,
				"Manual Verification",
				"reference/publication.md",
			}
			if testCase.kind == "pull" {
				wants = append(wants, facts.View.Source.Reviewed, facts.View.Source.Head)
			}
			for _, want := range wants {
				if !strings.Contains(active, want) {
					t.Errorf("%s publication is missing %q", testCase.skill, want)
				}
			}
			for _, forbidden := range []string{
				"## Included Skill:",
				"## Audit once",
				"Invoke the bundled Audit",
				"Run the Full Gate",
				"Implement one accepted change",
				"Claim:",
				"{{",
			} {
				if strings.Contains(active, forbidden) {
					t.Errorf("%s publication retained %q", testCase.skill, forbidden)
				}
			}
		})
	}
}

// TestPublicationPacketSpecializesCondition proves the rendered continuation
// follows the current condition instead of replaying a generic procedure.
func TestPublicationPacketSpecializesCondition(t *testing.T) {
	cases := []struct {
		condition string
		want      string
	}{
		{string(PublicationCurrentBody), "Reuse the registered current body"},
		{string(PublicationProseNeeded), "No applicable temporary public body remains"},
		{string(PublicationStale), "Do not publish it: author a fresh"},
		{string(PublicationAmbiguous), "could not establish a unique forge effect"},
		{string(PublicationSatisfied), "Author nothing, change nothing"},
	}
	for _, testCase := range cases {
		t.Run(testCase.condition, func(t *testing.T) {
			packet, err := BuildPacket("watchdog", InvocationFacts{Publication: publicationFacts("watchdog", "pull", testCase.condition)})
			if err != nil {
				t.Fatal(err)
			}
			for _, other := range cases {
				if other.condition == testCase.condition {
					continue
				}
				if strings.Contains(packet.Instructions, other.want) {
					t.Errorf("%s render also carried the %s continuation", testCase.condition, other.condition)
				}
			}
			if !strings.Contains(packet.Instructions, testCase.want) {
				t.Errorf("%s render is missing %q", testCase.condition, testCase.want)
			}
		})
	}
}

// TestPublicationPacketDefersAuthoringResource proves a prose-needed recovery
// binds the authoring resource and continuation without embedding the resource
// body, while a reusable body needs no new prose.
func TestPublicationPacketDefersAuthoringResource(t *testing.T) {
	needed, err := BuildPacket("implement", InvocationFacts{Publication: publicationFacts("implement", "pull", string(PublicationProseNeeded))})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(needed.Instructions, needed.Facts.Publication.ResourceCommand) {
		t.Fatal("prose-needed recovery lost the deferred authoring command")
	}
	if !strings.Contains(needed.Instructions, needed.Facts.Publication.RecoverCommand) {
		t.Fatal("prose-needed recovery lost the continuation")
	}
	for _, sentinel := range []string{"Author the deliberately public PR body", "deliberately public Final Review Package"} {
		if strings.Contains(needed.Instructions, sentinel) {
			t.Fatalf("prose-needed recovery embedded the deferred resource body %q", sentinel)
		}
	}

	reuse, err := BuildPacket("implement", InvocationFacts{Publication: publicationFacts("implement", "pull", string(PublicationCurrentBody))})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reuse.Instructions, "Retrieve the deferred authoring guidance") {
		t.Fatal("reusable body still asked for new prose")
	}
	if strings.Contains(reuse.Instructions, reuse.Facts.Publication.ResourceCommand) {
		t.Fatal("reusable body rendered the authoring resource command")
	}
	if !strings.Contains(reuse.Instructions, reuse.Facts.Publication.RecoverCommand) {
		t.Fatal("reusable body lost its continuation")
	}
}

// TestPublicationPacketMarkdownAndJSONCarryEquivalentFacts proves the default
// Markdown and explicit JSON forms specialize the same invocation.
func TestPublicationPacketMarkdownAndJSONCarryEquivalentFacts(t *testing.T) {
	packet, err := BuildPacket("watchdog", InvocationFacts{Publication: publicationFacts("watchdog", "pull", string(PublicationProseNeeded))})
	if err != nil {
		t.Fatal(err)
	}
	data, err := packet.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded Packet
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Facts, packet.Facts) {
		t.Fatalf("JSON facts changed:\n%#v\n%#v", decoded.Facts, packet.Facts)
	}
	if decoded.Markdown() != packet.Markdown() {
		t.Fatalf("Markdown and explicit JSON differ:\n%s\n%s", decoded.Markdown(), packet.Markdown())
	}
}

// TestPublicationResourceGuidesEachOwnerBody proves the deferred authoring
// resource distinguishes the issue, progress pull, and Final Review Package
// presentations while keeping private detail and human obligations explicit.
func TestPublicationResourceGuidesEachOwnerBody(t *testing.T) {
	cases := []struct {
		skill   string
		markers []string
	}{
		{"propose", []string{"Parent issue", "Child issue"}},
		{"implement", []string{"progress presentation"}},
		{"watchdog", []string{"Final Review Package", "every box unchecked"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.skill, func(t *testing.T) {
			resource, err := RenderResource(testCase.skill, "reference/publication.md", []string{"result_directory=/tmp/publication-result"})
			if err != nil {
				t.Fatal(err)
			}
			body := string(resource)
			if strings.Contains(body, "{{") {
				t.Fatalf("resource leaked an unrendered template action:\n%s", body)
			}
			for _, want := range append([]string{
				"`/tmp/publication-result/public.md`",
				"Manual Verification obligations remain privately accessible and human-owned",
				"Do not mutate GitHub directly",
				"skl publication recover",
			}, testCase.markers...) {
				if !strings.Contains(body, want) {
					t.Errorf("%s authoring resource is missing %q", testCase.skill, want)
				}
			}
		})
	}
}

// TestPublicationResourceInputContract proves result_directory is required and
// absolute before any procedure is rendered.
func TestPublicationResourceInputContract(t *testing.T) {
	if _, err := RenderResource("propose", "reference/publication.md", nil); err == nil || !strings.Contains(err.Error(), "missing required input") {
		t.Fatalf("missing result_directory error = %v", err)
	}
	if _, err := RenderResource("implement", "reference/publication.md", []string{"result_directory=relative/path"}); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative result_directory error = %v", err)
	}
	if _, err := RenderResource("audit", "reference/publication.md", []string{"result_directory=/tmp/result"}); err == nil || !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("non-owner publication resource error = %v", err)
	}
}

// TestNormalPacketsExcludeDeferredPublicationResource proves existing packets
// keep their accepted manifests and instructions: the authoring resource is
// exposed only to a publication invocation.
func TestNormalPacketsExcludeDeferredPublicationResource(t *testing.T) {
	for _, skill := range []string{"propose", "implement", "watchdog"} {
		packet, err := BuildPacket(skill, InvocationFacts{})
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(packet.Resources, "reference/publication.md") {
			t.Errorf("normal %s packet listed the publication resource", skill)
		}
		if strings.Contains(packet.Instructions, "Publication recovery") || strings.Contains(packet.Instructions, "Publication inspection") {
			t.Errorf("normal %s packet rendered the publication branch", skill)
		}
	}
}
