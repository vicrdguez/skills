package main

import (
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

func statusCommand(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{Name: "status", Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"}, &cli.StringFlag{Name: "item"}, implementationFormatFlag()}, Action: func(c *cli.Context) error {
		if c.NArg() != 0 {
			return fmt.Errorf("status takes no positional arguments")
		}
		repository, err := setup.ResolveRepository(c.Path("repo"), c.String("remote"))
		if err != nil {
			return err
		}
		format, err := implementationFormat(c.String("format"))
		if err != nil {
			return err
		}
		store, err := openConfiguredLedger()
		if err != nil {
			return renderLedgerRefusal(stdout, format, err)
		}
		adopted, err := store.Adopted(repository.Repository)
		if err != nil {
			return renderLedgerRefusal(stdout, format, err)
		}
		if !adopted {
			return renderLedgerRefusal(stdout, format, &ledger.Refusal{
				Invariant: "repository " + repository.Repository.Owner + "/" + repository.Repository.Name + " has no accepted ledger Project",
				Repair:    "accept a Proposal for this repository with `skl ledger accept --repo <path> --proposal-dir <dir> --issue <slice>=<body-file>` before running status",
			})
		}
		return ledgerStatus(c, store, repository.Repository, newBackend, stdout, format)
	}}
}
