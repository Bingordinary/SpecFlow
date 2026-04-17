# SpecFlow Tooling

This directory now contains two layers of tooling:

1. host-facing shell or PowerShell scripts
2. a standalone Go CLI for deterministic governance actions

The Go project is the new core for actions that should not depend on duplicated platform-specific scripts.

## Build

From the repository root:

```bash
go build -o ./bin/specflowctl ./specflow/tooling/cmd/specflowctl
```

Or from `specflow/tooling/`:

```bash
go build -o ../../bin/specflowctl ./cmd/specflowctl
```

## Current Command Surface

The current first batch intentionally covers only high-ROI deterministic actions:

1. `entry check`
   - verifies managed-block consistency across registered entry files
2. `entry sync`
   - syncs registered entry-file managed blocks from one chosen source
3. `registry validate`
   - validates `docs/project_standards/_registry.md`
4. `review collect-default-scope`
   - collects the default deterministic file scope for `spec_flow_review`
5. `process cleanup-fallback`
   - applies command-defined fallback cleanup for candidate-chain process files

## Boundary

This CLI does not try to replace semantic judgment performed by the runtime.

It is intentionally not responsible for:

1. `cand_check` closure judgment
2. shared or module boundary judgment
3. verification evidence judgment
4. severity or downgrade decisions

Those remain in the governance documents and the agent runtime.
