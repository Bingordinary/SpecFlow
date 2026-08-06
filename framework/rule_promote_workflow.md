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
| **Retired** (`status: retired`) | Rule is removed from stable | Agent must remove the rule from every unit that explicitly lists it in `rule_refs` before promote (explicit references only — a global rule's default applicability lifts when its file disappears); the CLI rejects a retire while explicit references remain |

## Retirement

A rule whose constraint no longer applies is retired by adding `status: retired` to the candidate rule frontmatter (see `framework/spec_writing_guide.md` §5.5). The retire promote follows the same cache gate, but instead of copying, the CLI removes the stable rule file and the candidate rule file. The version gate is skipped for retired rules (the stable copy is removed, not updated). Retirement is terminal — git history is the only record.

### Pre-check for retirement

1. Find all current-layer units that explicitly list the rule in `rule_refs`. `specflowctl consumers --rule <id>` does this for bound rules (`b_rule_*`) — its output is the explicit referrers. It must NOT be used for global rules (`g_rule_*`): for those it lists every current-layer unit (default applicability, not a reference). Instead, run `validate@{rule}` and read the retirement note of Check 7 (`unbound_retention` correctness) — it lists every remaining explicit referrer and fails the validate until they are gone. A global rule's default applicability to every unit is not a reference — it lifts automatically when the stable rule file disappears.
2. Each explicit referrer must drop the rule from its `rule_refs` (and the body explanation if present) and pass its own validate before the rule is retired. The CLI rejects the retire while any reference remains.
3. After retirement, `specflowctl consumers --rule <id>` reports no consumers for a bound rule (its explicit references were cleared before retire); for a global rule it reports the rule as not found because the rule file no longer exists — the default applicability has already lifted with the file, so no command confirmation is needed.

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
2. **Check validate cache freshness** — reads `docs/specs/meta/validation/rule/{id}/validate_result.md`. If missing or stale (hash mismatch), rejects promote with guidance to run `validate@{rule}` first.
3. **Validate frontmatter** — `rule_id`, `rule_scope`, `layer`, `rule_version`
4. **Detect current stable version** — reads `docs/specs/rules/stable/{rule_id}.md` frontmatter
5. **Version sanity** — candidate version > stable version
6. **Determine version change type** — MAJOR vs MINOR vs PATCH
7. **Copy candidate→stable** — layer transform (`layer: candidate`→`layer: stable`)
8. **Delete candidate** — removes the candidate rule file

**PASS:** `specflowctl promote --rule <id>` exits with code 0, rule file copied, candidate cleaned up.
**FAIL:** CLI returns non-zero exit — report the CLI output. Do not archive any files. Recommend re-running `rule_validate` before retrying. Do not attempt manual promotion.

### Post-promote Consumer Impact

After the CLI succeeds, the agent must act based on the change type:

**If MAJOR:**
1. Identify affected consumer units by running `specflowctl consumers --rule <id>` or searching for `rule_refs` containing the rule ID in `docs/specs/units/`
2. The agent should update affected units as needed and report to the user
3. Suggest running `validate` then `verify` on each affected unit

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
