# Install specFlow

Install specFlow into the current project. specFlow is a per-project installation — it lives in `./specflow/` alongside the project code.

## Install

The installation is agent-driven. Tell your agent to install specFlow (e.g. "install specFlow into this project"); the agent reads this document and runs the installation script from the project root:

```bash
curl -fsSL https://raw.githubusercontent.com/Bingordinary/SpecFlow/main/tooling/scripts/install.sh | bash
```

PowerShell:

```powershell
irm https://raw.githubusercontent.com/Bingordinary/SpecFlow/main/tooling/scripts/install.ps1 | iex
```

The installer:

1. Clones the specFlow repository into `./specflow/`
2. Adds `specflow/` to `.gitignore`
3. Installs the current platform's `specflowctl` binary and `SHA256SUMS`
4. Runs `specflowctl init` — installs framework files and platform hooks

After installation, platform hooks will automatically inject specFlow rules into the agent context at every session start.

## Existing Project Adoption

specFlow supports onboarding an existing (already-implemented) project. After installation, perform an adoption check:

1. Scan the project source code.
2. Determine whether this is a greenfield project or an existing project:
   - **Greenfield** (no existing source code) → proceed with normal usage; nothing further is needed.
   - **Existing project** → ask the user: "This project already contains code. Do you want to build specs from the existing implementation?"
3. If the user confirms, follow the adoption flow in `specflow/framework/operations/adopt.md`: scan → structure review and alignment decision → unit cut list (with suspicious-point markings and directory mapping) → user confirmation → batch candidate generation with evidence appendices → guided per-batch `validate` / `verify` / `review` / `promote`.

Adoption progress is tracked in-session; the flow can be resumed in later sessions by telling the agent to continue adoption (e.g. "继续建档").

## Verify

After installation, hooks should be active. The agent will automatically load specFlow governance rules and recognize triggers such as `validate`, `verify`, and `promote` on the next session start.

## Update

To update an existing installation, run `spec_flow_update` in the current agent session, then start a new session for the changes to take effect.
