package localstate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lockRetryInterval = 25 * time.Millisecond

// WithExclusiveLock executes fn while holding one operating-system advisory
// lock below relStateRoot. The lock is tied to the open file descriptor, so
// the operating system releases it when the process exits; the lock file may
// remain as an inert carrier without blocking later acquisitions.
func WithExclusiveLock(repoRoot, relStateRoot, lockName string, timeout time.Duration, fn func() error) error {
	if timeout <= 0 {
		return fmt.Errorf("local-state lock timeout must be positive")
	}
	lockPath, err := Path(repoRoot, relStateRoot, lockName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("create local-state lock directory: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open local-state lock %s: %w", lockName, err)
	}
	defer file.Close()

	deadline := time.Now().Add(timeout)
	for {
		acquired, lockErr := tryExclusiveFileLock(file)
		if lockErr != nil {
			return fmt.Errorf("acquire local-state lock %s: %w", lockName, lockErr)
		}
		if acquired {
			break
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timed out after %s waiting for local-state lock %s; another state transition is still running", timeout, lockName)
		}
		time.Sleep(lockRetryInterval)
	}

	fnErr := fn()
	unlockErr := unlockFile(file)
	if fnErr != nil {
		return fnErr
	}
	if unlockErr != nil {
		return fmt.Errorf("release local-state lock %s: %w", lockName, unlockErr)
	}
	return nil
}
