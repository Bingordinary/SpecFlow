# Rule Promote Workflow

`promote@{rule}` is the rule path of `promote`. It takes a candidate rule and promotes it to stable. The behavior depends on the version change type (MAJOR/MINOR/PATCH).

Agent runs this when the target is detected as a Rule via automatic type detection (see `framework/commands.md` §Target Resolution).

## HARD RULES

1. Never call `specflowctl promote --rule <id>` without user confirmation
2. Before promote, the `validate@{rule}` cache must be fresh and passing. If it is missing, stale, or blocking, stop and report; do not run the gate — gates are user-triggered
3. The agent does not decide when to promote — it suggests, the user confirms

## Version Change Behavior

| Change type | Meaning | Consumer impact |
|-------------|---------|----------------|
| **MAJOR** (x.0.0) | Breaking constraint change | Agent should identify affected units and update them. No automatic cascade. |
| **MINOR** (0.x.0) | Compatible extension | Assess consumer impact per rule content. Typically none. |
| **PATCH** (0.0.x) | Wording clarification | Assess consumer impact per rule content. Typically none. |
| None | Brand new rule (no previous stable) | No consumers exist yet. Rule promoted to stable. |

## Spec Removal

Explicit rule removal follows `framework/removal_workflow.md`. Read the constraint and relevant code before deciding. Publish surviving referrers that drop the rule first, then remove the specified rule. Promote never deletes unbound rules.

## Workflow

### Step 1 — Agent Pre-check (optional)

The agent may report cache state and version change type to help the user decide:

| Situation | What to say |
|-----------|-------------|
| MINOR/PATCH change, cache fresh | "Compatible change. Rule validate has passed. Ready for promotion — assess consumer impact after promote (typically none)." |
| MAJOR change, cache fresh | "Breaking change. Rule validate has passed. Ready for promotion — verify consumer impact after promote." |
| Cache stale/missing | "Cache is missing or expired. Run `validate@{rule}` first." |

### Step 2 — Run `specflowctl promote --rule <id>`

The CLI tool performs:

1. **Check candidate exists** — `docs/specs/rules/candidate/{rule_id}.md`
2. **Check validate cache freshness** — reads `docs/specs/meta/validation/rule/{id}/validate_result.md`. If missing or stale (dependency chunk changed), rejects promote with guidance to run `validate@{rule}` first.
3. **Validate frontmatter** — `rule_id`, `rule_scope`, `rule_version`
4. **Detect current stable version** — reads `docs/specs/rules/stable/{rule_id}.md` frontmatter
5. **Version sanity** — candidate version > stable version
6. **Determine version change type** — MAJOR vs MINOR vs PATCH
7. **Copy candidate→stable** — pure copy (the layer is encoded by the file path — no frontmatter field is transformed)
8. **Delete candidate** — removes the candidate rule file
9. **Rewrite the validate cache** into a stable confirmation cache (`target: candidate` → `target: stable`, physical path from `docs/specs/rules/candidate/` to `docs/specs/rules/stable/`) — consumed by `fresh@stable` as the rule's consumer/consistency confirmation state

Rule removal uses `framework/removal_workflow.md`; rule and unit promote only publish their specified objects.

**PASS:** `specflowctl promote --rule <id>` exits with code 0, rule file copied, candidate cleaned up.
**FAIL:** CLI returns non-zero exit — report the CLI output. Do not archive any files. Recommend re-running `validate@{rule}` before retrying. Do not attempt manual promotion.

### Post-promote Consumer Impact

After the CLI succeeds, the agent must act based on the change type:

**If MAJOR:**
1. Identify affected consumer units by running `specflowctl consumers --rule <id>`, or — for a bound (`b_rule_`) rule only — searching for `rule_refs` containing the rule ID in `docs/specs/units/` (a global `g_rule_` rule is not repeated in unit `rule_refs`; the `consumers` command is the only correct discovery path for it)
2. For each affected unit that needs a content update:
   - If the unit has no candidate file, fork it first per HARD RULE 5 in `framework/concepts.md` (`specflowctl fork --unit <name>` — stable is never edited directly)
   - Update the candidate content per the rule's new constraint
   - Run read-only `specflowctl fresh --unit <name>` and suggest only the applicable missing, stale, or blocking gates (user-triggered per HARD RULE 2 in `framework/concepts.md`)
3. Confirm gate state with `fresh` instead of assuming publication made every consumer stale. A bound-rule dependency already checked against the identical candidate content stays fresh when that content is promoted; global dependencies use stable content, so a changed published global dependency may stale the evidence. Unit content or implementation updates may independently require re-checks. Publication itself never requires an unconditional gate re-run.
4. Report the tool output and the affected-unit plan to the user

**If MINOR/PATCH:**
1. Assess consumer impact per rule content. Typically no impact — confirm and proceed.
2. The tool output already includes the "Assess consumer impact per rule content" guidance. Report the tool output to the user.

For any consumer being prepared for promotion, read `specflowctl fresh --unit <name>` after rule publication. Resolve its remaining rule publication blockers first and recommend only its actual applicable gate gaps. MINOR/PATCH version labels do not waive the unit's complete-content publication check.

## State After Promote

| Aspect | MAJOR | MINOR/PATCH |
|--------|-------|-------------|
| Stable rule file | Contains new version | Contains new version |
| Candidate rule file | Deleted | Deleted |
| Consumer impact | Agent must verify | Assess per rule content (typically none) |
| Next step | Agent identifies affected units and validates | Done |
