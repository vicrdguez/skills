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
	issueReserved      = "reserved"
)

// Publish attempts the initial publication surfaces after local acceptance:
// the ledger push through the clone's own remote configuration and the
// descriptive forge issues carrying the supplied temporary bodies. Every
// failure stays visibly pending; successful attachments are recorded, and
// no public body is persisted in the ledger.
func Publish(ctx context.Context, store *Store, project string, declaration *ProposalDeclaration, outcome *Acceptance, forge Forge, acceptedRevision string) error {
	reservations := publicationReservations{issues: make(map[string]bool)}
	if forge == nil {
		for index := range outcome.Slices {
			outcome.Slices[index].IssueStatus = &PublicationNote{
				Status: IssuePending,
				Detail: "issue publication inputs are unavailable: no forge attachment surface",
			}
		}
	} else {
		var err error
		reservations, acceptedRevision, err = store.reserveIssuePublication(project, declaration, outcome)
		if err != nil {
			outcome.BookkeepingStatus = &PublicationNote{Status: IssuePending, Detail: "issue publication was not reserved safely: " + err.Error()}
		} else {
			outcome.Commit = acceptedRevision
			publishIssues(ctx, declaration, outcome, forge, reservations)
		}
	}
	// The reported push outcome is the replication of the accepted records
	// themselves; a best-effort second push below only carries publication
	// bookkeeping and never downgrades that fact.
	note, _ := store.push(acceptedRevision)
	for index := range outcome.Slices {
		outcome.Slices[index].PushStatus = &note
	}
	pushed := note.Status == PushPushed
	bookkeepingRevision, err := store.recordPublication(project, declaration, outcome, pushed)
	if err != nil {
		outcome.BookkeepingStatus = &PublicationNote{
			Status: IssuePending,
			Detail: "publication bookkeeping was not written: " + err.Error() + "; preserve the accepted commit and forge attachments, repair the ledger clone, then repeat acceptance",
		}
		return nil
	}
	outcome.Commit = bookkeepingRevision
	if pushed {
		if retry, ok := store.push(bookkeepingRevision); !ok {
			for index := range outcome.Slices {
				outcome.Slices[index].PushStatus = &retry
			}
			pendingRevision, err := store.recordPublication(project, declaration, outcome, false)
			if err != nil {
				outcome.BookkeepingStatus = &PublicationNote{
					Status: IssuePending,
					Detail: "publication bookkeeping replication stayed " + retry.Status + ", and that pending fact could not be recorded: " + err.Error(),
				}
			} else {
				outcome.Commit = pendingRevision
			}
			detail := &PublicationNote{
				Status: PushPushed,
				Detail: "the accepted records were pushed, but pushing the publication bookkeeping stayed " + retry.Status + ": " + retry.Detail,
			}
			for index := range outcome.Slices {
				outcome.Slices[index].PushStatus = detail
			}
		}
	}
	return nil
}

// publishIssues creates, adopts, or leaves pending each slice's descriptive
// issue and the multi-slice parent grouping. Already-attached issues are
// never recreated.
type publicationReservations struct {
	issues map[string]bool
	parent bool
}

