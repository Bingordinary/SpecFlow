package buildrelease

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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

func CurrentBinaryName() string {
	return BinaryName(runtime.GOOS, runtime.GOARCH)
}

func BuildAll(repoRoot string, targets []Target) (BuildResult, error) {
	if len(targets) == 0 {
		targets = DefaultTargets
	}

	fingerprint, _, err := toolingfreshness.LiveFingerprint(repoRoot)
	if err != nil {
		return BuildResult{}, err
	}

	binDir := filepath.Join(repoRoot, "specflow/tooling/bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return BuildResult{}, fmt.Errorf("mkdir bin dir: %w", err)
	}

	result := BuildResult{Targets: make([]string, 0, len(targets))}
	for _, target := range targets {
		outputName := BinaryName(target.GOOS, target.GOARCH)
		outputPath := filepath.Join(binDir, outputName)
		ldflags := fmt.Sprintf(
			"-s -w -X github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness.BuildFingerprint=%s",
			fingerprint,
		)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags="+ldflags, "-o", outputPath, "./cmd/specflowctl")
		cmd.Dir = filepath.Join(repoRoot, "specflow/tooling")
		cmd.Env = append(os.Environ(),
			"GOOS="+target.GOOS,
			"GOARCH="+target.GOARCH,
			"CGO_ENABLED=0",
			"GOCACHE=/tmp/go-build",
			"GOMODCACHE=/tmp/go-mod-cache",
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			return result, fmt.Errorf("build %s/%s failed: %v: %s", target.GOOS, target.GOARCH, err, string(output))
		}
		result.Targets = append(result.Targets, filepath.ToSlash(filepath.Join("specflow/tooling/bin", outputName)))
	}

	return result, nil
}
