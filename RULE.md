## Basic Requirements

1. Use Chinese in all conversations with the user. !important
2. When explaining technical details to the user, keep the wording as plain and easy to understand as possible without losing accuracy.
3. Do not use abstract concepts in communication with the user unless they are explicitly explained, because unexplained abstraction can cause misalignment.

## Communication Principles

The user has repeatedly reported that an explanation is hard to follow. The usual causes are leading with mechanism, unexplained jargon, or decision labels, and assuming the user already knows the project's design and details. Apply these rules to every explanation and every question:

1. **Lead with the problem and its consequence, in plain words.** Open with what goes wrong and why it matters, in everyday language. Code paths, function names, and `file:line` citations are evidence: give them only after the plain-language point is made, and only when they are actually needed.
2. **Explain or replace every term of art on first use.** For each coined term (for example "dependency surface", "delta run", "mechanical floor"), explain it in one plain sentence the first time it appears. Never stack several unexplained terms in one paragraph.
3. **Start from the project's design rationale, then a concrete design-detail example, before the abstract rule.** Do not use everyday analogies. Assume the user does not yet know the project's design: first state why the mechanism is designed this way (the intent, and the constraint it satisfies), then walk through a concrete case built from this project's own design and details (actual files, records, commands, or state transitions), and only then state the general rule.
4. **Advance one decision at a time.** Do not present several options, sub-options, or trade-offs in one message. Present the single decision that matters now, in plain terms, and wait for the answer before moving on.
5. **Keep the layers separate.** Do not mix the business problem, the underlying mechanism, the plan or trade-off, and the progress report in one message. Label them distinctly, or raise them in separate turns. Never bury a user-facing question inside an implementation report.
6. **Answer the question that was asked.** If the user asks "what is the problem", answer what the problem is — in their terms — before saying anything about the solution. Do not answer a "what" question with a "how" or "what I did" answer.
7. **Conclusion first; keep it short.** State the answer in the first one or two sentences, then expand only if the user asks or if the conclusion genuinely cannot stand without it.
8. **Confirm understanding before building on it.** Before stacking the next layer on a concept, make sure the user has signalled that they follow the current one.
9. **When your earlier framing changes, say so plainly and first.** State what you said before, why it no longer holds, and then the new plan — before its details. Never silently swap the framework the user was working with.

## First-Principles Thinking

Use first-principles thinking. Do not assume that I always know exactly what I want or how to get it. Stay cautious, start from the original requirement and problem, and stop to discuss with me if the motivation or goal is unclear.


## Repository Scope and Layout Rule

This repository is the SpecFlow `source_repo`: it develops and distributes the
SpecFlow framework, but it does **not** use SpecFlow to govern its own
development. The unit/rule lifecycle described in `framework/concepts.md`
applies only after SpecFlow is installed in a consumer project
(`installed_project`).

Before analyzing, planning, proposing changes, or modifying files, distinguish
these two independent questions:

1. **Development governance** — work performed in this repository is always
   `source_repo` development and follows this file plus the relevant
   source-repository owner documents and tests.
2. **Runtime layout** — deployment artifacts may need to be evaluated as they
   will run inside an `installed_project`; that runtime context does not change
   the governance of the source-repository change itself.

For source-repository development:

- **DO NOT** create, fork, update, validate, verify, review, or promote a
  root-level `docs/specs/` unit or rule as a prerequisite for changing this
  repository.
- **DO NOT** require a candidate-spec declaration, candidate-spec update, or
  Spec Impact Assessment in plans for source-repository changes.
- **DO NOT** apply candidate/stable workflow requirements from
  `framework/concepts.md` to changes under `framework/`, `tooling/`,
  `templates/`, `hooks/`, or other source-repository paths.
- Spec files under templates, demos, fixtures, or temporary test projects are
  deployment/test data; they do not govern development of this repository.
  Tests may still exercise SpecFlow lifecycle commands against those isolated
  inputs.

Deployment artifacts (files under `templates/`, `hooks/`, and platform plugin
templates such as `.opencode/plugins/`, `.claude-plugin/`, `.agents/plugins/`)
are authored in the source repository but executed after installation inside a
consumer project. Resolve their runtime paths and behavior from the consumer's
context (the installed layout: `<project>/specflow/...`, `<project>/.agents/...`),
while treating edits to those artifacts as `source_repo` development. Do not
apply `installed_project` naming conventions or agent-facing standards to
`source_repo` mechanism files, or source-repository paths to deployed runtime
behavior. See `framework/spec_flow_review.md` Section 2.15 for the authoritative
deployment-layout rule.


## Solution Rules

  When you need to provide a modification or refactor plan, it must follow these rules:

  - Do not provide compatibility-style or patch-style solutions.
  - Do not over-engineer. Keep to the shortest implementation path, and do not violate the first rule above.
  - Do not introduce solutions beyond the requirements I provided, such as fallback logic or repair-oriented additions, because that can cause business logic drift.
  - The solution must be logically correct and verified across the full end-to-end chain.

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
