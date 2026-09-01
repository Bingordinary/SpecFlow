# spec_flow_push

When the user says `spec_flow_push`, follow this procedure. It ensures the local `main` is not behind `origin/main` before computing the tooling fingerprint, committing `tooling/fingerprint.txt`, and pushing. The flow is two stages: Stage 1 fetches and resolves conflicts with user confirmation; Stage 2 delegates to the push script. Any direct invocation of the push script without Stage 1 is guarded by a fail-fast check inside the script.

This command is for the `source_repo` layout (this repository). It does not apply to `installed_project` layouts — report that the command is unavailable there and stop.

## Prerequisites

- Layout must be `source_repo` (a `framework/` directory exists at the repository root and `tooling/scripts/common/layout.sh` reports `source_repo`). Otherwise report that `spec_flow_push` is a source-repo-only command and stop.
- Current branch must be `main`. If `git branch --show-current` is empty (detached HEAD) or not `main`, report it and stop.
- Working tree must be clean (`git status --porcelain` empty). If dirty, report it and stop — do not stash automatically.
- Git remote `origin` must exist (`git remote get-url origin` succeeds). If missing, report it and stop.
- Go toolchain must be available (`go version` succeeds) because Stage 2 computes the fingerprint via `go run ./cmd/specflowctl tooling-fingerprint`.

## Procedure

### Stage 1: Fetch and conflict check (before fingerprint)

Stage 1 must complete before any fingerprint computation. Do not compute the fingerprint or commit `tooling/fingerprint.txt` on a stale base.

#### Step 1.1: Fetch origin/main

Run:

```bash
git fetch origin main
```

- If the fetch fails (non-zero exit, network or repository unavailable), report it in the same style as `tooling/scripts/version_check.sh:58` — e.g. `cannot reach origin (network or repository unavailable)` or `fetch from origin failed (network or repository unavailable)` — and stop. Do not proceed to Stage 2. Tell the user to retry after the network recovers; do not attempt fingerprint or push.

#### Step 1.2: Compute behind/ahead and display remote commits

After a successful fetch, compute:

```bash
behind=$(git rev-list --count HEAD..origin/main)
ahead=$(git rev-list --count origin/main..HEAD)
```

Display the remote commits not yet in local:

```bash
git log HEAD..origin/main --oneline
```

Report `behind` and `ahead` counts to the user.

#### Step 1.3: Branch on behind/ahead

- **Case A — `behind == 0`:** No conflict. The local branch is up to date (it may be ahead by `ahead` commits, which is normal for source-repo development). Proceed directly to Stage 2.

- **Case B — `behind > 0 && ahead == 0`:** The local branch is behind and can be fast-forwarded. Report that the local is behind by `behind` commit(s) and show the log from Step 1.2. Then **stop and wait for user confirmation** (`y/N`). Do not run `git pull` automatically.
  - If the user answers `N` (or does not confirm), stop. Do not execute Stage 2. Leave the working tree and branch unchanged and tell the user to resolve manually when ready.
  - If the user answers `y`, run:

    ```bash
    git pull --rebase origin main
    ```

- **Case C — `behind > 0 && ahead > 0`:** The branches have diverged. Report that the local and `origin/main` have diverged (`behind` behind, `ahead` ahead), show the log from Step 1.2 and `git status --porcelain`, and warn that `tooling/fingerprint.txt` is likely to conflict during rebase because both sides may have recorded different fingerprints. Then **stop and wait for user confirmation** (`y/N`).
  - If the user answers `N` (or does not confirm), stop. Do not execute Stage 2.
  - If the user answers `y`, run:

    ```bash
    git pull --rebase origin main
    ```

  If the rebase reports conflicts, report the conflict (especially `tooling/fingerprint.txt`) and stop. Do not attempt automatic conflict resolution. Tell the user to resolve the conflicts manually, then re-run `spec_flow_push` from Stage 1.

#### Step 1.4: Re-verify after rebase

If Stage 1 performed a `git pull --rebase origin main`, re-verify before entering Stage 2:

```bash
behind=$(git rev-list --count HEAD..origin/main)
git status --porcelain
```

- If `behind != 0`, report that the local is still behind and stop. Tell the user to resolve manually.
- If `git status --porcelain` is not empty (rebase left conflicts or dirty state), report it and stop. Tell the user to resolve the working-tree state manually and re-run `spec_flow_push`.

Only when `behind == 0` and the working tree is clean may Stage 2 be entered. If the user declined confirmation in Step 1.3, do not enter Stage 2.

### Stage 2: Push with release (only after Stage 1 succeeds)

After Stage 1 confirms no conflict (or the rebase succeeded and re-verification passed), delegate to the existing push script. Do not reimplement its logic — execute it as-is.

From the repository root, run:

```bash
./tooling/scripts/push_with_release.sh
```

On Windows:

```powershell
tooling\scripts\push_with_release.ps1
```

This script:

- Re-validates that the branch is `main`, the layout is `source_repo`, the working tree is clean, and `origin` exists.
- Performs a defensive fail-fast: it fetches `origin/main` again and aborts with a non-zero exit if the local is behind `origin/main`, directing the user to run the `spec_flow_push` flow (Stage 1). The script itself does not prompt for confirmation or run `git pull --rebase` — interaction belongs to this document's Stage 1 only.
- Computes the tooling source fingerprint via `go run ./cmd/specflowctl tooling-fingerprint --repo-root <repo>` and, if `tooling/fingerprint.txt` differs, commits it as `chore(tooling): record tooling fingerprint <short>`.
- Pushes `main` to `origin` (`git push origin main`), then ensures the release tag `specflow-tooling-<short>` exists and pushes it if not already present on the remote.

Do not read the script's shell implementation beyond the fail-fast contract above. Execute it as-is.

- If the script succeeds (exit code 0), report that the push and release tag (if created) succeeded.
- If the script fails (non-zero exit, including the fail-fast behind check), report the error output verbatim and stop. If the failure is the behind check, direct the user back to Stage 1. Do not retry automatically.

## Failure handling

| Condition | Handling |
|-----------|----------|
| Fetch fails in Stage 1 or Stage 2 | Report network/repo unavailable and stop. Do not compute fingerprint or push. |
| Detached HEAD or branch is not `main` | Report it and stop. |
| Working tree is dirty (`git status --porcelain` non-empty) | Report it and stop. Do not stash or commit automatically. |
| `origin` missing | Report it and stop. |
| Go toolchain missing | Report it and stop — fingerprint cannot be computed. |
| Rebase conflicts (especially `tooling/fingerprint.txt`) | Report the conflict and stop. Ask the user to resolve manually, then re-run `spec_flow_push` from Stage 1. |
| Script fail-fast reports `behind > 0` | This means Stage 1 was bypassed or `origin/main` advanced between Stage 1 and Stage 2. Re-run Stage 1. |

## Verification

After a successful push:

- `git rev-list --count HEAD..origin/main` must be `0`.
- `git status --porcelain` must be empty.
- `tooling/fingerprint.txt` must contain the fingerprint printed by `go run ./cmd/specflowctl tooling-fingerprint`.

Do not modify governance files or tooling scripts outside this flow as part of `spec_flow_push`.
