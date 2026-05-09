# Unit Fork Command

## 1. Purpose

This command forks a new `candidate` Spec from an existing `stable`.

Goals:

1. open a new candidate-design round from the current formal version
2. provide candidate truth for downstream `unit_check / unit_plan / unit_impl`
3. update `docs/specs/_status.md`

## 2. Scope

By default it handles:

1. a new upgrade round for a unit that already has `stable`
2. deriving candidate truth from formal truth

### 2.1 Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `unit_fork`-local entry, output, and stop rules.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=unit_fork`
3. the unit already has `stable`
4. read `docs/specs/_status.md`
5. read any stable appendix files explicitly referenced by `s_unit_{unit}.md`
6. read bound stable Rule files if `rule_refs` is not empty
7. if the round will create, update, or delete any unit `rule_refs` value or any file under `docs/specs/rules/**`, read `rule_sync.md`
8. if the round may remove, retarget, or otherwise change an existing Rule binding, read every current-layer unit or scenario main file needed to derive the real binding set of each touched Rule from `rule_refs`
9. read `specflow/framework/onboarding_decision_policy.md` for stable-fork candidate source handling

## 4. Procedure

1. read `docs/specs/_status.md`
2. read `s_g_rule_repository_baseline.md` if it exists; otherwise continue with no formal global baseline
3. read `docs/specs/units/stable/s_unit_{unit}.md` and any explicitly referenced appendix files
4. read bound stable Rule files if any
5. apply the stable-fork candidate source rule from `specflow/framework/onboarding_decision_policy.md` Section 6.1
   - if the fork uses only stable formal truth plus the current round's selected design changes, prepare `source_basis=new_design` and `evidence_appendix_ref=none`
   - if the fork selects behavior from implementation, tests, runtime behavior, historical material, or other non-stable evidence, prepare the required `source_basis`, `evidence_appendix_ref`, and candidate evidence appendix in the same round
   - if that source decision or evidence appendix is not ready, stop before writing the candidate main Spec
6. determine the target formal version for this round:
   - compatible new capability -> next `MINOR`
   - incompatible change -> next `MAJOR`
   - compatible fix or alignment -> next `PATCH`
7. generate `docs/specs/units/candidate/c_unit_{unit}.md` from the current stable file and write the prepared `source_basis` and `evidence_appendix_ref` fields in the same candidate write
8. set candidate `frontmatter.version` to that target version
9. ensure the candidate `Testability / Acceptance Criteria` section uses explicit acceptance items that satisfy `spec_writing_guide.md` Section 5
   - if the stable source already has structured acceptance items, carry them forward and edit only the items affected by the new round
   - if the stable source still has historical prose-only acceptance text, convert the relevant current acceptance scope into explicit items in the candidate instead of preserving the ambiguity
11. re-check `rule_refs`:
   - interpret and rewrite that field using the Rule binding contract from `specflow/framework/spec_policy.md` Section 6.1
   - judge Rule bindings independently from whether `s_g_rule_repository_baseline.md` exists
   - if the stable layer depended on rule files and the candidate still depends on the same unchanged rule truth, keep binding those existing rule files in the candidate
   - create or bind candidate-layer rule files only when the current round changes the rule truth itself
   - write `rule_refs=none` only when the current round no longer reuses rule truth
   - do not write `rule_refs=none` merely because a rule-truth change for this round has not yet been formalized
12. if Step 11 removes or retargets any existing Rule binding:
   - derive the real repository-wide binding set of each touched Rule from current-layer unit and scenario `rule_refs` plus the target unit candidate writeback prepared in Step 11
   - if repository truth is insufficient to decide whether any touched Rule file would become unbound after this round, stop and reroute through natural-language rule governance from current repository truth instead of leaving cleanup ownership implicit
