package buildrelease

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
)

type Target struct {
	GOOS   string
	GOARCH string
}

type BuildResult struct {
	Targets []string
}

var DefaultTargets = []Target{
	{GOOS: "linux", GOARCH: "amd64"},
	{GOOS: "linux", GOARCH: "arm64"},
	{GOOS: "darwin", GOARCH: "amd64"},
	{GOOS: "darwin", GOARCH: "arm64"},
	{GOOS: "windows", GOARCH: "amd64"},
	{GOOS: "windows", GOARCH: "arm64"},
}

func BinaryName(goos, goarch string) string {
	name := fmt.Sprintf("specflowctl-%s-%s", goos, goarch)
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

const PosixLauncherName = "specflowctl"

const WindowsLauncherName = "specflowctl.cmd"

const posixLauncherScript = `#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
uname_os=$(uname -s)
uname_arch=$(uname -m)
case "${uname_os}" in
  Linux) os_name="linux" ;;
  Darwin) os_name="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) os_name="windows" ;;
  *)
    echo "Error: unsupported operating system: ${uname_os}" >&2
    exit 1
    ;;
esac
case "${uname_arch}" in
  x86_64|amd64) arch_name="amd64" ;;
  aarch64|arm64) arch_name="arm64" ;;
  *)
    echo "Error: unsupported CPU architecture: ${uname_arch}" >&2
    exit 1
    ;;
esac
if [ "${os_name}" = "windows" ]; then
  target="${script_dir}/specflowctl-${os_name}-${arch_name}.exe"
else
  target="${script_dir}/specflowctl-${os_name}-${arch_name}"
fi
if [ ! -x "${target}" ]; then
  echo "Error: specflowctl binary is missing or not executable: ${target}" >&2
  echo "Run update_tooling_binaries or build-release to install specflowctl binaries." >&2
  exit 1
fi
exec "${target}" "$@"
`

const windowsLauncherScript = `@echo off
setlocal
set "specflowctl_arch=%PROCESSOR_ARCHITECTURE%"
if /I "%specflowctl_arch%"=="ARM64" (
  set "specflowctl_target=%~dp0specflowctl-windows-arm64.exe"
) else (
  set "specflowctl_target=%~dp0specflowctl-windows-amd64.exe"
)
if not exist "%specflowctl_target%" (
  echo Error: specflowctl binary is missing: "%specflowctl_target%" 1>&2
  echo Run update_tooling_binaries or build-release to install specflowctl binaries. 1>&2
  exit /b 1
)
"%specflowctl_target%" %*
`

func WriteLaunchers(binDir string) error {
	launchers := []struct {
		name    string
		content string
	}{
		{name: PosixLauncherName, content: posixLauncherScript},
		{name: WindowsLauncherName, content: windowsLauncherScript},
	}
	for _, launcher := range launchers {
		path := filepath.Join(binDir, launcher.name)
		if err := os.WriteFile(path, []byte(launcher.content), 0o755); err != nil {
			return fmt.Errorf("write launcher %s: %w", launcher.name, err)
		}
		if launcher.name == PosixLauncherName {
			if err := os.Chmod(path, 0o755); err != nil {
				return fmt.Errorf("chmod launcher %s: %w", launcher.name, err)
			}
		}
	}
	return nil
}

func CurrentBinaryName() string {
	return BinaryName(runtime.GOOS, runtime.GOARCH)
}

func BuildAll(repoRoot string, targets []Target) (BuildResult, error) {
	if len(targets) == 0 {
		targets = DefaultTargets
	}

	layout, err := specflowlayout.Resolve(repoRoot)
	if err != nil {
		return BuildResult{}, err
	}

	fingerprint, _, err := toolingfreshness.LiveFingerprint(repoRoot)
	if err != nil {
		return BuildResult{}, err
	}

	binRelative := specflowlayout.Relative(layout.ToolingRoot, "bin")
	binDir := filepath.Join(repoRoot, filepath.FromSlash(binRelative))
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return BuildResult{}, fmt.Errorf("mkdir bin dir: %w", err)
	}
	cacheDir := filepath.Join(repoRoot, ".tmp", "go-build")
	modCacheDir := filepath.Join(repoRoot, ".tmp", "go-mod-cache")

	result := BuildResult{Targets: make([]string, 0, len(targets))}
	for _, target := range targets {
		outputName := BinaryName(target.GOOS, target.GOARCH)
		outputPath := filepath.Join(binDir, outputName)
		ldflags := ldflagsForFingerprint(fingerprint)
		cmd := exec.Command("go", buildCommandArgs(ldflags, outputPath, "./cmd/specflowctl")...)
		cmd.Dir = filepath.Join(repoRoot, filepath.FromSlash(layout.ToolingRoot))
		cmd.Env = append(os.Environ(),
			"GOOS="+target.GOOS,
			"GOARCH="+target.GOARCH,
			"CGO_ENABLED=0",
			"GOCACHE="+cacheDir,
			"GOMODCACHE="+modCacheDir,
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			return result, fmt.Errorf("build %s/%s failed: %v: %s", target.GOOS, target.GOARCH, err, string(output))
		}
		result.Targets = append(result.Targets, specflowlayout.Relative(binRelative, outputName))
	}

	if err := WriteLaunchers(binDir); err != nil {
		return result, err
	}

	return result, nil
}

func ldflagsForFingerprint(fingerprint string) string {
	return fmt.Sprintf(
		"-s -w -buildid= -X github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness.BuildFingerprint=%s",
		fingerprint,
	)
}

func buildCommandArgs(ldflags, outputPath, packagePath string) []string {
	return []string{
		"build",
		"-trimpath",
		"-buildvcs=false",
		"-ldflags=" + ldflags,
		"-o",
		outputPath,
		packagePath,
	}
}
