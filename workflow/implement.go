package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Source          *LifecycleObservation
	Synchronization bool
	Problem         string
	Submission      *Submission
	Feedback        []skilldist.ReviewComment
	// EvidenceSources names the repository-bound feedback streams the last
	// observation actually read. A required stream missing from this list is
	// pending for the worker, not evidence of an empty review.
	EvidenceSources       []skilldist.EvidenceSource
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
	Branch          string
	Head            string
	Base            string
	Body            string
	Author          string
	Association     string
	Draft           bool
	Comments        []skilldist.ReviewComment
	// EvidenceSources names the repository-bound streams the last observation
	// actually read for this Submission. A required stream missing from this
	// list is pending for the worker, not evidence of an empty review.
	EvidenceSources  []skilldist.EvidenceSource
	EvidenceComments []skilldist.ReviewComment
	EvidenceStreams  []skilldist.EvidenceStream
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
	Ledger                    *LedgerHistory             `json:"ledger,omitempty"`
	Head                      string                     `json:"head,omitempty"`
	Facts                     *skilldist.InvocationFacts `json:"-"`
	Status                    string                     `json:"status"`
	Reason                    string                     `json:"reason,omitempty"`
	Item                      *ImplementationItem        `json:"item,omitempty"`
	ClaimAcquisitionUncertain bool                       `json:"claim_acquisition_uncertain,omitempty"`
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

func InspectImplementation(ctx context.Context, root, remote string, id WorkItemID, endpoints ArtifactEndpoints, backend ImplementationBackend) (ImplementationOutcome, error) {
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
		if err != nil {
			return ImplementationOutcome{}, err
		}
		main, err := primaryWorktree(root)
		if err != nil {
			return ImplementationOutcome{}, err
		}
		facts := skilldist.ImplementationFacts{Branch: item.Branch, Remote: remote, Worktree: filepath.Join(main, ".worktrees", item.Branch), SuppliedArtifactBaseline: endpoints.Baseline, SuppliedArtifactCompletion: endpoints.Completion,
			ArtifactBaseline: history.Baseline, ArtifactCompletion: history.Completion, Inspection: inspectionFacts(history, head)}
		if item.State == Rework {
			facts.Procedure = skilldist.FindingDrivenRework
		} else {
			facts.Procedure = skilldist.ResumedSubmission
		}
		return ImplementationOutcome{Status: "inspected", Item: &item, Head: head, Ledger: &history, Facts: &skilldist.InvocationFacts{Implementation: &facts}}, nil
	}
	return ImplementationOutcome{Status: "fix_required", Reason: "Work Item unavailable; supply its explicit stable --item identity"}, nil
}

// inspectionFacts narrows the observed ledger state to the continuation that
// applies. Unresolved violations come first: an `inspected` status with
// violations never authorizes completion.
func inspectionFacts(history LedgerHistory, head string) *skilldist.InspectionFacts {
	facts := &skilldist.InspectionFacts{Violations: history.Violations}
	switch {
	case len(history.Violations) != 0:
		facts.Progress = skilldist.LedgerViolations
	case history.Completion != "" && history.Phase == "retired":
		facts.Progress = skilldist.RetiredLedger
	case history.Completion != "":
		facts.Progress = skilldist.CompletionPresent
	case head == history.Baseline:
		facts.Progress = skilldist.BaselineOnly
	default:
		facts.Progress = skilldist.ProvisionalLedger
	}
	return facts
}

