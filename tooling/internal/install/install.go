package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

const (
	codexHooksSource      = "templates/.codex/hooks.json"
	codexHooksDestination = ".codex/hooks.json"
	codexHookMarker       = "session-start codex"
	codexHookRunnerMarker = "specflow/hooks/run-hook.cmd"
)

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
		{"templates/.agents/plugins/specflow/plugin.json", ".agents/plugins/specflow/plugin.json"},
		{"templates/.agents/plugins/specflow/hooks.json", ".agents/plugins/specflow/hooks.json"},
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

	codexUpdated, err := installCodexHooks(repoRoot, layout)
	if err != nil {
		return result, err
	}
	if codexUpdated {
		result.Copied++
	}

	return result, nil
}

func installCodexHooks(repoRoot string, layout specflowlayout.Layout) (bool, error) {
	source := filepath.Join(repoRoot, specflowlayout.Relative(layout.ContentRoot, codexHooksSource))
	template, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", codexHooksSource, err)
	}
	if !json.Valid(template) {
		return false, fmt.Errorf("parse %s: invalid JSON", codexHooksSource)
	}

	destination := filepath.Join(repoRoot, filepath.FromSlash(codexHooksDestination))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", codexHooksDestination, err)
	}

	existing, err := os.ReadFile(destination)
	if os.IsNotExist(err) {
		if err := os.WriteFile(destination, template, 0o644); err != nil {
			return false, fmt.Errorf("write %s: %w", codexHooksDestination, err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", codexHooksDestination, err)
	}

	merged, err := mergeCodexHooks(existing, template)
	if err != nil {
		return false, err
	}
	if jsonDocumentsEqual(existing, merged) {
		return false, nil
	}
	if err := os.WriteFile(destination, merged, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", codexHooksDestination, err)
	}
	return true, nil
}

func mergeCodexHooks(existing, template []byte) ([]byte, error) {
	existingDocument, err := parseJSONDocument(existing, codexHooksDestination)
	if err != nil {
		return nil, err
	}
	templateDocument, err := parseJSONDocument(template, codexHooksSource)
	if err != nil {
		return nil, err
	}

	existingHooks, err := parseJSONMap(existingDocument["hooks"], codexHooksDestination+" hooks")
	if err != nil {
		return nil, err
	}
	templateHooks, err := parseJSONMap(templateDocument["hooks"], codexHooksSource+" hooks")
	if err != nil {
		return nil, err
	}

	existingSessionStart, err := parseJSONArray(existingHooks["SessionStart"], codexHooksDestination+" SessionStart")
	if err != nil {
		return nil, err
	}
	templateSessionStart, err := parseJSONArray(templateHooks["SessionStart"], codexHooksSource+" SessionStart")
	if err != nil {
		return nil, err
	}
	if len(templateSessionStart) == 0 {
		return nil, fmt.Errorf("parse %s: missing SessionStart hook", codexHooksSource)
	}

	mergedSessionStart := make([]json.RawMessage, 0, len(existingSessionStart)+len(templateSessionStart))
	for _, entry := range existingSessionStart {
		if !isManagedCodexHook(entry) {
			mergedSessionStart = append(mergedSessionStart, entry)
		}
	}
	mergedSessionStart = append(mergedSessionStart, templateSessionStart...)

	sessionStartJSON, err := json.Marshal(mergedSessionStart)
	if err != nil {
		return nil, fmt.Errorf("encode %s SessionStart: %w", codexHooksDestination, err)
	}
	existingHooks["SessionStart"] = sessionStartJSON
	hooksJSON, err := json.Marshal(existingHooks)
	if err != nil {
		return nil, fmt.Errorf("encode %s hooks: %w", codexHooksDestination, err)
	}
	existingDocument["hooks"] = hooksJSON

	merged, err := json.MarshalIndent(existingDocument, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", codexHooksDestination, err)
	}
	return append(merged, '\n'), nil
}

func isManagedCodexHook(entry json.RawMessage) bool {
	return bytes.Contains(entry, []byte(codexHookRunnerMarker)) && bytes.Contains(entry, []byte(codexHookMarker))
}

func parseJSONDocument(content []byte, name string) (map[string]json.RawMessage, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	if document == nil {
		return nil, fmt.Errorf("parse %s: expected JSON object", name)
	}
	return document, nil
}

func parseJSONMap(content json.RawMessage, name string) (map[string]json.RawMessage, error) {
	if len(content) == 0 || bytes.Equal(bytes.TrimSpace(content), []byte("null")) {
		return map[string]json.RawMessage{}, nil
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	if value == nil {
		value = map[string]json.RawMessage{}
	}
	return value, nil
}

func parseJSONArray(content json.RawMessage, name string) ([]json.RawMessage, error) {
	if len(content) == 0 || bytes.Equal(bytes.TrimSpace(content), []byte("null")) {
		return nil, nil
	}
	var value []json.RawMessage
	if err := json.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	return value, nil
}

func jsonDocumentsEqual(left, right []byte) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
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

func shortFingerprint(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
