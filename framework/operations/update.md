# spec_flow_update

When the user says `spec_flow_update`, follow this procedure. It pulls the latest SpecFlow framework, detects structural changes in framework rules since the last update, and applies the current expected format to the project's spec files under the update authorization.

Requests that do not explicitly invoke `spec_flow_update` carry no update write authority — route them through normal editing instead.

> If the platform adapter that injects the bootstrap is not loading — for example after a runtime breaking change — the agent cannot see the `spec_flow_update` trigger. The out-of-band recovery entry is the repository-root `UPDATE.md`: it records the same `meta/.update-baseline` and runs the Step 1 pull directly, then hands back to this procedure from Step 2.

## Procedure

### Step 0: Record pre-update state

Run this from the project root. Before pulling, establish the pre-update commit hash so you can diff what changed. Save it under the project's ignored local-state tree (`meta/`) or reuse the saved value: Step 1 can send the user into a new agent session, and that session must still see the pre-pull hash.

If `specflow/` does not exist at the project root, the framework is not installed. Report this to the user and stop.

```bash
SPECFLOW_DIR="$(pwd)/specflow"
BASELINE_FILE="$(pwd)/meta/.update-baseline"

if [ ! -d "$SPECFLOW_DIR" ]; then
  echo "Error: specflow/ does not exist at the project root." >&2
  exit 1
fi

if [ -f "$BASELINE_FILE" ]; then
  OLD_HASH=$(tr -d '[:space:]' < "$BASELINE_FILE") || exit 1
else
  OLD_HASH=$(git -C "$SPECFLOW_DIR" rev-parse HEAD) || exit 1
  mkdir -p "$(dirname "$BASELINE_FILE")" || exit 1
  printf '%s\n' "$OLD_HASH" > "$BASELINE_FILE" || exit 1
fi

if [ -z "$OLD_HASH" ]; then
  echo "Error: update baseline is empty." >&2
  exit 1
fi

if ! git -C "$SPECFLOW_DIR" cat-file -e "$OLD_HASH^{commit}" 2>/dev/null; then
  echo "Error: update baseline '$OLD_HASH' is not a valid commit in specflow/." >&2
  exit 1
fi
```

Use this hash in Step 2. Never overwrite an existing `meta/.update-baseline`: it belongs to an update that has not completed yet. If the file is empty or the recorded commit is invalid, stop and report the problem; do not fall back to the current HEAD.

### Step 1: Run pull_with_release.sh

From the project root, run:

```
specflow/tooling/scripts/pull_with_release.sh
```

On Windows:

```
specflow\tooling\scripts\pull_with_release.ps1
```

This script:
- Pulls the latest SpecFlow source from git (operates inside `specflow/`)
- Downloads matching tooling binaries
- Installs hook files to the project root

By default the script downloads binaries for all platforms. If `specflow/tooling/platforms.txt` exists, only the platforms listed there are downloaded. The file is a user-owned local preference (inside the git-ignored `specflow/` directory). Do not create or edit this file during the update.

Do not read the script's shell implementation. Execute it as-is.

If the command succeeds (exit code 0), proceed to Step 2.

If the command fails (non-zero exit or script not found), report the error output. The pre-update hash is already saved in `meta/.update-baseline`, so the recovery does not depend on this session. Tell the user to run the script manually from the project root, then start a new agent session so the updated hooks are injected (no process restart is required). Do not proceed. In the new session, Step 0 reads the saved hash and Step 2 compares it with the pulled HEAD.

### Step 2: Detect framework structural changes (run from the project root)

Read the pre-update hash from the persisted baseline; do not re-record the current HEAD:

```bash
SPECFLOW_DIR="$(pwd)/specflow"
OLD_HASH=$(tr -d '[:space:]' < "$(pwd)/meta/.update-baseline") || exit 1
if [ -z "$OLD_HASH" ]; then
  echo "Error: meta/.update-baseline is empty." >&2
  exit 1
fi
```

An empty or missing baseline is an error, not a no-change result: stop and report instead of falling back to the current HEAD.

Run this from the project root; the `-C` flag targets the `specflow/` repository (the framework source repository), NOT the project repository:

```bash
git -C "$SPECFLOW_DIR" diff "$OLD_HASH"..HEAD -- framework/ templates/
```

Read the diff output carefully. Extract every structural rule change that affects spec file format. The diff shows what changed; the current documents state what the expected state is. Do not infer a transformation from memory.

Inspect these categories:

- **Path and filename conventions**
- **Frontmatter fields and required values**
- **Reference formats**
- **Required sections and structural rules**
- **Lifecycle and ownership rules**

Read the current format owners for every affected category: `framework/spec_writing_guide.md`, and for unit promise content `framework/core/promise_model.md`. These documents state the current format only; this flow supplies no conversion table or per-format migration recipe.

### Step 3: Apply the current format

For each affected project spec file under `docs/specs/`, compare the file with the current format owners and apply the current format. This is the consumer agent's migration work: the framework supplies the target state, not a step-by-step transformation.

1. **Identify affected files** — use `find` / `glob` to locate every file that needs the change.
2. **Apply the current format** — use ordinary file editing under the update authorization.
3. **Ask for business judgment** — if a change requires a business decision (for example "what value should this new frontmatter field have?"), present the affected files to the user and ask; do not invent business truth.

If no structural changes were detected in Step 2, report that no format update is needed and skip to Step 5.

### Step 4: Verify and report

Verify each affected file against the current format owners. Run the validator the owner names for every changed category; for the promise model, `specflowctl validate promise-model --unit <name> --layer candidate|stable` for every affected unit file in a layer. Compare each spec file's path, frontmatter, and section structure with `framework/spec_writing_guide.md` (unit main specs, rule files, and appendices). A field or mechanism the current format does not define is a failure.

Report each check as PASSED or FAILED with details. If a check fails, the file is not in the current format; fix it under the update authorization or report the unresolved item to the user.

### Step 5: Impact classification

After the format update, classify downstream impact on existing units:

1. Determine whether Step 2 detected structural changes involving **path ownership, object registration, or support-surface boundaries** — i.e. boundary changes that cannot be resolved from unit or rule frontmatter. (To detect: check whether the framework diff changes a path, filename, ownership, registration, or support-surface rule that governs `docs/specs/`, or whether any governance flow explicitly reports an unresolved boundary change. The diff covers `framework/` and `templates/`; it never contains the project’s `docs/specs/` tree itself.)
2. If such boundary changes exist, run impact sync per `framework/governance/impact_sync.md` to perform consumer discovery and fallback reason classification for affected units.
3. Report the impact sync output contract: `affected_candidate_units`, `affected_stable_units`, and `freshness_review_required`.
4. If `freshness_review_required` is `true`, execute the caller-owned Freshness Review procedure from `framework/governance/impact_sync.md` before any fallback cleanup: run `specflowctl fresh` on each affected unit, classify each as cleanup-allowed or cleanup-blocked by gate status, and report the per-unit classification. Do not execute fallback cleanup on a unit whose gates are STALE, INVALID, MISSING, or BLOCKED — present the blocking gate to the user instead.
5. If no such boundary changes exist, report that no affected units require classification, then continue to Step 6. Do not infer consumers from implementation directories alone.

Do not invent business truth during impact classification — if a fallback decision requires business judgment, present it to the user and ask for input.

### Step 6: Clear the update baseline

After the format update and impact classification complete, run this from the project root to delete the baseline:

```bash
rm -f meta/.update-baseline
```

Delete it only when the update has finished. If the format update or impact classification stopped for user input, keep the file so the next session resumes from the same pre-update hash.
