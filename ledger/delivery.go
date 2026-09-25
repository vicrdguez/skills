package ledger

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"

	"github.com/vicrdguez/skills/github"
)

// DeliveryResult is a locally committed phase handoff. Publication may remain
// pending independently of this status; the report reference is never a forge ID.
type DeliveryResult struct {
	Status           string           `json:"status"`
	Item             string           `json:"item"`
	Report           Reference        `json:"report"`
	State            SliceState       `json:"state"`
	AlreadyCompleted bool             `json:"already_completed,omitempty"`
	Replication      *PublicationNote `json:"replication,omitempty"`
	Publication      *PublicationNote `json:"publication,omitempty"`
}

// CurrentReport reads one selected phase report from the current committed
// ledger, retaining its exact reference without discovering historical markers.
func CurrentReport(s *Store, repository github.RepositoryID, item, phase string) (Report, Reference, error) {
	if phase != ImplementPhase && phase != WatchdogPhase {
		return Report{}, Reference{}, fmt.Errorf("unknown delivery phase %q", phase)
	}
	_, directory, head, err := s.deliveryState(repository, item)
	if err != nil {
		return Report{}, Reference{}, err
	}
	ref := Reference{Commit: head, Path: directory + "/" + phase + "-report.md"}
	contents, err := showPath(s, ref.Commit, ref.Path)
	if err != nil {
		return Report{}, Reference{}, err
	}
	report, _, err := ParseReport(phase, []byte(contents))
	return report, ref, err
}

// HandoffDelivery commits the report and resulting state/Claim together. The
// caller validates source Git identities before entering this local mutation.
// Completion prose remains opaque; only the semantic outcome chooses state.
func HandoffDelivery(s *Store, repository github.RepositoryID, item, phase, claimCommit string, source SourceRevisions, outcome, body string) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.withMutation(func() error {
		state, directory, head, err := s.deliveryState(repository, item)
		if err != nil {
			return err
		}
		if phase != ImplementPhase && phase != WatchdogPhase {
			return fmt.Errorf("unknown delivery phase %q", phase)
		}
		reportPath := directory + "/" + phase + "-report.md"
		claimRef := Reference{Commit: claimCommit, Path: directory + "/state.json"}
		// The current exact report is sufficient replay evidence. We never search
		// arbitrary history or overwrite later work to reconstruct an old effect.
		if state.Claim == nil {
			raw, err := showPath(s, head, reportPath)
			if err != nil {
				return refuse("the supplied Claim is no longer active and no completed effect is established", "inspect the current selected record; preserve your original Result Document")
			}
			recorded, recordedBody, err := ParseReport(phase, []byte(raw))
			if err != nil {
				return err
			}
			target, err := deliveryDestination(phase, recorded.Outcome, recorded.Round)
			if err != nil {
				return err
			}
			if recorded.Ledger.Claim != claimRef || recorded.Outcome != outcome || !reflect.DeepEqual(recorded.Source, source) || recordedBody != body || state.State != target {
				return refuse("the old execution cannot replace the current result or lifecycle", "preserve later work; use the original exact handoff only when current evidence establishes its completed effect")
			}
			result = &DeliveryResult{Status: state.State, Item: item, Report: Reference{Commit: head, Path: reportPath}, State: state, AlreadyCompleted: true}
			return nil
		}
		execution, err := s.currentExecution(repository, item, phase, claimCommit, false)
		if err != nil {
			return err
		}
		if err := s.requireCleanPaths(directory); err != nil {
			return err
		}
		report := Report{Schema: 1, Outcome: outcome, Source: source, Ledger: ReportInputs{
			Claim: execution.Claim, Contract: execution.State.Claim.Inputs.Contract,
			Implement: execution.State.Claim.Inputs.Implement, Watchdog: execution.State.Claim.Inputs.Watchdog, Decision: execution.State.Claim.Inputs.Decision,
		}}
		if phase == WatchdogPhase {
			if execution.Implement == nil {
				return refuse("review handoff has no consumed implementation report", "restore the fixed implementation evidence")
			}
			report.Round = 1
			if execution.Watchdog != nil {
				if execution.Watchdog.Round == math.MaxUint64 {
					return refuse("completed review count is exhausted", "seek human repair without resetting the count")
				}
				report.Round = execution.Watchdog.Round + 1
			}
			if source.Reviewed != execution.Implement.Source.Head || source.Target != execution.Implement.Source.Target {
				return refuse("review source does not match the consumed implementation report", "review and submit the exact fixed implementation head and target")
			}
		}
		target, err := deliveryDestination(phase, outcome, report.Round)
		if err != nil {
			return err
		}
		contents, err := FormatReport(phase, report, body)
		if err != nil {
			return err
		}
		state.Claim = nil
		state.State = target
		// A directed implementation continues into its independent review. Keep
		// that direction available to the reviewer; completing review (or raising
		// a new implementation blocker) consumes it, never authorizing a later cycle.
		if phase == WatchdogPhase || outcome == NeedsHuman {
			state.Decision = false
		}
		if err := os.WriteFile(filepath.Join(s.Root, reportPath), contents, 0600); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(s.Root, directory, "state.json"), state); err != nil {
			return err
		}
		if err := s.commit("record "+phase+" "+repository.Name+"/"+item, reportPath, directory+"/state.json"); err != nil {
			return err
		}
		committed, err := s.head()
		if err != nil {
			return err
		}
		result = &DeliveryResult{Status: target, Item: item, Report: Reference{Commit: committed, Path: reportPath}, State: state}
		return nil
	})
	return result, err
}

