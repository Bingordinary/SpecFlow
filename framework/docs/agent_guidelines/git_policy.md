# Git Submission Flow And Versioning Policy

## 1. Basic Principles

1. Changes to `stable` represent formal contract changes.
2. Changes to `candidate` represent candidate progression and do not mean the behavior is formally active yet.
3. Therefore, candidate-progress commits and promotion commits must be defined separately.
4. Under `docs/specs/`, every Spec file except `candidate` main files and their appendix files is a behavior source of truth and should normally enter git history in the current task.
5. `candidate` main files and their appendix files are draft-layer artifacts, but draft-layer status does not forbid commits. When a round reaches a reviewable checkpoint, the current `candidate` should normally enter git history together with any linked process or code changes for that checkpoint.
6. `specflow/framework/docs/agent_guidelines/*.md` and `specflow/framework/docs/agent_guidelines/commands/*.md` are part of repository governance and should normally be committed in the current task.
7. Changes to registered entry index files are also governance changes and should normally be committed in the current task after entry-file sync is complete.
8. When `Active Layer=stable` and code changes introduce new formal-layer implementation drift, the unit's `Next Command` should normally fall back to `unit_stable_verify`.
9. `docs/specs/system_constraints/stable/s_system_constraints.md` is treated by default as a formal side product of unit `unit_promote`.
10. `docs/specs/shared_contracts/candidate/*.md` are draft-layer shared truth files and follow candidate-layer commit rules by default.

---

## 2. Candidate Progress Commits

Applicable cases:

1. the first implementation round for a brand-new unit
2. a candidate upgrade round for an existing unit

Rules:

1. `feat:` may be used for linked changes across `candidate + code + plan`.
2. These commits do not require `stable` to change at the same time.
3. But the target behavior of the commit must be traceable back to the `candidate`.
4. If the change only brings code back to the currently aligned layer, `fix:` may be used.
5. If the change is only structural and does not alter the behavior defined by the current aligned layer, `refactor:` may be used.
6. Candidate-progress commits should be created at reviewable checkpoints rather than for every incomplete draft save.
7. Default reviewable checkpoints include a candidate state ready for `unit_check`, a completed `unit_plan`, a coherent `unit_impl` slice that aligns code to the current candidate, and a passed `unit_verify`.
8. A candidate-progress commit may contain only draft-layer files when that checkpoint itself is the thing being reviewed.
9. A candidate-progress commit must stay separate in meaning from the later promotion commit that makes behavior formally active through `stable`.

---

## 3. Promotion Commits

Applicable case:

1. executing `unit_promote:{unit}`

Rules:

1. The commit must update or create the corresponding `stable`.
2. The commit must delete the round's `docs/specs/units/candidate/c_unit_{unit}.md` and that unit's round-specific candidate appendix files under `docs/specs/units/candidate/appendix/` or an equivalent dedicated subdirectory. If the round also handled Shared Contract files, it must also resolve the corresponding `docs/specs/shared_contracts/candidate/*.md` or `docs/specs/shared_contracts/stable/*.md`.
3. If `_check_result/{unit}.md`, `_verify_result/{unit}.md`, `_plans/draft/{unit}.md`, or `_plans/active/{unit}.md` exist for the round, they must be deleted in the same commit.
4. If the unit candidate contains a closed `system_constraints_change_proposal` that is promoted in the same round, the same commit must also update `docs/specs/system_constraints/stable/s_system_constraints.md`.

---

## 4. Semantic Version Rules

Versions use `MAJOR.MINOR.PATCH`.

### 4.1 Unit `stable`

1. `MAJOR`
   - incompatible formal contract change
2. `MINOR`
   - new capability or compatible behavior change in the formal contract
3. `PATCH`
   - implementation-only fix or alignment-only fix against the current aligned layer

### 4.2 `s_system_constraints.md`

1. `MAJOR`
   - incompatible global constraint change
2. `MINOR`
   - new global default rule, shared mechanism, or compatible extension
3. `PATCH`
   - wording-only clarification that does not change the meaning of formal constraints

### 4.3 Shared Contract

1. `shared_version` uses `MAJOR.MINOR.PATCH`
2. the first candidate-layer file for a brand-new shared object should normally start at `0.1.0`
3. when a current round opens the next candidate-layer file for a shared object that already has a stable-layer file, that candidate file must already carry the intended next stable `shared_version`
4. when Rule 3 applies, that candidate file must also record the exact `promotion_owner_unit` required by `specflow/framework/docs/agent_guidelines/spec_policy.md`
5. `MAJOR`
   - incompatible change to the formally bound shared semantics, required consumer interpretation, or cross-unit contract shape
