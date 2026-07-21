# Rule Verify Checklist

`rule_verify` is the rule-equivalent of `spec_verify`. Instead of checking spec-vs-code alignment (rules have no implementation code), it checks consumer alignment — whether all units that reference this rule are still consistent with it.

Agent runs this when `spec_verify {target}` is called and the target has a `g_rule_` or `b_rule_` prefix.

**Result:** ALIGNED writes `docs/specs/_validation/rule/{id}/verify_result.md`.
MISMATCH does not write cache. The agent reports which consumers have drifted and how.

## Mode Selection

Before executing, read `framework/verification_scope.md` to determine the scope mode:

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `spec_verify {rule}` | scoped (default) | Step 1 only — consumer ref version check (cheapest, most common drift) |
| `spec_verify {rule}:full` | full | All 3 steps |

## Execution Rules

- Agent may read rule files, unit specs, run `specflowctl rule consumers` (read-only), search text patterns
- Agent must NOT modify files or execute write commands
- On MISMATCH: identify which consumers are out of alignment and the specific contradiction

## Procedure

### Step 1 — Consumer Rule Ref Version Check (candidate only)

1. Run `specflowctl rule consumers --rule-id {id}` to discover all units referencing this rule
2. For each consumer, check its active layer:
   - **Candidate**: verify the ref version matches the current rule stable version. Flag mismatch as DRIFTED.
   - **Stable**: **skip** — stable units are frozen truth. They are not expected to track the latest rule version.

### Step 2 — Consumer Body Version Reference Scan

1. For each consumer identified in Step 1, scan the unit spec body text
2. Find any direct references to the rule version (e.g., `b_rule_x@0.3.0` in prose, path references, or code blocks)
3. Classify each reference:
   - **active ref** — a reference that determines behavior or intent (e.g., "This component must follow b_rule_x@0.3.0")
   - **historical ref** — a reference that describes past state (e.g., "Previously used b_rule_x@0.3.0, now migrated to 0.4.0")
4. If any active ref points to a version other than the current stable version → flag the consumer as DRIFTED

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
- The specific contradiction (wrong version ref, active ref drift, semantic contradiction)
- A suggestion for remediation
