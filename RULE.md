## Basic Requirements

1. Use Chinese in all conversations with the user. !important
2. When explaining technical details to the user, keep the wording as plain and easy to understand as possible without losing accuracy.
3. Do not use abstract concepts in communication with the user unless they are explicitly explained, because unexplained abstraction can cause misalignment.

## First-Principles Thinking

Use first-principles thinking. Do not assume that I always know exactly what I want or how to get it. Stay cautious, start from the original requirement and problem, and stop to discuss with me if the motivation or goal is unclear.


## Solution Rules

  When you need to provide a modification or refactor plan, it must follow these rules:

  - Do not provide compatibility-style or patch-style solutions.
  - Do not over-engineer. Keep to the shortest implementation path, and do not violate the first rule above.
  - Do not introduce solutions beyond the requirements I provided, such as fallback logic or repair-oriented additions, because that can cause business logic drift.
  - The solution must be logically correct and verified across the full end-to-end chain.

## Spec-First Planning Contract

  When planning or formulating an implementation plan / proposed changes:

  - **Explicit Spec Declaration**: If planned changes affect code belonging to an existing unit (or introduce a new unit), the plan's `Proposed Changes` list MUST explicitly declare the unit's candidate spec (`docs/specs/units/candidate/unit_{name}.md`) and any affected appendices before the code files. If no candidate exists, running `specflowctl fork --unit <name>` must be declared as a prerequisite step.
  - **No Silent Skip**: If a code modification is evaluated as a pure internal refactor or performance fix with no changes to external contracts, state transitions, rule constraints, or acceptance criteria declared in the spec, the plan MUST include an explicit one-sentence Spec Impact Assessment explaining why candidate spec modification is not required. Never remain silent on spec impact.
  - **Spec-First in Execution**: During execution, the agent MUST update the candidate spec first (establishing updated constraints and acceptance items) before modifying code and tests.

## Document Language Rules

  - All other documents (specflow document) must be written in English, because they are delivery documents.


## Atom System

specFlow uses an atom system (`framework/_atoms/`) to manage shared governance content
that appears identically across multiple files. This eliminates copy-paste drift.

### When to Edit Atoms

When you need to change content that an atom manages, **edit the atom source file** —
not the individual target files. The target files between `==ATOM_BEGIN:id==` and
`==ATOM_END:id==` markers are overwritten by `generate.sh`.

### Atom Workflow

1. **Edit** — modify the atom source file under `framework/_atoms/<category>/`.
2. **Generate** — run `./framework/_atoms/generate.sh` from the repo root. This propagates changes to all target files.
3. **Verify** — run `./framework/_atoms/verify.sh` to confirm all targets match their atom sources. A non-zero exit code means drift exists.
4. **Commit** — commit the atom source change AND all target file changes together.

### Adding a New Atom

1. Create the atom source file: `framework/_atoms/<category>/<name>.md`
2. Add a row to `framework/_atoms/manifest.txt`:
   ```
   <atom_id> | <category>/<name>.md | <target1>,<target2>,...
   ```
3. Add `==ATOM_BEGIN:<atom_id>==` and `==ATOM_END:<atom_id>==` markers to every target file at the desired injection point.
4. Run `./framework/_atoms/generate.sh` to populate the markers.
5. Run `./framework/_atoms/verify.sh` to confirm.

### Rules

- **DO NOT** manually edit content between `==ATOM_BEGIN:id==` and `==ATOM_END:id==` markers — it will be overwritten.
- **DO NOT** move or rename atom marker lines without updating `manifest.txt`.
- **DO** run `./framework/_atoms/verify.sh` before committing any governance-file changes.
- **DO** run `./framework/_atoms/generate.sh` after any atom source change.
- Atom marker lines must appear on their own lines with no leading/trailing whitespace.

For complete documentation, read `framework/_atoms/README.md`.


## Entry File Synchronization