func StartImplementation(ctx context.Context, root, remote string, id WorkItemID, endpoints ArtifactEndpoints, backend ImplementationBackend) (ImplementationOutcome, error) {
	selection, ok := backend.(SelectionBackend)
	if !ok {
		return ImplementationOutcome{}, errors.New("workflow backend does not support candidate selection")
	}
	if id != "" {
		return resumeImplementation(ctx, root, remote, id, endpoints, selection)
	}
	candidate, found, err := selectQueue(ctx, selection, ReworkQueue)
	if err != nil {
		return ImplementationOutcome{}, fmt.Errorf("implementation queue observation failed before Claim acquisition; repair backend observation and retry `skl implement next`: %w", err)
	}
	if !found {
		candidate, found, err = selectQueue(ctx, selection, ReadyQueue)
		if err != nil {
			return ImplementationOutcome{}, fmt.Errorf("implementation queue observation failed before Claim acquisition; repair backend observation and retry `skl implement next`: %w", err)
		}
	}
	if !found {
		return ImplementationOutcome{Status: "no_work"}, nil
	}
	item, outcome, err := selectedImplementation(ctx, selection, candidate)
	if err != nil {
		return ImplementationOutcome{}, fmt.Errorf("selected Work Item observation failed before Claim acquisition; repair backend observation and retry `skl implement next`: %w", err)
	}
	if outcome.Status != "" {
		return outcome, nil
	}
	if item.State != Ready && item.State != Rework {
		return implementationRefusal(item, "the selected Work Item is not an eligible implementation record; repair its projections"), nil
	}
	if item.Claimed {
		return implementationRefusal(item, "the selected Work Item was claimed before acquisition; resume with `skl implement resume --item "+string(item.ID)+"` if it is yours, otherwise inspect its projections"), nil
	}
	if !validConventionalBranch(root, item.Branch) {
		return implementationRefusal(item, "invalid conventional branch identity; repair the Work Item attachment"), nil
	}
	observed, outcome, err := claimSelected(ctx, selection, candidate, item)
	if err != nil {
		return ImplementationOutcome{}, fmt.Errorf("Claim acquisition for Work Item %s may have succeeded but read-back failed; inspect that Work Item and explicitly resume the same identity instead of running `skl implement next`: %w", item.ID, err)
	}
	if outcome.Status != "" {
		return outcome, nil
	}
	// The claimed record's reconciled Workflow State decides the procedure;
	// a queue name or a branch name never does.
	procedure := skilldist.InitialSubmission
	if observed.State == Rework {
		procedure = skilldist.FindingDrivenRework
	}
	outcome, err = implementationPacket(root, remote, observed, endpoints, procedure)
	if err != nil {
		return ImplementationOutcome{}, fmt.Errorf("Claim for Work Item %s was acquired but Execution Skill delivery failed; inspect it and explicitly resume the same identity: %w", observed.ID, err)
	}
	return outcome, nil
}

func resumeImplementation(ctx context.Context, root, remote string, id WorkItemID, endpoints ArtifactEndpoints, selection SelectionBackend) (ImplementationOutcome, error) {
	branch := ""
	if id == CurrentWorktree {
		id = ""
		current, err := git(root, "symbolic-ref", "--short", "HEAD")
		if err != nil || current == "" {
			return ImplementationOutcome{Status: "fix_required", Reason: "worktree identity is ambiguous; resume with --item after repairing attachments"}, nil
		}
		branch = current
	}
	item, err := selection.ResumedImplementation(ctx, id, branch)
	if err != nil {
		return ImplementationOutcome{}, fmt.Errorf("fixed Work Item observation failed during resume; its Claim was not released, so inspect it and retry `skl implement resume --item <number>` rather than selecting replacement work: %w", err)
	}
	if item.Problem != "" {
		return implementationRefusal(item, item.Problem+"; repair the Work Item projections before resuming"), nil
	}
	if !item.Claimed || item.State != Ready && item.State != Rework {
		return implementationRefusal(item, "explicit Work Item is not an unambiguous implementation Claim; repair its projections before resuming"), nil
	}
	if !validConventionalBranch(root, item.Branch) {
		return implementationRefusal(item, "invalid conventional branch identity; repair the Work Item attachment"), nil
	}
	// The reconciled Workflow State decides whether this resume follows
	// finding-driven Rework or simply continues an existing implementation.
	procedure := skilldist.ResumedSubmission
	if item.State == Rework {
		procedure = skilldist.FindingDrivenRework
	}
	outcome, err := implementationPacket(root, remote, item, endpoints, procedure)
	if err != nil {
		return ImplementationOutcome{}, fmt.Errorf("the existing Claim for Work Item %s remains protected but Execution Skill delivery failed; inspect it and explicitly resume the same identity: %w", item.ID, err)
	}
	return outcome, nil
}

func validConventionalBranch(root, branch string) bool {
	return branch != "" && gitOK(root, "check-ref-format", "--branch", branch) == nil && !strings.Contains(branch, "/")
}

func implementationPacket(root, remote string, item ImplementationItem, endpoints ArtifactEndpoints, procedure skilldist.ImplementProcedure) (ImplementationOutcome, error) {
	main, err := primaryWorktree(root)
	if err != nil {
		return ImplementationOutcome{}, err
	}
	facts := skilldist.ImplementationFacts{Branch: item.Branch, Worktree: filepath.Join(main, ".worktrees", item.Branch), SuppliedArtifactBaseline: endpoints.Baseline, SuppliedArtifactCompletion: endpoints.Completion, Procedure: procedure}
	if item.Submission != nil {
		facts.Comments = item.Submission.Comments
	} else {
		facts.Comments = item.Feedback
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
