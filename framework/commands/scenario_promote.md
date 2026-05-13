# Scenario Promote Command

## 1. Purpose

`scenario_promote:{scenario}` promotes the current candidate scenario into the new stable scenario truth.

## 2. Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `scenario_promote`-local entry, output, and stop rules.

Process-file consumption for `_verify_result/scenario/{scenario}.md` must follow `specflow/framework/process_snapshot_contract.md` Section 9. When deterministic snapshot validation tooling is available for scenario process files, `snapshot validate-process --process verify` is the mandatory tool-backed validation step before treating `_verify_result/scenario/{scenario}.md` as consumable, reporting the promotion handoff as valid, or advancing lifecycle state.

Stable acceptance summary writeback under `_verify_result/stable/scenario/{scenario}.md` must follow the stable summary field contract in `specflow/framework/process_snapshot_contract.md` Section 3.3. It is not validated with `snapshot validate-process` because that tooling command supports only `check`, `plan`, and `verify` process kinds.

Before reading `_verify_result/scenario/{scenario}.md` as a usable promotion input, run `specflowctl command preflight --command scenario_promote --object-type scenario --object {scenario}`. If command preflight is unavailable, run `snapshot validate-process --object-type scenario --object {scenario} --process verify` explicitly. Stable acceptance summary fingerprints must be computed under the process snapshot contract; shell checksums and manual hashes are not authoritative.

## 3. Preconditions

1. `_status.md` says `Object Type=scenario`, `Active Layer=candidate`, `Next Command=scenario_promote`
2. current valid `_verify_result/scenario/{scenario}.md` exists
3. read required current candidate appendix files explicitly linked by the candidate scenario
4. read `specflow/framework/candidate_handoff_contract.md`
5. read `specflow/framework/recovery_policy.md` before promotion

## 4. Procedure

1. run command preflight for `scenario_promote:{scenario}` and stop before stable truth-file mutation if authoritative validation is unavailable
2. read and re-check the latest `_verify_result/scenario/{scenario}.md`
3. read and re-check `docs/specs/scenarios/candidate/c_scenario_{scenario}.md` and all required current candidate appendix files
4. validate `_verify_result/scenario/{scenario}.md` according to the `scenario_verify -> scenario_promote` handoff in `specflow/framework/candidate_handoff_contract.md`
   - the verify result must cover the current candidate scenario acceptance item `id` set exactly
   - each current-gate acceptance item must have an allowed promotion state according to `scenario_verify` and any applicable downgrade policy
5. if `_verify_result/scenario/{scenario}.md` is invalid, stop before truth-file mutation and use only the preflight or `snapshot validate-process` result plus command-local evidence rules to classify the failure:
   - if candidate truth, repository mapping snapshot, required unit bindings, bound Rule snapshots, or formal global baseline snapshots drifted, delete current-round scenario `_check_result/scenario/{scenario}.md` and `_verify_result/scenario/{scenario}.md`, update `_status.md` to `Next Command=scenario_check`, and use the matching standardized code: `truth_drift`, `binding_drift`, `rule_drift`, or `baseline_drift`
   - if only the scenario check gate process shape is malformed while current scenario truth and bindings still match, delete `_check_result/scenario/{scenario}.md`, update `_status.md` to `Next Command=scenario_check`, and use `failure_layer=gate_layer`
   - if only verification coverage is stale, malformed, or incomplete while the check gate still covers current truth, delete `_verify_result/scenario/{scenario}.md`, update `_status.md` to `Next Command=scenario_verify`, and use `fallback_reason_code=evidence_incomplete` with `failure_layer=evidence_layer`
   - the deterministic command closure for these fallback cases may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> command close --command scenario_promote --object-type scenario --object {scenario} --outcome <verify_invalid_truth|verify_invalid_binding|verify_invalid_baseline|verify_invalid_rule|verify_invalid_gate|verify_invalid_evidence> --notes <status-note> --apply`
6. before the first truth-file mutation, resolve every current `unit_refs` and `rule_refs` entry in the candidate scenario:
   - each `unit_refs` entry must resolve to existing stable-layer unit truth
   - each `rule_refs` entry must resolve to existing stable-layer Rule truth
   - `rule_refs=none` is valid only when the candidate scenario formally binds no Rule
   - do not treat `unit_snapshot`, `rule_snapshot`, `bound_objects`, repository history, or directory shape as a replacement for the formal refs
   - if any dependency is candidate-layer, missing, or not safely resolvable, stop before stable writeback, keep candidate semantics, report the affected dependency and its current legal next step from `_status.md` when present, use `fallback_reason_code=stable_dependency_not_ready` with `failure_layer=dependency_readiness_layer`, and keep `Next Command=scenario_promote`
   - the deterministic command closure for dependency readiness failure may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> command close --command scenario_promote --object-type scenario --object {scenario} --outcome dependency_not_ready --reason stable_dependency_not_ready --notes <status-note> --apply`
   - after dependency landing, retry `scenario_promote` by revalidating the current verify handoff and stable dependency readiness; require fresh `scenario_check` and `scenario_verify` only when scenario truth, scenario bindings, repository mapping, Rule bindings, baseline, acceptance item ids, or verification evidence changed
