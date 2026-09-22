package setup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/ledger"
	"github.com/vicrdguez/skills/workflow"
)

var _ ledger.DeliveryForge = (*GitHubBackend)(nil)

// PresentPull presents one explicitly public delivery body on the bound
// repository's pull request for the exact planned source branch and head.
// The adapter alone translates Approved into GitHub's native draft/non-draft
// presentation; it carries no workflow authority, keeps the private report
// out of public material, and never merges, closes, labels, comments, or
// approves a revision other than the exact pushed source head.
func (b *GitHubBackend) PresentPull(ctx context.Context, presentation ledger.PullPresentation) (int, error) {
	if err := b.requireRepository(); err != nil {
		return 0, err
	}
	if presentation.Branch == "" || presentation.Head == "" {
		return 0, workflow.Refuse("delivery presentation requires an exact source branch and head; prepare and push the planned source before presenting it")
	}
	repository := b.repository
	pull, err := b.locatePresentedPull(ctx, repository, presentation)
	if err != nil {
		return 0, err
	}
	if pull == nil {
		if presentation.Title == "" {
			return 0, workflow.Refuse("creating a delivery pull request requires the recorded title; repair the selected record before presenting it")
		}
		created, err := b.createPresentedPull(ctx, repository, presentation)
		if err != nil {
			return 0, err
		}
		pull = created
	}
	if reason := presentedPullMismatch(*pull, repository, presentation); reason != "" {
		return 0, workflow.Refuse(reason + "; new presentation remains draft until the exact reviewed source is available")
	}
	attempt, err := b.applyPresentedPull(ctx, repository, presentation, *pull)
	if err != nil {
		return 0, b.correctPresentedReadiness(ctx, repository, attempt, err)
	}
	return b.refreshPresentedPull(ctx, repository, presentation, attempt)
}

// locatePresentedPull resolves the single pull request the presentation
// belongs to. An attached number is read and validated as-is; otherwise the
// open pull requests for the exact owner:branch and main base are listed to
// completion. Anything already occupying the branch that is not the exact
// expected source is a concrete refusal, never something to repair by guess.
func (b *GitHubBackend) locatePresentedPull(ctx context.Context, repository github.RepositoryID, presentation ledger.PullPresentation) (*githubPull, error) {
	if presentation.Number > 0 {
		pull, err := b.pullForPresentation(ctx, repository, presentation.Number)
		if err != nil {
			return nil, err
		}
		if reason := presentedPullMismatch(*pull, repository, presentation); reason != "" {
			return nil, workflow.Refuse(reason + "; present the recorded attachment for the exact pushed source head instead of rewriting its identity")
		}
		return pull, nil
	}
	pulls, err := b.listPresentedPulls(ctx, repository, presentation)
	if err != nil {
		return nil, err
	}
	var matches []githubPull
	for _, pull := range pulls {
		if presentedPullMismatch(pull, repository, presentation) == "" {
			matches = append(matches, pull)
		}
	}
	switch {
	case len(matches) == 1:
		return &matches[0], nil
	case len(matches) > 1:
		numbers := make([]string, 0, len(matches))
		for _, pull := range matches {
			numbers = append(numbers, fmt.Sprintf("#%d", pull.Number))
		}
		return nil, workflow.Refuse("multiple open pull requests match " + presentation.Branch + " at " + presentation.Head + ": " + strings.Join(numbers, ", ") + "; preserve the intended attachment and resolve the duplicates before presenting")
	case len(pulls) > 0:
		return nil, workflow.Refuse(presentedPullMismatch(pulls[0], repository, presentation) + "; present the exact pushed source head instead of guessing a repair")
	}
	return nil, nil
}

