# Unit New Command

## 1. Purpose

This command creates the first `candidate` Spec for a brand-new unit.

Goals:

1. define the first complete candidate design
2. establish the starting point of the candidate chain
3. register the unit in `docs/specs/repository_mapping.md`
4. register the unit state in `docs/specs/_status.md`

## 2. Scope

By default this command handles:

1. first-time project initiation for a new unit
2. units that do not yet have any formally effective version
3. creation of the first `candidate`
5. registration of the new unit in `docs/specs/repository_mapping.md`

It does not:

1. invent a shared/unit boundary when the first candidate still depends on rule truth that is not yet formalized
2. write `rule_refs=none` as a placeholder when the new unit already depends on rule truth

### 2.1 Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `unit_new`-local entry, output, and stop rules.

## 3. Preconditions

1. complete the required pre-checks
2. the target unit name is explicit
3. the unit is not yet in `_status.md`
4. the goal is future design first, not capturing current truth first
5. read `specflow/framework/repository_mapping_policy.md`
6. read `docs/specs/repository_mapping.md`
7. confirm the target unit is not already present in the Governed Object Map and does not conflict with any current `unit`, `scenario`, `rule`, support-surface, or ignore rule
8. read `specflow/framework/onboarding_decision_policy.md` and decide the first candidate's `source_basis` and `evidence_appendix_ref`
9. if the first candidate uses `source_basis=existing_implementation` or `source_basis=mixed`, prepare the required evidence appendix in the same round
10. if the first candidate depends on rule truth that is not yet formalized as `rule`, or if the shared/unit boundary is still unstable, do not start `unit_new`; resolve that rule truth through natural-language rule governance first
11. if the first candidate reuses already-existing rule truth, read the relevant `rule` files before writing `rule_refs`
12. if the round will create, update, or delete any unit `rule_refs` value or any file under `docs/specs/rules/**`, read `rule_sync.md`
13. if the round may update `bound_objects` or remove intentional-unbound retention fields from a touched Rule file, read every current-layer unit or scenario main file needed to derive the real repository-wide binding set of each touched Rule from `rule_refs`

## 4. Procedure

1. if `s_g_rule_repository_baseline.md` exists, read it as the current formal global baseline; otherwise continue with the "no formal global baseline yet" state
2. decide whether the first candidate already reuses existing rule truth:
   - if no, the round may initialize `rule_refs=none`
   - if yes, the round must bind that rule truth explicitly in the first candidate instead of using `none`
3. define the new unit's goals, boundaries, protocols, and main flow
4. prepare the `docs/specs/repository_mapping.md` writeback for the new unit before candidate or `_status.md` mutation:
   - add the target unit ID and one-line responsibility to the Governed Object Map
   - record the unit truth-surface rule for the fixed unit truth path template
   - record any implementation surface, support surface, governed root, ignore rule, or conflict rule that this first unit round already needs
   - if current repository truth is insufficient to write the exact mapping update without guessing, stop before candidate and `_status.md` writeback
5. create `docs/specs/units/candidate/c_unit_{unit}.md`
6. initialize `frontmatter.version` to `0.1.0`
7. initialize `frontmatter.source_basis` and `frontmatter.evidence_appendix_ref` according to `onboarding_decision_policy.md`
8. if `source_basis=existing_implementation` or `source_basis=mixed`, create the evidence appendix named by `evidence_appendix_ref`; if `source_basis=new_design` or `source_basis=replacement`, write `evidence_appendix_ref=none`
9. ensure the file covers the core sections of a formal Spec, including `Testability / Acceptance Criteria` with explicit acceptance items that satisfy `spec_writing_guide.md` Section 5
10. initialize `Rule Alignment`:
   - write `rule_refs=none` only when the first candidate does not yet reuse rule truth
   - if the first candidate already reuses existing rule truth, write the explicit `rule_refs` set using the Rule binding contract from `specflow/framework/spec_policy.md` Section 6.1 and explain that reuse in the candidate body in the same round
   - `rule_reuse_summary`
   - `rule_exceptions`
11. write the prepared `docs/specs/repository_mapping.md` update in the same round as the candidate writeback
12. if the round changed Rule bindings or touched Rule files:
   - derive the real repository-wide binding set of each touched Rule from current-layer unit and scenario `rule_refs` plus this round's prepared target-unit candidate writeback
   - if current repository truth is insufficient to derive that touched real binding set safely, stop and reroute through natural-language rule governance from current repository truth instead of guessing
   - update `bound_objects` only as declarative metadata so each touched Rule file matches the real binding set implied by that repository-wide binding view plus this round's prepared target-unit writeback
   - the deterministic metadata writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> rule reconcile-bound-objects --units {unit}` and additional `--rule-refs` / `--rule-ids` filters when the active flow has already identified them
   - if a touched Rule file now has one or more formal bound units after this round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
13. update `_status.md`:
   - `Stable=no`
   - `Candidate=yes`
   - `Active Layer=candidate`
   - `Next Command=unit_check`
   - the deterministic row writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> status set-object --type unit --object {unit} --stable no --candidate yes --active-layer candidate --next-command unit_check --notes <status-note> --create`
14. if the round changed any unit `rule_refs` value or any file under `docs/specs/rules/**`, run `rule_sync` after `_status.md` has been updated, even when no additional affected object is known yet
   - if any touched rule file changed only in `bound_objects` during this round, pass execution-local `bound_objects_only_rule_file_refs` with the exact file refs for those files
   - the deterministic reconciliation part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> rule sync-impact --rule-refs <rule-ref> --units {unit}` or the corresponding `--rule-ids` form, and at least one rule trigger input must already be known before this deterministic execution starts

## 5. Stop Conditions

1. the first `candidate` exists
2. `docs/specs/repository_mapping.md` includes the new unit in the Governed Object Map and contains the truth-surface or path-ownership entries required by this first unit round
3. `_status.md` registration is complete
4. any first-round rule binding required by the candidate has been written explicitly instead of being left as placeholder `none`
5. Rule side effects, if any, are closed
6. the command does not automatically continue into implementation
7. if repository truth was insufficient to write the required repository mapping update safely, the command stopped before candidate and `_status.md` writeback instead of guessing
8. if repository truth was insufficient to close rule-truth binding metadata safely, the command stopped and rerouted through natural-language rule governance instead of guessing

## 6. Output Contract

1. initiation judgment
2. created file path
3. initialized candidate version
4. initialized `source_basis`
5. initialized `evidence_appendix_ref` and evidence appendix write result when required
6. initialized formal global baseline reference or `none`
7. initialized acceptance-item structure result
8. initialized explicit Rule binding set or confirmed `rule_refs=none`
9. whether the command had to stop and reroute through natural-language rule governance because repository truth was insufficient to close rule-truth binding metadata safely
10. `docs/specs/repository_mapping.md` writeback result, including the new Governed Object Map entry and any truth-surface or path-ownership entries written in this round
11. `_status.md` update result
12. Rule reconciliation result when the round changed rule truth or bindings
13. remaining closure items
14. the `user-facing close-out block` required by Section 8.6 of `specflow/framework/command_policy.md`
   - `current state` must explicitly confirm `Active Layer=candidate` and `Next Command=unit_check`
   - `next-stage entry gap` must explicitly confirm that entry into the later different command `unit_check` is already satisfied after `unit_new` closes

## 7. Non-Goals

1. creating the first formal `stable`
2. capturing historical behavior
3. automatically entering `unit_impl`
4. creating an independent stable `g_` rule candidate file
5. using `rule_refs=none` to postpone required rule-truth closure

## 8. Example

```md
unit_new:executor
```
