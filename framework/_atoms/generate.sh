#!/usr/bin/env bash
# Atom Generation Script
# Reads manifest.txt and atom source files, writes generated content into target files
# between ==ATOM_BEGIN:atom_id== and ==ATOM_END:atom_id== markers.
#
# Usage: ./generate.sh [--check] [--verbose]
#   --check     Dry-run mode: report what would change without writing
#   --verbose   Show per-file status

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MANIFEST="$SCRIPT_DIR/manifest.txt"
CHECK_MODE=false
VERBOSE=false
CHANGED=0
UNCHANGED=0
MISSING_MARKER=0
ERRORS=0

for arg in "$@"; do
  case "$arg" in
    --check) CHECK_MODE=true ;;
    --verbose) VERBOSE=true ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

log_verbose() { if $VERBOSE; then echo "  $1"; fi; }

generate_atom() {
  local atom_id="$1"
  local source_rel="$2"
  local targets="$3"
  local source_file="$SCRIPT_DIR/$source_rel"

  if [ ! -f "$source_file" ]; then
    echo "ERROR: Atom source file not found: $source_file"
    ERRORS=$((ERRORS + 1))
    return
  fi

  # Read atom content (remove trailing blank line but preserve internal structure)
  local atom_content
  atom_content=$(<"$source_file")

  # Split target files
  IFS=',' read -ra TARGET_ARR <<< "$targets"
  for target_rel in "${TARGET_ARR[@]}"; do
    target_rel=$(echo "$target_rel" | xargs)  # trim whitespace
    local target_file="$REPO_ROOT/$target_rel"

    if [ ! -f "$target_file" ]; then
      echo "ERROR: Target file not found: $target_file (atom: $atom_id)"
      ERRORS=$((ERRORS + 1))
      continue
    fi

    local target_content
    target_content=$(<"$target_file")

    local begin_marker="==ATOM_BEGIN:${atom_id}=="
    local end_marker="==ATOM_END:${atom_id}=="

    if ! echo "$target_content" | grep -qF "$begin_marker"; then
      echo "WARNING: Missing begin marker '$begin_marker' in $target_rel (atom: $atom_id)"
      MISSING_MARKER=$((MISSING_MARKER + 1))
      continue
    fi
    if ! echo "$target_content" | grep -qF "$end_marker"; then
      echo "WARNING: Missing end marker '$end_marker' in $target_rel (atom: $atom_id)"
      MISSING_MARKER=$((MISSING_MARKER + 1))
      continue
    fi

    # Extract the part between markers in the target file (for comparison only)
    local old_block
    old_block=$(echo "$target_content" | sed -n "/^${begin_marker}$/,/^${end_marker}$/p")

    # Build new target content: everything before marker + atom content + everything after marker
    local new_content
    # Use awk for robust multi-line replacement
    new_content=$(awk -v begin="$begin_marker" -v end="$end_marker" -v atom="$atom_content" '
      BEGIN { in_block=0; printed_atom=0 }
      $0 == begin { print $0; printf "%s\n", atom; in_block=1; printed_atom=1; next }
      $0 == end   { if (in_block && printed_atom) { print $0; in_block=0; next } }
      !in_block   { print $0 }
    ' "$target_file")

    if [ "$new_content" = "$target_content" ]; then
      log_verbose "UNCHANGED $target_rel ($atom_id)"
      UNCHANGED=$((UNCHANGED + 1))
    else
      log_verbose "CHANGED   $target_rel ($atom_id)"
      CHANGED=$((CHANGED + 1))
      if ! $CHECK_MODE; then
        echo "$new_content" > "$target_file"
      fi
    fi
  done
}

echo "=== Atom Generation ==="
echo "Manifest: $MANIFEST"
echo "Repo root: $REPO_ROOT"
echo ""

while IFS='|' read -r atom_id source_file targets; do
  # Skip comments and blank lines
  [[ "$atom_id" =~ ^[[:space:]]*# ]] && continue
  [[ -z "$atom_id" ]] && continue
  atom_id=$(echo "$atom_id" | xargs)
  source_file=$(echo "$source_file" | xargs)
  targets=$(echo "$targets" | xargs)

  generate_atom "$atom_id" "$source_file" "$targets"
done < "$MANIFEST"

echo ""
echo "=== Summary ==="
echo "Changed:  $CHANGED"
echo "Unchanged: $UNCHANGED"
echo "Missing markers: $MISSING_MARKER"
echo "Errors: $ERRORS"

if [ "$ERRORS" -gt 0 ] || [ "$MISSING_MARKER" -gt 0 ]; then
  echo "WARNING: Generation completed with issues."
fi

if $CHECK_MODE; then
  echo "Dry-run mode: no files were modified."
fi
