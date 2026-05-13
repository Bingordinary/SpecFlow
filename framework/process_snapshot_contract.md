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
   - `docs/specs/_check_result/unit/{unit}.md`
   - `docs/specs/_plans/active/{unit}.md`
   - `docs/specs/_verify_result/unit/{unit}.md`
   - `docs/specs/_verify_result/stable/unit/{unit}.md`
2. `scenario`
   - `docs/specs/_check_result/scenario/{scenario}.md`
   - `docs/specs/_verify_result/scenario/{scenario}.md`
   - `docs/specs/_verify_result/stable/scenario/{scenario}.md`

It also governs any internal flow that revalidates those files, including:

1. `rule_sync`
2. `impact_sync`

It does not define object truth.
It defines only how process files record the truth they were written against.

## 3. Common Snapshot Field Families

### 3.1 Gate-Bearing Process Files

Every gate-bearing process file covered by this contract must record:

1. `object_type`
   - one of `unit`, `scenario`
2. `object_ref`
   - the bare formal object identifier, for example `ai` or `task_execution`
3. `truth_layer_ref`
   - the active truth layer used by that process file, either `stable` or `candidate`
4. `truth_file_ref`
   - the exact current-layer truth file used by that process file
5. `truth_version_ref`
   - `<file_prefix>@<version>`
6. `truth_fingerprint`
   - the Section 6 fingerprint of `truth_file_ref`
7. `rule_snapshot`
   - the normalized rule snapshot visible to the current truth file, or `none`
8. `acceptance_item_set`
   - the normalized acceptance item set from the current truth file used by the gate

Gate-bearing process files are:

1. `docs/specs/_check_result/{object_type}/{object}.md`
2. `docs/specs/_verify_result/{object_type}/{object}.md`

Rules:

1. the field names above are fixed
2. executors must not substitute `spec_fingerprint` for gate-bearing files
3. `rule_snapshot` includes all stable global rules and every formal rule listed by `rule_refs`
4. if no rule is visible to the current truth file, `rule_snapshot` must use literal `none`
5. `acceptance_item_set` must be an ordered list where each item records at least:
   - `id`
   - `verification_surface`
   - `not_runnable_yet` as `yes` or `no`
6. the order of `acceptance_item_set` is ascending lexical order by `id`
7. `docs/specs/_verify_result/{object_type}/{object}.md` must additionally record an `acceptance_item_evidence_matrix` that gives one status for every item in `acceptance_item_set`
8. `acceptance_item_evidence_matrix` must be an ordered list where each item records:
   - `id`
   - `status`
9. the order of `acceptance_item_evidence_matrix` is ascending lexical order by `id`
10. allowed evidence-matrix statuses are exactly `pass`, `fail`, `partial`, `not_checked`, and `not_runnable_yet`

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
5. `rule_snapshot`
   - the normalized rule snapshot visible to the current candidate-layer unit truth, or `none`
6. `acceptance_item_plan_coverage`
   - the active plan's mapping from current candidate acceptance item `id` values to implementation slices and verification targets

Rules:

1. `active/{unit}.md` does not carry `gate`, `decision`, `allow_next`, or `next_command`
2. `active/{unit}.md` still records the exact candidate unit truth and exact rule snapshot it was written against
3. if an active plan correctly binds no appendix files, `unit_appendix_snapshot` must use literal `none`
4. if no rule is visible to the current candidate-layer unit truth, `rule_snapshot` must use literal `none`
5. `acceptance_item_plan_coverage` must be an ordered list where each item records:
   - `id`
   - `coverage`
6. `coverage` must name the implementation slices or verification targets that cover the item
7. the order of `acceptance_item_plan_coverage` is ascending lexical order by `id`
8. `acceptance_item_plan_coverage` must cover every current-gate acceptance item that the candidate claims for the current round, or the active plan is not consumable

### 3.3 Stable Acceptance Summary Files

`docs/specs/_verify_result/stable/{object_type}/{object}.md` stores the minimal acceptance coverage summary preserved by promotion.

It is not:

1. a gate-bearing file
2. a substitute for stable truth
3. a current implementation-alignment claim after later code changes

