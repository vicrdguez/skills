package ledger

import (
	"context"
	"fmt"
	"path"
	"sort"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

// Merge attempt outcomes. Neither is persisted: Merged arrives through
// completion observation of the merged pull request.
const (
	MergeMerged  = "merged"
	MergeRefused = "refused"
)

// PullMerge asks the forge to squash-merge one owned pull request into Base,
// only while it still shows Head.
type PullMerge struct {
	Number             int
	Branch, Head, Base string
}

// MergeForge squash-merges an owned pull request and returns the commit that
// landed. An already merged pull request at Head is success; any other
// refusal is an error naming the forge's reason.
type MergeForge interface {
	MergePull(context.Context, PullMerge) (string, error)
}

// MergeAttempt is the reported result of one engine merge into a Proposal
// Branch: merged with its commit, or refused with the reason.
type MergeAttempt struct {
	Item   string `json:"item"`
	Status string `json:"status"`
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// RefusedMerge reports a merge that was not attempted for reason.
func RefusedMerge(item, reason string) MergeAttempt {
	return MergeAttempt{Item: item, Status: MergeRefused, Reason: reason}
}

// MergeReady squash-merges a Ready-for-Merge Slice's pull request into its
// open Proposal Branch when the branch's current remote head is an ancestor of
// the final reviewed head. It reads Workflow State and writes none; every
// failure is a refusal in the returned attempt.
func MergeReady(ctx context.Context, s *Store, repository github.RepositoryID, root, remote, item string, forge MergeForge) MergeAttempt {
	selected, err := SelectCurrentResult(s, repository, item)
	if err != nil {
		return RefusedMerge(item, err.Error())
	}
	attempt := RefusedMerge(item, "")
	if selected.Target != nil {
		attempt.Branch = selected.Target.Branch
	}
	refused := func(reason string) MergeAttempt { attempt.Reason = reason; return attempt }
	if selected.Lifecycle != ReadyForMerge {
		return refused("Work Item " + item + " is " + selected.Lifecycle + ", not " + ReadyForMerge)
	}
	_, directory, ledgerHead, err := s.deliveryState(repository, item)
	if err != nil {
		return refused(err.Error())
	}
	open, err := s.openProposalTarget(ledgerHead, directory, selected.Target)
	if err != nil {
		return refused(err.Error())
	}
	if !open {
		return refused("Work Item " + item + " does not target an open Proposal Branch; a human merges it")
	}
	if selected.Submission == nil {
		return refused("no pull request is attached to " + item)
	}
	if selected.Phase != WatchdogPhase || selected.Source.Head == "" {
		return refused("the passing review records no final head")
	}
	head := selected.Source.Head
	if _, err := workflow.RequireBranchAncestor(root, remote, attempt.Branch, selected.Branch, head); err != nil {
		return refused(err.Error())
	}
	if forge == nil {
		return refused("forge access is unavailable")
	}
	commit, err := forge.MergePull(ctx, PullMerge{Number: selected.Submission.Number, Branch: selected.Branch, Head: head, Base: attempt.Branch})
	if err != nil {
		return refused(fmt.Sprintf("pull request #%d was not merged: %v", selected.Submission.Number, err))
	}
	attempt.Status, attempt.Commit = MergeMerged, commit
	return attempt
}

// PendingMerges lists the Project's Ready-for-Merge Slices whose pull request
// is attached and whose target is an open Proposal Branch, in stable order.
func PendingMerges(s *Store, repository github.RepositoryID) ([]string, error) {
	items, err := StatusItems(s, repository, "")
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, item := range items {
		state, directory, head, err := s.deliveryState(repository, item)
		if err != nil {
			return nil, err
		}
		if state.State != ReadyForMerge || state.Submission == nil {
			continue
		}
		if open, err := s.openProposalTarget(head, directory, state.Target); err != nil {
			return nil, err
		} else if open {
			pending = append(pending, item)
		}
	}
	sort.Strings(pending)
	return pending, nil
}

// openProposalTarget reports whether target is the Proposal Branch of the
// Slice recorded at directory while that Proposal is open at the ledger head.
// main is never such a target.
func (s *Store) openProposalTarget(head, directory string, target *IntegrationTarget) (bool, error) {
	if target == nil || target.Branch == "" || target.Branch == "main" {
		return false, nil
	}
	var meta ProposalMeta
	if err := readJSONAt(s, head, path.Join(path.Dir(directory), "proposal.json"), &meta); err != nil {
		return false, err
	}
	return proposalOpen(meta) && *meta.Target == *target, nil
}
