package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type reviewCheckpoint struct {
	Count         uint64
	Head          string
	Path          string
	ObjectIDWidth int
}

func loadReviewCheckpoint(root, branch string) (reviewCheckpoint, error) {
	if !validConventionalBranch(root, branch) {
		return reviewCheckpoint{}, fmt.Errorf("invalid conventional branch identity; repair the Work Item attachment")
	}
	main, err := primaryWorktree(root)
	if err != nil {
		return reviewCheckpoint{}, fmt.Errorf("resolve selected Work Item worktree: %w", err)
	}
	worktree := filepath.Join(main, ".worktrees", branch)
	gitDir, err := git(worktree, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return reviewCheckpoint{}, fmt.Errorf("resolve private Git directory for selected Work Item worktree %s: %w", worktree, err)
	}
	width := 40
	if format, formatErr := git(worktree, "rev-parse", "--show-object-format"); formatErr == nil && format == "sha256" {
		width = 64
	}
	checkpoint := reviewCheckpoint{Path: filepath.Join(gitDir, ".watchdog"), ObjectIDWidth: width}
	data, err := os.ReadFile(checkpoint.Path)
	if errors.Is(err, os.ErrNotExist) {
		return checkpoint, nil
	}
	if err != nil {
		return checkpoint, fmt.Errorf("read Review Checkpoint %s: %w; repair access without deleting or resetting it", checkpoint.Path, err)
	}
	text := string(data)
	if strings.HasSuffix(text, "\n") {
		text = strings.TrimSuffix(text, "\n")
	}
	count, head, ok := strings.Cut(text, ":")
	digits := count != "" && strings.IndexFunc(count, func(r rune) bool { return r < '0' || r > '9' }) < 0
	if !ok || !digits || head == "" || strings.Contains(head, ":") || strings.ContainsAny(text, "\r\n") {
		return checkpoint, fmt.Errorf("corrupt Review Checkpoint %s: want <nonnegative-count>:<full-sha> with at most one terminal newline; repair it explicitly", checkpoint.Path)
	}
	checkpoint.Count, err = strconv.ParseUint(count, 10, 64)
	if err != nil {
		return checkpoint, fmt.Errorf("corrupt Review Checkpoint %s: count must be a nonnegative, nonoverflowing decimal; repair it explicitly", checkpoint.Path)
	}
	if !checkpoint.validHead(head) {
		return checkpoint, fmt.Errorf("corrupt Review Checkpoint %s: SHA must be a full %d-character hexadecimal object ID; repair it explicitly", checkpoint.Path, checkpoint.ObjectIDWidth)
	}
	checkpoint.Head = head
	return checkpoint, nil
}

func (c reviewCheckpoint) validHead(head string) bool {
	return len(head) == c.ObjectIDWidth && strings.IndexFunc(head, func(r rune) bool { return !unicode.Is(unicode.ASCII_Hex_Digit, r) }) < 0
}

func (c reviewCheckpoint) replace(count uint64, head string, guard func() error) error {
	stale, _ := filepath.Glob(filepath.Join(filepath.Dir(c.Path), ".watchdog-*"))
	for _, name := range stale {
		if err := os.Remove(name); err != nil {
			return fmt.Errorf("remove stale atomic Review Checkpoint replacement: %w; repair private Git-directory access and retry the same fixed-number command", err)
		}
	}
	temporary, err := os.CreateTemp(filepath.Dir(c.Path), ".watchdog-*")
	if err != nil {
		return fmt.Errorf("create atomic Review Checkpoint replacement: %w; repair private Git-directory access and retry the same fixed-number command", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = fmt.Fprintf(temporary, "%d:%s\n", count, head); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil && guard != nil {
		if err := guard(); err != nil {
			return err
		}
	}
	if err == nil {
		err = os.Rename(name, c.Path)
	}
	if err != nil {
		return fmt.Errorf("atomically replace Review Checkpoint: %w; repair private Git-directory access and retry the same fixed-number command", err)
	}
	err = syncCheckpointDirectory(c.Path)
	if err != nil {
		return fmt.Errorf("sync Review Checkpoint directory after atomic replacement: %w; retain the Claim and retry the same fixed-number command", err)
	}
	return nil
}

func (c reviewCheckpoint) remove() error {
	if err := os.Remove(c.Path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("review completed but remove Review Checkpoint %s: %w; remove it manually", c.Path, err)
	}
	if err := syncCheckpointDirectory(c.Path); err != nil {
		return fmt.Errorf("review completed and removed Review Checkpoint %s but directory sync failed: %w; cleanup durability is uncertain, inspect the private Git directory and retry the same command", c.Path, err)
	}
	return nil
}

func syncCheckpointDirectory(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	err = directory.Sync()
	if closeErr := directory.Close(); err == nil {
		err = closeErr
	}
	return err
}
