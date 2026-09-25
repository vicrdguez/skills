package ledger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vicrdguez/skills/github"
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
	// IssuePending is a durable pending phase-presentation fact.
	IssuePending = "pending"
)

// Immediate issue and parent publication outcomes. They are reported for the
// current invocation only: none is persisted, and none leaves a recovery
// obligation behind.
const (
	IssueCreated      = "created"
	IssueUpdated      = "updated"
	IssueMissingInput = "missing_input"
	IssueFailed       = "failed"
	IssueUncertain    = "uncertain"
	IssueSuperseded   = "superseded"
	IssueConflict     = "conflict"
)

// IssueProse is the current agent-authored public prose for one invocation,
// keyed by slice name, plus the optional parent body. It is transport input,
// never ledger content.
type IssueProse struct {
	Bodies map[string][]byte
	Parent []byte
}

// Publish attempts the current descriptive issue and parent presentation of
// one accepted proposal, then replicates the ledger through the clone's own
// remote configuration. The local records are already committed and stay
// authoritative: forge failures are reported for this invocation only, and
// only successfully established attachments are recorded.
func Publish(ctx context.Context, store *Store, outcome *Acceptance, prose IssueProse, forge Forge, acceptedRevision string) {
	if forge == nil {
		for index := range outcome.Slices {
			outcome.Slices[index].IssueStatus = &PublicationNote{
				Status: IssueFailed,
				Detail: "no forge attachment surface is available",
			}
		}
	} else {
		publishIssues(ctx, store, outcome, prose, forge)
	}
	// The reported push outcome is the replication of the accepted records
	// themselves; a best-effort second push below only carries attachment
	// bookkeeping and never downgrades that fact.
	note, _ := store.push(acceptedRevision)
	for index := range outcome.Slices {
		outcome.Slices[index].PushStatus = &note
	}
	pushed := note.Status == PushPushed
	bookkeepingRevision, err := store.recordPublication(outcome, pushed)
	if err != nil {
		outcome.BookkeepingStatus = &PublicationNote{
			Status: IssueFailed,
			Detail: "publication bookkeeping was not written: " + err.Error() + "; any forge object this invocation created stays unrecorded, and a later publication may create a duplicate",
		}
		return
	}
	outcome.Commit = bookkeepingRevision
	if !pushed || bookkeepingRevision == acceptedRevision {
		return
	}
	if retry, ok := store.push(bookkeepingRevision); !ok {
		for index := range outcome.Slices {
			outcome.Slices[index].PushStatus = &retry
		}
		pendingRevision, err := store.recordPublication(outcome, false)
		if err != nil {
			outcome.BookkeepingStatus = &PublicationNote{
				Status: IssueFailed,
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

// PublishCurrent publishes the current issue and parent presentation of one
// accepted proposal from its committed local records. It neither repeats
// acceptance nor needs a Claim or source preparation, and it selects the
// current view rather than an earlier unfinished attempt.
func PublishCurrent(ctx context.Context, store *Store, repository github.RepositoryID, proposal string, prose IssueProse, forge Forge) (*Acceptance, error) {
	if !ValidRecordName(proposal) {
		return nil, refuse("proposal name "+proposal+" is not a valid record name", "use the proposal name the acceptance reported, such as add-order-cancellation")
	}
	outcome := &Acceptance{Status: "attempted", Project: repository.Name, Repository: repository.Owner + "/" + repository.Name, Proposal: proposal}
	var revision string
	err := store.withMutation(func() error {
		project, err := store.resolveProject(repository)
		if err != nil {
			return err
		}
		proposalPath := filepath.Join(projectsRoot, project.Name, "proposals", proposal)
		meta, recorded, err := store.readProposalMeta(project.Name, proposal)
		if err != nil {
			return err
		}
		if project.Created || !recorded {
			return refuse("no accepted proposal "+proposal+" is recorded for "+outcome.Repository, "accept the proposal first, or use the proposal name its acceptance reported")
		}
		names, err := store.acceptedSlices(project.Name, proposal)
		if err != nil {
			return err
		}
		paths := []string{filepath.Join(proposalPath, "proposal.json")}
		for _, name := range names {
			paths = append(paths, filepath.Join(proposalPath, name, "state.json"))
		}
		if err := store.requireCleanPaths(paths...); err != nil {
			return err
		}
		for name := range prose.Bodies {
			if !slices.Contains(names, name) {
				return refuse("issue prose names "+name+", which is not a slice of proposal "+proposal, "supply --issue only for the accepted slices: "+strings.Join(names, ", "))
			}
		}
		if len(names) < 2 && prose.Parent != nil {
			return refuse("single-slice proposals take no parent body", "remove --parent-body; a single-slice proposal has no coordination parent")
		}
		outcome.ParentTitle, outcome.ParentIssue = meta.ParentTitle, meta.ParentIssue
		if err := store.loadAcceptedStates(project.Name, proposal, names, outcome); err != nil {
			return err
		}
		revision, err = store.head()
		return err
	})
	if err != nil {
		return nil, err
	}
	outcome.Commit = revision
	outcome.HeadRef = projectsRoot + "/" + outcome.Project
	Publish(ctx, store, outcome, prose, forge, revision)
	return outcome, nil
}

// acceptedSlices lists the slice records of one accepted proposal: its
// directory membership, sorted by name.
func (s *Store) acceptedSlices(project, proposal string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, projectsRoot, project, "proposals", proposal))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && ValidRecordName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// publishIssues presents the current prose on each slice issue and the
// multi-slice parent, then applies the current parent grouping. Established
// attachments are updated rather than recreated, a missing one may be
// created once, and nothing is adopted from a similar title or body.
func publishIssues(ctx context.Context, store *Store, outcome *Acceptance, prose IssueProse, forge Forge) {
	for index := range outcome.Slices {
		slice := &outcome.Slices[index]
		body, supplied := prose.Bodies[slice.Name]
		if !supplied {
			if slice.Issue == nil {
				slice.IssueStatus = &PublicationNote{Status: IssueMissingInput, Detail: "no current descriptive issue prose was supplied"}
			}
			continue
		}
		slice.Issue, slice.IssueStatus = presentIssue(ctx, forge, outcome.Repository, slice.Issue, slice.Title, string(body), func() (*ForgeAttachment, error) {
			return store.recordedIssue(outcome.Project, outcome.Proposal, slice.Name)
		})
	}
	if len(outcome.Slices) < 2 {
		return
	}
	if prose.Parent == nil {
		if outcome.ParentIssue == nil {
			outcome.ParentNote = &PublicationNote{Status: IssueMissingInput, Detail: "no current parent issue prose was supplied"}
		}
	} else if outcome.ParentIssue == nil && !anyAttached(outcome.Slices) {
		outcome.ParentNote = &PublicationNote{Status: IssueFailed, Detail: "not attempted: no child issue is attached yet to group under a parent"}
	} else {
		outcome.ParentIssue, outcome.ParentNote = presentIssue(ctx, forge, outcome.Repository, outcome.ParentIssue, outcome.ParentTitle, string(prose.Parent), func() (*ForgeAttachment, error) {
			return store.recordedParent(outcome.Project, outcome.Proposal)
		})
	}
	if outcome.ParentIssue != nil {
		groupChildren(ctx, store, outcome, forge)
	}
}

// presentIssue updates an established issue or creates a missing one with
// the current prose. recorded rereads the selected record so a mutation is
// never sent for a superseded selection, including between update retries.
// It returns the attachment to report and this invocation's outcome note.
func presentIssue(ctx context.Context, forge Forge, repository string, selected *ForgeAttachment, title, body string, recorded func() (*ForgeAttachment, error)) (*ForgeAttachment, *PublicationNote) {
	if err := checkSelection(recorded, selected, false); err != nil {
		return selectionOutcome(err, selected, "present the current view")
	}
	if selected != nil {
		if foreign(selected, repository) {
			return selected, foreignNote(selected, repository)
		}
		err := forge.UpdateIssue(ctx, selected.Number, title, body, func() error {
			return checkSelection(recorded, selected, false)
		})
		var changed *selectionChanged
		if errors.As(err, &changed) {
			return selectionOutcome(err, selected, "present the current view")
		}
		if err != nil {
			return selected, &PublicationNote{Status: IssueFailed, Detail: "updating the established issue failed: " + err.Error()}
		}
		return selected, &PublicationNote{Status: IssueUpdated}
	}
	number, err := forge.CreateIssue(ctx, title, body)
	switch {
	case err == nil:
		return &ForgeAttachment{Repository: repository, Number: number}, &PublicationNote{Status: IssueCreated}
	case isUnknownOutcome(err):
		// The forge may have created the issue. Another create could
		// duplicate it, so this invocation reports the uncertainty and stops.
		return nil, &PublicationNote{Status: IssueUncertain, Detail: "issue creation may or may not have succeeded (" + err.Error() + "); it was not retried, and a later explicit publication may create a duplicate"}
	default:
		return nil, &PublicationNote{Status: IssueFailed, Detail: err.Error()}
	}
}

// groupChildren links every currently attached child under the parent.
// Grouping is reevaluated from the observed children on every invocation;
// a failure is reported, never saved as a retry plan. Before each attach
// both the parent and the child record are reread, so no grouping is sent
// for a superseded, removed, or unreadable selection.
func groupChildren(ctx context.Context, store *Store, outcome *Acceptance, forge Forge) {
	if foreign(outcome.ParentIssue, outcome.Repository) {
		for index := range outcome.Slices {
			if outcome.Slices[index].Issue != nil {
				outcome.Slices[index].GroupingStatus = foreignNote(outcome.ParentIssue, outcome.Repository)
			}
		}
		return
	}
	linked, err := forge.ListChildren(ctx, outcome.ParentIssue.Number)
	if err != nil {
		for index := range outcome.Slices {
			if outcome.Slices[index].Issue != nil {
				outcome.Slices[index].GroupingStatus = &PublicationNote{Status: IssueFailed, Detail: "observing the parent grouping failed: " + err.Error()}
			}
		}
		return
	}
	recordedParent := func() (*ForgeAttachment, error) { return store.recordedParent(outcome.Project, outcome.Proposal) }
	for index := range outcome.Slices {
		child := &outcome.Slices[index]
		if child.Issue == nil {
			continue
		}
		if foreign(child.Issue, outcome.Repository) {
			child.GroupingStatus = foreignNote(child.Issue, outcome.Repository)
			continue
		}
		if slices.Contains(linked, child.Issue.Number) {
			continue
		}
		recordedChild := func() (*ForgeAttachment, error) {
			return store.recordedIssue(outcome.Project, outcome.Proposal, child.Name)
		}
		if err := checkSelection(recordedParent, outcome.ParentIssue, created(outcome.ParentNote)); err != nil {
			_, child.GroupingStatus = selectionOutcome(err, nil, "group under the current parent")
			continue
		}
		if err := checkSelection(recordedChild, child.Issue, created(child.IssueStatus)); err != nil {
			_, child.GroupingStatus = selectionOutcome(err, nil, "group the current child")
			continue
		}
		if err := forge.AttachChild(ctx, outcome.ParentIssue.Number, child.Issue.Number); err != nil {
			child.GroupingStatus = &PublicationNote{Status: IssueFailed, Detail: "grouping under the parent issue failed: " + err.Error()}
		}
	}
}

// selectionChanged reports that a selected record no longer holds the
// attachment this invocation is about to act on.
type selectionChanged struct{ current *ForgeAttachment }

func (*selectionChanged) Error() string { return "the local record changed during this invocation" }

// checkSelection rereads a selected record before a forge mutation. The
// selection still applies when the record holds the selected attachment, or
// when this invocation created that attachment and it awaits recording. A
// removed or different recorded attachment supersedes it.
func checkSelection(recorded func() (*ForgeAttachment, error), selected *ForgeAttachment, created bool) error {
	current, err := recorded()
	if err != nil {
		return fmt.Errorf("the current local record is unreadable: %w", err)
	}
	if sameAttachment(current, selected) || current == nil && created {
		return nil
	}
	return &selectionChanged{current: current}
}

// selectionOutcome reports a mutation that checkSelection stopped: a
// superseded selection reports the current recorded attachment, and an
// unreadable record keeps the selected one.
func selectionOutcome(err error, selected *ForgeAttachment, retry string) (*ForgeAttachment, *PublicationNote) {
	var changed *selectionChanged
	if errors.As(err, &changed) {
		return changed.current, &PublicationNote{Status: IssueSuperseded, Detail: changed.Error() + "; nothing further was sent for it, so publish again to " + retry}
	}
	return selected, &PublicationNote{Status: IssueFailed, Detail: "not attempted: " + err.Error()}
}

// created reports whether this invocation created the attachment a note
// describes, so it may be awaiting recording.
func created(note *PublicationNote) bool {
	return note != nil && note.Status == IssueCreated
}

// foreign reports whether a recorded attachment belongs to a repository
// other than the selected one the forge is bound to. Acting on its bare
// number would target a different object.
func foreign(attachment *ForgeAttachment, repository string) bool {
	return attachment != nil && !strings.EqualFold(attachment.Repository, repository)
}

func foreignNote(attachment *ForgeAttachment, repository string) *PublicationNote {
	return &PublicationNote{
		Status: IssueConflict,
		Detail: fmt.Sprintf("the recorded attachment %s#%d is outside the selected repository %s; nothing was sent for it and it stays recorded, so resolve it with human direction", attachment.Repository, attachment.Number, repository),
	}
}

func anyAttached(outcomes []SliceAcceptance) bool {
	for index := range outcomes {
		if outcomes[index].Issue != nil {
			return true
		}
	}
	return false
}

// recordedIssue reads the currently recorded issue attachment of one slice.
func (s *Store) recordedIssue(project, proposal, slice string) (*ForgeAttachment, error) {
	state, found, err := s.readSliceState(project, proposal, slice)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("record of slice %s is missing its state.json", slice)
		}
		return nil, err
	}
	return state.Issue, nil
}

// recordedParent reads the currently recorded parent issue attachment.
func (s *Store) recordedParent(project, proposal string) (*ForgeAttachment, error) {
	meta, _, err := s.readProposalMeta(project, proposal)
	return meta.ParentIssue, err
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

// recordPublication commits newly established attachments and pending push
// facts in state.json and proposal.json. It preserves unrelated fields and
// concurrent local work, reports rather than replaces a different recorded
// attachment, never rewrites Contract bytes, and commits nothing when no
// fact changed.
func (s *Store) recordPublication(outcome *Acceptance, pushed bool) (string, error) {
	var revision string
	err := s.withMutation(func() error {
		proposalPath := filepath.Join(projectsRoot, outcome.Project, "proposals", outcome.Proposal)
		recordPaths := []string{filepath.Join(proposalPath, "proposal.json")}
		for index := range outcome.Slices {
			recordPaths = append(recordPaths, filepath.Join(proposalPath, outcome.Slices[index].Name, "state.json"))
		}
		if err := s.requireCleanPaths(recordPaths...); err != nil {
			return err
		}
		meta, _, err := s.readProposalMeta(outcome.Project, outcome.Proposal)
		if err != nil {
			return err
		}
		parentChanged := false
		switch {
		case sameAttachment(meta.ParentIssue, outcome.ParentIssue):
		case meta.ParentIssue == nil && created(outcome.ParentNote):
			meta.ParentIssue = outcome.ParentIssue
			parentChanged = true
		default:
			outcome.ParentNote = recordedNote(outcome.ParentIssue, meta.ParentIssue, outcome.ParentNote)
			outcome.ParentIssue = meta.ParentIssue
		}

		type stateUpdate struct {
			path  string
			state SliceState
		}
		var updates []stateUpdate
		for index := range outcome.Slices {
			acceptance := &outcome.Slices[index]
			state, found, err := s.readSliceState(outcome.Project, outcome.Proposal, acceptance.Name)
			if err != nil || !found {
				if err == nil {
					return fmt.Errorf("record of slice %s is missing its state.json", acceptance.Name)
				}
				return err
			}
			before, previousIssue := state.Publication, state.Issue
			switch {
			case sameAttachment(state.Issue, acceptance.Issue):
			case state.Issue == nil && created(acceptance.IssueStatus):
				state.Issue = acceptance.Issue
			default:
				acceptance.IssueStatus = recordedNote(acceptance.Issue, state.Issue, acceptance.IssueStatus)
				acceptance.Issue = state.Issue
			}
			publication := PublicationState{}
			if before != nil {
				publication = *before
			}
			if pushed {
				publication.Push = nil
			} else if acceptance.PushStatus != nil && acceptance.PushStatus.Status != "" {
				publication.Push = acceptance.PushStatus
			}
			if publication.Push == nil && publication.Source == nil && publication.Pull == nil && publication.Active == nil {
				state.Publication = nil
			} else {
				state.Publication = &publication
			}
			if !samePublication(before, state.Publication) || !sameAttachment(previousIssue, state.Issue) {
				updates = append(updates, stateUpdate{
					path: filepath.Join(proposalPath, acceptance.Name, "state.json"), state: state,
				})
			}
		}

		var changedPaths []string
		if parentChanged {
			if err := s.writeProposalMeta(outcome.Project, outcome.Proposal, meta); err != nil {
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
			if err := s.commit("record publication "+outcome.Project+"/"+outcome.Proposal, changedPaths...); err != nil {
				return err
			}
		}
		var headErr error
		revision, headErr = s.head()
		return headErr
	})
	return revision, err
}

// recordedNote reports the outcome when the record no longer holds the
// attachment this invocation selected; the recorded one, or its absence, is
// retained. An attachment this invocation created conflicts with a different
// recorded one; any other selection was superseded by concurrent local work,
// and a removed established attachment is never restored.
func recordedNote(selected, recorded *ForgeAttachment, note *PublicationNote) *PublicationNote {
	if selected == nil {
		return note
	}
	if created(note) && recorded != nil {
		return &PublicationNote{
			Status: IssueConflict,
			Detail: fmt.Sprintf("this invocation established %s#%d, but %s#%d is already recorded and was retained; resolve the extra forge object with human direction", selected.Repository, selected.Number, recorded.Repository, recorded.Number),
		}
	}
	return &PublicationNote{Status: IssueSuperseded, Detail: fmt.Sprintf("the local record no longer holds %s#%d; the current record was retained, so publish again to present the current view", selected.Repository, selected.Number)}
}

func samePublication(before, after *PublicationState) bool {
	if before == nil || after == nil {
		return before == after
	}
	return sameNote(before.Push, after.Push) && sameNote(before.Source, after.Source) && sameNote(before.Pull, after.Pull) && sameReference(before.Active, after.Active)
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
