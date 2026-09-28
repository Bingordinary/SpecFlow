package buildrelease

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLdflagsForFingerprintFixesBuildID(t *testing.T) {
	ldflags := ldflagsForFingerprint("abc123")

	if !strings.Contains(ldflags, "-buildid=") {
		t.Fatalf("expected ldflags to clear Go build id, got %q", ldflags)
	}
	if !strings.Contains(ldflags, "toolingfreshness.BuildFingerprint=abc123") {
		t.Fatalf("expected ldflags to embed tooling fingerprint, got %q", ldflags)
	}
}

func TestBuildCommandArgsDisablesVCSMetadata(t *testing.T) {
	args := strings.Join(buildCommandArgs("flags", "out", "./cmd/specflowctl"), " ")

	if !strings.Contains(args, "-buildvcs=false") {
		t.Fatalf("expected build args to disable VCS metadata, got %q", args)
	}
	if !strings.Contains(args, "./cmd/specflowctl") {
		t.Fatalf("expected build args to include package path, got %q", args)
	}
}

func TestWriteLaunchersCreatesBothEntries(t *testing.T) {
	binDir := t.TempDir()

	if err := WriteLaunchers(binDir); err != nil {
		t.Fatalf("WriteLaunchers failed: %v", err)
	}

	posixPath := filepath.Join(binDir, PosixLauncherName)
	posixContent, err := os.ReadFile(posixPath)
	if err != nil {
		t.Fatalf("expected posix launcher to exist: %v", err)
	}
	if !strings.HasPrefix(string(posixContent), "#!/bin/sh\n") {
		t.Fatalf("expected posix launcher to start with shebang, got %q", string(posixContent[:20]))
	}
	for _, want := range []string{"Darwin) os_name=\"darwin\"", "MINGW*|MSYS*|CYGWIN*) os_name=\"windows\"", "exec \"${target}\" \"$@\""} {
		if !strings.Contains(string(posixContent), want) {
			t.Fatalf("expected posix launcher to contain %q", want)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(posixPath)
		if err != nil {
			t.Fatalf("stat posix launcher: %v", err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("expected posix launcher to be executable, got %v", info.Mode().Perm())
		}
	}

	windowsPath := filepath.Join(binDir, WindowsLauncherName)
	windowsContent, err := os.ReadFile(windowsPath)
	if err != nil {
		t.Fatalf("expected windows launcher to exist: %v", err)
	}
	for _, want := range []string{"specflowctl-windows-amd64.exe", "specflowctl-windows-arm64.exe", "%~dp0"} {
		if !strings.Contains(string(windowsContent), want) {
			t.Fatalf("expected windows launcher to contain %q", want)
		}
	}
}

func TestWriteLaunchersRepairsExistingNonExecutableLauncher(t *testing.T) {
	binDir := t.TempDir()
	posixPath := filepath.Join(binDir, PosixLauncherName)
	if err := os.WriteFile(posixPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed non-executable launcher failed: %v", err)
	}

	if err := WriteLaunchers(binDir); err != nil {
		t.Fatalf("WriteLaunchers failed: %v", err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(posixPath)
	if err != nil {
		t.Fatalf("stat posix launcher: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("expected pre-existing non-executable launcher to be repaired, got %v", info.Mode().Perm())
	}
}
