package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func implementationCommands(newBackend backendFactory, stdout io.Writer) []*cli.Command {
	var commands []*cli.Command
	for _, name := range []string{"next", "resume", "inspect", "submit", "needs-human"} {
		commands = append(commands, &cli.Command{Name: name,
			Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"}, &cli.IntFlag{Name: "item"}, &cli.PathFlag{Name: "body"}, &cli.PathFlag{Name: "decision"}, &cli.StringFlag{Name: "reason"}, &cli.StringFlag{Name: "target-snapshot"}, &cli.StringFlag{Name: "reviewed-head"}},
			Action: func(command *cli.Context) error {
				if command.Int("item") < 0 || command.NArg() != 0 || command.String("after") != "" && (name != "next" || command.IsSet("item") || command.String("target-snapshot") != "" || command.String("reviewed-head") != "") {
					return fmt.Errorf("invalid implementation invocation: use flags and a positive Work Item identity")
				}
				backend, err := newBackend(github.RepositoryID{})
				if err != nil {
					return err
				}
				port, ok := backend.(workflow.ImplementationBackend)
				if !ok {
					return fmt.Errorf("workflow backend does not support implementation")
				}
				var outcome workflow.ImplementationOutcome
				if name == "inspect" {
					if command.Int("item") <= 0 {
						return fmt.Errorf("inspect requires --item")
					}
					outcome, err = workflow.InspectImplementation(command.Context, command.Path("repo"), command.String("remote"), workItemID(command.Int("item")), port)
				} else if name == "needs-human" {
					outcome, err = workflow.PauseImplementation(command.Context, command.Path("repo"), command.String("remote"), workItemID(command.Int("item")), command.String("reason"), command.Path("decision"), command.Path("body"), port)
				} else if name == "submit" {
					outcome, err = workflow.SubmitImplementation(command.Context, command.Path("repo"), command.String("remote"), workItemID(command.Int("item")), command.Path("body"), port)
				} else {
					id := workItemID(command.Int("item"))
					if name == "resume" && id == "" {
						id = workflow.CurrentWorktree
					}
					outcome, err = continuedWork(command.Context, command.Path("repo"), command.String("remote"), command.String("after"), workflow.ImplementLane, command.Duration("wait"), command.Duration("poll"), command.IsSet("poll"), port, func() (workflow.ImplementationOutcome, error) {
						return workflow.StartImplementation(command.Context, command.Path("repo"), command.String("remote"), id, command.String("target-snapshot"), command.String("reviewed-head"), port)
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
			},
		})
	}
	commands[0].Aliases = []string{"start"}
	commands[0].Flags = append(commands[0].Flags, &cli.StringFlag{Name: "after", Usage: "verify one completed dispatch before selecting again; use once and stop for explicit recovery if the response is uncertain"})
	commands[0].Flags = append(commands[0].Flags, waitFlags()...)
	return commands
}

func workItemID(number int) workflow.WorkItemID {
	if number <= 0 {
		return ""
	}
	return workflow.WorkItemID(strconv.Itoa(number))
}
