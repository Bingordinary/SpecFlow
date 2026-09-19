package localstate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateID(t *testing.T) {
	if err := ValidateID("20260917-123456-a0b1c2"); err != nil {
		t.Fatalf("valid id rejected: %v", err)
	}
	for _, id := range []string{"", "nope", "../../outside", "20260917-123456-A0B1C2", "20260917-123456-a0b1c2/extra"} {
		if err := ValidateID(id); err == nil {
			t.Fatalf("invalid id %q was accepted", id)
		}
	}
}

func TestBindIDRejectsMismatch(t *testing.T) {
	err := BindID("20260917-123456-a0b1c2", "20260917-123456-d3e4f5")
	if err == nil || !strings.Contains(err.Error(), "does not match requested id") {
		t.Fatalf("expected identity mismatch, got %v", err)
	}
}

func TestPathRejectsLexicalAndSymlinkEscape(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := Path(repoRoot, "meta/state", ".."); err == nil {
		t.Fatal("expected lexical escape to be rejected")
	}

	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repoRoot, "meta", "state")); err != nil {
		t.Fatal(err)
	}
	if _, err := Path(repoRoot, "meta/state", "20260917-123456-a0b1c2.json"); err == nil || !strings.Contains(err.Error(), "resolves outside repository root") {
		t.Fatalf("expected symlinked state root to be rejected, got %v", err)
	}
}

// TestPathRejectsDanglingSymlink covers the fail-closed promise shared with
// repopath: an unresolvable symlink component is not treated as a plain
// missing suffix (see tooling/README.md §Operation scope).
func TestPathRejectsDanglingSymlink(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repoRoot, "meta", "missing-target"), filepath.Join(repoRoot, "meta", "state")); err != nil {
		t.Fatal(err)
	}
	if _, err := Path(repoRoot, "meta/state", "20260917-123456-a0b1c2.json"); err == nil || !strings.Contains(err.Error(), "unresolvable symlink") {
		t.Fatalf("expected the dangling state root symlink to be rejected, got %v", err)
	}

	// A dangling link on the target itself (below an existing state root)
	// must also fail closed instead of being reattached lexically.
	if err := os.MkdirAll(filepath.Join(repoRoot, "meta", "state2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repoRoot, "meta", "missing.json"), filepath.Join(repoRoot, "meta", "state2", "20260917-123456-a0b1c2.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Path(repoRoot, "meta/state2", "20260917-123456-a0b1c2.json"); err == nil || !strings.Contains(err.Error(), "unresolvable symlink") {
		t.Fatalf("expected the dangling state-file symlink to be rejected, got %v", err)
	}
}

func TestWithExclusiveLockSerializesCallers(t *testing.T) {
	repoRoot := t.TempDir()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- WithExclusiveLock(repoRoot, "meta", ".test.lock", time.Second, func() error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- WithExclusiveLock(repoRoot, "meta", ".test.lock", time.Second, func() error {
			close(secondEntered)
			return nil
		})
	}()

	select {
	case <-secondEntered:
		t.Fatal("second caller entered while the first caller still held the lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first lock: %v", err)
	}
	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("second caller did not enter after the first released the lock")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second lock: %v", err)
	}
}

func TestWithExclusiveLockReleasedWhenProcessExits(t *testing.T) {
	if os.Getenv("SPECFLOW_LOCK_EXIT_HELPER") == "1" {
		repoRoot := os.Getenv("SPECFLOW_LOCK_EXIT_ROOT")
		_ = WithExclusiveLock(repoRoot, "meta", ".test.lock", time.Second, func() error {
			os.Exit(0)
			return nil
		})
		os.Exit(2)
	}

	repoRoot := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWithExclusiveLockReleasedWhenProcessExits$")
	cmd.Env = append(os.Environ(), "SPECFLOW_LOCK_EXIT_HELPER=1", "SPECFLOW_LOCK_EXIT_ROOT="+repoRoot)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lock-holder process failed: %v\n%s", err, output)
	}
	if err := WithExclusiveLock(repoRoot, "meta", ".test.lock", time.Second, func() error { return nil }); err != nil {
		t.Fatalf("lock remained held after the holder process exited: %v", err)
	}
}
