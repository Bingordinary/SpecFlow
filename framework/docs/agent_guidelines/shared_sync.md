# Shared Sync Flow

## 1. Purpose

`shared_sync` is the internal flow that closes impact after `shared_contract` truth changes.

It answers four questions:

1. which modules still hold outdated shared bindings or snapshots
2. what the smallest actionable fallback step is for each affected module
3. which candidate-side process files must be deleted so invalid gates cannot be reused
4. when `shared_sync` must stop and return control to `shared_escape` instead of inventing its own stop path

This is not the user-facing command entry.
The user reaches it through `shared_ops:{natural-language request}`.

---

## 2. Scope

By default it handles:

1. version, body, layer, or binding changes under `docs/specs/shared_contracts/candidate/` and `docs/specs/shared_contracts/stable/`
2. whether each module's current-layer `Global Constraint Alignment.shared_contract_refs` and process-file snapshots still match the exact shared layer and file the module currently binds
3. unified fallback and cleanup for `_status.md`, `_check_result`, `_plans`, and `_verify_result` when modules still hold old snapshots
4. reporting `bound_modules` drift that must be fixed in the command responsible for the binding change
5. stopping and returning control to `shared_escape` when repository truth is insufficient to determine the real binding set safely

It does not:

1. rewrite module body content
2. replace `spec_flow_review`
3. replace `cand_check`, `stable_verify`, or `cand_promote`
4. decide whether a `shared_contract` should become `system_constraints`
5. define an independent checkpoint path for unstable shared-boundary cases

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/docs/agent_guidelines/spec_policy.md`
2. read `specflow/framework/docs/agent_guidelines/command_policy.md`
3. read `docs/specs/_status.md`
4. read the current `shared_contract` files under `docs/specs/shared_contracts/candidate/` and `docs/specs/shared_contracts/stable/`
5. identify the affected-object source:
   - if the current task changed `docs/specs/shared_contracts/**`, use those changed files to resolve the changed `shared_contract_id` plus layer/file set first
   - if the current task changed any module's `shared_contract_refs`, include those modules directly in the affected-module set
   - if `shared_ops` routed in from a named shared target, resolve that target first
6. scan `_status.md` and build the repository-wide current-layer module set
7. if a shared file changed or a shared target was named, read every current-layer module main file needed to derive the real binding set from `shared_contract_refs`
8. read each affected module's current-layer main file according to `_status.md`
9. if the task may modify `_status.md`, process files, or other commit-triggering governance files, read the Git closure rules first
10. if the current task changed which layer now carries a directly affected module's formal binding source, `_status.md` must already be updated before this flow builds the current-layer module set
11. if this flow was entered from a still-closing `cand_promote` round, carry that promoted module as the current promotion owner for same-round stable-invalidation judgment

---

## 4. Procedure

1. build the current `shared_contract` view:
   - read the files that currently exist under `docs/specs/shared_contracts/candidate/` and `docs/specs/shared_contracts/stable/`
   - record each object's `shared_contract_id`, `layer`, `file_ref`, `shared_version`, current body fingerprint, and `bound_modules`
2. build the repository-wide module current-layer binding index from the already-updated `_status.md`:
   - enumerate formal modules from `_status.md`
   - read `Global Constraint Alignment.shared_contract_refs` from each module's current-layer main file needed for binding resolution
   - interpret each module's `shared_contract_refs` through the Shared Contract binding contract from `specflow/framework/docs/agent_guidelines/spec_policy.md` Section 6.1 before deriving the real binding set
   - treat module `shared_contract_refs` as the only formal source of which modules currently bind which shared truth, which layer they bind, and which exact file currently carries that binding
   - treat `bound_modules` only as declarative metadata
3. derive the affected module set:
   - include modules whose `shared_contract_refs` point to any changed or named Shared Contract file, or to the changed layer of a named `shared_contract_id`
   - include modules whose own `shared_contract_refs` changed in the current task
   - do not include modules bound only to the sibling layer of the same `shared_contract_id` unless their own binding changed in the current task
   - do not use `bound_modules` as the sole source for deciding which modules are affected
   - if current repository truth is still insufficient to determine the real binding set or affected-module set safely:
     - stop `shared_sync` before local fallback cleanup
     - do not emit a `shared_sync`-local checkpoint
     - return control to `shared_escape` through `shared_ops` so the uncertainty is handled by the shared-governance stop flow
4. build the module current snapshot view:
   - if the module is at `candidate`, read any existing `_check_result/{module}.md`, `_plans/{module}.md`, and `_verify_result/{module}.md`
   - extract `shared_contract_snapshot` from those files when present
   - rebuild the normalized snapshot according to `process_snapshot_contract.md`
5. for each affected module, judge whether its shared binding is still valid:
   - if `shared_contract_refs=none` and the module is not in a changed-binding case, leave it unchanged
   - treat the binding as invalid if the referenced file is missing, the layer mismatches, the file target mismatches, the version reference mismatches, or the module-to-shared relation changed
   - for `candidate` modules, rebuild the snapshot from the exact currently bound Shared Contract files; treat the binding as invalid if any existing process file's `shared_contract_snapshot` differs from that rebuilt snapshot, except when the delta comes only from `bound_modules`
   - for `stable` modules, judge only against bound stable-layer Shared Contract files resolved through the binding contract; treat the binding as invalid if the resolved stable binding target changed in layer, file, or version, or if the current task changed that bound stable file in any way other than a `bound_modules`-only delta
   - exception for the current promotion owner: if the affected module is the promoted module carried into this flow from a still-closing `cand_promote` round, and the changed stable Shared Contract file or stable binding is exactly the post-promotion target written by that same round, do not treat that promoted module as invalid on that basis alone
6. for invalid `candidate` modules:
   - delete `_check_result/{module}.md`
   - delete `_plans/{module}.md`
   - delete `_verify_result/{module}.md`
   - set `Next Command=cand_check` in `_status.md`
   - keep `Candidate=yes` and `Active Layer=candidate`
7. for invalid `stable` modules:
   - do not generate candidate-side file deletions
   - set `Next Command=stable_verify` in `_status.md`
   - keep `Stable=yes` and `Active Layer=stable`
8. if `bound_modules` differs from the real binding set implied by the repository-wide binding index:
   - report governance drift
   - name the command or change owner that must fix it
   - do not invalidate modules on a `bound_modules`-only delta
9. if `_status.md` points to a step later than the real smallest actionable step, correct it
   - the deterministic reconciliation work in Steps 4, 6, 7, 8, and 9 may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared sync-impact [--modules module_a,module_b] [--shared-refs c_shared_x@0.1.0] [--shared-ids shared_x]`
10. finish git close-out if required by policy

---

## 5. Stop Conditions

Stop when one of the following is true:

1. all affected modules have been judged and required fallback or cleanup has been completed
2. no affected modules exist and repository shared state has still been reconciled against current truth
3. repository truth is insufficient to determine the real binding set, so `shared_sync` has stopped without a local checkpoint and returned control to `shared_escape`

---

## 6. Output Contract

The output must include at least:

1. a summary of shared-truth changes
2. the list of affected modules
3. the fallback result for each affected module
4. which changed Shared Contract layer or file caused each affected-module result
5. the list of deleted process files
6. any mismatch between `bound_modules` and the real binding set
7. the standardized `fallback_reason_code` for each affected module
8. any module kept valid under the current-round `cand_promote` owner exception
9. when repository truth was insufficient to continue safely, that `shared_sync` returned control to `shared_escape` and did not issue an independent local checkpoint
10. the git close-out result

Allowed `fallback_reason_code` values:

1. `shared_contract_drift`
2. `binding_drift`
3. `truth_drift`

---

## 7. Non-Goals

`shared_sync` does not:

1. create an independent lifecycle for `shared_contract`
2. directly modify module body text or `shared_contract` body text "just to fix the binding"
3. replace `cand_check` to re-pass candidate closure
4. replace `stable_verify` to re-check stable alignment
5. absorb `shared_contract` conclusions into `system_constraints`
6. silently retarget a module from candidate-layer shared truth to stable-layer shared truth, or the reverse
7. keep unstable stop decisions inside `shared_sync` when the real problem is shared-boundary uncertainty
