# Validation Cache Lifecycle

## Purpose

Cache files record the result and content-addressed dependency evidence (whole-file hash + dependency chunk CIDs) of the last `validate` or `verify` run. They are not a state machine — they do not determine what happens next. They only answer: "were these files checked and were they passing at that time?" — not "who executed the check" (see §Dependency Declaration → Known limit). A failure record (a delta/repair re-run's or a candidate verify full-run FAIL's fail cache) answers the negative variant: "were these files checked and which judgments failed?" — the per-check status map is the failure-recovery baseline (see §Write Rules and `framework/verification_scope.md` §Delta Runs → Failure recovery).

This file is the cache-lifecycle command package named by the trigger routing table in `framework/concepts.md`.

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
mode: full                   # complete results are always full; targeted runs never publish a complete cache
basis: full                  # audit metadata: full | delta | repair (full = full run; delta = re* incremental recovery from a pass baseline; repair = re* recovery from a failure record)
result: pass                 # pass | fail (fail = a failure record: a delta/repair re-run found P0/P1 and recorded them instead of deleting the cache; a candidate verify full-run FAIL also writes one — see §Failure handling by gate role)
target: candidate            # the layer the run checked: candidate | stable (all three commands record it; stable-only runs — @stable confirmation checks — write target: stable; the review gate separates the two cache sets by this field — the fresh stable report's review confirmation requires target: stable, see framework/verification_scope.md §Stable-only Targets)
blocking: false              # required on every fail/blocking-capable cache (validate/verify failure records and review caches): true iff result: fail (P0/P1 findings); pass caches may omit it (absent = not blocking)
p0_count: 0                  # (verify) severity counts; P2/P3 pending items when > 0
p1_count: 0
p2_count: 1
p3_count: 0
timestamp: "2026-06-30T10:00:00Z"
gate_run: 20260916-120000-3f9a1c   # audit: the gate run whose input snapshot this cache was finalized against (see §Write Rules → Tooled writes); never gated
invalidated_checks:                # failure records only; machine-owned targeted P0/P1 invalidations, omitted when empty
  - auth.login
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
<!-- GATE_JUDGMENTS_BEGIN
{"schema_version":2,"logical_status":{"1":"pass","cross":"pass"},"findings":[],"synthesis_digest":"sha256:..."}
GATE_JUDGMENTS_END -->
Generated human-readable summary of the result.
```

The `GATE_JUDGMENTS` block is the machine-readable baseline for delta/repair synthesis. Schema version 2 records the complete effective logical-status map, canonical retained findings, their complete source/affected-key sets, each finding's canonical renderable detail, and the accepted synthesis digest. `gate-finalize` generates it; agents never edit it. For rule validate delta/repair runs, `gate-finalize` combines the disjoint carried baseline judgments and re-run packet judgments, so the rewritten block still contains all eight logical check statuses; carried and current findings are de-duplicated by finding id. A check that appears in both sets rejects finalization as corrupted run state. For unit gates, a merge group contributes its terminal retained finding once with the union of the group's logical keys. `gate-plan` copies carried judgments from this block into the new run so cross sees the complete logical result set, including the original finding detail needed for evidence-backed retention, suppression, or merge. `gate-finalize` renders every retained carried finding into the new human-readable body exactly once. A cache without the current judgment schema cannot supply carried semantic results or a complete findings body and therefore cannot be used for a partial run; the planner requires a full run instead of guessing from prose.

**Failure record:** a run that finds P0/P1 writes a **failure record** instead of deleting the cache — a delta or repair re-run (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`) for all three gates, the candidate verify full FAIL, the blocking cache for review (full and delta FAIL), and the confirmation-cache FAIL for stable-only targets (see §Write Rules and §Failure handling by gate role). It carries `result: fail`, `blocking: true`, the severity counts, a findings body, and the same `files`/`checks` evidence as a pass cache plus a `status` per check. It is a valid cache file: fresh and promote report it as `BLOCKED` and promote rejects it — but unlike a deleted cache it remains the **failure-recovery baseline**. A later targeted P0/P1 does not rewrite that historical status map: `specflowctl gate-invalidate` records the contradicted judgment in the machine-owned `invalidated_checks` list. After the findings are resolved, the repair plan re-checks the failed checks, persisted invalidated checks, newly affected checks, current keys the baseline never declared, and any explicit `--rerun` override, then carries the rest over (see `framework/verification_scope.md` §Delta Runs → Failure recovery). A failure record written by a full run (candidate verify full FAIL, review full FAIL, stable-only confirmation FAIL) declares `status` on every judgment — `pass`/`fail`, no `carried` (the full run re-executed all of them).

**Gate run binding:** `gate_run` is the audit id of the gate run whose input snapshot this cache was finalized against (see §Write Rules → Tooled writes). It identifies the run's fixed inputs — it is not executor identity and proves nothing about the execution shape (see §Dependency Declaration → Known limit (execution shape)). `fresh` and `promote` never read it, and caches written before the field existed remain valid (absent = no run recorded).

Each `files` entry records two kinds of evidence:

- **`hash`** — the whole-file content hash at run time. Informational only:
  it detects changes outside the declared dependency chunks so fresh reports
  can warn about possible semantic coupling without failing the gate.
- **`deps`** — the content identifiers (CIDs) of the chunks the run actually
  depended on. **Freshness is judged on `deps` only.** Content changes inside
  the declared dependency chunks stale the cache; content changes elsewhere
  in the same file do not.

