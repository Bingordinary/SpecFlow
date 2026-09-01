param(
    [switch]$Help,
    [switch]$CurrentOnly,
    [switch]$All
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Show-Usage {
    [Console]::Error.WriteLine(@"
Usage: pull_with_release.ps1 [-CurrentOnly] [-All]

Pull the current SpecFlow branch from origin.
Then run update_tooling_binaries.ps1 to make sure specflowctl binaries
match the pulled tooling source fingerprint. By default downloads all
platforms so a Syncthing-synced directory stays usable on every machine.

Options:
  -CurrentOnly    Download only the current platform's binary
  -All            Download all platforms (default)
"@)
}

function Invoke-CheckedNative {
    param(
        [string]$FilePath,
        [string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed: $FilePath $($Arguments -join ' ')"
    }
}

function Invoke-CheckedOutput {
    param(
        [string]$FilePath,
        [string[]]$Arguments
    )

    $output = & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed: $FilePath $($Arguments -join ' ')"
    }
    ($output -join "`n").Trim()
}

if ($Help) {
    Show-Usage
    exit 0
}

if ($All -and $CurrentOnly) {
    [Console]::Error.WriteLine("Error: -All and -CurrentOnly are mutually exclusive.")
    exit 1
}

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "../..")).Path

. (Join-Path $scriptDir "common/layout.ps1")

try {
    Set-Location $repoRoot

    $layout = Detect-Layout -RepoRoot $repoRoot
    if ($layout -ne "installed_project") {
        throw "pull_with_release.ps1 is designed for projects that use SpecFlow. Run it from a SpecFlow installation inside your project. (For SpecFlow development, use push_with_release.ps1 instead.)"
    }

    $remoteUrl = Invoke-CheckedOutput "git" @("remote", "get-url", "origin")
    if ([string]::IsNullOrWhiteSpace($remoteUrl)) {
        throw "Git remote 'origin' is missing."
    }

    $branch = Invoke-CheckedOutput "git" @("branch", "--show-current")
    if ([string]::IsNullOrWhiteSpace($branch)) {
        Write-Host "Updating from origin (detached HEAD)..."
        Invoke-CheckedNative "git" @("fetch", "origin")
        Invoke-CheckedNative "git" @("checkout", "origin/HEAD")
    }
    else {
        Write-Host "Pulling $branch from origin..."
        Invoke-CheckedNative "git" @("fetch", "origin", $branch)
        Invoke-CheckedNative "git" @("reset", "--hard", "origin/$branch")
    }

    # Clear tooling/bin before updating binaries, so stale files are
    # removed before fresh ones are downloaded.
    $binDir = Join-Path $repoRoot "tooling/bin"
    if (Test-Path -LiteralPath $binDir) {
        Remove-Item -LiteralPath $binDir -Recurse -Force
        Write-Host "Cleared tooling/bin."
    }

    # Delegate binary update to the standalone script (defaults to all platforms).
    $updateArgs = @()
    if ($CurrentOnly) { $updateArgs += "-CurrentOnly" }
    elseif ($All) { $updateArgs += "-All" }
    & (Join-Path $scriptDir "update_tooling_binaries.ps1") @updateArgs

    # Install hook files from specflow source to project root
    $projectRoot = (Resolve-Path (Join-Path $repoRoot "..")).Path

    function Install-Hook {
        param([string]$Src, [string]$Dst)
        $dir = Split-Path -Parent $Dst
        if (-not (Test-Path -LiteralPath $dir)) {
            New-Item -ItemType Directory -Path $dir -Force | Out-Null
        }
        if (Test-Path -LiteralPath $Src -PathType Leaf) {
            Copy-Item -LiteralPath $Src -Destination $Dst -Force
            Write-Host "  Installed: $((Split-Path -Leaf $Dst))"
        }
        else {
            Write-Host "  Warning: source not found: $Src"
        }
    }

    Write-Host "Installing hook files..."
    Install-Hook -Src (Join-Path $repoRoot "hooks/hooks.json") -Dst (Join-Path $projectRoot "hooks/hooks.json")
    Install-Hook -Src (Join-Path $repoRoot "templates/.claude-plugin/plugin.json") -Dst (Join-Path $projectRoot ".claude-plugin/plugin.json")
    Install-Hook -Src (Join-Path $repoRoot "templates/.opencode/plugins/specflow.js") -Dst (Join-Path $projectRoot ".opencode/plugins/specflow.js")
    Install-Hook -Src (Join-Path $repoRoot "templates/.agents/plugins/specflow/plugin.json") -Dst (Join-Path $projectRoot ".agents/plugins/specflow/plugin.json")
    Install-Hook -Src (Join-Path $repoRoot "templates/.agents/plugins/specflow/hooks.json") -Dst (Join-Path $projectRoot ".agents/plugins/specflow/hooks.json")
    Write-Host "Hook installation complete."
}
catch {
    Write-Error $_.Exception.Message
    exit 1
}
