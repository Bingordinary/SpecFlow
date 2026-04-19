# SpecFlow Tooling

This directory contains the standalone Go CLI that performs deterministic governance actions for `specFlow`.

Compiled binaries are placed under:

1. `specflow/tooling/bin/specflowctl-linux-amd64`
2. `specflow/tooling/bin/specflowctl-linux-arm64`
3. `specflow/tooling/bin/specflowctl-darwin-amd64`
4. `specflow/tooling/bin/specflowctl-darwin-arm64`
5. `specflow/tooling/bin/specflowctl-windows-amd64.exe`
6. `specflow/tooling/bin/specflowctl-windows-arm64.exe`

## Build

To rebuild those binaries from source, run from the repository root:

```bash
go run ./specflow/tooling/cmd/specflowctl build-release --repo-root .
```

## Usage

Run the compiled binary from the repository root. On Linux amd64, the command path is:

```bash
./specflow/tooling/bin/specflowctl-linux-amd64
```

Examples:

```bash
./specflow/tooling/bin/specflowctl-linux-amd64 init
./specflow/tooling/bin/specflowctl-linux-amd64 doctor
./specflow/tooling/bin/specflowctl-linux-amd64 registry validate
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope
./specflow/tooling/bin/specflowctl-linux-amd64 snapshot rebuild --module module_ai
./specflow/tooling/bin/specflowctl-linux-amd64 process cleanup-fallback --module module_ai --from-command cand_promote --reason evidence_incomplete
./specflow/tooling/bin/specflowctl-linux-amd64 status set-module --module module_ai --stable yes --candidate no --active-layer stable --next-command spec_fork --notes "promoted"
./specflow/tooling/bin/specflowctl-linux-amd64 process cleanup-success --module module_ai --mode cand_promote
./specflow/tooling/bin/specflowctl-linux-amd64 shared sync-impact --modules module_ai --shared-refs c_shared_app_config_topology@0.2.0
./specflow/tooling/bin/specflowctl-linux-amd64 shared reconcile-bound-modules --shared-ids shared_app_config_topology
```

## Governance Boundary

The tooling layer exists only for fixed execution work whose meaning is already constrained by governance rules.

The boundary is:

1. tooling is justified only when the input is already fixed, the output is mechanically derivable, and the manual action is repetitive enough to be worth automating
2. tooling may collect, parse, validate, rebuild, compare, cleanup, and sync
3. tooling may narrow scope through explicit caller parameters, but it must not invent new governance meaning
4. tooling must not become a second semantic source of truth

Compiled-binary freshness is also part of that boundary:

1. source changes under `specflow/tooling/cmd/`, `specflow/tooling/internal/`, `go.mod`, or `go.sum` change the live tooling fingerprint
2. `build-release` embeds the current tooling fingerprint into each built binary
3. ordinary binary execution compares the embedded fingerprint with the live source fingerprint before continuing
4. if they differ, the binary stops and requires a rebuild instead of continuing with stale behavior
5. only the smallest recovery and inspection entry points may bypass that freshness gate

The CLI is intentionally not responsible for:

1. `cand_check` closure judgment
2. shared, module, or system boundary judgment
3. verification evidence sufficiency judgment
4. severity, downgrade, checkpoint, or `pass | blocked` judgment
5. deciding whether one tooling function is justified in the first place

Those remain in the governance documents and the agent runtime.

The tooling contract is split across these locations:

1. `specflow/framework/docs/agent_guidelines/tooling_execution_policy.md` defines the framework-level boundary rules.
2. this README defines the concrete command surface, build flow, recovery flow, and usage examples.
3. the Go source under `specflow/tooling/cmd/` and `specflow/tooling/internal/` implements the fixed execution actions.

Project-root `docs/` files are not a separate tooling contract layer.

## Current Command Surface

Each command below is present because it satisfies the tooling necessity contract: fixed input, mechanical output, clear governance owner, and repeatable execution value.

1. `init`
   - necessary because framework bootstrap must copy one fixed manifest-defined file set without manual omission
   - fixed input: repository root and `manifest.tsv`
   - fixed output: copied and skipped managed files
   - not responsible for: deciding which files the framework should own
2. `doctor`
   - necessary because install health checks are repetitive and shape-based
   - fixed input: repository root, hook path, and current-platform binary path
   - fixed output: deterministic warnings and failures about installation health
   - not responsible for: deciding whether one governance rule is good
