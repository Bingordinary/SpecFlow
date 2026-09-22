# Rule Validate Checklist

`validate@{rule}` is the rule path of `validate`. It checks rule metadata structural validity (Checks 1-7) and rule body quality (Check 8).
Agent runs this when the target is detected as a Rule via automatic type detection (see `framework/commands.md` §Target Resolution).

**Result:** PASS writes `docs/specs/meta/validation/rule/{id}/validate_result.md`.
A candidate full-run FAIL does not write cache (the validate cache is deleted — trust establishment failed). A delta/repair FAIL or a stable-only FAIL writes a failure record — see §Delta re-run and §Stable-only mode. Report per §Output Format.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `validate@{rule}` | full | All 8 checks. Quality checks are holistic — always runs full. |
| `validate@{rule}:check-{n}` | targeted | Single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@{rule}:{keyword}` | targeted | Match keyword to check name. User explicitly chooses focus. Does not write a cache. |

## Execution Rules

- **Subagent permissions:** rule validate executes in an independent read-only sub-agent session (the validate shape of `framework/verification_scope.md` §Sub-agent Prompt Assembly — rules have no cross-check, so the Check scope is "all 8 checks"). The sub-agent may read rule files, search text patterns, check file existence; it must NOT modify files, execute commands (beyond read-only tools), or delegate to other agents. Targeted runs (`:check-{n}` / `:{keyword}`) execute directly in the main agent session instead (see §Targeted Runs in `framework/verification_scope.md`).
- On FAIL: identify which checks failed and the contradictory information
- Resolution types (each finding is labeled with one):
  - **actionable** — A concrete repair can be made inside the current candidate rule file without user judgment.
  - **needs_decision** — Requires user input (unclear intent, missing decision, or external dependency). Stop and ask.

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

All quality-gate reports (validate, verify, review) share the same report skeleton below. The header lines (`Result`, `Blocking promote`, `Key counts`), the Findings block fields, and the `Next step` line are identical across commands; only the body content and the command-specific extra lines inside each finding block are command-specific and defined in each checklist file. The Findings section follows the header because on FAIL it is the decision surface — the body, Dependency scope, and Severity check sections are its audit and evidence.

```
────────────────────────────────────────────
{command}@{target} · {mode} · {layer}
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
    {command-specific detail lines}
    ref: {anchor or line reference}                # optional, tracking only
────────────────────────────────────────────
{body}
────────────────────────────────────────────
Dependency scope:
  {check key}: {file}: {declaration}   # per executed check; declaration = section heading text, acceptance item id list (acceptance_item:<id>), the whole-set token acceptance_items, line ranges, or "all"
