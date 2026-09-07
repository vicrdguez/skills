package workflow

import (
	"cmp"
	"context"
	"fmt"
	skilldist "github.com/vicrdguez/skills"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type State string

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
	Problem        string
	ResumeState    State
	Submission     *Submission
	Branch         string
	TargetSnapshot string
	Number         int
	State          State
	CreatedAt      string
	Claimed        bool
	Blockers       []int
}

type Submission struct {
	State                State
	Claimed              bool
	Number               int
	Head                 string
	Base                 string
	Body                 string
	Draft                bool
	PreviousReviewedHead string
	Comments             []skilldist.ReviewComment
}

type ImplementationBackend interface {
	ImplementationItems(context.Context, RepositoryID) ([]ImplementationItem, error)
	ClaimImplementation(context.Context, RepositoryID, ImplementationItem) error
	ImplementationHead(context.Context, RepositoryID, string) (string, error)
	PublishImplementation(context.Context, RepositoryID, ImplementationItem, Submission) (Submission, error)
	AwaitImplementationReview(context.Context, RepositoryID, ImplementationItem) error
	PauseImplementation(context.Context, RepositoryID, ImplementationItem, string) error
}

type ImplementationOutcome struct {
	Ledger *LedgerHistory      `json:"ledger,omitempty"`
	Head   string              `json:"head,omitempty"`
	Packet *skilldist.Packet   `json:"packet,omitempty"`
	Status string              `json:"status"`
	Reason string              `json:"reason,omitempty"`
	Item   *ImplementationItem `json:"item,omitempty"`
}

func InspectImplementation(ctx context.Context, root string, number int, backend ImplementationBackend) (ImplementationOutcome, error) {
	remote, err := git(root, "remote", "get-url", "origin")
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, err := ParseGitHubRemote(remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	items, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	for _, item := range items {
		if item.Number != number {
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

func StartImplementation(ctx context.Context, root string, number int, backend ImplementationBackend) (ImplementationOutcome, error) {
	remote, err := git(root, "remote", "get-url", "origin")
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, err := ParseGitHubRemote(remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	items, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	if number != 0 {
		if number == -1 {
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
					if number != -1 {
						return ImplementationOutcome{Status: "fix_required", Reason: "worktree identity is ambiguous; resume with --item after repairing attachments"}, nil
					}
					number = item.Number
				}
			}
		}
		for _, item := range items {
			if item.Number == number && item.Claimed && (item.State == Ready || item.State == Rework) {
				return implementationPacket(root, item)
			}
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "explicit Work Item is not an unambiguous implementation Claim; repair its projections before resuming"}, nil
	}
	merged := make(map[int]bool)
	for _, item := range items {
		merged[item.Number] = item.State == Merged
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
		return cmp.Compare(a.Number, b.Number)
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
		if item.State == Ready {
			item.TargetSnapshot, err = git(root, "rev-parse", "refs/remotes/origin/main")
			if err != nil {
				return ImplementationOutcome{Status: "fix_required", Reason: "target unavailable; fetch origin/main and retry"}, nil
			}
		}
		claimErr := backend.ClaimImplementation(ctx, repository, item)
		observed, err := backend.ImplementationItems(ctx, repository)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		for _, current := range observed {
			if current.Number == item.Number && current.Claimed && current.State == item.State {
				return implementationPacket(root, current)
			}
		}
		if claimErr != nil {
			return ImplementationOutcome{}, claimErr
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "Claim read-back contradicts selected state; repair the Work Item projections and explicitly resume"}, nil
	}
	return ImplementationOutcome{Status: "no_work"}, nil
}

func implementationPacket(root string, item ImplementationItem) (ImplementationOutcome, error) {
	main, err := primaryWorktree(root)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	target := item.TargetSnapshot
	if target == "" && item.State == Ready {
		target, err = git(root, "rev-parse", "refs/remotes/origin/main")
		if err != nil {
			return ImplementationOutcome{Status: "fix_required", Reason: "target unavailable; fetch origin/main and resume"}, nil
		}
	}
	history := LedgerHistory{}
	if item.Branch != "" {
		head, headErr := git(root, "rev-parse", "refs/heads/"+item.Branch)
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
	facts := skilldist.ImplementationFacts{WorkItem: item.Number, Branch: item.Branch, Worktree: filepath.Join(main, ".worktrees", item.Branch), TargetSnapshot: target, ArtifactBaseline: history.Baseline, ArtifactCompletion: history.Completion,
		ResumeCommand: fmt.Sprintf("skl implement resume --item %d --target-snapshot %s", item.Number, target)}
	if item.Submission != nil {
		facts.Submission, facts.PreviousReviewedHead, facts.Comments = item.Submission.Number, item.Submission.PreviousReviewedHead, item.Submission.Comments
		facts.TargetSnapshot = ""
		facts.ResumeCommand = fmt.Sprintf("skl implement resume --item %d", item.Number)
	}
	facts.ResultDirectory, err = os.MkdirTemp("", "skl-implement-")
	if err != nil {
		return ImplementationOutcome{}, err
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	facts.SubmitCommand = fmt.Sprintf("skl implement submit --repo %s --item %d --body %s", quote(facts.Worktree), item.Number, quote(filepath.Join(facts.ResultDirectory, "submission.md")))
	packet, err := skilldist.BuildPacket("implement", skilldist.InvocationFacts{Implementation: &facts})
	if err != nil {
		os.RemoveAll(facts.ResultDirectory)
		return ImplementationOutcome{}, err
	}
	if err := os.WriteFile(filepath.Join(facts.ResultDirectory, ".skl-result"), []byte("skl.implement/v1\n"), 0600); err != nil {
		os.RemoveAll(facts.ResultDirectory)
		return ImplementationOutcome{}, err
	}
	return ImplementationOutcome{Status: "work_available", Item: &item, Packet: &packet}, err
}
