package ledger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// unknownOutcomeFailure is implemented by forge transport failures whose
// outcome is unknown: the request was sent but no reliable response
// arrived, so the forge may or may not have processed it.
type unknownOutcomeFailure interface{ UnknownOutcome() bool }

func isUnknownOutcome(err error) bool {
	var unknown unknownOutcomeFailure
	return errors.As(err, &unknown) && unknown.UnknownOutcome()
}

// Push statuses. An unavailable remote is pending; unexpected competing
// upstream history is an explicit reconciliation requirement, never an
// outage and never something skl resolves by merging, rebasing, or
// force-pushing.
const (
	PushPushed         = "pushed"
	PushPending        = "pending"
	PushReconciliation = "reconciliation_required"
	PushNoUpstream     = "no_upstream"
	IssueAttached      = "attached"
	IssuePending       = "pending"
	IssueUnresolved    = "unresolved"
)

// Publication is the aggregate outcome of one invocation's publication
// attempts.
type Publication struct {
	Push PublicationNote
}

// Publish attempts the initial publication surfaces after local acceptance:
// the ledger push through the clone's own remote configuration and the
// descriptive forge issues carrying the supplied temporary bodies. Every
// failure stays visibly pending; successful attachments are recorded, and
// no public body is persisted in the ledger.
func Publish(ctx context.Context, store *Store, project string, declaration *ProposalDeclaration, outcome *Acceptance, forge Forge) (*Publication, error) {
	publication := &Publication{}
	if forge == nil {
		for index := range outcome.Slices {
			outcome.Slices[index].IssueStatus = &PublicationNote{
				Status: IssuePending,
				Detail: "issue publication inputs are unavailable: no forge attachment surface",
			}
		}
	} else {
		publishIssues(ctx, declaration, outcome, forge)
	}
	// The reported push outcome is the replication of the accepted records
	// themselves; a best-effort second push below only carries publication
	// bookkeeping and never downgrades that fact.
	note, _ := store.push()
	publication.Push = note
	for index := range outcome.Slices {
		outcome.Slices[index].PushStatus = &note
	}
	pushed := note.Status == PushPushed
	if err := store.recordPublication(project, declaration, outcome, pushed); err != nil {
		return nil, err
	}
	if pushed {
		if retry, ok := store.push(); !ok {
			detail := &PublicationNote{
				Status: PushPushed,
				Detail: "the accepted records were pushed, but pushing the publication bookkeeping stayed " + retry.Status + ": " + retry.Detail,
			}
			publication.Push = *detail
			for index := range outcome.Slices {
				outcome.Slices[index].PushStatus = detail
			}
		}
	}
	return publication, nil
}

// publishIssues creates, adopts, or leaves pending each slice's descriptive
// issue and the multi-slice parent grouping. Already-attached issues are
// never recreated.
func publishIssues(ctx context.Context, declaration *ProposalDeclaration, outcome *Acceptance, forge Forge) {
	repository := outcome.Repository
	attachedChildren := 0
	for index := range declaration.Slices {
		slice := &declaration.Slices[index]
		state := outcome.slice(slice.Name)
		if state == nil {
			continue
		}
		if state.Issue != nil {
			attachedChildren++
			continue
		}
		body, supplied := declaration.IssueBodies[slice.Name]
		if !supplied {
			state.IssueStatus = &PublicationNote{Status: IssuePending, Detail: "no descriptive issue body was supplied"}
			continue
		}
		// A previously unresolved publication is resolved safely before any
		// new creation, so a duplicate is never created blindly.
		previouslyUnresolved := state.IssueStatus != nil && state.IssueStatus.Status == IssueUnresolved
		number, note := createOrAdoptIssue(ctx, forge, slice.Title, string(body), previouslyUnresolved)
		if number != 0 {
			state.Issue = &ForgeAttachment{Repository: repository, Number: number}
			attachedChildren++
			continue
		}
		state.IssueStatus = &note
	}
	if len(declaration.Slices) < 2 {
		return
	}
	// The parent groups its children; publish it once at least one child is
	// attached, then link every currently attached child that is not linked
	// yet.
	if outcome.ParentIssue == nil {
		if attachedChildren == 0 {
			outcome.ParentNote = &PublicationNote{Status: IssuePending, Detail: "no child issue is attached yet"}
			return
		}
		if declaration.ParentBody == nil {
			outcome.ParentNote = &PublicationNote{Status: IssuePending, Detail: "no parent issue body was supplied"}
			return
		}
		number, note := createOrAdoptIssue(ctx, forge, strings.TrimSpace(declaration.ParentTitle), string(declaration.ParentBody), false)
		if number == 0 {
			outcome.ParentNote = &note
			return
		}
		outcome.ParentIssue = &ForgeAttachment{Repository: repository, Number: number}
	}
	for index := range declaration.Slices {
		child := outcome.slice(declaration.Slices[index].Name)
		if child == nil || child.Issue == nil {
			continue
		}
		linked, err := forge.ListChildren(ctx, outcome.ParentIssue.Number)
		if err == nil && containsInt(linked, child.Issue.Number) {
			continue
		}
		if err := forge.AttachChild(ctx, outcome.ParentIssue.Number, child.Issue.Number); err != nil {
			child.IssueStatus = &PublicationNote{Status: IssuePending, Detail: "grouping under the parent issue failed: " + err.Error()}
		}
	}
}

