# Promote Workflow

## Overview

When an agent executes `promote@{unit}`, it follows the 3 steps defined here. The trigger route in `framework/concepts.md` loads this file at promote time, not proactively.

## Execution Rules

- **Subagent permissions before promote:** may inspect file content. Must NOT modify files before `specflowctl promote` runs.
- **Subagent permissions after promote:** must NOT modify files.
- Each step reports **PASS** or **FAIL** with a reason.
- Resolution types:
  - **actionable** — A concrete repair can be made.
  - **needs_decision** — Requires user input (unclear intent, external dependency). Stop and ask.

## Output Format

```
Promote result: PASS | FAIL
1. Rule publication and agent pre-check: PASS | FAIL — reason
2. Body path check: PASS | FAIL — reason
3. specflowctl promote: PASS | FAIL — reason
Summary: ...
```

---

## Step 1 — Rule publication and agent pre-check

**Purpose:** Check direct rule publication and report the applicable gate state before invoking promote.

==ATOM_BEGIN:unit_rule_publication==
### Rule Publication Prerequisites

1. On a unit completion signal or `promote@{unit}`, run read-only `specflowctl fresh --unit <name>` before suggesting promote or supplementary gates. Ordinary discussion and editing do not trigger publication reminders.
2. For a normal candidate unit, publication blockers come from its own candidate `rule_refs`: an explicit rule with no stable file blocks; a missing rule blocks; a bound rule whose candidate differs from stable blocks until that rule is promoted. Compare normalized complete file content, including version and wording changes. Identical candidate/stable content passes. Unrelated bound rules and bindings dropped by this candidate do not block.
3. Changed or newly created candidate global rules are unpublished drafts. List them as advisories and recommend publishing them first only if this unit's current round is intended to adopt them. Their existence does not block work under the active stable global rules; identical global candidate/stable content needs no advisory.
4. Report every blocking rule ID and reason before recommending downstream checks. A read error stops with the failing path. The rule publication result is separate from validate/verify cache state: `READY` requires both the publication check and all applicable gates. Publication prerequisites apply to every unit; stable confirmation reports gain no promote condition.
5. Never automatically promote a rule or start a gate. After rule publication, read `fresh --unit <name>` and recommend only the applicable missing, stale, or blocking gates. Evidence captured against the same bound candidate content may remain fresh after its publication; do not require an unconditional re-run.
==ATOM_END:unit_rule_publication==

**Execution steps:**

1. Run `specflowctl fresh --unit <name>` and apply the publication prerequisites above. Stop on publication blockers or read errors; report applicable gate gaps without starting gates automatically.
2. **Optional unit-reference check:** read the candidate spec's `unit_refs`. A referenced unit that exists only in candidate will be rejected by the CLI and must be promoted first. Report such refs before running promote.
3. **Optional non-runnable review:**
   - Read the candidate spec at `docs/specs/units/candidate/unit_{name}.md`
   - Scan acceptance items for `runnable: no`
   - For each item found, assess:
     - Is `not_runnable_reason` present and substantive?
     - Is there a credible path or timeline for flipping to `yes`?
     - If the same item was already non-runnable in the stable predecessor (check git history for the previous stable spec), flag it as a concern — it has persisted across promote cycles
   - Report findings to the user

**PASS:** Rule publication prerequisites pass; applicable gate state and any global draft advisories are reported. The optional reviews may be skipped.
**FAIL:** Rule publication is blocked or a prerequisite input cannot be read. Report the rule IDs/reasons or failing path and stop before Step 3. No Spec or cache repair occurs in this workflow.

**Quality concern:** One or more non-runnable items persist from the previous stable spec; user attention recommended before promote

---

## Step 2 — Body path pre-check

**Purpose:** Find candidate-layer paths that would become invalid or point to the wrong design after publication.

**Execution steps:**

1. Read the candidate spec at `docs/specs/units/candidate/unit_{name}.md`
2. Parse the YAML frontmatter (`---...---`) to identify frontmatter boundaries
3. Search the full file content for all occurrences of:
   - `docs/specs/units/candidate/` (absolute form)
   - `candidate/` relative form with spec naming (e.g. `candidate/appendix/unit_{name}_...md`, `candidate/unit_{name}.md`)
4. For each occurrence, classify into:
   - **Structured field path** — Appears in `implementation_surface`, `affects.files`, `affects.appendices`, or `affects.dependencies` values. These are deterministic spec-to-spec references that must point to stable after promote. Per `framework/spec_writing_guide.md` §Acceptance Item Fields, `implementation_surface` is an "Implementation code surface path" — if it references a spec document path, use the stable layer path instead. A candidate-layer spec path in a structured field is invalid.
   - **Narrative reference** — Appears in prose, acceptance item `description`, or other free-text fields. May be semantically meaningful (e.g. "in the candidate phase...") — needs human judgment. Note: `validate` rejects such references at validate time (the mechanical Body layer-path check); if one reaches this step, the spec likely predates the rule or bypassed validate.
5. Report findings:
   - List each matched line with line number and surrounding context
   - Tag each match as `[structured]` or `[narrative]`
   - For structured matches, suggest the correct stable replacement path

**PASS:** No candidate-layer path references found in the file content.

