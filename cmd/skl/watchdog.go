package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/workflow"
)

func watchdogCommands(newBackend backendFactory, stdout io.Writer) []*cli.Command {
	return []*cli.Command{{Name: "next", Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"}}, Action: func(c *cli.Context) error {
		backend, err := newBackend()
		if err != nil {
			return err
		}
		port, ok := backend.(workflow.ImplementationBackend)
		if !ok {
			return fmt.Errorf("workflow backend does not support Watchdog")
		}
		outcome, err := workflow.StartWatchdog(c.Context, c.Path("repo"), c.String("remote"), port)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(outcome)
	}}}
}
