package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
)

type completionRefreshResult struct {
	Problems          map[string]string
	ObservationErrors map[string]error
	ForgeReadFailures map[string]bool
	// Proposals and ProposalProblems report each Proposal Submission
	// refresh. They never block selection.
	Proposals        map[string]*ledger.ProposalRefresh
	ProposalProblems map[string]string
}

type completionForgeReader struct {
	forge   ledger.CompletionForge
	lastErr error
}

func (reader *completionForgeReader) ObserveSubmission(ctx context.Context, attachment ledger.ForgeAttachment, target ledger.IntegrationTarget, branch string) (ledger.SubmissionObservation, error) {
	observation, err := reader.forge.ObserveSubmission(ctx, attachment, target, branch)
	reader.lastErr = err
	return observation, err
}

// refreshCompletions observes the selected items' attached Submissions, then
// refreshes their Proposals' Submissions. Forge setup and reads are
// best-effort; ObserveCompletion and RefreshProposal own the unlocked network
// reads and locked revalidation/recording boundaries.
func refreshCompletions(ctx context.Context, store *ledger.Store, repository github.RepositoryID, items []string, newBackend backendFactory) (completionRefreshResult, error) {
	result := completionRefreshResult{
		Problems:          make(map[string]string),
		ObservationErrors: make(map[string]error),
		ForgeReadFailures: make(map[string]bool),
		Proposals:         make(map[string]*ledger.ProposalRefresh),
		ProposalProblems:  make(map[string]string),
	}
	needsForge := false
	for _, item := range items {
		state, _, err := ledger.CompletionStatus(store, repository, []string{item}, false, nil)
		if err != nil {
			return result, err
		}
		if state[0].Submission != nil && state[0].State != ledger.Merged && state[0].State != ledger.Superseded {
			needsForge = true
		}
	}

	connect := sync.OnceValues(func() (any, error) { return newBackend(repository) })
	var backendError error
	var forge ledger.CompletionForge
	if needsForge {
		if backend, err := connect(); err != nil {
			backendError = err
		} else {
			forge, _ = backend.(ledger.CompletionForge)
			if forge == nil {
				backendError = fmt.Errorf("the forge adapter cannot observe an attached Submission")
			}
		}
	}

	var reader *completionForgeReader
	if forge != nil {
		reader = &completionForgeReader{forge: forge}
		forge = reader
	}
	var proposals []string
	for _, item := range items {
		if proposal, _, _ := strings.Cut(item, "/"); !slices.Contains(proposals, proposal) {
			proposals = append(proposals, proposal)
		}
		if reader != nil {
			reader.lastErr = nil
		}
		_, err := ledger.ObserveCompletion(ctx, store, repository, item, forge)
		if reader != nil && reader.lastErr != nil {
			result.ForgeReadFailures[item] = true
		}
		if err == nil {
			continue
		}
		result.ObservationErrors[item] = err
		if backendError != nil && strings.Contains(err.Error(), "forge observation is unavailable") {
			result.Problems[item] = backendError.Error()
		} else {
			result.Problems[item] = err.Error()
		}
	}
	proposalForge := func() (ledger.ProposalForge, error) {
		backend, err := connect()
		if err != nil {
			return nil, err
		}
		forge, _ := backend.(ledger.ProposalForge)
		if forge == nil {
			return nil, fmt.Errorf("the forge adapter cannot present or observe a Proposal Submission")
		}
		return forge, nil
	}
	for _, proposal := range proposals {
		refreshed, err := ledger.RefreshProposal(ctx, store, repository, proposal, proposalForge)
		if refreshed != nil {
			result.Proposals[proposal] = refreshed
		}
		if err != nil {
			result.ProposalProblems[proposal] = err.Error()
		}
	}
	return result, nil
}

// A missing owned Submission is ordinary local state, and an unavailable
// forge cannot establish a terminal fact. Forge read failures are marked
// separately; every other observation error must stop delivery selection.
func completionRefreshIntegrityError(item string, err error, forgeReadFailure bool) bool {
	if err == nil || forgeReadFailure {
		return false
	}
	var refusal *ledger.Refusal
	if errors.As(err, &refusal) {
		return refusal.Invariant != "forge observation is unavailable" &&
			refusal.Invariant != "no exact owned Submission is attached to "+item
	}
	return true
}
