# Promote Workflow

## Overview

When an agent executes `promote@{unit}`, it follows the 3 steps defined in this file. This file is referenced by `framework/concepts.md` §3 — the agent reads this file at promote time, not proactively.

## Execution Rules

- **Subagent permissions before promote:** may inspect file content. Must NOT modify files before `specflowctl promote` runs.
- **Subagent permissions after promote:** must NOT modify files.
- Each step reports **PASS** or **FAIL** with a reason.
- Resolution types:
  - **fix_required** — A concrete repair can be made.
  - **blocked** — Requires user input (unclear intent, external dependency). Stop and ask.

## Output Format

```
Promote result: PASS | FAIL
1. Agent pre-check (optional): PASS | FAIL — reason
2. Body path check: PASS | FAIL — reason
3. specflowctl promote: PASS | FAIL — reason
Summary: ...
```

---

## Step 1 — Agent pre-check (optional)

**Purpose:** Optionally check cache freshness and review non-runnable items before promote. Cache check is redundant with the CLI's own enforcement but provides transparency. The non-runnable review catches items that may have become runnable since the last verify cycle.

**Execution steps:**

1. Read `docs/specs/meta/validation/unit/{name}/validate_result.md` if it exists
2. Read `docs/specs/meta/validation/unit/{name}/verify_result.md` if it exists
3. Report freshness status to the user if reporting would be useful
4. **Reference target check:**
   - Read the candidate spec's `unit_refs` and `rule_refs` frontmatter fields
   - For each referenced unit/rule, check whether a stable-layer file exists (`docs/specs/units/stable/unit_{ref}.md` / `docs/specs/rules/stable/{ref}.md`)
   - A ref whose target exists only in the candidate layer (`docs/specs/units/candidate/` / `docs/specs/rules/candidate/`) will be rejected by `specflowctl promote` — the referenced unit/rule must be promoted first
   - Report any such ref to the user before running promote
5. **Non-runnable review:**
   - Read the candidate spec at `docs/specs/units/candidate/unit_{name}.md`
   - Scan acceptance items for `runnable: no`
   - For each item found, assess:
     - Is `not_runnable_reason` present and substantive?
     - Is there a credible path or timeline for flipping to `yes`?
     - If the same item was already non-runnable in the stable predecessor (check git history for the previous stable spec), flag it as a concern — it has persisted across promote cycles
   - Report findings to the user

**PASS:** Freshness information reported; no unresolved non-runnable concerns (or step skipped)
**FAIL:** Not applicable — this step is optional and cannot fail

**Quality concern:** One or more non-runnable items persist from the previous stable spec; user attention recommended before promote

---

## Step 2 — Body path pre-check

**Purpose:** Scan the candidate spec body for candidate-layer path references that will break after promote (candidate files are deleted). Per `framework/concepts.md` §4, body text should reference specs by concept name rather than layer-prefixed file paths. `validate` Check 1 step 10 already rejects layer-prefixed paths at validate time — this step is the last-resort gate for content that predates or bypassed that check.

**Execution steps:**

1. Read the candidate spec at `docs/specs/units/candidate/unit_{name}.md`
2. Parse the YAML frontmatter (`---...---`) to identify frontmatter boundaries
3. Search the full file content for all occurrences of:
   - `docs/specs/units/candidate/` (absolute form)
   - `candidate/` relative form with spec naming (e.g. `candidate/appendix/unit_{name}_...md`, `candidate/unit_{name}.md`)
4. For each occurrence, classify into:
   - **Structured field path** — Appears in `implementation_surface`, `affects.files`, `affects.appendices`, or `affects.dependencies` values. These are deterministic spec-to-spec references that must point to stable after promote. Per `framework/spec_writing_guide.md` §Acceptance Item Fields, `implementation_surface` is an "Implementation code surface path" — if it references a spec document path, use the stable layer path instead. A candidate-layer spec path in a structured field is invalid.
   - **Narrative reference** — Appears in prose, acceptance item `description`, or other free-text fields. May be semantically meaningful (e.g. "in the candidate phase...") — needs human judgment. Note: `validate` Check 1 step 10 rejects such references at validate time; if one reaches this step, the spec likely predates the rule or bypassed validate.
5. Report findings:
   - List each matched line with line number and surrounding context
   - Tag each match as `[structured]` or `[narrative]`
   - For structured matches, suggest the correct stable replacement path

**PASS:** No candidate-layer path references found in the file content.

**FAIL (fix_required):** One or more structured field paths found. Report exact line numbers and matched paths. Inform the user and recommend editing the candidate spec to replace `candidate/` with `stable/` before re-running promote. The agent must NOT modify files during the promote workflow.

**FAIL (blocked):** Narrative references found. These require user judgment — report each occurrence and ask the user whether each should be updated. Do not proceed to Step 3 until resolved.

---

## Step 3 — Run specflowctl promote

**Purpose:** The CLI performs the mechanical candidate-to-stable transition. This is the only gate that writes to stable.

**Execution steps:**

1. Run `specflowctl promote --unit <name>` from the repository root
2. The CLI independently checks:
   a. Validate cache — reads `docs/specs/meta/validation/unit/{name}/validate_result.md`. If missing or stale (hash mismatch), rejects promote with guidance to re-run `validate`.
   b. Verify cache — reads `docs/specs/meta/validation/unit/{name}/verify_result.md`. If missing or stale, rejects promote with guidance to re-run `verify`. The verify cache must have `result: pass` — any other value is rejected. P0/P1 findings never write a cache (the cache is deleted, so promote fails with "verify cache not found"); a full cache with `result: pass` and P2/P3 severity counts (non-blocking pending items) passes.
   c. Review cache — reads `docs/specs/meta/validation/unit/{name}/review_result.md`. Must exist, mode must be `full`, must not be `blocking: true`, and hashes must match. If missing: "Review not completed. Run `review@{unit}` first." If mode is not `full`: "review cache mode is %q, expected 'full' — run `review@{unit}` before promoting." If stale: "Review cache is stale. Run `review@{unit}` again." If blocking: "Review found P0/P1 finding(s). Resolve before promoting." The `blocking` field is required and must be consistent with `result` (`result: pass` → `blocking: false`, `result: fail` → `blocking: true`); a cache missing `blocking`, with an invalid `result` value, or with conflicting declarations fails closed and rejects promote.
   d. Appendix cache — reads the validate cache and verifies every non-exempt candidate appendix file is listed in the validate cache's file list. If any appendix is missing, rejects promote with guidance to re-run `validate@{unit}`.
   e. All four checks pass → format validation (frontmatter, required fields, and ref target check — `unit_refs`/`rule_refs` pointing only to candidate-layer files are rejected with "promote it first" guidance, since the referenced unit/rule must already be stable) + copy candidate files to stable + remove candidate files.
3. The CLI automatically:
   - Transforms the `layer` frontmatter field from `candidate` to `stable`
   - Appendix filenames are preserved since they no longer encode layer
   - Deletes candidate cache files after success

**PASS:** `specflowctl promote --unit <name>` exits with code 0, all files copied and candidate cleaned up
**FAIL:** CLI returns non-zero exit or reports cache stale — report the CLI output and recommend re-running the appropriate validation step

---


---

## Truth Semantics

Promote is the act of recording a reconciled design as authoritative truth. After promote, the candidate is removed and the stable spec becomes the sole recorded reference (level 3 — prior consensus in the Truth Hierarchy). The old stable is superseded (git history preserves it). Candidate-layer files are removed after promote — this keeps file existence as an unambiguous state signal. To start a new editing round, see the fork prerequisite in concepts.md §2 (Edit and implement).
