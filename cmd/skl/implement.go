package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

func implementationCommands(newBackend backendFactory, stdout io.Writer) []*cli.Command {
	var commands []*cli.Command
	for _, name := range []string{"next", "resume", "inspect", "submit", "needs-human"} {
		commands = append(commands, &cli.Command{Name: name,
			Flags: []cli.Flag{&cli.PathFlag{Name: "repo", Value: "."}, &cli.StringFlag{Name: "remote"}, &cli.IntFlag{Name: "item"}, &cli.PathFlag{Name: "body"}, &cli.PathFlag{Name: "decision"}, &cli.StringFlag{Name: "reason"}, &cli.StringFlag{Name: "artifact-baseline"}, &cli.StringFlag{Name: "artifact-completion"}, implementationFormatFlag(), implementationCapabilityFlag()},
			Action: func(command *cli.Context) error {
				if command.Int("item") < 0 || command.NArg() != 0 {
					return fmt.Errorf("invalid implementation invocation: use flags and a positive Work Item identity")
				}
				format, err := implementationFormat(command.String("format"))
				if err != nil {
					return err
				}
				capability, err := implementationCapability(command.String("capability"))
				if err != nil {
					return err
				}
				if name == "next" || name == "resume" {
					gated, err := gateUnsupportedDelivery(stdout, format, command.Path("repo"), command.String("remote"), "implement "+name)
					if gated || err != nil {
						return err
					}
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
				if err != nil {
					return implementationSetupFailure(name, command.Int("item"), "repository or remote resolution", err)
				}
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return implementationSetupFailure(name, command.Int("item"), "backend construction", err)
				}
				port, ok := backend.(workflow.ImplementationBackend)
				if !ok {
					return implementationSetupFailure(name, command.Int("item"), "backend construction", fmt.Errorf("workflow backend does not support implementation"))
				}
				var outcome workflow.ImplementationOutcome
				endpoints := workflow.ArtifactEndpoints{Baseline: command.String("artifact-baseline"), Completion: command.String("artifact-completion")}
				if name == "inspect" {
					if command.Int("item") <= 0 {
						return fmt.Errorf("inspect requires --item")
					}
					outcome, err = workflow.InspectImplementation(command.Context, repository.Root, repository.Remote, workItemID(command.Int("item")), endpoints, port)
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
						output, presentErr := setup.PresentImplementation(workflow.ImplementationOutcome{Status: "fix_required", Reason: violation.Reason}, setup.InvocationContext{Repository: repository.Repository, Capability: capability})
						if presentErr != nil {
							return presentErr
						}
						if format == formatMarkdown {
							_, err = fmt.Fprint(stdout, setup.ImplementationMarkdown(output))
						} else {
							err = json.NewEncoder(stdout).Encode(output)
						}
						if err != nil {
							return implementationDeliveryFailure(name, outcome, err)
						}
						return nil
					}
					return implementationOperationFailure(name, command.Int("item"), err)
				}
				output, err := setup.PresentImplementation(outcome, setup.InvocationContext{Repository: repository.Repository, Capability: capability})
				if err != nil {
					return implementationDeliveryFailure(name, outcome, err)
				}
				if format == formatMarkdown {
					_, err = fmt.Fprint(stdout, setup.ImplementationMarkdown(output))
				} else {
					err = json.NewEncoder(stdout).Encode(output)
				}
				if err != nil {
					return implementationDeliveryFailure(name, outcome, err)
				}
				return nil
			},
		})
	}
	commands[0].Aliases = []string{"start"}
	commands[0].Flags = append(commands[0].Flags, waitFlags()...)
	return commands
}

// implementationCapabilities is the complete set of established execution
// capabilities. It is never inferred from a harness name.
var implementationCapabilities = map[string]skilldist.ExecutionCapability{
	string(skilldist.UnknownCapability): skilldist.UnknownCapability,
	string(skilldist.ClaudeAgentReview): skilldist.ClaudeAgentReview,
	string(skilldist.PiSubagentReview):  skilldist.PiSubagentReview,
	string(skilldist.SequentialReview):  skilldist.SequentialReview,
}

func implementationCapabilityFlag() cli.Flag {
	return &cli.StringFlag{Name: "capability", Usage: "Established execution capability: claude-agents, pi-subagents, or sequential; omit it when the capability is unknown"}
}

// implementationCapability validates a supplied capability before any
// avoidable backend call, selection, Claim, publication, or result-directory
// creation. An omitted or empty value keeps the runtime choice.
func implementationCapability(value string) (skilldist.ExecutionCapability, error) {
	capability, ok := implementationCapabilities[value]
	if !ok {
		return "", fmt.Errorf("invalid execution capability %q; use claude-agents, pi-subagents, or sequential, or omit the flag when the capability is unknown", value)
	}
	return capability, nil
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

func implementationSetupFailure(operation string, item int, step string, err error) error {
	if item > 0 {
		return fmt.Errorf("implement %s stopped during %s before a Workflow operation ran; the existing Claim for Work Item #%d was not released, so repair access and retry the same command: %w", operation, step, item, err)
	}
	return fmt.Errorf("implement %s stopped during %s before Claim acquisition; repair access and retry `skl implement %s`: %w", operation, step, operation, err)
}

func implementationOperationFailure(operation string, item int, err error) error {
	switch operation {
	case "next":
		// StartImplementation classifies pre-acquisition observation failures and
		// uncertain post-acquisition failures at the point that knows which one occurred.
		return err
	case "resume":
		return fmt.Errorf("implement resume failed without releasing the existing Claim; inspect Work Item #%d and retry the same resume rather than selecting replacement work: %w", item, err)
	case "inspect":
		return fmt.Errorf("read-only Implement inspection failed and authorized no transition; repair the observation and retry inspect for Work Item #%d: %w", item, err)
	case "submit", "needs-human":
		return fmt.Errorf("implement %s failed before a verified handoff was reported; effects may already exist, so inspect Work Item #%d and retry the same operation with the same Result Documents instead of selecting or publishing replacement work: %w", operation, item, err)
	default:
		return err
	}
}

func implementationDeliveryFailure(operation string, outcome workflow.ImplementationOutcome, err error) error {
	identity := "the Work Item"
	if outcome.Item != nil && outcome.Item.ID != "" {
		identity = "Work Item #" + string(outcome.Item.ID)
	}
	return fmt.Errorf("implement %s established status %q but output delivery failed; that failure does not undo or prove the operation, so inspect %s and explicitly resume or retry the same operation rather than running next blindly: %w", operation, outcome.Status, identity, err)
}
