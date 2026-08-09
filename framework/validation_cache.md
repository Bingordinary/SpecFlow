# Validation Cache Lifecycle

## Purpose

Cache files record the result and content-addressed dependency evidence (whole-file hash + dependency chunk CIDs) of the last `validate` or `verify` run. They are not a state machine — they do not determine what happens next. They only answer: "were these files checked and were they passing at that time?"

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
    deps:
      - sha256:cdef01...
  - path: src/auth/login.go
    hash: sha256:def456...
    deps:
      - sha256:7890ab...
      - sha256:3456cd...
---
Free-form summary of the result.
```

Each `files` entry records two kinds of evidence:

- **`hash`** — the whole-file content hash at run time. Informational only:
  it detects changes outside the declared dependency chunks so fresh reports
  can warn about possible semantic coupling without failing the gate.
- **`deps`** — the content identifiers (CIDs) of the chunks the run actually
  depended on. **Freshness is judged on `deps` only.** Content changes inside
  the declared dependency chunks stale the cache; content changes elsewhere
  in the same file do not.

`deps` is produced by `specflowctl gate-evidence --file <path> --ranges <lines>`
(see [Dependency Declaration](#dependency-declaration)). A `files` entry with
no `deps` but a non-empty file fails closed (see Staleness Detection).

`files` paths are resolved against the repository root. Dependency matching
(staleness checks and baseline recording) uses the canonical repo-relative
path, so `./` prefixes, absolute paths, and platform separators in the
recorded path are equivalent.

### Logical References

Cross-unit and rule dependencies — any dependency object resolved by name: a dependency unit main spec read by unit `validate` Check 7, a protocol appendix of a dependency unit read by unit `validate` Check 7, a rule file read by unit `validate` Check 8, or a unit spec file scanned by rule `validate` consumer discovery — are recorded as **logical references** instead of physical paths:

```yaml
files:
  - path: unit:auth            # logical reference — resolves to the current-layer unit main spec
    hash: sha256:def456...
    deps:
      - sha256:7890ab...
      - region:acceptance_items:sha256:111222...   # structural region dependency
  - path: unit:auth:appendix:unit_auth_account_token_claims  # logical reference — resolves to the current-layer protocol appendix
    hash: sha256:def456...
    deps:
      - sha256:3456cd...
  - path: rule:g_rule_http     # logical reference — resolves to the current-layer rule file
    hash: sha256:abc123...
    deps:
      - sha256:3456cd...
```

A logical reference resolves at freshness-check time to the **current-layer** file (candidate first, stable fallback), and the recorded `hash`/`deps` come from the file the run actually read. This makes a promote of the referenced unit or rule (which deletes the candidate file without changing content) **not** stale caches whose dependency content is unchanged — a physical path would fail closed the moment the candidate file disappears. The appendix form (`unit:{name}:appendix:{file}`) takes the full appendix file base name without the `.md` extension (`unit_auth_account_token_claims`), resolved the same way (candidate first, stable fallback); the unit name is contextual. Logical references are allowed only for spec objects resolved by name (`unit:`, `unit:{name}:appendix:`, `rule:`); physical paths remain mandatory for every other entry (the unit's own main spec and appendices — the promote appendix gate keys on those physical paths — code files, constraints). An unresolved logical reference (no candidate and no stable file) fails closed.

### Structural Region Dependencies

Chunk CIDs are the chunk-boundary granularity (~2 KB, content-defined): a small file is a single chunk, so a line-range declaration on it degenerates to the whole file. For spec content whose semantic granularity is finer than a chunk — the acceptance item set of a dependency unit — a **structural region dependency** is used instead:

- `specflowctl gate-evidence --file <path> --acceptance-items` emits `region:acceptance_items:<cid>`, the content identifier of the `acceptance_item_set` region (from the marker to the next top-level heading).
- Freshness re-locates the region by structure (the marker and heading, not line numbers) and compares the region's CID. Edits outside the region — even inside the same content-defined chunk — do not stale the cache; edits inside it do.
- Region dependencies are the precise declaration mode for cross-unit checks, which depend on a dependency unit's acceptance item set and not on its prose. Rule files and protocol appendices are contract files (the whole file is the carrier) and keep whole-file declarations.

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
    deps:
      - sha256:7890ab...
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
    deps:
      - sha256:7890ab...
---
Review found no P0/P1 findings.
```

## Chunking and Content Identifiers

Each file is split into content-defined chunks (CDC) before any freshness
comparison:

