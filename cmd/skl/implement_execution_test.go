package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

// runImplementationTransport runs one Implement invocation in the requested
// transport and returns the raw stdout. It fails the check when the invocation
// returns an operational error.
func runImplementationTransport(t *testing.T, root string, backend *implementationMemory, args ...string) string {
	t.Helper()
	if backend.remoteHeads == nil {
		backend.remoteHeads = make(map[string]string)
	}
	if _, ok := backend.remoteHeads["main"]; !ok {
		backend.remoteHeads["main"] = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "refs/heads/main"))
	}
	var output bytes.Buffer
	app := newApp(func(repository github.RepositoryID) (setup.Backend, error) {
		backend.repository = repository
		return backend, nil
	}, bytes.NewReader(nil), &output, &output)
	command := append([]string{"skl", "implement"}, args...)
	command = append(command, "--repo", root)
	if err := app.Run(command); err != nil {
		t.Fatalf("%v: %v\n%s", command, err, &output)
	}
	return output.String()
}

func implementationJSON(t *testing.T, text string) setup.ImplementationOutput {
	t.Helper()
	var output setup.ImplementationOutput
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		t.Fatalf("json transport is not structured: %v\n%s", err, text)
	}
	if output.Packet != nil && output.Packet.Facts.Implementation.ResultDirectory != "" {
		directory := output.Packet.Facts.Implementation.ResultDirectory
		t.Cleanup(func() { os.RemoveAll(directory) })
	}
	return output
}

// cleanupResultDirectories removes the private Result Document directories a
// transport printed, so a Markdown invocation leaves no temporary state behind.
func cleanupResultDirectories(t *testing.T, text string) {
	t.Helper()
	for _, directory := range regexp.MustCompile(`[^\s'"`+"`"+`]*skl-implement-[0-9]+`).FindAllString(text, -1) {
		t.Cleanup(func() { os.RemoveAll(directory) })
	}
}

// normalizeExecution replaces the invocation-specific temporary paths that do
// not carry meaning for transport parity.
func normalizeExecution(text string, root string) string {
	text = strings.ReplaceAll(text, root, "<root>")
	text = strings.ReplaceAll(text, filepath.Dir(filepath.Dir(root)), "<repo>")
	return regexp.MustCompile(`skl-implement-[0-9]+`).ReplaceAllString(text, "<result>")
}

