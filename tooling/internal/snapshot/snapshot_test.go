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

	status := "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpec := `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

See [appendix](./appendix/c_unit_demo_prompt.md).

## Global Constraint Alignment

1. ` + "`system_constraints_ref`: `system_constraints@1.1.0`" + `
2. ` + "`shared_contract_refs`:" + `
   - ` + "`c_shared_demo@0.2.0`" + `
`
	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), mainSpec)

	appendix := `---
unit: demo
layer: candidate
spec_version_ref: c_unit_demo@0.1.0
---

# Appendix
`
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir), "c_unit_demo_prompt.md"), appendix)

	shared := `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.2.0
bound_objects:
  - unit:demo
system_constraints_ref: system_constraints@1.1.0
---

# Shared
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), shared)

	system := `---
version: 1.1.0
---

# System
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/system_constraints.md"), system)

	result, err := RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	if result.SpecFileRef != mainSpecRef {
		t.Fatalf("unexpected spec file ref: %s", result.SpecFileRef)
	}
	if result.SpecVersionRef != "c_unit_demo@0.1.0" {
		t.Fatalf("unexpected spec version ref: %s", result.SpecVersionRef)
	}
	if len(result.ModuleAppendixSnapshot) != 1 {
		t.Fatalf("expected one appendix snapshot entry, got %d", len(result.ModuleAppendixSnapshot))
	}
	if result.ModuleAppendixSnapshot[0].AppendixRef != "c_unit_demo_prompt@c_unit_demo@0.1.0" {
		t.Fatalf("unexpected appendix ref: %s", result.ModuleAppendixSnapshot[0].AppendixRef)
	}
	if result.SystemConstraintsVersionRef != "system_constraints@1.1.0" {
		t.Fatalf("unexpected system constraints version ref: %s", result.SystemConstraintsVersionRef)
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

	status := "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpec := `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

See [support](./support/c_unit_demo_prompt.md).

## Global Constraint Alignment

1. system_constraints_ref: system_constraints@1.1.0
2. shared_contract_refs:
   - c_shared_demo@0.2.0
`
	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), mainSpec)

	appendix := `---
unit: demo
layer: candidate
spec_version_ref: c_unit_demo@0.1.0
---

# Appendix
`
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir), "support", "c_unit_demo_prompt.md"), appendix)

	shared := `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.2.0
bound_objects:
  - unit:demo
system_constraints_ref: system_constraints@1.1.0
---

# Shared
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), shared)

	system := `---
version: 1.1.0
---

# System
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/system_constraints.md"), system)

	result, err := RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	if len(result.ModuleAppendixSnapshot) != 1 {
		t.Fatalf("expected one appendix snapshot entry, got %d", len(result.ModuleAppendixSnapshot))
	}
	if result.ModuleAppendixSnapshot[0].FileRef != "docs/specs/units/candidate/support/c_unit_demo_prompt.md" {
		t.Fatalf("unexpected appendix file ref: %s", result.ModuleAppendixSnapshot[0].FileRef)
	}
	if len(result.SharedContractSnapshot) != 1 || result.SharedContractSnapshot[0].VersionRef != "c_shared_demo@0.2.0" {
		t.Fatalf("unexpected shared snapshot: %+v", result.SharedContractSnapshot)
	}
	if result.SystemConstraintsVersionRef != "system_constraints@1.1.0" {
		t.Fatalf("unexpected system constraints version ref: %s", result.SystemConstraintsVersionRef)
	}
}

func TestRebuildCurrentRejectsUnsortedSharedContractRefs(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))

	status := "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs:
   - c_shared_zeta@0.1.0
   - c_shared_alpha@0.1.0
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_alpha.md"), `---
shared_contract_id: shared_alpha
layer: candidate
shared_version: 0.1.0
bound_objects:
  - unit:demo
---

# Shared Alpha
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_zeta.md"), `---
shared_contract_id: shared_zeta
layer: candidate
shared_version: 0.1.0
bound_objects:
  - unit:demo
---

# Shared Zeta
`)

	_, err = RebuildCurrent(repoRoot, "demo")
	if err == nil || !strings.Contains(err.Error(), "shared_contract_refs must be sorted") {
		t.Fatalf("expected unsorted shared_contract_refs error, got %v", err)
	}
}

func TestRebuildCurrentIgnoresRawAppendixPathLiteral(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir)))

	status := "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
	id: demo
	layer: candidate
	version: 0.1.0
---

# Demo

Use appendix path `+"`./appendix/c_unit_demo_prompt.md`"+` for detailed prompts.

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs: none
`)

	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir), "c_unit_demo_prompt.md"), `---
unit: demo
layer: candidate
spec_version_ref: c_unit_demo@0.1.0
---

	# Appendix
`)

	result, err := RebuildCurrent(repoRoot, "demo")
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

	status := "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

