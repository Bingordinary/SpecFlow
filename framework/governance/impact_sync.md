# Impact Sync

Impact sync reconciles downstream units after unit, rule, or global rule truth changes.

It owns consumer discovery and fallback reason classification for affected units. Deterministic freshness classification (the mechanical gate-state check) is owned by the caller per the Freshness Review section below — the two classifications are separate steps with separate owners.

## Triggers

Run impact sync when:

1. a stable unit version changes and another current-layer unit references the prior version.
2. path ownership, object registration, or support-surface boundaries used by current truth change in a way that cannot be resolved from unit or rule frontmatter. (To detect: check whether git changes include structural path changes in `docs/specs/`, or whether a governance flow explicitly reports unresolved boundary change.)
3. a governance flow cannot prove that downstream unit truth remains current.

## Inputs

Use the smallest durable truth that can prove affected consumers:

1. changed rule or global rule truth.
2. promoted stable unit reference and release version.
3. current-layer unit frontmatter and dependency fields.
4. when triggered by governance uncertainty (Trigger 3): the current truth snapshot and the governance flow's certainty boundaries.


Do not infer consumers from implementation directories alone.

## Consumer Discovery

Rule change impact is self-contained and does not route through `impact_sync`. For non-promote rule edits, the agent updates affected unit `rule_refs` directly per `spec_writing_guide.md` §6; cache staleness is detected at promote time.

The consumer discovery rules below apply only when `impact_sync` is triggered by a non-rule change (stable unit version change or governance-flow fallback).

Rule consumers are derived from current-layer unit frontmatter:

1. `g_rule_` files apply to every current-layer unit unless the global rule itself defines an explicit exception.
2. `b_rule_` files apply only to units whose `rule_refs` include that rule.
3. rule files must not store consumer lists.

Stable unit dependency consumers are derived from current-layer dependency fields.



## Fallback Reason Classification

Use the canonical fallback reason codes below. All codes except `no_drift_observed` are output codes that `impact_sync` may apply to affected units:

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

Layer discriminators for overlapping codes:

- `truth_drift` = the candidate truth content itself is invalidated; `spec_issue` = the Spec document needs repair while the underlying truth content may still be valid.
- `gate_missing` = a required gate's evidence is missing or invalid; `evidence_incomplete` = the candidate verification evidence is missing or invalid.

When classification is uncertain, use the earliest proven invalidated layer and its canonical reason code.

## Rule Change Impact

Rule impact is handled directly by the agent — `specflowctl promote --rule` simply promotes the rule file; the agent assesses and handles consumer impact per `rule_promote_workflow.md`.

For non-promote rule edits (creation, extraction, bindings): the agent updates affected unit `rule_refs` directly per `spec_writing_guide.md` §6. Cache staleness is detected at promote time.

When `impact_sync` is triggered by a non-rule change, consumer discovery follows the same rules as the Consumer Discovery section above: `g_rule_` files apply to every current-layer unit (unless the rule defines an explicit exception), `b_rule_` files apply only to units whose `rule_refs` include that rule, and rule files must not store consumer lists.

## Stop Conditions

`impact_sync` terminates through one of the following conditions:

| Condition | Description | Next Action |
|-----------|-------------|-------------|
| **Normal completion** | Fallback routing applied to all affected units. Consumer discovery and fallback reason classification are complete. | Return control to the caller (`spec_flow_update` for non-rule migration scenarios). If the output declares `freshness_review_required: true`, the caller must execute the Freshness Review procedure below before fallback cleanup. |
| **No affected units** | Consumer discovery found zero affected units. | Close with no further action. Report `affected_candidate_units: none` / `affected_stable_units: none`. |

## Output Contract

After impact_sync completes, it produces:

1. `affected_candidate_units` — list of candidate units and their applied fallback reason codes
2. `affected_stable_units` — list of stable units and their applied fallback reason codes
3. `freshness_review_required` — when set to `true`, at least one affected unit requires the caller to run the Freshness Review procedure below before fallback cleanup. When set to `false` or absent from the output, no freshness review is needed.

## Freshness Review (caller-owned)

`impact_sync` itself performs semantic classification only: it decides which units are affected and which fallback reason codes apply. It does not decide whether an affected unit's verification gates are currently fresh — gate freshness is a mechanical fact that the caller confirms with deterministic tooling before executing any fallback cleanup.

**When to run:** whenever the impact_sync output declares `freshness_review_required: true`. The review must complete before any fallback cleanup on the affected units.

**Procedure (deterministic, read-only):**

1. For every unit in `affected_candidate_units` and `affected_stable_units`, run the mechanical freshness report:
   ```bash
   specflowctl fresh --unit <name>
   ```
   (rule targets: `specflowctl fresh --rule <id>`; a whole-scope report `specflowctl fresh --scope all` may replace per-target runs when the affected set is large.)
2. Classify each affected unit by the reported gate statuses (FRESH / STALE / MISSING / BLOCKED, per `framework/validation_cache.md` §Freshness Check):
   - **FRESH** — all applicable gates have current, non-blocking caches. The unit's truth is mechanically current; fallback cleanup may proceed for this unit.
   - **STALE / MISSING / BLOCKED** — at least one applicable gate lacks current evidence (a declared dependency changed, the cache never ran, or a gate cache declares P0/P1 findings — a failure record). The unit's truth cannot be confirmed current; fallback cleanup must not proceed for this unit until the affected gate is re-run and the cache is fresh.
3. Report the classification per unit: `{unit}: {gate status} — cleanup allowed | cleanup blocked ({reason})`.

**Boundaries:**

1. `specflowctl fresh` is strictly read-only — it never writes or deletes caches or baselines (see `framework/validation_cache.md` §Freshness Check). The freshness review itself performs no writes.
2. The freshness review only decides whether fallback cleanup may proceed. It does not change the fallback reason codes assigned by `impact_sync`, and it does not reclassify affected units semantically.
3. A unit whose cleanup is blocked by freshness is reported to the user with the blocking gate; re-running the affected gate (validate / verify / review) is user-triggered per HARD RULE 2 in `framework/concepts.md`.
4. If the affected unit's target has no candidate file and no stable file (the target disappeared during the review), report it as `cleanup blocked (target missing)` and stop for user input — do not guess what the unit's cleanup should mean.

## Relationship to Governance Review Run-State

impact_sync operates independently of the governance-review run-state (`meta/governance_review/`).

- impact_sync outputs are ephemeral — communicated as agent output to the caller (user or governance flow).
- They are NOT written to `meta/governance_review/spec_flow_review.md` or any other run-state file.
- A governance-review slice (e.g., `process_and_impact_state`) may call impact_sync as part of its execution. The slice's findings and status are recorded in the run-state; the impact_sync outputs themselves remain ephemeral.
- The two mechanisms serve different purposes: impact_sync maintains downstream truth freshness; governance-review run-state tracks mechanism-review progress. They do not share state.

## Removed Scenario Lifecycle

Requests that use `scenario_*`, `scenario_advance:{id}`, or `object-type=scenario` are not impact-sync work.
Stop and report that scenario lifecycle support has been removed.