**FAIL (actionable):** One or more structured field paths found. Report exact line numbers and matched paths. Inform the user and recommend editing the candidate spec to replace `candidate/` with `stable/` before re-running promote. The agent must NOT modify files during the promote workflow.

**FAIL (needs_decision):** Narrative references found. These require user judgment — report each occurrence and ask the user whether each should be updated. Do not proceed to Step 3 until resolved.

---

## Step 3 — Run specflowctl promote

**Purpose:** The CLI performs normal candidate-to-stable publication. Explicit deletion is owned by `framework/removal_workflow.md`.

**Execution steps:**

1. Run `specflowctl promote --unit <name>` from the repository root
2. The CLI independently checks:
   0. Rule publication — before any cache check, inspect the candidate unit's direct rule refs and pending global drafts using the same read-only check as `fresh`. Reject missing/unpublished explicit rule refs and bound candidate/stable content differences; global drafts are advisory. The internal promote operation repeats this check before any write, so direct callers cannot bypass it.
   a. Validate cache — reads `docs/specs/meta/validation/unit/{name}/validate_result.md`. If missing or stale (recorded content changed), rejects promote with guidance to re-run `validate`. The validate cache must have `result: pass` — a failure record (`result: fail` + `blocking: true`, written by a candidate full-run FAIL or by a delta/repair re-run's FAIL) is rejected as BLOCKED with guidance to resolve the findings.
   b. Merged verify cache — reads `docs/specs/meta/validation/unit/{name}/verify_result.md`. Must exist, mode must be `full`, must record a check for every expected key of both lenses (`alignment` and `quality`), must not be `blocking: true`, and the recorded whole-file hashes must be unchanged. If missing: "Verify not completed. Run `verify@{unit}` first." If mode is not `full`: "verify cache mode is %q, expected 'full' — run `verify@{unit}` before promoting." If the cache does not cover both lenses: "verify cache does not cover both lenses — missing key(s): ... Run `verify@{unit}` again." If stale: "Verify cache is stale. Run `verify@{unit}` again." If blocking: "Verify found P0/P1 finding(s). Resolve before promoting." The `blocking` field is required and must be consistent with `result` (`result: pass` → `blocking: false`, `result: fail` → `blocking: true`); a cache missing `blocking`, with an invalid `result` value, or with conflicting declarations fails closed and rejects promote. A pass cache with P2/P3 severity counts (non-blocking pending items) passes.
   c. Appendix cache — reads the validate cache and verifies every non-exempt candidate appendix file is listed in the validate cache's file list. If any appendix is missing, rejects promote with guidance to re-run `validate@{unit}`.
   d. All required cache checks pass → format validation (frontmatter, required fields, and ref target check — `unit_refs`/`rule_refs` pointing only to candidate-layer files are rejected with "promote it first" guidance; `rule_refs` at a nonexistent rule (removed) are rejected with "does not exist in stable or candidate" guidance) + appendix destination ownership preflight + copy candidate files to stable + remove candidate files. Before any write, every existing appendix destination must declare this unit as its owner under the current filename/frontmatter rules. A foreign owner or unreadable/missing/inconsistent ownership rejects the operation with the destination path; main specs, appendices, candidates, caches and baselines remain unchanged. This check also applies to exempt destinations.

**Check scope:** publication plus steps a–d are artifact-level checks (published rule content, cache presence, freshness, format, coverage). The CLI does not verify who executed the validate or verify runs — execution shape (session independence, read-only capability) is a runtime property declared by the workflow, not observable from the cache (see `framework/verification_scope.md` §Guarantee Boundary).

3. The CLI automatically:
   - Copies candidate content to stable verbatim (the layer is encoded by the file path — no frontmatter field is transformed, so promoted content is byte-identical and content-addressed caches of dependent units stay fresh)
   - Appendix filenames are preserved since they no longer encode layer
   - Rewrites the candidate-layer gate caches into stable confirmation caches (`target: candidate` → `target: stable`, physical paths rewritten from `docs/specs/units/candidate/` to `docs/specs/units/stable/`). The rewritten caches become the delta-recovery baseline: `fresh@stable` reports them, `re*` restores a stale one, and `fork` inherits them into the next round. See `framework/validation_cache.md` §Cache lifecycle.

**PASS:** `specflowctl promote --unit <name>` exits with code 0, all files copied and candidate cleaned up
**FAIL:** CLI returns non-zero exit — report the concrete prerequisite, gate, or format failure. For an unpublished rule, report which rule must be promoted first; for a cache gap, recommend only the applicable check. Do not prescribe a gate re-run for a rule publication blocker alone.

---


---

## Truth Semantics

Promote records reconciled design as accepted truth. After promote, candidate is removed and stable becomes the sole recorded reference; git history preserves the superseded stable content. Removing candidate files keeps file existence unambiguous. The candidate's local gate-run state and unreferenced shared tasks are swept with it (see `framework/validation_cache.md` §Run lifecycle). A new editing round starts with the fork prerequisite in `framework/concepts.md` §Default Editing Workflow.


Shared implementation files may be associated with multiple units. Unit verify reuses immutable public code judgments while keeping each unit's design and architecture decisions separate. Related stable acceptance requirements must remain ALIGNED before promote. See `framework/shared_judgments.md` for records, delta invalidation and protocol migration.
