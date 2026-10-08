package main

// CLI coverage for optional Run Metadata on phase handoffs. Expected
// observations are the literal values each worker supplied; the accepted rule
// is that they are recorded as supplied and never decide the workflow.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

const runTestRevision = "0123456789abcdef0123456789abcdef01234567"

// runMetadataApp separates standard error so a metadata notice never mixes
// with the JSON outcome.
func runMetadataApp(t *testing.T) (ledgerCLI, *bytes.Buffer) {
	t.Helper()
	factory := func(github.RepositoryID) (setup.Backend, error) {
		return nil, errors.New("forge unavailable in Run Metadata test")
	}
	var output, notices bytes.Buffer
	return ledgerCLI{app: newApp(factory, bytes.NewReader(nil), &output, &notices), out: &output}, &notices
}

// runMetadataSlice accepts one Slice ready for implementation and returns the
// ledger clone, a CLI with separate notices, and the source repository.
func runMetadataSlice(t *testing.T) (fixture *ledgerFixture, cli ledgerCLI, notices *bytes.Buffer, source, target string) {
	t.Helper()
	fixture = newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target = deliverySourceRepo(t)
	deliveryAcceptFixture(t, forge, source)
	cli, notices = runMetadataApp(t)
	return fixture, cli, notices, source, target
}

// withBuildInfo stands in for the submitting binary's build information.
func withBuildInfo(t *testing.T, info *debug.BuildInfo) {
	t.Helper()
	original := buildInfo
	buildInfo = func() (*debug.BuildInfo, bool) { return info, info != nil }
	t.Cleanup(func() { buildInfo = original })
}

// startRunImplementation claims and prepares implementation and commits one
// source change, returning the Claim and the head to submit.
func startRunImplementation(t *testing.T, cli ledgerCLI, source string) (claim, head string) {
	t.Helper()
	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("implement next = %#v, err=%v", started, err)
	}
	claim = started.Execution.Claim.Commit
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("implement prepare = %#v, err=%v", prepared, err)
	}
	writeFile(t, filepath.Join(prepared.Source.Worktree, "change.txt"), "change\n")
	runGit(t, prepared.Source.Worktree, "add", "change.txt")
	runGit(t, prepared.Source.Worktree, "commit", "-q", "-m", "change")
	return claim, deliveryTrimmed(t, prepared.Source.Worktree, "rev-parse", "HEAD")
}

func runFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "run.json")
	writeFile(t, path, contents)
	return path
}

// decodeRun turns a JSON literal into the generic value a reader exposes,
// keeping each number's exact digits so a rounded value never compares equal.
func decodeRun(t *testing.T, literal string) any {
	t.Helper()
	var value any
	if err := decodeExact(literal, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func decodeExact(literal string, value any) error {
	decoder := json.NewDecoder(strings.NewReader(literal))
	decoder.UseNumber()
	return decoder.Decode(value)
}

// recordedRun reads the committed report's Run Metadata back through the
// public document browser, as JSON.
func recordedRun(t *testing.T, cli ledgerCLI, phase string) any {
	t.Helper()
	text, err := cli.deliveryRun(t, "skl", "browse", "documents", "--project", "widgets", "--item", deliveryTestItem, "--format", "json")
	if err != nil {
		t.Fatalf("browse documents: %v\n%s", err, text)
	}
	var outcome struct {
		Documents struct {
			Documents []struct {
				Kind   string `json:"kind"`
				Report *struct {
					Schema int `json:"schema"`
					Run    any `json:"run"`
				} `json:"report"`
			} `json:"documents"`
		} `json:"documents"`
	}
	if err := decodeExact(text, &outcome); err != nil {
		t.Fatalf("decode documents %q: %v", text, err)
	}
	for _, document := range outcome.Documents.Documents {
		if document.Kind == phase+"-report" {
			if document.Report == nil {
				t.Fatalf("%s report has no readable metadata: %s", phase, text)
			}
			if document.Report.Schema != 2 {
				t.Fatalf("%s report schema = %d, want 2", phase, document.Report.Schema)
			}
			return document.Report.Run
		}
	}
	t.Fatalf("documents hold no %s report", phase)
	return nil
}

func TestRunMetadataIsRecordedWithBothPhaseReports(t *testing.T) {
	_, cli, notices, source, target := runMetadataSlice(t)
	withBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: runTestRevision}}})

	// Questionable values, a partial child list and an unfamiliar key are
	// observations, so they are kept as supplied.
	implementRun := `{"harness":{"name":"pi","version":"1.0.3"},"mode":"standard","elapsed_ms":420000,` +
		`"worker":{"provider":"example-provider","model":"example-model","reasoning":"high"},` +
		`"usage":{"coverage":"worker_only","input_tokens":-3,"output_tokens":"many","input_tokens_include_cache":true,"tool_calls":42},` +
		`"children":[{"role":"standards-review","elapsed_ms":1200}],"unfamiliar":[1.5,null]}`
	claim, head := startRunImplementation(t, cli, source)
	body := writeTemp(t, t, "# Implementation\n")
	submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", head, "--target", target, "--body", body, "--run-metadata", runFile(t, implementRun), "--format", "json")
	if err != nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit with Run Metadata = %#v, err=%v", submitted, err)
	}
	want := decodeRun(t, `{"skl":{"revision":"`+runTestRevision+`"},"harness":{"name":"pi","version":"1.0.3"},"mode":"standard","elapsed_ms":420000,`+
		`"worker":{"provider":"example-provider","model":"example-model","reasoning":"high"},`+
		`"usage":{"coverage":"worker_only","input_tokens":-3,"output_tokens":"many","input_tokens_include_cache":true,"tool_calls":42},`+
		`"children":[{"role":"standards-review","elapsed_ms":1200}],"unfamiliar":[1.5,null]}`)
	if got := recordedRun(t, cli, ledger.ImplementPhase); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded implement Run Metadata = %#v, want %#v", got, want)
	}
	if notices.Len() != 0 {
		t.Fatalf("usable Run Metadata produced a notice: %s", notices)
	}

	// Review consumes the report normally and records its own observations
	// with a reported Needs Human outcome.
	started, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("watchdog next after a schema-2 report = %#v, err=%v", started, err)
	}
	paused, err := cli.deliveryJSON(t, "skl", "watchdog", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", started.Execution.Claim.Commit,
		"--outcome", "needs-human", "--body", writeTemp(t, t, "# Review\n"), "--run-metadata", runFile(t, `{"worker":{"model":"example-reviewer"}}`), "--format", "json")
	if err != nil || paused.Status != ledger.NeedsHuman {
		t.Fatalf("watchdog needs-human with Run Metadata = %#v, err=%v", paused, err)
	}
	want = decodeRun(t, `{"skl":{"revision":"`+runTestRevision+`"},"worker":{"model":"example-reviewer"}}`)
	if got := recordedRun(t, cli, ledger.WatchdogPhase); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded watchdog Run Metadata = %#v, want %#v", got, want)
	}
}

