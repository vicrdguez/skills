package main

// Public-CLI coverage of current-view pull request presentation
// (publish-current-pulls B1-B5, A1-A4). Real ledger and source Git
// repositories drive the ordinary handoffs offline; `skl ledger present` then
// supplies current evidence and guidance and presents the latest result
// through the production GitHub adapter against a controlled pull request
// server. Expected outcomes follow the accepted behavior, not the engine.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// pullServer is a controlled GitHub pull request surface whose head SHA is
// the local bare source remote's branch tip, so readiness is judged against
// the actually published source.
type pullServer struct {
	t      *testing.T
	server *httptest.Server
	bare   string

	mu        sync.Mutex
	pulls     map[int]map[string]any
	requests  []string
	bodies    []string
	readiness []string
}

func newPullServer(t *testing.T, bare string) *pullServer {
	t.Helper()
	p := &pullServer{t: t, bare: bare, pulls: map[int]map[string]any{}}
	p.server = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.server.Close)
	return p
}

func (p *pullServer) record(number int) map[string]any {
	pull := p.pulls[number]
	branch := pull["branch"].(string)
	sha := strings.TrimSpace(runGitOutput(p.t, p.bare, "rev-parse", "refs/heads/"+branch))
	return map[string]any{
		"number": number, "node_id": fmt.Sprintf("PR_%d", number), "state": "open",
		"body": pull["body"], "draft": pull["draft"], "title": pull["title"],
		"head": map[string]any{"ref": branch, "sha": sha, "repo": map[string]string{"full_name": "acme/widgets"}},
		"base": map[string]string{"ref": "main"},
	}
}

func (p *pullServer) serve(w http.ResponseWriter, r *http.Request) {
	raw := new(bytes.Buffer)
	_, _ = raw.ReadFrom(r.Body)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, r.Method+" "+r.URL.Path)
	if raw.Len() > 0 {
		p.bodies = append(p.bodies, raw.String())
	}
	var payload map[string]any
	_ = json.Unmarshal(raw.Bytes(), &payload)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widgets/pulls":
		records := []map[string]any{}
		for number, pull := range p.pulls {
			if r.URL.Query().Get("head") == "acme:"+pull["branch"].(string) {
				records = append(records, p.record(number))
			}
		}
		_ = json.NewEncoder(w).Encode(records)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/widgets/pulls":
		number := 21 + len(p.pulls)
		p.pulls[number] = map[string]any{"branch": payload["head"], "body": payload["body"], "draft": payload["draft"], "title": payload["title"]}
		_ = json.NewEncoder(w).Encode(p.record(number))
	case strings.HasPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"):
		var number int
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"), "%d", &number)
		if p.pulls[number] == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPatch {
			p.pulls[number]["body"] = payload["body"]
		}
		_ = json.NewEncoder(w).Encode(p.record(number))
	case r.URL.Path == "/graphql":
		query, _ := payload["query"].(string)
		ready := strings.Contains(query, "markPullRequestReadyForReview")
		p.readiness = append(p.readiness, map[bool]string{true: "ready", false: "draft"}[ready])
		for number := range p.pulls {
			if variables, _ := payload["variables"].(map[string]any); variables["id"] == fmt.Sprintf("PR_%d", number) {
				p.pulls[number]["draft"] = !ready
			}
		}
		_, _ = fmt.Fprint(w, `{"data":{}}`)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

// routeSourcePushes sends the GitHub-shaped source remote's Git transport to
// a local bare repository, so ordinary non-force pushes are real while the
// remote keeps its GitHub identity.
func routeSourcePushes(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "widgets.git")
	runGit(t, t.TempDir(), "init", "-q", "--bare", "-b", "main", bare)
	shim := filepath.Join(t.TempDir(), "ssh")
	writeFile(t, shim, "#!/bin/sh\nfor last; do :; done\nexec sh -c \"$(printf '%s' \"$last\" | sed -e \"s|'acme/widgets.git'|'"+bare+"'|\" -e 's|^git-|git |')\"\n")
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSH_COMMAND", shim)
	return bare
}

func presentForgeApp(t *testing.T, pulls *pullServer) ledgerCLI {
	t.Helper()
	factory := func(repository github.RepositoryID) (setup.Backend, error) {
		backend := setup.NewGitHubBackend(pulls.server.URL, "secret", pulls.server.Client())
		backend.BindRepository(repository)
		return backend, nil
	}
	var output bytes.Buffer
	return ledgerCLI{app: newApp(factory, bytes.NewReader(nil), &output, &output), out: &output}
}

