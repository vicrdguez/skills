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
	// mergeRefusal, when set, is the reason the forge refuses every merge.
	mergeRefusal string
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
	merged, _ := pull["merge_sha"].(string)
	state, mergedAt := "open", any(nil)
	if merged != "" {
		state, mergedAt = "closed", "2026-01-01T00:00:00Z"
	}
	return map[string]any{
		"number": number, "node_id": fmt.Sprintf("PR_%d", number), "state": state,
		"merged": merged != "", "merged_at": mergedAt, "merge_commit_sha": merged,
		"body": pull["body"], "draft": pull["draft"], "title": pull["title"],
		"head": map[string]any{"ref": branch, "sha": sha, "repo": map[string]string{"full_name": "acme/widgets"}},
		"base": map[string]any{"ref": pull["base"], "repo": map[string]string{"full_name": "acme/widgets"}},
	}
}

// squash lands the pull request's head tree as one commit on its base, as
// GitHub's squash merge does, and returns that commit.
func (p *pullServer) squash(number int) string {
	pull := p.pulls[number]
	base := "refs/heads/" + pull["base"].(string)
	parent := strings.TrimSpace(runGitOutput(p.t, p.bare, "rev-parse", base))
	tree := strings.TrimSpace(runGitOutput(p.t, p.bare, "rev-parse", "refs/heads/"+pull["branch"].(string)+"^{tree}"))
	commit := strings.TrimSpace(runGitOutput(p.t, p.bare, "-c", "user.name=GitHub", "-c", "user.email=noreply@github.com", "commit-tree", tree, "-p", parent, "-m", fmt.Sprintf("%v (#%d)", pull["title"], number)))
	runGit(p.t, p.bare, "update-ref", base, commit, parent)
	pull["merge_sha"] = commit
	return commit
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
		p.pulls[number] = map[string]any{"branch": payload["head"], "base": payload["base"], "body": payload["body"], "draft": payload["draft"], "title": payload["title"]}
		_ = json.NewEncoder(w).Encode(p.record(number))
	case strings.HasPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"):
		var number int
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/repos/acme/widgets/pulls/"), "%d", &number)
		if p.pulls[number] == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge") {
			switch record := p.record(number); {
			case p.mergeRefusal != "":
				w.WriteHeader(http.StatusMethodNotAllowed)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": p.mergeRefusal})
			case payload["merge_method"] != "squash" || payload["sha"] != record["head"].(map[string]any)["sha"] || record["draft"] == true || record["state"] != "open":
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "Head branch was modified"})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{"sha": p.squash(number), "merged": true})
			}
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

// presentEvidence runs every evidence command a presentation listed, as
// printed, and returns the retrieved documents' contents.
func presentEvidence(t *testing.T, cli ledgerCLI, guidance *presentationGuidance) string {
	t.Helper()
	var contents []string
	for _, command := range guidance.Evidence {
		shown := cli.ledgerJSON(t, append(shellWords(t, command), "--format", "json")...)
		if shown.Document == nil {
			t.Fatalf("%s retrieved no document", command)
		}
		contents = append(contents, shown.Document.Contents)
	}
	return strings.Join(contents, "\n")
}

