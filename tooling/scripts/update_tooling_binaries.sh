#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: update_tooling_binaries.sh [--all] [--current-only]

Download and install specflowctl binaries that match the current
tooling source fingerprint from the matching GitHub Release.

By default downloads binaries for all platforms (linux-amd64, linux-arm64,
darwin-amd64, darwin-arm64, windows-amd64.exe, windows-arm64.exe) so a
Syncthing-synced project directory stays usable on every platform.

If tooling/platforms.txt exists, only the platforms listed there are
downloaded (one per line, '#' comments allowed, the 'windows-*.exe'
suffix is optional). The file is a local per-checkout preference: it
lives inside the SpecFlow checkout, is not committed, and survives
pulls. A missing file or an explicit --all / --current-only flag falls
back to the default behavior (all platforms).

Options:
  --all            Download all platforms (default)
  --current-only   Download only the current platform's binary

The script checks whether the local binaries already match the expected
fingerprint. If any required binary is missing or stale, it downloads
fresh binaries from the matching GitHub Release.
USAGE
}

MODE="all"
SEEN_ALL=0
SEEN_CURRENT=0
for arg in "$@"; do
  case "${arg}" in
    -h|--help)
      usage
      exit 0
      ;;
    --all)
      MODE="all"
      SEEN_ALL=1
      ;;
    --current-only)
      MODE="current"
      SEEN_CURRENT=1
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
BIN_DIR="${REPO_ROOT}/tooling/bin"
PLATFORMS_FILE="${REPO_ROOT}/tooling/platforms.txt"
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

all_platform_suffixes() {
  printf 'linux-amd64\n'
  printf 'linux-arm64\n'
  printf 'darwin-amd64\n'
  printf 'darwin-arm64\n'
  printf 'windows-amd64.exe\n'
  printf 'windows-arm64.exe\n'
}

load_configured_suffixes() {
  local line suffix
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%%#*}"
    line="$(printf '%s' "${line}" | tr -d '[:space:]')"
    [[ -z "${line}" ]] && continue
    case "${line}" in
      linux-amd64|linux-arm64|darwin-amd64|darwin-arm64) suffix="${line}" ;;
      windows-amd64|windows-amd64.exe) suffix="windows-amd64.exe" ;;
      windows-arm64|windows-arm64.exe) suffix="windows-arm64.exe" ;;
      *)
        echo "Error: unknown platform '${line}' in ${PLATFORMS_FILE}." >&2
        echo "Valid platforms: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64, windows-arm64." >&2
        return 1
        ;;
    esac
    case " ${TARGET_SUFFIXES[*]:+${TARGET_SUFFIXES[*]}} " in
      *" ${suffix} "*) ;;
      *) TARGET_SUFFIXES+=("${suffix}") ;;
    esac
  done < "${PLATFORMS_FILE}"
  return 0
}

resolve_target_suffixes() {
  TARGET_SUFFIXES=()
  CONFIGURED_SET=0
  if [[ "${MODE}" == "all" && "${SEEN_ALL}" == "0" && -f "${PLATFORMS_FILE}" ]]; then
    CONFIGURED_SET=1
    load_configured_suffixes || return 1
    if [[ ${#TARGET_SUFFIXES[@]} -eq 0 ]]; then
      echo "Error: ${PLATFORMS_FILE} exists but lists no platform." >&2
      return 1
    fi
  else
    local suffix
    while IFS= read -r suffix; do
      [[ -z "${suffix}" ]] && continue
      TARGET_SUFFIXES+=("${suffix}")
    done < <(all_platform_suffixes)
  fi
  return 0
}

warn_current_platform_excluded() {
  local current_suffix suffix listed=0
  current_suffix="$(platform_suffix 2>/dev/null || true)"
  [[ -z "${current_suffix}" ]] && return 0
  for suffix in "${TARGET_SUFFIXES[@]}"; do
    if [[ "${suffix}" == "${current_suffix}" ]]; then
      listed=1
      break
    fi
  done
  if [[ "${listed}" == "0" ]]; then
    echo "Warning: current platform (${current_suffix}) is not listed in ${PLATFORMS_FILE}; no binary for this machine will be installed." >&2
  fi
}

write_launchers() {
  mkdir -p "${BIN_DIR}"
  cat > "${BIN_DIR}/specflowctl" <<'SPECFLOWCTL_LAUNCHER'
#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
uname_os=$(uname -s)
uname_arch=$(uname -m)
case "${uname_os}" in
  Linux) os_name="linux" ;;
  Darwin) os_name="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) os_name="windows" ;;
  *)
    echo "Error: unsupported operating system: ${uname_os}" >&2
    exit 1
    ;;
esac
case "${uname_arch}" in
  x86_64|amd64) arch_name="amd64" ;;
  aarch64|arm64) arch_name="arm64" ;;
  *)
    echo "Error: unsupported CPU architecture: ${uname_arch}" >&2
    exit 1
    ;;
esac
if [ "${os_name}" = "windows" ]; then
  target="${script_dir}/specflowctl-${os_name}-${arch_name}.exe"
else
  target="${script_dir}/specflowctl-${os_name}-${arch_name}"
fi
if [ ! -x "${target}" ]; then
  echo "Error: specflowctl binary is missing or not executable: ${target}" >&2
  echo "Run update_tooling_binaries or build-release to install specflowctl binaries." >&2
  exit 1
fi
exec "${target}" "$@"
SPECFLOWCTL_LAUNCHER
  chmod +x "${BIN_DIR}/specflowctl"
  cat > "${BIN_DIR}/specflowctl.cmd" <<'SPECFLOWCTL_LAUNCHER'
