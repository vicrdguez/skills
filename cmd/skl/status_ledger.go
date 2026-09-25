package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
)

type ledgerStatusOutcome struct {
	Status    string                  `json:"status"`
	Items     []ledger.StatusItem     `json:"items"`
	Proposals []ledger.ProposalStatus `json:"proposals,omitempty"`
}

func ledgerStatus(command *cli.Context, store *ledger.Store, repository github.RepositoryID, newBackend backendFactory, output io.Writer, format implementationFormatKind) error {
	selected := command.String("item")
	items, err := ledger.StatusItems(store, repository, selected)
	if err != nil {
		return renderLedgerRefusal(output, format, err)
	}
	var forge ledger.CompletionForge
	var backendError error
	needsForge := false
	// Only a nonterminal, attached item needs a forge read. Stored facts and
	// records lacking an attachment remain locally readable while offline.
	for _, item := range items {
		state, _, err := ledger.CompletionStatus(store, repository, []string{item}, false, nil)
		if err != nil {
			return renderLedgerRefusal(output, format, err)
		}
		if state[0].Submission != nil && state[0].State != ledger.Merged && state[0].State != ledger.Superseded {
			needsForge = true
		}
	}
	if needsForge {
		backend, err := newBackend(repository)
		if err != nil {
			backendError = err
		} else {
			forge, _ = backend.(ledger.CompletionForge)
			if forge == nil {
				backendError = fmt.Errorf("the forge adapter cannot observe an attached Submission")
			}
		}
	}
	problems := make(map[string]string)
	for _, item := range items {
		_, err := ledger.ObserveCompletion(command.Context, store, repository, item, forge)
		if err != nil {
			if backendError != nil && strings.Contains(err.Error(), "forge observation is unavailable") {
				problems[item] = backendError.Error()
			} else {
				problems[item] = err.Error()
			}
		}
	}
	if selected == "" {
		// Refresh project membership after network reads; a concurrently
		// accepted child must not be omitted from parent accounting.
		items, err = ledger.StatusItems(store, repository, "")
		if err != nil {
			return renderLedgerRefusal(output, format, err)
		}
	}
	states, proposals, err := ledger.CompletionStatus(store, repository, items, selected == "", problems)
	if err != nil {
		return renderLedgerRefusal(output, format, err)
	}
	outcome := ledgerStatusOutcome{Status: "shown", Items: states, Proposals: proposals}
	if format == formatJSON {
		return json.NewEncoder(output).Encode(outcome)
	}
	var text strings.Builder
	fmt.Fprintln(&text, "Status: shown")
	for _, state := range states {
		fmt.Fprintf(&text, "Work Item: %s (%s)\n", state.Item, state.State)
		if state.Claimed {
			fmt.Fprintln(&text, "  Claim: retained")
		}
		if state.Completion != nil {
			fmt.Fprintf(&text, "  Confirmed Submission: %s#%d into %s %s\n", state.Completion.Submission.Repository, state.Completion.Submission.Number, state.Completion.Target.Repository, state.Completion.Target.Branch)
			if state.Completion.SourceHead != "" {
				fmt.Fprintf(&text, "  Accepted source head: %s\n", state.Completion.SourceHead)
			}
			if state.Completion.MergeCommit != "" {
				fmt.Fprintf(&text, "  Merge revision: %s\n", state.Completion.MergeCommit)
			}
		} else if state.Submission == nil {
			fmt.Fprintln(&text, "  Observation: no owned Submission attachment")
		}
		if state.Observation != "" {
			fmt.Fprintf(&text, "  Observation unavailable: %s\n", state.Observation)
		}
		for _, dependency := range state.Dependencies {
			fmt.Fprintf(&text, "  Depends on: %s (%s)\n", dependency.Item, dependency.State)
		}
		if state.Pending != nil && state.Pending.Push != nil {
			fmt.Fprintf(&text, "  Pending ledger replication: %s: %s\n", state.Pending.Push.Status, state.Pending.Push.Detail)
		}
	}
	for _, proposal := range proposals {
		fmt.Fprintf(&text, "Proposal: %s (fully delivered: %t, safe to retire: %t)\n", proposal.Proposal, proposal.FullyDelivered, proposal.Retireable)
	}
	_, err = io.WriteString(output, text.String())
	return err
}
