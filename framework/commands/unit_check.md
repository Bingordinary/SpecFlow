# Unit Check Command

## 1. Purpose

This command checks whether a unit's `candidate` Spec is sufficiently closed to support stable downstream planning and implementation.

It is a review action, not a "store failed review results" action.

By default, closure means all of the following:

1. `progressability`
   - the unit behavior is clear enough to enter `unit_plan`
   - main flow, key protocols, key boundaries, error semantics, and acceptance criteria are strong enough to prevent planning or implementation divergence
2. `content completeness`
   - the candidate has formally acknowledged key behavior truth that affects implementation results
   - key decisions are not left outside the Spec in chat context, README vision, oral consensus, or author memory
3. `candidate design quality`
   - the candidate connects the user goal, first-round scope, selected direction, and acceptance criteria strongly enough to avoid implementing a merely well-formed but poor project design
4. the candidate is still aligned with the current formal global baseline state

## 2. Scope

By default this command reviews:

1. whether `progressability` holds
2. whether `content completeness` holds
3. whether `Rule Alignment` is explicit and internally consistent
4. whether bound Rule relations and body dependencies are consistent
5. whether shared-candidate signals require routing into rule governance or directly reporting a dual-source-of-truth conflict
6. whether the remaining blocker is actually a user-intent clarification or decision-point that must be written back before closure can pass
7. whether any registered project-local review standard applies on a `unit_check`-owned generic review extension surface and tightens the closure decision for the current candidate
8. whether the candidate records a coherent current design rather than an over-broad, unresolved, or chat-dependent proposal
9. whether the candidate source fields and evidence appendix requirements from `onboarding_decision_policy.md` are satisfied
10. whether `Testability / Acceptance Criteria` contains explicit acceptance items that satisfy `spec_writing_guide.md` Section 5

### 2.1 Command Read Summary

Read this summary before the detailed rules below.
It is navigation only and does not replace the preconditions, procedure, stop conditions, or output contract.

1. `unit_check` exists to decide whether the current candidate is strong enough to become implementation input.
2. The minimum inputs are the current candidate main Spec, required appendix files, bound Rule files, current global baseline when relevant, and registered project-local standards that apply to the command-owned extension surface.
3. A pass writes the current `_check_result/unit/{unit}.md` pass gate and advances the object to `unit_plan`.
4. A non-pass result must not write a failed check-result file; it either reports a blocker, asks for a checkpoint, or requires candidate-side repair before a fresh full-scope `unit_check` rerun.
5. Evidence appendix text proves what was reviewed; it does not become implementation truth.

### 2.2 Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `unit_check`-local entry, output, stop, and fresh-rerun rules.

Process-file writeback and validation for `_check_result/unit/{unit}.md` must follow `specflow/framework/process_snapshot_contract.md` Section 9. When deterministic snapshot validation tooling is available for the current process kind, the matching `snapshot validate-process` command is the mandatory tool-backed validation step before reporting a pass gate or lifecycle advance.

`unit_check` is not a "minimum can-move-forward review."
`unit_check pass` always means:

1. the current candidate may enter `unit_plan`
2. the current candidate already contains the key constraints needed as the truth input for implementation in this round

Result semantics for non-pass conclusions are fixed:

1. `blocked`
   - use when the smallest correct next step cannot be completed by executor-side repair alone in the current round
   - the blocker is waiting on user clarification, user decision, or rule-truth closure outside the active command's direct repair surface
   - if the blocker changes behavior truth, the answer must be written back before `unit_check` may pass
2. `fix_required`
   - use when the executor can already identify a concrete truth-side repair inside the current candidate, appendix, or explicit binding surface
   - no extra user choice is needed before that repair work starts
   - after the repair, the unit must return to `unit_check` rather than skipping forward

Authoritative rerun boundary:

This section is the `unit_check`-local elaboration of the centralized authoritative-run and non-authoritative-follow-up rules inherited above.

1. a new formal `unit_check` rerun may be entered either by explicit command syntax or by a later natural-language request that command routing correctly resolves to a fresh full-scope `unit_check` run for the current unit
   - after a prior `unit_check` ended as `blocked` or `fix_required`, that natural-language request must make rerun intent explicit enough to distinguish "rerun `unit_check` now" from "repair the candidate", "continue follow-up work", or "recheck only the reported blocker"
   - generic repair-oriented wording such as "fix it", "continue", "close this up", or equivalent wording does not by itself authorize a fresh authoritative `unit_check` rerun
