package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

func sliceCommands(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:  "slice",
		Usage: "Operate on an accepted Slice",
		Subcommands: []*cli.Command{{
			Name:  "show",
			Usage: "Read a Slice's current Contract and report availability, or its current phase report",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo", Value: ".", Usage: "Source repository whose Slice to read"},
				&cli.StringFlag{Name: "remote", Usage: "Source repository remote used to identify its Project"},
				&cli.StringFlag{Name: "slice", Usage: "Accepted Slice identity (<proposal>/<slice>)"},
				&cli.StringFlag{Name: "phase", Usage: "Current report phase: implement or watchdog; omit to read the Contract and report availability"},
				implementationFormatFlag(),
			},
			Action: func(command *cli.Context) error {
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				rerun := boundCommand(command, "skl slice show")
				refuse := func(err error) error { return refuseLedger(stdout, format, rerun, err) }
				if command.NArg() != 0 {
					return fmt.Errorf("slice show takes flags, not arguments")
				}
				item, phase := command.String("slice"), command.String("phase")
				if phase != "" && item == "" {
					return refuse(errors.New("slice show --phase is available only alongside --slice; use ledger show --commit with --path to retrieve an exact historical document"))
				}
				if phase != "" && phase != ledger.ImplementPhase && phase != ledger.WatchdogPhase {
					return refuse(errors.New("slice show --phase must be implement or watchdog; omit --phase to inspect availability, or use ledger show --commit with --path for an exact historical document"))
				}
				if item == "" {
					return refuse(errors.New("slice show requires --slice <proposal>/<slice>; use the identity and readback command the acceptance reported"))
				}
				store, failure := openConfiguredLedger()
				if failure != nil {
					return refuse(failure)
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return refuse(err)
				}
				if phase != "" {
					document, err := ledger.ShowReport(store, repository.Repository, item, phase)
					if err != nil {
						return refuse(err)
					}
					return renderLedgerOutcome(stdout, format, ledgerOutcome{Status: "shown", Document: &document})
				}
				readback, err := ledger.ShowItem(store, repository.Repository, item)
				if err != nil {
					return refuse(err)
				}
				return renderLedgerOutcome(stdout, format, ledgerOutcome{
					Status: "shown", Readback: readback,
					ReadbackCommand: fmt.Sprintf("skl slice show --repo %s --remote %s --slice %s",
						skilldist.ShellQuote(repository.Root), skilldist.ShellQuote(repository.Remote), skilldist.ShellQuote(item)),
				})
			},
		}, presentCommand(newBackend, stdout)},
	}
}
