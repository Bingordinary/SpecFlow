# Rule Promote Workflow

`rule_promote` is the rule-equivalent of `spec_promote`. It takes a candidate rule and promotes it to stable. The behavior depends on the version change type (MAJOR/MINOR/PATCH).

Agent runs this when `spec_promote {target}` is called and the target has a `g_rule_` or `b_rule_` prefix.

## HARD RULES

1. Never call `specflowctl promote --rule <id>` without user confirmation
2. Before promote, always run `rule_validate` then `rule_verify`. If either fails, stop and report
3. The agent does not decide when to promote — it suggests, the user confirms

## Version Change Behavior

| Change type | Meaning | Consumer impact |
|-------------|---------|----------------|
| **MAJOR** (x.0.0) | Breaking constraint change | All stable consumers are auto-forked to candidate. Update their rule_refs. Re-validate and re-promote them. |
| **MINOR** (0.x.0) | Compatible extension | No consumer impact. Rule promoted without cascading. |
| **PATCH** (0.0.x) | Wording clarification | No consumer impact. Rule promoted without cascading. |
| None | Brand new rule (no previous stable) | No consumers exist yet. Rule promoted to stable. |

## Workflow

### Step 1 — Agent Pre-check (optional)

The agent may report cache state and version change type to help the user decide:

| Situation | What to say |
|-----------|-------------|
| MINOR/PATCH change, both caches fresh | "Compatible change. Rule validate and consumer check have passed. Ready for promotion — no consumers will be affected." |
| MAJOR change, both caches fresh | "Breaking change. Rule validate and consumer check have passed. Ready for promotion — all stable consumers will be forked to candidate for re-validation." |
| Caches stale/missing | "Cache is missing or expired. Run rule_validate (and rule_verify) first." |

### Step 2 — Run `specflowctl promote --rule <id>`

The CLI tool performs:

1. **Check candidate exists** — `docs/specs/rules/candidate/c_{rule_id}.md`
2. **Validate frontmatter** — `rule_id`, `rule_scope`, `layer`, `rule_version`
3. **Detect current stable version** — reads `docs/specs/rules/stable/s_{rule_id}.md` frontmatter
4. **Version sanity** — candidate version > stable version
5. **Determine version change type** — MAJOR vs MINOR vs PATCH
6. **Copy candidate→stable** — layer transform (`c_`→`s_`, `layer: candidate`→`layer: stable`)
7. **If MAJOR**: fork all stable consumers to candidate, update all candidate consumer rule_refs
8. **Delete candidate** — removes the candidate rule file

### Step 3 — Post-promote Body Reference Cleanup

After the CLI succeeds, the agent must act based on the change type:

**If MAJOR:**
1. Read the `--rule` output to find forked units (`ForkedConsumers`) and updated consumers (`CandidateUpdated`)
2. For each forked unit: the body may still reference the old rule version in prose. Agent scans and updates active references
3. For each updated candidate consumer: same scan for old version references in body text
4. Report to the user:
   - Which stable units were forked to candidate (need re-validation)
   - Which candidate units had their rule_refs updated
   - Suggest running `spec_validate` then `spec_verify` on each forked unit

**If MINOR/PATCH:**
1. No consumer changes needed
2. Report: "Compatible change promoted. No consumer impact."

## State After Promote

| Aspect | MAJOR | MINOR/PATCH |
|--------|-------|-------------|
| Stable rule file | Contains new version | Contains new version |
| Candidate rule file | Deleted | Deleted |
| Stable consumers | Forked to candidate | Unchanged (still stable) |
| Candidate consumer refs | Updated to new version | Unchanged (compatible) |
| Next step | Re-validate forked units | Done |
