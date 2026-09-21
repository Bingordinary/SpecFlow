# Hooks Injection System

SpecFlow injects governance content into agent sessions at startup through a platform-independent hook mechanism. The hook-injected content (`framework/concepts.md`) is the session bootstrap: the primary instruction source that states the operating rules, the state model, and the trigger routing table for all specFlow-governed work.

This file is the single authoritative reference for the hooks system. It owns two contracts:

1. **Bootstrap contract** — what the injected content is and must contain.
2. **Adapter injection contract** — the runtime-neutral rules every platform adapter follows when delivering the bootstrap.

## Injection Chain

```
Platform hook config (JSON) or plugin
  └── hooks/run-hook.cmd (hook platforms) or platform plugin code
        └── hooks/session-start (hook platforms)
              └── reads framework/concepts.md
                    └── outputs JSON → injected into agent session context
```

The injected content arrives as platform-specific JSON or a message transform which the agent runtime loads into the session prompt. The agent does not need to read `framework/concepts.md` from disk — its content is already present in the session context. The bootstrap routes each trigger to its command package, and the agent reads the package files from disk only when a trigger fires.

## Core Files

| File | Role |
|------|------|
| `hooks/session-start` | Shell script: reads `framework/concepts.md`, JSON-escapes it, wraps it in a governance preamble, and outputs platform-specific JSON. |
| `hooks/run-hook.cmd` | Cross-platform polyglot wrapper (valid Windows batch + Unix shell). On Windows, finds Git Bash and delegates the hook script; on Unix, executes it directly. |
| `framework/concepts.md` | The injected session bootstrap. Contains identity, state model, key terms, default editing mode, HARD RULES, the trigger routing table, and framework identity information. Phase execution procedures are not contained here; they are command packages read on demand. |

## Bootstrap Contract

The injected bootstrap (`framework/concepts.md`) must contain the content categories below and nothing else. It is the entry control point: after reading it alone, an executor must be able to determine what action to take now and which command package to read when a trigger fires.

1. **Identity and purpose** — what SpecFlow is and that spec documents are the consensus protocol between the user and the agent.
2. **State model** — file existence is state; stable/candidate layers; distinct truth roles without an automatic winner; reference priority outside verify; key terms.
3. **Default editing mode** — read-only requests stay read-only; editing does not grant write permission; fork rules and the spec-first planning requirement (declare the candidate spec and appendices before code; state the Spec Impact Assessment when no spec change is needed).
4. **HARD RULES** — read specs before discussing/changing a topic; gates are user-triggered; promote checks the target's applicable gates; quality gates do not edit truth; documented stable deletion/migration exceptions stay explicit; never decide divergence alone; stop when unclear; fork through `specflowctl`.
5. **Trigger routing table** — one row per trigger or behaviorally identical trigger family, with exact syntax, first action, and full command-package paths. Routing row + listed packages (+ `gate-plan` output for full/delta/repair gates) must carry every instruction the step needs.
6. **Infrastructure** — `specflowctl` location, framework path convention, and framework identity (installed repository commit and `tooling/fingerprint.txt`).

The bootstrap must not contain phase execution procedures: checklists, packet sequences, delta/failure-recovery semantics, full command references, disclosure lookup tables, or operation-scope rule bodies. Those are owned by the command package files the routing table names.

**Size budget.** The injected payload (preamble + `framework/concepts.md`) must stay within 9,000 characters. The budget is a 10% margin under the two platform hook-output caps that bind injection: Claude Code caps hook `additionalContext` at 10,000 characters (no setting raises it; oversized output is replaced by a preview and a file path), and Codex spills `additionalContext` above 2,500 tokens by default (`ceil(bytes/4)` ≈ 10,000 bytes). The tooling closure test computes the payload and asserts both bounds; no adapter may raise or disable a platform cap to carry a larger bootstrap.

**Content conservation.** The six categories above must remain present with their normative force intact. Content may leave the bootstrap only in two ways: it is already present in a command package the routing table names, or it is relocated to one in the same change. Silent removal of unique normative content is not allowed — a change that reduces the bootstrap must account for every removed block in the named packages.

