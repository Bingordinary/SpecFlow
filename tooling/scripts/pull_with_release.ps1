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

    # Install hook files from specflow source to project root. The CLI owns the
    # Codex JSON merge so existing project hooks are preserved.
    $projectRoot = (Resolve-Path (Join-Path $repoRoot "..")).Path
    Write-Host "Installing hook files..."
    $suffix = Get-PlatformSuffix
    $specflowctl = Join-Path $repoRoot "tooling/bin/specflowctl-$suffix"
    if (-not (Test-Path -LiteralPath $specflowctl -PathType Leaf)) {
        throw "Expected binary was not installed: $specflowctl"
    }
    Invoke-CheckedNative $specflowctl @("init", "--hooks-only", "--repo-root", $projectRoot)
    Write-Host "Hook installation complete."
}
catch {
    Write-Error $_.Exception.Message
    exit 1
}
