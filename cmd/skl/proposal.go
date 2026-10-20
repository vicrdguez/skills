package main

import (
	"fmt"
	"io"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// Proposal operations adapt the canonical CLI to the existing ledger effects.
func proposalCommands(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:  "proposal",
		Usage: "Accept, export, amend, publish descriptive issues, or retire a Proposal",
		Subcommands: []*cli.Command{{
			Name:        "accept",
			Usage:       "Freeze one prepared proposal into the local Workflow Ledger and attempt initial publication",
			Description: "Commit accepted Contracts locally, then attempt ledger replication and descriptive issue publication. Unchanged acceptance reports the existing Proposal.",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: ".", Usage: "Source repository path (default: current directory)"},
				&cli.StringFlag{Name: "remote", Usage: "Source repository remote (default: origin)"},
				&cli.PathFlag{Name: "proposal-dir", Required: true, Usage: "Prepared intake directory with proposal.json and Contract files"},
				newIssueBodyFlag(),
				&cli.PathFlag{Name: "parent-body", Usage: "Path to freshly authored parent issue prose for multi-slice work"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				rerun := boundCommand(command, "skl proposal accept")
				refuse := func(err error) error { return refuseLedger(stdout, format, rerun, err) }
				bodies, parentBody, err := issueBodyInputs(command)
				if err != nil {
					return err
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "fix_required", Reason: "repository or remote resolution failed: " + err.Error(), Repair: "run acceptance inside the source repository, or pass --repo and --remote explicitly", rerun: rerun})
				}
				declaration, err := ledger.LoadDeclaration(command.Path("proposal-dir"), bodies, parentBody)
				if err != nil {
					return refuse(err)
				}
				store, failure := openConfiguredLedger(command)
				if failure != nil {
					return refuse(failure)
				}
				if err := store.RefuseSourceOverlap(repository.Root); err != nil {
					return refuse(err)
				}
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return refuse(err)
				}
				forge, ok := backend.(ledger.Forge)
				if !ok {
					return fmt.Errorf("workflow backend does not support ledger issue publication")
				}
				acceptance, err := ledger.Accept(command.Context, store, repository.Repository, declaration, forge, time.Now)
				if err != nil {
					return refuse(err)
				}
				authoring := authoringFor(repository, acceptance)
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: acceptance.Status, Acceptance: acceptance, Authoring: authoring,
					kind: "ledger-accept", facts: proposalFactsOf(repository, acceptance, authoring)})
			},
		}, {
			Name:        "amend",
			Usage:       "Record an edited intake of one accepted proposal as one ledger revision and publish the changed issues",
			Description: "Record eligible changes atomically, then attempt ledger replication and descriptive issue publication. Claimed Contracts remain frozen.",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: ".", Usage: "Source repository path (default: current directory)"},
				&cli.StringFlag{Name: "remote", Usage: "Source repository remote (default: origin)"},
				&cli.PathFlag{Name: "proposal-dir", Required: true, Usage: "Prepared intake directory with proposal.json and Contract files"},
				newIssueBodyFlag(),
				&cli.PathFlag{Name: "parent-body", Usage: "Path to freshly authored parent issue prose for multi-slice work"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				rerun := boundCommand(command, "skl proposal amend")
				refuse := func(err error) error { return refuseLedger(stdout, format, rerun, err) }
				bodies, parentBody, err := issueBodyInputs(command)
				if err != nil {
					return err
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "fix_required", Reason: "repository or remote resolution failed: " + err.Error(), Repair: "run the amendment inside the source repository, or pass --repo and --remote explicitly", rerun: rerun})
				}
				declaration, err := ledger.LoadDeclaration(command.Path("proposal-dir"), bodies, parentBody)
				if err != nil {
					return refuse(err)
				}
				store, failure := openConfiguredLedger(command)
				if failure != nil {
					return refuse(failure)
				}
				if err := store.RefuseSourceOverlap(repository.Root); err != nil {
					return refuse(err)
				}
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return refuse(err)
				}
				forge, ok := backend.(ledger.Forge)
				if !ok {
					return fmt.Errorf("workflow backend does not support ledger issue publication")
				}
				amendment, err := ledger.Amend(command.Context, store, repository.Repository, declaration, forge)
				if err != nil {
					return refuse(err)
				}
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: amendment.Status, Amendment: amendment,
					kind: "ledger-amend", facts: amendFactsOf(repository, amendment)})
			},
		}, {
			Name:        "publish",
			Usage:       "Publish the current descriptive issue and parent presentation of one accepted proposal",
			Description: "Create or update descriptive issues from fresh authored bodies; missing bodies are reported for later publication. This does not manage the Proposal Submission pull request.",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: ".", Usage: "Source repository path (default: current directory)"},
				&cli.StringFlag{Name: "remote", Usage: "Source repository remote (default: origin)"},
				&cli.StringFlag{Name: "proposal", Required: true, Usage: "Accepted Proposal name"},
				newIssueBodyFlag(),
				&cli.PathFlag{Name: "parent-body", Usage: "Path to freshly authored parent issue prose for multi-slice work"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				rerun := boundCommand(command, "skl proposal publish")
				refuse := func(err error) error { return refuseLedger(stdout, format, rerun, err) }
				bodies, parentBody, err := issueBodyInputs(command)
				if err != nil {
					return err
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "fix_required", Reason: "repository or remote resolution failed: " + err.Error(), Repair: "run publication inside the source repository, or pass --repo and --remote explicitly", rerun: rerun})
				}
				store, failure := openConfiguredLedger(command)
				if failure != nil {
					return refuse(failure)
				}
				if err := store.RefuseSourceOverlap(repository.Root); err != nil {
					return refuse(err)
				}
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return refuse(err)
				}
				forge, ok := backend.(ledger.Forge)
				if !ok {
					return fmt.Errorf("workflow backend does not support ledger issue publication")
				}
				publication, err := ledger.PublishCurrent(command.Context, store, repository.Repository, command.String("proposal"), ledger.IssueProse{Bodies: bodies, Parent: parentBody}, forge)
				if err != nil {
					return refuse(err)
				}
				authoring := authoringFor(repository, publication)
				return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: publication.Status, Publication: publication, Authoring: authoring,
					kind: "ledger-publish", facts: proposalFactsOf(repository, publication, authoring)})
			},
		}, {
			Name:        "export",
			Usage:       "Write one active accepted proposal back out as an intake directory",
			Description: "Read the committed ledger and export exact Contract bytes to an absent or empty directory. No ledger changes or forge contact.",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: ".", Usage: "Source repository path (default: current directory)"},
				&cli.StringFlag{Name: "remote", Usage: "Source repository remote (default: origin)"},
				&cli.StringFlag{Name: "proposal", Required: true, Usage: "Accepted Proposal name"},
				&cli.PathFlag{Name: "to", Required: true, Usage: "Absent or empty destination directory"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				rerun := boundCommand(command, "skl proposal export")
				refuse := func(err error) error { return refuseLedger(stdout, format, rerun, err) }
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return refuse(err)
				}
				store, failure := openConfiguredLedger(command)
				if failure != nil {
					return refuse(failure)
				}
				export, err := ledger.ExportProposal(store, repository.Repository, command.String("proposal"), command.Path("to"))
				if err != nil {
					return refuse(err)
				}
				outcome := ledgerOutcome{Status: "exported", Export: export, kind: "ledger-export"}
				outcome.facts = outcome
				return renderLedgerOutcome(stdout, format, outcome)
			},
		}, {
			Name:        "retire",
			Usage:       "Explicitly retire a fully terminal, unclaimed proposal parent",
			Description: "Record eligible retirement in the explicit Project, preserving frozen records and source work. Retirement neither archives ledger records nor removes local or remote source branches; use cleanup separately.",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "project", Usage: "Explicit Project name; never inferred from the current directory"},
				&cli.StringFlag{Name: "proposal", Usage: "Accepted Proposal name to retire"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error { return runDecisionRetire(command, newBackend, stdout) },
		}},
	}
}
