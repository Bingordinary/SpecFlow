# Rule Promote Workflow

`rule_promote` is the rule-equivalent of `promote`. It takes a candidate rule and promotes it to stable. The behavior depends on the version change type (MAJOR/MINOR/PATCH).

Agent runs this when the target is detected as a Rule via automatic type detection (see `framework/concepts.md` §Automatic Target Type Detection).

## HARD RULES

1. Never call `specflowctl promote --rule <id>` without user confirmation
2. Before promote, always run `rule_validate`. If it fails, stop and report
3. The agent does not decide when to promote — it suggests, the user confirms

## Version Change Behavior

| Change type | Meaning | Consumer impact |
|-------------|---------|----------------|
| **MAJOR** (x.0.0) | Breaking constraint change | Agent should identify affected units and update them. No automatic cascade. |
| **MINOR** (0.x.0) | Compatible extension | Assess consumer impact per rule content. Typically none. |
| **PATCH** (0.0.x) | Wording clarification | Assess consumer impact per rule content. Typically none. |
| None | Brand new rule (no previous stable) | No consumers exist yet. Rule promoted to stable. |

## Rule Removal

A rule whose constraint no longer applies is removed with `specflowctl remove --rule <id>` — the `status: retired` declaration flow no longer exists (see `framework/spec_writing_guide.md` §6.5). Removal is the end of the rule: the rule files are deleted and git history is the only record.

### Pre-check for removal

1. Find the rule's current-layer (effective) consumers: `specflowctl detect --rule <id>` reports them, or `specflowctl detect --all` lists every removal candidate (bound rules with no consumers and no `unbound_retention` declaration). Global rules (`g_rule_*`) are never listed — they apply to every unit by default.
2. Each consumer must drop the rule from its `rule_refs` (and the body explanation if present) and pass its own validate before the rule is removed. `remove` rejects the deletion while any current-layer reference remains, listing the referrers.
3. `unbound_retention` exempts a rule from removal: a rule declaring intentional retention is rejected by `remove` — remove the retention fields first, or keep the rule.
4. After removal, `specflowctl consumers --rule <id>` reports the rule as not found because the rule file no longer exists — no command confirmation is needed.

Removal is also triggered automatically: a unit promote removes every bound rule its candidate dropped from `rule_refs` that is left with no consumers and no retention declaration (reported explicitly in the promote actions). `fresh` reports embed the removal-candidate list read-only.

## Workflow

### Step 1 — Agent Pre-check (optional)

The agent may report cache state and version change type to help the user decide:

| Situation | What to say |
|-----------|-------------|
| MINOR/PATCH change, cache fresh | "Compatible change. Rule validate has passed. Ready for promotion — assess consumer impact after promote (typically none)." |
| MAJOR change, cache fresh | "Breaking change. Rule validate has passed. Ready for promotion — verify consumer impact after promote." |
| Cache stale/missing | "Cache is missing or expired. Run rule_validate first." |

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

Rule removal is a separate command: `specflowctl remove --rule <id>` (see `framework/spec_writing_guide.md` §6.5). A unit promote additionally removes every bound rule its candidate dropped from `rule_refs` that is left with no current-layer consumers and no `unbound_retention` declaration; the removed rules are listed explicitly in the promote report.

**PASS:** `specflowctl promote --rule <id>` exits with code 0, rule file copied, candidate cleaned up.
**FAIL:** CLI returns non-zero exit — report the CLI output. Do not archive any files. Recommend re-running `rule_validate` before retrying. Do not attempt manual promotion.

### Post-promote Consumer Impact

After the CLI succeeds, the agent must act based on the change type:

**If MAJOR:**
1. Identify affected consumer units by running `specflowctl consumers --rule <id>` or searching for `rule_refs` containing the rule ID in `docs/specs/units/`
2. For each affected unit that needs a content update:
   - If the unit has no candidate file, fork it first per HARD RULE 5 (`specflowctl fork --unit <name>` — stable is never edited directly)
   - Update the candidate content per the rule's new constraint
   - Suggest running `validate` then `verify` on each affected unit (user-triggered per HARD RULE 2)
3. Note that the rule promote already made each affected unit's validate cache stale (the cache declares `rule:{id}` as a logical reference), so the unit's promote is mechanically rejected until it is re-validated — no extra action is needed beyond the re-validation above
4. Report the tool output and the affected-unit plan to the user

**If MINOR/PATCH:**
1. Assess consumer impact per rule content. Typically no impact — confirm and proceed.
2. The tool output already includes the "Assess consumer impact per rule content" guidance. Report the tool output to the user.

## State After Promote

| Aspect | MAJOR | MINOR/PATCH |
|--------|-------|-------------|
| Stable rule file | Contains new version | Contains new version |
| Candidate rule file | Deleted | Deleted |
| Consumer impact | Agent must verify | Assess per rule content (typically none) |
| Next step | Agent identifies affected units and validates | Done |
