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
}

// phaseFacts serve the outcomes that concern one delivery phase and nothing
// else: no work, idle timeout and an interrupted wait.
type phaseFacts struct {
	Status        string
	Phase         string
	StatusCommand string
}

type releasedFacts struct {
	Status string
	Phase  string
	Item   string
	Claim  string
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
		name := flag.Names()[0]
		if !c.IsSet(name) {
			continue
		}
		if inputs, ok := c.Generic(name).(*rawInputs); ok {
			for _, input := range *inputs {
				words = append(words, "--"+name, skilldist.ShellQuote(input))
			}
			continue
		}
		switch value := c.Value(name).(type) {
		case bool:
			if value {
				words = append(words, "--"+name)
			}
		case time.Duration:
			words = append(words, "--"+name+"="+value.String())
		default:
			words = append(words, "--"+name, skilldist.ShellQuote(fmt.Sprint(value)))
		}
	}
	return strings.Join(words, " ")
}

// claimCommand binds one Claim-scoped delivery command.
func claimCommand(phase, operation string, repository setup.RepositoryContext, item, claim string) string {
	q := skilldist.ShellQuote
	return fmt.Sprintf("skl %s %s --repo %s --remote %s --item %s --claim %s", phase, operation, q(repository.Root), q(repository.Remote), q(item), q(claim))
}

// statusCommand binds the command that shows which Work Items hold a Claim.
func statusInvocation(repository setup.RepositoryContext) string {
	q := skilldist.ShellQuote
	return fmt.Sprintf("skl status --repo %s --remote %s", q(repository.Root), q(repository.Remote))
}
