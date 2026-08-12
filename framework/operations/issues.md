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

For each issue, launch a read-only subagent that performs the analysis independently (same pattern as `validate`/`review` — an independent session, not self-approval). The subagent prompt is a mission package for a zero-context worker: it has no conversation history and no framework knowledge beyond the prompt. The prompt must answer, without requiring inference: who am I, what am I doing, why, what counts as done, and who consumes my output. The triage prompt is assembled per the fixed forms below (task-package principle; the fixed forms of `framework/verification_scope.md` §Sub-agent Prompt Assembly apply to the three quality-gate commands only — the triage assembly forms are defined here):

- **Role line:** "read-only triage sub-agent for issue #{number}"
- **Mission (fixed form):** "Triage issue #{number}: locate the affected area, read the design intent, verify the actual behavior from the tooling source, classify the issue per the classification table, and produce the Step 3 output format."
- **Context (fixed form):** "You are part of the `spec_flow_issues` run. You are an independent read-only session — you do not hold the context of prior discussions or of how the issue was filed, and the main agent does not re-litigate your verdicts. Your output is collected verbatim into the local report; follow the Step 3 output format exactly."
- **Protocol reference:** "framework/operations/issues.md Step 3 — the only protocol source; the prompt contains no restatement of protocol rules."
- **Glossary:** one line per term used in the assembled prompt, sourced from this file's classification table and output contract: `classification`, `basis`, `REAL-bug`, `REAL-design`, `NOT-ISSUE`, `DUPLICATE`, `ENHANCEMENT` — each with a one-sentence definition and its source reference in this file (the classification table below). A term with no definition or source is a prompt defect.

**Input (mandatory, provided by the main agent):** the full issue content — title, body, all labels, and all comments — passed verbatim. Never pass only the title or a summary; the sub-agent must not infer issue content.

**Permissions (verbatim):** "You may read files, search text by pattern, glob for files, and run read-only git queries. You must NOT modify any file, run any command that changes state, or launch further sub-agents."

**Output (mandatory, per issue):**

- One line: `#{issue number} | {classification} | {basis}` where `{basis}` cites the deterministic evidence — a documentation `file:line` or a code `file:line` reference. A classification without a cited basis is not a valid output.
- For REAL-bug and REAL-design: root cause (first-principles analysis, not symptom description), affected files with `file:line` references, fix plan, and verification method (see Step 4).
- For NOT-ISSUE, DUPLICATE, and ENHANCEMENT: one or two sentences of reasoning.
- If the triage could not complete: "Triage could not complete — {reason}"

The subagent must:

1. **Locate the affected area** — map the issue's title and body keywords to the framework documentation (`framework/*.md`), the tooling source (`tooling/cmd/specflowctl/*.go`), the tooling scripts (`tooling/scripts/`), or the templates (`templates/`, `hooks/`).
2. **Read the design intent** — read the relevant framework documentation that states how the behavior *should* work (e.g. a checklist, a command reference entry, `framework/spec_flow_review.md`).
3. **Verify the actual behavior** — read the relevant tooling source to establish what the system *actually* does. Running `go test` or the binary is not part of the sub-agent's permission set; if static evidence is insufficient to determine the classification, state the missing evidence in `{basis}` — the main agent may run the tests itself.
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

For every REAL-bug and REAL-design finding, the subagent must produce:

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
