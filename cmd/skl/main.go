package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/setup"
)

type backendFactory func() (setup.Backend, error)

func newApp(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer) *cli.App {
	home, _ := os.UserHomeDir()
	return newAppWithSkillHome(newBackend, stdin, stdout, stderr, home)
}

func newAppWithSkillHome(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer, home string) *cli.App {
	app := cli.NewApp()
	app.Name = "skl"
	app.Writer = stdout
	app.ErrWriter = stderr
	app.Commands = []*cli.Command{{
		Name: "install",
		Action: func(_ *cli.Context) error {
			return skilldist.Install(home)
		},
	}, {
		Name:      "skill",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Value: "markdown"},
			&cli.StringFlag{Name: "resource"},
		},
		Action: func(command *cli.Context) error {
			if command.NArg() != 1 {
				return fmt.Errorf("skill name is required")
			}
			name := command.Args().First()
			if command.String("resource") != "" {
				resource, err := skilldist.Resource(name, command.String("resource"))
				if err != nil {
					return err
				}
				_, err = stdout.Write(resource)
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
				_, err = fmt.Fprint(stdout, packet.Markdown())
				return err
			}
			return fmt.Errorf("unsupported format %q", command.String("format"))
		},
	}, {
		Name: "setup",
		Flags: []cli.Flag{
			&cli.PathFlag{Name: "repo"},
			&cli.StringFlag{Name: "remote"},
		},
		Action: func(command *cli.Context) error {
			backend, err := newBackend()
			if err != nil {
				return err
			}
			location := command.Path("repo")
			if location == "" {
				location = "."
			}
			reader := bufio.NewReader(stdin)
			outcome, err := setup.Run(command.Context, setup.Request{
				Location: location,
				Remote:   command.String("remote"),
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
	return app
}

func main() {
	app := newApp(setup.NewGitHubBackendFromEnv, os.Stdin, os.Stdout, os.Stderr)
	if err := app.RunContext(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
