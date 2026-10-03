# Spec Removal

This file owns deletion of units, rules, and appendices in an installed project. Normal editing and publication remain governed by the unit and rule workflows.

## Agent decision

Read the original spec and necessary code. Establish whether the responsibility or constraint has been canceled, transferred, or replaced, and identify the exact objects to delete. A lack of consumers is evidence to investigate, never a deletion reason by itself. Rules with future value can remain without consumers; explain that value in their body when useful.

Deletion must fit the user's authorized change. Reuse clear authorization already given; discuss only unclear intent or scope. Do not add a separate deletion approval protocol. Explain the deletion basis and the selected objects in the ordinary change report. Search prose and code for semantic references and effects; the tool checks structured references only.

Deleting an object does not require fork, validate, verify, or promote. Any surviving unit or rule that changes still follows its normal editing and publication path. The tool never selects another unit or rule and never deletes business code.

## Command and routing

```text
specflowctl remove [--unit NAME]... [--rule ID]...
  [--appendix UNIT:unit_UNIT_NAME.md]...
  [--layer all|candidate] [--dry-run] [--repo-root PATH]
```

Targets may be repeated or mixed; the batch is deduplicated. Use full rule IDs and full appendix filenames. `--layer all` is the default and selects both stable and candidate. `--layer candidate` removes drafts while preserving stable. There is no force option.

Chat triggers:

- `remove@{unit}` and `remove@{rule}`: resolve the exact existing object in both layers, then pass `--unit` or `--rule`. If the name identifies multiple objects, discuss the ambiguity before execution; do not use substring matching to choose a deletion target.
- `remove@{unit}:appendix:{filename.md}`: pass `--appendix {unit}:{filename.md}`.
- Natural-language deletion requests use the same command and decision rules.

`--dry-run` is an optional preview using the same target expansion and checks. It lists files, current-record cleanup, task invalidations, and blocking references without writing. Preview is not a required step.

## Reference closure and ordering

The tool reads remaining stable and candidate specs and checks `unit_refs`, `rule_refs`, acceptance-item `affects.dependencies` and `affects.appendices`, `evidence_appendix_ref`, `affects.rules`, and `rule_exceptions` rule IDs. References within the deletion batch do not block it. References are resolved against the resulting files, using the existing resolver rules: unit names prefer candidate with stable fallback, bound rule IDs may resolve in either layer, global rules require stable, appendix filenames resolve in the referring file's layer, and physical spec paths require that exact file. A candidate-only deletion is allowed when stable can still legally satisfy a logical reference.

Deleting all copies cannot leave a reference in either layer. A candidate that already dropped a reference does not hide the stable predecessor's reference. Edit and normally publish surviving referrers before final deletion.

For appendix replacement, use this order:

1. Edit candidate to replace the design and remove old appendix references, including `evidence_appendix_ref` where applicable.
2. Run `remove --appendix UNIT:FILENAME.md --layer candidate` to remove the old draft appendix. Stable references can still resolve to the stable appendix.
3. Validate and verify the remaining unit through the normal user-triggered gates, then promote it under the normal publication authorization.
4. Run `remove --appendix UNIT:FILENAME.md` to delete the remaining stable appendix.

This prevents old candidate appendix content from participating in checks of the new design. Promote copies the remaining candidate files and does not remove omitted stable appendices or rules.

## Tool integrity and records

A whole-unit deletion includes all appendices whose `unit` frontmatter identifies that unit in the selected layers, including exempt appendices. Similar filename prefixes do not establish ownership. Ambiguous ownership, unreadable required files, unresolvable references, and paths outside the project stop execution before writes, with the file and reason reported. A target with no matching file or current record in the selected layers is an error.

Whole unit/rule deletion clears its selected-layer validation caches. Removing both layers also clears its publication baseline. An appendix deletion clears its owner's caches in the affected layers, preserving its code baseline. Removing a unit from both layers also removes pending deferred findings assigned to that unit; findings owned by other units remain even when this unit was their source.

Deletion shares the gate-state mutation lock. Affected unfinished runs are invalidated in the same transaction as file deletion and record cleanup, so they cannot write obsolete caches afterward. Consumed runs, historical reports, shared judgment records, and shared task history are preserved; there is no history garbage collection.

The tool checks first, performs one transaction, and checks the deletion list and remaining references again before reporting success. Controlled write failures roll back the transaction. Reports state concrete checks and execution results; they never infer that an object should be deleted because it has no consumers. No exit-state file, new repair workflow, or automatic recovery protocol is introduced.

## Upgrade from the former exit mechanisms

Follow `framework/operations/update.md`. Move useful retention rationale into rule prose and remove the former retention metadata. Have the agent inspect legacy retired files under the decision rules above and identify explicit deletion targets. Execute only when the user's authorization covers that deletion; otherwise report the unresolved intent. New tooling has no retired-status deletion or retention-based selection branch.