Each stable acceptance summary must record:

1. `object_type`
   - one of `unit`, `scenario`
2. `object_ref`
   - the bare formal object identifier
3. `stable_truth_file_ref`
   - the stable truth file written by promotion
4. `stable_truth_version_ref`
   - `<file_prefix>@<version>`
5. `stable_truth_fingerprint`
   - the Section 6 fingerprint of `stable_truth_file_ref`
6. `promotion_verify_result_ref`
   - the current-round verify result file consumed by promotion before cleanup
7. `acceptance_item_set`
   - the normalized acceptance item set promoted into stable
8. `acceptance_item_coverage_summary`
   - the final status of each promoted acceptance item
9. `key_evidence_source_refs`
   - the smallest useful references to commands, tests, inspections, or manual evidence that proved the promoted items

Rules:

1. a stable acceptance summary records what evidence closed the promoted version
2. later implementation drift does not rewrite this file into a new pass claim
3. later stable verification may read it as background, but must collect current evidence before making a new stable-alignment claim

### 3.4 Unit Draft Plan Files

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
2. `rule_snapshot`

`unit_appendix_snapshot` has only two legal forms:

1. literal `none`
2. a normalized ordered list where each item contains:
   - `file_ref`
   - `fingerprint`

`rule_snapshot` has only two legal forms:

1. literal `none`
2. a normalized ordered list where each item contains:
   - `rule_id`
   - `layer`
   - `file_ref`
   - `version_ref`
   - `fingerprint`

### 4.2 `scenario`

`scenario` process files may additionally record:

1. `repository_mapping_snapshot`
2. `unit_snapshot`
3. `scenario_appendix_snapshot`
4. `rule_snapshot`

`repository_mapping_snapshot` has only one legal form:

1. a normalized object containing:
   - `file_ref`
   - `version_ref`
   - `fingerprint`

`unit_snapshot` has only two legal forms:

1. literal `none`
2. a normalized ordered list where each item contains:
   - `unit`
   - `layer`
   - `file_ref`
   - `version_ref`
   - `fingerprint`

`scenario_appendix_snapshot` has only two legal forms:

1. literal `none`
2. a normalized ordered list where each item contains:
   - `file_ref`
   - `fingerprint`

`rule_snapshot` uses the same shape as `unit`.

## 5. Binding And Inclusion Boundary

Snapshot inclusion must follow the formal binding contract, not heuristic scanning.

Rules:

1. `unit_appendix_snapshot` includes only appendix files explicitly referenced by the current-layer unit truth
2. `scenario_appendix_snapshot` includes only appendix files explicitly referenced by the current-layer scenario truth
3. `repository_mapping_snapshot` captures only `docs/specs/repository_mapping.md`
4. `unit_snapshot` includes only units formally bound by current `scenario` truth
5. `rule_snapshot` includes all stable global rules and every formal rule listed by `rule_refs`
6. Rule files must not record `bound_objects`; consumer lists are derived from current-layer frontmatter `rule_refs`

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
4. `repository_mapping_snapshot` fingerprints
5. `unit_snapshot` item fingerprints
6. `rule_snapshot` item fingerprints
7. `stable_truth_fingerprint`

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
2. `scenario_appendix_snapshot`
   - sort by `file_ref`
3. `unit_snapshot`
   - sort by `unit`
   - then by `layer`
   - then by `file_ref`
4. `rule_snapshot`
   - sort by `rule_id`
   - then by `layer`
   - then by `file_ref`
5. `acceptance_item_set`
   - sort by `id`
   - then by `verification_surface`
6. `acceptance_item_plan_coverage`
   - sort by `id`
7. `acceptance_item_evidence_matrix`
   - sort by `id`
7. `acceptance_item_coverage_summary`
   - sort by `acceptance_item_id`

Executors must compare the normalized ordered form exactly.

## 9. Revalidation Rules

When a command or internal governance flow revalidates a process file, it must rebuild the current snapshot from current bound truth and compare it against the stored fields exactly.

At minimum:

