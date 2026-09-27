package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

func completionNextCLI(t *testing.T, app *stageApp, output *bytes.Buffer, phase, repo string, options ...string) deliveryOutput {
	t.Helper()
	output.Reset()
	args := append([]string{"skl", phase, "next", "--repo", repo, "--format", "json"}, options...)
	if err := app.Run(args); err != nil {
		t.Fatalf("%s next: %v (%s)", phase, err, output.String())
	}
	var result deliveryOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("%s next response %q: %v", phase, output.String(), err)
	}
	return result
}

func completionDependencyFixture(t *testing.T, proposal, phase string) (*ledgerFixture, string, string) {
	t.Helper()
	fixture := newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	spec := dualSlice(proposal)
	spec.depends = map[string][]string{"feature": {"foundation"}}
	if accepted := newLedgerApp(t, newForgeServer(t)).accept(t, source, writeProposal(t, "", spec)); accepted.Status != "accepted" {
		t.Fatalf("accept dependency fixture: %s", mustJSON(t, accepted))
	}
	attachFixture(t, fixture.clone, proposal)
	if phase == ledger.WatchdogPhase {
		completionAwaitReview(t, fixture.clone, proposal, source, target)
	}
	return fixture, source, target
}

func completionAwaitReview(t *testing.T, clone, proposal, source, target string) {
	t.Helper()
	statusRecord(t, clone, proposal, "feature", func(state *ledger.SliceState) {
		state.State = ledger.AwaitingReview
	})
	statePath := filepath.ToSlash(filepath.Join("projects", "widgets", "proposals", proposal, "feature", "state.json"))
	contractPath := filepath.ToSlash(filepath.Join("projects", "widgets", "proposals", proposal, "feature", "intent.md"))
	basis := strings.TrimSpace(runGitOutput(t, clone, "rev-parse", "HEAD"))
	report, err := ledger.FormatReport(ledger.ImplementPhase, ledger.Report{
		Schema: 1, Outcome: ledger.AwaitingReview,
		Source: ledger.SourceRevisions{Head: deliveryTrimmed(t, source, "rev-parse", "HEAD"), Target: target},
		Ledger: ledger.ReportInputs{
			Claim:    ledger.Reference{Commit: basis, Path: statePath},
			Contract: []ledger.Reference{{Commit: basis, Path: contractPath}},
		},
	}, "# Prior implementation report\n")
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(clone, filepath.FromSlash(filepath.ToSlash(filepath.Join("projects", "widgets", "proposals", proposal, "feature", "implement-report.md"))))
	writeFile(t, reportPath, string(report))
	runGit(t, clone, "add", filepath.ToSlash(filepath.Join("projects", "widgets", "proposals", proposal, "feature", "implement-report.md")))
	runGit(t, clone, "commit", "-q", "-m", "prior feature implementation report")
}

func completionRecordState(t *testing.T, clone, project, proposal, slice string, edit func(*ledger.SliceState)) {
	t.Helper()
	path := filepath.ToSlash(filepath.Join("projects", project, "proposals", proposal, slice, "state.json"))
	var state ledger.SliceState
	if err := json.Unmarshal([]byte(readLedgerFile(t, clone, path)), &state); err != nil {
		t.Fatal(err)
	}
	edit(&state)
	contents, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(clone, filepath.FromSlash(path)), string(contents)+"\n")
	runGit(t, clone, "add", path)
	runGit(t, clone, "commit", "-q", "-m", "completion fixture state")
}

func completionAttachProject(t *testing.T, clone, project, repository, proposal, slice string, number int) {
	t.Helper()
	completionRecordState(t, clone, project, proposal, slice, func(state *ledger.SliceState) {
		state.State = ledger.ReadyForMerge
		state.Submission = &ledger.ForgeAttachment{Repository: repository, Number: number}
		state.Target = &ledger.IntegrationTarget{Repository: repository, Branch: "main"}
		state.Completion = nil
	})
}

func completionReadState(t *testing.T, clone, project, proposal, slice string) ledger.SliceState {
	t.Helper()
	path := filepath.ToSlash(filepath.Join("projects", project, "proposals", proposal, slice, "state.json"))
	raw := runGitOutput(t, clone, "show", "HEAD:"+path)
	var state ledger.SliceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return state
}

func completionAppWithFactory(factory backendFactory) (*stageApp, *bytes.Buffer) {
	var output bytes.Buffer
	return newApp(factory, bytes.NewReader(nil), &output, &output), &output
}

