# SpecFlow Bootstrap

SpecFlow records accepted design, behavior, boundaries, and shared rules. Specs are the consensus protocol between the user and the agent: users confirm intent; agents carry it across sessions. Identify the trigger, resolve mode/target, then read its route. If unclear, stop and ask.

Normal user-triggered paths:

- unit: **fork/create → edit → validate → verify → promote**
- rule: **fork/create → edit → validate → promote**
- removal: **agent decision → structured-reference check → transactional deletion**

## State Model

**File existence is state.** Neither file: create the candidate. Stable only: fork before editing. Both: edit candidate; leave stable unchanged. Candidate only: keep editing it. Promote only after applicable gates pass and the user confirms.

Units (one independently governed engineering responsibility) live under `docs/specs/units/{stable,candidate}/`; rules (reusable shared constraints) under `docs/specs/rules/{stable,candidate}/`. **Stable** is accepted truth. **Candidate** is proposed truth and the normal editable layer. Stable changes use promote except authorized spec removal (`framework/removal_workflow.md`) and exact `spec_flow_update` migration. Caches: `docs/specs/meta/validation/` (`framework/validation_cache.md`).

Code is observed behavior, candidate is proposed intent, stable is prior agreement. None wins automatically. During `verify`, present divergence and let the user decide.

Outside `verify`: stable only = recorded truth; both = name the layer, never just "the spec"; candidate only = unverified draft; neither = use code read-only.

## Default Editing Workflow

Read-only requests remain read-only. Without a gate trigger, continue the requested discussion/editing without suggesting spec operations or exposing cache state. "Assume editing" selects a workflow, not write permission (`framework/agent_suggestion_rules.md`).

For a unit, run `specflowctl next --unit <name>`. For a rule, inspect exact stable and candidate paths.

Before editing:

- Candidate exists: edit it.
- Stable only: run `specflowctl fork --unit <name>` or `specflowctl fork --rule <id>`. Never use `cp`. Unit fork copies appendices and usable confirmation caches (`framework/validation_cache.md`).
- Neither exists: create the candidate using `framework/spec_writing_guide.md`; new rules follow its §6.

Plans declare candidate spec and appendices before code; stable-only requires fork first. A pure internal refactor/performance fix changing no contract, state, rule, or acceptance criterion needs a one-sentence Spec Impact Assessment. During execution, update candidate before code/tests.

Do not gate before editing. Read, then write. For "only change X" / "do not touch Y", open an operation before editing and complete its check before reporting done (`framework/operations/operation_scope.md`).

## Trigger Routing

Resolve mode, then read the target row and its required packages.

| Trigger | First action and required packages |
|---|---|
| `validate@{target}` | Resolve `framework/commands.md`; full run. Read `framework/verification_scope.md` and `framework/unit_validate_checklist.md` or `framework/rule_validate_checklist.md`. |
| `validate@{target}:check-{n}` / `validate@{target}:{keyword}` | Resolve with `framework/commands.md`; run targeted check directly; read `framework/verification_scope.md` and `framework/unit_validate_checklist.md` or `framework/rule_validate_checklist.md`. |
| `verify@{unit}` | Resolve candidate/stable; discover paths, then full run with `--input`. Read `framework/verification_scope.md`, `framework/unit_verify_checklist.md` and `framework/shared_judgments.md`. |
| `verify@{unit}:{keyword}` | Run targeted check directly; read `framework/verification_scope.md`, `framework/unit_verify_checklist.md` and `framework/shared_judgments.md`. |
| `verify@{rule}` | Stop: rule verify was removed; report `validate@{rule}`. Read `framework/verification_scope.md`. |
| `revalidate@{target}` | Resolve `framework/commands.md`; delta/repair. Read `framework/verification_scope.md` and `framework/unit_validate_checklist.md` or `framework/rule_validate_checklist.md`. |
| `reverify@{unit}` | Discover paths for all items, including carried; delta/repair with `--input`. Read `framework/verification_scope.md`, `framework/unit_verify_checklist.md` and `framework/shared_judgments.md`. |
| `promote@{target}` | Resolve with `framework/commands.md`; confirm intent; read `framework/unit_promote_workflow.md` or `framework/rule_promote_workflow.md`; check applicable gates only. |
| `fresh@{target}` / `fresh@candidate` / `fresh@stable` / `fresh@all` | For `{target}`, resolve via `framework/commands.md`; run read-only `specflowctl fresh`; use `framework/validation_cache.md`. |
| `remove@{unit}` / `remove@{rule}` / `remove@{unit}:appendix:{filename.md}` | Read `framework/removal_workflow.md`; decide the basis within authorized scope, resolve exact targets, then run `specflowctl remove`. |
| `deps@all` / `deps@{unit}` / `deps@{rule}` | Run read-only `specflowctl deps`; use `framework/verification_scope.md`. |
| `spec_flow_update` | Follow `framework/operations/update.md`; only its migration step may edit stable structure. |
| `spec_flow_version` | Follow `framework/operations/version.md`. |
| Design, quality-check, code-review, or completion signal | Read `framework/agent_suggestion_rules.md` before responding. |
| Adoption intent ("把这几个模块登记一下" / "继续建档") | Follow `framework/operations/adopt.md`. |
| Bounded-task wording | Follow `framework/operations/operation_scope.md` before editing. |
| "stuck" / "something is wrong" | Diagnose with `framework/recovery_patterns.md`. |

