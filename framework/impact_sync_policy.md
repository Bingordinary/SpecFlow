# Impact Sync Policy

## 1. Purpose

This file defines the internal governance flow `impact_sync`.

It answers six questions:

1. what `impact_sync` is responsible for
2. what it consumes as input
3. what it must write back
4. which object families it may reconcile
5. how it differs from `rule_sync`
6. which centralized snapshot and recovery contracts it must use

`impact_sync` is an internal governance flow.
It is not a user-facing command entry and it is not a second lifecycle.

## 2. Scope

`impact_sync` handles only deterministic downstream invalidation and fallback after an upstream truth or binding change is already known.

It may reconcile:

1. `unit`
2. `scenario`

It does not:

1. decide whether a change belongs to rule governance
2. decide whether a boundary is still unit-local or shared
3. rewrite object truth files
4. judge semantic sufficiency of candidate closure

## 3. Inputs

Before `impact_sync` runs, the caller must already know:

1. which upstream object changed
2. which downstream object set is in scope
3. which exception-resolved generic invalidation input applies, if any
4. the current binding and snapshot state of those downstream objects

`impact_sync` may consume:

1. `_status.md`
2. current object truth files
3. current process files
4. caller-resolved generic inputs such as:
   - `invalidating_rule_refs`
   - `explicit_fallback_scope`
   - `allowed_shared_snapshot_mismatch_file_refs`
   - same-round stable landing retarget fallback after `rule_sync` has already validated the retargeted candidate units and converted them into explicit fallback scope

`impact_sync` must not interpret raw rule-specific exception inputs such as stable landing owners, stable landing refs, or retargeted unit lists.

Before `impact_sync` revalidates any process file or writes any fallback result, the executor must read and apply:

1. `specflow/framework/process_snapshot_contract.md`
2. `specflow/framework/recovery_policy.md`

`impact_sync` must not invent local snapshot shapes, local fingerprint rules, local ordering rules, local cleanup maps, recovery layers, or local fallback targets when those rules are defined by those contracts.

## 4. Writeback Contract

`impact_sync` may write only:

1. `_status.md`
2. candidate-side process file cleanup for invalid downstream objects

### 4.1 Execution Order

When `impact_sync` is invoked, the executor must follow this sequence:

1. **Revalidate snapshots** — rebuild process snapshots from current bound truth using the Snapshot Revalidation Rules in this section; identify which process files no longer match
2. **Classify failure layer** — for each invalid process file, classify the failed surface using `recovery_policy.md` Section 4 (Failure Layers: `truth_layer`, `gate_layer`, `plan_layer`, `implementation_layer`, `evidence_layer`, `dependency_readiness_layer`)
3. **Select fallback target** — for each classified failure layer, apply the Candidate Fallback Rules (for candidate objects) or Stable-Side Fallback Rules (for stable objects) in this section to choose the smallest legal next command
4. **Execute cleanup per layer** — for each candidate-side fallback, apply the Candidate-Side Cleanup Rules in this section to delete process files and update `_status.md`
5. **Report output** — produce the output contract fields listed in this section

Each step depends on the previous step. Do not skip to cleanup before snapshot revalidation, and do not select a fallback target before classifying the failure layer.

### 4.2 Candidate Fallback Rules

1. invalid `unit` truth layer -> `unit_check`
2. invalid `unit` gate layer -> `unit_check`
3. invalid `unit` plan layer -> `unit_plan`
4. invalid `unit` implementation layer -> `unit_impl`
5. invalid `unit` evidence layer -> `unit_verify`
6. invalid `scenario` truth layer -> `scenario_check`
7. invalid `scenario` gate layer -> `scenario_check`
8. invalid `scenario` evidence layer -> `scenario_verify`
9. invalid `scenario` dependency readiness layer -> `scenario_promote`

### 4.3 Stable-Side Fallback Rules

1. when an invalid stable `unit` falls back to `unit_stable_verify`, update `_status.md` to that next step
2. when an invalid stable `scenario` falls back to `scenario_stable_verify`, update `_status.md` to that next step
3. stable-side fallback must not delete candidate-side process files solely because stable alignment became stale
4. stable-side fallback is invoked only after the upstream change is already known to affect stable-layer alignment claims; it must not be used as a periodic health check

### 4.4 Snapshot Revalidation Rules

1. rebuild current process snapshots according to `process_snapshot_contract.md`
2. apply the fingerprint, ordering, and exact-comparison rules from that contract
3. apply only caller-resolved generic exceptions that the contract allows, such as `allowed_shared_snapshot_mismatch_file_refs`
4. classify each invalid process file by `recovery_policy.md` Section 4 before cleanup
5. treat the process file as invalid for downstream use when any required stored field differs from the rebuilt value after allowed exceptions

### 4.5 Candidate-Side Cleanup Rules

1. update `_status.md` to the next step selected by the recovery layer
2. delete exactly the process files listed for that object family and recovery layer in `recovery_policy.md`
3. a cleanup target that is already absent is recorded as an absent cleanup target; it does not create a different fallback state

### 4.6 Output Report

`impact_sync` output must report:

1. the upstream change it consumed
2. the affected downstream objects
3. the fallback reason code for each invalid downstream object
4. each `_status.md` update
5. candidate cleanup files deleted
6. candidate cleanup targets already absent
7. any object that could not be reconciled and the legal resume owner

## 5. Relationship To `rule_sync`

`rule_sync` remains the rule-governance internal flow.

Responsibility split:

1. `rule_sync` owns rule-specific impact discovery, rule-specific exception handling, exception-to-generic-input conversion, and rule-governance stop conditions
2. `impact_sync` owns the generic downstream invalidation and fallback execution once the affected object set is already fixed

Therefore:

1. `rule_sync` may call `impact_sync`
2. `impact_sync` must not replace `rule_sync` as the rule-governance intent or boundary flow

## 6. Non-Goals

`impact_sync` does not:

1. create a new user-facing command
2. replace the rule-governance branch
3. replace `unit_check` or `scenario_check`
4. replace `unit_stable_verify` or `scenario_stable_verify`
