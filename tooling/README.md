# SpecFlow Tooling

This directory contains the standalone Go CLI that performs deterministic governance actions for `specFlow`.

The tooling layer exists only for fixed execution work whose meaning is already constrained by governance rules.
It validates spec files mechanically, but it does not judge business semantics.

In tooling contracts, `<tooling-root>` means `tooling/` in a `source_repo` layout and `specflow/tooling/` in an `installed_project` layout.
Installed-project usage examples below continue to show `specflow/tooling/...` directly.

## Build

`<tooling-root>/bin/` is a local binary cache.
It is ignored by git and must not be committed.

Installed-project rebuild example from the repository root:

```bash
cd specflow/tooling
go run ./cmd/specflowctl build-release --repo-root ../..
```

Source-repository rebuild example from the repository root:

```bash
cd tooling
go run ./cmd/specflowctl build-release --repo-root ..
```

Official platform binaries are GitHub Release assets.
Release tags use the tooling fingerprint form `specflow-tooling-<12-character-fingerprint>`.
The release workflow builds binaries from the tagged source and uploads the binaries plus `SHA256SUMS`.
The release is tied to the tooling input fingerprint, not to every source commit.
The fingerprint includes Go command code, Go internal code, and required tooling metadata.

Pull the repository and install specflowctl binaries for the pulled tooling source:

```bash
specflow/tooling/scripts/pull_with_release.sh
```

PowerShell:

```powershell
.\specflow\tooling\scripts\pull_with_release.ps1
```

The script runs a fast-forward pull (fetch + reset to the remote branch), reads the recorded tooling fingerprint from `tooling/fingerprint.txt`, and downloads specflowctl binaries and `SHA256SUMS` only when required binaries are missing, stale, or missing checksums. By default downloads binaries for all platforms (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64.exe, windows-arm64.exe) so a Syncthing-synced project directory stays usable on every platform. Use `--current-only` / `-CurrentOnly` to download only the current platform's binary. Note that the pull resets the local SpecFlow repository to the remote branch; unpushed local commits in `specflow/` are discarded.

Push the current branch and publish a tooling release when the current `main` fingerprint has no release tag. Must be run on the `main` branch of the SpecFlow source repository — both scripts reject non-main branches:

```bash
specflow/tooling/scripts/push_with_release.sh
```

PowerShell:

```powershell
.\specflow\tooling\scripts\push_with_release.ps1
```

## Governance Boundary

The tooling layer may:

1. collect
2. parse
3. validate
4. rebuild
5. compare
6. cleanup
7. preflight
8. transition
9. sync
10. render read-only local views
11. maintain mechanical review run-state fields
12. relation calculation
13. operation scope state

The tooling layer must not:

1. invent new governance semantics
2. replace governance judgment
3. replace shared-boundary judgment
4. replace review severity or final conclusion judgment owned by the active review policy
5. become a second semantic source of truth
6. write reader-derived conclusions back into project files

### Governed write zones (`validate write`)

`validate write --path <path>` first resolves the repository layout, then matches the normalized repository-relative path against the following zones in order. The classifier evaluates both the lexical repository path and its symlink-resolved repository path whenever either identity is inside the repository; a denied result for either identity is final, so a symlink cannot hide a protected source or target behind an allowed prefix. A path whose symlink identity cannot be resolved — a broken or dangling link on the path — is **denied**, not classified by its lexical identity alone: a write through the link would land in the unresolved location. Absolute paths are converted to repository-relative before matching; a path whose lexical and resolved identities are both outside the repository root (or cannot be relativized) is outside the write-zone contract and takes the default result. A missing or ambiguous SpecFlow layout is an error — the classifier does not guess a layout or fall through to the default allowed result.

| Order | Path prefix | Result |
|---|---|---|
| 1 | the resolved layout's framework root (`framework/` in `source_repo`; `specflow/framework/` in `installed_project`) | **denied** — framework files are never writable via `validate write` |
| 2 | `docs/specs/units/stable/` | **denied** — use `promote` to write stable specs |
| 3 | `docs/specs/rules/stable/` | **denied** — use the rule governance flows |
| 4 | `docs/specs/units/candidate/` | **allowed** — candidate spec file |
| 5 | `docs/specs/rules/candidate/` | **allowed** — candidate rule file |
| 6 | anything else | **allowed** — "not governed by specFlow write restrictions" (includes implementation source code) |

This table is the authoritative contract for the `validate write` zone check (`tooling/internal/writezone/writezone.go` implements it verbatim; `tooling/cmd/specflowctl/validate.go` holds the CLI surface and delegates to it). Any change to the zone table is a governance-boundary change and must update this section in the same change.

The zone table is the **global static policy layer**: it applies at write time and independently of any operation scope. The tooling resolves the layout once for the command and uses that layout's framework root consistently during declaration and final checking. An open operation never weakens it — a changed path inside a denied zone is reported as a static-policy violation by `operation check` even when some declaration allowed it.

### Target names

Unit names and rule ids are external input that the CLI turns into repository paths, so one grammar governs them everywhere:

