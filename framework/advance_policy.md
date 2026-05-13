# Advance Policy

## 1. Purpose

Advance is an exact `specFlow` entry for automatic progression through existing lifecycle commands.

It exists for users who want the executor to keep moving one current `unit` or `scenario` until the work reaches stable truth or reaches a real human-in-the-loop stop.

Advance does not create a new lifecycle command.
It coordinates existing commands by repeatedly reading current repository state, entering the one current legal command, and re-reading state after that command closes.

---

## 2. Entry Forms

Advance supports exactly these user-facing forms:

```text
unit_advance:{unit}
scenario_advance:{scenario}
```

Rules:

1. `unit_advance:{unit}` targets one existing `unit` row in `docs/specs/_status.md`.
2. `scenario_advance:{scenario}` targets one existing `scenario` row in `docs/specs/_status.md`.
3. Exact advance entries route to this file directly.
4. Natural-language routing may route a request here only when the user clearly asks the executor to keep advancing automatically until completion or blocker.
5. Generic wording such as "continue payment" remains a normal natural-language single-step request unless the user asks for automatic progression.

---

## 3. Ownership And Write Boundary

Advance owns only loop control.

Advance may:

1. read `docs/specs/_status.md`
2. choose the current standard command from the target row's `Next Command`
3. enter that command through `specflow/framework/command_policy.md` and the matching command file
4. re-read `docs/specs/_status.md` after each command closes
5. stop with a user-facing report when continuation is no longer legally clear

Advance must not:

1. create `specflow/framework/commands/unit_advance.md`
2. create `specflow/framework/commands/scenario_advance.md`
3. write `_status.md` directly
4. write `_check_result`, `_plans`, `_verify_result`, or stable acceptance summaries directly
5. write, delete, or promote Spec truth directly
6. edit implementation files outside the currently routed `unit_impl` command
7. replace command-local preflight, checkpoint, fallback, recovery, verification, or promotion rules
8. treat chat-only decisions as durable truth

When the current step is `unit_impl`, implementation edits happen only inside the routed `unit_impl` command.
When the current step is a verify or promote command, verification and promotion judgments remain owned by that command.

---

## 4. Required Reads

Before the first command iteration, read:

1. `specflow/framework/command_policy.md`
2. `docs/specs/_status.md`
3. the current command file named by the target row's `Next Command`, when that command is advance-runnable

Before each later command iteration:

1. re-read `docs/specs/_status.md`
2. read the new current command file if the target row's `Next Command` changed
3. read only the additional policy or truth files required by the current routed command

For `scenario_advance` recursion into affected units:

1. use only affected unit identities reported by the current `scenario_verify` result or close-out
2. confirm every affected unit's current row in `docs/specs/_status.md`
3. enter this policy recursively through `unit_advance:{unit}` for each affected unit

Advance must not read every command file by default.

---

## 5. Execution State

Advance uses an execution-local `advance_run` record.
This record is not durable truth and must not be written as a process file.

The execution-local record must track:

1. entry form
2. target object
3. command history
4. status transitions observed after each command
5. recursive affected-unit advances, when any
6. stop reason
7. whether the final state is stable completion or blocked continuation

Loop guard:

1. if the same target reaches the same `Next Command` twice without a command-reported progress reason, stop instead of repeating
2. `unit_impl` may repeat with the same `Next Command` only when the previous `unit_impl` run records concrete slice progress and no user, truth, plan, external-condition, or verification blocker
3. a recursive `scenario_advance -> unit_advance -> scenario_advance` cycle is forbidden
4. if a scenario's affected-unit chain points back to the same scenario before the unit work closes, stop and report the cycle

---

## 6. Unit Advance

`unit_advance:{unit}` may automatically enter only these unit commands:

1. `unit_check:{unit}`
2. `unit_plan:{unit}`
3. `unit_impl:{unit}`
4. `unit_verify:{unit}`
5. `unit_promote:{unit}`

Terminal completion:

1. when the unit row records `Active Layer=stable`, `Candidate=no`, and `Next Command=unit_fork`, advance is complete
2. advance must stop at that state and must not open the next candidate round

Continuation rules:

1. after `unit_check` advances to `unit_plan`, continue to `unit_plan`
2. after `unit_plan` advances to `unit_impl`, continue to `unit_impl`
3. after `unit_impl` advances to `unit_verify`, continue to `unit_verify`
4. after `unit_impl` remains at `unit_impl`, continue only when the command output records concrete implementation progress and no blocking condition
5. after `unit_verify` advances to `unit_promote`, continue to `unit_promote`
6. after `unit_verify` falls back to `unit_impl` for implementation deviation, continue to `unit_impl` only when no checkpoint, truth writeback, or human verification is required
7. after `unit_promote` falls back to an advance-runnable earlier unit command, continue only when the promote command completed its defined cleanup and did not report a human, rule-governance, repository-mapping, or truth-writeback blocker

