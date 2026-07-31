#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: version_check.sh

Check the installed SpecFlow version against the remote.
Prints the local commit, the remote latest commit, and how many commits
the local version is behind.

Output is machine-readable "key: value" lines.
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
if [[ "${layout}" == "unknown_nested" ]]; then
  echo "Error: cannot locate the SpecFlow installation." >&2
  echo "Run version_check.sh from a SpecFlow installation inside your project, or from the SpecFlow source repository." >&2
  exit 1
fi

echo "layout: ${layout}"

branch="$(git branch --show-current)"
if [[ -n "${branch}" ]]; then
  echo "branch: ${branch}"
  remote_ref="refs/heads/${branch}"
else
  echo "branch: (detached HEAD)"
  remote_ref=""
fi

echo "local_commit: $(git rev-parse --short=12 HEAD)"
echo "local_date: $(git log -1 --format=%cd --date=short HEAD)"
echo "local_subject: $(git log -1 --format=%s HEAD)"

report_unavailable() {
  echo "remote_commit: unavailable"
  echo "remote_date: unavailable"
  echo "remote_subject: unavailable"
  echo "behind_count: unavailable"
  echo "ahead_count: unavailable"
  echo "remote_reason: $1"
}

remote_url="$(git remote get-url origin 2>/dev/null || true)"
if [[ -z "${remote_url}" ]]; then
  report_unavailable "git remote 'origin' is missing"
  exit 0
fi

# Detached HEAD: resolve the remote default branch live instead of relying
# on the local symbolic ref origin/HEAD, which fetch never updates.
if [[ -z "${remote_ref}" ]]; then
  symref_exit=0
  symref_output="$(git ls-remote --symref origin HEAD 2>/dev/null)" || symref_exit=$?
  if [[ "${symref_exit}" -ne 0 ]]; then
    report_unavailable "cannot reach origin (network or repository unavailable)"
    exit 0
  fi
  remote_ref="$(printf '%s\n' "${symref_output}" | awk '$1 == "ref:" { print $2; exit }')"
  if [[ -z "${remote_ref}" ]]; then
    report_unavailable "remote HEAD does not point to a branch"
    exit 0
  fi
fi

# Probe the remote ref before fetching, so a missing ref (e.g. a branch
# never pushed) is not misreported as a network problem. ls-remote exits 2
# when no matching ref exists and non-zero for unreachable remotes.
ls_exit=0
git ls-remote --exit-code origin "${remote_ref}" >/dev/null 2>&1 || ls_exit=$?
if [[ "${ls_exit}" -eq 2 ]]; then
  report_unavailable "remote ref ${remote_ref} not found (local branch may not be pushed)"
  exit 0
elif [[ "${ls_exit}" -ne 0 ]]; then
  report_unavailable "cannot reach origin (network or repository unavailable)"
  exit 0
fi

if ! git fetch origin "${remote_ref}" >/dev/null 2>&1; then
  report_unavailable "fetch from origin failed (network or repository unavailable)"
  exit 0
fi

tracking_ref="origin/${remote_ref#refs/heads/}"
if git rev-parse -q --verify "${tracking_ref}" >/dev/null 2>&1; then
  echo "remote_commit: $(git rev-parse --short=12 "${tracking_ref}")"
  echo "remote_date: $(git log -1 --format=%cd --date=short "${tracking_ref}")"
  echo "remote_subject: $(git log -1 --format=%s "${tracking_ref}")"
  echo "behind_count: $(git rev-list --count HEAD.."${tracking_ref}")"
  echo "ahead_count: $(git rev-list --count "${tracking_ref}"..HEAD)"
else
  report_unavailable "remote ref ${tracking_ref} not found after fetch"
fi
