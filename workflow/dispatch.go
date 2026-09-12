package workflow

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vicrdguez/skills/github"
)

type DispatchLane string

const (
	ImplementLane DispatchLane = "implement"
	WatchdogLane  DispatchLane = "watchdog"
)

type DispatchRound struct {
	ID              string       `json:"id"`
	Lane            DispatchLane `json:"lane"`
	Item            WorkItemID   `json:"item"`
	Submission      SubmissionID `json:"submission,omitempty"`
	Obligation      string       `json:"obligation"`
	Synchronization bool         `json:"synchronization,omitempty"`
	Directory       string       `json:"directory"`
	Outcome         State        `json:"outcome,omitempty"`
	Head            string       `json:"head,omitempty"`
	Released        bool         `json:"released,omitempty"`
}

type DispatchBackend interface {
	DispatchRounds(context.Context, github.RepositoryID, WorkItemID) ([]DispatchRound, error)
	RecordDispatchRound(context.Context, github.RepositoryID, DispatchRound) error
}

type DispatchFacts struct {
	Reference string
	Lane      DispatchLane
	Root      string
	Remote    string
	Wait      string
	Poll      string
}

type CompletedHandoff struct {
	Item    WorkItemID
	Outcome State
}

type dispatchReference struct {
	Version    int          `json:"v"`
	Owner      string       `json:"owner"`
	Repository string       `json:"repository"`
	Lane       DispatchLane `json:"lane"`
	Item       WorkItemID   `json:"item"`
	Submission SubmissionID `json:"submission,omitempty"`
	Round      string       `json:"round"`
}

func newDispatchID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func encodeDispatchReference(repository github.RepositoryID, round DispatchRound) (string, error) {
	data, err := json.Marshal(dispatchReference{Version: 1, Owner: repository.Owner, Repository: repository.Name, Lane: round.Lane, Item: round.Item, Submission: round.Submission, Round: round.ID})
	return base64.RawURLEncoding.EncodeToString(data), err
}

func resultDirectory(prefix, marker, existing string) (string, error) {
	if existing == "" {
		directory, err := os.MkdirTemp("", prefix)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(directory, ".skl-result"), []byte(marker), 0600); err != nil {
			os.RemoveAll(directory)
			return "", err
		}
		return directory, nil
	}
	if filepath.Base(existing) != existing {
		return "", errors.New("invalid persisted Result Directory identity")
	}
	directory := filepath.Join(os.TempDir(), existing)
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		if err := os.Mkdir(directory, 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(directory, ".skl-result"), []byte(marker), 0600); err != nil {
			os.RemoveAll(directory)
			return "", err
		}
		return directory, nil
	}
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("persisted Result Directory is not a private directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		allowed := entry.Name() == ".skl-result" || marker == "skl.implement/v1\n" && slices.Contains([]string{"submission.md", "decision.md"}, entry.Name()) || marker == "skl.watchdog/v1\n" && slices.Contains([]string{".md", ".json"}, filepath.Ext(entry.Name()))
		if !allowed || !entry.Type().IsRegular() {
			return "", errors.New("persisted Result Directory contains unexpected files or symlinks")
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, ".skl-result"))
	if err != nil || string(data) != marker {
		return "", errors.New("persisted Result Directory marker is invalid")
	}
	return directory, nil
}

func dispatchBinding(item ImplementationItem, lane DispatchLane) (SubmissionID, string) {
	submission := SubmissionID("")
	if item.Submission != nil && (lane == WatchdogLane || item.State == Rework) {
		submission = item.Submission.ID
	}
	obligation := item.TargetSnapshot
	if lane == ImplementLane && item.State == Rework && !item.Synchronization && item.Submission != nil {
		obligation = item.Submission.PreviousReviewedHead
	} else if lane == WatchdogLane && item.Submission != nil {
		obligation = item.Submission.ReviewedHead
	}
	return submission, obligation
}

func dispatchRound(item ImplementationItem, lane DispatchLane, directory string) (DispatchRound, error) {
	id, err := newDispatchID()
	if err != nil {
		return DispatchRound{}, err
	}
	round := DispatchRound{ID: id, Lane: lane, Item: item.ID, Directory: filepath.Base(directory), Synchronization: lane == ImplementLane && item.Synchronization}
	round.Submission, round.Obligation = dispatchBinding(item, lane)
	return round, nil
}

func activeDispatch(rounds []DispatchRound, item ImplementationItem, lane DispatchLane) (*DispatchRound, error) {
	var active *DispatchRound
	for i := range rounds {
		round := &rounds[i]
		if round.Lane != lane || round.Outcome != "" {
			continue
		}
		if active != nil {
			return nil, Refuse("multiple active dispatch rounds require explicit recovery")
		}
		active = round
	}
	if active == nil {
		return nil, nil
	}
	wantedSubmission, wantedObligation := dispatchBinding(item, lane)
	if active.Item != item.ID || active.Submission != wantedSubmission || active.Obligation != wantedObligation || active.Synchronization != (lane == ImplementLane && item.Synchronization) {
		return nil, Refuse("active dispatch round contradicts the fixed worker obligation")
	}
	return active, nil
}

