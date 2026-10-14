package main

import (
	"cmp"
	"fmt"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
)

// Selection is explicit: neither the environment nor another harness supplies
// defaults. The CLI owns resolution; Workflow Mechanics only sees Claims.
func selectedLaunchRoles(config ledger.Config, harness string) (ledger.LaunchRoles, error) {
	switch harness {
	case "":
		return ledger.LaunchRoles{}, nil
	case "pi":
		return config.Harnesses.Pi, nil
	case "opencode":
		return config.Harnesses.OpenCode, nil
	case "claude":
		return config.Harnesses.Claude, nil
	case "codex":
		return config.Harnesses.Codex, nil
	default:
		return ledger.LaunchRoles{}, fmt.Errorf("unknown harness %q; use pi, opencode, claude or codex", harness)
	}
}

func resolveLaunchChoice(explicit skilldist.SubagentChoice, configured ledger.LaunchChoice) skilldist.SubagentChoice {
	return skilldist.SubagentChoice{
		Model:    cmp.Or(explicit.Model, configured.Model),
		Thinking: cmp.Or(explicit.Thinking, configured.Thinking),
	}
}
