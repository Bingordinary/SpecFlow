#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: update_tooling_binaries.sh

Download and install the specflowctl binary for the
current platform that matches the current tooling source fingerprint.

The script checks whether the local binary already matches the expected
fingerprint. If it is missing or stale, it downloads a fresh binary
from the matching GitHub Release.
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
BIN_DIR="${REPO_ROOT}/tooling/bin"
download_dir=""
trap 'rm -rf "${download_dir:-}"' EXIT

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
      return 1
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
      return 1
      ;;
  esac

  if [[ "${os}" == "windows" ]]; then
    printf '%s-%s.exe\n' "${os}" "${arch}"
  else
    printf '%s-%s\n' "${os}" "${arch}"
  fi
}

read_binary_fingerprint() {
  local binary_path="$1"
  if [[ ! -x "${binary_path}" ]]; then
    return 1
  fi
  "${binary_path}" __print-build-fingerprint 2>/dev/null || return 1
}

verify_checksums() {
  local dir="$1"
  local ctl_name="$2"
  local current_sums status
  current_sums="$(mktemp)"

  awk -v ctl="${ctl_name}" \
    '$2 == ctl { print }' \
    "${dir}/SHA256SUMS" >"${current_sums}"
  if [[ "$(wc -l <"${current_sums}" | tr -d ' ')" != "1" ]]; then
    echo "Error: SHA256SUMS does not contain the current platform binary." >&2
    rm -f "${current_sums}"
    return 1
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    if (
      cd "${dir}"
      sha256sum -c "${current_sums}"
    ); then
      status=0
    else
      status=$?
    fi
  elif command -v shasum >/dev/null 2>&1; then
    if (
      cd "${dir}"
      shasum -a 256 -c "${current_sums}"
    ); then
      status=0
    else
      status=$?
    fi
  else
    echo "Error: sha256sum or shasum is required." >&2
    rm -f "${current_sums}"
    return 1
  fi

  rm -f "${current_sums}"
  return "${status}"
}

needs_download() {
  local expected_fingerprint="$1"
  local ctl_binary="$2"
  local ctl_fingerprint

  ctl_fingerprint="$(read_binary_fingerprint "${ctl_binary}" || true)"

  [[ "${ctl_fingerprint}" == "${expected_fingerprint}" ]] || return 0
  [[ -f "${BIN_DIR}/SHA256SUMS" ]] || return 0

  verify_checksums "${BIN_DIR}" \
    "$(basename "${ctl_binary}")" \
    >/dev/null || return 0

  return 1
}

cd "${REPO_ROOT}"

if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then
  echo "Warning: working tree has uncommitted changes." >&2
fi

fingerprint="$("${REPO_ROOT}/tooling/scripts/tooling_fingerprint.sh")"
short_fingerprint="${fingerprint:0:12}"
tag="specflow-tooling-${short_fingerprint}"
suffix="$(platform_suffix)"
ctl_name="specflowctl-${suffix}"
ctl_path="${BIN_DIR}/${ctl_name}"

if ! needs_download "${fingerprint}" "${ctl_path}"; then
  echo "Local binary already matches ${tag}."
  exit 0
fi

if ! git ls-remote --exit-code --tags origin "refs/tags/${tag}" >/dev/null 2>&1; then
  echo "Error: release tag does not exist on origin: ${tag}" >&2
  echo "Run push_with_release.sh on main first, then run this script again." >&2
  exit 1
fi

download_dir="$(mktemp -d)"
base="https://github.com/Bingordinary/SpecFlow/releases/download/${tag}"

echo "Downloading ${tag} binary for ${suffix}..."
curl -fL -o "${download_dir}/${ctl_name}" "${base}/${ctl_name}"
curl -fL -o "${download_dir}/SHA256SUMS" "${base}/SHA256SUMS"

verify_checksums "${download_dir}" "${ctl_name}"

mkdir -p "${BIN_DIR}"
mv "${download_dir}/${ctl_name}" "${ctl_path}"
mv "${download_dir}/SHA256SUMS" "${BIN_DIR}/SHA256SUMS"
chmod +x "${ctl_path}"

echo "Installed ${ctl_name} and SHA256SUMS from ${tag}."