────────────────────────────────────────────
Severity check:
  confirmed: N | adjusted: N
  Severity confirmation: {finding_id} = confirmed {Px} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = adjusted {Px} -> {Py} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = confirmed {Py} — evidence: {file}; reason: {required final second check after an adjustment}
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{command}@{target}` — the command and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`, `review@user_auth`. Commands: `validate`, `verify`, `review`. Targets: unit or rule name.
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for incremental re-runs (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`).
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `Result` — `PASS | FAIL` for all three commands. For packet runs this value is generated by `gate-finalize` from the accepted synthesis result (or from the single rule-validate packet, which has no cross packet); the coordinator never supplies it. The gate is decided by severity: FAIL means retained P0/P1 findings exist. validate grades findings P0/P1 only, so any validate FAIL is a P0/P1 finding; verify/review FAIL means retained P0/P1 mismatches/findings exist.
- `Blocking promote: yes | no` — `yes` when P0/P1 findings exist (the run FAILs); `no` otherwise. Valid for all three commands.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the retained finding count and a/b/c/d are generated by `gate-finalize`; the coordinator never supplies them. validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. Command-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, review's suppressed-by-spec count) appear in the body.
- `{body}` — command-specific content defined in this file's body format section (validate: one line per check; verify: Items / Scope / Integrity / Coverage / first-principles divergence analysis; review: Architecture assessment and suppressed findings).
- `Findings:` — `Findings: none` when no finding is retained; otherwise one entry per finding: a unified entry line `[{severity}] {location} — {issue} (actionable | needs_decision)` followed by the finding block's required detail lines. `actionable` — a concrete repair can be made without user judgment (verify: direction spec_gap/code_gap; review: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (verify: direction needs_design/blocked; review: architecture trade-offs). The block is written for a reader who has not opened the spec or the code: delete every `ref:` line and every `§`, line-number, or other anchor reference from the field text, and the reader must still learn what is wrong, what the conflicting sides say, what is at risk, and what to fix or decide. Anchors appear only in the optional trailing `ref:` line, which carries tracking information and no meaning.
  - `problem:` — one sentence naming the concrete subject (acceptance item id, field or parameter name, behavior) and what is wrong with it; never an anchor, region name, or pointer.
  - `evidence:` — the quoted content that proves the claim, as one or more indented `- {verbatim quoted source content} — {source label}` sub-lines. Contradiction findings quote both conflicting sides verbatim; undefined or missing behavior lists the defined part and the missing part; spec-vs-code findings quote the declared behavior and the implemented behavior. A review P3 finding uses its `fact_anchor:` line as this evidence form.
  - `impact:` — what goes wrong, or stays undecidable, if the finding is not resolved.
  - `fix:` — actionable findings: the concrete repair action. `decision:` — needs_decision findings: the question the user must answer, with `options:` sub-lines listing the candidate choices.
  - The block's detail lines are contiguous indented lines directly under the entry line — no blank lines inside the block (the block is stored and re-rendered verbatim when a finding is carried into a delta/repair run).
  - `{command-specific detail lines}` — command-specific extras follow the shared fields: verify's `root_cause:`, `direction:`, and `confidence:`; review's `spec_context:` and its mechanically required `fact_anchor:` for P3 findings. validate and rule validate add none.
  - Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive. Batch-group entries carry the same fields; terse one-line values are fine.
- `Dependency scope:` — one line per check the run executed: `{check key}: {file}: {declaration}`. `{check key}` is the command's check identifier (validate: `check-{n}`; verify: the acceptance item id; review: the packet's file path). `{declaration}` is the section-region heading text the check's judgment read (e.g. `Description`, `Testability / Acceptance Criteria`; the frontmatter region is `frontmatter`), `acceptance_item:<id>[,<id>...]` (one or more acceptance item regions of the spec's `acceptance_item_set` — the precise declaration for a judgment over specific items, e.g. one verify item judgment), the reserved token `acceptance_items` (the whole `acceptance_item_set` structural region — for judgments over the set as a whole, e.g. validate's acceptance coverage check), 1-based closed line ranges (e.g. `120-180,300-320`), or `all` when the judgment covered the whole file. Every read-only subagent reports this scope for the checks in its packet; the report is submitted verbatim via `gate-submit`, which validates the declarations against that packet's `read_refs` — not merely the run-wide snapshot — (path membership and declaration parseability), and `gate-finalize` computes the CIDs and records the per-check breakdown in the cache's `checks` mapping (see `framework/validation_cache.md` §Format → Per-check evidence). Delta/repair runs report the scope of the re-run checks only — carried-over checks are not re-executed and get no new declaration (see `framework/verification_scope.md` §Sub-agent Prompt Assembly Check / Packet scope). Targeted runs may omit it.
- `Severity check:` — packet-run reports use the exact `Severity confirmation:` line grammar owned by `framework/verification_scope.md` §Gate Work Packets → Packet report contract. Unit cross reports carry one complete sequence for every terminal retained finding after disposition/merge resolution: one `confirmed` record, or an `adjusted` record followed by exactly one final record for the adjusted severity. `confirmed: N | adjusted: N` counts findings by final sequence outcome, not record lines. Rule validate reports carry the confirmation sequences required by their command checklist. `gate-submit` parses and validates these records; `gate-finalize` uses the resulting canonical severities. Targeted reports retain the command checklist's human-readable severity trace but do not create packet state.
- `Incremental scope:` — delta runs only (mode `delta`). One line per re-run check in the run's own structure (e.g. validate: "check-5 (acceptance coverage & correctness): re-run — section `Description` of the unit's own spec changed"), followed by a line declaring the carried-over checks ("checks 1-4, 6-8: carried over — their dependency evidence is unchanged") and the cross-check result (unit targets — rules have no cross-check). The scope is mechanism-derived at plan time from the cache's per-check evidence: `gate-plan` maps stale regions to the checks that declared them, adds the current keys the baseline never declared (a new acceptance item or review file has no evidence to carry, so it executes like a stale judgment), and generates the re-run packet set (see `framework/verification_scope.md` §Delta Runs). For a failure-record recovery (`basis: repair` — the run recovers a delta FAIL's failure record or a full-run FAIL's record), the plan's packet set is the record's failed checks plus its persisted `invalidated_checks` (written by `gate-invalidate` after a targeted P0/P1), the newly affected checks, the current keys the baseline never declared, any explicit `--rerun` overrides, and the cross-check; carried-over checks are the remaining `pass`/`carried` entries (see `framework/verification_scope.md` §Delta Runs → Failure recovery). A failure record whose per-check status map is absent or incomplete (legacy or malformed), or whose invalidated key cannot map to the current judgment surface, degrades the plan to the full packet set — nothing is carried over. When the re-run covers every declared check, the plan covers the full scope — nothing is carried over. The incremental scope is reported by `gate-plan` before execution begins (the user must see what will be re-run and what will be carried over) and again in the final report.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. A finding's fix lifecycle has three states with fixed wording: `finding_open` → "Resolve the findings, then re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}` / `review@{target}:{keyword}`) to confirm"; `fixed_pending_recheck` → "Fixes applied; re-run the target-appropriate re-check command to confirm." — only after the approved fix was actually written; `verified` → "Re-check passed." — only after a re-check confirmed the fix. A gate report is always produced before any fix is applied (nothing is implemented before the user approves the findings), so an actionable finding's report-time `Next step` is always the `finding_open` wording. Other guidance: all gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}"; nothing further → `None`.

