# Rule Validate Checklist

## Overview

`validate@{rule}` is the rule path of `validate`. It checks rule metadata structural validity (Checks 1-5) and rule body quality (Check 6).
Agent runs this when the target is detected as a Rule via automatic type detection (see `framework/commands.md` §Target Resolution).

**Result:** PASS writes `docs/specs/meta/validation/rule/{id}/validate_result.md`.
A candidate full-run FAIL writes a failure record (`result: fail` + `blocking: true` with the per-check `status` map) — the failure-recovery baseline. A stable-only FAIL writes the same record — see §Stable-only mode. Rule targets have no delta: the rule file is the rule's own content and its only external input is the global rule set, so recovery is always the full confirmation run. Report per §Output Format.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `validate@{rule}` | full | All 6 checks. Quality checks are holistic — always runs full. |
| `validate@{rule}:check-{n}` | targeted | Single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@{rule}:{keyword}` | targeted | Match keyword to check name. User explicitly chooses focus. Does not write a cache. |

**Mechanical pre-pass:** for a candidate target, every full `validate@{rule}` run starts with `specflowctl validate rule --id {rule_id}`, which mechanically enforces the rule's metadata checks — frontmatter completeness, file path consistency (the candidate-layer condition of Check 3), ID/scope consistency, `promotion_owner_unit` (warning), and prohibited fields — before any session launches. A mechanical FAIL stops the run — fix and re-run the tool before planning; no gate run is planned and no session launches until it passes. A stable-only confirmation has no mechanical pre-pass — the tool checks the candidate layer only — so the stable-layer path condition and Check 6 stay agent-verified.

## Execution Rules

- **Subagent permissions:** rule validate executes in an independent read-only sub-agent session using the generated `specflowctl gate-mission --run {run_id} --keys {keys} --format prompt` mission. The sub-agent may read rule files, search text patterns, check file existence, query read-only repository history, and run the read-only framework inspection subcommands its checks instruct it to run (`specflowctl deps`, `specflowctl surfaces`, `specflowctl validate candidate`). It must NOT modify files, run state-changing commands, or delegate to other agents. Targeted runs (`:check-{n}` / `:{keyword}`) execute directly in the main agent session instead.
- On FAIL: identify which checks failed and the contradictory information
- Resolution types (each finding is labeled with one):
  - **actionable** — A concrete repair can be made inside the current candidate rule file without user judgment.
  - **needs_decision** — Requires user input (unclear intent, missing decision, or external dependency). Stop and ask.

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

### Finding severity ownership

Each finding's `[{severity}]` is assigned by the session that raises the finding. When the final synthesis (`framework/verification_scope.md` §Final synthesis) retains or merges findings, it may raise a retained finding's canonical severity conservatively — never lower it; counts and blocking derive from the canonical severities (see `framework/severity_policy.md`).

### Report skeleton and field definitions

All quality-gate reports (validate and verify) share the same report skeleton below. The header lines (`Result`, `Blocking promote`, `Key counts`), the Findings block fields, and the `Next step` line are identical across gates; only the body content and the gate-specific extra lines inside each finding block are gate-specific and defined in each checklist file. The Findings section follows the header because on FAIL it is the decision surface — the body is its audit and evidence.

```
────────────────────────────────────────────
{gate}@{target} · {mode} · {layer}
Result: PASS | FAIL
Blocking promote: yes | no
Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)
────────────────────────────────────────────
Findings: none
# or, one entry per finding:
Findings:
  [{severity}] {location} — {issue} (actionable | needs_decision)
    problem: {one-sentence statement of what is wrong, naming the concrete subject}
    evidence:
      - {verbatim quoted source content} — {source label}
    impact: {what goes wrong or stays undecidable if this is not resolved}
    fix: {concrete repair action}                  # actionable
    # or
    decision: {the question the user must answer}  # needs_decision
    options:
      - {candidate option}
    {gate-specific detail lines}
    ref: {anchor or line reference}                # optional, tracking only