`deps` is computed by the tooling at `gate-finalize` from the entry's declared
scope (see [Dependency Declaration](#dependency-declaration)). A `files` entry with
no `deps` but a non-empty file fails closed (see Staleness Detection).

`files` paths are resolved against the repository root. Dependency matching
(staleness checks and baseline recording) uses the canonical repo-relative
path, so `./` prefixes, absolute paths, and platform separators in the
recorded path are equivalent.

#### Per-check evidence (`checks`)

For spec files whose judgments differ in read surface, the `files` entry also
carries the per-check breakdown — which check key depended on which CIDs:

```yaml
files:
  - path: docs/specs/units/candidate/unit_user_auth.md
    hash: sha256:abc123...
    checks:
      - check: "1"                        # agent validate check 1 (structural integrity) — its structural scans span the whole body, so declare every region it read (here: the frontmatter region and the Description section; the item region is check 5's)
        deps:
          - region:section::sha256:111... # frontmatter region dep (empty heading)
          - region:section:Description:sha256:444...
      - check: "5"                        # agent validate check 5 (acceptance coverage & correctness) — reads the item region + contract sections
        deps:
          - region:acceptance_items:sha256:222...
          - region:section:Contract:sha256:333...
    deps:                                 # union of all check deps — the promote gate's judgment basis
      - region:section::sha256:111...
      - region:section:Description:sha256:444...
      - region:acceptance_items:sha256:222...
      - region:section:Contract:sha256:333...
```

A `checks` entry may additionally declare `status` — the judgment outcome in a
**failure record**: `pass` (judgment ran and passed), `fail` (judgment ran and
retained at least one finding of any severity — P0–P3; the gate result itself
is still decided by P0/P1), or `carried` (not re-run — evidence unchanged from the pass
baseline; delta/repair-FAIL records only — full-run records have no `carried`). The
status map is the failure-recovery scope input, derived mechanically by
`gate-finalize` from the accepted packet verdicts: the recovery plan
(`gate-plan --mode repair`) re-runs the `fail` checks, derives the newly
affected checks from the declared deps against the current content, and
carries the `pass`/`carried` checks over (see
`framework/verification_scope.md` §Delta Runs → Failure recovery).

**Status declaration contract:** `status` is **required on every fail/blocking
cache** (a failure record) — a fail cache without a per-check status map is an
invalid write, and its recovery degrades to a full re-run (see
`framework/verification_scope.md` §Delta Runs → Failure recovery). A status map
is usable only when it covers exactly the checks the record's `GATE_JUDGMENTS`
baseline holds, every value is `pass`/`fail`/`carried`, and `carried` appears
only in records written by a delta or repair run (a full-run record —
`basis: full` — has no carried judgments). A key missing from either side, an
unknown value, and `carried` in a full-run record are malformed state, and the
recovery degrades to the full packet set instead of guessing. Pass caches may omit `status` (absent = pass), keeping
pre-failure caches valid — the absent-means-pass reading applies only to pass
caches, never to fail/blocking ones.

**Targeted invalidation contract:** `invalidated_checks` is an optional,
sorted, duplicate-free list present only on a failure record. It records
judgments whose recorded `pass`/`carried` result was contradicted later by a
targeted P0/P1 result. The coordinator writes it only through `specflowctl
gate-invalidate`; agents never edit it. `gate-plan --mode repair` unions these
keys into the mechanically derived re-run set before carry-over is computed.
An invalidated key that no longer maps to the current judgment surface makes
the plan degrade to the full packet set instead of being ignored. A successful
`gate-finalize` rewrites the complete cache without the field because every
persisted invalidation in that plan was re-executed; a failed or rejected
finalize leaves the original failure record unchanged. Pass caches must not
carry `invalidated_checks`.

- Check keys are command-specific: validate uses the **agent check number**
  `"1"`–`"8"` (the 8 agent checks of `validate@{unit}`; the mechanical
  `specflowctl validate` Check 9 — region locatability — is a gate-evidence
  support check and takes no per-check declaration); verify uses the
  acceptance item id the judgment aligned (e.g. `auth.login`); review uses the
  reviewed file path the assessment covered. The per-check granularity is
  the mechanism-derived delta scope input (see `framework/verification_scope.md`
  §Delta Runs) — it exists for delta derivation, not for the promote gate.
  The cross-check (unit validate/verify/review) uses the reserved key `"cross"`
  and reports its scope like any other check; its delta re-run is unconditional
  (always part of the affected set), so the key is a recording convention, not
  a derivation input — a `checks` entry with key `"cross"` derives its stale
  deps like any other key (only a stale declared dep puts it in the affected
  list), and omitting it does not change the derived scope. The coverage
  conclusion is the one place the unconditional re-run is material: the
  affected set always includes the cross-check, so a delta run covers every
  declared check exactly when nothing is carried over — a cache declaring
  only `"cross"` covers every declared check too, reported as a full-scope
  re-run. Whether `"cross"` is declared or fresh therefore never changes the
  coverage conclusion.
- **Rule validate caches carry per-check evidence too:** rule check keys are
  the 8 agent checks of `validate@{rule}` (`"1"`–`"8"`; rules have no
  cross-check). The rule file entry keeps its whole-file file-level `deps`
  (the rule file is a contract file — every rule-body check declares the
  chunks it read, per the whole-body rule below), and consumer-discovery
  entries (`unit:{name}` logical references read by Check 5/7) declare the
  checks that consumed them. This makes a consumer-unit change stale exactly
  the checks that read it instead of falling back to file-level scope
  derivation, and gives rule failure records a per-check `status` carrier.
- The file-level `deps` remains the union of all declared check deps (plus
  any undeclared remainder). **The promote gate judges freshness on that
  union only**, so the gate logic is identical with or without `checks`.
- A cache written before per-check evidence (no `checks` field) stays fully
  valid: the gate reads the union `deps` as before, and a delta plan degrades
  to the full packet set (see `framework/verification_scope.md`
  §Delta Runs → Incremental scope).
- `checks` is optional per file: code files and contract files (whole-file
  declarations) may omit it — a single judgment surface has nothing to break
  down. The parser distinguishes the check-level `deps:` block (8-space
  indent, inside a `checks` entry) from the file-level `deps:` block
  (4-space indent).
- Entries without a per-check breakdown (logical references, contract files,
  whole-file declarations, declare-heavy extras) leave their deps **unclaimed**
  by any check. The delta scope derivation reports such entries when they go
  stale and maps them by the command's fixed association; where no fixed
  association exists the plan degrades conservatively to the full packet set —
  they are never silently carried over (see
  `framework/verification_scope.md` §Delta Runs → Incremental scope).
- **Union discipline:** every per-check dep must also appear in the entry's
  file-level `deps` union. The promote gate and the delta scope derivation
  fail closed on a violation — a check dep missing from the union would let
  content the check declared change without staling the cache (false fresh).
  Extra file-level deps beyond the check union are legal (declare-heavy
  conservatism).
- **Whole-body checks declare everything they read:** a check whose steps
  scan the whole file (e.g. validate Check 1 — prose-path hygiene, layer-path
  scan, and section-structure checks run over every section) must declare
  every region its judgment read, not just the frontmatter. Under-declaring
  such a check leaves its evidence stale-safe: an edit to an undeclared
  section does not stale the cache, and promote can proceed on a judgment
  made against older content (false fresh). When unsure whether a region
  influenced a check, declare it (declare-heavy principle).

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
  - path: rule:g_rule_http     # logical reference — global rules resolve only to the stable rule file
    hash: sha256:abc123...
    deps:
      - sha256:3456cd...
```

Logical references preserve the rule object's applicability semantics. `unit:{name}`, `unit:{name}:appendix:{file}`, and bound-rule references (`rule:b_rule_*`) resolve to the **current-layer** file (candidate first, stable fallback). Global-rule references (`rule:g_rule_*`) resolve only to the stable file: a candidate global rule is unpublished design truth and must not constrain units before promotion. The recorded `hash`/`deps` come from the file the run actually read. Current-layer references therefore stay fresh across promotion when dependency content is unchanged, while stable-global references remain bound to the active stable constraint. The appendix form takes the full appendix file base name without the `.md` extension (`unit_auth_account_token_claims`); the unit name is contextual. Logical references are allowed only for spec objects resolved by name (`unit:`, `unit:{name}:appendix:`, `rule:`); physical paths remain mandatory for every other entry (the unit's own main spec and appendices — the promote appendix gate keys on those physical paths — code files, constraints). An unresolved logical reference fails closed; for `rule:g_rule_*`, a candidate file does not satisfy the reference when no stable file exists.

### Structural Region Dependencies

Chunk CIDs are the chunk-boundary granularity (roughly 2–4 KB, growing with chunk size; content-defined): a small file is a single chunk, so a line-range declaration on it degenerates to the whole file. For spec content whose semantic granularity is finer than a chunk, a **structural region dependency** is used instead:

- `specflowctl gate-evidence --file <path> --acceptance-items` emits `region:acceptance_items:<cid>`, the semantic content identifier of the whole `acceptance_item_set` (from the marker to the next `##` heading — the enclosing section's end; `###` and deeper headings belong to their `##` section and never terminate the set — or through the last real line of the file). The CID is computed from the set preamble, when present, and the sorted `(item id, item-region CID)` members. Item order and blank separators are presentation details: reordering otherwise unchanged item blocks does not stale the dependency. Changing an item, adding or removing an item, renaming an id, or changing non-blank set preamble content changes the CID. A missing marker, an empty set, an empty id, or a duplicated id fails closed. Only a `##` heading ends the set: an item separated from the previous one by a `###` subheading stays in the set, while an item placed after the next `##` heading sits outside it. A judgment for which presentation order itself is semantic must declare the containing section, a line range, or `all` instead of `acceptance_items`.
- `specflowctl gate-evidence --file <path> --acceptance-item <id>` emits `region:acceptance_item:<id>:<cid>`, the content identifier of one acceptance item's region: the item's `- id:` line (outside a code fence) through the line before the next `- id:` line, or the end of the acceptance item set, with trailing blank lines excluded. The id is the locator, so reordering items changes no item region (item-level declarations stay fresh) as long as the item set ends at its last item; set-level content that follows the last item but remains inside the set belongs to the last item's region, so moving an item to or from that position changes its CID. Renaming an item id makes the old declaration unlocatable; a missing id, a duplicated id, or an absent marker fails closed. `--acceptance-item` is repeatable; `--items` lists every acceptance item region (id, line range, CID) without declaring anything.
- `specflowctl gate-evidence --file <path> --section <heading>` emits `region:section:<heading>:<cid>`, the content identifier of the section region with that heading text — the frontmatter region (heading `""`, from the file start to the line before the first `##` heading) or one `##` heading section (the heading line through the line before the next `##` heading, or through the last real line for the final section — the file-final newline is not a line; `###` and deeper headings belong to their `##` section). The heading line is part of the region, so renaming a heading changes its CID. `--section` is repeatable; `--sections` lists every section region (heading, line range, CID) without declaring anything.
- Freshness re-locates the region by structure (the marker/item id/heading, not line numbers) and compares the region's CID. Edits outside the region — even inside the same content-defined chunk — do not stale the cache; edits inside it do. A section heading that is missing or duplicated fails closed (the section cannot be located unambiguously); an acceptance item id that is missing or duplicated fails closed the same way.
- Region dependencies are the precise declaration mode for judgments whose read surface is a spec region: cross-unit checks (a dependency unit's acceptance item set — `region:acceptance_items`), own-spec judgments that read specific acceptance items (`region:acceptance_item:<id>` — the default for verify's per-item judgments), and own-spec judgments that read only some sections (e.g. validate Check 5's item region + contract sections — `region:acceptance_items` + `region:section:<heading>`). Rule files and protocol appendices are contract files (the whole file is the carrier) and keep whole-file declarations.

`mode: full` means a complete run of all checks/steps — the judgment set is complete, not a subset. Only complete-coverage runs publish caches: full runs (`validate@{target}` / `verify@{unit}` / `review@{unit}`) and delta runs (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`, see `framework/verification_scope.md` §Delta Runs). Targeted runs (`:check-{n}` / `:{keyword}`) never publish a result cache, so `mode` is always `full`; a targeted P0/P1 may only delete a pass cache or add machine-owned invalidation metadata to a failure record through `gate-invalidate`. The `basis` field distinguishes the three complete-result writers for audit: `basis: full` (or absent) means the cache came from a full run; `basis: delta` means a delta run re-executed the stale judgments and carried the rest over from a pass baseline; `basis: repair` means a delta run recovered from a **failure record** — it re-executed the failed checks, persisted invalidated checks, newly affected checks, current keys the baseline never declared, and explicit `--rerun` overrides, then carried the rest over (see `framework/verification_scope.md` §Delta Runs → Failure recovery).

### Verify result semantics

The verify cache uses the same `pass` / `fail` vocabulary as validate and review, at the gate level:

- `result: pass` — no P0/P1 blocking findings. May carry P2/P3 pending items via `p2_count`/`p3_count` (with `blocking: false`).
- `result: fail` — a **failure record**: a delta or repair re-run (`reverify@{unit}`) found P0/P1 and recorded the failure instead of deleting the cache, or a candidate verify full-run FAIL wrote one (see §Failure handling by gate role). It must declare `blocking: true`; the gate rejects it as `BLOCKED` (promote must not proceed). A fail-result cache without the blocking declarations is an invalid write and fails closed.

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
  problem: the email field is written to the response without input validation.
  evidence:
    - `email := r.FormValue("email")` — src/auth/login.go:42
    - `pass_condition: "invalid email input is rejected with HTTP 400"` — unit_user_auth.md item AUTH-AC-001
  impact: malformed input reaches the response path; the declared rejection behavior is not implemented.
  fix: validate the email field and reject invalid input with HTTP 400.
  spec_context: Spec prioritizes shipping speed over input sanitization (accepted_tradeoff)

### P2 - src/auth/config.go:88 — Hardcoded secret key
  problem: the signing key is a hardcoded string instead of environment-loaded configuration.
  evidence:
    - `const signingKey = "dev-secret"` — src/auth/config.go:88
  impact: the key cannot be rotated per environment.
  fix: load the key with os.Getenv().
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
   size so boundaries are content-driven and average roughly 2–4 KB, larger
   for big files)
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

Freshness is judged on the chunks the run actually depended on. The executor
declares those dependencies in the packet report's `Dependency scope:` lines
(the same declaration grammar `gate-evidence` accepts; a declaration is the
literal `all` (whole file), a line-range list, the reserved token
`acceptance_items` (the spec's whole `acceptance_item_set` structural region),
one or more acceptance item declarations `acceptance_item:<id>[,<id>...]` (the
item regions of that set), or a section heading); `gate-submit` validates them
against that packet's `read_refs` — not merely the run-wide snapshot — and
the tooling computes the CIDs and writes them into the cache at
`gate-finalize` (see §Write Rules → Tooled writes):

1. During the validate/verify/review run, keep track of which files were
   read and which line ranges of each file the judgment actually depended on
   (1-based, inclusive; e.g. `auth.go:120-180`).
2. Report each such file and its dependency scope in the packet report's
   `Dependency scope:` lines (the `{check key}: {file}: {declaration}` form).
   `gate-submit` validates path membership against that packet's `read_refs`; at `gate-finalize`
   the tooling resolves the declarations to CIDs and computes the whole-file
   `hash` against the content the gate run bracketed.
3. `specflowctl gate-evidence` remains the inspection tool for the
   declaration surface: `--sections` lists every located region,
   `--section <heading>` probes a heading's locatability, `--items` lists
   every acceptance item region, and `--acceptance-item <id>` probes an
   item id's locatability. Its output is never transcribed into a cache —
   the declaration schema has no `hash`/`deps` fields.

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

**Section-region declarations for unit specs:** for the unit's own main spec,
declare dependencies as **section regions** (`--section <heading>`, see
§Structural Region Dependencies) instead of line ranges: the sections are
located by mechanism, the declaration needs no line arithmetic, and an edit
in one section never stales a judgment that declared another. This is the
primary declaration mode for own-spec judgments — each check declares the
sections its judgment actually read (the per-check `checks` mapping, see
§Format). A unit spec that cannot be split into sections (no `##` headings,
or duplicated headings) fails closed when declared by section — restructure
the spec per `framework/spec_writing_guide.md` §13 before declaring.
Whole-file declarations remain the mode for code files, rule files, and
protocol appendices (the whole file is the carrier) — the rule file's
file-level `deps` stays whole-file, with the per-check `checks` mapping
breaking that whole declaration down per rule check (see §Format → Per-check
evidence → rule validate caches).

**Item-region declarations for unit specs (verify):** a verify judgment for
one acceptance item declares that item's region (`acceptance_item:<id>`, see
§Structural Region Dependencies) — located by id, so item reordering never
stales it, and editing one item stales only the judgments that declared it —
plus the section regions and code ranges it read. Every item judgment of the
run declares its own item region: the per-item declarations become the cache
entry's `checks` mapping (check key = the acceptance item id, see §Format →
Per-check evidence), and the delta scope derivation re-runs exactly the items
whose evidence went stale (`framework/verification_scope.md` §Delta Runs).
The whole-set token `acceptance_items` remains the declaration for judgments
over the set as a whole — validate Check 5's coverage judgment, the
cross-check, and cross-unit checks reading a dependency unit's item set. A
whole-set dependency is membership- and content-sensitive but order-insensitive:
reordering otherwise unchanged item blocks does not stale it. A judgment that
depends on presentation order declares the containing section, a line range,
or the whole file instead. A
declaration naming an item id that is missing or duplicated fails closed, and
item-level declarations are optional: an item judgment may fall back to a
section region or `acceptance_items` (declare-heavy conservatism), at the
cost of coarser staleness.

**Known limit:** the framework cannot verify that the agent's judgment
depended only on the declared ranges. Under-declaration can produce a
false-fresh cache that the mechanical gate cannot detect. The gate reports
changes outside the declared dependencies as an informational note rather
than a failure; treat the note as a prompt to re-run when semantic coupling
exists.

**Known limit (execution shape):** the cache records the run's judgment and
content evidence only; it carries no execution-shape field. Session identity,
worker identity, and read-only capability are runtime properties the
runtime-neutral tooling cannot observe, and a value written by the run itself
would be a self-report from the party whose independence is in question.
`fresh` and `promote` therefore cannot distinguish a run executed in an
independent session from one executed by the main agent. The
independent-execution requirement is an execution policy (see
`framework/verification_scope.md` §Guarantee Boundary), not a
cache-verifiable fact.

## Failure handling by gate role

The three gates handle a candidate full-run FAIL differently, and the split is not historical accident — it follows from what each gate's judgment is anchored to:

- **validate is the upstream root.** Its checks judge the spec itself, with no external anchor. A candidate validate FAIL means the spec was never confirmed sound — and validate's holistic checks are semantically coupled in ways the declared-region mechanism cannot capture (editing acceptance coverage can overturn a design-soundness judgment without any region CID going stale). A `pass` judgment on such a spec has no anchor to trust, so nothing may be carried over: the cache is deleted and the first full confirm must re-run everything. The low token cost of a doc-only review makes this the correct trade.
- **verify is anchored to a validated spec.** Its trust anchor is the spec, which validate has confirmed sound (promote enforces the validate gate before verify's record matters). verify's mismatches are code-vs-spec mappings, and the coupling between acceptance items runs through shared code functions and spec regions — wide, but mechanically capturable: each item declares the code/spec regions its judgment read, and the delta derivation re-runs exactly the items whose evidence went stale. Because verify's full run is the most expensive (it reads the code surface) and its fixes are the most frequent, a candidate verify full FAIL writes a **failure record** (the per-item `status` map) so the delta re-run (`reverify@{unit}`) can recover incrementally — re-checking the failed items, persisted targeted invalidations, newly affected items, current items the baseline never declared, and explicit `--rerun` overrides while carrying the remaining `pass` items whose evidence is unchanged.
- **review is anchored to a spec as design context.** Same as verify: the spec is fixed design context, the review judgments are per-file quality assessments that are self-contained per file. A candidate review full FAIL writes the blocking cache with the per-file `status` map, recovered by `rereview@{unit}` the same way.

The delta and repair re-runs (`re*`) always write or update a failure record on FAIL regardless of gate — the split above governs **full-run** FAIL only. A delta/repair FAIL's record is trusted because it carries over only judgments whose dependency evidence is unchanged by construction (`carried`), never judgments whose content moved.

## Write Rules

### Tooled writes

The cache format is a byte-exact machine-consumed contract: `hash` and the dependency CIDs are recorded verbatim, and `fresh`/`promote` compare them byte-for-byte. Hand transcription of those values is the root of transcription errors (a 64-bit CID typo is silently treated as evidence). Since the values are mechanically derived from file content, they are **computed by the tooling, never supplied by the agent**.

A promote-consumable cache is written by a **packet gate run**: the input snapshot and the packet plan are fixed before any judgment executes; each packet report is validated and recorded as it is submitted; the cache is written only if every required packet is accepted and the inputs are still unchanged at the end.

```
gate-plan → immutable input snapshot + packet plan → execute packets → gate-submit (per packet) → gate-finalize
```

**1. `specflowctl gate-plan` fixes the run's input snapshot and generates the packet plan:**

```
specflowctl gate-plan --gate validate|verify|review (--unit NAME | --rule ID) \
  --target candidate|stable [--mode full|delta|repair] [--input PATH_OR_REF]... [--rerun CHECK_KEY]... [--repo-root PATH]
```

- **Derived input surface:** the tooling resolves the gate's protocol inputs for the target and layer — the target's own files (unit main spec + appendices; for a rule, the rule file and its stable sibling), the dependency spec objects (`unit_refs` units and their appendices, `rule_refs` rules, the stable global rule set; consumer unit specs for rule validate), and, for verify/review, the declared code surface (`implementation_surface` directories expanded recursively, plus `affects.files`). Verify/review planning fails closed on that surface: every acceptance item's non-`<pending>` `implementation_surface` must be a single path resolving to at least one real file — a semicolon list, wildcard pattern, nonexistent path, or empty directory rejects the plan with the item id and the reason before any run state is written. Verify/review planning also requires at least one structurally located acceptance item: an empty item set has no verifiable object, so it rejects the plan before any run state is written (the mechanical `specflowctl validate` Check 2 rejects the same spec). Logical unit and bound-rule references record their current-layer resolution; logical global-rule references record their stable-layer resolution. Candidate-only global rules are not unit-gate inputs. Every physical entry must resolve inside the project root and is recorded by its canonical repo-relative path, resolved file, and content hash; lexical escapes and symlinks that resolve outside the project are rejected before run state is written.
- **Extra inputs (`--input`):** these are evidence inputs available to every packet, not work targets. Files, recursively expanded directories, and logical references have the same role. Physical inputs must resolve inside the project root; absolute external paths, lexical `..` escapes, and escaping symlinks are rejected. Valid inputs enter the immutable snapshot and packet `read_refs`, but never create review-file, verify-item, or validate-check packets. Only the spec-derived surface determines work packets.
- **Packet plan:** `gate-plan` generates the deterministic packet set for the gate/target/mode (see `framework/verification_scope.md` §Gate Work Packets → Packet generation rules). `--mode delta` / `--mode repair` derive the re-run set mechanically from the baseline cache (or failure record) and record the carried-over check keys.
- The tool prints the `run_id`, the snapshot summary, and the packet list, and writes the run state to `meta/gate_runs/{run_id}/run.json`; each submitted packet gets one state file under `meta/gate_runs/{run_id}/packets/` (a pending packet has no file — pending is the absence of state) — local process state, not a spec, not committed. Packet ids are logical identities and may contain file separators, colons, Unicode, or long review paths, so the filename is always `{sha256(packet_id)}.json` (64 lowercase hexadecimal characters); the state object retains the original packet id and loading requires it to match. There is no legacy filename lookup: an unfinished run created by a different state layout is replanned instead of partially interpreted. At most one open run exists per (gate, target, layer); a new `gate-plan` for the same tuple replaces the previous run. Gate-run state mutations are linearized by one repository-local operating-system lock: planning holds it across replacement and creation, submission holds it across terminal-state validation and the complete packet-state transition, and finalization holds it from the fresh run load through cache publication and consumption. The lock is released by the operating system when the process exits, so a crash cannot leave a stale ownership marker. Packet execution remains parallel — only the short local-state transitions are serialized.

**Targeted P0/P1 invalidation:** after a targeted executor reports a P0/P1,
the coordinator records it before returning control:

```
specflowctl gate-invalidate --gate validate|verify|review (--unit NAME | --rule ID) \
  --target candidate|stable --check CHECK_KEY [--check CHECK_KEY]... [--repo-root PATH]
```

The command runs under the same repository mutation lock as planning and
finalization. It deletes a matching pass cache, or adds the keys to a matching
failure record's `invalidated_checks` list. In the same locked transition it
marks any matching open gate run `invalidated`, so a run planned before the
targeted finding cannot later overwrite the invalidation with a pass cache.
An invalidated run rejects submission and finalization; plan a new run. This
command records recovery state only — it never publishes a targeted result as
a complete gate cache.

**2. Execution.** Before launching an executor, run `specflowctl gate-packet --run <run_id> --packet <packet_id> [--repo-root PATH]` and include its output verbatim as packet context. The context lists the exact `read_refs`; analysis/cross contexts additionally carry the accepted dependency results and their digests. Each packet is executed independently and reads no file outside its packet-local surface.

**3. `specflowctl gate-submit` records each packet report:**

```
specflowctl gate-submit --run <run_id> --packet <packet_id> --report <path> [--repo-root PATH]
```

- **Mechanical validation:** the report must be non-empty and structurally complete; declarations must resolve inside the submitting packet's `read_refs`, not merely somewhere in the run snapshot. The accepted report and its parsed `packet_result` are persisted together.
- **Dependency order:** verify analysis depends on its detection packet; cross depends on every local and analysis packet. `not_required` satisfies only a conditional analysis dependency.
- **Result binding:** analysis and cross packet states record the digests of the exact accepted results they consumed. Cross additionally must dispose every input finding, publish every logical judgment's effective status, map each new cross finding to the logical keys it makes fail so repair scope remains derivable, and publish one complete structured severity-confirmation sequence for every terminal retained finding. A confirmed first record completes the sequence; an adjusted first record requires exactly one final second record. Every record names evidence that belongs to the cross packet snapshot and dependency scope. For each non-cross key, the submitted status must be `fail` if and only if a retained finding of any severity affects that key; the cross status must exactly mirror the cross verdict.
- **Outcome:** a valid report is `accepted` (terminal); an invalid report is `rejected` and may be re-submitted. An analysis packet becomes `not_required` mechanically when its detection verdict is not `MISMATCH`. Terminality is enforced inside the same locked transition that writes the packet state: two concurrent submissions cannot both pass the pre-write status check, and the later transition observes the first terminal result instead of overwriting it.

**4. `specflowctl gate-finalize` writes the cache:**

```
specflowctl gate-finalize --run <run_id> [--timestamp ...] [--repo-root PATH]
```

- **Completeness check:** every required packet must be `accepted`; verify analysis packets may instead be `not_required`. Unit targets require an accepted cross result covering every current and carried judgment. Rule validate derives from its accepted packet plus every carried baseline judgment; the combined logical-status map must cover checks `1`–`8` before a cache can be written.
- **Snapshot check:** the tool re-resolves the input surface and compares every entry — path set, layer resolution, content hash — against the snapshot. Evidence assembly is bound to the same snapshot: every freshly computed cache entry must have the whole-file hash recorded for that declaration at `gate-plan`, and the complete input surface is compared again after the candidate cache has been rendered and checked, immediately before publication. Any divergence (a modified, added, or removed file; evidence computed from different bytes; a logical reference that now resolves to a different layer; a resolution that appeared or disappeared) rejects the finalize: no cache is written, the run state is deleted, and a new `gate-plan` is required. This is the time-of-check/time-of-use closure: recorded evidence can only describe content that was stable for the whole judgment window.
- **Judgment is closed by accepted artifacts:** local and analysis executors assign their protocol-owned verdicts/severities, cross performs the complete semantic synthesis and confirms or adjusts every terminal retained finding's severity, and `gate-finalize` mechanically derives `result`, `blocking`, severity counts, and the already-validated effective-status map from the canonical adjusted finding set. No logical key can be marked failed without a retained finding, no retained finding can leave an affected key marked passed, and no unconfirmed terminal severity can affect the cache. The adjusted severity is written into `GATE_JUDGMENTS` and is the value carried into a later delta/repair run. The coordinator supplies none of these values and cannot override cross.
- **Evidence is computed by the tool from the accepted reports:** each report's `Dependency scope:` lines name the files and regions its check keys read (`sections` / `ranges` / `acceptance_items` / `acceptance_item:<id>` — the same grammar `gate-evidence` accepts); the tooling resolves them to CIDs, computes the whole-file hash, builds the `files` entries and the per-check `checks` mapping, and enforces union discipline. The declaration schema has **no `hash` or `deps` fields** — a transcribed CID cannot enter a cache file.
- **Failure-record status map:** for a derived FAIL cache the tooling copies the cross result's complete effective-status map and marks unchanged baseline judgments `carried`.
- **Carried-over evidence and judgments (delta/repair):** the tooling copies both the evidence entries and structured judgments into the run at plan time. Cross consumes carried unit judgments; rule finalize combines the disjoint carried and re-run rule judgments and rejects any overlapping key. The executor does not re-declare carried judgments.
- **Path-form validation:** a name-resolved spec object declared as a physical path is rejected before the write, with the correct logical spelling in the error — in a unit cache: a unit main spec or protocol appendix that is not the target unit's own, and any rule file; in a rule cache: any unit spec file (main or appendix). The run's own target files and code files stay physical (see §Logical References).
- **Render, validate, then publish:** the tool renders the complete candidate cache in memory and runs the gate's own freshness chain (`CheckValidate`/`CheckVerify`/`CheckReview` and their stable/rule variants) against that candidate. A pass cache must come out `FRESH`; a failure record must come out `BLOCKED` (its designed state — it blocks promote and is the failure-recovery baseline). For a pass `validate@` candidate cache the appendix gate runs too: every non-exempt candidate appendix must be listed. Only after every check succeeds does the tool publish the candidate atomically to the canonical cache path. Any rejection returns non-zero without changing the prior canonical cache; when no prior cache exists, none is created. Candidate validate full-run FAIL remains the explicit exception defined below: it deliberately deletes the prior validate cache because trust establishment failed.
- **Audit field:** the written cache records `gate_run: <run_id>` (see §Format → Gate run binding). It links the cache to the bracketed run for audit and nothing else — `fresh` and `promote` never read it, and it proves nothing about who executed the run: the run id correlates state, it is not executor identity.
- **Run lifecycle:** a successful finalize marks the run consumed (kept for audit, replaced by the next `gate-plan` for the same tuple). A targeted P0/P1 marks a matching open run invalidated before it mutates the cache; an invalidated run remains for audit but accepts no submission or finalize. Gate run ids use the tooling-generated `YYYYMMDD-HHMMSS-<6 lowercase hex>` form. Loading requires the requested id, the state file's embedded `run_id`, and the containing run directory name to agree exactly. Every run and packet-state path must remain inside `meta/gate_runs/` after normalization. Invalid or mismatched identity fails closed before any state write or cleanup. A snapshot-divergence rejection deletes only the validated directory of that exact run; a completeness or validation rejection leaves it open so packets can be fixed and re-submitted. A consumed or invalidated run cannot be finalized again.

Each row of the Write Rules table below is assembled with `gate-finalize` from the accepted packet results, the synthesis result, and the run snapshot. The coordinator supplies no judgment fields.

### Write matrix (event → cache result)

| Event | Action |
|-------|--------|
| `validate@{unit}` full PASS (candidate round) | Write `validate_result.md` with `mode: full`, `target: candidate`, and `hash` + `deps` evidence for every file read (the declarations are assembled at `gate-finalize`; see §Write Rules → Tooled writes) |
| `validate@{unit}` / `validate@{rule}` full PASS (stable-only target) | Write `validate_result.md` with `mode: full`, `target: stable` — the stable confirmation cache (content vs dependencies/rules), consumed by `fresh@stable` only (see `framework/verification_scope.md` §Stable-only Targets) |
| `validate` full FAIL / needs_decision (candidate target) | Delete `validate_result.md` if it exists **only when `basis: full`** (full-run trust establishment failed, so nothing may be carried over). A candidate delta/repair FAIL follows the delta FAIL row and writes a failure record; deleting it would destroy the recovery baseline |
| `validate` full FAIL / needs_decision (stable-only target) | Write a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check plus the cross-check; full runs have no `carried`); recommend forking the unit/rule to reconcile the stable content with the changed dependency or rule. The record keeps the confirmation state visible as BLOCKED and is the failure-recovery baseline |
| `verify@{unit}` full PASS (all aligned) | Write `verify_result.md` with `result: pass`, severity counts at 0, `mode: full`, `target: candidate`, `blocking: false`, and `hash` + `deps` evidence for every file read |
| `verify@{unit}` full PASS (P2/P3 non-blocking findings) | Write `verify_result.md` with `result: pass`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `mode: full`, `target: candidate`, and `hash` + `deps` evidence. Promote may proceed |
| `verify@{unit}` full PASS (stable-only target) | Write `verify_result.md` with `target: stable` — the drift confirmation cache (VERIFIED state), consumed by `fresh@stable` only |
| `verify` full FAIL (any P0/P1 findings, candidate target) | Write a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-item `status` map — `pass`/`fail` for every acceptance item; full runs have no `carried`) — the failure-recovery baseline. Agent must stop, not proceed to promote. After the findings are resolved, repair re-checks the failed items, persisted invalidated items, newly affected items, current items the baseline never declared, and explicit `--rerun` overrides, then carries the remaining `pass` items over (see §Failure handling by gate role and `framework/verification_scope.md` §Delta Runs → Failure recovery) |
| `verify` full FAIL (any P0/P1 findings, stable-only target) | Write a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-item `status` map — `pass`/`fail` for every acceptance item; full runs have no `carried`); report the drift and recommend forking (do not enter divergence resolution — see `framework/unit_verify_checklist.md` §Stable-only mode) |
| `review@{unit}` full PASS | Write `review_result.md` with `mode: full`, `target: candidate`, `blocking: false`, `hash` + `deps` evidence for every file read, and findings body |
| `review@{unit}` full FAIL (P0/P1 found) | Write `review_result.md` with `mode: full`, `target: candidate`, `blocking: true`, includes finding counts, findings body, and the per-file `status` map (`pass`/`fail` for every reviewed file; full runs have no `carried`) |
| `review@{unit}` full run (stable-only target) | Write `review_result.md` with `target: stable` (PASS or FAIL) — the quality confirmation cache, consumed by `fresh@stable` only |
| `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` delta PASS | Rewrite the gate's cache with `mode: full`, `basis: delta`: new `hash` + `deps` + `checks` evidence for the re-run judgments' files, the original evidence (including the per-check `checks` breakdown) for carried-over judgments' files, fresh `timestamp` (see `framework/verification_scope.md` §Delta Runs) |
| `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` delta PASS (stable-only target) | Rewrite the gate's confirmation cache with `mode: full`, `target: stable`, `basis: delta` — same evidence rules as the candidate delta rewrite; the recovery applies only when the prior cache has `result: pass` (review: `blocking: false`) — a MISSING stable cache needs the full confirmation run, and a BLOCKED stable cache is a failure record recovered by the failure-recovery delta run (`basis: repair`) (see `framework/verification_scope.md` §Delta Runs → Layer applicability) |
| `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` delta/repair FAIL (P0/P1) | **Write a failure record** — rewrite the gate's cache with `result: fail`, `blocking: true`, severity counts, a findings body, `mode: full`, `basis: delta` (`basis: repair` for a repair-FAIL record), and the per-check `status` map (`fail` for the failed re-run checks, `pass` for the passed re-run checks, `carried` for the carried-over checks) plus the usual `hash` + `deps` + `checks` evidence. The record is the failure-recovery baseline. Promote must not proceed. (review: this is the existing blocking-cache write, extended with the status map) |
| `specflowctl fork --unit <name>` (with pass stable confirmation caches) | Inherits the confirmation caches into the candidate round: rewrites `target: stable` → `target: candidate` and every physical path under `docs/specs/units/stable/` → `docs/specs/units/candidate/` (the fork copies the stable content verbatim apart from the version bump, so pass conclusions carry over; the version bump stales the frontmatter declarations, which the delta re-runs cover). Gates without a usable baseline (missing, non-pass, blocking review, failure records) are skipped and listed in the fork manifest — their full runs are required in the new round. The inherited cache stays valid until its evidence goes stale (see §Cache lifecycle). The rewrite **consumes** the stable confirmation state: the cache file holds one layer at a time, so after the fork `fresh@stable` reports the unit's gates as STALE (the stable confirmation is not separately retained; the unit is mid-round and the stable layer is replaced at promote — the STALE signal is the expected round-in-progress state) |
| Delta/repair run FAIL (P0/P1) | validate/verify/review: write or update a failure record (see the delta/repair FAIL row above). Promote must not proceed |
| Targeted run (`:check-{n}` / `:{keyword}`) PASS | Report findings only — do not publish or mutate a cache. A targeted run never satisfies the promote gate |
| Targeted run (`:check-{n}` / `:{keyword}`) FAIL (P0/P1) | Run `gate-invalidate` immediately. It deletes a matching pass cache; for a failure record it preserves the record and persists the contradicted key in `invalidated_checks`; it also invalidates a matching open gate run. The targeted result is not itself a complete cache result |


