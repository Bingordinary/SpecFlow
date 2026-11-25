#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: pull_with_release.sh [--all] [--current-only]

Pull the current SpecFlow branch from origin.
Re-applies the runtime sparse checkout and then runs
update_tooling_binaries.sh to make sure specflowctl binaries
match the pulled tooling source fingerprint. Downloads the
platforms recorded in tooling/platforms.txt; when that file is
missing, downloads all platforms. --all and --current-only
override the file. The current platform must be included: when
tooling/platforms.txt exists and does not list it, the pull stops
before replacing tooling/bin.

Options:
  --all            Download all platforms
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

# The pull always needs the current platform's binary to run
# specflowctl init at the end. The update script validates this before it
# downloads or installs anything, so a platform-list mismatch leaves the
# previous working binaries in place.
UPDATE_ARGS+=("--require-current")

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

# Re-apply the runtime checkout set against the pulled version, so a new
# runtime directory is materialized and development-only paths stay absent.
runtime_paths_file="${REPO_ROOT}/tooling/runtime-paths.txt"
if [[ ! -f "${runtime_paths_file}" ]]; then
  echo "Error: ${runtime_paths_file} is missing from the pulled checkout." >&2
  exit 1
fi
runtime_paths=()
while IFS= read -r line; do
  line="${line%%#*}"
  line="$(printf '%s' "${line}" | tr -d '[:space:]')"
  [[ -z "${line}" ]] && continue
  runtime_paths+=("${line}")
done < "${runtime_paths_file}"
if [[ ${#runtime_paths[@]} -eq 0 ]]; then
  echo "Error: tooling/runtime-paths.txt lists no runtime paths." >&2
  exit 1
fi
git sparse-checkout init --cone
git sparse-checkout set "${runtime_paths[@]}"

# Do not clear tooling/bin here. The update script installs new binaries
# only after the release assets are verified, and a failed update must leave
# the previous working binaries in place.
"${SCRIPT_DIR}/update_tooling_binaries.sh" ${UPDATE_ARGS[@]:+"${UPDATE_ARGS[@]}"}

# Install hook files from specflow source to project root. The CLI owns the
# Codex JSON merge so existing project hooks are preserved.
PROJECT_ROOT="$(cd "${REPO_ROOT}/.." && pwd)"
echo "Installing hook files..."
suffix="$(platform_suffix)"
specflowctl="${REPO_ROOT}/tooling/bin/specflowctl-${suffix}"
if [[ ! -x "${specflowctl}" ]]; then
  echo "Error: expected binary was not installed: ${specflowctl}" >&2
  echo "Add the current platform to specflow/tooling/platforms.txt or run this script with --all." >&2
  exit 1
fi
"${specflowctl}" init --hooks-only --repo-root "${PROJECT_ROOT}"
echo "Hook installation complete."
