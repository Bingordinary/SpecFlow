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

**Completion:** after all findings are resolved, present a completion summary:

```
───────────────────────────────────────────────────
All {N} findings resolved:
  {finding.1}: {direction} — {next step}
  {finding.2}: {direction} — {next step}
  ...
───────────────────────────────────────────────────
```
