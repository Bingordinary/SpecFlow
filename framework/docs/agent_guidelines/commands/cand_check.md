# Candidate Check Command

## 1. Purpose

This command checks whether a module's `candidate` Spec is sufficiently closed to support stable downstream planning and implementation.

It is a review action, not a "store failed review results" action.

By default, closure means all of the following:

1. `progressability`
   - the module behavior is clear enough to enter `cand_plan`
   - main flow, key protocols, key boundaries, error semantics, and acceptance criteria are strong enough to prevent planning or implementation divergence
2. `content completeness`
   - the candidate has formally acknowledged key behavior truth that affects implementation results
   - key decisions are not left outside the Spec in chat context, README vision, oral consensus, or author memory
3. the candidate is still aligned with the current formal global baseline state

## 2. Scope

By default this command reviews:

1. whether `progressability` holds
2. whether `content completeness` holds
3. whether `Global Constraint Alignment` is explicit and internally consistent
4. whether bound Shared Contract relations and body dependencies are consistent
5. whether `system_constraints_stable_ref` matches the current formal global baseline state
6. whether `system_constraints_change_proposal`, if present, is explicit enough to be implemented and verified in the current round
7. whether shared-candidate signals require routing into `shared_ops` or directly reporting a dual-source-of-truth conflict
8. whether the remaining blocker is actually a user-intent clarification or decision-point that must be written back before closure can pass
9. whether any registered project-local review standard applies on a `cand_check`-owned generic review extension surface and tightens the closure decision for the current candidate

### 2.1 Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the centralized authoritative-run and non-authoritative-follow-up rules from `specflow/framework/docs/agent_guidelines/command_policy.md` Section 8 Rules 27-30.
Only a new independent full-scope run of `cand_check` may produce that advancing result; later local confirmation, repair-side reassessment, or scoped follow-up review must not advance lifecycle state.

`cand_check` is not a "minimum can-move-forward review."
`cand_check pass` always means:

1. the current candidate may enter `cand_plan`
2. the current candidate already contains the key constraints needed as the truth input for implementation in this round

Result semantics for non-pass conclusions are fixed:

1. `blocked`
   - use when the smallest correct next step cannot be completed by executor-side repair alone in the current round
   - the blocker is waiting on user clarification, user decision, or shared-truth closure outside the active command's direct repair surface
   - if the blocker changes behavior truth, the answer must be written back before `cand_check` may pass
2. `fix_required`
   - use when the executor can already identify a concrete truth-side repair inside the current candidate, appendix, or explicit binding surface
   - no extra user choice is needed before that repair work starts
   - after the repair, the module must return to `cand_check` rather than skipping forward

Authoritative rerun boundary:

This section is the `cand_check`-local elaboration of the centralized authoritative-run and non-authoritative-follow-up rules inherited above.

1. a new formal `cand_check` rerun may be entered either by explicit command syntax or by a later natural-language request that command routing correctly resolves to a fresh full-scope `cand_check` run for the current module
   - after a prior `cand_check` ended as `blocked` or `fix_required`, that natural-language request must make rerun intent explicit enough to distinguish "rerun `cand_check` now" from "repair the candidate", "continue follow-up work", or "recheck only the reported blocker"
   - generic repair-oriented wording such as "fix it", "continue", "close this up", or equivalent wording does not by itself authorize a fresh authoritative `cand_check` rerun
2. for `cand_check`, a fresh full-scope run means rerunning the command's full mandatory closure surface for the current module:
   - reread the current candidate main file plus all required appendix and Shared Contract files
   - reread the current formal global baseline input when it exists
   - rerun the framework-baseline closure checks, including `progressability`, `content completeness`, binding checks, and baseline-alignment checks
   - rerun any applicable registered project-local review surface consumed by `cand_check`
   - re-judge the overall gate conclusion for the current candidate instead of confirming only the previously reported finding
3. truth repair performed after a `blocked` or `fix_required` result is not itself that rerun
4. any repair-side reassessment or scoped follow-up review performed after such repair is non-authoritative:
   - it may report only whether the reported findings appear resolved within the checked scope
   - it must not be labeled a formal `cand_check pass`
   - it must not write `docs/specs/_check_result/{module}.md`
   - it must not advance `_status.md` to `cand_plan`
   - checking only the repaired truth fragment, only the previously reported blocker, or any other narrowed review slice does not count as a fresh full-scope `cand_check` rerun

Project-local review extension contract:

1. `cand_check` supports project-local `review_standard` entries only on generic review extension surfaces formally defined in this file.
2. `cand_check` currently supports:
   - `candidate_closure_review`
3. `candidate_closure_review` means:
   - a command-owned generic extension surface used after framework-baseline closure checks for project-local review standards that may tighten closure judgment for the current candidate
4. A registered standard consumed on `candidate_closure_review` must define in its own file:
   - the concrete project review focus it owns
   - the applicability contract that decides when that standard applies to the current target inside this generic surface
   - the blocking and non-blocking rules it adds for that focus
   - the summary semantics of any allowed project-side write-back it requires
5. `candidate_closure_review` may tighten only:
   - `progressability`
   - `content completeness`
   - structured findings written by `cand_check`
6. `candidate_closure_review` must not:
   - redefine `cand_check`'s lifecycle position
   - create a new command-level result type
   - bypass `_check_result/{module}.md` pass-gate rules
7. `cand_check` may allow project-side extension write-back only where this file explicitly says so.
8. The currently allowed `_check_result` project extension write-back container for `candidate_closure_review` is:
   - `project_review_extensions`
9. `project_review_extensions` is a project extension field, not a framework fixed field.
10. When `project_review_extensions` is written, each consumed standard's item must record at least:
   - `standard_id`
   - `applied`
   - `decision`
   - `summary`
11. `project_review_extensions` items may be written only when:
   - `cand_check` is already writing a pass gate for the current round
   - a registered `candidate_closure_review` standard consumed by `cand_check` either applies to the current target or explicitly requires non-hit semantics for pass-gate write-back
12. If no consumed registered standard requires project-side write-back, `cand_check` may omit `project_review_extensions`.
13. If a consumed standard does not apply, `cand_check` may still write that standard's non-hit semantics only inside the same pass gate write-back. It must not create a standalone or failed-state `_check_result/{module}.md`.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=cand_check`
3. the module has `candidate`
4. read explicitly referenced candidate appendix files and bound Shared Contract files
5. read `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
6. if `docs/project_standards/_registry.md` exists, read it and only the registered project-local standard files enabled for a `cand_check`-defined supported generic review extension surface
7. if `docs/project_standards/_registry.md` is missing, stop and report governance drift according to `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
8. if this round may raise a checkpoint, read `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
9. if `_check_result/{module}.md`, `_status.md`, candidate truth, or other commit-triggering governance files may change, read the git policy first
10. if referenced appendix files have directory drift, fix that first and rerun the pre-check

## 4. Procedure

1. read `docs/specs/modules/candidate/c_{module}.md` plus all required appendix and Shared Contract files
2. if `stable` exists, also read `docs/specs/modules/stable/s_{module}.md` plus required stable appendix files
3. read `docs/specs/system/stable/s_system_constraints.md` if it exists; otherwise continue with the "no formal global baseline yet" state
4. judge `progressability`
5. judge `content completeness`
6. classify completeness gaps into:
   - `critical`
   - `important`
   - `elaboration`
7. use these fixed completeness review objects:
   - `Behavior Basis Completeness`
   - `Decision Surface Completeness`
   - `Acceptance Basis Completeness`
8. complete the framework-baseline closure checks owned by `cand_check`, including the fixed completeness review objects plus the baseline, shared-contract, and shared-truth checks below, before finalizing any project-local review merge
9. for each `cand_check`-owned supported generic review extension surface:
   - resolve matching registered `review_standard` entries from `docs/project_standards/_registry.md`
   - let each registered standard's own applicability contract decide whether it applies to the current target inside that surface
   - execute only the standards whose applicability contract applies to the current target
   - merge the result only as tightening or clarifying input into `progressability`, `content completeness`, and structured findings
   - do not let project-local review bypass framework-baseline closure checks
10. process `system_constraints_stable_ref`:
   - if the formal global baseline exists and the candidate is still compatible, a mechanical update to the current version is allowed
   - if incompatible, the result can only be `blocked` or `fix_required`
   - if the formal global baseline does not exist, `system_constraints_stable_ref` must be `none`
11. process `system_constraints_change_proposal`:
   - if present, it must clearly state the proposed global rule delta, the reason the current baseline is insufficient, the module-local implementation/verification impact, and the affected modules or shared contracts
   - if those fields are unclear, the result can only be `blocked` or `fix_required`
12. process `shared_contract_refs`:
   - if current behavior depends on Shared Contract truth but bindings are missing or incomplete, the result can only be `blocked` or `fix_required`
   - if bindings exist but the body does not explain which behavior chain reuses them, the result can only be `blocked` or `fix_required`
13. process shared-candidate signals:
   - by default, shared-candidate hints only trigger a suggestion to enter `shared_ops:{natural-language request}`
   - if the current required reading range already confirms a dual source of truth, report it directly as a blocking issue with `fallback_reason_code=shared_truth_conflict`
14. determine whether a blocking checkpoint is the correct stop form:
   - use `clarification` when user intent, boundary meaning, or acceptance meaning is still missing from truth
   - use `decision` when multiple materially different directions remain and the user must choose one
15. checkpoint rules:
   - a checkpoint is not `pass`
   - if a checkpoint conclusion changes behavior truth, it must be written back to candidate or appendix before `cand_check` may be rerun
   - do not write `_check_result/{module}.md` for checkpoint-only stops
16. merge conclusions in this order:
   - `progressability`
   - `content completeness`
   - overall gate conclusion
17. merge rules:
   - if `progressability` fails -> only `blocked` or `fix_required`
   - if any `critical` completeness gap exists -> only `blocked` or `fix_required`
   - if only `important` or `elaboration` issues remain, `pass` is still possible
18. if the result is `pass`, create or update `docs/specs/_check_result/{module}.md`
   - when a supported project-local review extension surface was consumed and this file allows project-side extension write-back for that surface, write the corresponding `project_review_extensions` items together with the pass gate
19. if the result is not `pass`, do not write a failed `_check_result/{module}.md`; delete an old pass gate if it is no longer valid
20. if the result is `blocked` or `fix_required`, close the current `cand_check` run after writing any required findings:
   - any later truth repair belongs to follow-up work, not to a still-open `cand_check`
   - any later repair-side reassessment or scoped follow-up review remains non-authoritative unless a new fresh full-scope `cand_check` run is entered through command routing
21. update `_status.md`:
   - if pass -> `Next Command=cand_plan`
   - otherwise -> `Next Command=cand_check`
22. perform git close-out if required

## 5. Stop Conditions

1. whether the candidate satisfies both `progressability` and `content completeness` is clear
2. if the round passes, `_check_result/{module}.md` holds the pass gate
3. if the round does not pass, no invalid old pass gate remains
4. `_status.md` is updated
5. if a supported project-local review extension surface was consumed and the round passes, its allowed project extension write-back is clear
6. no repair-side reassessment or scoped follow-up review has been mistaken for a formal `cand_check pass`

## 6. Output Contract

The output should include:

1. overall conclusion
2. severity summary
3. formal global baseline alignment result
4. the two-threshold conclusion:
   - `progressability`
   - `content completeness`
   - overall gate conclusion
5. whether `Check Result Snapshot` was written back or an old gate was cleaned up
6. `checkpoint result` when a checkpoint stop was raised
   - when present, it must satisfy the fixed checkpoint fields defined by `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
