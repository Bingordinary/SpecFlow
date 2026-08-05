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
