package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/setup"
	"github.com/vicrdguez/skills/workflow"
)

type backendFactory func(github.RepositoryID) (setup.Backend, error)

func newApp(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer) *stageApp {
	return newAppWithSkillHome(newBackend, stdin, stdout, stderr, "")
}

func newAppWithSkillHome(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer, home string) *stageApp {
	app := cli.NewApp()
	app.Name = "skl"
	app.Writer = stdout
	app.ErrWriter = stderr
	app.Commands = []*cli.Command{{
		Name: "install",
		Action: func(_ *cli.Context) error {
			if home == "" {
				var err error
				home, err = os.UserHomeDir()
				if err != nil {
					return err
				}
			}
			_, err := skilldist.Install(home)
			return err
		},
	}, {
		Name:      "skill",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Value: "markdown"},
			&cli.StringFlag{Name: "resource"},
			newInputFlag(),
			&cli.BoolFlag{Name: "describe-inputs", Usage: "Describe the named resource's accepted inputs without rendering it"},
		},
		Action: func(command *cli.Context) error {
			if command.NArg() != 1 {
				return fmt.Errorf("skill name is required")
			}
			name := command.Args().First()
			resource := command.String("resource")
			describe := command.Bool("describe-inputs")
			inputs := inputValues(command)
			if resource == "" && describe {
				return fmt.Errorf("--describe-inputs requires --resource <owner-relative-name>")
			}
			if resource == "" && len(inputs) > 0 {
				return fmt.Errorf("--input requires --resource <owner-relative-name>")
			}
			if resource == "" && name == "implement" {
				// The Implement Execution Skill is delivered by its own lane: a
				// read-only retrieval would name no Work Item, open no Workflow
				// Backend, and acquire no Claim.
				return fmt.Errorf("the implement Skill Definition is not retrievable read-only; run `skl implement next` for one Work Item's complete Execution Skill, or `skl implement resume --item <number>` to continue a Claim. Its named resources stay retrievable with `skl skill --resource <name> implement`")
			}
			if resource != "" {
				if describe {
					description, err := skilldist.DescribeResourceInputs(name, resource)
					if err != nil {
						return err
					}
					_, err = fmt.Fprint(stdout, description)
					return err
				}
				contents, err := skilldist.RenderResource(name, resource, inputs)
				if err != nil {
					return err
				}
				_, err = stdout.Write(contents)
				return err
			}
			if name == "watchdog" {
				return fmt.Errorf("the Watchdog Execution Skill requires selected work; run `skl watchdog next` for one Work Item or `skl watchdog resume --item <number>` for an interrupted Claim. Named resources remain available with --resource")
			}
			packet, err := skilldist.BuildPacket(name, skilldist.InvocationFacts{})
			if err != nil {
				return err
			}
			if command.String("format") == "json" {
				payload, err := packet.JSON()
				if err != nil {
					return err
				}
				_, err = stdout.Write(payload)
				return err
			}
			if command.String("format") == "markdown" {
				_, err = fmt.Fprint(stdout, packet.Markdown())
				return err
			}
			return fmt.Errorf("unsupported format %q", command.String("format"))
		},
	}, {
		Name: "propose",
		Subcommands: []*cli.Command{{
			Name: "publish",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo"},
				&cli.StringFlag{Name: "remote"},
				&cli.StringFlag{Name: "target", Required: true},
				&cli.StringSliceFlag{Name: "slice", Required: true},
				&cli.StringSliceFlag{Name: "depends"},
				&cli.StringFlag{Name: "parent-title"},
				&cli.PathFlag{Name: "parent-body"},
			},
			Action: func(command *cli.Context) error {
				request, err := proposalRequest(command)
				if err != nil {
					return err
				}
				repository, err := setup.ResolveRepository(request.Root, request.Remote)
				if err != nil {
					return err
				}
				request.Root, request.Remote = repository.Root, repository.Remote
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return err
				}
				proposalBackend, ok := backend.(workflow.Backend)
				if !ok {
					return fmt.Errorf("workflow backend does not support proposal publication")
				}
				outcome, err := workflow.Publish(command.Context, request, proposalBackend)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintln(stdout, outcome.Status); err != nil {
					return err
				}
				if outcome.Reason != "" {
					_, err = fmt.Fprintln(stdout, outcome.Reason)
				}
				return err
			},
		}, {
			Name: "cleanup",
			Flags: []cli.Flag{
				&cli.PathFlag{Name: "repo"},
				&cli.StringFlag{Name: "remote"},
			},
			Action: func(command *cli.Context) error {
				remote := command.String("remote")
				if remote == "" {
					remote = "origin"
				}
				gated, err := gateUnsupportedDelivery(stdout, formatMarkdown, command.Path("repo"), remote, "propose cleanup")
				if gated || err != nil {
					return err
				}
				repository, err := setup.ResolveRepository(command.Path("repo"), remote)
				if err != nil {
					return err
				}
				backend, err := newBackend(repository.Repository)
				if err != nil {
					return err
				}
				proposalBackend, ok := backend.(workflow.Backend)
				if !ok {
					return fmt.Errorf("workflow backend does not support proposal cleanup")
				}
				outcome, err := workflow.Cleanup(command.Context, repository.Root, proposalBackend)
				if err != nil {
					return err
				}
				for _, slug := range outcome.Removed {
					fmt.Fprintln(stdout, "removed", slug)
				}
				for _, slug := range outcome.Preserved {
					fmt.Fprintln(stdout, "preserved", slug)
				}
				return nil
			},
		}},
	}, {
		Name:        "implement",
		Subcommands: implementationCommands(newBackend, stdout),
	}, {
		Name:        "watchdog",
		Subcommands: watchdogCommands(newBackend, stdout),
	}, {
		Name: "setup",
		Flags: []cli.Flag{
			&cli.PathFlag{Name: "repo"},
			&cli.StringFlag{Name: "remote"},
		},
		Action: func(command *cli.Context) error {
			repository, err := setup.ResolveRepository(command.Path("repo"), command.String("remote"))
			if err != nil {
				return err
			}
			backend, err := newBackend(repository.Repository)
			if err != nil {
				return err
			}
			reader := bufio.NewReader(stdin)
			outcome, err := setup.Run(command.Context, setup.Request{
				Location: repository.Root,
				Confirm: func(prompt string) (bool, error) {
					if _, err := fmt.Fprint(stdout, prompt); err != nil {
						return false, err
					}
					answer, err := reader.ReadString('\n')
					if err == io.EOF {
						err = nil
					}
					return strings.EqualFold(strings.TrimSpace(answer), "y"), err
				},
			}, backend)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "Prepared %s for GitHub workflow on %s.\n", outcome.Root, outcome.TargetBranch)
			return err
		},
	}}
	app.Commands = append(app.Commands, statusCommand(newBackend, stdout), ledgerCommands(newBackend, stdout))
	return &stageApp{app}
}

