# Impact Sync

Impact sync reconciles downstream units after unit, rule, global rule, or repository mapping truth changes.

It owns consumer discovery and freshness classification for affected units.

## Triggers

Run impact sync when:

1. a stable unit version changes and another current-layer unit references the prior version.
2. a rule is created, changed, promoted, retired, renamed, merged, split, or rebound.
3. a stable global rule changes or gains an explicit exception.
4. repository mapping changes path ownership, object registration, implementation path registration, or support-surface boundaries used by current truth.
5. a governance flow cannot prove that downstream unit truth remains current.

## Inputs

Use the smallest durable truth that can prove affected consumers:

1. changed rule or global rule truth.
2. changed repository mapping entries.
3. promoted stable unit reference and release version.
4. current-layer unit frontmatter and dependency fields.


Do not infer consumers from implementation directories alone.

## Consumer Discovery

When `impact_sync` is called from `rule_sync` via the Rule Sync Handoff path (see below), it must accept the pre-computed affected-unit set as authoritative. The handoff input fields are: `invalidating_rule_refs` (rule refs whose truth changed), `affected_candidate_units` (candidate-layer unit names with invalidated evidence), `affected_stable_units` (stable-layer unit names with invalidated evidence), and `stable_landing_exceptions` (stable units that are landing targets and excluded from invalidation). `impact_sync` must not re-derive consumers from `rule_refs` in that case, because `rule_sync` already computed the affected set from the execution-local inputs that the caller proved. Independent consumer re-derivation is required only when `impact_sync` is triggered directly by a non-rule change (repository mapping update, stable unit version change, or governance-flow fallback).

Rule consumers are derived from current-layer unit frontmatter:

1. `g_rule_` files apply to every current-layer unit unless the global rule itself defines an explicit exception.
2. `b_rule_` files apply only to units whose `rule_refs` include that rule.
3. rule files must not store consumer lists.

Stable unit dependency consumers are derived from current-layer dependency fields and release-version references.

Repository mapping consumers are derived from object, implementation path, and support-surface registrations that overlap the changed mapping entry.

## Fallback Reason Classification

Use the canonical fallback reason codes below:
`no_drift_observed`, `plan_drift`, and `implementation_deviation` are additional codes used for
agent-internal routing decisions:

1. `no_drift_observed` — pre-trigger classification for the caller, not an output code of `impact_sync` itself. If the caller determines no evidence is invalidated, it may skip invoking `impact_sync`. `impact_sync` never assigns this code.
2. `truth_drift` - candidate behavior, boundary, or acceptance truth must be rewritten or rechecked.
3. `binding_drift` - a current unit or rule binding no longer matches current truth.
4. `baseline_drift` - a captured dependency or baseline no longer matches current truth.
5. `rule_drift` - a rule snapshot no longer matches the current rule.
6. `truth_incomplete` - required candidate truth is missing or incomplete.
7. `gate_missing` - required check gate evidence is missing or invalid.
8. `plan_drift` - candidate truth remains current, but the plan no longer validates.
9. `implementation_deviation` - implementation no longer satisfies current truth.
10. `evidence_incomplete` - candidate verification evidence is missing or invalid.
11. `stable_verify_invalid` - stable verification evidence is missing or invalid.
12. `spec_issue` - the candidate Spec requires repair.

When classification is uncertain, use the earliest proven invalidated layer and its canonical reason code.

## Rule Sync Handoff

Rule impact is handled directly — `specflowctl promote --rule` cascades MAJOR changes by forking stable consumers and updating candidate rule_refs (see `rule_promote_workflow.md`).

For non-promote rule edits (creation, extraction, bindings): the agent updates affected unit `rule_refs` directly per `spec_writing_guide.md` §5. Cache staleness is detected at promote time.

When `impact_sync` is triggered by a non-rule change, consumer discovery uses:
- `g_rule_` files → every current-layer unit (unless the rule defines an explicit exception)
- `b_rule_` files → only units whose `rule_refs` include that rule
- Rule files must not store consumer lists

## Stop Conditions

`impact_sync` terminates through one of the following conditions:

| Condition | Description | Next Action |
|-----------|-------------|-------------|
| **Normal completion** | Fallback routing applied to all affected units. All consumer discovery, freshness classification, and fallback routing steps are complete. | Return control to the caller (`rule_sync`, or direct governance trigger). |
| **No affected units** | Consumer discovery found zero affected units. | Close with no further action. Report `affected_units: none`. |

## Output Contract

After impact_sync completes, it produces:

1. `affected_candidate_units` — list of candidate units and their applied fallback reason codes
2. `affected_stable_units` — list of stable units and their applied fallback reason codes
3. `freshness_review_required` — when set to `true`, at least one affected unit requires the caller to run deterministic freshness classification before fallback cleanup. When set to `false` or absent from the output, no freshness review is needed.

## Removed Scenario Lifecycle

Requests that use `scenario_*`, `scenario_advance:{id}`, or `object-type=scenario` are not impact-sync work.
Stop and report that scenario lifecycle support has been removed.
