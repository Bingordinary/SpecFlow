# Rule Bind

`rule_bind` is the internal rule-governance flow for binding one unit to an already-existing rule.

The binding is real only when the unit frontmatter contains the exact rule ref and the unit body explains how that rule is consumed.

### Entry Condition

This flow is valid only when a rule already exists (either global or bound shared) and a candidate unit needs to declare a dependency on it via `rule_refs`. It is not for creating or modifying rule truth.

## 1. Scope

`rule_bind` may:

1. add, remove, or retarget a rule ref in one candidate unit main Spec
2. update that candidate unit's body-level rule consumption explanation
3. validate `promotion_owner_unit` for a touched candidate rule that already has a stable sibling
4. run `rule_sync` after a binding change
5. invalidate the target unit candidate process state after the target unit main Spec changes

`rule_bind` must not:

1. create new rule truth as the main action
2. extract duplicated unit-local truth into a new rule
3. redesign the target rule body
4. modify stable unit truth directly
5. write consumer lists or `bound_objects` into rule files
6. allow a frontmatter-only binding with no body-level consumption explanation

## 2. Required Reads

Before any write, read:

1. `framework/spec_writing_guide.md`
2. `framework/governance/rules/rule_sync.md` (routes fallback to `framework/governance/impact_sync.md`)
3. the target unit current-layer main Spec
4. the target rule file
5. any currently bound rule file that may be replaced by this binding
6. every current-layer unit main Spec needed to derive the repository-wide bound shared rule consumer set for each touched rule from `rule_refs`
7. `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the request may affect a repository-wide default rule

==ATOM_BEGIN:shared_footer==
Bound shared rule consumer discovery must use only current-layer unit frontmatter `rule_refs`.
==ATOM_END:shared_footer==

==ATOM_BEGIN:rule_layout_note==
**Layout-aware path note:** Paths in this file use `<framework-root>` and `<tooling-root>` as layout-relative roots. In `source_repo` layout, `<framework-root>` is `framework/` and `<tooling-root>` is `tooling/`. In `installed_project` layout, both use a `specflow/` prefix before the root name (e.g., `specflow/framework/`, `specflow/tooling/`). `docs/specs/` paths are project-instance paths and are present only in `installed_project` layout.
==ATOM_END:rule_layout_note==

## 3. Procedure

1. Confirm that the target unit truly depends on the target rule truth.
2. If the target unit is stable, stop before writeback and return control to `rule_escape` to raise a `prerequisite_action` checkpoint.
3. Read the target unit's current `rule_refs` and record any previous rule ref that this round will remove or replace.
4. Build the repository-wide bound shared rule consumer set for the target rule and every previous touched rule from current-layer unit `rule_refs`.
5. Rewrite the target candidate unit `rule_refs` using exact rule refs and the sorting rules from `spec_writing_guide.md`.
6. Rewrite the target candidate unit body so the relevant behavior or acceptance chain explains the rule consumption.
7. If a touched candidate rule has a stable sibling, validate that exactly one `promotion_owner_unit` remains correct after this binding change. If that cannot be proven from current truth, stop and return to `rule_escape`.
8. If removing or retargeting the previous ref would leave a touched bound shared rule with no formal current consumers, do not leave its terminal state implicit:
    - delete it only when cleanup is already proven legal by current repository truth
    - otherwise write intentional unbound-retention fields (including `unbound_retention_owner: rule_bind`) when current truth proves the rule should remain independently authored
    - otherwise stop and return to `rule_escape` so the terminal-state decision can route to `rule_topology`
9. Remove unbound-retention fields from any touched bound shared rule that still has formal current consumers after the binding change.
10. Do not write consumer lists or `bound_objects` into touched rule files.
11. Run `rule_sync` after any unit `rule_refs` write or touched rule-file write.
    Execution-local inputs for `rule_sync`:
    - `rule_refs`: the exact refs added, removed, or retargeted in this bind round
    - `rule_ids`: the touched rule ids
    - `units`: the target unit plus any unit whose binding was read to prove the consumer set
    - `deleted_rule_refs`: the removed rule refs only when terminal deletion is proven by current repository truth
12. Ensure target unit candidate process state falls back after the candidate main Spec changes. If the `rule_sync` handoff does not include that target unit, use the candidate fallback rules from `framework/governance/impact_sync.md`.

If repository truth becomes insufficient before any mutation, stop and return to `rule_escape`.

## 4. Stop Conditions

Stop when one of these is true:

1. the candidate unit binding, body explanation, target unit fallback, touched rule terminal state, and `rule_sync` reconciliation are complete.
2. the request is not binding and must route to another rule flow or unit governance work
3. the target unit does not actually consume the rule truth
4. the target unit is stable and `rule_escape` must raise a `prerequisite_action` checkpoint before writeback can continue
5. a touched candidate rule with a stable sibling cannot keep or receive exactly one valid `promotion_owner_unit`
6. a touched rule would become unbound and its terminal state cannot be decided safely

## 5. Output Contract

The output must report:

1. the target unit and target rule
2. why the unit consumes that rule truth
3. whether the target unit was candidate or stopped with a `rule_escape` prerequisite checkpoint
4. the unit `rule_refs` writeback result
5. the body-level consumption explanation result
6. the consumer-set review result for each touched rule
7. the `promotion_owner_unit` result when required
8. any touched rule terminal-state result
9. confirmation that touched rule files do not carry `bound_objects`
10. the `rule_sync` result
11. the target unit candidate fallback result