2. for `unit_check`, a fresh full-scope run means rerunning the command's full mandatory closure surface for the current unit:
   - reread the current candidate main file plus all required appendix and Rule files
   - reread the current formal global baseline input when it exists
   - rerun the framework-baseline closure checks, including `progressability`, `content completeness`, binding checks, and baseline-alignment checks
   - rerun any applicable registered project-local review surface consumed by `unit_check`
   - re-judge the overall gate conclusion for the current candidate instead of confirming only the previously reported finding
3. truth repair performed after a `blocked` or `fix_required` result is not itself that rerun
4. any repair-side reassessment or scoped follow-up review performed after such repair is non-authoritative:
   - it may report only whether the reported findings appear resolved within the checked scope
   - it must not be labeled a formal `unit_check pass`
   - it must not write `docs/specs/_check_result/unit/{unit}.md`
   - it must not advance `_status.md` to `unit_plan`
   - checking only the repaired truth fragment, only the previously reported blocker, or any other narrowed review slice does not count as a fresh full-scope `unit_check` rerun

Project-local review extension contract:

1. `unit_check` supports project-local `review_standard` entries only on generic review extension surfaces formally defined in this file.
2. `unit_check` currently supports:
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
   - structured findings written by `unit_check`
6. `candidate_closure_review` must not:
   - redefine `unit_check`'s lifecycle position
   - create a new command-level result type
   - bypass `_check_result/unit/{unit}.md` pass-gate rules
7. `unit_check` may allow project-side extension write-back only where this file explicitly says so.
8. The currently allowed `_check_result` project extension write-back container for `candidate_closure_review` is:
   - `project_review_extensions`
9. `project_review_extensions` is a project extension field, not a framework fixed field.
10. When `project_review_extensions` is written, each consumed standard's item must record at least:
   - `standard_id`
   - `applied`
   - `decision`
   - `summary`
11. `project_review_extensions` items may be written only when:
   - `unit_check` is already writing a pass gate for the current round
   - a registered `candidate_closure_review` standard consumed by `unit_check` either applies to the current target or explicitly requires non-hit semantics for pass-gate write-back
12. If no consumed registered standard requires project-side write-back, `unit_check` may omit `project_review_extensions`.
13. If a consumed standard does not apply, `unit_check` may still write that standard's non-hit semantics only inside the same pass gate write-back. It must not create a standalone or failed-state `_check_result/unit/{unit}.md`.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=unit_check`
3. the unit has `candidate`
4. read explicitly referenced candidate appendix files and bound Rule files
5. read `specflow/framework/project_standards_policy.md`
6. if `docs/project_standards/_registry.md` exists, read it and only the registered project-local standard files enabled for a `unit_check`-defined supported generic review extension surface
7. if `docs/project_standards/_registry.md` is missing, stop and report governance drift according to `specflow/framework/project_standards_policy.md`
8. if this round may raise a checkpoint, read `specflow/framework/checkpoint_protocol.md`
9. if referenced appendix files have directory drift, fix that first and rerun the pre-check
10. read `specflow/framework/onboarding_decision_policy.md`

## 4. Procedure

1. read `docs/specs/units/candidate/c_unit_{unit}.md` plus all required appendix and Rule files
2. if `stable` exists, also read `docs/specs/units/stable/s_unit_{unit}.md` plus required stable appendix files
3. read `docs/specs/rules/stable/s_g_rule_repository_baseline.md` if it exists; otherwise continue with the "no formal global baseline yet" state
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
   - `Content Organization Completeness`
     - the candidate main Spec and its appendix files must satisfy `spec_writing_guide.md` Section 6
     - explanatory and normative content must be separated at the subsection level
     - mixed-paragraph violations are an `important` completeness gap by default
8. complete `Candidate Design Quality` review as part of the framework baseline:
   - the candidate must connect the current user or actor goal to the behavior being proposed
   - the candidate must define the first-round scope and non-goals clearly enough that future capabilities are not silently implemented now
   - the candidate must record one current selected direction when multiple solution options were discussed
   - the candidate must define acceptance criteria that can prove the result is useful, not only that artifacts exist
   - the candidate must not depend on chat context, guidance discussion, README vision, or rejected alternatives for implementation-critical meaning
   - over-broad scope, unresolved direction, unverifiable success, or chat-dependent behavior truth can only result in `blocked` or `fix_required`
