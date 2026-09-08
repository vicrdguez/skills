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

func StartWatchdog(ctx context.Context, root, remote string, number int, backend ImplementationBackend) (ImplementationOutcome, error) {
	remote, err := github.ResolveGitHubRemote(root, remote)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	repository, items, err := loadImplementation(ctx, root, remote, backend)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	items = slices.DeleteFunc(items, func(item ImplementationItem) bool { return item.Submission == nil })
	slices.SortFunc(items, func(a, b ImplementationItem) int {
		if order := cmp.Compare(a.Submission.CreatedAt, b.Submission.CreatedAt); order != 0 {
			return order
		}
		return cmp.Compare(a.Number, b.Number)
	})
	for _, item := range items {
		if number != 0 && item.Number != number {
			continue
		}
		if item.State != AwaitingReview || number == 0 && item.Claimed || number != 0 && !item.Claimed || item.Problem != "" {
			continue
		}
		if item.Submission.ReviewedHead != "" && item.Submission.ReviewedHead != item.Submission.Head && item.Claimed {
			return ImplementationOutcome{Status: "fix_required", Reason: "Submission moved after Claim; restore the fixed reviewed head before resuming"}, nil
		}
		history, err := InspectLedger(root, item.Submission.Head, item.Branch)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		if history.Phase != "retired" || len(history.Violations) != 0 {
			return ImplementationOutcome{Status: "fix_required", Reason: fmt.Sprint(history.Violations) + "; fetch and restore retired ledger history"}, nil
		}
		submission := *item.Submission
		submission.ReviewedHead = submission.Head
		item.Submission = &submission
		if err := backend.ClaimImplementation(ctx, repository, item); err != nil {
			return ImplementationOutcome{}, err
		}
		observed, err := backend.ImplementationItems(ctx, repository)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		for _, current := range observed {
			if current.Number != item.Number || !current.Claimed || current.State != AwaitingReview || current.Problem != "" || current.Submission == nil || current.Submission.Head != submission.Head || current.Submission.ReviewedHead != submission.Head {
				continue
			}
			facts := skilldist.WatchdogFacts{WorkItem: item.Number, Submission: submission.Number, Branch: item.Branch, ReviewedHead: submission.Head, ArtifactBaseline: history.Baseline, ArtifactCompletion: history.Completion, AuditBody: submission.Body, Comments: submission.Comments}
			facts.BaselineFiles, err = ledgerFiles(root, history.Baseline, ".changes/"+item.Branch)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			facts.CompletionFiles, err = ledgerFiles(root, history.Completion, ".changes/"+item.Branch)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			facts.Bounces = submission.Bounces
			if port, ok := backend.(ReviewBackend); ok {
				observed, err := port.ReviewSubmission(ctx, repository, submission.Number)
				if err != nil {
					return ImplementationOutcome{}, err
				}
				if observed.Head != submission.Head {
					return ImplementationOutcome{}, Refuse("Submission changed during packet construction")
				}
				facts.Bounces = observed.Bounces
			}
			main, err := primaryWorktree(root)
			if err != nil {
				return ImplementationOutcome{}, err
			}
			facts.Worktree = filepath.Join(main, ".worktrees", item.Branch)
			facts.Remote = remote
			facts.ResultDirectory, err = os.MkdirTemp("", "skl-watchdog-")
			if err != nil {
				return ImplementationOutcome{}, err
			}
			if err := os.WriteFile(filepath.Join(facts.ResultDirectory, ".skl-result"), []byte("skl.watchdog/v1\n"), 0600); err != nil {
				os.RemoveAll(facts.ResultDirectory)
				return ImplementationOutcome{}, err
			}
			quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
			facts.ResumeCommand = fmt.Sprintf("skl watchdog resume --repo %s --remote %s --item %d", quote(facts.Worktree), quote(remote), item.Number)
			facts.SubmitCommand = fmt.Sprintf("skl watchdog submit --repo %s --remote %s --item %d --reviewed-head %s --summary %s", quote(facts.Worktree), quote(remote), item.Number, submission.Head, quote(filepath.Join(facts.ResultDirectory, "summary.md")))
			packet, err := skilldist.BuildPacket("watchdog", skilldist.InvocationFacts{Watchdog: &facts})
			if err != nil {
				os.RemoveAll(facts.ResultDirectory)
			}
			return ImplementationOutcome{Status: "work_available", Item: &current, Packet: &packet}, err
		}
		return ImplementationOutcome{Status: "fix_required", Reason: "Watchdog Claim changed; inspect and explicitly resume"}, nil
	}
	if number != 0 {
		return ImplementationOutcome{Status: "fix_required", Reason: "explicit Work Item is not an unambiguous Awaiting Review Claim"}, nil
	}
	return ImplementationOutcome{Status: "no_work"}, nil
}