See [support](./c_unit_demo_prompt.md).

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs: none
`)

	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir), "c_unit_demo_prompt.md"), `---
unit: demo
layer: candidate
spec_version_ref: c_unit_demo@0.1.0
---

# Drift
`)

	_, err = RebuildCurrent(repoRoot, "demo")
	if err == nil || !strings.Contains(err.Error(), "directory drift") {
		t.Fatalf("expected directory drift error, got %v", err)
	}
}

func TestValidateProcessFileRejectsMissingRequiredSnapshotField(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	writeCheckProcessFile(t, repoRoot, strings.Join([]string{
		"object_type: unit",
		"object_ref: demo",
		"gate: unit_check",
		"decision: pass",
		"allow_next: true",
		"next_command: unit_plan",
		"blocking_summary: none",
		"coverage_summary: current candidate",
		"truth_layer_ref: candidate",
		"truth_file_ref: docs/specs/units/candidate/c_unit_demo.md",
		"truth_version_ref: c_unit_demo@0.1.0",
		"unit_appendix_snapshot: none",
		"system_constraints_file_ref: none",
		"system_constraints_version_ref: none",
		"system_constraints_fingerprint: none",
		"shared_contract_snapshot: none",
	}, "\n"))

	result, err := ValidateProcessFile(repoRoot, "demo", "check")
	if err != nil {
		t.Fatalf("ValidateProcessFile: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result, got valid")
	}
	if !containsMismatch(result.Mismatches, "missing required field: truth_fingerprint") {
		t.Fatalf("expected missing truth_fingerprint mismatch, got %+v", result.Mismatches)
	}
}

func TestValidateProcessFileAcceptsExplicitNoneSnapshots(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	expected, err := RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}

	writeCheckProcessFile(t, repoRoot, renderFormalCheckProcessBody(expected))

	result, err := ValidateProcessFile(repoRoot, "demo", "check")
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

	expected, err := RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/unit/demo.md"), strings.Join([]string{
		"# check",
		"",
		renderFormalCheckProcessBody(expected),
		"",
	}, "\n"))

	result, err := ValidateProcessFile(repoRoot, "demo", "check")
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

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

See [appendix](./appendix/c_unit_demo_prompt.md).

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs: none
`)
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir), "c_unit_demo_prompt.md"), `---
unit: demo
layer: candidate
spec_version_ref: c_unit_demo@0.1.0
---

# Appendix
`)

	expected, err := RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/unit/demo.md"), strings.Join([]string{
		"# demo unit_check snapshot",
		"",
		"## Check Result Snapshot",
		"",
		"- `object_type`: `unit`",
		"- `object_ref`: `" + expected.Module + "`",
		"- `gate`: `unit_check`",
		"- `decision`: `pass`",
		"- `allow_next`: `true`",
		"- `next_command`: `unit_plan`",
		"- `blocking_summary`: `none`",
		"- `coverage_summary`: `current candidate`",
		"- `truth_layer_ref`: `" + expected.TruthLayerRef + "`",
		"- `truth_file_ref`: `" + expected.SpecFileRef + "`",
		"- `truth_version_ref`: `" + expected.SpecVersionRef + "`",
		"- `truth_fingerprint`: `" + expected.SpecFingerprint + "`",
		"- `unit_appendix_snapshot`:",
		"  - `file_ref`: `" + expected.ModuleAppendixSnapshot[0].FileRef + "`",
		"  - `appendix_ref`: `" + expected.ModuleAppendixSnapshot[0].AppendixRef + "`",
		"  - `fingerprint`: `" + expected.ModuleAppendixSnapshot[0].Fingerprint + "`",
		"- `system_constraints_file_ref`: `none`",
		"- `system_constraints_version_ref`: `none`",
		"- `system_constraints_fingerprint`: `none`",
		"- `shared_contract_snapshot`: `none`",
		"",
	}, "\n"))

	result, err := ValidateProcessFile(repoRoot, "demo", "check")
	if err != nil {
		t.Fatalf("ValidateProcessFile: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result, got mismatches %+v", result.Mismatches)
	}
}

func TestValidateProcessFileRejectsUnexpectedGate(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	expected, err := RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}

	writeCheckProcessFile(t, repoRoot, strings.Replace(renderFormalCheckProcessBody(expected), "gate: unit_check", "gate: unit_plan", 1))

	result, err := ValidateProcessFile(repoRoot, "demo", "check")
	if err != nil {
		t.Fatalf("ValidateProcessFile: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result, got valid")
	}
	if !containsMismatch(result.Mismatches, "gate mismatch: actual=unit_plan expected=unit_check") {
		t.Fatalf("expected gate mismatch, got %+v", result.Mismatches)
	}
}

