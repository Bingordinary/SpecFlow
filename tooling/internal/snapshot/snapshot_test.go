package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func TestRebuildCurrentCollectsAppendixAndSharedSnapshot(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/system/stable"))

	status := "# Spec Status\n\n## Formal Modules\n\n| Module | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|\n| `module_demo` | `no` | `yes` | `candidate` | `cand_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpec := `---
id: module_demo
layer: candidate
version: 0.1.0
---

# Demo

See [appendix](./appendix/c_module_demo_prompt.md).

## Global Constraint Alignment

1. ` + "`system_constraints_stable_ref`: `s_system_constraints@1.1.0`" + `
2. ` + "`shared_contract_refs`:" + `
   - ` + "`c_shared_demo@0.2.0`" + `
`
	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), mainSpec)

	appendix := `---
module: module_demo
layer: candidate
spec_version_ref: c_module_demo@0.1.0
---

# Appendix
`
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir), "c_module_demo_prompt.md"), appendix)

	shared := `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.2.0
bound_modules:
  - module_demo
system_constraints_stable_ref: s_system_constraints@1.1.0
---

# Shared
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), shared)

	system := `---
version: 1.1.0
---

# System
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/system/stable/s_system_constraints.md"), system)

	result, err := RebuildCurrent(repoRoot, "module_demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	if result.SpecFileRef != mainSpecRef {
		t.Fatalf("unexpected spec file ref: %s", result.SpecFileRef)
	}
	if result.SpecVersionRef != "c_module_demo@0.1.0" {
		t.Fatalf("unexpected spec version ref: %s", result.SpecVersionRef)
	}
	if len(result.ModuleAppendixSnapshot) != 1 {
		t.Fatalf("expected one appendix snapshot entry, got %d", len(result.ModuleAppendixSnapshot))
	}
	if result.ModuleAppendixSnapshot[0].AppendixRef != "c_module_demo_prompt@c_module_demo@0.1.0" {
		t.Fatalf("unexpected appendix ref: %s", result.ModuleAppendixSnapshot[0].AppendixRef)
	}
	if result.SystemConstraintsStableVersionRef != "s_system_constraints@1.1.0" {
		t.Fatalf("unexpected system constraints version ref: %s", result.SystemConstraintsStableVersionRef)
	}
	if len(result.SharedContractSnapshot) != 1 {
		t.Fatalf("expected one shared snapshot entry, got %d", len(result.SharedContractSnapshot))
	}
	if result.SharedContractSnapshot[0].VersionRef != "c_shared_demo@0.2.0" {
		t.Fatalf("unexpected shared version ref: %s", result.SharedContractSnapshot[0].VersionRef)
	}
}

func TestRebuildCurrentCollectsEquivalentAppendixSubdirAndPlainFieldNames(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir), "support"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/system/stable"))

	status := "# Spec Status\n\n## Formal Modules\n\n| Module | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|\n| `module_demo` | `no` | `yes` | `candidate` | `cand_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpec := `---
id: module_demo
layer: candidate
version: 0.1.0
---

# Demo

See [support](./support/c_module_demo_prompt.md).

## Global Constraint Alignment

1. system_constraints_stable_ref: s_system_constraints@1.1.0
2. shared_contract_refs:
   - c_shared_demo@0.2.0
`
	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), mainSpec)

	appendix := `---
module: module_demo
layer: candidate
spec_version_ref: c_module_demo@0.1.0
---

# Appendix
`
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir), "support", "c_module_demo_prompt.md"), appendix)

	shared := `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.2.0
bound_modules:
  - module_demo
system_constraints_stable_ref: s_system_constraints@1.1.0
---

# Shared
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), shared)

	system := `---
version: 1.1.0
---

# System
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/system/stable/s_system_constraints.md"), system)

	result, err := RebuildCurrent(repoRoot, "module_demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	if len(result.ModuleAppendixSnapshot) != 1 {
		t.Fatalf("expected one appendix snapshot entry, got %d", len(result.ModuleAppendixSnapshot))
	}
	if result.ModuleAppendixSnapshot[0].FileRef != "docs/specs/modules/candidate/support/c_module_demo_prompt.md" {
		t.Fatalf("unexpected appendix file ref: %s", result.ModuleAppendixSnapshot[0].FileRef)
	}
	if len(result.SharedContractSnapshot) != 1 || result.SharedContractSnapshot[0].VersionRef != "c_shared_demo@0.2.0" {
		t.Fatalf("unexpected shared snapshot: %+v", result.SharedContractSnapshot)
	}
	if result.SystemConstraintsStableVersionRef != "s_system_constraints@1.1.0" {
		t.Fatalf("unexpected system constraints version ref: %s", result.SystemConstraintsStableVersionRef)
	}
}

func TestRebuildCurrentIgnoresRawAppendixPathLiteral(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir)))

	status := "# Spec Status\n\n## Formal Modules\n\n| Module | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|\n| `module_demo` | `no` | `yes` | `candidate` | `cand_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
	id: module_demo
	layer: candidate
	version: 0.1.0
