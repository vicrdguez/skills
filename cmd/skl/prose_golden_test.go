package main

// Goldens of the agent-visible prose on the ledger path. Independent journeys
// drive the public `skl` surface against local Git ledger and source
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
	// Without process-wide TMPDIR overrides, the CLI owns these temporary
	// result directories until this journey registers their cleanup.
	if match := proseResultLine.FindStringSubmatch(output); match != nil {
		directory := match[1]
		g.t.Cleanup(func() {
			if err := os.RemoveAll(directory); err != nil {
				g.t.Error(err)
			}
		})
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

// Each group owns its apps, buffers, source/ledger repositories and renderings.
// Only the cheap completeness check joins their observations. Updates stay
// serial so the existing regeneration entry point is safe and cohesive.
func TestAgentProseGoldens(t *testing.T) {
	if !*updateProse {
		t.Parallel()
	}
	groups := []struct {
		name    string
		capture func(*testing.T, *proseGoldens)
		names   []string
	}{
		{"delivery", captureProseDelivery, strings.Fields(`
			decision-inbox
			decision-inbox-empty
			decision-inbox-unavailable
			decision-result
			implement-directed
			implement-inspect
			implement-prepare
			implement-resume
			implement-rework
			implement-start
			implement-start-reviewer
			implement-team-resume
			implement-team-start
			outcome-implement-fix-required
			outcome-implement-needs-human
			outcome-implement-next-idle-timeout
			outcome-implement-next-no-work
			outcome-implement-release
			outcome-implement-submit
			outcome-ledger-accept-accepted
			outcome-ledger-accept-existing
			outcome-ledger-accept-fix-required
			outcome-ledger-export
			outcome-ledger-present
			outcome-ledger-present-public
			outcome-ledger-present-watchdog
			outcome-ledger-publish
			outcome-ledger-show
			outcome-propose-cleanup
			outcome-watchdog-next-interrupted
			outcome-watchdog-next-no-work
			outcome-watchdog-release
			outcome-watchdog-submit-needs-human
			outcome-watchdog-submit-pass
			outcome-watchdog-submit-rework
			watchdog-inspect
			watchdog-inspect-full
			watchdog-inspect-incremental
			watchdog-prepare
			watchdog-repeat
			watchdog-repeat-full
			watchdog-repeat-incremental
			watchdog-resume
			watchdog-start
		`)},
		{"integration", captureProseIntegration, strings.Fields(`
			outcome-watchdog-prepare-conflict
			watchdog-integration-inspect
			watchdog-integration-prepare
			watchdog-integration-resume
			watchdog-integration-start
		`)},
		{"supervisor", captureProseSupervisor, strings.Fields(`
			outcome-implement-next-dispatch
			outcome-implement-next-dispatch-continued
			outcome-implement-next-dispatch-held
			outcome-implement-next-dispatch-idle-timeout
			outcome-implement-next-dispatch-interrupted
			outcome-implement-next-dispatch-refused
			outcome-implement-next-dispatch-released
			outcome-ledger-amend-amended
			outcome-ledger-amend-existing
			outcome-ledger-amend-fix-required
		`)},
		{"readiness", captureProseReadiness, strings.Fields(`
			outcome-watchdog-auto-readiness
		`)},
		{"standalone", captureProseStandalone, strings.Fields(`
			adapter-claude-implement-auto
			adapter-claude-implement-loop
			adapter-claude-implement-team-auto
			adapter-claude-implement-team-loop
			adapter-claude-implement-team
			adapter-claude-implement
			adapter-claude-watchdog-auto
			adapter-claude-watchdog-loop
			adapter-claude-watchdog
			adapter-opencode-implement-auto
			adapter-opencode-implement-loop
			adapter-opencode-implement-team-auto
			adapter-opencode-implement-team-loop
			adapter-opencode-implement-team
			adapter-opencode-implement
			adapter-opencode-watchdog-auto
			adapter-opencode-watchdog-loop
			adapter-opencode-watchdog
			adapter-pi-implement-auto
			adapter-pi-implement-loop
			adapter-pi-implement-team-auto
			adapter-pi-implement-team-loop
			adapter-pi-implement-team
			adapter-pi-implement
			adapter-pi-watchdog-auto
			adapter-pi-watchdog-loop
			adapter-pi-watchdog
			outcome-implement-next-unconfigured
			outcome-skill-implement
			outcome-skill-watchdog
			resource-audit-acceptance
			resource-audit-smells
			resource-decision-triage
			resource-design-DEEPENING
			resource-design-DESIGN-IT-TWICE
			resource-domain-ADR-FORMAT
			resource-domain-CONTEXT-FORMAT
			resource-implement-ledger-submission
			resource-implement-pull-presentation
			resource-propose-behavior
			resource-propose-intent
			resource-propose-issue-publication
			resource-propose-plan
			resource-propose-tasks
			resource-testing-mocking
			resource-testing-tests
			resource-watchdog-ledger-review
			resource-watchdog-pull-presentation
			resource-writing-for-agents-SKILL-MECHANICS
			setup-agents-block
			skill-audit
			skill-brainstorm
			skill-decision
			skill-design
			skill-domain
			skill-explore
			skill-propose
			skill-testing
			skill-walkthrough
			skill-writing-for-agents
			stub-audit
			stub-brainstorm
			stub-decision
			stub-design
			stub-domain
			stub-explore
			stub-implement-team
			stub-implement
			stub-propose
			stub-testing
			stub-walkthrough
			stub-watchdog
			stub-writing-for-agents
		`)},
	}
	owners := map[string]string{}
	for _, group := range groups {
		for _, name := range group.names {
			if owner, exists := owners[name]; exists {
				t.Fatalf("golden %s owned by both %s and %s", name, owner, group.name)
			}
			owners[name] = group.name
		}
	}
	observations := make([]*proseGoldens, len(groups))
	t.Run("journeys", func(t *testing.T) {
		for index, group := range groups {
			t.Run(group.name, func(t *testing.T) {
				if !*updateProse {
					t.Parallel()
				}
				g := newProseGoldens(t)
				observations[index] = g
				group.capture(t, g)
				for _, name := range group.names {
					if !g.written[name] {
						t.Errorf("group %s omitted rendering %s", group.name, name)
					}
				}
				for name := range g.written {
					if owners[name] != group.name {
						t.Errorf("group %s rendered %s owned by %q", group.name, name, owners[name])
					}
				}
			})
		}
	})
	all := newProseGoldens(t)
	for _, g := range observations {
		if g == nil {
			continue
		}
		for name := range g.written {
			if all.written[name] {
				t.Errorf("golden %s rendered by multiple groups", name)
			}
			all.written[name] = true
		}
	}
	all.finish()
}

type proseJourney struct {
	source, target, temp string
	proposer, worker     ledgerCLI
}

func newProseJourney(t *testing.T, g *proseGoldens, issueOffset int) proseJourney {
	t.Helper()
	fixture := newPrivateLedgerFixture(t)
	forge := newForgeServer(t)
	forge.issueOffset = issueOffset
	source, target := deliverySourceRepo(t)
	temp := t.TempDir()
	g.replace(source, "/work/widgets")
	g.replace(fixture.clone, "/work/ledger")
	g.replace(temp, "/tmp")
	g.replace(filepath.Dir(temp), "/tmp/test")
	g.replace(filepath.Clean(os.TempDir()), "/tmp")
	g.replace(filepath.Clean(proseFixturePath(".")), "/tmp/fixtures")
	g.replace(forge.server.URL, "https://forge.example")
	return proseJourney{source, target, temp, fixture.app(newLedgerApp(t, forge)), fixture.app(deliveryNoForgeApp(t))}
}

func proseIntake(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	spec := proposalSpec{name: name, description: proseFixture(t, "proposal.md"), slices: []proposalSliceSpec{{name: "foundation", title: "Dashboard foundation", branch: name, files: files}}}
	return writeProposal(t, "", spec)
}

func proseContract(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"intent.md": proseFixture(t, "intent.md"), "behavior.md": proseFixture(t, "behavior.md")}
}

