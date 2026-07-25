# Promote Workflow

## Overview

When an agent executes `spec_promote {unit}`, it follows the 2 steps defined in this file. This file is referenced by `framework/concepts.md` §3 — the agent reads this file at promote time, not proactively.

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
2. specflowctl promote: PASS | FAIL — reason
Summary: ...
```

---

## Step 1 — Agent pre-check (optional)

**Purpose:** Optionally check cache freshness and review non-runnable items before promote. Cache check is redundant with the CLI's own enforcement but provides transparency. The non-runnable review catches items that may have become runnable since the last verify cycle.

**Execution steps:**

1. Read `docs/specs/meta/validation/unit/{name}/validate_result.md` if it exists
2. Read `docs/specs/meta/validation/unit/{name}/verify_result.md` if it exists
3. Report freshness status to the user if reporting would be useful
4. **Non-runnable review:**
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

## Step 2 — Run specflowctl promote

**Purpose:** The CLI performs the mechanical candidate-to-stable transition. This is the only gate that writes to stable.

**Execution steps:**

1. Run `specflowctl promote --unit <name>` from the repository root
2. The CLI independently checks:
   a. Validate cache — reads `docs/specs/meta/validation/unit/{name}/validate_result.md`. If missing or stale (hash mismatch), rejects promote with guidance to re-run `spec_validate`.
   b. Verify cache — reads `docs/specs/meta/validation/unit/{name}/verify_result.md`. If missing or stale, rejects promote with guidance to re-run `spec_verify`.
   c. Review cache — reads `docs/specs/meta/validation/unit/{name}/review_result.md`. If present and `blocking: true` (P0/P1 findings exist), rejects promote with guidance: "Review found P0/P1 finding(s). Resolve before promoting." If missing, stale, or non-blocking, does not block promote.
   d. All three checks pass → format validation (frontmatter, required fields) + copy candidate files to stable + remove candidate files.
3. The CLI automatically:
   - Transforms the `layer` frontmatter field from `candidate` to `stable`
   - Appendix filenames are preserved since they no longer encode layer
   - Deletes candidate cache files after success

**PASS:** `specflowctl promote --unit <name>` exits with code 0, all files copied and candidate cleaned up
**FAIL:** CLI returns non-zero exit or reports cache stale — report the CLI output and recommend re-running the appropriate validation step

---


---

## Truth Semantics

Promote is the act of recording a reconciled design as authoritative truth. After promote, the stable spec becomes the new level-2 truth. The old stable is superseded (git history preserves it). Candidate-layer files are removed after promote — this keeps file existence as an unambiguous state signal. To start a new editing round, see the fork prerequisite in concepts.md §2 (Edit and implement).