// TestB1StartupTransportDoesNotRepeatTheOperation materializes the B1 outline:
// the default transport is the complete Execution Skill, explicit JSON carries
// the same operation, and formatting never performs a second workflow action.
func TestB1StartupTransportDoesNotRepeatTheOperation(t *testing.T) {
	nextFixture := func(t *testing.T) (string, *implementationMemory) {
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")
		backend := &implementationMemory{work: []workflow.ImplementationItem{
			{ID: "1", Branch: "other", State: workflow.Ready, CreatedAt: "2026-02-01T00:00:00Z"},
			{ID: "7", Branch: "widget", State: workflow.Ready, CreatedAt: "2026-01-01T00:00:00Z"},
		}}
		return root, backend
	}
	resumeItemFixture := func(t *testing.T) (string, *implementationMemory) {
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")
		backend := &implementationMemory{work: []workflow.ImplementationItem{
			{ID: "1", Branch: "other", State: workflow.Ready, CreatedAt: "2026-02-01T00:00:00Z"},
			{ID: "7", Branch: "widget", State: workflow.Ready, CreatedAt: "2026-01-01T00:00:00Z", Claimed: true},
		}}
		return root, backend
	}
	resumeWorktreeFixture := func(t *testing.T) (string, *implementationMemory) {
		root := proposalRepository(t)
		prepareSlice(t, root, "widget")
		runGit(t, root, "switch", "main")
		worktree := filepath.Join(root, ".worktrees", "widget")
		runGit(t, root, "worktree", "add", worktree, "widget")
		backend := &implementationMemory{work: []workflow.ImplementationItem{
			{ID: "1", Branch: "other", State: workflow.Ready, CreatedAt: "2026-02-01T00:00:00Z"},
			{ID: "7", Branch: "widget", State: workflow.Ready, CreatedAt: "2026-01-01T00:00:00Z", Claimed: true},
		}}
		return worktree, backend
	}
	for _, entrypoint := range []struct {
		name  string
		args  []string
		build func(*testing.T) (string, *implementationMemory)
	}{
		{"implement next", []string{"next"}, nextFixture},
		{"implement start", []string{"start"}, nextFixture},
		{"implement resume --item", []string{"resume", "--item", "7"}, resumeItemFixture},
		{"implement resume", []string{"resume"}, resumeWorktreeFixture},
	} {
		t.Run(entrypoint.name, func(t *testing.T) {
			markdownRoot, markdownBackend := entrypoint.build(t)
			markdown := runImplementationTransport(t, markdownRoot, markdownBackend, entrypoint.args...)
			cleanupResultDirectories(t, markdown)

			jsonRoot, jsonBackend := entrypoint.build(t)
			encoded := runImplementationTransport(t, jsonRoot, jsonBackend, append(append([]string{}, entrypoint.args...), "--format", "json")...)
			structured := implementationJSON(t, encoded)

			if structured.Status != "work_available" || structured.Item == nil || structured.Item.Number != 7 {
				t.Fatalf("json outcome = %#v", structured)
			}
			if structured.Packet == nil {
				t.Fatal("json outcome lacks the Execution Skill")
			}
			if normalizeExecution(markdown, markdownRoot) != normalizeExecution(structured.Packet.Instructions, jsonRoot) {
				t.Fatalf("default transport is not the complete Execution Skill:\n--- markdown ---\n%s\n--- instructions ---\n%s", markdown, structured.Packet.Instructions)
			}
			if strings.HasPrefix(strings.TrimSpace(markdown), "{") || strings.Contains(markdown, "\"instructions\"") || strings.Contains(markdown, "\nFacts: {") {
				t.Fatalf("default transport repeats the envelope or assembles facts:\n%s", markdown)
			}
			if !strings.Contains(markdown, "#7") || !strings.Contains(markdown, "widget") {
				t.Fatalf("Execution Skill does not name the selected identity:\n%s", markdown)
			}
			for _, claim := range []*implementationMemory{markdownBackend, jsonBackend} {
				if !claim.work[1].Claimed || claim.work[0].Claimed {
					t.Fatalf("formatting changed the underlying operation: %#v", claim.work)
				}
			}
		})
	}
}

// procedureMarkers are the applicable-procedure statements every Execution
// Skill must carry exactly once. They are deliberately independent of branch
// names, PR presence, artifact phase, and feedback contents.
var procedureMarkers = map[skilldist.ImplementProcedure]string{
	skilldist.InitialSubmission:   "This invocation starts the accepted change:",
	skilldist.ResumedSubmission:   "This invocation resumes existing work:",
	skilldist.FindingDrivenRework: "This invocation follows finding-driven Rework:",
}

