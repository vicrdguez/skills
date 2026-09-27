package main

// Goldens of the agent-visible prose on the ledger path. One scripted journey
// drives the public `skl` surface against local Git ledger and source
// fixtures and records every Markdown rendering a worker receives, so a
// template or outcome change shows up as a golden diff. `mise run
// prose-metrics` measures these files; see docs/agent-prose.md.
//
// Regenerate after an intended rendering change with:
//
//	go test ./cmd/skl -run TestAgentProseGoldens -update-prose

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

var updateProse = flag.Bool("update-prose", false, "rewrite testdata/prose goldens from the current renderings")

const (
	proseProposal = "widget-dashboard"
	proseItem     = proseProposal + "/foundation"
)

var (
	proseClaimLine  = regexp.MustCompile("Claim: `([0-9a-f]{40})`")
	proseResultLine = regexp.MustCompile("Result Documents: `([^`]+)`")
	proseSHA        = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
	proseResultDir  = regexp.MustCompile(`skl-(implement|watchdog)-[0-9]+`)
	proseTimestamp  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	// A Dispatch's Claim and continue command, as the Supervisor reads them.
	dispatchClaim    = regexp.MustCompile("is claimed for [A-Za-z ]+ with Claim `([0-9a-f]{40})`")
	dispatchContinue = regexp.MustCompile("When the subagent returns, run:\n\n`([^`]+)`")
	proseResource    = regexp.MustCompile("skl skill --resource ([^<\\s]\\S*)(?: --input [^`\\s]+)* ([a-z-]+)")
)

// proseGoldens compares or rewrites each rendering after replacing the values
// that differ between runs: temporary paths, commit identities, the forge URL
// and timestamps.
type proseGoldens struct {
	t         *testing.T
	directory string
	paths     [][2]string
	written   map[string]bool
}

func newProseGoldens(t *testing.T) *proseGoldens {
	return &proseGoldens{t: t, directory: proseDirectory(), written: map[string]bool{}}
}

// replace maps one run-specific path to a stable placeholder, including its
// symlink-resolved spelling.
func (g *proseGoldens) replace(value, placeholder string) {
	g.paths = append(g.paths, [2]string{value, placeholder})
	if resolved, err := filepath.EvalSymlinks(value); err == nil && resolved != value {
		g.paths = append(g.paths, [2]string{resolved, placeholder})
	}
}

func (g *proseGoldens) normalize(text string) string {
	paths := slices.Clone(g.paths)
	sort.SliceStable(paths, func(i, j int) bool { return len(paths[i][0]) > len(paths[j][0]) })
	for _, pair := range paths {
		text = strings.ReplaceAll(text, pair[0], pair[1])
	}
	text = proseResultDir.ReplaceAllString(text, "skl-$1-result")
	text = proseTimestamp.ReplaceAllString(text, "2026-01-01T00:00:00Z")
	numbers := map[string]int{}
	return proseSHA.ReplaceAllStringFunc(text, func(sha string) string {
		if _, ok := numbers[sha]; !ok {
			numbers[sha] = len(numbers) + 1
		}
		return fmt.Sprintf("%040d", numbers[sha])
	})
}

func (g *proseGoldens) check(name, rendered string) {
	g.t.Helper()
	if g.written[name] {
		g.t.Fatalf("golden %s recorded twice", name)
	}
	g.written[name] = true
	got := g.normalize(rendered)
	path := filepath.Join(g.directory, name+".md")
	if *updateProse {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			g.t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		g.t.Errorf("golden %s is missing; run with -update-prose: %v", name, err)
		return
	}
	if string(want) != got {
		g.t.Errorf("rendering %s differs from testdata/prose/%s.md; run with -update-prose and review the diff:\n%s", name, name, got)
	}
}

