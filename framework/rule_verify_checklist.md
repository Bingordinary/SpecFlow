# Rule Verify Checklist

`rule_verify` is the rule-equivalent of `spec_verify`. Instead of checking spec-vs-code alignment (rules have no implementation code), it checks consumer alignment — whether all units that reference this rule are still consistent with it.

Agent runs this when the target is detected as a Rule via automatic type detection (see `framework/concepts.md` §Automatic Target Type Detection).

**Result:** ALIGNED writes `docs/specs/_validation/rule/{id}/verify_result.md`.
MISMATCH does not write cache. The agent reports which consumers have drifted and how.

## Mode Selection

Before executing, read `framework/verification_scope.md` to determine the scope mode:

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `spec_verify {rule}` | scoped (default) | Git-aware: `git diff HEAD` → match changed files to spec content → verify that content |
| `spec_verify {rule}:full` | full | All 3 steps |

## Execution Rules

- Agent may read rule files, unit specs, search text patterns
- Agent must NOT modify files or execute write commands
- On MISMATCH: identify which consumers are out of alignment and the specific contradiction

## Procedure

### Step 1 — Consumer Rule Ref Check (candidate only)

1. Search for `rule_refs` containing this `rule_id` in unit spec files under `docs/specs/units/` to discover all units referencing this rule
2. For each consumer, check its active layer:
   - **Candidate**: verify the consumer's `rule_refs` contain a bare ref matching this rule. If the ref uses old `@version` format, flag as DRIFTED.
   - **Stable**: **skip** — stable units are frozen truth.

### Step 2 — Consumer Body Reference Scan

1. For each consumer identified in Step 1, scan the unit spec body text
2. Find any direct references to the rule by name in prose, path references, or code blocks
3. Check for any lingering `@version` suffix (e.g., `b_rule_x@0.3.0`). If an active ref uses `@version` format, the version may be stale — flag the consumer as DRIFTED
4. Bare refs (e.g., `b_rule_x`) are current by design; no version drift is possible

### Step 3 — Semantic Consumer Alignment Check

For each consumer, compare the rule's core constraint with the consumer's stated behavior:

1. Read the rule file body to understand its core constraint (scope, prohibitions, exceptions)
2. Read the consumer unit's body explanation of how it consumes this rule
3. Check for obvious contradictions at the prose level:
   - Rule says "prohibition X" → consumer says "uses X" (without explicit exception)
   - Rule scope excludes area Y → consumer operates in area Y (without mentioning the exclusion)
4. If an obvious contradiction exists → flag the consumer as DRIFTED

Do NOT perform deep semantic analysis. This check catches only clear, surface-level contradictions. Deep analysis is the responsibility of `spec_validate` on individual units.

## Result Classification

| Outcome | Condition |
|---------|-----------|
| ALIGNED | All consumers pass all 3 steps (or rule has no consumers and unbound_retention is correctly set) |
| MISMATCH | One or more consumers are DRIFTED |
| UNKNOWN | Cannot determine — insufficient data to check (e.g., tool not available) |

When MISMATCH, the agent must report:
- Which consumers are drifted, and at which step
- The specific contradiction (old `@version` format, semantic contradiction)
- A suggestion for remediation
