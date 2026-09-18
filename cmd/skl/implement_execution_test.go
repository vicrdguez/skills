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

// inspectionLedger writes one two-task ledger so the fixture can produce every
// observable progress state an inspection must distinguish.
func inspectionLedger(t *testing.T, root, intent string) {
	t.Helper()
	directory := filepath.Join(root, ".changes", "widget")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"intent.md": intent, "behavior.md": "# Behavior\n\nOne accepted scenario.\n"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func inspectionFixture(t *testing.T, stage string) (string, string, *implementationMemory) {
	t.Helper()
	root := proposalRepository(t)
	root = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "--show-toplevel"))
	runGit(t, root, "switch", "-c", "widget", "main")
	partial := "# Intent\n\n- [ ] First accepted task\n- [ ] Second accepted task\n"
	completed := "# Intent\n\n- [x] First accepted task\n- [x] Second accepted task\n"
	inspectionLedger(t, root, partial)
	runGit(t, root, "add", ".changes/widget")
	runGit(t, root, "commit", "-m", "[baseline] widget")
	baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	switch stage {
	case "baseline-only":
	case "provisional":
		inspectionLedger(t, root, "# Intent\n\n- [x] First accepted task\n- [ ] Second accepted task\n")
		if err := os.WriteFile(filepath.Join(root, "progress.txt"), []byte("preserved work\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", "progress.txt", ".changes/widget")
		runGit(t, root, "commit", "-m", "preserve partial work")
	case "completion-present":
		inspectionLedger(t, root, completed)
		runGit(t, root, "add", ".changes/widget")
		runGit(t, root, "commit", "-m", "[completion] widget")
	case "retired", "retired-rework":
		inspectionLedger(t, root, completed)
		runGit(t, root, "add", ".changes/widget")
		runGit(t, root, "commit", "-m", "[completion] widget")
		runGit(t, root, "rm", "-r", ".changes/widget")
		runGit(t, root, "commit", "-m", "retire widget")
	default:
		t.Fatalf("unknown stage %q", stage)
	}
	runGit(t, root, "switch", "main")
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "worktree", "add", worktree, "widget")
	state := workflow.Ready
	claimed := false
	if stage == "retired-rework" {
		state, claimed = workflow.Rework, true
	}
	backend := &implementationMemory{work: []workflow.ImplementationItem{
		{ID: "1", Branch: "other", State: workflow.Ready, CreatedAt: "2026-02-01T00:00:00Z"},
		{ID: "7", Branch: "widget", State: state, Claimed: claimed, CreatedAt: "2026-01-01T00:00:00Z"},
	}}
	return worktree, baseline, backend
}