Stop rules:

1. stop when the unit row is missing or is not a unit row
2. stop when `Next Command` is `unit_init`, `unit_new`, `unit_fork`, or `unit_stable_verify`
3. stop when any routed command reports `blocked`, `fix_required`, `plan-blocked`, `decision-checkpoint`, `human_verify`, unresolved `evidence_incomplete`, or any other non-continuable result
4. stop when the next required action is candidate truth repair, appendix writeback, Rule governance, repository mapping writeback, stable global baseline handling, or a user decision
5. stop when command preflight, process validation, deterministic cleanup, or command close cannot run authoritatively
6. stop when loop guard triggers

---

## 7. Scenario Advance

`scenario_advance:{scenario}` may automatically enter only these scenario commands:

1. `scenario_check:{scenario}`
2. `scenario_verify:{scenario}`
3. `scenario_promote:{scenario}`

Terminal completion:

1. when the scenario row records `Active Layer=stable`, `Candidate=no`, and `Next Command=scenario_fork`, advance is complete
2. advance must stop at that state and must not open the next candidate round

Continuation rules:

1. after `scenario_check` advances to `scenario_verify`, continue to `scenario_verify`
2. after `scenario_verify` advances to `scenario_promote`, continue to `scenario_promote`
3. after `scenario_promote` succeeds, stop at stable completion
4. when `scenario_verify` stops because affected units still require unit-local work, enter `unit_advance:{unit}` for each affected unit that is explicitly reported and confirmed in `_status.md`
5. after every affected unit reaches its own stable completion, re-read the scenario row and continue from the scenario's current `Next Command`

Affected-unit rules:

1. process affected units in the order reported by `scenario_verify`
2. do not infer affected units from directory shape, test failure text, or implementation files
3. if any affected unit is missing, not a unit row, or blocked by a human-in-the-loop stop, stop the parent scenario advance and report that unit as the immediate blocker
4. if affected-unit work changes Rule truth, repository mapping, global baseline, or another scenario dependency, stop and reroute through natural-language routing from current repository truth

Stop rules:

1. stop when the scenario row is missing or is not a scenario row
2. stop when `Next Command` is `scenario_new`, `scenario_fork`, or `scenario_stable_verify`
3. stop when `scenario_check` reports `blocked` or `fix_required`
4. stop when `scenario_verify` reports evidence incomplete without affected-unit work that can be advanced automatically
5. stop when `scenario_promote` reports stable dependency readiness failure, unless the active command file has already produced a legal routed affected-unit continuation
6. stop when the next required action is scenario truth repair, Rule governance, repository mapping writeback, stable dependency authoring, or a user decision
7. stop when command preflight, process validation, deterministic cleanup, or command close cannot run authoritatively
8. stop when loop guard triggers

---

## 8. Natural-Language Routing Relationship

Natural-language routing may route to advance only when the user's request includes an automatic-progression intent.

Examples that may route to advance:

```text
Advance payment until it is stable or blocked.
Advance payment until completion or a decision I must make.
Keep moving the checkout scenario until the next human decision.
```

Examples that must remain normal single-step routing:

```text
Continue payment.
Run the next step for checkout.
What is the current state of payment?
```

When natural-language routing routes to advance, advance becomes the active policy.
When advance stops because a later command or governance flow needs rerouting, the resume path is natural-language routing from current repository truth unless the active command's checkpoint says a narrower resume path.

---

## 9. Output Contract

Advance output must use the shared `specflow_response` user-facing surface.

The user-facing answer must state:

1. current state
2. commands actually completed during this advance run
3. next action
4. why advance stopped or why stable completion was reached
5. expected result of the next action when blocked
6. remaining gap, or that no gap remains for the current advance target

The execution note may state:

1. exact entry form
2. command history
3. observed `_status.md` transitions
4. recursive affected-unit targets
5. files changed by routed commands
6. stop reason
7. next legal entry

The execution note must not be required for the user to understand whether the advance completed or why it stopped.

---

## 10. Non-Goals

Advance does not:

1. weaken any command gate
2. create a new standard command
3. create a new process file
4. replace natural-language routing
5. replace rule governance
6. replace repository mapping writeback
7. run project-instance migration
8. open a new candidate round after stable completion
9. turn tooling into a semantic decision-maker