func TestRunMetadataProblemsNeverBlockTheHandoff(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		run     func(t *testing.T) string
		info    *debug.BuildInfo
		want    string // recorded JSON, empty when nothing is recorded
		noticed bool
	}{
		{
			name: "no file and no build identity",
			run:  func(t *testing.T) string { return filepath.Join(t.TempDir(), "run.json") },
		},
		{
			name:    "undecodable file",
			run:     func(t *testing.T) string { return runFile(t, `{"usage": {"input_tokens": 1`) },
			info:    &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}},
			want:    `{"skl":{"version":"v1.2.3"}}`,
			noticed: true,
		},
		{
			name:    "several JSON values",
			run:     func(t *testing.T) string { return runFile(t, `{"mode":"team"} {"mode":"standard"}`) },
			noticed: true,
		},
		{
			name: "a YAML merge key stays an ordinary key",
			run:  func(t *testing.T) string { return runFile(t, `{"<<":"x","usage":{"<<":{"input_tokens":1}}}`) },
			want: `{"<<":"x","usage":{"<<":{"input_tokens":1}}}`,
		},
		{
			name: "numbers beyond float64 keep their exact value",
			run: func(t *testing.T) string {
				return runFile(t, `{"usage":{"input_tokens":9223372036854775809,"output_tokens":-9223372036854775809},"tiny":1e-400,"ratio":0.1}`)
			},
			want: `{"usage":{"input_tokens":9223372036854775809,"output_tokens":"-9223372036854775809"},"tiny":"1e-400","ratio":0.1}`,
		},
		{
			name: "a non-object keeps its value without identity",
			run:  func(t *testing.T) string { return runFile(t, `["unexpected"]`) },
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: runTestRevision}}},
			want: `["unexpected"]`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, cli, notices, source, target := runMetadataSlice(t)
			withBuildInfo(t, testCase.info)

			claim, head := startRunImplementation(t, cli, source)
			submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
				"--head", head, "--target", target, "--body", writeTemp(t, t, "# Implementation\n"), "--run-metadata", testCase.run(t), "--format", "json")
			if err != nil || submitted.Status != ledger.AwaitingReview {
				t.Fatalf("submit = %#v, err=%v", submitted, err)
			}
			got := recordedRun(t, cli, ledger.ImplementPhase)
			if testCase.want == "" {
				if got != nil {
					t.Fatalf("recorded Run Metadata = %#v, want none", got)
				}
			} else if want := decodeRun(t, testCase.want); !reflect.DeepEqual(got, want) {
				t.Fatalf("recorded Run Metadata = %#v, want %#v", got, want)
			}
			if noticed := strings.Contains(notices.String(), "Run Metadata left out"); noticed != testCase.noticed {
				t.Fatalf("notice = %q, want noticed=%t", notices, testCase.noticed)
			}
			if started, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json"); err != nil || started.Execution == nil {
				t.Fatalf("watchdog next after the handoff = %#v, err=%v", started, err)
			}
		})
	}
}