// TestB4InspectionContinuesTheActualLedgerProgress materializes the B4 outline.
func TestB4InspectionContinuesTheActualLedgerProgress(t *testing.T) {
	cases := []struct {
		stage        string
		continuation []string
	}{
		{"baseline-only", []string{
			"Only the Artifact Baseline is resolved and no completed work is recorded",
			"create Completion only once every automated box is provably done",
		}},
		{"provisional", []string{
			"partial completion ticks and preserved work",
			"without restarting the tasks already marked done",
		}},
		{"completion-present", []string{
			"A valid Artifact Completion already exists and the ledger is still present",
			"Reuse that resolved Completion instead of creating a second one",
			"remove the entire `.changes/widget/` ledger",
		}},
		{"retired", []string{
			"The completed ledger is already retired",
			"Keep it absent, reuse the resolved endpoints",
			"without recreating it",
		}},
		{"retired-rework", []string{
			"already retired during finding-driven Rework",
			"resolve the supplied findings against the current PR comparison",
			"read the historical accepted artifacts at the resolved endpoints instead of recreating the ledger",
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.stage, func(t *testing.T) {
			worktree, baseline, backend := inspectionFixture(t, testCase.stage)
			got := implementCLI(t, worktree, backend, "inspect", "--item", "7")
			if got.Status != "inspected" || got.Packet == nil || got.Packet.Facts.Implementation == nil {
				t.Fatalf("inspection = %#v", got)
			}
			continuation := got.Packet.Instructions
			if got.Packet.Facts.Implementation.ArtifactBaseline != baseline || got.Packet.Facts.Implementation.Repository != "acme/widgets" || got.Packet.Facts.Implementation.Remote != "origin" || got.Packet.Facts.Implementation.WorkItem != 7 || got.Head != strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD")) {
				t.Fatalf("inspection lost an established identity: %#v", got.Packet.Facts.Implementation)
			}
			for _, required := range append(testCase.continuation,
				"Repository: acme/widgets on the selected remote `origin`",
				"Work Item: #7",
				"Branch: `widget`",
				"Artifact Baseline: `"+baseline+"`",
				"git -C '"+filepath.Join(filepath.Dir(filepath.Dir(worktree)), ".worktrees", "widget")+"' show '"+baseline+":.changes/widget/intent.md'",
				"git -C '"+filepath.Join(filepath.Dir(filepath.Dir(worktree)), ".worktrees", "widget")+"' show '"+baseline+":.changes/widget/behavior.md'",
			) {
				if !strings.Contains(continuation, required) {
					t.Errorf("continuation lacks %q:\n%s", required, continuation)
				}
			}
			for _, forbidden := range []string{"## Included Skill:", "## The scope is already decided", "--artifact-"} {
				if strings.Contains(continuation, forbidden) {
					t.Errorf("continuation returned a full skill or an invented override: %q", forbidden)
				}
			}
			if len(got.Packet.IncludedSkills) != 0 {
				t.Errorf("continuation bundled definitions: %v", got.Packet.IncludedSkills)
			}
			if backend.work[1].Claimed != (testCase.stage == "retired-rework") || backend.work[0].Claimed {
				t.Fatalf("inspection selected or claimed work: %#v", backend.work)
			}
		})
	}
}

// TestB5InspectionViolationsAndStaleIntegrityCannotImplyReadiness materializes
// the B5 scenario: an `inspected` status with violations is not readiness, and a
// repeated inspection re-reads the worktree instead of replaying success.
func TestB5InspectionViolationsAndStaleIntegrityCannotImplyReadiness(t *testing.T) {
	const prose = "Prose that the accepted contract keeps unchanged.\n"
	root := proposalRepository(t)
	root = strings.TrimSpace(runGitOutput(t, root, "rev-parse", "--show-toplevel"))
	runGit(t, root, "switch", "-c", "widget", "main")
	inspectionLedger(t, root, "# Intent\n\n"+prose+"\n- [ ] First accepted task\n- [ ] Second accepted task\n")
	runGit(t, root, "add", ".changes/widget")
	runGit(t, root, "commit", "-m", "[baseline] widget")
	baseline := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	inspectionLedger(t, root, "# Intent\n\nA rewritten contract line.\n\n- [x] First accepted task\n- [x] Second accepted task\n")
	runGit(t, root, "add", ".changes/widget")
	runGit(t, root, "commit", "-m", "[completion] widget")
	invalidCompletion := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "switch", "main")
	worktree := filepath.Join(root, ".worktrees", "widget")
	runGit(t, root, "worktree", "add", worktree, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{
		{ID: "1", Branch: "other", State: workflow.Ready, CreatedAt: "2026-02-01T00:00:00Z"},
		{ID: "7", Branch: "widget", State: workflow.Ready, Claimed: true, CreatedAt: "2026-01-01T00:00:00Z"},
	}}

	got := implementCLI(t, worktree, backend, "inspect", "--item", "7")
	if got.Status != "inspected" || got.Packet == nil || got.Packet.Facts.Implementation == nil || got.Packet.Facts.Implementation.Inspection == nil {
		t.Fatalf("inspection = %#v", got)
	}
	violations := got.Packet.Facts.Implementation.Inspection.Violations
	if len(violations) == 0 || got.Packet.Facts.Implementation.Inspection.Progress != "violations" {
		t.Fatalf("violations not observed: %#v", got.Packet.Facts.Implementation.Inspection)
	}
	continuation := got.Packet.Instructions
	for _, violation := range violations {
		if !strings.Contains(continuation, violation) {
			t.Errorf("continuation hides the reported violation %q", violation)
		}
	}
	for _, required := range []string{
		"Repair or stop before doing anything else",
		"none of them authorizes completion",
		"Refresh integrity with `skl implement inspect --repo '" + worktree + "' --remote 'origin' --item 7` before editing, before Audit, and before handoff",
		"never replays a cached success",
	} {
		if !strings.Contains(continuation, required) {
			t.Errorf("continuation lacks %q:\n%s", required, continuation)
		}
	}
	for _, forbidden := range []string{"## Observed progress", "create Completion only once", "Reuse that resolved Completion", "Keep it absent, reuse the resolved endpoints"} {
		if strings.Contains(continuation, forbidden) {
			t.Errorf("violations authorized completion with %q", forbidden)
		}
	}

	// Repair inside the worktree, then observe the current evidence again.
	inspectionLedger(t, worktree, "# Intent\n\n"+prose+"\n- [x] First accepted task\n- [x] Second accepted task\n")
	runGit(t, worktree, "add", ".changes/widget")
	runGit(t, worktree, "commit", "--amend", "-m", "[completion] widget")
	repaired := strings.TrimSpace(runGitOutput(t, worktree, "rev-parse", "HEAD"))
	if repaired == invalidCompletion {
		t.Fatal("repair did not move the observed head")
	}
	again := implementCLI(t, worktree, backend, "inspect", "--item", "7")
	facts := again.Packet.Facts.Implementation
	if again.Status != "inspected" || facts.Inspection.Progress != "completion-present" || len(facts.Inspection.Violations) != 0 || facts.ArtifactCompletion != repaired {
		t.Fatalf("repeated inspection replayed stale evidence: %#v", facts.Inspection)
	}
	if !strings.Contains(again.Packet.Instructions, "Reuse that resolved Completion instead of creating a second one") {
		t.Fatalf("repaired continuation is not the applicable one:\n%s", again.Packet.Instructions)
	}
	markers := 0
	for _, line := range strings.Split(runGitOutput(t, worktree, "log", "--format=%s", "HEAD"), "\n") {
		if line == "[completion] widget" {
			markers++
		}
	}
	if markers != 1 || baseline == repaired {
		t.Fatalf("repair duplicated Completion or moved the Baseline: markers=%d baseline=%s repaired=%s", markers, baseline, repaired)
	}
	if !backend.work[1].Claimed || backend.work[0].Claimed || len(backend.work) != 2 {
		t.Fatalf("repair released the Claim or restarted selection: %#v", backend.work)
	}
}