// presentHandoff runs one claimed phase through its public commands offline.
func presentHandoff(t *testing.T, cli ledgerCLI, source, phase string, arguments ...string) (deliveryOutput, string) {
	t.Helper()
	started, err := cli.deliveryJSON(t, "skl", phase, "next", "--repo", source, "--format", "json")
	if err != nil || started.Execution == nil {
		t.Fatalf("%s next = %#v, %v", phase, started, err)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := cli.deliveryJSON(t, "skl", phase, "prepare", "--repo", source, "--item", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil || prepared.Source == nil {
		t.Fatalf("%s prepare = %#v, %v", phase, prepared, err)
	}
	head := prepared.Source.Head
	if phase == ledger.ImplementPhase {
		name := fmt.Sprintf("change-%d.txt", len(runGitOutput(t, prepared.Source.Worktree, "log", "--oneline")))
		writeFile(t, filepath.Join(prepared.Source.Worktree, name), name+"\n")
		runGit(t, prepared.Source.Worktree, "add", name)
		runGit(t, prepared.Source.Worktree, "commit", "-q", "-m", name)
		head = deliveryTrimmed(t, prepared.Source.Worktree, "rev-parse", "HEAD")
		arguments = append(arguments, "--head", head)
	}
	body := filepath.Join(t.TempDir(), phase+"-report.md")
	writeFile(t, body, "PRIVATE "+phase+" report at "+head+"\n")
	public := filepath.Join(t.TempDir(), "public.md")
	writeFile(t, public, "missed public "+phase+" update\n")
	submitted, err := cli.deliveryJSON(t, append([]string{"skl", phase, "submit", "--repo", source, "--item", deliveryTestItem, "--claim", claim,
		"--body", body, "--public-body", public, "--format", "json"}, arguments...)...)
	if err != nil || submitted.Result == nil {
		t.Fatalf("%s submit = %#v, %v", phase, submitted, err)
	}
	if submitted.Result.Publication == nil || submitted.Result.Publication.Status == ledger.PullPresented || !strings.Contains(submitted.Present, "skl ledger present") {
		t.Fatalf("offline %s handoff = %#v, want pending presentation with a current-view continuation", phase, submitted)
	}
	return submitted, head
}

// TestLedgerPresentReauthorsTheCurrentResultAfterMissedPhases covers the B5
// reauthoring scenario and B1/B3 through the public CLI: implementation,
// rejection, rework, and approval completed while every public update was
// missed. Explicit presentation offers the current result's evidence and
// guidance rather than the missed updates, then presents freshly authored
// prose for only that result; the private reports never reach the forge.
func TestLedgerPresentReauthorsTheCurrentResultAfterMissedPhases(t *testing.T) {
	fixture := newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	offline := deliveryNoForgeApp(t)

	presentHandoff(t, offline, source, ledger.ImplementPhase, "--target", target)
	presentHandoff(t, offline, source, ledger.WatchdogPhase, "--outcome", "rework")
	_, final := presentHandoff(t, offline, source, ledger.ImplementPhase, "--target", target)
	passed, _ := presentHandoff(t, offline, source, ledger.WatchdogPhase, "--outcome", "pass")
	before := deliveryPersistedState(t, fixture.clone)
	ledgerHead := deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD")

	// Without prose, both transports carry the same current evidence and bound
	// continuation, and no effect happens.
	guided := offline.ledgerJSON(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem, "--format", "json")
	if guided.Status != "prose_required" || guided.Guidance == nil {
		t.Fatalf("present without prose = %#v", guided)
	}
	result := guided.Guidance.Result
	if result.Phase != ledger.WatchdogPhase || result.Outcome != "pass" || result.Round != 2 || !result.Approved || result.Source.Reviewed != final || result.Report != passed.Result.Report {
		t.Fatalf("guided result = %#v, want the approved second review of %s", result, final)
	}
	if len(guided.Guidance.Evidence) != 2 || !strings.Contains(guided.Guidance.Evidence[0], passed.Result.Report.Commit) || !strings.Contains(guided.Guidance.Evidence[1], "implement-report.md") {
		t.Fatalf("evidence = %v, want the current review and its consumed implementation", guided.Guidance.Evidence)
	}
	if !strings.Contains(guided.Guidance.Continue, "--item '"+deliveryTestItem+"'") || !strings.Contains(guided.Guidance.Authoring, "reference/pull-presentation.md watchdog") {
		t.Fatalf("guidance commands = %#v", guided.Guidance)
	}
	markdown, err := offline.deliveryRun(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range append([]string{"Status: prose_required", "Current result: watchdog pass, review round 2", guided.Guidance.Authoring, guided.Guidance.Continue, "reviewed " + final}, guided.Guidance.Evidence...) {
		if !strings.Contains(markdown, want) {
			t.Errorf("Markdown guidance lacks %q:\n%s", want, markdown)
		}
	}
	for _, missed := range []string{"missed public", "PRIVATE"} {
		if strings.Contains(markdown, missed) {
			t.Errorf("guidance replays or exports %q:\n%s", missed, markdown)
		}
	}
	if deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD") != ledgerHead {
		t.Fatal("guidance changed the ledger")
	}

	// The bound evidence retrieval and authoring resource work as printed.
	shown := offline.ledgerJSON(t, "skl", "ledger", "show", "--commit", result.Report.Commit, "--path", result.Report.Path, "--format", "json")
	if shown.Document == nil || !strings.Contains(shown.Document.Contents, "PRIVATE watchdog report") {
		t.Fatalf("evidence retrieval = %#v", shown.Document)
	}
	authoring, err := offline.deliveryRun(t, "skl", "skill", "--resource", "reference/pull-presentation.md", "watchdog")
	if err != nil || !strings.Contains(authoring, "`pass`") || !strings.Contains(authoring, "not replayed") {
		t.Fatalf("authoring guidance = %q, %v", authoring, err)
	}

	prose := filepath.Join(t.TempDir(), "fresh.md")
	writeFile(t, prose, "# Foundation ready for human merge\n\nIndependent review approved the reviewed revision.\n")

	// While source cannot be published, both transports report the concrete
	// limitation with the same guidance, and nothing is presented or recorded.
	unreachable := presentForgeApp(t, newPullServer(t, ""))
	pending := unreachable.ledgerJSON(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem, "--public-body", prose, "--format", "json")
	if pending.Status != ledger.IssuePending || pending.Presentation == nil || !strings.Contains(pending.Presentation.Publication.Detail, "source push unavailable") || pending.Guidance == nil {
		t.Fatalf("present without source publication = %s", mustJSON(t, pending))
	}
	pendingMarkdown, err := unreachable.deliveryRun(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem, "--public-body", prose)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Status: pending", "Public presentation: pending — " + pending.Presentation.Publication.Detail, "Present with fresh prose: `" + pending.Guidance.Continue + "`"} {
		if !strings.Contains(pendingMarkdown, want) {
			t.Errorf("Markdown limitation lacks %q:\n%s", want, pendingMarkdown)
		}
	}
	if deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD") != ledgerHead {
		t.Fatal("a failed presentation changed the ledger")
	}

	// Fresh prose presents only the current approved result: the reviewed
	// revision is pushed normally, created as draft, then marked ready.
	bare := routeSourcePushes(t)
	pulls := newPullServer(t, bare)
	online := presentForgeApp(t, pulls)
	presented := online.ledgerJSON(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem, "--public-body", prose, "--format", "json")
	if presented.Status != ledger.PullPresented || presented.Presentation == nil || presented.Guidance != nil {
		t.Fatalf("present = %s", mustJSON(t, presented))
	}
	if remote := deliveryTrimmed(t, bare, "rev-parse", "refs/heads/"+deliveryTestBranch); remote != final {
		t.Fatalf("published source = %s, want the reviewed %s", remote, final)
	}
	pulls.mu.Lock()
	pull, readiness, bodies := pulls.pulls[21], append([]string(nil), pulls.readiness...), append([]string(nil), pulls.bodies...)
	pulls.mu.Unlock()
	if pull == nil || pull["body"] != readFileString(t, prose) || pull["draft"] != false {
		t.Fatalf("presented pull = %#v, want the fresh prose presented ready", pull)
	}
	if len(readiness) != 1 || readiness[0] != "ready" {
		t.Fatalf("readiness mutations = %v, want one ready presentation", readiness)
	}
	for _, body := range bodies {
		if strings.Contains(body, "PRIVATE") || strings.Contains(body, "missed public") {
			t.Fatalf("the forge received private or missed content: %s", body)
		}
	}

	again, err := online.deliveryRun(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem, "--public-body", prose)
	if err != nil || !strings.Contains(again, "Status: presented") || !strings.Contains(again, "Public presentation: presented — pull request #21") || !strings.Contains(again, "Submission: acme/widgets#21") || strings.Contains(again, "Present with fresh prose") {
		t.Fatalf("Markdown presentation = %q, %v", again, err)
	}

	after := deliveryPersistedState(t, fixture.clone)
	if after.Submission == nil || after.Submission.Number != 21 || after.State != before.State || after.Claim != nil {
		t.Fatalf("state after presentation = %#v, want only the association added to %#v", after, before)
	}
	if report, _ := deliveryCommittedReport(t, offline, ledger.Reference{Commit: deliveryTrimmed(t, fixture.clone, "rev-parse", "HEAD"), Path: result.Report.Path}, ledger.WatchdogPhase); report.Round != 2 {
		t.Fatalf("review count = %d, want 2", report.Round)
	}
	raw := runGitOutput(t, fixture.clone, "show", "HEAD:"+deliveryItemStatePath())
	for _, key := range []string{"active_delivery", `"source"`, `"pull"`} {
		if strings.Contains(raw, key) {
			t.Fatalf("state.json persists publication coordination %s: %s", key, raw)
		}
	}
}