@echo off
setlocal
set "specflowctl_arch=%PROCESSOR_ARCHITECTURE%"
if /I "%specflowctl_arch%"=="ARM64" (
  set "specflowctl_target=%~dp0specflowctl-windows-arm64.exe"
) else (
  set "specflowctl_target=%~dp0specflowctl-windows-amd64.exe"
)
if not exist "%specflowctl_target%" (
  echo Error: specflowctl binary is missing: "%specflowctl_target%" 1>&2
  echo Run update_tooling_binaries or build-release to install specflowctl binaries. 1>&2
  exit /b 1
)
"%specflowctl_target%" %*
SPECFLOWCTL_LAUNCHER
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
    echo "Error: SHA256SUMS does not contain the expected binary: ${ctl_name}" >&2
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

verify_checksums_many() {
  local dir="$1"
  shift
  local name
  for name in "$@"; do
    verify_checksums "${dir}" "${name}" || return 1
  done
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

needs_download_all() {
  local expected_fingerprint="$1"
  shift
  local suffix ctl_path current_suffix
  current_suffix="$(platform_suffix 2>/dev/null || true)"
  for suffix in "$@"; do
    ctl_path="${BIN_DIR}/specflowctl-${suffix}"
    if [[ ! -f "${ctl_path}" ]]; then
      return 0
    fi
    if [[ "${suffix}" == "${current_suffix}" ]]; then
      if ! needs_download "${expected_fingerprint}" "${ctl_path}"; then
        continue
      fi
      return 0
    else
      # Cross-platform binary: cannot execute, check existence + checksum only.
      if ! verify_checksums "${BIN_DIR}" "specflowctl-${suffix}" >/dev/null 2>&1; then
        return 0
      fi
    fi
  done
  # All binaries present — verify SHA256SUMS contains every entry
  local missing=0
  for suffix in "$@"; do
    if ! grep -qF "specflowctl-${suffix}" "${BIN_DIR}/SHA256SUMS" 2>/dev/null; then
      missing=1
      break
    fi
  done
  if [[ "${missing}" == "1" ]]; then
    return 0
  fi
  return 1
}

resolve_target_suffixes || exit 1

if [[ "${CONFIGURED_SET}" == "1" ]]; then
  echo "Platform set from ${PLATFORMS_FILE}: ${TARGET_SUFFIXES[*]}"
  warn_current_platform_excluded
fi

cd "${REPO_ROOT}"

if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then
  echo "Warning: working tree has uncommitted changes." >&2
fi

fingerprint_file="${REPO_ROOT}/tooling/fingerprint.txt"
if [[ ! -f "${fingerprint_file}" ]]; then
  echo "Error: tooling/fingerprint.txt is missing in this checkout." >&2
  echo "This version has no recorded fingerprint metadata — pull to a version that ships it first." >&2
  exit 1
fi
fingerprint="$(cat "${fingerprint_file}")"
short_fingerprint="${fingerprint:0:12}"
tag="specflow-tooling-${short_fingerprint}"

if [[ "${MODE}" == "all" ]]; then
  all_suffixes=("${TARGET_SUFFIXES[@]}")
  all_names=()
  for suffix in "${all_suffixes[@]}"; do
    all_names+=("specflowctl-${suffix}")
  done

  if ! needs_download_all "${fingerprint}" "${all_suffixes[@]}"; then
    write_launchers
    if [[ "${CONFIGURED_SET}" == "1" ]]; then
      echo "Local binaries already match ${tag} (${#all_suffixes[@]} platforms: ${all_suffixes[*]})."
    else
      echo "Local binaries already match ${tag} (all platforms)."
    fi
    exit 0
  fi

  if ! git ls-remote --exit-code --tags origin "refs/tags/${tag}" >/dev/null 2>&1; then
    echo "Error: release tag does not exist on origin: ${tag}" >&2
    echo "Run push_with_release.sh on main first, then run this script again." >&2
    exit 1
  fi

  download_dir="$(mktemp -d)"
  base="https://github.com/Bingordinary/SpecFlow/releases/download/${tag}"

  echo "Downloading ${tag} binaries for ${#all_suffixes[@]} platform(s): ${all_suffixes[*]}..."
  curl -fL -o "${download_dir}/SHA256SUMS" "${base}/SHA256SUMS"
  for suffix in "${all_suffixes[@]}"; do
    ctl_name="specflowctl-${suffix}"
    echo "  Downloading ${ctl_name}..."
    curl -fL -o "${download_dir}/${ctl_name}" "${base}/${ctl_name}"
  done

  verify_checksums_many "${download_dir}" "${all_names[@]}"

  mkdir -p "${BIN_DIR}"
  for suffix in "${all_suffixes[@]}"; do
    ctl_name="specflowctl-${suffix}"
    mv "${download_dir}/${ctl_name}" "${BIN_DIR}/${ctl_name}"
    if [[ "${ctl_name}" != *.exe ]]; then
      chmod +x "${BIN_DIR}/${ctl_name}"
    fi
  done
  mv "${download_dir}/SHA256SUMS" "${BIN_DIR}/SHA256SUMS"

  write_launchers
  echo "Installed ${#all_names[@]} binaries and SHA256SUMS from ${tag}."
  exit 0
fi

# --current-only
suffix="$(platform_suffix)"
ctl_name="specflowctl-${suffix}"
ctl_path="${BIN_DIR}/${ctl_name}"

if ! needs_download "${fingerprint}" "${ctl_path}"; then
  write_launchers
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

write_launchers
echo "Installed ${ctl_name} and SHA256SUMS from ${tag}."