### Cache lifecycle

| Event | Action |
|-------|--------|
| `specflowctl promote` succeeds (unit, non-retired) | Rewrite `validate_result.md`, `verify_result.md`, and `review_result.md` into stable confirmation caches — `target: candidate` → `target: stable`, and every physical path under `docs/specs/units/candidate/` (and `docs/specs/rules/candidate/` for rule validate) → the `stable/` equivalent. The rewritten caches become the stable-layer delta-recovery baseline (`fresh@stable` reports them FRESH/STALE; `re*` can restore a stale one; `fork` inherits them into the next round) |
| `specflowctl promote` succeeds (unit, retired) | Delete `validate_result.md`, `verify_result.md`, and `review_result.md` — the stable content is removed, so a rewritten cache would point at non-existent files and fail closed |
| `specflowctl promote --rule <id>` succeeds | Rewrite the rule validate cache into a stable confirmation cache (same candidate→stable layer transform); consumed by `fresh@stable` as the consumer/consistency state |
| `specflowctl promote` with review cache | When review cache is missing, stale, or `blocking: true`, promote is rejected with guidance. |
| A declared dependency chunk changes (CID no longer present) | Cache becomes stale — detected at promote time (candidate caches) or at fresh time (stable confirmation caches). Recovery: a delta run (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`) re-runs only the affected judgments and rewrites the cache with `basis: delta`, or the full command re-runs everything. Delta recovery requires a usable baseline — for a stable confirmation cache, the cache must exist with `result: pass` (review: `blocking: false`); a MISSING stable cache needs the full confirmation run, and a BLOCKED stable cache (failure record) is recovered by the failure-recovery delta run (see `framework/verification_scope.md` §Delta Runs → Layer applicability) |
| Content changes outside the declared dependency chunks | Cache stays fresh; fresh reports and promote print an informational note |
| Targeted run when full/delta cache exists | PASS leaves the cache unchanged. P0/P1 runs `gate-invalidate`; targeted execution never publishes a replacement result cache |
| Targeted run FAILs (P0/P1 findings) | `gate-invalidate` deletes a **pass** cache. A **failure record** (`blocking: true` / `result: fail`) is kept and gains the targeted key in `invalidated_checks`; the next repair plan reads it automatically. A matching open run is invalidated so it cannot overwrite this state |
| Targeted run PASSes (P2/P3 findings or all aligned) | Does NOT write cache — reports findings; existing cache stays valid |

### Targeted-run rule

A targeted run (`:check-{n}` / `:{keyword}`) never publishes a complete result cache, so it never satisfies the promote gate. A PASS leaves existing cache state unchanged.

**Exception (blocking):** If a targeted run FAILs (P0/P1 findings), the coordinator immediately runs `specflowctl gate-invalidate` for the reported judgment. The command deletes a pass cache regardless of prior state — P0/P1 at any granularity means promote must not proceed. A failure record is kept (it is already blocking; the gate refuses it either way, and it remains the failure-recovery baseline), and the contradicted key is persisted in `invalidated_checks`. The failure-recovery plan reads that list automatically and must re-run the judgment instead of carrying it over. The same transition invalidates a matching open gate run so an earlier plan cannot overwrite the new state (see `framework/verification_scope.md` §Delta Runs → Failure recovery).

Delta runs (`re*`) are not targeted runs: they write a cache with `basis: delta` (from a pass baseline) or `basis: repair` (from a failure record). A delta run's judgment set is complete (stale/failed judgments re-executed + carried-over judgments with unchanged evidence), which is what makes its cache valid for the promote gate.

## Staleness Detection

`specflowctl promote --unit <name>` reads the three cache files and the appendix cache, then checks:

1. **Mode check** — `mode` must be `full`. Fail closed: a missing or invalid mode value cannot prove a complete-coverage run. Targeted runs never write caches, so any cache with a non-`full` mode is invalid. The `basis` field is audit metadata and never gates — full, delta, and repair caches pass the same mode and dependency checks.
2. **Dependency check** — re-chunks every listed file and verifies each declared `deps` CID still exists in the file's current chunk set. If any declared dependency chunk is gone, the file's dependency changed and the cache is stale. A file with content but no declared `deps` (pre-content-addressed cache) also fails closed. A missing file fails the check regardless of `deps`.
3. **Verify result check** — `verify_result.md` with `result: pass` passes (P2/P3 pending items are carried by the severity counts). A failure record (`result: fail`) is rejected as `BLOCKED` (see below). Any other result value is rejected.
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

### Validate/verify failure-record promote check

The validate and verify gates share the same failure-record handling: a cache declaring `blocking: true` (the failure record shape) is rejected as `BLOCKED` — the reason follows the review gate's shape with the gate's command name ("Validate found {p0_count} P0 and {p1_count} P1 finding(s). Resolve before promoting.") — and a `result: fail` cache without the blocking declarations (missing `blocking` or a conflicting result/blocking pair) fails closed. A pass cache stays under the dependency-check gate only. The checks run in the same order as the review gate — stale records are STALE, never BLOCKED — so a fresh report and a promote run never disagree.

## Freshness Check (read-only)

`specflowctl fresh` (agent triggers `fresh@{target}` / `fresh@candidate` / `fresh@stable` / `fresh@all`) reports freshness without executing any check:

- **`specflowctl fresh`** (alias of `--scope candidate`) — summary for every unit and rule with a candidate file. One row per target with per-gate status and the overall `READY FOR PROMOTE: N of M` count.
- **`specflowctl fresh --scope stable`** — summary for every stable unit and rule. One row per target with its three confirmation states — `validate` (dependencies/rules), `verify` (code alignment), `review` (code quality) — plus the drift state (see Stable Drift Baseline below). Stable targets have no promote gate and are never counted in `READY FOR PROMOTE`.
- **`specflowctl fresh --scope all`** — candidate summary and stable summary in one report. `READY FOR PROMOTE` covers the candidate section only.
- **`specflowctl fresh --unit <name>`** / **`--rule <id>`** — detail for one target. A target that exists only in stable (no candidate file) reports the stable confirmation + drift detail instead of candidate gate statuses.

Every summary report (candidate/stable/all) ends with the full removal-candidate list — bound rules (`b_rule_*`) with no current-layer consumers and no retention declaration (the same detection primitive behind `specflowctl detect` and `specflowctl remove --rule`, see `framework/spec_writing_guide.md` §6.5). The list is layer-independent: removability is decided by consumers and the retention declaration alone, not by which layer holds the rule file, so each scope shows the same complete list exactly once. Read-only — deletion always happens through `specflowctl remove --rule` after user confirmation.

The gate vocabulary is `FRESH` / `STALE` / `MISSING` / `BLOCKED` (a failure record — validate, verify, and review with P0/P1) / `OK` (appendix). Classification reuses the same checks as `specflowctl promote` (Staleness Detection above), so a fresh report and a promote run never disagree. The detail view of a fresh cache shows its `basis` (`full`, `delta`, or `repair`) alongside mode — it is audit visibility, not a gate. For a retiring unit only the validate gate is reported, matching promote's gate set.

Stable confirmation states use the same vocabulary: `FRESH` means the stable-layer cache exists and its dependency evidence is unchanged; `STALE` means a declared dependency changed (a rule or dependency contract for validate, code for verify/review); `MISSING` means the stable confirmation was never run; `BLOCKED` means a failure record exists (a stable-only full-run FAIL or a stable delta-run FAIL — `result: fail` + `blocking: true`), recovered by the failure-recovery delta run (`basis: repair`); when the stable content itself can no longer hold against the changed dependency or rule, the record stays and forking reconciles it (see `framework/verification_scope.md` §Stable-only Targets). The states are informational — they grant nothing and gate nothing.

`fresh` is strictly read-only: it never writes or deletes caches or baselines and never triggers validate/verify/review. Its purpose is operational visibility while iterating on multiple units that share files — but a shared-file change stales a cache **only when it falls inside the declared dependency chunks**. Changes outside the declared dependencies keep the cache fresh and surface as an informational note (see Staleness Detection above): visibility is carried by the note, not by an over-broad STALE verdict. Cross-unit and bound-rule dependencies declared as logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:b_rule_*`) stay fresh across a promote of the referenced target when the dependency content is unchanged. Global-rule dependencies (`rule:g_rule_*`) remain bound to stable truth, so candidate-global edits do not stale unit caches and stable-global edits do.

