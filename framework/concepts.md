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
| `docs/specs/_validation/` | Validate/verify cache (§3c) |

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

At natural transition points the agent suggests the next action. The user confirms with any affirmative response:

| Agent says | Meaning |
|-----------|---------|
| "Shall I run **validate** to check the spec design?" | Read-only subagent per §3a |
| "Shall I run **verify** to check the implementation?" | Read-only subagent per §3b |
| "Ready to **promote** to stable?" | Finalize and archive |

The user can also use explicit triggers at any time:

| Trigger | What agent does |
|---------|-----------------|
| `spec_validate {unit}` | Read-only subagent with the validate checklist below. See §3a. |
| `spec_verify {unit}` | Read-only subagent with the verify checklist below. See §3b. |
| `spec_promote {unit}` | Runs validate then verify. If both pass: `specflowctl promote --unit {unit}`. If fails: stop, report. |

If the user's language is vague ("check this", "see if it's right"), clarify:
"Did you mean **spec_validate** (check design) or **spec_verify** (verify implementation)?"

If the user declines a suggestion, continue editing. Do not insist.

### 3.0. Agent suggestion rules

When the user's request does not match an explicit trigger (`spec_validate`, `spec_verify`, `spec_promote`), classify the intent into one of the 5 categories below. Do not use keyword matching — infer from context.

**State disclosure first:** Before suggesting any action, communicate the current file state using concrete, user-understandable language. Never ask vague questions like "要不要走validate-verify-promote流程?". Instead, disclose the state first, then state what is available for the user to choose from. See each intent's disclosure pattern below.

| State to disclose | What to say |
|-------------------|-------------|
| Candidate spec exists for the unit | "当前已有 candidate spec（`...`），记录了你正在编辑的设计" |
| No candidate spec for the unit | "当前没有 candidate spec，说明还没有设计被记录下来" |
| Validate cache fresh | "validate 已经通过，且读取的文件没有变化" |
| Validate cache missing/stale | "validate 缓存不存在/已过期，需要重新检查" |
| Verify cache fresh | "verify 已经通过，且检查的文件没有变化" |
| Verify cache missing/stale | "verify 缓存不存在/已过期，需要重新检查" |

**State transition lookup table:**

This table maps the three boolean state dimensions to the exact disclosure text and suggested action. `N/A` means the cache state is irrelevant. When `candidate_exists` is N, all cache dimensions are N/A. When `candidate_exists` is Y and `validate_fresh` is N, `verify_fresh` is N/A because verify is not actionable until validate passes.

| candidate_exists | validate_fresh | verify_fresh | Disclose | Then offer |
|---|---|---|---|---|
| N | N/A | N/A | "当前没有 candidate spec" | "可以开始编写 candidate spec，记录你的设计思路。" |
| Y | N | N/A | "candidate 已存在，但 validate 未通过或缓存过期。" | "可以继续更新 candidate，或者跑 validate 做设计质量检查。" |
| Y | Y | N | "candidate 已存在，validate 已通过，但 verify 未做或缓存过期。" | "可以继续更新 candidate，或者跑 verify 检查实现是否匹配设计。" |
| Y | N | Y | "candidate 已存在，verify 已通过，但 validate 缓存过期。" | "建议重新跑 validate，因为 design 可能已变更。" |
| Y | Y | Y | "candidate 已通过 validate 和 verify。" | "如果设计已定稿，可以 promote 到 stable。" |

| Intent | What user wants | Agent action | File state check |
|--------|----------------|-------------|-----------------|
| **designing** | Plan, change direction, explore approach | **Use the state transition lookup table above** to determine the correct disclosure and action for the current state. Never phrase as a binary "要不要走流程". See the example conversations below for illustration. | Use the state transition lookup table above |
| **implementing** | Write code, iterate, debug, test | Do not touch spec. Do not suggest validate/verify/promote. Let the user focus. | Candidate exists → ensure it's read but do not interrupt. No candidate, changing stable behavior → suggest fork first |
| **verifying** | Check correctness, see if it's right | **Disclose cache state first** — if verify cache is fresh: "verify 缓存有效，上次检查结果全为 ALIGNED，文件未变化。需要重新跑 verify 吗？" If stale or missing: "verify 缓存不存在或已过期，需要重新检查。" Then run `spec_verify`. | Candidate exists → verify candidate vs code. Only stable → verify stable vs code |
| **finalizing** | Lock in, wrap up, promote | Check validate cache then verify cache. Both fresh → suggest `spec_promote`. Cache stale/missing → suggest re-running the appropriate step |
| **recovering** | Something is wrong, stuck, error | Diagnose first: is it a code bug (→ implementing), design flaw (→ designing), or external blocker (→ blocked, ask user) |

