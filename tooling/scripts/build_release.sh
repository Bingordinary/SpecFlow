#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: build_release.sh

Rebuild all local SpecFlow tooling release binaries under <tooling-root>/bin.

Environment:
  GOCACHE  Go build cache directory. Defaults to /tmp/go-build-cache.
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
TOOLING_DIR="${REPO_ROOT}/tooling"

export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"

cd "${TOOLING_DIR}"
go run ./cmd/specflowctl build-release --repo-root "${REPO_ROOT}"
