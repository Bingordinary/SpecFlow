# SpecFlow Tooling

This directory contains the standalone Go CLI that performs deterministic governance actions for `specFlow`.

The tooling layer exists only for fixed execution work whose meaning is already constrained by governance rules.

## Build

Rebuild binaries from the repository root:

```bash
cd specflow/tooling
go run ./cmd/specflowctl build-release --repo-root ../..
```

## Governance Boundary

The tooling layer may:

1. collect
2. parse
3. validate
4. rebuild
5. compare
6. cleanup
7. sync

The tooling layer must not:

1. invent new lifecycle semantics
2. replace command closure judgment
3. replace shared-boundary judgment
4. replace review severity or `pass | blocked` judgment
5. become a second semantic source of truth

`impact_sync` is a governance concept first.
The current CLI exposes only the deterministic pieces already justified by rules.
For shared-change reconciliation, the current mechanical entry remains `shared sync-impact`, but that entry must first compute `shared_sync` scope and exceptions and only then hand the fixed downstream object set to internal `impact_sync`.

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
8. `review collect-default-scope`
   - collect the deterministic default `spec_flow_review` scope
9. `snapshot rebuild`
   - rebuild current process snapshots from bound truth
10. `snapshot validate-process`
   - compare one process file against rebuilt current truth
11. `process cleanup-fallback`
   - execute deterministic module fallback cleanup
12. `process cleanup-success`
   - execute deterministic module success cleanup
13. `status set-module`
   - write one legacy module row in `_status.md`
14. `status set-object`
   - write one unified object row in `_status.md`
15. `shared sync-impact`
   - compute shared-specific scope, resolve shared-only exceptions into generic impact input, then execute deterministic downstream fallback for the fixed affected objects through internal `impact_sync`
   - when stable landing self-exemption is needed, the caller must pass both `--stable-landing-module` and exact `--stable-landing-shared-refs`
   - the caller may narrow the derived module subset with `--modules`, but at least one shared trigger input must still be provided through `--shared-refs` or `--shared-ids`
16. `shared reconcile-bound-modules`
   - rewrite Shared Contract `bound_modules` metadata from current formal bindings

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
2. `status set-module` remains available for module-scoped deterministic writeback
3. tooling must not infer the next command; callers must pass it explicitly

## Usage Examples

Run ordinary governance commands from the repository root using the matching platform binary.

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
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope
./specflow/tooling/bin/specflowctl-linux-amd64 snapshot rebuild --module module_ai
./specflow/tooling/bin/specflowctl-linux-amd64 process cleanup-fallback --module module_ai --from-command module_promote --reason evidence_incomplete
./specflow/tooling/bin/specflowctl-linux-amd64 status set-object --type flow --object flow_task_execution --stable yes --candidate no --active-layer stable --next-command flow_fork
./specflow/tooling/bin/specflowctl-linux-amd64 shared sync-impact --shared-refs c_shared_app_config_topology@0.2.0 --modules module_ai
./specflow/tooling/bin/specflowctl-linux-amd64 shared reconcile-bound-modules --shared-ids shared_app_config_topology
```

## Freshness Rule

Compiled binaries under `specflow/tooling/bin/` must fail closed when the embedded tooling fingerprint no longer matches current source.

The normal recovery path is:

```bash
cd specflow/tooling
go run ./cmd/specflowctl build-release --repo-root ../..
```

The minimal stale-binary recovery and inspection surface remains:

1. `build-release`
2. `doctor`
3. `help`
4. the internal build-fingerprint query command
