# Validation Cache Lifecycle

## Purpose

Cache files record the result and content-addressed dependency evidence (whole-file hash + dependency chunk CIDs) of the last `validate` or `verify` run. They are not a state machine — they do not determine what happens next. They only answer: "were these files checked and were they passing at that time?" — not "who executed the check" (see §Dependency Declaration → Known limit). A failure record (a delta/repair re-run's or a candidate validate/verify full-run FAIL's fail cache) answers the negative variant: "were these files checked and which judgments failed?" — the per-check status map is the failure-recovery baseline (see §Write Rules and `framework/verification_scope.md` §Delta Runs → Failure recovery).

This file is the cache-lifecycle command package named by the trigger routing table in `framework/concepts.md`.

## File Locations

### Unit

- `docs/specs/meta/validation/unit/{name}/validate_result.md`
- `docs/specs/meta/validation/unit/{name}/verify_result.md` (one merged file carrying the `alignment` and `quality` lens sections)

### Rule

- `docs/specs/meta/validation/rule/{id}/validate_result.md`
- (Rule verify cache has been removed — rule does not need verify)

## Format

YAML frontmatter + markdown body:

```yaml
---
command: validate            # validate | verify
unit: user_auth
mode: full                   # complete results are always full; targeted runs never publish a complete cache
basis: full                  # audit metadata: full | delta | repair (full = full run; delta = re* incremental recovery from a pass baseline; repair = re* recovery from a failure record)
result: pass                 # pass | fail (fail = a failure record: a delta/repair re-run found P0/P1 and recorded them instead of deleting the cache; a candidate validate/verify full-run FAIL also writes one — see §Failure handling by gate role)
target: candidate            # the layer the run checked: candidate | stable (both gates record it; stable-only runs — @stable confirmation checks — write target: stable; see framework/verification_scope.md §Stable-only Targets)
blocking: false              # required on every fail/blocking-capable cache (validate/verify failure records): true iff result: fail (P0/P1 findings); pass caches may omit it (absent = not blocking)
p0_count: 0                  # (verify) severity counts; P2/P3 pending items when > 0
p1_count: 0
p2_count: 1
p3_count: 0
timestamp: "2026-06-30T10:00:00Z"
gate_run: 20260916-120000-3f9a1c   # audit: the gate run whose input snapshot this cache was finalized against (see §Write Rules → Tooled writes); never gated
invalidated_checks:                # failure records only; machine-owned targeted P0/P1 invalidations, omitted when empty
  - item:user_auth:auth.login
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
{"schema_version":3,"logical_status":{"1":"pass","cross":"pass"},"findings":[],"synthesis_digest":"sha256:...","deferred_findings":[],"relationships":[]}
GATE_JUDGMENTS_END -->
Generated human-readable summary of the result.
```

The `GATE_JUDGMENTS` block is the machine-readable baseline for delta/repair synthesis. Schema version 4 for unit verify, and schema version 3 for other caches, records the complete effective logical-status map, canonical retained findings, their complete source/affected-key sets, each finding's canonical renderable detail, the checked relationship names and `relationship:<name>` statuses, the accepted synthesis digest, and (merged verify caches) the run's deferred findings. `gate-finalize` generates it; agents never edit it. The `findings` array carries only gate-driving findings — those owned by the unit or unassigned; findings deferred to another unit are recorded in the optional `deferred_findings` array for audit and are never carried into a later run of this unit (routing lives in the deferred-findings ledger, see below). For rule validate delta/repair runs, `gate-finalize` combines the disjoint carried baseline judgments and re-run session judgments, so the rewritten block still contains all eight logical check statuses; carried and current findings are de-duplicated by finding id. A check that appears in both sets rejects finalization as corrupted run state. For unit gates, a merge group contributes its terminal retained finding once with the union of the group's logical keys. `gate-plan` copies carried judgments from this block into the new run so the final synthesis sees the complete logical result set, including the original finding detail needed for evidence-backed retention, suppression, or merge. `gate-finalize` renders every terminal retained finding that no accepted session report already contains — a carried judgment or a deferral routed in from another unit — into the new human-readable body exactly once. A cache without the current judgment schema cannot supply carried semantic results or a complete findings body and therefore cannot be used for a partial run; the planner requires a full run instead of guessing from prose.

**Deferred-findings ledger:** `docs/specs/meta/validation/deferred_findings.json` is the durable handoff of pending verify deferrals — findings whose recorded ownership routes them to another unit (`framework/verification_scope.md` §Coverage Model → Deferred findings). It lives beside the validation caches and shares their lifecycle (durable project state, version controlled). Schema version 1:

```json
{
  "schema_version": 1,
  "entries": [
    {
      "finding_id": "20260924-101112-abc123/src/shared.go/F1",
      "owner_unit": "agent",
      "source_unit": "tool",
      "source_run": "20260924-101112-abc123",
      "severity": "P1",
      "text": "…",
      "detail": "…",
      "affected_keys": ["src/shared.go"],
      "evidence_path": "docs/specs/units/candidate/unit_tool.md",
      "reason": "unit_tool.md records the behavior as agent-owned"
    }
  ]
}
```

- **Written by `gate-finalize`** of a verify run: it consumes the pending entries the run disposed, supersedes the unit's own older deferrals for the files the run re-reviewed, and upserts the run's new deferrals (keyed by finding id). The write is idempotent and the file is published atomically; an empty ledger removes the file.
- **Read by `gate-plan`** of the owner's verify run (either layer): the owner's pending entries become the run's immutable deferred-finding inputs, the plan output and `gate-mission` context display them, and the final synthesis must dispose each one. `fresh` reports the pending count.
- **Fail closed:** a malformed, duplicated, or unsupported-schema ledger rejects the plan; a corrupted routing state never silently drops a pending finding.

**Failure record:** a run that finds P0/P1 writes a **failure record** instead of deleting the cache — a delta or repair re-run (`revalidate@{target}` / `reverify@{unit}`) for both gates, the candidate validate/verify full FAIL, and the confirmation-cache FAIL for stable-only targets (see §Write Rules and §Failure handling by gate role). It carries `result: fail`, `blocking: true`, the severity counts, a findings body, and the same `files`/`checks` evidence as a pass cache plus a `status` per check. It is a valid cache file: fresh and promote report it as `BLOCKED` and promote rejects it — but unlike a deleted cache it remains the **failure-recovery baseline**. A later targeted P0/P1 does not rewrite that historical status map: `specflowctl gate-invalidate` records the contradicted key in the machine-owned `invalidated_checks` list. For verify it also invalidates the corresponding immutable evidence under the same lock; consuming caches become stale and must recheck, while historical records and status maps remain intact. Acceptance-item ids are stored as `item:<unit>:<item>` keys. After the findings are resolved, the repair plan re-checks the failed checks, persisted invalidated checks, newly affected checks, current keys the baseline never declared, and any explicit `--rerun` override, then carries the rest over (see `framework/verification_scope.md` §Delta Runs → Failure recovery). A failure record written by a full run (candidate validate/verify full FAIL, stable-only confirmation FAIL) declares `status` on every judgment — `pass`/`fail`, no `carried` (the full run re-executed all of them).

**Gate run binding:** `gate_run` is the audit id of the gate run whose input snapshot this cache was finalized against (see §Write Rules → Tooled writes). It identifies the run's fixed inputs — it is not executor identity and proves nothing about the execution shape (see §Dependency Declaration → Known limit (execution shape)). `fresh` and `promote` never read it, and caches written before the field existed remain valid (absent = no run recorded).

