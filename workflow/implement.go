package workflow

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	skilldist "github.com/vicrdguez/skills"
)

type State string

// CurrentWorktree selects an invocation's worktree; it is never a backend identity.
const CurrentWorktree WorkItemID = "<current-worktree>"

const (
	Ready          State = "ready_for_implementation"
	Rework         State = "rework"
	NeedsHuman     State = "needs_human"
	AwaitingReview State = "awaiting_review"
	ReadyForMerge  State = "ready_for_merge"
	Merged         State = "merged"
	Superseded     State = "superseded"
)

type ImplementationItem struct {
	Source                *LifecycleObservation
	Synchronization       bool
	Problem               string
	Submission            *Submission
	Feedback              []skilldist.ReviewComment
	Branch                string
	ID                    WorkItemID
	Order                 int
	State                 State
	CreatedAt             string
	Claimed               bool
	SourceClaimAcquiredAt string
	Blockers              []WorkItemID
}

type Submission struct {
	Lifecycle       *LifecycleObservation
	PendingReview   State
	ClaimAcquiredAt string
	BodyUpdatedAt   string
	Merged          bool
	Mergeability    string
	CreatedAt       string
	State           State
	Claimed         bool
	ID              SubmissionID
	Head            string
	Base            string
	Body            string
	Draft           bool
	Comments        []skilldist.ReviewComment
}

// LifecycleObservation retains overlaps while a multi-record transition is in flight.
// Source and every attached Submission must supply an observation, even with no states.
type LifecycleObservation struct {
	States  []State
	Open    bool
	Claimed bool
	Merged  bool
}

type ImplementationBackend interface {
	// ImplementationItems must retain source and Submission lifecycle observations;
	// derived State and Claimed fields are not substitutes for those records.
	ImplementationItems(context.Context) ([]ImplementationItem, error)
	ClaimImplementation(context.Context, ImplementationItem) error
	ImplementationHead(context.Context, string) (string, error)
	// SubmissionBodyMatches compares an observation with the body publication would produce.
	SubmissionBodyMatches(id WorkItemID, actual, supplied string) (bool, error)
	PublishImplementation(context.Context, ImplementationItem, Submission) (Submission, error)
	AwaitImplementationReview(context.Context, ImplementationItem, func() error) error
	PauseImplementation(context.Context, ImplementationItem, string, func() error) error
}

type InvariantError struct{ Reason string }

func (e *InvariantError) Error() string { return e.Reason }
func Refuse(reason string) error        { return &InvariantError{Reason: reason} }

// RefuseNonMainBase reports an observed Submission destination other than main.
// An unobserved base keeps its existing meaning; earlier valid effects are never rolled back.
func RefuseNonMainBase(id SubmissionID, base string) error {
	if base == "" || base == "main" {
		return nil
	}
	return Refuse("existing Submission " + string(id) + " targets " + base + "; inspect it and explicitly repair its base to main before retrying")
}

type ImplementationOutcome struct {
	Ledger *LedgerHistory             `json:"ledger,omitempty"`
	Head   string                     `json:"head,omitempty"`
	Facts  *skilldist.InvocationFacts `json:"-"`
	Status string                     `json:"status"`
	Reason string                     `json:"reason,omitempty"`
	Item   *ImplementationItem        `json:"item,omitempty"`
}

func loadImplementation(ctx context.Context, backend ImplementationBackend) ([]ImplementationItem, error) {
	items, err := backend.ImplementationItems(ctx)
	for i := range items {
		items[i] = ReconcileImplementation(items[i])
	}
	return items, err
}

// ReconcileImplementation applies canonical lifecycle rules to backend observations.
// Integrations also use it for the existing shared review and Claim read-backs.
func ReconcileImplementation(item ImplementationItem) ImplementationItem {
	if item.Source == nil || item.Submission != nil && item.Submission.Lifecycle == nil {
		item.Problem = "missing lifecycle observations; repair the Backend record"
		return item
	}
	sourceState, sourceProblem := item.Source.state()
	sourceClaimed := item.Source.Claimed
	item.State, item.Claimed = sourceState, sourceClaimed
	if item.Problem == "" {
		item.Problem = sourceProblem
	}
	if submission := item.Submission; submission != nil {
		copy := *submission
		item.Submission, submission = &copy, &copy
		prState, prProblem := submission.Lifecycle.state()
		submission.State, submission.Claimed = prState, submission.Lifecycle.Claimed
		if item.Problem == "" || item.Problem == "contradictory lifecycle projections" {
			item.Problem = prProblem
			if item.Problem == "" {
				item.Problem = sourceProblem
			}
		}
		state := sourceState
		item.Claimed = item.Claimed || submission.Claimed
		if state == NeedsHuman && (prState == Rework || prState == AwaitingReview) && prProblem == "" {
			item.State = prState
		} else if state == NeedsHuman || prState == NeedsHuman {
			item.State = NeedsHuman
		} else if prState != "" {
			if state == Ready && prState != AwaitingReview {
				if item.Problem == "" || item.Problem == "contradictory lifecycle projections" {
					item.Problem = "source Ready contradicts Submission lifecycle"
				}
			} else if state != Ready {
				item.State = prState
			}
		}
		if submission.Merged || submission.Lifecycle.Merged || sourceState == Merged {
			item.State = Merged
		} else if !submission.Lifecycle.Open || sourceState == Superseded {
			item.State = Superseded
		}
	}
	if !item.Source.Open && item.State != Merged && item.State != ReadyForMerge && item.State != Superseded && (item.Problem == "" || item.Problem == "contradictory lifecycle projections" || item.Problem == "source Ready contradicts Submission lifecycle") {
		item.Problem = "source is closed without a merged Submission"
	}
	if item.Submission != nil && item.Submission.PendingReview != "" && sourceProblem == "" && (item.Problem == "" || item.Problem == "contradictory lifecycle projections") {
		item.Problem = ""
		item.State = item.Submission.PendingReview
	}
	if item.Submission != nil && item.Submission.Merged {
		item.Claimed = false
	}
	return item
}

