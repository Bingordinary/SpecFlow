param(
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Show-Usage {
    [Console]::Error.WriteLine(@"
Usage: push_with_release.ps1 [-Help]

Push the current SpecFlow branch to origin and optionally tag a release
for CI when on the main branch.

Options:
    -Help    Display this help message.
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

    $branch = Invoke-CheckedOutput "git" @("branch", "--show-current")
    if ([string]::IsNullOrWhiteSpace($branch)) {
        throw "Detached HEAD is not supported. Check out a branch before pushing."
    }

    $status = Invoke-CheckedOutput "git" @("status", "--porcelain")
    if (-not [string]::IsNullOrWhiteSpace($status)) {
        throw "Working tree is not clean. Commit or stash changes before pushing."
    }

    $layout = Detect-Layout -RepoRoot $repoRoot
    if ($layout -ne "source_repo") {
        throw "push_with_release.ps1 is designed for the SpecFlow development repository. Run it from the SpecFlow source repository root. (For project usage, use pull_with_release.ps1 instead.)"
    }

    $remoteUrl = Invoke-CheckedOutput "git" @("remote", "get-url", "origin")
    if ([string]::IsNullOrWhiteSpace($remoteUrl)) {
        throw "Git remote 'origin' is missing."
    }

    Write-Host "Pushing $branch to origin..."
    Invoke-CheckedNative "git" @("push", "-u", "origin", $branch)

    if ($branch -ne "main") {
        Write-Host "Current branch is $branch, not main."
        Write-Host "Release tag push is skipped."
        exit 0
    }

    $fingerprintScript = Join-Path $repoRoot "tooling/scripts/tooling_fingerprint.ps1"
    $fingerprint = (& $fingerprintScript -Short).Trim()
    $tag = "specflow-tooling-$fingerprint"

    & git ls-remote --exit-code --tags origin "refs/tags/$tag" *> $null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Release tag already exists on origin: $tag"
        Write-Host "No release tag push needed."
        exit 0
    }

    & git rev-parse -q --verify "refs/tags/$tag" *> $null
    if ($LASTEXITCODE -eq 0) {
        $tagCommit = Invoke-CheckedOutput "git" @("rev-list", "-n", "1", $tag)
        $headCommit = Invoke-CheckedOutput "git" @("rev-parse", "HEAD")
        if ($tagCommit -ne $headCommit) {
            throw "Local tag $tag exists but does not point to HEAD. Delete or inspect the local tag manually before pushing a release."
        }
    }
    else {
        Invoke-CheckedNative "git" @("tag", $tag)
    }

    Write-Host "Pushing release tag $tag..."
    Invoke-CheckedNative "git" @("push", "origin", $tag)
    Write-Host "Release workflow triggered by $tag."
}
finally {
}
