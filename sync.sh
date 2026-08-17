#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RULE_FILE="${SCRIPT_DIR}/RULE.md"
ENTRY_FILES=("AGENTS.md" "CLAUDE.md" "GEMINI.md")

if [[ ! -f "${RULE_FILE}" ]]; then
  echo "Error: ${RULE_FILE} not found" >&2
  exit 1
fi

updated=0
for entry in "${ENTRY_FILES[@]}"; do
  target="${SCRIPT_DIR}/${entry}"
  if [[ -f "${target}" ]] && cmp -s "${RULE_FILE}" "${target}"; then
    echo "  Unchanged: ${entry}"
    continue
  fi
  cp "${RULE_FILE}" "${target}"
  echo "  Updated:   ${entry}"
  updated=$((updated + 1))
done

for entry in "${ENTRY_FILES[@]}"; do
  cmp -s "${RULE_FILE}" "${SCRIPT_DIR}/${entry}" || {
    echo "Error: ${entry} does not match ${RULE_FILE} after sync" >&2
    exit 1
  }
done

echo "sync.sh complete. ${updated} file(s) updated, all entry files match RULE.md."