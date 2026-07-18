# SpecFlow Tooling

This directory contains the standalone Go CLI that performs deterministic governance actions for `specFlow`.

The tooling layer exists only for fixed execution work whose meaning is already constrained by governance rules.
It validates spec files mechanically, but it does not judge business semantics.

In tooling contracts, `<tooling-root>` means `tooling/` in a `source_repo` layout and `specflow/tooling/` in an `installed_project` layout.
Installed-project usage examples below continue to show `specflow/tooling/...` directly.

## Build

`<tooling-root>/bin/` is a local binary cache.
It is ignored by git and must not be committed.

Installed-project rebuild example from the repository root:

```bash
cd specflow/tooling
go run ./cmd/specflowctl build-release --repo-root ../..
```

Source-repository rebuild example from the repository root:

```bash
cd tooling
go run ./cmd/specflowctl build-release --repo-root ..
```

Official platform binaries are GitHub Release assets.
Release tags use the tooling fingerprint form `specflow-tooling-<12-character-fingerprint>`.
The release workflow builds binaries from the tagged source and uploads the binaries plus `SHA256SUMS`.
The release is tied to the tooling input fingerprint, not to every source commit.
The fingerprint includes Go command code, Go internal code, and required tooling metadata.

Pull the repository and install the current platform binaries for the pulled tooling source:

```bash
specflow/tooling/scripts/pull_with_release.sh
```

PowerShell:

```powershell
.\specflow\tooling\scripts\pull_with_release.ps1
```

The script runs a fast-forward pull, computes the current tooling fingerprint, and downloads the current platform's `specflowctl`, `specflow-reader`, and `SHA256SUMS` only when the local binaries are missing, stale, or missing checksums.

Push the current branch and publish a tooling release when the current `main` fingerprint has no release tag:

```bash
specflow/tooling/scripts/push_with_release.sh
```

PowerShell:

```powershell
.\specflow\tooling\scripts\push_with_release.ps1
```

## Governance Boundary

The tooling layer may:

1. collect
2. parse
3. validate
4. rebuild
5. compare
6. cleanup
7. sync
8. render read-only local views
9. maintain mechanical review run-state fields

The tooling layer must not:

1. invent new governance semantics
2. replace governance judgment
3. replace shared-boundary judgment
4. replace review severity or final conclusion judgment owned by the active review policy
5. become a second semantic source of truth
6. write reader-derived conclusions back into project files

`impact_sync` is a governance concept first.
The current CLI exposes only the deterministic pieces already justified by rules.
For shared-change reconciliation, the current mechanical entry remains `rule sync-impact`, but that entry must first compute `rule_sync` scope and exceptions and only then hand the fixed downstream object set to internal `impact_sync`.

`specflow-reader` is a read-only local view over current truth files.
It may parse `docs/specs/**`, build an in-memory graph, serve local HTML from `<tooling-root>/reader/web`, and refresh that view when truth files change.
It must not edit files, advance governance state, or store semantic conclusions outside process memory.

## Current Command Surface

1. `init`
   - bootstrap framework-managed files
2. `doctor`
   - inspect installation and binary freshness health
3. `build-release`
   - rebuild cross-platform binaries
4. `migrate`
   - update hook files and check the specFlow binary version
5. `next`
   - discover a unit's files and dependencies
   - `next --unit <name> [--explain]`: outputs candidate/stable spec files, appendix files, rule refs, and related units
   - `--explain` additionally inlines the current spec format rules
   - this is a render action: read-only, does not modify any project file
6. `promote`
    - validate candidate spec format, copy candidate files to stable directories, and remove candidate files
   - `promote --unit <name>`: runs format checks, required-field validation, and reference integrity
   - this is the only write gate
7. `review collect-default-scope --flow <review_flow>`
   - collect the deterministic default scope for the explicit review flow
8. `review run-init --flow <review_flow>`
   - create or reuse the full-scope run-state file for the explicit review flow
9. `review run-validate --flow <review_flow>`
   - validate required run-state fields, timestamps, all fixed statuses including closed statuses, baseline slices, score state when present, and dynamic slice parent links
10. `review run-refresh --flow <review_flow>`
   - recompute slice input fingerprints for an open run-state file, mark changed `passed` slices as `stale`, and refresh `last_updated_at`
11. `review run-touch --flow <review_flow>`
   - refresh only `last_updated_at`
12. `rule sync-impact`
   - compute rule-specific scope, resolve rule-only exceptions into generic impact input, then execute deterministic downstream fallback for the fixed affected objects through internal `impact_sync`
   - when a rule-governance topology round deleted an exact Rule ref only after proving it has no current-layer unit consumers, the caller may pass that ref through `--deleted-rule-refs`; the command verifies the ref is absent from rule files and current-layer unit `rule_refs`, then reports a no-impact result with no unit fallback
   - when stable landing self-exemption is needed, the caller must pass both `--stable-landing-unit` and exact `--stable-landing-rule-refs`
   - when the same stable landing round retargeted candidate units to those stable landing rule refs, the caller must pass those units through `--retargeted-units` and must select both the old candidate Rule refs and the new stable Rule refs through exact `--rule-refs`
   - the caller may narrow the derived unit subset with `--units`, but at least one rule trigger input must still be provided through `--rule-refs`, `--rule-ids`, or `--deleted-rule-refs`; retargeted stable landing requires exact `--rule-refs`
