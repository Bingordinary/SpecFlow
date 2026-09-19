//go:build !darwin && !linux && !windows

package localstate

import (
	"fmt"
	"os"
	"runtime"
)

func tryExclusiveFileLock(_ *os.File) (bool, error) {
	return false, fmt.Errorf("operating-system file locks are not supported on %s", runtime.GOOS)
}

func unlockFile(_ *os.File) error {
	return nil
}
