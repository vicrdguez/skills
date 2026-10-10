package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/ledger"
)

// exportProposal runs skl ledger export and returns the parsed JSON outcome.
func (c ledgerCLI) exportProposal(t *testing.T, repo, proposal, destination string) ledgerOutcome {
	t.Helper()
	outcome, _ := c.run(t, []string{"skl", "ledger", "export", "--repo", repo, "--proposal", proposal, "--to", destination, "--format", "json"})
	return outcome
}

// directoryFiles maps every file under root, by its relative path, to its
// contents.
func directoryFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = readFileString(t, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// exportedDeclaration decodes an exported proposal.json generically, so the
// test sees exactly which fields it carries.
func exportedDeclaration(t *testing.T, directory string) map[string]any {
	t.Helper()
	return decodeRecord(t, readFileString(t, filepath.Join(directory, "proposal.json")))
}

func exportedSlice(t *testing.T, declaration map[string]any, name string) map[string]any {
	t.Helper()
	slices, _ := declaration["slices"].([]any)
	for _, slice := range slices {
		if entry, _ := slice.(map[string]any); entry["name"] == name {
			return entry
		}
	}
	t.Fatalf("exported declaration lacks slice %s: %v", name, declaration)
	return nil
}

func requireLedgerUnchanged(t *testing.T, clone, head string) {
	t.Helper()
	if after := ledgerHead(t, clone); after != head {
		t.Fatalf("export moved the ledger head from %s to %s", head, after)
	}
	if status := runGitOutput(t, clone, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Fatalf("export left changes in the ledger clone:\n%s", status)
	}
}

func TestExportedProposalReacceptsUnchanged(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	if outcome := cli.accept(t, root, writeProposal(t, "", singleSlice("earlier-work"))); outcome.Status != "accepted" {
		t.Fatalf("setup acceptance failed: %s", mustJSON(t, outcome))
	}
	spec := cleanupSpec("trio", "alpha", "beta", "gamma")
	spec.slices[1].files["plan.md"] = "# Beta plan\n\nNo trailing newline at the end"
	spec.depends = map[string][]string{"beta": {"alpha"}, "gamma": {"proposals/earlier-work/foundation", "proposals/trio/beta"}}
	intake := writeProposal(t, "", spec)
	if outcome := cli.accept(t, root, intake); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	head := ledgerHead(t, fixture.clone)

	first := filepath.Join(t.TempDir(), "absent", "trio")
	outcome := cli.exportProposal(t, root, "trio", first)
	if outcome.Status != "exported" || outcome.Export == nil {
		t.Fatalf("export failed: %s", mustJSON(t, outcome))
	}
	if outcome.Export.Commit != head || outcome.Export.Destination != first {
		t.Fatalf("export names commit %s and destination %s, want %s and %s", outcome.Export.Commit, outcome.Export.Destination, head, first)
	}
	requireLedgerUnchanged(t, fixture.clone, head)

	// The description and every Contract file are the accepted bytes.
	exported := directoryFiles(t, first)
	want := directoryFiles(t, intake)
	delete(want, "proposal.json")
	delete(exported, "proposal.json")
	if mustJSON(t, exported) != mustJSON(t, want) {
		t.Fatalf("exported files differ from the accepted intake:\ngot  %v\nwant %v", exported, want)
	}
	declaration := exportedDeclaration(t, first)
	if declaration["proposal"] != "trio" || declaration["parent_title"] != "Deliver trio" || declaration["branch"] != "proposal/trio" {
		t.Fatalf("exported declaration lacks the recorded Proposal facts: %v", declaration)
	}
	gamma := exportedSlice(t, declaration, "gamma")
	if gamma["title"] != "Deliver trio gamma" || gamma["branch"] != "trio-gamma" || len(gamma["depends"].([]any)) != 2 {
		t.Fatalf("exported slice gamma lacks its recorded facts: %v", gamma)
	}
	if _, flagged := gamma["superseded"]; flagged {
		t.Fatalf("a live slice is exported as superseded: %v", gamma)
	}

	// A second export of the same revision is identical, into an empty directory.
	second := t.TempDir()
	if outcome := cli.exportProposal(t, root, "trio", second); outcome.Status != "exported" {
		t.Fatalf("second export failed: %s", mustJSON(t, outcome))
	}
	if a, b := directoryFiles(t, first), directoryFiles(t, second); mustJSON(t, a) != mustJSON(t, b) {
		t.Fatalf("two exports of one revision differ:\n%v\n%v", a, b)
	}

	if outcome := cli.accept(t, root, first); outcome.Status != "existing" {
		t.Fatalf("the export did not re-accept unchanged: %s", mustJSON(t, outcome))
	}
	requireLedgerUnchanged(t, fixture.clone, head)
}

func TestExportOmitsAnUnrecordedProposalBranch(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	if outcome := cli.accept(t, root, writeProposal(t, "", singleSlice("historical"))); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	// A Proposal accepted before the Proposal Branch fact was recorded.
	metaPath := "projects/widgets/proposals/historical/proposal.json"
	meta := decodeRecord(t, readLedgerFile(t, fixture.clone, metaPath))
	delete(meta, "branch")
	commitLedgerRecord(t, fixture.clone, metaPath, mustJSON(t, meta)+"\n")

	destination := t.TempDir()
	if outcome := cli.exportProposal(t, root, "historical", destination); outcome.Status != "exported" {
		t.Fatalf("export failed: %s", mustJSON(t, outcome))
	}
	declaration := exportedDeclaration(t, destination)
	for _, field := range []string{"branch", "parent_title"} {
		if _, present := declaration[field]; present {
			t.Fatalf("export carries %s the record lacks: %v", field, declaration)
		}
	}
	if outcome := cli.accept(t, root, destination); outcome.Status != "existing" {
		t.Fatalf("the export did not re-accept unchanged: %s", mustJSON(t, outcome))
	}
}

func TestExportCarriesMergedAndSupersededSlices(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	spec := cleanupSpec("mixed", "done", "old", "next")
	if outcome := cli.accept(t, root, writeProposal(t, "", spec)); outcome.Status != "accepted" {
		t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
	}
	statusRecord(t, fixture.clone, "mixed", "done", merged(""))
	statusRecord(t, fixture.clone, "mixed", "old", lifecycle(ledger.Superseded))
	head := ledgerHead(t, fixture.clone)

	destination := t.TempDir()
	if outcome := cli.exportProposal(t, root, "mixed", destination); outcome.Status != "exported" {
		t.Fatalf("export failed: %s", mustJSON(t, outcome))
	}
	declaration := exportedDeclaration(t, destination)
	flags := map[string]any{}
	for _, name := range []string{"done", "old", "next"} {
		flags[name] = exportedSlice(t, declaration, name)["superseded"]
	}
	if flags["old"] != true || flags["done"] != nil || flags["next"] != nil {
		t.Fatalf("superseded flags = %v, want only old", flags)
	}
	if outcome := cli.accept(t, root, destination); outcome.Status != "existing" {
		t.Fatalf("the export did not re-accept unchanged: %s", mustJSON(t, outcome))
	}

	// The superseded flag is compared like every other declared fact.
	if outcome := cli.accept(t, root, writeProposal(t, "", spec)); outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "superseded") || !strings.Contains(outcome.Reason, "old") {
		t.Fatalf("an intake omitting the recorded supersession was not refused: %s", mustJSON(t, outcome))
	}
	live := cleanupSpec("mixed", "done", "old", "next")
	live.slices[1].superseded = true
	live.slices[2].superseded = true
	if outcome := cli.accept(t, root, writeProposal(t, "", live)); outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "next") {
		t.Fatalf("an intake superseding a live slice was not refused: %s", mustJSON(t, outcome))
	}
	requireLedgerUnchanged(t, fixture.clone, head)
}

