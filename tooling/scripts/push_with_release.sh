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

source "${SCRIPT_DIR}/common/layout.sh"

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

if ! git fetch origin main >/dev/null 2>&1; then
  echo "Error: fetch from origin failed (network or repository unavailable). Retry spec_flow_push after the network recovers." >&2
  exit 1
fi

behind="$(git rev-list --count HEAD..origin/main)"
if [[ "${behind}" -gt 0 ]]; then
  echo "Error: local is behind origin/main by ${behind} commit(s). Run spec_flow_push (Stage 1) to fetch and resolve before pushing." >&2
  exit 1
fi

echo "Computing tooling source fingerprint..."
fingerprint="$(cd "${REPO_ROOT}/tooling" && go run ./cmd/specflowctl tooling-fingerprint --repo-root "${REPO_ROOT}")"
short_fingerprint="${fingerprint:0:12}"

fingerprint_file="${REPO_ROOT}/tooling/fingerprint.txt"
if [[ ! -f "${fingerprint_file}" ]] || [[ "$(cat "${fingerprint_file}")" != "${fingerprint}" ]]; then
  printf '%s\n' "${fingerprint}" >"${fingerprint_file}"
  git add tooling/fingerprint.txt
  if ! git commit -m "chore(tooling): record tooling fingerprint ${short_fingerprint}"; then
    echo "Error: failed to commit fingerprint metadata. Configure git user.name and user.email first." >&2
    exit 1
  fi
fi

echo "Pushing ${branch} to origin..."
git push origin "${branch}"

tag="specflow-tooling-${short_fingerprint}"

if git ls-remote --exit-code --tags origin "refs/tags/${tag}" >/dev/null 2>&1; then
  echo "Release tag already exists on origin: ${tag}"
  echo "No release tag push needed."
  exit 0
fi

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null 2>&1; then
  tag_commit="$(git rev-list -n 1 "${tag}")"
  head_commit="$(git rev-parse HEAD)"
  if [[ "${tag_commit}" != "${head_commit}" ]]; then
    echo "Error: local tag ${tag} exists but does not point to HEAD." >&2
    echo "Delete or inspect the local tag manually before pushing a release." >&2
    exit 1
  fi
else
  git tag "${tag}"
fi

echo "Pushing release tag ${tag}..."
git push origin "${tag}"
echo "Release workflow triggered by ${tag}."
