# SpecFlow Tooling

This directory contains the standalone Go CLI that performs deterministic governance actions for `specFlow`.

The tooling layer exists only for fixed execution work whose meaning is already constrained by governance rules.

## Build

`specflow/tooling/bin/` is a local binary cache.
It is ignored by git and must not be committed.

Rebuild local binaries from the repository root:

```bash
cd specflow/tooling
go run ./cmd/specflowctl build-release --repo-root ../..
```

Official platform binaries are GitHub Release assets.
Release tags use the source commit form `specflow-<12-character-commit>`.
The release workflow builds binaries from the tagged source and uploads the binaries plus `SHA256SUMS`.

Download release binaries for the installed source revision:

```bash
mkdir -p specflow/tooling/bin
tag="specflow-$(git -C specflow rev-parse --short=12 HEAD)"
base="https://github.com/Bingordinary/SpecFlow/releases/download/${tag}"
curl -L -o specflow/tooling/bin/specflowctl-linux-amd64 "${base}/specflowctl-linux-amd64"
curl -L -o specflow/tooling/bin/specflow-reader-linux-amd64 "${base}/specflow-reader-linux-amd64"
curl -L -o specflow/tooling/bin/SHA256SUMS "${base}/SHA256SUMS"
chmod +x specflow/tooling/bin/specflowctl-linux-amd64 specflow/tooling/bin/specflow-reader-linux-amd64
(cd specflow/tooling/bin && sha256sum -c SHA256SUMS --ignore-missing)
```

Replace `linux-amd64` with the target platform suffix:
`darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64`, `windows-amd64.exe`, or `windows-arm64.exe`.

## Governance Boundary

The tooling layer may:

1. collect
2. parse
3. validate
4. rebuild
5. compare
6. cleanup
7. sync
8. render read-only local views
9. maintain mechanical review run-state fields

The tooling layer must not:

1. invent new lifecycle semantics
2. replace command closure judgment
3. replace shared-boundary judgment
4. replace review severity or `pass | blocked` judgment
5. become a second semantic source of truth
6. write reader-derived conclusions back into project files

`impact_sync` is a governance concept first.
The current CLI exposes only the deterministic pieces already justified by rules.
For shared-change reconciliation, the current mechanical entry remains `shared sync-impact`, but that entry must first compute `shared_sync` scope and exceptions and only then hand the fixed downstream object set to internal `impact_sync`.

`specflow-reader` is a read-only local view over current truth files.
It may parse `docs/specs/**`, build an in-memory graph, serve local HTML from `specflow/tooling/reader/web`, and refresh that view when truth files change.
It must not edit files, advance lifecycle state, or store semantic conclusions outside process memory.

## Current Command Surface

1. `init`
   - bootstrap framework-managed files
2. `doctor`
   - inspect installation and binary freshness health
3. `upgrade`
   - refresh framework-managed files and managed blocks
4. `build-release`
   - rebuild cross-platform binaries
5. `entry check`
   - inspect registered entry managed-block consistency
6. `entry sync`
   - sync registered project-side entry managed blocks from one explicit source
7. `registry validate`
   - validate `docs/project_standards/_registry.md`
8. `repository-mapping validate`
   - validate `docs/specs/repository_mapping.md` path rules against `_status.md` and declared shared contract files
9. `review collect-default-scope --flow <review_flow>`
   - collect the deterministic default scope for the explicit review flow
10. `review run-init --flow <review_flow>`
   - create or reuse the full-scope run-state file for the explicit review flow
11. `review run-validate --flow <review_flow>`
   - validate required run-state fields, timestamps, all fixed statuses including closed statuses, baseline slices, score state when present, and dynamic slice parent links
12. `review run-refresh --flow <review_flow>`
   - recompute slice input fingerprints for an open run-state file, mark changed `passed` slices as `stale`, and refresh `last_updated_at`
13. `review run-touch --flow <review_flow>`
   - refresh only `last_updated_at`
14. `snapshot rebuild`
   - rebuild current process snapshots from bound truth