// TestB6TheCompleteBundlePreservesTheCurrentImplementationContract materializes
// the B6 scenario against the complete rendered Execution Skill.
func TestB6TheCompleteBundlePreservesTheCurrentImplementationContract(t *testing.T) {
	root := proposalRepository(t)
	baseline := prepareSlice(t, root, "widget")
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "widget", State: workflow.Ready}}}
	got := implementCLI(t, root, backend, "next", "--artifact-baseline", strings.Repeat("d", 40))
	if got.Packet == nil {
		t.Fatalf("start = %#v", got)
	}
	instructions := got.Packet.Instructions
	facts := got.Packet.Facts.Implementation
	for _, included := range []string{"tdd", "audit", "design", "domain"} {
		if count := strings.Count(instructions, "\n\n## Included Skill: "+included+"\n\n"); count != 1 {
			t.Errorf("bundled %s definition occurs %d times", included, count)
		}
	}
	// The accepted change supplies the fixed point and the artifact pointers, so
	// the bundle replaces independent-mode discovery with the resolved ones or
	// with the exact command that establishes them.
	for _, resolved := range []string{
		"merge-base with `main` and the parent of this change's first commit",
		"run `" + facts.InspectCommand + "` to resolve the Artifact Baseline and Completion",
		"`--artifact-baseline " + facts.SuppliedArtifactBaseline + "`",
		"The artifacts are the accepted `.changes/widget/` files of this Work Item",
	} {
		if !strings.Contains(instructions, resolved) {
			t.Errorf("bundle lacks the resolved reference %q", resolved)
		}
	}
	for _, superseded := range []string{
		"If no PR comparison or fixed point can be resolved, ask for one",
		"If nothing is found, ask the user where the artifacts are",
		"Before writing any test, write down the seams under test and confirm them with the user",
		"`skl implement inspect --remote <name> --item <number>` when invoked independently",
	} {
		if strings.Contains(instructions, superseded) {
			t.Errorf("bundle retains the resolved alternative %q", superseded)
		}
	}
	// The construction loop, the review obligations, and the judgment branches stay.
	for _, required := range []string{
		"Red before green", "One slice at a time", "One Gherkin `Scenario Outline` with its `Examples` table is one cycle", "pinned in the accepted artifacts",
		"two axes", "exactly once per invocation", "endpoint inspection", "complete final implementation",
		"`HARD`", "`JUDGEMENT`", "Do **not** merge or rerank findings", "disposition",
		"integration with `main`, conflict resolution, and merge belong to the human Merge Authority",
		"## When only a human can decide", "The scope is already decided",
		"does not require a new design exercise", "outside the accepted change",
	} {
		if !strings.Contains(instructions, required) {
			t.Errorf("bundle dropped the retained obligation %q", required)
		}
	}
	// ADR 0005's redesign and blanket target synchronization are out of scope.
	for _, outOfScope := range []string{"target-snapshot", "Target Snapshot", "Synchronization Rework", "git merge ", "git rebase", "contract conformance", "instead of prescribing test order"} {
		if strings.Contains(instructions, outOfScope) {
			t.Errorf("bundle introduced out-of-scope behavior %q", outOfScope)
		}
	}
	_ = baseline
}