1. Read the file content as UTF-8 text
2. Normalize line endings: `\r\n` → `\n`, then standalone `\r` → `\n`;
   ensure a trailing `\n` (append if missing)
3. Split the normalized text into content-defined chunks using a rolling
   hash (Rabin-style, 64-byte window; the boundary mask grows with chunk
   size so boundaries are content-driven and average ~2 KB)
4. Compute the SHA-256 of each chunk's bytes; format as `sha256:<hex>` —
   this is the chunk's **content identifier (CID)**
5. The whole-file hash is the SHA-256 of the entire normalized text,
   formatted the same way

**A CID is content identity, not position.** A chunk's CID changes if and
only if its content changes — inserting or deleting content elsewhere in the
file does not move or renumber the depended-on chunks (boundaries re-align
only near the edit, then resynchronize). This is what makes dependency
evidence stable across unrelated edits to shared files.

The normalization is the same one used by `specflowctl review` input
fingerprints. It guarantees cross-platform consistency regardless of git's
autocrlf settings or the agent's operating system.

## Dependency Declaration

Freshness is judged on the chunks the run actually depended on. The agent
declares those dependencies when writing the cache:

1. During the validate/verify/review run, keep track of which files were
   read and which line ranges of each file the judgment actually depended on
   (1-based, inclusive; e.g. `auth.go:120-180`).
2. For each such file, run
   `specflowctl gate-evidence --file <path> --ranges <lines>` (no `--ranges`
   means the whole file). It outputs the whole-file `hash` and the `deps`
   block — the CIDs of the chunks the declared ranges overlap.
3. Record both in the cache's `files` entry.

The declared ranges are a means to an end: **only the CIDs are recorded.**
Line numbers are never persisted, so later insertions/deletions cannot
invalidate the evidence as long as the depended-on content itself is
unchanged.

The chunk boundary is the mechanical granularity line: a declared range maps
to whole chunks, so content adjacent to the range inside the same chunk is
included automatically. The agent does not need line precision — it declares
what it read, the CLI maps it to chunks.

**Declaration rules (declare-heavy principle):** the agent is responsible
for declaring the ranges its judgment depended on, including regions it
references (called functions, shared structures). The cost of declaring too
much is a possibly-redundant re-run; the cost of declaring too little is a
false-fresh cache. When unsure whether content influenced a judgment,
declare it. During `verify`/`review`, ranges must cover every function or
structure whose behavior the alignment judgment relies on.

**Known limit:** the framework cannot verify that the agent's judgment
depended only on the declared ranges. Under-declaration can produce a
false-fresh cache that the mechanical gate cannot detect. The gate reports
changes outside the declared dependencies as an informational note rather
than a failure; treat the note as a prompt to re-run when semantic coupling
exists.

## Write Rules

### Agent writes

| Event | Action |
|-------|--------|
| `validate@{unit}` full PASS | Write `validate_result.md` with `mode: full`, and `hash` + `deps` evidence for every file read (via `specflowctl gate-evidence`) |
| `validate` FAIL / blocked | Delete `validate_result.md` if it exists |
| `verify@{unit}` full PASS (all aligned) | Write `verify_result.md` with `result: pass`, severity counts at 0, `mode: full`, `blocking: false`, and `hash` + `deps` evidence for every file read |
| `verify@{unit}` full PASS (P2/P3 non-blocking findings) | Write `verify_result.md` with `result: pass`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `mode: full`, and `hash` + `deps` evidence. Promote may proceed |
| `verify` FAIL (any P0/P1 findings) | Delete `verify_result.md` if it exists. Agent must stop, not proceed to promote |
| `review@{unit}` full PASS | Write `review_result.md` with `mode: full`, `blocking: false`, `hash` + `deps` evidence for every file read, and findings body |
| `review@{unit}` full FAIL (P0/P1 found) | Write `review_result.md` with `mode: full`, `blocking: true`, includes finding counts and findings body |
| Targeted run (`:check-{n}` / `:{keyword}`) PASS | Report findings only — do NOT write any cache. A targeted run never writes a cache (the promote gate accepts only full-run caches, and writing would not downgrade an existing full cache) |
| Targeted run (`:check-{n}` / `:{keyword}`) FAIL (P0/P1) | Delete the cache regardless of prior state. P0/P1 at any granularity means promote must not proceed |


### Cache lifecycle

