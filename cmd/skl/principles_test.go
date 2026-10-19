package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

const (
	principleA = "---\nstatement: One native representation; no duplicate payload graph beside it.\n---\n\n# One representation\n\nKeep one shape.\n"
	principleB = "---\nstatement: No compatibility facade over a replaced interface.\n---\n\n# No facade\n\nReplace callers instead.\n"
	// principlesBlock is the block a rendering for a repository with
	// principles a and b carries, in file-name order.
	principlesBlock = "## Project principles\n\nThis repository's principles apply to this work. Read one in full with `skl skill --resource <name>.md principles`.\n\n" +
		"- `a`: One native representation; no duplicate payload graph beside it.\n- `b`: No compatibility facade over a replaced interface.\n"
)

// principledRepository commits principles a and b, and a non-principle file
// the reader ignores, to a fresh source repository.
func principledRepository(t *testing.T) string {
	t.Helper()
	root, _ := deliverySourceRepo(t)
	writeFile(t, filepath.Join(root, ".skl", "principles", "b.md"), principleB)
	writeFile(t, filepath.Join(root, ".skl", "principles", "a.md"), principleA)
	writeFile(t, filepath.Join(root, ".skl", "principles", "notes.txt"), "not a principle\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "principles")
	runGit(t, root, "update-ref", "refs/remotes/origin/main", deliveryTrimmed(t, root, "rev-parse", "HEAD"))
	return root
}

func skillOutput(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := newApp(nil, strings.NewReader(""), &output, &output).Run(append([]string{"skl", "skill"}, args...))
	return output.String(), err
}

func requireBlockOnce(t *testing.T, what, rendering string) {
	t.Helper()
	if strings.Count(rendering, principlesBlock) != 1 || strings.Count(rendering, "## Project principles") != 1 {
		t.Fatalf("%s does not carry the principles block once:\n%s", what, rendering)
	}
}

// Every Procedure rendered for a repository with principles lists their
// statements once; the principles themselves carry no block.
func TestProjectPrinciplesReachEveryProcedure(t *testing.T) {
	newLedgerFixture(t)
	source := principledRepository(t)
	files := lintedContract()
	files["plan.md"] = strings.Replace(files["plan.md"], "- A second representation.", "- A second representation (principle:a).", 1)
	if outcome := newLedgerApp(t, newForgeServer(t)).accept(t, source, writeProposal(t, "", lintSpec(deliveryTestProposal, files))); outcome.Status != "accepted" {
		t.Fatalf("accept citing an existing principle: %s", mustJSON(t, outcome))
	}
	cli := deliveryNoForgeApp(t)
	started, err := cli.deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || started.Packet == nil {
		t.Fatalf("implement next: %#v %v", started, err)
	}
	requireBlockOnce(t, "implement", started.Packet.Instructions)
	if !strings.Contains(started.Packet.Instructions, "Tell the Contracts reviewer to read each principle a Forbidden line cites") {
		t.Fatalf("the audit step does not send the Contracts reviewer to cited principles:\n%s", started.Packet.Instructions)
	}
	if principles := started.Packet.Facts.Principles; len(principles) != 2 || principles[0].Name != "a" || principles[1].Name != "b" {
		t.Fatalf("implement facts = %#v", principles)
	}
	claim := started.Execution.Claim.Commit
	prepared, err := cli.deliveryJSON(t, "skl", "implement", "prepare", "--repo", source, "--slice", deliveryTestItem, "--claim", claim, "--format", "json")
	if err != nil || prepared.Packet == nil || prepared.Source == nil {
		t.Fatalf("implement prepare: %#v %v", prepared, err)
	}
	requireBlockOnce(t, "implement prepare", prepared.Packet.Instructions)
	worktree := prepared.Source.Worktree
	writeFile(t, filepath.Join(worktree, "feature.txt"), "the rule\n")
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "feature")
	body := filepath.Join(t.TempDir(), "report.md")
	writeFile(t, body, "# Implementation result\n")
	if submitted, err := cli.deliveryJSON(t, "skl", "implement", "submit", "--repo", source, "--slice", deliveryTestItem, "--claim", claim,
		"--head", deliveryTrimmed(t, worktree, "rev-parse", "HEAD"), "--target", deliveryTrimmed(t, source, "rev-parse", "HEAD"), "--body", body, "--format", "json"); err != nil || submitted.Status != ledger.AwaitingReview {
		t.Fatalf("implement submit: %#v %v", submitted, err)
	}
	review, err := cli.deliveryJSON(t, "skl", "watchdog", "next", "--repo", source, "--format", "json")
	if err != nil || review.Packet == nil {
		t.Fatalf("watchdog next: %#v %v", review, err)
	}
	requireBlockOnce(t, "watchdog", review.Packet.Instructions)
	if !strings.Contains(review.Packet.Instructions, "Read each principle a Forbidden line cites as `principle:<name>` with `skl skill --resource <name>.md principles` before judging that line.") {
		t.Fatalf("Watchdog does not send the reviewer to cited principles:\n%s", review.Packet.Instructions)
	}

	for _, name := range []string{"audit", "brainstorm", "decision", "explore", "principles", "propose", "verify-create", "verify-maintain", "walkthrough"} {
		rendered, err := skillOutput(t, "--repo", source, name)
		if err != nil {
			t.Fatalf("skl skill %s: %v", name, err)
		}
		requireBlockOnce(t, name, rendered)
	}
	for _, name := range []string{"domain", "modules", "testing", "writing-for-agents"} {
		if rendered, err := skillOutput(t, "--repo", source, name); err != nil || strings.Contains(rendered, "Project principles") {
			t.Fatalf("principle %s carries the block: %v\n%s", name, err, rendered)
		}
	}
}

