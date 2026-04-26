# Candidate Handoff Contract

## 1. Purpose

This file defines the shared handoff contract for the candidate-side command chain.

It answers five questions:

1. what each downstream step minimally requires from the upstream step
2. which bindings must be re-validated before consuming that handoff
3. which fallback step is the smallest valid recovery point when the handoff is invalid
4. which standardized `fallback_reason_code` must be used
5. how this contract relates to process-file READMEs and command files

This file is a centralized contract document. It does not replace command-specific procedure text.

---

## 2. Standard Reason Taxonomy

Candidate-side fallback, blocking, and resume explanations must use these standard `fallback_reason_code` values:

1. `truth_incomplete`
2. `gate_missing`
3. `truth_drift`
4. `binding_drift`
5. `baseline_drift`
6. `shared_contract_drift`
7. `shared_truth_conflict`
8. `governance_drift`
9. `implementation_deviation`
10. `evidence_incomplete`
11. `implementation_unknown`
12. `direction_unresolved`
13. `promotion_recovery`

Meaning rules:

1. `truth_incomplete`
   - the current candidate truth is still missing user-intent, boundary, protocol, or acceptance content needed for stable downstream work
2. `gate_missing`
   - a required upstream pass gate or plan file does not exist or no longer qualifies as a current valid gate
3. `truth_drift`
   - the candidate truth changed and the old downstream artifact no longer matches it
4. `binding_drift`
   - a process file still exists but its required bindings no longer match the current truth
5. `baseline_drift`
   - the formal global baseline relation no longer matches the current round
6. `shared_contract_drift`
   - a bound `shared_contract` truth, layer, version, body, or binding changed enough to invalidate the handoff
7. `shared_truth_conflict`
   - the current required reading range already confirms that the same formal behavior truth is defined twice and shared closure must happen before downstream work may continue
8. `governance_drift`
   - a required governance surface, registry, or framework-owned execution rule is missing, invalid, or contradictory enough that the current command cannot safely continue under the formal mechanism
9. `implementation_deviation`
   - implementation no longer satisfies the current candidate even though candidate truth still stands
10. `evidence_incomplete`
   - current verification evidence is still insufficient to close the next gate safely
11. `implementation_unknown`
   - candidate truth still stands, but bounded implementation-critical unknowns, external conditions, or missing implementation facts still prevent a stable plan from being written
12. `direction_unresolved`
   - candidate truth still stands, but more than one materially different implementation direction remains viable and a user decision is required before a stable plan may be written
13. `promotion_recovery`
   - `unit_promote` had already started mutating repository state and had to restore the unit back to candidate semantics before the chain could continue

Executors may add natural-language explanation, but the standardized code must appear first when a fallback or blocking reason is reported.

---

## 3. Handoff: `unit_check -> unit_plan`

### 3.1 Minimum Upstream Artifact

`unit_plan` minimally requires a current valid `_check_result/unit/{unit}.md`.

### 3.2 Required Re-Validation

Before consumption, `unit_plan` must re-validate:

1. `decision=pass`
2. `allow_next=true`
3. `next_command=unit_plan`
4. `truth_layer_ref`, `truth_file_ref`, `truth_version_ref`, and `truth_fingerprint` against the current candidate truth
5. current `unit_appendix_snapshot`
6. current `system_constraints` binding fields
7. current `shared_contract_snapshot`

### 3.3 Allowed Entry Condition

`unit_plan` may continue only when the pass gate still covers the current candidate round exactly.

### 3.4 Smallest Fallback

If the handoff is invalid, the smallest fallback is `unit_check`.

### 3.5 Allowed Reason Codes

Use only:

1. `truth_incomplete`
2. `gate_missing`
3. `truth_drift`
4. `binding_drift`
5. `baseline_drift`
6. `shared_contract_drift`
7. `implementation_unknown`
8. `direction_unresolved`

---

## 4. Handoff: `unit_plan -> unit_impl`

### 4.1 Minimum Upstream Artifacts

`unit_impl` minimally requires both:

1. a current valid `_check_result/unit/{unit}.md`
2. a current valid `_plans/active/{unit}.md`

### 4.2 Required Re-Validation

Before consumption, `unit_impl` must re-validate:

1. all required `_check_result` bindings from Section 3
2. current plan file path and existence
3. plan-bound `spec_file_ref`, `spec_version_ref`, and `spec_fingerprint`
4. plan-bound `unit_appendix_snapshot`
5. plan-bound `system_constraints` fields
6. plan-bound `shared_contract_snapshot`

### 4.3 Allowed Entry Condition

`unit_impl` may continue only when both the pass gate and the plan still cover the same current candidate round.

### 4.4 Smallest Fallback

If either artifact is missing or invalid, the smallest fallback is `unit_check`.

### 4.5 Allowed Reason Codes

Use only:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_contract_drift`

---

## 5. Handoff: `unit_impl -> unit_verify`

### 5.1 Minimum Upstream Artifacts

`unit_verify` minimally requires both:

1. a current valid `_check_result/unit/{unit}.md`
2. a current valid `_plans/active/{unit}.md`

### 5.2 Required Re-Validation

Before consumption, `unit_verify` must re-validate:

1. all required gate bindings
2. all required plan bindings
3. that the implementation state still matches the coverage scope claimed by the current round's plan progress

### 5.3 Allowed Entry Condition

`unit_verify` may continue only when verification still targets the same candidate truth round that implementation used.

### 5.4 Smallest Fallback

If bindings drift, the smallest fallback is `unit_check`.
If candidate truth still stands but implementation diverged, the fallback is `unit_impl`.

### 5.5 Allowed Reason Codes

Use only:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_contract_drift`
6. `implementation_deviation`
7. `evidence_incomplete`
8. `truth_incomplete`