| Event | Action |
|-------|--------|
| `specflowctl promote` succeeds | Delete `validate_result.md`, `verify_result.md`, and `review_result.md` |
| `specflowctl promote` with review cache | When review cache is missing, stale, or `blocking: true`, promote is rejected with guidance. |
| A declared dependency chunk changes (CID no longer present) | Cache becomes stale — detected at promote time |
| Content changes outside the declared dependency chunks | Cache stays fresh; fresh reports and promote print an informational note |
| Targeted run when full cache exists | Does NOT touch the full cache — a targeted run never writes or downgrades (see below) |
| Targeted run FAILs (P0/P1 findings) | Deletes cache even if a full cache existed |
| Targeted run PASSes (P2/P3 findings or all aligned) | Does NOT write cache — reports findings; existing full cache stays valid |

### Targeted-run rule

A targeted run (`:check-{n}` / `:{keyword}`) never writes a cache, so it cannot invalidate an already-passed full verification needed for promote.

**Exception (blocking):** If a targeted run FAILs (P0/P1 findings), it deletes the cache regardless of prior state. P0/P1 at any granularity means promote must not proceed.

## Staleness Detection

`specflowctl promote --unit <name>` reads the three cache files and the appendix cache, then checks:

1. **Mode check** — `mode` must be `full`. Fail closed: a missing or invalid mode value cannot prove a full run. Targeted runs never write caches, so any cache with a non-`full` mode is invalid.
2. **Dependency check** — re-chunks every listed file and verifies each declared `deps` CID still exists in the file's current chunk set. If any declared dependency chunk is gone, the file's dependency changed and the cache is stale. A file with content but no declared `deps` (pre-content-addressed cache) also fails closed. A missing file fails the check regardless of `deps`.
3. **Verify result check** — `verify_result.md` with `result: pass` passes (P2/P3 pending items are carried by the severity counts). Any other result value is rejected. `result: fail` never appears in a verify cache: P0/P1 findings delete the cache instead of writing it.
4. **Review cache check (required)** — `review_result.md` must exist, mode must be `full`, must not be `blocking: true`, and the dependency check must pass. If any condition fails, promote is rejected with guidance.
5. **Appendix cache check** — reads the validate cache and verifies every non-exempt candidate appendix file is listed in the validate cache's file list. If any non-exempt appendix on disk is missing from the cache's file list, the appendix was not validated and promote is rejected with guidance to run `validate@{unit}`.
6. **Main file check** — the validate and verify cache file lists must include the main candidate spec file (`docs/specs/units/candidate/unit_{name}.md`), and the rule validate cache must include the candidate rule file (`docs/specs/rules/candidate/{rule_id}.md`). A cache whose file list omits the main file cannot prove that file was read during the run, so promote is rejected with guidance to re-run the corresponding check.

When the dependency check passes but the whole-file hash differs from the
recorded `hash` (content changed outside the declared dependency chunks),
promote and fresh reports print an informational note naming the files. The
note never fails the gate — it is a prompt to re-run when semantic coupling
exists.

`specflowctl promote --rule <id>` enforces cache freshness — reads the validate cache, rejects if missing or stale. Rule verify cache is no longer required (rule verify has been removed).

### Review cache promote check (required gate)

The review cache at `docs/specs/meta/validation/unit/{name}/review_result.md` is a hard prerequisite for promote. All conditions must pass:

1. **Existence check** — if the file does not exist, promote is rejected: "Review not completed. Run `review@{unit}` first."
2. **Mode check** — `mode` must be `full`. If not, promote is rejected: "review cache mode is %q, expected 'full' — run `review@{unit}` before promoting."
3. **Dependency check** — re-chunks every listed file and verifies each declared `deps` CID still exists. If a declared dependency chunk changed or a file is missing, the cache is stale and promote is rejected: "Review cache is stale. Run `review@{unit}` again."
4. **Blocking check** — if `blocking: true`, promote is rejected: "Review found {p0_count} P0 and {p1_count} P1 finding(s). Resolve before promoting."
5. **Blocking declaration check** — `blocking` is a required field on every review cache. A cache without it fails closed (the gate cannot determine blocking status): promote is rejected: "review cache missing required field `blocking` — cannot determine blocking status".
6. **Result value check** — `result` must be `pass` or `fail`. Any other value is rejected.
7. **Consistency check** — `result: fail` must declare `blocking: true` and `result: pass` must declare `blocking: false`. A conflicting declaration is rejected: the cache was written incorrectly and its blocking status cannot be trusted.

