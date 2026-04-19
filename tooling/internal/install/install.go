package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/buildrelease"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/managedblock"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/manifest"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
)

type InitResult struct {
	Copied  int
	Skipped int
}

type UpgradeResult struct {
	Updated int
	Skipped int
}

type DoctorResult struct {
	Failures []string
	Warnings []string
}

func Init(repoRoot string, force bool) (InitResult, error) {
	items, err := manifest.Load(repoRoot)
	if err != nil {
		return InitResult{}, err
	}

	result := InitResult{}
	for _, item := range items {
		source := filepath.Join(repoRoot, "specflow", filepath.FromSlash(item.SourceRelative))
		dest := filepath.Join(repoRoot, filepath.FromSlash(item.DestinationRelative))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return result, fmt.Errorf("mkdir %s: %w", item.DestinationRelative, err)
		}
		if _, err := os.Stat(dest); err == nil && !force {
			result.Skipped++
			continue
		}

		if _, err := os.Stat(dest); err == nil && isManagedEntryFile(item.DestinationRelative) {
			if err := replaceManagedBlockFile(source, dest); err != nil {
				return result, fmt.Errorf("install managed block %s: %w", item.DestinationRelative, err)
			}
			result.Copied++
			continue
		}

		if err := copyFile(source, dest); err != nil {
			return result, fmt.Errorf("copy %s: %w", item.DestinationRelative, err)
		}
		result.Copied++
	}

	return result, nil
}

func Upgrade(repoRoot string) (UpgradeResult, error) {
	items, err := manifest.Load(repoRoot)
	if err != nil {
		return UpgradeResult{}, err
	}

	result := UpgradeResult{}
	for _, item := range items {
		source := filepath.Join(repoRoot, "specflow", filepath.FromSlash(item.SourceRelative))
		dest := filepath.Join(repoRoot, filepath.FromSlash(item.DestinationRelative))
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return result, fmt.Errorf("mkdir %s: %w", item.DestinationRelative, err)
			}
			if err := copyFile(source, dest); err != nil {
				return result, fmt.Errorf("install missing %s: %w", item.DestinationRelative, err)
			}
			result.Updated++
			continue
		}

		if isManagedEntryFile(item.DestinationRelative) {
			sourceContent, err := os.ReadFile(source)
			if err != nil {
				return result, err
			}
			destContent, err := os.ReadFile(dest)
			if err != nil {
				return result, err
			}
			sourceBlock, err := managedblock.Extract(string(sourceContent))
			if err != nil {
				return result, err
			}
			destBlock, err := managedblock.Extract(string(destContent))
			if err != nil {
				return result, err
			}
			if sourceBlock == destBlock {
				continue
			}
			if err := replaceManagedBlockFile(source, dest); err != nil {
				return result, fmt.Errorf("update managed block %s: %w", item.DestinationRelative, err)
			}
			result.Updated++
			continue
		}

		if item.Mode != "framework" {
			result.Skipped++
			continue
		}
		same, err := fileContentsEqual(source, dest)
		if err != nil {
			return result, err
		}
		if same {
			continue
		}
		if err := copyFile(source, dest); err != nil {
			return result, fmt.Errorf("update %s: %w", item.DestinationRelative, err)
		}
		result.Updated++
	}

	return result, nil
}

func Doctor(repoRoot string) (DoctorResult, error) {
	items, err := manifest.Load(repoRoot)
	if err != nil {
		return DoctorResult{}, err
	}

	result := DoctorResult{}
	for _, item := range items {
		dest := filepath.Join(repoRoot, filepath.FromSlash(item.DestinationRelative))
		if _, err := os.Stat(dest); err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("MISSING %s", item.DestinationRelative))
		}
	}

	if err := checkManagedEntryConsistency(repoRoot, &result); err != nil {
		return result, err
	}
	checkHookPath(repoRoot, &result)
	checkBinary(repoRoot, &result)
	checkHook(repoRoot, &result)
	return result, nil
}

func isManagedEntryFile(path string) bool {
	switch filepath.ToSlash(path) {
	case "AGENTS.md", "GEMINI.md", "CLAUDE.md":
		return true
	default:
		return false
	}
}

func replaceManagedBlockFile(source, dest string) error {
	sourceContent, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	destContent, err := os.ReadFile(dest)
	if err != nil {
		return err
	}
	block, err := managedblock.Extract(string(sourceContent))
	if err != nil {
		return err
	}
	updated, err := managedblock.Replace(string(destContent), block)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(updated), 0o644)
}

func copyFile(source, dest string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, content, 0o644)
}

func fileContentsEqual(left, right string) (bool, error) {
	leftContent, err := os.ReadFile(left)
	if err != nil {
		return false, err
	}
	rightContent, err := os.ReadFile(right)
	if err != nil {
		return false, err
	}
	return string(leftContent) == string(rightContent), nil
}

func checkManagedEntryConsistency(repoRoot string, result *DoctorResult) error {
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	if _, err := os.Stat(agentsPath); err != nil {
		return nil
	}

	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		return err
	}
	agentsBlock, err := managedblock.Extract(string(agentsContent))
	if err != nil {
		result.Failures = append(result.Failures, "INVALID managed block in AGENTS.md")
		return nil
	}

	for _, peer := range []string{"GEMINI.md", "CLAUDE.md"} {
		peerPath := filepath.Join(repoRoot, peer)
		if _, err := os.Stat(peerPath); err != nil {
			continue
		}
		peerContent, err := os.ReadFile(peerPath)
		if err != nil {
			return err
		}
		peerBlock, err := managedblock.Extract(string(peerContent))
		if err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("INVALID managed block in %s", peer))
			continue
		}
		if agentsBlock != peerBlock {
			result.Failures = append(result.Failures, fmt.Sprintf("DIFF managed blocks in AGENTS.md and %s", peer))
		}
	}
	return nil
}

func checkHookPath(repoRoot string, result *DoctorResult) {
	cmd := exec.Command("git", "-C", repoRoot, "config", "--get", "core.hooksPath")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	if strings.TrimSpace(string(output)) != ".githooks" {
		result.Warnings = append(result.Warnings, "WARN git core.hooksPath is not .githooks")
	}
}

func checkBinary(repoRoot string, result *DoctorResult) {
	relPath := filepath.ToSlash(filepath.Join("specflow/tooling/bin", buildrelease.CurrentBinaryName()))
	binaryPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	if _, err := os.Stat(binaryPath); err != nil {
		result.Failures = append(result.Failures, fmt.Sprintf("MISSING %s", relPath))
		return
	}

	if !toolingfreshness.IsToolingRepo(repoRoot) {
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

func checkHook(repoRoot string, result *DoctorResult) {
	hookPath := filepath.Join(repoRoot, ".githooks/pre-commit")
	content, err := os.ReadFile(hookPath)
	if err != nil {
		return
	}
	text := string(content)
	if !strings.Contains(text, "specflow/tooling/bin/specflowctl-") || !strings.Contains(text, "entry sync --stage") {
		result.Failures = append(result.Failures, "INVALID .githooks/pre-commit does not call specflow binary entry sync")
	}
}

func shortFingerprint(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
