package main

import (
	"fmt"
	"io"

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

func writeGlobalRuleAdvisories(w io.Writer, items []promote.RulePrerequisite) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(w, "PENDING GLOBAL RULES (advisory):")
	writeRulePrerequisiteItems(w, items)
}
