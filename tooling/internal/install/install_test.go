package install

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/buildrelease"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
)

func TestDoctorPassesForFreshBinary(t *testing.T) {
	repoRoot := t.TempDir()
	liveFingerprint := setupDoctorRepo(t, repoRoot)
	writeFingerprintProbeBinary(t, repoRoot, liveFingerprint)

	result, err := Doctor(repoRoot)
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures, got %v", result.Failures)
	}
}

func TestDoctorSkipsProjectModeItems(t *testing.T) {
	repoRoot := t.TempDir()
	setupDoctorRepoAt(t, repoRoot, "specflow", "specflow/tooling")
	// Add a project-mode entry to the manifest (doctor should skip it)
	addManifestEntry(t, repoRoot, "specflow/tooling/manifest.tsv", "templates/docs/mapping.md\tdocs/mapping.md\tproject")
	// Don't create the project file — doctor should not report it
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/templates/docs/mapping.md"), "repo mapping\n")

	liveFingerprint := setupDoctorRepoLiveFingerprint(t, repoRoot)
	writeFingerprintProbeBinary(t, repoRoot, liveFingerprint)

	result, err := Doctor(repoRoot)
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	for _, failure := range result.Failures {
		if strings.Contains(failure, "docs/mapping.md") {
			t.Fatalf("Doctor reported project-mode file as missing: %s", failure)
		}
	}
}

func TestDoctorFailsForStaleBinary(t *testing.T) {
	repoRoot := t.TempDir()
	_ = setupDoctorRepo(t, repoRoot)
	writeFingerprintProbeBinary(t, repoRoot, "stale-binary-fingerprint")

	result, err := Doctor(repoRoot)
	if err != nil {
		t.Fatalf("Doctor returned unexpected error: %v", err)
	}

	joined := strings.Join(result.Failures, "\n")
	if !strings.Contains(joined, "STALE specflow/tooling/bin/"+buildrelease.CurrentBinaryName()) {
		t.Fatalf("expected stale binary failure, got %v", result.Failures)
	}
}

func TestInstallHooksCreatesCodexHooks(t *testing.T) {
	repoRoot := t.TempDir()
	setupCodexHookRepo(t, repoRoot)

	result, err := InstallHooks(repoRoot)
	if err != nil {
		t.Fatalf("InstallHooks returned error: %v", err)
	}
	if result.Copied != 1 {
		t.Fatalf("expected one installed hook file, got %d", result.Copied)
	}

	content, err := os.ReadFile(filepath.Join(repoRoot, ".codex/hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(.codex/hooks.json) failed: %v", err)
	}
	if !strings.Contains(string(content), "session-start codex") {
		t.Fatalf("installed Codex hook is missing the SpecFlow command: %s", content)
	}
	if strings.Contains(string(content), "additionalContextLimit") {
		t.Fatalf("installed Codex hook must use the platform default context limit: %s", content)
	}
}

func TestInstallHooksMergesCodexHooks(t *testing.T) {
	repoRoot := t.TempDir()
	setupCodexHookRepo(t, repoRoot)
	existing := `{
  "customSetting": true,
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [{"type": "command", "command": "./keep-me"}]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [{"type": "command", "command": "./also-keep-me"}]
      }
    ]
  }
}
`
	mustWriteFile(t, filepath.Join(repoRoot, ".codex/hooks.json"), existing)

	result, err := InstallHooks(repoRoot)
	if err != nil {
		t.Fatalf("InstallHooks returned error: %v", err)
	}
	if result.Copied != 1 {
		t.Fatalf("expected one updated hook file, got %d", result.Copied)
	}

	content, err := os.ReadFile(filepath.Join(repoRoot, ".codex/hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(.codex/hooks.json) failed: %v", err)
	}
	for _, preserved := range []string{"customSetting", "./keep-me", "UserPromptSubmit", "./also-keep-me"} {
		if !strings.Contains(string(content), preserved) {
			t.Fatalf("merged Codex hooks lost %q: %s", preserved, content)
		}
	}
	if count := strings.Count(string(content), `"matcher": "startup|resume|clear|compact"`); count != 1 {
		t.Fatalf("expected exactly one SpecFlow Codex hook, got %d: %s", count, content)
	}
}