**Targeted runs:** end the report with the command's targeted note ("This was a targeted check — no complete cache was written. Run `{command}@{target}` for a complete ...") after the `Next step` line. If the result contains P0/P1, run `gate-invalidate` before reporting completion.

### Completion — Persist Gate Cache

A quality-gate run is complete only when its result is persisted and visible to `fresh`/`promote`. A report alone does not satisfy the gate.

- **Full (`validate@{target}` / `verify@{unit}` / `review@{unit}`, `mode: full`, `basis: full`)** — complete only when the packet run finished and its cache was written: (1) `specflowctl gate-plan --gate {validate|verify|review} (--unit {name} | --rule {id}) --target {candidate|stable} [--input PATH_OR_REF]...` ran **before any executor read input** (before prompt assembly / sub-agent launch) — `--input` adds evidence that every packet may read but never creates a work packet; (2) every required detection/check/review/analysis report was submitted via `specflowctl gate-submit --run <run_id> --packet <packet_id> --report PATH` and accepted (verify analysis packets are `not_required` when their detection packet is not `MISMATCH`); (3) the cross packet consumed the accepted packet results and was accepted (unit targets only; rule validate has one complete packet and no cross); (4) the coordinator ran `specflowctl gate-finalize --run <run_id>` and the cache `docs/specs/meta/validation/{unit|rule}/{name}/validate_result.md` | `verify_result.md` | `review_result.md` was written; (5) `fresh@{target}` (`fresh --unit {name}` / `fresh --rule {id}`) shows the expected gate state (`FRESH` for `result: pass` / `blocking: false`, `BLOCKED` for `result: fail` / `blocking: true`). A candidate validate **full-run** FAIL is the exception: `gate-finalize` deletes any existing validate cache and writes no failure record; candidate validate delta/repair FAIL writes the failure record required for recovery. A finalize that finds any input changed since `gate-plan` is rejected and writes no cache. See `framework/validation_cache.md` §Write Rules and §Failure handling by gate role.
- **Delta (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`, `mode: full`, `basis: delta` or `basis: repair`)** — the same packet sequence with `specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record): the plan derives the re-run packet set mechanically and records the carried-over checks; `gate-finalize` merges the carried evidence from the baseline cache. See `framework/verification_scope.md` §Delta Runs.
- **Targeted (`:check-{n}` / `:{keyword}`, no complete cache)** — complete when the report is produced and, for P0/P1, the coordinator has run `specflowctl gate-invalidate` for every affected check key. Targeted runs intentionally do NOT create a gate run — `gate-plan` / `gate-submit` / `gate-finalize` are not run. A targeted PASS leaves cache state unchanged. A targeted P0/P1 deletes a pass cache or persists `invalidated_checks` on a failure record and invalidates any matching open run; it never publishes a complete result cache or satisfies the promote gate. The report ends with `This was a targeted check — no complete cache was written. Run {command}@{target} for a complete ...`.

