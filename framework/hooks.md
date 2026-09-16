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
| `framework/concepts.md` | The injected governance content. Contains key terms, workflow, trigger phrases (`validate`, `verify`, `promote`), agent suggestion flow, HARD RULES, and commands reference. |

## Platform Support

The `session-start` script detects the target platform from an explicit argument or environment variables and selects the JSON output format accordingly:

| Platform | Detection | Output Format |
|----------|-----------|---------------|
| Claude Code | `CLAUDE_PLUGIN_ROOT` set AND `COPILOT_CLI` not set | `{ "hookSpecificOutput": { "hookEventName": "SessionStart", "additionalContext": "..." } }` |
| Codex | Argument `codex` | `{ "hookSpecificOutput": { "hookEventName": "SessionStart", "additionalContext": "..." } }` |
| OpenCode | OpenCode plugin (see below) | Message transform via JS plugin |
| Antigravity | Argument `antigravity` or `ANTIGRAVITY` set | `{ "injectSteps": [ { "ephemeralMessage": "..." } ] }` |

> The `COPILOT_CLI` guard covers Copilot CLI, which also sets `CLAUDE_PLUGIN_ROOT`. When `COPILOT_CLI` is set, the Claude-format JSON output is suppressed so Copilot CLI's own session handling does not receive Claude-specific hook JSON.

Codex support covers Codex CLI and the Local environment in the ChatGPT desktop app. Codex Worktree and Cloud environments are outside the current support boundary.

==ATOM_BEGIN:specflowctl_location==
specflowctl is not on PATH. Its binary is at `<tooling-root>/bin/specflowctl-<os>-<arch>`. `<tooling-root>` is `specflow/tooling`. Replace `<os>` and `<arch>` with your platform (e.g. `linux-amd64`, `darwin-arm64`, `windows-amd64.exe`). Use the full path when running specflowctl commands.
==ATOM_END:specflowctl_location==

### Hook Configuration Files

Each platform requires a hook configuration JSON file that registers `session-start` as a startup or pre-invocation trigger. These files are installed by `specflowctl init`:

| File | Install To | Platform | Command |
|------|-----------|----------|---------|
| `hooks/hooks.json` | `hooks/hooks.json` | Claude Code | `"${CLAUDE_PLUGIN_ROOT}/specflow/hooks/run-hook.cmd" session-start` |
| `templates/.codex/hooks.json` | `.codex/hooks.json` | Codex | Resolves the repository root, then runs `specflow/hooks/run-hook.cmd session-start codex` |
| `templates/.agents/plugins/specflow/hooks.json` | `.agents/plugins/specflow/hooks.json` | Antigravity | `../../../specflow/hooks/run-hook.cmd session-start antigravity` |

Claude Code discovers hooks by convention at `${CLAUDE_PLUGIN_ROOT}/hooks/hooks.json`. Codex reads project-scoped hooks from `<repository>/.codex/hooks.json`; the installer merges the SpecFlow `SessionStart` entry into that file and preserves unrelated hooks and top-level settings. Antigravity discovers hooks within the plugin directory at `.agents/plugins/specflow/hooks.json`.

The Codex command defines both Unix and Windows launchers. It sets `additionalContextLimit` to `0` because SpecFlow deliberately injects the complete `framework/concepts.md`, which can exceed Codex's default command-hook context limit. Codex asks the user to review and trust new or changed project hooks before running them.

### Platform Registration

Each platform needs the SpecFlow integration registered so it knows to trigger `session-start` at startup or pre-invocation. The registration mechanism differs by platform:

| Platform | Registration Method | Installed By |
|----------|-------------------|-------------|
| Claude Code | `.claude-plugin/plugin.json` (discovers hooks by convention at `hooks/hooks.json`) | `specflowctl` installs the file from `templates/.claude-plugin/plugin.json` |
| Codex | `.codex/hooks.json` (project-scoped hook configuration) | `specflowctl` merges the managed entry from `templates/.codex/hooks.json` |
| OpenCode | `.opencode/plugins/specflow.js` (auto-discovered by OpenCode) | `specflowctl` installs the file from `templates/.opencode/plugins/specflow.js` |
| Antigravity | `.agents/plugins/specflow/plugin.json` (auto-discovered by Antigravity) | `specflowctl` installs the file from `templates/.agents/plugins/specflow/plugin.json` |