13. `rule consumers`
   - read current-layer `unit` frontmatter `rule_refs` and print the consumers for one `rule_id` or exact `rule_ref`
14. `rule release-version`
   - publish an already-existing stable Rule version by retargeting current-layer consumers from `--from-ref` to `--to-ref`
   - candidate current-layer objects are rewritten directly
15. `validate write`
   - check whether a file path may be written under current governance constraints
   - `validate write --path <path>` checks the executor's write permission for the given path
16. `validate candidate --unit UNIT`
    - validate candidate spec structure (checks: frontmatter, acceptance items, anchor integrity, references, appendices, version consistency)

## Reader Command Surface

`specflow-reader` is a separate binary from `specflowctl`.
It starts one local reader server directly and has no public subcommands:

```bash
cd specflow/tooling/bin
./specflow-reader-linux-amd64 --addr 127.0.0.1:17863
```

Rules:

1. running `specflow-reader` starts a local HTTP server and prints the URL.
2. the reader does not open a browser automatically.
3. when `--repo-root` is omitted, it defaults to `../../..` from the current working directory.
4. the server reads front-end files from `<tooling-root>/reader/web`.
5. the server reads current project truth from `docs/specs/**`.
6. each `/api/snapshot` request rebuilds the displayed snapshot from disk before returning data.
7. the server does not watch files and does not expose a server-sent event stream.
8. `/api/source` may return source text only from allowed truth and support files under the requested repository root.
9. `/api/source-diff` returns diff hunks between the candidate and stable versions of a Spec document. Request parameter: `path` (repo-relative path to the Spec file). Returns a list of hunks with added/removed lines. Implemented at `/tooling/internal/reader/server.go` and consumed by the front-end diff panel.
10. the hidden build-fingerprint query command is reserved for freshness checks.
11. the snapshot includes candidate and stable spec metadata from the current truth files.

Reader front-end rules:

1. `<tooling-root>/reader/web` is the only runtime source for reader HTML, CSS, and JavaScript.
2. `specflow-reader` does not embed a fallback copy of the front-end.
3. editing front-end files does not require rebuilding `specflow-reader`; refresh the browser after the file change.
4. `doctor` reports missing required reader front-end files as installation failures.
5. the reader front-end supports Chinese and English interface text.
6. language switching affects only reader-owned UI text.
7. Spec document source text, file paths, object IDs, command names, and version values are displayed as source data and must not be translated by the front-end.
8. the selected reader language may be stored in browser-local state and must not be written into project files.
9. the front-end refresh button requests a new snapshot immediately.
10. the front-end also polls `/api/snapshot` on a fixed interval so open pages converge to the latest disk state without relying on filesystem events.
11. the Markdown document panel builds an in-memory side guide from the source document's Markdown headings, lets the reader open or close that guide locally, and uses it only for scrolling inside the currently opened document.
12. the Spec View front-end view shows current candidate main Specs and candidate appendices as candidate truth, shows stable baseline truth separately for active candidate units, and shows current stable main Specs plus stable rule Specs from the existing snapshot; it must not create a new review state or write any page conclusion back to project files.

## Review Run-State Commands

The `review run-*` commands require an explicit review flow:

1. `spec_flow_review`
2. `spec_flow_design_review`

Review run-state commands use the `source_repo` layout:
- framework inputs from `framework/`
- templates from `templates/`
- tooling from `tooling/`
- project-instance compatibility: template bootstrap compatibility under `templates/docs/specs/` (no real project-instance `docs/specs/` required)

They maintain only mechanical fields in:

```text
docs/specs/_governance_review/spec_flow_review.md
docs/specs/_governance_review/spec_flow_design_review.md
```

Rules:

1. timestamps are written from Go runtime UTC time using `YYYY-MM-DDTHH:MM:SSZ`
2. run-state files record `review_layout` as `source_repo`
3. input fingerprints are computed from repository-relative input files
4. `run-refresh` may change `passed` slices to `stale` when inputs change or disappear
5. tooling must not change `pending`, `blocked`, or `skipped_not_in_scope` into a passing judgment
6. tooling may create and validate the `spec_flow_design_review` score-state skeleton
7. tooling must not write findings, severities, non-blocking optimizations, question scores, score basis, hard-blocker judgments, or final conclusions owned by the active review policy
   - `spec_flow_review` final conclusions are `pass | blocked`
   - `spec_flow_design_review` final conclusions are `pass | pass-with-optimization | blocked`
