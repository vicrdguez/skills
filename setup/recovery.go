package setup

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/workflow"
)

var _ ledger.RecoveryForge = (*GitHubBackend)(nil)

// Recovery receipt vocabulary understood by the ledger's publication
// normalization. The shared recovery types do not export these constants, so
// the adapter repeats the exact values the ledger recognizes.
const (
	recoveryPublished         = "published"
	recoveryAlreadySatisfied  = "already-satisfied"
	recoveryPending           = "pending"
	recoveryAmbiguous         = "ambiguous"
	recoveryFindingSatisfied  = "satisfied"
	recoveryFindingUnresolved = "unresolved"
)

// RecoverPresentation reconciles one pending public presentation against the
// bound repository. It observes the recorded attachment, or an unambiguous
// exact identity for a create whose response was lost, before it writes
// anything; it never guesses a similar title, adopts an arbitrary match, or
// removes private evidence from the ledger. Every mutation and decision-grade
// readback is guarded.
//
// A partial or ambiguous outcome is returned as a described receipt with a
// nil error so the caller can preserve a known number and record the
// remaining work; an error is reserved for an outcome the adapter cannot
// describe, such as a guard failure, an unavailable read, or a conflict that
// must be repaired before anything else happens.
func (b *GitHubBackend) RecoverPresentation(ctx context.Context, presentation ledger.RecoveryPresentation) (ledger.RecoveryReceipt, error) {
	if err := b.requireRepository(); err != nil {
		return ledger.RecoveryReceipt{}, err
	}
	guard := presentation.Guard
	if guard == nil {
		guard = func() error { return nil }
	}
	switch presentation.Kind {
	case "issue", "parent":
		return b.recoverIssuePresentation(ctx, presentation, guard)
	case "pull":
		return b.recoverPullPresentation(ctx, presentation, guard)
	default:
		return ledger.RecoveryReceipt{}, fmt.Errorf("unsupported recovery presentation kind %q; select an issue, parent, or pull presentation", presentation.Kind)
	}
}

// recoveryBodyDigest is the exact SHA-256 of one public body. It lets an
// issue created without a recorded attachment be recognized from its original
// bytes after the response was lost or the temporary prose is gone, which is
// stronger evidence than a similar title or a decision-shaped comment.
func recoveryBodyDigest(body string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
}

