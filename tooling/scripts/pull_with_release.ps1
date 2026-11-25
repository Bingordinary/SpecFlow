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
Re-applies the runtime sparse checkout and then runs
update_tooling_binaries.ps1 to make sure specflowctl binaries
match the pulled tooling source fingerprint. Downloads the
platforms recorded in tooling/platforms.txt; when that file is
missing, downloads all platforms. -All and -CurrentOnly override
the file. The current platform must be included: when
tooling/platforms.txt exists and does not list it, the pull stops
before replacing tooling/bin.

Options:
  -CurrentOnly    Download only the current platform's binary
  -All            Download all platforms
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

function Get-OSArchitecture {
    $runtimeInfo = [System.Runtime.InteropServices.RuntimeInformation]
    $property = $runtimeInfo.GetProperty("OSArchitecture")
    if ($null -ne $property) {
        return [string]$property.GetValue($null, $null)
    }

    $arch = [System.Environment]::GetEnvironmentVariable("PROCESSOR_ARCHITEW6432")
    if ([string]::IsNullOrWhiteSpace($arch)) {
        $arch = [System.Environment]::GetEnvironmentVariable("PROCESSOR_ARCHITECTURE")
    }
    if (-not [string]::IsNullOrWhiteSpace($arch)) {
        return $arch
    }

    throw "Unable to determine CPU architecture."
}

function Get-PlatformSuffix {
    $os = ""
    if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
        $os = "windows"
    }
    elseif ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Linux)) {
        $os = "linux"
    }
    elseif ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::OSX)) {
        $os = "darwin"
    }
    else {
        throw "Unsupported operating system."
    }

    $osArchitecture = Get-OSArchitecture
    $arch = switch ($osArchitecture.ToString().ToUpperInvariant()) {
        "X64" { "amd64" }
        "AMD64" { "amd64" }
        "ARM64" { "arm64" }
        default { throw "Unsupported CPU architecture: $osArchitecture" }
    }

    if ($os -eq "windows") {
        "$os-$arch.exe"
    }
    else {
        "$os-$arch"
    }
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

    # Re-apply the runtime checkout set against the pulled version, so a new
    # runtime directory is materialized and development-only paths stay absent.
    $runtimePathsFile = Join-Path $repoRoot "tooling/runtime-paths.txt"
    if (-not (Test-Path -LiteralPath $runtimePathsFile -PathType Leaf)) {
        throw "tooling/runtime-paths.txt is missing from the pulled checkout."
    }
    $runtimePaths = @()
    foreach ($rawLine in Get-Content -LiteralPath $runtimePathsFile) {
        $line = ($rawLine -split "#", 2)[0].Trim()
        if ($line -eq "") { continue }
        $runtimePaths += $line
    }
    if ($runtimePaths.Count -eq 0) {
        throw "tooling/runtime-paths.txt lists no runtime paths."
    }
    Invoke-CheckedNative "git" @("sparse-checkout", "init", "--cone")
    Invoke-CheckedNative "git" (@("sparse-checkout", "set") + $runtimePaths)

    # Do not clear tooling/bin here. The update script installs new binaries
    # only after the release assets are verified, and a failed update must
    # leave the previous working binaries in place.
    $updateArgs = @("-RequireCurrent")
    if ($CurrentOnly) { $updateArgs += "-CurrentOnly" }
    elseif ($All) { $updateArgs += "-All" }
    & (Join-Path $scriptDir "update_tooling_binaries.ps1") @updateArgs

    # Install hook files from specflow source to project root. The CLI owns the
    # Codex JSON merge so existing project hooks are preserved.
    $projectRoot = (Resolve-Path (Join-Path $repoRoot "..")).Path
    Write-Host "Installing hook files..."
    $suffix = Get-PlatformSuffix
    $specflowctl = Join-Path $repoRoot "tooling/bin/specflowctl-$suffix"
    if (-not (Test-Path -LiteralPath $specflowctl -PathType Leaf)) {
        throw "Expected binary was not installed: $specflowctl. Add the current platform to specflow/tooling/platforms.txt or run this script with -All."
    }
    Invoke-CheckedNative $specflowctl @("init", "--hooks-only", "--repo-root", $projectRoot)
    Write-Host "Hook installation complete."
}
catch {
    Write-Error $_.Exception.Message
    exit 1
}
