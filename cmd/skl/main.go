package main

import (
	"bufio"
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
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
			return installSkills(home, stdin, stdout)
		},
	}, {
		Name:      "skill",
		Usage:     "Retrieve a standalone skill or a named resource; Implement and Watchdog use delivery commands",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Value: "markdown", Usage: "Output format for whole-skill retrieval only: markdown|json"},
			&cli.StringFlag{Name: "resource", Usage: "Retrieve one exact owner-relative resource as raw text"},
			newInputFlag(),
			&cli.BoolFlag{Name: "describe-inputs", Usage: "Describe the named resource's accepted inputs as raw text without rendering it; cannot combine with --input"},
			&cli.PathFlag{Name: "repo", Usage: "Repository the skill renders for: its .skl/principles/, and the .skl/verify.md of verify (default: the repository containing the working directory)"},
		},
		Action: func(command *cli.Context) error {
			if command.NArg() != 1 {
				return fmt.Errorf("skill name is required")
			}
			name := command.Args().First()
			if command.IsSet("repo") && command.IsSet("resource") && name != "principles" {
				return fmt.Errorf("--repo applies to whole-skill retrieval and to resources of principles")
			}
			directory := cmp.Or(command.Path("repo"), ".")
			resource := command.String("resource")
			describe := command.Bool("describe-inputs")
			inputs := inputValues(command)
			if command.IsSet("resource") && resource == "" {
				return fmt.Errorf("--resource requires a nonempty owner-relative name")
			}
			if resource != "" && command.IsSet("format") {
				return fmt.Errorf("--format applies only to whole-skill retrieval, not --resource")
			}
			if command.IsSet("describe-inputs") && command.IsSet("input") {
				return fmt.Errorf("--input cannot be combined with --describe-inputs")
			}
			if resource == "" && command.IsSet("describe-inputs") {
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
			if resource != "" && name == "principles" {
				root, err := repositoryRoot(directory, name)
				if err != nil {
					return err
				}
				principles, err := ledger.ReadPrinciples(root)
				if err != nil {
					return err
				}
				if describe {
					inputs = nil
				}
				contents, err := skilldist.RenderPrinciple(principles, resource, inputs)
				if err != nil {
					return err
				}
				if describe {
					contents = []byte(resource + " accepts no inputs.\n")
				}
				_, err = stdout.Write(contents)
				return err
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
			// A skill renders for the repository it runs in, when there is one;
			// verify and principles need one.
			var facts skilldist.InvocationFacts
			root, err := repositoryRoot(directory, name)
			if err == nil {
				facts.Principles, err = ledger.ReadPrinciples(root)
				if err != nil {
					return err
				}
			} else if name == "verify" || name == "principles" || command.IsSet("repo") {
				return err
			}
			if name == "verify" {
				verify, err := verifyFacts(root)
				if err != nil {
					return err
				}
				facts.Verify = verify
			}
			packet, err := skilldist.BuildPacket(name, facts)
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
	}, cleanupCommand(stdout), sliceCommands(newBackend, stdout), {
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
	app.Commands = append(app.Commands, reconcileCommand(newBackend, stdout), ledgerCommands(newBackend, stdout), proposalCommands(newBackend, stdout), decisionCommands(stdout), browseCommand(stdin, stdout))
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

// repositoryRoot is the root of the Git repository containing directory, for
// which skill renders.
func repositoryRoot(directory, skill string) (string, error) {
	output, err := exec.Command("git", "-C", directory, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not inside a Git repository; run skl skill %s inside the repository or pass --repo", directory, skill)
	}
	return strings.TrimSpace(string(output)), nil
}

// verifyFacts binds the verify skill to the verification skill of the
// repository at root.
func verifyFacts(root string) (*skilldist.VerifyFacts, error) {
	verification, err := ledger.ReadVerification(root)
	if err != nil {
		return nil, err
	}
	if verification == nil {
		return nil, fmt.Errorf("%s has no %s; create it with the verify-create skill: skl skill verify-create", root, ledger.VerificationFile)
	}
	return skilldist.NewVerifyFacts(root, verification)
}