Project entry instructions, not this router, own meta-governance commands.

## HARD RULES

**1. Read specs before discussing/changing a unit or rule.** Read its stable and candidate files and name the quoted layer. If neither exists for a read-only request, say so and use code. If the spec does not cover the topic, say so before new work. Create or update the candidate spec only for requested design changes. Plans declare candidate first or state why spec impact is absent.

**2. Gates are user-triggered; promote checks applicable gates.** Never promote without confirmation. Unit: validate+verify (the verify gate carries the alignment and quality lenses). Rule: validate. Never start/repeat any gate without its trigger. P0/P1 blocks promote (`framework/agent_suggestion_rules.md`, `framework/validation_cache.md`).

**3. Gates do not edit truth.** `validate`/`verify` write cache only. Normal stable changes use promote. Authorized spec removal (`framework/removal_workflow.md`) and `spec_flow_update` own their documented exceptions. `next`, `deps`, `doctor`, and `init` are not gates.

**3a. Never resolve divergence yourself.** Follow `framework/unit_verify_checklist.md` Step 7, present the analysis, and wait for the user's decision. Do not silently choose code or spec.

**4. Stop and reassess.** Ask when target, mode, package, permission, or next step is unclear. If execution reveals possible impact on recorded behavior, ownership, rules, acceptance, or implementation permission, pause affected implementation mutations; read-only diagnosis may continue. Read affected units/rules and reassess the request, authorization, and original no-impact basis. Shared files alone do not establish impact; reading or creating a candidate does not expand authorization. Resume through the existing route only when the recorded basis and authorization are clear; ask for missing decisions and apply Rule 3a to divergence. Never invent a substitute audit or reconciliation flow.

**5. Fork only through specflowctl.** Use `specflowctl fork --unit <name>` / `--rule <id>`; never manual `cp`.

## Infrastructure

==ATOM_BEGIN:specflowctl_location==
specflowctl is not on PATH. Run it as `<tooling-root>/bin/specflowctl` (for example `<tooling-root>/bin/specflowctl doctor`). `<tooling-root>` is `specflow/tooling`. The launcher resolves the platform binary for the current machine automatically; on Windows the shell resolves the name to `specflowctl.cmd` in the same directory.
==ATOM_END:specflowctl_location==

## Framework Path

==ATOM_BEGIN:framework_path==
Framework documentation files are referenced with the `framework/` prefix (e.g. `framework/operations/update.md`). These files are located at `specflow/framework/`.
==ATOM_END:framework_path==

Bootstrap identity (`framework/hooks.md`): installed framework commit plus `tooling/fingerprint.txt`; `spec_flow_version` compares remote.
