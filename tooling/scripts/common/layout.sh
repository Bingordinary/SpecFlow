#!/usr/bin/env bash
# Shared layout detection library for tooling scripts.
# Source this file, then call detect_layout <repo_root>.
# Prints one of: source_repo | installed_project | unknown_nested

detect_layout() {
  local repo_root="$1"
  local parent_git_root

  # A valid SpecFlow installation is itself a git repository (clone or
  # submodule). Without this check, git commands run inside it would
  # resolve to an enclosing repository and report data from the wrong repo.
  if [[ ! -e "${repo_root}/.git" ]]; then
    echo "unknown_nested"
    return 0
  fi

  parent_git_root=$(cd "${repo_root}/.." && git rev-parse --show-toplevel 2>/dev/null || true)

  if [[ -z "${parent_git_root}" ]]; then
    echo "source_repo"
    return 0
  fi

  if [[ -f "${parent_git_root}/.gitignore" ]] && grep -qxF "specflow/" "${parent_git_root}/.gitignore"; then
    echo "installed_project"
    return 0
  fi

  if [[ -f "${parent_git_root}/.gitmodules" ]] && grep -qE '^\s*path\s*=\s*specflow\s*$' "${parent_git_root}/.gitmodules"; then
    echo "installed_project"
    return 0
  fi

  echo "unknown_nested"
  return 0
}