// run executes one CLI invocation and returns what the worker sees: stdout,
// followed by the error text a failed invocation prints.
func (g *proseGoldens) run(cli ledgerCLI, args ...string) string {
	g.t.Helper()
	output, err := cli.deliveryRun(g.t, append([]string{"skl"}, args...)...)
	if err != nil {
		output += err.Error() + "\n"
	}
	return output
}

func (g *proseGoldens) capture(name string, cli ledgerCLI, args ...string) string {
	g.t.Helper()
	output := g.run(cli, args...)
	g.check(name, output)
	return output
}

// finish refuses goldens that no rendering produced any more, and resources a
// golden names without a golden of its own.
func (g *proseGoldens) finish() {
	g.t.Helper()
	entries, err := os.ReadDir(g.directory)
	if err != nil {
		g.t.Fatal(err)
	}
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".md")
		if ok && !g.written[name] {
			g.t.Errorf("testdata/prose/%s has no rendering; remove it", entry.Name())
		}
	}
	for name := range g.written {
		contents, err := os.ReadFile(filepath.Join(g.directory, name+".md"))
		if err != nil {
			continue
		}
		for _, match := range proseResource.FindAllStringSubmatch(string(contents), -1) {
			if resource := proseResourceGolden(match[2], match[1]); !g.written[resource] {
				g.t.Errorf("%s tells the worker to retrieve %s from %s, which has no golden %s", name, match[1], match[2], resource)
			}
		}
	}
}

func proseResourceGolden(skill, resource string) string {
	return "resource-" + skill + "-" + strings.TrimSuffix(filepath.Base(resource), ".md")
}

func proseFixture(t *testing.T, name string) string {
	t.Helper()
	return readRepositoryFile(t, filepath.Join("testdata", "prose", "fixtures", name))
}

func proseDirectory() string {
	_, testFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(testFile), "..", "..", "testdata", "prose")
}

func proseFixturePath(name string) string {
	return filepath.Join(proseDirectory(), "fixtures", name)
}

func proseMatch(t *testing.T, pattern *regexp.Regexp, output string) string {
	t.Helper()
	match := pattern.FindStringSubmatch(output)
	if match == nil {
		t.Fatalf("output lacks %s:\n%s", pattern, output)
	}
	return match[1]
}

func proseCommit(t *testing.T, worktree, file, contents string) string {
	t.Helper()
	writeFile(t, filepath.Join(worktree, file), contents)
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "change "+file)
	return deliveryTrimmed(t, worktree, "rev-parse", "HEAD")
}

