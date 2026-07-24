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

  # Read atom content
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

    # Escape markers for use as sed regex patterns
    local escaped_begin escaped_end
    escaped_begin=$(printf '%s' "$begin_marker" | sed -e 's/[][\.*^$()+{}?|/]/\\&/g')
    escaped_end=$(printf '%s' "$end_marker" | sed -e 's/[][\.*^$()+{}?|/]/\\&/g')

    # Build new target content: lines before begin marker + begin marker + atom content + end marker + lines after end marker
    local tmpfile
    tmpfile=$(mktemp)
    # Print lines before begin marker (excluding the marker line itself)
    sed "/^${escaped_begin}$/q" "$target_file" | sed '$d' > "$tmpfile"
    # Print begin marker
    echo "$begin_marker" >> "$tmpfile"
    # Print atom content
    printf '%s\n' "$atom_content" >> "$tmpfile"
    # Print end marker
    echo "$end_marker" >> "$tmpfile"
    # Print lines after end marker
    sed "1,/^${escaped_end}$/d" "$target_file" >> "$tmpfile"

    local new_content
    new_content=$(<"$tmpfile")
    rm -f "$tmpfile"

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
