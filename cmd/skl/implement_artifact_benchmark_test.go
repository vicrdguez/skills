package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type artifactInspectionFixture struct {
	root, baseline, completion, head string
	history                          int
	blobs                            map[string]bool
}

type artifactInspectionTrace struct {
	processes, contentLoads, markerScans int
}

func TestB14KeepArtifactInspectionCostIndependentOfIntermediateContent(t *testing.T) {
	short := newArtifactInspectionFixture(t, 0)
	long := newArtifactInspectionFixture(t, 1000)
	traceFile := filepath.Join(t.TempDir(), "git-trace.json")
	t.Setenv("GIT_TRACE2_EVENT", traceFile)

	shortResult, shortTrace := inspectArtifactFixture(t, short, traceFile)
	longResult, longTrace := inspectArtifactFixture(t, long, traceFile)
	if long.history-short.history < 1000 || !maps.Equal(short.blobs, long.blobs) {
		t.Fatalf("histories do not preserve identical endpoint artifacts: short=%d long=%d", short.history, long.history)
	}
	for name, result := range map[string]setup.ImplementationOutput{"short": shortResult, "long": longResult} {
		if result.Status != "inspected" || result.Ledger == nil || result.Ledger.Phase != "retired" || len(result.Ledger.Violations) != 0 {
			t.Fatalf("%s inspection = %#v", name, result)
		}
	}
	if shortResult.Ledger.Baseline != short.baseline || shortResult.Ledger.Completion != short.completion ||
		longResult.Ledger.Baseline != long.baseline || longResult.Ledger.Completion != long.completion {
		t.Fatalf("resolved endpoints differ from fixtures: short=%#v long=%#v", shortResult.Ledger, longResult.Ledger)
	}
	if shortTrace.contentLoads == 0 || longTrace.contentLoads > shortTrace.contentLoads+2 {
		t.Fatalf("artifact content loads grew with history: short=%+v long=%+v", shortTrace, longTrace)
	}
	if shortTrace.processes == 0 || longTrace.processes > shortTrace.processes+5 {
		t.Fatalf("Git process work grew with history: short=%+v long=%+v", shortTrace, longTrace)
	}
	if shortTrace.markerScans == 0 || longTrace.markerScans == 0 {
		t.Fatalf("trace missed metadata marker scan: short=%+v long=%+v", shortTrace, longTrace)
	}
	t.Logf("short history=%d processes=%d content_loads=%d marker_scans=%d", short.history, shortTrace.processes, shortTrace.contentLoads, shortTrace.markerScans)
	t.Logf("long history=%d processes=%d content_loads=%d marker_scans=%d", long.history, longTrace.processes, longTrace.contentLoads, longTrace.markerScans)
}

func BenchmarkArtifactEndpointInspection(b *testing.B) {
	fixtures := []artifactInspectionFixture{
		newArtifactInspectionFixture(b, 0),
		newArtifactInspectionFixture(b, 1000),
	}
	for _, fixture := range fixtures {
		b.Run(fmt.Sprintf("history=%d", fixture.history), func(b *testing.B) {
			traceFile := filepath.Join(b.TempDir(), "git-trace.json")
			b.Setenv("GIT_TRACE2_EVENT", traceFile)
			backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "ship-widget", State: workflow.AwaitingReview}}}
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
			command := []string{"skl", "implement", "inspect", "--item", "7", "--repo", fixture.root}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				output.Reset()
				if err := app.Run(command); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			trace := readArtifactInspectionTrace(b, traceFile, fixture)
			b.ReportMetric(float64(fixture.history), "history-commits")
			b.ReportMetric(float64(trace.processes)/float64(b.N), "git-processes/op")
			b.ReportMetric(float64(trace.contentLoads)/float64(b.N), "content-loads/op")
		})
	}
}

func inspectArtifactFixture(t testing.TB, fixture artifactInspectionFixture, traceFile string) (setup.ImplementationOutput, artifactInspectionTrace) {
	t.Helper()
	if err := os.WriteFile(traceFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &implementationMemory{work: []workflow.ImplementationItem{{ID: "7", Branch: "ship-widget", State: workflow.AwaitingReview}}}
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) { return backend, nil }, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "implement", "inspect", "--item", "7", "--repo", fixture.root}); err != nil {
		t.Fatalf("inspect: %v\n%s", err, &output)
	}
	var result setup.ImplementationOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode inspection: %v\n%s", err, &output)
	}
	return result, readArtifactInspectionTrace(t, traceFile, fixture)
}