- **Unit names** match `^[A-Za-z0-9][A-Za-z0-9_-]*$`.
- **Rule ids** match the same grammar and carry the `g_rule_`/`b_rule_` prefix convention (the operation-scope target contract additionally enforces the prefix).

Every CLI entry that accepts a unit name or rule id (`--unit`, `--rule`, and `validate rule --id`) validates it with `tooling/internal/specpaths.ValidateTargetName` before any path construction or filesystem access; a name outside the grammar fails the command without reading or writing anything. The gate-run planner, the validation-cache paths, and the operation-scope target share the same predicate. A path separator, traversal segment, whitespace, or any character outside the grammar is invalid, not normalized — external names must not reach a path builder unvalidated.

### Operation scope (`operation ...`)

`specflowctl operation` maintains a declared, frozen change scope for one bounded piece of work and verifies the final working-tree change mechanically against that scope. It is runtime-neutral: nothing intercepts writes; the comparison happens when the caller runs the check. The mechanism requires git (baseline diff) and the repository root passed as `--repo-root` must be the git worktree top level.

State is one JSON file per operation at `meta/operations/<operation_id>.json` (local process state, not committed — the same class as `meta/gate_runs/`). Operation ids use the tooling-generated `YYYYMMDD-HHMMSS-<6 lowercase hex>` form. A supplied id that does not match that form, a state file whose embedded `operation_id` does not equal the requested id, or a state path that does not remain inside `meta/operations/` after normalization is malformed state and fails closed before any read-modify-write transition. Normalization resolves symlinks under the same rule as scope entries below: the path's nearest existing ancestor is resolved before the missing suffix is reattached, an unresolvable (dangling) symlink component fails closed, and the resolved location must remain inside the repository — in-repository symlinks whose real target stays inside the repository are valid. Loading also validates the complete state object: unknown JSON fields, invalid status/target/source values, non-canonical paths, invalid required-spec declarations, malformed timestamps, and lifecycle-inconsistent open/closed fields or update history are rejected before the state can be checked, listed, or changed.

`close` and `update` are read-modify-write transitions on one state file. Each enters the repository-local operating-system lock (`meta/.operations.lock`) before loading the state and holds it until the write commits, so concurrent transitions in a shared working tree serialize: an update cannot overwrite a committed close with a stale open snapshot, and no update event is lost. The operating system releases the lock when the process exits, so a crash cannot leave a stale ownership marker.

The state object's exact fields (this table is the schema contract for `operation open` / `update` / `close`):