// A repository without principles renders no block, and its principles
// skill says it has none.
func TestNoProjectPrinciplesRenderNoBlock(t *testing.T) {
	source, _ := deliverySourceRepo(t)
	for _, name := range []string{"verify-maintain", "principles"} {
		rendered, err := skillOutput(t, "--repo", source, name)
		if err != nil || strings.Contains(rendered, "## Project principles") {
			t.Fatalf("skl skill %s without principles: %v\n%s", name, err, rendered)
		}
	}
	if rendered, _ := skillOutput(t, "--repo", source, "principles"); !strings.Contains(rendered, "it has none") {
		t.Fatalf("principles without any:\n%s", rendered)
	}
}

// A principle without a valid statement refuses every rendering for its
// repository, naming the file and the fault.
func TestInvalidProjectPrincipleRefusesRendering(t *testing.T) {
	newLedgerFixture(t)
	source := principledRepository(t)
	if outcome := newLedgerApp(t, newForgeServer(t)).accept(t, source, writeProposal(t, "", lintSpec(deliveryTestProposal, lintedContract()))); outcome.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, outcome))
	}
	writeFile(t, filepath.Join(source, ".skl", "principles", "broken.md"), "---\nowner: nobody\n---\n\nNo statement.\n")
	runGit(t, source, "add", ".")
	runGit(t, source, "commit", "-q", "-m", "broken principle")
	runGit(t, source, "update-ref", "refs/remotes/origin/main", deliveryTrimmed(t, source, "rev-parse", "HEAD"))

	if _, err := skillOutput(t, "--repo", source, "verify-maintain"); err == nil || !strings.Contains(err.Error(), "broken.md") || !strings.Contains(err.Error(), "statement") {
		t.Fatalf("verify-maintain with a broken principle: %v", err)
	}
	refused, err := deliveryNoForgeApp(t).deliveryJSON(t, "skl", "implement", "next", "--repo", source, "--format", "json")
	if err != nil || refused.Status != "fix_required" || !strings.Contains(refused.Reason, "broken.md") || !strings.Contains(refused.Reason, "statement") {
		t.Fatalf("implement with a broken principle: %#v %v", refused, err)
	}
}

// A principle's full file is a resource of the principles skill; an unknown
// one is refused with the names present.
func TestProjectPrincipleResources(t *testing.T) {
	source := principledRepository(t)
	if rendered, err := skillOutput(t, "--repo", source, "--resource", "a.md", "principles"); err != nil || rendered != principleA {
		t.Fatalf("resource a.md = %v\n%q", err, rendered)
	}
	if rendered, err := skillOutput(t, "--repo", source, "--resource", "a.md", "--describe-inputs", "principles"); err != nil || rendered != "a.md accepts no inputs.\n" {
		t.Fatalf("describe a.md = %v\n%q", err, rendered)
	}
	for _, args := range [][]string{
		{"--repo", source, "--resource", "zzz.md", "principles"},
		{"--repo", source, "--resource", "zzz.md", "--describe-inputs", "principles"},
	} {
		if _, err := skillOutput(t, args...); err == nil || !strings.Contains(err.Error(), `"zzz"`) || !strings.Contains(err.Error(), "a, b") {
			t.Fatalf("%v = %v", args, err)
		}
	}
	if _, err := skillOutput(t, "--repo", source, "--resource", "a.md", "--input", "x=1", "principles"); err == nil || !strings.Contains(err.Error(), `unknown input "x"`) {
		t.Fatalf("resource input = %v", err)
	}
	if _, err := skillOutput(t, "--repo", t.TempDir(), "--resource", "a.md", "principles"); err == nil || !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Fatalf("principles outside a repository = %v", err)
	}
	if _, err := skillOutput(t, "--repo", source, "--resource", "tests.md", "testing"); err == nil || !strings.Contains(err.Error(), "--repo applies") {
		t.Fatalf("--repo with another skill's resource = %v", err)
	}
}