**Fallback:** If the intent is unclear after reasonable effort, ask the user directly:
"Are you designing something new, implementing code, checking your work, finalizing, or stuck on a problem?"

**Designing disclosure pattern:** Use the state transition lookup table above to determine the correct disclosure for any `(candidate_exists, validate_fresh, verify_fresh)` combination. The examples below illustrate the most common states.

**Example conversation (no candidate, user says "我想设计用户认证"):**
> "好的。当前还没有 user_auth 单元的 candidate spec，说明还没有记录设计。我可以做以下事情： 1. 用 guidance 引导你完成设计，然后写到 candidate 2. 如果你已经有想法，我可以直接开始写 candidate。你倾向哪种？"

**Example conversation (candidate exists, user says "检查一下"):**
> "当前 user_auth 的 candidate spec 已存在，validate 缓存已过期。我可以： 1. 跑 validate — 检查 candidate spec 的设计质量 2. 跑 verify — 检查代码实现是否匹配设计 3. 继续编辑 candidate。你说的检查是指检查设计（validate）还是检查实现（verify）？"

### 3a. Validate

When running `spec_validate {unit}`, read this section, then open a read-only subagent with the checklist below embedded in the task prompt.

**Subagent permissions:**
- ALLOWED: Read, Grep, Glob
- FORBIDDEN: Write, Edit, Bash, Task — do not modify files, execute commands, or spawn sub-agents

**Checklist:**

1. **Structural integrity** — Read `docs/specs/units/candidate/c_unit_{unit}.md`. Verify `id`, `layer` (must be "candidate"), `version`, `unit_refs`, `rule_refs` are all present. Verify `acceptance_item_set` exists with at least one item, each with: `id`, `description`, `verification_type`, `verification_surface`, `implementation_surface`, `verification_method`, `pass_condition`, `not_runnable_yet`. Check that all `unit_refs` point to existing stable spec files, all `rule_refs` point to existing rule files, and any referenced appendix files exist.
2. **Scope clarity** — Is the unit's goal and responsibility scope clearly stated? Are first-round non-goals and boundaries defined? Are dependencies, rule bindings, and ownership boundaries explicit?
3. **Behavior completeness** — Are the main flow, key protocols, states and transitions, error paths, and data contracts described fully enough that implementation can proceed without guessing? Can verification proceed without guessing behavior, boundaries, or acceptance?
4. **Decision completeness** — Are all implementation-critical decisions recorded in the spec or its appendices? Does the spec avoid depending on chat context, oral consensus, README vision, or rejected alternatives for essential meaning?
5. **Acceptance verifiability** — Do the acceptance criteria prove the design goal is met, not just that artifacts exist? Are they testable and unambiguous? Does each item's `pass_condition` describe a verifiable outcome rather than a tautology or placeholder?
6. **Cross-unit consistency** — Read related unit candidate specs (from `unit_refs`). Check for contradicting statements about shared protocols, data formats, or behavior.
7. **Global constraint alignment** — Read `docs/specs/system_constraints.md` if it exists. Is the candidate compatible? If `system_constraints_ref` points to a version, does it match?

**Resolution:** When a check fails, determine which type:
- **fix_required** — The executor can identify a concrete repair inside the current candidate. Repair and re-run validate.
- **blocked** — The next step requires user input (unclear intent, missing decision, or external dependency). Stop and ask.

**Output:**
```
Validate result: PASS | FAIL (fix_required | blocked)
1. Structural integrity: PASS | FAIL — reason
2. Scope clarity: PASS | FAIL — reason
3. Behavior completeness: PASS | FAIL — reason
4. Decision completeness: PASS | FAIL — reason
5. Acceptance verifiability: PASS | FAIL — reason
6. Cross-unit consistency: PASS | FAIL — reason
7. Global constraints: PASS | FAIL — reason
Resolution: fix_required | blocked — next step
Summary: ...
```

**Cache:** After the subagent reports PASS, write a cache file to document which files were checked and their content hashes. This allows `specflowctl promote` to verify freshness without re-running the subagent. See §3c for file format and rules.

```
docs/specs/_validation/unit/{unit}/validate_result.md
```

After the subagent reports FAIL or blocked, delete any existing validate cache for this unit.

### 3b. Verify

When running `spec_verify {unit}`, read this section, then open a read-only subagent with the steps below embedded in the task prompt.

**Subagent permissions:**
- ALLOWED: Read, Grep, Glob
- FORBIDDEN: Write, Edit, Bash, Task — do not modify files, execute commands, or spawn sub-agents

**Target selection:**
- If a candidate spec exists → verify code against **candidate** (the current working proposal). Candidate is not truth; mismatches trigger divergence resolution below.
- If no candidate exists but a stable spec does → verify code against **stable** (check if current implementation still conforms to recorded truth).