## Adapter Injection Contract

This contract is runtime-neutral. It defines what any platform adapter must guarantee when delivering the bootstrap; it does not prescribe how a platform injects. Platform implementations remain independent.

1. **Fresh bootstrap by default.** An adapter injects the current on-disk `framework/concepts.md` content. The bootstrap is bounded by the Bootstrap Contract's payload budget; caching is not required for performance. An adapter must not serve a static bootstrap copy across sessions or turns.
2. **No stale content.** A framework content change (`spec_flow_update`, a pull, or a local edit) must be visible without restarting the host process. An adapter that cannot guarantee re-reading must not cache.
3. **Directory-keyed and version-keyed caching, if caching exists.** If a platform mechanism makes per-injection reads impossible, a cache key must include at least the project directory and a framework content identity (bootstrap content hash, mtime+size, or the recorded framework fingerprint). A change to any framework content must invalidate the entry, and one process serving multiple project directories must not share cache entries between them.
4. **Framework identity inputs.** The installed framework repository commit and `tooling/fingerprint.txt` are the recorded version identifiers. Adapters may use them, or the bootstrap file's own content identity, as cache keys.
5. **Platform parameters.** Adapters use the platform's default injection limits; no adapter may raise or disable a platform hook-output cap to make a larger bootstrap fit. The Bootstrap Contract's payload budget keeps the bootstrap inside the supported platform hook-output caps, and the tooling closure test enforces it.
6. **Independent implementations.** Each adapter delivers the same bootstrap contract in its own mechanism: Claude Code, Codex, and Antigravity through `hooks/session-start`; OpenCode through its message-transform plugin.

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

The Codex command defines both Unix and Windows launchers. It uses the platform's default command-hook context limit (`additionalContextLimit` unset = the 2,500-token spill threshold); the Bootstrap Contract's payload budget — enforced by the tooling closure test — keeps the bootstrap under that threshold. Codex asks the user to review and trust new or changed project hooks before running them.

### Platform Registration

Each platform needs the SpecFlow integration registered so it knows to trigger `session-start` at startup or pre-invocation. The registration mechanism differs by platform:

| Platform | Registration Method | Installed By |
|---------|-------------------|-------------|
| Claude Code | `.claude-plugin/plugin.json` (discovers hooks by convention at `hooks/hooks.json`) | `specflowctl` installs the file from `templates/.claude-plugin/plugin.json` |
| Codex | `.codex/hooks.json` (project-scoped hook configuration) | `specflowctl` merges the managed entry from `templates/.codex/hooks.json` |
| OpenCode | `.opencode/plugins/specflow.js` (auto-discovered by OpenCode) | `specflowctl` installs the file from `templates/.opencode/plugins/specflow.js` |
| Antigravity | `.agents/plugins/specflow/plugin.json` (auto-discovered by Antigravity) | `specflowctl` installs the file from `templates/.agents/plugins/specflow/plugin.json` |

## How session-start Works

1. Reads `framework/concepts.md` from the repository root at invocation time (fresh read — no cache)
2. JSON-escapes the contents (backslash, double-quote, newline, carriage-return, tab)
3. Wraps in a preamble: `"<SPECFLOW_CONCEPTS>\nThis project uses SpecFlow to manage design documents.\n\n**Below is the SpecFlow session bootstrap — read it before starting work. It states the operating rules and routes each supported trigger to its command package, which you read on demand:**\n\n{concepts_escaped}\n</SPECFLOW_CONCEPTS>"`
4. Detects the platform from the explicit platform argument or environment variables and outputs the correct JSON shape
5. Returns exit code 0 on success

## How run-hook.cmd Works

1. Receives the hook script name as its first argument (e.g. `session-start`)
2. On Windows: searches for `bash.exe` in `C:\Program Files\Git\bin`, `C:\Program Files (x86)\Git\bin`, and PATH, then executes the script via bash
3. On Unix: executes the script directly via bash
4. If bash is not found on Windows, exits silently (plugin still works, just without SessionStart injection)

