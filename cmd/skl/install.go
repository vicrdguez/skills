package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	skilldist "github.com/vicrdguez/skills"
)

func installSkills(home string, stdin io.Reader, stdout io.Writer) error {
	plan, err := skilldist.PlanInstall(home)
	if err != nil {
		return fmt.Errorf("could not preview installation: %w", err)
	}
	if err := printPreserved(stdout, plan.Preserved); err != nil {
		return err
	}
	if len(plan.Actions) == 0 {
		_, err := fmt.Fprintln(stdout, "Already up to date")
		return err
	}
	if err := printInstallActions(stdout, "Planned changes:", plan.Actions); err != nil {
		return err
	}
	if _, err := fmt.Fprint(stdout, "Install these changes? [y/N] "); err != nil {
		return err
	}
	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	answer = strings.TrimSpace(answer)
	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		_, err := fmt.Fprintln(stdout, "Installation cancelled. No changes made.")
		return err
	}

	outcome, installErr := skilldist.Install(home)
	if err := printInstallActions(stdout, "Completed changes:", outcome.Completed); err != nil {
		return errors.Join(installErr, err)
	}
	if err := printPreserved(stdout, outcome.Preserved); err != nil {
		return errors.Join(installErr, err)
	}
	if installErr != nil {
		if outcome.Failed != nil {
			if _, err := fmt.Fprintf(stdout, "Failed: %s %s\n", outcome.Failed.Action, outcome.Failed.Path); err != nil {
				return errors.Join(installErr, err)
			}
		}
		_, err := fmt.Fprintln(stdout, "Installation did not finish. Completed changes were retained.")
		return errors.Join(installErr, err)
	}
	_, err = fmt.Fprintln(stdout, "Installation completed successfully.")
	return err
}

func printInstallActions(out io.Writer, heading string, actions []skilldist.InstallAction) error {
	if _, err := fmt.Fprintln(out, heading); err != nil {
		return err
	}
	for _, action := range actions {
		if _, err := fmt.Fprintf(out, "  %s %s\n", action.Action, action.Path); err != nil {
			return err
		}
	}
	return nil
}

func printPreserved(out io.Writer, paths []string) error {
	for _, path := range paths {
		if _, err := fmt.Fprintf(out, "Preserved user-owned collision: %s\n", path); err != nil {
			return err
		}
	}
	return nil
}
