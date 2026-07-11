# Rule New

`rule_new` is the internal rule-governance flow for authoring independent rule truth.

It is used only after natural-language routing has already decided that the requested truth belongs in a rule file rather than inside one unit.

### Entry Condition

If invoked via the exact command `rule_new` without prior conversational context establishing the rule topic, stop and ask the user: what rule truth do they want to create, and whether it should be a global rule (`g_rule_`) or bound shared rule (`b_rule_`). Do not proceed until the rule topic is clear.

## 1. Scope

`rule_new` may:

1. create the first candidate rule file for a new rule object
2. update an existing candidate rule file that still represents the same rule object
3. open the next candidate-layer round for a rule object that already has a stable-layer file
4. update `docs/specs/repository_mapping.md` when a new rule object is registered or the rule object map changes
5. write `promotion_owner_unit` when a candidate rule has a stable sibling and one unit owns the later promotion decision
6. run `rule_sync` after rule truth writeback

`rule_new` must not:

1. extract existing unit-local truth from a unit body
2. bind a unit to an existing rule as the main action
3. modify stable unit truth directly
4. create, update, or promote stable rule semantics directly
5. write implementation files

When the request also requires unit binding, `rule_escape` must either decompose the work into a safe sequence or route the binding part to `rule_bind`. `rule_new` must not invent a unit consumer binding merely because the new rule exists.

## 2. Required Reads

Before any write, read:

1. `framework/spec_writing_guide.md`
2. `framework/governance/rules/rule_sync.md` (routes fallback to `framework/governance/impact_sync.md`)
3. every current-layer unit main Spec needed to check whether the target truth already exists as unit-local truth or is already bound through `rule_refs`
4. every existing rule file that names or overlaps the requested rule truth
5. `docs/specs/repository_mapping.md` when a new rule id is created or the rule object map may change
6. `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the request may become a repository-wide default rule

==ATOM_BEGIN:shared_footer==
Bound shared rule consumer discovery must use only current-layer unit frontmatter `rule_refs`.
==ATOM_END:shared_footer==

==ATOM_BEGIN:rule_layout_note==
**Layout-aware path note:** Paths in this file use `<framework-root>` and `<tooling-root>` as layout-relative roots. In `source_repo` layout, `<framework-root>` is `framework/` and `<tooling-root>` is `tooling/`. In `installed_project` layout, both use a `specflow/` prefix before the root name (e.g., `specflow/framework/`, `specflow/tooling/`). `docs/specs/` paths are project-instance paths and are present only in `installed_project` layout.
==ATOM_END:rule_layout_note==

## 3. Rule Identity

The rule id must use the rule's real scope:

1. `g_rule_` for a global rule
2. `b_rule_` for a bound shared rule

Each rule file must include `rule_id`, `rule_scope`, `layer`, and `rule_version`.

A brand-new candidate rule starts at `rule_version: 0.1.0`.

When a stable sibling already exists, the candidate file must carry the exact intended next stable `rule_version`. If the next version cannot be justified from current repository truth, stop and return to `rule_escape`.

## 4. Procedure

1. Confirm that the requested truth is independent rule truth, not unit-local behavior, a pure binding change, extraction, topology cleanup, or implementation work.
2. Build the repository-wide duplicate-truth review set from current-layer unit Specs.
3. Check that the same formal rule truth is not already present in another rule file or duplicated as unit-local formal truth.
4. Choose the smallest stable rule boundary. One rule file must carry one coherent shared constraint.
5. If the target bound shared rule already has a stable sibling, derive the current consumer set from current-layer unit `rule_refs` and choose exactly one valid `promotion_owner_unit`.
6. Create or update the candidate rule file at `docs/specs/rules/candidate/c_{rule_id}.md` (using the `rule_id` from section 3, e.g., `c_g_rule_example.md` for global rules, `c_b_rule_example.md` for bound rules).
7. If the bound shared rule has no formal current consumers after this write, keep it only when the file explicitly records intentional unbound retention with:
   - `unbound_retention: intentional`
   - `unbound_retention_reason: <why this rule is intentionally independent now>`
   - `unbound_retention_owner: rule_new`
8. If the bound shared rule has formal current consumers, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields.
9. Do not write consumer lists or `bound_objects` into the rule file.
10. Update `docs/specs/repository_mapping.md` in the same round when the rule object map changed.
11. Run `rule_sync` after any rule-file write or rule object-map write.
    Execution-local inputs for `rule_sync`:
    - `rule_refs`: the exact candidate ref that was written
    - `rule_ids`: the target rule id of the newly created or updated rule.
      Omit when opening a candidate round for a rule object that already has
      a stable-layer file (Scope item 3) and the stable-layer truth is not
      modified by this round — passing `rule_ids` in that case would trigger
      `rule_sync` step 6 all-unit invalidation even though stable truth
      did not change.
    - `units`: none by default (no binding changes in this flow); pass explicitly only when a writeback-required unit was touched

If repository truth becomes insufficient before any mutation, stop and return to `rule_escape`.

## 5. Stop Conditions

Stop when one of these is true:

1. the candidate rule file is written, any required repository mapping update is complete, and `rule_sync` has closed reconciliation.
2. the request belongs to `rule_extract`, `rule_bind`, `rule_topology`, or unit governance work instead
3. duplicate formal truth remains and cannot be removed by this flow
4. current repository truth is insufficient to justify the rule boundary, rule version, mapping update, or promotion owner
5. a candidate rule with a stable sibling would exist without exactly one valid `promotion_owner_unit`

## 6. Output Contract

The output must report:

1. the recognized rule truth and why it belongs in a rule file
2. the rule file created or updated
3. the written `rule_version`
4. whether a stable sibling exists
5. the `promotion_owner_unit` result when required
6. the duplicate-truth review set used for the decision
7. whether the bound shared rule is formally consumed through current-layer unit `rule_refs`
8. any repository mapping writeback
9. confirmation that the rule file does not carry `bound_objects`
10. the `rule_sync` result