## How session-start Works

1. Reads `framework/concepts.md` from the repository root
2. JSON-escapes the contents (backslash, double-quote, newline, carriage-return, tab)
3. Wraps in a preamble: `"<SPECFLOW_CONCEPTS>\nThis project uses SpecFlow to manage design documents.\n\n**Below is the full SpecFlow framework guide — read it carefully before starting work:**\n\n{concepts_escaped}\n</SPECFLOW_CONCEPTS>"`
4. Detects the platform from the explicit platform argument or environment variables and outputs the correct JSON shape
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
4. **HARD RULES** — six rules (read specs before discussing or changing a topic, promote is the only gate to stable, validate/verify/review check quality and only promote writes, never decide divergence resolution alone, stop when unclear, fork must use specflowctl fork)
5. **Commands reference** — all specFlow triggers and their effects
6. **Checklist references** — pointers to the validate/verify/review checklists the agent reads when a trigger fires

## Verification Checklist

When a deep-audit review requires hooks system verification, the following checks apply. See `framework/spec_flow_review.md` §2.8.1 for the review standard context.

### File Existence

- `hooks/session-start` exists and is executable
- `hooks/run-hook.cmd` exists and is a valid polyglot script
- `hooks/hooks.json` exists (Claude Code hook registration by convention)
- `templates/.codex/hooks.json` exists (Codex project-hook template)

### Platform Hook Configuration

For each supported platform, the corresponding hook JSON file exists at the install destination and points to the correct `run-hook.cmd` path:

- Claude Code: `hooks/hooks.json` (project root, per Claude Code convention)
- Codex: `.codex/hooks.json` (merged into the consumer project without replacing unrelated hooks)
- Antigravity: `.agents/plugins/specflow/hooks.json` (inside plugin directory)

### Script Correctness

- `session-start` reads `framework/concepts.md`
- `session-start` JSON-escapes the content correctly
- `session-start` wraps the content in the required preamble
- `session-start` detects platform variables or arguments (`CLAUDE_PLUGIN_ROOT`, `codex`, `antigravity`) and outputs the matching JSON format
- `run-hook.cmd` is valid cross-platform polyglot (Windows batch + Unix shell)

### Injected Content Completeness

- `framework/concepts.md` contains all essential governance instructions (triggers, HARD RULES, commands reference, workflow, key terms, and checklist references)

### Platform-Specific Registration

- Claude Code: `.claude-plugin/plugin.json` is the plugin manifest. Hooks are discovered by convention at `hooks/hooks.json`. `specflowctl` installs both.
- Codex: `.codex/hooks.json` contains the project-scoped `SessionStart` hook for Codex CLI and the desktop app's Local environment. `specflowctl` preserves unrelated Codex settings and hooks, replaces only the managed SpecFlow entry, and rejects invalid existing JSON instead of overwriting it. The entry handles `startup`, `resume`, `clear`, and `compact`, provides Unix and Windows commands, and sets `additionalContextLimit` to `0`. Worktree and Cloud environments are not supported.
- OpenCode: `.opencode/plugins/specflow.js` installed by `specflowctl`. OpenCode auto-discovers plugins in `.opencode/plugins/` at startup — no config file registration needed.
- Antigravity: `.agents/plugins/specflow/plugin.json` is the plugin manifest and `hooks.json` defines lifecycle hooks. `specflowctl` installs both.
- **Consumer path validation**: For every platform integration that reads files from disk (`.opencode/plugins/specflow.js`, `.codex/hooks.json`), verify that its paths resolve correctly from the plugin runtime's working directory or repository root, not from the source-repo layout. See `framework/spec_flow_review.md` Section 2.16.
