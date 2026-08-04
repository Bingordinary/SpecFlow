# SpecFlow Concepts

This project uses SpecFlow to manage design documents. SpecFlow maintains spec documents that record accepted design, behavior, boundaries, and shared rules. These documents serve as the consensus protocol between the user and the agent — the user reviews spec documents to confirm intent, and the agent reads spec documents to understand design intent across sessions.

## Spec Workflow

Every spec document follows a five-step loop:

**Fork → Edit → Validate → Verify → Promote**

| Step | What happens | Entry condition | Exit condition |
|-------|-------------|----------------|---------------|
| **Fork** | Run `specflowctl fork --unit <name>` to copy stable spec + appendices to candidate layer (or create from scratch). | User wants to design or modify a unit. | Candidate file exists at `unit_<name>.md`. |
| **Edit** | Modify the candidate spec and corresponding code. | Candidate exists (forked or created). | Agent and user agree the design is ready for review. |
| **Validate** | Check candidate design quality against checklist. Appendix content is part of the unit's design and included in all checks. | Agent triggered by `validate`. | All checks pass. |
| **Verify** | Check candidate vs code alignment. Appendix content is verified against code alongside the main spec. | Agent triggered by `verify`. | All items aligned. On mismatch, user decides reconciliation direction. |
| **Promote** | Replace stable with the validated+verified candidate. Checks that all appendix files were included in validation. | User confirms `promote`. | New stable spec at `unit_<name>.md`; candidate files removed. |

After promote, the new stable becomes the fork source for the next iteration — the cycle repeats.

### State by File Existence

There is no status metadata in the files. The file system itself is the state signal:

| Stable exists | Candidate exists | What it means | What the agent does |
|:---:|:---:|---|---|
| No | No | No design recorded for this unit. | Create `unit_<name>.md` from scratch. |
| Yes | No | Accepted design exists, no changes in progress. | Fork from stable (copy to candidate layer) before editing. |
| Yes | Yes | Changes are in progress. | Edit the existing candidate; stable is unchanged until promote. |
| No | Yes | *(transient — promotes only from candidate to stable, so stable always ends up existing)* | Proceed to promote when ready. |

### The Two Layers

| Layer | Prefix | Directory | Purpose |
|-------|--------|-----------|---------|
| **Stable** | `unit_<name>.md` | `docs/specs/units/stable/` | Accepted recorded truth. **Never edit directly.** Only created by promote. |
| **Candidate** | `unit_<name>.md` | `docs/specs/units/candidate/` | Working draft. Created by fork or from scratch. The only layer the agent edits. |

The relationship is versioned: a candidate is always a proposed next version of its corresponding stable (or the first version if no stable exists).

## Core Principle

**File existence is state.** No state machine, no status table, no lifecycle phases. Candidate file exists = being edited. No candidate file = not being edited.

| Directory | Meaning |
|-----------|---------|
| `docs/specs/units/stable/` | Accepted, promoted design truth |
| `docs/specs/units/candidate/` | Design currently being edited |
| `docs/specs/rules/stable/` | Accepted shared rules |
| `docs/specs/rules/candidate/` | Rules being edited |
| `docs/specs/meta/validation/` | Validate/verify cache files at `docs/specs/meta/validation/unit/{name}/validate_result.md` and `docs/specs/meta/validation/unit/{name}/verify_result.md` (unit); `docs/specs/meta/validation/rule/{id}/validate_result.md` (rule). See `framework/validation_cache.md` for lifecycle details. |

### Truth Hierarchy

When code, stable spec, and candidate spec disagree, their authority is not equal:

| Level | Source | Status |
|-------|--------|--------|
| 1 — Ground truth | Running code | What the system actually does |
| 2 — Current design intent | Candidate spec | What the current iteration proposes |
| 3 — Prior consensus | Stable spec | What was previously accepted (superseded by candidate during active work) |

**Candidate is the current design intent but not automatically correct.** When verify finds a mismatch between candidate and code, the user decides which direction to reconcile. Stable records the prior consensus for reference.

