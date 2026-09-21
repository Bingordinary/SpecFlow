# Commands Reference

This file is the on-demand command surface reference. The session bootstrap (`framework/concepts.md`) owns trigger routing (trigger → first action → command package files); this file documents what each command does and who calls it, plus target resolution. Phase execution protocols are owned by the command package files the bootstrap names (checklists, workflows, operation procedures).

## Target Resolution

`validate`, `revalidate`, and `promote` accept either a unit or a rule. `verify`, `reverify`, `review`, and `rereview` are unit-only; a rule uses validate as its sole quality gate. Resolve `validate`, `revalidate`, and `promote` with this three-stage process. Resolve `fresh@{target}` with the separate existing-target process below.

**Stage 1 — Physical file check.** Must complete both steps before deciding.

Step 1: Glob for unit candidate files — run `Glob "docs/specs/units/candidate/unit_{target}.md"`.

Step 2: Glob for rule candidate files — run `Glob "docs/specs/rules/candidate/g_rule_{target}.md"` and `Glob "docs/specs/rules/candidate/b_rule_{target}.md"` (both `g_rule_` and `b_rule_` prefixes).

**Decision (use after both steps complete):**

| Unit candidate found | Rule candidate found | Result |
|:---:|:---:|---|
| Yes | No | Type = Unit |
| No | Yes | Type = Rule. Resolve full `rule_id` from matched filename (e.g., matched `b_rule_runtime_model.md` → `rule_id = b_rule_runtime_model`). |
| Yes | Yes | Ambiguous. List found files and ask user to clarify. |
| No | No | → Stage 2 |

**Stage 2 — Prefix fallback (new target).** Only reach here if neither unit nor rule files were found in Stage 1:

| Target format | Detected as | Example |
|---------------|-------------|---------|
| Name without prefix (`auth`, `user_service`) | Unit | `validate@auth` |
| Name with `g_rule_` or `b_rule_` prefix | Rule | `validate@b_rule_auth` |

**Stage 3 — Rule directory fallback.** Only reach here when Stage 2 detected "Unit" but no unit files exist for the target.

When Stage 2 classifies a no-prefix target as Unit, the agent looks for its unit files (`Glob "docs/specs/units/candidate/unit_{target}.md"` and `Glob "docs/specs/units/stable/unit_{target}.md"`). If **neither** candidate nor stable unit files exist, do not report "does not exist" yet. Instead:

Run `Glob "docs/specs/rules/candidate/*.md"` and `Glob "docs/specs/rules/stable/*.md"`. Scan all filenames for one whose name contains the target (e.g., `b_rule_runtime_model.md` contains `runtime_model`).

| Result | Action |
|---|---|
| Matching rule file found | Type = Rule. Resolve full `rule_id` from the matched filename (e.g., `b_rule_runtime_model.md` → `rule_id = b_rule_runtime_model`). Proceed with the rule pipeline. |
| No matching rule file found | Report: target does not exist. |

### Existing-Target Resolution for `fresh@{target}`

`fresh` reports an existing object; it never classifies or creates a new target. Before invoking the CLI, inspect both candidate and stable layers for every exact possibility:

- Unit: `docs/specs/units/{candidate,stable}/unit_{target}.md`.
- Rule when `{target}` starts with `g_rule_` or `b_rule_`: `docs/specs/rules/{candidate,stable}/{target}.md`.
- Rule when `{target}` has no rule prefix: both `docs/specs/rules/{candidate,stable}/g_rule_{target}.md` and `docs/specs/rules/{candidate,stable}/b_rule_{target}.md`.

Treat the same unit or full rule ID found in both layers as one logical match. Then act on the complete result:

| Matches | Action |
|---|---|
| One unit, no rule | Run `specflowctl fresh --unit {target}`. |
| No unit, one full rule ID | Run `specflowctl fresh --rule {rule_id}`. |
| A unit and any rule, or multiple full rule IDs | Ambiguous. List every matching path and ask the user to choose. |
| None | Report that the target does not exist. |

Never run bare `specflowctl fresh` for `fresh@{target}`. Without `--unit` or `--rule`, the CLI produces a scope summary rather than target detail.

The applicable path depends on the target:

