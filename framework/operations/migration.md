> **DEPRECATED:** `spec_flow_migrate` is superseded by `spec_flow_update`. See `framework/operations/update.md`.
>
# spec_flow_migrate

When the user says `spec_flow_migrate`, follow this procedure. It first runs the deterministic tooling, then checks what only an LLM can judge.

## Procedure

### Step 1: Run specflowctl migrate

==ATOM_BEGIN:specflowctl_location==
specflowctl is not on PATH. Its binary is at `specflow/tooling/bin/specflowctl-<os>-<arch>`. Replace `<os>` and `<arch>` with your platform (e.g. `linux-amd64`, `darwin-arm64`, `windows-amd64.exe`). Use the full path when running specflowctl commands.
==ATOM_END:specflowctl_location==

Execute from the project root:
```
specflow/tooling/bin/specflowctl-<os>-<arch> migrate
```

If the command succeeds (exit code 0), hook files are up to date and the binary version is current. Proceed to Step 2.

If the command fails (non-zero exit or command not found), report the error output. Tell the user to run the full command above manually from the project root, then restart the agent session so the updated hooks take effect. Do not proceed to Step 2.

### Step 2: Check Project Document Format

After hooks are up to date and the binary is current, check the following files against the format in `framework/spec_writing_guide.md`:

| Check | What to verify |
|-------|---------------|
| `docs/specs/repository_mapping.md` | Table header matches expected format. `kind` is `unit` or `rule`. `registration_state` is `planned` or `landed`. |
| Candidate spec files | For each `docs/specs/units/candidate/c_unit_*.md`: `id`, `layer`, `version`, `unit_refs`, `rule_refs`, `acceptance_item_set` present. Compare field format against `spec_writing_guide.md`. |
| Stable spec files | For each `docs/specs/units/stable/s_unit_*.md`: required frontmatter fields present. Compare against `spec_writing_guide.md`. |
| Appendix files | Path follows: `docs/specs/units/<layer>/appendix/<prefix>_<unit>_<name>.md`. |

Do not judge business truth correctness. Only check format and structural compliance.

### Step 3: Report

Report each check as PASSED or FAILED. If any check fails, list the specific issue and the recommended fix.
