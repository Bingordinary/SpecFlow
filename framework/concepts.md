# SpecFlow Concepts

This project uses SpecFlow to manage design documents. SpecFlow maintains spec documents that record accepted design, behavior, boundaries, and shared rules. These documents serve as the consensus protocol between the user and the agent — the user reviews spec documents to confirm intent, and the agent reads spec documents to understand design intent across sessions.

## Core Principle

**File existence is state.** No state machine, no status table, no lifecycle phases. Candidate file exists = being edited. No candidate file = not being edited.

| Directory | Meaning |
|-----------|---------|
| `docs/specs/units/stable/` | Accepted, promoted design truth |
| `docs/specs/units/candidate/` | Design currently being edited |
| `docs/specs/rules/stable/` | Accepted shared rules |
| `docs/specs/rules/candidate/` | Rules being edited |
| `docs/specs/_validation/` | Validate/verify cache. See `framework/validation_cache.md`. |

### Truth Hierarchy

When code, stable spec, and candidate spec disagree, their authority is not equal:

| Level | Source | Status |
|-------|--------|--------|
| 1 — Ground truth | Running code | What the system actually does |
| 2 — Recorded agreement | Stable spec | What the system should do (promoted candidate) |
| 3 — Working draft | Candidate spec | Proposed evolution, **not truth** |

**Candidate is not automatically correct.** When verify finds a mismatch between candidate and code, the user decides which direction to reconcile. Only stable is the authoritative recorded truth.

## Key Terms

- **unit** — One independently governed engineering responsibility
- **rule** — A reusable shared constraint that multiple units may follow
- **stable** — Accepted current project truth. The authoritative recorded design.
- **candidate** — Proposed next project truth. A working draft, not truth on its own. Only stable is authoritative.

## specflowctl Location

==ATOM_BEGIN:specflowctl_location==
specflowctl is not on PATH. Its binary is at `specflow/tooling/bin/specflowctl-<os>-<arch>`. Replace `<os>` and `<arch>` with your platform (e.g. `linux-amd64`, `darwin-arm64`, `windows-amd64.exe`). Use the full path when running specflowctl commands.
==ATOM_END:specflowctl_location==

## Workflow

### 1. Discover

Run `specflowctl next --unit <name>` to discover the unit's candidate and stable spec files, appendices, rules, and related units.

### 2. Edit and implement (default mode)

Update the candidate spec and code. No gate before this step. Read first, then write.

### 3. Validate, verify, promote (triggered by user)

The user can use explicit triggers at any time:

| Trigger | What agent does |
|---------|-----------------|
| `spec_validate {unit}` | Read-only subagent with 9-point validate checklist. See `framework/validate_checklist.md`. |
| `spec_verify {unit}` | Read-only subagent with 6-step verify checklist. See `framework/verify_checklist.md`. |
| `spec_promote {unit}` | Runs validate then verify. If both pass: `specflowctl promote --unit {unit}`. If fails: stop, report. |

**Cache lifecycle:** See `framework/validation_cache.md`.

**Recovery patterns:** See `framework/recovery_patterns.md`.

If the user's language is vague ("check this", "see if it's right"), clarify:
"Did you mean **spec_validate** (check design) or **spec_verify** (verify implementation)?"

If the user declines a suggestion, continue editing. Do not insist.

### 3.0. Agent suggestion rules

When the user's request does not match an explicit trigger (`spec_validate`, `spec_verify`, `spec_promote`), classify the intent into one of the 5 categories below. Do not use keyword matching — infer from context.

**State disclosure first:** Before suggesting any action, communicate the current file state using concrete, user-understandable language. Never ask vague questions like "Do you want to go through the validate-verify-promote process?". Instead, disclose the state first, then state what is available for the user to choose from. See each intent's disclosure pattern below.

| State to disclose | What to say |
|-------------------|-------------|
| Candidate spec exists for the unit | "A candidate spec (`...`) exists, recording the design you are currently editing" |
| No candidate spec for the unit | "No candidate spec exists, meaning no design has been recorded yet" |
| Validate cache fresh | "Validate has passed, and the read files have not changed" |
| Validate cache missing/stale | "Validate cache does not exist or is expired, needs re-checking" |
| Verify cache fresh | "Verify has passed, and the checked files have not changed" |
| Verify cache missing/stale | "Verify cache does not exist or is expired, needs re-checking" |

