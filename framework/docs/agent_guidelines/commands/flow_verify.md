# Flow Verify Command

## 1. Purpose

`flow_verify:{flow}` verifies whether the current candidate flow is actually wired from entry to outcome under the current bound object set.

## 2. Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `flow_verify` may produce that advancing result; later repair confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. `_status.md` says `Object Type=flow`, `Active Layer=candidate`, `Next Command=flow_verify`
2. current valid `_check_result/{flow}.md` exists

## 4. Procedure

1. read current candidate flow truth
2. revalidate current bound module, shared, and baseline snapshots
3. verify the business path from entry to claimed outcome
4. report `affected_modules` when implementation work is still required downstream
5. if pass, write `_verify_result/{flow}.md` and advance `Next Command=flow_promote`
6. if bindings drifted, fall back to `flow_check`

## 5. Output Contract

The output must report:

1. verification gate result
2. `_verify_result/{flow}.md` write, delete, or keep result
3. `_status.md` update result
4. `affected_modules` when downstream implementation is still required
5. `round conclusion`
6. `current state`
7. `next step`
8. `why this next step`
9. `next-stage entry gap`
10. the `user-facing close-out block` required by `specflow/framework/docs/agent_guidelines/command_policy.md` Section 8.6
11. if a future extension introduces a checkpoint stop, the same close-out block must also report `resume signal`

## 6. Non-Goals

1. replacing `module_impl:{module}`
2. implicitly repairing affected modules