7. continue only when candidate truth, verification coverage, required bindings, and stable dependency readiness still remain valid
8. before the first truth-file mutation, capture the recovery baseline required by `recovery_policy.md`
9. write `docs/specs/scenarios/stable/s_scenario_{scenario}.md`
   - candidate-only frontmatter fields must not be copied into stable
   - if current-round candidate appendix files exist, in the same promotion round either migrate retained content to `docs/specs/scenarios/stable/appendix/`, absorb the content into `docs/specs/scenarios/stable/s_scenario_{scenario}.md`, or delete candidate appendix files no longer needed
   - delete evidence appendix files by default because they are current-round evidence, not stable behavior truth
   - absorb only the small amount of background needed for stable readers into the stable main Spec; do not migrate evidence appendix files as stable appendix files unless a command-specific rule explicitly makes them stable supporting truth
   - before candidate cleanup, the stable main Spec must not point to `docs/specs/scenarios/candidate/appendix/c_scenario_{scenario}_*.md`
10. write the minimal stable acceptance coverage summary for this promoted round before current-round verify cleanup:
   - target path: `docs/specs/_verify_result/stable/scenario/{scenario}.md`
   - record the promoted stable truth file, version, fingerprint, acceptance item `id` set, each item's final verification status, and the key evidence source refs from the current `_verify_result/scenario/{scenario}.md`
   - this summary is not behavior truth and must not replace the stable scenario Spec's `Testability / Acceptance Criteria` section
   - if this summary cannot be written while promotion otherwise needs to delete the current `_verify_result/scenario/{scenario}.md`, stop before cleanup rather than losing the only acceptance coverage record for the promoted round
11. write `_status.md`:
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=scenario_fork`
   - the deterministic command closure may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> command close --command scenario_promote --object-type scenario --object {scenario} --outcome promoted --notes <status-note> --apply`
   - command close writes `Candidate=no` before it deletes the candidate main file and current process files through success cleanup
12. only after `_status.md` has already been updated to `Candidate=no`, delete:
   - `docs/specs/scenarios/candidate/c_scenario_{scenario}.md`
   - current-round scenario candidate appendix files after the Step 9 appendix handling is complete
   - current-round scenario `_check_result/scenario/{scenario}.md`
   - current-round scenario `_verify_result/scenario/{scenario}.md`
   - command close performs this success cleanup for the `promoted` outcome
13. if the command is interrupted after promotion internals started but before final cleanup finished, run incomplete promotion recovery according to `recovery_policy.md` instead of claiming success

## 5. Stop Conditions

1. if authoritative validation is unavailable, the command stops before stable truth-file mutation and does not claim promotion, stable summary writeback, cleanup, or lifecycle advancement
2. if the verify handoff is invalid before stable truth-file mutation, the command has completed the selected fallback cleanup and `_status.md` points to the fallback target selected by the failure layer
3. if stable dependency readiness fails, no stable scenario truth is written, candidate semantics remain active, `_status.md` keeps `Next Command=scenario_promote`, and the affected dependency plus its current legal next step is reported when `_status.md` can provide it
4. if the stable acceptance summary cannot be written before current-round verify cleanup, the current-round verify result remains undeleted; if promotion mutation has already started, incomplete promotion recovery is complete before close-out
5. if promotion succeeds, stable scenario truth exists, the stable acceptance summary for the promoted round is written, `_status.md` records `Stable=yes`, `Candidate=no`, `Active Layer=stable`, and `Next Command=scenario_fork`
6. if promotion succeeds, candidate scenario truth, current-round candidate appendix files, current-round scenario check result, and current-round scenario verify result are deleted only after `_status.md` already records candidate removal and after retained appendix content has been migrated, absorbed, or intentionally deleted
7. if promotion internals started but final cleanup did not finish, incomplete promotion recovery restored candidate semantics and the scenario can restart from `scenario_check`

## 6. Output Contract

The output must report:

1. stable truth file write result
2. candidate truth file delete result
3. candidate appendix migration, absorption, or deletion result, including evidence appendix deletion or absorption result
4. `_check_result/scenario/{scenario}.md` and `_verify_result/scenario/{scenario}.md` cleanup result
5. stable acceptance coverage summary write result
6. stable dependency readiness result, including affected dependency refs when promotion stopped before stable writeback
7. lifecycle-state transition result
8. `_status.md` update result
9. `handoff validation result`, including acceptance-item coverage validation and stable dependency readiness validation
10. fallback cleanup result when verification became invalid before promotion could start
11. `fallback_reason_code` if verification became invalid or stable dependency readiness failed
12. `fallback_reason_code=promotion_recovery` when incomplete promotion recovery occurred
13. recovery-state explanation if incomplete promotion occurred
14. the `user-facing close-out block` required by `specflow/framework/command_policy.md` Section 8.6
   - when promotion recovery occurred, also report `resume signal`

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

1. `binding_drift`
2. `truth_drift`
3. `baseline_drift`
4. `rule_drift`
5. `evidence_incomplete`
6. `stable_dependency_not_ready`
7. `promotion_recovery`

## 7. Non-Goals

1. unit promotion
2. changing repository mapping