func TestNewAcceptanceRefusesDeclaredSupersession(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	head := ledgerHead(t, fixture.clone)
	spec := cleanupSpec("fresh", "keep", "drop")
	spec.slices[1].superseded = true
	outcome := cli.accept(t, root, writeProposal(t, "", spec))
	if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, "drop") || !strings.Contains(outcome.Reason, "never supersedes") {
		t.Fatalf("a new Proposal declaring supersession was not refused: %s", mustJSON(t, outcome))
	}
	requireLedgerUnchanged(t, fixture.clone, head)
	if paths := ledgerPaths(t, fixture.clone); len(paths) != 1 || paths[0] != "README.md" {
		t.Fatalf("refused acceptance wrote records: %v", paths)
	}
}

func TestExportRefusalsWriteNothing(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	for _, name := range []string{"active", "finished"} {
		if outcome := cli.accept(t, root, writeProposal(t, "", cleanupSpec(name, "only"))); outcome.Status != "accepted" {
			t.Fatalf("acceptance failed: %s", mustJSON(t, outcome))
		}
	}
	statusRecord(t, fixture.clone, "finished", "only", merged(""))
	if outcome := runCleanup(t, offlineForge(t), root); len(archivedNames(outcome)) != 1 {
		t.Fatalf("setup did not archive finished: %s", mustJSON(t, outcome))
	}
	head := ledgerHead(t, fixture.clone)

	occupied := t.TempDir()
	writeFile(t, filepath.Join(occupied, "notes.md"), "keep me\n")
	cases := []struct {
		name, proposal, destination, reason, repair string
	}{
		{"unknown proposal", "missing", filepath.Join(t.TempDir(), "out"), "no active Proposal missing", "skl browse project"},
		{"archived proposal", "finished", filepath.Join(t.TempDir(), "out"), "archived", "skl browse proposal --project widgets --proposal finished --archived"},
		{"occupied destination", "active", occupied, "not empty", "absent or empty"},
		{"destination in the ledger", "active", filepath.Join(fixture.clone, "export"), "inside the ledger clone", "outside the ledger clone"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, statErr := os.Stat(test.destination)
			absent := os.IsNotExist(statErr)
			before := map[string]string{}
			if !absent {
				before = directoryFiles(t, test.destination)
			}
			outcome := cli.exportProposal(t, root, test.proposal, test.destination)
			if outcome.Status != "fix_required" || !strings.Contains(outcome.Reason, test.reason) || !strings.Contains(outcome.Repair, test.repair) {
				t.Fatalf("export was not refused with its repair: %s", mustJSON(t, outcome))
			}
			if absent {
				if _, err := os.Stat(test.destination); !os.IsNotExist(err) {
					t.Fatalf("refused export created %s", test.destination)
				}
			} else if after := directoryFiles(t, test.destination); mustJSON(t, after) != mustJSON(t, before) {
				t.Fatalf("refused export changed the destination: %v", after)
			}
			requireLedgerUnchanged(t, fixture.clone, head)
		})
	}
}
