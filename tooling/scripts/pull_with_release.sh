#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: pull_with_release.sh [--all] [--current-only]

Pull the current SpecFlow branch from origin.
Then run update_tooling_binaries.sh to make sure specflowctl binaries
match the pulled tooling source fingerprint. By default downloads all
platforms so a Syncthing-synced directory stays usable on every machine.

Options:
  --all            Download all platforms (default)
  --current-only   Download only the current platform's binary
USAGE
}

UPDATE_ARGS=()
SEEN_ALL=0
SEEN_CURRENT=0
for arg in "$@"; do
  case "${arg}" in
    -h|--help)
      usage
      exit 0
      ;;
    --current-only)
      UPDATE_ARGS+=("--current-only")
      SEEN_CURRENT=1
      ;;
    --all)
      UPDATE_ARGS+=("--all")
      SEEN_ALL=1
      ;;
    *)
      usage
      exit 1
      ;;
  esac
done

if [[ "${SEEN_ALL}" == "1" && "${SEEN_CURRENT}" == "1" ]]; then
  echo "Error: --all and --current-only are mutually exclusive." >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

source "${SCRIPT_DIR}/common/layout.sh"

platform_suffix() {
  local os arch
  case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    MINGW*|MSYS*|CYGWIN*) os="windows" ;;
    *)
      echo "Error: unsupported operating system: $(uname -s)" >&2
      return 1
      ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *)
      echo "Error: unsupported CPU architecture: $(uname -m)" >&2
      return 1
      ;;
  esac

  if [[ "${os}" == "windows" ]]; then
    printf '%s-%s.exe\n' "${os}" "${arch}"
  else
    printf '%s-%s\n' "${os}" "${arch}"
  fi
}

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

# Delegate binary update to the standalone script (defaults to all platforms).
"${SCRIPT_DIR}/update_tooling_binaries.sh" ${UPDATE_ARGS[@]:+"${UPDATE_ARGS[@]}"}

# Install hook files from specflow source to project root. The CLI owns the
# Codex JSON merge so existing project hooks are preserved.
PROJECT_ROOT="$(cd "${REPO_ROOT}/.." && pwd)"
echo "Installing hook files..."
suffix="$(platform_suffix)"
specflowctl="${REPO_ROOT}/tooling/bin/specflowctl-${suffix}"
if [[ ! -x "${specflowctl}" ]]; then
  echo "Error: expected binary was not installed: ${specflowctl}" >&2
  exit 1
fi
"${specflowctl}" init --hooks-only --repo-root "${PROJECT_ROOT}"
echo "Hook installation complete."
