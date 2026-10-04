# spec_flow_update

When the user says `spec_flow_update`, follow this procedure. It pulls the latest SpecFlow framework, detects structural changes in framework rules since the last update, and migrates project spec files to match.

Requests that do not explicitly invoke `spec_flow_update` carry no migration write authority — route them through normal editing instead.

> If the platform adapter that injects the bootstrap is not loading — for example after a runtime breaking change — the agent cannot see the `spec_flow_update` trigger. The out-of-band recovery entry is the repository-root `UPDATE.md`: it runs the Step 1 pull directly, then hands back to this procedure from Step 2.

## Procedure

### Step 0: Record pre-update state

Before pulling, record the current specflow commit hash so you can diff what changed.

```bash
SPECFLOW_DIR="$(pwd)/specflow"
OLD_HASH=$(git -C "$SPECFLOW_DIR" rev-parse HEAD)
```

Save this hash; you will use it in Step 2.

If `specflow/` does not exist at the project root, the framework is not installed. Report this to the user and stop.

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

If the command fails (non-zero exit or script not found), report the error output. Tell the user to run the script manually from the project root, then start a new agent session so the updated hooks are injected (no process restart is required). Do not proceed.

### Step 2: Detect framework structural changes (inside specflow/ repository)

Run this inside the `specflow/` directory (the framework source repository), NOT the project root:

```bash
git -C "$SPECFLOW_DIR" diff $OLD_HASH..HEAD -- framework/ templates/docs/specs/
```

Read the diff output carefully. Extract every structural rule change that affects spec file format. Examples of what to look for:

- **Path/filename convention changes**: e.g. unit and rule files no longer use `s_`/`c_` prefix, appendix path rules changed
- **Frontmatter field changes**: new required fields, removed fields, renamed fields, changed value format (e.g. `rule_refs` from `@version` suffixed to bare names)
- **Reference format changes**: how `unit_refs` or `rule_refs` are written, what prefix/suffix is expected
- **Structural rule changes**: new required sections, removed sections, changed validation rules
- **Template bootstrap rule changes**: the layout-selected global rule bootstrap file `templates/docs/specs/rules/stable/g_rule_repository_baseline.md` changed — its clause numbering and prohibition clauses are referenced by number from framework instructions (e.g. `framework/unit_validate_checklist.md` Check 8 executes "§5.1 items 4-5"), so a shape change must be migrated to the project copy at `docs/specs/rules/stable/g_rule_repository_baseline.md`

Do NOT guess or infer changes from memory. Read the actual `git diff` output.

Also read the current `framework/spec_writing_guide.md` to understand the latest rules.

### Step 3: Plan and execute migration

Based on the changes detected in Step 2, plan migration operations for the project's spec files under `docs/specs/`. Common operation types:

| Operation | Example |
|-----------|---------|
| **Rename files** | `mv docs/specs/rules/stable/s_g_rule_foo.md docs/specs/rules/stable/g_rule_foo.md` |
| **Update frontmatter** | Change a field value, add a missing required field, remove a deprecated field |
| **Update references** | Bulk-replace old ref format in `rule_refs` / `unit_refs` across all spec files |
| **Restructure directories** | Move files between directories when path rules change |

For each operation:

1. **Identify affected files** — use `find` / `glob` to locate every file that needs the change
2. **Apply the change** — use `mv`, `sed`, `grep` / agent file editing
3. **Verify completeness** — check there are no remaining matches of the old pattern (e.g. `grep -r 'old_pattern' docs/specs/` should return nothing after migration)

If multiple operations are needed, order them so later operations don't break earlier ones (e.g. rename files before updating internal references).

If a change requires business judgment (e.g. "what value should this new frontmatter field have?"), present the affected files to the user and ask for input. Do not invent business truth.

If no structural changes were detected in Step 2, report that no migration is needed and skip to Step 4.

### Removal-model migration

- Move useful `unbound_retention_reason` and owner rationale into rule body prose, then remove `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner`. Rules without consumers may remain valid.
- Inspect legacy `status: retired` unit and appendix files. Under `framework/removal_workflow.md`, the agent decides whether the responsibility ended and identifies exact deletion targets. Delete only within clear existing authorization; discuss unresolved intent. Do not recreate the old promote-based exit logic. Surviving files use normal active/exempt status rules.
- Rule validate now has six agent checks: metadata 1–5 and body quality 6. Existing caches using the former check set require normal revalidation against the current contract, not translation of the old retention judgment.

### Step 4: Verify and report

After migration, run the format compliance check against `framework/spec_writing_guide.md`:

| Check | What to verify |
|-------|---------------|
| Candidate spec files | For each `docs/specs/units/candidate/unit_*.md`: `id`, `unit_refs`, `rule_refs`, `acceptance_item_set` present. Compare field format against `spec_writing_guide.md`. |
| Stable spec files | For each `docs/specs/units/stable/unit_*.md`: required frontmatter fields present. Compare against `spec_writing_guide.md`. |
| Appendix files | Path follows: `docs/specs/units/<layer>/appendix/unit_<unit>_<name>.md`. |
| Rule files | For each rule file: `rule_id`, `rule_scope` present. Path matches convention. |
| Template bootstrap rule | The project copy `docs/specs/rules/stable/g_rule_repository_baseline.md` agrees with the current template `specflow/templates/docs/specs/rules/stable/g_rule_repository_baseline.md` on clause numbering; the project's filled content (Tech Stack, Reusable Mechanisms) stays project-owned. |

Report each check as PASSED or FAILED with details. If any check fails and the cause is a missed migration, fix it. If the cause is unclear or requires business judgment, report it to the user.

### Step 5: Impact classification

After format verification, classify downstream impact on existing units:

1. Determine whether Step 2 detected structural changes involving **path ownership, object registration, or support-surface boundaries** — i.e. boundary changes that cannot be resolved from unit or rule frontmatter. (To detect: check whether the framework diff includes structural path changes in `docs/specs/`, or whether any governance flow explicitly reports an unresolved boundary change.)
2. If such boundary changes exist, run impact sync per `framework/governance/impact_sync.md` to perform consumer discovery and fallback reason classification for affected units.
3. Report the impact sync output contract: `affected_candidate_units`, `affected_stable_units`, and `freshness_review_required`.
4. If `freshness_review_required` is `true`, execute the caller-owned Freshness Review procedure from `framework/governance/impact_sync.md` before any fallback cleanup: run `specflowctl fresh` on each affected unit, classify each as cleanup-allowed or cleanup-blocked by gate status, and report the per-unit classification. Do not execute fallback cleanup on a unit whose gates are STALE, MISSING, or BLOCKED — present the blocking gate to the user instead.
5. If no such boundary changes exist, report that no affected units require classification and finish. Do not infer consumers from implementation directories alone.

Do not invent business truth during impact classification — if a fallback decision requires business judgment, present it to the user and ask for input.
