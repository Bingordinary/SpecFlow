# spec_flow_update

When the user says `spec_flow_update`, follow this procedure. It pulls the latest SpecFlow framework, updates binaries and hooks, then checks project document format.

## Procedure

### Step 1: Run pull_with_release.sh

Determine the project root (the directory containing `specflow/`). Then from the project root, run:

```
specflow/tooling/scripts/pull_with_release.sh
```

On Windows:
```
specflow\tooling\scripts\pull_with_release.ps1
```

This script:
- Pulls the latest SpecFlow source from git
- Downloads matching tooling binaries
- Installs hook files to the project root

If the command succeeds (exit code 0), hooks and binaries are up to date. Proceed to Step 2.

If the command fails (non-zero exit or script not found), report the error output. Tell the user to run the script manually from the project root, then restart the agent session so the updated hooks take effect. Do not proceed to Step 2.

### Step 2: Check Project Document Format

After hooks are up to date and binaries are current, check the following files against the format in `framework/spec_writing_guide.md`:

| Check | What to verify |
|-------|---------------|
| `docs/specs/repository_mapping.md` | Table header matches expected format. `kind` is `unit` or `rule`. `registration_state` is `planned` or `landed`. |
| Candidate spec files | For each `docs/specs/units/candidate/c_unit_*.md`: `id`, `layer`, `version`, `unit_refs`, `rule_refs`, `acceptance_item_set` present. Compare field format against `spec_writing_guide.md`. |
| Stable spec files | For each `docs/specs/units/stable/s_unit_*.md`: required frontmatter fields present. Compare against `spec_writing_guide.md`. |
| Appendix files | Path follows: `docs/specs/units/<layer>/appendix/<prefix>_<unit>_<name>.md`. |

Do not judge business truth correctness. Only check format and structural compliance.

### Step 3: Report

Report each check as PASSED or FAILED. If any check fails, list the specific issue and the recommended fix.
