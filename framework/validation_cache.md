# Validation Cache Lifecycle

## Purpose

Cache files record the result and file content hashes of the last `spec_validate` or `spec_verify` run. They are not a state machine — they do not determine what happens next. They only answer: "were these files checked and were they passing at that time?"

This file is referenced by `framework/concepts.md` §3.

## File Locations

### Unit

- `docs/specs/_validation/unit/{name}/validate_result.md`
- `docs/specs/_validation/unit/{name}/verify_result.md`

### Rule

- `docs/specs/_validation/rule/{id}/validate_result.md`
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
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_user_auth.md
    hash: sha256:abc123...
  - path: src/auth/login.go
    hash: sha256:def456...
---
Free-form summary of the result.
```

`mode: full` means all checks/steps were executed. `mode: scoped` means only a subset was checked (single check for validate, single item for verify). See `framework/verification_scope.md` for the full scoped vs full design.

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
| `spec_validate {unit}` scoped PASS | Write `validate_result.md` with `mode: scoped`, `scoped_check: "1"`, hashes of checked files |
| `spec_validate {unit}:check-{n}` PASS | Write `validate_result.md` with `mode: scoped`, `scoped_check: "{n}"`, hashes |
| `spec_validate {unit}:full` PASS | Write `validate_result.md` with `mode: full`, hashes of all read files |
| `spec_validate` FAIL / blocked | Delete `validate_result.md` if it exists |
| `spec_verify {unit}` scoped ALIGNED | Write `verify_result.md` with `mode: scoped`, `scoped_item: "{representative id}"` (first matching item for git-aware mode), hashes |
| `spec_verify {unit}:{keyword}` (matches item) ALIGNED | Write `verify_result.md` with `mode: scoped`, `scoped_item: "{matched id}"`, hashes |
| `spec_verify {unit}:full` ALIGNED | Write `verify_result.md` with `mode: full`, hashes of all read files |
| `spec_verify` MISMATCH | Delete `verify_result.md` if it exists |
| Scoped trigger falls back to full (edge case, see `framework/verification_scope.md` §Edge cases) PASS / ALIGNED | Write with `mode: full`, same as explicit `:full` run |

### Cache lifecycle

| Event | Action |
|-------|--------|
| `specflowctl promote` succeeds | Delete both `validate_result.md` and `verify_result.md` |
| File content changes (hash mismatch) | Cache becomes stale — detected at promote time |
| Scoped run when full cache exists | Does NOT downgrade full cache (see scoped-over-full rule below) |
| Scoped run finds MISMATCH | Deletes cache even if full cache existed |

### Scoped-over-full rule

A `mode: full` cache is overwritten **only** by another full run. A scoped run does not downgrade a full cache to scoped — the full cache stays valid. This ensures that intermediate scoped checks during development do not invalidate an already-passed full verification needed for promote.

**Exception:** If a scoped run finds a MISMATCH, it deletes the cache regardless of prior mode. A MISMATCH at any granularity means promote must not proceed.

## Staleness Detection

`specflowctl promote --unit <name>` reads both cache files and checks:

1. **Mode check** — if `mode` is `scoped` (not `full`), the cache is rejected with scope detail: "cache is scoped (check {n} | item: {id}), run full verification before promoting."
2. **Hash check** — re-computes SHA-256 hashes of every listed file and compares against stored hashes. If any hash differs or a file is missing, the cache is stale and promote is rejected with guidance.

`specflowctl promote --rule <id>` enforces cache freshness — reads the validate cache, rejects if missing, stale, or scoped. Rule verify cache is no longer required (rule verify has been removed).

## Important

Cache is never refreshed automatically. Only the agent writing a new cache after a fresh validate/verify changes it. This is because validate and verify are semantic operations that require AI judgment — they cannot be reduced to a mechanical hash check.

## Cache File Access Strategy

When you need to read a cache file, use this ordered strategy:

1. **Explicit path (preferred):** construct the full known path
   `docs/specs/_validation/{kind}/{name}/{file}` and read it directly.
   This is the most reliable method and works in any agent environment.

2. **Fallback search:** if the exact path is unknown, search for the file.
   Note that some search tools may not descend into directories starting
   with `_`. If the search returns no results despite knowing the file
   exists, scope the search explicitly to `docs/specs/_validation/`
   (rather than searching from a broader root).
