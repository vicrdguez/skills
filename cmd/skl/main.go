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
	"github.com/vicrdguez/skills/workflow"
)

type backendFactory func() (setup.Backend, error)

func newApp(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer) *cli.App {
	return newAppWithSkillHome(newBackend, stdin, stdout, stderr, "")
}

func newAppWithSkillHome(newBackend backendFactory, stdin io.Reader, stdout, stderr io.Writer, home string) *cli.App {
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
				backend, err := newBackend()
				if err != nil {
					return err
				}
				proposalBackend, ok := backend.(workflow.Backend)
				if !ok {
					return fmt.Errorf("workflow backend does not support proposal publication")
				}
				request, err := proposalRequest(command)
				if err != nil {
					return err
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
				backend, err := newBackend()
				if err != nil {
					return err
				}
				proposalBackend, ok := backend.(workflow.Backend)
				if !ok {
					return fmt.Errorf("workflow backend does not support proposal cleanup")
				}
				root := command.Path("repo")
				if root == "" {
					root = "."
				}
				outcome, err := workflow.Cleanup(command.Context, root, command.String("remote"), proposalBackend)
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
	if err := app.RunContext(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
