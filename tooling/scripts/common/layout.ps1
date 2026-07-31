# Shared layout detection library for tooling scripts.
# Dot-source this file, then call Detect-Layout -RepoRoot <repo_root>.
# Returns one of: source_repo | installed_project | unknown_nested

function Detect-Layout {
    param(
        [string]$RepoRoot
    )

    # A valid SpecFlow installation is itself a git repository (clone or
    # submodule). Without this check, git commands run inside it would
    # resolve to an enclosing repository and report data from the wrong repo.
    if (-not (Test-Path -LiteralPath (Join-Path $RepoRoot ".git"))) {
        return "unknown_nested"
    }

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
