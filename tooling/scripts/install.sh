#!/usr/bin/env bash
set -euo pipefail

REPO_URL="https://github.com/Bingordinary/SpecFlow.git"
TARGET_DIR="specflow"
IGNORE_ENTRY="specflow/"

usage() {
  cat >&2 <<'USAGE'
Usage: install.sh

Run from the root of the repository that should adopt specFlow.
The installer clones SpecFlow into ./specflow, adds specflow/ to .gitignore,
installs the current platform's local binaries, and runs specflowctl init.
USAGE
}

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    echo "Error: required command is missing: ${name}" >&2
    exit 1
  fi
}

platform_suffix() {
  local os arch
  case "$(uname -s)" in
    Linux)
      os="linux"
      ;;
    Darwin)
      os="darwin"
      ;;
    MINGW*|MSYS*|CYGWIN*)
      os="windows"
      ;;
    *)
      echo "Error: unsupported operating system: $(uname -s)" >&2
      exit 1
      ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64)
      arch="amd64"
      ;;
    aarch64|arm64)
      arch="arm64"
      ;;
    *)
      echo "Error: unsupported CPU architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac

  if [[ "${os}" == "windows" ]]; then
    printf '%s-%s.exe\n' "${os}" "${arch}"
  else
    printf '%s-%s\n' "${os}" "${arch}"
  fi
}

add_gitignore_entry() {
  if [[ -f .gitignore ]] && grep -qxF "${IGNORE_ENTRY}" .gitignore; then
    return 0
  fi

  if [[ -s .gitignore ]]; then
    printf '\n%s\n' "${IGNORE_ENTRY}" >> .gitignore
  else
    printf '%s\n' "${IGNORE_ENTRY}" >> .gitignore
  fi
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

require_command git
require_command curl

project_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "${project_root}" ]]; then
  echo "Error: run this installer inside the repository that should adopt specFlow." >&2
  exit 1
fi

current_dir="$(pwd -P)"
project_root="$(cd "${project_root}" && pwd -P)"
if [[ "${current_dir}" != "${project_root}" ]]; then
  echo "Error: run this installer from the repository root: ${project_root}" >&2
  exit 1
fi

if [[ -e "${TARGET_DIR}" ]]; then
  echo "Error: ./${TARGET_DIR} already exists. This installer is only for first-time setup." >&2
  echo "Use ${TARGET_DIR}/tooling/scripts/pull_with_release.sh for an existing specFlow checkout." >&2
  exit 1
fi

echo "Cloning SpecFlow into ./${TARGET_DIR}..."
git clone "${REPO_URL}" "${TARGET_DIR}"

echo "Adding ${IGNORE_ENTRY} to .gitignore..."
add_gitignore_entry

echo "Installing local specFlow binaries..."
bash "${TARGET_DIR}/tooling/scripts/pull_with_release.sh"

suffix="$(platform_suffix)"
specflowctl="${TARGET_DIR}/tooling/bin/specflowctl-${suffix}"
if [[ ! -x "${specflowctl}" ]]; then
  echo "Error: expected binary was not installed: ${specflowctl}" >&2
  exit 1
fi

echo "Running specflowctl init..."
"${specflowctl}" init

echo "specFlow setup complete."
