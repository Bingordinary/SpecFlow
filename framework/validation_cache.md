# Validation Cache Lifecycle

## Purpose

Cache files record the result and file content hashes of the last `validate` or `verify` run. They are not a state machine — they do not determine what happens next. They only answer: "were these files checked and were they passing at that time?"

This file is referenced by `framework/concepts.md` §3.

## File Locations

### Unit

- `docs/specs/meta/validation/unit/{name}/validate_result.md`
- `docs/specs/meta/validation/unit/{name}/verify_result.md`
- `docs/specs/meta/validation/unit/{name}/review_result.md`

### Rule

- `docs/specs/meta/validation/rule/{id}/validate_result.md`
- (Rule verify cache has been removed — rule does not need verify)

## Format

YAML frontmatter + markdown body:

```yaml
---
command: validate            # or verify
unit: user_auth
mode: full                   # always full — targeted runs (:check-{n}, :{keyword}) do not write caches
result: pass                 # validate/verify: pass; review: pass | fail
target: candidate            # (verify only) candidate | stable
blocking: false              # (verify) informational: P0/P1 findings never write a cache, so always false; P2/P3 pending items carried by counts
p0_count: 0                  # (verify) severity counts; P2/P3 pending items when > 0
p1_count: 0
p2_count: 1
p3_count: 0
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_user_auth.md
    hash: sha256:abc123...
  - path: src/auth/login.go
    hash: sha256:def456...
---
Free-form summary of the result.
```

`mode: full` means a complete run of all checks/steps. Only full runs write caches — targeted runs (`:check-{n}` / `:{keyword}`) never do, so `mode` is always `full`. See `framework/verification_scope.md` for the full design.

### Verify result semantics

The verify cache uses the same `pass` / `fail` vocabulary as validate and review, at the gate level:

- `result: pass` — no P0/P1 blocking findings. May carry P2/P3 pending items via `p2_count`/`p3_count` (with `blocking: false`).
- `result: fail` — reserved vocabulary definition. P0/P1 findings never write a cache (the cache is deleted), so a fail-result verify cache does not occur in normal operation; if one is present, promote rejects it.

The per-item ALIGNED / MISMATCH / CANNOT_DETERMINE verdicts in the verify report are finding-level vocabulary and are unrelated to the cache `result` field.

### Review cache

```yaml
---
command: review
unit: user_auth
mode: full                  # always full
result: fail                # pass | fail
p0_count: 0
p1_count: 1
p2_count: 2
p3_count: 0
blocking: true              # required on every review cache: true if P0 or P1 findings exist
target: candidate           # candidate | stable
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: src/auth/login.go
    hash: sha256:def456...
---
## Findings

### P1 - src/auth/login.go:42 — Missing input validation on email field
  The email field lacks input validation, potential XSS risk.
  spec_context: Spec prioritizes shipping speed over input sanitization (accepted_tradeoff)
  recommendation: Add input validation middleware

### P2 - src/auth/config.go:88 — Hardcoded secret key
  Secret key is hardcoded instead of using environment variable.
  recommendation: Use os.Getenv() to load from environment
```

A PASS review cache follows the same shape with `result: pass`, `blocking: false`, and
all severity counts at 0:

```yaml
---
command: review
unit: user_auth
mode: full                  # always full
result: pass                # pass | fail
p0_count: 0
p1_count: 0
p2_count: 0
p3_count: 0
blocking: false             # required on every review cache: false when no P0/P1 findings exist
target: candidate           # candidate | stable
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: src/auth/login.go
    hash: sha256:def456...
---
Review found no P0/P1 findings.
```

## Hash Algorithm

Each file hash is computed as:

1. Read the file content as UTF-8 text
2. Normalize line endings: `\r\n` → `\n`, then standalone `\r` → `\n`
3. Ensure trailing newline (append `\n` if missing)
4. Compute SHA-256 of the normalized content
5. Format as `sha256:<hex>` (e.g. `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`)

This is the same normalization used by `specflowctl review` input fingerprints. It guarantees cross-platform consistency regardless of git's autocrlf settings or the agent's operating system.

## Write Rules

### Agent writes

| Event | Action |
|-------|--------|
| `validate@{unit}` full PASS | Write `validate_result.md` with `mode: full`, hashes of all read files |
| `validate` FAIL / blocked | Delete `validate_result.md` if it exists |
| `verify@{unit}` full PASS (all aligned) | Write `verify_result.md` with `result: pass`, severity counts at 0, `mode: full`, `blocking: false`, hashes of all read files |
| `verify@{unit}` full PASS (P2/P3 non-blocking findings) | Write `verify_result.md` with `result: pass`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `mode: full`, hashes of all read files. Promote may proceed |
| `verify` FAIL (any P0/P1 findings) | Delete `verify_result.md` if it exists. Agent must stop, not proceed to promote |
| `review@{unit}` full PASS | Write `review_result.md` with `mode: full`, `blocking: false`, hashes of all read files, and findings body |
| `review@{unit}` full FAIL (P0/P1 found) | Write `review_result.md` with `mode: full`, `blocking: true`, includes finding counts and findings body |
| Targeted run (`:check-{n}` / `:{keyword}`) PASS | Report findings only — do NOT write any cache. A targeted run never writes a cache (the promote gate accepts only full-run caches, and writing would not downgrade an existing full cache) |
| Targeted run (`:check-{n}` / `:{keyword}`) FAIL (P0/P1) | Delete the cache regardless of prior state. P0/P1 at any granularity means promote must not proceed |