func containsInt(values []int, wanted int) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// createOrAdoptIssue publishes one descriptive issue. An uncertain creation
// attempt is resolved safely by exact title-and-body match before any
// duplicate could be created; an unresolved outcome is reported for repair
// rather than guessed.
func createOrAdoptIssue(ctx context.Context, forge Forge, title, body string, resolveFirst bool) (int, PublicationNote) {
	if resolveFirst {
		if number, note, decided := adoptMatchingIssue(ctx, forge, title, body); decided {
			return number, note
		}
	}
	number, err := forge.CreateIssue(ctx, title, body)
	if err == nil {
		return number, PublicationNote{}
	}
	if !isUnknownOutcome(err) {
		return 0, PublicationNote{Status: IssuePending, Detail: err.Error()}
	}
	// The request may have been processed without a reliable response.
	// Resolve safely: adopt exactly one open issue with the same title and
	// the exact supplied body, never a label, a comment, or a near match.
	if number, note, decided := adoptMatchingIssue(ctx, forge, title, body); decided {
		return number, note
	}
	return 0, PublicationNote{Status: IssueUnresolved, Detail: "issue creation failed uncertainly (" + err.Error() + ") and no unambiguous matching open issue is observable; repeat the acceptance to retry with a checked listing"}
}

// adoptMatchingIssue looks for one open issue with the exact title and body.
// decided is false when no match exists and creating one remains safe.
func adoptMatchingIssue(ctx context.Context, forge Forge, title, body string) (int, PublicationNote, bool) {
	found, err := forge.ListOpenIssues(ctx)
	if err != nil {
		return 0, PublicationNote{Status: IssueUnresolved, Detail: "the resolution listing failed: " + err.Error()}, true
	}
	var matches []int
	for _, issue := range found {
		if issue.Title == title && issue.Body == body {
			matches = append(matches, issue.Number)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], PublicationNote{}, true
	case 0:
		return 0, PublicationNote{}, false
	default:
		return 0, PublicationNote{
			Status: IssueUnresolved,
			Detail: fmt.Sprintf("multiple open issues match the exact title and body: %v; resolve the grouping with human direction", matches),
		}, true
	}
}

// push attempts replication through the clone's own remote/upstream
// configuration and classifies the outcome. It never merges, rebases, or
// force-pushes, and never discards local or remote history.
func (s *Store) push() (PublicationNote, bool) {
	remote, branch, err := s.upstream()
	if err != nil {
		return PublicationNote{Status: PushPending, Detail: err.Error()}, false
	}
	if remote == "" {
		return PublicationNote{Status: PushNoUpstream, Detail: "the ledger clone has no configured remote upstream"}, false
	}
	if _, err := git(s.Root, "fetch", "--quiet", remote); err != nil {
		return PublicationNote{Status: PushPending, Detail: "upstream is unavailable: " + gitError(s.Root, []string{"fetch", remote}, err).Error()}, false
	}
	diverged, err := s.diverged(remote, branch)
	if err != nil {
		return PublicationNote{Status: PushPending, Detail: "replication state is unobservable: " + err.Error()}, false
	}
	if diverged {
		return PublicationNote{
			Status: PushReconciliation,
			Detail: "upstream " + remote + "/" + branch + " holds competing ledger history; skl merges, rebases, and force-pushes nothing",
		}, false
	}
	if _, err := git(s.Root, "push", remote, "refs/heads/"+branch+":refs/heads/"+branch); err != nil {
		// A pre-checked push can still race another writer; re-observe
		// before classifying so competing history is never misreported as
		// an ordinary outage.
		if diverged, divErr := s.diverged(remote, branch); divErr == nil && diverged {
			return PublicationNote{
				Status: PushReconciliation,
				Detail: "upstream " + remote + "/" + branch + " accepted competing ledger history during replication; skl merges, rebases, and force-pushes nothing",
			}, false
		}
		return PublicationNote{Status: PushPending, Detail: "push failed: " + gitError(s.Root, []string{"push", remote, branch}, err).Error()}, false
	}
	return PublicationNote{Status: PushPushed}, true
}