// presentHandoff runs one claimed phase through its public commands offline
// and returns its rendering, its handoff and the handed-off source head.
func presentHandoff(t *testing.T, cli ledgerCLI, source, phase string, arguments ...string) (deliveryOutput, deliveryOutput, string) {
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
	return started, submitted, head
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
	_, rejected, _ := presentHandoff(t, offline, source, ledger.WatchdogPhase, "--outcome", "rework")
	_, reworked, final := presentHandoff(t, offline, source, ledger.ImplementPhase, "--target", target)
	_, passed, _ := presentHandoff(t, offline, source, ledger.WatchdogPhase, "--outcome", "pass")
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
	if result.ReviewCount != 2 || len(guided.Guidance.Evidence) == 0 || !strings.Contains(guided.Guidance.Evidence[0], passed.Result.Report.Commit) {
		t.Fatalf("guidance = %#v, want two completed reviews and the current review first", guided.Guidance)
	}
	// Beside the review, the evidence reaches the accepted Contract, the
	// reworked implementation and the rejecting review it followed.
	evidence := presentEvidence(t, offline, guided.Guidance)
	for _, want := range []ledger.Reference{reworked.Result.Report, rejected.Result.Report} {
		if shown := offline.ledgerJSON(t, "skl", "ledger", "show", "--commit", want.Commit, "--path", want.Path, "--format", "json"); !strings.Contains(evidence, shown.Document.Contents) {
			t.Errorf("evidence lacks %s at %s:\n%s", want.Path, want.Commit, evidence)
		}
	}
	if !strings.Contains(evidence, "# Foundation intent") || !strings.Contains(evidence, "# Foundation behavior") {
		t.Errorf("evidence lacks the accepted Contract:\n%s", evidence)
	}
	if !strings.Contains(guided.Guidance.Continue, "--item '"+deliveryTestItem+"'") || !strings.Contains(guided.Guidance.Authoring, "pull-presentation.md watchdog") {
		t.Fatalf("guidance commands = %#v", guided.Guidance)
	}
	markdown, err := offline.deliveryRun(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range append([]string{"Status: prose_required", "Current result: watchdog pass", "Completed reviews: 2", guided.Guidance.Authoring, guided.Guidance.Continue, "reviewed " + final}, guided.Guidance.Evidence...) {
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
	if authoring, err := offline.deliveryRun(t, "skl", "skill", "--resource", "pull-presentation.md", "watchdog"); err != nil {
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
	for _, want := range []string{"Status: pending", "Public presentation: pending — " + pending.Presentation.Publication.Detail, "`" + pending.Guidance.Continue + "`"} {
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

// presentGuided reads explicit presentation's authoring guidance twice, so a
// repeated presentation is observed to leave the result and its count alone.
func presentGuided(t *testing.T, cli ledgerCLI, source string) *presentationGuidance {
	t.Helper()
	var first *presentationGuidance
	for range 2 {
		guided := cli.ledgerJSON(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem, "--format", "json")
		if guided.Status != "prose_required" || guided.Guidance == nil {
			t.Fatalf("present without prose = %s", mustJSON(t, guided))
		}
		if first != nil && mustJSON(t, guided.Guidance) != mustJSON(t, first) {
			t.Fatalf("repeated presentation changed its guidance:\n%s\nwant\n%s", mustJSON(t, guided.Guidance), mustJSON(t, first))
		}
		first = guided.Guidance
	}
	return first
}

// TestPresentationAuthoringCarriesTheCompletedReviewCount covers
// self-contained-forge-briefs B2 and B4 at both authoring interfaces: every
// handoff's report resource and explicit presentation carry the cumulative
// Review Count. A completed review, including one that pauses, advances it;
// an interrupted review attempt, re-implementation and repeated presentation
// leave it unchanged, as does human direction, and an implementation report's
// own round zero never stands in for it.
func TestPresentationAuthoringCarriesTheCompletedReviewCount(t *testing.T) {
	newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	offline := deliveryNoForgeApp(t)

	// authoring runs a rendering's report resource command as printed.
	authoring := func(started deliveryOutput, binds ...string) string {
		t.Helper()
		command := started.Packet.Facts.Delivery.ResultResourceCommand
		if !strings.Contains(started.Packet.Instructions, "`"+command+"`") {
			t.Fatalf("rendering does not print its report resource command %s", command)
		}
		for _, bind := range binds {
			if !strings.Contains(command, bind) {
				t.Errorf("report resource command does not bind %q: %s", bind, command)
			}
		}
		return runDeferredCommand(t, command, "", "")
	}
	// presented checks the current result explicit presentation authors from.
	presented := func(phase, lifecycle string, count uint64) {
		t.Helper()
		result := presentGuided(t, offline, source).Result
		if result.Phase != phase || result.Lifecycle != lifecycle || result.ReviewCount != count {
			t.Fatalf("current result = %s, want %s in %s after %d completed reviews", mustJSON(t, result), phase, lifecycle, count)
		}
		markdown, err := offline.deliveryRun(t, "skl", "ledger", "present", "--repo", source, "--item", deliveryTestItem)
		if want := fmt.Sprintf("Completed reviews: %d\n", count); err != nil || !strings.Contains(markdown, want) {
			t.Fatalf("Markdown guidance lacks %q: %v\n%s", want, err, markdown)
		}
	}

	started, _, _ := presentHandoff(t, offline, source, ledger.ImplementPhase, "--target", target)
	authoring(started, "--input review_count=0")
	presented(ledger.ImplementPhase, ledger.AwaitingReview, 0)

	attempt, err := offline.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil || attempt.Execution == nil {
		t.Fatalf("review attempt = %#v, %v", attempt, err)
	}
	if _, err := offline.deliveryJSON(t, "skl", "watchdog", "release", "--repo", source, "--item", deliveryTestItem, "--claim", attempt.Execution.Claim.Commit, "--format", "json"); err != nil {
		t.Fatal(err)
	}
	presented(ledger.ImplementPhase, ledger.AwaitingReview, 0)

	started, _, _ = presentHandoff(t, offline, source, ledger.WatchdogPhase, "--outcome", "rework")
	authoring(started, "--input round=1", "--input rework_pauses=false")
	presented(ledger.WatchdogPhase, ledger.Rework, 1)

	started, reworked, _ := presentHandoff(t, offline, source, ledger.ImplementPhase, "--target", target)
	if resource := authoring(started, "--input procedure=rework", "--input review_count=1"); !strings.Contains(resource, "Completed reviews: 1.") {
		t.Errorf("rework report resource does not carry the completed review:\n%s", resource)
	}
	if report, _ := deliveryCommittedReport(t, offline, reworked.Result.Report, ledger.ImplementPhase); report.Round != 0 {
		t.Fatalf("implementation report round = %d, want its own round zero", report.Round)
	}
	presented(ledger.ImplementPhase, ledger.AwaitingReview, 1)

	started, _, _ = presentHandoff(t, offline, source, ledger.WatchdogPhase, "--outcome", "needs-human")
	authoring(started, "--input round=2", "--input rework_pauses=true")
	presented(ledger.WatchdogPhase, ledger.NeedsHuman, 2)

	// Human direction continues the implementation without resetting the count.
	if direction := deliveryRecordHumanDirection(t, offline, target, ledger.RouteImplement); direction.Status != ledger.DecisionApplied {
		t.Fatalf("human direction = %#v", direction)
	}
	started, _, _ = presentHandoff(t, offline, source, ledger.ImplementPhase, "--target", target)
	authoring(started, "--input review_count=2")
	presented(ledger.ImplementPhase, ledger.AwaitingReview, 2)
}

// TestPresentationEvidenceReachesADecisionThroughRework covers
// self-contained-forge-briefs B3 and A1: after independent review sends
// directed work back, the Human Decision that shaped it no longer governs the
// new round, yet explicit presentation's evidence still reaches it through
// the consumed reports, beside the Contract's human-owned check.
func TestPresentationEvidenceReachesADecisionThroughRework(t *testing.T) {
	newLedgerFixture(t)
	source, target := deliverySourceRepo(t)
	deliveryAcceptFixture(t, newForgeServer(t), source)
	offline := deliveryNoForgeApp(t)

	start, err := offline.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || start.Execution == nil {
		t.Fatalf("start = %#v, %v", start, err)
	}
	question := filepath.Join(t.TempDir(), "pause.md")
	writeFile(t, question, "# Decision needed\n\nWhich widgets does the dashboard list?\n")
	if paused, err := offline.deliveryJSON(t, "skl", "implement", "needs-human", "--repo", source, "--item", deliveryTestItem, "--claim", start.Execution.Claim.Commit, "--body", question, "--format", "json"); err != nil || paused.Status != ledger.NeedsHuman {
		t.Fatalf("pause = %#v, %v", paused, err)
	}
	if direction := deliveryRecordHumanDirection(t, offline, target, ledger.RouteImplement); direction.Status != ledger.DecisionApplied {
		t.Fatalf("human direction = %#v", direction)
	}
	answer := "Continue at " + target + " within the frozen Contract."

	presentHandoff(t, offline, source, ledger.ImplementPhase, "--target", target)
	if evidence := presentEvidence(t, offline, presentGuided(t, offline, source)); !strings.Contains(evidence, answer) {
		t.Fatalf("directed implementation evidence lacks its decision:\n%s", evidence)
	}
	presentHandoff(t, offline, source, ledger.WatchdogPhase, "--outcome", "rework")
	started, _, _ := presentHandoff(t, offline, source, ledger.ImplementPhase, "--target", target)
	for _, document := range started.Execution.Documents {
		if strings.HasSuffix(document.Path, "/decision.md") {
			t.Fatalf("rework rendering supplies the settled decision as direction: %s", document.Path)
		}
	}

	guidance := presentGuided(t, offline, source)
	if guidance.Result.Decision != nil || guidance.Result.Watchdog == nil {
		t.Fatalf("rework result = %s, want no governing decision and the consumed review", mustJSON(t, guidance.Result))
	}
	if evidence := presentEvidence(t, offline, guidance); !strings.Contains(evidence, "Confirm the dashboard by hand.") {
		t.Errorf("evidence lacks the Contract's human-owned check:\n%s", evidence)
	}
	review, _ := deliveryCommittedReport(t, offline, *guidance.Result.Watchdog, ledger.WatchdogPhase)
	if review.Ledger.Decision == nil {
		t.Fatal("the consumed review names no decision to follow")
	}
	if !strings.Contains(started.Packet.Instructions, "path: "+review.Ledger.Decision.Path) {
		t.Errorf("rework rendering does not show the consumed review's decision reference")
	}
	followed := offline.ledgerJSON(t, "skl", "ledger", "show", "--commit", review.Ledger.Decision.Commit, "--path", review.Ledger.Decision.Path, "--format", "json")
	if followed.Document == nil || !strings.Contains(followed.Document.Contents, answer) {
		t.Fatalf("following the decision reference = %#v", followed.Document)
	}
}
