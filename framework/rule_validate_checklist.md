# Rule Validate Checklist

`rule_validate` is the rule-equivalent of `spec_validate`. It checks rule design quality.
Agent runs this when `spec_validate {target}` is called and the target has a `g_rule_` or `b_rule_` prefix.

**Result:** PASS writes `docs/specs/_validation/rule/{id}/validate_result.md`.
FAIL does not write cache. The agent reports which checks failed and why.

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

Verify the rule file is in the correct directory and uses the correct prefix:

| layer | Directory | File prefix |
|-------|-----------|-------------|
| `candidate` | `docs/specs/rules/candidate/` | `c_` |
| `stable` | `docs/specs/rules/stable/` | `s_` |

File prefix must also match scope: `c_g_rule_`/`s_g_rule_` for global, `c_b_rule_`/`s_b_rule_` for bound.

If path does not match the declared layer and ID prefix → FAIL.

### Check 4 — Version Semantics

If this is a brand-new rule (no stable file exists): verify `rule_version` equals `0.1.0`.

If a stable sibling exists (`docs/specs/rules/stable/s_{rule_id}.md`): read the stable file's frontmatter, extract its `rule_version`, and verify the candidate `rule_version` is semantically greater (MAJOR.MINOR.PATCH comparison). If candidate version is not greater than stable version → FAIL.

### Check 5 — `promotion_owner_unit` Correctness

If the candidate rule has a stable sibling: verify exactly one `promotion_owner_unit` field is present in the candidate frontmatter, and the value is a valid unit name (exists in `docs/specs/units/`).

If the candidate rule has no stable sibling: verify `promotion_owner_unit` is NOT present. If it is present → FAIL.

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
