package ledger

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/vicrdguez/skills/github"
)

// Claim endings that are not a phase handoff. A handed-off Claim ends in the
// outcome of the phase report that recorded it instead.
const (
	ClaimHeld     = "held"
	ClaimReleased = "released"
)

// ClaimEnding is how one Claim ended, established from ledger history.
type ClaimEnding struct {
	Item   string `json:"item"`
	Claim  string `json:"claim"`
	Ending string `json:"ending"`
}

// Handoff reports whether the Claim ended in a recorded phase handoff:
// Implement's submission or pause, or a Watchdog review's outcome.
func (e ClaimEnding) Handoff() bool {
	switch e.Ending {
	case AwaitingReview, NeedsHuman, "pass", Rework:
		return true
	}
	return false
}

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ClaimEndingOf establishes how the Claim acquired by claimCommit ended. The
// answer is keyed on that acquisition alone: the Work Item's current state and
// any earlier or later Claim on it are irrelevant. A reference that is not an
// acquisition commit of this Project and phase is refused.
func ClaimEndingOf(s *Store, repository github.RepositoryID, phase, claimCommit string) (ClaimEnding, error) {
	unknown := func(invariant string) (ClaimEnding, error) {
		return ClaimEnding{}, refuse(invariant, "name the Claim acquired by the last dispatch of this phase and Project")
	}
	if claimCommit == "" {
		return unknown("the Claim reference is empty")
	}
	if !fullCommit.MatchString(claimCommit) {
		return unknown("Claim reference " + claimCommit + " is not a full acquisition commit")
	}
	parent, err := git(s.Root, "rev-parse", "--verify", "--end-of-options", claimCommit+"^")
	if err != nil || !gitOK(s.Root, "merge-base", "--is-ancestor", claimCommit, "HEAD") {
		return unknown("Claim " + claimCommit + " is not in this ledger's history")
	}
	changed, err := git(s.Root, "diff-tree", "--no-commit-id", "--name-only", "-r", parent, claimCommit)
	if err != nil {
		return ClaimEnding{}, err
	}
	path := changed
	parts := strings.Split(path, "/")
	if strings.Contains(changed, "\n") || len(parts) != 6 || parts[0] != projectsRoot || parts[2] != "proposals" || parts[5] != "state.json" {
		return unknown("commit " + claimCommit + " is not a Claim acquisition")
	}
	if parts[1] != repository.Name {
		return unknown("Claim " + claimCommit + " belongs to Project " + parts[1])
	}
	var before, acquired SliceState
	if readJSONAt(s, parent, path, &before) != nil || before.Claim != nil || readJSONAt(s, claimCommit, path, &acquired) != nil || acquired.Claim == nil || acquired.Claim.Basis != parent {
		return unknown("commit " + claimCommit + " is not a Claim acquisition")
	}
	if acquired.Claim.Phase != phase {
		return unknown("Claim " + claimCommit + " belongs to the " + acquired.Claim.Phase + " phase")
	}
	ending := ClaimEnding{Item: parts[3] + "/" + parts[4], Claim: claimCommit, Ending: ClaimHeld}
	later, err := git(s.Root, "log", "--reverse", "--format=%H", claimCommit+"..HEAD", "--", path)
	if err != nil {
		return ClaimEnding{}, err
	}
	for _, commit := range strings.Fields(later) {
		var state SliceState
		if readJSONAt(s, commit, path, &state) == nil && reflect.DeepEqual(state.Claim, acquired.Claim) {
			continue
		}
		// The first commit without this Claim ended it. Only a phase report
		// recording this exact Claim makes that ending a handoff.
		ending.Ending = ClaimReleased
		reportPath := strings.TrimSuffix(path, "state.json") + phase + "-report.md"
		if raw, err := showPath(s, commit, reportPath); err == nil {
			if report, _, err := ParseReport(phase, []byte(raw)); err == nil && report.Ledger.Claim.Commit == claimCommit {
				ending.Ending = report.Outcome
			}
		}
		return ending, nil
	}
	return ending, nil
}
