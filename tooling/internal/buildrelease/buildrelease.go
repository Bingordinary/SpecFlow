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