**State transition lookup table:**

This table maps the three boolean state dimensions to the exact disclosure text and suggested action. `N/A` means the cache state is irrelevant. When `candidate_exists` is N, all cache dimensions are N/A. When `candidate_exists` is Y and `validate_fresh` is N, `verify_fresh` is N/A because verify is not actionable until validate passes.

| candidate_exists | validate_fresh | verify_fresh | Disclose | Then offer |
|---|---|---|---|---|
| N | N/A | N/A | "No candidate spec exists" | "You can start writing a candidate spec to record your design." |
| Y | N | N/A | "Candidate exists, but validate has not passed or the cache is expired." | "You can continue updating the candidate, or run validate for a design quality check." |
| Y | Y | N | "Candidate exists, validate has passed, but verify has not been done or the cache is expired." | "You can continue updating the candidate, or run verify to check if the implementation matches the design." |
| Y | N | Y | "Candidate exists, verify has passed, but the validate cache is expired." | "It is recommended to re-run validate because the design may have changed." |
| Y | Y | Y | "Candidate has passed both validate and verify." | "If the design is finalized, you can promote it to stable." |

| Intent | What user wants | Agent action | File state check |
|--------|----------------|-------------|-----------------|
| **designing** | Plan, change direction, explore approach | **Use the state transition lookup table above** to determine the correct disclosure and action for the current state. Never phrase it as a binary "do you want to go through the process". See the example conversations below for illustration. | Use the state transition lookup table above |
| **implementing** | Write code, iterate, debug, test | Do not touch spec. Do not suggest validate/verify/promote. Let the user focus. | Candidate exists → ensure it's read but do not interrupt. No candidate, changing stable behavior → suggest fork first |
| **verifying** | Check correctness, see if it's right | **Disclose cache state first** — if verify cache is fresh: "Verify cache is valid, all results from the last check were ALIGNED, and no files have changed. Need to re-run verify?" If stale or missing: "Verify cache does not exist or is expired, needs re-checking." Then run `spec_verify`. | Candidate exists → verify candidate vs code. Only stable → verify stable vs code |
| **finalizing** | Lock in, wrap up, promote | Check validate cache then verify cache. Both fresh → suggest `spec_promote`. Cache stale/missing → suggest re-running the appropriate step |
| **recovering** | Something is wrong, stuck, error | Diagnose first: is it a code bug (→ implementing), design flaw (→ designing), or external blocker (→ blocked, ask user) |

**Fallback:** If the intent is unclear after reasonable effort, ask the user directly:
"Are you designing something new, implementing code, checking your work, finalizing, or stuck on a problem?"

**Designing disclosure pattern:** Use the state transition lookup table above to determine the correct disclosure for any `(candidate_exists, validate_fresh, verify_fresh)` combination. The examples below illustrate the most common states.

**Example conversation (no candidate, user says "I want to design user authentication"):**
> "OK. There is currently no candidate spec for the user_auth unit, meaning no design has been recorded yet. Here's what I can do: 1. Use guidance to walk through the design, then write it to the candidate. 2. If you already have an idea, I can write the candidate directly. Which do you prefer?"

**Example conversation (candidate exists, user says "Check it"):**
> "A candidate spec for user_auth already exists, but the validate cache has expired. I can: 1. Run validate — check the candidate spec's design quality. 2. Run verify — check if the code implementation matches the design. 3. Continue editing the candidate. When you say 'check it', do you mean checking the design (validate) or checking the implementation (verify)?"

### 4. Promote (only gate)

`specflowctl promote --unit <name>` is the only operation that writes to stable. Before promoting, the CLI independently checks cache freshness:

1. **Check validate cache** — The CLI reads `docs/specs/_validation/unit/{name}/validate_result.md`. If missing or stale (hash mismatch), it rejects promote with guidance to re-run `spec_validate`.
2. **Check verify cache** — The CLI reads `docs/specs/_validation/unit/{name}/verify_result.md`. If missing or stale, it rejects promote with guidance to re-run `spec_verify`.
3. **Both fresh** → Format validation + copy candidate → stable.
4. **After promote succeeds** → Both cache files are deleted.

