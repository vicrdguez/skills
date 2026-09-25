package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
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
		_, exists, err := ledger.SettingsLocation(os.Getenv)
		if err != nil {
			return err
		}
		if exists {
			store, err := openConfiguredLedger()
			if err != nil {
				return renderLedgerRefusal(stdout, format, err)
			}
			adopted, err := store.Adopted(repository.Repository)
			if err != nil {
				return renderLedgerRefusal(stdout, format, err)
			}
			if adopted {
				return ledgerStatus(c, store, repository.Repository, newBackend, stdout, format)
			}
		}
		if c.String("item") != "" {
			return renderLedgerRefusal(stdout, format, fmt.Errorf("fixed-item status requires an accepted ledger Project"))
		}
		backend, err := newBackend(repository.Repository)
		if err != nil {
			return err
		}
		port, ok := backend.(workflow.ImplementationBackend)
		if !ok {
			return fmt.Errorf("backend does not support status")
		}
		outcome, err := workflow.ObserveStatus(c.Context, port)
		if err != nil {
			var violation *workflow.InvariantError
			if errors.As(err, &violation) {
				return json.NewEncoder(stdout).Encode(workflow.ImplementationOutcome{Status: "fix_required", Reason: violation.Reason})
			}
			return err
		}
		output, err := setup.PresentStatus(outcome)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(output)
	}}
}
