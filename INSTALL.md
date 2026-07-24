# Install specFlow

Install specFlow into the current project. specFlow is a per-project installation — it lives in `./specflow/` alongside the project code.

## One-Line Install

Run the following command in the project root:

```bash
curl -fsSL https://raw.githubusercontent.com/Bingordinary/SpecFlow/main/tooling/scripts/install.sh | bash
```

PowerShell:

```powershell
irm https://raw.githubusercontent.com/Bingordinary/SpecFlow/main/tooling/scripts/install.ps1 | iex
```

### What the installer does

1. Clones the specFlow repository into `./specflow/`
2. Adds `specflow/` to `.gitignore`
3. Installs the current platform's `specflowctl` binary and `SHA256SUMS`
4. Runs `specflowctl init` — installs framework files and platform hooks

After installation, platform hooks will automatically inject specFlow rules into the agent context at every session start.

## Manual Setup

```bash
git clone https://github.com/Bingordinary/SpecFlow.git specflow
printf "\nspecflow/\n" >> .gitignore
specflow/tooling/scripts/pull_with_release.sh
specflow/tooling/bin/specflowctl-<os>-<arch> init
```

Replace `<os>` and `<arch>` with the current platform (e.g. `linux-amd64`, `darwin-arm64`, `windows-amd64.exe`).

## Verify

After installation, hooks should be active. The agent will automatically load specFlow governance rules and recognize triggers such as `spec_validate`, `spec_verify`, and `spec_promote` on the next session start.

## Update

To update an existing installation, run `spec_flow_update` in the current agent session, then start a new session for the changes to take effect.