9. complete the framework-baseline closure checks owned by `unit_check`, including the fixed completeness review objects, `Candidate Design Quality`, baseline, rule, and rule-truth checks below, before finalizing any project-local review merge
10. process formal global baseline alignment and any candidate-carried global rule proposal:
   - if the formal global baseline exists and the candidate is still compatible, a mechanical update to the current version is allowed
   - if incompatible, the result can only be `blocked` or `fix_required`
   - if a global rule proposal is present, it must clearly state the proposed global rule delta, the reason the current baseline is insufficient, the unit-local implementation/verification impact, and the affected units or rules
   - if those fields are unclear, the result can only be `blocked` or `fix_required`
11. for each `unit_check`-owned supported generic review extension surface:
   - resolve matching registered `review_standard` entries from `docs/project_standards/_registry.md`
   - let each registered standard's own applicability contract decide whether it applies to the current target inside that surface
   - execute only the standards whose applicability contract applies to the current target
   - merge the result only as tightening or clarifying input into `progressability`, `content completeness`, and structured findings
   - do not let project-local review bypass framework-baseline closure checks
12. process `rule_refs`:
   - if current behavior depends on Rule truth but bindings are missing or incomplete, the result can only be `blocked` or `fix_required`
   - if bindings exist but the body does not explain which behavior chain reuses them, the result can only be `blocked` or `fix_required`
13. process candidate source fields:
   - `source_basis` must be one of `new_design`, `existing_implementation`, `mixed`, or `replacement`
   - `evidence_appendix_ref` must be present
   - if `source_basis=existing_implementation` or `source_basis=mixed`, `evidence_appendix_ref` must name an existing candidate evidence appendix and that appendix must be read
   - if `source_basis=new_design` or `source_basis=replacement`, `evidence_appendix_ref` must be `none`
   - evidence appendix conflicts or unknowns that still affect selected candidate behavior are critical completeness gaps unless the candidate main Spec explicitly makes a bounded selected rule that no longer depends on them
   - evidence appendix text must not be treated as implementation truth; only the candidate main Spec and bound formal truth may constrain implementation
14. process explicit acceptance items:
   - the candidate must contain a `Testability / Acceptance Criteria` section, or an explicitly equivalent acceptance section title
   - each acceptance item must record `id`, `target`, `verification_surface`, `implementation_surface`, `verification_method`, and `pass_condition`
   - `verification_surface` must use only the fixed values from `spec_writing_guide.md` Section 5
   - vague acceptance text such as "works", "can be replaced", "aligns with design", "supports integration", or equivalent result-only wording is not sufficient unless the required fields make the item directly verifiable
   - for `verification_surface=public_api`, the item must name the public package or exported contract surface, describe an external-consumer style verification method, and require a pass condition that does not import `internal` packages
   - for `verification_surface=integration`, the item must name the runnable integration entrypoint or mark the item as `not_runnable_yet` with a concrete missing-entrypoint reason
   - if an item is marked `not_runnable_yet`, `unit_check` may treat the item as explicitly bounded only when the reason is concrete and the candidate does not use that same item as a current pass claim
   - if a current-gate acceptance item is missing, vague, structurally incomplete, or falsely implied to pass while marked non-runnable, the result can only be `blocked` or `fix_required` with `fallback_reason_code=truth_incomplete`
15. process shared-candidate signals:
   - by default, shared-candidate hints only trigger a suggestion to enter natural-language rule governance
   - if the current required reading range already confirms a dual source of truth, report it directly as a blocking issue with `fallback_reason_code=shared_truth_conflict`
16. determine whether a blocking checkpoint is the correct stop form:
   - use `clarification` when user intent, boundary meaning, or acceptance meaning is still missing from truth
   - use `decision` when multiple materially different directions remain and the user must choose one
17. checkpoint rules:
   - a checkpoint is not `pass`
   - if a checkpoint conclusion changes behavior truth, it must be written back to candidate or appendix before `unit_check` may be rerun
   - do not write `_check_result/unit/{unit}.md` for checkpoint-only stops
