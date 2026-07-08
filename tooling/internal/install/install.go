package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/buildrelease"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/manifest"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
)

type InitResult struct {
	Copied  int
	Skipped int
}

func Init(repoRoot string, force bool) (InitResult, error) {
	layout, err := specflowlayout.Resolve(repoRoot)
	if err != nil {
		return InitResult{}, err
	}
	items, err := manifest.Load(repoRoot)
	if err != nil {
		return InitResult{}, err
	}

	result := InitResult{}
	for _, item := range items {
		sourceRelative := specflowlayout.Relative(layout.ContentRoot, item.SourceRelative)
		source := filepath.Join(repoRoot, filepath.FromSlash(sourceRelative))
		dest := filepath.Join(repoRoot, filepath.FromSlash(item.DestinationRelative))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return result, fmt.Errorf("mkdir %s: %w", item.DestinationRelative, err)
		}
		if _, err := os.Stat(dest); err == nil && !force {
			result.Skipped++
			continue
		}
		if err := copyFile(source, dest); err != nil {
			return result, fmt.Errorf("copy %s: %w", item.DestinationRelative, err)
		}
		result.Copied++
	}

	return result, nil
}

type DoctorResult struct {
	Failures []string
	Warnings []string
}

type HooksResult struct {
	Copied int
}

func InstallHooks(repoRoot string) (HooksResult, error) {
	result := HooksResult{}

	type hookFile struct {
		source string
		dest   string
	}

	files := []hookFile{
		{"hooks/hooks.json", "hooks/hooks.json"},
		{"hooks/run-hook.cmd", "specflow/hooks/run-hook.cmd"},
		{"hooks/session-start", "specflow/hooks/session-start"},
		{"templates/.claude-plugin/plugin.json", ".claude-plugin/plugin.json"},
		{"templates/.opencode/plugins/specflow.js", ".opencode/plugins/specflow.js"},
	}

	layout, err := specflowlayout.Resolve(repoRoot)
	if err != nil {
		return result, err
	}

	for _, f := range files {
		source := filepath.Join(repoRoot, specflowlayout.Relative(layout.ContentRoot, f.source))
		if _, err := os.Stat(source); os.IsNotExist(err) {
			continue
		}
		dest := filepath.Join(repoRoot, filepath.FromSlash(f.dest))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return result, fmt.Errorf("mkdir %s: %w", f.dest, err)
		}
		srcContent, err := os.ReadFile(source)
		if err != nil {
			return result, fmt.Errorf("read %s: %w", f.source, err)
		}

		dstContent, err := os.ReadFile(dest)
		if err != nil || !bytes.Equal(srcContent, dstContent) {
			if err := os.WriteFile(dest, srcContent, 0o644); err != nil {
				return result, fmt.Errorf("write %s: %w", f.dest, err)
			}
			result.Copied++
		}
	}

	return result, nil
}

func Doctor(repoRoot string) (DoctorResult, error) {
	layout, err := specflowlayout.Resolve(repoRoot)
	if err != nil {
		return DoctorResult{}, err
	}
	items, err := manifest.Load(repoRoot)
	if err != nil {
		return DoctorResult{}, err
	}

	result := DoctorResult{}
	for _, item := range items {
		if item.Mode != "framework" {
			continue
		}
		expectedRelative := item.DestinationRelative
		if layout.Kind == specflowlayout.SourceRepo {
			expectedRelative = specflowlayout.Relative(layout.ContentRoot, item.SourceRelative)
		}
		expected := filepath.Join(repoRoot, filepath.FromSlash(expectedRelative))
		if _, err := os.Stat(expected); err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("MISSING %s", expectedRelative))
		}
	}

	checkBinary(repoRoot, layout, &result)
	checkReaderWeb(repoRoot, layout, &result)
	return result, nil
}

// CheckProjectInit checks that project-mode manifest files exist in the project.
// Unlike Doctor (framework integrity), this verifies the project initialization
// state — i.e. whether 'specflowctl init' has been run successfully.
func CheckProjectInit(repoRoot string) (DoctorResult, error) {
	layout, err := specflowlayout.Resolve(repoRoot)
	if err != nil {
		return DoctorResult{}, err
	}
	items, err := manifest.Load(repoRoot)
	if err != nil {
		return DoctorResult{}, err
	}

	result := DoctorResult{}
	for _, item := range items {
		if item.Mode != "project" {
			continue
		}
		expectedRelative := item.DestinationRelative
		if layout.Kind == specflowlayout.SourceRepo {
			expectedRelative = specflowlayout.Relative(layout.ContentRoot, item.SourceRelative)
		}
		expected := filepath.Join(repoRoot, filepath.FromSlash(expectedRelative))
		if _, err := os.Stat(expected); err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("MISSING %s (project init file)", expectedRelative))
		}
	}
	return result, nil
}

func copyFile(source, dest string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, content, 0o644)
}

func checkBinary(repoRoot string, layout specflowlayout.Layout, result *DoctorResult) {
	checkOneBinary(repoRoot, specflowlayout.Relative(layout.ToolingRoot, filepath.ToSlash(filepath.Join("bin", buildrelease.CurrentBinaryName()))), result)
	checkOneBinary(repoRoot, specflowlayout.Relative(layout.ToolingRoot, filepath.ToSlash(filepath.Join("bin", buildrelease.CurrentReaderBinaryName()))), result)
}

func checkOneBinary(repoRoot, relPath string, result *DoctorResult) {
	binaryPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	if _, err := os.Stat(binaryPath); err != nil {
		result.Failures = append(result.Failures, fmt.Sprintf("MISSING %s", relPath))
		return
	}

	liveFingerprint, _, err := toolingfreshness.LiveFingerprint(repoRoot)
	if err != nil {
		result.Failures = append(result.Failures, fmt.Sprintf("INVALID tooling live fingerprint: %v", err))
		return
	}

	builtFingerprint, err := toolingfreshness.ReadBuildFingerprintFromBinary(binaryPath)
	if err != nil {
		result.Failures = append(result.Failures, fmt.Sprintf("INVALID %s freshness probe failed: %v", relPath, err))
		return
	}
	if strings.TrimSpace(builtFingerprint) == "" {
		result.Failures = append(result.Failures, fmt.Sprintf("INVALID %s missing embedded build fingerprint", relPath))
		return
	}
	if strings.TrimSpace(builtFingerprint) != strings.TrimSpace(liveFingerprint) {
		result.Failures = append(result.Failures, fmt.Sprintf(
			"STALE %s built_fingerprint=%s live_fingerprint=%s",
			relPath,
			shortFingerprint(builtFingerprint),
			shortFingerprint(liveFingerprint),
		))
	}
}

func checkReaderWeb(repoRoot string, layout specflowlayout.Layout, result *DoctorResult) {
	for _, asset := range []string{
		"index.html",
		"styles.css",
		"app.js",
		"cytoscape.min.js",
		"mermaid.min.js",
	} {
		relPath := specflowlayout.Relative(layout.ToolingRoot, filepath.ToSlash(filepath.Join("reader", "web", asset)))
		path := filepath.Join(repoRoot, filepath.FromSlash(relPath))
		info, err := os.Stat(path)
		if err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("MISSING %s", relPath))
			continue
		}
		if info.IsDir() {
			result.Failures = append(result.Failures, fmt.Sprintf("INVALID %s is a directory", relPath))
		}
	}
}

func shortFingerprint(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
