package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/setup"
)

type backendFactory func() (setup.Backend, error)

func newApp(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer) *cli.App {
	app := cli.NewApp()
	app.Name = "skl"
	app.Writer = stdout
	app.ErrWriter = stderr
	app.Commands = []*cli.Command{{
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
