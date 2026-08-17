#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: pull_with_release.sh

Pull the current SpecFlow branch from origin.
Then run update_tooling_binaries.sh to make sure the current platform's
specflowctl binary matches the pulled tooling source fingerprint.
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

source "${SCRIPT_DIR}/common/layout.sh"

cd "${REPO_ROOT}"

layout="$(detect_layout "${REPO_ROOT}")"
if [[ "${layout}" != "installed_project" ]]; then
  echo "Error: pull_with_release.sh is designed for projects that use SpecFlow." >&2
  echo "Run it from a SpecFlow installation inside your project." >&2
  echo "(For SpecFlow development, use push_with_release.sh instead.)" >&2
  exit 1
fi

remote_url="$(git remote get-url origin 2>/dev/null || true)"
if [[ -z "${remote_url}" ]]; then
  echo "Error: git remote 'origin' is missing." >&2
  exit 1
fi

branch="$(git branch --show-current)"
if [[ -z "${branch}" ]]; then
  # detached HEAD — submodule scenario; update to remote default branch
  echo "Updating from origin (detached HEAD)..."
  git fetch origin
  git checkout origin/HEAD
else
  echo "Pulling ${branch} from origin..."
  git fetch origin "${branch}"
  git reset --hard "origin/${branch}"
fi

# Clear tooling/bin before updating binaries, so stale files are
# removed before fresh ones are downloaded.
BIN_DIR="${REPO_ROOT}/tooling/bin"
if [[ -d "${BIN_DIR}" ]]; then
  rm -rf "${BIN_DIR}"
  echo "Cleared tooling/bin."
fi

# Delegate binary update to the standalone per-platform script.
"${SCRIPT_DIR}/update_tooling_binaries.sh"

# Install hook files from specflow source to project root
PROJECT_ROOT="$(cd "${REPO_ROOT}/.." && pwd)"

install_hook() {
  local src="$1" dst="$2"
  mkdir -p "$(dirname "${dst}")"
  if [ -f "${src}" ]; then
    cp "${src}" "${dst}"
    echo "  Installed: $(basename "${dst}")"
  else
    echo "  Warning: source not found: ${src}"
  fi
}

echo "Installing hook files..."
install_hook "${REPO_ROOT}/hooks/hooks.json" "${PROJECT_ROOT}/hooks/hooks.json"
install_hook "${REPO_ROOT}/templates/.claude-plugin/plugin.json" "${PROJECT_ROOT}/.claude-plugin/plugin.json"
install_hook "${REPO_ROOT}/templates/.opencode/plugins/specflow.js" "${PROJECT_ROOT}/.opencode/plugins/specflow.js"
install_hook "${REPO_ROOT}/templates/.agents/plugins/specflow/plugin.json" "${PROJECT_ROOT}/.agents/plugins/specflow/plugin.json"
install_hook "${REPO_ROOT}/templates/.agents/plugins/specflow/hooks.json" "${PROJECT_ROOT}/.agents/plugins/specflow/hooks.json"
echo "Hook installation complete."
