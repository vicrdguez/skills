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
	refreshed, err := refreshCompletions(command.Context, store, repository, items, newBackend)
	if err != nil {
		return renderLedgerRefusal(output, format, err)
	}
	problems := refreshed.Problems
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
	for index := range proposals {
		proposals[index].Refresh = refreshed.Proposals[proposals[index].Proposal]
		proposals[index].Observation = refreshed.ProposalProblems[proposals[index].Proposal]
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
		if proposal.Branch != "" {
			fmt.Fprintf(&text, "  Proposal Branch: %s (%s)\n", proposal.Branch, proposal.BranchState)
		}
		if proposal.Submission != nil {
			fmt.Fprintf(&text, "  Proposal Submission: %s#%d\n", proposal.Submission.Repository, proposal.Submission.Number)
		}
		if proposal.Completion != nil {
			fmt.Fprintf(&text, "  Proposal completion: into %s %s at %s\n", proposal.Completion.Target.Repository, proposal.Completion.Target.Branch, proposal.Completion.MergeCommit)
		}
		if refresh := proposal.Refresh; refresh != nil {
			if refresh.ClosedUnmerged {
				fmt.Fprintln(&text, "  Observation: Proposal Submission closed without merge; nothing was recorded")
			}
			if refresh.Presentation != nil {
				fmt.Fprintf(&text, "  Proposal presentation: %s: %s\n", refresh.Presentation.Status, refresh.Presentation.Detail)
			}
			if refresh.Replication != nil && refresh.Replication.Status != ledger.PushPushed {
				fmt.Fprintf(&text, "  Pending ledger replication: %s: %s\n", refresh.Replication.Status, refresh.Replication.Detail)
			}
		}
		if proposal.Observation != "" {
			fmt.Fprintf(&text, "  Observation unavailable: %s\n", proposal.Observation)
		}
	}
	_, err = io.WriteString(output, text.String())
	return err
}
