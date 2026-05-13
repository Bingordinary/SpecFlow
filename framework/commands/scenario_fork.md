# Scenario Fork Command

## 1. Purpose

`scenario_fork:{scenario}` opens a new candidate scenario round from the current stable scenario truth.

## 2. Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `scenario_fork`-local entry, output, and stop rules.

## 3. Preconditions

1. `_status.md` says `Object Type=scenario`, `Active Layer=stable`, `Next Command=scenario_fork`
2. stable scenario truth exists
3. read any stable appendix files explicitly referenced by `s_scenario_{scenario}.md`
4. read `specflow/framework/onboarding_decision_policy.md` for stable-fork candidate source handling

## 4. Procedure

1. read stable scenario truth and any explicitly referenced appendix files
2. apply the stable-fork candidate source rule from `specflow/framework/onboarding_decision_policy.md` Section 6.1
   - if the fork uses only stable formal truth plus the current round's selected design changes, prepare `source_basis=new_design` and `evidence_appendix_ref=none`
   - if the fork selects behavior from implementation, tests, runtime behavior, historical material, or other non-stable evidence, prepare the required `source_basis`, `evidence_appendix_ref`, and candidate evidence appendix in the same round
   - if that source decision or evidence appendix is not ready, stop before writing the candidate main Spec
3. create or overwrite `docs/specs/scenarios/candidate/c_scenario_{scenario}.md` and write the prepared `source_basis` and `evidence_appendix_ref` fields in the same candidate write
   - before writing current-round candidate appendices, delete any previous-round `docs/specs/scenarios/candidate/appendix/c_scenario_{scenario}_*.md` files for the same scenario
   - for each same-scenario stable appendix explicitly linked by the stable main Spec and still needed by the candidate truth, write a same-purpose candidate appendix under `docs/specs/scenarios/candidate/appendix/` using the `c_scenario_{scenario}_*.md` prefix
   - each copied candidate appendix must write `scenario: {scenario}` and `layer: candidate` in frontmatter and must not carry `spec_version_ref`
   - retarget the candidate main Spec links and direct same-scenario appendix path literals from `./appendix/s_scenario_{scenario}_*.md` or `docs/specs/scenarios/stable/appendix/s_scenario_{scenario}_*.md` to the copied candidate appendix paths
   - retarget copied appendix markdown links and direct same-scenario main or appendix path literals that point to the stable main Spec or copied same-scenario stable appendices so they point to the candidate main Spec or copied candidate appendices
   - do not leave the candidate main Spec dependent on a stable-layer appendix as current-layer supporting truth
4. ensure the candidate `Testability / Acceptance Criteria` section uses explicit acceptance items that satisfy `spec_writing_guide.md` Section 5
   - if the stable source already has structured acceptance items, carry them forward and edit only the items affected by the new round
   - if the stable source still has historical prose-only acceptance text, convert the relevant scenario acceptance scope into explicit items in the candidate instead of preserving the ambiguity
5. carry forward stable bindings
6. delete outdated candidate-side scenario process files if they exist
7. write `_status.md`:
   - `Candidate=yes`
   - `Active Layer=candidate`
   - `Next Command=scenario_check`
   - outdated candidate-side scenario process files are deleted by command close success cleanup
   - previous-round candidate appendix files are deleted before current-round candidate appendix writeback, and current-round candidate appendix files are preserved by command close success cleanup
   - the deterministic command closure may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> command close --command scenario_fork --object-type scenario --object {scenario} --outcome candidate_created --notes <status-note> --apply`

## 5. Stop Conditions

1. the new `candidate` exists with valid `source_basis` and `evidence_appendix_ref`
2. the new candidate contains explicit acceptance items for the current round
3. any current-layer appendix needed by the candidate truth exists under the candidate appendix directory and is linked from the candidate main Spec by candidate-layer path
4. candidate-side scenario process files are cleaned up
5. `_status.md` is updated

## 6. Output Contract

The output must report:

1. candidate truth file write result
2. initialized `source_basis`
3. initialized `evidence_appendix_ref` and evidence appendix write result when required
4. appendix retarget result, including copied candidate appendix paths and removed previous-round candidate appendix paths
5. candidate acceptance-item structure result
6. candidate-side scenario process file cleanup result
7. lifecycle-state transition result
8. `_status.md` update result
9. the `user-facing close-out block` required by `specflow/framework/command_policy.md` Section 8.6

## 7. Non-Goals

1. stable verification
2. scenario promotion