13. if the round changed rule bindings or rule files, resolve Rule terminal state and `bound_objects` in the same round:
   - if a touched Rule file would have no formal bound units after this round, in the same round either delete it when cleanup is legal under `spec_policy.md` or explicitly keep it as independently authored rule truth by writing that file with:
     - `unbound_retention: intentional`
     - `unbound_retention_reason: <why this unbound state is intentional now>`
     - `unbound_retention_owner: unit_fork`
   - reject closure if neither deletion nor explicit keep-writeback has happened for a touched now-unbound Rule file
   - if a touched Rule file still has one or more formal bound units after this round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
   - update `bound_objects` only as declarative metadata so each remaining touched Rule file matches the real binding set implied by unit and scenario `rule_refs`
   - the deterministic metadata writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> rule reconcile-bound-objects --units {unit}` and additional `--rule-refs` / `--rule-ids` filters when the active flow has already identified them
14. delete old `_check_result/unit/{unit}.md`, `_verify_result/unit/{unit}.md`, `_plans/draft/{unit}.md`, `_plans/active/{unit}.md`, and previous-round candidate appendix files
   - the deterministic cleanup part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> process cleanup-success --object-type unit --object {unit} --mode unit_fork`
15. update `_status.md`:
   - `Stable=yes`
   - `Candidate=yes`
   - `Active Layer=candidate`
   - `Next Command=unit_check`
   - the deterministic row writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> status set-object --type unit --object {unit} --stable yes --candidate yes --active-layer candidate --next-command unit_check --notes <status-note>`
16. do not update `docs/specs/repository_mapping.md` only because this fork changed the active layer from `stable` to `candidate`; the current unit main Spec path is resolved from `_status.md` plus the `unit_default` truth-surface rule
17. if the round changed any unit `rule_refs` value or any file under `docs/specs/rules/**`, run `rule_sync` only after `_status.md` already reflects `Active Layer=candidate` for this unit, even when no additional affected object is known yet
   - if any touched rule file changed only in `bound_objects` during this round, pass execution-local `bound_objects_only_rule_file_refs` with the exact file refs for those files
   - the deterministic reconciliation part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> rule sync-impact --rule-refs <rule-ref> --units {unit}` or the corresponding `--rule-ids` form, and at least one rule trigger input must already be known before this deterministic execution starts
   - if that `rule_sync` returns control because repository truth is still insufficient to continue safely, stop `unit_fork` as `blocked`, keep the newly created candidate-layer state in place, and reroute through natural-language rule governance from current repository truth instead of claiming Rule side effects are closed

## 5. Stop Conditions

1. the new `candidate` exists with valid `source_basis` and `evidence_appendix_ref`
2. previous-round process files are cleaned up
3. the new candidate contains explicit acceptance items for the current round
4. Rule side effects are closed
5. `_status.md` is updated
6. if a touched Rule file became unbound because of this round's binding change, its terminal state is already resolved or the command has stopped and rerouted through natural-language rule governance
7. if post-fork `rule_sync` could not continue safely, the command result is `blocked`, the candidate-layer state remains the current formal layer, and the required next step is rerunning natural-language routing from current repository truth so it can re-enter rule governance

## 6. Output Contract

1. fork decision
2. created file path
3. initialized candidate version
4. initialized `source_basis`
5. initialized `evidence_appendix_ref` and evidence appendix write result when required
6. candidate acceptance-item structure result
7. written formal global baseline reference or `none`
8. Rule terminal-state result when the round changed rule bindings or rule files
9. cleanup result
10. `_status.md` update result
11. Rule reconciliation result when the round changed rule truth or bindings
12. when post-fork `rule_sync` could not continue safely, that the command stopped as `blocked` and must resume through natural-language rule governance
13. the `user-facing close-out block` required by Section 8.6 of `specflow/framework/command_policy.md`
   - `current state` must explicitly confirm the candidate-layer state written to `_status.md`
   - if post-fork follow-up is blocked on rule governance, the block must name natural-language rule-governance rerouting as the immediate `next step`

## 7. Non-Goals

1. directly modifying `stable`
2. directly generating a plan
3. directly entering implementation
4. creating an independent stable `g_` rule candidate file
5. leaving a touched now-unbound Rule file for later guesswork

## 8. Example

```md
unit_fork:ai
```