## Stable Drift Baseline

Promote records a **baseline**: a snapshot of the promoted target's code surface. Baselines live under `docs/specs/meta/baseline/` (`unit/{name}.yaml`, `rule/{id}.yaml`), are written by `specflowctl promote` (removed by `specflowctl remove --rule <id>` for rules, and by the retire promote for retired units and appendices), and are kept after promote rewrites the candidate caches into stable confirmation caches. The baseline is a data snapshot, not a state machine — "drift" is never persisted, it is recomputed on every read. Baselines are durable records with the same lifecycle as the stable spec and must be kept under version control. The promoted confirmation caches under `docs/specs/meta/validation/` are durable delta baselines too — with `docs/specs/meta/validation/` no longer ignored, a fresh clone starts with every promoted target's gate state (FRESH or STALE after drift) instead of MISSING.

- **Unit baseline:** the files declared by `implementation_surface` (directories expanded recursively) and `affects.files`. For each surface file the baseline records the whole-file hash and, when the promote-time verify run declared dependencies on that file, the dependency chunk CIDs (copied from the verify cache, which must be fresh for promote). The drift comparison is complemented by the stable confirmation caches (`target: stable`, see `framework/verification_scope.md` §Stable-only Targets): a fresh stable verify cache adds the VERIFIED state ("code was recently confirmed to still conform"); the stable validate and review caches add their own confirmation states. The confirmation states and the drift column are independent dimensions — a fresh verify cache does not hide a mechanical CHANGED surface, it explains it.
- **Rule baseline:** the stable rule file itself (a rule declares no code surface) — it detects direct edits to a stable rule that bypass the fork flow. Rule baselines keep the whole-file hash comparison: rules have no verify run, so no dependency CIDs exist. A stable rule's validate confirmation cache (`target: stable`) covers the consumer/consistency dimension.