// TestB2ProcedureSelectionIgnoresIncidentalEvidence materializes the B2 outline.
func TestB2ProcedureSelectionIgnoresIncidentalEvidence(t *testing.T) {
	cases := []struct {
		name       string
		item       workflow.ImplementationItem
		action     []string
		want       skilldist.ImplementProcedure
		wantText   []string
		absentText []string
	}{
		{
			name: "initial work",
			item: workflow.ImplementationItem{ID: "7", Branch: "rework", State: workflow.Ready,
				Feedback: []skilldist.ReviewComment{{Body: "looks like a rework finding", Author: "reviewer"}}},
			action:     []string{"next"},
			want:       skilldist.InitialSubmission,
			wantText:   []string{"read the accepted artifacts at their Artifact Baseline before changing code"},
			absentText: nil,
		},
		{
			name:   "resumed implementation",
			item:   workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true},
			action: []string{"resume", "--item", "7"},
			want:   skilldist.ResumedSubmission,
			wantText: []string{
				"inspect the branch, the preserved files, the applicable artifacts, and the visible feedback",
				"do not restart completed work",
			},
		},
		{
			name: "resumed draft progress",
			item: workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true,
				Submission: &workflow.Submission{ID: "11", Base: "main", Draft: true, Body: "draft body",
					Comments: []skilldist.ReviewComment{{Body: "draft comment", Author: "owner", Association: "OWNER"}}}},
			action: []string{"resume", "--item", "7"},
			want:   skilldist.ResumedSubmission,
			wantText: []string{
				"an attached draft Submission is preserved rather than replaced",
			},
		},
		{
			name: "finding-driven rework with fetched empty comments",
			item: workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Rework, Claimed: true,
				Submission: &workflow.Submission{ID: "11", Base: "main"}},
			action:     []string{"resume", "--item", "7"},
			want:       skilldist.FindingDrivenRework,
			wantText:   []string{"Map every finding to its resolution", "even when the supplied feedback is empty or still pending"},
			absentText: []string{"This invocation starts the accepted change:", "This invocation resumes existing work:"},
		},
		{
			name: "finding-driven rework whose feedback is not fetched yet",
			item: workflow.ImplementationItem{ID: "7", Branch: "widget", State: workflow.Rework, Claimed: true,
				Submission: &workflow.Submission{ID: "11", Base: "main"}},
			action:   []string{"resume", "--item", "7"},
			want:     skilldist.FindingDrivenRework,
			wantText: []string{"Map every finding to its resolution", "even when the supplied feedback is empty or still pending"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			backend := &implementationMemory{work: []workflow.ImplementationItem{testCase.item}}
			got := implementCLI(t, root, backend, testCase.action...)
			if got.Packet == nil || got.Packet.Facts.Implementation == nil {
				t.Fatalf("no Execution Skill: %#v", got)
			}
			facts := got.Packet.Facts.Implementation
			if facts.Procedure != testCase.want {
				t.Fatalf("procedure = %q, want %q", facts.Procedure, testCase.want)
			}
			rendered := got.Packet.Instructions
			for procedure, marker := range procedureMarkers {
				present := strings.Contains(rendered, marker)
				if procedure == testCase.want && !present {
					t.Errorf("Execution Skill lacks the applicable procedure %q", procedure)
				}
				if procedure != testCase.want && present {
					t.Errorf("Execution Skill retains the resolved alternative %q", procedure)
				}
			}
			for _, text := range testCase.wantText {
				if !strings.Contains(rendered, text) {
					t.Errorf("Execution Skill lacks %q", text)
				}
			}
			for _, text := range testCase.absentText {
				if strings.Contains(rendered, text) {
					t.Errorf("Execution Skill asserts an unobserved phase with %q", text)
				}
			}
		})
	}
}

