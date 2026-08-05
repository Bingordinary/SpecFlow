# Spec Adoption (Existing Code Baseline)

## Purpose

Spec adoption brings an existing, already-implemented project under specFlow governance. Instead of designing from scratch, the agent records observed implementation behavior as evidence and builds candidate specs from it. Adoption produces the same artifacts as normal development — candidate specs, appendices, and acceptance items — and flows through the same validate → verify → promote pipeline. The only difference is the starting point: code, not design.

## Trigger

The adoption flow is triggered in two situations:

1. **Install-time (see `INSTALL.md`):** after installing specFlow into a project that contains existing source code, the agent scans the project, reports whether it is a greenfield or an existing project, and asks the user whether to build specs from the existing code. On confirmation, run this flow.
2. **Session-time (see `framework/concepts.md`):** when the user expresses adoption intent during any later session — e.g. "把这几个模块登记一下", "继续建档", "把现有实现记录一下" — run this flow for the requested scope.

Adoption progress is tracked in-session by the agent; there is no adoption state directory. The user drives the pacing: batches are validated, verified, and promoted at the user's own rhythm, possibly across many sessions. The flow can be resumed at any time by telling the agent to continue adoption.

## Flow

### Step 1 — Scan the project

Identify module boundaries and dependency relationships from the code structure (packages, services, entry points). Produce a proposed unit cut with candidate unit names, the files each unit covers, and inter-unit dependencies.

### Step 2 — Unit cut list with suspicious-point column

Present the cut list to the user. Each row: unit name, covered files, dependencies. The list carries an additional column — **suspicious points** — where the agent marks (without judging) code locations that look suspect: TODO markers, panic/fallback swallowing, dead code, contradictory logic. Marking is non-judgmental: it only surfaces spots the user may want to look at; it does not classify them as defects. The user may ignore the column entirely.

### Step 3 — User confirmation and adjustment

The user confirms the cut, adjusts it (merge, split, or rename units), or defers units to a later batch. Units may be adopted in any order; the user decides the batch size and pacing.

### Step 4 — Batch candidate generation

For each unit in the confirmed batch, the agent:

1. Reads the code and writes the evidence appendix (`unit_{unit}_evidence.md`) under `docs/specs/units/candidate/appendix/`, organized by behavior domain — one section per behavior domain, each domain corresponding to exactly one acceptance item in the main spec.
2. Writes the candidate main spec (`unit_{unit}.md`) with the acceptance item set. Each evidence-driven acceptance item lists the evidence appendix in its `affects.appendices` and uses a `verification_type` appropriate to the recorded behavior (`testable` / `inspectable`).
3. Sets `evidence_appendix_ref: unit_{unit}_evidence.md` in the main spec frontmatter.

**Coverage standard for adoption:** main flow + boundary behaviors. Cover entry/exit points, the normal path, key error paths, and boundary conditions. Do not exhaustively document internal details — detail is added by later iterations as the unit evolves.

### Step 5 — Guided per-batch promotion

The agent guides the user through the normal pipeline for each batch, at the user's pace: `validate@{unit}` → `verify@{unit}` → `review@{unit}` → `promote@{unit}`. Evidence-driven items skip the design-rationale review (see `framework/unit_validate_checklist.md` Check 2 Step 2); all other quality gates apply unchanged. After a batch is promoted, the next batch may start.

## Evidence Lifecycle

Evidence is a transitional artifact: its mission is to bring a unit from "no spec" to "governed spec". It has a defined retirement path.

### Adoption round

The evidence appendix records all behavior domains; every evidence-driven acceptance item references it via `affects.appendices`; the unit-level `evidence_appendix_ref` points to the file.

### Incremental replacement (later rounds)

When a behavior domain is redesigned in a later iteration:

1. Convert the corresponding acceptance item to design-driven: remove the evidence appendix from its `affects.appendices` and provide design rationale.
2. Retire the corresponding section from the evidence appendix (delete it).

Other behavior domains keep their evidence references and continue to waive rationale review. Mixed states — some items evidence-driven, others design-driven — are legal and expected.

### Retirement (final round)

When no acceptance item references the evidence appendix:

1. Retire the last evidence section if one remains.
2. Promote normally: the emptied appendix is copied to stable, flushing any leftover stable sections.
3. Remove `evidence_appendix_ref` from the spec frontmatter (set to `none`).

Do not delete the candidate appendix file before promote — the stable copy cannot be deleted by any governance mechanism (only `promote` writes to stable), so deleting the candidate copy leaves the stable sections in place and the orphan finding re-appears in every later round. After the final promote the appendix remains as an empty file with no governance effect: no section means no zombie, orphan, or residual finding, and future forks copy it as inert content.

### Enforcement

`validate` detects zombie, orphan, and residual evidence states and reports them at **default severity P1** (blocking — promote must not proceed until resolved):

- **Zombie:** an acceptance item references the evidence appendix but no corresponding behavior-domain section exists.
- **Orphan:** an evidence appendix section exists but no acceptance item whose behavior domain corresponds to it.
- **Residual:** an evidence-driven item's behavior domain has been redesigned in the candidate.

See `framework/unit_validate_checklist.md` Check 4.

## Rules

1. Evidence appendix content must record directly readable observed behavior — not only background, motivation, or patch notes (`framework/unit_validate_checklist.md` Check 6).
2. One evidence appendix per unit; the appendix is organized by behavior domain, one section per acceptance item.
3. Adoption does not create a new spec lifecycle — adoption products flow through the standard validate → verify → promote pipeline.
4. The agent tracks adoption progress in-session; no adoption state directory is created.