func TestRunMetadataLeavesAuthoritativeRefusalsInPlace(t *testing.T) {
	fixture, cli, _, source, target := runMetadataSlice(t)

	claim, head := startRunImplementation(t, cli, source)
	divergent := deliveryTrimmed(t, source, "commit-tree", deliveryTrimmed(t, source, "rev-parse", head+"^{tree}"), "-m", "divergent root")
	refused, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--head", divergent, "--target", target, "--body", writeTemp(t, t, "# Implementation\n"), "--run-metadata", runFile(t, `{"mode":"standard"}`), "--format", "json")
	if err != nil {
		t.Fatalf("submit a head off the branch: %v", err)
	}
	if refused.Status != "fix_required" || refused.Result != nil {
		t.Fatalf("head off the branch with Run Metadata = %#v, want the source refusal", refused)
	}
	if state := deliveryPersistedState(t, fixture.clone); state.Claim == nil || state.State == ledger.AwaitingReview {
		t.Fatalf("refused handoff changed the Work Item: %#v", state)
	}
}

func TestRunMetadataRetryReturnsTheRecordedReport(t *testing.T) {
	_, cli, _, source, target := runMetadataSlice(t)
	withBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}})

	claim, head := startRunImplementation(t, cli, source)
	body := writeTemp(t, t, "# Implementation\n")
	submit := func(run string) deliveryOutput {
		t.Helper()
		out, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
			"--head", head, "--target", target, "--body", body, "--run-metadata", runFile(t, run), "--format", "json")
		if err != nil || out.Result == nil {
			t.Fatalf("submit = %#v, err=%v", out, err)
		}
		return out
	}
	first := submit(`{"elapsed_ms":1000}`)
	withBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: "v2.0.0"}})
	retry := submit(`{"elapsed_ms":9999,"mode":"team"}`)
	if !retry.Result.AlreadyCompleted || retry.Result.Report != first.Result.Report {
		t.Fatalf("retry = %#v, want the recorded report %#v", retry.Result, first.Result.Report)
	}
	want := decodeRun(t, `{"skl":{"version":"v1.0.0"},"elapsed_ms":1000}`)
	if got := recordedRun(t, cli, ledger.ImplementPhase); !reflect.DeepEqual(got, want) {
		t.Fatalf("Run Metadata after retry = %#v, want the original %#v", got, want)
	}
}

func TestSchemaOneReportIsConsumedUnchanged(t *testing.T) {
	fixture, cli, _, source, target := runMetadataSlice(t)

	state := deliveryPersistedState(t, fixture.clone)
	state.State = ledger.AwaitingReview
	deliveryCommitState(t, fixture.clone, state)
	acceptance := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")
	worktree := filepath.Join(source, ".worktrees", deliveryTestBranch)
	runGit(t, source, "worktree", "add", "-q", "-b", deliveryTestBranch, worktree, target)
	writeFile(t, filepath.Join(worktree, "change.txt"), "change\n")
	runGit(t, worktree, "add", "change.txt")
	runGit(t, worktree, "commit", "-q", "-m", "change")
	head := deliveryTrimmed(t, worktree, "rev-parse", "HEAD")

	reportPath := deliveryItemDirectory() + "/implement-report.md"
	report := fmt.Sprintf("---\nschema: 1\noutcome: awaiting_review\nsource:\n  head: %s\n  target: %s\nledger:\n  claim:\n    commit: %s\n    path: %s\n  contract:\n    - commit: %s\n      path: %s\n---\n# Historical implementation\n",
		head, target, acceptance, deliveryItemStatePath(), acceptance, deliveryItemDirectory()+"/behavior.md")
	writeFile(t, filepath.Join(fixture.clone, filepath.FromSlash(reportPath)), report)
	runGit(t, fixture.clone, "add", "-A")
	runGit(t, fixture.clone, "commit", "-q", "-m", "fixture schema-1 report")

	started, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil || started.Execution.Implement == nil || started.Execution.Implement.Source.Head != head {
		t.Fatalf("watchdog next over a schema-1 report = %#v, err=%v", started, err)
	}
	if historical := runGitOutput(t, fixture.clone, "show", "HEAD:"+reportPath); historical != report {
		t.Fatalf("schema-1 report bytes changed:\n%s", historical)
	}
}
