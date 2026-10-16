package main

import (
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

func reconcileCommand(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{
		Name:        "reconcile",
		Usage:       "Observe forge completion and maintain recorded Slice and Proposal facts",
		Description: "Contacts the forge to observe attached submissions for merge or closure, record completion, and create, update or ready Proposal Submissions. A selected Slice still refreshes its parent Proposal. Unlike browse, this operation can write ledger facts and forge presentation; it does not acquire a Claim or retry Auto Mode merges.",
		Flags: []cli.Flag{
			&cli.PathFlag{Name: "repo", Value: ".", Usage: "Source repository path (default: current directory)"},
			&cli.StringFlag{Name: "remote", Usage: "Source repository remote (default: origin)"},
			&cli.StringFlag{Name: "slice", Usage: "Select one Slice as `proposal/slice`; its parent Proposal is still refreshed"},
			implementationFormatFlag(),
		}, Action: func(c *cli.Context) error {
			if c.NArg() != 0 {
				return fmt.Errorf("reconcile takes no positional arguments")
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
					Repair:    "accept a Proposal for this repository with `skl proposal accept --repo <path> --proposal-dir <dir> --issue <slice>=<body-file>`; if another repository owns this Project name, select the correct ledger or resolve the name collision before running reconcile",
				})
			}
			return ledgerStatus(c, store, repository.Repository, newBackend, stdout, format)
		},
	}
}
