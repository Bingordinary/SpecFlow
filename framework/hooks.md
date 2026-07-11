# Hooks Injection System

SpecFlow injects governance content into agent sessions at startup through a platform-independent hook mechanism. The hook-injected content (`framework/concepts.md`) is the primary instruction source for all specFlow-governed work.

This file is the single authoritative reference for the hooks system.

## Injection Chain

```
Platform hook config (JSON)
  └── triggers hooks/run-hook.cmd
        └── triggers hooks/session-start
              └── reads framework/concepts.md
                    └── outputs JSON → injected into agent session context
```

The injected content arrives as platform-specific JSON which the agent runtime loads into the session prompt. The agent does not need to read `framework/concepts.md` from disk — its content is already present in the session context.

## Core Files

| File | Role |
|------|------|
| `hooks/session-start` | Shell script: reads `framework/concepts.md`, JSON-escapes it, wraps it in a governance preamble, and outputs platform-specific JSON. |
| `hooks/run-hook.cmd` | Cross-platform polyglot wrapper (valid Windows batch + Unix shell). On Windows, finds Git Bash and delegates the hook script; on Unix, executes it directly. |
| `framework/concepts.md` | The injected governance content. Contains key terms, workflow, trigger phrases (`spec_validate`, `spec_verify`, `spec_promote`), agent suggestion flow, HARD RULES, and commands reference. |

## Platform Support

The `session-start` script detects the target platform from environment variables and selects the JSON output format accordingly:

| Platform | Detection | Output Format |
|----------|-----------|---------------|
| Claude Code | `CLAUDE_PLUGIN_ROOT` set AND `COPILOT_CLI` not set | `{ "hookSpecificOutput": { "hookEventName": "SessionStart", "additionalContext": "..." } }` |
| OpenCode | OpenCode plugin (see below) | Message transform via JS plugin |
| Gemini CLI or no plugin platform | No platform-specific variable detected | `{ "additionalContext": "..." }` |

==ATOM_BEGIN:specflowctl_location==
specflowctl is not on PATH. Its binary is at `<tooling-root>/bin/specflowctl-<os>-<arch>`. In the `installed_project` layout (deployed to a parent project), `<tooling-root>` is `specflow/tooling`. In the `source_repo` layout (the specFlow framework repository itself), it is `tooling`. Replace `<os>` and `<arch>` with your platform (e.g. `linux-amd64`, `darwin-arm64`, `windows-amd64.exe`). Use the full path when running specflowctl commands.
==ATOM_END:specflowctl_location==

### Hook Configuration Files

Each platform requires a hook configuration JSON file that registers `session-start` as a SessionStart event trigger. These files are installed by `specflowctl install` / `specflowctl migrate`:

| File | Install To | Platform | Command |
|------|-----------|----------|---------|
| `hooks/hooks.json` | `hooks/hooks.json` | Claude Code | `"${CLAUDE_PLUGIN_ROOT}/specflow/hooks/run-hook.cmd" session-start` |

Claude Code discovers hooks by convention at `{pluginRoot}/hooks/hooks.json`.

### Platform Plugin Registration

Each platform needs the SpecFlow hooks registered so it knows to trigger `session-start` at startup. The registration mechanism differs by platform:

| Platform | Registration Method | Installed By |
|----------|-------------------|-------------|
| Claude Code | `.claude-plugin/plugin.json` (discovers hooks by convention at `hooks/hooks.json`) | `specflowctl` installs the file from `templates/.claude-plugin/plugin.json` |
| OpenCode | `.opencode/plugins/specflow.js` (auto-discovered by OpenCode) | `specflowctl` installs the file from `templates/.opencode/plugins/specflow.js` |

## How session-start Works

1. Reads `framework/concepts.md` from the repository root
2. JSON-escapes the contents (backslash, double-quote, newline, carriage-return, tab)
3. Wraps in a preamble: `"<SPECFLOW_CONCEPTS>\nThis project uses SpecFlow to manage design documents.\n\n**Below is the full SpecFlow framework guide — read it carefully before starting work:**\n\n{concepts_escaped}\n</SPECFLOW_CONCEPTS>"`
4. Detects platform from environment variables and outputs the correct JSON shape
5. Returns exit code 0 on success

## How run-hook.cmd Works

1. Receives the hook script name as its first argument (e.g. `session-start`)
2. On Windows: searches for `bash.exe` in `C:\Program Files\Git\bin`, `C:\Program Files (x86)\Git\bin`, and PATH, then executes the script via bash
3. On Unix: executes the script directly via bash
4. If bash is not found on Windows, exits silently (plugin still works, just without SessionStart injection)

Hook scripts use extensionless filenames (`session-start` not `session-start.sh`) to avoid Claude Code's Windows auto-detection, which prepends `bash` to any command containing `.sh`.

## Injected Content

The full text of `framework/concepts.md` is injected. It must contain:

1. **Core principle** — file existence is state (no state machine, no lifecycle phases)
2. **Key terms** — unit, rule, stable, candidate
3. **Workflow** — discover, edit, validate, verify, promote with agent suggestion flow
4. **HARD RULES** — four immutable rules (read specs before implementation, promote is the only gate, no command is a gate except promote, stop when unclear)
5. **Commands reference** — all specFlow triggers and their effects
6. **Validation and verification checklists** — structured criteria for subagent execution

## Verification Checklist

When a deep-audit review requires hooks system verification, the following checks apply. See `framework/spec_flow_review.md` §2.8.1 for the review standard context.

### File Existence

- `hooks/session-start` exists and is executable
- `hooks/run-hook.cmd` exists and is a valid polyglot script
- `hooks/hooks.json` exists (Claude Code hook registration by convention)

### Platform Hook Configuration

For each supported platform, the corresponding hook JSON file exists at the install destination and points to the correct `run-hook.cmd` path:

- Claude Code: `hooks/hooks.json` (project root, per Claude Code convention)

### Script Correctness

- `session-start` reads `framework/concepts.md`
- `session-start` JSON-escapes the content correctly
- `session-start` wraps the content in the required preamble
- `session-start` detects platform variables (`CLAUDE_PLUGIN_ROOT`) and outputs the matching JSON format
- `run-hook.cmd` is valid cross-platform polyglot (Windows batch + Unix shell)

### Injected Content Completeness

- `framework/concepts.md` contains all essential governance instructions (triggers, HARD RULES, commands reference, workflow, key terms, validate/verify checklists)

### Platform-Specific Registration

- Claude Code: `.claude-plugin/plugin.json` is the plugin manifest. Hooks are discovered by convention at `hooks/hooks.json`. `specflowctl` installs both.
- OpenCode: `.opencode/plugins/specflow.js` installed by `specflowctl`. OpenCode auto-discovers plugins in `.opencode/plugins/` at startup — no config file registration needed.
- **Consumer path validation**: For every platform plugin that reads files from disk (`.opencode/plugins/specflow.js`), verify that its hardcoded file paths resolve correctly from the plugin runtime's working directory or project root, not from the source-repo layout. See `framework/spec_flow_review.md` Section 2.16.
