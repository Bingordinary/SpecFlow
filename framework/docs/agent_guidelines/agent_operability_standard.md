# Agent Operability Standard

## 1. Purpose

This file defines the review standard for whether `specFlow` governance documents are usable by an agent at reasonable reading cost.

A document passes only when both conditions are true:

1. a capable agent with no prior `specFlow` knowledge can read the document and its explicit links, then choose the correct next action or stop point
2. the document uses no avoidable repetition, local restatement, history, or explanation that does not change execution

Accuracy has priority over brevity.
Brevity is still mandatory once accuracy is preserved.

## 2. Scope

This standard applies to governance documents that define or route behavior, including:

1. entry managed blocks
2. routing policies
3. command policies and command files
4. shared-governance flows
5. review policies
6. process-state contracts
7. tooling execution contracts in the current review scope

It does not review business truth by default.

## 3. Reader Model

Review with this reader model:

1. the reader is a capable executor
2. the reader has the current user request and repository files
3. the reader has no prior `specFlow` concept memory
4. the reader follows explicit links and required read instructions
5. the reader must not infer rules from previous conversations, repository history, directory shape, or ordinary meanings of `specFlow` terms

## 4. Required Checks

### 4.1 Concept Load

The document must define project-specific terms before using them, or link to the owner file that defines them.

Terms must not be left to ordinary interpretation when relevant:

1. `Spec`
2. `unit`
3. `scenario`
4. `stable`
5. `candidate`
6. `_status.md`
7. `repository_mapping`
8. `shared_contract`
9. `shared_ops`
10. `checkpoint`
11. `implementation_change_policy`

### 4.2 Owner And Entry Resolution

The document must let the reader determine:

1. which policy, command, or governance flow owns the request
2. whether an entry is a precise command, a review entry, or natural-language routing
3. which intent fragments may coexist in one request
4. which source-of-truth file resolves object ownership
5. which names are internal flows and not user-facing commands

### 4.3 Action Boundary

The document must state, or link to the rule that states:

1. what may be edited
2. what must not be edited
3. whether lifecycle state may advance
4. whether process files may be written
5. whether implementation files may be modified
6. whether `git_policy.md` controls the commit decision

### 4.4 Stop Boundary

The document must stop the reader instead of letting it guess when any of these are missing or ambiguous:

1. user intent
2. object boundary
3. truth writeback target
4. shared or system boundary
5. prerequisite command
6. implementation-change classification
7. conflicting policy rules

### 4.5 Dependency Closure

Every required dependency must be explicit enough to answer:

1. which file to read first
2. why that file is required
3. which file owns current state
4. which file owns durable writeback
5. which downstream flow must run after checkpoint, writeback, or promotion

### 4.6 Output And Resume Contract

The document must define the required output or stop report:

1. result meaning
2. current state
3. next legal step
4. why the next step is legal
5. checkpoint fields when checkpointing is allowed
6. resume path
7. whether temporary contracts are execution-local or durable truth

### 4.7 Content Economy

The document must use the smallest content that preserves correct execution.

Required rules:

1. do not repeat full command lists, path tables, or lifecycle details when a precise link is enough
2. do not restate another owner document's durable rule unless the local document changes how that rule is used
3. keep examples only when they remove a real routing or execution ambiguity
4. remove patch-note language, design history, and motivation that does not affect the next action
5. remove generic prose that does not change allowed action, forbidden action, stop condition, output, or dependency order
6. if shortening would make the reader guess a `specFlow` concept, keep the clarification and remove duplication elsewhere

Deletion test:

1. if deleting text does not change what the reader reads, decides, does, stops on, reports, or resumes from, the text is redundant
2. if deleting text makes the reader guess an owner, boundary, dependency, or next action, the text is required

## 5. Finding Rules

Report a governance finding when a document can cause a new reader to:

1. start from code when truth writeback is required
2. choose the wrong command or governance flow
3. treat an internal flow or non-command object as a user-facing command
4. guess `unit` or `scenario` ownership from directory shape
5. leave checkpoint or resume behavior unclear
6. treat chat agreement as durable truth
7. skip required downstream reconciliation
8. narrow default review scope without explicit user instruction
9. spend execution or review budget on duplicate explanation that should be a link
10. miss the owner rule because a dependent document restates too much

The finding must name:

1. the failing document
2. the missing or excessive content
3. the likely executor mistake
4. the smallest repair that preserves accuracy while reducing avoidable cost

## 6. Passing Rule

A document passes only when a new reader can answer:

1. what object or flow is governed
2. which terms must not be guessed
3. which file to read first
4. what action is allowed
5. what action is forbidden
6. when to stop
7. what to report
8. how to resume
9. which details are delegated to owner documents
10. why each local section is worth reading

## 7. Review Integration

`spec_flow_review` consumes this standard as a framework-baseline governance standard.

Rules:

1. default `spec_flow_review` must report an agent-operability result
2. the result must cover both execution clarity and content economy
3. a narrowed review that includes routing, command, checkpoint, shared governance, process-state, or entry behavior must apply this standard to the in-scope files
4. a pass claim for in-scope governance documents must not ignore agent-operability failures

`spec_flow_design_review` may use this standard as evidence for human operability, but this file does not change that review's scoring model by itself.
