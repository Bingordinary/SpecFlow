package operation

import (
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
)

const (
	mutationLockDir = "meta"
	mutationLock    = ".operations.lock"
	mutationTimeout = 30 * time.Second
)

// withMutation serializes one operation-state read-modify-write transition
// across processes. Close and Update load mutable state, decide, and save;
// both hold the repository-local operating-system lock for the whole
// transition so a concurrent transition cannot lose an update or overwrite a
// committed status with a stale snapshot.
func withMutation(repoRoot string, fn func() error) error {
	return localstate.WithExclusiveLock(repoRoot, mutationLockDir, mutationLock, mutationTimeout, fn)
}