// createPresentedPull creates the one pull request for the exact source. A
// creation whose response is missing or unusable is resolved only by an exact
// checked listing of the same branch, repository, base, and head; an
// unobservable outcome stays a concrete pending error and never creates twice.
func (b *GitHubBackend) createPresentedPull(ctx context.Context, repository github.RepositoryID, presentation ledger.PullPresentation) (*githubPull, error) {
	payload := map[string]any{
		"title": presentation.Title,
		"head":  presentation.Branch,
		"base":  "main",
		"body":  presentation.Body,
		// Creation cannot atomically pin GitHub's current branch SHA. Start as
		// draft, validate the returned source, then apply any approval.
		"draft": true,
	}
	var created githubPull
	writeErr := b.request(ctx, http.MethodPost, b.repositoryPath(repository)+"/pulls", payload, &created)
	if writeErr == nil && created.Number > 0 {
		return &created, nil
	}
	observed, err := b.listPresentedPulls(ctx, repository, presentation)
	if err != nil {
		return nil, fmt.Errorf("pull request creation for %s at %s was not confirmed and the exact branch listing was unavailable: %v; the creation outcome remains unknown and no duplicate was created", presentation.Branch, presentation.Head, err)
	}
	var matches []githubPull
	for _, pull := range observed {
		if presentedPullMismatch(pull, repository, presentation) == "" {
			matches = append(matches, pull)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		if writeErr == nil {
			writeErr = errors.New("pull request creation returned no reliable identity")
		}
		return nil, fmt.Errorf("pull request creation for %s at %s is unresolved: %v; no exact matching open pull request is observable and no duplicate was created", presentation.Branch, presentation.Head, writeErr)
	default:
		return nil, workflow.Refuse("multiple open pull requests match the attempted creation for " + presentation.Branch + " at " + presentation.Head + "; preserve them and resolve the duplicates before presenting")
	}
}

// presentationAttempt names the one readiness direction a presentation
// dispatched for a pull request. It lets a later proof that the source was not
// the reviewed one correct exactly the non-draft readiness this attempt may
// have established, never readiness another writer owns.
type presentationAttempt struct {
	number int
	nodeID string
	ready  bool
}

// applyPresentedPull writes only the supplied public body and the native
// readiness implied by Approved. The title of an existing pull request is
// never rewritten. An approval re-reads the exact reviewed source immediately
// before writing, so a branch that already moved receives neither the public
// body nor the ready presentation.
func (b *GitHubBackend) applyPresentedPull(ctx context.Context, repository github.RepositoryID, presentation ledger.PullPresentation, pull githubPull) (presentationAttempt, error) {
	attempt := presentationAttempt{number: pull.Number}
	if presentation.Approved {
		fresh, err := b.pullForPresentation(ctx, repository, pull.Number)
		if err != nil {
			return attempt, err
		}
		if reason := presentedPullMismatch(*fresh, repository, presentation); reason != "" {
			return attempt, workflow.Refuse(reason + "; no public body or readiness was applied to unreviewed code")
		}
		pull = *fresh
	}
	if pull.Body != presentation.Body {
		if err := b.request(ctx, http.MethodPatch, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d", pull.Number), map[string]string{"body": presentation.Body}, nil); err != nil {
			return attempt, err
		}
	}
	draft := !presentation.Approved
	if pull.Draft != draft {
		if pull.NodeID == "" {
			return attempt, workflow.Refuse(fmt.Sprintf("pull request #%d has no stable node identity for the required %s presentation", pull.Number, presentationReadiness(draft)))
		}
		attempt.nodeID = pull.NodeID
		attempt.ready = !draft
		if err := b.setPullPresentation(ctx, pull.NodeID, draft); err != nil {
			return attempt, err
		}
	}
	return attempt, nil
}

// correctPresentedReadiness restores draft readiness only for the non-draft
// mutation this attempt dispatched. It re-reads the attachment before mutating
// so readiness this attempt did not establish is left untouched, and it reports
// a correction it could not confirm as explicitly unresolved rather than
// treating a still-ready presentation of unreviewed code as harmless.
func (b *GitHubBackend) correctPresentedReadiness(ctx context.Context, repository github.RepositoryID, attempt presentationAttempt, cause error) error {
	if !attempt.ready {
		return cause
	}
	pull, err := b.pullForPresentation(ctx, repository, attempt.number)
	if err != nil {
		return fmt.Errorf("%v; the ready presentation this attempt established could not be inspected (%v) and its correction remains unresolved: pull request #%d may still present unreviewed code as ready", cause, err, attempt.number)
	}
	if pull.Draft {
		return cause
	}
	nodeID := pull.NodeID
	if nodeID == "" {
		nodeID = attempt.nodeID
	}
	if nodeID == "" {
		return fmt.Errorf("%v; the ready presentation this attempt established has no stable node identity for correction and remains unresolved: pull request #%d may still present unreviewed code as ready", cause, attempt.number)
	}
	if err := b.setPullPresentation(ctx, nodeID, true); err != nil {
		return fmt.Errorf("%v; restoring draft readiness failed (%v) and remains unresolved: pull request #%d may still present unreviewed code as ready", cause, err, attempt.number)
	}
	return fmt.Errorf("%v; the ready presentation this attempt established was restored to draft", cause)
}

// setPullPresentation performs the one native readiness mutation the adapter
// owns. Approved is never assumed: the mutation is named from its exact
// direction.
func (b *GitHubBackend) setPullPresentation(ctx context.Context, nodeID string, draft bool) error {
	mutation := "markPullRequestReadyForReview"
	if draft {
		mutation = "convertPullRequestToDraft"
	}
	var response struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	err := b.request(ctx, http.MethodPost, "/graphql", map[string]any{"query": "mutation($id:ID!){" + mutation + "(input:{pullRequestId:$id}){pullRequest{id}}}", "variables": map[string]string{"id": nodeID}}, &response)
	if err != nil {
		return err
	}
	if len(response.Errors) > 0 {
		return errors.New(response.Errors[0].Message)
	}
	return nil
}

// refreshPresentedPull re-reads the final attachment and refuses unless the
// observed source identity, public body, and readiness are exactly the ones
// this presentation established. A source that moved after the readiness
// mutation cannot remain presented as approved: the attempt restores the
// non-draft readiness it established and reports the mismatch explicitly.
func (b *GitHubBackend) refreshPresentedPull(ctx context.Context, repository github.RepositoryID, presentation ledger.PullPresentation, attempt presentationAttempt) (int, error) {
	pull, err := b.pullForPresentation(ctx, repository, attempt.number)
	if err != nil {
		return 0, b.correctPresentedReadiness(ctx, repository, attempt, fmt.Errorf("the presented pull request could not be re-read: %v", err))
	}
	if reason := presentedPullMismatch(*pull, repository, presentation); reason != "" {
		return 0, b.correctPresentedReadiness(ctx, repository, attempt, workflow.Refuse(reason+"; inspect the pull request before retrying the same handoff"))
	}
	if pull.Body != presentation.Body {
		return 0, workflow.Refuse(fmt.Sprintf("pull request #%d does not present the supplied public body; inspect the current content before retrying", attempt.number))
	}
	if pull.Draft != !presentation.Approved {
		return 0, b.correctPresentedReadiness(ctx, repository, attempt, workflow.Refuse(fmt.Sprintf("pull request #%d readiness was not observed as %s; inspect it before retrying", attempt.number, presentationReadiness(!presentation.Approved))))
	}
	return pull.Number, nil
}

// pullForPresentation reads one pull request and refuses a response that does
// not carry the requested stable number.
func (b *GitHubBackend) pullForPresentation(ctx context.Context, repository github.RepositoryID, number int) (*githubPull, error) {
	var pull githubPull
	if err := b.request(ctx, http.MethodGet, b.repositoryPath(repository)+fmt.Sprintf("/pulls/%d", number), nil, &pull); err != nil {
		return nil, err
	}
	if pull.Number != number {
		return nil, fmt.Errorf("GitHub returned pull request #%d for the requested #%d", pull.Number, number)
	}
	return &pull, nil
}

// listPresentedPulls reads every open pull request of the exact owner:branch
// against main, page by page, so an incomplete listing can never pass as an
// absent attachment.
func (b *GitHubBackend) listPresentedPulls(ctx context.Context, repository github.RepositoryID, presentation ledger.PullPresentation) ([]githubPull, error) {
	var all []githubPull
	for page := 1; ; page++ {
		path := b.repositoryPath(repository) + "/pulls?state=open&base=main&head=" + url.QueryEscape(repository.Owner+":"+presentation.Branch) + fmt.Sprintf("&per_page=100&page=%d", page)
		var batch []githubPull
		if err := b.request(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			return all, nil
		}
	}
}

// presentedPullMismatch describes the first observed identity deviation from
// the exact planned branch, repository, base, head, and open state. An empty
// result means the pull request is the exact attachment the presentation owns.
func presentedPullMismatch(pull githubPull, repository github.RepositoryID, presentation ledger.PullPresentation) string {
	switch {
	case pull.Head.Ref != presentation.Branch:
		return fmt.Sprintf("pull request #%d tracks branch %q, not the planned %q", pull.Number, pull.Head.Ref, presentation.Branch)
	case !strings.EqualFold(pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name):
		return fmt.Sprintf("pull request #%d has source repository %q, not %q", pull.Number, pull.Head.Repo.FullName, repository.Owner+"/"+repository.Name)
	case pull.Base.Ref != "main":
		return fmt.Sprintf("pull request #%d targets base %q, not %q", pull.Number, pull.Base.Ref, "main")
	case pull.Head.SHA != presentation.Head:
		return fmt.Sprintf("pull request #%d presents source head %s, not the expected %s", pull.Number, pull.Head.SHA, presentation.Head)
	case pull.State != "open":
		return fmt.Sprintf("pull request #%d is %s, not open", pull.Number, pull.State)
	}
	return ""
}

func presentationReadiness(draft bool) string {
	if draft {
		return "draft"
	}
	return "ready for review"
}
