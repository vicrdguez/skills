package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/vicrdguez/skills/github"
)

const (
	AwaitingReview = "awaiting_review"
	Rework         = "rework"
	NeedsHuman     = "needs_human"
	ReadyForMerge  = "ready_for_merge"
	Merged         = "merged"
	Superseded     = "superseded"
)

// Claim is a non-expiring reservation with fixed inputs. Basis is the ledger
// parent of its acquisition commit, not a generated execution or result ID.
type Claim struct {
	Phase  string      `json:"phase"`
	Basis  string      `json:"basis"`
	Inputs ClaimInputs `json:"inputs"`
}

type ClaimInputs struct {
	Contract  []Reference `json:"contract"`
	Implement *Reference  `json:"implement,omitempty"`
	Watchdog  *Reference  `json:"watchdog,omitempty"`
	Decision  *Reference  `json:"decision,omitempty"`
}

// Execution identifies one reservation and the exact evidence it consumed.
// Documents are data; no report or decision prose controls Workflow State.
type Execution struct {
	Project    string             `json:"project"`
	Repository string             `json:"repository"`
	Item       string             `json:"item"`
	State      SliceState         `json:"state"`
	Claim      Reference          `json:"claim"`
	Documents  []ContractDocument `json:"documents"`
	Implement  *Report            `json:"implement,omitempty"`
	Watchdog   *Report            `json:"watchdog,omitempty"`
}

func itemDirectory(project, item string) (string, error) {
	proposal, slice, ok := strings.Cut(item, "/")
	if !ok || !ValidRecordName(proposal) || !ValidRecordName(slice) {
		return "", refuse("invalid Work Item identity "+item, "use the accepted proposal/slice identity")
	}
	return filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals", proposal, slice)), nil
}

func (s *Store) deliveryState(repository github.RepositoryID, item string) (SliceState, string, string, error) {
	head, err := s.head()
	if err != nil {
		return SliceState{}, "", "", err
	}
	var project ProjectIdentity
	if err := readJSONAt(s, head, filepath.Join(projectsRoot, repository.Name, "project.json"), &project); err != nil || project.Repository != repository.Owner+"/"+repository.Name {
		return SliceState{}, "", "", refuse("no matching committed Project for "+repository.Owner+"/"+repository.Name, "configure the correct private ledger and accept the Project before delivery")
	}
	directory, err := itemDirectory(repository.Name, item)
	if err != nil {
		return SliceState{}, "", "", err
	}
	var state SliceState
	if err := readJSONAt(s, head, directory+"/state.json", &state); err != nil {
		return state, "", "", refuse("selected Work Item "+item+" has no readable committed state", "repair the selected record; forge conversations and source markers are not substitutes")
	}
	if state.Title == "" || !ValidRecordName(state.Branch) {
		return state, "", "", refuse("selected Work Item has invalid title or planned branch", "repair the selected source identity with human direction")
	}
	switch state.State {
	case ReadyForImplementation, AwaitingReview, Rework, NeedsHuman, ReadyForMerge, Merged, Superseded:
	default:
		return state, "", "", refuse("selected Work Item has unknown state "+state.State, "repair its recorded lifecycle with human direction")
	}
	return state, directory, head, nil
}

// StartDelivery selects and reserves only this Project's work. Network
// observation precedes the brief local lock; stale known competing history
// still refuses acquisition even when the network is unavailable.
func StartDelivery(s *Store, repository github.RepositoryID, phase string) (*Execution, error) {
	return StartDeliveryContext(context.Background(), s, repository, phase)
}

