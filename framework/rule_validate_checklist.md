# Rule Validate Checklist

`rule_validate` is the rule-equivalent of `validate`. It checks rule metadata structural validity (Checks 1-7) and rule body quality (Check 8).
Agent runs this when the target is detected as a Rule via automatic type detection (see `framework/concepts.md` §Automatic Target Type Detection).

**Result:** PASS writes `docs/specs/meta/validation/rule/{id}/validate_result.md`.
FAIL does not write cache. Report per §Output Format.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `validate@ {rule}` | full | All 8 checks. Quality checks are holistic — always runs full. |
| `validate@ {rule}:check-{n}` | targeted | Single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@ {rule}:{keyword}` | targeted | Match keyword to check name. User explicitly chooses focus. Does not write a cache. |

## Execution Rules

- Agent may read rule files, search text patterns, check file existence
- Agent must NOT modify files, execute commands (beyond read-only tools), or delegate to other agents
- On FAIL: identify which checks failed and the contradictory information

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

All quality-gate reports (validate, verify, review) share the same report skeleton below. The header, the `Blocking promote` line, and the `Next step` line are identical across commands; the verdict vocabulary, the key counts, and the body content are command-specific and defined in each checklist file.

```
────────────────────────────────────────────
{command}@{target} · {mode} · {layer}
Result: {verdict}
Blocking promote: yes | no
Key counts: {command-specific counts}
────────────────────────────────────────────
{body}
────────────────────────────────────────────
Findings:
  {batch group / decision group per this file's batch classification, or flat when grouping is inactive}
Summary: {command-specific summary counts}
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{command}@{target}` — the command and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`, `review@user_auth`. Commands: `validate`, `verify`, `review`. Targets: unit or rule name.
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs.
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `{verdict}` — each command keeps its own verdict vocabulary on the unified line: validate — `PASS | FAIL` (unit targets add the `(fix_required | blocked)` resolution defined in `framework/unit_validate_checklist.md`); verify — `PASS | FAIL`; review — `PASS | FAIL`.
- `Blocking promote: yes | no` — `yes` when the result blocks promote (validate FAIL; verify P0/P1 findings; review P0/P1 findings); `no` otherwise.
- `{command-specific counts}` — validate: `Failed checks: N / Total findings: M / Advisory findings: K`; verify: `Blocking mismatches: N / Non-blocking mismatches: N`; review: `Findings: N (P0: a | P1: b | P2: c | P3: d)`.
- `{body}` — command-specific content defined in this file's body format section (validate: one line per check; verify: Items / Scope / Integrity / Coverage / first-principles divergence analysis; review: Architecture assessment and suppressed findings).
- `Findings:` — the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive.
- `Summary:` — command-specific final counts.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. Guidance: fixes applied → "fixes applied — re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}` / `review@{target}:{keyword}`) to confirm"; all gates green → "if the design is finalized, run `promote@{target}`"; blocked → "awaiting your decision on {item}".

**Targeted runs:** end the report with the command's targeted note ("This was a targeted check — no cache was written. Run `{command}@{target}` for a complete ...") after the `Next step` line.
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
```

**Counting rules:**

- `Failed checks` is the number of FAIL checks among executed checks. WARNING is not a failed check.
- `Total findings` is the total number of distinct findings across all FAIL checks. In targeted runs, only executed checks are counted.
- WARNING findings are presented on their check line's reason and counted separately as `Advisory findings: K` — they are never counted in `Total findings` and never affect `Failed checks`.
- `Blocking promote` is `yes` on any FAIL (validate FAIL blocks promote; a FAIL also deletes the validate cache).

Rule validate findings are always presented flat — rules have no batch classification; each FAIL finding is listed directly under the `Findings:` section.

## Checklist

### Check 1 — Frontmatter Completeness

Verify the rule file has all required frontmatter fields:

| Field | Required | Valid values |
|-------|----------|-------------|
| `rule_id` | Yes | `g_rule_{name}` or `b_rule_{name}` |
| `rule_scope` | Yes | `global` or `bound` |
| `layer` | Yes | `candidate` or `stable` |
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

Verify the rule file is in the correct directory:

| layer | Directory |
|-------|-----------|
| `candidate` | `docs/specs/rules/candidate/` |
| `stable` | `docs/specs/rules/stable/` |

Filenames follow the pattern `{g_or_b}_rule_{id}.md` — `g_rule_` for global rules, `b_rule_` for bound rules.

If path does not match the declared layer → FAIL.

### Check 4 — Version Semantics

If the rule is marked for retirement (`status: retired` in frontmatter), the version gate is skipped: the stable copy is removed on promote, not updated — PASS with the note "rule is marked retired — version comparison skipped". The other retirement requirements still apply: `status` must be the literal `retired`, and no current-layer unit may explicitly reference the rule in `rule_refs` — the mechanical Check 7 rejects a retiring rule that still has explicit consumers, for bound and global rules alike (a global rule's default applicability to every unit lifts automatically when its stable file disappears, so only explicit references block the retire). The retiring rule must not have consumers.

Otherwise:

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

If the rule is marked for retirement (`status: retired` in frontmatter), the `unbound_retention` fields are not required: the rule is being removed from stable, not retained, so the retention declaration has no object. The mechanical Check 7 only verifies that no current-layer unit still explicitly references the rule in `rule_refs` — pass with the note "rule is marked retired — no rule_refs references remain" when no explicit reference remains (bound and global rules alike; a global rule's default applicability to every unit lifts automatically when the stable rule file disappears, so only explicit references block the retire).

Otherwise:

If the rule is a bound shared rule (`b_rule_`) AND has no current consumers: verify the file explicitly records:
- `unbound_retention: intentional`
- `unbound_retention_reason: <why this rule is intentionally independent>`
- `unbound_retention_owner: <flow name>`

If any of the three fields is missing → FAIL.

If the rule has current consumers: verify `unbound_retention` and its related fields are NOT present. If they are present → FAIL.

**Consumer discovery method:** search for `rule_refs` containing this `rule_id` in unit spec files under `docs/specs/units/`.

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

### Write cache

After all 8 checks complete:

- **If all PASS:** write `docs/specs/meta/validation/rule/{id}/validate_result.md` per `framework/validation_cache.md` format. Include `result: pass`, `mode: full`, file hashes. Targeted runs (`:check-{n}` / `:{keyword}`) never write a cache, and a targeted run that FAILs deletes the existing cache — any FAIL at any granularity means promote must not proceed — see `framework/validation_cache.md`.

- **If any FAIL:** delete existing `validate_result.md` if present. Do not write cache.