────────────────────────────────────────────
{body}
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{gate}@{target}` — the gate and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`. Gates: `validate`, `verify`. Targets: unit or rule name. The `verify` gate carries two lenses — `alignment` (spec-vs-code) and `quality` (spec-aware code quality) — and its sessions report under one of them. The `alignment` lens runs in item sessions (`item:<unit>:<item>`, Steps 2, 3, 4, and 6, with Step 7 for the mismatches they raise) and in one unit-level scan session per run (`unit:<unit>`, Steps 1 and 5, with Step 7 for its own mismatches).
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for a delta re-run (`revalidate@{unit}` / `reverify@{unit}`). A validate delta that finds an own-content change is recorded as a full run (`basis: full`); validate has no repair mode.
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `Result` — `PASS | FAIL` for both gates. For coverage runs this value is generated by `gate-finalize` from the accepted sessions (or from the single rule-validate session); the coordinator never supplies it. The gate is decided by severity: FAIL means gate-driving retained P0/P1 findings exist. Unit validate grades P0-P3, where P2/P3 are non-blocking advisories (debt and polish recorded in the cache, never gate-driving), so a validate FAIL is always driven by P0/P1 findings; rule validate grades findings P0/P1 only; verify FAIL means gate-driving retained P0/P1 mismatches/findings exist. A finding deferred to another unit is retained and routed but is not gate-driving — it never causes FAIL.
- `Blocking promote: yes | no` — `yes` when the run FAILs (gate-driving P0/P1 findings exist); `no` otherwise. Valid for both gates.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the gate-driving retained finding count and a/b/c/d are generated by `gate-finalize`; the coordinator never supplies them. Unit validate's P0/P1 counts are gate-driving and its P2/P3 counts are recorded non-blocking advisories (`framework/validation_cache.md` §Advisory carry); rule validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. A finding deferred to another unit is excluded from these counts (it is presented under `Deferred to {unit}:`). Gate-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, the quality lens's suppressed-by-spec count) appear in the body.
- `{body}` — gate-specific content defined in this file's body format section (validate: one line per check; verify `alignment`: per-item alignment and divergence analysis; verify `quality`: architecture assessment and suppressed findings).
- `Findings:` — `Findings: none` when no finding is retained; otherwise one entry per finding: a unified entry line `[{severity}] {location} — {issue} (actionable | needs_decision)` followed by the finding block's required detail lines. `actionable` — a concrete repair can be made without user judgment (alignment: direction spec_gap/code_gap; quality: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (alignment: direction needs_design/blocked; quality: architecture trade-offs). The block is written for a reader who has not opened the spec or the code: delete every `ref:` line and every `§`, line-number, or other anchor reference from the field text, and the reader must still learn what is wrong, what the conflicting sides say, what is at risk, and what to fix or decide. Anchors appear only in the optional trailing `ref:` line, which carries tracking information and no meaning.
  - `problem:` — one sentence naming the concrete subject (acceptance item id, field or parameter name, behavior) and what is wrong with it; never an anchor, region name, or pointer.
  - `evidence:` — the quoted content that proves the claim, as one or more indented `- {verbatim quoted source content} — {source label}` sub-lines. Contradiction findings quote both conflicting sides verbatim; undefined or missing behavior lists the defined part and the missing part; spec-vs-code findings quote the declared behavior and the implemented behavior. A quality P3 finding uses its `fact_anchor:` line as this evidence form.
  - `impact:` — what goes wrong, or stays undecidable, if the finding is not resolved.
  - `fix:` — actionable findings: the concrete repair action. `decision:` — needs_decision findings: the question the user must answer, with `options:` sub-lines listing the candidate choices.
  - The block's detail lines are contiguous indented lines directly under the entry line — no blank lines inside the block (the block is stored and re-rendered verbatim when a finding is carried into a partial run).
  - `{gate-specific detail lines}` — gate-specific extras follow the shared fields: alignment `root_cause:`, `direction:`, and `confidence:`; quality `spec_context:` and its mechanically required `fact_anchor:` for P3 findings; unit validate its mechanically required `fact_anchor:` for P3 findings. Rule validate adds none.
  - Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive. Batch-group entries carry the same fields; terse one-line values are fine.
  - Findings whose recorded ownership routes them to another unit are presented in a `Deferred to {unit}:` section after the Findings section, with the same block fields plus an `ownership:` line. They are retained and routed but do not count toward `Key counts`, `Blocking promote`, or the gate result (see `framework/unit_verify_checklist.md` §Output Format → Deferred findings).