### Cache lifecycle

| Event | Action |
|-------|--------|
| `specflowctl promote` succeeds | Delete `validate_result.md`, `verify_result.md`, and `review_result.md` |
| `specflowctl promote` with review cache | When review cache is missing, stale, or `blocking: true`, promote is rejected with guidance. |
| File content changes (hash mismatch) | Cache becomes stale — detected at promote time |
| Targeted run when full cache exists | Does NOT touch the full cache — a targeted run never writes or downgrades (see below) |
| Targeted run FAILs (P0/P1 findings) | Deletes cache even if a full cache existed |
| Targeted run PASSes (P2/P3 findings or all aligned) | Does NOT write cache — reports findings; existing full cache stays valid |

### Targeted-run rule

A targeted run (`:check-{n}` / `:{keyword}`) never writes a cache, so it cannot invalidate an already-passed full verification needed for promote.

**Exception (blocking):** If a targeted run FAILs (P0/P1 findings), it deletes the cache regardless of prior state. P0/P1 at any granularity means promote must not proceed.

## Staleness Detection

`specflowctl promote --unit <name>` reads the three cache files and the appendix cache, then checks:

1. **Mode check** — `mode` must be `full`. Fail closed: a missing or invalid mode value cannot prove a full run. Targeted runs never write caches, so any cache with a non-`full` mode is invalid.
2. **Hash check** — re-computes SHA-256 hashes of every listed file and compares against stored hashes. If any hash differs or a file is missing, the cache is stale and promote is rejected with guidance.
3. **Verify result check** — `verify_result.md` with `result: pass` passes (P2/P3 pending items are carried by the severity counts). Any other result value is rejected. `result: fail` never appears in a verify cache: P0/P1 findings delete the cache instead of writing it.
4. **Review cache check (required)** — `review_result.md` must exist, mode must be `full`, must not be `blocking: true`, and hashes must match. If any condition fails, promote is rejected with guidance.
5. **Appendix cache check** — reads the validate cache and verifies every non-exempt candidate appendix file is listed in the validate cache's file list. If any non-exempt appendix on disk is missing from the cache's file list, the appendix was not validated and promote is rejected with guidance to run `validate@{unit}`.
6. **Main file check** — the validate and verify cache file lists must include the main candidate spec file (`docs/specs/units/candidate/unit_{name}.md`), and the rule validate cache must include the candidate rule file (`docs/specs/rules/candidate/{rule_id}.md`). A cache whose file list omits the main file cannot prove that file was read during the run, so promote is rejected with guidance to re-run the corresponding check.

`specflowctl promote --rule <id>` enforces cache freshness — reads the validate cache, rejects if missing or stale. Rule verify cache is no longer required (rule verify has been removed).

### Review cache promote check (required gate)

The review cache at `docs/specs/meta/validation/unit/{name}/review_result.md` is a hard prerequisite for promote. All conditions must pass:

1. **Existence check** — if the file does not exist, promote is rejected: "Review not completed. Run `review@{unit}` first."
2. **Mode check** — `mode` must be `full`. If not, promote is rejected: "review cache mode is %q, expected 'full' — run `review@{unit}` before promoting."
3. **Hash check** — re-computes SHA-256 hashes of every listed file and compares against stored hashes. If any hash differs or a file is missing, the cache is stale and promote is rejected: "Review cache is stale. Run `review@{unit}` again."
4. **Blocking check** — if `blocking: true`, promote is rejected: "Review found {p0_count} P0 and {p1_count} P1 finding(s). Resolve before promoting."
5. **Blocking declaration check** — `blocking` is a required field on every review cache. A cache without it fails closed (the gate cannot determine blocking status): promote is rejected: "review cache missing required field `blocking` — cannot determine blocking status".
6. **Result value check** — `result` must be `pass` or `fail`. Any other value is rejected.
7. **Consistency check** — `result: fail` must declare `blocking: true` and `result: pass` must declare `blocking: false`. A conflicting declaration is rejected: the cache was written incorrectly and its blocking status cannot be trusted.

## Important

Cache is never refreshed automatically. Only the agent writing a new cache after a fresh validate/verify changes it. This is because validate and verify are semantic operations that require AI judgment — they cannot be reduced to a mechanical hash check.

**Cache serves the promote gate only.** During iteration, an expired cache is the normal state — fixes applied after a validate/verify/review run make the cache stale, and the agent must NOT re-run quality-gate commands to restore freshness. Executing validate, verify, or review (including re-runs after a fix) is user-triggered only (see HARD RULE 2 in `framework/concepts.md`); the agent guides the user to a targeted re-check (`:check-{n}` / `:{keyword}`) or a concrete full command and waits for the user. The only way a cache becomes fresh again is a user-triggered validate/verify/review run.

## Cache File Access Strategy

When you need to read a cache file, use this ordered strategy:

1. **Explicit path (preferred):** construct the full known path
   `docs/specs/meta/validation/{kind}/{name}/{file}` and read it directly.
   This is the most reliable method and works in any agent environment.

2. **Fallback search:** if the exact path is unknown, search for the file.
   Note that some search tools may not descend into directories starting
   with `_`. If the search returns no results despite knowing the file
   exists, scope the search explicitly to `docs/specs/meta/validation/`
   (rather than searching from a broader root).
