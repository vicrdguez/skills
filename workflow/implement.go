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
	"github.com/vicrdguez/skills/github"
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
	Synchronization  bool
	Problem          string
	ResumeState      State
	Submission       *Submission
	Branch           string
	TargetSnapshot   string
	TargetBranch     string
	ID               WorkItemID
	Order            int
	ClosingReference string
	State            State
	CreatedAt        string
	Claimed          bool
	Blockers         []WorkItemID
	Transition       *ImplementationTransition
}

type Submission struct {
	PendingReview        State
	Merged               bool
	Mergeability         string
	Bounces              int
	CreatedAt            string
	ReviewedHead         string
	State                State
	Claimed              bool
	ID                   SubmissionID
	Head                 string
	Base                 string
	Body                 string
	Draft                bool
	PreviousReviewedHead string
	Comments             []skilldist.ReviewComment
}

type ImplementationBackend interface {
	DispatchBackend
	ImplementationTarget(context.Context, github.RepositoryID) (string, error)
	ImplementationItems(context.Context, github.RepositoryID) ([]ImplementationItem, error)
	ClaimImplementation(context.Context, github.RepositoryID, ImplementationItem) error
	ImplementationHead(context.Context, github.RepositoryID, string) (string, error)
	PublishImplementation(context.Context, github.RepositoryID, ImplementationItem, Submission) (Submission, error)
	RecordImplementationTransition(context.Context, github.RepositoryID, ImplementationItem, ImplementationTransition) error
	RetainImplementationClaim(context.Context, github.RepositoryID, ImplementationItem) error
	AwaitImplementationReview(context.Context, github.RepositoryID, ImplementationItem, func() error) error
	PauseImplementation(context.Context, github.RepositoryID, ImplementationItem, string, func() error) error
}

type ImplementationTransition struct {
	From           State  `json:"from"`
	Target         State  `json:"target"`
	Head           string `json:"head"`
	BodyDigest     string `json:"body_digest"`
	DecisionDigest string `json:"decision_digest"`
	Directory      string `json:"directory"`
	Completed      bool   `json:"completed"`
}

type InvariantError struct{ Reason string }

func (e *InvariantError) Error() string { return e.Reason }
func Refuse(reason string) error        { return &InvariantError{Reason: reason} }

type ImplementationOutcome struct {
	Ledger          *LedgerHistory             `json:"ledger,omitempty"`
	Head            string                     `json:"head,omitempty"`
	Facts           *skilldist.InvocationFacts `json:"-"`
	Status          string                     `json:"status"`
	Reason          string                     `json:"reason,omitempty"`
	Item            *ImplementationItem        `json:"item,omitempty"`
	Dispatch        *DispatchFacts             `json:"-"`
	PreviousHandoff *CompletedHandoff          `json:"-"`
}

func loadImplementation(ctx context.Context, root, remote string, backend ImplementationBackend) (github.RepositoryID, []ImplementationItem, error) {
	remote, err := git(root, "remote", "get-url", remote)
	if err != nil {
		return github.RepositoryID{}, nil, err
	}
	repository, err := github.ParseGitHubRemote(remote)
	if err != nil {
		return github.RepositoryID{}, nil, err
	}
	items, err := backend.ImplementationItems(ctx, repository)
	for i := range items {
		if items[i].Submission != nil && items[i].Submission.Merged {
			items[i].State = Merged
			items[i].Claimed = false
		}
	}
	return repository, items, err
}

func InspectImplementation(ctx context.Context, root, remote string, id WorkItemID, backend ImplementationBackend) (ImplementationOutcome, error) {
	remote, err := github.ResolveGitHubRemote(root, remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	_, items, err := loadImplementation(ctx, root, remote, backend)
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
		history, err := InspectLedger(root, head, item.Branch)
		return ImplementationOutcome{Status: "inspected", Item: &item, Head: head, Ledger: &history}, err
	}
	return ImplementationOutcome{Status: "fix_required", Reason: "Work Item unavailable; supply its explicit stable --item identity"}, nil
}