func readArtifactInspectionTrace(t testing.TB, path string, fixture artifactInspectionFixture) artifactInspectionTrace {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	allowed := []string{fixture.baseline + ":.changes/ship-widget", fixture.completion + ":.changes/ship-widget"}
	var trace artifactInspectionTrace
	for _, line := range bytes.Split(contents, []byte("\n")) {
		var event struct {
			Event string   `json:"event"`
			Argv  []string `json:"argv"`
		}
		if len(line) == 0 || json.Unmarshal(line, &event) != nil || event.Event != "start" {
			continue
		}
		trace.processes++
		command := strings.Join(event.Argv, " ")
		if strings.Contains(command, " log --format=") {
			trace.markerScans++
		}
		if strings.Contains(command, " cat-file blob ") {
			trace.contentLoads++
			if len(event.Argv) == 0 || !fixture.blobs[event.Argv[len(event.Argv)-1]] {
				t.Fatalf("artifact blob read escaped endpoints: %s", command)
			}
			continue
		}
		if strings.Contains(command, " cat-file -e ") {
			if !strings.Contains(command, fixture.head+":.changes/ship-widget") {
				t.Fatalf("artifact head-presence read escaped fixed head: %s", command)
			}
			continue
		}
		if strings.Contains(command, " cat-file -t ") {
			if !strings.Contains(command, allowed[0]) && !strings.Contains(command, allowed[1]) {
				t.Fatalf("artifact tree read escaped endpoints: %s", command)
			}
		}
		if strings.Contains(command, " ls-tree ") &&
			(!slices.Contains(event.Argv, fixture.baseline) && !slices.Contains(event.Argv, fixture.completion) || !slices.Contains(event.Argv, ".changes/ship-widget")) {
			t.Fatalf("artifact tree read escaped endpoints: %s", command)
		}
	}
	return trace
}

func newArtifactInspectionFixture(t testing.TB, unrelated int) artifactInspectionFixture {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	git("remote", "add", "origin", "git@github.com:acme/widgets.git")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("widget\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "-m", "initial")
	git("switch", "-c", "ship-widget")
	ledger := filepath.Join(root, ".changes", "ship-widget")
	if err := os.MkdirAll(ledger, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"intent.md", "behavior.md"} {
		if err := os.WriteFile(filepath.Join(ledger, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".changes/ship-widget")
	git("commit", "-m", "[baseline] ship-widget")
	baseline := git("rev-parse", "HEAD")

	if unrelated > 0 {
		var stream strings.Builder
		parent := baseline
		for i := 0; i < unrelated; i++ {
			message, body := fmt.Sprintf("unrelated %d\n", i), fmt.Sprintf("%d\n", i)
			fmt.Fprintf(&stream, "commit refs/heads/b14-import\nmark :%d\ncommitter Test <test@example.com> %d +0000\ndata %d\n%sfrom %s\nM 100644 inline unrelated.txt\ndata %d\n%s\n", i+1, 1700000000+i, len(message), message, parent, len(body), body)
			parent = fmt.Sprintf(":%d", i+1)
		}
		command := exec.Command("git", "-C", root, "fast-import", "--quiet")
		command.Stdin = strings.NewReader(stream.String())
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fast-import: %v\n%s", err, output)
		}
		git("reset", "--hard", "b14-import")
		git("branch", "-D", "b14-import")
	}
	if err := os.WriteFile(filepath.Join(ledger, "behavior.md"), []byte("temporary artifact churn\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("commit", "-am", "temporary artifact churn")
	if err := os.WriteFile(filepath.Join(ledger, "behavior.md"), []byte("behavior.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("commit", "-am", "restore artifacts")
	git("commit", "--allow-empty", "-m", "[completion] ship-widget")
	completion := git("rev-parse", "HEAD")
	blobs := make(map[string]bool)
	for _, endpoint := range []string{baseline, completion} {
		for _, blob := range strings.Fields(git("ls-tree", "-r", "--format=%(objectname)", endpoint, "--", ".changes/ship-widget")) {
			blobs[blob] = true
		}
	}
	git("rm", "-r", ".changes/ship-widget")
	git("commit", "-m", "retire ship-widget")
	head := git("rev-parse", "HEAD")
	history, err := strconv.Atoi(git("rev-list", "--count", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	return artifactInspectionFixture{root: root, baseline: baseline, completion: completion, head: head, history: history, blobs: blobs}
}