- `Evidence recording:` — reports carry no dependency declarations. `gate-finalize` records the run's whole input surface: one cache entry per input-surface file with its whole-file `hash` and ordered chunk sequence, plus a per-check marker (`check` + lens, and the failure status on failure records) for every executed judgment. Spec objects resolved by name are recorded as logical references so a layer move does not stale the cache (see `framework/validation_cache.md` §Format → Entries). Partial runs record the re-run keys' markers alongside the carried keys' snapshotted markers; targeted runs write no cache.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. A finding's fix lifecycle has three states with fixed wording: `finding_open` → "Resolve the findings, then re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}`) to confirm"; `fixed_pending_recheck` → "Fixes applied; re-run the target-appropriate re-check command to confirm." — only after the approved fix was actually written; `verified` → "Re-check passed." — only after a re-check confirmed the fix. A gate report is always produced before any fix is applied (nothing is implemented before the user approves the findings), so an actionable finding's report-time `Next step` is always the `finding_open` wording. Other guidance: both gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}"; nothing further → `None`.

**Targeted runs:** end the report with the gate's targeted note ("This was a targeted check — no complete cache was written. Run `{gate}@{target}` for a complete ...") after the `Next step` line. If the result contains P0/P1, run `gate-invalidate` before reporting completion.

### Completion — Persist Gate Cache

A quality-gate run is complete only when its result is persisted and visible to `fresh`/`promote`. A report alone does not satisfy the gate.

