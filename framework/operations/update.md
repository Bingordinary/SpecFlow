# spec_flow_update

When the user says `spec_flow_update`, follow this procedure. It pulls the latest SpecFlow framework, detects structural changes in framework rules since the last update, and migrates project spec files to match.

Requests that do not explicitly invoke `spec_flow_update` carry no migration write authority — route them through normal editing instead.

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

Read the diff output carefully. Extract every structural rule change that affects spec file format. Examples of what to look for:

- **Path/filename convention changes**: e.g. unit and rule files no longer use `s_`/`c_` prefix, appendix path rules changed
- **Frontmatter field changes**: new required fields, removed fields, renamed fields, changed value format (e.g. `rule_refs` from `@version` suffixed to bare names)
- **Reference format changes**: how `rule_refs` are written, what prefix/suffix is expected
- **Structural rule changes**: new required sections, removed sections, changed validation rules (for example the three-section promise model: `## Design Narrative` → `## Promises` → `## Acceptance Items`, with dependency edges on the consumer promise's `depends_on`)
- **Retired lifecycle mechanisms**: former `unbound_retention*` metadata or `status: retired` files that the current format no longer uses

Do NOT guess or infer changes from memory. Read the actual `git diff` output.

Also read the current `framework/spec_writing_guide.md` to understand the latest rules.

### Step 3: Plan and execute migration

Based on the changes detected in Step 2, plan migration operations for the project's spec files under `docs/specs/`. Common operation types:

| Operation | Example |
|-----------|---------|
| **Rename files** | `mv docs/specs/rules/stable/s_g_rule_foo.md docs/specs/rules/stable/g_rule_foo.md` |
| **Update frontmatter** | Change a field value, add a missing required field, remove a deprecated field |
| **Update references** | Bulk-replace old ref format in `rule_refs` across all spec files |
| **Restructure directories** | Move files between directories when path rules change |

**Promise-model migration:** when Step 2 reports the three-section unit shape (`## Design Narrative` → `## Promises` → `## Acceptance Items`), migrate **every unit file in each layer** (candidate and stable):

1. Derive each promise from the unit's existing formal carrier — the acceptance item set and protocol appendices. A promise keeps the stable contract (API surface, exchanged formats, error codes, state machine transitions, timing); the acceptance item keeps its code/test scope and references the promise it proves.
2. Restructure the main spec into the three sections in order. `## Promises` and `## Acceptance Items` contain structured content only; move narrative subsections, including governance and alignment content, under `## Design Narrative`.
3. Move `unit_refs` and `affects.dependencies` to the consumer promise's `depends_on`; remove both old fields. Add `promise:` to every contract-verifying item and `non_promise: true` plus `non_promise_reason` to every non-contract item.
4. Do not invent contract truth. When the old carrier does not establish a contract value (a missing error code, timing guarantee, or field shape), present the affected unit to the user and ask; the migration is agent work, and the deployed gates reject a unit without `## Promises`. Do not add a converter or a compatibility path.
5. Caches written before the promise-pin format are incompatible. After migration every unit needs one full `validate` and one full `verify`; `delta`/`repair` refuses an old cache as a baseline, and `promote` rejects it. `fresh` reports such a cache as `INVALID`.

**Legacy exit-mechanism migration:** when Step 2 reports the former retention metadata or retired status:

1. Move every useful `unbound_retention_reason` and owner rationale into the rule body prose, then remove `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner`.
2. Inspect every unit or appendix file carrying `status: retired` under `framework/removal_workflow.md`. The agent decides whether the responsibility has ended and identifies the exact deletion targets; delete only within clear existing authorization, and report unresolved intent instead of recreating the former promote-based exit logic.
3. Surviving files use the normal `status: active|exempt` rules. A rule validate cache written by the former retention-based check set is not translatable; revalidate the rule against the current six-check checklist when it is next used.

For each operation:

1. **Identify affected files** — use `find` / `glob` to locate every file that needs the change
2. **Apply the change** — use `mv`, `sed`, `grep` / agent file editing
3. **Verify completeness** — check there are no remaining matches of the old pattern (e.g. `grep -r 'old_pattern' docs/specs/` should return nothing after migration)

If multiple operations are needed, order them so later operations don't break earlier ones (e.g. rename files before updating internal references).

If a change requires business judgment (e.g. "what value should this new frontmatter field have?"), present the affected files to the user and ask for input. Do not invent business truth.

If no structural changes were detected in Step 2, report that no migration is needed and skip to Step 4.

### Step 4: Verify and report

After migration, run the format compliance check against `framework/spec_writing_guide.md`:

| Check | What to verify |
|-------|---------------|
| Candidate spec files | For each `docs/specs/units/candidate/unit_*.md`: `id`, `rule_refs`, `acceptance_item_set` present, and the promise model present (`## Design Narrative` → `## Promises` → `## Acceptance Items`). Compare field format against `spec_writing_guide.md`. |
| Stable spec files | For each `docs/specs/units/stable/unit_*.md`: required frontmatter fields present and the promise model present (`## Design Narrative` → `## Promises` → `## Acceptance Items`). Compare against `spec_writing_guide.md`. |
| Promise-model units | For **every unit file present in a layer** (candidate or stable): the three sections are in order, every promise has a stable ID and contract, every contract item declares `promise` or `non_promise`, and no `###` subheading appears inside `## Promises` or `## Acceptance Items`. Run `specflowctl validate promise-model --unit <name> --layer candidate` for a candidate file and `--layer stable` for a stable file. |
| Legacy dependency fields | No `unit_refs` or `affects.dependencies` remains in any unit spec; every dependency edge is on a consumer promise's `depends_on`. |
| Legacy exit mechanisms | No `unbound_retention*` field remains in any rule; no unit or appendix uses `status: retired`; surviving files use `status: active` or `exempt`. |
| Appendix files | Path follows: `docs/specs/units/<layer>/appendix/unit_<unit>_<name>.md`. |
| Rule files | For each rule file: `rule_id`, `rule_scope` present. Path matches convention. |

Report each check as PASSED or FAILED with details. If any check fails and the cause is a missed migration, fix it. If the cause is unclear or requires business judgment, report it to the user.

### Step 5: Impact classification

After format verification, classify downstream impact on existing units:

1. Determine whether Step 2 detected structural changes involving **path ownership, object registration, or support-surface boundaries** — i.e. boundary changes that cannot be resolved from unit or rule frontmatter. (To detect: check whether the framework diff includes structural path changes in `docs/specs/`, or whether any governance flow explicitly reports an unresolved boundary change.)
2. If such boundary changes exist, run impact sync per `framework/governance/impact_sync.md` to perform consumer discovery and fallback reason classification for affected units.
3. Report the impact sync output contract: `affected_candidate_units`, `affected_stable_units`, and `freshness_review_required`.
4. If `freshness_review_required` is `true`, execute the caller-owned Freshness Review procedure from `framework/governance/impact_sync.md` before any fallback cleanup: run `specflowctl fresh` on each affected unit, classify each as cleanup-allowed or cleanup-blocked by gate status, and report the per-unit classification. Do not execute fallback cleanup on a unit whose gates are STALE, INVALID, MISSING, or BLOCKED — present the blocking gate to the user instead.
5. If no such boundary changes exist, report that no affected units require classification, then continue to Step 6. Do not infer consumers from implementation directories alone.

Do not invent business truth during impact classification — if a fallback decision requires business judgment, present it to the user and ask for input.

### Step 6: Clear the update baseline

After Step 4 and Step 5 both complete, run this from the project root to delete the baseline:

```bash
rm -f meta/.update-baseline
```

Delete it only when the update has finished. If migration or impact classification stopped for user input, keep the file so the next session resumes from the same pre-update hash.
