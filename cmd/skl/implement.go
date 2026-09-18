package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func implementationCommands(newBackend backendFactory, stdout io.Writer) []*cli.Command {
	var commands []*cli.Command
	for _, name := range []string{"next", "resume", "inspect", "submit", "needs-human"} {
		commands = append(commands, &cli.Command{Name: name,
			Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"}, &cli.IntFlag{Name: "item"}, &cli.PathFlag{Name: "body"}, &cli.PathFlag{Name: "decision"}, &cli.StringFlag{Name: "reason"}, &cli.StringFlag{Name: "artifact-baseline"}, &cli.StringFlag{Name: "artifact-completion"}, implementationFormatFlag()},
			Action: func(command *cli.Context) error {
				if command.Int("item") < 0 || command.NArg() != 0 {
					return fmt.Errorf("invalid implementation invocation: use flags and a positive Work Item identity")
				}
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return err
				}
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return err
				}
				port, ok := backend.(workflow.ImplementationBackend)
				if !ok {
					return fmt.Errorf("workflow backend does not support implementation")
				}
				var outcome workflow.ImplementationOutcome
				endpoints := workflow.ArtifactEndpoints{Baseline: command.String("artifact-baseline"), Completion: command.String("artifact-completion")}
				if name == "inspect" {
					if command.Int("item") <= 0 {
						return fmt.Errorf("inspect requires --item")
					}
					outcome, err = workflow.InspectImplementation(command.Context, repository.Root, workItemID(command.Int("item")), endpoints, port)
				} else if name == "needs-human" {
					outcome, err = workflow.PauseImplementation(command.Context, repository.Root, repository.Remote, workItemID(command.Int("item")), command.String("reason"), command.Path("decision"), command.Path("body"), endpoints, port)
				} else if name == "submit" {
					outcome, err = workflow.SubmitImplementation(command.Context, repository.Root, repository.Remote, workItemID(command.Int("item")), command.Path("body"), endpoints, port)
				} else {
					id := workItemID(command.Int("item"))
					if name == "resume" && id == "" {
						id = workflow.CurrentWorktree
					}
					outcome, err = nextWork(command.Context, command.Duration("wait"), command.Duration("poll"), func() (workflow.ImplementationOutcome, error) {
						return workflow.StartImplementation(command.Context, repository.Root, repository.Remote, id, endpoints, port)
					})
				}
				if err != nil {
					var violation *workflow.InvariantError
					if errors.As(err, &violation) {
						output, presentErr := setup.PresentImplementation(workflow.ImplementationOutcome{Status: "fix_required", Reason: violation.Reason})
						if presentErr != nil {
							return presentErr
						}
						if format == formatMarkdown {
							_, err = fmt.Fprint(stdout, setup.ImplementationMarkdown(output))
							return err
						}
						return json.NewEncoder(stdout).Encode(output)
					}
					return err
				}
				output, err := setup.PresentImplementation(outcome)
				if err != nil {
					return err
				}
				if format == formatMarkdown {
					_, err = fmt.Fprint(stdout, setup.ImplementationMarkdown(output))
					return err
				}
				return json.NewEncoder(stdout).Encode(output)
			},
		})
	}
	commands[0].Aliases = []string{"start"}
	commands[0].Flags = append(commands[0].Flags, waitFlags()...)
	return commands
}

// implementationFormatKind is the complete set of supported transports. Explicit
// JSON preserves the same operation and outcome; it never names a second
// operation or an alternative authority for success.
type implementationFormatKind string

const (
	formatMarkdown implementationFormatKind = "markdown"
	formatJSON     implementationFormatKind = "json"
)

func implementationFormatFlag() cli.Flag {
	return &cli.StringFlag{Name: "format", Value: string(formatMarkdown), Usage: "Output transport: markdown (default) or json"}
}

// implementationFormat validates the requested transport before any avoidable
// backend call, selection, Claim, publication, or result-directory creation.
func implementationFormat(value string) (implementationFormatKind, error) {
	format := implementationFormatKind(value)
	if format != formatMarkdown && format != formatJSON {
		return "", fmt.Errorf("unsupported format %q; use markdown or json", value)
	}
	return format, nil
}

func workItemID(number int) workflow.WorkItemID {
	if number <= 0 {
		return ""
	}
	return workflow.WorkItemID(strconv.Itoa(number))
}
