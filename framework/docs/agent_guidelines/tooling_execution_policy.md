# Tooling Execution Policy

## 1. Purpose

This file defines what governance tooling is allowed to exist in this repository and what that tooling must never do.

It answers five questions:

1. what counts as governance tooling here
2. when one tooling function is justified
3. which kinds of actions tooling may execute
4. which kinds of semantic judgment tooling must not perform
5. how `spec_flow_review` must review the tooling layer

This file is a framework-governance rule.
It is not a command, not a module Spec, and not a project-local standard.

## 2. What Counts As Governance Tooling

In this repository, governance tooling means repo-tracked executable implementation whose job is to perform fixed governance actions for `specFlow`.

The definition depends on responsibility, not on file suffix.

Therefore:

1. Go source under `specflow/tooling/` that performs fixed governance actions is governance tooling
2. a later shell, Python, or other executable implementation would also count if it performs the same kind of fixed governance action
3. a Markdown explanation file is not tooling implementation, but it is still part of the tooling review surface when it defines or explains the tooling contract

Default review-target rule:

1. the default governance review target is source and rule-level material
2. compiled binaries under `specflow/tooling/bin/` are build artifacts, not the default source-of-truth review target
3. default `spec_flow_review` should review tooling source and tooling contract documents rather than platform binaries
4. the framework policy owns the tooling boundary rules
5. `specflow/tooling/README.md` owns the project-local tooling command surface and usage explanation
6. project-root `docs/` files must not be required as an additional tooling contract layer

## 3. Tooling Necessity Contract

A tooling function is justified only when all of the following hold:

1. the upstream input has already been fixed by governance rules, by an earlier human judgment, or by explicit caller parameters
2. the output can be produced mechanically from that fixed input without inventing a new behavior decision
3. the manual action is repetitive enough, error-prone enough, or omission-prone enough that tooling materially reduces governance risk
4. the function has an explicit upstream owner and explicit downstream consumer inside the governance flow
5. the function does not create a second semantic source of truth by duplicating judgment that should stay in rule documents or the runtime

If any item above is missing, the tooling function is not justified as governance tooling.

In plain words:

1. tooling is allowed for execution work
2. tooling is not allowed just because the work looks annoying
3. tooling is not allowed to exist without a clear place in the governance chain

## 4. Allowed Tooling Action Surface

Governance tooling may execute only fixed actions whose result is mechanically constrained by already-decided truth.

The allowed action families are:

1. collect
   - gather files, registry entries, bindings, or scoped targets from already-defined locations
2. parse
   - parse frontmatter, tables, flags, or fixed file structures
3. validate
   - validate shape, presence, supported values, and declared references against already-defined contracts
4. rebuild
   - rebuild snapshots or other deterministic derivatives from formal truth files
5. compare
   - compare rebuilt current state against stored snapshots or managed content
6. cleanup
   - delete or reset process artifacts when a command-defined cleanup rule already says that cleanup must happen
7. sync
   - align managed content or metadata when the source, target, and writeback contract are already explicit

Writeback rule:

1. tooling may write only to locations whose writeback contract is already defined by governance rules
2. tooling must not invent a new durable output container on its own
3. execution-local caller parameters may narrow scope, but they must not redefine the governance meaning of the action

## 5. Forbidden Semantic Judgment

Governance tooling must not perform semantic judgment that belongs to rule documents, governance review, or runtime reasoning.

At minimum, tooling must not decide:

1. whether a user request should route to one governance flow or another from natural-language intent alone
2. whether a boundary is module-local truth, Shared Contract truth, or global default-rule truth
3. whether candidate truth is sufficiently closed, complete, or progressable
4. whether verification evidence is sufficient
5. whether a finding is `pass`, `blocked`, or which severity it should have
6. whether downgrade or checkpoint handling is required
7. whether a shared change is only thematically similar or is truly the same shared truth object
8. whether a tooling function itself is justified under Section 3

Additional rule:

1. ordinary branching, parsing guards, and shape checks inside code do not become forbidden merely because they use `if`
2. the forbidden case is semantic decision-making that substitutes for governance judgment

## 6. Relationship To `spec_flow_review`

Default `spec_flow_review` must review the tooling layer when the repository includes governance tooling.

That review must cover at least:

1. whether each current tooling function satisfies the necessity contract from Section 3
2. whether each current tooling function stays inside the allowed action surface from Section 4
3. whether any tooling path performs forbidden semantic judgment from Section 5
4. whether tooling rule documents, tooling explanation documents, and tooling source still describe the same contract

The required tooling-contract document set is:

1. this policy file for framework-level boundary rules
2. `specflow/tooling/README.md` for the concrete command surface, build flow, recovery flow, and usage examples
3. the in-scope tooling source files under `specflow/tooling/cmd/` and `specflow/tooling/internal/`

Default `spec_flow_review` must not issue `pass` when any of the following is true:

1. a tooling function is present but does not satisfy the necessity contract
2. tooling performs forbidden semantic judgment
3. tooling source and tooling-governing documents disagree about what the tooling is responsible for
4. the review output did not explicitly report tooling coverage and result

## 7. Compiled Tooling Freshness

When governance tooling is executed through compiled binaries under `specflow/tooling/bin/`, the repository must prevent stale binaries from continuing to execute governance actions.

Required rules:

1. the freshness check target is the current tooling source input set, not filesystem timestamps or other environment metadata
2. the current tooling source input set must include the files that change current binary behavior:
   - `specflow/tooling/cmd/**/*.go`
   - `specflow/tooling/internal/**/*.go`
   - `specflow/tooling/go.mod`
   - `specflow/tooling/go.sum` when it exists
3. `build-release` must embed one build-time fingerprint derived from that source input set into the produced binaries
4. a compiled tooling binary must compare its embedded fingerprint against the current live source fingerprint before executing ordinary governance actions
5. when the fingerprints differ, the binary must stop and require a rebuild instead of continuing
6. the bypass surface for that freshness gate must stay minimal and cover only recovery or inspection entry points needed to rebuild or diagnose the binary state
7. `doctor` must report stale current-platform binaries as failures rather than treating binary presence alone as sufficient

In plain words:

1. "binary exists" is not enough
2. "binary was rebuilt from the current source" is the required state
3. stale binaries must fail closed rather than continuing silently

## 8. Non-Goals

This file does not:

1. require every governance action to become tooling
2. require a separate tooling registry file
3. let tooling replace module Specs, shared-governance files, or review judgment
4. treat platform binaries as the default governance review target
