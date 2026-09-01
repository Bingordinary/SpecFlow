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

if ($Help) {
    Show-Usage
    exit 0
}

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "../..")).Path

. (Join-Path $scriptDir "common/layout.ps1")

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

    if ($branch -ne "main") {
        throw "push_with_release.ps1 must be run on the main branch. Current branch is $branch."
    }

    & git fetch origin main *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Fetch from origin failed (network or repository unavailable). Retry spec_flow_push after the network recovers."
    }

    $behind = Invoke-CheckedOutput "git" @("rev-list", "--count", "HEAD..origin/main")
    if ([int]$behind -gt 0) {
        throw "Local is behind origin/main by $behind commit(s). Run spec_flow_push (Stage 1) to fetch and resolve before pushing."
    }

    Write-Host "Computing tooling source fingerprint..."
    $toolingRoot = Join-Path $repoRoot "tooling"
    Push-Location $toolingRoot
    try {
        $fingerprint = Invoke-CheckedOutput "go" @("run", "./cmd/specflowctl", "tooling-fingerprint", "--repo-root", $repoRoot)
    }
    finally {
        Pop-Location
    }
    $shortFingerprint = $fingerprint.Substring(0, 12)

    $fingerprintFile = Join-Path $toolingRoot "fingerprint.txt"
    $recorded = if (Test-Path -LiteralPath $fingerprintFile -PathType Leaf) { (Get-Content -LiteralPath $fingerprintFile -Raw).Trim() } else { "" }
    if ($recorded -ne $fingerprint) {
        Set-Content -LiteralPath $fingerprintFile -Value $fingerprint -NoNewline
        Invoke-CheckedNative "git" @("add", "tooling/fingerprint.txt")
        Invoke-CheckedNative "git" @("commit", "-m", "chore(tooling): record tooling fingerprint $shortFingerprint")
    }

    Write-Host "Pushing $branch to origin..."
    Invoke-CheckedNative "git" @("push", "-u", "origin", $branch)

    $tag = "specflow-tooling-$shortFingerprint"

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