// TestB3MetadataOnlyStartupBindsEveryAlreadyEstablishedReference materializes
// the B3 outline. Startup may only use metadata the invocation already
// established, so every bound command must be literal and usable as printed.
func TestB3MetadataOnlyStartupBindsEveryAlreadyEstablishedReference(t *testing.T) {
	quote := skilldist.ShellQuote
	for _, availability := range []string{"unavailable locally", "available locally with artifact marker history"} {
		freshFixture := func() (string, *implementationMemory) {
			root := selectionRepository(t)
			runGit(t, root, "remote", "rename", "origin", "upstream")
			runGit(t, root, "remote", "add", "origin", "https://github.com/other/widgets.git")
			root = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "--show-toplevel"))
			if availability == "available locally with artifact marker history" {
				prepareSlice(t, root, "slice-seven")
				runGit(t, root, "switch", "main")
				runGit(t, root, "worktree", "add", filepath.Join(root, ".worktrees", "slice-seven"), "slice-seven")
			}
			return root, &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "slice-seven", State: workflow.Ready}}}
		}
		t.Run(availability, func(t *testing.T) {
			root, backend := freshFixture()
			baseline, completion := strings.Repeat("a", 40), strings.Repeat("b", 40)
			flags := []string{"--artifact-baseline", baseline, "--artifact-completion", completion}
			got := implementCLI(t, root, backend, append([]string{"next", "--remote", "upstream"}, flags...)...)
			if got.Packet == nil || got.Status != "work_available" {
				t.Fatalf("start = %#v", got)
			}
			facts := got.Packet.Facts.Implementation
			rendered := got.Packet.Instructions
			worktree := filepath.Join(root, ".worktrees", "slice-seven")
			if facts.Repository != "acme/widgets" || facts.Remote != "upstream" || facts.Worktree != worktree || facts.ResultDirectory == "" {
				t.Fatalf("established identities = %#v", facts)
			}
			for _, bound := range []string{
				"Repository: acme/widgets on the selected remote `upstream`",
				"Work Item: #7",
				"Branch: `slice-seven`",
				"Worktree: `" + worktree + "`",
				"Private result location: `" + facts.ResultDirectory + "`",
				"Prepare: `git -C " + quote(root) + " fetch " + quote("upstream") + " " + quote("+refs/heads/slice-seven:refs/remotes/upstream/slice-seven") + "` then `git -C " + quote(root) + " worktree add -b " + quote("slice-seven") + " " + quote(worktree) + " " + quote("upstream/slice-seven") + "`",
				"Push: `git -C " + quote(worktree) + " push " + quote("upstream") + " " + quote("slice-seven") + "`",
				"Inspect: `skl implement inspect --repo " + quote(worktree) + " --remote " + quote("upstream") + " --item 7 --artifact-baseline " + baseline + " --artifact-completion " + completion + "`",
				"Resume: `skl implement resume --item 7 --remote " + quote("upstream") + " --artifact-baseline " + baseline + " --artifact-completion " + completion + "`",
				"`skl implement submit --repo " + quote(worktree) + " --remote " + quote("upstream") + " --item 7 --body " + quote(filepath.Join(facts.ResultDirectory, "submission.md")) + " --artifact-baseline " + baseline + " --artifact-completion " + completion + "`",
				"`skl implement needs-human --repo " + quote(worktree) + " --remote " + quote("upstream") + " --item 7 --reason <permitted-reason> --decision " + quote(filepath.Join(facts.ResultDirectory, "decision.md")) + " --artifact-baseline " + baseline + " --artifact-completion " + completion + "`",
				"gh api --paginate repos/acme/widgets/issues/7/comments",
				"are known pointers, not validated contents or ancestry",
			} {
				if !strings.Contains(rendered, bound) {
					t.Errorf("Execution Skill lacks the bound reference:\n%s", bound)
				}
			}
			if info, err := os.Stat(facts.ResultDirectory); err != nil || !info.IsDir() {
				t.Fatalf("private result location is not established: %v", err)
			}
			if availability == "unavailable locally" {
				if gitRefExists(root, "refs/heads/slice-seven") || gitRefExists(root, "refs/remotes/upstream/slice-seven") {
					t.Fatal("startup fetched or created the local branch")
				}
				if _, err := os.Stat(worktree); err == nil {
					t.Fatal("startup prepared the worktree")
				}
			}

			// No override is invented for an endpoint a normal invocation never supplies.
			plainRoot, plainBackend := freshFixture()
			plain := implementCLI(t, plainRoot, plainBackend, "next", "--remote", "upstream")
			if plain.Packet == nil {
				t.Fatalf("plain start = %#v", plain)
			}
			plainFacts := plain.Packet.Facts.Implementation
			if plainFacts.SuppliedArtifactBaseline != "" || plainFacts.SuppliedArtifactCompletion != "" {
				t.Fatalf("plain invocation carried endpoints: %#v", plainFacts)
			}
			worktree = filepath.Join(plainRoot, ".worktrees", "slice-seven")
			for _, unoverridden := range []string{
				"Inspect: `skl implement inspect --repo " + quote(worktree) + " --remote " + quote("upstream") + " --item 7`",
				"Resume: `skl implement resume --item 7 --remote " + quote("upstream") + "`",
				"`skl implement submit --repo " + quote(worktree) + " --remote " + quote("upstream") + " --item 7 --body " + quote(filepath.Join(plainFacts.ResultDirectory, "submission.md")) + "`",
				"`skl implement needs-human --repo " + quote(worktree) + " --remote " + quote("upstream") + " --item 7 --reason <permitted-reason> --decision " + quote(filepath.Join(plainFacts.ResultDirectory, "decision.md")) + "`",
			} {
				if !strings.Contains(plain.Packet.Instructions, unoverridden) {
					t.Errorf("plain invocation invented an override or lost a bound value:\n%s", unoverridden)
				}
			}
		})
	}
}