6. `MINOR`
   - compatible shared-truth extension, additional reusable capability, or compatible topology evolution that requires consumer awareness but not contract breakage
7. `PATCH`
   - wording-only clarification, compatible tightening, or alignment-only update that does not change the required consumer interpretation

Notes:

1. `candidate` content may change frequently.
2. It enters formal version semantics only when promoted into a new `stable`.

---

## 5. Promotion Commit Closure Scope

Rules:

1. The default closure scope of `unit_promote` includes only the round's unit `stable`, any linked update to `s_system_constraints.md`, any Shared Contract handled in the round, and cleanup of the round's candidate main file, candidate appendix files, and candidate-side process files.
2. Promotion does not by itself force a Shared Contract to be absorbed into `s_system_constraints.md` or unit `stable`.
3. A Shared Contract may remain an independent stable shared truth after promotion.
4. This repository does not currently require maintaining a root `VERSION` file during `unit_promote`.
5. This repository does not currently require creating a Git tag during `unit_promote`.

---

## 6. Documentation Changes And Commit Rules

### 6.1 `docs/specs/*.md`

If the task changes only `docs/specs/*.md`:

1. If it changes `docs/specs/units/candidate/c_unit_{unit}.md`, candidate appendix files under `docs/specs/units/candidate/appendix/` or an equivalent dedicated subdirectory, or `docs/specs/shared_contracts/candidate/*.md`, commit when the round has reached a reviewable checkpoint. Purely temporary incomplete draft saves do not require their own commit.
2. If it changes `docs/specs/units/stable/*.md`, stable appendix files under `docs/specs/units/stable/appendix/*.md` or an equivalent dedicated subdirectory, `docs/specs/shared_contracts/stable/*.md`, `docs/specs/system_constraints/stable/*.md`, `docs/specs/_status.md`, `docs/specs/_check_result/*.md`, `docs/specs/_verify_result/*.md`, or `docs/specs/_plans/**/*.md`, it should normally be committed in the current task.
3. `docs/specs/_plans/draft/*.md` are planning working artifacts; they may be committed together with a reviewable checkpoint, but a blocked planning round does not require a standalone commit solely to preserve draft accumulation.
4. If `stable` changes, treat it as a formal contract change. If the task hits `unit_promote`, follow the promotion-commit rules.
5. If a `candidate` change belongs to the same command flow as the corresponding code implementation, plan file, check result, verify result, or promotion commit, commit the checkpoint as one traceable unit instead of leaving candidate-only drift in the worktree.

### 6.2 `specflow/framework/docs/agent_guidelines/*.md` And `specflow/framework/docs/agent_guidelines/commands/*.md`

If the task changes only `specflow/framework/docs/agent_guidelines/*.md` or `specflow/framework/docs/agent_guidelines/commands/*.md`:

1. those changes should normally enter git history because they are part of repository governance
2. they should normally be committed in the current task instead of being batched for later

### 6.3 Registered Entry Index Files

If the task changes a registered entry index file listed in `specflow/framework/docs/agent_guidelines/entry_index_registry.md`, such as `AGENTS.md`, `GEMINI.md`, or `CLAUDE.md`:

1. the change should normally enter git history because it directly affects command listing, match explanation, or governance-flow routing
2. it should normally be committed in the current task
3. entry-file sync must be completed before commit; that sync aligns only the managed block defined in `specflow/framework/docs/agent_guidelines/entry_index_registry.md`
4. if multiple registered entry files were modified and their managed blocks still differ, an explicit sync source must be chosen before continuing
   - use `specflow/tooling/bin/specflowctl-<os>-<arch> entry sync --source <registered-entry-file>` before retrying the commit

### 6.4 Tooling Binary Freshness

If the task changes current-binary tooling inputs under:

1. `specflow/tooling/cmd/**/*.go`
2. `specflow/tooling/internal/**/*.go`
3. `specflow/tooling/go.mod`
4. `specflow/tooling/go.sum`

and the repository tracks compiled tooling binaries under `specflow/tooling/bin/`:

1. from the repository root, run `cd specflow/tooling` and then `go run ./cmd/specflowctl build-release --repo-root ../..` in the current task
2. include the refreshed tracked binaries in the same checkpoint or commit rather than leaving source/binary drift in the worktree
3. do not treat binary presence alone as proof that the binaries are current; the required state is that the binaries were rebuilt from the current tooling input set
