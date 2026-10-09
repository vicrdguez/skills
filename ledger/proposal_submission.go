package ledger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// proposalBase is the branch a Proposal Submission merges into.
const proposalBase = "main"

// ProposalForge presents and observes a Proposal Submission through the same
// pull request seams Slices use.
type ProposalForge interface {
	CompletionForge
	DeliveryForge
}

// ProposalRefresh is the immediate outcome of one Proposal Submission
// refresh. Only a newly attached Submission or an observed completion is
// persisted; a presentation outcome or an unmerged closure is reported only.
type ProposalRefresh struct {
	Proposal       string           `json:"proposal"`
	ClosedUnmerged bool             `json:"closed_unmerged,omitempty"`
	Presentation   *PublicationNote `json:"presentation,omitempty"`
	Commit         string           `json:"ledger_commit,omitempty"`
	Replication    *PublicationNote `json:"replication,omitempty"`
}

// ProposalSlice is one member Slice of a Proposal with its recorded state.
type ProposalSlice struct {
	Name  string
	State SliceState
}

// RenderProposalBody renders the Proposal Submission body from recorded
// facts: the Proposal description, each Slice with its lifecycle, where it
// merged and its pull request, and a Closes line per Slice issue. It is a
// view, never a record.
func RenderProposalBody(description string, slices []ProposalSlice) string {
	var body strings.Builder
	body.WriteString(strings.TrimSpace(description))
	body.WriteString("\n\n## Slices\n\n")
	var closes []string
	for _, slice := range slices {
		state := slice.State
		label, known := LifecycleLabels[state.State]
		if !known {
			label = state.State
		}
		title := state.Title
		if title == "" {
			title = slice.Name
		}
		fmt.Fprintf(&body, "- %s (`%s`): %s", title, slice.Name, label)
		if state.State == Merged && state.Completion != nil {
			fmt.Fprintf(&body, " into `%s`", state.Completion.Target.Branch)
		}
		if state.Submission != nil {
			fmt.Fprintf(&body, ", %s#%d", state.Submission.Repository, state.Submission.Number)
		}
		body.WriteString("\n")
		if state.Issue != nil {
			closes = append(closes, fmt.Sprintf("Closes #%d", state.Issue.Number))
		}
	}
	if len(closes) > 0 {
		body.WriteString("\n" + strings.Join(closes, "\n") + "\n")
	}
	return body.String()
}

// proposalRecord is one active Proposal's committed record at one revision.
type proposalRecord struct {
	path        string
	meta        ProposalMeta
	description string
	slices      []ProposalSlice
}

func (s *Store) readProposalAt(head, project, proposal string) (proposalRecord, bool, error) {
	directory := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals", proposal))
	read := proposalRecord{path: directory + "/proposal.json"}
	if !gitOK(s.Root, "cat-file", "-e", head+":"+read.path) {
		return read, false, nil
	}
	if err := readJSONAt(s, head, read.path, &read.meta); err != nil {
		return read, true, refuse("proposal.json of proposals/"+proposal+" is unreadable", "repair the damaged record with human direction")
	}
	description, err := showPath(s, head, directory+"/proposal.md")
	if err != nil {
		return read, true, refuse("proposal.md of proposals/"+proposal+" is unavailable", "restore the accepted record with human direction")
	}
	read.description = description
	children, err := s.proposalChildren(head, directory)
	if err != nil {
		return read, true, err
	}
	for _, name := range children {
		var state SliceState
		if err := readJSONAt(s, head, directory+"/"+name+"/state.json", &state); err != nil {
			return read, true, refuse("slice "+name+" of proposal "+proposal+" is unreadable", "repair the damaged record with human direction")
		}
		read.slices = append(read.slices, ProposalSlice{Name: name, State: state})
	}
	return read, true, nil
}

// title is the Proposal's parent title, or its sole Slice's title.
func (r proposalRecord) title(proposal string) string {
	if title := strings.TrimSpace(r.meta.ParentTitle); title != "" {
		return title
	}
	if len(r.slices) == 1 && r.slices[0].State.Title != "" {
		return r.slices[0].State.Title
	}
	return proposal
}

func (r proposalRecord) mergedIntoBranch() bool {
	for _, slice := range r.slices {
		if slice.State.State == Merged && slice.State.Completion != nil && slice.State.Completion.Target.Branch == r.meta.Branch {
			return true
		}
	}
	return false
}

func (r proposalRecord) allMerged() bool {
	for _, slice := range r.slices {
		if slice.State.State != Merged {
			return false
		}
	}
	return len(r.slices) > 0
}

