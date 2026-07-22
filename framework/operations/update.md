# spec_flow_update

When the user says `spec_flow_update`, follow this procedure. It pulls the latest SpecFlow framework, detects structural changes in framework rules since the last update, and migrates project spec files to match.

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

Do not read the script's shell implementation. Execute it as-is.

If the command succeeds (exit code 0), proceed to Step 2.

If the command fails (non-zero exit or script not found), report the error output. Tell the user to run the script manually from the project root, then restart the agent session so the updated hooks take effect. Do not proceed.

### Step 2: Detect framework structural changes (inside specflow/ repository)

Run this inside the `specflow/` directory (the framework source repository), NOT the project root:

```bash
git -C "$SPECFLOW_DIR" diff $OLD_HASH..HEAD -- framework/
```

Read the diff output carefully. Extract every structural rule change that affects spec file format. Examples of what to look for:

- **Path/filename convention changes**: e.g. rule files no longer use `s_`/`c_` prefix, unit path patterns changed, appendix path rules changed
- **Frontmatter field changes**: new required fields, removed fields, renamed fields, changed value format (e.g. `rule_refs` from `@version` suffixed to bare names)
- **Reference format changes**: how `unit_refs` or `rule_refs` are written, what prefix/suffix is expected
- **Structural rule changes**: new required sections, removed sections, changed validation rules

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

### Step 4: Verify and report

After migration, run the format compliance check against `framework/spec_writing_guide.md`:

| Check | What to verify |
|-------|---------------|
| Candidate spec files | For each `docs/specs/units/candidate/c_unit_*.md`: `id`, `layer`, `version`, `unit_refs`, `rule_refs`, `acceptance_item_set` present. Compare field format against `spec_writing_guide.md`. |
| Stable spec files | For each `docs/specs/units/stable/s_unit_*.md`: required frontmatter fields present. Compare against `spec_writing_guide.md`. |
| Appendix files | Path follows: `docs/specs/units/<layer>/appendix/<prefix>_<unit>_<name>.md`. |
| Rule files | For each rule file: `rule_id`, `rule_scope`, `layer`, `rule_version` present. Path matches convention. |

Report each check as PASSED or FAILED with details. If any check fails and the cause is a missed migration, fix it. If the cause is unclear or requires business judgment, report it to the user.
