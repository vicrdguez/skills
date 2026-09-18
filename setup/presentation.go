package setup

import (
	"fmt"
	"os"
	"path/filepath"

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
	Submission      *submissionOutput
	Branch          string
	Number          int
	State           workflow.State
	CreatedAt       string
	Claimed         bool
	Blockers        []int
}

type submissionOutput struct {
	PendingReview workflow.State
	Merged        bool
	Mergeability  string
	CreatedAt     string
	State         workflow.State
	Claimed       bool
	Number        int
	Head          string
	Base          string
	Body          string
	Draft         bool
	Comments      []skilldist.ReviewComment
}

type StatusOutput struct {
	workflow.StatusOutcome
	CompleteProposals []int                      `json:"complete_proposals,omitempty"`
	Items             []implementationItemOutput `json:"items"`
}

func presentItem(item workflow.ImplementationItem) (implementationItemOutput, error) {
	output := implementationItemOutput{
		Synchronization: item.Synchronization, Problem: item.Problem,
		Branch: item.Branch,
		State:  item.State, CreatedAt: item.CreatedAt, Claimed: item.Claimed,
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
			PendingReview: s.PendingReview,
			Merged:        s.Merged, Mergeability: s.Mergeability,
			CreatedAt: s.CreatedAt, State: s.State, Claimed: s.Claimed,
			Head: s.Head, Base: s.Base, Body: s.Body, Draft: s.Draft, Comments: s.Comments,
		}
		if item.Submission.ID != "" {
			output.Submission.Number, err = githubIssueNumber(workflow.WorkItemID(item.Submission.ID))
		}
	}
	return output, err
}

// primary returns the main worktree the conventional worktree is attached to.
func primary(worktree string) string {
	return filepath.Dir(filepath.Dir(worktree))
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
	quote := skilldist.ShellQuote
	endpointFlags := func(baseline, completion string) string {
		var flags string
		if baseline != "" {
			flags += " --artifact-baseline " + baseline
		}
		if completion != "" {
			flags += " --artifact-completion " + completion
		}
		return flags
	}
	var skill, directory string
	if source := facts.Implementation; source != nil {
		f := *source
		facts.Implementation = &f
		skill, directory = "implement", f.ResultDirectory
		f.WorkItem = output.Item.Number
		f.WorkItemReference = fmt.Sprintf("#%d", f.WorkItem)
		// The reconciled Workflow State, not the presence of a preserved draft
		// Submission, establishes which procedure this invocation follows.
		f.Procedure = skilldist.InitialSubmission
		if output.Item.State == workflow.Rework {
			f.Procedure = skilldist.FindingDrivenRework
		}
		if output.Item.Submission != nil {
			f.Submission = output.Item.Submission.Number
		}
		f.ResumeCommand = fmt.Sprintf("skl implement resume --item %d", f.WorkItem)
		f.ResumeCommand += " --remote " + quote(f.Remote)
		flags := endpointFlags(f.SuppliedArtifactBaseline, f.SuppliedArtifactCompletion)
		f.ResumeCommand += flags
		if outcome.Status == "fix_required" {
			output.Reason += "; resume with `" + f.ResumeCommand + "`"
			return output, nil
		}
		f.FetchCommand = fmt.Sprintf("git -C %s fetch %s %s", quote(primary(f.Worktree)), quote(f.Remote), quote("+refs/heads/"+f.Branch+":refs/remotes/"+f.Remote+"/"+f.Branch))
		f.WorktreeCommand = fmt.Sprintf("git -C %s worktree add -b %s %s %s", quote(primary(f.Worktree)), quote(f.Branch), quote(f.Worktree), quote(f.Remote+"/"+f.Branch))
		f.InspectCommand = fmt.Sprintf("skl implement inspect --repo %s --remote %s --item %d", quote(f.Worktree), quote(f.Remote), f.WorkItem)
		f.SubmitCommand = fmt.Sprintf("skl implement submit --repo %s --remote %s --item %d --body %s", quote(f.Worktree), quote(f.Remote), f.WorkItem, quote(filepath.Join(directory, "submission.md")))
		f.NeedsHumanCommand = fmt.Sprintf("skl implement needs-human --repo %s --remote %s --item %d --reason <permitted-reason> --decision %s", quote(f.Worktree), quote(f.Remote), f.WorkItem, quote(filepath.Join(directory, "decision.md")))
		f.InspectCommand += flags
		f.SubmitCommand += flags
		f.NeedsHumanCommand += flags
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
		f.SubmitCommand = fmt.Sprintf("skl watchdog submit --repo %s --remote %s --item %d --review-number %d --reviewed-head %s --summary %s", quote(f.Worktree), quote(f.Remote), f.WorkItem, f.ReviewNumber, f.ReviewedHead, quote(filepath.Join(directory, "summary.md")))
		flags := endpointFlags(f.SuppliedArtifactBaseline, f.SuppliedArtifactCompletion)
		f.ResumeCommand += flags
		f.SubmitCommand += flags
		f.FetchCommand = fmt.Sprintf("git -C %s fetch %s %s", quote(primary(f.Worktree)), quote(f.Remote), quote("+refs/heads/"+f.Branch+":refs/remotes/"+f.Remote+"/"+f.Branch))
		f.WorktreeCommand = fmt.Sprintf("git -C %s worktree add -b %s %s %s", quote(primary(f.Worktree)), quote(f.Branch), quote(f.Worktree), quote(f.Remote+"/"+f.Branch))
		f.InspectCommand = fmt.Sprintf("skl implement inspect --repo %s --remote %s --item %d", quote(f.Worktree), quote(f.Remote), f.WorkItem)
		f.InspectCommand += flags
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