func publishIssues(ctx context.Context, declaration *ProposalDeclaration, outcome *Acceptance, forge Forge, reservations publicationReservations) {
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
		var number int
		var note PublicationNote
		switch {
		case reservations.issues[slice.Name]:
			// A fresh, durably reserved attempt creates the supplied issue; it
			// never adopts an unrelated pre-existing title/body match.
			number, note = createOrAdoptIssue(ctx, forge, slice.Title, string(body), false)
		case state.IssueStatus != nil && state.IssueStatus.Status == issueReserved:
			number, note = waitForReservedIssue()
		default:
			number, note = createOrAdoptIssue(ctx, forge, slice.Title, string(body), true)
		}
		if number != 0 {
			state.Issue = &ForgeAttachment{Repository: repository, Number: number}
			state.IssueStatus = nil
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
		var number int
		var note PublicationNote
		switch {
		case reservations.parent:
			number, note = createOrAdoptIssue(ctx, forge, strings.TrimSpace(declaration.ParentTitle), string(declaration.ParentBody), false)
		case outcome.ParentNote != nil && outcome.ParentNote.Status == issueReserved:
			number, note = waitForReservedIssue()
		default:
			number, note = createOrAdoptIssue(ctx, forge, strings.TrimSpace(declaration.ParentTitle), string(declaration.ParentBody), true)
		}
		if number == 0 {
			outcome.ParentNote = &note
			return
		}
		outcome.ParentIssue = &ForgeAttachment{Repository: repository, Number: number}
		outcome.ParentNote = nil
	}
	for index := range declaration.Slices {
		child := outcome.slice(declaration.Slices[index].Name)
		if child == nil || child.Issue == nil {
			continue
		}
		linked, err := forge.ListChildren(ctx, outcome.ParentIssue.Number)
		if err == nil && containsInt(linked, child.Issue.Number) {
			child.GroupingStatus = nil
			continue
		}
		if err := forge.AttachChild(ctx, outcome.ParentIssue.Number, child.Issue.Number); err != nil {
			child.GroupingStatus = &PublicationNote{Status: IssuePending, Detail: "grouping under the parent issue failed: " + err.Error()}
			continue
		}
		child.GroupingStatus = nil
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

// waitForReservedIssue leaves another invocation's reservation unresolved.
// An exact title/body match cannot prove that an issue belongs to an in-flight
// creation when unrelated matching work may already exist.
func waitForReservedIssue() (int, PublicationNote) {
	return 0, PublicationNote{
		Status: issueReserved,
		Detail: "another invocation reserved issue creation; repeat after that attempt settles rather than guessing which open issue belongs to it or creating a duplicate",
	}
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

// reserveIssuePublication durably establishes which fresh issue creations
// belong to this invocation. Concurrent invocations only observe/adopt those
// attempts; they never perform the same list-then-create race.
func (s *Store) reserveIssuePublication(project string, declaration *ProposalDeclaration, outcome *Acceptance) (publicationReservations, string, error) {
	reservations := publicationReservations{issues: make(map[string]bool)}
	var revision string
	err := s.withMutation(func() error {
		proposalPath := filepath.Join(projectsRoot, project, "proposals", declaration.Proposal)
		paths := []string{filepath.Join(proposalPath, "proposal.json")}
		for index := range declaration.Slices {
			paths = append(paths, filepath.Join(proposalPath, declaration.Slices[index].Name, "state.json"))
		}
		if err := s.requireCleanPaths(paths...); err != nil {
			return err
		}
		meta, _, err := s.readProposalMeta(project, declaration.Proposal)
		if err != nil {
			return err
		}
		metaChanged := false
		if meta.ParentIssue != nil {
			outcome.ParentIssue = meta.ParentIssue
			outcome.ParentNote = nil
		} else if len(declaration.Slices) > 1 && declaration.ParentBodySupplied() {
			switch {
			case meta.ParentPublication == nil || meta.ParentPublication.Status == IssuePending:
				note := &PublicationNote{Status: issueReserved, Detail: "parent issue creation is reserved before network publication"}
				meta.ParentPublication = note
				outcome.ParentNote = note
				outcome.ownsParentReservation = true
				reservations.parent = true
				metaChanged = true
			default:
				outcome.ParentNote = meta.ParentPublication
			}
		}

		type stateUpdate struct {
			path  string
			state SliceState
		}
		var updates []stateUpdate
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
			if state.Issue != nil {
				acceptance.Issue = state.Issue
				acceptance.IssueStatus = nil
				continue
			}
			publication := PublicationState{}
			if state.Publication != nil {
				publication = *state.Publication
			}
			if publication.Issue != nil {
				acceptance.IssueStatus = publication.Issue
			}
			if _, supplied := declaration.IssueBodies[slice.Name]; !supplied {
				continue
			}
			if publication.Issue != nil && publication.Issue.Status != IssuePending {
				continue
			}
			note := &PublicationNote{Status: issueReserved, Detail: "issue creation is reserved before network publication"}
			publication.Issue = note
			state.Publication = &publication
			acceptance.IssueStatus = note
			acceptance.ownsIssueReservation = true
			reservations.issues[slice.Name] = true
			updates = append(updates, stateUpdate{
				path: filepath.Join(proposalPath, slice.Name, "state.json"), state: state,
			})
		}
		var changedPaths []string
		if metaChanged {
			if err := s.writeProposalMeta(project, declaration.Proposal, meta); err != nil {
				return err
			}
			changedPaths = append(changedPaths, filepath.Join(proposalPath, "proposal.json"))
		}
		for _, update := range updates {
			if err := writeJSON(filepath.Join(s.Root, update.path), update.state); err != nil {
				return err
			}
			changedPaths = append(changedPaths, update.path)
		}
		if len(changedPaths) > 0 {
			if err := s.commit("reserve publication "+project+"/"+declaration.Proposal, changedPaths...); err != nil {
				return err
			}
		}
		var headErr error
		revision, headErr = s.head()
		return headErr
	})
	return reservations, revision, err
}

// push attempts replication through the clone's own remote/upstream
// configuration and classifies the outcome. It never merges, rebases, or
// force-pushes, and never discards local or remote history.
func (s *Store) push(revision string) (PublicationNote, bool) {
	remote, remoteBranch, err := s.upstream()
	if err != nil {
		return PublicationNote{Status: PushPending, Detail: err.Error()}, false
	}
	if remote == "" {
		return PublicationNote{Status: PushNoUpstream, Detail: "the ledger clone has no configured remote upstream"}, false
	}
	if _, err := git(s.Root, "rev-parse", "--verify", revision+"^{commit}"); err != nil {
		return PublicationNote{Status: PushPending, Detail: "the ledger revision to replicate is unavailable: " + revision}, false
	}
	if _, err := git(s.Root, "fetch", "--quiet", remote); err != nil {
		return PublicationNote{Status: PushPending, Detail: "upstream is unavailable: " + gitError(s.Root, []string{"fetch", remote}, err).Error()}, false
	}
	if s.destinationContains(remote, remoteBranch, revision) {
		return PublicationNote{Status: PushPushed}, true
	}
	diverged, err := s.diverged(remote, remoteBranch, revision)
	if err != nil {
		return PublicationNote{Status: PushPending, Detail: "replication state is unobservable: " + err.Error()}, false
	}
	if diverged {
		return PublicationNote{
			Status: PushReconciliation,
			Detail: "upstream " + remote + "/" + remoteBranch + " holds competing ledger history; skl merges, rebases, and force-pushes nothing",
		}, false
	}
	refspec := revision + ":refs/heads/" + remoteBranch
	if _, err := git(s.Root, "push", remote, refspec); err != nil {
		if observed, ok := s.observeReplication(remote, remoteBranch, revision, false); ok {
			return observed, observed.Status == PushPushed
		}
		return PublicationNote{Status: PushPending, Detail: "push failed and the destination could not be refreshed: " + gitError(s.Root, []string{"push", remote, refspec}, err).Error()}, false
	}
	if observed, ok := s.observeReplication(remote, remoteBranch, revision, true); ok {
		return observed, observed.Status == PushPushed
	}
	return PublicationNote{Status: PushPending, Detail: "push returned successfully, but the configured destination could not be observed"}, false
}

// upstream resolves the configured destination branch: the tracked upstream
// when set, otherwise the current branch on the sole configured remote.
func (s *Store) upstream() (remote, remoteBranch string, err error) {
	tracked, trackErr := git(s.Root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if trackErr == nil && strings.Contains(tracked, "/") {
		remote, remoteBranch, _ = strings.Cut(tracked, "/")
		return remote, remoteBranch, nil
	}
	names, err := git(s.Root, "remote")
	if err != nil {
		return "", "", errors.New("the ledger clone's remotes are unreadable")
	}
	remotes := strings.Fields(names)
	if len(remotes) == 1 {
		branch, branchErr := git(s.Root, "branch", "--show-current")
		if branchErr != nil || branch == "" {
			return "", "", errors.New("the ledger clone's current branch is unreadable")
		}
		return remotes[0], branch, nil
	}
	return "", "", nil
}

// observeReplication refreshes the destination after a push attempt and
// distinguishes the exact replicated revision from concurrently advanced or
// replaced history.
func (s *Store) observeReplication(remote, branch, revision string, pushSucceeded bool) (PublicationNote, bool) {
	if _, err := git(s.Root, "fetch", "--quiet", remote); err != nil {
		return PublicationNote{}, false
	}
	upstream := "refs/remotes/" + remote + "/" + branch
	remoteHead, err := git(s.Root, "rev-parse", upstream)
	if err != nil {
		return PublicationNote{}, false
	}
	if remoteHead == revision || gitOK(s.Root, "merge-base", "--is-ancestor", revision, upstream) {
		return PublicationNote{Status: PushPushed}, true
	}
	if !pushSucceeded && gitOK(s.Root, "merge-base", "--is-ancestor", upstream, revision) {
		return PublicationNote{
			Status: PushPending,
			Detail: "push failed and upstream " + remote + "/" + branch + " remains behind ledger revision " + revision,
		}, true
	}
	return PublicationNote{
		Status: PushReconciliation,
		Detail: "upstream " + remote + "/" + branch + " changed while replicating " + revision + "; skl merges, rebases, and force-pushes nothing",
	}, true
}

func (s *Store) destinationContains(remote, branch, revision string) bool {
	upstream := "refs/remotes/" + remote + "/" + branch
	return gitOK(s.Root, "show-ref", "--verify", "--quiet", upstream) &&
		gitOK(s.Root, "merge-base", "--is-ancestor", revision, upstream)
}

// diverged reports whether the upstream branch holds history that is not an
// ancestor of the exact local revision being replicated. A missing upstream
// branch is a plain first push, not divergence.
func (s *Store) diverged(remote, branch, revision string) (bool, error) {
	upstream := "refs/remotes/" + remote + "/" + branch
	if !gitOK(s.Root, "show-ref", "--verify", "--quiet", upstream) {
		return false, nil
	}
	if gitOK(s.Root, "merge-base", "--is-ancestor", upstream, revision) {
		return false, nil
	}
	return true, nil
}

// recordPublication commits the publication bookkeeping: attachments and
// pending facts in state.json and proposal.json. Contract bytes are never
// rewritten, and nothing is committed when no fact changed.
func (s *Store) recordPublication(project string, declaration *ProposalDeclaration, outcome *Acceptance, pushed bool) (string, error) {
	var revision string
	err := s.withMutation(func() error {
		proposalPath := filepath.Join(projectsRoot, project, "proposals", declaration.Proposal)
		recordPaths := []string{filepath.Join(proposalPath, "proposal.json")}
		for index := range declaration.Slices {
			recordPaths = append(recordPaths, filepath.Join(proposalPath, declaration.Slices[index].Name, "state.json"))
		}
		if err := s.requireCleanPaths(recordPaths...); err != nil {
			return err
		}
		meta, _, err := s.readProposalMeta(project, declaration.Proposal)
		if err != nil {
			return err
		}
		previousParentIssue, previousParentNote := meta.ParentIssue, meta.ParentPublication
		switch {
		case meta.ParentIssue != nil && outcome.ParentIssue != nil && !sameAttachment(meta.ParentIssue, outcome.ParentIssue):
			return refuse(
				"concurrent publication recorded a different parent issue for proposals/"+declaration.Proposal,
				"preserve both attachments and reconcile the proposal record with human direction",
			)
		case meta.ParentIssue != nil:
			outcome.ParentIssue = meta.ParentIssue
			outcome.ParentNote = nil
			meta.ParentPublication = nil
		case outcome.ParentIssue != nil:
			meta.ParentIssue = outcome.ParentIssue
			meta.ParentPublication = nil
		case meta.ParentPublication != nil && meta.ParentPublication.Status == issueReserved && !outcome.ownsParentReservation:
			outcome.ParentNote = meta.ParentPublication
		case outcome.ParentNote != nil:
			meta.ParentPublication = outcome.ParentNote
		default:
			outcome.ParentNote = meta.ParentPublication
		}
		parentChanged := !sameAttachment(previousParentIssue, meta.ParentIssue) || !sameNote(previousParentNote, meta.ParentPublication)

		type stateUpdate struct {
			path  string
			state SliceState
		}
		var updates []stateUpdate
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
			if state.Issue != nil && acceptance.Issue != nil && !sameAttachment(state.Issue, acceptance.Issue) {
				return refuse(
					"concurrent publication recorded a different issue for "+ItemPath(declaration.Proposal, slice.Name),
					"preserve both attachments and reconcile the Work Item record with human direction",
				)
			}
			if state.Issue != nil {
				acceptance.Issue = state.Issue
				acceptance.IssueStatus = nil
			}
			publication := PublicationState{}
			if before != nil {
				publication = *before
			}
			switch {
			case acceptance.Issue != nil:
				state.Issue = acceptance.Issue
				publication.Issue = nil
			case publication.Issue != nil && publication.Issue.Status == issueReserved && !acceptance.ownsIssueReservation:
				acceptance.IssueStatus = publication.Issue
			case acceptance.IssueStatus != nil && acceptance.IssueStatus.Status != "":
				publication.Issue = acceptance.IssueStatus
			}
			if acceptance.GroupingStatus != nil && acceptance.GroupingStatus.Status != "" {
				publication.Grouping = acceptance.GroupingStatus
			} else {
				publication.Grouping = nil
			}
			if pushed {
				publication.Push = nil
			} else if acceptance.PushStatus != nil && acceptance.PushStatus.Status != "" {
				publication.Push = acceptance.PushStatus
			}
			if publication.Push == nil && publication.Issue == nil && publication.Grouping == nil {
				state.Publication = nil
			} else {
				state.Publication = &publication
			}
			if !samePublication(before, state.Publication) || !sameAttachment(previousIssue, state.Issue) {
				updates = append(updates, stateUpdate{
					path: filepath.Join(proposalPath, slice.Name, "state.json"), state: state,
				})
			}
		}

		var changedPaths []string
		if parentChanged {
			if err := s.writeProposalMeta(project, declaration.Proposal, meta); err != nil {
				return err
			}
			changedPaths = append(changedPaths, filepath.Join(proposalPath, "proposal.json"))
		}
		for _, update := range updates {
			if err := writeJSON(filepath.Join(s.Root, update.path), update.state); err != nil {
				return err
			}
			changedPaths = append(changedPaths, update.path)
		}
		if len(changedPaths) > 0 {
			if err := s.commit("record publication "+project+"/"+declaration.Proposal, changedPaths...); err != nil {
				return err
			}
		}
		var headErr error
		revision, headErr = s.head()
		return headErr
	})
	return revision, err
}

func samePublication(before, after *PublicationState) bool {
	if before == nil || after == nil {
		return before == after
	}
	return sameNote(before.Push, after.Push) && sameNote(before.Issue, after.Issue) && sameNote(before.Grouping, after.Grouping)
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
