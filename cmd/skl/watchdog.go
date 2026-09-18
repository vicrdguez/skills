package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func watchdogCommands(newBackend backendFactory, stdout io.Writer) []*cli.Command {
	var commands []*cli.Command
	for _, name := range []string{"next", "resume", "inspect", "submit"} {
		commands = append(commands, &cli.Command{Name: name, Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"}, &cli.IntFlag{Name: "item"}, &cli.IntFlag{Name: "submission"}, &cli.StringFlag{Name: "base"}, &cli.StringFlag{Name: "submission-body-sha256"}, &cli.StringFlag{Name: "format", Value: "markdown"}, &cli.Uint64Flag{Name: "review-number"}, &cli.StringFlag{Name: "previous-reviewed-head"}, &cli.StringFlag{Name: "result-directory"}, &cli.StringFlag{Name: "verdict"}, &cli.StringFlag{Name: "reviewed-head"}, &cli.PathFlag{Name: "summary"}, &cli.PathFlag{Name: "findings"}, &cli.PathFlag{Name: "body"}, &cli.StringFlag{Name: "head"}, &cli.StringFlag{Name: "artifact-baseline"}, &cli.StringFlag{Name: "artifact-completion"}}, Action: func(c *cli.Context) error {
			if c.String("format") != "markdown" && c.String("format") != "json" {
				return fmt.Errorf("unsupported format %q: choose markdown or json", c.String("format"))
			}
			if c.NArg() != 0 || (name == "resume" || name == "inspect") && c.Int("item") <= 0 || name == "next" && c.IsSet("item") {
				return fmt.Errorf("resume requires --item; next selects its own Work Item")
			}
			repository, err := setup.ResolveRepository(c.Path("repo"), c.String("remote"))
			if err != nil {
				return err
			}
			backend, err := newBackend(repository.Repository)
			if err != nil {
				return err
			}
			port, ok := backend.(workflow.ImplementationBackend)
			if !ok {
				return fmt.Errorf("workflow backend does not support Watchdog")
			}
			var outcome workflow.ImplementationOutcome
			endpoints := workflow.ArtifactEndpoints{Baseline: c.String("artifact-baseline"), Completion: c.String("artifact-completion")}
			if name == "inspect" {
				inspected, err := workflow.InspectWatchdog(c.Context, repository.Root, repository.Remote, workItemID(c.Int("item")), workflow.SubmissionID(workItemID(c.Int("submission"))), c.String("base"), c.String("submission-body-sha256"), c.String("reviewed-head"), c.Uint64("review-number"), c.String("previous-reviewed-head"), c.String("result-directory"), endpoints, port)
				if err != nil {
					return err
				}
				output := setup.PresentWatchdogInspection(repository.Root, inspected)
				if c.String("format") == "json" {
					return json.NewEncoder(stdout).Encode(output)
				}
				_, err = fmt.Fprint(stdout, output.Instructions)
				return err
			}
			if name == "submit" {
				review, ok := backend.(workflow.ReviewBackend)
				if !ok {
					return fmt.Errorf("backend does not support review publication")
				}
				outcome, err = workflow.SubmitWatchdog(c.Context, repository.Root, repository.Remote, workItemID(c.Int("item")), c.Uint64("review-number"), c.String("reviewed-head"), c.String("head"), c.String("verdict"), c.Path("summary"), c.Path("findings"), c.Path("body"), endpoints, review)
			} else {
				outcome, err = nextWork(c.Context, c.Duration("wait"), c.Duration("poll"), func() (workflow.ImplementationOutcome, error) {
					return workflow.StartWatchdog(c.Context, repository.Root, repository.Remote, workItemID(c.Int("item")), endpoints, port)
				})
			}
			if err != nil {
				var violation *workflow.InvariantError
				if errors.As(err, &violation) {
					refusal := setup.ImplementationOutput{ImplementationOutcome: workflow.ImplementationOutcome{Status: "fix_required", Reason: violation.Reason}}
					if c.String("format") == "json" {
						return json.NewEncoder(stdout).Encode(refusal)
					}
					_, writeErr := fmt.Fprint(stdout, setup.WatchdogOutcomeMarkdown(refusal))
					return writeErr
				}
				return err
			}
			if outcome.Facts != nil && outcome.Facts.Watchdog != nil {
				outcome.Facts.Watchdog.Repository = repository.Repository.Owner + "/" + repository.Repository.Name
			}
			output, err := setup.PresentImplementation(outcome)
			if err != nil {
				return err
			}
			if c.String("format") == "json" {
				return json.NewEncoder(stdout).Encode(output)
			}
			if output.Packet != nil {
				_, err = fmt.Fprint(stdout, output.Packet.Instructions)
				return err
			}
			_, err = fmt.Fprint(stdout, setup.WatchdogOutcomeMarkdown(output))
			return err
		}})
	}
	commands[0].Flags = append(commands[0].Flags, waitFlags()...)
	return commands
}
