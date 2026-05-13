# Unit Promote Command

## 1. Purpose

This command promotes the specified unit's `candidate` into the new `stable`.

## 2. Scope

By default it handles:

1. promoting the candidate version into the formal version
2. updating state files
3. cleaning this round's candidate and process files
4. updating `s_g_rule_repository_baseline.md` when a closed unit-carried global proposal is ready
5. consuming the `unit_verify -> unit_promote` handoff only when verification still covers the current round
6. preserving the promoted round's minimal stable acceptance coverage summary before deleting current-round process files
7. retargeting other candidate-layer units from the promoted candidate Rule ref to the same-round stable Rule ref when the retarget is only a same-`rule_id`, same-`rule_version`, candidate-to-stable layer landing

### 2.1 Command Read Summary

Read this summary before the detailed rules below.
It is navigation only and does not replace the preconditions, procedure, stop conditions, or output contract.

1. `unit_promote` exists to land the verified candidate as the active stable truth and clean the completed candidate round.
2. The minimum inputs are the current candidate, latest valid `_verify_result/unit/{unit}.md`, required appendix files, touched or bound Rule files, current global baseline, and recovery policy.
3. Promotion must stop before truth-file mutation if verification, bindings, rule truth, or baseline no longer match the current round.
4. If mutation has started and promotion cannot safely complete, incomplete-promotion recovery restores candidate semantics and sends the object back to the smallest restart point.
5. Rule topology, terminal-state handling, `promotion_owner_unit`, `release-version`, and post-promotion `rule_sync` remain detailed closure rules; this summary does not shorten them.

### 2.2 Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `unit_promote`-local entry, output, and stop rules.

Process-file consumption for `_verify_result/unit/{unit}.md` must follow `specflow/framework/process_snapshot_contract.md` Section 9. When deterministic snapshot validation tooling is available for the current process kind, `snapshot validate-process --process verify` is the mandatory tool-backed validation step before treating `_verify_result/unit/{unit}.md` as consumable, reporting the promotion handoff as valid, or advancing lifecycle state.

Stable acceptance summary writeback under `_verify_result/stable/unit/{unit}.md` must follow the stable summary field contract in `specflow/framework/process_snapshot_contract.md` Section 3.3. It is not validated with `snapshot validate-process` because that tooling command supports only `check`, `plan`, and `verify` process kinds.

