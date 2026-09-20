package setup

import (
	"fmt"
	"os"
	"path/filepath"

	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/github"
	"github.com/vicrdguez/skills/workflow"
)

// InvocationContext is the presentation-only metadata the CLI established
// before the engine operation ran. It never carries workflow authority.
type InvocationContext struct {
	Repository github.RepositoryID
	Capability skilldist.ExecutionCapability
}

// These projections retain the numeric GitHub CLI contract, not engine identity.
type ImplementationOutput struct {
	workflow.ImplementationOutcome
	Packet   *skilldist.Packet         `json:"packet,omitempty"`
	Item     *implementationItemOutput `json:"item,omitempty"`
	Guidance *ImplementationGuidance   `json:"guidance,omitempty"`
}

// ImplementationGuidance carries the same claim certainty and applicable next
// action as the default Markdown report without making prose authoritative for
// workflow success.
type ImplementationGuidance struct {
	Claim       string `json:"claim"`
	Explanation string `json:"explanation"`
	Recovery    string `json:"recovery,omitempty"`
	NextStep    string `json:"next_step,omitempty"`
}

type implementationItemOutput struct {
	Synchronization bool
	Problem         string
	Submission      *submissionOutput
	EvidenceSources []skilldist.EvidenceSource
	Branch          string
	Number          int
	State           workflow.State
	CreatedAt       string
	Claimed         bool
	Blockers        []int
}

type submissionOutput struct {
	PendingReview   workflow.State
	Merged          bool
	Mergeability    string
	CreatedAt       string
	State           workflow.State
	Claimed         bool
	Number          int
	Head            string
	Base            string
	Body            string
	Author          string
	Association     string
	Draft           bool
	Comments        []skilldist.ReviewComment
	EvidenceSources []skilldist.EvidenceSource
}

type StatusOutput struct {
	workflow.StatusOutcome
	CompleteProposals []int                      `json:"complete_proposals,omitempty"`
	Items             []implementationItemOutput `json:"items"`
}

func presentItem(item workflow.ImplementationItem) (implementationItemOutput, error) {
	output := implementationItemOutput{
		Synchronization: item.Synchronization, Problem: item.Problem,
		EvidenceSources: item.EvidenceSources,
		Branch:          item.Branch,
		State:           item.State, CreatedAt: item.CreatedAt, Claimed: item.Claimed,
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
			Head: s.Head, Base: s.Base, Body: s.Body, Author: s.Author, Association: s.Association, Draft: s.Draft, Comments: s.Comments, EvidenceSources: s.EvidenceSources,
		}
		if item.Submission.ID != "" {
			output.Submission.Number, err = githubIssueNumber(workflow.WorkItemID(item.Submission.ID))
		}
	}
	return output, err
}

// requiredEvidenceStreams is the complete set of repository-bound sources one
// Implement execution may need: the attached Submission's own body and its three
// discussion streams, plus the source Work Item's comments. A stream the
// invocation never observed keeps its retrieval command, so `pending` is never
// reported as `fetched empty`.
func evidenceStreams(item *implementationItemOutput, f skilldist.ImplementationFacts) []skilldist.EvidenceStream {
	observed := map[skilldist.EvidenceSource]bool{}
	for _, source := range item.EvidenceSources {
		observed[source] = true
	}
	if item.Submission != nil {
		for _, source := range item.Submission.EvidenceSources {
			observed[source] = true
		}
	}
	counts := map[skilldist.EvidenceSource]int{}
	for _, comment := range f.Comments {
		if comment.Source == "" {
			continue
		}
		counts[comment.Source]++
	}
	type requiredEvidence struct {
		source  skilldist.EvidenceSource
		command string
	}
	required := []requiredEvidence{
		{skilldist.IssueCommentsEvidenceSource(f.Repository, f.WorkItem), fmt.Sprintf("gh api --paginate repos/%s/issues/%d/comments", f.Repository, f.WorkItem)},
	}
	if f.Submission != 0 {
		required = append(required,
			requiredEvidence{skilldist.PullEvidenceSource(f.Repository, f.Submission), fmt.Sprintf("gh api repos/%s/pulls/%d", f.Repository, f.Submission)},
			requiredEvidence{skilldist.PullDiscussionEvidenceSource(f.Repository, f.Submission), fmt.Sprintf("gh api --paginate repos/%s/issues/%d/comments", f.Repository, f.Submission)},
			requiredEvidence{skilldist.PullReviewsEvidenceSource(f.Repository, f.Submission), fmt.Sprintf("gh api --paginate repos/%s/pulls/%d/reviews", f.Repository, f.Submission)},
			requiredEvidence{skilldist.PullCommentsEvidenceSource(f.Repository, f.Submission), fmt.Sprintf("gh api --paginate repos/%s/pulls/%d/comments", f.Repository, f.Submission)},
		)
	}
	var streams []skilldist.EvidenceStream
	for _, stream := range required {
		if !observed[stream.source] {
			streams = append(streams, skilldist.EvidenceStream{Source: stream.source, Command: stream.command})
			continue
		}
		bodies := counts[stream.source]
		if f.SubmissionBody != nil && f.SubmissionBody.Source == stream.source {
			bodies++
		}
		streams = append(streams, skilldist.EvidenceStream{Source: stream.source, Bodies: bodies})
	}
	return streams
}

