package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/workflow"
)

type stageApp struct{ *cli.App }

func (app *stageApp) Run(args []string) error {
	return app.RunContext(context.Background(), args)
}

func (app *stageApp) RunContext(ctx context.Context, args []string) error {
	// urfave/cli requires values for duration flags. Expand only bare --wait,
	// leaving all ordinary flag parsing and errors to the CLI library.
	args = append([]string(nil), args...)
	if len(args) >= 3 && (args[1] == "implement" || args[1] == "watchdog") {
		command := app.Command(args[1]).Command(args[2])
		if command != nil && command.Name == "next" {
			for i := 3; i < len(args); i++ {
				arg := args[i]
				if arg == "--" {
					break
				}
				if arg == "--wait" && (i+1 == len(args) || strings.HasPrefix(args[i+1], "--") || args[i+1] == "-h") {
					args[i] = "--wait=15m"
					continue
				}
				if !strings.Contains(arg, "=") {
					for _, flag := range command.Flags {
						for _, name := range flag.Names() {
							if arg == "--"+name || arg == "-"+name {
								i++
								break
							}
						}
					}
				}
			}
		}
	}
	return app.App.RunContext(ctx, args)
}

func waitFlags() []cli.Flag {
	var flags []cli.Flag
	for _, flag := range []*cli.DurationFlag{
		{Name: "wait", Usage: "wait for claimable work: --wait (15m), --wait=2m, or --wait 2m; idle_timeout is queue-local, not global completion"},
		{Name: "poll", Value: 30 * time.Second, Usage: "interval between empty observations; only --wait enables polling"},
	} {
		flag.Action = func(_ *cli.Context, value time.Duration) error {
			if value <= 0 {
				return fmt.Errorf("--%s must be a positive duration with units", flag.Name)
			}
			return nil
		}
		flags = append(flags, flag)
	}
	return flags
}

func nextWork(wait, poll time.Duration, selectWork func() (workflow.ImplementationOutcome, error)) (workflow.ImplementationOutcome, error) {
	deadline := time.Now().Add(wait)
	for {
		outcome, err := selectWork()
		if err != nil || outcome.Status != "no_work" || wait == 0 {
			return outcome, err
		}
		remaining := time.Until(deadline)
		if remaining > 0 {
			time.Sleep(min(poll, remaining))
		}
		if !time.Now().Before(deadline) {
			return workflow.ImplementationOutcome{Status: "idle_timeout", Reason: "no claimable work in this queue during the idle window; not global completion"}, nil
		}
	}
}
