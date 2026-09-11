package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func watchdogCommands(newBackend backendFactory, stdout io.Writer) []*cli.Command {
	var commands []*cli.Command
	for _, name := range []string{"next", "resume", "submit"} {
		commands = append(commands, &cli.Command{Name: name, Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"}, &cli.IntFlag{Name: "item"}, &cli.StringFlag{Name: "verdict"}, &cli.StringFlag{Name: "reviewed-head"}, &cli.PathFlag{Name: "summary"}, &cli.PathFlag{Name: "findings"}, &cli.PathFlag{Name: "body"}, &cli.StringFlag{Name: "head"}}, Action: func(c *cli.Context) error {
			if c.NArg() != 0 || name == "resume" && c.Int("item") <= 0 || name == "next" && c.IsSet("item") || c.String("after") != "" && name != "next" {
				return fmt.Errorf("resume requires --item; next selects its own Work Item")
			}
			backend, err := newBackend(github.RepositoryID{})
			if err != nil {
				return err
			}
			port, ok := backend.(workflow.ImplementationBackend)
			if !ok {
				return fmt.Errorf("workflow backend does not support Watchdog")
			}
			var outcome workflow.ImplementationOutcome
			if name == "submit" {
				review, ok := backend.(workflow.ReviewBackend)
				if !ok {
					return fmt.Errorf("backend does not support review publication")
				}
				outcome, err = workflow.SubmitWatchdog(c.Context, c.Path("repo"), c.String("remote"), workItemID(c.Int("item")), c.String("reviewed-head"), c.String("head"), c.String("verdict"), c.Path("summary"), c.Path("findings"), c.Path("body"), review)
			} else {
				outcome, err = continuedWork(c.Context, c.Path("repo"), c.String("remote"), c.String("after"), workflow.WatchdogLane, c.Duration("wait"), c.Duration("poll"), c.IsSet("poll"), port, func() (workflow.ImplementationOutcome, error) {
					return workflow.StartWatchdog(c.Context, c.Path("repo"), c.String("remote"), workItemID(c.Int("item")), port)
				})
			}
			if err != nil {
				var violation *workflow.InvariantError
				if errors.As(err, &violation) {
					return json.NewEncoder(stdout).Encode(workflow.ImplementationOutcome{Status: "fix_required", Reason: violation.Reason})
				}
				return err
			}
			output, err := setup.PresentImplementation(outcome)
			if err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(output)
		}})
	}
	commands[0].Flags = append(commands[0].Flags, &cli.StringFlag{Name: "after", Usage: "verify one completed dispatch before selecting again; use once and stop for explicit recovery if the response is uncertain"})
	commands[0].Flags = append(commands[0].Flags, waitFlags()...)
	return commands
}
