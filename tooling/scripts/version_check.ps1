param(
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Show-Usage {
    [Console]::Error.WriteLine(@"
Usage: version_check.ps1

Check the installed SpecFlow version against the remote.
Prints the local commit, the remote latest commit, and how many commits
the local version is behind.

Output is machine-readable "key: value" lines.
"@)
}

function Invoke-Output {
    param(
        [string]$FilePath,
        [string[]]$Arguments
    )

    $output = & $FilePath @Arguments 2>$null
    if ($LASTEXITCODE -ne 0) {
        return $null
    }
    ($output -join "`n").Trim()
}

function Invoke-Required {
    param(
        [string]$FilePath,
        [string[]]$Arguments
    )

    $output = Invoke-Output $FilePath $Arguments
    if ($null -eq $output) {
        throw "Command failed: $FilePath $Arguments"
    }
    $output
}

function Write-Unavailable {
    param(
        [string]$Reason
    )

    Write-Output "remote_commit: unavailable"
    Write-Output "remote_date: unavailable"
    Write-Output "remote_subject: unavailable"
    Write-Output "behind_count: unavailable"
    Write-Output "ahead_count: unavailable"
    Write-Output "remote_reason: $Reason"
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

    $layout = Detect-Layout -RepoRoot $repoRoot
    if ($layout -eq "unknown_nested") {
        throw "Cannot locate the SpecFlow installation. Run version_check.ps1 from a SpecFlow installation inside your project, or from the SpecFlow source repository."
    }
    Write-Output "layout: $layout"

    $branch = Invoke-Required "git" @("branch", "--show-current")
    if ([string]::IsNullOrWhiteSpace($branch)) {
        Write-Output "branch: (detached HEAD)"
        $remoteRef = $null
    }
    else {
        Write-Output "branch: $branch"
        $remoteRef = "refs/heads/$branch"
    }

    $localCommit = Invoke-Required "git" @("rev-parse", "--short=12", "HEAD")
    $localDate = Invoke-Required "git" @("log", "-1", "--format=%cd", "--date=short", "HEAD")
    $localSubject = Invoke-Required "git" @("log", "-1", "--format=%s", "HEAD")
    Write-Output "local_commit: $localCommit"
    Write-Output "local_date: $localDate"
    Write-Output "local_subject: $localSubject"

    $remoteUrl = Invoke-Output "git" @("remote", "get-url", "origin")
    if ([string]::IsNullOrWhiteSpace($remoteUrl)) {
        Write-Unavailable "git remote 'origin' is missing"
        exit 0
    }

    if ($null -eq $remoteRef) {
        $symrefOutput = Invoke-Output "git" @("ls-remote", "--symref", "origin", "HEAD")
        if ($null -eq $symrefOutput) {
            Write-Unavailable "cannot reach origin (network or repository unavailable)"
            exit 0
        }
        $symrefLine = ($symrefOutput -split "`n") | Where-Object { $_ -match '^ref:' } | Select-Object -First 1
        if ($null -eq $symrefLine) {
            Write-Unavailable "remote HEAD does not point to a branch"
            exit 0
        }
        $remoteRef = ($symrefLine -split "\s+")[1]
    }

    & git ls-remote --exit-code origin $remoteRef *> $null
    $lsExit = $LASTEXITCODE
    if ($lsExit -eq 2) {
        Write-Unavailable "remote ref $remoteRef not found (local branch may not be pushed)"
        exit 0
    }
    if ($lsExit -ne 0) {
        Write-Unavailable "cannot reach origin (network or repository unavailable)"
        exit 0
    }

    & git fetch origin $remoteRef *> $null
    if ($LASTEXITCODE -ne 0) {
        Write-Unavailable "fetch from origin failed (network or repository unavailable)"
        exit 0
    }

    $trackingRef = "origin/" + $remoteRef.Replace("refs/heads/", "")
    $remoteCommit = Invoke-Output "git" @("rev-parse", "--short=12", $trackingRef)
    if ([string]::IsNullOrWhiteSpace($remoteCommit)) {
        Write-Unavailable "remote ref $trackingRef not found after fetch"
        exit 0
    }

    $remoteDate = Invoke-Required "git" @("log", "-1", "--format=%cd", "--date=short", $trackingRef)
    $remoteSubject = Invoke-Required "git" @("log", "-1", "--format=%s", $trackingRef)
    $behindCount = Invoke-Required "git" @("rev-list", "--count", "HEAD..$trackingRef")
    $aheadCount = Invoke-Required "git" @("rev-list", "--count", "$trackingRef..HEAD")
    Write-Output "remote_commit: $remoteCommit"
    Write-Output "remote_date: $remoteDate"
    Write-Output "remote_subject: $remoteSubject"
    Write-Output "behind_count: $behindCount"
    Write-Output "ahead_count: $aheadCount"
}
catch {
    Write-Error $_.Exception.Message
    exit 1
}
