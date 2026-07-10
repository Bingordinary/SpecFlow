# Recovery Patterns

Common situations where the standard validate → verify → promote flow diverges. This file is referenced by `framework/concepts.md` §3.

## 1. Code changed without updating candidate

This is the normal iteration pattern. Do not interrupt. When the user signals they are ready to check (verify intent → run `spec_verify`), verify will detect the divergence and enter divergence resolution.

## 2. Candidate changed without implementing

If the user changes the spec mid-implementation and then wants to check:
1. Run `spec_validate` first (to confirm the new design is sound)
2. Then run `spec_verify`

## 3. Stable and code have drifted (no candidate exists)

The implementation no longer matches recorded stable truth:
1. Run `spec_verify` in stable-only mode to see the gap
2. If a gap exists, suggest creating a candidate fork to reconcile

## 4. Validate fails repeatedly

1. Check whether the issue is `fix_required` (concrete repair possible in the candidate) or `blocked` (requires user input)
2. If blocked: stop and present the question to the user