func TestAgentProseGoldens(t *testing.T) {
	g := newProseGoldens(t)
	fixture := newLedgerFixture(t)
	forge := newForgeServer(t)
	source, target := deliverySourceRepo(t)
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	g.replace(source, "/work/widgets")
	g.replace(fixture.clone, "/work/ledger")
	g.replace(temp, "/tmp")
	g.replace(filepath.Dir(temp), "/tmp/test")
	g.replace(filepath.Clean(proseFixturePath(".")), "/tmp/fixtures")
	g.replace(forge.server.URL, "https://forge.example")

	proposer := newLedgerApp(t, forge)
	worker := deliveryNoForgeApp(t)
	issue := "--issue=foundation=" + proseFixturePath("issue.md")
	intake := func(name string, files map[string]string) string {
		spec := proposalSpec{name: name, description: proseFixture(t, "proposal.md"), slices: []proposalSliceSpec{{name: "foundation", title: "Dashboard foundation", branch: name, files: files}}}
		return writeProposal(t, "", spec)
	}
	contract := map[string]string{"intent.md": proseFixture(t, "intent.md"), "behavior.md": proseFixture(t, "behavior.md")}

	// Propose: acceptance, readback, publication and cleanup.
	g.capture("outcome-ledger-accept-fix-required", proposer, "ledger", "accept", "--repo", source, "--proposal-dir", intake(proseProposal, map[string]string{"intent.md": contract["intent.md"]}), issue)
	accepted := intake(proseProposal, contract)
	g.capture("outcome-ledger-accept-accepted", proposer, "ledger", "accept", "--repo", source, "--proposal-dir", accepted, issue)
	g.capture("outcome-ledger-accept-existing", proposer, "ledger", "accept", "--repo", source, "--proposal-dir", accepted, issue)
	g.capture("outcome-ledger-show", proposer, "ledger", "show", "--repo", source, "--item", proseItem)
	g.capture("outcome-ledger-publish", proposer, "ledger", "publish", "--repo", source, "--proposal", proseProposal, issue)
	g.capture("outcome-propose-cleanup", proposer, "propose", "cleanup", "--repo", source)

	// Implement: start, release, resume, prepare and inspect, then hand off.
	g.capture("outcome-watchdog-next-no-work", worker, "watchdog", "next", "--repo", source)
	released := proseMatch(t, proseClaimLine, g.capture("implement-start", worker, "implement", "next", "--repo", source))
	g.capture("outcome-implement-release", worker, "implement", "release", "--repo", source, "--item", proseItem, "--claim", released)
	reviewer := []string{"--reviewer-model", "openai-codex/gpt-6-sol", "--reviewer-thinking", "xhigh"}
	team := append([]string{"--mode", "team", "--helper-model", "openai-codex/gpt-6-luna", "--helper-thinking", "xhigh"}, reviewer...)
	released = proseMatch(t, proseClaimLine, g.capture("implement-start-reviewer", worker, append([]string{"implement", "next", "--repo", source}, reviewer...)...))
	g.run(worker, "implement", "release", "--repo", source, "--item", proseItem, "--claim", released)
	started := g.capture("implement-team-start", worker, append([]string{"implement", "next", "--repo", source}, team...)...)
	claim, result := proseMatch(t, proseClaimLine, started), proseMatch(t, proseResultLine, started)
	identity := []string{"--repo", source, "--remote", "origin", "--item", proseItem, "--claim", claim, "--result-directory", result}
	g.capture("implement-prepare", worker, append([]string{"implement", "prepare"}, identity...)...)
	g.capture("implement-inspect", worker, append([]string{"implement", "inspect", "--target", target}, identity...)...)
	g.capture("implement-resume", worker, append([]string{"implement", "resume"}, identity...)...)
	g.capture("implement-team-resume", worker, append(append([]string{"implement", "resume"}, identity...), team...)...)
	g.capture("outcome-implement-fix-required", worker, "implement", "resume", "--repo", source, "--item", proseItem, "--claim", released)
	worktree := filepath.Join(source, ".worktrees", proseProposal)
	head := proseCommit(t, worktree, "dashboard.txt", "every widget\n")
	report, public := proseFixturePath("implement-report.md"), proseFixturePath("public.md")
	submit := func(name, claim, head string) {
		t.Helper()
		output := g.run(worker, "implement", "submit", "--repo", source, "--item", proseItem, "--claim", claim, "--head", head, "--target", target, "--body", report, "--public-body", public)
		if !strings.Contains(output, "Status: "+ledger.AwaitingReview) {
			t.Fatalf("implement submit:\n%s", output)
		}
		if name != "" {
			g.check(name, output)
		}
	}
	submit("outcome-implement-submit", claim, head)
	g.capture("outcome-ledger-present", worker, "ledger", "present", "--repo", source, "--item", proseItem)

	// First Watchdog review routes the Work Item to Rework.
	review := func(name string) (string, string) {
		t.Helper()
		output := g.run(worker, "watchdog", "next", "--repo", source)
		if name != "" {
			g.check(name, output)
		}
		return proseMatch(t, proseClaimLine, output), proseMatch(t, proseResultLine, output)
	}
	reviewIdentity := func(claim, result string) []string {
		return []string{"--repo", source, "--remote", "origin", "--item", proseItem, "--claim", claim, "--result-directory", result}
	}
	verdict := func(name, claim, outcome, status string) {
		t.Helper()
		output := g.run(worker, "watchdog", "submit", "--repo", source, "--item", proseItem, "--claim", claim, "--outcome", outcome,
			"--body", proseFixturePath("watchdog-report.md"), "--public-body", public)
		if !strings.Contains(output, "Status: "+status) {
			t.Fatalf("watchdog submit %s:\n%s", outcome, output)
		}
		g.check(name, output)
	}
	// The CLI renders a review packet before inspection resolves its scope, so
	// the template's scoped sections render from the same Claim's facts with
	// the scope that inspection reports.
	scoped := func(name string, identity []string, scope string) {
		t.Helper()
		out, err := worker.deliveryJSON(t, append([]string{"skl", "watchdog", "resume", "--format", "json"}, identity...)...)
		if err != nil || out.Packet == nil || out.Packet.Facts.Delivery == nil {
			t.Fatalf("watchdog resume: %#v %v", out, err)
		}
		facts := *out.Packet.Facts.Delivery
		facts.Operation, facts.Procedure, facts.ReviewScope = "next", "initial", scope
		packet, err := skilldist.BuildPacket("watchdog", skilldist.InvocationFacts{Delivery: &facts})
		if err != nil {
			t.Fatal(err)
		}
		g.check(name, packet.Instructions)
	}
	releasedReview, _ := review("watchdog-start")
	g.capture("outcome-watchdog-release", worker, "watchdog", "release", "--repo", source, "--item", proseItem, "--claim", releasedReview)
	reviewClaim, reviewResult := review("")
	g.capture("watchdog-prepare", worker, append([]string{"watchdog", "prepare"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	g.capture("watchdog-inspect", worker, append([]string{"watchdog", "inspect"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	g.capture("watchdog-resume", worker, append([]string{"watchdog", "resume"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	verdict("outcome-watchdog-submit-rework", reviewClaim, "rework", ledger.Rework)
	g.capture("outcome-ledger-present-watchdog", worker, "ledger", "present", "--repo", source, "--item", proseItem)
	g.capture("outcome-ledger-present-public", worker, "ledger", "present", "--repo", source, "--item", proseItem, "--public-body", public)

	// Rework, then a repeat review of a descendant head is incremental and
	// routes the Work Item to Needs Human.
	rework := g.capture("implement-rework", worker, "implement", "next", "--repo", source)
	claim, result = proseMatch(t, proseClaimLine, rework), proseMatch(t, proseResultLine, rework)
	g.run(worker, "implement", "prepare", "--repo", source, "--item", proseItem, "--claim", claim, "--result-directory", result)
	submit("", claim, proseCommit(t, worktree, "dashboard.txt", "every widget, unhealthy ones marked\n"))
	reviewClaim, reviewResult = review("watchdog-repeat")
	g.capture("watchdog-inspect-incremental", worker, append([]string{"watchdog", "inspect"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	scoped("watchdog-repeat-incremental", reviewIdentity(reviewClaim, reviewResult), "incremental")
	verdict("outcome-watchdog-submit-needs-human", reviewClaim, "rework", ledger.NeedsHuman)

	// Decision Inbox: the request, the recorded answer, then empty and
	// unavailable reads.
	g.capture("decision-inbox", worker, "decision", "inbox")
	request := decisionRequest(t, worker, "widgets", proseItem)
	g.capture("decision-result", worker, "decision", "apply", "--project", "widgets", "--item", proseItem,
		"--request-commit", request.RequestCommit, "--request-path", request.RequestPath, "--route", ledger.RouteImplement, "--answer", proseFixturePath("answer.md"))
	g.capture("decision-inbox-empty", worker, "decision", "inbox")
	g.capture("decision-inbox-unavailable", worker, "decision", "inbox", "--project", "no-such-project")

	// Directed implementation rewrites the branch, so the next repeat review
	// cannot build on the previous reviewed revision and is full.
	directed := g.capture("implement-directed", worker, "implement", "next", "--repo", source)
	claim, result = proseMatch(t, proseClaimLine, directed), proseMatch(t, proseResultLine, directed)
	g.run(worker, "implement", "prepare", "--repo", source, "--item", proseItem, "--claim", claim, "--result-directory", result)
	runGit(t, worktree, "reset", "-q", "--hard", target)
	submit("", claim, proseCommit(t, worktree, "dashboard.txt", "every widget, retired ones kept\n"))
	reviewClaim, reviewResult = review("")
	g.capture("watchdog-inspect-full", worker, append([]string{"watchdog", "inspect"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	scoped("watchdog-repeat-full", reviewIdentity(reviewClaim, reviewResult), "full")
	verdict("outcome-watchdog-submit-pass", reviewClaim, "pass", ledger.ReadyForMerge)

	// A pause before any source progress, then no remaining work.
	pausedProposal := "widget-export"
	if outcome := proposer.accept(t, source, intake(pausedProposal, contract), issue); outcome.Status != "accepted" {
		t.Fatalf("accept %s: %s", pausedProposal, mustJSON(t, outcome))
	}
	paused := proseMatch(t, proseClaimLine, g.run(worker, "implement", "next", "--repo", source))
	g.capture("outcome-implement-needs-human", worker, "implement", "needs-human", "--repo", source, "--item", pausedProposal+"/foundation", "--claim", paused,
		"--body", proseFixturePath("pause.md"), "--public-body", public)
	g.capture("outcome-implement-next-no-work", worker, "implement", "next", "--repo", source)
	g.capture("outcome-implement-next-idle-timeout", worker, "implement", "next", "--repo", source, "--wait", "1ms", "--poll", "1ms")
	interrupted, cancel := context.WithCancel(context.Background())
	cancel()
	worker.out.Reset()
	err := worker.app.RunContext(interrupted, []string{"skl", "watchdog", "next", "--repo", source, "--wait", "1m"})
	g.check("outcome-watchdog-next-interrupted", worker.out.String()+err.Error()+"\n")

	// A Supervisor's Dispatches: claimed, stopped on a held Claim, continued
	// after a handoff into an idle window, an interrupted wait and a new
	// Dispatch, stopped on a released Claim, and refused.
	for _, name := range []string{"widget-search", "widget-share"} {
		if outcome := proposer.accept(t, source, intake(name, contract), issue); outcome.Status != "accepted" {
			t.Fatalf("accept %s: %s", name, mustJSON(t, outcome))
		}
	}
	dispatched := g.capture("outcome-implement-next-dispatch", worker, "implement", "next", "--dispatch", "--repo", source, "--worker-model", "openai-codex/gpt-6-astra", "--worker-thinking", "high")
	searchClaim, continuation := proseMatch(t, dispatchClaim, dispatched), proseMatch(t, dispatchContinue, dispatched)
	g.capture("outcome-implement-next-dispatch-held", worker, "implement", "next", "--dispatch", "--repo", source, "--after", searchClaim)
	g.run(worker, "implement", "prepare", "--repo", source, "--item", "widget-search/foundation", "--claim", searchClaim)
	submitted := g.run(worker, "implement", "submit", "--repo", source, "--item", "widget-search/foundation", "--claim", searchClaim,
		"--head", proseCommit(t, filepath.Join(source, ".worktrees", "widget-search"), "search.txt", "search widgets\n"), "--target", target, "--body", report, "--public-body", public)
	if !strings.Contains(submitted, "Status: "+ledger.AwaitingReview) {
		t.Fatalf("submit widget-search:\n%s", submitted)
	}
	// Hold the remaining Slice so the continued Dispatch finds no work.
	shareClaim := proseMatch(t, proseClaimLine, g.run(worker, "implement", "next", "--repo", source))
	g.capture("outcome-implement-next-dispatch-idle-timeout", worker, "implement", "next", "--dispatch", "--repo", source, "--after", searchClaim, "--wait", "1ms", "--poll", "1ms")
	interrupted, cancel = context.WithCancel(context.Background())
	cancel()
	worker.out.Reset()
	err = worker.app.RunContext(interrupted, []string{"skl", "implement", "next", "--dispatch", "--repo", source, "--after", searchClaim, "--wait", "1m"})
	g.check("outcome-implement-next-dispatch-interrupted", worker.out.String()+err.Error()+"\n")
	g.run(worker, "implement", "release", "--repo", source, "--item", "widget-share/foundation", "--claim", shareClaim)
	continued := g.capture("outcome-implement-next-dispatch-continued", worker, shellWords(t, continuation)[1:]...)
	shareClaim = proseMatch(t, dispatchClaim, continued)
	g.run(worker, "implement", "release", "--repo", source, "--item", "widget-share/foundation", "--claim", shareClaim)
	g.capture("outcome-implement-next-dispatch-released", worker, "implement", "next", "--dispatch", "--repo", source, "--after", shareClaim)
	g.capture("outcome-implement-next-dispatch-refused", worker, "implement", "next", "--dispatch", "--repo", source, "--after", "not-a-claim")

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	g.capture("outcome-implement-next-unconfigured", worker, "implement", "next", "--repo", source)

	// Standalone Execution Skills, the refusals for the delivered ones, and
	// every named resource a rendering tells the worker to retrieve.
	for _, name := range skilldist.SkillNames() {
		if name == "implement" || name == "watchdog" {
			g.capture("outcome-skill-"+name, worker, "skill", name)
			continue
		}
		g.capture("skill-"+name, worker, "skill", name)
	}
	resources := map[string][]string{
		"audit":              {"acceptance.md", "smells.md"},
		"decision":           {"triage.md"},
		"design":             {"DEEPENING.md", "DESIGN-IT-TWICE.md"},
		"domain":             {"ADR-FORMAT.md", "CONTEXT-FORMAT.md"},
		"propose":            {"behavior.md", "intent.md", "plan.md", "tasks.md"},
		"implement":          {"pull-presentation.md"},
		"testing":            {"mocking.md", "tests.md"},
		"watchdog":           {"pull-presentation.md"},
		"writing-for-agents": {"SKILL-MECHANICS.md"},
	}
	for skill, names := range resources {
		for _, resource := range names {
			g.capture(proseResourceGolden(skill, resource), worker, "skill", "--resource", resource, skill)
		}
	}
	g.capture("resource-propose-issue-publication", worker, "skill", "--resource", "issue-publication.md",
		"--input", "proposal="+proseProposal, "--input", "repo="+source, "--input", "remote=origin", "propose")
	g.capture("resource-implement-ledger-submission", worker, "skill", "--resource", "ledger-submission.md",
		"--input", "result_directory="+result, "--input", "procedure=initial", "implement")
	g.capture("resource-watchdog-ledger-review", worker, "skill", "--resource", "ledger-review.md",
		"--input", "result_directory="+reviewResult, "--input", "round=1", "--input", "reviewed_head="+head, "watchdog")

	// Installed stubs and Harness Adapters, and the AGENTS.md block setup
	// writes. Codex keeps every stub, including Implement's and Watchdog's.
	home := t.TempDir()
	if _, err := skilldist.Install(home); err != nil {
		t.Fatal(err)
	}
	for _, name := range append(skilldist.SkillNames(), "implement-team") {
		g.check("stub-"+name, readFileString(t, filepath.Join(home, ".codex", "skills", name, "SKILL.md")))
	}
	for harness, location := range entryPoints {
		if harness == "codex" {
			continue
		}
		for _, operation := range []string{"implement", "implement-team", "watchdog"} {
			g.check("adapter-"+harness+"-"+operation, readFileString(t, filepath.Join(home, fmt.Sprintf(location, operation))))
		}
	}
	g.check("setup-agents-block", setup.AgentsBlock)

	g.finish()
}