- **Full (`validate@{target}` / `verify@{unit}`, `mode: full`, `basis: full`)** — complete only when the coverage run finished and its cache was written: (1) `specflowctl gate-plan --gate {validate|verify} (--unit {name} | --rule {id}) --target {candidate|stable} [--relationships NAMES|none] [--inputs-file PATH]` fixed the immutable input snapshot, local coverage set, and relationship scope (`--relationships` is required for full unit runs and optional for delta/repair; select names or `none`) **before any executor read input** (before mission assembly / sub-agent launch) — the input manifest adds evidence that every session may read but never creates a coverage key, and it must not be a repository-content file (keep it under the ignored `meta/plan_inputs/` or outside the repository); (2) the tooling's computed reviewer units for the uncovered coverage were joined by the coordinator into sessions — one per report kind and lens (one per reviewed file for verify quality), and a session's key set is split only when a session declares it exceeds that session's capacity — reusing accepted public tasks and waiting for assigned tasks and, for each session, generated a mission with `specflowctl gate-mission --run <run_id> --keys <k1,k2,...> --format prompt`, sent it verbatim to one independent reviewer, and recorded its report with `specflowctl gate-submit --run <run_id> --session <session_id> --keys <k1,k2,...> --report PATH` until `gate-status` reports full coverage; (3) when the run requires a final synthesis — unit validate: assigned relationships or blocking P0/P1 findings only (its P2/P3 advisories need no session); verify: assigned relationships or any finding — one independent final synthesis was generated with `specflowctl gate-mission --run <run_id> --final --format prompt` and recorded with `specflowctl gate-submit --run <run_id> --session cross --keys cross --report PATH`; rule targets never synthesize (a rule with blocking findings writes the failure record directly) and always skip this step; a run with neither assigned relationships nor such findings skips this step; (4) the coordinator ran `specflowctl gate-finalize --run <run_id>` and the cache `docs/specs/meta/validation/{unit|rule}/{name}/validate_result.md` | `verify_result.md` was written (the merged verify cache carries its `alignment` and `quality` sections); (5) `fresh@{target}` (`fresh --unit {name}` / `fresh --rule {id}`) shows the expected gate state (`FRESH` for `result: pass` / `blocking: false`, `BLOCKED` for `result: fail` / `blocking: true`). A candidate **full-run** FAIL writes a failure record (`result: fail` / `blocking: true`; validate/verify records declare `pass`/`fail` for every executed judgment) — the failure-recovery baseline. A finalize that finds mid-run content drift on a full run salvages the run: the result is persisted with the drift's affected report keys recorded in `invalidated_checks` — a salvaged pass cache is a recovery baseline that `fresh` reports `STALE` until its recovery plan re-runs the invalidated keys (validate or rule: full scope; verify: repair); a salvaged failure record keeps its blocking findings (`BLOCKED`, or `STALE` with the findings named when its evidence also changed), and the next plan re-runs the invalidated keys with the failed judgments. A structural divergence (the input surface itself changed) and any drift on a partial run are rejected and write no cache. See `framework/validation_cache.md` §Write Rules and §Failure handling by gate role.
- **Delta (`revalidate@{unit}` / `reverify@{unit}`, `mode: full`, `basis: delta` or `basis: full`)** — the same coverage-run sequence. A **validate** delta is planned mechanically: an external-only change (the unit's own spec union is unchanged; only a provider promise or a rule moved) runs the `external` group and carries the rest; any own-content change, failure record, salvaged cache, moved judging standard, or unusable baseline runs all four groups and is recorded as a full run (`basis: full`). There is no review session and no repair mode for validate. For unit validate, the final synthesis runs when relationships are assigned (the affected standing relationships for an external delta, or the baseline's standing relationships for a full-scope plan) or blocking P0/P1 findings exist; a P2/P3-only run presents its findings directly. A **verify** delta derives the re-run set from each conclusion's own record pins and from the evidence its key currently requires (a new file in a declared surface), adds current keys the baseline never declared and explicit `--rerun` overrides (repair also forces failed judgments, persisted invalidations, and failed relationships), and carries the rest; the whole derived set executes under the standard checklist, and `gate-finalize` merges the re-run evidence with the carried baseline evidence. Rule targets have no delta.
- **Targeted (`:check-{n}` / `:{keyword}`, no complete cache)** — complete when the report is produced and, for P0/P1, the coordinator has run `specflowctl gate-invalidate` for every affected check key. Targeted runs intentionally do NOT create a gate run — `gate-plan` / `gate-mission` / `gate-submit` / `gate-finalize` are not run. A targeted PASS leaves cache state unchanged. A targeted P0/P1 deletes a pass cache or persists `invalidated_checks` on a failure record and invalidates any matching open run. For verify, the same locked transition invalidates the contradicted immutable evidence, so consuming caches and open runs must recheck the affected judgments. It never publishes a complete result cache or satisfies the promote gate. The report ends with `This was a targeted check — no complete cache was written. Run {gate}@{target} for a complete ...`.

**Self-check:** `ls docs/specs/meta/validation/{unit|rule}/{name}/` and `fresh@{target}` (unit: `fresh --unit {name}`, rule: `fresh --rule {id}`).
==ATOM_END:report_skeleton==

### Body format (rule validate)

One line per check, numbered as in this file:

```
1. Frontmatter completeness: PASS | FAIL — reason
2. ID/Scope consistency: PASS | FAIL — reason
3. File path consistency: PASS | FAIL — reason
4. Promotion owner unit: PASS | WARNING — reason
5. Prohibited fields: PASS | FAIL — reason
6. Rule body quality: PASS | WARNING | FAIL — reason
Failed checks: N | Advisory findings: K
```

**Counting rules:**