15. `snapshot validate-process`
   - compare one process file against rebuilt current truth
16. `process cleanup-fallback`
   - execute deterministic unit fallback cleanup
17. `process cleanup-success`
   - execute deterministic unit success cleanup
18. `status set-unit`
   - write one deterministic `unit` row in `_status.md`
19. `status set-object`
   - write one unified object row in `_status.md`
20. `shared sync-impact`
   - compute shared-specific scope, resolve shared-only exceptions into generic impact input, then execute deterministic downstream fallback for the fixed affected objects through internal `impact_sync`
   - when stable landing self-exemption is needed, the caller must pass both `--stable-landing-unit` and exact `--stable-landing-shared-refs`
   - when a current-round Shared Contract file delta is proven to be limited to `bound_objects` metadata, the caller must pass its exact file path through `--bound-objects-only-shared-file-refs`
   - the caller may narrow the derived unit subset with `--units`, but at least one shared trigger input must still be provided through `--shared-refs` or `--shared-ids`
21. `shared reconcile-bound-objects`
   - rewrite Shared Contract `bound_objects` metadata from current formal bindings

## Reader Command Surface

`specflow-reader` is a separate binary from `specflowctl`.
It starts one local reader server directly and has no public subcommands:

```bash
cd specflow/tooling/bin
./specflow-reader-linux-amd64 --addr 127.0.0.1:17863
```

Rules:

1. running `specflow-reader` starts a local HTTP server and prints the URL.
2. the reader does not open a browser automatically.
3. when `--repo-root` is omitted, it defaults to `../../..` from the current working directory.
4. the server reads front-end files from `specflow/tooling/reader/web`.
5. the server reads current project truth from `docs/specs/**`.
6. each `/api/snapshot` request rebuilds the displayed snapshot from disk before returning data.
7. the server does not watch files and does not expose a server-sent event stream.
8. `/api/source` may return source text only from allowed truth and support files under the requested repository root.
9. the hidden build-fingerprint query command is reserved for freshness checks.

Reader front-end rules:

1. `specflow/tooling/reader/web` is the only runtime source for reader HTML, CSS, and JavaScript.
2. `specflow-reader` does not embed a fallback copy of the front-end.
3. editing front-end files does not require rebuilding `specflow-reader`; refresh the browser after the file change.
4. `doctor` reports missing required reader front-end files as installation failures.
5. the reader front-end supports Chinese and English interface text.
6. language switching affects only reader-owned UI text.
7. Spec document source text, file paths, object IDs, command names, and version values are displayed as source data and must not be translated by the front-end.
8. the selected reader language may be stored in browser-local state and must not be written into project files.
9. the front-end refresh button requests a new snapshot immediately.
10. the front-end also polls `/api/snapshot` on a fixed interval so open pages converge to the latest disk state without relying on filesystem events.

## Review Run-State Commands

The `review run-*` commands require an explicit review flow:

1. `spec_flow_review`
2. `spec_flow_design_review`

They maintain only mechanical fields in:

```text
docs/specs/_governance_review/spec_flow_review.md
docs/specs/_governance_review/spec_flow_design_review.md
```

Rules:

1. timestamps are written from Go runtime UTC time using `YYYY-MM-DDTHH:MM:SSZ`
2. input fingerprints are computed from repository-relative input files
3. `run-refresh` may change `passed` slices to `stale` when inputs change or disappear
4. tooling must not change `pending`, `blocked`, or `skipped_not_in_scope` into a passing judgment
5. tooling may create and validate the `spec_flow_design_review` score-state skeleton
6. tooling must not write findings, severities, question scores, score basis, hard-blocker judgments, or final `pass | blocked` conclusions
7. each review flow uses one fixed run-state file
8. when the fixed run-state file is missing, tooling creates the file for a new full-scope review
9. when a new full-scope review starts after a closed or invalid run-state file, tooling deletes the old fixed file before writing the new run state
10. `run-validate` checks structural validity only; a closed run-state file can validate successfully while still remaining unavailable for reuse
11. when the fixed run-state file is valid and open, `run-init` applies the owning review policy's age rule:
   - no more than two hours old: reuse automatically
   - for `spec_flow_review`, more than two hours and no more than 24 hours old: stop for a manual reuse-or-delete decision
   - for `spec_flow_review`, more than 24 hours and no more than seven days old: stop for a manual reuse-or-delete decision and recommend deleting the old run state and starting a new run
   - for `spec_flow_design_review`, more than two hours and no more than seven days old: stop for a manual reuse-or-delete decision
   - more than seven days old: delete as expired and create a new run state
