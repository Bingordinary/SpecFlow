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
8. maintain mechanical review run-state fields

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
8. `review collect-default-scope --flow <review_flow>`
   - collect the deterministic default scope for the explicit review flow
9. `review run-init --flow <review_flow>`
   - create or reuse the full-scope run-state file for the explicit review flow
10. `review run-validate --flow <review_flow>`
   - validate required run-state fields, timestamps, fixed statuses, baseline slices, score state when present, and dynamic slice parent links
11. `review run-refresh --flow <review_flow>`
   - recompute slice input fingerprints, mark changed `passed` slices as `stale`, and refresh `last_updated_at`
12. `review run-touch --flow <review_flow>`
   - refresh only `last_updated_at`
13. `snapshot rebuild`
   - rebuild current process snapshots from bound truth
14. `snapshot validate-process`
   - compare one process file against rebuilt current truth
15. `process cleanup-fallback`
   - execute deterministic unit fallback cleanup
16. `process cleanup-success`
   - execute deterministic unit success cleanup
17. `status set-unit`
   - write one deterministic `unit` row in `_status.md`
18. `status set-object`
   - write one unified object row in `_status.md`
19. `shared sync-impact`
   - compute shared-specific scope, resolve shared-only exceptions into generic impact input, then execute deterministic downstream fallback for the fixed affected objects through internal `impact_sync`
   - when stable landing self-exemption is needed, the caller must pass both `--stable-landing-unit` and exact `--stable-landing-shared-refs`
   - when a current-round Shared Contract file delta is proven to be limited to `bound_objects` metadata, the caller must pass its exact file path through `--bound-objects-only-shared-file-refs`
   - the caller may narrow the derived unit subset with `--units`, but at least one shared trigger input must still be provided through `--shared-refs` or `--shared-ids`
20. `shared reconcile-bound-objects`
   - rewrite Shared Contract `bound_objects` metadata from current formal bindings

## Review Run-State Commands

The `review run-*` commands require an explicit review flow:

1. `spec_flow_review`
2. `spec_flow_design_review`

They maintain only mechanical fields in:

```text
docs/specs/_governance_review/spec_flow_review/{review_run_id}.md
docs/specs/_governance_review/spec_flow_design_review/{review_run_id}.md
```

Rules:

1. timestamps are written from Go runtime UTC time using `YYYY-MM-DDTHH:MM:SSZ`
2. input fingerprints are computed from repository-relative input files
3. `run-refresh` may change `passed` slices to `stale` when inputs change or disappear
4. tooling must not change `pending`, `blocked`, or `skipped_not_in_scope` into a passing judgment
5. tooling may create and validate the `spec_flow_design_review` score-state skeleton
6. tooling must not write findings, severities, question scores, score basis, hard-blocker judgments, or final `pass | blocked` conclusions

## Tooling Input Set

The current tooling source input set is:

1. `specflow/tooling/cmd/**/*.go`
2. `specflow/tooling/internal/**/*.go`
3. `specflow/tooling/go.mod`
4. `specflow/tooling/manifest.tsv`
5. `specflow/tooling/go.sum` when it exists

The manifest is included because it controls which framework-managed and project-managed files `init`, `upgrade`, and `doctor` inspect or write.

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
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-init --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-init --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-validate --flow spec_flow_review --file docs/specs/_governance_review/spec_flow_review/20260426-103000-default_governance_baseline.md
./specflow/tooling/bin/specflowctl-linux-amd64 review run-refresh --flow spec_flow_design_review --file docs/specs/_governance_review/spec_flow_design_review/20260426-103000-default_design_baseline.md
./specflow/tooling/bin/specflowctl-linux-amd64 review run-touch --flow spec_flow_design_review --file docs/specs/_governance_review/spec_flow_design_review/20260426-103000-default_design_baseline.md
./specflow/tooling/bin/specflowctl-linux-amd64 snapshot rebuild --unit ai
./specflow/tooling/bin/specflowctl-linux-amd64 process cleanup-fallback --unit ai --from-command unit_promote --reason evidence_incomplete
./specflow/tooling/bin/specflowctl-linux-amd64 status set-object --type scenario --object task_execution --stable yes --candidate no --active-layer stable --next-command scenario_fork
./specflow/tooling/bin/specflowctl-linux-amd64 shared sync-impact --shared-refs c_shared_app_config_topology@0.2.0 --units ai
./specflow/tooling/bin/specflowctl-linux-amd64 shared sync-impact --shared-refs s_shared_app_config_topology@0.2.0 --bound-objects-only-shared-file-refs docs/specs/shared_contracts/stable/s_shared_app_config_topology.md
./specflow/tooling/bin/specflowctl-linux-amd64 shared reconcile-bound-objects --shared-ids shared_app_config_topology
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
