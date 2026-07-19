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
It is not a command, not a unit Spec, and not a project-level agent instruction.

## 2. What Counts As Governance Tooling

In this repository, governance tooling means repo-tracked executable implementation whose job is to perform fixed governance actions for `specFlow`.

The definition depends on responsibility, not on file suffix.

Path root rule:

1. in `installed_project`, `<tooling-root>` is `specflow/tooling/`
2. in `source_repo`, `<tooling-root>` is `tooling/`
3. installed-project user examples may use `specflow/tooling/...`, but review and migration contracts must use `<tooling-root>/...`

Therefore:

1. Go source under `<tooling-root>/` that performs fixed governance actions is governance tooling
2. a later shell, Python, or other executable implementation would also count if it performs the same kind of fixed governance action
3. a Markdown explanation file is not tooling implementation, but it is still part of the tooling review surface when it defines or explains the tooling contract

Default review-target rule:

1. the default governance review target is source and rule-level material
2. compiled binaries under `<tooling-root>/bin/` are local build or release artifacts, not the default source-of-truth review target
3. default `spec_flow_review` should review tooling source and tooling contract documents rather than platform binaries
4. the framework policy owns the tooling boundary rules
5. `<tooling-root>/README.md` owns the project-local tooling command surface and usage explanation
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
7. preflight
    - verify command entry facts that are already mechanically determined
8. transition
   - close a standard command by applying a fixed transition table to an explicit caller-provided command outcome
    - validate only mechanical prerequisites such as supported flag combinations and required process snapshot files
    - execute process cleanup only when the transition table already defines that action
9. sync
   - align managed content or metadata when the source, target, and writeback contract are already explicit
10. render
    - expose a read-only local view derived from already-written truth files without creating, editing, or promoting truth
11. work-state maintenance
   - create, validate, refresh, or touch review slice work-state carriers when the adopting owner defines the exact path, fields, statuses, and stale rules
    - maintain only mechanical data such as timestamps, skeleton rows, input fingerprints, and stale marks
12. relation calculation
   - compute candidate readiness, candidate blockers, candidate cycles, and reference-only edges from explicit already-written references
   - read only declared truth and support-surface files
   - write no project files and create no durable process artifact
Writeback rule:

1. tooling may write only to locations whose writeback contract is already defined by governance rules
2. tooling must not invent a new durable output container on its own
3. execution-local caller parameters may narrow scope, but they must not redefine the governance meaning of the action

## 5. Forbidden Semantic Judgment

Governance tooling must not perform semantic judgment that belongs to rule documents, governance review, or runtime reasoning.

At minimum, tooling must not decide:

1. whether a user request should route to one governance flow or another from natural-language intent alone
2. whether a boundary is unit-local truth, Rule truth, or global default-rule truth
3. whether candidate truth is sufficiently closed, complete, or progressable
4. whether verification evidence is sufficient
5. whether a finding is `pass`, `blocked`, or which severity it should have
6. whether downgrade or checkpoint handling is required
7. whether a rule change is only thematically similar or is truly the same rule truth object
8. whether a tooling function itself is justified under Section 3

Additional rule:

1. ordinary branching, parsing guards, and shape checks inside code do not become forbidden merely because they use `if`
2. the forbidden case is semantic decision-making that substitutes for governance judgment
3. command preflight tooling may report whether the current status row and required process snapshots mechanically allow a command to continue, but it must not decide whether candidate truth is complete, whether evidence is sufficient, whether downgrade is allowed, or whether a promotion should happen
4. promote tooling must not choose semantic outcome values, repair contradictory values, or infer a judgment from repository content. Mechanically transforming the `layer` frontmatter field (from `candidate` to `stable`) when copying from the candidate directory to the stable directory is not a semantic judgment — it is a defined transition rule determined by the fixed source and target directories.
5. promote tooling may reject an unsupported state combination and must apply the defined transition rules
6. slice work-state tooling may mark stale slices from fingerprint changes only when the adopting owner defines that mechanical action, but it must not mark semantic slices as passed, write finding content, choose severity, decide review scores, decide verification sufficiency, or decide a final command or review result
7. relation calculation tooling may report explicit candidate references, ready candidates, blocked candidates, and cycles, but it must not infer dependencies from prose, judge candidate content quality, or repair references