`RULE.md` is the single source of truth for agent instructions (`AGENTS.md`,
`CLAUDE.md`, `GEMINI.md`). **After every edit to `RULE.md`, run `./sync.sh`**
to regenerate the three entry files. Do not edit `AGENTS.md`, `CLAUDE.md`, or
`GEMINI.md` directly — direct edits are overwritten by the next sync run and are
lost. If you need to change agent instructions, change `RULE.md` and run
`./sync.sh`. Personal, non-shared preferences do not belong in these files;
use the runtime's own local-only instruction mechanism instead (e.g. the
agent's user-global config, not this repository).


## Layout Context Rule

Before analyzing any governance file, identify its layout: `source_repo` (this repository) or `installed_project` (a project using SpecFlow). A file's layout determines its reader, its path resolution, and what constitutes a valid finding. Do not apply `installed_project` naming conventions or agent-facing standards to `source_repo` mechanism files, and vice versa. When a finding derives from cross-layout comparison, stop and verify the finding still holds within the target file's own layout.

Deployment artifacts (files under `templates/`, `hooks/`, and platform plugin
templates such as `.opencode/plugins/`, `.claude-plugin/`, `.agents/plugins/`)
are authored in the source repository but executed after installation inside a
consumer project. Always resolve their paths from the consumer's runtime
context (the installed layout: `<project>/specflow/...`, `<project>/.agents/...`)
and never judge a deployment artifact using the source-repository path as the
runtime truth. See `framework/spec_flow_review.md` Section 2.16 for the
authoritative rule.

## Governance Review Shortcut

This is the specFlow source repository, so use local `framework/...` paths.
SpecFlow governance instructions are delivered via hook injection (`framework/concepts.md`).
This section only routes governance review requests within the source repo — it is not the instruction source.

For `spec_flow_review` or ordinary governance review requests:

1. Read `framework/governance/review.md` to determine the default path.
2. Default to `scoped_review` (see `framework/governance/review_scope.md` for scoped vs deep_audit modes).

Use `framework/spec_flow_review.md`, `meta/governance_review/` run-state files, baseline slice tables, or dynamic slice tables only for exact `spec_flow_review:full`.

For `spec_flow_design_review`:

1. Read `framework/governance/review.md`.
2. Read `framework/spec_flow_design_review.md`.
3. Run the default full-scope design-baseline review. Do not narrow it to `scoped_review`.

## Review Experience Rules

Before reporting a finding in any governance review, check `REVIEW_EXPERIENCE_RULES.md`. Each rule describes a category of non-issue. If the finding falls within the same category — do not treat the listed keywords as exhaustive — close it without reporting.

## Git Commit Rule

0. **Never commit automatically.** All commits must be manually triggered by the user — wait for explicit instruction before running any commit command.
1. Analyze first – check changes in the working directory.
2. Decide – commit all at once or split into logical batches.
3. Commit message – must be in English, clear and concise.
4. **Follow Conventional Commits** – `<type>(<scope>): <short description>`
   **Example:** `feat(auth): add password validation`

## Others

1. When proposing modification plans or suggestions, do not propose minimal-change solutions. Analyze the essence of the problem based on first principles and provide the most correct solution.

2. When fixing problems, do not apply patch-style fixes. Analyze the essence of the problem based on first principles, and fundamentally redesign and fix from the root.

3. When the user inputs the `spec_flow_push` command, follow `framework/operations/push.md`. It first fetches `origin/main` to check for conflicts, analyzes the solution and waits for user confirmation to fix, and only then runs `./tooling/scripts/push_with_release.sh[.ps1]` to compute the tooling fingerprint via `go run ./cmd/specflowctl tooling-fingerprint` and record it into `tooling/fingerprint.txt`. This command is source-repo-only.

4. When the user inputs the `spec_flow_issues` command, follow `framework/operations/issues.md`. It pulls the GitHub issues of this repository via `gh`, triages each issue (spec + code comparison) to determine whether it is a real problem, and produces a fix plan for real problems. Reports locally only — never writes back to GitHub. Implementation happens only after the user explicitly confirms. This command is source-repo-only.