func TestValidateProcessFileAcceptsPlanSchemaWithoutGateFields(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	expected, err := RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_plans/active/demo.md"), "# plan\n\n```yaml\n"+renderFormalPlanProcessBody(expected)+"\n```\n")

	result, err := ValidateProcessFile(repoRoot, "demo", "plan")
	if err != nil {
		t.Fatalf("ValidateProcessFile: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid plan result, got mismatches %+v", result.Mismatches)
	}
}

func TestRebuildCurrentRejectsEmptySharedContractRefsList(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs:
`)

	_, err = RebuildCurrent(repoRoot, "demo")
	if err == nil || !strings.Contains(err.Error(), "must not be an empty list") {
		t.Fatalf("expected empty-list error, got %v", err)
	}
}

func TestRebuildCurrentRejectsDuplicateSharedContractRefs(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs:
   - c_shared_demo@0.1.0
   - c_shared_demo@0.1.0
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_objects:
  - unit:demo
---

# Shared
`)

	_, err = RebuildCurrent(repoRoot, "demo")
	if err == nil || !strings.Contains(err.Error(), "duplicate item") {
		t.Fatalf("expected duplicate-item error, got %v", err)
	}
}

func TestRebuildCurrentRejectsSharedVersionMismatch(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs:
   - c_shared_demo@0.1.0
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.2.0
bound_objects:
  - unit:demo
---

# Shared
`)

	_, err = RebuildCurrent(repoRoot, "demo")
	if err == nil || !strings.Contains(err.Error(), "does not match frontmatter shared_version") {
		t.Fatalf("expected shared-version mismatch error, got %v", err)
	}
}

func TestRebuildCurrentRejectsStableModuleBindingCandidateShared(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.StableDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))

	status := "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n| `unit` | `demo` | `yes` | `no` | `stable` | `unit_fork` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpecRef, err := specpaths.MainSpecFileRef("stable", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), `---
id: demo
layer: stable
version: 1.0.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs:
   - c_shared_demo@0.1.0
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_objects:
  - unit:demo
---

# Shared
`)

	_, err = RebuildCurrent(repoRoot, "demo")
	if err == nil || !strings.Contains(err.Error(), "stable-layer unit binding must use an s_ shared ref") {
		t.Fatalf("expected stable-layer binding error, got %v", err)
	}
}

func TestRebuildCurrentRespectsExplicitNoneSystemConstraintsBinding(t *testing.T) {
	repoRoot := t.TempDir()
	setupSnapshotValidationRepo(t, repoRoot)

	system := `---
version: 1.1.0
---

# System
`
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/system_constraints.md"), system)

	result, err := RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	if result.SystemConstraintsFileRef != "none" {
		t.Fatalf("expected system file ref none, got %s", result.SystemConstraintsFileRef)
	}
	if result.SystemConstraintsVersionRef != "none" {
		t.Fatalf("expected system version ref none, got %s", result.SystemConstraintsVersionRef)
	}
	if result.SystemConstraintsFingerprint != "none" {
		t.Fatalf("expected system fingerprint none, got %s", result.SystemConstraintsFingerprint)
	}
}

func setupSnapshotValidationRepo(t *testing.T, repoRoot string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_check_result/unit"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_plans/active"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_plans/draft"))

	status := "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_check` | note |\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)

	mainSpec := `---
id: demo
layer: candidate
version: 0.1.0
---

# Demo

## Global Constraint Alignment

1. system_constraints_ref: none
2. shared_contract_refs: none
`
	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), mainSpec)
}

func writeCheckProcessFile(t *testing.T, repoRoot, yamlBody string) {
	t.Helper()
	content := "# check\n\n```yaml\n" + yamlBody + "\n```\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/unit/demo.md"), content)
}

func renderFormalCheckProcessBody(expected Snapshot) string {
	return strings.Join([]string{
		"object_type: unit",
		"object_ref: " + expected.Module,
		"gate: unit_check",
		"decision: pass",
		"allow_next: true",
		"next_command: unit_plan",
		"blocking_summary: none",
		"coverage_summary: current candidate",
		"truth_layer_ref: " + expected.TruthLayerRef,
		"truth_file_ref: " + expected.SpecFileRef,
		"truth_version_ref: " + expected.SpecVersionRef,
		"truth_fingerprint: " + expected.SpecFingerprint,
		"unit_appendix_snapshot: none",
		"system_constraints_file_ref: none",
		"system_constraints_version_ref: none",
		"system_constraints_fingerprint: none",
		"shared_contract_snapshot: none",
	}, "\n")
}

func renderFormalPlanProcessBody(expected Snapshot) string {
	return strings.Join([]string{
		"spec_file_ref: " + expected.SpecFileRef,
		"spec_version_ref: " + expected.SpecVersionRef,
		"spec_fingerprint: " + expected.SpecFingerprint,
		"unit_appendix_snapshot: none",
		"system_constraints_file_ref: none",
		"system_constraints_version_ref: none",
		"system_constraints_fingerprint: none",
		"shared_contract_snapshot: none",
	}, "\n")
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
