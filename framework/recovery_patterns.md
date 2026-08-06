# Recovery Patterns

Common situations where the standard validate → verify → promote flow diverges. This file is referenced by `framework/concepts.md` §3.

## 1. Code changed without updating candidate

This is the normal iteration pattern. Do not interrupt. When the user signals they are ready to check (quality check signal → run `verify`), verify will detect the divergence and enter divergence resolution.

## 2. Candidate changed without implementing

If the user changes the spec mid-implementation and then wants to check:
1. Run `validate` first (to confirm the new design is sound)
2. Then run `verify`

## 3. Stable and code have drifted (no candidate exists)

The implementation no longer matches recorded stable truth:
1. Run `verify` in stable-only mode to see the gap
2. If a gap exists, suggest creating a candidate fork to reconcile

## 4. Validate fails repeatedly

1. Check whether the issue is `fix_required` (concrete repair possible in the candidate) or `blocked` (requires user input)
2. If blocked: stop and present the question to the user

## 5. User disagrees with divergence suggestion

When the user disagrees with the agent's suggested direction (code_gap / spec_gap / needs_design / blocked) during divergence resolution:

1. Record the user's stated direction as the verdict.
2. Do not argue or re-suggest — the user has more context.
3. Proceed with the agreed next step per the direction table in `unit_verify_checklist.md` Step 7.

## 6. Rule Operation Unsafe or Blocked

When a rule operation cannot proceed safely (ambiguous, combines multiple actions, or previous step returned blocked):

1. **Route to exactly one action** — reduce the request to the smallest distinct rule action:
   - Creating new rule truth → write candidate rule (see `spec_writing_guide.md` §5.1)
   - Extracting unit-local truth → extract to rule (see `spec_writing_guide.md` §5.2)
   - Binding/unbinding a unit → edit unit `rule_refs` and body explanation (normal spec editing)
   - Splitting/merging/renaming rules → manual multi-step change: create/update rule files (see `spec_writing_guide.md` §5.1), update consumer `rule_refs`, delete old files
   - Retiring a rule → follow the formal retirement flow (see `spec_writing_guide.md` §5.5 and `rule_promote_workflow.md`): add `status: retired` to the candidate rule frontmatter, clear every explicit `rule_refs` reference to it, run `validate@{rule}`, then `specflowctl promote --rule <id>` with user confirmation. Do not delete rule files manually — `promote` is the only operation that removes stable files
2. **Raise a clarification checkpoint** when the requested meaning is unclear — ask the user for specifics before proceeding
3. **Raise a decision checkpoint** when the user must choose between two valid approaches
4. **Raise a prerequisite checkpoint** when a legal upstream action must happen before the rule change (e.g., a consuming unit must be forked to candidate before its binding can change)

## 7. File-not-found false negative

Pattern-based search (directory-wide or wildcard search) results are
agent-dependent — an empty result does not guarantee the file is absent.

1. When the workflow provides an exact path, always use **direct path access**
   to read or check the file, rather than searching with a pattern
2. If pattern-based search returns empty but a file is expected at a known
   path, fall back to direct path access for the specific file to confirm
3. Only proceed with the normal "file missing" procedure after direct path
   access confirms the file does not exist
