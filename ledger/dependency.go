package ledger

import (
	"path/filepath"
	"strings"
)

// proposalOpen reports a Proposal Branch that still collects Slice merges.
// Completion or retirement spends it for good.
func proposalOpen(meta ProposalMeta) bool {
	return meta.Target != nil && meta.Completion == nil && !meta.Retired
}

// Proposal Branch states shown by status and browse.
const (
	BranchOpen  = "open"
	BranchSpent = "spent"
)

// proposalBranchState is BranchOpen, BranchSpent, or empty for a Proposal
// whose branch was never opened.
func proposalBranchState(meta ProposalMeta) string {
	switch {
	case meta.Target == nil:
		return ""
	case proposalOpen(meta):
		return BranchOpen
	}
	return BranchSpent
}

// dependencyBuildable requires a confirmed merge into a branch the dependent
// can build on. Another Proposal's branch counts only after that Proposal has
// recorded completion into main.
func dependencyBuildable(blocker SliceState, blockerProposal, dependentProposal ProposalMeta) bool {
	if blocker.State != Merged || blocker.Completion == nil {
		return false
	}
	branch := blocker.Completion.Target.Branch
	if branch == "main" {
		return true
	}
	if dependentProposal.Branch != "" && branch == dependentProposal.Branch {
		return true
	}
	return blockerProposal.Branch != "" && branch == blockerProposal.Branch && blockerProposal.Completion != nil && blockerProposal.Completion.Target.Branch == "main"
}

func (s *Store) dependencySatisfiedAt(head, project, dependent, reference string) (bool, error) {
	blocker, slice, ok := strings.Cut(strings.TrimPrefix(reference, "proposals/"), "/")
	if !ok {
		return false, nil
	}
	var state SliceState
	if err := readJSONAt(s, head, filepath.Join(s.proposalDirectoryAt(head, project, blocker), slice, "state.json"), &state); err != nil {
		return false, err
	}
	var blockerMeta, dependentMeta ProposalMeta
	if err := readJSONAt(s, head, filepath.Join(s.proposalDirectoryAt(head, project, blocker), "proposal.json"), &blockerMeta); err != nil {
		return false, err
	}
	if blocker == dependent {
		dependentMeta = blockerMeta
	} else if err := readJSONAt(s, head, filepath.Join(s.proposalDirectoryAt(head, project, dependent), "proposal.json"), &dependentMeta); err != nil {
		return false, err
	}
	return dependencyBuildable(state, blockerMeta, dependentMeta), nil
}