12. after reusing an open run-state file, callers must run `review run-refresh` before continuing review work so changed inputs become stale slices instead of hidden drift

## Tooling Input Set

The default `spec_flow_review` tooling review input set is:

1. the framework tooling policy and this README
2. the current tooling source input set listed below
3. reader runtime files under `specflow/tooling/reader/web/**`

The current tooling source input set is:

1. `specflow/tooling/cmd/**/*.go`
2. `specflow/tooling/internal/**/*.go`
3. `specflow/tooling/go.mod`
4. `specflow/tooling/manifest.tsv`
5. `specflow/tooling/go.sum` when it exists

The manifest is included because it controls which framework-managed and project-managed files `init`, `upgrade`, and `doctor` inspect or write.
Reader front-end files under `specflow/tooling/reader/web/**` are runtime files, not binary freshness inputs.

## Unified Status Table

`docs/specs/_status.md` now uses the unified object-state table:

1. `Object Type`
2. `Object`
3. `Stable`
4. `Candidate`
5. `Active Layer`
6. `Next Command`
7. `Notes`

Rules:

1. `status set-object` is the primary write surface for the unified table
2. `status set-unit` remains available for unit-scoped deterministic writeback
3. tooling must not infer the next command; callers must pass it explicitly

## Usage Examples

Run ordinary governance commands from the repository root using the matching platform binary under `specflow/tooling/bin/`.
For normal use, download the matching `specflowctl-*` and `specflow-reader-*` files from the GitHub Release for the installed source revision.
For local tooling development, rebuild them with `build-release`.

When developing the tooling itself, do not assume that ordinary commands may run through `go run`.
The freshness gate requires an embedded build fingerprint for ordinary governance actions.
The supported `go run` recovery and inspection surface remains:

1. `build-release`
2. `doctor`
3. `help`
4. the internal build-fingerprint query command

Examples:

```bash
./specflow/tooling/bin/specflowctl-linux-amd64 doctor
./specflow/tooling/bin/specflow-reader-linux-amd64 --repo-root . --addr 127.0.0.1:17863
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-init --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-init --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-validate --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-refresh --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-touch --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 snapshot rebuild --unit ai
./specflow/tooling/bin/specflowctl-linux-amd64 process cleanup-fallback --unit ai --from-command unit_promote --reason evidence_incomplete
./specflow/tooling/bin/specflowctl-linux-amd64 status set-object --type scenario --object task_execution --stable yes --candidate no --active-layer stable --next-command scenario_fork
./specflow/tooling/bin/specflowctl-linux-amd64 shared sync-impact --shared-refs c_shared_app_config_topology@0.2.0 --units ai
./specflow/tooling/bin/specflowctl-linux-amd64 shared sync-impact --shared-refs s_shared_app_config_topology@0.2.0 --bound-objects-only-shared-file-refs docs/specs/shared_contracts/stable/s_shared_app_config_topology.md
./specflow/tooling/bin/specflowctl-linux-amd64 shared reconcile-bound-objects --shared-ids shared_app_config_topology
```

## Freshness Rule

Compiled binaries under `specflow/tooling/bin/` are local cache files.
They must fail closed when the embedded tooling fingerprint no longer matches current source.

The local development recovery path is:

```bash
cd specflow/tooling
go run ./cmd/specflowctl build-release --repo-root ../..
```

The normal user recovery path is to download the matching release binaries again for the installed source revision.

The minimal stale-binary recovery and inspection surface remains:

1. `build-release`
2. `doctor`
3. `help`
4. the internal build-fingerprint query command