`fresh`'s stable scope compares the current code surface against the baseline:

| State | Meaning |
|---|---|
| `VERIFIED` | A fresh stable verify cache exists — the code was recently confirmed to still conform. In the summary this surfaces as `verify: FRESH`; the drift column keeps reporting the mechanical surface comparison independently. |
| `OK` | No verify cache; the code surface matches the baseline. For unit files with declared dependency CIDs, "matches" means every declared dependency chunk still exists — content changes outside the declared chunks do not fail the check. Such changes surface as an informational note ("content changed outside declared dependencies — re-verify if semantic coupling exists") instead. Files without declared dependency CIDs (including all legacy baselines written before dependency support) are judged on the whole-file hash. |
| `CHANGED` | The surface differs from the baseline: a declared dependency chunk is gone, a hash-only file changed, or files are missing or added (the report names them). The spec may have drifted; confirmation requires `verify@{unit}` against stable. |
| `MISSING` | No baseline recorded (the target was promoted before baseline support). |

The report states what it mechanically knows: `CHANGED` means "code changed since promote" — it never claims the spec is violated. Semantic confirmation is always a user-triggered `verify@{unit}` against the stable target (or `validate@{unit}` / `review@{unit}` for the dependency/rule and quality dimensions). The dependency-CID judgment is the same approximation the promote gate already accepts for caches: a note is a prompt to re-run when semantic coupling exists, never a failure.