---

# Demo

Use appendix path `+"`./appendix/c_module_demo_prompt.md`"+` for detailed prompts.

## Global Constraint Alignment

1. system_constraints_stable_ref: none
2. shared_contract_refs: none
`)

	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir), "c_module_demo_prompt.md"), `---
module: module_demo
layer: candidate
spec_version_ref: c_module_demo@0.1.0
---

	# Appendix
`)

	result, err := RebuildCurrent(repoRoot, "module_demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	if len(result.ModuleAppendixSnapshot) != 0 {
		t.Fatalf("expected raw path literal to be ignored, got %+v", result.ModuleAppendixSnapshot)
	}
}

func TestRebuildCurrentRejectsRootDirectoryAppendixDrift(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir)))

	status := "# Spec Status\n\n## Formal Modules\n\n| Module | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|\n| `module_demo` | `no` | `yes` | `candidate` | `cand_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: module_demo
layer: candidate
version: 0.1.0
---

# Demo

See [support](./c_module_demo_prompt.md).

## Global Constraint Alignment

1. system_constraints_stable_ref: none
2. shared_contract_refs: none
`)

	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir), "c_module_demo_prompt.md"), `---
module: module_demo
layer: candidate
spec_version_ref: c_module_demo@0.1.0
---

# Drift
`)

	_, err = RebuildCurrent(repoRoot, "module_demo")
	if err == nil || !strings.Contains(err.Error(), "directory drift") {
		t.Fatalf("expected directory drift error, got %v", err)
	}
}

func TestValidateProcessFileRejectsMissingRequiredSnapshotField(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	writeCheckProcessFile(t, repoRoot, strings.Join([]string{
		"spec_file_ref: docs/specs/modules/candidate/c_module_demo.md",
		"spec_version_ref: c_module_demo@0.1.0",
		"module_appendix_snapshot: none",
		"system_constraints_stable_file_ref: none",
		"system_constraints_stable_version_ref: none",
		"system_constraints_stable_fingerprint: none",
		"shared_contract_snapshot: none",
	}, "\n"))

	result, err := ValidateProcessFile(repoRoot, "module_demo", "check")
	if err != nil {
		t.Fatalf("ValidateProcessFile: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result, got valid")
	}
	if !containsMismatch(result.Mismatches, "missing required field: spec_fingerprint") {
		t.Fatalf("expected missing spec_fingerprint mismatch, got %+v", result.Mismatches)
	}
}

func TestValidateProcessFileAcceptsExplicitNoneSnapshots(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	expected, err := RebuildCurrent(repoRoot, "module_demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}

	writeCheckProcessFile(t, repoRoot, strings.Join([]string{
		"spec_file_ref: " + expected.SpecFileRef,
		"spec_version_ref: " + expected.SpecVersionRef,
		"spec_fingerprint: " + expected.SpecFingerprint,
		"module_appendix_snapshot: none",
		"system_constraints_stable_file_ref: none",
		"system_constraints_stable_version_ref: none",
		"system_constraints_stable_fingerprint: none",
		"shared_contract_snapshot: none",
	}, "\n"))

	result, err := ValidateProcessFile(repoRoot, "module_demo", "check")
	if err != nil {
		t.Fatalf("ValidateProcessFile: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result, got mismatches %+v", result.Mismatches)
	}
}

func TestValidateProcessFileAcceptsSnapshotFieldsWithoutYAMLFence(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	expected, err := RebuildCurrent(repoRoot, "module_demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/module_demo.md"), strings.Join([]string{
		"# check",
		"",
		"spec_file_ref: " + expected.SpecFileRef,
		"spec_version_ref: " + expected.SpecVersionRef,
		"spec_fingerprint: " + expected.SpecFingerprint,
		"module_appendix_snapshot: none",
		"system_constraints_stable_file_ref: none",
		"system_constraints_stable_version_ref: none",
		"system_constraints_stable_fingerprint: none",
		"shared_contract_snapshot: none",
		"",
	}, "\n"))

	result, err := ValidateProcessFile(repoRoot, "module_demo", "check")
	if err != nil {
		t.Fatalf("ValidateProcessFile: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result, got mismatches %+v", result.Mismatches)
	}
}

func TestValidateProcessFileAcceptsMarkdownBulletSnapshotFormat(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir)))

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: module_demo
layer: candidate
version: 0.1.0
---

# Demo

See [appendix](./appendix/c_module_demo_prompt.md).

## Global Constraint Alignment

1. system_constraints_stable_ref: none
2. shared_contract_refs: none
`)
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir), "c_module_demo_prompt.md"), `---
module: module_demo
layer: candidate
spec_version_ref: c_module_demo@0.1.0
---

# Appendix
`)

	expected, err := RebuildCurrent(repoRoot, "module_demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/module_demo.md"), strings.Join([]string{
		"# module_demo cand_check snapshot",
		"",
		"## Check Result Snapshot",
		"",
		"- `spec_file_ref`: `" + expected.SpecFileRef + "`",
		"- `spec_version_ref`: `" + expected.SpecVersionRef + "`",
		"- `spec_fingerprint`: `" + expected.SpecFingerprint + "`",
		"- `module_appendix_snapshot`:",
		"  - `file_ref`: `" + expected.ModuleAppendixSnapshot[0].FileRef + "`",
		"  - `appendix_ref`: `" + expected.ModuleAppendixSnapshot[0].AppendixRef + "`",
		"  - `fingerprint`: `" + expected.ModuleAppendixSnapshot[0].Fingerprint + "`",
		"- `system_constraints_stable_file_ref`: `none`",
		"- `system_constraints_stable_version_ref`: `none`",
		"- `system_constraints_stable_fingerprint`: `none`",
		"- `shared_contract_snapshot`: `none`",
		"",
	}, "\n"))

	result, err := ValidateProcessFile(repoRoot, "module_demo", "check")
	if err != nil {
		t.Fatalf("ValidateProcessFile: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result, got mismatches %+v", result.Mismatches)
	}
}

