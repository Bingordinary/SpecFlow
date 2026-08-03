# spec_flow_issues

When the user says `spec_flow_issues`, follow this procedure. It pulls the GitHub issues of the current repository, triages each issue to determine whether it is a real problem, and for every real problem produces a root-cause analysis and a fix plan. The agent reports locally and never writes back to GitHub. Implementation happens only after the user explicitly confirms.

This command is for the `source_repo` layout (this repository). It does not apply to `installed_project` layouts — report that the command is unavailable there and stop.

## Prerequisites

- `gh` CLI authenticated (`gh auth status` succeeds). If authentication is missing, report it and stop.
- Layout must be `source_repo` (a `framework/` directory exists at the repository root). Otherwise report that `spec_flow_issues` is a source-repo-only command and stop.

## Procedure

### Step 1: Locate the repository

Determine the GitHub repository from the git remote:

```bash
git remote get-url origin
gh repo view --json nameWithOwner --jq .nameWithOwner
```

Use the `nameWithOwner` value (e.g. `owner/repo`) for all `gh` commands in Step 2. If no remote exists or `gh repo view` fails, report it and stop.

### Step 2: Extract issues

List open issues by default. If the user supplied a filter argument, use the matching command:

```bash
gh issue list --repo <owner/repo> --state open      # default
gh issue list --repo <owner/repo> --state all       # :all
gh issue list --repo <owner/repo> --state closed    # :closed
gh issue list --repo <owner/repo> --label <name>    # :label <name>
```

For every issue in the list, fetch its detail:

```bash
gh issue view --repo <owner/repo> <number>
```

Capture the title, body, labels, and comments. Do not infer issue content from the list output alone — read the full body before triaging.

### Step 3: Triage each issue (spec + code comparison)

For each issue, launch a read-only subagent that performs the analysis independently (same pattern as `validate`/`review` — an independent session, not self-approval). The subagent must:

1. **Locate the affected area** — map the issue's title and body keywords to the framework documentation (`framework/*.md`), the tooling source (`tooling/cmd/specflowctl/*.go`), the tooling scripts (`tooling/scripts/`), or the templates (`templates/`, `hooks/`).
2. **Read the design intent** — read the relevant framework documentation that states how the behavior *should* work (e.g. a checklist, a command reference entry, `framework/spec_flow_review.md`).
3. **Verify the actual behavior** — read the relevant tooling source (or run `go test` / the binary when needed) to establish what the system *actually* does.
4. **Compare and classify** using the table below.
5. **Check `REVIEW_EXPERIENCE_RULES.md`** — before reporting a NOT-ISSUE or REAL finding, check whether the pattern falls into a documented non-issue category (e.g. deployment-artifact path expectations). Do not treat the listed keywords as exhaustive.

#### Classification

| Class | Meaning | Basis |
|-------|---------|-------|
| **REAL-bug** | Behavior contradicts the documented promise | Documentation says X, code does Y |
| **REAL-design** | Documentation and behavior agree, but the mechanism itself is defective | e.g. the triage keyword mapping is too coarse and merges distinct defect classes into one, with the docs truthfully describing that mapping |
| **NOT-ISSUE** | False report; behavior matches the design | User misread, environment issue, or a documented non-issue pattern |
| **DUPLICATE** | Already covered by another issue or by existing documentation | — |
| **ENHANCEMENT** | Not a defect; a new feature or improvement request | — |

Severity follows the shared P0–P3 grading defined in `framework/severity_policy.md` (§4). This flow explicitly adopts that policy; the scale is not redefined here.

### Step 4: Produce a fix plan for real problems

For every REAL-bug and REAL-design finding, the subagent (or the main agent, if the subagent returned only the classification) must produce:

1. **Root cause** — first-principles analysis of why the defect exists, not a description of the symptom.
2. **Affected files** — the exact files to change, with `file_path:line` references.
3. **Fix plan** — what to change and how, following the Solution Rules (no compatibility-style or patch-style solutions, no over-engineering):
   - Tooling change: edit `tooling/cmd/specflowctl/*.go`, add or update Go tests, run `go test ./...`. Remember that a later `spec_flow_push` records the tooling fingerprint via `go run ./cmd/specflowctl tooling-fingerprint` into `tooling/fingerprint.txt`.
   - Framework change: edit the documentation file; if the content is managed by the atom system (between `==ATOM_BEGIN:id==` and `==ATOM_END:id==` markers), edit the atom source under `framework/_atoms/` and run `./framework/_atoms/generate.sh`, then `./framework/_atoms/verify.sh`.
   - Template/script change: edit `templates/` or `hooks/` accordingly.
4. **Verification** — how to confirm the fix (e.g. `go test`, running the affected command, `verify.sh`).

Layout note: this file targets the `source_repo` layout, so all paths are local (`tooling/...`, `framework/...`) without a `specflow/` prefix — unlike `framework/operations/update.md`, which targets the `installed_project` layout by design, and `framework/operations/version.md`, which covers both layouts (it has a `source_repo` variant in addition to the `installed_project` one).

### Step 5: Report and let the user decide

Present a local report only. Do not comment on, label, or close any GitHub issue, and do not modify the working tree.

The report is a summary table plus per-issue detail:

| Issue | Title | Class | Severity | Affected files | Fix plan (summary) |
|-------|-------|-------|----------|----------------|---------------------|

For each issue, give the root cause, affected files, and fix plan from Step 4. For NOT-ISSUE, DUPLICATE, and ENHANCEMENT, give the reasoning in one or two sentences.

Then state explicitly that implementation is the user's decision, and ask which findings (all, some, or none) to implement. Do not implement anything without explicit user confirmation. After the user confirms, implement per the repository's normal development flow — this repository does not develop itself through the SpecFlow validate/verify/promote workflow, so no governance gates apply.