func TestNextCompletionRefreshEnablesImplementDependency(t *testing.T) {
	fixture, source, _ := completionDependencyFixture(t, "implement-completion", ledger.ImplementPhase)
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/widgets/pulls/21" {
			t.Errorf("unexpected forge request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
	})

	started := completionNextCLI(t, app, output, ledger.ImplementPhase, source)
	if started.Status != ledger.WorkAvailable || started.Execution == nil || started.Execution.Item != "implement-completion/feature" {
		t.Fatalf("next did not claim the dependent Slice after observing its blocker merge: %s", mustJSON(t, started))
	}
	blocker := completionReadState(t, fixture.clone, "widgets", "implement-completion", "foundation")
	if blocker.State != ledger.Merged || blocker.Completion == nil || blocker.Claim != nil {
		t.Fatalf("the observed blocker was not safely recorded Merged: %+v", blocker)
	}
}

func TestNextCompletionRefreshEnablesWatchdogDependency(t *testing.T) {
	fixture, source, _ := completionDependencyFixture(t, "watchdog-completion", ledger.WatchdogPhase)
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/widgets/pulls/21" {
			t.Errorf("unexpected forge request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
	})

	started := completionNextCLI(t, app, output, ledger.WatchdogPhase, source)
	if started.Status != ledger.WorkAvailable || started.Execution == nil || started.Execution.Item != "watchdog-completion/feature" || started.Execution.Implement == nil {
		t.Fatalf("watchdog next did not claim the review-ready dependent Slice: %s", mustJSON(t, started))
	}
	blocker := completionReadState(t, fixture.clone, "widgets", "watchdog-completion", "foundation")
	if blocker.State != ledger.Merged || blocker.Completion == nil {
		t.Fatalf("watchdog next did not persist the exact blocker completion: %+v", blocker)
	}
}

func TestNextCompletionWaitRetryObservesMerge(t *testing.T) {
	fixture, source, _ := completionDependencyFixture(t, "wait-completion", ledger.ImplementPhase)
	var calls atomic.Int32
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/widgets/pulls/21" {
			t.Errorf("unexpected forge request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) == 1 {
			_, _ = fmt.Fprint(w, fixturePull("open", "false", "accepted-head", "", "acme/widgets", "acme/widgets", "main"))
			return
		}
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
	})

	started := completionNextCLI(t, app, output, ledger.ImplementPhase, source, "--wait=5s", "--poll=5ms")
	if started.Status != ledger.WorkAvailable || started.Execution == nil || started.Execution.Item != "wait-completion/feature" || calls.Load() < 2 {
		t.Fatalf("wait retry did not see the blocker merge (requests %d): %s", calls.Load(), mustJSON(t, started))
	}
	if state := completionReadState(t, fixture.clone, "widgets", "wait-completion", "foundation"); state.State != ledger.Merged {
		t.Fatalf("wait retry claimed against a stale blocker: %+v", state)
	}
}