// upstream resolves the clone's configured upstream branch: the tracked
// upstream when set, otherwise the sole configured remote.
func (s *Store) upstream() (remote, branch string, err error) {
	tracked, trackErr := git(s.Root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if trackErr == nil && strings.Contains(tracked, "/") {
		remote, branch, _ = strings.Cut(tracked, "/")
		return remote, branch, nil
	}
	names, err := git(s.Root, "remote")
	if err != nil {
		return "", "", errors.New("the ledger clone's remotes are unreadable")
	}
	remotes := strings.Fields(names)
	if len(remotes) == 1 {
		branch, err = git(s.Root, "branch", "--show-current")
		if err != nil || branch == "" {
			return "", "", errors.New("the ledger clone's current branch is unreadable")
		}
		return remotes[0], branch, nil
	}
	return "", "", nil
}

// diverged reports whether the upstream branch holds history the local
// head does not contain. A push is safe only when the observed upstream is
// an ancestor of the local head; anything else is competing history that
// requires explicit reconciliation. A missing upstream branch is a plain
// first push, not divergence.
func (s *Store) diverged(remote, branch string) (bool, error) {
	upstream := "refs/remotes/" + remote + "/" + branch
	if !gitOK(s.Root, "show-ref", "--verify", "--quiet", upstream) {
		return false, nil
	}
	if gitOK(s.Root, "merge-base", "--is-ancestor", upstream, "HEAD") {
		return false, nil
	}
	return true, nil
}

// recordPublication commits the publication bookkeeping: attachments and
// pending facts in state.json and proposal.json. Contract bytes are never
// rewritten, and nothing is committed when no fact changed.
func (s *Store) recordPublication(project string, declaration *ProposalDeclaration, outcome *Acceptance, pushed bool) error {
	meta, _, err := s.readProposalMeta(project, declaration.Proposal)
	if err != nil {
		return err
	}
	parentChanged := !sameAttachment(meta.ParentIssue, outcome.ParentIssue)
	meta.ParentIssue = outcome.ParentIssue
	changed := parentChanged
	for index := range declaration.Slices {
		slice := &declaration.Slices[index]
		acceptance := outcome.slice(slice.Name)
		if acceptance == nil {
			return fmt.Errorf("acceptance outcome lacks slice %s", slice.Name)
		}
		state, found, err := s.readSliceState(project, declaration.Proposal, slice.Name)
		if err != nil || !found {
			if err == nil {
				return fmt.Errorf("record of slice %s is missing its state.json", slice.Name)
			}
			return err
		}
		before, previousIssue := state.Publication, state.Issue
		publication := PublicationState{}
		if before != nil {
			publication = *before
		}
		if acceptance.Issue != nil {
			state.Issue = acceptance.Issue
			publication.Issue = nil
		} else if acceptance.IssueStatus != nil && acceptance.IssueStatus.Status != "" {
			publication.Issue = acceptance.IssueStatus
		}
		if pushed {
			publication.Push = nil
		} else if acceptance.PushStatus != nil && acceptance.PushStatus.Status != "" {
			publication.Push = acceptance.PushStatus
		}
		if publication.Push == nil && publication.Issue == nil {
			state.Publication = nil
		} else {
			state.Publication = &publication
		}
		if !samePublication(before, state.Publication) || !sameAttachment(previousIssue, state.Issue) {
			changed = true
			if err := s.writeSliceState(project, declaration.Proposal, slice.Name, state); err != nil {
				return err
			}
		}
	}
	if !changed {
		return nil
	}
	if err := s.writeProposalMeta(project, declaration.Proposal, meta); err != nil {
		return err
	}
	return s.commit("record publication "+project+"/"+declaration.Proposal, filepath.Join(projectsRoot, project))
}

func samePublication(before, after *PublicationState) bool {
	if before == nil || after == nil {
		return before == after
	}
	return sameNote(before.Push, after.Push) && sameNote(before.Issue, after.Issue)
}

func sameNote(before, after *PublicationNote) bool {
	if before == nil || after == nil {
		return before == after
	}
	return *before == *after
}

func sameAttachment(before, after *ForgeAttachment) bool {
	if before == nil || after == nil {
		return before == after
	}
	return *before == *after
}
