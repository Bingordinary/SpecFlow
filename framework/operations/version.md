# spec_flow_version

When the user says `spec_flow_version`, follow this procedure. It checks the installed SpecFlow version against the remote latest version and reports whether the project is up to date.

## Procedure

### Step 1: Run version_check script

From the project root, run:

```
specflow/tooling/scripts/version_check.sh
```

On Windows:

```
specflow\tooling\scripts\version_check.ps1
```

From the SpecFlow source repository root, run:

```
tooling/scripts/version_check.sh
```

This script does not modify the working tree or local branches. It only fetches remote refs (updating remote-tracking refs) to compare versions. It prints the local commit, the remote latest commit, and how many commits the local version is behind.

Do not read the script's shell implementation. Execute it as-is.

### Step 2: Interpret the output

The script prints `key: value` lines. Interpret them as follows:

| Field | Meaning |
|-------|---------|
| `layout` | `source_repo` (development repository) or `installed_project` (project using SpecFlow) |
| `branch` | Current branch, or `(detached HEAD)` |
| `local_commit` / `local_date` / `local_subject` | Installed local version |
| `remote_commit` / `remote_date` / `remote_subject` | Latest version on origin |
| `behind_count` | Commits the local version is behind the remote |
| `ahead_count` | Commits the local version is ahead of the remote |
| `remote_reason` | Why the remote could not be checked, when remote fields are `unavailable` |

### Step 3: Report the result

Report to the user:

- Local version (commit + date + subject)
- Remote latest version (commit + date + subject)
- Status

Determine the status:

| Condition | Status |
|-----------|--------|
| `behind_count` > 0 and `ahead_count` > 0 | **DIVERGED** — local and remote have diverged: the local version has unpushed commits, and the remote has commits not present locally |
| `behind_count` > 0 and `ahead_count` = 0 | **OUTDATED** — local version is behind the remote by N commits |
| `behind_count` = 0 and `ahead_count` = 0 | **UP TO DATE** |
| `behind_count` = 0 and `ahead_count` > 0 | **AHEAD** — local version has commits not yet on the remote (normal for source_repo development) |
| remote fields are `unavailable` | **UNKNOWN** — report the `remote_reason` and that only the local version was checked |

### Step 4: Recommend next action

- **OUTDATED**: recommend running `spec_flow_update` to bring the project to the latest version.
- **UP TO DATE**: report that no update is needed.
- **AHEAD**: report that no update is needed; local changes are simply not pushed yet.
- **DIVERGED**: report the divergence (how many commits each side has). Do not recommend `spec_flow_update`: in an `installed_project` it resets the specflow repository to the remote and would discard the local commits; in `source_repo` it is not applicable. Ask the user how to handle the local commits before any update.
- **UNKNOWN**: report that the remote could not be checked and the user may retry later. Do not recommend `spec_flow_update` on an unverifiable comparison.

### Step 5: Script failure handling

If the script fails (non-zero exit or script not found), report the error output to the user and stop. Do not guess version state from other sources.
