# Process Snapshot Contract

## 1. Purpose

This file defines the fixed snapshot contract used by Spec Flow process files.

It answers six questions:

1. which object families write process snapshots
2. which snapshot fields are common across object families
3. which snapshot fields are object-specific
4. how snapshot values must be normalized before comparison
5. which downstream invalidation decisions may rely on those snapshots
6. which values executors must never invent ad hoc

This contract is centralized governance truth.
Executors must not create per-command snapshot shapes.

## 2. Scope

This contract governs process files for:

1. `unit`
   - `docs/specs/_check_result/{unit}.md`
   - `docs/specs/_plans/active/{unit}.md`
   - `docs/specs/_verify_result/{unit}.md`
2. `scenario`
   - `docs/specs/_check_result/{flow}.md`
   - `docs/specs/_verify_result/{flow}.md`
3. `project`
   - `docs/specs/_check_result/project.md`
   - `docs/specs/_verify_result/project.md`

It also governs any internal flow that revalidates those files, including:

1. `shared_sync`
2. `impact_sync`

It does not define object truth.
It defines only how process files record the truth they were written against.

## 3. Common Snapshot Field Families

### 3.1 Gate-Bearing Process Files

Every gate-bearing process file covered by this contract must record:

1. `object_type`
   - one of `unit`, `scenario`, `project`
2. `object_ref`
   - the bare formal object identifier, for example `ai`, `task_execution`, or `project`
3. `truth_file_ref`
   - the exact current-layer truth file used by that process file
4. `truth_version_ref`
   - `<file_prefix>@<version>`
5. `truth_fingerprint`
   - the Section 6 fingerprint of `truth_file_ref`
6. `system_constraints_stable_file_ref`
   - the currently bound stable system-constraints file, or `none`
7. `system_constraints_stable_version_ref`
   - the currently bound stable system-constraints version, or `none`
8. `system_constraints_stable_fingerprint`
   - the fingerprint of the currently bound stable system-constraints file, or `none`

Gate-bearing process files are:

1. `docs/specs/_check_result/{object}.md`
2. `docs/specs/_verify_result/{object}.md`

Rules:

1. the field names above are fixed
2. executors must not substitute `spec_fingerprint` for gate-bearing files
3. if a file correctly binds no system constraints, all three system-constraints fields must use literal `none`

### 3.2 Unit Active Plan Files

`docs/specs/_plans/active/{unit}.md` is governed by the same snapshot contract but it is not a gate-bearing file.

Every unit active plan file must record:

1. `spec_file_ref`
   - the exact candidate-layer unit truth file used by that plan
2. `spec_version_ref`
   - `<file_prefix>@<version>`
3. `spec_fingerprint`
   - the Section 6 fingerprint of `spec_file_ref`
4. `unit_appendix_snapshot`
   - the normalized appendix snapshot of the current candidate-layer unit truth, or `none`
5. `system_constraints_stable_file_ref`
   - the currently bound stable system-constraints file, or `none`
6. `system_constraints_stable_version_ref`
   - the currently bound stable system-constraints version, or `none`
7. `system_constraints_stable_fingerprint`
   - the fingerprint of the currently bound stable system-constraints file, or `none`
8. `shared_contract_snapshot`
   - the normalized shared snapshot of the current candidate-layer unit truth, or `none`

Rules:

1. `active/{unit}.md` does not carry `gate`, `decision`, `allow_next`, or `next_command`
2. `active/{unit}.md` still records the exact candidate unit truth and exact current global-binding snapshot it was written against
3. if an active plan correctly binds no appendix or shared files, `unit_appendix_snapshot` or `shared_contract_snapshot` must use literal `none`
4. if an active plan correctly binds no system constraints, all three system-constraints fields must use literal `none`

### 3.3 Unit Draft Plan Files

`docs/specs/_plans/draft/{unit}.md` is a planning working artifact.

It is not:

1. a gate-bearing file
2. a consumable downstream handoff artifact
3. a substitute for `active/{unit}.md`

If a draft plan records snapshot anchors, it may record only:

1. `object_ref`
2. `truth_file_ref`
3. `truth_version_ref`
4. `truth_fingerprint`

It may additionally record planning-local fields such as:

1. `fallback_reason_code`
2. `blocking_summary`
3. `resume_signal`
4. `known_findings`
5. `open_unknowns`
6. `research_notes`

Rules:

1. draft plan files must never be treated as valid inputs for `unit_impl` or `unit_verify`
2. draft plan files do not inherit the active-plan binding revalidation contract
3. draft plan files may be deleted whenever the current round falls back, forks, promotes, or closes candidate state

## 4. Object-Specific Snapshot Fields

### 4.1 `unit`

`unit` process files may additionally record:

1. `unit_appendix_snapshot`
2. `shared_contract_snapshot`

`unit_appendix_snapshot` has only two legal forms:

1. literal `none`
2. a normalized ordered list where each item contains:
   - `file_ref`
   - `appendix_ref`
   - `fingerprint`

