# Impact Sync Policy

## 1. Purpose

This file defines the internal governance flow `impact_sync`.

It answers five questions:

1. what `impact_sync` is responsible for
2. what it consumes as input
3. what it must write back
4. which object families it may reconcile
5. how it differs from `shared_sync`

`impact_sync` is an internal governance flow.
It is not a user-facing command entry and it is not a second lifecycle.

## 2. Scope

`impact_sync` handles only deterministic downstream invalidation and fallback after an upstream truth or binding change is already known.

It may reconcile:

1. `module`
2. `flow`
3. `project`

It does not:

1. decide whether a change belongs to shared governance
2. decide whether a boundary is still module-local or shared
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
   - `invalidating_shared_refs`
   - `explicit_fallback_scope`
   - `allowed_shared_snapshot_mismatch_file_refs`

`impact_sync` must not interpret raw shared-specific exception inputs.

## 4. Writeback Contract

`impact_sync` may write only:

1. `_status.md`
2. candidate-side process file cleanup for invalid downstream objects

Candidate fallback rules:

1. invalid `module` -> `module_check`
2. invalid `flow` -> `flow_check`
3. invalid `project` -> `project_check`

Stable fallback rules:

1. invalid `module` -> `module_stable_verify`
2. invalid `flow` -> `flow_stable_verify`
3. invalid `project` -> `project_stable_verify`

## 5. Relationship To `shared_sync`

`shared_sync` remains the shared-governance internal flow.

Responsibility split:

1. `shared_sync` owns shared-specific impact discovery, shared-specific exception handling, exception-to-generic-input conversion, and shared-governance stop conditions
2. `impact_sync` owns the generic downstream invalidation and fallback execution once the affected object set is already fixed

Therefore:

1. `shared_sync` may call `impact_sync`
2. `impact_sync` must not replace `shared_sync` as the shared-governance intent or boundary flow

## 6. Non-Goals

`impact_sync` does not:

1. create a new user-facing command
2. replace `shared_ops`
3. replace `module_check`, `flow_check`, or `project_check`
4. replace `module_stable_verify`, `flow_stable_verify`, or `project_stable_verify`
