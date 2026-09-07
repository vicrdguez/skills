package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/workflow"
)

func implementationCommands(newBackend backendFactory, stdout io.Writer) []*cli.Command {
	var commands []*cli.Command
	for _, name := range []string{"next", "resume", "submit"} {
		commands = append(commands, &cli.Command{Name: name,
			Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.IntFlag{Name: "item"}, &cli.PathFlag{Name: "body"}, &cli.StringFlag{Name: "target-snapshot"}},
			Action: func(command *cli.Context) error {
				backend, err := newBackend()
				if err != nil {
					return err
				}
				port, ok := backend.(workflow.ImplementationBackend)
				if !ok {
					return fmt.Errorf("workflow backend does not support implementation")
				}
				var outcome workflow.ImplementationOutcome
				if name == "submit" {
					outcome, err = workflow.SubmitImplementation(command.Context, command.Path("repo"), command.Int("item"), command.Path("body"), port)
				} else {
					if name == "resume" && command.Int("item") == 0 {
						return fmt.Errorf("resume requires --item")
					}
					outcome, err = workflow.StartImplementation(command.Context, command.Path("repo"), command.Int("item"), port)
				}
				if err != nil {
					return err
				}
				return json.NewEncoder(stdout).Encode(outcome)
			},
		})
	}
	commands[0].Aliases = []string{"start"}
	return commands
}