## Freshness Check (read-only)

`specflowctl fresh` (agent triggers `fresh@{target}` / `fresh@candidate` / `fresh@stable` / `fresh@all`) reports freshness without executing any check:

- **`specflowctl fresh`** (alias of `--scope candidate`) — summary for every unit and rule with a candidate file. One row per target with per-gate status and the overall `READY FOR PROMOTE: N of M` count.
- **`specflowctl fresh --scope stable`** — summary for every stable unit and rule. One row per target with its drift state (see Stable Drift Baseline below). Stable targets have no promote gate and are never counted in `READY FOR PROMOTE`.
- **`specflowctl fresh --scope all`** — candidate summary and stable summary in one report. `READY FOR PROMOTE` covers the candidate section only.
- **`specflowctl fresh --unit <name>`** / **`--rule <id>`** — detail for one target. A target that exists only in stable (no candidate file) reports the stable drift detail instead of candidate gate statuses.

The gate vocabulary is `FRESH` / `STALE` / `MISSING` / `BLOCKED` (review with P0/P1) / `OK` (appendix). Classification reuses the same checks as `specflowctl promote` (Staleness Detection above), so a fresh report and a promote run never disagree. For a retiring unit only the validate gate is reported, matching promote's gate set.

`fresh` is strictly read-only: it never writes or deletes caches or baselines and never triggers validate/verify/review. Its purpose is operational visibility while iterating on multiple units that share files — but a shared-file change stales a cache **only when it falls inside the declared dependency chunks**. Changes outside the declared dependencies keep the cache fresh and surface as an informational note (see Staleness Detection above): visibility is carried by the note, not by an over-broad STALE verdict. Cross-unit and rule dependencies declared as logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`) stay fresh across a promote of the referenced target when the dependency content is unchanged.

## Stable Drift Baseline

Promote records a **baseline**: a snapshot of the promoted target's code surface. Baselines live under `docs/specs/meta/baseline/` (`unit/{name}.yaml`, `rule/{id}.yaml`), are written by `specflowctl promote` (and removed when the target is retired), and are kept after promote deletes the caches. The baseline is a data snapshot, not a state machine — "drift" is never persisted, it is recomputed on every read. Baselines are durable records with the same lifecycle as the stable spec and must be kept under version control; only `docs/specs/meta/validation/` (the caches) is meant to be ignored.

- **Unit baseline:** the files declared by `implementation_surface` (directories expanded recursively) and `affects.files`. For each surface file the baseline records the whole-file hash and, when the promote-time verify run declared dependencies on that file, the dependency chunk CIDs (copied from the verify cache, which must be fresh for promote). A fresh `verify@{unit}` against stable silences the comparison: the code was recently confirmed to still conform.
- **Rule baseline:** the stable rule file itself (a rule declares no code surface) — it detects direct edits to a stable rule that bypass the fork flow. Rule baselines keep the whole-file hash comparison: rules have no verify run, so no dependency CIDs exist.

`fresh`'s stable scope compares the current code surface against the baseline:

| State | Meaning |
|---|---|
| `VERIFIED` | A fresh stable verify cache exists — the code was recently confirmed to still conform. Silences the baseline comparison. |
| `OK` | No verify cache; the code surface matches the baseline. For unit files with declared dependency CIDs, "matches" means every declared dependency chunk still exists — content changes outside the declared chunks do not fail the check. Such changes surface as an informational note ("content changed outside declared dependencies — re-verify if semantic coupling exists") instead. Files without declared dependency CIDs (including all legacy baselines written before dependency support) are judged on the whole-file hash. |
| `CHANGED` | The surface differs from the baseline: a declared dependency chunk is gone, a hash-only file changed, or files are missing or added (the report names them). The spec may have drifted; confirmation requires `verify@{unit}` against stable. |
| `MISSING` | No baseline recorded (the target was promoted before baseline support). |

The report states what it mechanically knows: `CHANGED` means "code changed since promote" — it never claims the spec is violated. Semantic confirmation is always a user-triggered `verify@{unit}` against the stable target. The dependency-CID judgment is the same approximation the promote gate already accepts for caches: a note is a prompt to re-run when semantic coupling exists, never a failure.

## Important

Cache is never refreshed automatically. Only the agent writing a new cache after a fresh validate/verify changes it. This is because validate and verify are semantic operations that require AI judgment — they cannot be reduced to a mechanical freshness comparison.

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
