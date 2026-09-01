param(
    [switch]$Help,
    [switch]$All,
    [switch]$CurrentOnly
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Show-Usage {
    [Console]::Error.WriteLine(@"
Usage: update_tooling_binaries.ps1 [-All] [-CurrentOnly]

Download and install specflowctl binaries that match the current
tooling source fingerprint from the matching GitHub Release.

By default downloads binaries for all platforms (linux-amd64, linux-arm64,
darwin-amd64, darwin-arm64, windows-amd64.exe, windows-arm64.exe) so a
Syncthing-synced project directory stays usable on every platform.

Options:
  -All            Download all platforms (default)
  -CurrentOnly    Download only the current platform's binary

The script checks whether the local binaries already match the expected
fingerprint. If any required binary is missing or stale, it downloads
fresh binaries from the matching GitHub Release.
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

function Get-AllPlatformSuffixes {
    return @(
        "linux-amd64",
        "linux-arm64",
        "darwin-amd64",
        "darwin-arm64",
        "windows-amd64.exe",
        "windows-arm64.exe"
    )
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

function Test-NeedsDownloadAll {
    param(
        [string]$ExpectedFingerprint,
        [string]$BinDir,
        [string[]]$Suffixes
    )

    $currentSuffix = ""
    try { $currentSuffix = Get-PlatformSuffix } catch { $currentSuffix = "" }

    foreach ($suffix in $Suffixes) {
        $ctlName = "specflowctl-$suffix"
        $ctlPath = Join-Path $BinDir $ctlName
        if (-not (Test-Path -LiteralPath $ctlPath -PathType Leaf)) {
            return $true
        }
        if ($suffix -eq $currentSuffix) {
            if (Test-NeedsDownload $ExpectedFingerprint $ctlPath $BinDir $ctlName) {
                return $true
            }
        } else {
            # Cross-platform binary: cannot execute, check checksum only.
            if (-not (Test-Checksums $BinDir $ctlName)) {
                return $true
            }
        }
    }
    # All binaries present — verify SHA256SUMS contains every entry
    $sumsPath = Join-Path $BinDir "SHA256SUMS"
    if (-not (Test-Path -LiteralPath $sumsPath -PathType Leaf)) {
        return $true
    }
    $content = Get-Content -LiteralPath $sumsPath -Raw -ErrorAction SilentlyContinue
    if ($null -eq $content) { $content = "" }
    foreach ($suffix in $Suffixes) {
        if ($content -notmatch [regex]::Escape("specflowctl-$suffix")) {
            return $true
        }
    }
    return $false
}

function Test-ChecksumsMany {
    param(
        [string]$Directory,
        [string[]]$CtlNames
    )

    foreach ($name in $CtlNames) {
        if (-not (Test-Checksums $Directory $name)) {
            return $false
        }
    }
    return $true
}

if ($Help) {
    Show-Usage
    exit 0
}

# Default is all platforms; -CurrentOnly opts into single-platform mode
$downloadAll = -not $CurrentOnly
if ($All -and $CurrentOnly) {
    [Console]::Error.WriteLine("Error: -All and -CurrentOnly are mutually exclusive.")
    exit 1
}
if ($All) { $downloadAll = $true }

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = (Resolve-Path (Join-Path $scriptDir "../..")).Path
$binDir = Join-Path $repoRoot "tooling/bin"
$downloadDir = $null

try {
    Set-Location $repoRoot

    $fingerprintFile = Join-Path $repoRoot "tooling/fingerprint.txt"
    if (-not (Test-Path -LiteralPath $fingerprintFile -PathType Leaf)) {
        throw "tooling/fingerprint.txt is missing in this checkout. This version has no recorded fingerprint metadata — pull to a version that ships it first."
    }
    $fingerprint = (Get-Content -LiteralPath $fingerprintFile -Raw).Trim()
    $shortFingerprint = $fingerprint.Substring(0, 12)
    $tag = "specflow-tooling-$shortFingerprint"

    if ($downloadAll) {
        $allSuffixes = Get-AllPlatformSuffixes
        $allNames = @()
        foreach ($s in $allSuffixes) { $allNames += "specflowctl-$s" }

        if (-not (Test-NeedsDownloadAll $fingerprint $binDir $allSuffixes)) {
            Write-Host "Local binaries already match $tag (all platforms)."
            exit 0
        }

        & git ls-remote --exit-code --tags origin "refs/tags/$tag" *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "Release tag does not exist on origin: $tag. Run push_with_release.ps1 on main first, then run this script again."
        }

        $downloadDir = Join-Path ([System.IO.Path]::GetTempPath()) ("specflow-download-" + [System.Guid]::NewGuid().ToString("N"))
        New-Item -ItemType Directory -Path $downloadDir | Out-Null
        $base = "https://github.com/Bingordinary/SpecFlow/releases/download/$tag"

        Write-Host "Downloading $tag binaries for all platforms..."
        Invoke-WebRequest -Uri "$base/SHA256SUMS" -OutFile (Join-Path $downloadDir "SHA256SUMS")
        foreach ($suffix in $allSuffixes) {
            $ctlName = "specflowctl-$suffix"
            Write-Host "  Downloading $ctlName..."
            Invoke-WebRequest -Uri "$base/$ctlName" -OutFile (Join-Path $downloadDir $ctlName)
        }

        if (-not (Test-ChecksumsMany $downloadDir $allNames)) {
            throw "Downloaded files failed checksum verification."
        }

        New-Item -ItemType Directory -Path $binDir -Force | Out-Null
        foreach ($suffix in $allSuffixes) {
            $ctlName = "specflowctl-$suffix"
            Move-Item -LiteralPath (Join-Path $downloadDir $ctlName) -Destination (Join-Path $binDir $ctlName) -Force
            if ($ctlName -notlike "*.exe" -and -not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
                Invoke-CheckedNative "chmod" @("+x", (Join-Path $binDir $ctlName))
            }
        }
        Move-Item -LiteralPath (Join-Path $downloadDir "SHA256SUMS") -Destination (Join-Path $binDir "SHA256SUMS") -Force

        Write-Host "Installed $($allNames.Count) binaries and SHA256SUMS from $tag."
        exit 0
    }

    # -CurrentOnly
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
