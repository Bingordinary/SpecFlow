package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/promote"
)

func writeRulePrerequisites(w io.Writer, report promote.RulePrerequisites) {

	status := "OK"
	if len(report.Blockers) > 0 {
		status = "BLOCKED"
	}
	fmt.Fprintf(w, "RULE PREREQUISITES: %s\n", status)
	writeRulePrerequisiteItems(w, report.Blockers)
	writeGlobalRuleAdvisories(w, report.Advisories)
}

func writeRulePrerequisiteItems(w io.Writer, items []promote.RulePrerequisite) {
	for _, item := range items {
		fmt.Fprintf(w, "  %s: %s\n", item.RuleID, item.Reason)
	}
}

// writeBlockingRules prints the candidate section's publication blockers once
// per rule. A bound rule blocks every unit that binds it, and the reason comes
// from the rule's own candidate/stable content, so the summary states the rule
// fact at rule level while each affected unit keeps its own `rules: BLOCKED`
// verdict. Detail mode (`fresh --unit`) keeps the per-unit prerequisite view.
func writeBlockingRules(w io.Writer, items []promote.RulePrerequisite) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(w, "BLOCKING RULES (publication prerequisites):")
	writeRulePrerequisiteItems(w, items)
}

func writeGlobalRuleAdvisories(w io.Writer, items []promote.RulePrerequisite) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(w, "PENDING GLOBAL RULES (advisory):")
	writeRulePrerequisiteItems(w, items)
}

// sortedPrerequisites flattens a rule-ID-keyed set into a stable, sorted slice
// for display. Every consumer of the same rule carries the identical reason, so
// the last write for an ID is representative.
func sortedPrerequisites(byID map[string]promote.RulePrerequisite) []promote.RulePrerequisite {
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items := make([]promote.RulePrerequisite, 0, len(ids))
	for _, id := range ids {
		items = append(items, byID[id])
	}
	return items
}
