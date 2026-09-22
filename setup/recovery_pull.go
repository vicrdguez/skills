package setup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/workflow"
)

// recoverPullPresentation completes one phase pull presentation through the
// same GitHub surfaces as normal delivery. Ownership is validated without
// requiring the intended head, an expected source lag is caught up through
// the normal source-publication callback, and the intended exact head is
// re-read before any public body or approval. Already-satisfied effects are
// observed and never repeated.
func (b *GitHubBackend) recoverPullPresentation(ctx context.Context, presentation ledger.RecoveryPresentation, guard func() error) (ledger.RecoveryReceipt, error) {
	receipt := ledger.RecoveryReceipt{}
	repository := b.repository
	if presentation.Branch == "" || presentation.Head == "" {
		return receipt, workflow.Refuse("recovering a pull presentation requires the exact recorded source branch and intended head")
	}
	pull, err := b.locateRecoveryPull(ctx, presentation, guard, &receipt)
	if err != nil {
		return receipt, err
	}
	created := false
	if pull == nil {
		if presentation.MayHaveCreated {
			receipt.Status = recoveryAmbiguous
			receipt.Detail = "an earlier pull request creation remains unconfirmed and no pull request is observable on the recorded branch and base; inspect the repository rather than creating a duplicate"
			return receipt, nil
		}
		if presentation.ObserveOnly {
			receipt.Status = recoveryAmbiguous
			receipt.Detail = "no pull request attachment is observable and observation only makes no write"
			return receipt, nil
		}
		if presentation.Title == "" || presentation.Body == nil {
			receipt.Status = recoveryPending
			return receipt, workflow.Refuse("creating the pull request requires the recorded title and current public body")
		}
		if presentation.PrepareSource != nil {
			if err := guard(); err != nil {
				receipt.Status = recoveryPending
				return receipt, err
			}
			if err := presentation.PrepareSource(""); err != nil {
				receipt.Status = recoveryPending
				return receipt, err
			}
		}
		if err := guard(); err != nil {
			receipt.Status = recoveryPending
			return receipt, err
		}
		createdPull, err := b.createPresentedPull(ctx, repository, ledger.PullPresentation{Title: presentation.Title, Body: *presentation.Body, Branch: presentation.Branch, Head: presentation.Head})
		if err != nil {
			receipt.Status = recoveryAmbiguous
			receipt.Detail = err.Error()
			return receipt, nil
		}
		receipt.Number = createdPull.Number
		if reason := recoveryPullMismatch(*createdPull, repository, presentation); reason != "" {
			receipt.Status = recoveryPending
			return receipt, workflow.Refuse(reason + "; inspect the created pull request before retrying")
		}
		pull = createdPull
		created = true
	}
	if !created {
		if presentation.PrepareSource != nil && !presentation.ObserveOnly {
			if err := guard(); err != nil {
				receipt.Status = recoveryPending
				return receipt, err
			}
			if err := presentation.PrepareSource(pull.Head.SHA); err != nil {
				receipt.Status = recoveryPending
				return receipt, err
			}
		}
		if err := guard(); err != nil {
			receipt.Status = recoveryPending
			return receipt, err
		}
		fresh, err := b.pullForPresentation(ctx, repository, pull.Number)
		if err != nil {
			receipt.Status = recoveryPending
			return receipt, err
		}
		if reason := recoveryPullMismatch(*fresh, repository, presentation); reason != "" {
			receipt.Status = recoveryPending
			return receipt, workflow.Refuse(reason + "; inspect the pull request before retrying the same presentation")
		}
		pull = fresh
	}
	if pull.Head.SHA != presentation.Head {
		receipt.Status = recoveryPending
		receipt.Detail = fmt.Sprintf("pull request #%d presents source head %s, not the intended %s; neither the public body nor an approval was applied, so publish the intended source and retry", pull.Number, pull.Head.SHA, presentation.Head)
		return receipt, nil
	}

	// Findings are independent of body publication: each selected finding is
	// validated and reconciled on its own, so an unresolved or failed body
	// write neither publishes nor suppresses them.
	if len(presentation.Findings) > 0 {
		receipt.Findings, err = b.publishSelectedFindings(ctx, repository, *pull, presentation, guard)
		if err != nil {
			receipt.Status = recoveryPending
			return receipt, err
		}
	}

	wrote := created
	var problems []string
	if presentation.Body != nil && pull.Body != *presentation.Body {
		switch {
		case presentation.ObserveOnly:
			problems = append(problems, "the public body is not yet the supplied presentation")
		default:
			if err := guard(); err != nil {
				receipt.Status = recoveryPending
				return receipt, err
			}
			writeErr := b.request(ctx, http.MethodPatch, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d", pull.Number), map[string]string{"body": *presentation.Body}, nil)
			observed, readErr := b.pullForPresentation(ctx, repository, pull.Number)
			switch {
			case readErr != nil:
				problems = append(problems, fmt.Sprintf("the body update of pull request #%d could not be confirmed: %v", pull.Number, readErr))
			case observed.Body != *presentation.Body:
				if writeErr != nil {
					problems = append(problems, writeErr.Error())
				} else {
					problems = append(problems, fmt.Sprintf("pull request #%d did not present the supplied public body", pull.Number))
				}
			default:
				pull = observed
				wrote = true
			}
		}
	}

	wantedDraft := !presentation.Approved
	if pull.Draft != wantedDraft && !presentation.ObserveOnly {
		if pull.NodeID == "" {
			receipt.Status = recoveryPending
			return receipt, workflow.Refuse(fmt.Sprintf("pull request #%d has no stable node identity for the required %s presentation", pull.Number, presentationReadiness(wantedDraft)))
		}
		if err := guard(); err != nil {
			receipt.Status = recoveryPending
			return receipt, err
		}
		mutationErr := b.setPullPresentation(ctx, pull.NodeID, wantedDraft)
		observed, readErr := b.pullForPresentation(ctx, repository, pull.Number)
		switch {
		case readErr != nil:
			problems = append(problems, fmt.Sprintf("the readiness mutation of pull request #%d could not be confirmed: %v", pull.Number, readErr))
		case observed.Head.SHA != presentation.Head:
			// The source moved between the exact-head check and the readiness
			// mutation. Restore only a ready presentation this attempt
			// established for the reviewed body, never a newer one.
			restoreErr := b.restoreRecoveryDraft(ctx, repository, *observed, presentation)
			detail := fmt.Sprintf("pull request #%d presents source head %s after the readiness attempt instead of the intended %s", observed.Number, observed.Head.SHA, presentation.Head)
			if restoreErr != nil {
				detail += "; draft restoration remains unresolved: " + restoreErr.Error()
			}
			problems = append(problems, detail)
		case observed.Draft != wantedDraft:
			if mutationErr != nil {
				problems = append(problems, mutationErr.Error())
			} else {
				problems = append(problems, fmt.Sprintf("pull request #%d readiness was not observed as %s", observed.Number, presentationReadiness(wantedDraft)))
			}
		default:
			pull = observed
			wrote = true
		}
	}

	if len(problems) == 0 {
		if err := guard(); err != nil {
			receipt.Status = recoveryPending
			return receipt, err
		}
		final, err := b.pullForPresentation(ctx, repository, receipt.Number)
		if err != nil {
			receipt.Status = recoveryPending
			return receipt, fmt.Errorf("pull request #%d could not be re-read to confirm the presentation: %v", receipt.Number, err)
		}
		switch {
		case recoveryPullMismatch(*final, repository, presentation) != "":
			problems = append(problems, recoveryPullMismatch(*final, repository, presentation))
		case final.Head.SHA != presentation.Head:
			problems = append(problems, fmt.Sprintf("pull request #%d presents source head %s, not the intended %s", final.Number, final.Head.SHA, presentation.Head))
		case presentation.Body != nil && final.Body != *presentation.Body:
			problems = append(problems, "the public body is not yet the supplied presentation")
		case final.Draft != wantedDraft:
			problems = append(problems, fmt.Sprintf("pull request #%d readiness is not observable as %s", final.Number, presentationReadiness(wantedDraft)))
		}
	}

	if len(problems) > 0 {
		receipt.Status = recoveryPending
		receipt.Detail = strings.Join(problems, "; ")
		return receipt, nil
	}
	if wrote {
		receipt.Status = recoveryPublished
	} else {
		receipt.Status = recoveryAlreadySatisfied
	}
	return receipt, nil
}

// restoreRecoveryDraft reverses only a ready presentation this attempt
// established after the source moved away from the intended revision. An
// observed newer body, or the absence of a stable node identity, leaves the
// newer presentation untouched. A ready result that was merely confirmed
// despite a lost response is never reversed by this path.
func (b *GitHubBackend) restoreRecoveryDraft(ctx context.Context, repository github.RepositoryID, observed githubPull, presentation ledger.RecoveryPresentation) error {
	if observed.Draft || observed.NodeID == "" {
		return nil
	}
	if presentation.Body != nil && observed.Body != *presentation.Body {
		return errors.New("the pull request changed since the readiness attempt; newer content was preserved")
	}
	if err := b.setPullPresentation(ctx, observed.NodeID, true); err != nil {
		return err
	}
	confirmed, err := b.pullForPresentation(ctx, repository, observed.Number)
	if err != nil {
		return err
	}
	if !confirmed.Draft {
		return errors.New("draft restoration was not observed")
	}
	return nil
}