**Steps:**

1. **Per-item verification** (functional) — For each acceptance item in the target spec:
   - Read the implementation files listed in `affects.files` (or files matching `implementation_surface` / `verification_surface`)
   - Does the implementation satisfy the `pass_condition`?
   - Report per item: ALIGNED / MISMATCH / CANNOT_DETERMINE with evidence (file paths, line ranges, observations)

2. **Scope check** — Scan affected files for behavioral changes not declared in acceptance items. Verify `affects.files` declarations match actual changes.

3. **Retirement verification** (replacement scenario) — If the candidate's `source_basis` is `replacement`, verify old code paths are fully removed with no remaining references. Check for `old_code_deleted` and `no_remaining_refs` evidence.

4. **Implementation quality** — Check for dead code, over-engineering, or disproportionate change volume.

5. ⭐ **Divergence resolution** — For each MISMATCH item, analyze the root cause:
   - **code_ahead** — The implementation has behavior that the spec does not describe. The candidate spec is stale and needs updating to match code.
   - **spec_ahead** — The spec describes behavior that the implementation does not satisfy. Code is incomplete.
   - **needs_design** — Neither matches a coherent design; the approach needs rethinking.
   - **blocked** — The mismatch depends on external input or unresolved decisions.
   
   **Present findings to the user, do not decide automatically.** Example:
   > "Item `auth.login` reports MISMATCH. The spec describes rate-limiting (5 req/min) but the implementation allows 10 req/min. Is the spec outdated (code_ahead) or is implementation incomplete (spec_ahead)?"
   
   After the user decides, record the resolution direction and next step.

6. ⭐ **Stable-only mode** — When verifying against stable (no candidate exists):
   - If code and stable are aligned → report ALIGNED. No further action needed.
   - If code and stable diverge → this means the current implementation has drifted from recorded truth. Do NOT enter divergence resolution for stable directly — instead, recommend a `unit_fork` to create a candidate round that reconciles the difference.

**Divergence resolution rules:**

| User verdict | Meaning | Next step |
|-------------|---------|-----------|
| code_ahead | Code is correct, candidate is stale | Update candidate spec → re-run validate → re-run verify → promote |
| spec_ahead | Candidate design is correct, code incomplete | Implement code → re-run verify → promote |
| needs_design | Both need redesign | Redesign candidate → validate → verify → promote |
| blocked | External dependency or missing decision | User unblocks → re-run verify |

**Output:**
```
Verify result: ALIGNED | MISMATCH
Target: candidate | stable
Items:
  - {item.id}: ALIGNED | MISMATCH — evidence
    Direction: (only if MISMATCH) code_ahead | spec_ahead | needs_design
    Resolution: update_candidate | implement_code | redesign | blocked
Scope: PASS | FAIL — findings
Quality: PASS | FAIL — findings
Divergence summary: (only if any MISMATCH)
  - {item.id}: file:line — {description}
    User verdict: ...
    Next step: ...
Summary: ...
```

**Cache:** After the subagent reports ALIGNED, write a cache file with the content hashes of all checked files. After MISMATCH, delete any existing verify cache for this unit.

```
docs/specs/_validation/unit/{unit}/verify_result.md
```

### 3c. Validation cache lifecycle

Cache files record the result and file content hashes of the last `spec_validate` or `spec_verify` run. They are not a state machine — they do not determine what happens next. They only answer: "were these files checked and were they passing at that time?"

**File locations:**

- `docs/specs/_validation/unit/{name}/validate_result.md`
- `docs/specs/_validation/unit/{name}/verify_result.md`

**Format (YAML frontmatter + markdown body):**

```yaml
---
command: validate            # or verify
unit: user_auth
result: pass                 # pass | aligned | blocked | mismatch
target: candidate            # (verify only) candidate | stable
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/c_unit_user_auth.md
    hash: sha256:abc123...
  - path: src/auth/login.go
    hash: sha256:def456...
---
Free-form summary of the result.
```

**Hash algorithm (must be consistent across all agents and CLI):**

Each file hash is computed as:
1. Read the file content as UTF-8 text
2. Normalize line endings: `\r\n` → `\n`, then standalone `\r` → `\n`
3. Ensure trailing newline (append `\n` if missing)
4. Compute SHA-256 of the normalized content
5. Format as `sha256:<hex>` (e.g. `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`)

This is the same normalization used by `specflowctl review` input fingerprints. It guarantees cross-platform consistency regardless of git's autocrlf settings or the agent's operating system.

**Write rules:**