**Self-check:** `ls docs/specs/meta/validation/{unit|rule}/{name}/` and `fresh@{target}` (unit: `fresh --unit {name}`, rule: `fresh --rule {id}`).
==ATOM_END:report_skeleton==

### Body format (rule validate)

One line per check, numbered as in this file:

```
1. Frontmatter completeness: PASS | FAIL — reason
2. ID/Scope consistency: PASS | FAIL — reason
3. File path consistency: PASS | FAIL — reason
4. Version semantics: PASS | FAIL — reason
5. Promotion owner unit: PASS | WARNING — reason
6. Prohibited fields: PASS | FAIL — reason
7. Unbound retention correctness: PASS | FAIL — reason
8. Rule body quality: PASS | WARNING | FAIL — reason
Failed checks: N | Advisory findings: K
```

**Counting rules:**

- `Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the total number of distinct findings across all FAIL checks; a/b/c/d the count per severity. rule validate grades findings P0/P1 only — P1 is the contract-decided default, P0 requires severity confirmation per `framework/severity_policy.md` §9 (see Severity check below) — so `c` and `d` are always 0. In targeted runs, only executed checks are counted.
- `Failed checks` is the number of FAIL checks among executed checks, shown in the body's check lines. WARNING is not a failed check.
- WARNING findings (Check 5 / Check 8 step 3) are presented on their check line's reason and counted separately as `Advisory findings: K` in the body — they are never counted in `Findings` and never affect `Failed checks`.
- `Blocking promote` is `yes` when P0/P1 findings exist (rule validate FAIL blocks promote; a candidate full-run FAIL deletes the validate cache, while a delta FAIL or a stable-only FAIL writes a failure record).

Rule validate findings are always presented flat — rules have no batch classification; each FAIL finding is listed directly under the `Findings:` section in the unified finding format `[{severity}] {location} — {issue} (actionable | needs_decision)`, followed by the shared finding block (`problem:` / `evidence:` / `impact:` / `fix:` or `decision:` — see §Output Format).

### Severity check

rule validate grades findings P0/P1. P1 is the contract-decided default for every FAIL check and needs no record unless it is the result of adjusting a P0 confirmation. A P0 grade is judgment-based and the single checks packet must carry a complete mechanically parsed confirmation sequence before its result can be accepted: read the impact surface the P0 claim depends on (the consumer units or the governance file governing the affected mechanism), verify the §9.3 boundary, and publish the exact record syntax owned by `framework/verification_scope.md` §Gate Work Packets → Packet report contract. A confirmed P0 completes the sequence; an adjusted `P0 -> P1` result requires exactly one final record for P1. Every evidence path must be in the packet's read refs and dependency scope. The records appear in the report's Severity check section after the body, so the trace shows the check ran.

```
Severity check:
  confirmed: N | adjusted: N
  Severity confirmation: {finding_id} = confirmed P0 — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = adjusted P0 -> P1 — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = confirmed P1 — evidence: {file}; reason: {required final second check after an adjustment}
