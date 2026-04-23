# Candidate Plan Drafts

This directory stores non-consumable planning working artifacts for `unit_plan`.

Rules:

1. Each unit may have one `draft/{unit}.md`.
2. Draft plans are not valid inputs to `unit_impl` or `unit_verify`.
3. A draft plan may record:
   - changed execution surfaces for the round
   - current known paths and target paths
   - legacy dependencies that look ready for retirement
   - known implementation findings
   - open implementation unknowns
   - blocking summary
   - resume signal
   - research notes
4. Minimum fields are:
   - `object_ref`
   - `truth_file_ref`
   - `truth_version_ref`
   - `truth_fingerprint`
   - `fallback_reason_code`
   - `blocking_summary`
   - `resume_signal`
   - `changed_execution_surfaces`
   - `current_known_paths`
   - `target_paths`
   - `legacy_candidates`
   - `retirement_goals`
   - `known_findings`
   - `open_modeling_unknowns`
   - `slice_cutover_plan`
   - `research_notes`
5. Draft plans do not carry gate-bearing semantics and do not replace the active plan.
6. Draft plans may carry implementation-fact accumulation and implementation convergence planning only; if planning discovers missing behavior truth, boundary truth, or acceptance truth, the round must go back to `unit_check`.
7. `truth-fallback`, `unit_fork`, `unit_promote`, recovery, and `Candidate=no` must delete the corresponding draft file.
8. After a round successfully writes `active/{unit}.md`, the corresponding draft file should normally be deleted.