1. for gate-bearing files, rebuild the common `truth_*` fields, including `truth_layer_ref`
2. for unit plan files, rebuild `spec_file_ref`, `spec_version_ref`, and `spec_fingerprint`
3. rebuild the currently bound `stable g_ rule_*` fields
4. rebuild the object-specific snapshot fields allowed for that object type
5. rebuild `acceptance_item_set` from the current truth file for gate-bearing files
6. for active plan files, rebuild the current candidate acceptance item set and verify that `acceptance_item_plan_coverage` still covers it
7. compare stored and rebuilt values exactly

Tool-backed validation rule:

1. when the repository provides deterministic snapshot validation tooling for the target object family and process kind, process-file writeback and process-file consumption must use that tooling as the authoritative validation step
2. for current `unit` and `scenario` process files, the required validation command is:

```text
specflow/tooling/bin/specflowctl-<os>-<arch> snapshot validate-process --repo-root <repo-root> --object-type unit|scenario --object <object> --process check|plan|verify
```

`plan` is valid only for `--object-type unit`.

3. a command that consumes a covered process file must run the matching validation command before treating that file as current and consumable
4. a command that writes a covered process file must run the matching validation command after writeback and before reporting a pass gate, active handoff, verification pass, or lifecycle advance
5. when validation fails, the tool result is the authoritative failure input; fallback cleanup and `_status.md` fallback may use only the tool-reported mismatch surface, failure layer, and recommended next command, or a command-local failure layer that the active command file explicitly defines for that validation failure shape
6. a manual hash calculation, shell checksum, editor display, conversation-derived value, or temporary script result may be used only as diagnostic evidence; it must not replace the tooling result, trigger drift classification, trigger fallback cleanup, trigger `_status.md` writeback, or support lifecycle progression
7. if the required validation tooling is missing, stale, unsupported for the target process kind, or fails to execute, the command must report `tooling validation unavailable`, stop before lifecycle judgment, and must not claim a new pass gate, active handoff, verification pass, stable-alignment pass, fallback cleanup, or lifecycle advance from that process file
8. if a command writes a stable acceptance summary that is not supported by `snapshot validate-process`, it must still compute the summary fields with this fingerprint contract and must not use shell checksum output, editor display, or conversation-derived values as the authoritative field source

Command preflight rule:

1. when the repository provides `specflowctl command preflight`, standard commands that consume current process files must run it before reading process-file contents for command judgment
2. `command preflight` may validate only mechanical entry facts: current `_status.md` row, expected command, required process-file existence, and required `snapshot validate-process` results
3. if `command preflight` is unavailable, the command must explicitly run each required `snapshot validate-process` command before continuing
4. if `command preflight` fails, the command must not compensate with manual hash calculation or local file inspection; it must stop or enter the command's tool-backed fallback path using only the reported failure data
5. a command with no covered process-file input may treat preflight as a status-row check only; this does not create permission to skip command-local semantic checks
6. `command close` must repeat the same mechanical preflight for non-fallback close outcomes of commands that consume covered process files, so lifecycle progression cannot succeed only because the executor forgot to run preflight at command entry
7. `command close` must report close-time input validation separately from output process validation through `input_validation_action`, `input_validated_processes`, and `input_validation_mismatches`
8. fallback and recovery close outcomes may skip close-time input validation only when their purpose is to return to the smallest legal recovery point rather than to consume the old process file as still current

Rule-specific exception rule:

1. executors must not infer a metadata-only Rule exception from fingerprint difference alone
2. any Rule file that records `bound_objects` is invalid before snapshot comparison continues

If any required field differs after applying only allowed exceptions, the process file is invalid for downstream use.

## 10. Relationship To Other Files

This contract works together with:

1. `specflow/framework/spec_policy.md`
2. `specflow/framework/command_policy.md`
3. `specflow/framework/impact_sync_policy.md`
4. process-file READMEs under `docs/specs/` and `specflow/templates/docs/specs/`

Priority rules:

1. `spec_policy.md` defines object families and binding surfaces
2. this file defines the exact snapshot fields and normalization rules
3. command files and process-file READMEs must remain consistent with this file

## 11. Non-Goals

This file does not:

1. define object behavior truth
2. replace command-specific stop conditions
3. define new lifecycle stages
