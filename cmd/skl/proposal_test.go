package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestProposalRejectsRemovedPathsAndInapplicableFlags(t *testing.T) {
	fixture := newLedgerFixture(t)
	head := ledgerHead(t, fixture.clone)
	for _, args := range [][]string{
		{"ledger", "accept"},
		{"ledger", "export"},
		{"ledger", "amend"},
		{"ledger", "publish"},
		{"decision", "retire"},
		{"proposal", "retire", "--repo", "."},
		{"proposal", "retire", "--remote", "origin"},
		{"proposal", "retire", "--issue", "foundation=body.md"},
		{"proposal", "accept", "--project", "widgets"},
		{"proposal", "export", "--issue", "foundation=body.md"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			command := exec.Command(os.Args[0], append([]string{"-test.run=^TestProposalSyntaxProcess$", "--"}, args...)...)
			command.Env = append(os.Environ(), "SKL_PROPOSAL_SYNTAX_PROCESS=1")
			output, err := command.CombinedOutput()
			wantCode, wantError := 1, "flag provided but not defined"
			if args[0] != "proposal" {
				wantCode, wantError = 3, "No help topic"
			}
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != wantCode || !strings.Contains(string(output), wantError) {
				t.Fatalf("syntax refusal = %v, %s; want exit %d and %q", err, output, wantCode, wantError)
			}
			if json.Valid(output) {
				t.Fatalf("syntax error became a structured operation outcome: %s", output)
			}
			requireLedgerUnchanged(t, fixture.clone, head)
		})
	}
}

func TestProposalSyntaxProcess(t *testing.T) {
	if os.Getenv("SKL_PROPOSAL_SYNTAX_PROCESS") != "1" {
		return
	}
	os.Args = append([]string{"skl"}, os.Args[3:]...)
	main()
}

func TestProposalRetirementRequiresExplicitProjectAsStructuredRefusal(t *testing.T) {
	fixture := newLedgerFixture(t)
	root := sourceRepository(t, "acme", "widgets")
	cli := newLedgerApp(t, newForgeServer(t))
	if got := cli.accept(t, root, writeProposal(t, "", singleSlice("selected-work"))); got.Status != "accepted" {
		t.Fatalf("accept: %s", mustJSON(t, got))
	}
	head := ledgerHead(t, fixture.clone)
	// Even in the accepted Project's checkout, omission remains a semantic
	// refusal, not a parser failure or an inferred Project selection.
	t.Chdir(root)
	outcome := decisionReadOnlyApp(t).decisionJSON(t, "skl", "proposal", "retire", "--proposal", "selected-work", "--format", "json")
	if outcome.Status != "refused" || outcome.Facts == nil || !strings.Contains(outcome.Facts.Reason, "--project") {
		t.Fatalf("retirement without explicit scope: %#v", outcome)
	}
	requireLedgerUnchanged(t, fixture.clone, head)
}

func TestProposalHelpExposesCanonicalFormsAndFlags(t *testing.T) {
	for _, tc := range []struct {
		operation string
		flags     []string
	}{
		{"accept", []string{"--repo", "--remote", "--proposal-dir", "--issue", "--parent-body", "--format"}},
		{"export", []string{"--repo", "--remote", "--proposal", "--to", "--format"}},
		{"amend", []string{"--repo", "--remote", "--proposal-dir", "--issue", "--parent-body", "--format"}},
		{"publish", []string{"--repo", "--remote", "--proposal", "--issue", "--parent-body", "--format"}},
		{"retire", []string{"--project", "--proposal", "--format"}},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			var output bytes.Buffer
			app := newApp(nil, bytes.NewReader(nil), &output, io.Discard)
			if err := app.Run([]string{"skl", "proposal", tc.operation, "--help"}); err != nil {
				t.Fatal(err)
			}
			for _, want := range append(tc.flags, "skl proposal "+tc.operation, "markdown", "json") {
				if !strings.Contains(output.String(), want) {
					t.Errorf("help lacks %q:\n%s", want, output.String())
				}
			}
			if tc.operation == "retire" && strings.Contains(output.String(), "--repo") {
				t.Fatalf("retirement help offers inferred source scope: %s", output.String())
			}
		})
	}
}
