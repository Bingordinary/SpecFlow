#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: tooling_fingerprint.sh [--short]

Print the full tooling source fingerprint.
Use --short to print the first 12 characters.
USAGE
}

SHORT=0
for arg in "$@"; do
  case "${arg}" in
    --short)
      SHORT=1
      ;;
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
TOOLING_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

FILE_LIST="$(mktemp)"
SORTED_FILE_LIST="$(mktemp)"
trap 'rm -f "${FILE_LIST}" "${SORTED_FILE_LIST}"' EXIT

add_go_tree() {
  local rel_root="$1"
  local abs_root="${TOOLING_ROOT}/${rel_root}"

  if [[ ! -d "${abs_root}" ]]; then
    echo "Error: required tooling source directory missing: ${rel_root}" >&2
    exit 1
  fi

  find "${abs_root}" -type f -name '*.go' | while IFS= read -r abs_path; do
    case "$(basename "${abs_path}")" in
      *_test.go) continue ;;
    esac
    printf '%s\n' "${abs_path#"${TOOLING_ROOT}/"}"
  done >>"${FILE_LIST}"
}

add_required_file() {
  local rel_path="$1"
  if [[ ! -f "${TOOLING_ROOT}/${rel_path}" ]]; then
    echo "Error: required tooling source file missing: ${rel_path}" >&2
    exit 1
  fi
  printf '%s\n' "${rel_path}" >>"${FILE_LIST}"
}

add_optional_file() {
  local rel_path="$1"
  if [[ -f "${TOOLING_ROOT}/${rel_path}" ]]; then
    printf '%s\n' "${rel_path}" >>"${FILE_LIST}"
  fi
}

add_go_tree "cmd"
add_go_tree "internal"
add_required_file "go.mod"
add_required_file "manifest.tsv"
add_optional_file "go.sum"

LC_ALL=C sort -u "${FILE_LIST}" >"${SORTED_FILE_LIST}"

if command -v sha256sum >/dev/null 2>&1; then
  HASH_CMD=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  HASH_CMD=(shasum -a 256)
else
  echo "Error: sha256sum or shasum is required." >&2
  exit 1
fi

fingerprint="$(
  while IFS= read -r rel_path; do
    printf '%s\0' "${rel_path}"
    tr -d '\r' <"${TOOLING_ROOT}/${rel_path}"
    printf '\0'
  done <"${SORTED_FILE_LIST}" | "${HASH_CMD[@]}" | awk '{print $1}'
)"

if [[ "${SHORT}" -eq 1 ]]; then
  printf '%s\n' "${fingerprint:0:12}"
else
  printf '%s\n' "${fingerprint}"
fi
