package ledger

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

// Merge attempt outcomes. Neither is persisted: Merged arrives through
// completion observation of the merged pull request.
const (
	MergeMerged    = "merged"
	MergeRefused   = "refused"
	MergeUncertain = "uncertain"
)

// ErrMergeUncertain marks a dispatched merge whose effect was not confirmed.
var ErrMergeUncertain = errors.New("pull request merge outcome is uncertain")

// PullMerge asks the forge to squash-merge one owned pull request into Base,
// only while it still shows Head.
type PullMerge struct {
	Number             int
	Branch, Head, Base string
	Current            func() error `json:"-"`
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
	// Readiness is separate from public-body publication and merge completion.
	Readiness *PublicationNote `json:"readiness,omitempty"`
	Review    string           `json:"review,omitempty"`
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
	return mergeReady(ctx, s, repository, root, remote, item, forge, false)
}

// RecoverAndMergeReady derives readiness from the current PASS and established
// attachment, preserving public prose and never synchronizing source.
func RecoverAndMergeReady(ctx context.Context, s *Store, repository github.RepositoryID, root, remote, item string, forge MergeForge) MergeAttempt {
	return mergeReady(ctx, s, repository, root, remote, item, forge, true)
}

func mergeReady(ctx context.Context, s *Store, repository github.RepositoryID, root, remote, item string, forge MergeForge, recoverReadiness bool) MergeAttempt {
	selected, err := SelectCurrentResult(s, repository, item)
	if err != nil {
		return RefusedMerge(item, err.Error())
	}
	attempt := RefusedMerge(item, "")
	attempt.Review = selected.Outcome
	if recoverReadiness {
		attempt.Readiness = &PublicationNote{Status: "not_attempted", Detail: "readiness recovery did not run; existing public prose was preserved"}
	}
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
		return refused("no pull request is attached to " + item + "; explicit PR recovery is required before Auto Mode can recover readiness; no discovery or creation was attempted")
	}
	if selected.Phase != WatchdogPhase || selected.Source.Head == "" {
		return refused("the passing review records no final head")
	}
	head := selected.Source.Head
	current := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := selected.superseded(s, repository); err != nil {
			return err
		}
		state, directory, ledgerHead, err := s.deliveryState(repository, item)
		if err != nil {
			return err
		}
		if state.Branch != selected.Branch || !sameAttachment(state.Submission, selected.Submission) {
			return fmt.Errorf("the recorded PR/source binding changed since selection")
		}
		open, err := s.openProposalTarget(ledgerHead, directory, state.Target)
		if err != nil {
			return err
		}
		if !open {
			return fmt.Errorf("the Proposal Branch is no longer open")
		}
		identity := repository.Owner + "/" + repository.Name
		if !strings.EqualFold(selected.Submission.Repository, identity) || !strings.EqualFold(selected.Target.Repository, identity) || selected.Submission.Number <= 0 {
			return fmt.Errorf("the recorded PR or target repository does not match %s", identity)
		}
		observed, err := git(root, "ls-remote", "--exit-code", remote, "refs/heads/"+selected.Branch)
		fields := strings.Fields(observed)
		if err != nil || len(fields) != 2 || fields[0] != head || fields[1] != "refs/heads/"+selected.Branch {
			return fmt.Errorf("remote source branch %s could not be verified at final PASS head %s; source was left untouched", selected.Branch, head)
		}
		return nil
	}
	if err := current(); err != nil {
		return refused(err.Error())
	}
	if forge == nil {
		return refused("forge access is unavailable")
	}
	if recoverReadiness {
		attempt.Readiness = &PublicationNote{Status: IssuePending}
		readinessForge, ok := forge.(ReadinessForge)
		if !ok {
			attempt.Readiness.Detail = "the forge adapter cannot recover readiness"
			return refused(attempt.Readiness.Detail)
		}
		err := readinessForge.ReadyPull(ctx, PullReadiness{Number: selected.Submission.Number, Branch: selected.Branch, Head: head, Base: attempt.Branch, Current: current})
		if err != nil {
			if errors.Is(err, ErrPresentationUncertain) {
				attempt.Readiness.Status = IssueUncertain
			}
			attempt.Readiness.Detail = err.Error()
			return refused("readiness recovery was not confirmed: " + err.Error())
		}
		attempt.Readiness.Status = "ready"
		attempt.Readiness.Detail = "native readiness confirmed; existing PR body and title preserved, not republished; latest review narrative publication is not established"
	}
	if err := current(); err != nil {
		return refused(err.Error())
	}
	if _, err := workflow.RequireBranchAncestor(root, remote, attempt.Branch, selected.Branch, head); err != nil {
		return refused(err.Error() + "; historical PASS and Ready for Merge are preserved. For fresh integration and review, a human can use skl decision apply --route watchdog with --project " + repository.Name + " --item " + item + " --request-commit " + selected.Report.Commit + " --request-path " + selected.Report.Path + " --answer <human-answer-file>; reselect current request context if it changes")
	}
	if err := current(); err != nil {
		return refused(err.Error())
	}
	commit, err := forge.MergePull(ctx, PullMerge{Number: selected.Submission.Number, Branch: selected.Branch, Head: head, Base: attempt.Branch, Current: current})
	if err != nil {
		if errors.Is(err, ErrMergeUncertain) {
			attempt.Status = MergeUncertain
			return refused(fmt.Sprintf("pull request #%d merge was not confirmed: %v", selected.Submission.Number, err))
		}
		return refused(fmt.Sprintf("pull request #%d was not merged: %v", selected.Submission.Number, err))
	}
	attempt.Status, attempt.Commit = MergeMerged, commit
	return attempt
}

// PendingMerges lists the Project's Ready-for-Merge Slices whose pull request
// target is an open Proposal Branch, in stable order. Missing attachments are
// included so the lane reports explicit recovery instead of silently skipping.
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
		if state.State != ReadyForMerge {
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