Before reading `_verify_result/unit/{unit}.md` as a usable promotion input, run `specflowctl command preflight --command unit_promote --object-type unit --object {unit}`. If command preflight is unavailable, run `snapshot validate-process --object-type unit --object {unit} --process verify` explicitly. Stable acceptance summary fingerprints must be computed under the process snapshot contract; shell checksums and manual hashes are not authoritative.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=unit_promote`
3. a latest valid `_verify_result/unit/{unit}.md` still covers the current candidate, current implementation, and current formal global baseline state
4. implementation alignment is complete and no blocking verification issue remains
6. read required candidate appendix files and any Rule files already bound by the unit candidate or otherwise already known to be touched by this promotion round, and decide how each touched Rule file will be handled after promotion
   - if any touched candidate-layer Rule file already has a stable-layer sibling, also read that file's `promotion_owner_unit`
7. read `specflow/framework/recovery_policy.md` before promotion
8. if the round may create, update, or delete any unit `rule_refs` value or any file under `docs/specs/rules/**`, read `specflow/framework/rule_sync.md` before promotion
9. if the unit candidate currently binds any candidate-layer Rule file, or if the round may change the layer, version, or terminal state of any touched Rule file, read `docs/specs/_status.md` and every affected unit or scenario current-layer main file needed to derive the real repository-wide binding set from `rule_refs` before file mutation starts
10. if repository truth is insufficient to derive that real binding set safely, do not start file mutation; reroute through natural-language rule governance from current repository truth instead of guessing promotion-local topology
11. if same-round stable landing retargeting may be required, read every candidate-layer unit main file that currently binds the landing candidate Rule ref and include those files and their current-round process files in the recovery baseline before mutation starts
12. if deleting `docs/specs/units/candidate/c_unit_{unit}.md` may leave formal Spec references behind, scan `docs/specs/units/**` and `docs/specs/scenarios/**` before mutation starts and include every file and status row that may be mechanically retargeted in the recovery baseline
13. read `specflow/framework/candidate_intent_policy.md` and the selected intent standard for the current candidate

## 4. Procedure

1. run command preflight for `unit_promote:{unit}` and stop before truth-file mutation if authoritative validation is unavailable
2. read and re-check the latest `_verify_result/unit/{unit}.md`
3. read `docs/specs/units/candidate/c_unit_{unit}.md` and all required appendix files
4. read `candidate_intent` from the candidate frontmatter and apply the selected intent standard from `candidate_intent_policy.md`
5. validate the full binding relation of `_verify_result/unit/{unit}.md` according to the candidate handoff contract
   - the verify result must cover the current candidate acceptance item `id` set exactly
   - each current-gate acceptance item must have an allowed promotion state according to `unit_verify` and any applicable downgrade policy
6. if `_verify_result/unit/{unit}.md` is invalid, identify the failure layer before cleanup from the preflight or `snapshot validate-process` result and command-local evidence rules only:
   - if code changed after verification or evidence is stale while truth, check gate, and active plan still stand:
     - delete `_verify_result/unit/{unit}.md`
     - use `evidence_layer` and fall back to `unit_verify`
   - if implementation drift against candidate exists:
     - delete `_verify_result/unit/{unit}.md`
     - use `implementation_layer` and fall back to `unit_impl`
   - if the active plan is missing, malformed, not tool-valid, or no longer covers current acceptance ids while the check gate still covers current truth:
     - delete `_plans/draft/{unit}.md`
     - delete `_plans/active/{unit}.md`
     - delete `_verify_result/unit/{unit}.md`
     - use `plan_layer` and fall back to `unit_plan`
   - if the check gate process shape is invalid while current truth and bindings still match:
     - delete `_check_result/unit/{unit}.md`
     - delete `_verify_result/unit/{unit}.md`
     - use `gate_layer` and fall back to `unit_check`
   - if another required binding of `_verify_result/unit/{unit}.md` no longer matches the current round:
     - delete `_check_result/unit/{unit}.md`
     - delete `_plans/draft/{unit}.md`
     - delete `_plans/active/{unit}.md`
     - delete `_verify_result/unit/{unit}.md`
     - use `fallback_reason_code=binding_drift`, `failure_layer=truth_layer`, and fall back to `unit_check`
   - if bound Rule truth, layer, version, or snapshot drifted:
     - delete `_check_result/unit/{unit}.md`
     - delete `_plans/draft/{unit}.md`
     - delete `_plans/active/{unit}.md`
     - delete `_verify_result/unit/{unit}.md`
     - use `fallback_reason_code=rule_drift`, `failure_layer=truth_layer`, and fall back to `unit_check`
   - if candidate truth or formal global baseline changed:
     - delete `_check_result/unit/{unit}.md`
     - delete `_plans/draft/{unit}.md`
     - delete `_plans/active/{unit}.md`
     - delete `_verify_result/unit/{unit}.md`
     - use `truth_layer` and fall back to `unit_check`
   - if the candidate acceptance item set changed after verification:
     - delete `_verify_result/unit/{unit}.md`
     - if the existing `_check_result/unit/{unit}.md` and `_plans/active/{unit}.md` still match the current candidate truth and acceptance item set, fall back to `unit_verify`
     - otherwise delete `_check_result/unit/{unit}.md`, `_plans/draft/{unit}.md`, and `_plans/active/{unit}.md`, then fall back to `unit_check`
     - use `fallback_reason_code=evidence_incomplete` when only verification coverage is stale, or `fallback_reason_code=truth_drift` when candidate truth changed
   - the deterministic command closure for these fallback cases may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> command close --command unit_promote --object-type unit --object {unit} --outcome <verify_invalid_truth|verify_invalid_binding|verify_invalid_baseline|verify_invalid_rule|verify_invalid_plan|verify_invalid_implementation|verify_invalid_gate|verify_invalid_evidence> --notes <status-note> --apply`
7. continue only when bindings, coverage, and gate fields all remain valid
8. before the first file mutation, capture the recovery baseline required by `recovery_policy.md`
   - when promotion dependency reference retargeting may occur, the baseline must include every referencing Spec file, every affected `_status.md` row, and every candidate-side process file that may be deleted because of the retarget
9. confirm that candidate `frontmatter.version` is the new `stable` version for this round
10. if the round touches any Rule file, Rule layer/version target, or Rule terminal state, build the repository-wide real binding view for every touched shared item before deciding post-promotion topology:
   - start from `docs/specs/_status.md`
   - read every affected unit or scenario current-layer main file needed to derive which command-target objects currently bind each touched Rule file or sibling layer through `rule_refs`
   - interpret every unit-side and scenario-side `rule_refs` through the Rule binding contract from `specflow/framework/spec_policy.md` Section 6.1 before deriving that real binding view
   - treat unit and scenario `rule_refs` as the formal source of the real binding set rather than `bound_objects`
   - if repository truth is insufficient to state the post-promotion topology safely, stop before file mutation and reroute through natural-language rule governance from current repository truth
11. if the round touches any Rule file, Rule layer/version target, or Rule terminal state, decide for each touched shared item against that repository-wide binding view:
   - determine the post-promotion binding target for the promoted unit stable truth; a promoted stable unit must not keep binding a candidate-layer Rule file
   - if it should remain an independent cross-unit truth after promotion, promote it into `docs/specs/rules/stable/`
   - when this round writes or updates a stable-layer Rule file, use the already-decided candidate `rule_version` for that file; do not invent or bump a Rule version during unit promotion itself
   - when this round writes or updates a stable-layer Rule file from a candidate-layer Rule file that already had a stable-layer sibling before promotion, require that candidate file's `promotion_owner_unit` to equal the promoted unit name; otherwise stop before file mutation and reroute through natural-language rule governance
   - if a candidate-layer Rule file for the same `rule_id` remains in place after this round lands the stable-layer Rule file, rewrite that remaining candidate-layer file in the same round as an explicit next-round draft:
     - set its `rule_version` to the intended next stable version after the just-landed stable file
     - write exactly one next `promotion_owner_unit`
     - do not leave it as a candidate-layer duplicate of the just-landed stable truth
   - if current repository truth is insufficient to define that retained next-round draft or its next `promotion_owner_unit` safely, stop before file mutation and reroute through natural-language rule governance
   - if part of its conclusion has become a project-wide default rule, also absorb that specific conclusion into `s_g_rule_repository_baseline.md`
   - do not absorb a Rule into unit `stable` merely because promotion happened
   - do not treat promotion itself as a reason to delete a still-needed Rule
   - if the round changed a shared item that has both stable-layer and candidate-layer files, resolve which units are expected to remain bound to each layer after promotion from the repository-wide binding view before continuing
   - when another current-layer unit is at `candidate` and currently binds the promoted candidate-layer Rule ref, this command may retarget that unit to the just-written stable-layer Rule ref in the same round only when all of these are true:
     - the candidate and stable Rule refs have the same `rule_id`
     - the candidate and stable Rule refs have the same `rule_version`
     - the target unit is already at `candidate`
     - the target unit's required body-level reference wording can be updated without changing behavior truth, acceptance meaning, or Rule body truth
   - same-round stable landing retargeting must not modify stable-layer unit truth; if a stable unit must be retargeted, stop before file mutation and require `unit_fork` first
   - record every same-round retargeted unit explicitly for the later `rule_sync` call; these retargeted units must fall back to `unit_check` because their current process files still describe the pre-retarget binding
   - do not delete the old candidate-layer Rule file before post-promotion `rule_sync` has run; `rule_sync` must be able to see both the old candidate ref and the new stable ref when it reconciles same-round landing impact
   - if this round's topology change or linked stable `g_` rule absorption would leave a touched Rule file with no formal bound units, this promotion round owns resolving that file's terminal state instead of leaving orphaned rule truth for later cleanup
   - if such a touched file now has no formal bound units and cleanup is legal under `spec_policy.md`, delete it in this round when it has been replaced by the promoted target or when its remaining conclusion has been fully absorbed into `s_g_rule_repository_baseline.md`
   - when same-round stable landing retargeting needs the old candidate-layer Rule file as a `rule_sync` trigger, decide that file's terminal state in this step but defer its physical deletion or next-round draft rewrite until after post-promotion `rule_sync` has completed successfully
   - if such a touched file now has no formal bound units and the round intentionally keeps it as independently authored rule truth, write that same file with:
     - `unbound_retention: intentional`
     - `unbound_retention_reason: <why this unbound state is intentional now>`
     - `unbound_retention_owner: unit_promote`
   - if the required post-promotion truth shape is still unclear, or the round cannot safely judge whether an unbound touched file should be deleted or kept as independently authored rule truth, stop promotion and require rerouting through natural-language rule governance from current repository truth instead of guessing a unit-local-only continuation
12. generate or update `docs/specs/units/stable/s_unit_{unit}.md`
   - candidate-only frontmatter fields must not be copied into stable
   - `candidate_intent`, `repair_basis`, and candidate-only repair guidance such as `Repair Scope` must be removed from the stable landing
   - when `candidate_intent=repair`, the stable version must remain a `PATCH` version of the repair basis unless another rule-governed write in the same round requires a higher version
13. write the minimal stable acceptance coverage summary for this promoted round before current-round verify cleanup:
   - target path: `docs/specs/_verify_result/stable/unit/{unit}.md`
   - record the promoted stable truth file, version, fingerprint, acceptance item `id` set, each item's final verification status, and the key evidence source refs from the current `_verify_result/unit/{unit}.md`
   - this summary is not behavior truth and must not replace the stable Spec's `Testability / Acceptance Criteria` section
   - if this summary cannot be written while promotion otherwise needs to delete the current `_verify_result/unit/{unit}.md`, stop before cleanup rather than losing the only acceptance coverage record for the promoted round
14. if current-round candidate appendix files exist, in the same promotion round either:
   - migrate retained content to `docs/specs/units/stable/appendix/` or an equivalent dedicated subdirectory
   - absorb the content into `docs/specs/units/stable/s_unit_{unit}.md`
   - delete candidate appendix files no longer needed
   - delete evidence appendix files by default because they are current-round evidence, not stable behavior truth
   - absorb only the small amount of background needed for stable readers into the stable main Spec; do not migrate evidence appendix files as stable appendix files unless a command-specific rule explicitly makes them stable supporting truth
15. do not delete `docs/specs/units/candidate/c_unit_{unit}.md` until `_status.md` has already been updated to `Candidate=no`
16. record the promoted stable state that final command closure must write:
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=unit_fork`
   - do not execute the `promoted` command close yet
   - final command closure must wait until stable acceptance summary writeback, required appendix handling, Rule release-version work, and promotion dependency reference retargeting are complete
17. do not update `docs/specs/repository_mapping.md` only because this promotion changed the active layer from `candidate` to `stable`; the current unit main Spec path is resolved from `_status.md` plus the `unit_default` truth-surface rule
18. if the round lands a new stable Rule version or changes a stable Rule `rule_version`, do not hand-edit consumer `rule_refs`; execute `specflow/tooling/bin/specflowctl-<os>-<arch> rule release-version --rule-id <rule-id> --from-ref <old-stable-rule-ref> --to-ref <new-stable-rule-ref>` after the stable Rule file exists and before claiming promotion closure
   - `release-version` is the only command allowed to retarget stable current-layer consumers; it auto-forks those consumers and rewrites only the candidate `rule_refs`
   - if a remaining touched Rule file now has one or more formal consumers after this promotion round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
18a. before deleting the current-round candidate main file, perform promotion dependency reference retargeting:
     - build the cross-reference scan set from existing main Spec and appendix files under `docs/specs/units/**` and `docs/specs/scenarios/**`
     - scan for references to the promoted candidate unit through:
       - `docs/specs/units/candidate/c_unit_{unit}.md`
       - relative paths that resolve to `docs/specs/units/candidate/c_unit_{unit}.md`, including `../candidate/c_unit_{unit}.md` and `./c_unit_{unit}.md`
       - version refs matching `c_unit_{unit}@<promoted-version>`
     - for each reference that can be mechanically retargeted to the same unit at stable layer, auto-update only the reference target:
       - path refs must point to `docs/specs/units/stable/s_unit_{unit}.md` or the correct relative path from the referencing file
       - version refs must change to `s_unit_{unit}@<promoted-version>`
     - mechanical retargeting must not change behavior truth, acceptance meaning, Rule binding, ownership boundary, or any explanatory claim beyond the directly required path or version-ref wording
     - if a reference sentence depends on candidate-only meaning, such as saying the dependency is temporary, not formally accepted, or only valid while the target is candidate-layer truth, stop before deleting the candidate file and report a blocking prerequisite action with the affected file and reference text
     - if a current-layer candidate unit is retargeted, keep its candidate truth but set its next command to `unit_check`, and delete `_check_result/unit/{unit}.md`, `_plans/draft/{unit}.md`, `_plans/active/{unit}.md`, and `_verify_result/unit/{unit}.md` for that retargeted unit when present
     - if a current-layer candidate scenario is retargeted, keep its candidate truth but set its next command to `scenario_check`, and delete `_check_result/scenario/{scenario}.md` and `_verify_result/scenario/{scenario}.md` for that retargeted scenario when present
     - if a current-layer stable unit other than the promoted unit is retargeted, do not fork it and do not create candidate truth; preserve its stable state and set its next command to `unit_stable_verify`
     - if a current-layer stable scenario is retargeted, do not fork it and do not create candidate truth; preserve its stable state and set its next command to `scenario_stable_verify`
     - if the promoted unit's own newly written stable file still contains a mechanically retargetable reference to its just-promoted candidate file, retarget that reference as part of the promoted stable landing and keep the promoted unit's successful follow-up state at `Next Command=unit_fork`
     - if a non-current-layer historical Spec file is retargeted, record the retarget but do not update `_status.md` for that object only because the historical file changed
     - record every retargeted file, every status-row update, every deleted process file, and every non-retargeted blocking reference in the output contract
19. after Step 18a completes or proves that no retargeting is needed, close the command with the `promoted` outcome:
   - the deterministic command closure may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> command close --command unit_promote --object-type unit --object {unit} --outcome promoted --notes <status-note> --apply`
   - command close writes the promoted stable state from Step 16 before it deletes:
   - `docs/specs/units/candidate/c_unit_{unit}.md`
   - current-round candidate appendix files after the Step 14 appendix handling is complete
   - `_check_result/unit/{unit}.md`
   - `_plans/draft/{unit}.md`
   - `_plans/active/{unit}.md`
   - `_verify_result/unit/{unit}.md`
   - `process cleanup-success` is a low-level cleanup tool and is not the standard `unit_promote` closing entry
20. if the command is interrupted after promotion internals started but before final cleanup finished, run incomplete promotion recovery according to `recovery_policy.md` instead of claiming success
21. if the round changed any unit `rule_refs` value or any file under `docs/specs/rules/**`, run `rule_sync` only after `_status.md` already reflects the promoted stable layer and Step 18 has completed any required `release-version`, even when no additional affected object is known yet
   - this post-promotion `rule_sync` closes external affected-object fallout and Rule-state reconciliation; it must not overturn the promoted unit's own successful stable landing merely because the same promotion round also wrote the stable Rule file or stable binding that the promoted unit now legally uses
   - pass execution-local `current_stable_landing_unit={unit}` into that `rule_sync` run
   - pass execution-local `stable_landing_rule_refs=<exact-shared-ref-list-written-by-this-landing>` into that same `rule_sync` run; `current_stable_landing_unit` alone is not sufficient
   - when same-round stable landing retargeting changed other candidate units, pass execution-local `retargeted_units=<exact-unit-list>` into that same `rule_sync` run
   - the deterministic reconciliation part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> rule sync-impact --rule-refs <old-candidate-rule-ref>,<new-stable-rule-ref> --stable-landing-unit {unit} --stable-landing-rule-refs <exact-stable-landing-rule-ref-list> --retargeted-units <exact-retargeted-unit-list>` or the corresponding narrowed form when no retargeted unit exists; at least one rule trigger input must already be known before this deterministic execution starts
   - if that post-promotion `rule_sync` returns control because repository truth is still insufficient to continue safely, do not claim promotion success:
     - immediately run incomplete promotion recovery according to `recovery_policy.md`
     - after recovery, require rerouting through natural-language rule governance from the restored candidate-layer repository truth
     - do not leave the repository in partially promoted semantics while waiting for rule-governance clarification
22. after a successful post-promotion `rule_sync`, complete any deferred terminal-state action for the old candidate-layer Rule file used as a same-round stable landing trigger:
   - delete it when it has no remaining formal bound units and has been replaced by the stable landing Rule file
   - rewrite it as an explicit next-round draft only when current repository truth requires a remaining candidate-layer draft with a next `promotion_owner_unit`
   - if neither deletion nor next-round draft rewrite is safe from current truth, run incomplete promotion recovery instead of claiming success

## 5. Stop Conditions

1. promotion succeeded or a blocking reason is explicit
2. `_status.md` is updated to:
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=unit_fork`
3. this round's candidate cleanup is complete
4. the stable acceptance coverage summary for the promoted round is written before current-round verify cleanup
5. if verification became invalid, the command stopped and `_status.md` fell back appropriately
6. if the command entered incomplete-promotion recovery state, candidate semantics were restored and the unit can restart from `unit_check`
7. if post-promotion `rule_sync` could not continue safely, incomplete promotion recovery is complete and the next required action is rerunning natural-language routing from restored candidate truth so it can re-enter rule governance before any later candidate-chain restart
8. if a candidate-layer Rule sibling remains after promotion, its next-round draft state is already explicit for the current repository truth
9. if same-round stable landing retargeting occurred, every retargeted candidate unit has fallen back to `unit_check` and its unsafe current-round process files have been deleted

## 6. Output Contract

1. promotion conclusion
2. formal version confirmation result
3. candidate intent promotion result
4. file and state update result
5. stable `g_` rule linked-promotion result
6. post-promotion Rule topology result, including which rule files remain at stable, which remain at candidate, and which binding target the promoted unit now uses
7. `promotion_owner_unit` validation result for each touched candidate-layer Rule file that already had a stable-layer sibling before promotion
8. next-round draft rewrite result for each candidate-layer Rule file retained after this promotion created or updated the stable-layer sibling
9. confirmation that every remaining touched Rule file omits `bound_objects`
10. terminal-state result for any touched Rule file that became unbound in this round
11. Rule reconciliation result when the round changed rule truth or bindings
12. candidate appendix migration, absorption, or deletion result, including evidence appendix deletion or absorption result
13. stable acceptance coverage summary write result
14. cleanup result
15. `handoff validation result`, including acceptance-item coverage validation
16. fallback cleanup result when verification became invalid before promotion could start
17. `fallback_reason_code` if verification became invalid
18. fallback reason if verification became invalid
19. `fallback_reason_code=promotion_recovery` when incomplete promotion recovery occurred
20. recovery-state explanation if incomplete promotion occurred
21. when post-promotion `rule_sync` was executed, the passed `current_stable_landing_unit` value
22. when post-promotion `rule_sync` was executed, the passed `stable_landing_rule_refs` value
23. when `release-version` was executed, the exact `rule_id`, `from-ref`, `to-ref`, forked consumers, directly updated consumers, and deleted process files
24. when same-round stable landing retargeting occurred, the exact retargeted units, the old candidate Rule refs, the new stable Rule refs, and each retargeted unit's fallback result
25. when promotion stopped because post-promotion Rule topology, retained candidate next-round draft shape, `promotion_owner_unit`, same-round retarget shape, unbound-file terminal state, or post-promotion `rule_sync` uncertainty was unclear, the required next step through natural-language rule governance
26. promotion dependency reference retarget result, including retargeted files, non-current historical retargets, status-row updates, deleted process files, and any blocking prerequisite action caused by candidate-only reference meaning
27. follow-up state explanation
   - when promotion succeeds, the follow-up state must explicitly confirm:
     - `Stable=yes`
     - `Candidate=no`
     - `Active Layer=stable`
     - `Next Command=unit_fork`
   - when promotion recovery occurred because post-promotion `rule_sync` could not continue safely, the follow-up state must explicitly confirm:
     - `Stable=yes|no` restored from the recovery baseline
     - `Candidate=yes`
     - `Active Layer=candidate`
     - `Next Command=unit_check`
     - `resume through natural-language rule governance` before any later promotion retry
28. the `user-facing close-out block` required by Section 8.6 of `specflow/framework/command_policy.md`
   - when promotion recovery or rule-governance reroute occurred, also report `resume signal`
   - `current state` must match the post-promotion or post-recovery state actually restored in `_status.md`

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

1. `truth_drift`
2. `binding_drift`
3. `baseline_drift`
4. `rule_drift`
5. `implementation_deviation`
6. `evidence_incomplete`
7. `promotion_recovery`

## 7. Non-Goals

1. redesigning `candidate`
2. replacing implementation verification with promotion
3. automatically opening the next candidate round
4. maintaining an independent stable `g_` rule candidate file