| Event | Action |
|-------|--------|
| `spec_validate` PASS | Write `validate_result.md` with hashes of all files read during the check |
| `spec_validate` FAIL / blocked | Delete `validate_result.md` if it exists |
| `spec_verify` ALIGNED | Write `verify_result.md` with hashes of spec + implementation files checked |
| `spec_verify` MISMATCH | Delete `verify_result.md` if it exists |
| `specflowctl promote` succeeds | Delete both `validate_result.md` and `verify_result.md` |
| File content changes (hash mismatch) | Cache becomes stale — detected at promote time |

**Staleness detection:**

`specflowctl promote --unit <name>` reads both cache files, re-computes SHA-256 hashes of every listed file, and compares against the stored hashes. If any hash differs or a file is missing, the cache is stale and promote is rejected with guidance.

**Important:** Cache is never refreshed automatically. Only the agent writing a new cache after a fresh validate/verify changes it. This is because validate and verify are semantic operations that require AI judgment — they cannot be reduced to a mechanical hash check.

### 3d. Recovery patterns

Common situations where the standard flow diverges:

1. **Code changed without updating candidate** — This is the normal iteration pattern. Do not interrupt. When the user signals they are ready to check (verify intent → run `spec_verify`). Verify will detect the divergence and enter divergence resolution.

2. **Candidate changed without implementing** — If the user changes the spec mid-implementation and then wants to check, run `spec_validate` first (to confirm the new design is sound), then `spec_verify`.

3. **Stable and code have drifted (no candidate exists)** — The implementation no longer matches recorded stable truth. Run `spec_verify` in stable-only mode to see the gap. If a gap exists, suggest creating a candidate fork to reconcile.

4. **Validate fails repeatedly** — Check whether the issue is `fix_required` (concrete repair possible in the candidate) or `blocked` (requires user input). If blocked, stop and present the question to the user.

### 4. Promote (only gate)

`specflowctl promote --unit <name>` is the only operation that writes to stable. Before promoting, the CLI independently checks cache freshness:

1. **Check validate cache** — The CLI reads `docs/specs/_validation/unit/{name}/validate_result.md`. If missing or stale (hash mismatch), it rejects promote with guidance to re-run `spec_validate`.
2. **Check verify cache** — The CLI reads `docs/specs/_validation/unit/{name}/verify_result.md`. If missing or stale, it rejects promote with guidance to re-run `spec_verify`.
3. **Both fresh** → Format validation + copy candidate → stable.
4. **After promote succeeds** → Both cache files are deleted.

**Agent-side pre-check (optional):** Before calling `specflowctl promote`, the agent may optionally read the cache files to report freshness status to the user. This is redundant with the CLI's own enforcement but provides transparency. The agent can safely skip this step and call `specflowctl promote --unit <name>` directly — the CLI will reject with clear guidance if caches are missing or stale.

The CLI `specflowctl promote --unit <name>` also validates format (frontmatter, required fields, reference integrity) and copies candidate files to stable.

**Truth semantics:** Promote is the act of recording a reconciled design as authoritative truth. After promote, the stable spec becomes the new level-2 truth. The old stable is superseded (git history preserves it). Candidate-layer files are preserved for the next round. See [Truth Hierarchy](#truth-hierarchy).

## HARD RULES

These override default helpful-assistant behavior. They are not suggestions.

**HARD RULE 1: Read Specs Before Discussing or Changing a Topic**
Before discussing, analyzing, or modifying any topic related to a unit, first read the unit's stable and/or candidate spec. If the spec already documents relevant design decisions, constraints, or boundaries, summarize them to the user before starting new analysis or proposals. If the spec has no relevant coverage on the topic, state so explicitly before starting new work: "当前 spec 中没有记录关于此主题的设计内容。我们可以从零开始设计。" Create or update spec when design changes. If no spec exists for the unit, create one. Read `framework/spec_writing_guide.md` or reference existing specs for format.

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
| `spec_validate {name}` (agent trigger) | Read-only subagent with validate checklist (§3a). Writes cache on PASS. | User says "spec_validate" or confirms agent suggestion |
| `spec_verify {name}` (agent trigger) | Read-only subagent with verify checklist (§3b). Writes cache on ALIGNED. | User says "spec_verify" or confirms agent suggestion |
| `spec_promote {name}` (agent trigger) | Checks cache freshness → if stale, suggests re-run validate/verify → if fresh, calls promote | User says "spec_promote" or confirms agent suggestion |
| `specflowctl init` | Initialize specFlow project | Human |
| `specflowctl doctor` | Diagnose project setup | Human |
| `specflowctl migrate` | Update hook files and check tooling version | Agent or human (fallback) |
| `spec_flow_migrate` (agent trigger) | Full migration: run tool then check document format | User says "spec_flow_migrate" |
| `specflowctl rule *` | Rule governance | Human maintainer |
| `specflowctl validate` | Validate candidate spec structure (7 checks) or file write permissions | Human maintainer or agent |

Project truth inputs: `docs/specs/`.
