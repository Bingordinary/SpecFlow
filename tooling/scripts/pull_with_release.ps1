param(
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Show-Usage {
    [Console]::Error.WriteLine(@"
Usage: pull_with_release.ps1

Pull the current SpecFlow branch from origin.
Then run update_tooling_binaries.ps1 to make sure the current platform's
specflowctl and specflow-reader binaries match the pulled tooling source
fingerprint.
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

function Detect-Layout {
    param(
        [string]$RepoRoot
    )

    $parentDir = Split-Path -Parent $RepoRoot
    $parentGitRoot = & git -C $parentDir rev-parse --show-toplevel 2>$null
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($parentGitRoot)) {
        return "source_repo"
    }

    $gitignore = Join-Path $parentGitRoot ".gitignore"
    if (Test-Path -LiteralPath $gitignore -PathType Leaf) {
        $lines = Get-Content -LiteralPath $gitignore
        if ($lines -contains "specflow/") {
            return "installed_project"
        }
    }

    $gitmodules = Join-Path $parentGitRoot ".gitmodules"
    if (Test-Path -LiteralPath $gitmodules -PathType Leaf) {
        $lines = Get-Content -LiteralPath $gitmodules
        if ($lines -match '^\s*path\s*=\s*specflow\s*$') {
            return "installed_project"
        }
    }

    return "unknown_nested"
}

if ($Help) {
    Show-Usage
    exit 0
}

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "../..")).Path

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

    # Delegate binary update to the standalone per-platform script.
    & (Join-Path $scriptDir "update_tooling_binaries.ps1")
}
catch {
    Write-Error $_.Exception.Message
    exit 1
}
