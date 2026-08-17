## Unified Report Skeleton

All quality-gate reports (validate, verify, review) share the same report skeleton below. The header lines (`Result`, `Blocking promote`, `Key counts`) and the `Next step` line are identical across commands; only the body content and the Findings section entries' detail lines are command-specific and defined in each checklist file.

```
────────────────────────────────────────────
{command}@{target} · {mode} · {layer}
Result: PASS | FAIL
Blocking promote: yes | no
Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)
────────────────────────────────────────────
{body}
────────────────────────────────────────────
Dependency scope:
  {check key}: {file}: {declaration}   # per executed check; declaration = section heading text, line ranges, or "all"
────────────────────────────────────────────
Findings:
  [{severity}] {location} — {issue} (actionable | needs_decision)
  {command-specific detail lines, indented}
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{command}@{target}` — the command and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`, `review@user_auth`. Commands: `validate`, `verify`, `review`. Targets: unit or rule name.
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for incremental re-runs (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`).
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `Result` — `PASS | FAIL` for all three commands. The gate is decided by severity: FAIL means P0/P1 findings exist. validate grades findings P0/P1 only, so any validate FAIL is a P0/P1 finding; verify/review FAIL means P0/P1 mismatches/findings exist.
- `Blocking promote: yes | no` — `yes` when P0/P1 findings exist (the run FAILs); `no` otherwise. Valid for all three commands.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the total finding count, a/b/c/d the count per severity. validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. Command-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, review's suppressed-by-spec count) appear in the body.
- `{body}` — command-specific content defined in this file's body format section (validate: one line per check; verify: Items / Scope / Integrity / Coverage / first-principles divergence analysis; review: Architecture assessment and suppressed findings).
- `Findings:` — one entry per finding in the unified format `[{severity}] {location} — {issue} (actionable | needs_decision)`. `actionable` — a concrete repair can be made without user judgment (verify: direction spec_gap/code_gap; review: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (verify: direction needs_design/blocked; review: architecture trade-offs). Command-specific detail (verify's suggested direction, review's spec_context/recommendation, validate's contradicting information sources) appears as indented detail lines under the entry. Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive.
- `Dependency scope:` — one line per check the run executed: `{check key}: {file}: {declaration}`. `{check key}` is the command's check identifier (validate: `check-{n}`; verify: the acceptance item id; review: the batch/file dimension). `{declaration}` is the section-region heading text the check's judgment read (e.g. `Description`, `Testability / Acceptance Criteria`; the frontmatter region is `frontmatter`), 1-based closed line ranges (e.g. `120-180,300-320`), or `all` when the judgment covered the whole file. In full runs, every read-only subagent reports this scope for the checks it executed and the main agent carries it over verbatim; in single-executor flows, the executor reports its own scope for the checks it executed. The main agent uses it when writing the validation cache — section headings become `--section` declarations, line ranges become `--ranges`, `all` is a whole-file declaration — and records the per-check breakdown in the cache's `checks` mapping (see `framework/validation_cache.md` §Format → Per-check evidence). Delta runs report the scope of the re-run checks only — carried-over checks are not re-executed and get no new declaration (see `framework/verification_scope.md` §Sub-agent Prompt Assembly Check scope). Targeted runs may omit it.
- `Incremental scope:` — delta runs only (mode `delta`). One line per re-run check in the run's own structure (e.g. validate: "check-5 (acceptance coverage & correctness): re-run — section `Description` of the unit's own spec changed"), followed by a line declaring the carried-over checks ("checks 1-4, 6-8: carried over — their dependency evidence is unchanged") and the cross-check result (unit targets — rules have no cross-check). The scope is mechanism-derived from the cache's per-check evidence: stale regions map to the checks that declared them (see `framework/verification_scope.md` §Delta Runs). For a failure-record recovery (`basis: repair` — the re-run recovers a delta FAIL's failure record or a full-run FAIL's record), the re-run set is the record's failed checks plus the newly affected ones and the cross-check; carried-over checks are the record's `pass`/`carried` entries (see `framework/verification_scope.md` §Delta Runs → Failure recovery). A failure record without a per-check status map (legacy) degrades the recovery to a full re-run — nothing is carried over. The incremental scope must be reported before execution begins (the user must see what will be re-run and what will be carried over) and again in the final report.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. Guidance: fixes applied → "fixes applied — re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}` / `review@{target}:{keyword}`) to confirm"; all gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}".

**Targeted runs:** end the report with the command's targeted note ("This was a targeted check — no cache was written. Run `{command}@{target}` for a complete ...") after the `Next step` line.