Hook scripts use extensionless filenames (`session-start` not `session-start.sh`) to avoid Claude Code's Windows auto-detection, which prepends `bash` to any command containing `.sh`.

## Injected Content

The full text of `framework/concepts.md` is injected. Its required content categories are defined by the Bootstrap Contract above. In summary it must contain:

1. **Identity and purpose** — the consensus-protocol role of spec documents
2. **State model** — file existence is state, two layers, truth roles, reference priority, key terms
3. **Default editing mode** — read-only boundary, fork rules, and the spec-first planning requirement
4. **HARD RULES** — the binding behavior rules
5. **Trigger routing table** — supported triggers and their command packages
6. **Infrastructure** — `specflowctl` location, framework path convention, framework identity

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

- `session-start` reads `framework/concepts.md` at invocation time
- `session-start` JSON-escapes the content correctly
- `session-start` wraps the content in the required preamble
- `session-start` detects platform variables or arguments (`CLAUDE_PLUGIN_ROOT`, `codex`, `antigravity`) and outputs the matching JSON format
- `run-hook.cmd` is valid cross-platform polyglot (Windows batch + Unix shell)

### Bootstrap Content and Routing Closure

- `framework/concepts.md` contains the six Bootstrap Contract content categories (identity, state model, default editing mode, HARD RULES, trigger routing table, infrastructure)
- every trigger in the routing table names at least one command package file, and every named package file exists and is non-empty
- the routing table covers the supported trigger set (the deterministic closure test in the tooling asserts the golden trigger set)
- the bootstrap contains no phase execution procedures (checklist bodies, packet sequences, delta/failure-recovery semantics, full command reference, disclosure lookup tables, operation-scope rule bodies) — those live in the command package files
- the trigger-to-package mapping agrees with `framework/commands.md` and the phase documents (no contract drift per `framework/spec_flow_review.md` Section 2.6)
- the bootstrap stays within the Bootstrap Contract size budget — the tooling closure test computes the injected payload (preamble + `framework/concepts.md`) and asserts the character budget and the Codex `ceil(bytes/4) ≤ 2,500` bound
- when a change reduces the bootstrap, every removed block is present in, or was relocated in the same change to, a command package named by the routing table (content conservation — no silent semantic loss)

### Adapter Injection Contract

- for every platform adapter (Claude Code, Codex, Antigravity, OpenCode), verify the adapter injects the current on-disk bootstrap and does not serve a static copy across sessions or turns
- if an adapter caches, verify the cache key contains the project directory and a framework content identity, that framework content changes invalidate the entry, and that one process serving multiple directories does not share entries
- verify the Codex hook entry uses the platform's default context limit (no `additionalContextLimit` override)
- verify the OpenCode plugin performs a fresh read per injection with no module-level bootstrap cache

### Platform-Specific Registration

- Claude Code: `.claude-plugin/plugin.json` is the plugin manifest. Hooks are discovered by convention at `hooks/hooks.json`. `specflowctl` installs both.
- Codex: `.codex/hooks.json` contains the project-scoped `SessionStart` hook for Codex CLI and the desktop app's Local environment. `specflowctl` preserves unrelated Codex settings and hooks, replaces only the managed SpecFlow entry, and rejects invalid existing JSON instead of overwriting it. The entry handles `startup`, `resume`, `clear`, and `compact`, provides Unix and Windows commands, and uses the default context limit. Worktree and Cloud environments are not supported.
- OpenCode: `.opencode/plugins/specflow.js` installed by `specflowctl`. OpenCode auto-discovers plugins in `.opencode/plugins/` at startup — no config file registration needed. The plugin reads the bootstrap at message-transform time.
- Antigravity: `.agents/plugins/specflow/plugin.json` is the plugin manifest and `hooks.json` defines lifecycle hooks. `specflowctl` installs both.
- **Consumer path validation**: For every platform integration that reads files from disk (`.opencode/plugins/specflow.js`, `.codex/hooks.json`), verify that its paths resolve correctly from the plugin runtime's working directory or repository root, not from the source-repo layout. See `framework/spec_flow_review.md` Section 2.16.