---

## 6. Handoff: `unit_verify -> unit_promote`

### 6.1 Minimum Upstream Artifact

`unit_promote` minimally requires a current valid `_verify_result/unit/{unit}.md`.

### 6.2 Required Re-Validation

Before consumption, `unit_promote` must re-validate:

1. `decision=pass`
2. `allow_next=true`
3. `next_command=unit_promote`
4. `truth_layer_ref`, `truth_file_ref`, `truth_version_ref`, and `truth_fingerprint` against the current candidate truth
5. current `unit_appendix_snapshot`
6. current implementation still covered by `verification_scope_ref`
7. current `system_constraints` binding fields
8. current `shared_contract_snapshot`

### 6.3 Allowed Entry Condition

`unit_promote` may continue only when the verify result still covers current candidate truth, current implementation, and current baseline state together.

### 6.4 Smallest Fallback

If verification evidence is outdated or incomplete but candidate truth still stands, the smallest fallback is `unit_verify`.
If implementation no longer aligns with the candidate, the fallback is `unit_impl`.
If candidate truth or upstream bindings drifted, the fallback is `unit_check`.

### 6.5 Allowed Reason Codes

Use only:

1. `truth_drift`
2. `binding_drift`
3. `baseline_drift`
4. `shared_contract_drift`
5. `implementation_deviation`
6. `evidence_incomplete`

---

## 7. Handoff: `scenario_check -> scenario_verify`

### 7.1 Minimum Upstream Artifact

`scenario_verify` minimally requires a current valid `_check_result/scenario/{scenario}.md`.

### 7.2 Required Re-Validation

Before consumption, `scenario_verify` must re-validate:

1. `decision=pass`
2. `allow_next=true`
3. `next_command=scenario_verify`
4. `truth_layer_ref`, `truth_file_ref`, `truth_version_ref`, and `truth_fingerprint` against the current candidate scenario truth
5. current `repository_mapping_snapshot`
6. current `unit_snapshot`
7. current `system_constraints` binding fields
8. current `shared_contract_snapshot`

### 7.3 Allowed Entry Condition

`scenario_verify` may continue only when the pass gate still covers the current scenario candidate round exactly.

### 7.4 Smallest Fallback

If the handoff is invalid, the smallest fallback is `scenario_check`.
If current `repository_mapping_snapshot` no longer matches `docs/specs/repository_mapping.md`, use `binding_drift`.

### 7.5 Allowed Reason Codes

Use only:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_contract_drift`

---

## 8. Handoff: `scenario_verify -> scenario_promote`

### 8.1 Minimum Upstream Artifact

`scenario_promote` minimally requires a current valid `_verify_result/scenario/{scenario}.md`.

### 8.2 Required Re-Validation

Before consumption, `scenario_promote` must re-validate:

1. `decision=pass`
2. `allow_next=true`
3. `next_command=scenario_promote`
4. `truth_layer_ref`, `truth_file_ref`, `truth_version_ref`, and `truth_fingerprint` against the current candidate scenario truth
5. current `repository_mapping_snapshot`
6. current `unit_snapshot`
7. current `verification_scope_ref`
8. current `system_constraints` binding fields
9. current `shared_contract_snapshot`

### 8.3 Allowed Entry Condition

`scenario_promote` may continue only when the verify result still covers current candidate scenario truth, current repository mapping, current bound units, current bound Shared Contract files, current verification scope, and current formal global baseline state together.

### 8.4 Smallest Fallback

If verification evidence is outdated or incomplete but the check gate still covers current truth, the smallest fallback is `scenario_verify`.
If candidate scenario truth or upstream bindings drifted, the smallest fallback is `scenario_check`.
If current `repository_mapping_snapshot` no longer matches `docs/specs/repository_mapping.md`, the smallest fallback is `scenario_check` with `binding_drift`.

### 8.5 Allowed Reason Codes

Use only:

1. `truth_drift`
2. `binding_drift`
3. `baseline_drift`
4. `shared_contract_drift`
5. `evidence_incomplete`

---

## 9. Relationship To Other Files

This handoff contract works together with:

1. `specflow/framework/spec_policy.md`
2. `specflow/framework/command_policy.md`
3. `specflow/framework/commands/*.md`
4. process-file READMEs under `docs/specs/` and `specflow/templates/docs/specs/`
5. `specflow/framework/process_snapshot_contract.md`
6. `specflow/framework/recovery_policy.md`

Priority rules:

1. policy files define the top-level governance rules
2. this file defines the centralized candidate-chain handoff contract
3. command files define command-local procedure and output details consistent with this contract
4. process-file READMEs define file-specific consumption and invalidation semantics consistent with this contract

---

## 10. Non-Goals

This file does not:

1. define new commands
2. redefine unit behavior truth
3. replace command-specific stop conditions
4. expand process-file fixed snapshot fields by itself