func deliveryDestination(phase, outcome string, round uint64) (string, error) {
	if phase == ImplementPhase {
		switch outcome {
		case AwaitingReview, NeedsHuman:
			return outcome, nil
		}
	}
	if phase == WatchdogPhase {
		switch outcome {
		case "pass":
			return ReadyForMerge, nil
		case Rework:
			if round >= 2 {
				return NeedsHuman, nil
			}
			return Rework, nil
		case NeedsHuman:
			return NeedsHuman, nil
		}
	}
	return "", refuse("unsupported "+phase+" outcome "+outcome, "use the phase's documented semantic outcomes")
}

// ReplicateDelivery attempts the configured ledger push outside the mutation
// lock, then records only the still-current result's pending replication fact.
func ReplicateDelivery(s *Store, repository github.RepositoryID, result *DeliveryResult) {
	note, _ := s.push(result.Report.Commit)
	result.Replication = &note
	err := s.withMutation(func() error {
		state, directory, head, err := s.deliveryState(repository, result.Item)
		if err != nil {
			return err
		}
		if err := s.requireCleanPaths(directory + "/state.json"); err != nil {
			return err
		}
		wanted, err := showPath(s, result.Report.Commit, result.Report.Path)
		if err != nil {
			return err
		}
		current, err := showPath(s, head, result.Report.Path)
		if err != nil {
			return err
		}
		if wanted != current {
			return nil
		}
		if state.Publication == nil {
			state.Publication = &PublicationState{}
		}
		if note.Status == PushPushed {
			state.Publication.Push = nil
		} else {
			state.Publication.Push = &note
		}
		// No pending fact is recorded as an absent publication, as in
		// recordPublication, never as an empty object.
		if *state.Publication == (PublicationState{}) {
			state.Publication = nil
		}
		if reflect.DeepEqual(state.Publication, result.State.Publication) {
			return nil
		}
		if err := writeJSON(filepath.Join(s.Root, directory, "state.json"), state); err != nil {
			return err
		}
		if err := s.commit("record delivery replication "+repository.Name+"/"+result.Item, directory+"/state.json"); err != nil {
			return err
		}
		result.State = state
		return nil
	})
	if err != nil {
		result.Replication = &PublicationNote{Status: PushPending, Detail: note.Status + ": " + note.Detail + "; replication bookkeeping remains pending: " + err.Error()}
	}
}