8. each review flow uses one fixed run-state file
9. when the fixed run-state file is missing, tooling creates the file for a new full-scope review
10. when a new full-scope review starts after a closed or invalid run-state file, tooling deletes the old fixed file before writing the new run state
11. `run-validate` checks structural validity only; a closed run-state file can validate successfully while still remaining unavailable for reuse
12. when the fixed run-state file is valid and open, `run-init` applies the owning review policy's age rule:
   - no more than two hours old: reuse automatically
   - for `spec_flow_review`, more than two hours and no more than 24 hours old: stop for a manual reuse-or-delete decision
   - for `spec_flow_review`, more than 24 hours and no more than seven days old: stop for a manual reuse-or-delete decision and recommend deleting the old run state and starting a new run
   - for `spec_flow_design_review`, more than two hours and no more than seven days old: stop for a manual reuse-or-delete decision
   - more than seven days old: delete as expired and create a new run state
13. after reusing an open run-state file, callers must run `review run-refresh` before continuing review work so changed inputs become stale slices instead of hidden drift
14. `review run-refresh` is the authoritative command for updating `input_fingerprint`; callers must not write manual hash output into run-state files
15. `spec_flow_review` baseline run state includes `supporting_layer_convergence` to force explicit review of promote paths for stable and candidate supporting truth
16. review run-state slice fields follow the generic protocol only through the adoption rules in the active review policy


## Tooling Input Set

The default `spec_flow_review` tooling review input set is:

1. the framework tooling policy and this README
2. the current tooling source input set listed below
3. the tooling helper script input set listed below
4. reader runtime files under `<tooling-root>/reader/web/**`

The current tooling source input set is:

1. `<tooling-root>/cmd/**/*.go`
2. `<tooling-root>/internal/**/*.go`
3. `<tooling-root>/go.mod`
4. `<tooling-root>/manifest.tsv`
5. `<tooling-root>/go.sum` when it exists

The tooling helper script input set is every regular file under:

```text
<tooling-root>/scripts/**
```

This includes install, pull-with-release, push-with-release, build-release, and tooling-fingerprint scripts.

The manifest is included because it controls which framework-managed and project-managed files `init` and `doctor` inspect or write.
Reader front-end files under `<tooling-root>/reader/web/**` are runtime files, not binary freshness inputs.
Tooling helper scripts are review inputs because they rebuild or select binaries for the installed tooling source.
They are not binary freshness inputs unless they change compiled binary behavior.

## Usage Examples

Run ordinary governance commands from the repository root using the matching platform binary under `specflow/tooling/bin/`.
For normal use, download the matching `specflowctl-*` and `specflow-reader-*` files from the GitHub Release for the installed tooling fingerprint.
For local tooling development, rebuild them with `build-release`.

When developing the tooling itself, do not assume that ordinary commands may run through `go run`.
The freshness gate requires an embedded build fingerprint for ordinary governance actions.
The supported `go run` recovery and inspection surface remains:

1. `build-release`
2. `doctor`
3. `help`
4. the internal build-fingerprint query command

Examples:

```bash
./specflow/tooling/bin/specflowctl-linux-amd64 doctor
./specflow/tooling/bin/specflow-reader-linux-amd64 --repo-root . --addr 127.0.0.1:17863
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review collect-default-scope --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-init --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-init --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-validate --flow spec_flow_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-refresh --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 review run-touch --flow spec_flow_design_review
./specflow/tooling/bin/specflowctl-linux-amd64 next --unit ai
./specflow/tooling/bin/specflowctl-linux-amd64 promote --unit ai
./specflow/tooling/bin/specflowctl-linux-amd64 rule sync-impact --rule-refs c_b_rule_app_config_topology@0.2.0 --units ai
./specflow/tooling/bin/specflowctl-linux-amd64 rule sync-impact --deleted-rule-refs c_b_rule_unused@0.1.0
./specflow/tooling/bin/specflowctl-linux-amd64 rule sync-impact --rule-refs c_b_rule_runtime_model@0.3.0,s_b_rule_runtime_model@0.3.0 --stable-landing-unit skill --stable-landing-rule-refs s_b_rule_runtime_model@0.3.0 --retargeted-units agent
./specflow/tooling/bin/specflowctl-linux-amd64 rule consumers --rule-ref s_b_rule_runtime_model@0.4.0
./specflow/tooling/bin/specflowctl-linux-amd64 rule release-version --rule-id b_rule_runtime_model --from-ref s_b_rule_runtime_model@0.3.0 --to-ref s_b_rule_runtime_model@0.4.0
```

## Freshness Rule

Compiled binaries under `<tooling-root>/bin/` are local cache files.
They must fail closed when the embedded tooling fingerprint no longer matches current source.
The fingerprint hashes tooling-root-relative keys such as `cmd/...`, `internal/...`, `go.mod`, and `manifest.tsv`, so identical tooling content has one fingerprint in both layouts.

The local development recovery path is:

```bash
cd specflow/tooling
go run ./cmd/specflowctl build-release --repo-root ../..
```

The normal user recovery path is to download the matching release binaries again for the installed tooling fingerprint.

The minimal stale-binary recovery and inspection surface remains:

1. `build-release`
2. `doctor`
3. `help`
4. the internal build-fingerprint query command
5. `next` — these are read-only render actions that do not modify project files or advance governance state
