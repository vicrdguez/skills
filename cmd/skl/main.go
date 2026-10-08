package main

import (
	"bufio"
	"errors"
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
)

type backendFactory func(github.RepositoryID) (setup.Backend, error)

// Release builds stamp this with -ldflags "-X main.releaseVersion=v0.5.0".
// Leave it empty for module installs and checkout builds.
var releaseVersion string

func buildVersion() string {
	if releaseVersion != "" {
		return releaseVersion
	}
	if info, ok := buildInfo(); ok {
		// Go 1.27 may assign a pseudo-version to a checkout build. Unlike
		// version-suffixed module installs, those builds carry VCS settings.
		for _, setting := range info.Settings {
			if setting.Key == "vcs" {
				return "development"
			}
		}
		if version := info.Main.Version; version != "" && version != "(devel)" {
			return version
		}
	}
	return "development"
}

func newApp(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer) *stageApp {
	return newAppWithSkillHome(newBackend, stdin, stdout, stderr, "")
}

func newAppWithSkillHome(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer, home string) *stageApp {
	app := cli.NewApp()
	app.Name = "skl"
	app.Version = buildVersion()
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
			if resource == "" && (name == "implement" || name == "watchdog") {
				// Delivery Execution Skills arrive with selected work: a read-only
				// retrieval would name no Work Item and acquire no Claim.
				refusal, err := skilldist.RenderOutcome("skill-delivered", struct{ Name string }{name})
				if err != nil {
					return err
				}
				return errors.New(strings.TrimRight(refusal, "\n"))
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
				_, err = fmt.Fprint(stdout, packet.Instructions)
				return err
			}
			return fmt.Errorf("unsupported format %q", command.String("format"))
		},
	}, {
		Name:        "propose",
		Subcommands: []*cli.Command{cleanupCommand(stdout)},
	}, {
		Name:        "implement",
		Subcommands: deliveryCommands("implement", newBackend, stdout),
	}, {
		Name:        "watchdog",
		Subcommands: deliveryCommands("watchdog", newBackend, stdout),
	}, {
		Name: "setup",
		Flags: []cli.Flag{
			&cli.PathFlag{Name: "repo"},
		},
		Action: func(command *cli.Context) error {
			reader := bufio.NewReader(stdin)
			outcome, err := setup.Run(setup.Request{
				Location: command.Path("repo"),
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
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "Prepared local workflow guidance in %s.\n", outcome.Root)
			return err
		},
	}}
	app.Commands = append(app.Commands, statusCommand(newBackend, stdout), ledgerCommands(newBackend, stdout), decisionCommands(stdout), browseCommand(stdin, stdout))
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
