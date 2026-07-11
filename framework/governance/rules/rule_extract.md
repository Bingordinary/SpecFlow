# Rule Extract

`rule_extract` is the internal rule-governance flow for moving existing unit-local formal truth into one independent rule.

It is used only when the shared constraint already exists in current unit truth and must stop being duplicated inside unit bodies.

### Entry Condition

This flow is only valid when a shared constraint already exists within one or more unit bodies and needs to be extracted into an independent rule file. It is not for creating new rule truth — use `rule_new` for that.

## 1. Scope

`rule_extract` may:

1. create or update a candidate rule file for the extracted truth
2. rewrite candidate unit `rule_refs` so the affected units consume the extracted rule
3. rewrite candidate unit prose so the unit body explains how it consumes the rule
4. remove duplicated formal truth from candidate unit bodies
5. update `docs/specs/repository_mapping.md` when a new rule object is registered or the rule object map changes
6. run `rule_sync` after rule truth or unit binding writeback

`rule_extract` must not:

1. design a new rule with no current unit-local source truth
2. bind a unit to an unchanged existing rule as the main action
3. modify stable unit truth directly
4. promote rule truth into a stable rule file
5. leave the same formal constraint duplicated in both unit truth and rule truth

## 2. Required Reads

Before any write, read:

1. `framework/spec_writing_guide.md`
2. `framework/governance/rules/rule_sync.md` (routes fallback to `framework/governance/impact_sync.md`)
3. every named source unit's current-layer main Spec
4. every current-layer unit main Spec needed to determine whether that unit already carries, duplicates, or consumes the target truth
5. every relevant existing rule file that may overlap the target truth
6. `docs/specs/repository_mapping.md` when a new rule id is created or the rule object map may change
7. `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the request may become a repository-wide default rule

==ATOM_BEGIN:shared_footer==
Bound shared rule consumer discovery must use only current-layer unit frontmatter `rule_refs`.
==ATOM_END:shared_footer==

==ATOM_BEGIN:rule_layout_note==
**Layout-aware path note:** Paths in this file use `<framework-root>` and `<tooling-root>` as layout-relative roots. In `source_repo` layout, `<framework-root>` is `framework/` and `<tooling-root>` is `tooling/`. In `installed_project` layout, both use a `specflow/` prefix before the root name (e.g., `specflow/framework/`, `specflow/tooling/`). `docs/specs/` paths are project-instance paths and are present only in `installed_project` layout.
==ATOM_END:rule_layout_note==

## 3. Procedure

1. Confirm that the request is extraction of existing unit-local formal truth.
2. Identify the smallest rule object that carries only the shared constraint.
3. Build the complete involved-unit set from current repository truth:
   - units that currently carry the source truth
   - units that already consume equivalent rule truth
   - units whose current-layer body or `rule_refs` must change for extraction closure
4. Decide the writeback-required unit subset. A unit is writeback-required only when its current-layer `rule_refs` or body text must change.
5. If any writeback-required unit is currently stable, stop before writeback and return control to `rule_escape` to raise a `prerequisite_action` checkpoint.
6. Create or update the target candidate rule file.
7. If this is the first file for a new rule object, write `rule_version: 0.1.0`.
8. If the target rule has a stable sibling, write or validate exactly one `promotion_owner_unit` from the writeback-required unit subset. If no such owner can be named from current truth, stop and return to `rule_escape`.
9. Update `docs/specs/repository_mapping.md` in the same round when the rule object map changed.
10. Rewrite each source candidate unit so the extracted truth no longer remains as duplicated unit-local formal truth.
11. Rewrite each affected candidate unit's `rule_refs` and body explanation required by the extraction result.
12. Remove unbound-retention fields from the target rule when the resulting current-layer unit `rule_refs` graph has bound shared rule consumers.
13. If the resulting rule remains intentionally unbound, write intentional unbound-retention fields in the target rule (including `unbound_retention_owner: rule_extract`); otherwise reject closure.
14. Do not write consumer lists or `bound_objects` into any rule file.
15. Run `rule_sync` after any rule-file write or unit `rule_refs` write.
    Execution-local inputs for `rule_sync`:
    - `rule_refs`: the new candidate rule refs and any refs changed by rewriting affected unit `rule_refs`
    - `rule_ids`: the touched rule ids (newly created rule + any modified sibling rule ids)
    - `units`: the writeback-required unit subset (units whose `rule_refs` were rewritten)
    - `deleted_rule_refs`: none (extraction does not delete rules)

If repository truth becomes insufficient before any mutation, stop and return to `rule_escape`.

## 4. Stop Conditions

Stop when one of these is true:

1. extraction is complete, duplicated unit-local formal truth is removed, required unit bindings are written, and `rule_sync` has closed reconciliation.
2. the request is not extraction and must route to another rule flow or unit governance work
3. a writeback-required unit is stable and `rule_escape` must raise a `prerequisite_action` checkpoint before extraction can continue
4. unit-local truth and shared rule truth cannot be separated safely from current repository truth
5. involved-unit coverage is incomplete or uncertain
6. a candidate rule with a stable sibling would exist without exactly one valid `promotion_owner_unit`

## 5. Output Contract

The output must report:

1. the extracted rule truth and source unit truth
2. the complete involved-unit set
3. which units were rewritten and which units were read only
4. any stable unit that blocked writeback and the `rule_escape` prerequisite checkpoint result
5. the rule file created or updated
6. the written `rule_version`
7. the `promotion_owner_unit` result when required
8. the unit-side rewrite result and whether duplicate formal truth was removed
9. any repository mapping writeback
10. confirmation that touched rule files do not carry `bound_objects`
11. the `rule_sync` result