### Spec Reference Priority (Outside Verify)

When referencing spec content in discussion, analysis, or implementation reasoning outside the `verify` workflow, the file system state determines the reference semantics. The state is the same signal used in the [State by File Existence](#state-by-file-existence) table:

| Stable | Candidate | What it means | How to reference |
|:---:|:---:|---|---|
| Yes | No | Accepted design, no active changes. | Reference as recorded truth. Layer qualifier optional (only one layer exists). |
| Yes | Yes | Active development. Candidate is current design intent; stable is prior consensus. | **Must name the layer.** Stable = "accepted spec records..." Candidate = "current draft proposes..." Candidate is a working hypothesis — combine with code to determine truth. Do not use bare "spec says" when both exist. |
| No | Yes | New design, no prior consensus. | Reference as a working draft. Label content as unverified. |
| No | No | No design recorded. | No spec to reference. Code is the only truth source. |

## Key Terms

- **unit** — One independently governed engineering responsibility
- **rule** — A reusable shared constraint that multiple units may follow
- **stable** — Prior consensus. Superseded by candidate during active work.
- **candidate** — Current design intent. Working draft, not automatically correct.

### Automatic Target Type Detection

`validate`, `verify`, and `promote` work for both units and rules.
The agent automatically detects the target type using a three-stage process.

**Stage 1 — Physical file check.** Must complete both steps before deciding.

Step 1: Glob for unit candidate files — run `Glob "docs/specs/units/candidate/unit_{target}.md"`.

Step 2: Glob for rule candidate files — run `Glob "docs/specs/rules/candidate/g_rule_{target}.md"` and `Glob "docs/specs/rules/candidate/b_rule_{target}.md"` (both `g_rule_` and `b_rule_` prefixes).

**Decision (use after both steps complete):**

| Unit candidate found | Rule candidate found | Result |
|:---:|:---:|---|
| Yes | No | Type = Unit |
| No | Yes | Type = Rule. Resolve full `rule_id` from matched filename (e.g., matched `b_rule_runtime_model.md` → `rule_id = b_rule_runtime_model`). |
| Yes | Yes | Ambiguous. List found files and ask user to clarify. |
| No | No | → Stage 2 |

**Stage 2 — Prefix fallback (new target).** Only reach here if neither unit nor rule files were found in Stage 1:

| Target format | Detected as | Example |
|---------------|-------------|---------|
| Name without prefix (`auth`, `user_service`) | Unit | `validate@auth` |
| Name with `g_rule_` or `b_rule_` prefix | Rule | `validate@b_rule_auth` |

**Stage 3 — Rule directory fallback.** Only reach here when Stage 2 detected "Unit" but no unit files exist for the target.

When Stage 2 classifies a no-prefix target as Unit, the agent looks for its unit files (`Glob "docs/specs/units/candidate/unit_{target}.md"` and `Glob "docs/specs/units/stable/unit_{target}.md"`). If **neither** candidate nor stable unit files exist, do not report "does not exist" yet. Instead:

Run `Glob "docs/specs/rules/candidate/*.md"` and `Glob "docs/specs/rules/stable/*.md"`. Scan all filenames for one whose name contains the target (e.g., `b_rule_runtime_model.md` contains `runtime_model`).

| Result | Action |
|---|---|
| Matching rule file found | Type = Rule. Resolve full `rule_id` from the matched filename (e.g., `b_rule_runtime_model.md` → `rule_id = b_rule_runtime_model`). Proceed with the rule pipeline. |
| No matching rule file found | Report: target does not exist. |

Unit and rule follow the validate→promote pipeline, but each has different internal checks:

| Step | Unit executes | Rule executes |
|-------|--------------|--------------|
| validate | 8-point unit design checklist (`unit_validate_checklist.md`) | 8-point rule metadata & body quality checklist (`rule_validate_checklist.md`) |
| verify | 7-step spec-vs-code alignment check (`unit_verify_checklist.md`) | Removed — rule does not need verify. Consumer alignment is the consuming unit's responsibility. Impact analysis (consumer discovery) is a separate mechanism. |
| promote | candidate→stable archive (`unit_promote_workflow.md`) | version promotion + consumer ref migration + body ref cleanup (`rule_promote_workflow.md`) |

## specflowctl Location

==ATOM_BEGIN:specflowctl_location==
specflowctl is not on PATH. Its binary is at `<tooling-root>/bin/specflowctl-<os>-<arch>`. `<tooling-root>` is `specflow/tooling`. Replace `<os>` and `<arch>` with your platform (e.g. `linux-amd64`, `darwin-arm64`, `windows-amd64.exe`). Use the full path when running specflowctl commands.
==ATOM_END:specflowctl_location==

## Framework Path

==ATOM_BEGIN:framework_path==
Framework documentation files are referenced with the `framework/` prefix (e.g. `framework/operations/update.md`). These files are located at `specflow/framework/`.
==ATOM_END:framework_path==

## Workflow

### 1. Discover

Run `specflowctl next --unit <name>` to discover the unit's candidate and stable spec files, appendices, rules, and related units.

### 2. Edit and implement (default mode)

**Fork prerequisite — before editing, determine the candidate state:**
- **Candidate exists** → edit the existing candidate spec directly.
- **No candidate, stable exists** → **fork from stable:** run `specflowctl fork --unit <name>` from the repository root. This copies the unit spec and all associated appendix files to the candidate layer, updates frontmatter (`layer: stable → candidate`, increments version), and reports the complete fork manifest. This is the **only** allowed way to fork. Manual `cp` is not permitted.
- **No candidate, no stable** → brand-new design. Create `unit_<name>.md` from scratch following `framework/spec_writing_guide.md` or reference existing specs for format. No fork step needed.

**Rule fork — same logic applies to rules:** if a stable rule file exists at `docs/specs/rules/stable/{rule_id}.md` and no candidate exists, run `specflowctl fork --rule <rule_id>` to fork from stable. This updates `layer` from `stable` to `candidate` and increments `rule_version`. For brand-new rules (no stable file), create the candidate rule file from scratch following `framework/spec_writing_guide.md` §5.

After the fork (if applicable), update the candidate spec and code. No gate before editing. Read first, then write.

### 3. Validate, verify, promote (triggered by user)

The user can use explicit triggers at any time:

| Trigger | What agent does |
|---------|-----------------|
| `validate@{target}` | Read-only subagent. Unit: 8-point validate checklist (`unit_validate_checklist.md`). Rule: 8-point rule metadata & body quality checklist (`rule_validate_checklist.md`). Auto-detects type from target name. Default: full (always runs all checks + cross-check — quality checks are holistic). Add `:check-{n}` or `:{keyword}` for specific check. See `framework/verification_scope.md`. |
| `verify@{target}` | Read-only subagent. Unit only: 7-step spec-vs-code verify checklist (`unit_verify_checklist.md`), `:full` for all 7 steps + cross-check. If target is a Rule, report: "Rule verify has been removed. Run `validate@{rule}` instead." Default: scoped (git-aware). Add `:{keyword}` for specific content. See `framework/verification_scope.md`. |
| `promote@{target}` | 3-step promote workflow (unit) / 2-step promote workflow (rule). Unit: candidate→stable archive (`unit_promote_workflow.md`). Rule: version promotion + consumer ref migration + body ref cleanup (`rule_promote_workflow.md`). Auto-detects type from target name. |

**Cache lifecycle:** See `framework/validation_cache.md`.

**Recovery patterns:** See `framework/recovery_patterns.md`.

If the user declines a suggestion, continue editing. Do not insist.

### 3.0. Agent suggestion rules

**Default mode: assume editing.** When the user's request does not match an explicit trigger (`validate`, `verify`, `promote`), the agent MUST assume the user is editing or iterating. Do not classify intent. Do not suggest spec operations. Do not disclose cache state.

The agent only reacts to these concrete signals:

| User signal | What the user likely wants | Agent action |
|-------------|---------------------------|-------------|
| **design**: "I want to design X", "let's design X", "I need a design for X" | Create or update the candidate spec | Default mode: assume editing. If no candidate exists, offer to start one. If candidate exists, begin editing. Do not suggest spec operations. |
| **quality check**: "check this", "is it right?", "review the design", "validate", "verify" | Validate or verify the spec against the code | Clarify: "Did you mean **validate** (check design quality) or **verify** (verify implementation)?" Then disclose relevant cache state (see disclosure table below). |
| **code review**: "review the code", "code review", "review quality" | Review code quality with spec awareness | Route to `review`. Disclose review cache state. Review must pass before promote. |
| **completion**: "it's done", "lock it in", "finalize", "promote this", "wrap it up", "ship it" | Promote the candidate to stable | Candidate exists → check validate+verify+review cache. All three fresh, full mode, and non-blocking (validate PASS, verify PASS, review no P0/P1) → suggest `promote`. Any cache missing/stale/scoped → "Pre-promote checks are not complete yet. I need to run those first. Shall I?" If review cache has P0/P1 findings, disclose: "Review found P0/P1 finding(s) — resolve before promoting." |
| **stuck**: "something is wrong", "it's broken", "I'm stuck" | Diagnose and recover | Diagnose first: is it a code bug, design flaw, or external blocker? See `framework/recovery_patterns.md`. |

If none of these signals are present, continue with the conversation without suggesting spec operations.

**State disclosure (only use when triggered by a quality check or completion signal):**

Before suggesting any action, communicate the current file state using concrete, user-understandable language. Never ask vague questions like "Do you want to go through the validate-verify-promote process?". Instead, disclose the state first, then state what is available for the user to choose from.

| State to disclose | What to say |
|-------------------|-------------|
| Candidate spec exists for the unit | "A candidate spec (`...`) exists, recording the design you are currently editing" |
| No candidate spec for the unit | "No candidate spec exists, meaning no design has been recorded yet" |
| Validate cache fresh (full) | "Validate has passed all checks, and the read files have not changed" |
| Validate cache fresh (scoped) | "Validate has passed for check-{n} (user requested scoped), but other checks are not yet verified" |
| Validate cache missing/stale | "Validate cache does not exist or is expired, needs re-checking" |
| Verify cache fresh (full) | "Verify has passed for all items, and the checked files have not changed" |
| Verify cache fresh (scoped) | "Verify has passed for item {id}, but other items are not yet verified" |
| Verify cache missing/stale | "Verify cache does not exist or is expired, needs re-checking" |
| Review cache fresh (full) | "Review has passed — no P0 or P1 findings" |
| Review cache fresh (scoped) | "Review has passed for changed files — no P0 or P1 findings in the diff" |
| Review cache with P0/P1 findings | "Review found {N} P0/P1 finding(s) — review blocks promote until resolved" |
| Review cache missing/stale | "Review cache does not exist or is expired" |
| Appendix validate check pass | "All appendix files are included in validation" |
| Appendix validate check fail | "One or more appendix files were not validated — run `validate@{unit}:full` before promoting" |

When scoped cache exists, the agent should mention: "This is not a full check — run `:full` for complete verification before promoting."

**State transition lookup table (use when the user has triggered a quality check or completion signal):**

This table maps the boolean state dimensions to the exact disclosure text and suggested action. `N/A` means the cache state is irrelevant. When `candidate_exists` is N, all cache dimensions are N/A. When `candidate_exists` is Y and `validate_fresh` is N, `verify_fresh` is N/A because verify is not actionable until validate passes.

Review cache is required for promote: when the review cache is missing, stale, scoped, or `blocking: true`, the agent must disclose the gap before promote and advise running `review@{unit}:full`.

The scope mode (scoped vs full) affects disclosure wording. If the fresh cache is `mode: full`, use "all checks" / "all items" language. If scoped, specify which check or item was verified. Only `mode: full` caches satisfy the promote gate — scoped results are for iterative feedback only, and the disclosure text should reflect that.

| candidate_exists | validate_fresh | verify_fresh | Disclose | Then offer |
|---|---|---|---|---|
| N | N/A | N/A | "No candidate spec exists" | "You can start writing a candidate spec to record your design." |
| Y | N | N/A | "Candidate exists, but validate has not passed or the cache is expired." | "You can continue updating the candidate, or run validate for a design quality check." |
| Y | Y (full) | N | "Candidate exists, validate has passed all checks, but verify has not been done or the cache is expired." | "You can continue updating the candidate, or run verify to check if the implementation matches the design." |
| Y | Y (scoped) | N | "Candidate exists, validate has passed for check-{n} (user requested scoped), but other checks and verify are not done." | "Run `validate@{unit}:full` for complete validation before verifying." |
| Y | N | Y (full) | "Candidate exists, verify has passed all items, but the validate cache is expired." | "It is recommended to re-run validate because the design may have changed." |
| Y | N | Y (scoped) | "Candidate exists, verify has passed for item {id}, but other items are not verified and validate cache is expired." | "It is recommended to re-run validate and run complete verify." |
| Y | Y (scoped) | Y (scoped) | "Candidate has scoped results only — validate passed check-{n} (scoped) and verify passed item {id} (scoped)." | "Run `validate@{unit}:full` and `verify@{unit}:full` for complete pre-promote checks." |
| Y | Y (full) | Y (full) | "Candidate has passed validate (all checks) and verify (all items)." | "If review cache is missing, stale, scoped, or has P0/P1 findings, advise running `review@{unit}:full` before promote. Otherwise: 'If the design is finalized, you can promote it to stable.'" |
| Y | Y (full) | Y (scoped) | "Candidate has full validate and scoped verify (item {id} only)." | "Run `verify@{unit}:full` for complete verification before promoting." |
| Y | Y (scoped) | Y (full) | "Candidate has full verify and scoped validate (check-{n} only)." | "Run `validate@{unit}:full` for complete validation before promoting." |

**Example conversations:**

> User: "I want to design user authentication" (design signal)
> Agent: "OK. There is currently no candidate spec for the user_auth unit, meaning no design has been recorded yet. Here's what I can do: 1. Use guidance to walk through the design, then write it to the candidate. 2. If you already have an idea, I can write the candidate directly. Which do you prefer?"

> User: "Check it" (quality check signal)
> Agent: "A candidate spec for user_auth already exists, but the validate cache has expired. I can: 1. Run validate — check the candidate spec's design quality. 2. Run verify — check if the code implementation matches the design. 3. Continue editing the candidate. When you say 'check it', do you mean checking the design (validate) or checking the implementation (verify)?"

> User: "It's done, promote this" (completion signal)
> Agent: "Candidate exists, validate has passed all checks, verify has passed all items, and review has no P0/P1 findings. Ready to promote. Running `promote@user_auth`..."

**RED FLAGS — common agent mistakes**

| Agent thought | Reality |
|---------------|---------|
| "The user finished this change, they might want to check it" | Unless the user explicitly says "check" or "done", assume they are still iterating. Do not proactively suggest. |
| "Let me give the user an option: verify+promote first, then handle remaining issues" | Remaining issues = still iterating. Do not mention promote during iteration. |
| "Proactively disclosing cache status helps the user track progress" | When the user hasn't asked, cache status is noise. Only disclose on quality check or completion signals. |
| "Suggesting validate/verify/promote helps the user maintain quality" | This interrupts the user's flow. They will ask when needed. |
| "Let me ask if the user wants to finalize" | Do not ask abstract category questions. Detect concrete signals. |
| "This example is ambiguous, safer to fall back to category questions" | Falling back to abstract categories is the last resort. Check for signals first. If still uncertain, ask a concrete question instead of category questions. |
| "Fixes are applied, I should re-run validate/verify/review to confirm and restore the cache" | Executing quality-gate commands is user-triggered only (HARD RULE 2). After fixes, at most suggest a scoped re-check (`validate@{unit}:check-{n}`, `verify@{unit}:{keyword}`) and wait for the user's decision. Cache expiry during iteration is normal — do not restore it on your own initiative. |

### 4. Promote (only gate)

The detailed promote workflow is defined in `framework/unit_promote_workflow.md` (unit) and `framework/rule_promote_workflow.md` (rule). The unit workflow follows 3 steps: optional agent pre-check, body path check, and `specflowctl promote`. The rule workflow follows 2 steps: optional agent pre-check and `specflowctl promote`.

Key rules that override the checklist:

1. `specflowctl promote --unit <name>` and `specflowctl promote --rule <id>` are the only operations that write to stable.
2. Unit promote: the CLI independently checks cache freshness (validate, verify, review, and appendix) before promoting. Rule promote: the CLI independently validates rule frontmatter and version.
3. After promote succeeds, the agent must NOT modify the promoted stable spec. Body text references are maintained as-is because they use concept names (e.g. `auth`) rather than layer-prefixed file names. Body text must never contain layer-prefixed spec paths (`candidate/`, `stable/`, or their `docs/specs/...` absolute forms): candidate paths break after promote (candidate files are deleted), and stable paths point to the prior-consensus layer during an active candidate round. `validate` enforces this at validate time: the validate@ checklist's Check 1 step 10 covers both candidate- and stable-layer forms; `specflowctl validate` Check 7 mechanically rejects candidate-layer forms only — stable-layer spec paths are legal in structured fields, so the mechanical check intentionally leaves stable forms to the checklist.

`specflowctl promote --unit <name>` validates format (frontmatter, required fields), checks appendix validation coverage (every non-exempt appendix must be in the validate cache's file list), and copies candidate files to stable. Reference integrity is checked by `validate` before promote runs. `specflowctl promote` additionally rejects `unit_refs`/`rule_refs` that point only to candidate-layer files. During the copy, the tool automatically updates each file's frontmatter `layer` field from `candidate` to `stable`. Appendix filenames are preserved since they no longer encode layer. After promote succeeds, candidate cache files are deleted.

`specflowctl promote --rule <id>` validates rule frontmatter, copies the candidate rule to stable (with layer transform), then deletes the candidate rule file. Consumer impact assessment is the agent's responsibility.

**Truth semantics:** Promote is the act of recording a reconciled design as authoritative truth. After promote, the candidate is removed and the stable spec becomes the sole recorded reference (level 3 — prior consensus). The level-2 position (current design intent) is vacant because no candidate file exists; it will be recreated when someone forks to start a new editing round. The old stable is superseded (git history preserves it). Candidate-layer files are removed after promote — this keeps file existence as an unambiguous state signal. To start a new editing round, see §2 (Edit and implement) for the fork procedure. Before promoting, the CLI verifies that every non-exempt appendix file is listed in the validate cache's file list — ensuring all appendix content was checked. See [Truth Hierarchy](#truth-hierarchy).

### 5. Spec Review (required quality gate, before promote)

`review` is a standalone spec-aware code quality review. It is NOT part of `verify` — the user triggers it independently. `review` inspects the code with the spec's design intent as context and suppresses findings that the spec explains.

The review result is cached independently. P0/P1 findings block promote regardless of mode; only `:full` mode cache satisfies the promote gate. See `framework/spec_review_checklist.md` and `framework/verification_scope.md` for detail.

## HARD RULES

These override default helpful-assistant behavior. They are not suggestions.

**HARD RULE 1: Read Specs Before Discussing or Changing a Topic**
Before discussing, analyzing, or modifying any topic related to a unit, first read the unit's stable spec (if it exists) and the candidate spec (if it exists). If both exist, read both — understand that stable records prior consensus and candidate is the current design intent. Their authority differs per the Truth Hierarchy and the [Spec Reference Priority](#spec-reference-priority-outside-verify) table. When summarizing spec content to the user, and both layers exist, name which layer you are quoting. If the spec has no relevant coverage on the topic, state so explicitly before starting new work: "The spec currently has no recorded design content on this topic. We can start designing from scratch." Create or update spec when design changes. If no spec exists for the unit, create one. Read `framework/spec_writing_guide.md` or reference existing specs for format.

**HARD RULE 2: Promote Is the Only Gate to Stable**
Never call `specflowctl promote` without user confirmation. Before promote, always run validate, verify, and review. If any fails, stop and report. The agent does not decide when to validate, verify, or promote — it suggests, the user confirms. This includes re-runs after a fix: the agent must not re-run validate, verify, or review on its own initiative to confirm a fix or restore cache freshness. After applying fixes, the agent only suggests a scoped re-check and waits for the user to trigger it.

validate, verify, and review are quality gates. They write cache files (`meta/validation/`) but never spec or stable files. For verify: only P0/P1 findings count as a gate failure (verify FAIL) — P2/P3 findings are non-blocking (verify PASS with pending items) and do not stop promote. For validate and review: any failure stops promote.

**HARD RULE 3: validate, verify, and review Check Quality, Promote Writes**
`validate` and `verify` check quality and report findings. They are read-only for spec and stable files — they write cache files (`meta/validation/`) but never modify governance truth or advance governance state. Only `promote` writes to stable. Commands like `next`, `doctor`, `init` are for discovery and maintenance and do not check quality.

**HARD RULE 3a: Suggest But Never Decide Divergence Resolution**
When `verify` reports findings (FAIL with P0/P1 findings, or PASS with pending P2/P3 items), the agent MUST present the findings to the user and wait for a decision. Before presenting, the agent MUST run the first-principles divergence analysis (see `unit_verify_checklist.md` Step 7), which launches a sub-agent per mismatch to analyze spec intent vs code intent using first-principles reasoning. The agent MUST NOT silently choose a direction, proceed to promote, or treat candidate as automatically correct — the suggestion is advisory only, the user decides. Batch grouping (see `unit_verify_checklist.md` §Batch classification, `unit_validate_checklist.md` §Batch classification, and `spec_review_checklist.md` §Batch classification) is a presentation mechanism, not an action authorization: findings remain listed per item, and no fix is applied until the user explicitly agrees to the batch group.

**HARD RULE 4: Stop When Unclear**
Stop and ask when the target unit is unclear, the required spec or framework file cannot be found, or the next workflow step cannot be determined. Do not guess or proceed with incomplete information.

**HARD RULE 5: Fork Must Use `specflowctl fork`**
All fork operations (stable → candidate) must use `specflowctl fork --unit <name>` or `specflowctl fork --rule <id>`. Manual `cp` of spec files is not permitted. This ensures appendix files are not missed and frontmatter is updated consistently.

## Commands Reference

| Command | What it does | Who calls it |
|---------|-------------|-------------|
| `specflowctl fork --unit <name>` | Copy stable unit spec + appendices to candidate layer with layer transform and version bump. Rejects if candidate already exists or stable does not exist. | Agent (as fork prerequisite) |
| `specflowctl fork --rule <id>` | Copy stable rule to candidate layer with layer transform and version bump. Rejects if candidate already exists or stable does not exist. | Agent (as fork prerequisite) |
| `specflowctl next --unit <name>` | Discover unit files and dependencies. Fails if unit is not found or tool errors. | Agent |
| `specflowctl promote --unit <name>` | Checks validate+verify+review+appendix cache freshness, validates format + copies candidate→stable. Rejects if any cache stale, missing, scoped, or blocking. Also rejects if any non-exempt appendix file is missing from the validate cache. | Agent (after user confirmation, after validate+verify+review) |
| `specflowctl promote --rule <id>` | Checks rule validate cache freshness, validates rule frontmatter, copies candidate rule→stable, deletes candidate. Rejects if the cache is missing, stale, or scoped. Consumer impact assessment is the agent's responsibility. See `framework/spec_writing_guide.md` §5. | Agent or human maintainer |
| `specflowctl review run-*` | Governance review run-state management. Subcommands: `run-init` (create/reuse run-state), `run-validate` (validate run-state shape), `run-refresh` (recompute fingerprints, mark stale), `run-touch` (update timestamp). See `framework/spec_flow_review.md` §6. | Deep audit executor |
| `validate@{target}` (agent trigger) | Read-only subagent. Unit: 8-point checklist (`unit_validate_checklist.md`). Rule: 8-point rule metadata & body quality checklist (`rule_validate_checklist.md`). Auto-detects type from target name. Default: full (always runs all checks + cross-check — quality checks are holistic). Add `:check-{n}` or `:{keyword}` for specific check. See `framework/verification_scope.md`. Writes cache on PASS. On FAIL: deletes cache, reports findings. Agent must stop and not proceed to promote. | User says "validate" or confirms agent suggestion |
| `verify@{target}` (agent trigger) | Read-only subagent. Unit only: 7-step spec-vs-code alignment (`unit_verify_checklist.md`), `:full` for all 7 steps + cross-check. If target is a Rule, report: "Rule verify has been removed. Run `validate@{rule}` for the rule instead." Default: scoped (git-aware). Add `:{keyword}` for specific content. See `framework/verification_scope.md`. Writes cache on PASS. On FAIL (P0/P1 findings): deletes cache, reports findings, must stop, not proceed to promote. All P2/P3 → full run writes cache with `result: pass` and severity counts (non-blocking, promote allowed); scoped run reports findings without writing a cache. | User says "verify" or confirms agent suggestion |
| `review@{target}` (agent trigger) | Read-only subagent. Spec-aware code quality review (`spec_review_checklist.md`). Reads the candidate spec for design context, then reviews code quality. Suppresses findings that the spec explains. P0/P1 block promote; P2/P3 advisory. Default: scoped (git-aware). `:full` for all unit code. Writes cache on completion with full findings body. On FAIL (subagent error, target not found, checklist missing): reports error, does not write cache. Agent must not proceed to promote. Review cache is required for promote — see `unit_promote_workflow.md`. See `framework/verification_scope.md`. | User says "review" or confirms agent suggestion |
| `promote@{target}` (agent trigger) | 3-step promote workflow (unit) / 2-step promote workflow (rule). Unit: archive (`unit_promote_workflow.md`). Rule: version promotion + consumer migration + body ref cleanup (`rule_promote_workflow.md`). Auto-detects type from target name. On FAIL: rejects if cache stale, format invalid, or copy fails. Reports CLI output. No files archived. Agent recommends re-running the failed quality gate (validate, verify, or review) as indicated by the failure report and waits for the user to trigger it before retrying. | User says "promote" or confirms agent suggestion |
| `specflowctl init` | Initialize specFlow project | Human |
| `specflowctl doctor` | Diagnose project setup | Human |
| `spec_flow_update` (agent trigger) | Full update: pull framework, detect format changes, migrate spec files, check document format. See `framework/operations/update.md` for full procedure. | User says "spec_flow_update" |
| `spec_flow_version` (agent trigger) | Check the installed SpecFlow version against the remote latest and report whether the project is up to date. If behind, recommend running `spec_flow_update`. See `framework/operations/version.md` for full procedure. | User says "spec_flow_version" |
| `specflowctl consumers --rule <id>` | List all units that reference the given rule in their rule_refs. For global rules (`g_rule_*`): returns every current-layer unit — global rules apply to all units by default and are not repeated in rule_refs. For bound rules (`b_rule_*`): empty output means no consumers. | Agent for impact analysis |
| `specflowctl validate` | Validate candidate spec structure (7 checks), rule validation (7 mechanical checks), or file write permissions | Human maintainer or agent |

Project truth inputs: `docs/specs/`.