// StartDeliveryContext permits a waiting CLI to cancel network observation
// before reservation; a committed acquisition is never undone by cancellation.
func StartDeliveryContext(ctx context.Context, s *Store, repository github.RepositoryID, phase string) (*Execution, error) {
	if phase != ImplementPhase && phase != WatchdogPhase {
		return nil, fmt.Errorf("unknown delivery phase %q", phase)
	}
	s.observeDeliveryUpstream(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var execution *Execution
	err := s.withMutation(func() error {
		if err := s.requireReconciled(); err != nil {
			return err
		}
		head, err := s.head()
		if err != nil {
			return err
		}
		var project ProjectIdentity
		if err := readJSONAt(s, head, filepath.Join(projectsRoot, repository.Name, "project.json"), &project); err != nil || project.Repository != repository.Owner+"/"+repository.Name {
			return refuse("no matching committed Project for "+repository.Owner+"/"+repository.Name, "configure the ledger and accept the Project before selecting work")
		}
		paths, err := git(s.Root, "ls-tree", "-r", "--name-only", head, "--", filepath.Join(projectsRoot, repository.Name, "proposals"))
		if err != nil {
			return err
		}
		type candidate struct{ item, accepted, lane string }
		var candidates []candidate
		for _, path := range strings.Split(paths, "\n") {
			if !strings.HasSuffix(path, "/state.json") {
				continue
			}
			relative := strings.TrimPrefix(path, projectsRoot+"/"+repository.Name+"/proposals/")
			item := strings.TrimSuffix(relative, "/state.json")
			state, _, _, err := s.deliveryState(repository, item)
			if err != nil {
				return err
			}
			if state.Claim != nil || !phaseEligible(phase, state.State) {
				continue
			}
			eligible := true
			for _, dependency := range state.Dependencies {
				status, err := s.readStateByReferenceAt(head, repository.Name, dependency)
				if err != nil || status != Merged {
					eligible = false
					break
				}
			}
			if !eligible {
				continue
			}
			proposal, _, _ := strings.Cut(item, "/")
			var meta ProposalMeta
			if err := readJSONAt(s, head, filepath.Join(projectsRoot, repository.Name, "proposals", proposal, "proposal.json"), &meta); err != nil {
				return err
			}
			lane := "1"
			if state.State == Rework {
				lane = "0"
			}
			candidates = append(candidates, candidate{item, meta.Accepted, lane})
		}
		sort.Slice(candidates, func(i, j int) bool {
			a, b := candidates[i], candidates[j]
			if a.lane != b.lane {
				return a.lane < b.lane
			}
			if a.accepted != b.accepted {
				return a.accepted < b.accepted
			}
			return a.item < b.item
		})
		if len(candidates) == 0 {
			return nil
		}
		state, directory, head, err := s.deliveryState(repository, candidates[0].item)
		if err != nil {
			return err
		}
		if err := s.requireBranchOwner(repository.Name, candidates[0].item, state.Branch); err != nil {
			return err
		}
		if err := s.requireCleanPaths(directory); err != nil {
			return err
		}
		inputs, err := s.deliveryInputs(head, directory, state)
		if err != nil {
			return err
		}
		state.Claim = &Claim{Phase: phase, Basis: head, Inputs: inputs}
		// Validate all required report schemas before acquiring a reservation.
		proposed := &Execution{Project: repository.Name, Repository: project.Repository, Item: candidates[0].item, State: state}
		if err := s.hydrateExecution(proposed); err != nil {
			return err
		}
		if phase == WatchdogPhase && (proposed.Implement == nil || proposed.Implement.Source.Head == "" || proposed.Implement.Source.Target == "") {
			return refuse("review requires a committed implementation report with a fixed source head and target", "complete the implementation source handoff before selecting review")
		}
		if phase == WatchdogPhase && proposed.Watchdog != nil && proposed.Watchdog.Round == math.MaxUint64 {
			return refuse("completed-review count cannot be advanced", "resolve the exhausted count with human direction before selecting review")
		}
		if state.State == Rework && proposed.Watchdog == nil {
			return refuse("Rework requires a committed watchdog report", "restore the required review evidence")
		}
		if requiresDecision(proposed) && inputs.Decision == nil {
			return refuse("continued work requires recorded human direction", "have the human-decision operation record direction and requeue within the frozen Contract")
		}
		if err := writeJSON(filepath.Join(s.Root, directory, "state.json"), state); err != nil {
			return err
		}
		if err := s.commit("claim "+phase+" "+repository.Name+"/"+proposed.Item, directory+"/state.json"); err != nil {
			return err
		}
		acquired, err := s.head()
		if err != nil {
			return err
		}
		proposed.Claim = Reference{Commit: acquired, Path: directory + "/state.json"}
		execution = proposed
		return nil
	})
	return execution, err
}

// requireBranchOwner keeps source identity unique within a Project, including
// historical records whose worktrees or branches may still contain progress.
// Check under the ledger mutation lock at both acceptance and acquisition.
func (s *Store) requireBranchOwner(project, item, branch string) error {
	head, err := s.head()
	if err != nil {
		return err
	}
	prefix := filepath.ToSlash(filepath.Join(projectsRoot, project, "proposals"))
	paths, err := git(s.Root, "ls-tree", "-r", "--name-only", head, "--", prefix)
	if err != nil {
		return err
	}
	for _, path := range strings.Split(paths, "\n") {
		if !strings.HasSuffix(path, "/state.json") {
			continue
		}
		other := strings.TrimSuffix(strings.TrimPrefix(path, prefix+"/"), "/state.json")
		if other == item {
			continue
		}
		var state SliceState
		if err := readJSONAt(s, head, path, &state); err != nil {
			return err
		}
		if state.Branch == branch {
			return refuse("planned branch "+branch+" is already owned by "+other, "give "+item+" a distinct source branch without reusing another Work Item's worktree or progress")
		}
	}
	return nil
}

func phaseEligible(phase, state string) bool {
	return phase == ImplementPhase && (state == ReadyForImplementation || state == Rework) || phase == WatchdogPhase && state == AwaitingReview
}

func requiresDecision(e *Execution) bool {
	return e.Implement != nil && e.Implement.Outcome == NeedsHuman || e.Watchdog != nil && (e.Watchdog.Outcome == NeedsHuman || e.Watchdog.Outcome == Rework && e.Watchdog.Round >= 2)
}

func (s *Store) deliveryInputs(head, directory string, state SliceState) (ClaimInputs, error) {
	var inputs ClaimInputs
	// The active marker means the exact decision document sits at the current
	// full head. Pin that document; a missing one is a damaged record rather
	// than a silently directionless continuation.
	if state.Decision {
		decisionPath := directory + "/decision.md"
		if !gitOK(s.Root, "cat-file", "-e", head+":"+decisionPath) {
			return inputs, refuse(
				"selected Work Item records an active Human Decision without its decision document",
				"restore decision.md at the current head or clear the marker with human direction",
			)
		}
		inputs.Decision = &Reference{Commit: head, Path: decisionPath}
	}
	names, err := acceptedFileNamesAt(s, head, directory)
	if err != nil {
		return inputs, err
	}
	if !containsString(names, "intent.md") || !containsString(names, "behavior.md") {
		return inputs, refuse("selected Contract is incomplete", "restore its exact accepted documents")
	}
	for _, name := range names {
		inputs.Contract = append(inputs.Contract, Reference{Commit: head, Path: directory + "/" + name})
	}
	for _, entry := range []struct {
		name   string
		target **Reference
	}{{"implement-report.md", &inputs.Implement}, {"watchdog-report.md", &inputs.Watchdog}} {
		if gitOK(s.Root, "cat-file", "-e", head+":"+directory+"/"+entry.name) {
			*entry.target = &Reference{Commit: head, Path: directory + "/" + entry.name}
		}
	}
	return inputs, nil
}

func (s *Store) hydrateExecution(e *Execution) error {
	inputs := e.State.Claim.Inputs
	for _, ref := range inputs.Contract {
		doc, err := ShowReference(s, ref.Commit, ref.Path)
		if err != nil {
			return err
		}
		e.Documents = append(e.Documents, doc)
	}
	for _, entry := range []struct {
		phase  string
		ref    *Reference
		target **Report
	}{{ImplementPhase, inputs.Implement, &e.Implement}, {WatchdogPhase, inputs.Watchdog, &e.Watchdog}} {
		if entry.ref == nil {
			continue
		}
		doc, err := ShowReference(s, entry.ref.Commit, entry.ref.Path)
		if err != nil {
			return err
		}
		report, _, err := ParseReport(entry.phase, []byte(doc.Contents))
		if err != nil {
			return refuse("required "+entry.phase+" report is incompatible: "+err.Error(), "use a compatible CLI or repair the selected report with human direction")
		}
		*entry.target = &report
		e.Documents = append(e.Documents, doc)
	}
	if inputs.Decision != nil {
		doc, err := ShowReference(s, inputs.Decision.Commit, inputs.Decision.Path)
		if err != nil {
			return err
		}
		e.Documents = append(e.Documents, doc)
	}
	return nil
}

// ResumeDelivery requires the exact acquisition reference: an old execution
// cannot silently take over a later Claim on the same Work Item.
func ResumeDelivery(s *Store, repository github.RepositoryID, item, phase, claimCommit string) (*Execution, error) {
	var execution *Execution
	err := s.withMutation(func() error {
		var err error
		execution, err = s.currentExecution(repository, item, phase, claimCommit)
		return err
	})
	return execution, err
}

func (s *Store) currentExecution(repository github.RepositoryID, item, phase, claimCommit string) (*Execution, error) {
	state, directory, _, err := s.deliveryState(repository, item)
	if err != nil {
		return nil, err
	}
	if state.Claim == nil || state.Claim.Phase != phase || !phaseEligible(phase, state.State) {
		return nil, refuse("no matching active "+phase+" Claim for "+item, "inspect the selected Work Item; do not acquire replacement work or release a later reservation")
	}
	ref := Reference{Commit: claimCommit, Path: directory + "/state.json"}
	doc, err := ShowReference(s, ref.Commit, ref.Path)
	if err != nil {
		return nil, err
	}
	var acquired SliceState
	if err := json.Unmarshal([]byte(doc.Contents), &acquired); err != nil || acquired.Claim == nil || !reflect.DeepEqual(acquired.Claim, state.Claim) {
		return nil, refuse("the supplied execution does not own the current Claim", "preserve later work and use the exact acquisition reference for your reservation")
	}
	parent, err := git(s.Root, "rev-parse", claimCommit+"^")
	if err != nil || parent != state.Claim.Basis {
		return nil, refuse("Claim reference is not its acquisition commit", "use the exact Claim commit returned by Work Start")
	}
	if acquired.State != state.State || acquired.Branch != state.Branch || acquired.Title != state.Title || !reflect.DeepEqual(acquired.Dependencies, state.Dependencies) || !reflect.DeepEqual(acquired.Decision, state.Decision) {
		return nil, refuse("selected execution inputs changed", "inspect the selected record before proceeding")
	}
	e := &Execution{Project: repository.Name, Repository: repository.Owner + "/" + repository.Name, Item: item, State: state, Claim: ref}
	if err := s.hydrateExecution(e); err != nil {
		return nil, err
	}
	// Only selected-item inputs participate. Unrelated commits do not stale it.
	head, err := s.head()
	if err != nil {
		return nil, err
	}
	inputs, err := s.deliveryInputs(head, directory, state)
	if err != nil {
		return nil, err
	}
	refs := append([]Reference(nil), state.Claim.Inputs.Contract...)
	for _, r := range []*Reference{state.Claim.Inputs.Implement, state.Claim.Inputs.Watchdog, state.Claim.Inputs.Decision} {
		if r != nil {
			refs = append(refs, *r)
		}
	}
	if len(inputs.Contract) != len(state.Claim.Inputs.Contract) || (inputs.Implement == nil) != (state.Claim.Inputs.Implement == nil) || (inputs.Watchdog == nil) != (state.Claim.Inputs.Watchdog == nil) {
		return nil, refuse("selected input set changed after Claim", "inspect and resolve the changed inputs before continuing")
	}
	for _, ref := range refs {
		before, err := showPath(s, ref.Commit, ref.Path)
		if err != nil {
			return nil, err
		}
		current, err := showPath(s, head, ref.Path)
		if err != nil || before != current {
			return nil, refuse("selected input changed after Claim: "+ref.Path, "preserve the fixed input and resolve the competing change")
		}
	}
	return e, nil
}

// ReleaseDelivery explicitly releases exactly one reservation without touching
// source progress, phase reports, or lifecycle eligibility.
func ReleaseDelivery(s *Store, repository github.RepositoryID, item, phase, claimCommit string) error {
	return s.withMutation(func() error {
		e, err := s.currentExecution(repository, item, phase, claimCommit)
		if err != nil {
			return err
		}
		if err := s.requireCleanPaths(e.Claim.Path); err != nil {
			return err
		}
		e.State.Claim = nil
		if err := writeJSON(filepath.Join(s.Root, e.Claim.Path), e.State); err != nil {
			return err
		}
		return s.commit("release "+phase+" "+repository.Name+"/"+item, e.Claim.Path)
	})
}

func (s *Store) observeDeliveryUpstream(ctx context.Context) {
	remote, _, err := s.upstream()
	if err == nil && remote != "" {
		_ = exec.CommandContext(ctx, "git", "-C", s.Root, "fetch", "--quiet", remote).Run()
	}
}

func (s *Store) requireReconciled() error {
	remote, branch, err := s.upstream()
	if err != nil {
		return err
	}
	if remote == "" {
		return nil
	}
	head, err := s.head()
	if err != nil {
		return err
	}
	diverged, err := s.diverged(remote, branch, head)
	if err != nil {
		return err
	}
	if diverged {
		return refuse("known competing ledger history requires reconciliation", "preserve local results and reconcile the ledger explicitly before granting more work")
	}
	return nil
}
