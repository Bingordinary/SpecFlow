# Severity Policy

## 1. Purpose

This file defines the centralized severity levels used by Spec Flow findings and deviation reports.

It answers four questions:

1. what each severity level means
2. which review or verification flows use the same scale
3. how severity relates to blocking status
4. what a report must explain when it assigns a severity

This is a shared governance contract.
Executors must not invent a different severity meaning per command.

---

## 2. Scope

This policy applies when a Spec Flow review or verification output needs to grade a real problem.

By default it governs:

1. `spec_flow_review`
2. `spec_flow_design_review`
3. `cand_check`
4. `cand_verify`
5. `stable_verify`

It may also be reused by other governance flows if those flows explicitly say so.

It does not define:

1. `fallback_reason_code`
2. lifecycle progression
3. whether a command may continue after binding validation

---

## 3. Core Principle

Severity answers only one question:

1. how harmful the confirmed problem is to flow correctness, behavior stability, or safe downstream work

Severity does not answer:

1. which fallback step is required
2. whether the issue came from truth drift, implementation drift, or evidence incompleteness
3. whether a user should prefer one product choice over another

Blocking status must still be stated explicitly.
Do not assume that severity alone fully determines the next action.

---

## 4. Severity Levels

### 4.1 `P0`

Use `P0` for:

1. main-chain break
2. truth conflict
3. key gate distortion
4. governance ambiguity that can make executors run the wrong flow or skip a required gate

Plain meaning:

1. the flow is not safely controllable until this is repaired

### 4.2 `P1`

Use `P1` for:

1. behavior or implementation meaning that is unstable enough to block safe downstream planning, implementation, verification, or promotion
2. verification deviations that already threaten the current round's externally meaningful result

Plain meaning:

1. the flow structure still exists
2. but the current round must not continue past the affected gate

### 4.3 `P2`

Use `P2` for:

1. issues that do not block the current next gate by themselves
2. but materially harm review stability, readability, maintainability, or future closure

Plain meaning:

1. downstream work may still continue if no higher-severity blocker exists
2. but the repository is accumulating governance or verification debt

### 4.4 `P3`

Use `P3` for:

1. minor elaboration or clarity issues
2. low-impact reporting gaps

Plain meaning:

1. the issue is real
2. but it does not materially change current flow control or review safety

---

## 5. Blocking Relationship

Severity and blocking are related but not identical.

Rules:

1. `P0` is normally blocking.
2. `P1` is normally blocking for the affected downstream step.
3. `P2` is normally non-blocking unless a command-specific rule says otherwise.
4. `P3` is normally non-blocking.
5. reports must still state blocking status explicitly instead of making the reader infer it.

---

## 6. Required Explanation Fields

When a governed flow assigns a severity to a real problem, the report should explain:

1. background
2. what happened
3. impact
4. recommended fix
5. why that fix is the minimal correct fix
6. whether the issue is blocking

Commands or flows may add more required fields, but must not weaken this baseline.

---

## 7. Relationship To Other Files

This policy works together with:

1. `specflow/framework/docs/agent_guidelines/spec_flow_review.md`
2. `specflow/framework/docs/agent_guidelines/spec_flow_design_review.md`
3. `specflow/framework/docs/agent_guidelines/commands/cand_check.md`
4. `specflow/framework/docs/agent_guidelines/commands/cand_verify.md`
5. `specflow/framework/docs/agent_guidelines/commands/stable_verify.md`

Priority rules:

1. the active review or command file decides whether grading is required
2. this file defines the shared meaning of `P0 / P1 / P2 / P3`
3. command-local or flow-local text may add required report fields, but must not redefine the shared severity meaning

---

## 8. Non-Goals

This file does not:

1. define behavior truth
2. define verification evidence formats
3. replace `fallback_reason_code`
4. decide git close-out policy