func captureProseDelivery(t *testing.T, g *proseGoldens) {
	j := newProseJourney(t, g, 0)
	source, target, temp, proposer, worker := j.source, j.target, j.temp, j.proposer, j.worker
	issue := "--issue=foundation=" + proseFixturePath("issue.md")
	intake := func(name string, files map[string]string) string { return proseIntake(t, name, files) }
	contract := proseContract(t)
	// Propose: acceptance, readback, publication and cleanup.
	g.capture("outcome-ledger-accept-fix-required", proposer, "proposal", "accept", "--repo", source, "--proposal-dir", intake(proseProposal, map[string]string{"intent.md": contract["intent.md"]}), issue)
	accepted := intake(proseProposal, contract)
	g.capture("outcome-ledger-accept-accepted", proposer, "proposal", "accept", "--repo", source, "--proposal-dir", accepted, issue)
	g.capture("outcome-ledger-accept-existing", proposer, "proposal", "accept", "--repo", source, "--proposal-dir", accepted, issue)
	g.capture("outcome-ledger-show", proposer, "slice", "show", "--repo", source, "--slice", proseItem)
	g.capture("outcome-ledger-publish", proposer, "proposal", "publish", "--repo", source, "--proposal", proseProposal, issue)
	g.capture("outcome-ledger-export", proposer, "proposal", "export", "--repo", source, "--proposal", proseProposal, "--to", filepath.Join(temp, "export"))
	g.capture("outcome-propose-cleanup", proposer, "cleanup", "--repo", source)

	// Implement: start, release, resume, prepare and inspect, then hand off.
	g.capture("outcome-watchdog-next-no-work", worker, "watchdog", "next", "--repo", source)
	standard, err := worker.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--result-directory", filepath.Join(temp, "skl-implement-0"), "--format", "json")
	if err != nil || standard.Execution == nil || standard.Packet == nil {
		t.Fatalf("implement next: %#v %v", standard, err)
	}
	g.check("implement-start", standard.Packet.Instructions)
	// TestImplementChoicesRenderWhereTheyApplyAndCarryThroughResume owns
	// reviewer CLI wiring. Render this pure variant from the live Claim rather
	// than acquiring and releasing another Claim for the same initial state.
	repository, err := setup.ResolveRepository(source, "origin")
	if err != nil {
		t.Fatal(err)
	}
	choices := setup.ImplementChoices{Reviewer: skilldist.SubagentChoice{Model: "openai-codex/gpt-6.1-sol", Thinking: "high"}}
	packet, err := setup.PresentDelivery(standard.Execution, repository, ledger.ImplementPhase, "next", nil, standard.Packet.Facts.Delivery.ResultDirectory, choices, choices)
	if err != nil {
		t.Fatal(err)
	}
	g.check("implement-start-reviewer", packet.Instructions)
	released := standard.Execution.Claim.Commit
	g.capture("outcome-implement-release", worker, "implement", "release", "--repo", source, "--slice", proseItem, "--claim", released)
	reviewer := []string{"--reviewer-model", "openai-codex/gpt-6.1-sol", "--reviewer-thinking", "high"}
	team := append([]string{"--mode", "team", "--helper-model", "openai-codex/gpt-6-luna", "--helper-thinking", "xhigh"}, reviewer...)
	started := g.capture("implement-team-start", worker, append([]string{"implement", "next", "--repo", source}, team...)...)
	claim, result := proseMatch(t, proseClaimLine, started), proseMatch(t, proseResultLine, started)
	identity := []string{"--repo", source, "--remote", "origin", "--slice", proseItem, "--claim", claim, "--result-directory", result}
	g.capture("implement-prepare", worker, append([]string{"implement", "prepare"}, identity...)...)
	g.capture("implement-inspect", worker, append([]string{"implement", "inspect", "--target", target}, identity...)...)
	g.capture("implement-resume", worker, append([]string{"implement", "resume"}, identity...)...)
	g.capture("implement-team-resume", worker, append(append([]string{"implement", "resume"}, identity...), team...)...)
	g.capture("outcome-implement-fix-required", worker, "implement", "resume", "--repo", source, "--slice", proseItem, "--claim", released)
	worktree := filepath.Join(source, ".worktrees", proseProposal)
	head := proseCommit(t, worktree, "dashboard.txt", "every widget\n")
	report, public := proseFixturePath("implement-report.md"), proseFixturePath("public.md")
	submit := func(name, claim, head string) {
		t.Helper()
		output := g.run(worker, "implement", "submit", "--repo", source, "--slice", proseItem, "--claim", claim, "--head", head, "--target", target, "--body", report, "--public-body", public)
		if !strings.Contains(output, "Status: "+ledger.AwaitingReview) {
			t.Fatalf("implement submit:\n%s", output)
		}
		if name != "" {
			g.check(name, output)
		}
	}
	submit("outcome-implement-submit", claim, head)
	g.capture("outcome-ledger-present", worker, "slice", "publish", "--repo", source, "--slice", proseItem)

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
		return []string{"--repo", source, "--remote", "origin", "--slice", proseItem, "--claim", claim, "--result-directory", result}
	}
	verdict := func(name, claim, outcome, status string) {
		t.Helper()
		output := g.run(worker, "watchdog", "submit", "--repo", source, "--slice", proseItem, "--claim", claim, "--outcome", outcome,
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
	g.capture("outcome-watchdog-release", worker, "watchdog", "release", "--repo", source, "--slice", proseItem, "--claim", releasedReview)
	reviewClaim, reviewResult := review("")
	g.capture("watchdog-prepare", worker, append([]string{"watchdog", "prepare"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	g.capture("watchdog-inspect", worker, append([]string{"watchdog", "inspect"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	g.capture("watchdog-resume", worker, append([]string{"watchdog", "resume"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	verdict("outcome-watchdog-submit-rework", reviewClaim, "rework", ledger.Rework)
	g.capture("outcome-ledger-present-watchdog", worker, "slice", "publish", "--repo", source, "--slice", proseItem)
	g.capture("outcome-ledger-present-public", worker, "slice", "publish", "--repo", source, "--slice", proseItem, "--public-body", public)

	// Rework, then a repeat review of a descendant head is incremental and
	// routes the Work Item to Needs Human.
	rework := g.capture("implement-rework", worker, "implement", "next", "--repo", source)
	claim, result = proseMatch(t, proseClaimLine, rework), proseMatch(t, proseResultLine, rework)
	g.run(worker, "implement", "prepare", "--repo", source, "--slice", proseItem, "--claim", claim, "--result-directory", result)
	submit("", claim, proseCommit(t, worktree, "dashboard.txt", "every widget, unhealthy ones marked\n"))
	reviewClaim, reviewResult = review("watchdog-repeat")
	g.capture("watchdog-inspect-incremental", worker, append([]string{"watchdog", "inspect"}, reviewIdentity(reviewClaim, reviewResult)...)...)
	scoped("watchdog-repeat-incremental", reviewIdentity(reviewClaim, reviewResult), "incremental")
	verdict("outcome-watchdog-submit-needs-human", reviewClaim, "rework", ledger.NeedsHuman)

	// Decision Inbox: the request, the recorded answer, then empty and
	// unavailable reads.
	g.capture("decision-inbox", worker, "decision", "inbox")
	request := decisionRequest(t, worker, "widgets", proseItem)
	g.capture("decision-result", worker, "decision", "apply", "--project", "widgets", "--slice", proseItem,
		"--request-commit", request.RequestCommit, "--request-path", request.RequestPath, "--route", ledger.RouteImplement, "--answer", proseFixturePath("answer.md"))
	g.capture("decision-inbox-empty", worker, "decision", "inbox")
	g.capture("decision-inbox-unavailable", worker, "decision", "inbox", "--project", "no-such-project")

	// Directed implementation rewrites the branch, so the next repeat review
	// cannot build on the previous reviewed revision and is full.
	directed := g.capture("implement-directed", worker, "implement", "next", "--repo", source)
	claim, result = proseMatch(t, proseClaimLine, directed), proseMatch(t, proseResultLine, directed)
	g.run(worker, "implement", "prepare", "--repo", source, "--slice", proseItem, "--claim", claim, "--result-directory", result)
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
	g.capture("outcome-implement-needs-human", worker, "implement", "needs-human", "--repo", source, "--slice", pausedProposal+"/foundation", "--claim", paused,
		"--body", proseFixturePath("pause.md"), "--public-body", public)
	g.capture("outcome-implement-next-no-work", worker, "implement", "next", "--repo", source)
	g.capture("outcome-implement-next-idle-timeout", worker, "implement", "next", "--repo", source, "--wait", "1ms", "--poll", "1ms")
	interrupted, cancel := context.WithCancel(context.Background())
	cancel()
	worker.out.Reset()
	err = worker.app.RunContext(interrupted, []string{"skl", "watchdog", "next", "--repo", source, "--wait", "1m"})
	g.check("outcome-watchdog-next-interrupted", worker.out.String()+err.Error()+"\n")

}

func captureProseIntegration(t *testing.T, g *proseGoldens) {
	j := newProseJourney(t, g, 0)
	source, proposer, worker := j.source, j.proposer, j.worker
	issue := "--issue=foundation=" + proseFixturePath("issue.md")
	intake := func(name string, files map[string]string) string { return proseIntake(t, name, files) }
	contract := proseContract(t)
	report := proseFixturePath("implement-report.md")
	// Auto Mode review of a Slice whose Proposal Branch moved: the Integration
	// Merge, then a conflicting branch head that ends the next review.
	integrationProposal := "widget-alerts"
	integrationItem := integrationProposal + "/foundation"
	if outcome := proposer.accept(t, source, intake(integrationProposal, contract), issue); outcome.Status != "accepted" {
		t.Fatalf("accept %s: %s", integrationProposal, mustJSON(t, outcome))
	}
	bare := routeSourcePushes(t, source)
	runGit(t, source, "push", "-q", "origin", "main")
	pusher := filepath.Join(t.TempDir(), "pusher")
	integrationWorktree := filepath.Join(source, ".worktrees", integrationProposal)
	advance := func(file, contents string) {
		t.Helper()
		if _, err := os.Stat(pusher); err != nil {
			runGit(t, source, "clone", "-q", "-b", "proposal/"+integrationProposal, bare, pusher)
			runGit(t, pusher, "config", "user.name", "Other")
			runGit(t, pusher, "config", "user.email", "other@example.com")
		}
		runGit(t, pusher, "pull", "-q")
		proseCommit(t, pusher, file, contents)
		runGit(t, pusher, "push", "-q", "origin", "proposal/"+integrationProposal)
	}
	implementAlerts := func(contents string) {
		t.Helper()
		output := g.run(worker, "implement", "next", "--auto", "--repo", source)
		claim := proseMatch(t, proseClaimLine, output)
		g.run(worker, "implement", "prepare", "--repo", source, "--remote", "origin", "--slice", integrationItem, "--claim", claim)
		submitted := g.run(worker, "implement", "submit", "--repo", source, "--slice", integrationItem, "--claim", claim,
			"--head", proseCommit(t, integrationWorktree, "alerts.txt", contents), "--target", deliveryTrimmed(t, source, "rev-parse", "refs/remotes/origin/proposal/"+integrationProposal), "--body", report)
		if !strings.Contains(submitted, "Status: "+ledger.AwaitingReview) {
			t.Fatalf("implement submit %s:\n%s", integrationItem, submitted)
		}
	}
	implementAlerts("alert on every widget\n")
	advance("other.txt", "another slice\n")
	output := g.capture("watchdog-integration-start", worker, "watchdog", "next", "--repo", source)
	integrationClaim, integrationResult := proseMatch(t, proseClaimLine, output), proseMatch(t, proseResultLine, output)
	integrationIdentity := []string{"--repo", source, "--remote", "origin", "--slice", integrationItem, "--claim", integrationClaim, "--result-directory", integrationResult}
	g.capture("watchdog-integration-prepare", worker, append([]string{"watchdog", "prepare"}, integrationIdentity...)...)
	g.capture("watchdog-integration-inspect", worker, append([]string{"watchdog", "inspect"}, integrationIdentity...)...)
	g.capture("watchdog-integration-resume", worker, append([]string{"watchdog", "resume"}, integrationIdentity...)...)
	reworked := g.run(worker, "watchdog", "submit", "--repo", source, "--slice", integrationItem, "--claim", integrationClaim, "--outcome", "rework",
		"--head", deliveryTrimmed(t, integrationWorktree, "rev-parse", "HEAD"), "--body", proseFixturePath("watchdog-report.md"))
	if !strings.Contains(reworked, "Status: "+ledger.Rework) {
		t.Fatalf("rework at the Integration Merge:\n%s", reworked)
	}
	implementAlerts("alert on unhealthy widgets\n")
	advance("alerts.txt", "alerts from another slice\n")
	output = g.run(worker, "watchdog", "next", "--repo", source)
	integrationClaim, integrationResult = proseMatch(t, proseClaimLine, output), proseMatch(t, proseResultLine, output)
	g.capture("outcome-watchdog-prepare-conflict", worker, "watchdog", "prepare", "--repo", source, "--remote", "origin", "--slice", integrationItem, "--claim", integrationClaim, "--result-directory", integrationResult)

}

func captureProseSupervisor(t *testing.T, g *proseGoldens) {
	// Keep the existing issue identities without creating the three unrelated
	// issues that preceded this group in the old growing-ledger journey.
	j := newProseJourney(t, g, 3)
	source, target, temp, proposer, worker := j.source, j.target, j.temp, j.proposer, j.worker
	issue := "--issue=foundation=" + proseFixturePath("issue.md")
	intake := func(name string, files map[string]string) string { return proseIntake(t, name, files) }
	contract := proseContract(t)
	report, public := proseFixturePath("implement-report.md"), proseFixturePath("public.md")
	// A Supervisor's Dispatches: claimed, stopped on a held Claim, continued
	// after a handoff into an idle window, an interrupted wait and a new
	// Dispatch, stopped on a released Claim, and refused.
	for _, name := range []string{"widget-search", "widget-share"} {
		if outcome := proposer.accept(t, source, intake(name, contract), issue); outcome.Status != "accepted" {
			t.Fatalf("accept %s: %s", name, mustJSON(t, outcome))
		}
	}
	dispatched := g.capture("outcome-implement-next-dispatch", worker, "implement", "dispatch", "--repo", source, "--worker-model", "openai-codex/gpt-6-astra", "--worker-thinking", "high")
	searchClaim, continuation := proseMatch(t, dispatchClaim, dispatched), proseMatch(t, dispatchContinue, dispatched)
	g.capture("outcome-implement-next-dispatch-held", worker, "implement", "dispatch", "--repo", source, "--after", searchClaim)
	g.run(worker, "implement", "prepare", "--repo", source, "--slice", "widget-search/foundation", "--claim", searchClaim)
	submitted := g.run(worker, "implement", "submit", "--repo", source, "--slice", "widget-search/foundation", "--claim", searchClaim,
		"--head", proseCommit(t, filepath.Join(source, ".worktrees", "widget-search"), "search.txt", "search widgets\n"), "--target", target, "--body", report, "--public-body", public)
	if !strings.Contains(submitted, "Status: "+ledger.AwaitingReview) {
		t.Fatalf("submit widget-search:\n%s", submitted)
	}
	// Hold the remaining Slice so the continued Dispatch finds no work.
	shareClaim := proseMatch(t, proseClaimLine, g.run(worker, "implement", "next", "--repo", source))
	g.capture("outcome-implement-next-dispatch-idle-timeout", worker, "implement", "dispatch", "--repo", source, "--after", searchClaim, "--wait", "1ms", "--poll", "1ms")
	interrupted, cancel := context.WithCancel(context.Background())
	cancel()
	worker.out.Reset()
	err := worker.app.RunContext(interrupted, []string{"skl", "implement", "dispatch", "--repo", source, "--after", searchClaim, "--wait", "1m"})
	g.check("outcome-implement-next-dispatch-interrupted", worker.out.String()+err.Error()+"\n")
	g.run(worker, "implement", "release", "--repo", source, "--slice", "widget-share/foundation", "--claim", shareClaim)
	continued := g.capture("outcome-implement-next-dispatch-continued", worker, shellWords(t, continuation)[1:]...)
	shareClaim = proseMatch(t, dispatchClaim, continued)
	g.run(worker, "implement", "release", "--repo", source, "--slice", "widget-share/foundation", "--claim", shareClaim)
	g.capture("outcome-implement-next-dispatch-released", worker, "implement", "dispatch", "--repo", source, "--after", shareClaim)
	g.capture("outcome-implement-next-dispatch-refused", worker, "implement", "dispatch", "--repo", source, "--after", "not-a-claim")

	// Amend: an unchanged export, a refused change to submitted work, then a
	// Slice added behind the released widget-share/foundation, whose Contract
	// is replaced next.
	exports := 0
	amendIntake := func(proposal string) string {
		t.Helper()
		exports++
		directory := filepath.Join(temp, fmt.Sprintf("amend-%d", exports), proposal)
		g.run(proposer, "proposal", "export", "--repo", source, "--proposal", proposal, "--to", directory)
		return directory
	}
	g.capture("outcome-ledger-amend-existing", proposer, "proposal", "amend", "--repo", source, "--proposal-dir", amendIntake("widget-share"))
	submittedIntake := amendIntake("widget-search")
	writeFile(t, filepath.Join(submittedIntake, "foundation", "intent.md"), "# Search intent, revised\n")
	g.capture("outcome-ledger-amend-fix-required", proposer, "proposal", "amend", "--repo", source, "--proposal-dir", submittedIntake)
	grown := amendIntake("widget-share")
	addIntakeSlice(t, grown, "followup", "foundation")
	if output := g.run(proposer, "proposal", "amend", "--repo", source, "--proposal-dir", grown,
		"--issue=followup="+proseFixturePath("issue.md"), "--parent-body", proseFixturePath("issue.md")); !strings.Contains(output, "Status: amended") {
		t.Fatalf("proposal amend widget-share:\n%s", output)
	}
	replaced := amendIntake("widget-share")
	writeFile(t, filepath.Join(replaced, "foundation", "behavior.md"), "# Share behavior, revised\n")
	g.capture("outcome-ledger-amend-amended", proposer, "proposal", "amend", "--repo", source, "--proposal-dir", replaced)

}

func captureProseStandalone(t *testing.T, g *proseGoldens) {
	source, _ := deliverySourceRepo(t)
	g.replace(source, "/work/widgets")
	worker := deliveryNoForgeApp(t)
	// Resource inputs are rendering parameters, not delivery history. Their
	// bound commands are executed separately by TestRenderedResourceCommands.
	result, reviewResult := filepath.Join(t.TempDir(), "implement"), filepath.Join(t.TempDir(), "watchdog")
	head := strings.Repeat("a", 40)
	g.replace(result, "/tmp/skl-implement-result")
	g.replace(reviewResult, "/tmp/skl-watchdog-result")
	missing := &ledgerFixture{config: t.TempDir(), home: t.TempDir()}
	// Preserve the existing missing-config path placeholder, not its incidental
	// TempDir allocation number in the old combined journey.
	g.replace(missing.config, "/tmp/test/018")
	g.capture("outcome-implement-next-unconfigured", missing.app(worker), "implement", "next", "--repo", source)
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
		"--input", "proposal="+proseProposal, "--input", "project=widgets", "--input", "repo="+source, "--input", "remote=origin", "propose")
	g.capture("resource-implement-ledger-submission", worker, "skill", "--resource", "ledger-submission.md",
		"--input", "result_directory="+result, "--input", "procedure=initial", "--input", "review_count=0", "--input", "mode=standard", "implement")
	g.capture("resource-watchdog-ledger-review", worker, "skill", "--resource", "ledger-review.md",
		"--input", "result_directory="+reviewResult, "--input", "round=1", "--input", "reviewed_head="+head, "--input", "rework_pauses=false", "watchdog")

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
		operations := []string{"implement", "implement-team", "watchdog"}
		operations = append(operations, "implement-loop", "implement-team-loop", "watchdog-loop", "implement-auto", "implement-team-auto", "watchdog-auto")
		for _, operation := range operations {
			g.check("adapter-"+harness+"-"+operation, readFileString(t, filepath.Join(home, fmt.Sprintf(location, operation))))
		}
	}
	g.check("setup-agents-block", setup.AgentsBlock)

}

func captureProseReadiness(t *testing.T, g *proseGoldens) {
	// A fresh Auto poll reconciles a stranded Slice without its worker prose.
	recovery := newAutoMergeFixture(t, true)
	recovery.pass(t, false, false)
	g.replace(recovery.source, "<recovery-source>")
	g.capture("outcome-watchdog-auto-readiness", recovery.ledger.app(presentForgeApp(t, recovery.pulls)), "watchdog", "next", "--auto", "--repo", recovery.source)
}
