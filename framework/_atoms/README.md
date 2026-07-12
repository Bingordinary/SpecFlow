# Atom System

The Atom System manages shared governance content that appears identically across multiple
framework files. Instead of copy-pasting the same content into every file (creating
maintenance drift), content is defined once in an atom source file and generated into all
target files via deterministic scripts.

## Why Atoms Exist

The specFlow governance framework has documented procedural content that must appear in
multiple files simultaneously (Section 2.12 self-containment requirement). Examples include
rule governance shared footer (6 rule files).

Without atoms, updating any shared content requires editing every affected file manually —
a maintenance risk that has already produced documented drift (see `spec_flow_review:full`
findings F-001, F-002, and related P2/P3 findings).

With atoms:
1. **Edit once** — change the atom source file.
2. **Generate** — run `./generate.sh` to propagate the change to all target files.
3. **Verify** — run `./verify.sh` to confirm all targets match.

## Directory Structure

```
framework/_atoms/
├── README.md                    # This file
├── manifest.txt                 # Atom registry (atom_id → source → targets)
├── generate.sh                  # Generation script
├── verify.sh                    # Verification script
├── rules/
│   └── shared_footer.md         # Rule governance shared footer
└── misc/
    ├── layout_note.md           # Layout-aware path note for rule files
    └── framework_path.md        # Framework path resolution reference
```

## How Target Files Reference Atoms

Target files use marker lines to declare where atom content lives:

```
==ATOM_BEGIN:atom_id==
(content between markers is managed by the atom system — DO NOT EDIT DIRECTLY)
==ATOM_END:atom_id==
```

**Rules:**
- The content between `==ATOM_BEGIN:==` and `==ATOM_END:==` is owned by the atom system.
- Do NOT manually edit content between atom markers. Edit the atom source file instead, then run `generate.sh`.
- Content outside atom markers is NOT managed by the atom system and may be edited freely.
- Each atom_id may appear exactly once per target file.
- The marker lines must appear on their own lines with no leading/trailing whitespace.

## Usage

### Adding a New Atom

1. Create the atom source file under `framework/_atoms/<category>/<name>.md` with the shared content.
2. Add a row to `manifest.txt`:
   ```
   <atom_id> | <category>/<name>.md | <target1>,<target2>,...
   ```
3. Add `==ATOM_BEGIN:<atom_id>==` and `==ATOM_END:<atom_id>==` markers to each target file at the desired injection point.
4. Run `./generate.sh` to populate the markers with atom content.
5. Run `./verify.sh` to confirm correctness.

### Modifying Existing Atom Content

1. Edit the atom source file (`framework/_atoms/<category>/<name>.md`).
2. Run `./generate.sh` to propagate the change to all target files.
3. Run `./verify.sh` to confirm all targets match.
4. Commit both the atom source change AND the target file changes together.

### Verifying Atom Integrity

Run `./verify.sh` at any time to check whether all target files contain the
correct atom content. This should be run:
- After any framework governance changes
- Before committing changes that touch governance files
- As part of CI if the repository has one
- As part of `spec_flow_review` (see the atom verification standard)

### Removing an Atom

1. Remove the atom row from `manifest.txt`.
2. Remove the `==ATOM_BEGIN:==` / `==ATOM_END:==` markers from all target files.
   The content between markers stays in the file (it becomes locally owned).
3. Optionally delete the atom source file if no longer needed.

## Contract

1. The atom source file is the **single canonical source** for the shared content.
2. `generate.sh` is **deterministic and idempotent** — running it twice produces identical output.
3. `verify.sh` is **deterministic** — same input always produces the same pass/fail result.
4. `manifest.txt` is the **authoritative registry** of all atom → target mappings.
5. The atom system is **layout-agnostic** — all paths in `manifest.txt` are repository-root relative.
6. Atom markers are inert markdown — they do not affect rendering or execution.

## Historical Managed Blocks

The `==SPECFLOW:BEGIN==` / `==SPECFLOW:END==` managed block system was a legacy mechanism
for synchronizing specFlow governance content into project entry files (`AGENTS.md`,
`CLAUDE.md`, `GEMINI.md`). It has been replaced by the hook-based injection system
(`framework/hooks.md`). All managed block parsing and synchronization tooling has been
removed.