func (observation LifecycleObservation) state() (State, string) {
	var state State
	problem := ""
	for _, next := range observation.States {
		if state != "" && state != next {
			problem = "contradictory lifecycle projections"
		}
		if state == "" || next == NeedsHuman {
			state = next
		}
	}
	return state, problem
}

// PermitImplementationReview checks the fresh Submission observation before its
// review projection is written. Any unresolved lifecycle contradiction must stop
// the handoff; a normalized first state is not direction proof.
func PermitImplementationReview(observation *LifecycleObservation) error {
	if observation == nil {
		return Refuse("review requires a durable Submission; publish it before retrying")
	}
	state, problem := observation.state()
	if problem != "" {
		return Refuse("Submission lifecycle contradicts review handoff: " + problem + "; repair its projections")
	}
	if state == NeedsHuman || state == ReadyForMerge || state == Ready {
		return Refuse("Submission lifecycle contradicts review handoff; repair its projections")
	}
	return nil
}

func InspectImplementation(ctx context.Context, root string, id WorkItemID, endpoints ArtifactEndpoints, backend ImplementationBackend) (ImplementationOutcome, error) {
	items, err := loadImplementation(ctx, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		head, err := git(root, "rev-parse", "refs/heads/"+item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		history, err := InspectLedger(root, head, item.Branch, endpoints, implementationLedgerPolicy(item.State))
		return ImplementationOutcome{Status: "inspected", Item: &item, Head: head, Ledger: &history}, err
	}
	return ImplementationOutcome{Status: "fix_required", Reason: "Work Item unavailable; supply its explicit stable --item identity"}, nil
}

func StartImplementation(ctx context.Context, root, remote string, id WorkItemID, endpoints ArtifactEndpoints, backend ImplementationBackend) (ImplementationOutcome, error) {
	items, err := loadImplementation(ctx, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if id != "" {
		if id == CurrentWorktree {
			main, err := primaryWorktree(root)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			location, err := git(root, "rev-parse", "--show-toplevel")
			if err != nil {
				return ImplementationOutcome{}, err
			}
			for _, item := range items {
				if item.Claimed && filepath.Clean(location) == filepath.Join(main, ".worktrees", item.Branch) {
					if id != CurrentWorktree {
						return ImplementationOutcome{Status: "fix_required", Reason: "worktree identity is ambiguous; resume with --item after repairing attachments"}, nil
					}
					id = item.ID
				}
			}
		}
		for _, item := range items {
			if item.ID == id && item.Claimed && (item.State == Ready || item.State == Rework) {
				prepared, outcome, err := prepareImplementationStart(ctx, root, remote, item, endpoints, backend)
				if err != nil || outcome.Status != "" {
					return outcome, err
				}
				if err := backend.ClaimImplementation(ctx, prepared); err != nil {
					return ImplementationOutcome{}, err
				}
				item = prepared
				return implementationPacket(root, remote, item, endpoints)
			}
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "explicit Work Item is not an unambiguous implementation Claim; repair its projections before resuming"}, nil
	}
	merged := make(map[WorkItemID]bool)
	for _, item := range items {
		merged[item.ID] = item.State == Merged
	}
	slices.SortFunc(items, func(a, b ImplementationItem) int {
		if a.State != b.State {
			if a.State == Rework {
				return -1
			}
			if b.State == Rework {
				return 1
			}
		}
		if age := cmp.Compare(a.CreatedAt, b.CreatedAt); age != 0 {
			return age
		}
		return cmp.Compare(a.Order, b.Order)
	})
	for _, item := range items {
		if item.Claimed || item.State != Ready && item.State != Rework {
			continue
		}
		blocked := false
		if item.State == Ready {
			for _, blocker := range item.Blockers {
				blocked = blocked || !merged[blocker]
			}
		}
		if blocked {
			continue
		}
		prepared, outcome, err := prepareImplementationStart(ctx, root, remote, item, endpoints, backend)
		if err != nil || outcome.Status != "" {
			return outcome, err
		}
		item = prepared
		claimErr := backend.ClaimImplementation(ctx, item)
		observed, err := loadImplementation(ctx, backend)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		for _, current := range observed {
			if current.ID == item.ID && current.Claimed && current.State == item.State && current.Problem == "" && current.Branch == item.Branch {
				return implementationPacket(root, remote, current, endpoints)
			}
		}
		if claimErr != nil {
			return ImplementationOutcome{}, claimErr
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "Claim read-back contradicts selected state; repair the Work Item projections and explicitly resume"}, nil
	}
	return ImplementationOutcome{Status: "no_work"}, nil
}

func validConventionalBranch(root, branch string) bool {
	return branch != "" && gitOK(root, "check-ref-format", "--branch", branch) == nil && !strings.Contains(branch, "/")
}

func prepareImplementationStart(ctx context.Context, root, remote string, item ImplementationItem, endpoints ArtifactEndpoints, backend ImplementationBackend) (ImplementationItem, ImplementationOutcome, error) {
	refuse := func(reason string) (ImplementationItem, ImplementationOutcome, error) {
		return item, ImplementationOutcome{Status: "fix_required", Reason: reason, Item: &item}, nil
	}
	if item.Problem != "" {
		return refuse(item.Problem + "; repair contradictory projections before resuming")
	}
	if !validConventionalBranch(root, item.Branch) {
		return refuse("invalid conventional branch identity; repair the Work Item attachment")
	}
	head, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
	if err != nil {
		head, err = git(root, "rev-parse", "--verify", "refs/remotes/"+remote+"/"+item.Branch+"^{commit}")
	}
	if err != nil {
		return refuse("branch unavailable; fetch the published branch and resume")
	}
	history, err := InspectLedger(root, head, item.Branch, endpoints, implementationLedgerPolicy(item.State))
	if err != nil {
		return item, ImplementationOutcome{}, err
	}
	if len(history.Violations) > 0 {
		return refuse(fmt.Sprint(history.Violations) + "; repair frozen ledger history")
	}
	if item.State == Ready {
	} else {
		if item.Submission == nil {
			return refuse("Rework requires its existing Submission; repair the attachment")
		}
		if !item.Submission.Draft && history.Phase != "retired" {
			return refuse("finding-driven Rework must keep the ledger retired; restore its deletion history")
		}
	}
	return item, ImplementationOutcome{}, nil
}

func implementationPacket(root, remote string, item ImplementationItem, endpoints ArtifactEndpoints) (ImplementationOutcome, error) {
	main, err := primaryWorktree(root)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	history := LedgerHistory{}
	if item.Branch != "" {
		head, headErr := git(root, "rev-parse", "refs/heads/"+item.Branch)
		if headErr != nil {
			head, headErr = git(root, "rev-parse", "refs/remotes/"+remote+"/"+item.Branch)
		}
		if headErr != nil {
			return ImplementationOutcome{Status: "fix_required", Reason: "branch unavailable; fetch and create the conventional worktree before resuming"}, nil
		}
		history, err = InspectLedger(root, head, item.Branch, endpoints, implementationLedgerPolicy(item.State))
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if len(history.Violations) != 0 {
			return ImplementationOutcome{Status: "fix_required", Reason: fmt.Sprint(history.Violations) + "; repair ledger history and resume"}, nil
		}
	}
	facts := skilldist.ImplementationFacts{Branch: item.Branch, Worktree: filepath.Join(main, ".worktrees", item.Branch), ArtifactBaseline: history.Baseline, ArtifactCompletion: history.Completion, SuppliedArtifactBaseline: endpoints.Baseline, SuppliedArtifactCompletion: endpoints.Completion}
	if item.Submission != nil {
		facts.Comments = item.Submission.Comments
	}
	facts.ResultDirectory, err = os.MkdirTemp("", "skl-implement-")
	if err != nil {
		return ImplementationOutcome{}, err
	}
	facts.Remote = remote
	if err := os.WriteFile(filepath.Join(facts.ResultDirectory, ".skl-result"), []byte("skl.implement/v1\n"), 0600); err != nil {
		os.RemoveAll(facts.ResultDirectory)
		return ImplementationOutcome{}, err
	}
	return ImplementationOutcome{Status: "work_available", Item: &item, Facts: &skilldist.InvocationFacts{Implementation: &facts}}, nil
}

func implementationLedgerPolicy(state State) LedgerPolicy {
	if state == Rework || state == AwaitingReview || state == ReadyForMerge {
		return RequireRetiredArtifacts
	}
	return InspectArtifacts
}
