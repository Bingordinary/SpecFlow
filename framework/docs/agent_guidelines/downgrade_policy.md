# Downgrade Policy

## 1. Purpose

This file defines the shared downgrade rules used when verification evidence is not fully clean but the executor still needs to decide whether the flow may continue, must stay at the current step, or must fall back.

It answers five questions:

1. what "downgrade" means in Spec Flow
2. which evidence states may still be downgraded
3. which evidence states must stop progression immediately
4. how `cand_verify` and `stable_verify` consume the same downgrade rules
5. which standardized `fallback_reason_code` values are allowed when downgrade does not hold

This is a centralized governance policy. It does not replace command-local procedure text.

---

## 2. Scope

This policy applies only where a command has already completed substantive verification work and now needs to judge whether imperfect coverage may still support the next lifecycle decision.

By default it governs:

1. `cand_verify`
2. `stable_verify`

It does not govern:

1. `cand_check` closure blocking
2. pass-gate binding validation in `cand_plan`, `cand_impl`, or `cand_promote`
3. human clarification checkpoints that must be written back into truth first

---

## 3. Core Terms

### 3.1 Downgrade

`downgrade` means:

1. the verification result is not a full clean pass
2. but the remaining gap is narrow enough, explicit enough, and low-risk enough that the command may still make a smaller safe conclusion instead of treating the whole verification round as failed

In plain words:

1. downgrade never upgrades weak evidence into strong evidence
2. downgrade only allows a narrower safe conclusion when the remaining uncertainty is already bounded

### 3.2 Verification Status Terms

These status meanings are fixed:

1. `pass`
   - checked and aligned
2. `partial`
   - some verification evidence is missing or reduced, but the checked evidence still bounds the unverified part closely enough for a narrower conclusion
3. `not_checked`
   - a planned verification item was not checked in this round
4. `fail`
   - checked evidence confirms misalignment with current truth
5. `evidence_incomplete`
   - the current round cannot safely judge whether the missing or weakly checked area is low-risk

### 3.3 Narrower Safe Conclusion

A narrower safe conclusion means:

1. for `cand_verify`, promotion may proceed only when the remaining uncertainty does not weaken the candidate's acceptance basis for this round
2. for `stable_verify`, "still aligned with stable" may be claimed only when the remaining uncertainty does not weaken confidence in the stable contract's externally observable behavior

---

## 4. Allowed Downgrade Cases

Downgrade is allowed only when all of the following hold:

1. there is no `fail`
2. the checked evidence does not indicate implementation deviation
3. every `partial` or `not_checked` item has an explicit risk note
4. the risk note explains why the unchecked area is bounded and why it does not affect the command's current release decision
5. the remaining uncertainty does not hide a likely change in externally observable behavior, protocol meaning, or acceptance meaning

Additional rules:

1. `partial` may be downgraded only when the missing portion is smaller than the evidence already collected and the checked evidence still constrains the same behavior path tightly.
2. `not_checked` may be downgraded only when the unchecked item is non-core for the current gate and its risk is explicitly bounded by other checked evidence, stable implementation symmetry, or a clearly stated environmental limitation.
3. downgrade is never allowed merely because the executor "believes it is probably fine."
4. downgrade is never allowed when the missing evidence concerns the only proof for a key acceptance point.

---

## 5. Forbidden Downgrade Cases

Downgrade is forbidden when any of the following hold:

1. any `fail` exists
2. any unchecked or partially checked item covers a key acceptance point with no other direct evidence
3. the missing evidence could hide a protocol break, state-machine break, persistence break, or externally visible behavior break
4. the current round cannot explain why the remaining uncertainty is low-risk
5. the missing evidence was caused by truth drift, binding drift, shared-appendix drift, or baseline drift

When downgrade is forbidden:

1. do not claim the stronger lifecycle conclusion
2. emit the command's standardized `fallback_reason_code`
3. move to the smallest still-valid next step

---

## 6. Command Mapping

### 6.1 `cand_verify`

`cand_verify` may allow promotion under downgrade only when:

1. all rules from Section 4 hold
2. the candidate's key acceptance basis remains covered
3. the remaining uncertainty does not weaken promotion confidence for the current candidate version

If downgrade does not hold:

1. use `implementation_deviation` when checked evidence shows the code does not satisfy the candidate
2. use `evidence_incomplete` when the code may still be correct but the remaining uncertainty is not bounded tightly enough
3. use `truth_drift`, `binding_drift`, `baseline_drift`, or `shared_appendix_drift` when the upstream truth relation changed

### 6.2 `stable_verify`

`stable_verify` may still conclude "aligned with stable" under downgrade only when:

1. all rules from Section 4 hold
2. the remaining uncertainty does not weaken confidence in the current stable contract
3. no unchecked area could hide an externally visible drift against `stable`

If downgrade does not hold:

1. use `implementation_deviation` when checked evidence shows drift from `stable`
2. use `evidence_incomplete` when alignment cannot be claimed safely because the remaining uncertainty is still too large
3. use `shared_appendix_drift` when a bound stable Shared Appendix changed enough that stable alignment can no longer be claimed safely

---

## 7. Output Contract

When a governed command applies this policy, its output should include:

1. whether downgrade was considered
2. the list of `partial` or `not_checked` items
3. the risk note for each downgraded item
4. whether downgrade was accepted or rejected
5. the resulting next-step conclusion
6. the standardized `fallback_reason_code` first when downgrade was rejected

---

## 8. Relationship To Other Files

This policy works together with:

1. `specflow/framework/docs/agent_guidelines/commands/cand_verify.md`
2. `specflow/framework/docs/agent_guidelines/commands/stable_verify.md`
3. `specflow/framework/docs/agent_guidelines/command_policy.md`

Priority rules:

1. command files define command-local procedure and state updates
2. this file defines the shared downgrade semantics consumed by those commands
3. command-local text must not redefine a conflicting downgrade meaning

---

## 9. Non-Goals

This file does not:

1. redefine verification evidence formats
2. replace `cand_verify` or `stable_verify`
3. convert weak evidence into a normal pass
4. decide business truth completeness
