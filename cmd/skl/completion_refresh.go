package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
)

type completionRefreshResult struct {
	Problems          map[string]string
	ObservationErrors map[string]error
	ForgeReadFailures map[string]bool
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

// refreshCompletions observes only the selected items' attached Submissions.
// Forge setup and reads are best-effort; ObserveCompletion owns the unlocked
// network read and locked revalidation/recording boundary.
func refreshCompletions(ctx context.Context, store *ledger.Store, repository github.RepositoryID, items []string, newBackend backendFactory) (completionRefreshResult, error) {
	result := completionRefreshResult{
		Problems:          make(map[string]string),
		ObservationErrors: make(map[string]error),
		ForgeReadFailures: make(map[string]bool),
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

	var forge ledger.CompletionForge
	var backendError error
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

	var reader *completionForgeReader
	if forge != nil {
		reader = &completionForgeReader{forge: forge}
		forge = reader
	}
	for _, item := range items {
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
	return result, nil
}

// A missing owned Submission is ordinary local state, and an unavailable
// forge cannot establish a terminal fact. Other ledger refusals protect
// project integrity and must stop delivery selection.
func completionRefreshIntegrityError(item string, err error, forgeReadFailure bool) bool {
	if forgeReadFailure {
		return false
	}
	var refusal *ledger.Refusal
	if !errors.As(err, &refusal) {
		return false
	}
	return refusal.Invariant != "forge observation is unavailable" &&
		refusal.Invariant != "no exact owned Submission is attached to "+item
}
