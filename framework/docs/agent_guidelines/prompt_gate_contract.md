# Prompt Gate Contract

## 1. Purpose

This file defines the centralized contract for Prompt-gated candidate closure.

It answers six questions:

1. when a module is considered Prompt-triggered
2. which command must run Prompt Adequacy Review
3. which review objects must be judged
4. which Prompt issues are blocking
5. what the minimum write-back contract is
6. how this contract relates to the detailed Prompt-writing guideline

This file does not replace `docs/prompt_guidelines.md`.
That file remains the detailed content-writing and review rule document.

---

## 2. Scope

This contract governs only the Prompt gate inside the candidate closure chain.

By default it governs:

1. when `cand_check` must run Prompt Adequacy Review
2. how `cand_check` decides pass versus `blocked` or `fix_required`
3. what prompt-related result fields mean in `_check_result/{module}.md`

It does not govern:

1. how to author a concrete Prompt block line by line
2. whether a business module should use an LLM at all
3. downstream candidate-side binding validation after `cand_check` already wrote the gate result

---

## 3. Trigger Conditions

A module is Prompt-triggered when its current candidate hits at least one of these:

1. it defines Prompt structure, Prompt assembly order, or Prompt blocks
2. it defines system prompt, role prompt, output prompt, system base, or an equivalent prompt layer
3. its correctness depends materially on the model understanding role, goal, state, terminology, or object relations
4. it injects runtime context, shared rules, state snapshots, tool inventory, or output protocol into model input

If none of those are hit, Prompt Adequacy Review is `n/a`.

If any of those are hit:

1. `cand_check` must execute Prompt Adequacy Review
2. `cand_check` must not skip the review merely because the candidate is otherwise complete

---

## 4. Review Objects

Prompt Adequacy Review uses these fixed review objects:

1. `foundational adequacy`
2. `structured output adequacy` when applicable
3. `ordering adequacy`

Their meanings are fixed:

1. `foundational adequacy`
   - role completeness
   - context sufficiency
   - concept clarity
   - execution-context completeness
   - logical closure
2. `structured output adequacy`
   - output protocol clarity
   - schema completeness
   - required few-shot examples when the structured output or boundary judgment is complex
3. `ordering adequacy`
   - whether KV-cache-friendly ordering still preserves semantic clarity and read order

Detailed review content stays in `docs/prompt_guidelines.md`.
This contract defines only the governance interface for using that review.

---

## 5. Blocking Rules

`cand_check` must conclude `blocked` or `fix_required` when any of the following hold:

1. the module is Prompt-triggered and the role, goal, or success condition is not stable enough to constrain implementation
2. the module is Prompt-triggered and the model still needs unwritten assumptions, hidden state, or unexplained project terms to succeed
3. the Prompt's goal, rules, constraints, and output protocol do not form a closed chain
4. structured output is required but the formal protocol, schema, or required example support is not sufficient
5. ordering breaks semantic clarity by referencing objects or terms before they are defined

Non-blocking rule:

1. ordering that is merely "not cache-friendly enough" but still semantically clear should be recorded as an improvement item, not a blocking gate defect

---

## 6. Write-Back Contract

When `cand_check` writes `_check_result/{module}.md`, the Prompt gate fields have these minimum meanings:

1. `prompt_adequacy_review_required`
   - `true` only when the module is Prompt-triggered
   - `false` only when the trigger conditions were not hit
2. `prompt_adequacy_decision`
   - `pass`
   - `blocked`
   - `fix_required`
   - `n/a` only when the review was not required
3. `prompt_adequacy_summary`
   - whether the Prompt gate was triggered
   - the conclusion for `foundational adequacy`
   - whether `structured output adequacy` applied, and its conclusion
   - the conclusion for `ordering adequacy`
   - the blocking items, or `none`

Additional rules:

1. `prompt_adequacy_summary` must be reviewable. Do not write an empty sentence such as "Prompt review passed."
2. If Prompt Adequacy Review is required and fails, `cand_check` must not keep or write a pass gate.

---

## 7. Command Mapping

### 7.1 `cand_check`

`cand_check` is the command responsible for:

1. deciding whether the Prompt gate is triggered
2. running Prompt Adequacy Review
3. merging the Prompt gate result into the candidate closure conclusion
4. writing the Prompt gate fields into `_check_result/{module}.md` when the overall candidate result is `pass`

### 7.2 Downstream Candidate Commands

`cand_plan`, `cand_impl`, `cand_verify`, and `cand_promote` do not rerun Prompt Adequacy Review by default.

They only consume the existing pass gate according to:

1. `specflow/framework/docs/agent_guidelines/candidate_handoff_contract.md`
2. the current bindings of `_check_result/{module}.md`

If the pass gate becomes invalid, the fallback is still `cand_check`.

---

## 8. Relationship To `docs/prompt_guidelines.md`

The boundary is fixed:

1. `docs/prompt_guidelines.md`
   - defines detailed Prompt writing rules and detailed review content
2. `specflow/framework/docs/agent_guidelines/prompt_gate_contract.md`
   - defines when that review becomes a lifecycle gate, how blocking is decided at governance level, and what the write-back contract is

Do not let these two files redefine each other's top-level responsibility.

---

## 9. Non-Goals

This contract does not:

1. define a module's business behavior truth
2. create a second lifecycle outside `cand_check`
3. force Prompt review on modules that are not Prompt-triggered
4. replace `docs/prompt_guidelines.md` as the detailed Prompt rule document