// primary returns the main worktree the conventional worktree is attached to.
func primary(worktree string) string {
	return filepath.Dir(filepath.Dir(worktree))
}

func PresentImplementation(outcome workflow.ImplementationOutcome, invocation InvocationContext) (ImplementationOutput, error) {
	output := ImplementationOutput{ImplementationOutcome: outcome}
	if outcome.Item != nil {
		item, err := presentItem(*outcome.Item)
		if err != nil {
			return output, err
		}
		output.Item = &item
	}
	output.Guidance = implementationGuidance(output)
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
		f.Repository = invocation.Repository.Owner + "/" + invocation.Repository.Name
		f.Capability = invocation.Capability
		// The engine established the procedure from authoritative Workflow State;
		// presentation never reselects it from the attached records.
		if f.Procedure == "" {
			f.Procedure = skilldist.InitialSubmission
		}
		if submission := output.Item.Submission; submission != nil {
			f.Submission = submission.Number
			f.SubmissionBody = &skilldist.SubmissionEvidence{
				Source:      skilldist.PullEvidenceSource(f.Repository, submission.Number),
				Author:      submission.Author,
				Association: submission.Association,
				CreatedAt:   submission.CreatedAt,
				Body:        submission.Body,
			}
		}
		f.EvidenceStreams = evidenceStreams(output.Item, f)
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
		f.PushCommand = fmt.Sprintf("git -C %s push %s %s", quote(f.Worktree), quote(f.Remote), quote(f.Branch))
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
		f.ResumeCommand = fmt.Sprintf("skl watchdog resume --repo %s --remote %s --item %d", quote(f.Worktree), quote(f.Remote), f.WorkItem)
		f.SubmitCommand = fmt.Sprintf("skl watchdog submit --repo %s --remote %s --item %d --review-number %d --reviewed-head %s --submission %d --base %s --submission-body-sha256 %s --summary %s", quote(f.Worktree), quote(f.Remote), f.WorkItem, f.ReviewNumber, f.ReviewedHead, f.Submission, quote(f.SubmissionBase), quote(f.SubmissionBodySHA256), quote(filepath.Join(directory, "summary.md")))
		flags := endpointFlags(f.SuppliedArtifactBaseline, f.SuppliedArtifactCompletion)
		f.ResumeCommand += flags
		f.SubmitCommand += flags
		f.FetchCommand = fmt.Sprintf("git -C %s fetch %s %s", quote(primary(f.Worktree)), quote(f.Remote), quote("+refs/heads/"+f.Branch+":refs/remotes/"+f.Remote+"/"+f.Branch))
		f.WorktreeCommand = fmt.Sprintf("git -C %s worktree add -b %s %s %s", quote(primary(f.Worktree)), quote(f.Branch), quote(f.Worktree), quote(f.Remote+"/"+f.Branch))
		f.InspectCommand = fmt.Sprintf("skl watchdog inspect --repo %s --remote %s --item %d --submission %d --base %s --submission-body-sha256 %s --review-number %d --reviewed-head %s --result-directory %s", quote(f.Worktree), quote(f.Remote), f.WorkItem, f.Submission, quote(f.SubmissionBase), quote(f.SubmissionBodySHA256), f.ReviewNumber, quote(f.ReviewedHead), quote(f.ResultDirectory))
		if f.PreviousReviewedHead != "" {
			f.InspectCommand += " --previous-reviewed-head " + quote(f.PreviousReviewedHead)
		}
		f.InspectCommand += flags
		if f.Repository == "" {
			if selected, err := ResolveRepository(primary(f.Worktree), f.Remote); err == nil {
				f.Repository = selected.Repository.Owner + "/" + selected.Repository.Name
			}
		}
		f.EvidenceInstructions = watchdogEvidenceInstructions(f)
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