func RemoveMarkerOnlyResultDirectory(directory, marker string) error {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 || filepath.Dir(directory) != os.TempDir() {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != ".skl-result" || !entries[0].Type().IsRegular() {
		return err
	}
	data, err := os.ReadFile(filepath.Join(directory, ".skl-result"))
	if err != nil || string(data) != marker {
		return err
	}
	return os.RemoveAll(directory)
}

func prepareDispatch(ctx context.Context, repository github.RepositoryID, item ImplementationItem, lane DispatchLane, resume bool, backend DispatchBackend) (DispatchRound, string, error) {
	if _, obligation := dispatchBinding(item, lane); obligation == "" {
		return DispatchRound{}, "", Refuse("fixed dispatch obligation is missing; inspect the Work Item and resume --reviewed-head <full-sha> for finding-driven Rework")
	}
	rounds, err := backend.DispatchRounds(ctx, repository, item.ID)
	if err != nil {
		return DispatchRound{}, "", err
	}
	active, err := activeDispatch(rounds, item, lane)
	if err != nil {
		return DispatchRound{}, "", err
	}
	if active != nil && !resume {
		return DispatchRound{}, "", Refuse("selected Work Item already has an active dispatch round; explicitly resume it")
	}
	prefix, marker := "skl-implement-", "skl.implement/v1\n"
	if lane == WatchdogLane {
		prefix, marker = "skl-watchdog-", "skl.watchdog/v1\n"
	}
	existing := ""
	if active != nil {
		existing = active.Directory
	}
	directory, err := resultDirectory(prefix, marker, existing)
	if err != nil {
		return DispatchRound{}, "", err
	}
	if active == nil {
		round, err := dispatchRound(item, lane, directory)
		if err != nil {
			os.RemoveAll(directory)
			return DispatchRound{}, "", err
		}
		if err := backend.RecordDispatchRound(ctx, repository, round); err != nil {
			os.RemoveAll(directory)
			return DispatchRound{}, "", err
		}
		rounds, err = backend.DispatchRounds(ctx, repository, item.ID)
		if err != nil {
			return DispatchRound{}, "", err
		}
		if !slices.Contains(rounds, round) {
			return DispatchRound{}, "", Refuse("dispatch round was not observed; retain the Claim and explicitly resume")
		}
		active = &round
	}
	reference, err := encodeDispatchReference(repository, *active)
	if err != nil {
		return DispatchRound{}, "", err
	}
	return *active, reference, nil
}

func dispatchRecovery(lane DispatchLane, item *ImplementationItem, outcome *ImplementationOutcome, err *error) {
	if item == nil {
		return
	}
	guidance := fmt.Sprintf("stop; inspect the retained Work Item and explicitly run %s resume --item %s; do not replay next or next --after", lane, item.ID)
	var violation *InvariantError
	if errors.As(*err, &violation) {
		*outcome = ImplementationOutcome{Status: "fix_required", Item: item, Reason: violation.Reason + "; " + guidance}
		*err = nil
	} else if *err != nil {
		*err = fmt.Errorf("%s: %w", guidance, *err)
	} else if outcome.Status == "fix_required" {
		outcome.Item = item
		outcome.Reason += "; " + guidance
	}
}

func permitsDispatchOutcome(lane DispatchLane, outcome State) bool {
	return lane == ImplementLane && slices.Contains([]State{AwaitingReview, NeedsHuman}, outcome) || lane == WatchdogLane && slices.Contains([]State{ReadyForMerge, Rework, NeedsHuman}, outcome)
}

func VerifyDispatch(ctx context.Context, root, remote string, lane DispatchLane, reference string, backend ImplementationBackend) (CompletedHandoff, error) {
	data, err := base64.RawURLEncoding.DecodeString(reference)
	if err != nil {
		return CompletedHandoff{}, errors.New("--after requires a supported opaque dispatch reference")
	}
	var decoded dispatchReference
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil || decoder.Decode(&struct{}{}) != io.EOF || decoded.Version != 1 || decoded.Owner == "" || decoded.Repository == "" || decoded.Item == "" || decoded.Round == "" || decoded.Lane != ImplementLane && decoded.Lane != WatchdogLane {
		return CompletedHandoff{}, errors.New("--after requires a supported opaque dispatch reference")
	}
	handoff := CompletedHandoff{Item: decoded.Item}
	if decoded.Lane != lane {
		return handoff, Refuse("dispatch reference belongs to another workflow lane")
	}
	remote, err = github.ResolveGitHubRemote(root, remote)
	if err != nil {
		return handoff, err
	}
	url, err := git(root, "remote", "get-url", remote)
	if err != nil {
		return handoff, err
	}
	repository, err := github.ParseGitHubRemote(url)
	if err != nil {
		return handoff, err
	}
	if repository.Owner != decoded.Owner || repository.Name != decoded.Repository {
		return handoff, Refuse("dispatch reference belongs to another repository")
	}
	rounds, err := backend.DispatchRounds(ctx, repository, decoded.Item)
	if err != nil {
		return handoff, err
	}
	var found *DispatchRound
	for i := range rounds {
		if rounds[i].ID == decoded.Round {
			if found != nil && *found != rounds[i] {
				return handoff, Refuse("dispatch round has conflicting durable evidence")
			}
			copy := rounds[i]
			found = &copy
		}
	}
	if found == nil || found.Item != decoded.Item || found.Lane != lane || found.Submission != decoded.Submission {
		return handoff, Refuse("dispatch round is unknown or its durable binding contradicts the reference")
	}
	handoff.Outcome = found.Outcome
	if !permitsDispatchOutcome(lane, found.Outcome) || found.Head == "" || !found.Released {
		return handoff, Refuse("dispatch handoff is incomplete or has the wrong stage; explicitly inspect and resume the Work Item")
	}
	// Observe identity and contradictions, not historical state/head equality.
	items, err := backend.ImplementationItems(ctx, repository)
	if err != nil {
		return handoff, err
	}
	var item *ImplementationItem
	for i := range items {
		if items[i].ID != decoded.Item {
			continue
		}
		if item != nil {
			return handoff, Refuse("ambiguous Work Item attachment; inspect and explicitly resume")
		}
		item = &items[i]
	}
	if item == nil || item.Problem != "" {
		return handoff, Refuse("Work Item attachment is missing or contradictory; inspect and explicitly resume")
	}
	if (found.Submission != "" || found.Outcome == AwaitingReview) && (item.Submission == nil || found.Submission != "" && item.Submission.ID != found.Submission) {
		return handoff, Refuse("Submission attachment needed for handoff proof is missing or contradictory; inspect and explicitly resume")
	}
	if lane == ImplementLane && item.Transition != nil && item.Transition.Directory == found.Directory && !item.Transition.Completed {
		return handoff, Refuse("this dispatch has a pending implementation handoff; inspect and explicitly resume")
	}
	latest := ""
	for _, round := range rounds {
		if round.Lane == lane {
			latest = round.ID
		}
	}
	sameLaneClaim := lane == ImplementLane && (item.State == Ready || item.State == Rework) || lane == WatchdogLane && item.State == AwaitingReview
	if item.Claimed && sameLaneClaim && latest == found.ID {
		return handoff, Refuse("this dispatch still has a retained Claim; inspect and explicitly resume")
	}
	return handoff, nil
}

func completeDispatch(ctx context.Context, repository github.RepositoryID, item ImplementationItem, lane DispatchLane, outcome State, head string, backend DispatchBackend) error {
	rounds, err := backend.DispatchRounds(ctx, repository, item.ID)
	if err != nil {
		return err
	}
	var active *DispatchRound
	for i := range rounds {
		if rounds[i].Lane == lane && rounds[i].Outcome == "" {
			if active != nil {
				return Refuse("multiple active dispatch rounds prevent completion proof")
			}
			copy := rounds[i]
			active = &copy
		}
	}
	if active == nil { // Legacy explicit handoffs remain supported but cannot authorize continuation.
		return nil
	}
	submission, obligation := SubmissionID(""), item.TargetSnapshot
	if lane == WatchdogLane && item.Submission != nil {
		submission, obligation = item.Submission.ID, item.Submission.ReviewedHead
	} else if lane == ImplementLane && active.Submission != "" && item.Submission != nil {
		submission = item.Submission.ID
		if !active.Synchronization {
			obligation = item.Submission.PreviousReviewedHead
		}
	}
	if active.Item != item.ID || active.Submission != submission || active.Obligation != obligation {
		return Refuse("active dispatch binding changed before completion proof")
	}
	if !permitsDispatchOutcome(lane, outcome) {
		return Refuse(fmt.Sprintf("%s cannot complete a %s dispatch", outcome, lane))
	}
	active.Outcome, active.Head, active.Released = outcome, head, true
	if err := backend.RecordDispatchRound(ctx, repository, *active); err != nil {
		return err
	}
	observed, err := backend.DispatchRounds(ctx, repository, item.ID)
	if err != nil {
		return err
	}
	if !slices.Contains(observed, *active) {
		return Refuse("completed dispatch evidence was not observed; explicitly inspect the handoff")
	}
	return nil
}