Each `files` entry records two kinds of evidence:

- **`hash`** — the whole-file content hash at run time. Informational only:
  for spec content it detects changes outside declared dependency regions. Every code dependency also has an exact whole-file fingerprint in its immutable verify record; any code change makes that record stale.
- **`deps`** — the content identifiers (CIDs) of the chunks the run actually
  depended on. **Spec freshness uses `deps`; verify code freshness uses exact whole-file fingerprints.** Changes outside declared spec regions retain valid judgments. Any change in a read code file invalidates its dependent verify records.

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
baseline; local checks may be carried only by delta/repair runs, while unchanged relationship checks may also be carried by full runs). The
status map is the failure-recovery scope input, derived mechanically by
`gate-finalize` from the accepted session verdicts: the recovery plan
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
only for local checks in delta/repair records, or for unchanged relationship checks in any run. A key missing from either side, an unknown value, or a carried local check in a full-run record is malformed state, and the
recovery degrades to the full coverage set instead of guessing. Pass caches may omit `status` (absent = pass), keeping
pre-failure caches valid — the absent-means-pass reading applies only to pass
caches, never to fail/blocking ones.

**Targeted invalidation contract:** `invalidated_checks` is an optional,
sorted, duplicate-free list present only on a failure record. It records
judgments whose recorded `pass`/`carried` result was contradicted later by a
targeted P0/P1 result. The coordinator writes it only through `specflowctl
gate-invalidate`; agents never edit it. `gate-plan --mode repair` unions these
keys into the mechanically derived re-run set before carry-over is computed.
An invalidated key that no longer maps to the current judgment surface makes
the plan degrade to the full coverage set instead of being ignored. A successful
`gate-finalize` rewrites the complete cache without the field because every
persisted invalidation in that plan was re-executed; a failed or rejected
finalize leaves the original failure record unchanged. Pass caches must not
carry `invalidated_checks`.

- Check keys are command-specific: validate uses the **agent check number**
  `"1"`–`"10"` (the 10 agent checks of `validate@{unit}`; the mechanical
  `specflowctl validate` Check 8 — region locatability — is a gate-evidence
  support check and takes no per-check declaration, while mechanical Check 9 —
  surface associations — is the audit evidence behind agent Check 9's
  declarations); verify uses the
  `item:<unit>:<item>` and `preserve:<unit>:<item>` for alignment; the `quality`
  lens uses `code:<file>`, `design:<unit>:<file>` and `architecture:<unit>`. The per-check granularity is
  the mechanism-derived delta scope input (see `framework/verification_scope.md`
  §Delta Runs) — it exists for delta derivation, not for the promote gate.
  Each `checks` entry carries its `lens` tag (`alignment` or `quality`) so the
  merged verify cache records both sections; validate checks carry no lens tag.
  Each checked relationship records its own `checks` entry under
  `relationship:<name>`, without a lens tag. Its evidence derives incremental
  relationship scope independently of local checks. `cross` is the summary
  verdict; any `cross` declarations are finding-disposition or ownership
  evidence. `gate-finalize` assigns that evidence to the current logical keys
  affected by the input and terminal findings, including suppressed and
  deferred findings. A merged finding's keys also inherit the source
  judgments' evidence. These dependencies enter each affected key's `checks`
  entry and the file-level union; there is no independent `cross` check entry.
  A carried key retains its prior dependencies and gains any evidence used by
  the current synthesis without being counted as re-executed. Freshness,
  delta scope, and promote therefore observe the same decision evidence.
- **Rule validate caches carry per-check evidence too:** rule check keys are
  the 7 agent checks of `validate@{rule}` (`"1"`–`"7"`; rules have no
  cross-check). The rule file entry keeps its whole-file file-level `deps`
  (the rule file is a contract file — every rule-body check declares the
  chunks it read, per the whole-body rule below), and optional-owner existence
  entries (`unit:{name}` logical references read by Check 5) declare the
  checks that consumed them. This makes a consumer-unit change stale exactly
  the checks that read it instead of falling back to file-level scope
  derivation, and gives rule failure records a per-check `status` carrier.
- The file-level `deps` remains the union of all declared check deps (plus
  any undeclared remainder). **The promote gate judges file-level freshness
  on that union only**, so the file-level freshness logic is identical with
  or without `checks`; the merged verify gate's lens-coverage requirement is
  the exception below.
- There is no compatibility shim for old caches: a `verify_result.md` written
  before per-check evidence (no `checks` field) is invalid. `fresh` and
  `promote` fail it closed with "no per-check evidence" and require a re-run
  (`verify@{unit}`). A delta plan against it degrades to the full coverage
  set (see `framework/verification_scope.md` §Delta Runs → Incremental scope).
- `checks` is optional per file entry in the cache format, but not for
  coverage keys: the merged verify gate requires a `checks` entry for every
  alignment and quality key. The parser distinguishes the check-level `deps:`
  block (8-space indent, inside a `checks` entry) from the file-level `deps:`
  block (4-space indent).
- Entries without a per-check breakdown (logical references, contract files,
  whole-file declarations, declare-heavy extras) leave their deps **unclaimed**
  by any check. The delta scope derivation reports such entries when they go
  stale and maps them by the command's fixed association; where no fixed
  association exists the plan degrades conservatively to the full coverage set —
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

Protected stable inputs selected by the verify plan are physical stable paths so an unfinished candidate never replaces confirmed truth. Both preserve and final synthesis declare these exact paths within their read refs, including synthesis over carried protection judgments. Cache writing and freshness checks preserve this binding; fork and promote change only the current unit's bindings. Adding an unrelated stable path through `--input` does not make it protected evidence. Other cross-unit and rule dependencies — any dependency object resolved by name: a dependency unit main spec read by unit `validate` Check 7, a protocol appendix of a dependency unit read by unit `validate` Check 7, a rule file read by unit `validate` Check 8, every peer unit main spec read by unit `validate` Check 9 (the surface-association audit scans all current-layer units — each peer is declared by its `frontmatter` and `acceptance_items` regions, so a declaration change in any unit stales exactly the surface-association judgment), or a unit spec file read by rule `validate` to check the optional promotion owner — are recorded as **logical references** instead of physical paths:

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

`mode: full` means a complete run of all checks/steps — the judgment set is complete, not a subset. Only complete-coverage runs publish caches: full runs (`validate@{target}` / `verify@{unit}`) and delta runs (`revalidate@{target}` / `reverify@{unit}`, see `framework/verification_scope.md` §Delta Runs). Targeted runs (`:check-{n}` / `:{keyword}`) never publish a result cache, so `mode` is always `full`; a targeted P0/P1 may only delete a pass cache or add machine-owned invalidation metadata to a failure record through `gate-invalidate`. The `basis` field distinguishes the three complete-result writers for audit: `basis: full` (or absent) means the cache came from a full run; `basis: delta` means a delta run re-executed the stale judgments and carried the rest over from a pass baseline; `basis: repair` means a delta run recovered from a **failure record** — it re-executed the failed checks, persisted invalidated checks, newly affected checks, current keys the baseline never declared, and explicit `--rerun` overrides, then carried the rest over (see `framework/verification_scope.md` §Delta Runs → Failure recovery).

### Verify result semantics

The verify cache uses the same `pass` / `fail` vocabulary as validate, at the gate level:

- `result: pass` — no P0/P1 blocking findings. May carry P2/P3 pending items via `p2_count`/`p3_count` (with `blocking: false`).
- `result: fail` — a **failure record**: a delta or repair re-run (`reverify@{unit}`) found P0/P1 and recorded the failure instead of deleting the cache, or a candidate verify full-run FAIL wrote one (see §Failure handling by gate role). It must declare `blocking: true`; the gate rejects it as `BLOCKED` (promote must not proceed). A fail-result cache without the blocking declarations is an invalid write and fails closed.

The per-item ALIGNED / MISMATCH / CANNOT_DETERMINE verdicts of the `alignment` lens are finding-level vocabulary and are unrelated to the cache `result` field.

### Merged verify cache (alignment + quality)

The one `verify_result.md` carries both lenses. Its `files` entries tag every check with its lens (`alignment` for item and preservation keys, `quality` for public code, design and architecture keys), so the cache records an `alignment` section and a `quality` section:

```yaml
---
command: verify
unit: user_auth
mode: full                  # always full
result: pass                # pass | fail
p0_count: 0
p1_count: 0
p2_count: 2
p3_count: 0
blocking: false             # required on every verify cache: true iff result: fail (P0/P1)
target: candidate           # candidate | stable
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: src/auth/login.go
    hash: sha256:def456...
    checks:
      - check: "code:src/auth/login.go"
        lens: quality
        deps:
          - sha256:7890ab...
      - check: "design:user_auth:src/auth/login.go"
        lens: quality
        deps:
          - sha256:7890ab...
      - check: "architecture:user_auth"
        lens: quality
        deps:
          - sha256:7890ab...
    deps:
      - sha256:7890ab...
  - path: docs/specs/units/candidate/unit_user_auth.md
    hash: sha256:abc123...
    checks:
      - check: "item:user_auth:AUTH-AC-001"
        lens: alignment
        deps:
          - region:acceptance_item:AUTH-AC-001:sha256:...
      - check: "design:user_auth:src/auth/login.go"
        lens: quality
        deps:
          - region:section:Description:sha256:...
      - check: "architecture:user_auth"
        lens: quality
        deps:
          - region:section:Description:sha256:...
    deps:
      - region:section:Description:sha256:...
      - region:acceptance_item:AUTH-AC-001:sha256:...
---
<!-- GATE_JUDGMENTS_BEGIN
{"schema_version":4,"logical_status":{"item:user_auth:AUTH-AC-001":"pass","code:src/auth/login.go":"pass","design:user_auth:src/auth/login.go":"pass","architecture:user_auth":"pass"},"records":{"code:src/auth/login.go":{"id":"<digest>","digest":"<digest>","layer":"candidate","source":"reused"},"design:user_auth:src/auth/login.go":{"id":"<digest>","digest":"<digest>","layer":"candidate","source":"executed"},"architecture:user_auth":{"id":"<digest>","digest":"<digest>","layer":"candidate","source":"executed"},"item:user_auth:AUTH-AC-001":{"id":"<digest>","digest":"<digest>","layer":"candidate","source":"executed"}},"findings":[],"synthesis_digest":"sha256:...","deferred_findings":[],"relationships":[]}
GATE_JUDGMENTS_END -->
## Findings

### P1 - src/auth/login.go:42 — Missing input validation on email field
  problem: the email field is written to the response without input validation.
  evidence:
    - `email := r.FormValue("email")` — src/auth/login.go:42
    - `pass_condition: "invalid email input is rejected with HTTP 400"` — unit_user_auth.md item AUTH-AC-001
  impact: malformed input reaches the response path; the declared rejection behavior is not implemented.
  fix: validate the email field and reject invalid input with HTTP 400.
  # A quality finding suppressed by a recorded spec rationale is not retained:
  # (suppressed) src/auth/config.go:88 — hardcoded key — accepted_tradeoff (dev-only config)
```