func TestNextCompletionRefreshIsProjectWideAndKeepsSelectionPriority(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	cli := newLedgerApp(t, newForgeServer(t))
	priority := singleSlice("a-priority")
	priority.slices[0].branch = "priority"
	if accepted := cli.accept(t, source, writeProposal(t, "", priority)); accepted.Status != "accepted" {
		t.Fatalf("accept priority Slice: %s", mustJSON(t, accepted))
	}
	dependent := dualSlice("z-dependent")
	dependent.depends = map[string][]string{"feature": {"foundation"}}
	if accepted := cli.accept(t, source, writeProposal(t, "", dependent)); accepted.Status != "accepted" {
		t.Fatalf("accept dependent proposal: %s", mustJSON(t, accepted))
	}
	unrelated := singleSlice("m-unrelated")
	unrelated.slices[0].branch = "unrelated"
	if accepted := cli.accept(t, source, writeProposal(t, "", unrelated)); accepted.Status != "accepted" {
		t.Fatalf("accept unrelated attached proposal: %s", mustJSON(t, accepted))
	}
	attachFixture(t, fixture.clone, "z-dependent")
	completionAttachProject(t, fixture.clone, "widgets", "acme/widgets", "m-unrelated", "foundation", 22)

	gadgetRoot := sourceRepository(t, "acme", "gadgets")
	if accepted := cli.accept(t, gadgetRoot, writeProposal(t, "", singleSlice("foreign-project"))); accepted.Status != "accepted" {
		t.Fatalf("accept foreign Project: %s", mustJSON(t, accepted))
	}
	completionAttachProject(t, fixture.clone, "gadgets", "acme/gadgets", "foreign-project", "foundation", 31)

	var mu sync.Mutex
	var paths []string
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/repos/acme/widgets/pulls/21":
			_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
		case "/repos/acme/widgets/pulls/22":
			body := fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "main")
			body = strings.Replace(body, `"number":21`, `"number":22`, 1)
			body = strings.Replace(body, `"ref":"foundation"`, `"ref":"unrelated"`, 1)
			_, _ = fmt.Fprint(w, body)
		case "/repos/acme/gadgets/pulls/31":
			_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/gadgets", "acme/gadgets", "main"))
		default:
			http.NotFound(w, r)
		}
	})
	started := completionNextCLI(t, app, output, ledger.ImplementPhase, source)
	if started.Status != ledger.WorkAvailable || started.Execution == nil || started.Execution.Item != "a-priority/foundation" {
		t.Fatalf("refresh changed the existing eligible-work priority: %s", mustJSON(t, started))
	}
	for _, item := range []struct{ proposal, want string }{
		{"z-dependent", ledger.Merged}, {"m-unrelated", ledger.Merged}, {"foreign-project", ledger.ReadyForMerge},
	} {
		project := "widgets"
		if item.proposal == "foreign-project" {
			project = "gadgets"
		}
		state := completionReadState(t, fixture.clone, project, item.proposal, "foundation")
		if state.State != item.want || (item.want == ledger.Merged) != (state.Completion != nil) {
			t.Errorf("%s state after next = %+v, want %s", item.proposal, state, item.want)
		}
	}
	mu.Lock()
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	if len(gotPaths) != 2 || !strings.Contains(strings.Join(gotPaths, " "), "/pulls/21") || !strings.Contains(strings.Join(gotPaths, " "), "/pulls/22") || strings.Contains(strings.Join(gotPaths, " "), "gadgets") {
		t.Fatalf("next did not refresh only every attached item in the selected Project: %v", gotPaths)
	}
}

type completionNoObserveBackend struct{}

func (completionNoObserveBackend) Validate(context.Context) (string, error) { return "main", nil }
func (completionNoObserveBackend) Prepare(context.Context) error            { return nil }

func TestNextCompletionForgeFailureDoesNotBlockIndependentLocalClaim(t *testing.T) {
	for _, mode := range []string{"construction", "capability", "read"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newLedgerFixture(t)
			source, _ := deliverySourceRepo(t)
			cli := newLedgerApp(t, newForgeServer(t))
			dependent := dualSlice("offline-dependent")
			dependent.depends = map[string][]string{"feature": {"foundation"}}
			if accepted := cli.accept(t, source, writeProposal(t, "", dependent)); accepted.Status != "accepted" {
				t.Fatalf("accept dependent proposal: %s", mustJSON(t, accepted))
			}
			local := singleSlice("offline-local")
			local.slices[0].branch = "offline"
			if accepted := cli.accept(t, source, writeProposal(t, "", local)); accepted.Status != "accepted" {
				t.Fatalf("accept local work: %s", mustJSON(t, accepted))
			}
			attachFixture(t, fixture.clone, "offline-dependent")
			var factoryCalls atomic.Int32
			factory := func(repository github.RepositoryID) (setup.Backend, error) {
				factoryCalls.Add(1)
				switch mode {
				case "construction":
					return nil, fmt.Errorf("forge construction unavailable")
				case "capability":
					return completionNoObserveBackend{}, nil
				default:
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						http.Error(w, "forge read unavailable", http.StatusServiceUnavailable)
					}))
					t.Cleanup(server.Close)
					backend := setup.NewGitHubBackend(server.URL, "token", server.Client())
					backend.BindRepository(repository)
					return backend, nil
				}
			}
			app, output := completionAppWithFactory(factory)
			started := completionNextCLI(t, app, output, ledger.ImplementPhase, source)
			if started.Status != ledger.WorkAvailable || started.Execution == nil || started.Execution.Item != "offline-local/foundation" {
				t.Fatalf("forge %s prevented unrelated local work: %s", mode, mustJSON(t, started))
			}
			if factoryCalls.Load() == 0 {
				t.Fatal("next did not attempt completion observation for the attached blocker")
			}
			blocker := completionReadState(t, fixture.clone, "widgets", "offline-dependent", "foundation")
			dependentState := completionReadState(t, fixture.clone, "widgets", "offline-dependent", "feature")
			if blocker.State != ledger.ReadyForMerge || blocker.Completion != nil || dependentState.Claim != nil {
				t.Fatalf("an unavailable forge invented completion or released the dependency: blocker=%+v dependent=%+v", blocker, dependentState)
			}
		})
	}
}

