### Resolution entry

After presenting the summary, offer the user a choice:

```
Found {N} finding(s). How would you like to proceed?
  [1] One by one — explain each, then decide
  [2] Batch — I will give direction for all at once
  [3] Skip — I will handle these separately
```

- **[1] One-by-one** → Interactive Resolution Protocol (§Interactive Resolution Protocol)
- **[2] Batch** → record user's consolidated verdict; no per-finding dialogue
- **[3] Skip** → report findings without resolving; user takes over

### Interactive Resolution Protocol

When the user chooses one-by-one mode, present each finding sequentially.
Resolve one before showing the next.

```
═══════════════════════════════════════════════════
Finding {n} of {N}

Finding: {description}
Suggested direction: {direction}
───────────────────────────────────────────────────
  [1] Agree — apply this direction
  [2] Disagree — specify alternative
  [3] Discuss — I have questions about this one
───────────────────────────────────────────────────
```

On **[3] Discuss**: enter a free-form dialogue. Record a direction when the user states it clearly.

**Per-finding rule:** record the agreed direction before moving to the next finding.

### Progress overview (per-finding gate)

After each finding's direction is recorded, present a progress overview before deciding how to proceed:

```
═══════════════════════════════════════════════════
Progress: {n}/{N} findings resolved

Resolved:
  - {finding.1}: {direction} — direction_recorded
  - {finding.2}: {direction} — direction_recorded

Remaining:
  - {finding.3}: pending
  - {finding.4}: pending
═══════════════════════════════════════════════════
```

### Transition gate (mandatory)

After the progress overview, the agent **must not** automatically advance to the next finding or begin implementation. One of the following must hold before proceeding:

- **User-driven advance:** the user explicitly says "next", "next finding", or an equivalent signal.
- **Agent-initiated confirmation:** if the agent believes the current finding is settled but the user has not issued an advance signal, the agent **must ask**:  
  `"The direction for this finding is recorded. Shall I proceed to the next finding?"`  
  Wait for explicit user confirmation before continuing.

An agent **must not** interpret any local action instruction (e.g., "OK, let's go ahead and delete this section") as:  
(a) a signal to advance to the next finding, or  
(b) a signal to begin global implementation.

### Implementation gate (mandatory)

After all findings are resolved and the completion summary is presented, the agent must not automatically enter the modification phase. The agent **must ask**:

```
All {N} findings have directions recorded. Shall I proceed to implement all agreed changes?
  [Yes] / [No]
```

Proceed to implementation only after the user explicitly confirms `[Yes]`.

**Completion:** after all findings are resolved, present a completion summary:

```
───────────────────────────────────────────────────
All {N} findings resolved:
  {finding.1}: {direction} — {next step}
  {finding.2}: {direction} — {next step}
  ...
───────────────────────────────────────────────────
```