A PASS cache follows the same shape with `result: pass`, `blocking: false`, and all severity counts at 0. `promote --unit` requires the merged cache to record a check for **every** expected key of **both** lenses and to be fresh, `mode: full`, and non-blocking; a cache that covers only one lens is rejected.

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
declares those dependencies in the session report's `Dependency scope:` lines
(the same declaration grammar `gate-evidence` accepts; a declaration is the
literal `all` (whole file), a line-range list, the reserved token
`acceptance_items` (the spec's whole `acceptance_item_set` structural region),
one or more acceptance item declarations `acceptance_item:<id>[,<id>...]` (the
item regions of that set), or a section heading); `gate-submit` validates them
against that session's `read_refs` — not merely the run-wide snapshot — and
the tooling computes the CIDs and writes them into the cache at
`gate-finalize` (see §Write Rules → Tooled writes):

1. During the validate/verify run, keep track of which files were
   read and which line ranges of each file the judgment actually depended on
   (1-based, inclusive; e.g. `auth.go:120-180`).
2. Report each such file and its dependency scope in the session report's
   `Dependency scope:` lines (the `{check key}: {file}: {declaration}` form).
   `gate-submit` validates path membership against that session's `read_refs`; at `gate-finalize`
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
declare it. During `verify`, ranges must cover every function or
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

Every candidate full-run FAIL writes a **failure record** for both gates — `result: fail` + `blocking: true`, a findings body, and a per-check `status` map (`pass`/`fail` for every executed judgment; full runs re-execute local checks and may carry unchanged relationship judgments). The record replaces any prior cache: it blocks promote exactly like a missing cache, and it is the failure-recovery baseline. No full-run FAIL deletes a cache.

The uniform rule follows from what a carried-over judgment is anchored to. A repair run may carry a `pass` judgment only when the judgment was executed as a complete check by an independent reviewer, its declared dependency evidence is unchanged, and affected relationships are rechecked against the repaired content; promote still refuses anything but a complete-coverage pass. Those conditions hold identically whether the judgment passed inside a failed full run or inside a passed one — the earlier run's overall outcome cannot strengthen or weaken them — and the failed judgments themselves are never carried: repair re-runs every `fail` judgment, the persisted invalidations, the newly affected judgments, the current keys the baseline never declared, and explicit `--rerun` overrides. This applies to validate as much as to verify: validate remains the upstream root because every downstream gate is anchored to its pass cache, and that root trust is established by the complete-coverage pass promote eventually sees. (An earlier rule deleted the candidate validate cache on full FAIL because carrying over a `pass` judgment from a never-confirmed spec looked unanchored and a doc-only check looked cheap; under the coverage model a full unit validate is a multi-session reviewer run that also reads every peer unit spec, and the declared-evidence carry rule is the same one every delta run already trusts.)

Stable-only confirmation FAILs use the same record shape with `target: stable` and `basis: full`.

The verify gate's carry-over rests on the same evidence rule on top of its own anchor: its trust anchor is the spec, and promote enforces the validate gate before the verify record matters — so a carried verify key whose declared evidence is unchanged is anchored to a spec that is itself confirmed sound. The quality lens's per-file assessments are self-contained per file, so an unchanged file's assessment carries on the same basis.

The delta and repair re-runs (`re*`) write or update the same record on FAIL regardless of gate. A delta/repair FAIL's record is trusted on the same basis: it carries over only judgments whose dependency evidence is unchanged by construction (`carried`), never judgments whose content moved.

## Write Rules

### Tooled writes

The cache format is a byte-exact machine-consumed contract: `hash` and the dependency CIDs are recorded verbatim, and `fresh`/`promote` compare them byte-for-byte. Hand transcription of those values is the root of transcription errors (a 64-bit CID typo is silently treated as evidence). Since the values are mechanically derived from file content, they are **computed by the tooling, never supplied by the agent**.

A promote-consumable cache is written by a **coverage gate run**: the input snapshot and the coverage set are fixed before any judgment executes; each session report is validated and recorded as it is submitted; the coverage set is closed mechanically; and the cache is written only if every coverage key is covered (and the final synthesis is accepted when relationships are assigned or findings exist) and the inputs are still unchanged at the end. Verify runs first discover evidence file paths without making judgments, then include them in the plan-time snapshot.

```
gate-plan → immutable input snapshot + coverage set → agent chooses session batches with the same kind and lens → gate-mission (per batch) + gate-submit (per session) → gate-mission --final (only when relationships are assigned or findings exist) → gate-finalize
```

For `verify`, a path-only discovery pass precedes this sequence: use `specflowctl next --unit <name>`, find related tests, direct callers/callees, and context dependencies for every acceptance item, then provide every discovered file through repeated `--input` flags, including files already in the declared implementation surface. Do this for full, delta, and repair runs, including items that might be carried. The discovery pass does not assign verdicts. If an independent reviewer later identifies a required file outside its `read_refs`, it returns `Verification could not complete — missing read ref: <repo-relative path>` without a verdict. Do not submit the incomplete report; re-plan with the accumulated file paths and execute the replacement run. The earlier run's accepted results do not transfer to it.

**1. `specflowctl gate-plan` fixes the run's input snapshot and computes the coverage set:**

```
specflowctl gate-plan --gate validate|verify (--unit NAME | --rule ID) \
  --target candidate|stable [--relationships NAMES|none] [--mode full|delta|repair] [--input PATH_OR_REF]... [--rerun CHECK_KEY]... [--repo-root PATH]
```

- **Derived input surface:** the tooling resolves the gate's protocol inputs for the target and layer — the target's own files (unit main spec + appendices; for a rule, the rule file and its stable sibling), the dependency spec objects (`unit_refs` units and their appendices, `rule_refs` rules, the stable global rule set; every peer unit main spec for unit validate — Check 9's surface-association audit reads all current-layer units; consumer unit specs for rule validate), and, for verify, the declared code surface (`implementation_surface` directories expanded to their repository-content files — the files Git tracks plus untracked files that are not ignored — plus `affects.files`). Verify planning fails closed on that surface: every acceptance item's non-`<pending>` `implementation_surface` must be a single path resolving to at least one real file — a semicolon list, wildcard pattern, nonexistent path, or a directory with no repository-content files rejects the plan with the item id and the reason before any run state is written. Verify planning also requires at least one structurally located acceptance item: an empty item set has no alignment object, so it rejects the plan before any run state is written (the mechanical `specflowctl validate` Check 2 rejects the same spec). Logical unit and bound-rule references record their current-layer resolution; logical global-rule references record their stable-layer resolution. Candidate-only global rules are not unit-gate inputs. Every physical entry must resolve inside the project root and is recorded by its canonical repo-relative path, resolved file, and content hash; lexical escapes and symlinks that resolve outside the project are rejected before run state is written.
- **Extra inputs (`--input`):** these are evidence inputs available to every session, not work targets. Files, directories (expanded to their repository-content files), and logical references have the same role. Physical inputs must resolve inside the project root; absolute external paths, lexical `..` escapes, and escaping symlinks are rejected. Valid inputs enter the immutable snapshot and session `read_refs`, but never create coverage keys. Only the spec-derived surface determines coverage keys.
- **New verify evidence in delta/repair:** after directory inputs expand, any physical `--input` file absent from the baseline cache's recorded evidence makes the plan cover every verify alignment key. No judgment is carried from that baseline; the plan notice names the new paths. Inputs already recorded in the baseline retain the existing affected-key derivation.
- **Relationship scope:** unit plans require `--relationships name[,name...]|none`. The coordinator selects relationships touched by the current change; source dependencies select stale baseline relationships, and repair also selects failed or invalidated relationships. Each checked relationship declares its own evidence and status under `relationship:<name>`. Unchanged relationships are carried independently of local checks. A relationship-only incremental run can have an empty local coverage set. Rules accept no relationship flag.
- **Coverage set:** `gate-plan` computes the deterministic coverage set for the gate/target/mode (see `framework/verification_scope.md` §Coverage Model → Coverage keys). `--mode delta` / `--mode repair` derive the re-run set mechanically from the baseline cache (or failure record) and record the carried-over check keys.
- The tool prints the `run_id`, the snapshot summary, and the coverage set, and writes the run state to `meta/gate_runs/{run_id}/run.json`; each submitted session gets one state file under `meta/gate_runs/{run_id}/sessions/` (a pending session has no file — pending is the absence of state) — local process state, not a spec, not committed. A session id is a logical identity; its state filename is a fixed-length hash-derived name and the state object retains the original id, which loading validates. There is no legacy filename lookup: an unfinished run created by a different state layout is replanned instead of partially interpreted. At most one open run exists per (gate, target, layer); a new `gate-plan` for the same tuple replaces the previous run. Gate-run state mutations are linearized by one repository-local operating-system lock: planning holds it across replacement and creation, submission holds it across terminal-state validation and the session-state transition, and finalization holds it from the fresh run load through cache publication and consumption. The lock is released by the operating system when the process exits, so a crash cannot leave a stale ownership marker. Session execution remains parallel — only the short local-state transitions are serialized.

**Targeted P0/P1 invalidation:** after a targeted executor reports a P0/P1,
the coordinator records it before returning control:

```
specflowctl gate-invalidate --gate validate|verify (--unit NAME | --rule ID) \
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

**2. Execution.** Read `specflowctl gate-status --run <run_id> --format json`, partition the coverage set into batches with the same kind and lens, and for each batch run `specflowctl gate-mission --run <run_id> --keys <k1,k2,...> --format prompt [--repo-root PATH]` and send its output verbatim as the mission to one independent read-only reviewer. The mission includes exact `read_refs`, the semantic checklist path, constraints, report contract, and submission command. `gate-mission --format json` exposes the same mission as structured data. Each session executes independently and declares evidence only from its session-local read refs. A rejected session's regenerated mission includes its latest rejection reason.

**3. `specflowctl gate-submit` records each session report:**

```
specflowctl gate-submit --run <run_id> --session <session_id> --keys <k1,k2,...> --report <path> [--repo-root PATH]
```

- **Mechanical validation:** the report must be non-empty and structurally complete; declarations must resolve inside the submitting session's `read_refs`, not merely somewhere in the run snapshot. The accepted report and its parsed result are persisted together.
- **Key assignment:** the assigned keys must belong to the run's coverage set, must not already be covered by an accepted session, and must share one kind and must not mix lenses. The session id is derived from the key batch.
- **Result binding:** the final synthesis records the digests of the exact accepted results it consumed. It additionally must dispose every input finding, publish every logical judgment's effective status, and map each new synthesis finding to the logical keys it makes fail so repair scope remains derivable. Each retained finding's severity is the severity assigned by the session that raised it; the synthesis may raise it conservatively on retain or merge when the read evidence proves a larger impact (`framework/severity_policy.md` §9). For each non-cross key, the submitted status must be `fail` if and only if a retained finding of any severity affects that key; the `cross` status must exactly mirror the synthesis verdict.
- **Outcome:** a valid report is `accepted` (terminal); an invalid report is `rejected` and may be re-submitted. Terminality is enforced inside the same locked transition that writes the session state: two concurrent submissions cannot both pass the pre-write status check, and the later transition observes the first terminal result instead of overwriting it.

**4. `specflowctl gate-finalize` writes the cache:**

```
specflowctl gate-finalize --run <run_id> [--timestamp ...] [--repo-root PATH]
```

- **Coverage closure:** every coverage key must be covered by exactly one accepted session (plus the carried baseline judgments for delta/repair). Unit targets require an accepted final synthesis when relationships are assigned or the primary pass produced findings; a run with neither assigned relationships nor findings finalizes directly. Rule validate derives from its accepted session plus every carried baseline judgment; the combined logical-status map must cover checks `1`–`8` before a cache can be written.
- **Snapshot check:** the tool re-resolves the input surface and compares every entry — path set, layer resolution, content hash — against the snapshot. Evidence assembly is bound to the same snapshot: every freshly computed cache entry must have the whole-file hash recorded for that declaration at `gate-plan`, and the complete input surface is compared again after the candidate cache has been rendered and checked, immediately before publication. Any divergence (a modified, added, or removed file; evidence computed from different bytes; a logical reference that now resolves to a different layer; a resolution that appeared or disappeared) rejects the finalize: no cache is written, the run state is deleted, and a new `gate-plan` is required. This is the time-of-check/time-of-use closure: recorded evidence can only describe content that was stable for the whole judgment window.
- **Judgment is closed by accepted artifacts:** the session executors assign their protocol-owned verdicts/severities, the final synthesis (when it runs) checks only the assigned relationships and disposes existing findings, raises each terminal retained finding's severity conservatively when the read evidence warrants it, and routes deferred findings by recorded ownership, and `gate-finalize` mechanically derives `result`, `blocking`, severity counts, and the already-validated effective-status map from the canonical finding set. No logical key can be marked failed without a gate-driving retained finding, no gate-driving retained finding can leave an affected key marked passed, and no severity other than the canonical one can affect the cache. A finding deferred to another unit is retained for audit and routed, but marks no key and does not block. The canonical severity is written into `GATE_JUDGMENTS` and is the value carried into a later delta/repair run. The coordinator supplies none of these values and cannot override the final synthesis.
- **Evidence is computed by the tool from the accepted reports:** each report's `Dependency scope:` lines name the files and regions its check keys read (`sections` / `ranges` / `acceptance_items` / `acceptance_item:<id>` — the same grammar `gate-evidence` accepts); the tooling resolves them to CIDs, computes the whole-file hash, builds the `files` entries and the per-check `checks` mapping tagged with each check's lens, and enforces union discipline. The declaration schema has **no `hash` or `deps` fields** — a transcribed CID cannot enter a cache file.
- **Failure-record status map:** for a derived FAIL cache the tooling copies the final synthesis's complete effective-status map and marks unchanged baseline judgments `carried`.
- **Carried-over evidence and judgments (delta/repair):** the tooling copies both the evidence entries and structured judgments into the run at plan time. The final synthesis consumes carried unit judgments; rule finalize combines the disjoint carried and re-run rule judgments and rejects any overlapping key. The executor does not re-declare carried judgments.
- **Path-form validation:** a name-resolved spec object declared as a physical path is rejected before the write, with the correct logical spelling in the error — in a unit cache: a unit main spec or protocol appendix that is not the target unit's own, and any rule file; in a rule cache: any unit spec file (main or appendix). The run's own target files and code files stay physical (see §Logical References).
- **Render, validate, then publish:** the tool renders the complete candidate cache in memory and runs the gate's own freshness chain (`CheckValidate`/`CheckVerify` and their stable/rule variants) against that candidate — including the merged verify live-lens check. A pass cache must come out `FRESH`; a failure record must come out `BLOCKED` (its designed state — it blocks promote and is the failure-recovery baseline). For a pass `validate@` candidate cache the appendix gate runs too: every non-exempt candidate appendix must be listed. Only after every check succeeds does the tool publish the candidate atomically to the canonical cache path. Any rejection returns non-zero without changing the prior canonical cache; when no prior cache exists, none is created.
- **Audit field:** the written cache records `gate_run: <run_id>` (see §Format → Gate run binding). It links the cache to the bracketed run for audit and nothing else — `fresh` and `promote` never read it, and it proves nothing about who executed the run: the run id correlates state, it is not executor identity.
- **Run lifecycle:** a successful finalize marks the run consumed (kept for audit, replaced by the next `gate-plan` for the same tuple). A targeted P0/P1 marks a matching open run invalidated before it mutates the cache; an invalidated run remains for audit but accepts no submission or finalize. Gate run ids use the tooling-generated `YYYYMMDD-HHMMSS-<6 lowercase hex>` form. Loading requires the requested id, the state file's embedded `run_id`, and the containing run directory name to agree exactly. Every run and session-state path must remain inside `meta/gate_runs/` after normalization. Invalid or mismatched identity fails closed before any state write or cleanup. A snapshot-divergence rejection deletes only the validated directory of that exact run; a coverage or validation rejection leaves it open so sessions can be fixed and re-submitted. A consumed or invalidated run cannot be finalized again.

Each row of the Write Rules table below is assembled with `gate-finalize` from the accepted session results, the final synthesis result, and the run snapshot. The coordinator supplies no judgment fields.

### Write matrix (event → cache result)

| Event | Action |
|-------|--------|
| `validate@{unit}` full PASS (candidate round) | Write `validate_result.md` with `mode: full`, `target: candidate`, and `hash` + `deps` evidence for every file read (the declarations are assembled at `gate-finalize`; see §Write Rules → Tooled writes) |
| `validate@{unit}` / `validate@{rule}` full PASS (stable-only target) | Write `validate_result.md` with `mode: full`, `target: stable` — the stable confirmation cache (content vs dependencies/rules), consumed by `fresh@stable` only (see `framework/verification_scope.md` §Stable-only Targets) |
| `validate` full FAIL / needs_decision (candidate target) | Write a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check; full runs re-execute local checks and may carry unchanged relationship judgments) — the failure-recovery baseline for `revalidate@{target}`. Promote must not proceed. After the findings are resolved, repair re-checks the failed checks, persisted invalidated checks, newly affected checks, current keys the baseline never declared, and explicit `--rerun` overrides, then carries the rest over (see §Failure handling by gate role and `framework/verification_scope.md` §Delta Runs → Failure recovery) |
| `validate` full FAIL / needs_decision (stable-only target) | Write a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check; full runs re-execute local checks and may carry unchanged relationship judgments); recommend forking the unit/rule to reconcile the stable content with the changed dependency or rule. The record keeps the confirmation state visible as BLOCKED and is the failure-recovery baseline |
| `verify@{unit}` full PASS (all aligned and every declared code file passing) | Write `verify_result.md` with `result: pass`, severity counts at 0, `mode: full`, `target: candidate`, `blocking: false`, `hash` + `deps` evidence for every file read, and the merged `alignment` + `quality` sections |
| `verify@{unit}` full PASS (P2/P3 non-blocking findings) | Write `verify_result.md` with `result: pass`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `mode: full`, `target: candidate`, `hash` + `deps` evidence, and the merged `alignment` + `quality` sections. Promote may proceed |
| `verify@{unit}` full PASS (stable-only target) | Write `verify_result.md` with `target: stable` — the drift confirmation cache (VERIFIED state), consumed by `fresh@stable` only |
| `verify` full FAIL (any P0/P1 findings, candidate target) | Write a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-key `status` map — `pass`/`fail` for every acceptance item and declared code file; full runs re-execute local checks and may carry unchanged relationship judgments) — the failure-recovery baseline. Agent must stop, not proceed to promote. After the findings are resolved, repair re-checks the failed keys, persisted invalidated keys, newly affected keys, current keys the baseline never declared, and explicit `--rerun` overrides, then carries the remaining `pass` keys over (see §Failure handling by gate role and `framework/verification_scope.md` §Delta Runs → Failure recovery) |
| `verify` full FAIL (any P0/P1 findings, stable-only target) | Write a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-key `status` map — `pass`/`fail` for every acceptance item and declared code file; full runs re-execute local checks and may carry unchanged relationship judgments); report the drift and recommend forking (do not enter divergence resolution — see `framework/unit_verify_checklist.md` §Stable-only mode) |
| `revalidate@{target}` / `reverify@{unit}` delta PASS | Rewrite the gate's cache with `mode: full`, `basis: delta`: new `hash` + `deps` + `checks` evidence for the re-run keys' files, the original evidence (including the per-check `checks` breakdown) for carried-over keys' files, fresh `timestamp` (see `framework/verification_scope.md` §Delta Runs) |
| `revalidate@{target}` / `reverify@{unit}` delta PASS (stable-only target) | Rewrite the gate's confirmation cache with `mode: full`, `target: stable`, `basis: delta` — same evidence rules as the candidate delta rewrite; the recovery applies only when the prior cache has `result: pass` — a MISSING stable cache needs the full confirmation run, and a BLOCKED stable cache is a failure record recovered by the failure-recovery delta run (`basis: repair`) (see `framework/verification_scope.md` §Delta Runs → Layer applicability) |
| `revalidate@{target}` / `reverify@{unit}` delta/repair FAIL (P0/P1) | **Write a failure record** — rewrite the gate's cache with `result: fail`, `blocking: true`, severity counts, a findings body, `mode: full`, `basis: delta` (`basis: repair` for a repair-FAIL record), and the per-check `status` map (`fail` for the failed re-run checks, `pass` for the passed re-run checks, `carried` for the carried-over checks) plus the usual `hash` + `deps` + `checks` evidence. The record is the failure-recovery baseline. Promote must not proceed |
| `specflowctl fork --unit <name>` (with pass stable confirmation caches) | Inherits the confirmation caches into the candidate round: rewrites `target: stable` → `target: candidate` and every physical path under `docs/specs/units/stable/` → `docs/specs/units/candidate/` (the fork copies the stable content verbatim, so pass conclusions carry over and stay valid until the round's edits stale the affected declarations, which the delta re-runs cover). Gates without a usable baseline (missing, non-pass, failure records) are skipped and listed in the fork manifest — their full runs are required in the new round. The inherited cache stays valid until its evidence goes stale (see §Cache lifecycle). The rewrite **consumes** the stable confirmation state: the cache file holds one layer at a time, so after the fork `fresh@stable` reports the unit's gates as STALE (the stable confirmation is not separately retained; the unit is mid-round and the stable layer is replaced at promote — the STALE signal is the expected round-in-progress state) |
| Delta/repair run FAIL (P0/P1) | validate/verify: write or update a failure record (see the delta/repair FAIL row above). Promote must not proceed |
| Targeted run (`:check-{n}` / `:{keyword}`) PASS | Report findings only — do not publish or mutate a cache. A targeted run never satisfies the promote gate |
| Targeted run (`:check-{n}` / `:{keyword}`) FAIL (P0/P1) | Run `gate-invalidate` immediately. It deletes a matching pass cache; for a failure record it preserves the record and persists the contradicted key in `invalidated_checks`; it also invalidates a matching open gate run. The targeted result is not itself a complete cache result |

Every verify finalize additionally synchronizes the deferred-findings ledger (consume the pending deferrals the run disposed, supersede the unit's own older deferrals for re-reviewed files, record the run's new deferrals) — see §Format → Deferred-findings ledger. The severity counts and the blocking decision cover gate-driving findings only; a deferred finding is recorded in `GATE_JUDGMENTS.deferred_findings` and routed.


### Cache lifecycle

| Event | Action |
|-------|--------|
| `specflowctl promote` succeeds (unit) | Rewrite `validate_result.md` and `verify_result.md` into stable confirmation caches — `target: candidate` → `target: stable`, and every physical path under `docs/specs/units/candidate/` (and `docs/specs/rules/candidate/` for rule validate) → the `stable/` equivalent. The rewritten caches become the stable-layer delta-recovery baseline (`fresh@stable` reports them FRESH/STALE; `re*` can restore a stale one; `fork` inherits them into the next round) |
| `specflowctl promote --rule <id>` succeeds | Rewrite the rule validate cache into a stable confirmation cache (same candidate→stable layer transform); consumed by `fresh@stable` as the consumer/consistency state |
| `specflowctl promote` with a merged verify cache | When the verify cache is missing, stale, does not cover both lenses, or `blocking: true`, promote is rejected with guidance. |
| A declared dependency chunk changes (CID no longer present) | Cache becomes stale — detected at promote time (candidate caches) or at fresh time (stable confirmation caches). Recovery: a delta run (`revalidate@{target}` / `reverify@{unit}`) re-runs only the affected judgments and rewrites the cache with `basis: delta`, or the full command re-runs everything. Delta recovery requires a usable baseline — for a stable confirmation cache, the cache must exist with `result: pass`; a MISSING stable cache needs the full confirmation run, and a BLOCKED stable cache (failure record) is recovered by the failure-recovery delta run (see `framework/verification_scope.md` §Delta Runs → Layer applicability) |
| Content changes outside the declared dependency chunks | Cache stays fresh; fresh reports and promote print an informational note |
| Targeted run when full/delta cache exists | PASS leaves the cache unchanged. P0/P1 runs `gate-invalidate`; targeted execution never publishes a replacement result cache |
| Targeted run FAILs (P0/P1 findings) | `gate-invalidate` deletes a **pass** cache. A **failure record** (`blocking: true` / `result: fail`) is kept and gains the targeted key in `invalidated_checks`; the next repair plan reads it automatically. A matching open run is invalidated so it cannot overwrite this state |
| Targeted run PASSes (P2/P3 findings or all aligned) | Does NOT write cache — reports findings; existing cache stays valid |

### Targeted-run rule

A targeted run (`:check-{n}` / `:{keyword}`) never publishes a complete result cache, so it never satisfies the promote gate. A PASS leaves existing cache state unchanged.

**Exception (blocking):** If a targeted run FAILs (P0/P1 findings), the coordinator immediately runs `specflowctl gate-invalidate` for the reported judgment. The command deletes a pass cache regardless of prior state — P0/P1 at any granularity means promote must not proceed. A failure record is kept (it is already blocking; the gate refuses it either way, and it remains the failure-recovery baseline), and the contradicted key is persisted in `invalidated_checks`. The failure-recovery plan reads that list automatically and must re-run the judgment instead of carrying it over. The same transition invalidates a matching open gate run so an earlier plan cannot overwrite the new state (see `framework/verification_scope.md` §Delta Runs → Failure recovery).

Delta runs (`re*`) are not targeted runs: they write a cache with `basis: delta` (from a pass baseline) or `basis: repair` (from a failure record). A delta run's judgment set is complete (stale/failed judgments re-executed + carried-over judgments with unchanged evidence), which is what makes its cache valid for the promote gate.

## Rule Publication Before Promote

Rule publication is a separate read-only prerequisite, not a validate/verify cache status. Before checking caches, `specflowctl promote --unit <name>` reads the candidate unit's direct `rule_refs`: an explicit rule without stable content is rejected; a bound rule with different candidate/stable content is rejected until the rule is promoted. Complete normalized content is compared, including version and wording changes. Identical content passes. Unrelated bound candidates and bindings dropped by the unit candidate are not prerequisites.

Pending new or changed global drafts are advisory; unit checks continue to use stable global content. Every unit uses publication prerequisites; stable confirmation reports gain no promote gate. Input read errors stop with the failing path. The CLI and internal promote operation use the same publication check as `fresh`; rejection performs no writes. This check adds no cache fields or persistent state and does not stop validate/verify from reading bound candidate content.

A rule publication blocker does not stale or invalidate a passing cache. After publishing identical bound candidate content, its logical dependency may remain fresh and can satisfy the existing gate without a re-run. Check the actual gate state instead of requiring revalidation merely because publication occurred.

## Staleness Detection

`specflowctl promote --unit <name>` first checks rule publication, then reads the validate and merged verify caches and checks appendix coverage:

1. **Mode check** — `mode` must be `full`. Fail closed: a missing or invalid mode value cannot prove a complete-coverage run. Targeted runs never write caches, so any cache with a non-`full` mode is invalid. The `basis` field is audit metadata and never gates — full, delta, and repair caches pass the same mode and dependency checks.
2. **Dependency check** — re-chunks every listed file and verifies each declared `deps` CID still exists in the file's current chunk set. If any declared dependency chunk is gone, the file's dependency changed and the cache is stale. A file with content but no declared `deps` (pre-content-addressed cache) also fails closed. A missing file fails the check regardless of `deps`.
3. **Verify result check** — `verify_result.md` with `result: pass` passes (P2/P3 pending items are carried by the severity counts). A failure record (`result: fail`) is rejected as `BLOCKED` (see below). Any other result value is rejected.
4. **Merged verify lens check (required)** — `verify_result.md` must record a check for every expected coverage key of **both** lenses. The expected keys are the unit's current acceptance item ids (tagged `alignment`) and its declared code files (tagged `quality`); a cache that covers only one lens is rejected with guidance to re-run `verify@{unit}`.
5. **Appendix cache check** — reads the validate cache and verifies every non-exempt candidate appendix file is listed in the validate cache's file list. If any non-exempt appendix on disk is missing from the cache's file list, the appendix was not validated and promote is rejected with guidance to run `validate@{unit}`.
6. **Main file check** — the validate and verify cache file lists must include the main candidate spec file (`docs/specs/units/candidate/unit_{name}.md`), and the rule validate cache must include the candidate rule file (`docs/specs/rules/candidate/{rule_id}.md`). A cache whose file list omits the main file cannot prove that file was read during the run, so promote is rejected with guidance to re-run the corresponding check.

When the dependency check passes but the whole-file hash differs from the
recorded `hash` (content changed outside the declared dependency chunks),
promote and fresh reports print an informational note naming the files. The
note never fails the gate — it is a prompt to re-run when semantic coupling
exists.

`specflowctl promote --rule <id>` enforces cache freshness — reads the validate cache, rejects if missing or stale. Rule verify cache is no longer required (rule verify has been removed).

### Merged verify cache promote check (required gate)

The merged verify cache at `docs/specs/meta/validation/unit/{name}/verify_result.md` is a hard prerequisite for promote. All conditions must pass:

1. **Existence check** — if the file does not exist, promote is rejected: "Verify not completed. Run `verify@{unit}` first."
2. **Mode check** — `mode` must be `full`. If not, promote is rejected: "verify cache mode is %q, expected 'full' — run `verify@{unit}` before promoting."
3. **Dependency check** — re-chunks every listed file and verifies each declared `deps` CID still exists. If a declared dependency chunk changed or a file is missing, the cache is stale and promote is rejected: "Verify cache is stale. Run `verify@{unit}` again."
4. **Lens coverage check** — the cache must record a check for every expected key of both lenses (the current acceptance item ids tagged `alignment` and the declared code files tagged `quality`). A cache covering only one lens is rejected: "verify cache does not cover both lenses — missing key(s): ... Run `verify@{unit}` again."
5. **Blocking check** — if `blocking: true`, promote is rejected: "Verify found {p0_count} P0 and {p1_count} P1 finding(s). Resolve before promoting."
6. **Blocking declaration check** — `blocking` is a required field on every verify cache. A cache without it fails closed (the gate cannot determine blocking status): promote is rejected: "verify cache missing required field `blocking` — cannot determine blocking status".
7. **Result value check** — `result` must be `pass` or `fail`. Any other value is rejected.
8. **Consistency check** — `result: fail` must declare `blocking: true` and `result: pass` must declare `blocking: false`. A conflicting declaration is rejected: the cache was written incorrectly and its blocking status cannot be trusted.

### Validate failure-record promote check

The validate gate shares the same failure-record handling: a cache declaring `blocking: true` (the failure record shape) is rejected as `BLOCKED` — the reason follows the merged verify gate's shape with the gate's command name ("Validate found {p0_count} P0 and {p1_count} P1 finding(s). Resolve before promoting.") — and a `result: fail` cache without the blocking declarations (missing `blocking` or a conflicting result/blocking pair) fails closed. A pass cache stays under the dependency-check gate only. The checks run in the same order as the merged verify gate — stale records are STALE, never BLOCKED — so a fresh report and a promote run never disagree.

## Freshness Check (read-only)

`specflowctl fresh` (agent triggers `fresh@{target}` / `fresh@candidate` / `fresh@stable` / `fresh@all`) reports freshness without executing any check:

- **`specflowctl fresh`** (alias of `--scope candidate`) — summary for every unit and rule with a candidate file. One row per target with per-gate status and the overall `READY FOR PROMOTE: N of M` count. Normal candidate units also show `rules: OK | BLOCKED`. Every blocking rule ID/reason is listed, and global draft advisories are shown once for the candidate section. Unit readiness includes the publication check; advisories do not lower readiness.
- **`specflowctl fresh --scope stable`** — summary for every stable unit and rule. One row per target with its two confirmation states — `validate` (dependencies/rules) and `verify` (code alignment + quality) — plus the drift state (see Stable Drift Baseline below). Stable targets have no promote gate and are never counted in `READY FOR PROMOTE`.
- **`specflowctl fresh --scope all`** — candidate summary and stable summary in one report. `READY FOR PROMOTE` covers the candidate section only.
- **`specflowctl fresh --unit <name>`** / **`--rule <id>`** — detail for one target. A normal candidate unit detail includes `RULE PREREQUISITES: OK | BLOCKED`, blocking IDs/reasons, and pending global advisories independently of the cache statuses. A target that exists only in stable (no candidate file) reports the stable confirmation + drift detail instead of candidate gate statuses or publication prerequisites.

Fresh reports gate status and publication prerequisites only. Deletion previews use `remove --dry-run` under `framework/removal_workflow.md`.

The gate vocabulary is `FRESH` / `STALE` / `MISSING` / `BLOCKED` (a failure record — validate and verify with P0/P1) / `OK` (appendix). Cache classification reuses the same checks as `specflowctl promote` (Staleness Detection above); rule publication uses its separate shared check. A fresh report and a promote run therefore agree on publication prerequisites and applicable cache requirements. The detail view of a fresh cache shows its `basis` (`full`, `delta`, or `repair`) alongside mode — it is audit visibility, not a gate.

Stable confirmation states use the same vocabulary: `FRESH` means the stable-layer cache exists and its dependency evidence is unchanged; `STALE` means a declared dependency changed (a rule or dependency contract for validate, code for verify); `MISSING` means the stable confirmation was never run; `BLOCKED` means a failure record exists (a stable-only full-run FAIL or a stable delta-run FAIL — `result: fail` + `blocking: true`), recovered by the failure-recovery delta run (`basis: repair`); when the stable content itself can no longer hold against the changed dependency or rule, the record stays and forking reconciles it (see `framework/verification_scope.md` §Stable-only Targets). The states are informational — they grant nothing and gate nothing.

`fresh` is strictly read-only: it never writes or deletes caches or baselines and never triggers validate/verify. Its purpose is operational visibility while iterating on multiple units that share files — a shared-code change invalidates every verify record depending on that file. Spec changes outside declared dependencies keep the cache fresh and surface as an informational note (see Staleness Detection above): visibility is carried by the note, not by an over-broad STALE verdict. Cross-unit and bound-rule dependencies declared as logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:b_rule_*`) stay fresh across a promote of the referenced target when the dependency content is unchanged. Global-rule dependencies (`rule:g_rule_*`) remain bound to stable truth, so candidate-global edits do not stale unit caches and stable-global edits do.

## Stable Drift Baseline

Promote records a **baseline**: a snapshot of the promoted target's code surface. Baselines live under `docs/specs/meta/baseline/` (`unit/{name}.yaml`, `rule/{id}.yaml`), are written by `specflowctl promote` (cleared by whole-object all-layer removal; appendix removal preserves the code baseline, see `framework/removal_workflow.md`), and are kept after promote rewrites the candidate caches into stable confirmation caches. The baseline is a data snapshot, not a state machine — "drift" is never persisted, it is recomputed on every read. Baselines are durable records with the same lifecycle as the stable spec and must be kept under version control. The promoted confirmation caches under `docs/specs/meta/validation/` are durable delta baselines too — with `docs/specs/meta/validation/` no longer ignored, a fresh clone starts with every promoted target's gate state (FRESH or STALE after drift) instead of MISSING.

- **Unit baseline:** the files declared by `implementation_surface` (directories expanded to their repository-content files — the files Git tracks plus untracked files that are not ignored) and `affects.files`. For each surface file the baseline records the whole-file hash and, when the promote-time verify run declared dependencies on that file, the dependency chunk CIDs (copied from the verify cache, which must be fresh for promote). The drift comparison is complemented by the stable confirmation caches (`target: stable`, see `framework/verification_scope.md` §Stable-only Targets): a fresh stable verify cache adds the VERIFIED state ("code was recently confirmed to still conform"), covering both the alignment and quality lenses. The confirmation states and the drift column are independent dimensions — a fresh verify cache does not hide a mechanical CHANGED surface, it explains it.
- **Rule baseline:** the stable rule file itself (a rule declares no code surface) — it detects direct edits to a stable rule that bypass the fork flow. Rule baselines keep the whole-file hash comparison: rules have no verify run, so no dependency CIDs exist. A stable rule's validate confirmation cache (`target: stable`) covers the consumer/consistency dimension.

`fresh`'s stable scope compares the current code surface against the baseline:

| State | Meaning |
|---|---|
| `VERIFIED` | A fresh stable verify cache exists — the code was recently confirmed to still conform. In the summary this surfaces as `verify: FRESH`; the drift column keeps reporting the mechanical surface comparison independently. |
| `OK` | No verify cache; the code surface matches the baseline. For unit files with declared dependency CIDs, "matches" means every declared dependency chunk still exists — content changes outside the declared chunks do not fail the check. Such changes surface as an informational note ("content changed outside declared dependencies — re-verify if semantic coupling exists") instead. Files without declared dependency CIDs (including all legacy baselines written before dependency support) are judged on the whole-file hash. |
| `CHANGED` | The surface differs from the baseline: a declared dependency chunk is gone, a hash-only file changed, or files are missing or added (the report names them). The spec may have drifted; confirmation requires `verify@{unit}` against stable. |
| `MISSING` | No baseline recorded (the target was promoted before baseline support). |

The report states what it mechanically knows: `CHANGED` means "code changed since promote" — it never claims the spec is violated. Semantic confirmation is always a user-triggered `verify@{unit}` against the stable target (or `validate@{unit}` for the dependency/rule dimension). The dependency-CID judgment is the same approximation the promote gate already accepts for caches: a note is a prompt to re-run when semantic coupling exists, never a failure.

## Important

Cache is never refreshed automatically. Only a new complete-coverage gate run, finalized by `gate-finalize` from its accepted session reports, changes it. This is because validate and verify are semantic operations that require AI judgment — they cannot be reduced to a mechanical freshness comparison.

A cache answers "were these files checked and were they passing at that time?" A **failure record** extends that answer to the negative: "were these files checked and which judgments failed?" — the per-check `status` map is the failure-recovery baseline (which judgments are trusted, which must be re-run), consumed only by a user-triggered delta re-run. It never grants promote eligibility: a failure record is `BLOCKED` and promote rejects it exactly like a missing cache.

**Cache serves the promote gate only.** During iteration, an expired cache is the normal state — fixes applied after a validate/verify run make the cache stale, and the agent must NOT re-run quality-gate commands to restore freshness. Executing validate or verify (including re-runs after a fix) is user-triggered only (see HARD RULE 2 in `framework/concepts.md`); the agent guides the user to a targeted re-check (`:check-{n}` / `:{keyword}`), a delta re-run (`revalidate@{target}` / `reverify@{unit}`), or a concrete full command and waits for the user. The only way a cache becomes fresh again is a user-triggered complete-coverage run (full or delta). A failure record is recovered the same way: resolve the findings, then the delta re-run restores the pass cache with `basis: repair` — no full re-run needed unless the affected scope degrades to the whole run (see `framework/verification_scope.md` §Delta Runs → Failure recovery).

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


## Shared verify records (schema 4)

Unit verify uses `framework/shared_judgments.md`. Its structured block contains `records`, mapping every necessary task key to `{id, digest, layer, source}`. Sources are executed, carried or reused. Public, design, architecture and acceptance records are immutable JSON files in `docs/specs/meta/validation/judgments/`; preserve keys reference acceptance records bound to stable. Other caches retain schema 3.

Code dependencies use exact whole-file fingerprints, including related evidence read by public checks. Spec dependencies retain their region declarations. Fresh and promote check the complete record-reference chain and current required coverage. A damaged, missing, invalidated or old-protocol record is a concrete gate gap. Old verify caches require full verify and are never converted into public judgments.

Fork and promote rewrite only the current unit's layer bindings and unchanged own-spec paths. Protected peer paths remain stable. Removal clears this unit's current cache references without deleting shared records. No history cleanup is performed.

A schema 4 verify binding example (the ids and digests below are illustrative):

```json
{
  "schema_version": 4,
  "logical_status": {"code:contracts.js": "pass", "design:auth:contracts.js": "pass", "architecture:auth": "pass", "item:auth:auth.login": "pass"},
  "records": {
    "code:contracts.js": {"id": "<digest>", "digest": "<digest>", "layer": "candidate", "source": "reused"},
    "design:auth:contracts.js": {"id": "<digest>", "digest": "<digest>", "layer": "candidate", "source": "executed"},
    "architecture:auth": {"id": "<digest>", "digest": "<digest>", "layer": "candidate", "source": "executed"},
    "item:auth:auth.login": {"id": "<digest>", "digest": "<digest>", "layer": "candidate", "source": "executed"}
  },
  "findings": [],
  "relationships": [],
  "synthesis_digest": "sha256:<digest>"
}
```

Explicit removal clears selected-layer caches, invalidates affected open runs, and clears whole-object publication baselines only for all-layer deletion; appendix removal preserves the code baseline. See `framework/removal_workflow.md`.