// recoverIssuePresentation completes one descriptive issue or multi-slice
// parent presentation. A known attachment is read and validated as-is; an
// uncertain create is attributed only by its exact recorded title and
// original body digest; a fresh initial create is allowed only when no
// uncertain attempt exists.
func (b *GitHubBackend) recoverIssuePresentation(ctx context.Context, presentation ledger.RecoveryPresentation, guard func() error) (ledger.RecoveryReceipt, error) {
	receipt := ledger.RecoveryReceipt{Number: presentation.Number}
	number := presentation.Number
	wrote := false
	if number == 0 {
		if presentation.MayHaveCreated {
			matched, err := b.observeRecordedIssue(ctx, presentation, guard)
			if err != nil {
				return receipt, err
			}
			if matched == 0 {
				receipt.Status = recoveryAmbiguous
				receipt.Detail = "an earlier " + presentation.Kind + " issue creation remains unconfirmed and no single open issue matches the recorded exact title and original body digest; inspect the repository rather than creating a duplicate"
				return receipt, nil
			}
			number = matched
			receipt.Number = number
		} else {
			if presentation.ObserveOnly {
				receipt.Status = recoveryAmbiguous
				receipt.Detail = "no recorded " + presentation.Kind + " issue attachment is observable and observation only makes no write"
				return receipt, nil
			}
			if presentation.Title == "" || presentation.Body == nil {
				receipt.Status = recoveryPending
				return receipt, workflow.Refuse("creating the " + presentation.Kind + " issue requires the recorded title and current public body")
			}
			if err := guard(); err != nil {
				receipt.Status = recoveryPending
				return receipt, err
			}
			created, err := b.CreateIssue(ctx, presentation.Title, *presentation.Body)
			if err != nil {
				// The request may have taken effect without a reliable response,
				// so observe the exact recorded identity before any retry. A
				// definite rejection simply has no attributable match.
				observed, observeErr := b.observeExactIssue(ctx, presentation.Title, recoveryBodyDigest(*presentation.Body))
				if observeErr != nil {
					receipt.Status = recoveryPending
					return receipt, observeErr
				}
				if observed == 0 {
					receipt.Status = recoveryAmbiguous
					receipt.Detail = fmt.Sprintf("issue creation did not return a reliable response (%v) and no single open issue matches the exact title and body digest; no duplicate was created and the outcome remains unresolved", err)
					return receipt, nil
				}
				created = observed
			}
			if created <= 0 {
				receipt.Status = recoveryPending
				return receipt, errors.New("the forge returned no usable issue identity")
			}
			number = created
			receipt.Number = number
			wrote = true
		}
	}

	if err := guard(); err != nil {
		receipt.Status = recoveryPending
		return receipt, err
	}
	issue, err := b.issueRecord(ctx, number)
	if err != nil {
		receipt.Status = recoveryPending
		return receipt, err
	}
	if issue.Number != number || len(issue.PullRequest) != 0 {
		receipt.Status = recoveryPending
		return receipt, workflow.Refuse(fmt.Sprintf("recorded attachment #%d is not the descriptive issue this presentation owns; inspect the attachment instead of reassigning it", number))
	}
	if presentation.Title != "" && issue.Title != presentation.Title {
		receipt.Status = recoveryPending
		return receipt, workflow.Refuse(fmt.Sprintf("recorded attachment #%d presents title %q instead of %q; inspect the attachment instead of rewriting its identity", number, issue.Title, presentation.Title))
	}

	var problems []string
	if presentation.Body != nil && issue.Body != *presentation.Body {
		switch {
		case issue.State == "closed":
			problems = append(problems, fmt.Sprintf("attachment #%d is closed; its public body was not rewritten", number))
		case presentation.ObserveOnly:
			problems = append(problems, "the public body is not yet the supplied presentation")
		default:
			if err := guard(); err != nil {
				receipt.Status = recoveryPending
				return receipt, err
			}
			writeErr := b.request(ctx, http.MethodPatch, b.repositoryPath(b.repository)+fmt.Sprintf("/issues/%d", number), map[string]string{"body": *presentation.Body}, nil)
			observed, readErr := b.issueRecord(ctx, number)
			switch {
			case readErr != nil:
				problems = append(problems, fmt.Sprintf("the body update of issue #%d could not be confirmed: %v", number, readErr))
			case observed.Body != *presentation.Body:
				if writeErr != nil {
					problems = append(problems, writeErr.Error())
				} else {
					problems = append(problems, fmt.Sprintf("issue #%d did not present the supplied public body", number))
				}
			default:
				wrote = true
			}
		}
	}

	switch presentation.Kind {
	case "issue":
		if presentation.Parent != nil && presentation.Parent.Number > 0 && presentation.Parent.Number != number {
			grouped, groupingWrote, err := b.ensureIssueGrouped(ctx, presentation.Parent.Number, number, presentation.ObserveOnly, guard)
			if err != nil {
				problems = append(problems, fmt.Sprintf("issue #%d grouping under parent #%d remains unresolved: %v", number, presentation.Parent.Number, err))
			} else if !grouped {
				problems = append(problems, fmt.Sprintf("issue #%d is not yet grouped under parent #%d", number, presentation.Parent.Number))
			} else {
				wrote = wrote || groupingWrote
			}
		}
	case "parent":
		for _, child := range presentation.Children {
			if child.Number <= 0 || child.Number == number {
				continue
			}
			grouped, groupingWrote, err := b.ensureIssueGrouped(ctx, number, child.Number, presentation.ObserveOnly, guard)
			if err != nil {
				problems = append(problems, fmt.Sprintf("child #%d grouping under parent #%d remains unresolved: %v", child.Number, number, err))
			} else if !grouped {
				problems = append(problems, fmt.Sprintf("child #%d is not yet grouped under parent #%d", child.Number, number))
			} else {
				wrote = wrote || groupingWrote
			}
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

// observeRecordedIssue attributes an uncertain create only from the exact
// title and original body digest recorded before the attempt. Missing
// identity evidence stays unresolved rather than guessed.
func (b *GitHubBackend) observeRecordedIssue(ctx context.Context, presentation ledger.RecoveryPresentation, guard func() error) (int, error) {
	if presentation.Title == "" || presentation.OriginalBodySHA256 == "" {
		return 0, nil
	}
	if err := guard(); err != nil {
		return 0, err
	}
	return b.observeExactIssue(ctx, presentation.Title, presentation.OriginalBodySHA256)
}

// observeExactIssue finds open descriptive issues whose title and exact body
// digest match the recorded identity. Exactly one match is attributable;
// none or several stay unresolved instead of creating a duplicate.
func (b *GitHubBackend) observeExactIssue(ctx context.Context, title, digest string) (int, error) {
	if title == "" || digest == "" {
		return 0, nil
	}
	found, err := b.ListOpenIssues(ctx)
	if err != nil {
		return 0, fmt.Errorf("the issue resolution listing failed: %v; no duplicate was created and the attempt remains unresolved", err)
	}
	wanted := strings.ToLower(digest)
	var matches []int
	for _, issue := range found {
		if issue.Title == title && recoveryBodyDigest(issue.Body) == wanted {
			matches = append(matches, issue.Number)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return 0, nil
	default:
		return 0, workflow.Refuse(fmt.Sprintf("multiple open issues match the exact recorded title and body digest: %v; inspect and resolve the duplicate publication rather than creating another", matches))
	}
}

// ensureIssueGrouped reads the current parent membership and performs only a
// missing grouping write, confirming the relationship by readback so a lost
// attach response cannot become a duplicate. It reports whether the child is
// grouped and whether this attempt wrote the relationship.
func (b *GitHubBackend) ensureIssueGrouped(ctx context.Context, parent, child int, observeOnly bool, guard func() error) (bool, bool, error) {
	if err := guard(); err != nil {
		return false, false, err
	}
	linked, err := b.listRecoveryChildren(ctx, parent)
	if err != nil {
		return false, false, fmt.Errorf("the parent grouping could not be read: %v", err)
	}
	if slices.Contains(linked, child) {
		return true, false, nil
	}
	if observeOnly {
		return false, false, nil
	}
	if err := guard(); err != nil {
		return false, false, err
	}
	if err := b.AttachChild(ctx, parent, child); err != nil {
		if guardErr := guard(); guardErr != nil {
			return false, false, guardErr
		}
		confirmed, readErr := b.listRecoveryChildren(ctx, parent)
		if readErr == nil && slices.Contains(confirmed, child) {
			return true, true, nil
		}
		return false, false, err
	}
	if err := guard(); err != nil {
		return false, false, err
	}
	confirmed, err := b.listRecoveryChildren(ctx, parent)
	if err != nil {
		return false, false, fmt.Errorf("the parent grouping was not confirmed: %v", err)
	}
	if !slices.Contains(confirmed, child) {
		return false, false, fmt.Errorf("child #%d was not observed under parent #%d after the grouping write", child, parent)
	}
	return true, true, nil
}

// listRecoveryChildren reads every page of a parent's grouped sub-issues so
// an incomplete listing can never pass as an absent relationship and become a
// duplicate grouping write.
func (b *GitHubBackend) listRecoveryChildren(ctx context.Context, parent int) ([]int, error) {
	var numbers []int
	for page := 1; ; page++ {
		var children []struct {
			Number int `json:"number"`
		}
		path := b.repositoryPath(b.repository) + fmt.Sprintf("/issues/%d/sub_issues?per_page=100&page=%d", parent, page)
		if err := b.request(ctx, http.MethodGet, path, nil, &children); err != nil {
			return nil, err
		}
		for _, child := range children {
			numbers = append(numbers, child.Number)
		}
		if len(children) < 100 {
			return numbers, nil
		}
	}
}

// listRecoveryPulls reads every recorded pull request of the exact
// owner:branch against main, including closed and merged ones, page by page.
// A closed object on the recorded branch is an attachment whose requested
// active presentation no longer applies, never permission to create a
// replacement.
func (b *GitHubBackend) listRecoveryPulls(ctx context.Context, repository github.RepositoryID, branch string) ([]githubPull, error) {
	var all []githubPull
	for page := 1; ; page++ {
		path := b.repositoryPath(repository) + "/pulls?state=all&base=main&head=" + url.QueryEscape(repository.Owner+":"+branch) + fmt.Sprintf("&per_page=100&page=%d", page)
		var batch []githubPull
		if err := b.request(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return nil, err
		}
		for _, pull := range batch {
			if pull.Head.Ref != branch || !strings.EqualFold(pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name) || pull.Base.Ref != "main" {
				continue
			}
			all = append(all, pull)
		}
		if len(batch) < 100 {
			return all, nil
		}
	}
}

// recoveryPullMismatch describes the first observed identity deviation from
// the recorded repository, branch, main base, and open state. The exact head
// is deliberately not part of this check: an expected source-publication lag
// is caught up through the normal source path before any public body or
// approval is applied.
func recoveryPullMismatch(pull githubPull, repository github.RepositoryID, presentation ledger.RecoveryPresentation) string {
	switch {
	case pull.Head.Ref != presentation.Branch:
		return fmt.Sprintf("pull request #%d tracks branch %q, not the recorded %q", pull.Number, pull.Head.Ref, presentation.Branch)
	case !strings.EqualFold(pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name):
		return fmt.Sprintf("pull request #%d has source repository %q, not %q", pull.Number, pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name)
	case pull.Base.Ref != "main":
		return fmt.Sprintf("pull request #%d targets base %q, not %q", pull.Number, pull.Base.Ref, "main")
	case pull.Merged || pull.MergedAt != "":
		return fmt.Sprintf("pull request #%d is merged; the requested active presentation no longer applies", pull.Number)
	case pull.State != "open":
		return fmt.Sprintf("pull request #%d is %s, not open", pull.Number, pull.State)
	}
	return ""
}

// locateRecoveryPull resolves the single pull request the presentation
// belongs to, accepting an expected source lag but refusing a different
// repository, branch, base, closure, or an ambiguous branch occupant.
func (b *GitHubBackend) locateRecoveryPull(ctx context.Context, presentation ledger.RecoveryPresentation, guard func() error, receipt *ledger.RecoveryReceipt) (*githubPull, error) {
	repository := b.repository
	if presentation.Number > 0 {
		if err := guard(); err != nil {
			return nil, err
		}
		observed, err := b.pullForPresentation(ctx, repository, presentation.Number)
		if err != nil {
			return nil, err
		}
		receipt.Number = observed.Number
		if reason := recoveryPullMismatch(*observed, repository, presentation); reason != "" {
			return nil, workflow.Refuse(reason + "; inspect the recorded attachment instead of reassigning or replacing it")
		}
		return observed, nil
	}
	if err := guard(); err != nil {
		return nil, err
	}
	candidates, err := b.listRecoveryPulls(ctx, repository, presentation.Branch)
	if err != nil {
		return nil, err
	}
	switch len(candidates) {
	case 0:
		return nil, nil
	case 1:
		receipt.Number = candidates[0].Number
		if reason := recoveryPullMismatch(candidates[0], repository, presentation); reason != "" {
			return nil, workflow.Refuse(reason + "; inspect the recorded attachment instead of creating a replacement")
		}
		return &candidates[0], nil
	default:
		numbers := make([]string, 0, len(candidates))
		for _, pull := range candidates {
			numbers = append(numbers, fmt.Sprintf("#%d", pull.Number))
		}
		return nil, workflow.Refuse("multiple pull requests exist for " + presentation.Branch + ": " + strings.Join(numbers, ", ") + "; inspect and resolve the duplicates before presenting")
	}
}