// inputFlag collects repeated --input occurrences verbatim. The resource
// parser, not the CLI, splits each occurrence at its first "=", so commas,
// quotes and surrounding padding reach the renderer intact.
type inputFlag struct {
	cli.GenericFlag
}

func newInputFlag() *inputFlag {
	return &inputFlag{GenericFlag: cli.GenericFlag{
		Name:  "input",
		Usage: "Repeatable `name=value` input for the named resource; split at the first '='",
	}}
}

// Apply binds a fresh collector to the flag set built for this run, so repeated
// invocations never share input values.
func (f *inputFlag) Apply(set *flag.FlagSet) error {
	f.Value = &rawInputs{}
	return f.GenericFlag.Apply(set)
}

type rawInputs []string

func (r *rawInputs) Set(value string) error {
	*r = append(*r, value)
	return nil
}

func (r *rawInputs) String() string { return "" }

func inputValues(command *cli.Context) []string {
	values, _ := command.Generic("input").(*rawInputs)
	if values == nil {
		return nil
	}
	return *values
}

func proposalRequest(command *cli.Context) (workflow.PublishRequest, error) {
	request := workflow.PublishRequest{
		Root: command.Path("repo"), Remote: command.String("remote"), Target: command.String("target"),
		ParentTitle: command.String("parent-title"), ParentBody: command.Path("parent-body"),
	}
	if request.Root == "" {
		request.Root = "."
	}
	for _, value := range command.StringSlice("slice") {
		slug, body, ok := strings.Cut(value, "=")
		if !ok {
			return request, fmt.Errorf("invalid --slice %q; want slug=body-file", value)
		}
		request.Slices = append(request.Slices, workflow.Slice{Slug: slug, BodyPath: body})
	}
	for _, value := range command.StringSlice("depends") {
		dependent, blocker, ok := strings.Cut(value, ":")
		if !ok {
			return request, fmt.Errorf("invalid --depends %q; want dependent:blocker", value)
		}
		request.Dependencies = append(request.Dependencies, workflow.Dependency{Dependent: dependent, Blocker: blocker})
	}
	return request, nil
}

func main() {
	app := newApp(setup.NewGitHubBackendFromEnv, os.Stdin, os.Stdout, os.Stderr)
	for _, lane := range []string{"implement", "watchdog"} {
		command := app.Command(lane).Command("next")
		action := command.Action
		command.Action = func(c *cli.Context) error {
			// Only waiting selections translate process signals into cancellation.
			if c.Duration("wait") > 0 {
				ctx, stop := signal.NotifyContext(c.Context, os.Interrupt, syscall.SIGTERM)
				defer stop()
				c.Context = ctx
			}
			return action(c)
		}
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
