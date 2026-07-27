![specFlow hero](./assets/hero1_min.png)

![SpecFlow title block](./assets/specflow-title-block.svg)

<p>
  <img alt="规格驱动" src="https://img.shields.io/badge/spec-driven-111111?style=for-the-badge&labelColor=111111&color=2F855A">
  <img alt="单元治理" src="https://img.shields.io/badge/unit-governed-111111?style=for-the-badge&labelColor=111111&color=1F6FEB">
  <img alt="Agent 运行时就绪" src="https://img.shields.io/badge/agent-runtime%20ready-111111?style=for-the-badge&labelColor=111111&color=C2410C">
  <img alt="人机协作" src="https://img.shields.io/badge/human%20%2B%20AI-collaboration-111111?style=for-the-badge&labelColor=111111&color=7C3AED">
</p>

[English](./README.md) · **简体中文**

---

> ⚠️ **实验项目** — specFlow 还在发展期，结构和规则都可能调整。如果你要用，建议 fork 后按自己的方式改造，而不是直接当作模板使用。

> 本文档中，"agent" 指你的 AI 编码助手（例如 OpenCode、Claude Code）。

## 解决的问题

specFlow 把 AI 辅助开发从聊天驱动的即兴编程变成有门控的工程交付——通过 spec-driven 的 **validate → verify → promote** 流水线，让设计、实现、验证之间的真相始终对齐。

- **AI 会话没有记忆** → spec 文件是跨会话的持久真相
- **设计缺乏质量门控** → validate 在落地前卡住不完整的设计
- **多个 agent 无法协调** → 统一的 spec 层给人跟 AI 同一个真相源
- **实现容易偏离设计** → verify 确定性检查实现与 spec 是否一致

## 安装

将以下指令复制给你的 agent：

> 读取 https://raw.githubusercontent.com/Bingordinary/SpecFlow/main/INSTALL.md 并按照其中的指引在当前项目中安装 specFlow。

## 平台支持

specFlow 目前支持以下 agent runtime：

- **OpenCode** — 推荐配合使用
- **Claude Code**

Codex 支持正在推进中，敬请期待。

## 快速开始

安装后，在项目根目录启动你的 agent，直接用自然语言说你的需求：

```
你：给 auth 模块加一个 rate limiter。
Agent：没有找到 candidate spec，先了解一下设计...
```

Agent 会自动加载 specFlow 规则、发现已有真相、并引导你走完整条流程——验证 spec、检查实现、在 promote 前跟你确认。你不需要记住命令；agent 会在合适的时机提示你下一步做什么。

## 概念

| 术语 | 含义 |
|------|------|
| **spec** | 一份关于某个功能应该怎么跑的书面约定 |
| **candidate** | 正在编辑的 spec（`docs/specs/units/candidate/`） |
| **stable** | 已通过的真相（`docs/specs/units/stable/`）——从不直接编辑 |
| **unit** | 一块独立可治理的工程责任 |
| **rule** | 跨 unit 复用的正式共享约束。全局规则（`g_`）作用于整个仓库；绑定规则（`b_`）只作用于通过 `rule_refs` 引用它的 unit |
| **promote** | 唯一的门控——candidate → stable |

**文件存在即状态。** candidate spec 存在 = 在编辑。不存在 = 没在改。

## 命令 & 工作流程

这些触发词通常不需要你手动输入——agent 会在合适的时机主动建议。当你清楚下一步要做什么时，可以直接说出触发词。

### Agent 触发词（对 agent 说）

| 触发词 | agent 做什么 |
|--------|-------------|
| `validate@ {unit}` | 对 spec 执行结构化质量检查（只读，不改文件） |
| `verify@ {unit}` | 对实现执行结构化一致性检查（只读，不改文件） |
| `promote@ {unit}` | 先 validate 再 verify，都通过后调 `specflowctl promote` |
| `spec_flow_update` | 拉取最新 specFlow，更新二进制和 hooks，检查项目格式 |

Agent 也会在适当时候主动询问：*"需要跑 validate 吗？"* / *"要 promote 吗？"*

### 典型流程

```
1. Agent 创建/编辑 candidate spec + 代码（没有门控）
2. 你说 validate@ → agent 检查 spec 质量
3. 你说 verify@ → agent 检查实现与 spec 是否一致
4. 你说 promote@ → 验证后 promote 到 stable
5. 进入下一轮迭代...
```

自然语言也可以——描述你的目标，agent 会读取仓库真相并建议下一步操作。

## 更新

在当前 agent 会话中运行 `spec_flow_update` 触发更新。更新完成后，**启动一个新的 agent 会话**——hooks 和规则会在会话启动时重新注入，确保更新内容生效。