func TestInstallHooksReplacesManagedCodexHookAndIsIdempotent(t *testing.T) {
	repoRoot := t.TempDir()
	setupCodexHookRepo(t, repoRoot)
	existing := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [{"type": "command", "command": "bash /old/specflow/hooks/run-hook.cmd session-start codex"}]
      },
      {
        "matcher": "resume",
        "hooks": [{"type": "command", "command": "./keep-me"}]
      }
    ]
  }
}
`
	mustWriteFile(t, filepath.Join(repoRoot, ".codex/hooks.json"), existing)

	first, err := InstallHooks(repoRoot)
	if err != nil {
		t.Fatalf("first InstallHooks returned error: %v", err)
	}
	if first.Copied != 1 {
		t.Fatalf("expected first install to update one file, got %d", first.Copied)
	}

	content, err := os.ReadFile(filepath.Join(repoRoot, ".codex/hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(.codex/hooks.json) failed: %v", err)
	}
	if strings.Contains(string(content), "old-runner") {
		t.Fatalf("stale managed Codex hook was not replaced: %s", content)
	}
	if count := strings.Count(string(content), `"matcher": "startup|resume|clear|compact"`); count != 1 {
		t.Fatalf("expected exactly one SpecFlow Codex hook, got %d: %s", count, content)
	}
	if !strings.Contains(string(content), "./keep-me") {
		t.Fatalf("unrelated SessionStart hook was not preserved: %s", content)
	}

	second, err := InstallHooks(repoRoot)
	if err != nil {
		t.Fatalf("second InstallHooks returned error: %v", err)
	}
	if second.Copied != 0 {
		t.Fatalf("expected second install to be idempotent, copied %d files", second.Copied)
	}
}

func TestInstallHooksRejectsInvalidExistingCodexHooks(t *testing.T) {
	repoRoot := t.TempDir()
	setupCodexHookRepo(t, repoRoot)
	target := filepath.Join(repoRoot, ".codex/hooks.json")
	invalid := []byte("{not valid json\n")
	mustWriteFile(t, target, string(invalid))

	_, err := InstallHooks(repoRoot)
	if err == nil {
		t.Fatal("expected invalid existing .codex/hooks.json to be rejected")
	}
	after, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("ReadFile(.codex/hooks.json) failed: %v", readErr)
	}
	if string(after) != string(invalid) {
		t.Fatalf("invalid existing Codex hooks were overwritten: %q", after)
	}
}

func TestSessionStartOutputsCodexAdditionalContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash hook test is not supported on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not available")
	}

	hookPath := filepath.Join("..", "..", "..", "hooks", "session-start")
	output, err := exec.Command("bash", hookPath, "codex").Output()
	if err != nil {
		t.Fatalf("Codex session-start hook failed: %v", err)
	}
	var envelope struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("Codex session-start output is not valid JSON: %v\n%s", err, output)
	}
	if envelope.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Fatalf("unexpected hook event name: %q", envelope.HookSpecificOutput.HookEventName)
	}
	if !strings.Contains(envelope.HookSpecificOutput.AdditionalContext, "<SPECFLOW_CONCEPTS>") {
		t.Fatalf("Codex output is missing SpecFlow context: %s", output)
	}
}

const codexHookTemplateForTest = `{
  "description": "SpecFlow governance context for Codex",
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"$(git rev-parse --show-toplevel)/specflow/hooks/run-hook.cmd\" session-start codex",
            "commandWindows": "powershell.exe -NoProfile -Command \"$repo = git rev-parse --show-toplevel; & (Join-Path $repo 'specflow/hooks/run-hook.cmd') session-start codex\"",
            "async": false
          }
        ]
      }
    ]
  }
}
`

func setupCodexHookRepo(t *testing.T, repoRoot string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/templates/.codex/hooks.json"), codexHookTemplateForTest)
}

func setupDoctorRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	return setupDoctorRepoAt(t, repoRoot, "specflow", "specflow/tooling")
}

func setupDoctorRepoAt(t *testing.T, repoRoot, contentRoot, toolingRoot string) string {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "manifest.tsv"), strings.Join([]string{
		"templates/AGENTS.md\tAGENTS.md\tframework",
	}, "\n")+"\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(contentRoot), "templates/AGENTS.md"), "template\n==SPECFLOW:BEGIN==\nmanaged\n==SPECFLOW:END==\n")
	mustWriteFile(t, filepath.Join(repoRoot, "AGENTS.md"), "host\n==SPECFLOW:BEGIN==\nmanaged\n==SPECFLOW:END==\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "cmd/specflowctl/main.go"), "package main\n\nfunc main() {}\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "internal/demo/demo.go"), "package demo\n\nfunc Value() string { return \"demo\" }\n")

	return setupDoctorRepoLiveFingerprint(t, repoRoot)
}

func setupDoctorRepoLiveFingerprint(t *testing.T, repoRoot string) string {
	t.Helper()
	fingerprint, _, err := toolingfreshness.LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}
	return fingerprint
}

func addManifestEntry(t *testing.T, repoRoot, manifestPath, line string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(repoRoot, filepath.FromSlash(manifestPath)), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(manifest.tsv) failed: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("WriteString(manifest.tsv) failed: %v", err)
	}
}

func writeFingerprintProbeBinary(t *testing.T, repoRoot, fingerprint string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("script-based executable probe is not supported on windows")
	}
	script := "#!/usr/bin/env bash\nif [[ \"$1\" == \"" + toolingfreshness.HiddenBuildFingerprintCommand + "\" ]]; then\n  printf '%s\\n' \"" + fingerprint + "\"\n  exit 0\nfi\nexit 0\n"
	mustWriteExecutableFile(t, filepath.Join(repoRoot, "specflow/tooling/bin", buildrelease.CurrentBinaryName()), script)
}

func TestCheckProjectInitPassesWhenProjectFilesPresent(t *testing.T) {
	repoRoot := t.TempDir()
	// Setup a repo as InstalledProject with a project-mode manifest entry
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), strings.Join([]string{
		"templates/docs/guide.md\tdocs/guide.md\tproject",
	}, "\n")+"\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	// Create both the template source and the destination project file
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/templates/docs/guide.md"), "guide template\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/guide.md"), "guide project file\n")

	result, err := CheckProjectInit(repoRoot)
	if err != nil {
		t.Fatalf("CheckProjectInit returned error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures, got %v", result.Failures)
	}
}

func TestCheckProjectInitFailsWhenProjectFileMissing(t *testing.T) {
	repoRoot := t.TempDir()
	// Setup a repo as InstalledProject with a project-mode manifest entry
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), strings.Join([]string{
		"templates/docs/guide.md\tdocs/guide.md\tproject",
	}, "\n")+"\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	// Create the template but NOT the destination project file
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/templates/docs/guide.md"), "guide template\n")

	result, err := CheckProjectInit(repoRoot)
	if err != nil {
		t.Fatalf("CheckProjectInit returned error: %v", err)
	}
	if len(result.Failures) == 0 {
		t.Fatal("expected CheckProjectInit to report missing project file, but got no failures")
	}
	joined := strings.Join(result.Failures, "\n")
	if !strings.Contains(joined, "MISSING docs/guide.md") {
		t.Fatalf("expected MISSING docs/guide.md, got %v", result.Failures)
	}
}

func TestCheckProjectInitSkipsFrameworkModeItems(t *testing.T) {
	repoRoot := t.TempDir()
	// Setup a repo as InstalledProject with ONLY framework-mode entries
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), strings.Join([]string{
		"templates/AGENTS.md\tAGENTS.md\tframework",
	}, "\n")+"\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")

	result, err := CheckProjectInit(repoRoot)
	if err != nil {
		t.Fatalf("CheckProjectInit returned error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures for framework-only manifest, got %v", result.Failures)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", path, err)
	}
}

func mustWriteExecutableFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", path, err)
	}
}