## 6. Relationship To `spec_flow_review`

Default `spec_flow_review` must review the tooling layer when the repository includes governance tooling.

That review must cover at least:

1. whether each current tooling function satisfies the necessity contract from Section 3
2. whether each current tooling function stays inside the allowed action surface from Section 4
3. whether any tooling path performs forbidden semantic judgment from Section 5
4. whether tooling rule documents, tooling explanation documents, and tooling source still describe the same contract

The required tooling-contract document set is:

1. this policy file for framework-level boundary rules
2. `<tooling-root>/README.md` for the concrete command surface, build flow, recovery flow, and usage examples
3. the current tooling source input files:
   - `<tooling-root>/cmd/**/*.go`
   - `<tooling-root>/internal/**/*.go`
   - `<tooling-root>/go.mod`
   - `<tooling-root>/manifest.tsv`
   - `<tooling-root>/go.sum` when it exists
4. the tooling helper script files:
   - all regular files under `<tooling-root>/scripts/**`
Default `spec_flow_review` must not issue `pass` when any of the following is true:

1. a tooling function is present but does not satisfy the necessity contract
2. tooling performs forbidden semantic judgment
3. tooling source and tooling-governing documents disagree about what the tooling is responsible for

## 7. Compiled Tooling Freshness

When governance tooling is executed through compiled binaries under `<tooling-root>/bin/`, the repository must prevent stale binaries from continuing to execute governance actions.
That directory is a git-ignored local cache.
Official platform binaries are produced from tagged source by the Release workflow and distributed as GitHub Release assets.

Required rules:

1. the freshness check target is the current tooling source input set, not filesystem timestamps or other environment metadata
2. the current tooling source input set must include the files that change current binary behavior:
   - `<tooling-root>/cmd/**/*.go`
   - `<tooling-root>/internal/**/*.go`
   - `<tooling-root>/go.mod`
   - `<tooling-root>/manifest.tsv`
   - `<tooling-root>/go.sum` when it exists
3. `build-release` must embed one build-time fingerprint derived from that source input set into the produced binaries
4. a compiled tooling binary must compare its embedded fingerprint against the current live source fingerprint before executing ordinary governance actions
5. when the fingerprints differ, the binary must stop and require a rebuild instead of continuing
6. the bypass surface for that freshness gate must stay minimal and cover only recovery or inspection entry points needed to rebuild or diagnose the binary state, plus read-only render actions (`next` — the deterministic directive for the current governance step, which is a read-only render action that does not modify project files or advance state)
7. `doctor` must report stale current-platform binaries as failures rather than treating binary presence alone as sufficient
8. compiled binaries must not be committed to git

The tooling freshness fingerprint is separate from process snapshot fingerprints and review input fingerprints.
It proves that a compiled tool matches the current tooling source input set.
It must not be used as evidence that a unit, Rule, process file, or review slice is fresh.

Tooling helper scripts under `<tooling-root>/scripts/` are default review inputs because they rebuild or select binaries for the installed tooling source.
They are not binary freshness inputs unless they change compiled binary behavior.

In plain words:

1. "binary exists" is not enough
2. "binary was rebuilt from the current source" is the required state
3. stale binaries must fail closed rather than continuing silently
4. release assets, not git history, carry official compiled binaries

## 8. Non-Goals

This file does not:

1. require every governance action to become tooling
2. require a separate tooling registry file
3. let tooling replace unit Specs, rule-governance files, or review judgment
4. treat platform binaries as the default governance review target