| Field | Type | Contract |
|---|---|---|
| `operation_id` | string | `YYYYMMDD-HHMMSS-<6 lowercase hex>`; must equal the requested id; no surrounding whitespace |
| `status` | string | `open` or `closed` |
| `target` | object | `{kind, name?}` — `kind` is `unit`, `rule`, or `none`; `none` must not carry a name; `unit`/`rule` names match §Target names; rule names carry the `g_rule_`/`b_rule_` prefix |
| `baseline` | object | `{ref, sha, recorded_at}` — `ref` is the requested ref (non-empty); `sha` is the resolved full lowercase commit id (40 or 64 hex); `recorded_at` is UTC `YYYY-MM-DDTHH:MM:SSZ` and not after `opened_at` |
| `allowed_paths` | array | non-empty, sorted by `path`, no duplicates; each `{path, source}` — `path` is a canonical repository-relative path that resolves inside the repository; `source` is `spec:file` (must match the target's spec path), `spec:implementation_surface` / `spec:affects.files` (unit targets only), or `declared` |
| `required_spec_paths` | array | canonical candidate spec paths (see Required spec paths), unique and sorted |
| `parent_operation` | string, optional | id of the predecessor operation; must not equal `operation_id` |
| `opened_at` / `updated_at` | string | UTC `YYYY-MM-DDTHH:MM:SSZ`; `updated_at` must not be before `opened_at` |
| `closed_at` | string, optional | required on `closed`; must equal `updated_at` and not be before `opened_at`; forbidden on `open` |
| `close_outcome` | string, optional | `passed` or `abandoned`; required on `closed`; forbidden on `open` |
| `updates` | array, optional | one event per `operation update`: `{at, declared_allowed_paths, required_spec_paths}` — each list canonical/unique/sorted, events monotonic in `at` and within the operation timeline, never removing a path recorded by the previous event; the latest event must equal the frozen declared scope (`declared_allowed_paths` = the frozen `declared`-source entries, `required_spec_paths` = the frozen set) |

Unknown JSON fields and trailing JSON values are rejected, so every field a state file carries is defined above.

| Command | Effect |
|---|---|
| `operation open` | Declare an operation: resolve the scope sources, freeze the allowed scope, record the baseline commit, write the state file. Prints the operation id. |
| `operation check` | Read-only evaluation of the frozen scope against the current change set. Exit code 1 when the result is `FAIL`. |
| `operation close` | The same evaluation as `check`; marks the operation `closed` only on `PASS`. On `FAIL` it refuses the close and leaves the state unchanged (exit code 1). With `--abandon`, a violating operation is ended explicitly: the state records the `abandoned` close outcome and the violation report is printed — an operation is never silently passed, but it is also never a trap. |
| `operation update` | Add caller-declared scope and/or required spec paths to the frozen values, recording the resulting union as an update event. Only while the operation is open; an update never removes an existing path. |
| `operation status` | Read-only listing of open operations, or detail for one `--id`. |

**Scope sources (explicit by construction).** `allowed_paths` entries come from exactly two sources, and every entry records its source label:

1. **spec-derived** — from the target's spec: the candidate spec file and its candidate appendix files (`spec:file`), the acceptance items' `implementation_surface` values (`spec:implementation_surface`), and the `affects.files` values (`spec:affects.files`). For a rule target the derived entry is the candidate rule file (`spec:file`). When only a stable spec exists, the scope carries the deterministic candidate paths — the fork targets (candidate main spec and the candidate equivalents of the stable appendices) — instead of the never-writable stable files; the fork is the standard first step of a spec change, so its output must already be in scope. The spec-derived part is frozen at `open` — a later spec edit cannot silently widen an open operation; `operation update` can only add caller-declared paths.
2. **caller-declared** — repeatable `--allow` flags (`declared`), representing the additional scope explicitly declared in the user-approved plan. `operation update` unions new entries with this frozen part; it never removes an existing entry.

A scope entry is accepted only when both its lexical path and its symlink-resolved location remain inside the repository. For a path that does not exist yet, the nearest existing ancestor is resolved before the missing suffix is reattached, so an escaping parent symlink is still rejected. This containment rule applies to every source: candidate specs and appendices, `implementation_surface`, `affects.files`, `--allow`, and `--require-spec`. It is re-evaluated whenever persisted operation state is loaded, so replacing an allowed directory with an escaping symlink makes `check`/`close` fail closed. In-repository symlinks whose real target remains inside the same repository are valid.

A `--allow` or `--require-spec` entry that falls inside a denied write zone or inside the exclusions below is rejected at `open`/`update` time (fail closed at declaration). Spec-derived entries remain subject to the static policy layer at check time.

**Required spec paths.** Repeatable `--require-spec` entries declare candidate spec files this operation must change. Accepted paths are unit mains under `docs/specs/units/candidate/unit_*.md`, unit appendices under `docs/specs/units/candidate/appendix/unit_*_*.md`, and rule candidates matching `docs/specs/rules/candidate/g_rule_*.md` or `docs/specs/rules/candidate/b_rule_*.md`. Ordinary code, stable specs, meta files, directories, and other Markdown files are rejected at `open`/`update`; the candidate file need not exist yet because creating it may be part of the operation. An update unions new required paths with the frozen set and cannot remove an existing spec-first obligation. `check`/`close` fail when any required path is absent from the change set **or no longer exists in the working tree** — a deletion is not a change of the spec's content, so it satisfies neither half — the spec-first obligation is verified mechanically, not by prose.

**Baseline and change set.** `open` records the baseline commit (`--baseline REF`, default `HEAD`) as the resolved commit SHA. The change set is:

1. tracked changes against the baseline commit — `git diff --name-only --no-renames <sha>` (this includes committed, staged, and unstaged changes and deletions), plus
2. untracked, non-ignored files — `git ls-files --others --exclude-standard`.

Renames are deliberately decomposed into delete + add (`--no-renames`) so both paths are evaluated. Excluded from the change set: `meta/` and `docs/specs/meta/` — the tooling's own process state and derived caches are not operation deliverables.

**Matching.** Every entry is a path scope, not a file snapshot: a changed path is in scope when it equals the entry or starts with `entry + "/"`. Directory entries therefore cover files created during the operation. There is no glob support.

**Result.** `PASS` requires all three lists empty: out-of-scope changed paths, static-policy violations, and missing required spec paths. On `FAIL` the caller must resolve the violation by one of: reverting the out-of-scope changes; ending the current operation with `operation close --abandon` (the state records the `abandoned` outcome) and opening a new user-authorized operation for the newly authorized target (`--parent` records the lineage); or widening the scope through `operation update` after explicit user approval. Whether that approval happened is declared by the caller — the tooling records the update and cannot verify authorization.

**Concurrency and attribution.** The comparison is a mechanical baseline diff over the shared working tree. When other sessions or processes change the same working tree, `operation check` cannot attribute a change to an author, and it deliberately does not guess: every out-of-scope path is reported conservatively. For reliable attribution, run the operation in a dedicated `git worktree` — operation state is per working tree (`meta/` is local), and the baseline is that worktree's commit.

**Fail closed.** A missing git repository, a missing operation, a missing baseline commit, a malformed state file, an invalid or mismatched operation id, an operation-state path outside `meta/operations/`, a scope path that currently resolves outside the repository, or a git error makes `check`/`close` fail with exit code 1. `check` never modifies project files; `close` changes only its own validated state file (`status`, `close_outcome`, and timestamps).

## Current Command Surface

1. `init`
   - bootstrap framework-managed files
2. `doctor`
   - inspect installation and binary freshness health
3. `build-release`
   - rebuild cross-platform binaries
4. `next`
   - discover a unit's files and dependencies
   - `next --unit <name>`: outputs candidate/stable spec files, appendix files, rule refs, related units, and the acceptance-item-derived fields (implementation surfaces, affects files, acceptance item ids — when the spec declares them)
   - this is a render action: read-only, does not modify any project file
5. `fork`
   - copy a stable spec/rule (and appendix files for units) to the candidate layer with a version bump
   - `fork --unit <name>` / `fork --rule <id>`: rejects if a candidate already exists or the stable source does not exist
   - this is the only allowed fork path (see HARD RULE 5 in `framework/concepts.md`)
6. `consumers`
   - list units that reference a given rule in their `rule_refs`
   - `consumers --rule <id>`: for global rules (`g_rule_*`) returns every current-layer unit; for bound rules (`b_rule_*`) returns only matching units (empty output means no consumers); a global rule with no rule file is reported as not found (same contract as `deps --rule`)
7. `deps`
   - read-only dependency analysis over the in-scope units' declared `unit_refs`
   - `deps [--scope all|candidate|stable]` (default `all` — current-layer units, candidate preferred, stable fallback; retiring units with `status: retired` are excluded — their references disappear with them): reports the directed dependency graph (unit nodes + `unit_refs` edges), rule refs per unit, cycle member lists, and the promotion order (dependencies first; units without a promotion order are blocked by a cycle)
   - `deps --unit <name>`: the unit's depends-on refs, bound rules, referrers, and cycle state; `deps --rule <id>`: the units bound to the rule — explicit `rule_refs` consumers for a bound rule (`b_rule_*`), every current-layer unit for a global rule (`g_rule_*`, which applies by default and is not repeated in `rule_refs`); a global rule with no rule file is reported as not found
   - pure mechanical computation: only explicit `unit_refs`/`rule_refs` edges count, no prose inference, no judgment, no file writes
   - corresponds to the `deps@all` / `deps@{unit}` / `deps@{rule}` agent triggers (see `framework/verification_scope.md` §Dependency Analysis)
8. `fresh`
   - report cache freshness and promote readiness without executing any check
    - `fresh --scope candidate|stable|all` (default `candidate`): candidate scope reports every unit/rule with a candidate file and its gate statuses plus the `READY FOR PROMOTE: N of M` count; stable scope reports every stable target's three confirmation states (validate: dependencies/rules, verify: code alignment, review: code quality — rule targets report validate only) plus the baseline drift state (`OK` / `CHANGED` / `MISSING`, see Stable Drift Baseline in `framework/validation_cache.md`); all scope reports both (`READY FOR PROMOTE` covers the candidate section only)
     - `fresh --unit <name>` / `fresh --rule <id>`: detail for one target — candidate gate statuses, or confirmation states + drift for a stable-only target (no candidate file); every STALE unit gate appends a `DELTA SCOPE (<gate>)` section — the mechanism-derived delta re-run scope (affected check keys from the cache's per-check `checks` mapping, unclaimed entries, stale-dep count, degradation state), the input for `revalidate@`/`reverify@`/`rereview@`; a unit with pending deferred findings shows their count and source in a note (the next review disposes them)
    - every summary report (candidate/stable/all) ends with the full removal-candidate list — bound rules (`b_rule_*`) with no current-layer consumers and no retention declaration; the list is layer-independent (removability is decided by consumers and the retention declaration alone, not by which layer holds the rule file) and appears exactly once per report
    - strictly read-only: never writes or deletes caches or baselines, never triggers validate/verify/review; a `fresh` report and a `promote` run never disagree because both use the same cache checks
   - corresponds to the `fresh@{target}` / `fresh@candidate` / `fresh@stable` / `fresh@all` agent triggers (see `framework/concepts.md` and `framework/validation_cache.md` §Freshness Check)
 9. `detect`
    - read-only detection of rule removal readiness
    - `detect --rule <id>`: reports the rule's current-layer (effective) consumers (candidate preferred, stable fallback, same resolution as `deps`) and its `unbound_retention` declaration; removable = no consumers and no retention declaration
    - `detect --all`: lists every bound rule (`b_rule_*`) in the candidate and stable layers with no consumers and no retention declaration; global rules (`g_rule_*`) are never listed — they apply to every unit by default, so "no consumers" is not a meaningful state for them
    - pure read-only: never writes or deletes files
    - corresponds to the `detect@{rule}` / `detect@all` agent triggers (see `framework/concepts.md` and `framework/spec_writing_guide.md` §6.5)
 10. `remove`
    - delete a rule whose constraint no longer applies (user-confirmed only)
    - `remove --rule <id>`: final verification reuses the detection primitive — rejected while any current-layer unit still references the rule in `rule_refs` (referrers listed) and while it declares `unbound_retention` (intentional retention); for a global rule only explicit references block removal, the default applicability lifts with the file. On success deletes the stable copy (and candidate copy if present), then the rule's baseline and validate cache; a rule with no file in either layer degrades to residual metadata cleanup (re-entrant recovery path after a partial deletion)
    - corresponds to the `remove@{rule}` agent trigger (see `framework/concepts.md` and `framework/spec_writing_guide.md` §6.5)
 11. `gate-evidence`
    - compute the dependency evidence for one file read during a validate/verify/review run (inspection — the cache evidence is computed by the tooling at `gate-finalize`; nothing is transcribed)
    - `gate-evidence --file <path>` with optional `--ranges START-END,START-END` (1-based, inclusive; empty means the whole file): maps the declared line ranges onto content-defined chunks and outputs the whole-file `hash` + the `deps` chunk CIDs
    - with `--acceptance-items`: emits the order-insensitive semantic identity of the whole `acceptance_item_set` as the dependency (`region:acceptance_items:<cid>`), computed from the set preamble and item regions sorted by id; changing membership or item content stales it, while reordering otherwise unchanged item blocks does not. A missing marker, empty set, empty id, or duplicated id fails closed. With an empty `--ranges` it replaces the whole-file declaration; with `--ranges` both are declared (see `framework/validation_cache.md` §Structural Region Dependencies)
    - with `--acceptance-item <id>` (repeatable): emits one acceptance item's region as the dependency (`region:acceptance_item:<id>:<cid>`), located by item id — reordering items changes no item region unless set-level content follows the last item (that content belongs to the last item's region), while a missing or duplicated id fails closed
    - with `--items`: lists every acceptance item region (id, line range, CID) without declaring dependencies — the informational output that names the `--acceptance-item` values and probes locatability
    - with `--section <heading>` (repeatable): emits the section region with that heading text as the dependency (`region:section:<heading>:<cid>`), located by heading rather than line numbers — the frontmatter region (file head through the line before the first `##` heading) is named `frontmatter`; a missing or duplicated heading fails closed
    - with `--sections`: lists every section region (heading, line range, CID) without declaring dependencies — the informational output that names the `--section` values and probes locatability
    - corresponds to the `specflowctl gate-evidence` command row (see `framework/commands.md` and `framework/validation_cache.md` §Dependency Declaration)
  12. `gate-plan`
    - fix the immutable input snapshot and generate the deterministic packet plan for a quality-gate run before any executor reads input (see `framework/verification_scope.md` §Gate Work Packets and `framework/validation_cache.md` §Write Rules → Tooled writes)
    - `gate-plan --gate validate|verify|review (--unit NAME | --rule ID) --target candidate|stable [--mode full|delta|repair] [--input PATH_OR_REF]... [--rerun CHECK_KEY]...`: fixes the snapshot and deterministic packet graph. Unit validate plans resolve `unit_refs` and bound `rule_refs` from the current layer (candidate first, stable fallback), but enumerate global rules only from the stable layer; candidate global rules remain unpublished until promotion. `--input` entries are evidence available to packets; physical paths must resolve inside the project root, and files and recursively expanded directories never create review targets. Packet ids and dependency edges are validated before run state is written; duplicate acceptance-item ids therefore reject the plan instead of aliasing packet state. Verify/review plans also fail closed on the declared code surface: every acceptance item's non-`<pending>` `implementation_surface` must be a single repository-relative path resolving to at least one real file (a directory expands recursively) — a semicolon list, wildcard pattern, nonexistent path, or empty directory rejects the plan with the item id, the value, and the reason before run state is written (the same rule is mechanical validate Check 3). They require at least one structurally located acceptance item as well: an empty item set has no verifiable object, so it rejects the plan before run state is written (the same spec fails mechanical validate Check 2). A review plan also loads the unit's pending deferred findings from `docs/specs/meta/validation/deferred_findings.json` into the run (immutable input; the cross synthesis must dispose them) and prints them in the plan output; a malformed ledger rejects the plan. Packet ids remain logical identities in state; their per-packet filenames are fixed-length lowercase SHA-256 values, so colons, separators, Unicode, and long review paths never become platform-specific filenames. Verify plans `detect:{item}` + `analysis:{item}` pairs and `cross`; analysis is conditionally required by the detection verdict. Delta/repair plans snapshot carried structured judgments for cross; repair automatically unions failure-record `invalidated_checks` into the re-run set. `--rerun` remains an explicit additional override, not the persistence mechanism for targeted findings. The replacement and creation form one repository-locked transition, so concurrent plans cannot leave two live runs.
    - rule targets support the validate gate only (rule verify/review removed)
  13. `gate-packet`
    - `gate-packet --run RUN_ID --packet PACKET_ID`: emit one packet's exact execution context, including read refs, scope, accepted dependency results/digests, carried judgments for cross, and — for a review run — the unit's pending deferred findings (the cross packet always, a file packet when its key is affected). Read-only; include the output verbatim in the executor prompt
  14. `gate-status`
    - read-only report of packet-run progress: without `--run`, every open run with its gate/target and packet counts, filterable with `--gate` / `--unit` / `--rule` (`--unit` and `--rule` are mutually exclusive); with `--run RUN_ID`, per-packet status (`pending` / `accepted` / `rejected` / conditional `not_required`), attempt numbers, the latest rejection reason, result digests, and the next action (`gate-packet` + `gate-submit`, or `gate-finalize`)
    - the recovery point after an interrupted run — progress is readable from the run state alone
  15. `gate-submit`
    - record one packet report after mechanical validation, then make it visible to `gate-status`/`gate-finalize`
    - `gate-submit --run RUN_ID --packet PACKET_ID --report PATH`: validates against packet-local read refs, persists the verbatim report and parsed result, resolves conditional verify analysis, and binds dependency result digests for analysis/cross. The command holds the gate-run mutation lock from state load through the complete write, so a concurrent submission observes an already accepted/not-required packet as terminal instead of replacing it. Cross submissions must cover every input finding and logical status — input findings include the unit's pending deferred findings — and every ownership record must name a terminal retained finding, cite evidence inside the cross packet's read refs and a `cross` dependency-scope declaration, and name a unit that exists in the repository (ownership records are review-only). Every merge chain must terminate at a retained input or new cross finding, and every terminal retained finding must have one complete evidence-backed severity-confirmation sequence. A confirmed first result completes the sequence; an adjusted first result moves one level and requires exactly one final second record for the adjusted severity. Every evidence path must be in the cross packet's read refs and dependency scope. A non-cross key is `fail` if and only if at least one gate-driving retained finding of any severity affects it — a finding deferred to another unit is retained and routed but marks no key. The cross key must mirror the cross verdict, and a `FAIL` verdict requires a gate-driving P0/P1 cross finding. Rule validate has no cross packet, so its single checks packet carries the required sequence for each P0 finding.
  16. `gate-finalize`
    - render the gate cache from the run's accepted packet reports after completeness and snapshot checks, validate the candidate with the gate's own freshness chain, then publish it atomically
    - `gate-finalize --run RUN_ID [--timestamp T]`: reapplies the accepted severity confirmations and ownership records to the terminal finding set, then derives result, blocking, severity counts, logical statuses, and the judgment baseline from the gate-driving canonical findings (or the rule-validate packet merged with carried baseline judgments). Deferred findings are recorded in `GATE_JUDGMENTS.deferred_findings` for audit but are excluded from counts, blocking, statuses, and the carry chain. A review finalize additionally synchronizes the deferred-findings ledger: it consumes the pending deferrals the run disposed, supersedes the unit's own older deferrals for re-reviewed files, and records the run's new deferrals under their owner unit; the ledger write is idempotent and a failure after cache publication leaves the run open for a retry. It holds the gate-run mutation lock from the fresh run load through cache publication, ledger synchronization, and run consumption, so a replacement plan cannot overtake an in-flight finalize and an already replaced run cannot publish. It assembles and validates the complete cache before atomically publishing it. A rejected candidate never changes or removes the prior canonical cache. Rule delta/repair writes all eight logical statuses, not only the re-run subset. The coordinator supplies no judgment values. Candidate validate cache deletion applies only to a full-run FAIL; delta/repair FAIL writes the failure record needed for recovery.
    - corresponds to the tooled cache write of every complete-coverage quality-gate run (see `framework/commands.md` and `framework/validation_cache.md` §Write Rules → Tooled writes)
  17. `gate-invalidate`
    - `gate-invalidate --gate validate|verify|review (--unit NAME | --rule ID) --target candidate|stable --check CHECK_KEY [--check CHECK_KEY]...`: record a targeted P0/P1 without publishing a targeted result cache
    - under the gate-run mutation lock, deletes a matching pass cache or persists sorted, duplicate-free `invalidated_checks` on a matching failure record; also marks matching open gate runs `invalidated`, so an earlier plan cannot later overwrite the targeted finding
    - repair reads the persisted keys automatically. An unmappable key degrades to the full packet set; successful finalize clears the handled invalidations, while rejected finalize leaves the failure record unchanged
  18. `promote`
    - validate candidate spec format, copy candidate files to stable directories, remove candidate files, and rewrite the candidate gate caches into stable confirmation caches
   - `promote --unit <name>`: runs format checks and required-field validation (reference integrity is checked by `validate`; promote additionally rejects unit_refs/rule_refs that point only to candidate-layer files). The tool independently checks validate+verify+review+appendix cache freshness before promoting; if any cache is missing, stale, or blocking, promote is rejected with guidance to re-run the appropriate step. The review cache must be non-blocking (no P0/P1 findings). Every non-exempt candidate appendix must be listed in the validate cache. On success, the candidate gate caches are rewritten into stable confirmation caches (`target: stable`, paths rewritten to `stable/`) — the stable delta-recovery baseline for `fresh@stable`, `re*`, and `fork` (a retired promote deletes them instead). Cleanup side effect: for every bound rule (`b_rule_*`) the unit's candidate dropped from `rule_refs`, promote runs the removable-rule detection — a rule with no remaining consumers (global rules are exempt; an `unbound_retention` record defers deletion) is deleted together with the unit (its stable and candidate copies, baseline, and validate cache), and every deletion is listed explicitly in the promote report (`Removed unbound rule: <id>`; see `framework/spec_writing_guide.md` §6.5)
   - `promote --rule <id>`: validates rule frontmatter, copies candidate→stable, deletes candidate, and rewrites the rule validate cache into a stable confirmation cache. Consumer impact assessment is the agent's responsibility. The tool validates rule frontmatter and version semantics, and independently checks the rule validate cache freshness; if the cache is missing or stale, promote is rejected with guidance to re-run `validate@{rule}`
   - this is the only write gate
  19. `review collect-default-scope --flow <review_flow>`
    - collect the deterministic default scope for the explicit review flow
  20. `review run-init --flow <review_flow>`
    - create or reuse the full-scope run-state file for the explicit review flow
  21. `review run-validate --flow <review_flow>`
    - validate required run-state fields, timestamps, all fixed statuses including closed statuses, baseline slices, score state when present, and dynamic slice parent links
  22. `review run-refresh --flow <review_flow>`
    - recompute slice input fingerprints for an open run-state file, mark changed `passed` slices as `stale`, and refresh `last_updated_at`
  23. `review run-touch --flow <review_flow>`
    - refresh only `last_updated_at`
  24. `validate write`
    - check whether a file path may be written under current governance constraints
    - `validate write --path <path>` checks whether a path is in an allowed write zone under current governance constraints. The path may be absolute or relative to the current working directory; in-repository paths are matched against the governed write zones enumerated under §Governed write zones above
  25. `validate candidate --unit UNIT`
    - validate candidate spec structure (checks: frontmatter, acceptance items, anchor integrity, references, appendices, version consistency, body layer-path check, dependency cycle check, region locatability)
  26. `validate rule --id RULE_ID`
    - validate candidate rule structure (checks: frontmatter, ID/scope consistency, version semantics, promotion_owner_unit warning, prohibited fields, unbound_retention correctness)
    - File Path Consistency (Check 3) and Rule Body Quality (Check 8) are agent-only, not covered by this command
  27. `operation open`
    - declare a bounded change scope and freeze it (see §Operation scope): `operation open (--unit NAME | --rule ID)? [--allow PATH]... [--require-spec PATH]... [--baseline REF] [--parent OP_ID]`
    - resolves the spec-derived scope (target spec files, `implementation_surface`, `affects.files`) plus the caller-declared `--allow` entries, rejects entries inside denied write zones or the exclusions, records the resolved baseline commit, writes `meta/operations/<operation_id>.json`, prints the operation id and the frozen scope, and prints an informational notice when the working tree already contains changes outside the declared scope
    - a path-only operation (no `--unit`/`--rule`) requires at least one `--allow` entry; `--unit`/`--rule` are mutually exclusive
  28. `operation check --id OP_ID`
    - read-only evaluation of the frozen scope against the change set since the baseline: reports out-of-scope paths, static-policy violations, missing required spec paths, and the `PASS`/`FAIL` result; exit code 1 on `FAIL` or on any fail-closed error (missing operation, missing baseline commit, git error)
  29. `operation close --id OP_ID [--abandon]`
    - runs the same evaluation; on `PASS` marks the operation `closed` and records `closed_at` with close outcome `passed`; on `FAIL` refuses (exit code 1) and leaves the state unchanged, unless `--abandon` is given — then the operation is ended explicitly, the close outcome `abandoned` is recorded, and the violation report is printed (exit code 0)
  30. `operation update --id OP_ID [--allow PATH]... [--require-spec PATH]...`
    - adds caller-declared allowed paths and/or required spec paths to the frozen values while the operation is open, records the resulting union as an update event, and keeps the spec-derived part unchanged; it never removes an existing path and rejects a closed operation, a denied/excluded entry, or an update with neither flag
  31. `operation status [--id OP_ID]`
    - read-only: without `--id`, lists every open operation (id, target, baseline, opened_at); with `--id`, prints the full frozen scope, required spec paths, status, and update history

## Review Run-State Commands

The `review run-*` commands require an explicit review flow:

1. `spec_flow_review`
2. `spec_flow_design_review`

Review run-state commands use the `source_repo` layout:
- framework inputs from `framework/`
- templates from `templates/`
- tooling from `tooling/`
- project-instance compatibility: template bootstrap compatibility under `templates/docs/specs/` (no real project-instance `docs/specs/` required)

They maintain only mechanical fields in:

```text
meta/governance_review/spec_flow_review.md
meta/governance_review/spec_flow_design_review.md
```

Rules:

1. timestamps are written from Go runtime UTC time using `YYYY-MM-DDTHH:MM:SSZ`
2. run-state files record `review_layout` as `source_repo`
3. input fingerprints are computed from repository-relative input files
4. `run-refresh` may change `passed` slices to `stale` when inputs change or disappear
5. tooling must not change `pending`, `blocked`, or `skipped_not_in_scope` into a passing judgment
6. tooling may create and validate the `spec_flow_design_review` score-state skeleton
7. tooling must not write findings, severities, non-blocking optimizations, question scores, score basis, hard-blocker judgments, or final conclusions owned by the active review policy
   - `spec_flow_review` final conclusions are `pass | blocked`
   - `spec_flow_design_review` final conclusions are `pass | pass-with-optimization | blocked`
8. each review flow uses one fixed run-state file
9. when the fixed run-state file is missing, tooling creates the file for a new full-scope review
10. when a new full-scope review starts after a closed or invalid run-state file, tooling deletes the old fixed file before writing the new run state
11. `run-validate` checks structural validity only; a closed run-state file can validate successfully while still remaining unavailable for reuse
12. when the fixed run-state file is valid and open, `run-init` applies the owning review policy's age rule:
   - no more than two hours old: reuse automatically
   - for `spec_flow_review`, more than two hours and no more than 24 hours old: stop for a manual reuse-or-delete decision
   - for `spec_flow_review`, more than 24 hours and no more than seven days old: stop for a manual reuse-or-delete decision and recommend deleting the old run state and starting a new run
   - for `spec_flow_design_review`, more than two hours and no more than seven days old: stop for a manual reuse-or-delete decision
   - more than seven days old: delete as expired and create a new run state
13. after reusing an open run-state file, callers must run `review run-refresh` before continuing review work so changed inputs become stale slices instead of hidden drift
14. `review run-refresh` is the authoritative command for updating `input_fingerprint`; callers must not write manual hash output into run-state files
15. `spec_flow_review` baseline run state includes `supporting_layer_convergence` to force explicit review of promote paths for stable and candidate supporting truth
16. review run-state slice fields follow the generic protocol only through the adoption rules in the active review policy


## Tooling Input Set

The default `spec_flow_review` tooling review input set is:

1. the framework tooling policy and this README
2. the current tooling source input set listed below
3. the tooling helper script input set listed below
The current tooling source input set is:

1. `<tooling-root>/cmd/**/*.go`
2. `<tooling-root>/internal/**/*.go`
3. `<tooling-root>/go.mod`
4. `<tooling-root>/manifest.tsv`
5. `<tooling-root>/go.sum` when it exists

The tooling helper script input set is every regular file under:

```text
<tooling-root>/scripts/**
```

This includes install, pull-with-release, push-with-release, build-release, update-tooling-binaries, and version-check scripts.

The manifest is included because it controls which framework-managed and project-managed files `init` and `doctor` inspect or write.
Tooling helper scripts are review inputs because they rebuild or select binaries for the installed tooling source.
They are not binary freshness inputs unless they change compiled binary behavior.

## Tooling Fingerprint Distribution

The tooling source fingerprint has a single authoritative implementation: `toolingfreshness.LiveFingerprint` (used by `build-release` to embed the fingerprint into release binaries). Release scripts do not recompute hashes themselves.

- `specflowctl tooling-fingerprint [--short] [--repo-root PATH]` prints the live fingerprint of the current working tree.
- `push_with_release.sh`/`.ps1` compute the fingerprint via `go run ./cmd/specflowctl tooling-fingerprint` (requires a Go toolchain on the machine running the push) and record it into `tooling/fingerprint.txt` as a release-metadata commit before tagging.
- `tooling/fingerprint.txt` is tracked by git and ships with every checkout. It is not part of the tooling source input set above, so recording it never changes the fingerprint it records.
- Consumer projects run `update_tooling_binaries.sh`/`.ps1`, which read `tooling/fingerprint.txt` and download the matching release binary from the `specflow-tooling-<short-fingerprint>` tag. No local hash computation is needed.

## Usage Examples

Run ordinary governance commands from the repository root using the matching platform binary under `specflow/tooling/bin/`.
For normal use, download the matching `specflowctl-*` files from the GitHub Release for the installed tooling fingerprint.
For local tooling development, rebuild them with `build-release`.

When developing the tooling itself, do not assume that ordinary commands may run through `go run`.
The freshness gate requires an embedded build fingerprint for ordinary governance actions.
The commands that may still run through `go run` are exactly the recovery and inspection surface listed under [Freshness Rule](#freshness-rule) — the two surfaces are the same bypass set, so there is only one list to keep current.

Examples:

```bash
./specflow/tooling/bin/specflowctl-linux-amd64 doctor
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-init --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-init --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-validate --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-refresh --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-touch --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 next --unit ai
./specflow/tooling/bin/specflowctl-linux-amd64 promote --unit ai
```

## Freshness Rule

Compiled binaries under `<tooling-root>/bin/` are local cache files.
They must fail closed when the embedded tooling fingerprint no longer matches current source.
The fingerprint hashes tooling-root-relative keys such as `cmd/...`, `internal/...`, `go.mod`, and `manifest.tsv`, so identical tooling content has one fingerprint in both layouts.

The local development recovery path is:

```bash
cd specflow/tooling
go run ./cmd/specflowctl build-release --repo-root ../..
```

The normal user recovery path is to download the matching release binaries again for the installed tooling fingerprint.

The minimal stale-binary recovery and inspection surface remains:

1. `build-release`
2. `tooling-fingerprint`
3. `doctor`
4. `help`
5. the internal build-fingerprint query command
6. `next`, `deps` — these are read-only render actions that do not modify project files or advance governance state