## Important

Cache is never refreshed automatically. Only a new complete-coverage gate run, finalized by `gate-finalize` from its accepted packet reports, changes it. This is because validate and verify are semantic operations that require AI judgment — they cannot be reduced to a mechanical freshness comparison.

A cache answers "were these files checked and were they passing at that time?" A **failure record** extends that answer to the negative: "were these files checked and which judgments failed?" — the per-check `status` map is the failure-recovery baseline (which judgments are trusted, which must be re-run), consumed only by a user-triggered delta re-run. It never grants promote eligibility: a failure record is `BLOCKED` and promote rejects it exactly like a missing cache.

**Cache serves the promote gate only.** During iteration, an expired cache is the normal state — fixes applied after a validate/verify/review run make the cache stale, and the agent must NOT re-run quality-gate commands to restore freshness. Executing validate, verify, or review (including re-runs after a fix) is user-triggered only (see HARD RULE 2 in `framework/concepts.md`); the agent guides the user to a targeted re-check (`:check-{n}` / `:{keyword}`), a delta re-run (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`), or a concrete full command and waits for the user. The only way a cache becomes fresh again is a user-triggered complete-coverage run (full or delta). A failure record is recovered the same way: resolve the findings, then the delta re-run restores the pass cache with `basis: repair` — no full re-run needed unless the affected scope degrades to the whole run (see `framework/verification_scope.md` §Delta Runs → Failure recovery).

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
