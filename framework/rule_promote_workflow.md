# Rule Promote Workflow

`rule_promote` is the rule-equivalent of `spec_promote`. It takes a candidate rule and promotes it to stable. The behavior depends on the version change type (MAJOR/MINOR/PATCH).

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
2. **Validate frontmatter** — `rule_id`, `rule_scope`, `layer`, `rule_version`
3. **Detect current stable version** — reads `docs/specs/rules/stable/{rule_id}.md` frontmatter
4. **Version sanity** — candidate version > stable version
5. **Determine version change type** — MAJOR vs MINOR vs PATCH
6. **Copy candidate→stable** — layer transform (`layer: candidate`→`layer: stable`)
7. **Delete candidate** — removes the candidate rule file

**PASS:** `specflowctl promote --rule <id>` exits with code 0, rule file copied, candidate cleaned up.
**FAIL:** CLI returns non-zero exit — report the CLI output. Do not archive any files. Recommend re-running `rule_validate` before retrying. Do not attempt manual promotion.

### Post-promote Consumer Impact

After the CLI succeeds, the agent must act based on the change type:

**If MAJOR:**
1. Identify affected consumer units by running `specflowctl consumers --rule <id>` or searching for `rule_refs` containing the rule ID in `docs/specs/units/`
2. The agent should update affected units as needed and report to the user
3. Suggest running `spec_validate` then `spec_verify` on each affected unit

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
