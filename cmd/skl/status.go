package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

func statusCommand(newBackend backendFactory, stdout io.Writer) *cli.Command {
	return &cli.Command{Name: "status", Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"}}, Action: func(c *cli.Context) error {
		if c.NArg() != 0 {
			return fmt.Errorf("status takes no positional arguments")
		}
		backend, err := newBackend(github.RepositoryID{})
		if err != nil {
			return err
		}
		port, ok := backend.(workflow.ImplementationBackend)
		if !ok {
			return fmt.Errorf("backend does not support status")
		}
		outcome, err := workflow.ObserveStatus(c.Context, c.Path("repo"), c.String("remote"), port)
		if err != nil {
			var violation *workflow.InvariantError
			if errors.As(err, &violation) {
				return json.NewEncoder(stdout).Encode(workflow.ImplementationOutcome{Status: "fix_required", Reason: violation.Reason})
			}
			return err
		}
		return json.NewEncoder(stdout).Encode(outcome)
	}}
}
