param(
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoUrl = "https://github.com/Bingordinary/SpecFlow.git"
$TargetDir = "specflow"
$IgnoreEntries = @("specflow/", "docs/specs/meta/validation/", ".tmp/", "/meta/", ".claude-plugin/", ".opencode/", ".agents/", "hooks/hooks.json")

function Show-Usage {
    [Console]::Error.WriteLine(@"
Usage: install.ps1

Run from the root of the repository that should adopt specFlow.
The installer clones SpecFlow into ./specflow, adds ignore entries
(specflow/, validation caches, build caches, tool configs) to .gitignore,
installs the current platform's local binaries, and runs specflowctl init.
"@)
}

function Assert-Command {
    param(
        [string]$Name
    )

    if ($null -eq (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command is missing: $Name"
    }
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

function Add-GitignoreEntry {
    param(
        [string]$Path,
        [string]$Entry
    )

    if (Test-Path -LiteralPath $Path -PathType Leaf) {
        $lines = Get-Content -LiteralPath $Path
        if ($lines -contains $Entry) {
            return
        }

        if ((Get-Item -LiteralPath $Path).Length -gt 0) {
            Add-Content -LiteralPath $Path -Value ""
        }
        Add-Content -LiteralPath $Path -Value $Entry
        return
    }

    Set-Content -LiteralPath $Path -Value $Entry
}

if ($Help) {
    Show-Usage
    exit 0
}

Assert-Command "git"

$projectRoot = Invoke-CheckedOutput "git" @("rev-parse", "--show-toplevel")
if ([string]::IsNullOrWhiteSpace($projectRoot)) {
    throw "Run this installer inside the repository that should adopt specFlow."
}

$currentDir = (Resolve-Path ".").Path
$projectRoot = (Resolve-Path $projectRoot).Path
if (-not [string]::Equals($currentDir, $projectRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Run this installer from the repository root: $projectRoot"
}

if (Test-Path -LiteralPath $TargetDir) {
    throw "./$TargetDir already exists. This installer is only for first-time setup. Use $TargetDir/tooling/scripts/pull_with_release.ps1 for an existing specFlow checkout."
}

Write-Host "Cloning SpecFlow into ./$TargetDir..."
Invoke-CheckedNative "git" @("clone", $RepoUrl, $TargetDir)

Write-Host "Adding ignore entries to .gitignore..."
foreach ($entry in $IgnoreEntries) {
    Add-GitignoreEntry ".gitignore" $entry
}

Write-Host "Installing local specFlow binaries..."
$pullScript = Join-Path $TargetDir "tooling/scripts/pull_with_release.ps1"
& $pullScript
if ($LASTEXITCODE -ne 0) {
    throw "Command failed: $pullScript"
}

$suffix = Get-PlatformSuffix
$specflowctl = Join-Path $TargetDir "tooling/bin/specflowctl-$suffix"
if (-not (Test-Path -LiteralPath $specflowctl -PathType Leaf)) {
    throw "Expected binary was not installed: $specflowctl"
}

Write-Host "Running specflowctl init..."
Invoke-CheckedNative $specflowctl @("init")

Write-Host "specFlow setup complete."