3. `upgrade`
   - necessary because refreshing framework-managed files and managed blocks is repetitive and omission-prone
   - fixed input: repository root and the framework-owned managed sources
   - fixed output: updated and skipped framework-managed files
   - not responsible for: deciding whether a project-specific customization should replace the framework baseline
4. `build-release`
   - necessary because cross-platform binary builds are repetitive and shape-driven
   - fixed input: repository root and the current tooling source tree
   - fixed output: rebuilt binaries under `specflow/tooling/bin/`
   - not responsible for: deciding what the tooling command surface should be
   - freshness role: embeds the current tooling source fingerprint into each built binary
5. `entry check`
   - necessary because registered entry managed-block consistency is a deterministic comparison
   - fixed input: repository root and the registered entry-file set
   - fixed output: consistent or inconsistent status plus the staged-change context
   - not responsible for: deciding whether entry-file wording is semantically correct
6. `entry sync`
   - necessary because managed-block alignment is deterministic once the source entry file is known
   - fixed input: repository root, one chosen source entry file, and the managed-block contract
   - fixed output: synchronized registered entry files
   - not responsible for: guessing the source when the governance rules require an explicit source choice
7. `registry validate`
   - necessary because project-standard registry shape checks are repetitive and contract-based
   - fixed input: `docs/project_standards/_registry.md`, allowed surfaces, and allowed selector shapes
   - fixed output: registry diagnostics or a valid result
   - not responsible for: deciding whether one project-local standard is a good idea
8. `review collect-default-scope`
   - necessary because the default `spec_flow_review` scope is deterministic and easy to miss by hand
   - fixed input: repository root, the framework-defined default review scope, and the active registry state
   - fixed output: grouped file lists for the default governance review, including tooling contract files and tooling source files
   - not responsible for: deciding whether the reviewed files pass governance review
9. `process cleanup-fallback`
   - necessary because command-defined fallback cleanup is mechanical once the origin command and fallback reason are known
   - fixed input: module name, origin command, fallback reason, and the command-defined cleanup contract
   - fixed output: deleted process files and deterministic `_status.md` writeback
   - not responsible for: deciding whether fallback should happen in the first place
10. `snapshot rebuild`
    - necessary because process snapshots are deterministic derivatives of current formal truth
    - fixed input: module name and the bound truth-reading surface
    - fixed output: rebuilt current snapshot data
    - not responsible for: deciding whether the truth itself is sufficient
11. `snapshot validate-process`
    - necessary because process-snapshot comparisons are deterministic once rebuilt current truth is available
    - fixed input: one existing process file and rebuilt current snapshot inputs
    - fixed output: matching or drift diagnostics
    - not responsible for: deciding whether a drift is acceptable
12. `status set-module`
    - necessary because one `_status.md` row writeback is deterministic once the row data is fixed
    - fixed input: module identity and explicit row field values
    - fixed output: one created or updated module row
    - not responsible for: deciding what the next command should be
13. `process cleanup-success`
    - necessary because success-path cleanup for `spec_fork` and `cand_promote` is command-defined and repetitive
    - fixed input: module name, success mode, and the command-defined cleanup contract
    - fixed output: deleted process files for the closed round
    - not responsible for: deciding whether promotion or fork semantics are correct
14. `shared sync-impact`
    - necessary because comparing shared truth, bindings, and process snapshots is deterministic after the relevant scope is fixed
    - fixed input: scoped modules, scoped shared refs or ids, current bindings, current shared truth, execution-local `promotion_owner_module`, and execution-local `bound_modules_only_shared_file_refs`
    - fixed output: candidate invalidation, stable reroute, or metadata-drift reporting according to the existing shared-governance rules
    - not responsible for: deciding shared boundary semantics or inferring caller-only facts that must be provided explicitly
15. `shared reconcile-bound-modules`
    - necessary because reconciling `bound_modules` metadata from current module-side bindings is deterministic once the scope is fixed
    - fixed input: scoped modules, scoped shared refs or ids, and current module-side `shared_contract_refs`
    - fixed output: rewritten touched Shared Contract `bound_modules` metadata
    - not responsible for: deciding whether one shared file should continue to exist

## Freshness Recovery

When the tooling source changed but the current binary was not rebuilt yet, ordinary binary execution fails closed with a stale-binary error.

The normal recovery path is:

```bash
go run ./specflow/tooling/cmd/specflowctl build-release --repo-root .
```

`doctor` is also allowed to run in that stale state so it can report the binary mismatch explicitly.

The minimal stale-binary recovery and inspection surface is:

1. `build-release`
2. `doctor`
3. `help`
4. the internal build-fingerprint query command