// capabilityRecipes are the established execution capabilities and the one
// Audit recipe each of them selects. A harness name is never enough.
var capabilityRecipes = []struct {
	capability string
	recipe     string
	others     []string
}{
	{"claude-agents", "a single message with two `Agent` tool calls", []string{"a single asynchronous `subagent` call with a `workflowScript` using `runs.all`", "run the two axes sequentially, Standards first"}},
	{"pi-subagents", "a single asynchronous `subagent` call with a `workflowScript` using `runs.all`", []string{"a single message with two `Agent` tool calls", "run the two axes sequentially, Standards first"}},
	{"sequential", "run the two axes sequentially, Standards first", []string{"a single message with two `Agent` tool calls", "a single asynchronous `subagent` call with a `workflowScript` using `runs.all`"}},
	{"", "Choose among the supported recipes at runtime", nil},
}

// TestB7AuditRecipesFollowCapabilitiesNotHarnessNames materializes the B7 outline.
func TestB7AuditRecipesFollowCapabilitiesRatherThanHarnessNames(t *testing.T) {
	for _, testCase := range capabilityRecipes {
		name := testCase.capability
		if name == "" {
			name = "capability unknown"
		}
		t.Run(name, func(t *testing.T) {
			root := proposalRepository(t)
			prepareSlice(t, root, "widget")
			backend := &implementationMemory{work: []workflow.ImplementationItem{
				{ID: "1", Branch: "other", State: workflow.Ready, CreatedAt: "2026-02-01T00:00:00Z"},
				{ID: "7", Branch: "widget", State: workflow.Ready, CreatedAt: "2026-01-01T00:00:00Z"},
			}}
			args := []string{"next"}
			if testCase.capability != "" {
				args = append(args, "--capability", testCase.capability)
			}
			got := implementCLI(t, root, backend, args...)
			if got.Status != "work_available" || got.Packet == nil {
				t.Fatalf("start = %#v", got)
			}
			if string(got.Packet.Facts.Implementation.Capability) != testCase.capability {
				t.Fatalf("capability = %q, want %q", got.Packet.Facts.Implementation.Capability, testCase.capability)
			}
			instructions := got.Packet.Instructions
			if !strings.Contains(instructions, testCase.recipe) {
				t.Errorf("bundle lacks the applicable Audit recipe %q", testCase.recipe)
			}
			for _, other := range testCase.others {
				if strings.Contains(instructions, other) {
					t.Errorf("bundle retains a recipe this capability does not support: %q", other)
				}
			}
			for _, axis := range []string{"- **Standards** — does the code conform", "- **Artifacts** — does the code faithfully implement", "Do **not** merge or rerank findings"} {
				if !strings.Contains(instructions, axis) {
					t.Errorf("capability changed the review contract: %q missing", axis)
				}
			}
			// Capability knowledge changes instructions only, never the workflow.
			if !backend.work[1].Claimed || backend.work[0].Claimed || len(backend.work) != 2 {
				t.Fatalf("capability changed eligibility or the Claim: %#v", backend.work)
			}
		})
	}
}