func TestRebuildCurrentRejectsEmptySharedContractRefsList(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: module_demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_stable_ref: none
2. shared_contract_refs:
`)

	_, err = RebuildCurrent(repoRoot, "module_demo")
	if err == nil || !strings.Contains(err.Error(), "must not be an empty list") {
		t.Fatalf("expected empty-list error, got %v", err)
	}
}

func TestRebuildCurrentRejectsDuplicateSharedContractRefs(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: module_demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_stable_ref: none
2. shared_contract_refs:
   - c_shared_demo@0.1.0
   - c_shared_demo@0.1.0
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_demo
---

# Shared
`)

	_, err = RebuildCurrent(repoRoot, "module_demo")
	if err == nil || !strings.Contains(err.Error(), "duplicate item") {
		t.Fatalf("expected duplicate-item error, got %v", err)
	}
}

func TestRebuildCurrentRejectsSharedVersionMismatch(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: module_demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_stable_ref: none
2. shared_contract_refs:
   - c_shared_demo@0.1.0
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.2.0
bound_modules:
  - module_demo
---

# Shared
`)

	_, err = RebuildCurrent(repoRoot, "module_demo")
	if err == nil || !strings.Contains(err.Error(), "does not match frontmatter shared_version") {
		t.Fatalf("expected shared-version mismatch error, got %v", err)
	}
}

func TestRebuildCurrentRejectsStableModuleBindingCandidateShared(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.StableDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))

	status := "# Spec Status\n\n## Formal Modules\n\n| Module | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|\n| `module_demo` | `yes` | `no` | `stable` | `spec_fork` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpecRef, err := specpaths.MainSpecFileRef("stable", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: module_demo
layer: stable
version: 1.0.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_stable_ref: none
2. shared_contract_refs:
   - c_shared_demo@0.1.0
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_demo
---

# Shared
`)

	_, err = RebuildCurrent(repoRoot, "module_demo")
	if err == nil || !strings.Contains(err.Error(), "stable-layer module binding must use an s_ shared ref") {
		t.Fatalf("expected stable-layer binding error, got %v", err)
	}
}

func TestRebuildCurrentRespectsExplicitNoneSystemConstraintsBinding(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/system/stable"))

	system := `---
version: 1.1.0
---

# System
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/system/stable/s_system_constraints.md"), system)

	result, err := RebuildCurrent(repoRoot, "module_demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	if result.SystemConstraintsStableFileRef != "none" {
		t.Fatalf("expected system file ref none, got %s", result.SystemConstraintsStableFileRef)
	}
	if result.SystemConstraintsStableVersionRef != "none" {
		t.Fatalf("expected system version ref none, got %s", result.SystemConstraintsStableVersionRef)
	}
	if result.SystemConstraintsStableFingerprint != "none" {
		t.Fatalf("expected system fingerprint none, got %s", result.SystemConstraintsStableFingerprint)
	}
}

func setupSnapshotValidationRepo(t *testing.T, repoRoot string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_check_result"))

	status := "# Spec Status\n\n## Formal Modules\n\n| Module | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|\n| `module_demo` | `no` | `yes` | `candidate` | `cand_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpec := `---
id: module_demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_stable_ref: none
2. shared_contract_refs: none
`
	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), mainSpec)
}

func writeCheckProcessFile(t *testing.T, repoRoot, yamlBody string) {
	t.Helper()
	content := "# check\n\n```yaml\n" + yamlBody + "\n```\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/module_demo.md"), content)
}

func containsMismatch(mismatches []string, target string) bool {
	for _, mismatch := range mismatches {
		if mismatch == target {
			return true
		}
	}
	return false
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
