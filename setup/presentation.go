package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

// These projections retain the numeric GitHub CLI contract, not engine identity.
type ImplementationOutput struct {
	workflow.ImplementationOutcome
	Packet *skilldist.Packet         `json:"packet,omitempty"`
	Item   *implementationItemOutput `json:"item,omitempty"`
}

type implementationItemOutput struct {
	Synchronization bool
	Problem         string
	ResumeState     workflow.State
	Submission      *submissionOutput
	Branch          string
	TargetSnapshot  string
	TargetBranch    string
	Number          int
	State           workflow.State
	CreatedAt       string
	Claimed         bool
	Blockers        []int
	Transition      *workflow.ImplementationTransition
}

type submissionOutput struct {
	PendingReview        workflow.State
	Merged               bool
	Mergeability         string
	Bounces              int
	CreatedAt            string
	ReviewedHead         string
	State                workflow.State
	Claimed              bool
	Number               int
	Head                 string
	Base                 string
	Body                 string
	Draft                bool
	PreviousReviewedHead string
	Comments             []skilldist.ReviewComment
}

type StatusOutput struct {
	workflow.StatusOutcome
	CompleteProposals []int                      `json:"complete_proposals,omitempty"`
	Items             []implementationItemOutput `json:"items"`
}

func presentItem(item workflow.ImplementationItem) (implementationItemOutput, error) {
	output := implementationItemOutput{
		Synchronization: item.Synchronization, Problem: item.Problem, ResumeState: item.ResumeState,
		Branch: item.Branch, TargetSnapshot: item.TargetSnapshot, TargetBranch: item.TargetBranch,
		State: item.State, CreatedAt: item.CreatedAt, Claimed: item.Claimed, Transition: item.Transition,
	}
	var err error
	if item.ID != "" {
		output.Number, err = githubIssueNumber(item.ID)
		if err != nil {
			return output, err
		}
	}
	for _, id := range item.Blockers {
		number, err := githubIssueNumber(id)
		if err != nil {
			return output, err
		}
		output.Blockers = append(output.Blockers, number)
	}
	if item.Submission != nil {
		s := item.Submission
		output.Submission = &submissionOutput{
			PendingReview: s.PendingReview, Merged: s.Merged, Mergeability: s.Mergeability, Bounces: s.Bounces,
			CreatedAt: s.CreatedAt, ReviewedHead: s.ReviewedHead, State: s.State, Claimed: s.Claimed,
			Head: s.Head, Base: s.Base, Body: withClosingReference(s.Body, output.Number), Draft: s.Draft, PreviousReviewedHead: s.PreviousReviewedHead, Comments: s.Comments,
		}
		if item.Submission.ID != "" {
			output.Submission.Number, err = githubIssueNumber(workflow.WorkItemID(item.Submission.ID))
		}
	}
	return output, err
}

func PresentImplementation(outcome workflow.ImplementationOutcome) (ImplementationOutput, error) {
	output := ImplementationOutput{ImplementationOutcome: outcome}
	if outcome.Item != nil {
		item, err := presentItem(*outcome.Item)
		if err != nil {
			return output, err
		}
		output.Item = &item
	}
	if outcome.Facts == nil {
		return output, nil
	}
	if output.Item == nil {
		return output, fmt.Errorf("instruction facts require a Work Item")
	}
	facts := *outcome.Facts
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	var skill, directory string
	if source := facts.Implementation; source != nil {
		f := *source
		facts.Implementation = &f
		skill, directory = "implement", f.ResultDirectory
		f.WorkItem = output.Item.Number
		f.WorkItemReference = fmt.Sprintf("#%d", f.WorkItem)
		if output.Item.Submission != nil {
			f.Submission = output.Item.Submission.Number
		}
		f.ResumeCommand = fmt.Sprintf("skl implement resume --item %d --target-snapshot %s", f.WorkItem, f.TargetSnapshot)
		if outcome.Item.State == workflow.Rework && !outcome.Item.Synchronization {
			f.ResumeCommand = fmt.Sprintf("skl implement resume --item %d", f.WorkItem)
		}
		f.ResumeCommand += " --remote " + quote(f.Remote)
		f.InspectCommand = fmt.Sprintf("skl implement inspect --repo %s --remote %s --item %d", quote(f.Worktree), quote(f.Remote), f.WorkItem)
		f.SubmitCommand = fmt.Sprintf("skl implement submit --repo %s --remote %s --item %d --body %s", quote(f.Worktree), quote(f.Remote), f.WorkItem, quote(filepath.Join(directory, "submission.md")))
	} else if source := facts.Watchdog; source != nil {
		if output.Item.Submission == nil {
			return output, fmt.Errorf("Watchdog facts require a Submission")
		}
		f := *source
		facts.Watchdog = &f
		skill, directory = "watchdog", f.ResultDirectory
		f.WorkItem, f.Submission = output.Item.Number, output.Item.Submission.Number
		f.WorkItemReference, f.SubmissionReference = fmt.Sprintf("#%d", f.WorkItem), fmt.Sprintf("#%d", f.Submission)
		f.ResumeCommand = fmt.Sprintf("skl watchdog resume --repo %s --remote %s --item %d", quote(f.Worktree), quote(f.Remote), f.WorkItem)
		f.SubmitCommand = fmt.Sprintf("skl watchdog submit --repo %s --remote %s --item %d --reviewed-head %s --summary %s", quote(f.Worktree), quote(f.Remote), f.WorkItem, f.ReviewedHead, quote(filepath.Join(directory, "summary.md")))
	}
	packet, err := skilldist.BuildPacket(skill, facts)
	if err != nil {
		if directory != "" {
			os.RemoveAll(directory)
		}
		return output, err
	}
	output.Packet = &packet
	return output, nil
}

func PresentStatus(outcome workflow.StatusOutcome) (StatusOutput, error) {
	output := StatusOutput{StatusOutcome: outcome}
	for _, id := range outcome.CompleteProposals {
		number, err := githubIssueNumber(id)
		if err != nil {
			return output, err
		}
		output.CompleteProposals = append(output.CompleteProposals, number)
	}
	for _, item := range outcome.Items {
		presented, err := presentItem(item)
		if err != nil {
			return output, err
		}
		output.Items = append(output.Items, presented)
	}
	return output, nil
}