18. merge conclusions in this order:
   - `progressability`
   - `content completeness`
   - `Candidate Design Quality`
   - overall gate conclusion
19. merge rules:
   - if `progressability` fails -> only `blocked` or `fix_required`
   - if any `critical` completeness gap exists -> only `blocked` or `fix_required`
   - if `Candidate Design Quality` fails on scope, selected direction, acceptance usefulness, or chat-dependent truth -> only `blocked` or `fix_required`
   - if only `important` or `elaboration` issues remain, `pass` is still possible
20. if the result is `pass`, create or update `docs/specs/_check_result/unit/{unit}.md`
   - when a supported project-local review extension surface was consumed and this file allows project-side extension write-back for that surface, write the corresponding `project_review_extensions` items together with the pass gate
   - write the accepted acceptance-item set into the pass gate summary by item `id`, `verification_surface`, and `not_runnable_yet` state
21. if the result is not `pass`, do not write a failed `_check_result/unit/{unit}.md`; delete an old pass gate if it is no longer valid
22. if the result is `blocked` or `fix_required`, close the current `unit_check` run after writing any required findings:
   - any later truth repair belongs to follow-up work, not to a still-open `unit_check`
   - any later repair-side reassessment or scoped follow-up review remains non-authoritative unless a new fresh full-scope `unit_check` run is entered through command routing
23. update `_status.md`:
   - if pass -> `Next Command=unit_plan`
   - otherwise -> `Next Command=unit_check`

## 5. Stop Conditions

1. whether the candidate satisfies both `progressability` and `content completeness` is clear
2. whether `Candidate Design Quality` passes or blocks the candidate is clear
3. if the round passes, `_check_result/unit/{unit}.md` holds the pass gate
4. if the round does not pass, no invalid old pass gate remains
5. `_status.md` is updated
6. if a supported project-local review extension surface was consumed and the round passes, its allowed project extension write-back is clear
7. the explicit acceptance-item set is either accepted as specific enough for downstream planning or reported as a blocking truth gap
8. no repair-side reassessment or scoped follow-up review has been mistaken for a formal `unit_check pass`

## 6. Output Contract

The output should include:

1. overall conclusion
2. severity summary
3. formal global baseline alignment result
4. candidate source and evidence appendix result
5. acceptance-item completeness result, including any vague, missing, or non-runnable item findings
6. the gate conclusion set:
   - `progressability`
   - `content completeness`
   - `Candidate Design Quality`
   - overall gate conclusion
7. whether `Check Result Snapshot` was written back or an old gate was cleaned up
8. `checkpoint result` when a checkpoint stop was raised
   - when present, it must satisfy the fixed checkpoint fields defined by `specflow/framework/checkpoint_protocol.md`
9. `fallback_reason_code` for blocked, fix-required, or checkpoint stops
10. structured findings when `blocked` or `fix_required`
11. next-step suggestion
12. `_status.md` update result
13. when a project-local review extension surface was consumed:
   - which `surface` matched
   - which registered project-local standard file was used
   - how that surface affected `progressability`, `content completeness`, or structured findings
14. when follow-up work only confirmed local repair or ran a scoped review instead of a new formal rerun, that this result was non-authoritative and did not change lifecycle state
15. the `user-facing close-out block` required by Section 8.6 of `specflow/framework/command_policy.md`
   - when a checkpoint was raised, also report `resume signal`
   - if `Next Command=unit_check`, `why this next step` must explicitly state whether the blocker is truth repair, user clarification, or a required decision rather than only repeating that closure is incomplete

When the result is `blocked` or `fix_required`, findings must be structured and must not be reduced to vague summaries.

Severity must use the shared meanings defined in:

1. `specflow/framework/severity_policy.md`

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
3. `rule_drift`
4. `shared_truth_conflict`
5. `governance_drift`

Candidate source-field and evidence-appendix blockers must use `truth_incomplete`.
The output must name the exact source or evidence condition in the natural-language explanation after the standardized code.
Do not introduce source-basis-specific or evidence-appendix-specific `fallback_reason_code` values in this command.

## 7. Non-Goals

1. directly generating a plan
2. directly entering code implementation
3. creating, updating, or deleting an independent stable `g_` rule candidate file

## 8. Examples

```md
unit_check:ai
```