```

## Checklist

### Check 1 — Frontmatter Completeness

Verify the rule file has all required frontmatter fields:

| Field | Required | Valid values |
|-------|----------|-------------|
| `rule_id` | Yes | `g_rule_{name}` or `b_rule_{name}` |
| `rule_scope` | Yes | `global` or `bound` |
| `rule_version` | Yes | `x.y.z` (semver) |

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

Verify the rule file is in the correct layer directory: the file being validated must be at `docs/specs/rules/candidate/{rule_id}.md`. The rule layer is encoded by the file path — no `layer` frontmatter field is declared (see `framework/spec_writing_guide.md` §6).

Filenames follow the pattern `{g_or_b}_rule_{id}.md` — `g_rule_` for global rules, `b_rule_` for bound rules.

### Check 4 — Version Semantics

If this is a brand-new rule (no stable file exists): verify `rule_version` equals `0.1.0`.

If a stable sibling exists (`docs/specs/rules/stable/{rule_id}.md`): read the stable file's frontmatter, extract its `rule_version`, and verify the candidate `rule_version` is semantically greater (MAJOR.MINOR.PATCH comparison). If candidate version is not greater than stable version → FAIL.

### Check 5 — `promotion_owner_unit` (optional documentation field)

`promotion_owner_unit` is an optional documentation field with no effect on tooling behavior. This check produces no execution failure.

→ WARNING if present but the value does not name a unit that exists in `docs/specs/units/`.

### Check 6 — Prohibited Fields

Verify the rule file does NOT contain:
- `bound_objects` — rule files must not store consumer lists
- A consumer list in any form — consumers are derived from unit `rule_refs`, never stored in rule files

If either is found → FAIL.

### Check 7 — `unbound_retention` Correctness

If the rule is a bound shared rule (`b_rule_`) AND has no current consumers: verify the file explicitly records:
- `unbound_retention: intentional`
- `unbound_retention_reason: <why this rule is intentionally independent>`
- `unbound_retention_owner: <flow name>`

If any of the three fields is missing → FAIL.

If the rule has current consumers: verify `unbound_retention` and its related fields are NOT present. If they are present → FAIL.

The `unbound_retention` declaration doubles as the removal exemption: a bound rule with no consumers either declares intentional retention (kept) or is a removal candidate for `specflowctl remove --rule <id>` (see `framework/spec_writing_guide.md` §6.5). Global rules (`g_rule_*`) are skipped — the retention model applies to bound rules only.

**Consumer discovery method:** search for `rule_refs` containing this `rule_id` in current-layer (effective) unit spec files — candidate preferred, stable fallback, the same resolution `specflowctl deps` uses. A stable file whose candidate dropped the reference no longer counts as a consumer.

### Check 8 — Rule Body Quality

**Purpose:** Evaluate whether the rule's body content (constraint definition, exceptions, scope) is internally consistent and clearly stated. Unlike Checks 1-7 (metadata structural validity), this check evaluates the rule's written content.

**Execution steps:**

1. **Constraint clarity:** read the rule file body and identify the core constraint statement. Verify it is unambiguous — a reader can determine what behavior is required, prohibited, or permitted without guessing.

2. **Exception consistency:** read all exception clauses or scope limitations in the body. Verify no exception effectively nullifies the constraint (e.g., a rule saying "all APIs must use HTTPS" with an exception "except when HTTP is used"). If an exception contradicts the constraint → FAIL.

3. **Verifiability:** assess whether the constraint can be verified through static code inspection or verify. A rule like "be intuitive" is not verifiable — flag as WARNING. A rule like "all API handlers must validate input before processing" is verifiable — PASS.

4. **Self-contradiction scan:** scan the body for statements that conflict with each other (e.g., "versions must be in semver format" in one paragraph, "versions are integers" in another). If found → FAIL.

5. **Layer-prefix path check:** scan the rule body for layer-prefixed spec paths (`docs/specs/rules/candidate/`, `docs/specs/rules/stable/`, `docs/specs/units/candidate/`, `docs/specs/units/stable/`, or the relative forms `candidate/`, `stable/`). Reference other rules and specs by `rule_id` or concept name instead — rule files do not encode layer, so the reference stays valid before and after promote. Candidate paths break after promote (candidate files are deleted) and stable paths point to the prior-consensus layer during an active round. If found → FAIL with quoted path and line reference.

**PASS:** Rule body is clear, internally consistent, and verifiable.

**WARNING:** Constraint is too vague to verify mechanically (step 3 only). WARNING does not affect the overall validate result — a validate with no FAIL checks passes, cache is written, and the WARNING is recorded in the result body.

**FAIL:** Contradictory exceptions, self-contradicting statements, constraint nullified by exceptions, or layer-prefixed spec paths in the body.

**Check method:** Content reasoning — the agent reads and evaluates rule prose.

---

### Write cache (tooling finalize)

After all 8 checks complete:

==ATOM_BEGIN:cache_evidence_path_forms==
**Declaring cache evidence — path forms:** spec objects resolved by name other than the run's own target files are declared as **logical references** instead of physical paths — `unit:{name}` for a unit main spec, `unit:{name}:appendix:{file}` for a unit protocol appendix (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file — with the `hash` + `deps` of the file actually read. The run's own target files (the unit's own main spec and appendices; for a rule target, the candidate rule file and its stable sibling) and code files keep physical paths. A logical reference resolves at freshness time to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose dependency content is unchanged (see `framework/validation_cache.md` §Logical References).
==ATOM_END:cache_evidence_path_forms==

- **If all PASS:** the packet run writes `docs/specs/meta/validation/rule/{id}/validate_result.md` per `framework/validation_cache.md` format:
  - `gate-finalize` creates `docs/specs/meta/validation/rule/{id}/` as needed
  - Collect dependency evidence for every file read during validation, including:
    - The candidate rule file itself (all checks)
    - The stable sibling rule file, if present (Check 4 reads its `rule_version`)
    - All unit spec files searched under `docs/specs/units/` (Check 5 existence check and Check 7 consumer discovery)
  - Each packet report declares its files' dependency scope in its `Dependency scope:` lines (`ranges` / `sections` / `acceptance_items`); `gate-submit` validates path membership against that packet's `read_refs` (not merely the run snapshot), and `gate-finalize` computes the `hash` + `deps` evidence from the accepted reports. The values come from the executor's own `Dependency scope` report (see §Output Format); a file reported as `all` (or not reported) is declared as a whole file. The declared ranges must cover every region the validation judgment depended on — when unsure, declare more (declare-heavy principle; see `framework/validation_cache.md` §Dependency Declaration). **The candidate rule file keeps a whole-file declaration** — rule files are contract files, the whole file is the carrier (see `framework/validation_cache.md` §Structural Region Dependencies); the per-check `checks` mapping breaks that whole declaration down per rule check (check key = the agent check number `"1"`–`"8"`; each rule-body check declares the scope its judgment read — see `framework/validation_cache.md` §Format → Per-check evidence → rule validate caches). Unit spec files scanned by Check 7 consumer discovery record the per-check breakdown of which checks consumed them (the consumer-discovery checks, Check 5/7).
  - The `gate-finalize` write produces `validate_result.md` with `result: pass`, `target: candidate`, `mode: full`, file hashes and dependency CIDs, assembled from the accepted packet reports.
  - Targeted runs (`:check-{n}` / `:{keyword}`) never write a cache, and a targeted run that FAILs deletes a pass cache (a failure record is kept — it is already blocking and is the recovery baseline) — any FAIL at any granularity means promote must not proceed — see `framework/validation_cache.md`

### Stable-only mode

When no candidate rule exists (validate against stable), run the same 8 checks against the **stable** rule file and its current consumers:

1. Read the stable rule: `docs/specs/rules/stable/{rule_id}.md`
2. Run all 8 checks — the consumer-discovery checks (Check 5/7 scanning `docs/specs/units/`) are the live part: consumer units may have changed since promote
3. **PASS** → `gate-finalize` writes the validate cache with `target: stable` (confirmation state consumed by `fresh@stable`; same packet sequence as Write cache)
4. **FAIL** → `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check; rules have no cross-check and full runs have no `carried` — assembled by `gate-finalize` from the accepted packet reports), present the findings, and recommend forking the rule (or reconciling the consumer binding) — do not edit the stable rule directly

