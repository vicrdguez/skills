package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
)

func TestStatusCommandIsRemoved(t *testing.T) {
	for _, args := range [][]string{{"status"}, {"status", "--item", "proposal/slice"}} {
		command := exec.Command(os.Args[0], append([]string{"-test.run=^TestRemovedStatusProcess$", "--"}, args...)...)
		command.Env = append(os.Environ(), "SKL_REMOVED_STATUS_PROCESS=1")
		output, err := command.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "No help topic for 'status'") {
			t.Fatalf("removed invocation = %v, %q", err, output)
		}
	}
}

func TestRemovedStatusProcess(t *testing.T) {
	if os.Getenv("SKL_REMOVED_STATUS_PROCESS") != "1" {
		return
	}
	os.Args = append([]string{"skl"}, os.Args[3:]...)
	main()
}

func TestReconcileRejectsRemovedAndInapplicableSyntax(t *testing.T) {
	root := sourceRepository(t, "acme", "widgets")
	for _, args := range [][]string{
		{"reconcile", "--item", "proposal/slice"},
		{"reconcile", "--auto"},
		{"reconcile", "proposal/slice"},
		{"reconcile", "--format", "raw"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var output bytes.Buffer
			app := newApp(func(github.RepositoryID) (setup.Backend, error) {
				t.Fatal("invalid syntax contacted the forge")
				return nil, nil
			}, bytes.NewReader(nil), &output, &output)
			if err := app.Run(append([]string{"skl"}, append(args, "--repo", root)...)); err == nil {
				t.Fatalf("removed or inapplicable syntax was accepted: %v", args)
			}
			if strings.Contains(output.String(), `"status":"fix_required"`) {
				t.Fatalf("syntax error became a structured refusal: %s", &output)
			}
		})
	}
}

func TestReconcileHelpExposesCanonicalFlags(t *testing.T) {
	var output bytes.Buffer
	app := newApp(nil, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "reconcile", "--help"}); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--slice", "--repo", "--remote", "--format"} {
		if !strings.Contains(output.String(), flag) {
			t.Fatalf("help omits %s: %s", flag, &output)
		}
	}
	if strings.Contains(output.String(), "--item") {
		t.Fatalf("help exposes removed flag: %s", &output)
	}
	output.Reset()
	if err := app.Run([]string{"skl", "--help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "reconcile") || strings.Contains(output.String(), "\n   status") {
		t.Fatalf("top-level help exposes the wrong command: %s", &output)
	}
}

func TestReconcileUnknownSliceRemainsStructuredRefusal(t *testing.T) {
	newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	newLedgerApp(t, newForgeServer(t)).accept(t, root, writeProposal(t, "", singleSlice("known")))
	var output bytes.Buffer
	app := newApp(func(github.RepositoryID) (setup.Backend, error) {
		t.Fatal("unknown Slice contacted the forge")
		return nil, nil
	}, bytes.NewReader(nil), &output, &output)
	if err := app.Run([]string{"skl", "reconcile", "--repo", root, "--slice", "known/absent", "--format", "json"}); err != nil {
		t.Fatalf("semantic refusal became a parser error: %v", err)
	}
	var outcome ledgerOutcome
	if err := json.Unmarshal(output.Bytes(), &outcome); err != nil || outcome.Status != "fix_required" || outcome.Reason == "" || outcome.Repair == "" {
		t.Fatalf("structured refusal lost: %v %s", err, &output)
	}
}

func TestClaimInspectionCommandRunsWithBoundRepositoryAndRemote(t *testing.T) {
	newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	runGit(t, root, "remote", "rename", "origin", "upstream")
	command := statusInvocation(setup.RepositoryContext{Root: root, Remote: "upstream"})
	args := shellWords(t, command)
	if len(args) != 6 || args[1] != "reconcile" || args[3] != root || args[5] != "upstream" {
		t.Fatalf("generated command lost canonical syntax or bound facts: %s", command)
	}
	var output bytes.Buffer
	app := newApp(nil, bytes.NewReader(nil), &output, &output)
	if err := app.Run(args); err != nil || !strings.Contains(output.String(), "fix_required") {
		t.Fatalf("bound command cannot run: %v %s", err, &output)
	}
}