7. `fallback_reason_code` for blocked, fix-required, or checkpoint stops
8. structured findings when `blocked` or `fix_required`
9. next-step suggestion
10. git close-out result
11. `_status.md` update result
12. when a project-local review extension surface was consumed:
   - which `surface` matched
   - which registered project-local standard file was used
   - how that surface affected `progressability`, `content completeness`, or structured findings
13. when follow-up work only confirmed local repair or ran a scoped review instead of a new formal rerun, that this result was non-authoritative and did not change lifecycle state
14. `user-facing close-out block` required by `specflow/framework/docs/agent_guidelines/command_policy.md`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - when a checkpoint was raised, also report `resume signal`
   - if `Next Command=cand_check`, `why this next step` must explicitly state whether the blocker is truth repair, user clarification, or a required decision rather than only repeating that closure is incomplete

When the result is `blocked` or `fix_required`, findings must be structured and must not be reduced to vague summaries.

Severity must use the shared meanings defined in:

1. `specflow/framework/docs/agent_guidelines/severity_policy.md`

Each finding must explain:

1. background
2. what happened
3. impact
4. best recommendation
5. why that recommendation is best
6. whether it is blocking
7. which constraint layer it belongs to

Allowed checkpoint types:

1. `clarification`
2. `decision`

Allowed `fallback_reason_code` values:

1. `truth_incomplete`
2. `baseline_drift`
3. `shared_contract_drift`
4. `shared_truth_conflict`
5. `governance_drift`

## 7. Non-Goals

1. directly generating a plan
2. directly entering code implementation
3. creating, updating, or deleting an independent `system_constraints` candidate file

## 8. Examples

```md
cand_check:module_ai
```
