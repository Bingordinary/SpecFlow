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

Pull the repository and install the current platform binaries for the pulled tooling source:

```bash
specflow/tooling/scripts/pull_with_release.sh
```

PowerShell:

```powershell
.\specflow\tooling\scripts\pull_with_release.ps1
```

The script runs a fast-forward pull (fetch + reset to the remote branch), reads the recorded tooling fingerprint from `tooling/fingerprint.txt`, and downloads the current platform's `specflowctl` and `SHA256SUMS` only when the local binary is missing, stale, or missing checksums. Note that the pull resets the local SpecFlow repository to the remote branch; unpushed local commits in `specflow/` are discarded.

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

The tooling layer must not:

1. invent new governance semantics
2. replace governance judgment
3. replace shared-boundary judgment
4. replace review severity or final conclusion judgment owned by the active review policy
5. become a second semantic source of truth
6. write reader-derived conclusions back into project files

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
     - `fresh --unit <name>` / `fresh --rule <id>`: detail for one target — candidate gate statuses, or confirmation states + drift for a stable-only target (no candidate file); every STALE unit gate appends a `DELTA SCOPE (<gate>)` section — the mechanism-derived delta re-run scope (affected check keys from the cache's per-check `checks` mapping, unclaimed entries, stale-dep count, degradation state), the input for `revalidate@`/`reverify@`/`rereview@`
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
    - compute the dependency evidence for one file read during a validate/verify/review run
    - `gate-evidence --file <path>` with optional `--ranges START-END,START-END` (1-based, inclusive; empty means the whole file): maps the declared line ranges onto content-defined chunks and outputs the whole-file `hash` + the `deps` chunk CIDs to record in the cache's `files` entry
    - with `--acceptance-items`: declares the `acceptance_item_set` structural region as the dependency (`region:acceptance_items:<cid>`), located by structure rather than line numbers or chunk boundaries; with an empty `--ranges` it replaces the whole-file declaration, with `--ranges` both are declared (see `framework/validation_cache.md` §Structural Region Dependencies)
    - with `--section <heading>` (repeatable): declares the section region with that heading text as the dependency (`region:section:<heading>:<cid>`), located by heading rather than line numbers — the frontmatter region (file head through the line before the first `##` heading) is named `frontmatter`; a missing or duplicated heading fails closed
    - with `--sections`: lists every section region (heading, line range, CID) without declaring dependencies — the informational output that names the `--section` values
    - corresponds to the `specflowctl gate-evidence` agent-trigger row (see `framework/concepts.md` and `framework/validation_cache.md` §Dependency Declaration)
  12. `cache-write`
    - write a gate cache file with the machine-consumed evidence computed by the tooling, then self-check the written file with the gate's own freshness chain
    - `cache-write --gate validate|verify|review (--unit NAME | --rule ID) --result pass|fail --target candidate|stable [--basis full|delta|repair] [--blocking] [--p0-count N --p1-count N --p2-count N --p3-count N]`: the agent supplies the judgment (result, basis, target, blocking, severity counts, findings body, per-check status map) and declares each files entry with a repeatable `--file '<json>'` — `{"path":"src/auth/login.go","checks":[{"check":"5","status":"pass","sections":["Description"]}],"sections":[...],"ranges":"...","acceptance_items":bool}` (path may be a logical reference `unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`). The tooling computes each entry's whole-file `hash` and `deps` CIDs from the declared scope (same declarations `gate-evidence` accepts; `sections` accepts the reserved spelling `frontmatter` naming the pre-`##` region, same as `gate-evidence --section`; the schema has no hash/deps fields, so a transcribed CID cannot enter the cache), enforces union discipline (every per-check dep joins the file-level `deps`), requires a per-check `status` map on failure records, then re-reads the file and runs the gate's own checks (pass → FRESH; failure record → BLOCKED; a pass validate@ candidate cache additionally requires appendix coverage). A non-accepted result is a write failure (non-zero exit)
    - rule targets support the validate gate only (rule verify/review removed)
    - corresponds to the tooled cache-write step of every quality-gate run (see `framework/concepts.md` and `framework/validation_cache.md` §Write Rules → Tooled writes)
  13. `promote`
    - validate candidate spec format, copy candidate files to stable directories, and remove candidate files
   - `promote --unit <name>`: runs format checks and required-field validation (reference integrity is checked by `validate`; promote additionally rejects unit_refs/rule_refs that point only to candidate-layer files). The tool independently checks validate+verify+review+appendix cache freshness before promoting; if any cache is missing, stale, or blocking, promote is rejected with guidance to re-run the appropriate step. The review cache must be non-blocking (no P0/P1 findings). Every non-exempt candidate appendix must be listed in the validate cache
   - `promote --rule <id>`: validates rule frontmatter, copies candidate→stable, deletes candidate. Consumer impact assessment is the agent's responsibility. The tool validates rule frontmatter and version semantics, and independently checks the rule validate cache freshness; if the cache is missing or stale, promote is rejected with guidance to re-run `validate@{rule}`
   - this is the only write gate
  14. `review collect-default-scope --flow <review_flow>`
    - collect the deterministic default scope for the explicit review flow
  15. `review run-init --flow <review_flow>`
    - create or reuse the full-scope run-state file for the explicit review flow
  16. `review run-validate --flow <review_flow>`
    - validate required run-state fields, timestamps, all fixed statuses including closed statuses, baseline slices, score state when present, and dynamic slice parent links
  17. `review run-refresh --flow <review_flow>`
    - recompute slice input fingerprints for an open run-state file, mark changed `passed` slices as `stale`, and refresh `last_updated_at`
  18. `review run-touch --flow <review_flow>`
    - refresh only `last_updated_at`
  19. `validate write`
    - check whether a file path may be written under current governance constraints
    - `validate write --path <path>` checks whether a path is in an allowed write zone under current governance constraints. The path may be absolute or relative to the current working directory; in-repository paths are matched against the governed write zones
  20. `validate candidate --unit UNIT`
    - validate candidate spec structure (checks: frontmatter, acceptance items, anchor integrity, references, appendices, version consistency, body layer-path check, dependency cycle check, region locatability)
  21. `validate rule --id RULE_ID`
    - validate candidate rule structure (checks: frontmatter, ID/scope consistency, version semantics, promotion_owner_unit warning, prohibited fields, unbound_retention correctness)
    - File Path Consistency (Check 3) and Rule Body Quality (Check 8) are agent-only, not covered by this command

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