func TestNextCompletionUnsafeObservationDoesNotClaimDependency(t *testing.T) {
	for _, scenario := range []string{"identity", "state interleave", "report interleave"} {
		t.Run(scenario, func(t *testing.T) {
			fixture, source, _ := completionDependencyFixture(t, "unsafe-completion", ledger.ImplementPhase)
			if scenario == "report interleave" {
				reportPath := filepath.Join(fixture.clone, "projects", "widgets", "proposals", "unsafe-completion", "foundation", "implement-report.md")
				writeFile(t, reportPath, "initial evidence\n")
				runGit(t, fixture.clone, "add", reportPath)
				runGit(t, fixture.clone, "commit", "-q", "-m", "initial blocker report")
			}
			var calls atomic.Int32
			app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/widgets/pulls/21" {
					t.Errorf("unexpected forge request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				switch scenario {
				case "identity":
					_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "other/widgets", "acme/widgets", "main"))
				case "state interleave":
					statusRecord(t, fixture.clone, "unsafe-completion", "foundation", func(state *ledger.SliceState) {
						state.Submission.Number = 22
					})
					_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
				case "report interleave":
					reportPath := filepath.Join(fixture.clone, "projects", "widgets", "proposals", "unsafe-completion", "foundation", "implement-report.md")
					writeFile(t, reportPath, "changed evidence during observation\n")
					runGit(t, fixture.clone, "add", reportPath)
					runGit(t, fixture.clone, "commit", "-q", "-m", "interleaved blocker report")
					_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
				}
			})

			started := completionNextCLI(t, app, output, ledger.ImplementPhase, source)
			if calls.Load() != 1 {
				t.Fatalf("completion observation calls = %d, want one", calls.Load())
			}
			blocker := completionReadState(t, fixture.clone, "widgets", "unsafe-completion", "foundation")
			dependent := completionReadState(t, fixture.clone, "widgets", "unsafe-completion", "feature")
			if started.Execution != nil && started.Execution.Item == "unsafe-completion/feature" || dependent.Claim != nil {
				t.Fatalf("unsafe %s observation granted the dependent Claim: output=%s state=%+v", scenario, mustJSON(t, started), dependent)
			}
			if blocker.State != ledger.ReadyForMerge || blocker.Completion != nil {
				t.Fatalf("unsafe %s observation recorded a terminal fact: %+v", scenario, blocker)
			}
		})
	}
}

func TestNextCompletionStoppedDispatchSkipsForgeObservation(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, _ := deliverySourceRepo(t)
	cli := newLedgerApp(t, newForgeServer(t))
	dependent := dualSlice("stopped-dependent")
	dependent.depends = map[string][]string{"feature": {"foundation"}}
	if accepted := cli.accept(t, source, writeProposal(t, "", dependent)); accepted.Status != "accepted" {
		t.Fatalf("accept dependent proposal: %s", mustJSON(t, accepted))
	}
	var calls atomic.Int32
	app, output := completionStatusApp(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprint(w, fixturePull("closed", "true", "accepted-head", "merge-head", "acme/widgets", "acme/widgets", "main"))
	})
	first := completionNextCLI(t, app, output, ledger.ImplementPhase, source, "--dispatch")
	if first.Status != "dispatched" || first.Dispatch == nil || first.Dispatch.Item != "stopped-dependent/foundation" || calls.Load() != 0 {
		t.Fatalf("initial dispatch did not claim local work without a forge read: %s (calls %d)", mustJSON(t, first), calls.Load())
	}
	attachFixture(t, fixture.clone, "stopped-dependent")
	stopped := completionNextCLI(t, app, output, ledger.ImplementPhase, source, "--dispatch", "--after", first.Dispatch.Claim)
	if stopped.Status != "stopped" || stopped.Previous == nil || stopped.Previous.Ending != ledger.ClaimHeld || calls.Load() != 0 {
		t.Fatalf("stopped continuation observed completion or continued: %s (calls %d)", mustJSON(t, stopped), calls.Load())
	}
	blocker := completionReadState(t, fixture.clone, "widgets", "stopped-dependent", "foundation")
	dependentState := completionReadState(t, fixture.clone, "widgets", "stopped-dependent", "feature")
	if blocker.State != ledger.ReadyForMerge || blocker.Completion != nil || dependentState.Claim != nil {
		t.Fatalf("stopped continuation observed or claimed work: blocker=%+v dependent=%+v", blocker, dependentState)
	}
}