- `Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the total number of distinct findings across all FAIL checks; a/b/c/d the count per severity. rule validate grades findings P0/P1 only — P1 is the contract-decided default, a P0 grade requires the §9 boundary check (see Severity handling below) — so `c` and `d` are always 0. In targeted runs, only executed checks are counted.
- `Failed checks` is the number of FAIL checks among executed checks, shown in the body's check lines. WARNING is not a failed check.
- WARNING findings (Check 4 / Check 6 step 3) are presented on their check line's reason and counted separately as `Advisory findings: K` in the body — they are never counted in `Findings` and never affect `Failed checks`.
- `Blocking promote` is `yes` when P0/P1 findings exist (rule validate FAIL blocks promote; a candidate full-run FAIL and a stable-only FAIL write a failure record).

Rule validate findings are always presented flat — rules have no batch classification; each FAIL finding is listed directly under the `Findings:` section in the unified finding format `[{severity}] {location} — {issue} (actionable | needs_decision)`, followed by the shared finding block (`problem:` / `evidence:` / `impact:` / `fix:` or `decision:` — see §Output Format).

### Severity handling

rule validate grades findings P0/P1. P1 is the contract-decided default for every FAIL check. A P0 grade is judgment-based: the single checks session assigns it directly and must first read the impact surface the P0 claim depends on — the governance file governing the affected mechanism, or the consumer units added to the run as read-only extra inputs (`specflowctl gate-extend`) — and establish the §9.3 boundary in `framework/severity_policy.md`. Rule validate has no final synthesis, so no later step re-grades the severity; the session-assigned value is canonical and drives the derived result and counts. The finding's severity appears on its entry line in the report.

## Target-layer applicability

Use the planned `Target layer` (`candidate` or `stable`) shown in the gate mission when applying this checklist. For a targeted run without a gate mission, use the layer resolved by the command: candidate when present, otherwise stable. All six checks apply to both layers, with the layer-specific path condition defined in Check 3. A stable confirmation checks accepted content; it does not require a candidate file.

## Checklist

### Check 1 — Frontmatter Completeness

Verify the rule file has all required frontmatter fields:

| Field | Required | Valid values |
|-------|----------|-------------|
| `rule_id` | Yes | `g_rule_{name}` or `b_rule_{name}` |
| `rule_scope` | Yes | `global` or `bound` |

If any required field is missing or empty → FAIL.

### Check 2 — ID/Scope Consistency

Verify `rule_scope` matches the ID prefix:

| rule_id prefix | Expected rule_scope |
|----------------|---------------------|
| `g_rule_` | `global` |
| `b_rule_` | `bound` |

If `rule_scope` is `global` but ID starts with `b_rule_` → FAIL.
If `rule_scope` is `bound` but ID starts with `g_rule_` → FAIL.
When both are present, `rule_scope` in frontmatter takes precedence (per `spec_writing_guide.md`).

### Check 3 — File Path Consistency

Verify the rule file is in the target layer directory: `docs/specs/rules/candidate/{rule_id}.md` for a candidate run, or `docs/specs/rules/stable/{rule_id}.md` for a stable confirmation. The rule layer is encoded by the file path — no `layer` frontmatter field is declared (see `framework/spec_writing_guide.md` §6).

Filenames follow the pattern `{g_or_b}_rule_{id}.md` — `g_rule_` for global rules, `b_rule_` for bound rules.

Mechanical enforcement: `specflowctl validate rule --id <rule_id>` runs this check's candidate-layer condition before the session — it fails when the candidate file is not at the canonical candidate-layer path or when the file's `rule_id` frontmatter disagrees with its filename. The stable-layer condition for a stable confirmation remains agent-verified.

### Check 4 — `promotion_owner_unit` (optional documentation field)

`promotion_owner_unit` is an optional documentation field with no effect on tooling behavior. This check produces no execution failure.

→ WARNING if present but the value does not name a unit that exists in `docs/specs/units/`.

The existence test is a live path fact, not a read of the unit's content: it needs no input-surface entry, and the candidate mechanical pre-pass reports the same test before the session. The mission states the rule's consumer set (derived from current-layer unit `rule_refs`) as a plan-time naming fact; a judgment that needs a consumer spec's content reports the missing read ref instead of judging from memory.

### Check 5 — Prohibited Fields

Verify the rule file does NOT contain:
- `bound_objects` — rule files must not store consumer lists
- A consumer list in any form — consumers are derived from unit `rule_refs`, never stored in rule files

If either is found → FAIL.

### Check 6 — Rule Body Quality

**Purpose:** Evaluate whether the rule's body content (constraint definition, exceptions, scope) is internally consistent and clearly stated. Unlike Checks 1-5 (metadata structural validity), this check evaluates the rule's written content.

**Execution steps:**

1. **Constraint clarity:** read the rule file body and identify the core constraint statement. Verify it is unambiguous — a reader can determine what behavior is required, prohibited, or permitted without guessing.

2. **Exception consistency:** read all exception clauses or scope limitations in the body. Verify no exception effectively nullifies the constraint (e.g., a rule saying "all APIs must use HTTPS" with an exception "except when HTTP is used"). If an exception contradicts the constraint → FAIL.

3. **Verifiability:** assess whether the constraint can be verified through static code inspection or verify. A rule like "be intuitive" is not verifiable — flag as WARNING. A rule like "all API handlers must validate input before processing" is verifiable — PASS.

4. **Self-contradiction scan:** scan the body for statements that conflict with each other (e.g., "versions must be in semver format" in one paragraph, "versions are integers" in another). If found → FAIL.

5. **Layer-prefix path check:** scan the rule body for layer-prefixed spec paths (`docs/specs/rules/candidate/`, `docs/specs/rules/stable/`, `docs/specs/units/candidate/`, `docs/specs/units/stable/`, or the relative forms `candidate/`, `stable/`). Reference other rules and specs by `rule_id` or concept name instead — rule files do not encode layer, so the reference stays valid before and after promote. Candidate paths break after promote (candidate files are deleted) and stable paths point to the prior-consensus layer during an active round. If found → FAIL with quoted path and line reference.

6. **Content boundary:** apply `framework/spec_writing_guide.md` §15 to the rule body. Rules carry shared constraints, so the observability test is consumer-side: a rule states what its consuming units must observe or respect — not how an implementation satisfies it. The mission states the rule's consumer set as a plan-time naming fact (derived from current-layer unit `rule_refs`); judge the boundary against that set, and when a judgment needs a consumer spec's content, report the missing read ref instead of assuming it. The rule body must stay within §15's three clauses (§15.1 current truth only, §15.2 externally observable only, §15.3 not stated elsewhere) → FAIL on a violation (actionable: restate as the constraint the consumer must respect, or delete the history — version control owns it). Rule-body content whose §15.2 boundary is undeclared (disposition 3) → FAIL (needs_decision per §15.2). §15 is the single authoritative definition; this step cites it and does not restate it.

**PASS:** Rule body is clear, internally consistent, verifiable, and within the §15 content boundary.

**WARNING:** Constraint is too vague to verify mechanically (step 3 only). WARNING does not affect the overall validate result — a validate with no FAIL checks passes, cache is written, and the WARNING is recorded in the result body.

**FAIL:** Contradictory exceptions, self-contradicting statements, constraint nullified by exceptions, layer-prefixed spec paths in the body, a §15 content-boundary violation (step 6), or a §15.2 boundary left undeclared (step 6, needs_decision).

**Check method:** Content reasoning — the agent reads and evaluates rule prose.

---

### Write cache (tooling finalize)

After all 6 checks complete:

==ATOM_BEGIN:cache_evidence_path_forms==
**Recording cache evidence — tool-generated:** gate cache entries are never agent-declared. `gate-finalize` records one entry per file of the run's input surface (the union of every coverage key's read refs and the cross-synthesis surface), each carrying the whole-file `hash` and the ordered content-defined chunk sequence of the current bytes, plus a per-check marker (check key + lens; failure status on failure records). Reports carry no `Dependency scope` lines; there is nothing for a reviewer to declare and nothing to validate against `read_refs`.

Spec objects that are resolved by name (a dependency unit's main spec or appendix, a rule file) are recorded as **logical references** instead of physical paths — `unit:{name}` for a unit main spec, `unit:{name}:appendix:{file}` for a unit appendix (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file. The run's own target files (the unit's own main spec and appendices; for a rule target, the candidate rule file and its stable sibling) and code files keep physical paths. A logical reference resolves at freshness time to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose recorded content is unchanged (see `framework/validation_cache.md` §Logical References).
==ATOM_END:cache_evidence_path_forms==

- **If all PASS:** the coverage run writes `docs/specs/meta/validation/rule/{id}/validate_result.md` per `framework/validation_cache.md` format:
  - `gate-finalize` creates `docs/specs/meta/validation/rule/{id}/` as needed
  - Collect dependency evidence for the rule file read during validation:
    - The candidate rule file itself (all checks), and its stable sibling when it exists
  - `gate-finalize` assembles the cache from the accepted reports and the run input surface: one entry per input-surface file (whole-file hash + ordered chunk sequence; the rule file and its stable sibling) and one marker per executed rule check (`"1"`–`"6"`; see `framework/validation_cache.md` §Format). On the normal path, consumer units are derived from current-layer unit `rule_refs` and reach the session as a name set rather than as derived input-surface entries: a rule without consumers remains valid, and a consumer-side edit does not stale the cache by itself. A consumer unit the run adds with `specflowctl gate-extend` (Check 4 / Check 6 recovery; §Severity handling) is recorded like any extra input, so a later edit to that unit stales that cache.
  - The `gate-finalize` write produces `validate_result.md` with `result: pass`, `target: candidate`, `mode: full`, and the complete input-surface evidence (whole-file hash + ordered chunk sequence per file).
  - Targeted runs (`:check-{n}` / `:{keyword}`) never write a cache, and a targeted run that FAILs deletes a pass cache (a failure record is kept — it is already blocking and is the recovery baseline) — any FAIL at any granularity means promote must not proceed — see `framework/validation_cache.md`

### Stable-only mode

When no candidate rule exists (validate against stable), run all 6 checks against the **stable** rule file using §Target-layer applicability:

1. Read the stable rule: `docs/specs/rules/stable/{rule_id}.md`
2. Run all 6 checks with the stable condition of Check 3: validate the stable path. The optional owner existence check (Check 4 scanning `docs/specs/units/`) is a live path fact read at check time, not a recorded dependency.
3. **PASS** → `gate-finalize` writes the validate cache with `target: stable` (confirmation state consumed by `fresh@stable`; same coverage sequence as Write cache)
4. **FAIL** → `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check; rules have no cross-check; full runs have no `carried` — assembled by `gate-finalize` from the accepted session reports), present the findings, and recommend forking the rule (or reconciling the consumer binding) — do not edit the stable rule directly

The stable confirmation cache grants no promote eligibility (stable has no gate). A stale stable confirmation cache is restored by the full confirmation run; a failure record or a MISSING stable cache also needs the full run — see `framework/verification_scope.md` §Stable-only Targets and §Delta Runs → Layer applicability.

- **If any FAIL (full run, candidate):** `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check; rules have no cross-check; full runs have no `carried`) — the failure-recovery baseline. Promote must not proceed. The recovery path is the full confirmation run.

### No partial run for rules

Rule validate has no delta and no repair mode. The rule file is the rule's own content and its only external input is the global rule set, so the tooling cannot carry a rule's checks across a change; recovery is always the full confirmation run (`validate@{rule}`). A stale cache, a failure record, and a MISSING cache all resolve the same way.