| Step | Unit | Rule | Retiring unit |
|---|---|---|---|
| validate | 8-point unit design checklist (`unit_validate_checklist.md`) | 8-point rule checklist (`rule_validate_checklist.md`) | Retirement validation in the unit checklist |
| verify | Required: 7-step spec-vs-code check (`unit_verify_checklist.md`) | Not applicable; rule verify was removed | Not applicable |
| review | Required: spec-aware code review (`spec_review_checklist.md`) | Not applicable | Not applicable |
| promote | candidate→stable archive (`unit_promote_workflow.md`) | version promotion + consumer migration (`rule_promote_workflow.md`) | remove retired unit truth (`unit_promote_workflow.md`) |

## Command Reference

| Command | What it does | Who calls it |
|---------|-------------|-------------|
| `specflowctl fork --unit <name>` | Copy stable unit spec + appendices to candidate layer (pure copy, layer encoded by path) with version bump, and inherit pass stable confirmation caches into the candidate round (rewritten to the candidate layer; gates without a usable baseline are listed in the manifest). Rejects if candidate already exists or stable does not exist. | Agent (as fork prerequisite) |
| `specflowctl fork --rule <id>` | Copy stable rule to candidate layer (pure copy, layer encoded by path) with version bump. Rejects if candidate already exists or stable does not exist. | Agent (as fork prerequisite) |
| `specflowctl next --unit <name>` | Discover unit files and dependencies. When neither a candidate nor a stable spec exists for the unit, reports the empty state (no design recorded) with exit code 0; fails on tool errors. | Agent |
| `specflowctl promote --unit <name>` | Checks validate+verify+review+appendix cache freshness, validates format + copies candidate→stable, then rewrites the candidate gate caches into stable confirmation caches (`target: stable`, paths rewritten to `stable/`; a retired promote deletes them). Rejects if any cache stale, missing, or blocking. Also rejects if any non-exempt appendix file is missing from the validate cache. | Agent (after user confirmation, after validate+verify+review) |
| `specflowctl promote --rule <id>` | Checks rule validate cache freshness, validates rule frontmatter, copies candidate rule→stable, deletes candidate, rewrites the rule validate cache into a stable confirmation cache. Rejects if the cache is missing or stale. Consumer impact assessment is the agent's responsibility. See `framework/spec_writing_guide.md` §6. | Agent or human maintainer |
| `specflowctl review run-*` | Governance review run-state management. Subcommands: `collect-default-scope` (collect the deterministic default scope for a review flow), `run-init` (create/reuse run-state), `run-validate` (validate run-state shape), `run-refresh` (recompute fingerprints, mark stale), `run-touch` (update timestamp). See `framework/spec_flow_review.md` §6. | Deep audit executor |
| `specflowctl operation ...` | Operation-scope state carrier. `open` declares and freezes a bounded change scope (target spec-derived surface + explicit `--allow` paths + `--require-spec` paths + baseline commit); `check` mechanically evaluates the final change set against it (read-only, fail closed); `close` marks it closed only on pass; `update` widens the declared scope explicitly (user-authorized only); `status` inspects open operations. State: `meta/operations/<id>.json` (local process state). See `framework/operations/operation_scope.md` and `tooling/README.md` §Operation scope. | Agent (when the project rules or the user require bounded scope) |
| `validate@{target}` (agent trigger) | Complete runs use independent validate packets plus unit cross and write through `gate-finalize`; candidate full FAIL deletes the cache. Targeted checks execute directly and never publish a complete cache. A targeted P0/P1 must immediately run `gate-invalidate --gate validate ... --check {key}` so a pass cache is deleted or a failure-record invalidation is persisted. | User says "validate" or confirms agent suggestion |
| `verify@{target}` (agent trigger) | Complete unit runs use detection, conditional analysis, and cross packets; rule verify has been removed. Complete FAIL writes the failure record. Targeted checks never publish a complete cache; targeted P0/P1 immediately runs `gate-invalidate --gate verify ... --check {item}`. | User says "verify" or confirms agent suggestion |
| `review@{target}` (agent trigger) | Complete unit runs use one reviewed-file packet per file plus cross and write the review cache. Targeted file review never publishes a complete cache; targeted P0/P1 immediately runs `gate-invalidate --gate review ... --check {file}`. | User says "review" or confirms agent suggestion |
| `revalidate@{target}` (agent trigger) | Mechanism-derived delta/repair packet run. Repair includes failed checks and persisted `invalidated_checks` automatically, plus stale/new keys, explicit `--rerun` overrides, and unit cross; unmappable invalidations degrade to full scope. Writes `basis: delta|repair`; FAIL updates the failure record. | User says "revalidate" or confirms agent suggestion |
| `reverify@{unit}` (agent trigger) | Mechanism-derived detection/analysis packet run for stale, failed, persisted-invalidated, new, and explicitly forced items plus cross. Writes `basis: delta|repair`; FAIL updates the failure record. | User says "reverify" or confirms agent suggestion |
| `rereview@{unit}` (agent trigger) | Mechanism-derived file packet run for stale, failed, persisted-invalidated, new, and explicitly forced files plus cross. Writes `basis: delta|repair`; FAIL updates the blocking failure record. | User says "rereview" or confirms agent suggestion |
| `promote@{target}` (agent trigger) | Resolves type through §Target Resolution, then uses the unit 3-step or rule 2-step workflow. Unit: archive (`unit_promote_workflow.md`). Rule: version promotion + consumer migration + body-ref cleanup (`rule_promote_workflow.md`). On FAIL: rejects if an applicable cache is stale/missing/blocking, format is invalid, or copy fails; changes no truth. Report the failure and wait for the user to trigger the indicated gate before retrying. | User says "promote" or confirms agent suggestion |
| `specflowctl init` | Initialize specFlow project | Human |
| `specflowctl doctor` | Diagnose project setup | Human |
| `spec_flow_update` (agent trigger) | Full update: pull framework, detect format changes, migrate spec files, check document format. See `framework/operations/update.md` for full procedure. | User says `spec_flow_update` |
| `spec_flow_version` (agent trigger) | Check the installed SpecFlow version against the remote latest and report whether the project is up to date. If behind, recommend running `spec_flow_update`. See `framework/operations/version.md` for full procedure. | User says `spec_flow_version` |
| `specflowctl consumers --rule <id>` | List all units that reference the given rule in their rule_refs. For global rules (`g_rule_*`): returns every unit with a file in either layer (retiring candidates included) — global rules apply to all units by default and are not repeated in rule_refs. For bound rules (`b_rule_*`): empty output means no consumers. Current-layer (effective) semantics: each unit resolves to its candidate file when one exists, falling back to the stable file (a stale stable file whose candidate dropped the reference no longer counts); `deps --rule` excludes retiring units — see `framework/verification_scope.md` §Dependency Analysis. | Agent for impact analysis |
| `specflowctl detect` | Read-only detection of removable rules. `--rule <id>`: reports the rule's current-layer (effective) consumers and its `unbound_retention` declaration (removable = no consumers and no retention declaration). `--all`: lists every bound rule (`b_rule_*`) in the candidate and stable layers with no consumers and no retention declaration. Global rules (`g_rule_*`) are never listed — they apply to every unit by default, so "no consumers" is not a meaningful state; they are removed only by an explicit `specflowctl remove --rule`. Pure read-only — never writes or deletes files. See `framework/spec_writing_guide.md` §6.5. | Agent before rule removal (`detect@{rule}` / `detect@all`) |
| `specflowctl remove --rule <id>` | Delete a rule whose constraint no longer applies. Final verification reuses the detection primitive: rejected while any current-layer unit still references the rule in `rule_refs` (referrers listed), and while it declares `unbound_retention` (intentional retention). For a global rule, only explicit references block removal — the default applicability lifts with the file. On success deletes the stable copy (and candidate copy if present), then the rule's baseline and validate cache. User-confirmed only. See `framework/spec_writing_guide.md` §6.5. | Agent on user instruction |
| `specflowctl deps` | Read-only dependency analysis. `--scope all` (default, current-layer units — candidate preferred, stable fallback; retiring units with `status: retired` are excluded — their references disappear with them) / `candidate` / `stable`: reports the dependency graph from all in-scope units' `unit_refs`, cycle member lists, and promotion order (dependencies first). `--unit <name>`: the unit's depends-on refs, bound rules, referrers, and cycle state. `--rule <id>`: the units bound to the rule — explicit `rule_refs` consumers for a bound rule (`b_rule_*`), every current-layer unit for a global rule (`g_rule_*`, which applies by default and is not repeated in `rule_refs`); a global rule with no rule file is reported as not found. Pure mechanical computation — never infers dependencies from prose, never writes files. See `framework/verification_scope.md` §Dependency Analysis. | Agent on `deps@all` / `deps@{unit}` / `deps@{rule}` |
| `specflowctl gate-evidence` | Inspect the dependency evidence for a file read during validate/verify/review: maps the declared line ranges onto content-defined chunks and outputs the whole-file `hash` + `deps` chunk CIDs (inspection only — the cache evidence is computed by the tooling at `gate-finalize`; nothing is transcribed). `--file <path>` required; `--ranges START-END,START-END` optional (empty = whole file); `--acceptance-items` additionally emits the order-insensitive semantic set CID of a spec's `acceptance_item_set` (`region:acceptance_items:<cid>`, computed over the set preamble and the item regions sorted by id) — the precise declaration for cross-unit checks; `--acceptance-item <id>` (repeatable) emits one acceptance item's region (`region:acceptance_item:<id>:<cid>`) — the precise declaration for a per-item judgment; `--section <heading>` (repeatable) declares a section region by heading text (`region:section:<heading>:<cid>`) — the precise declaration for own-spec section judgments (`frontmatter` names the pre-`##` region); `--sections` lists every section region (heading, lines, CID) and `--items` lists every acceptance item region (id, lines, CID), both without declaring anything — the informational outputs that name `--section` / `--acceptance-item` values and probe locatability. See `framework/validation_cache.md` §Dependency Declaration and §Structural Region Dependencies. | Agent when inspecting the declaration surface |
| `specflowctl gate-plan` | Fix the immutable input snapshot and deterministic packet plan. `--input` adds evidence available to packets but never creates a work packet. Verify plans detection + conditional analysis packets per item; unit gates end in one cross packet. Delta/repair runs also snapshot carried judgments from the baseline. See `framework/verification_scope.md` §Gate Work Packets. | Agent before executing any full/delta/repair quality-gate run that persists a cache |
| `specflowctl gate-packet` | Materialize one packet's execution context: exact read refs, packet scope, and accepted dependency results/digests (plus carried judgments for cross). `--run RUN_ID --packet PACKET_ID`. Include the output verbatim in the independent executor's prompt. Read-only. | Agent before launching each packet executor |
| `specflowctl gate-status` | Read-only report of packet-run progress. `--gate` / `--unit` / `--rule` filter the listing; without a filter, every open run with its gate/target and packet counts. `--run RUN_ID`: per-packet status (`pending` / `accepted` / `rejected`, plus verify's conditional `not_required`), attempt numbers, the latest rejection reason, result digests, and the next action (the `gate-packet` + `gate-submit` pair, or `gate-finalize`). Reads run state only — never writes. The recovery point after an interrupted run. | Agent on `gate-status` / after a run interruption |
| `specflowctl gate-submit` | Validate and record one packet report plus its immutable parsed result. Declarations must belong to that packet's read refs. Verify detection resolves its conditional analysis packet; analysis/cross bind consumed result digests; cross must dispose every finding and publish every effective logical status. | Agent (coordinator) after each packet report is produced |
| `specflowctl gate-finalize` | `--run RUN_ID` only (plus optional timestamp/repo root). Derives result, blocking, P0–P3 counts, and failure statuses from the accepted synthesis result; the coordinator cannot supply judgment values. Candidate validate cache deletion applies only to a full-run FAIL; delta/repair FAIL writes a recovery record. | Agent after every required packet is resolved |
| `specflowctl validate` | Validate candidate spec structure (9 checks), rule validation (6 mechanical checks), or file write permissions | Human maintainer or agent |
| `specflowctl fresh` | Read-only cache freshness report. `--scope candidate` (default): summary for every unit/rule with a candidate file. `--scope stable`: drift state for every stable unit/rule. `--scope all`: both. `--unit <name>` / `--rule <id>`: detail for one target (candidate gate statuses, or stable drift state for a stable-only target). A STALE gate additionally prints its `DELTA SCOPE` section. Every summary report (candidate/stable/all) ends with the full removal-candidate list — bound rules with no current-layer consumers and no retention declaration; the list is layer-independent (removability is decided by consumers and the retention declaration alone, not by which layer holds the rule file) and appears exactly once per report, read-only. Never writes/deletes caches or baselines. See `framework/validation_cache.md` §Freshness Check. | Agent on `fresh@{target}` / `fresh@candidate` / `fresh@stable` / `fresh@all` |