`shared_contract_snapshot` has only two legal forms:

1. literal `none`
2. a normalized ordered list where each item contains:
   - `shared_contract_id`
   - `layer`
   - `file_ref`
   - `version_ref`
   - `fingerprint`

### 4.2 `scenario`

`scenario` process files may additionally record:

1. `unit_snapshot`
2. `shared_contract_snapshot`

`unit_snapshot` has only two legal forms:

1. literal `none`
2. a normalized ordered list where each item contains:
   - `unit`
   - `layer`
   - `file_ref`
   - `version_ref`
   - `fingerprint`

`shared_contract_snapshot` uses the same shape as `unit`.

### 4.3 `project`

`project` process files may additionally record:

1. `scenario_snapshot`
2. `unit_snapshot`
3. `shared_contract_snapshot`

`scenario_snapshot` has only two legal forms:

1. literal `none`
2. a normalized ordered list where each item contains:
   - `scenario`
   - `layer`
   - `file_ref`
   - `version_ref`
   - `fingerprint`

`unit_snapshot` and `shared_contract_snapshot` use the shapes defined above.

## 5. Binding And Inclusion Boundary

Snapshot inclusion must follow the formal binding contract, not heuristic scanning.

Rules:

1. `unit_appendix_snapshot` includes only appendix files explicitly referenced by the current-layer unit truth
2. `unit_snapshot` includes only units formally bound by current `scenario` or `project` truth
3. `scenario_snapshot` includes only scenarios formally bound by current `project` truth
4. `shared_contract_snapshot` includes only currently bound shared files from formal `shared_contract_refs`
5. `bound_objects` metadata is never a formal inclusion source
6. a `bound_objects`-only delta does not by itself invalidate downstream process files

## 6. Fingerprint Contract

The hash algorithm is fixed:

1. normalize file text according to Section 7
2. encode the normalized text as UTF-8
3. compute `sha256`
4. render the result as lowercase hexadecimal

This same fingerprint contract applies to:

1. `truth_fingerprint`
2. `spec_fingerprint`
3. appendix file fingerprints
4. `unit_snapshot` item fingerprints
5. `scenario_snapshot` item fingerprints
6. `shared_contract_snapshot` item fingerprints
7. `system_constraints_stable_fingerprint`

## 7. Text Normalization Rules

Before hashing a markdown truth file, normalize it in this exact order:

1. read the full file text
2. convert all line endings to `LF`
3. if the file does not end with `LF`, append exactly one trailing `LF`
4. do not trim leading spaces
5. do not trim trailing spaces inside lines
6. do not remove blank lines
7. do not reorder frontmatter keys
8. do not apply markdown-aware semantic rewriting

Plain meaning:

1. this contract fingerprints the actual file text
2. it is intentionally stricter than "same meaning"
3. if the text changed, the fingerprint changed

## 8. Ordering Rules

Whenever a snapshot field uses a list, executors must normalize ordering before writeback or comparison.

Ordering rules:

1. `unit_appendix_snapshot`
   - sort by `file_ref`
   - then by `appendix_ref`
2. `unit_snapshot`
   - sort by `unit`
   - then by `layer`
   - then by `file_ref`
3. `scenario_snapshot`
   - sort by `scenario`
   - then by `layer`
   - then by `file_ref`
4. `shared_contract_snapshot`
   - sort by `shared_contract_id`
   - then by `layer`
   - then by `file_ref`

Executors must compare the normalized ordered form exactly.

## 9. Revalidation Rules

When a command or internal governance flow revalidates a process file, it must rebuild the current snapshot from current bound truth and compare it against the stored fields exactly.

At minimum:

1. for gate-bearing files, rebuild the common `truth_*` fields
2. for unit plan files, rebuild `spec_file_ref`, `spec_version_ref`, and `spec_fingerprint`
3. rebuild the currently bound `system_constraints_stable_*` fields
4. rebuild the object-specific snapshot fields allowed for that object type
5. compare stored and rebuilt values exactly

Shared-specific exception rule:

1. if a shared file is explicitly declared by the active caller as `bound_objects`-only for the current round, a difference caused only by that metadata delta does not invalidate the process file on that basis alone
2. executors must not infer a `bound_objects`-only delta from fingerprint difference alone

If any required field differs after applying only allowed exceptions, the process file is invalid for downstream use.

## 10. Relationship To Other Files

This contract works together with:

1. `specflow/framework/docs/agent_guidelines/spec_policy.md`
2. `specflow/framework/docs/agent_guidelines/command_policy.md`
3. `specflow/framework/docs/agent_guidelines/impact_sync_policy.md`
4. process-file READMEs under `docs/specs/` and `specflow/templates/root/docs/specs/`

Priority rules:

1. `spec_policy.md` defines object families and binding surfaces
2. this file defines the exact snapshot fields and normalization rules
3. command files and process-file READMEs must remain consistent with this file

## 11. Non-Goals

This file does not:

1. define object behavior truth
2. replace command-specific stop conditions
3. define new lifecycle stages
