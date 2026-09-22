#!/usr/bin/env bash
# Atom Verification Script
# Checks that all target files contain the correct atom content between markers.
# Returns non-zero exit code if any drift is detected.
#
# Also runs a content regression guard on the report skeleton (atom: report_skeleton):
# the pre-fix claim "fixes applied —" must not reappear in the atom source or its targets,
# the three fix-lifecycle state tokens must exist in the atom source, and the shared
# finding-block field tokens (issue #40) must stay present in the atom source.
#
# Usage: ./verify.sh [--verbose]
#   --verbose   Show per-file verification status

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MANIFEST="$SCRIPT_DIR/manifest.txt"
VERBOSE=false
PASSED=0
DRIFTED=0
MISSING_MARKER=0
ERRORS=0
REGRESSION_ERRORS=0

for arg in "$@"; do
  case "$arg" in
    --verbose) VERBOSE=true ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

log_verbose() { if $VERBOSE; then echo "  $1"; fi; }

verify_atom() {
  local atom_id="$1"
  local source_rel="$2"
  local targets="$3"
  local source_file="$SCRIPT_DIR/$source_rel"

  if [ ! -f "$source_file" ]; then
    echo "ERROR: Atom source file not found: $source_file"
    ERRORS=$((ERRORS + 1))
    return
  fi

  local atom_content
  atom_content=$(<"$source_file")

  IFS=',' read -ra TARGET_ARR <<< "$targets"
  for target_rel in "${TARGET_ARR[@]}"; do
    target_rel=$(echo "$target_rel" | xargs)
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

    if ! grep -qF -- "$begin_marker" "$target_file"; then
      echo "MISSING  $target_rel — begin marker '$begin_marker' not found"
      MISSING_MARKER=$((MISSING_MARKER + 1))
      continue
    fi
    if ! grep -qF -- "$end_marker" "$target_file"; then
      echo "MISSING  $target_rel — end marker '$end_marker' not found"
      MISSING_MARKER=$((MISSING_MARKER + 1))
      continue
    fi

    # Extract content between markers from target file
    local target_block
    target_block=$(echo "$target_content" | sed -n "/^${begin_marker}$/,/^${end_marker}$/p" | sed '1d;$d')

    # Normalize: trim surrounding blank lines and trailing spaces
    local atom_norm target_norm
    atom_norm=$(echo "$atom_content" | awk '{lines[count++]=$0} END {for(i=0;i<count;i++) if(lines[i]~/./){first=i;break} if(first<0)exit; for(i=count-1;i>=0;i--) if(lines[i]~/./){last=i;break} for(i=first;i<=last;i++) print lines[i]}')
    target_norm=$(echo "$target_block" | awk '{lines[count++]=$0} END {for(i=0;i<count;i++) if(lines[i]~/./){first=i;break} if(first<0)exit; for(i=count-1;i>=0;i--) if(lines[i]~/./){last=i;break} for(i=first;i<=last;i++) print lines[i]}')
    atom_norm=$(echo "$atom_norm" | sed 's/[[:space:]]*$//')
    target_norm=$(echo "$target_norm" | sed 's/[[:space:]]*$//')

    if [ "$atom_norm" = "$target_norm" ]; then
      log_verbose "OK       $target_rel ($atom_id)"
      PASSED=$((PASSED + 1))
    else
      echo "DRIFT    $target_rel ($atom_id) — target content does not match atom source"
      DRIFTED=$((DRIFTED + 1))
    fi
  done
}

# Content regression guard for the report skeleton (issue #38).
# A gate report is always produced before any fix is applied, so an actionable finding's
# report-time Next step must use the `finding_open` wording — it must never claim the fix
# was executed. The old pre-fix claim "fixes applied —" must not reappear anywhere in the
# atom source or its target files (also catches command-specific templates outside the
# atom block), and the three fix-lifecycle state tokens must exist in the atom source.
check_report_skeleton_regression() {
  local atom_id="report_skeleton"
  local source_file=""
  local targets=""
  local found=false

  while IFS='|' read -r id src tgts; do
    [[ "$id" =~ ^[[:space:]]*# ]] && continue
    [[ -z "$id" ]] && continue
    id=$(echo "$id" | xargs)
    [[ "$id" = "$atom_id" ]] || continue
    source_file="$SCRIPT_DIR/$(echo "$src" | xargs)"
    targets=$(echo "$tgts" | xargs)
    found=true
  done < "$MANIFEST"

  if ! $found; then
    echo "ERROR: Atom '$atom_id' not found in manifest — cannot run the report wording regression guard"
    ERRORS=$((ERRORS + 1))
    return
  fi

  local files=("$source_file")
  IFS=',' read -ra TARGET_ARR <<< "$targets"
  local target_rel
  for target_rel in "${TARGET_ARR[@]}"; do
    target_rel=$(echo "$target_rel" | xargs)
    files+=("$REPO_ROOT/$target_rel")
  done

  local f
  for f in "${files[@]}"; do
    if [ ! -f "$f" ]; then
      echo "ERROR: Regression guard target file not found: $f (atom: $atom_id)"
      ERRORS=$((ERRORS + 1))
      continue
    fi
    if grep -qiF -- 'fixes applied —' "$f" || grep -qiF -- 'fixes applied -' "$f"; then
      echo "REGRESSION $f — pre-fix claim 'fixes applied —' found (an actionable finding's report must use the finding_open wording)"
      REGRESSION_ERRORS=$((REGRESSION_ERRORS + 1))
    fi
  done

  local token
  for token in finding_open fixed_pending_recheck verified; do
    if ! grep -qF -- "$token" "$source_file"; then
      echo "REGRESSION $source_file — required fix-lifecycle state token '$token' missing"
      REGRESSION_ERRORS=$((REGRESSION_ERRORS + 1))
    fi
  done

  # Finding-block contract guard (issue #40): every finding must carry the
  # self-contained block fields. A deliberate rename must update this guard
  # together with the contract.
  for token in 'problem:' 'evidence:' 'impact:' 'fix:' 'decision:' 'options:' 'ref:'; do
    if ! grep -qF -- "$token" "$source_file"; then
      echo "REGRESSION $source_file — required finding-block field token '$token' missing"
      REGRESSION_ERRORS=$((REGRESSION_ERRORS + 1))
    fi
  done
}

echo "=== Atom Verification ==="
echo "Manifest: $MANIFEST"
echo "Repo root: $REPO_ROOT"
echo ""

while IFS='|' read -r atom_id source_file targets; do
  [[ "$atom_id" =~ ^[[:space:]]*# ]] && continue
  [[ -z "$atom_id" ]] && continue
  atom_id=$(echo "$atom_id" | xargs)
  source_file=$(echo "$source_file" | xargs)
  targets=$(echo "$targets" | xargs)

  verify_atom "$atom_id" "$source_file" "$targets"
done < "$MANIFEST"

echo ""
echo "=== Report wording regression guard ==="
check_report_skeleton_regression

echo ""
echo "=== Summary ==="
echo "Passed:   $PASSED"
echo "Drifted:  $DRIFTED"
echo "Missing:  $MISSING_MARKER"
echo "Errors:   $ERRORS"
echo "Regression failures: $REGRESSION_ERRORS"

if [ "$DRIFTED" -gt 0 ] || [ "$MISSING_MARKER" -gt 0 ] || [ "$ERRORS" -gt 0 ] || [ "$REGRESSION_ERRORS" -gt 0 ]; then
  echo "RESULT: VERIFICATION FAILED"
  exit 1
else
  echo "RESULT: VERIFICATION PASSED"
  exit 0
fi