func StartImplementation(ctx context.Context, root, remote string, id WorkItemID, snapshot, reviewedHead string, backend ImplementationBackend) (ImplementationOutcome, error) {
	remote, err := github.ResolveGitHubRemote(root, remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, items, err := loadImplementation(ctx, root, remote, backend)
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
				prepared, outcome, err := prepareImplementationStart(ctx, root, remote, repository, item, snapshot, reviewedHead, backend)
				if err != nil || outcome.Status != "" {
					return outcome, err
				}
				if err := backend.ClaimImplementation(ctx, repository, prepared); err != nil {
					return ImplementationOutcome{}, err
				}
				item = prepared
				round, reference, err := prepareDispatch(ctx, root, remote, repository, item, ImplementLane, true, backend)
				if err != nil {
					return ImplementationOutcome{}, err
				}
				outcome, err = implementationPacket(root, remote, item, filepath.Join(os.TempDir(), round.Directory))
				if err == nil {
					main, _ := primaryWorktree(root)
					outcome.Dispatch = &DispatchFacts{Reference: reference, Lane: ImplementLane, Root: main, Remote: remote}
				}
				return outcome, err
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
		if item.Claimed || item.Submission != nil && item.Submission.PendingReview != "" || item.Transition != nil && !item.Transition.Completed || item.State != Ready && item.State != Rework {
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
		prepared, outcome, err := prepareImplementationStart(ctx, root, remote, repository, item, snapshot, reviewedHead, backend)
		if err != nil || outcome.Status != "" {
			return outcome, err
		}
		item = prepared
		claimErr := backend.ClaimImplementation(ctx, repository, item)
		observed, err := backend.ImplementationItems(ctx, repository)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		for _, current := range observed {
			if current.ID == item.ID && current.Claimed && current.State == item.State && current.Problem == "" && current.Branch == item.Branch && current.TargetSnapshot == item.TargetSnapshot {
				round, reference, err := prepareDispatch(ctx, root, remote, repository, current, ImplementLane, false, backend)
				if err != nil {
					return ImplementationOutcome{}, err
				}
				outcome, err := implementationPacket(root, remote, current, filepath.Join(os.TempDir(), round.Directory))
				if err == nil {
					main, _ := primaryWorktree(root)
					outcome.Dispatch = &DispatchFacts{Reference: reference, Lane: ImplementLane, Root: main, Remote: remote}
				}
				return outcome, err
			}
		}
		if claimErr != nil {
			return ImplementationOutcome{}, claimErr
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "Claim read-back contradicts selected state; repair the Work Item projections and explicitly resume"}, nil
	}
	return ImplementationOutcome{Status: "no_work"}, nil
}

func prepareImplementationStart(ctx context.Context, root, remote string, repository github.RepositoryID, item ImplementationItem, snapshot, reviewedHead string, backend ImplementationBackend) (ImplementationItem, ImplementationOutcome, error) {
	refuse := func(reason string) (ImplementationItem, ImplementationOutcome, error) {
		return item, ImplementationOutcome{Status: "fix_required", Reason: reason, Item: &item}, nil
	}
	if item.Problem != "" {
		return refuse(item.Problem + "; repair contradictory projections before resuming")
	}
	if item.Branch == "" || gitOK(root, "check-ref-format", "--branch", item.Branch) != nil || strings.Contains(item.Branch, "/") {
		return refuse("invalid conventional branch identity; repair the Work Item attachment")
	}
	head, err := git(root, "rev-parse", "--verify", "refs/heads/"+item.Branch+"^{commit}")
	if err != nil {
		head, err = git(root, "rev-parse", "--verify", "refs/remotes/"+remote+"/"+item.Branch+"^{commit}")
	}
	if err != nil {
		return refuse("branch unavailable; fetch the published branch and resume")
	}
	history, err := InspectLedger(root, head, item.Branch)
	if err != nil {
		return item, ImplementationOutcome{}, err
	}
	if len(history.Violations) > 0 {
		return refuse(fmt.Sprint(history.Violations) + "; repair frozen ledger history")
	}
	if item.State == Ready {
		if snapshot != "" {
			if !item.Claimed && item.TargetSnapshot == "" {
				return refuse("a new Claim observes its target on the backend; use --target-snapshot only to resume an existing obligation")
			}
			resolved, err := git(root, "rev-parse", "--verify", snapshot+"^{commit}")
			if err != nil || resolved != snapshot {
				return refuse("Target Snapshot must be an available full commit SHA; fetch the pinned commit")
			}
			if item.TargetSnapshot != "" && item.TargetSnapshot != snapshot {
				return refuse("Target Snapshot contradicts the recorded obligation; use the original snapshot")
			}
			item.TargetSnapshot = snapshot
		}
		if item.TargetBranch == "" {
			item.TargetBranch, err = backend.ImplementationTarget(ctx, repository)
			if err != nil {
				return item, ImplementationOutcome{}, err
			}
		}
		if item.TargetSnapshot == "" {
			if item.Claimed && head != history.Baseline {
				return refuse("Target Snapshot is unknown after history changed; read the original packet and resume with --target-snapshot <sha>")
			}
			item.TargetSnapshot, err = backend.ImplementationHead(ctx, repository, item.TargetBranch)
			if err != nil {
				return item, ImplementationOutcome{}, err
			}
			if item.TargetSnapshot == "" {
				return refuse("target unavailable on the backend; restore target branch " + item.TargetBranch + " and retry")
			}
		}
		resolved, err := git(root, "rev-parse", "--verify", "--end-of-options", item.TargetSnapshot+"^{commit}")
		if err != nil || resolved != item.TargetSnapshot {
			return refuse("Target Snapshot " + item.TargetSnapshot + " is not an available full commit SHA; fetch the target branch and pinned commit, then retry without replacing a recorded snapshot")
		}
	} else {
		if item.Submission == nil {
			return refuse("Rework requires its existing Submission; repair the attachment")
		}
		if reviewedHead != "" {
			resolved, err := git(root, "rev-parse", "--verify", reviewedHead+"^{commit}")
			if err != nil || resolved != reviewedHead || gitOK(root, "merge-base", "--is-ancestor", reviewedHead, head) != nil {
				return refuse("previous reviewed head must be an available ancestor; fetch the original reviewed commit")
			}
			copy := *item.Submission
			copy.PreviousReviewedHead = reviewedHead
			item.Submission = &copy
		}
		if !item.Submission.Draft && history.Phase != "retired" {
			return refuse("finding-driven Rework must keep the ledger retired; restore its deletion history")
		}
		if previous := item.Submission.PreviousReviewedHead; previous != "" {
			resolved, err := git(root, "rev-parse", "--verify", "--end-of-options", previous+"^{commit}")
			if err != nil || resolved != previous || gitOK(root, "merge-base", "--is-ancestor", previous, head) != nil {
				return refuse("recorded reviewed head is unavailable or not an ancestor; fetch the reviewed snapshot and repair its metadata")
			}
		}
	}
	return item, ImplementationOutcome{}, nil
}

func implementationPacket(root, remote string, item ImplementationItem, directory string) (ImplementationOutcome, error) {
	if item.State == Rework && !item.Synchronization && (item.Submission == nil || item.Submission.PreviousReviewedHead == "") {
		return ImplementationOutcome{Status: "fix_required", Item: &item, Reason: "previous reviewed head needs agent extraction from the supplied watchdog summary; resume --reviewed-head <full-sha> without rewriting history"}, nil
	}
	main, err := primaryWorktree(root)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	target := item.TargetSnapshot
	history := LedgerHistory{}
	if item.Branch != "" {
		head, headErr := git(root, "rev-parse", "refs/heads/"+item.Branch)
		if headErr != nil {
			head, headErr = git(root, "rev-parse", "refs/remotes/"+remote+"/"+item.Branch)
		}
		if headErr != nil {
			return ImplementationOutcome{Status: "fix_required", Reason: "branch unavailable; fetch and create the conventional worktree before resuming"}, nil
		}
		history, err = InspectLedger(root, head, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if len(history.Violations) != 0 {
			return ImplementationOutcome{Status: "fix_required", Reason: fmt.Sprint(history.Violations) + "; repair ledger history and resume"}, nil
		}
	}
	facts := skilldist.ImplementationFacts{Branch: item.Branch, Worktree: filepath.Join(main, ".worktrees", item.Branch), TargetSnapshot: target, ArtifactBaseline: history.Baseline, ArtifactCompletion: history.Completion}
	if item.Submission != nil {
		facts.PreviousReviewedHead, facts.Comments = item.Submission.PreviousReviewedHead, item.Submission.Comments
	}
	if item.State == Rework && !item.Synchronization {
		facts.TargetSnapshot = ""
	}
	if item.Synchronization {
		facts.PreviousReviewedHead = ""
	}
	facts.ResultDirectory = directory
	facts.Remote = remote
	return ImplementationOutcome{Status: "work_available", Item: &item, Facts: &skilldist.InvocationFacts{Implementation: &facts}}, nil
}
