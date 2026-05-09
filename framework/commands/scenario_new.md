# Scenario New Command

## 1. Purpose

`scenario_new:{scenario}` creates the first candidate truth for a brand-new formal scenario object.

## 2. Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `scenario_new`-local entry, output, and stop rules.

## 3. Preconditions

1. the scenario ID is clear and non-conflicting
2. no current row for that scenario exists in `_status.md`
3. read `specflow/framework/repository_mapping_policy.md`
4. read `docs/specs/repository_mapping.md`
5. confirm the target scenario is not already present in the Governed Object Map and does not conflict with any current `unit`, `scenario`, `rule`, support-surface, or ignore rule
6. read `specflow/framework/onboarding_decision_policy.md` and decide the first candidate's `source_basis` and `evidence_appendix_ref`
7. if the first candidate uses `source_basis=existing_implementation` or `source_basis=mixed`, prepare the required scenario evidence appendix in the same round

## 4. Procedure

1. prepare the `docs/specs/repository_mapping.md` writeback for the new scenario before candidate or `_status.md` mutation:
   - add the target scenario ID and one-line responsibility to the Governed Object Map
   - replace `none` in the current scenario list when this is the first formal scenario
   - record the scenario truth-surface rule for the fixed scenario truth path template
   - record any support surface, governed root, ignore rule, or conflict rule that this first scenario round already needs
   - if current repository truth is insufficient to write the exact mapping update without guessing, stop before candidate and `_status.md` writeback
2. create `docs/specs/scenarios/candidate/c_scenario_{scenario}.md`
3. initialize:
   - `source_basis`
   - `evidence_appendix_ref`
   - `repository_mapping_ref` to the post-writeback `docs/specs/repository_mapping.md` version
   - `unit_refs`
   - `rule_refs`
4. ensure the candidate scenario contains `Testability / Acceptance Criteria` with explicit acceptance items that satisfy `spec_writing_guide.md` Section 5
5. write the prepared `docs/specs/repository_mapping.md` update in the same round as the candidate writeback
6. write or upsert `_status.md` row:
   - `Object Type=scenario`
   - `Object={scenario}`
   - `Stable=no`
   - `Candidate=yes`
   - `Active Layer=candidate`
   - `Next Command=scenario_check`

## 5. Stop Conditions

1. the first scenario `candidate` exists
2. `docs/specs/repository_mapping.md` includes the new scenario in the Governed Object Map and contains the truth-surface or path-ownership entries required by this first scenario round
3. `_status.md` registration is complete
4. if repository truth was insufficient to write the required repository mapping update safely, the command stopped before candidate and `_status.md` writeback instead of guessing

## 6. Output Contract

The output must report:

1. candidate truth file write result
2. initialized `source_basis`
3. initialized `evidence_appendix_ref` and evidence appendix write result when required
4. initialized post-writeback `repository_mapping_ref`
5. initialized acceptance-item structure result
6. `docs/specs/repository_mapping.md` writeback result, including the new Governed Object Map entry and any truth-surface or path-ownership entries written in this round
7. `_status.md` registration result
8. lifecycle-state transition result
9. the `user-facing close-out block` required by `specflow/framework/command_policy.md` Section 8.6

## 7. Non-Goals

1. creating stable scenario truth
2. editing unit code
