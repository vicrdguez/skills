package setup

import (
	skilldist "github.com/vicrdguez/skills"
	"github.com/vicrdguez/skills/workflow"
)

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
			Head: s.Head, Base: s.Base, Body: s.Body, Author: s.Author, Association: s.Association, Draft: s.Draft, Comments: s.Comments,
		}
		if item.Submission.ID != "" {
			output.Submission.Number, err = githubIssueNumber(workflow.WorkItemID(item.Submission.ID))
		}
	}
	return output, err
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