// RefreshProposal observes an open Proposal's Submission and keeps it
// presented. sliceMerged reports a Slice of this Proposal observed Merged by
// the same completion observation. A merged Submission records the
// Proposal's completion into main; an unmerged closure is reported and
// recorded nowhere. Presentation runs after a Slice merge into the branch, or
// while no Submission is attached yet so a failed first presentation is
// retried; its failure is reported, never returned. The forge is requested
// only when there is something to observe or present.
func RefreshProposal(ctx context.Context, s *Store, repository github.RepositoryID, proposal string, sliceMerged bool, forge func() (ProposalForge, error)) (*ProposalRefresh, error) {
	head, err := s.head()
	if err != nil {
		return nil, err
	}
	read, found, err := s.readProposalAt(head, repository.Name, proposal)
	if err != nil || !found || !proposalOpen(read.meta) {
		return nil, err
	}
	present := read.mergedIntoBranch() && (sliceMerged || read.meta.Submission == nil)
	if read.meta.Submission == nil && !present {
		return nil, nil
	}
	result := &ProposalRefresh{Proposal: proposal}
	f, err := forge()
	if err != nil {
		return result, err
	}
	identity := repository.Owner + "/" + repository.Name
	base := IntegrationTarget{Repository: identity, Branch: proposalBase}
	attached := read.meta.Submission
	if attached != nil {
		observation, err := f.ObserveSubmission(ctx, *attached, base, read.meta.Branch)
		if err != nil {
			return result, err
		}
		switch observation.State {
		case Merged:
			evidence := TerminalEvidence{Submission: *attached, Target: base, SourceHead: observation.SourceHead, MergeCommit: observation.MergeCommit}
			s.observeDeliveryUpstream(ctx)
			if err := ctx.Err(); err != nil {
				return result, err
			}
			result.Commit, err = s.recordProposal(repository, proposal, attached, func(meta *ProposalMeta) { meta.Completion = &evidence }, "observe proposal completion")
			if err == nil {
				note, _ := s.push(result.Commit)
				result.Replication = &note
			}
			return result, err
		case "closed":
			result.ClosedUnmerged = true
			return result, nil
		case "open":
		default:
			return result, refuse("Proposal Submission returned no confirmed outcome", "inspect the exact pull request and observe again")
		}
	}
	if !present {
		return result, nil
	}
	number := 0
	if attached != nil {
		number = attached.Number
	}
	current := func() error {
		now, err := s.head()
		if err != nil {
			return err
		}
		latest, _, err := s.readProposalAt(now, repository.Name, proposal)
		if err != nil {
			return err
		}
		if !proposalOpen(latest.meta) || !sameAttachment(latest.meta.Submission, attached) {
			return refuse("Proposal "+proposal+" changed since its Submission was rendered", "observe completions again to present the current view")
		}
		return nil
	}
	approved := read.allMerged()
	presented, err := f.PresentPull(ctx, PullPresentation{
		Number: number, Title: read.title(proposal), Body: RenderProposalBody(read.description, read.slices),
		Branch: read.meta.Branch, Base: proposalBase, Approved: approved, Unpinned: true, Current: current,
	})
	switch {
	case errors.Is(err, ErrPresentationUncertain):
		result.Presentation = &PublicationNote{Status: IssueUncertain, Detail: err.Error()}
		return result, nil
	case err != nil:
		result.Presentation = &PublicationNote{Status: IssuePending, Detail: err.Error()}
		return result, nil
	case presented <= 0:
		result.Presentation = &PublicationNote{Status: IssuePending, Detail: "forge returned no valid Proposal Submission attachment"}
		return result, nil
	}
	readiness := "draft"
	if approved {
		readiness = "ready for review"
	}
	result.Presentation = &PublicationNote{Status: PullPresented, Detail: fmt.Sprintf("pull request #%d presents Proposal Branch %s as %s", presented, read.meta.Branch, readiness)}
	if attached == nil {
		submission := &ForgeAttachment{Repository: identity, Number: presented}
		result.Commit, err = s.recordProposal(repository, proposal, nil, func(meta *ProposalMeta) { meta.Submission = submission }, "record proposal submission")
		if err != nil {
			result.Presentation.Detail += fmt.Sprintf(", but recording it as the Proposal Submission is pending: %v", err)
			return result, nil
		}
		note, _ := s.push(result.Commit)
		result.Replication = &note
	}
	return result, nil
}

// recordProposal applies one Proposal Submission fact to the current record
// while its branch is still open and its attachment is the one observed.
func (s *Store) recordProposal(repository github.RepositoryID, proposal string, attached *ForgeAttachment, apply func(*ProposalMeta), message string) (string, error) {
	var committed string
	err := s.withMutation(func() error {
		if err := s.requireReconciled(); err != nil {
			return err
		}
		head, err := s.head()
		if err != nil {
			return err
		}
		read, found, err := s.readProposalAt(head, repository.Name, proposal)
		if err != nil {
			return err
		}
		if !found || !proposalOpen(read.meta) || !sameAttachment(read.meta.Submission, attached) {
			return refuse("Proposal "+proposal+" changed during Submission observation", "observe completions again")
		}
		if err := s.requireCleanPaths(read.path); err != nil {
			return err
		}
		apply(&read.meta)
		if err := writeJSON(filepath.Join(s.Root, read.path), read.meta); err != nil {
			return err
		}
		if err := s.commit(message+" "+repository.Name+"/"+proposal, read.path); err != nil {
			return err
		}
		committed, err = s.head()
		return err
	})
	return committed, err
}
