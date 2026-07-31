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
mode: full                   # full | scoped
scoped_check: "1"            # present only when mode=scoped (validate)
scoped_item: AUTH-AC-003     # present only when mode=scoped (verify)
result: pass                 # pass | aligned | blocked | mismatch
target: candidate            # (verify only) candidate | stable
blocking: false              # (verify) true if any P0/P1 mismatch found
p0_count: 0                  # (verify) severity counts; present on mismatch
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

`mode: full` means all checks/steps were executed. `mode: scoped` means only a subset was checked (single check via `:check-{n}` for validate, single item for verify). See `framework/verification_scope.md` for the full scoped vs full design.

### Review cache

```yaml
---
command: review
unit: user_auth
mode: full                  # full | scoped
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
mode: full                  # full | scoped
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
| `validate@{unit}:check-{n}` PASS | Write `validate_result.md` with `mode: scoped`, `scoped_check: "{n}"`, hashes |
| `validate@{unit}:full` PASS | Write `validate_result.md` with `mode: full`, hashes of all read files |
| `validate` FAIL / blocked | Delete `validate_result.md` if it exists |
| `verify@{unit}` scoped ALIGNED | Write `verify_result.md` with `mode: scoped`, `scoped_item: "{representative id}"` (first matching item for git-aware mode), `blocking: false`, hashes |
| `verify@{unit}:{keyword}` (matches item) ALIGNED | Write `verify_result.md` with `mode: scoped`, `scoped_item: "{matched id}"`, `blocking: false`, hashes |
| `verify@{unit}:full` ALIGNED | Write `verify_result.md` with `mode: full`, `blocking: false`, hashes of all read files |
| `verify` MISMATCH (any P0/P1) | Delete `verify_result.md` if it exists. Agent must stop, not proceed to promote |
| `verify@{unit}:full` MISMATCH (all P2/P3) | Write `verify_result.md` with `result: mismatch`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `mode: full`, hashes of all read files. Promote may proceed |
| `verify` scoped MISMATCH (all P2/P3) | Report findings only — do NOT write `verify_result.md`. A scoped run never writes a cache (a scoped cache cannot satisfy promote's mode gate, and it must not overwrite or downgrade an existing full cache) |
| `review@{unit}` scoped PASS | Write `review_result.md` with `mode: scoped`, `blocking: false`, hashes of checked files, and findings body |
| `review@{unit}:full` PASS | Write `review_result.md` with `mode: full`, `blocking: false`, hashes of all read files, and findings body |
| `review` scoped or full FAIL (P0/P1 found) | Write `review_result.md` with `mode: {scoped|full}`, `blocking: true`, includes finding counts and findings body |
| Scoped trigger falls back to full (edge case, see `framework/verification_scope.md` §Edge cases) PASS / ALIGNED | Write with `mode: full`, same as explicit `:full` run |


### Cache lifecycle

| Event | Action |
|-------|--------|
| `specflowctl promote` succeeds | Delete `validate_result.md`, `verify_result.md`, and `review_result.md` |
| `specflowctl promote` with review cache | When review cache is missing, stale, scoped, or `blocking: true`, promote is rejected with guidance. |
| File content changes (hash mismatch) | Cache becomes stale — detected at promote time |
| Scoped run when full cache exists | Does NOT downgrade full cache (see scoped-over-full rule below) |
| Scoped run finds P0/P1 MISMATCH | Deletes cache even if full cache existed |
| Scoped run finds only P2/P3 MISMATCH | Does NOT write cache — reports findings; existing full cache stays valid |

### Scoped-over-full rule

A `mode: full` cache is overwritten **only** by another full run. A scoped run (from `:check-{n}`) does not downgrade a full cache to scoped — the full cache stays valid. This ensures that intermediate scoped checks during development do not invalidate an already-passed full verification needed for promote.

**Exception (blocking):** If a scoped run finds a P0 or P1 MISMATCH, it deletes the cache regardless of prior mode. P0/P1 at any granularity means promote must not proceed.

**Exception (non-blocking):** If a scoped run finds only P2/P3 MISMATCH (no P0/P1), it reports the findings but does NOT write the cache — a scoped cache cannot pass promote's mode gate, and writing would downgrade an existing full cache. Only a `:full` P2/P3 mismatch run writes a cache, with `blocking: false` and severity counts. Promote may proceed on that full non-blocking cache.

## Staleness Detection

`specflowctl promote --unit <name>` reads the three cache files and the appendix cache, then checks:

1. **Mode check** — if `mode` is `scoped` (not `full`), the cache is rejected with scope detail: "cache is scoped (check {n} | item: {id}), run full verification before promoting."
2. **Hash check** — re-computes SHA-256 hashes of every listed file and compares against stored hashes. If any hash differs or a file is missing, the cache is stale and promote is rejected with guidance.
3. **Verify result check** — `verify_result.md` with `result: aligned`, or with `result: mismatch` and `blocking: false` (P2/P3 only), passes. A `result: mismatch` with `blocking: true` (P0/P1 findings) is rejected: "verify found {p0_count} P0 and {p1_count} P1 finding(s). Resolve before promoting."
4. **Review cache check (required)** — `review_result.md` must exist, mode must be `full`, must not be `blocking: true`, and hashes must match. If any condition fails, promote is rejected with guidance.
5. **Appendix cache check** — reads the validate cache and verifies every non-exempt candidate appendix file is listed in the validate cache's file list. If any non-exempt appendix on disk is missing from the cache's file list, the appendix was not validated and promote is rejected with guidance to run `validate@{unit}:full`.

`specflowctl promote --rule <id>` enforces cache freshness — reads the validate cache, rejects if missing, stale, or scoped. Rule verify cache is no longer required (rule verify has been removed).

### Review cache promote check (required gate)

The review cache at `docs/specs/meta/validation/unit/{name}/review_result.md` is a hard prerequisite for promote. All conditions must pass:

1. **Existence check** — if the file does not exist, promote is rejected: "Review not completed. Run `review@{unit}:full` first."
2. **Mode check** — if `mode` is `scoped` (not `full`), promote is rejected: "Review cache is scoped, run `review@{unit}:full` before promoting."
3. **Hash check** — re-computes SHA-256 hashes of every listed file and compares against stored hashes. If any hash differs or a file is missing, the cache is stale and promote is rejected: "Review cache is stale. Run `review@{unit}:full` again."
4. **Blocking check** — if `blocking: true`, promote is rejected: "Review found {p0_count} P0 and {p1_count} P1 finding(s). Resolve before promoting."
5. **Blocking declaration check** — `blocking` is a required field on every review cache. A cache without it fails closed (the gate cannot determine blocking status): promote is rejected: "review cache missing required field `blocking` — cannot determine blocking status".
6. **Result value check** — `result` must be `pass` or `fail`. Any other value is rejected.
7. **Consistency check** — `result: fail` must declare `blocking: true` and `result: pass` must declare `blocking: false`. A conflicting declaration is rejected: the cache was written incorrectly and its blocking status cannot be trusted.

## Important

Cache is never refreshed automatically. Only the agent writing a new cache after a fresh validate/verify changes it. This is because validate and verify are semantic operations that require AI judgment — they cannot be reduced to a mechanical hash check.

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
