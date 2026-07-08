#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: pull_with_release.sh

Pull the current SpecFlow branch from origin.
Then run update_tooling_binaries.sh to make sure the current platform's
specflowctl and specflow-reader binaries match the pulled tooling source
fingerprint.
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

cd "${REPO_ROOT}"

branch="$(git branch --show-current)"
if [[ -z "${branch}" ]]; then
  echo "Error: detached HEAD is not supported. Check out a branch before pulling." >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Error: working tree is not clean. Commit or stash changes before pulling." >&2
  exit 1
fi

remote_url="$(git remote get-url origin 2>/dev/null || true)"
if [[ -z "${remote_url}" ]]; then
  echo "Error: git remote 'origin' is missing." >&2
  exit 1
fi

echo "Pulling ${branch} from origin..."
git pull --ff-only origin "${branch}"

# Clear tooling/bin before updating binaries, so stale files are
# removed before fresh ones are downloaded.
BIN_DIR="${REPO_ROOT}/tooling/bin"
if [[ -d "${BIN_DIR}" ]]; then
  rm -rf "${BIN_DIR}"
  echo "Cleared tooling/bin."
fi

# Delegate binary update to the standalone per-platform script.
"${SCRIPT_DIR}/update_tooling_binaries.sh"
