package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// publicationLease excludes live publishers while an operator reconciles an
// abandoned reservation. Unlike the brief ledger mutation lock, this lock
// protects the full external operation and is released if its process exits.
// It carries no Workflow State and never blocks ledger commits or Claims.
func (s *Store) publicationLease() (*os.File, error) {
	gitDirectory, err := gitCommonDir(s.Root)
	if err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(gitDirectory, "skl-publication.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open publication lease: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("another publication is still in flight (publication lease unavailable): %w", err)
	}
	return lock, nil
}

func releasePublicationLease(lock *os.File) {
	if lock != nil {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}
}
