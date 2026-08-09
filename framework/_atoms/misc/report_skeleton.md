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
  {file}: {lines}   # 1-based closed line ranges the judgment depended on; "all" for whole-file judgments
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
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs.
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `Result` — `PASS | FAIL` for all three commands. The gate is decided by severity: FAIL means P0/P1 findings exist. validate grades findings P0/P1 only, so any validate FAIL is a P0/P1 finding; verify/review FAIL means P0/P1 mismatches/findings exist.
- `Blocking promote: yes | no` — `yes` when P0/P1 findings exist (the run FAILs); `no` otherwise. Valid for all three commands.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the total finding count, a/b/c/d the count per severity. validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. Command-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, review's suppressed-by-spec count) appear in the body.
- `{body}` — command-specific content defined in this file's body format section (validate: one line per check; verify: Items / Scope / Integrity / Coverage / first-principles divergence analysis; review: Architecture assessment and suppressed findings).
- `Findings:` — one entry per finding in the unified format `[{severity}] {location} — {issue} (actionable | needs_decision)`. `actionable` — a concrete repair can be made without user judgment (verify: direction spec_gap/code_gap; review: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (verify: direction needs_design/blocked; review: architecture trade-offs). Command-specific detail (verify's suggested direction, review's spec_context/recommendation, validate's contradicting information sources) appears as indented detail lines under the entry. Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive.
- `Dependency scope:` — one line per file the run's judgment actually depended on: `{file}: {lines}` with 1-based closed line ranges (e.g. `120-180,300-320`), `all` when the judgment covered the whole file. In full runs, every read-only subagent reports this scope for the files it read and the main agent carries it over verbatim; in single-executor flows, the executor reports its own scope for the files it read. The main agent uses it when writing the validation cache (`--ranges`, see `framework/validation_cache.md` §Dependency Declaration). Targeted runs may omit it.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. Guidance: fixes applied → "fixes applied — re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}` / `review@{target}:{keyword}`) to confirm"; all gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}".

**Targeted runs:** end the report with the command's targeted note ("This was a targeted check — no cache was written. Run `{command}@{target}` for a complete ...") after the `Next step` line.
