# Rule Validate Checklist

`rule_validate` is the rule-equivalent of `spec_validate`. It checks rule metadata structural validity (Checks 1-7) and rule body quality (Check 8).
Agent runs this when the target is detected as a Rule via automatic type detection (see `framework/concepts.md` §Automatic Target Type Detection).

**Result:** PASS writes `docs/specs/meta/validation/rule/{id}/validate_result.md`.
FAIL does not write cache. The agent reports which checks failed and why.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `spec_validate {rule}` | scoped (default) | git diff HEAD on rule file → map changes to check(s) → run with dependency handling |
| `spec_validate {rule}:check-{n}` | scoped | Single check `{n}` only |
| `spec_validate {rule}:{keyword}` | scoped | Match keyword to check name |
| `spec_validate {rule}:full` | full | All 8 checks |

**Scoped mapping (changed content → check):** Frontmatter (rule_id, rule_scope, layer, rule_version) → Checks 1-4, 6, 7. Body content → Check 8. promotion_owner_unit → Check 5 (WARNING only). No diff match → safety default (Check 1).

**Edge cases:** No rule file changes → auto fallback to full. New/untracked file → auto fallback to full. See `framework/verification_scope.md` §Scoped Validate and §Edge cases for full mapping detail.

## Execution Rules

- Agent may read rule files, search text patterns, check file existence
- Agent must NOT modify files, execute commands (beyond read-only tools), or delegate to other agents
- On FAIL: identify which checks failed and the contradictory information

## Checklist

### Check 1 — Frontmatter Completeness

Verify the rule file has all required frontmatter fields:

| Field | Required | Valid values |
|-------|----------|-------------|
| `rule_id` | Yes | `g_rule_{name}` or `b_rule_{name}` |
| `rule_scope` | Yes | `global` or `bound` |
| `layer` | Yes | `candidate` or `stable` |
| `rule_version` | Yes | `x.y.z` (semver) |

If any required field is missing or empty → FAIL.

### Check 2 — ID/Scope Consistency

Verify `rule_scope` matches the ID prefix:

| rule_id prefix | Expected rule_scope |
|----------------|---------------------|
| `g_rule_` | `global` |
| `b_rule_` | `bound` |

If `rule_scope` is `global` but ID starts with `b_rule_` → FAIL.
If `rule_scope` is `bound` but ID starts with `g_rule_` → FAIL.
When both are present, `rule_scope` in frontmatter takes precedence (per `spec_writing_guide.md`).

### Check 3 — File Path Consistency

Verify the rule file is in the correct directory:

| layer | Directory |
|-------|-----------|
| `candidate` | `docs/specs/rules/candidate/` |
| `stable` | `docs/specs/rules/stable/` |

Filenames follow the pattern `{g_or_b}_rule_{id}.md` — `g_rule_` for global rules, `b_rule_` for bound rules.

If path does not match the declared layer → FAIL.

### Check 4 — Version Semantics

If this is a brand-new rule (no stable file exists): verify `rule_version` equals `0.1.0`.

If a stable sibling exists (`docs/specs/rules/stable/{rule_id}.md`): read the stable file's frontmatter, extract its `rule_version`, and verify the candidate `rule_version` is semantically greater (MAJOR.MINOR.PATCH comparison). If candidate version is not greater than stable version → FAIL.

### Check 5 — `promotion_owner_unit` (optional documentation field)

`promotion_owner_unit` is an optional documentation field with no effect on tooling behavior. This check produces no execution failure.

→ WARNING if present but the value does not name a unit that exists in `docs/specs/units/`.

### Check 6 — Prohibited Fields

Verify the rule file does NOT contain:
- `bound_objects` — rule files must not store consumer lists
- A consumer list in any form — consumers are derived from unit `rule_refs`, never stored in rule files

If either is found → FAIL.

### Check 7 — `unbound_retention` Correctness

If the rule is a bound shared rule (`b_rule_`) AND has no current consumers: verify the file explicitly records:
- `unbound_retention: intentional`
- `unbound_retention_reason: <why this rule is intentionally independent>`
- `unbound_retention_owner: <flow name>`

If any of the three fields is missing → FAIL.

If the rule has current consumers: verify `unbound_retention` and its related fields are NOT present. If they are present → FAIL.

**Consumer discovery method:** search for `rule_refs` containing this `rule_id` in unit spec files under `docs/specs/units/`.

### Check 8 — Rule Body Quality

**Purpose:** Evaluate whether the rule's body content (constraint definition, exceptions, scope) is internally consistent and clearly stated. Unlike Checks 1-7 (metadata structural validity), this check evaluates the rule's written content.

**Execution steps:**

1. **Constraint clarity:** read the rule file body and identify the core constraint statement. Verify it is unambiguous — a reader can determine what behavior is required, prohibited, or permitted without guessing.

2. **Exception consistency:** read all exception clauses or scope limitations in the body. Verify no exception effectively nullifies the constraint (e.g., a rule saying "all APIs must use HTTPS" with an exception "except when HTTP is used"). If an exception contradicts the constraint → FAIL.

3. **Verifiability:** assess whether the constraint can be verified through static code inspection or spec_verify. A rule like "be intuitive" is not verifiable — flag as WARNING. A rule like "all API handlers must validate input before processing" is verifiable — PASS.

4. **Self-contradiction scan:** scan the body for statements that conflict with each other (e.g., "versions must be in semver format" in one paragraph, "versions are integers" in another). If found → FAIL.

**PASS:** Rule body is clear, internally consistent, and verifiable.

**WARNING:** Constraint is too vague to verify mechanically (step 3 only). WARNING does not affect the overall validate result — a validate with no FAIL checks passes, cache is written, and the WARNING is recorded in the result body.

**FAIL:** Contradictory exceptions, self-contradicting statements, or constraint nullified by exceptions.

**Check method:** Content reasoning — the agent reads and evaluates rule prose.
