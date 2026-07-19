param(
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Show-Usage {
    [Console]::Error.WriteLine(@"
Usage: update_tooling_binaries.ps1

Download and install the specflowctl binary for the
current platform that matches the current tooling source fingerprint.

The script checks whether the local binary already matches the expected
fingerprint. If it is missing or stale, it downloads a fresh binary
from the matching GitHub Release.
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

function Read-BinaryFingerprint {
    param(
        [string]$BinaryPath
    )

    if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
        return ""
    }

    try {
        $output = & $BinaryPath "__print-build-fingerprint" 2>$null
        if ($LASTEXITCODE -ne 0) {
            return ""
        }
        return (($output -join "`n").Trim())
    }
    catch {
        return ""
    }
}

function Test-Checksums {
    param(
        [string]$Directory,
        [string]$CtlName
    )

    $sumsPath = Join-Path $Directory "SHA256SUMS"
    if (-not (Test-Path -LiteralPath $sumsPath -PathType Leaf)) {
        return $false
    }

    $expected = @{}
    foreach ($line in Get-Content -LiteralPath $sumsPath) {
        $parts = $line -split "\s+", 2
        if ($parts.Count -ne 2) {
            continue
        }
        $name = $parts[1].Trim()
        if ($name -eq $CtlName) {
            $expected[$name] = $parts[0].Trim().ToLowerInvariant()
        }
    }

    if (-not $expected.ContainsKey($CtlName)) {
        return $false
    }

    foreach ($name in @($CtlName)) {
        $path = Join-Path $Directory $name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            return $false
        }
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
        if ($actual -ne $expected[$name]) {
            return $false
        }
    }

    return $true
}

function Test-NeedsDownload {
    param(
        [string]$ExpectedFingerprint,
        [string]$CtlPath,
        [string]$BinDir,
        [string]$CtlName
    )

    $ctlFingerprint = Read-BinaryFingerprint $CtlPath
    if ($ctlFingerprint -ne $ExpectedFingerprint) {
        return $true
    }
    if (-not (Test-Checksums $BinDir $CtlName)) {
        return $true
    }

    return $false
}

if ($Help) {
    Show-Usage
    exit 0
}

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "../..")).Path
$binDir = Join-Path $repoRoot "tooling/bin"
$downloadDir = $null

try {
    Set-Location $repoRoot

    $fingerprintScript = Join-Path $repoRoot "tooling/scripts/tooling_fingerprint.ps1"
    $fingerprint = (& $fingerprintScript).Trim()
    $shortFingerprint = $fingerprint.Substring(0, 12)
    $tag = "specflow-tooling-$shortFingerprint"
    $suffix = Get-PlatformSuffix
    $ctlName = "specflowctl-$suffix"
    $ctlPath = Join-Path $binDir $ctlName

    if (-not (Test-NeedsDownload $fingerprint $ctlPath $binDir $ctlName)) {
        Write-Host "Local binary already matches $tag."
        exit 0
    }

    & git ls-remote --exit-code --tags origin "refs/tags/$tag" *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Release tag does not exist on origin: $tag. Run push_with_release.ps1 on main first, then run this script again."
    }

    $downloadDir = Join-Path ([System.IO.Path]::GetTempPath()) ("specflow-download-" + [System.Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $downloadDir | Out-Null
    $base = "https://github.com/Bingordinary/SpecFlow/releases/download/$tag"

    Write-Host "Downloading $tag binary for $suffix..."
    Invoke-WebRequest -Uri "$base/$ctlName" -OutFile (Join-Path $downloadDir $ctlName)
    Invoke-WebRequest -Uri "$base/SHA256SUMS" -OutFile (Join-Path $downloadDir "SHA256SUMS")

    if (-not (Test-Checksums $downloadDir $ctlName)) {
        throw "Downloaded file failed checksum verification."
    }

    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    Move-Item -LiteralPath (Join-Path $downloadDir $ctlName) -Destination $ctlPath -Force
    Move-Item -LiteralPath (Join-Path $downloadDir "SHA256SUMS") -Destination (Join-Path $binDir "SHA256SUMS") -Force

    # chmod is only meaningful on Unix-like systems — skip on Windows
    if (-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
        Invoke-CheckedNative "chmod" @("+x", $ctlPath)
    }

    Write-Host "Installed $ctlName and SHA256SUMS from $tag."
}
finally {
    if ($null -ne $downloadDir) {
        Remove-Item -LiteralPath $downloadDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
