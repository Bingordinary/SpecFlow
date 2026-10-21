A unit main spec has exactly three top-level `##` sections, in this order:

1. `## Design Narrative` — why the unit is designed this way; narrative
   subsections use `###`.
2. `## Promises` — the unit's stable contract; structured content only.
3. `## Acceptance Items` — the items that prove each promise; structured
   content only.

Rules: exactly one of each section; no other `##` section is allowed; no
`###` subheading inside `## Promises` or `## Acceptance Items`; one optional
`#` title may precede the sections; frontmatter is not a section; the
acceptance item set is the last content of the spec.
