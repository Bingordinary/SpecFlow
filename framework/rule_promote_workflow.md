# Rule Promote Workflow

`promote@{rule}` is the rule path of `promote`. It takes a candidate rule and promotes it to stable. Every successful publication, including the first, requires consumer discovery and content-impact assessment.

Agent runs this when the target is detected as a Rule via automatic type detection (see `framework/commands.md` §Target Resolution).

## HARD RULES

1. Never call `specflowctl promote --rule <id>` without user confirmation
2. Before promote, the `validate@{rule}` cache must be fresh and passing. If it is missing, stale, or blocking, stop and report; do not run the gate — gates are user-triggered
3. The agent does not decide when to promote — it suggests, the user confirms

## Publication Behavior

Every publication requires consumer discovery and content-impact assessment, whether the rule is new or changes an existing stable rule. No publication automatically starts consumer gates or changes consumer truth.

## Spec Removal

Explicit rule removal follows `framework/removal_workflow.md`. Read the constraint and relevant code before deciding. Publish surviving referrers that drop the rule first, then remove the specified rule. Promote never deletes unbound rules.

## Workflow

### Step 1 — Agent Pre-check (optional)

The agent may report cache state to help the user decide:

| Situation | What to say |
|-----------|-------------|
| Cache fresh | "Rule validate has passed. Ready for promotion — assess consumer impact after promote." |
| First publication, cache fresh | "Rule validate has passed. Ready for first publication — discover consumers and assess impact after promote." |
| Cache stale/missing | "Cache is missing or expired. Run `validate@{rule}` first." |

### Step 2 — Run `specflowctl promote --rule <id>`

The CLI tool performs:

1. **Check candidate exists** — `docs/specs/rules/candidate/{rule_id}.md`
2. **Check validate cache freshness** — reads `docs/specs/meta/validation/rule/{id}/validate_result.md`. If missing or stale (recorded content changed), rejects promote with guidance to run `validate@{rule}` first.
3. **Validate frontmatter** — `rule_id`, `rule_scope`
4. **Copy candidate→stable** — pure copy (the layer is encoded by the file path — no frontmatter field is transformed)
5. **Delete candidate** — removes the candidate rule file (the promoted rule's local gate-run state and unreferenced shared tasks are swept with it; see `framework/validation_cache.md` §Run lifecycle)
6. **Rewrite the validate cache** into a stable confirmation cache (`target: candidate` → `target: stable`, physical path from `docs/specs/rules/candidate/` to `docs/specs/rules/stable/`) — consumed by `fresh@stable` as the rule's consumer/consistency confirmation state

The cache projection preserves the published target's original evidence and live consumer evidence; no new content hashes or review verdicts are invented. Evidence that no longer matches the published content goes stale through the normal dependency check and drives the delta re-run scope (see `framework/validation_cache.md` §Cache lifecycle).

Rule removal uses `framework/removal_workflow.md`; rule and unit promote only publish their specified objects.

**PASS:** `specflowctl promote --rule <id>` exits with code 0, rule file copied, candidate cleaned up.
**FAIL:** CLI returns non-zero exit — report the CLI output. Do not archive any files. Recommend re-running `validate@{rule}` before retrying. Do not attempt manual promotion.

### Post-promote Consumer Impact

After every successful publication, including first publication, the agent must complete this procedure:

1. Run read-only `specflowctl consumers --rule <id>`. Global rules apply to every current-layer unit by default and are not repeated in `rule_refs`; bound consumers come from current-layer `rule_refs`. Never infer an empty consumer set from the absence of a previous stable rule.
2. Read the published rule, including its scope and exceptions. For each discovered unit, read its current-layer spec and non-exempt appendices, then assess whether the constraint applies and whether its content needs adjustment. Report applicable exceptions with their basis. If meaning or the required change is unclear, present the affected unit and decision to the user rather than inventing truth.
3. For each applicable consumer, run read-only `specflowctl fresh --unit <name>`. Report its actual publication blockers and missing, stale, or blocking gates. A bound-rule dependency already checked against identical candidate content may remain fresh after publication. A new or changed published global rule may introduce new required evidence. Publication alone never requires an unconditional gate re-run.
4. Report the publication result, consumer set, content-impact assessment and per-unit gate gaps. If changes are required, present the concrete affected-unit plan; apply only authorized changes through the existing candidate workflow. Stable-only units require `specflowctl fork --unit <name>` before editing; stable truth is never edited directly. Recommend only the actual applicable checks and wait for their user trigger. Resolve remaining rule publication blockers before recommending downstream checks.
5. If discovery returns no consumers, report `consumers: none` and close the impact step. If applicable consumers require no content changes and have no gate gaps, report that result explicitly. A discovery or input-read error leaves publication successful but impact assessment incomplete: report the failing path and stop; do not claim the consumer step completed.

The CLI output reminds the agent to assess consumer impact for every publication. Rule validate checks the rule's metadata and internal quality; it does not replace this consumer assessment.

## State After Promote

| Aspect | Every publication, including the first |
|--------|---------------------------------------|
| Stable rule file | Contains the published content |
| Candidate rule file | Deleted |
| Consumer impact | Discovered and assessed against the published content |
| Next step | Report required changes and actual gate gaps, or explicitly close with no remaining impact; gates remain user-triggered |
