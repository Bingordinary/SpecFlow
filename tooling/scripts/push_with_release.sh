#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: push_with_release.sh

Push the current SpecFlow branch to origin and tag a release for CI.
Before pushing, run update_tooling_binaries.sh to rebuild tooling binaries.
USAGE
}

for arg in "$@"; do
  case "${arg}" in
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 1
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

detect_layout() {
  local repo_root="$1"
  local parent_git_root

  parent_git_root=$(cd "${repo_root}/.." && git rev-parse --show-toplevel 2>/dev/null || true)

  if [[ -z "${parent_git_root}" ]]; then
    echo "source_repo"
    return 0
  fi

  if [[ -f "${parent_git_root}/.gitignore" ]] && grep -qxF "specflow/" "${parent_git_root}/.gitignore"; then
    echo "installed_project"
    return 0
  fi

  echo "unknown_nested"
  return 0
}

cd "${REPO_ROOT}"

branch="$(git branch --show-current)"
if [[ -z "${branch}" ]]; then
  echo "Error: detached HEAD is not supported. Check out a branch before pushing." >&2
  exit 1
fi

if [[ "${branch}" != "main" ]]; then
  echo "Error: push_with_release.sh must be run on the main branch." >&2
  exit 1
fi

layout="$(detect_layout "${REPO_ROOT}")"
if [[ "${layout}" != "source_repo" ]]; then
  echo "Error: push_with_release.sh is designed for the SpecFlow development repository." >&2
  echo "Run it from the SpecFlow source repository root." >&2
  echo "(For project usage, use pull_with_release.sh instead.)" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Error: working tree is not clean. Commit or stash changes before pushing." >&2
  exit 1
fi

remote_url="$(git remote get-url origin 2>/dev/null || true)"
if [[ -z "${remote_url}" ]]; then
  echo "Error: git remote 'origin' is missing." >&2
  exit 1
fi

echo "Pushing ${branch} to origin..."
git push origin "${branch}"

echo "Tagging release..."
RELEASE_TAG="specflow-tooling-$(git rev-parse --short HEAD)"
git tag "${RELEASE_TAG}"
git push origin "${RELEASE_TAG}"
echo "Tagged ${RELEASE_TAG} and pushed."
