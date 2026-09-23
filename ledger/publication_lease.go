package ledger

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// A publication lease excludes live publishers while an operator reconciles
// an interrupted reservation. Unlike the brief ledger mutation lock, it is
// held across forge I/O, scoped to the selected repository, proposal, kind,
// and Work Item. It is released automatically if its process exits and never
// blocks unrelated ledger commits, slices, or Claims.
func publicationLeaseName(repository, proposal, slice, kind string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(repository+"/"+proposal+"/"+slice+"/"+kind)))
}

func (s *Store) publicationLeases(names ...string) ([]*os.File, error) {
	gitDirectory, err := gitCommonDir(s.Root)
	if err != nil {
		return nil, err
	}
	// Normal acceptance may publish several issues. Acquire their leases
	// before reserving any, in a stable order, and release all on refusal.
	sort.Strings(names)
	var locks []*os.File
	for _, name := range names {
		lock, err := os.OpenFile(filepath.Join(gitDirectory, "skl-publication-"+name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
		if err == nil {
			err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		}
		if err != nil {
			if lock != nil {
				_ = lock.Close()
			}
			releasePublicationLeases(locks)
			return nil, fmt.Errorf("another publication is still in flight (publication lease unavailable): %w", err)
		}
		locks = append(locks, lock)
	}
	return locks, nil
}

func releasePublicationLeases(locks []*os.File) {
	for _, lock := range locks {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}
}