**Agent-side pre-check (optional):** Before calling `specflowctl promote`, the agent may optionally read the cache files to report freshness status to the user. This is redundant with the CLI's own enforcement but provides transparency. The agent can safely skip this step and call `specflowctl promote --unit <name>` directly — the CLI will reject with clear guidance if caches are missing or stale.

The CLI `specflowctl promote --unit <name>` also validates format (frontmatter, required fields, reference integrity) and copies candidate files to stable. During the copy, the tool automatically updates each file's frontmatter `layer` field from `candidate` to `stable`, ensuring the field matches the target directory.

**Truth semantics:** Promote is the act of recording a reconciled design as authoritative truth. After promote, the stable spec becomes the new level-2 truth. The old stable is superseded (git history preserves it). Candidate-layer files are removed after promote — this keeps file existence as an unambiguous state signal. To start a new editing round, the agent forks from stable: copies `s_unit_<name>.md` (and its appendices) back to the candidate layer as `c_unit_<name>.md`. See [Truth Hierarchy](#truth-hierarchy).

## HARD RULES

These override default helpful-assistant behavior. They are not suggestions.

**HARD RULE 1: Read Specs Before Discussing or Changing a Topic**
Before discussing, analyzing, or modifying any topic related to a unit, first read the unit's stable and/or candidate spec. If the spec already documents relevant design decisions, constraints, or boundaries, summarize them to the user before starting new analysis or proposals. If the spec has no relevant coverage on the topic, state so explicitly before starting new work: "The spec currently has no recorded design content on this topic. We can start designing from scratch." Create or update spec when design changes. If no spec exists for the unit, create one. Read `framework/spec_writing_guide.md` or reference existing specs for format.

**HARD RULE 2: Promote Is the Only Gate to Stable**
Never call `specflowctl promote` without user confirmation. Before promote, always run validate then verify. If either fails, stop and report. The agent does not decide when to validate, verify, or promote — it suggests, the user confirms.

Validate and verify are quality gates. They write cache files (`_validation/`) but never spec or stable files. If validate or verify fails, the agent MUST NOT proceed to promote.

**HARD RULE 3: Validate and Verify Check Quality, Promote Writes**
`validate` and `verify` check quality and report findings. They are read-only — they do not modify files or advance state. Only `promote` writes to stable. Commands like `next`, `rule`, `doctor`, `init`, `migrate` are for discovery and maintenance and do not check quality.

**HARD RULE 3a: Never Skip Divergence Resolution**
When `verify` reports a MISMATCH, the agent MUST present the findings to the user and wait for a decision. The agent MUST NOT silently choose a direction, proceed to promote, or treat candidate as automatically correct.

**HARD RULE 4: Stop When Unclear**
Stop and ask when the target unit is unclear, the required spec or framework file cannot be found, or the next workflow step cannot be determined. Do not guess or proceed with incomplete information.

## Commands Reference

| Command | What it does | Who calls it |
|---------|-------------|-------------|
| `specflowctl next --unit <name>` | Discover unit files and dependencies. Fails if unit is not found or tool errors. | Agent |
| `specflowctl promote --unit <name>` | Checks validate+verify cache freshness, then validates format + copies candidate→stable. Rejects if cache stale. | Agent (after user confirmation, after validate+verify) |
| `spec_validate {name}` (agent trigger) | Read-only subagent with validate checklist. Writes cache on PASS. See `framework/validate_checklist.md`. | User says "spec_validate" or confirms agent suggestion |
| `spec_verify {name}` (agent trigger) | Read-only subagent with verify checklist. Writes cache on ALIGNED. See `framework/verify_checklist.md`. | User says "spec_verify" or confirms agent suggestion |
| `spec_promote {name}` (agent trigger) | Checks cache freshness → if stale, suggests re-run validate/verify → if fresh, calls promote | User says "spec_promote" or confirms agent suggestion |
| `specflowctl init` | Initialize specFlow project | Human |
| `specflowctl doctor` | Diagnose project setup | Human |
| `specflowctl migrate` (deprecated) | Update hook files and check tooling version — use `spec_flow_update` instead | Agent or human (fallback) |
| `spec_flow_update` (agent trigger) | Full update: pull framework, update binaries & hooks, check document format | User says "spec_flow_update" |
| `specflowctl rule *` | Rule governance | Human maintainer |
| `specflowctl validate` | Validate candidate spec structure (7 checks) or file write permissions | Human maintainer or agent |

Project truth inputs: `docs/specs/`.
