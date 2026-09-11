package workflow

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/vicrdguez/skills/github"
)

type DispatchLane string

const (
	ImplementLane DispatchLane = "implement"
	WatchdogLane  DispatchLane = "watchdog"
)

type DispatchRound struct {
	ID         string       `json:"id"`
	Lane       DispatchLane `json:"lane"`
	Item       WorkItemID   `json:"item"`
	Submission SubmissionID `json:"submission,omitempty"`
	Obligation string       `json:"obligation"`
	Directory  string       `json:"directory"`
	Outcome    State        `json:"outcome,omitempty"`
	Head       string       `json:"head,omitempty"`
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
	data, err := json.Marshal(dispatchReference{Version: 1, Owner: repository.Owner, Repository: repository.Name, Lane: round.Lane, Item: round.Item, Round: round.ID})
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
	data, err := os.ReadFile(filepath.Join(directory, ".skl-result"))
	if err != nil || string(data) != marker {
		return "", errors.New("persisted Result Directory marker is invalid")
	}
	return directory, nil
}

func dispatchRound(item ImplementationItem, lane DispatchLane, directory string) (DispatchRound, error) {
	id, err := newDispatchID()
	if err != nil {
		return DispatchRound{}, err
	}
	round := DispatchRound{ID: id, Lane: lane, Item: item.ID, Directory: filepath.Base(directory)}
	if item.Submission != nil && (lane == WatchdogLane || item.State == Rework) {
		round.Submission = item.Submission.ID
	}
	if lane == ImplementLane {
		round.Obligation = item.TargetSnapshot
		if item.State == Rework && item.Submission != nil {
			round.Obligation = item.Submission.PreviousReviewedHead
		}
	} else if item.Submission != nil {
		round.Obligation = item.Submission.ReviewedHead
	}
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
	wanted, err := dispatchRound(item, lane, filepath.Join(os.TempDir(), active.Directory))
	if err != nil {
		return nil, err
	}
	if active.Item != wanted.Item || active.Submission != wanted.Submission || active.Obligation != wanted.Obligation {
		return nil, Refuse("active dispatch round contradicts the fixed worker obligation")
	}
	return active, nil
}

func prepareDispatch(ctx context.Context, root, remote string, repository github.RepositoryID, item ImplementationItem, lane DispatchLane, resume bool, backend DispatchBackend) (DispatchRound, string, error) {
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
	_ = root
	_ = remote
	return *active, reference, nil
}