The stable confirmation cache grants no promote eligibility (stable has no gate). Delta re-runs (`revalidate`) apply to stable-only targets with a usable baseline — a pass cache (STALE recovery) or a failure record (failure-record recovery); a MISSING stable cache needs the full confirmation run — see `framework/verification_scope.md` §Stable-only Targets and §Delta Runs → Layer applicability.

- **If any FAIL (full run, candidate):** delete existing `validate_result.md` if present. Do not write cache.

### Delta re-run (revalidate@{rule})

Candidate targets, plus stable-only targets with a usable baseline — a delta re-run against a stable-only target whose confirmation cache is MISSING reports the missing-baseline message — "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." — and stops (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Triggered by the user when the cache is STALE or BLOCKED (a failure record; most often: a consumer unit spec changed and the consumer-discovery checks no longer have their evidence). Follow §Delta Runs in `framework/verification_scope.md`:

1. **Preconditions** — the existing cache must have `mode: full` and one of the two baselines: `result: pass` (stale-cache recovery) or `result: fail` + `blocking: true` (failure-record recovery, §Failure recovery). A MISSING cache has no baseline — run `validate@{rule}` instead.
2. **Plan (mechanism-derived)** — run `specflowctl gate-plan --gate validate --rule {id} --target {candidate|stable} --mode delta` (or `--mode repair` for a failure record). The planner derives the re-run set from the cache's per-check `checks` mapping — affected = {checks whose declared deps went stale} ∪ {current checks the baseline never declared} ∪ {persisted `invalidated_checks`} ∪ {explicit `--rerun` overrides} (rules have no cross-check) — and degrades conservatively to the full packet set where no per-check association exists or an invalidated key cannot map to the current rule surface. After a targeted P0/P1, the coordinator runs `gate-invalidate --check {check key}` immediately; repair reads that state automatically and never carries the contradicted judgment. The rule file is normally declared whole by every rule-body check, so a rule-file change re-runs every declared check — the plan covers the full scope and carries nothing over. It reports the re-run packet, carried-over checks (`carried_keys`), and any degradation or full-scope coverage statement. The plan is the scope; the executor does not re-derive or extend it.
3. **Execution** — execute the planned packets (protocol: `framework/verification_scope.md` §Gate Work Packets), submitting each report with `specflowctl gate-submit --run <run_id> --packet <packet_id> --report PATH`. Any P0/P1 finding → the finalize writes a **failure record** (`result: fail`, `blocking: true`, severity counts, findings body, `mode: full`, `basis: delta`, per-check `status` map — `fail`/`pass` for the re-run checks, `carried` for the carried-over ones, derived by `gate-finalize` from the accepted packet verdicts). Stop, present findings (same as full FAIL).
4. **On PASS** — `gate-finalize` rewrites `validate_result.md` with `mode: full`, `basis: delta` (or `basis: repair` when recovering from a failure record), `target: candidate` (stable-only target: `target: stable`), a fresh `timestamp`, and a **complete** `files` list: new `hash` + `deps` + per-check `checks` evidence (computed by the tooling from the accepted packet reports) for the re-run checks' files, and the original evidence (including the per-check `checks` breakdown) for the carried-over checks' files, merged by the tooling from the baseline cache (their CIDs are unchanged by construction). Logical references (`unit:{name}` for consumer units) are carried over the same way. The rewritten `GATE_JUDGMENTS` state is also complete: it contains exactly one status for each check `1`–`8`, formed from the disjoint re-run and carried sets, with findings de-duplicated by id, so the cache remains a valid baseline for the next partial delta/repair run.
