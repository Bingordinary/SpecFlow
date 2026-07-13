# Promote Workflow

## Overview

When an agent executes `spec_promote {unit}`, it follows the 3 steps defined in this file. This file is referenced by `framework/concepts.md` §3 — the agent reads this file at promote time, not proactively.

## Execution Rules

- **Subagent permissions before promote:** may inspect file content. Must NOT modify files before `specflowctl promote` runs.
- **Subagent permissions after promote:** may inspect file content, modify the promoted stable spec directly for Step 3 cleanup only (candidate-layer reference normalization). Must NOT change behavioral content, acceptance criteria, or structural truth.
- Each step reports **PASS** or **FAIL** with a reason.
- Resolution types:
  - **fix_required** — A concrete repair can be made.
  - **blocked** — Requires user input (unclear intent, external dependency). Stop and ask.
- Post-promote stable spec modifications (Step 3) are explicitly authorized here. They are not a bypass of the promote gate — they occur after promote has already completed and only normalize references that would otherwise be dangling.

## Output Format

```
Promote result: PASS | FAIL
1. Agent pre-check (optional): PASS | FAIL — reason
2. specflowctl promote: PASS | FAIL — reason
3. Body reference cleanup: PASS | SKIP — reason
Summary: ...
```

---

## Step 1 — Agent pre-check (optional)

**Purpose:** Optionally check cache freshness and review `not_runnable_yet` items before promote. Cache check is redundant with the CLI's own enforcement but provides transparency. The `not_runnable_yet` review catches items that may have become runnable since the last verify cycle.

**Execution steps:**

1. Read `docs/specs/_validation/unit/{name}/validate_result.md` if it exists
2. Read `docs/specs/_validation/unit/{name}/verify_result.md` if it exists
3. Report freshness status to the user if reporting would be useful
4. **not_runnable_yet review:**
   - Read the candidate spec at `docs/specs/units/candidate/c_unit_{name}.md`
   - Scan acceptance items for `not_runnable_yet: yes`
   - For each item found, assess:
     - Is `not_runnable_yet_reason` present and substantive?
     - Is there a credible path or timeline for flipping to `no`?
     - If the same item was already `not_runnable_yet` in the stable predecessor (check git history for the previous stable spec), flag it as a concern — it has persisted across promote cycles
   - Report findings to the user

**PASS:** Freshness information reported; no unresolved not_runnable_yet concerns (or step skipped)
**FAIL:** Not applicable — this step is optional and cannot fail

**Quality concern:** One or more not_runnable_yet items persist from the previous stable spec; user attention recommended before promote

---

## Step 2 — Run specflowctl promote

**Purpose:** The CLI performs the mechanical candidate-to-stable transition. This is the only gate that writes to stable.

**Execution steps:**

1. Run `specflowctl promote --unit <name>` from the repository root
2. The CLI independently checks:
   a. Validate cache — reads `docs/specs/_validation/unit/{name}/validate_result.md`. If missing or stale (hash mismatch), rejects promote with guidance to re-run `spec_validate`.
   b. Verify cache — reads `docs/specs/_validation/unit/{name}/verify_result.md`. If missing or stale, rejects promote with guidance to re-run `spec_verify`.
   c. Both fresh → format validation (frontmatter, required fields) + copy candidate files to stable + remove candidate files.
3. The CLI automatically:
   - Transforms the `layer` frontmatter field from `candidate` to `stable`
   - Renames appendix files from `c_unit_` prefix to `s_unit_` prefix
   - Deletes candidate cache files after success

**PASS:** `specflowctl promote --unit <name>` exits with code 0, all files copied and candidate cleaned up
**FAIL:** CLI returns non-zero exit or reports cache stale — report the CLI output and recommend re-running the appropriate validation step

---

## Step 3 — Post-promote body reference cleanup

**Purpose:** The promote CLI mechanically transforms filenames and frontmatter but cannot safely rewrite body text (distinguishing file references from prose requires semantic judgment). This step closes that gap by having the agent review and normalize candidate-layer file references in the promoted stable spec.

**Why this exists:** The promote CLI renames appendix files (`c_unit_`→`s_unit_`) and updates the `layer` frontmatter field, but body content is copied verbatim. If the candidate spec contained literal file references such as `c_unit_auth` or `docs/specs/units/candidate/`, those references become dangling in the stable layer. A blind mechanical `c_unit_`→`s_unit_` replacement in body text would be unsafe because the same string pattern may appear in prose that is not a file reference. The agent must use semantic judgment to distinguish the two cases.

**Execution steps:**

1. Read the newly created stable spec at `docs/specs/units/stable/s_unit_{name}.md`
2. Scan body text for candidate-layer reference patterns:
   - `c_unit_` prefix (e.g. `c_unit_auth`, `c_unit_foo_config`)
   - `docs/specs/units/candidate/` path prefix
   - `docs/specs/rules/candidate/` path prefix
3. For each match, determine whether it is a **file reference** or **prose content**:
   - File reference: refers to a unit, rule, or appendix file that was renamed from `c_` prefix to `s_` prefix during promote. These should be updated to the stable equivalent.
   - Prose content: uses the `c_unit_` or `candidate` pattern as prose (e.g. "the c_unit_ naming convention", "candidate approaches were evaluated"). These must remain unchanged.
4. If any file references are found, update them in the stable spec:
   - `c_unit_` → `s_unit_` for unit/rule/appendix file references
   - `docs/specs/units/candidate/` → `docs/specs/units/stable/` for path references
   - `docs/specs/rules/candidate/` → `docs/specs/rules/stable/` for path references
5. No re-validate or re-verify is required — behavioral content has not changed.

**PASS:** No dangling candidate-layer file references found, or all found references have been updated
**SKIP:** No candidate-layer references found in body text

---

## Truth Semantics

Promote is the act of recording a reconciled design as authoritative truth. After promote, the stable spec becomes the new level-2 truth. The old stable is superseded (git history preserves it). Candidate-layer files are removed after promote — this keeps file existence as an unambiguous state signal. To start a new editing round, see the fork prerequisite in concepts.md §2 (Edit and implement).
