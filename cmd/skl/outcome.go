package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/setup"
)

// Claim states an Outcome Instruction reports. The empty state means the
// operation concerns no Claim of the worker's.
const (
	claimKept      = "kept"
	claimNone      = "none"
	claimUncertain = "uncertain"
	claimUnchanged = "unchanged"
)

// refusalFacts are what the refused outcome renders: the refusal's own
// invariant and repair, the worker's Claim state, and the bound commands.
type refusalFacts struct {
	Status        string
	Reason        string
	Repair        string
	ClaimState    string
	Claim         string
	StatusCommand string
	Rerun         string
	// Stop replaces the rerun: the refusal ends a Supervisor's lane.
	Stop     bool
	Previous *ledger.ClaimEnding
}

// phaseFacts serve the outcomes that concern one delivery phase and nothing
// else: no work, idle timeout and an interrupted wait.
type phaseFacts struct {
	Status        string
	Phase         string
	StatusCommand string
	Previous      *ledger.ClaimEnding
}

type releasedFacts struct {
	Status string
	Phase  string
	Item   string
	Claim  string
	Next   string
}

type renderingFailedFacts struct {
	Status  string
	Phase   string
	Item    string
	Claim   string
	Reason  string
	Resume  string
	Release string
}

// handoffFacts are one committed handoff. Notes keep a fixed order: ledger
// replication, then public presentation.
type handoffFacts struct {
	Status           string
	Phase            string
	Item             string
	Report           ledger.Reference
	AlreadyCompleted bool
	Notes            []noteFact
	Present          string
}

type noteFact struct {
	Label  string
	Status string
	Detail string
}

// notesOf lists the present notes in their fixed order.
func notesOf(replication, publication *ledger.PublicationNote) []noteFact {
	var notes []noteFact
	for _, note := range []struct {
		label string
		note  *ledger.PublicationNote
	}{{"Ledger replication", replication}, {"Public presentation", publication}} {
		if note.note != nil {
			notes = append(notes, noteFact{Label: note.label, Status: note.note.Status, Detail: note.note.Detail})
		}
	}
	return notes
}

// writeOutcome renders one outcome kind's Outcome Instruction to stdout.
func writeOutcome(stdout io.Writer, kind string, facts any) error {
	text, err := skilldist.RenderOutcome(kind, facts)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, text)
	return err
}

// refusalParts separates a ledger refusal's repair from its invariant, keeping
// any context that wraps the invariant. Other errors are their own reason.
func refusalParts(err error) (reason, repair string) {
	var refusal *ledger.Refusal
	if errors.As(err, &refusal) {
		if reason, ok := strings.CutSuffix(err.Error(), "; "+refusal.Repair); ok {
			return reason, refusal.Repair
		}
	}
	return err.Error(), ""
}

// boundCommand is the exact invocation to rerun: the command path followed by
// every flag the caller set, in declared order.
func boundCommand(c *cli.Context, path string) string {
	words := []string{path}
	for _, flag := range c.Command.Flags {
		words = append(words, flagWords(c, flag.Names()[0])...)
	}
	return strings.Join(words, " ")
}

// flagWords repeats one flag as the caller set it, or nothing when unset.
func flagWords(c *cli.Context, name string) []string {
	if !c.IsSet(name) {
		return nil
	}
	if inputs, ok := c.Generic(name).(*rawInputs); ok {
		var words []string
		for _, input := range *inputs {
			words = append(words, "--"+name, skilldist.ShellQuote(input))
		}
		return words
	}
	switch value := c.Value(name).(type) {
	case bool:
		if value {
			return []string{"--" + name}
		}
		return nil
	case time.Duration:
		return []string{"--" + name + "=" + value.String()}
	default:
		return []string{"--" + name, skilldist.ShellQuote(fmt.Sprint(value))}
	}
}

// claimCommand binds one Claim-scoped delivery command.
func claimCommand(phase, operation string, repository setup.RepositoryContext, item, claim string) string {
	q := skilldist.ShellQuote
	return fmt.Sprintf("skl %s %s --repo %s --remote %s --item %s --claim %s", phase, operation, q(repository.Root), q(repository.Remote), q(item), q(claim))
}

// statusInvocation binds the command that shows which Work Items hold a Claim.
func statusInvocation(repository setup.RepositoryContext) string {
	q := skilldist.ShellQuote
	return fmt.Sprintf("skl status --repo %s --remote %s", q(repository.Root), q(repository.Remote))
}

// nextInvocation binds the command that selects the phase's next Work Item.
func nextInvocation(phase string, repository setup.RepositoryContext) string {
	q := skilldist.ShellQuote
	return fmt.Sprintf("skl %s next --repo %s --remote %s", phase, q(repository.Root), q(repository.Remote))
}
