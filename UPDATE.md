# Update specFlow

Update an existing specFlow installation in the current project.

This document is the out-of-band update entry. Use it whenever you update, and in particular when the platform adapter that injects the specFlow session bootstrap no longer loads — for example after a runtime breaking change such as the OpenCode V2 plugin API change — so the agent can no longer see specFlow triggers. It assumes nothing about the injected bootstrap.

specFlow is a per-project installation — it lives in `./specflow/` alongside the project code.

## Procedure

### Step 1: Record the current framework commit

Run this from the project root (the directory containing `specflow/`). Record the current specFlow commit so the migration can diff what changed. Persist it under the project's ignored local-state tree so a later session can still read it:

```bash
BASELINE_FILE="meta/.update-baseline"

if [ -f "$BASELINE_FILE" ]; then
  OLD_HASH=$(tr -d '[:space:]' < "$BASELINE_FILE") || exit 1
else
  OLD_HASH=$(git -C specflow rev-parse HEAD) || exit 1
  mkdir -p meta || exit 1
  printf '%s\n' "$OLD_HASH" > "$BASELINE_FILE" || exit 1
fi

if [ -z "$OLD_HASH" ]; then
  echo "Error: update baseline is empty." >&2
  exit 1
fi

if ! git -C specflow cat-file -e "$OLD_HASH^{commit}" 2>/dev/null; then
  echo "Error: update baseline '$OLD_HASH' is not a valid commit in specflow/." >&2
  exit 1
fi
```

This is the same `meta/.update-baseline` baseline used by `spec_flow_update`. Never overwrite an existing file: it belongs to an update that has not completed yet. If the file is empty or the recorded commit is invalid, stop and report the problem; do not fall back to the current HEAD.

### Step 2: Pull the latest framework and reinstall hooks

From the project root, run:

```bash
specflow/tooling/scripts/pull_with_release.sh
```

PowerShell:

```powershell
specflow\tooling\scripts\pull_with_release.ps1
```

The script fetches the latest specFlow source, downloads matching `specflowctl` binaries, and installs the platform hook files by running `specflowctl init --hooks-only`. It reinstalls `.opencode/plugins/specflow.js` and the other adapter files, which fixes an adapter that the previous version left broken.

If the script is missing or fails, refresh the checkout first, then rerun it:

```bash
git -C specflow fetch origin
git -C specflow reset --hard "origin/$(git -C specflow branch --show-current)"
specflow/tooling/scripts/pull_with_release.sh
```

The baseline in `meta/.update-baseline` is already saved, so refreshing the checkout cannot lose the pre-update hash.

### Step 3: Continue the framework migration

Read the freshly pulled `specflow/framework/operations/update.md` and execute it from **Step 2** through **Step 6**. That document's Step 2 reads the baseline from `meta/.update-baseline`, which Step 1 above saved. Do not repeat that document's Step 0 or Step 1 — the baseline is already recorded and the pull already ran in Step 2 above. If you are resuming in a new session, read the baseline file instead of recording the current HEAD.

### Step 4: Start a new agent session

Start a new agent session so the runtime reloads the adapter and injects the updated bootstrap. No host process restart is required; command packages and the bootstrap are read fresh from disk.

## Verify

- In OpenCode, `opencode plugin list` shows `specflow` with no failed status.
- The agent recognizes triggers such as `spec_flow_version`.

## Why this document exists

The agent learns specFlow triggers only from the injected session bootstrap. A platform breaking change disables the adapter and removes that channel at the same moment the update is needed, so an in-session trigger such as `spec_flow_update` cannot start the recovery. This document restores the update path without depending on the adapter or the bootstrap: it speaks directly to the agent, which can run the update script from disk.
