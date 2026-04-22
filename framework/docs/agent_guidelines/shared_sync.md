# Shared Sync Flow

## 1. Purpose

`shared_sync` is the internal shared-governance flow that resolves one question only:

1. after a `shared_contract` change, which downstream formal objects are truly affected

It is not the generic cleanup engine.
It is the shared-specific impact-discovery and exception-adaptation layer that runs before generic fallback execution.

This file answers five questions:

1. what `shared_sync` owns
2. what it must read before it can decide scope
3. when it must stop and return control to `shared_escape`
4. what it passes to `impact_sync`
5. which shared-specific exceptions it may interpret

`shared_sync` is not a user-facing command.
Users still enter shared work through `shared_ops:{natural-language request}`.

## 2. Scope

`shared_sync` handles only shared-governance questions created by Shared Contract change.

It may:

1. resolve which `shared_contract` files changed or are explicitly in scope
2. rebuild the repository-wide real binding set from downstream truth files
3. detect `bound_modules` metadata drift
4. determine the affected downstream object set for:
   - `module`
   - `flow`
   - `project`
5. interpret shared-specific execution-local exceptions such as:
   - `current_stable_landing_module`
   - `stable_landing_shared_refs`
   - `bound_modules_only_shared_file_refs`
6. convert those exceptions into exception-resolved downstream impact input
7. pass the final affected object set to `impact_sync`

It does not:

1. rewrite downstream truth files
2. delete downstream process files directly
3. update `_status.md` directly
4. replace `impact_sync`
5. replace `shared_escape`

## 3. Preconditions

Before `shared_sync` runs:

1. read `specflow/framework/docs/agent_guidelines/spec_policy.md`
2. read `specflow/framework/docs/agent_guidelines/command_policy.md`
3. read `specflow/framework/docs/agent_guidelines/impact_sync_policy.md`
4. read `docs/specs/_status.md`
5. read the current in-scope `shared_contract` files under:
   - `docs/specs/shared_contracts/stable/`
   - `docs/specs/shared_contracts/candidate/`
6. if the current task changed any current-layer truth file or binding source used to derive downstream scope, that writeback must already be present before `shared_sync` computes impact

Execution-local caller inputs may include:

1. `current_stable_landing_module`
   - use only when the same round has just landed stable truth and must not invalidate that same landing merely because the new stable shared file now exists
   - this input declares only which stable module may use the landing exception
2. `stable_landing_shared_refs`
   - use only when the caller can name the exact shared refs written by that same landing round
   - this input is required whenever `current_stable_landing_module` is present
3. `bound_modules_only_shared_file_refs`
   - use only when the caller has already proven that the current-round delta for those exact Shared Contract files is limited to `bound_modules` metadata

`shared_sync` must not invent either input when the caller did not provide it.

## 4. Procedure

### 4.1 Build Current Shared View

1. load the current `shared_contract` files from repository truth
2. record for each current file:
   - `shared_contract_id`
   - `layer`
   - `file_ref`
   - `version_ref`
   - current fingerprint
   - declared `bound_modules`

### 4.2 Rebuild Real Binding Set

1. read current object rows from `docs/specs/_status.md`
2. rebuild the real binding set from downstream formal truth:
   - `module.shared_contract_refs`
   - `flow.shared_contract_refs`
   - `project.shared_contract_refs`
3. treat downstream `shared_contract_refs` as the only formal source of which shared files are currently bound
4. treat `bound_modules` only as declarative metadata

### 4.3 Derive Affected Object Set

1. include any downstream object whose current formal `shared_contract_refs` bind:
   - a changed shared file
   - a changed layer of a named `shared_contract_id`
   - a changed shared binding introduced by the current task
2. do not include a sibling layer only because it shares the same `shared_contract_id`
3. apply the dependency direction from `spec_policy.md`:
   - `shared_contract -> module/flow/project`
4. if repository truth is insufficient to determine the real binding set safely:
   - stop `shared_sync`
   - do not perform local fallback cleanup
   - return control to `shared_escape`

### 4.4 Interpret Shared-Specific Exceptions

`shared_sync` may apply only these shared-specific exception rules:

1. `bound_modules`-only exception
   - if a changed Shared Contract file is explicitly declared in `bound_modules_only_shared_file_refs`, do not treat that metadata-only delta as downstream invalidation by itself
2. current stable landing exception
   - if `current_stable_landing_module` is present, apply the exception only to the exact shared refs explicitly listed in `stable_landing_shared_refs`
   - if any other selected shared ref still invalidates that same module, do not suppress that invalidation

`shared_sync` must not infer either exception from fingerprint difference alone.

### 4.5 Hand Off To `impact_sync`

After the affected downstream object set and exception set are fixed:

1. convert shared-specific exceptions into already-resolved downstream impact input:
   - final `invalidating_shared_refs`
   - final `explicit_fallback_scope`
   - final `allowed_shared_snapshot_mismatch_file_refs`
2. pass that exception-resolved downstream object set to `impact_sync`
2. let `impact_sync` perform:
   - candidate-side process cleanup
   - `_status.md` fallback writeback
   - stable-side reroute to the correct verify command

`shared_sync` remains responsible for the scope and exception judgment.
`impact_sync` remains responsible for the generic fallback execution.
`impact_sync` must not receive raw `current_stable_landing_module`, raw `stable_landing_shared_refs`, or raw `bound_modules_only_shared_file_refs`.

## 5. Output Contract

The output must report at least:

1. which `shared_contract` files or ids were treated as changed or in scope
2. the affected downstream object set grouped by:
   - `module`
   - `flow`
   - `project`
3. any `bound_modules` metadata drift
4. which execution-local shared exceptions were applied
5. whether control was passed to `impact_sync`
6. whether control was returned to `shared_escape`

## 6. Stop Conditions

`shared_sync` stops only when one of these is true:

1. the affected downstream object set is fully determined and passed to `impact_sync`
2. no affected downstream object exists and the current shared scope has been reconciled
3. repository truth is insufficient and control has been returned to `shared_escape`

## 7. Non-Goals

`shared_sync` does not:

1. create an independent lifecycle for `shared_contract`
2. replace `shared_ops`
3. replace `impact_sync`
4. write downstream truth files
5. perform generic fallback execution by itself
