# Rule Escape Flow

## 1. Purpose

`rule_escape` is the internal flow for stopping unsafe rule-governance requests and decomposing them only when a stable next action exists.

It answers four questions:

1. whether a rule-governance request can be routed into exactly one standard rule flow
2. whether the request can be decomposed into a safe sequence of standard rule flows
3. when a checkpoint is mandatory instead of automatic continuation
4. how to hand work back to the smallest legal next step without guessing

This is not a user-facing command entry.
The user reaches it through natural-language routing when that routing enters the rule-governance branch.

---

## 2. Scope

By default it handles rule-governance requests that cannot be stably routed into exactly one of:

1. `rule_new`
2. `rule_extract`
3. `rule_bind`
4. `rule_topology`
5. `rule_sync`

It also handles cases where one of those already-routed internal rule flows later discovers that current repository truth is insufficient to continue safely.

It may:

1. decompose one complex request into a safe sequence of standard rule flows
2. raise a `clarification` checkpoint
3. raise a `decision` checkpoint
4. raise a `prerequisite_action` checkpoint when one required upstream command must happen first
5. emit a formal `remaining_steps_contract` when safe decomposition exists

It does not:

1. act as a catch-all executor freedom zone
2. replace the downstream standard rule flows
3. create an independent system command chain
4. keep truth in chat without required writeback
5. turn `remaining_steps_contract` into durable truth that survives a later resume without fresh routing

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/spec_policy.md`
2. read `specflow/framework/command_policy.md`
3. read `specflow/framework/checkpoint_protocol.md`
4. read `docs/specs/_status.md` when the request names existing formal units or scenarios
5. resolve every named existing unit or scenario's current layer from `_status.md` before reading its main Spec
6. read any current-layer unit files, scenario files, appendix files, and `rule` files needed to judge the true boundary
7. read `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the request may cross into project-wide default-rule promotion

---

## 4. Procedure

1. identify the smallest distinct action units inside the current request
2. if control was returned by an already-routed internal rule flow, identify the unresolved remainder from that flow using current repository truth instead of assuming the earlier route is still sufficient
2.5. if control was returned by an already-routed internal rule flow after file mutation (i.e., rule_sync returned control because repository truth is insufficient), check whether a recovery baseline exists in execution context:
     - if a recovery baseline exists and files were mutated, execute `specflow/framework/recovery_policy.md` Section 6.5.3 (Rule-Governance Recovery Procedure) before proceeding to decomposition
     - if no recovery baseline exists but files were mutated, raise a `prerequisite_action` checkpoint listing each file believed to have been mutated
3. test whether the request can be routed into exactly one standard rule flow without ambiguity
4. if yes, stop and route back to that one standard flow instead of continuing inside `rule_escape`
5. if more than one rule flow is involved, test whether a sequence exists whose order does not change formal truth
6. if such a stable sequence exists, build a formal `remaining_steps_contract` that records:
   - the full ordered step list
   - the current step
   - the remaining steps after the current step
   - the closure condition that rule governance stays open until the final listed step finishes
   - that the contract is execution-local and must be discarded if the current rule-governance handling stops before final closure
7. if such a stable sequence exists, report that contract and route to the first legal flow only
8. stop immediately and raise a checkpoint when any of the following holds:
   - the same truth has two or more plausible formal landing points
   - the boundary between unit-local truth and rule truth is unstable
   - the action order would change resulting formal truth
   - current repository truth is insufficient to support a stable decomposition
10. if the current rule-governance handling stops before all listed steps finish, require rerunning natural-language routing from current repository truth rather than resuming an old `remaining_steps_contract`

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the request has been reduced to exactly one legal next rule flow and any required `remaining_steps_contract` has been emitted
2. a checkpoint has been raised because automatic continuation would be unsafe
3. the request has crossed out of rule-only governance and must return to command-target candidate truth writeback before resume

---

## 6. Output Contract

The output must include at least:

1. the complex intent recognized from the request
2. why single-flow routing was unstable
3. whether a safe decomposition exists
4. when control was returned from an already-routed internal rule flow, which flow returned control and why its continuation was no longer stable
5. when a safe decomposition exists, the formal `remaining_steps_contract`, including:
   - `step_order`
   - `current_step`
   - `remaining_steps`
   - `shared_governance_closure_rule`
   - `durability=execution_local`
   - `resume_rule=rerun_natural_language_routing_from_current_truth_if_interrupted`
6. the smallest legal next rule flow if decomposition is stable
7. if a checkpoint is raised:
   - `type`
   - `blocking`
   - `command=rule_escape`
   - `entry=natural_language_routing`
   - `branch=shared_governance`
   - `routed_flow=rule_escape`
   - `unit` or `none`; use `none` when the checkpoint is scenario-only or cross-object, and name the scenario in `required_writeback_target`
   - `question_or_action`
   - `why_blocking`
   - `required_writeback_target`
   - `resume_signal`
   - `resume_next_step`

---

## 7. Non-Goals

`rule_escape` does not:

1. guess through unstable boundaries
2. continue automatically when action order changes formal truth
3. keep system-boundary conclusions only in chat
4. replace the actual downstream rule flow that must perform the work
5. treat a first-step route as the full closure of a multi-step rule request
