![specFlow hero](./assets/hero1_min.png)

![SpecFlow title block](./assets/specflow-title-block.svg)

<p>
  <img alt="spec-driven" src="https://img.shields.io/badge/spec-driven-111111?style=for-the-badge&labelColor=111111&color=2F855A">
  <img alt="unit-governed" src="https://img.shields.io/badge/unit-governed-111111?style=for-the-badge&labelColor=111111&color=1F6FEB">
  <img alt="agent-runtime-ready" src="https://img.shields.io/badge/agent-runtime%20ready-111111?style=for-the-badge&labelColor=111111&color=C2410C">
  <img alt="human-and-ai" src="https://img.shields.io/badge/human%20%2B%20AI-collaboration-111111?style=for-the-badge&labelColor=111111&color=7C3AED">
</p>

**English** · [简体中文](./README.zh-CN.md)

---

> ⚠️ **Experimental** — specFlow is still evolving. Fork it and adapt it to how **you** work. Don't treat it as a template — expect things to change.

> In this document, "agent" refers to your AI coding assistant (e.g. OpenCode, Claude Code).

## What Problem It Solves

specFlow turns AI-assisted development from chat-driven improvisation into engineered delivery — through a spec-driven **validate → verify → promote** pipeline that keeps design, implementation, and verification aligned to the same truth.

- **AI sessions have no memory** → spec files are persistent truth across sessions
- **Design has no quality gate** → validate catches incomplete design before it lands
- **Multiple agents can't coordinate** → a unified spec layer gives everyone the same source of truth
- **Implementation drifts from design** → verify checks implementation against spec deterministically

## Install

Copy the following instruction to your agent:

> Read https://raw.githubusercontent.com/Bingordinary/SpecFlow/main/INSTALL.md and follow its instructions to install specFlow in this project.

## Platform Support

specFlow currently supports these agent runtimes:

- **OpenCode** — recommended
- **Claude Code**

Codex support is coming — stay tuned.

## Quick Start

After install, start your agent in the project root and tell it what you want in plain language:

```
You: Add a rate limiter to the auth module.
Agent: I don't see a candidate spec yet. Let me understand the design...
```

The agent reads the specFlow rules automatically, discovers existing truth, and guides you through the flow — validating specs, verifying implementation, and asking for confirmation before promoting. You don't need to memorize commands; the agent suggests the right step at the right time.

## Concepts

| Term | Meaning |
|------|---------|
| **spec** | A written agreement on how something should behave |
| **candidate** | Spec being edited (`docs/specs/units/candidate/`) |
| **stable** | Accepted truth (`docs/specs/units/stable/`) — never edited directly |
| **unit** | One independently governable engineering responsibility |
| **rule** | Formally reusable truth shared across units. Global (`g_`) applies repo-wide; bound (`b_`) applies only to units that reference it via `rule_refs` |
| **promote** | The only gate — copies candidate → stable |

**File existence is state.** A candidate spec exists = being edited. No candidate = not being edited.

## Commands & Workflow

You rarely need to type these triggers yourself — the agent suggests them at the right time. Use a trigger directly when you know what step you want.

### Agent Triggers (say to your agent)

| Trigger | What the agent does |
|---------|---------------------|
| `validate@ {unit}` | Runs a structured quality check against the spec (read-only, no file changes) |
| `verify@ {unit}` | Runs a structured implementation check against the spec (read-only, no file changes) |
| `promote@ {unit}` | Runs validate then verify, then calls `specflowctl promote` if both pass |
| `spec_flow_update` | Pulls the latest specFlow source, updates binaries and hooks, checks project format |

The agent also proactively suggests these at natural transition points: *"Shall I run validate?"* / *"Ready to promote?"*

### Typical Session

```
1. Agent creates/edits candidate spec + code (no gate)
2. You: validate@ → agent checks spec quality
3. You: verify@ → agent checks implementation against spec
4. You: promote@ → validates, verifies, then promotes to stable
5. Next iteration...
```

Natural language works too — describe your goal, and the agent reads repo truth and proposes the next action.

## Update

Run the trigger `spec_flow_update` in your current agent session. Once the update finishes, **start a new agent session** — hooks and rules are re-injected at session start, ensuring the updated content takes effect.
