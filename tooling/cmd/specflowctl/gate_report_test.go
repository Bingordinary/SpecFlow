package main

import (
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// TestExtractFindingsSkeletonIndentedEntries is the regression guard for the
// unified report skeleton's finding format: entries live at one indentation
// level under `Findings:` and their detail lines are deeper. A sibling entry
// must open a new finding, never be absorbed as the previous entry's detail.
func TestExtractFindingsSkeletonIndentedEntries(t *testing.T) {
	report := "Findings:\n" +
		"  [P2] src/a.go:1 — advisory issue (actionable)\n" +
		"    spec_context: design says X\n" +
		"  [P0] src/a.go:2 — blocking issue (actionable)\n" +
		"    spec_context: design says Y\n"
	got := extractFindings(report)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %#v", len(got), got)
	}
	if got[0].severity != "P2" || got[1].severity != "P0" {
		t.Fatalf("expected severities P2, P0 — got %s, %s", got[0].severity, got[1].severity)
	}
	if strings.Contains(got[0].detail, "blocking issue") {
		t.Fatalf("first finding's detail swallowed the second finding:\n%s", got[0].detail)
	}
	if !strings.Contains(got[0].detail, "spec_context: design says X") {
		t.Fatalf("first finding's detail lost its own detail line:\n%s", got[0].detail)
	}
	if !strings.Contains(got[1].detail, "spec_context: design says Y") {
		t.Fatalf("second finding's detail lost its own detail line:\n%s", got[1].detail)
	}
}

// TestExtractFindingsUnindentedEntries keeps the flat spelling working: both
// entries at column zero, details indented below them.
func TestExtractFindingsUnindentedEntries(t *testing.T) {
	report := "[P2] src/a.go:1 — advisory issue (actionable)\n" +
		"  spec_context: X\n" +
		"\n" +
		"[P0] src/a.go:2 — blocking issue (actionable)\n" +
		"  spec_context: Y\n"
	got := extractFindings(report)
	if len(got) != 2 || got[0].severity != "P2" || got[1].severity != "P0" {
		t.Fatalf("expected 2 findings P2/P0, got %#v", got)
	}
}

// TestExtractFindingsDeeperQuotedFindingStaysDetail pins the other half of the
// rule: a deeper-indented `[Px]` line is a quoted detail line, not a new
// finding entry.
func TestExtractFindingsDeeperQuotedFindingStaysDetail(t *testing.T) {
	report := "  [P2] src/a.go:1 — advisory issue (actionable)\n" +
		"    spec_context: the spec quotes this older record:\n" +
		"      [P1] src/a.go:2 — quoted from the spec, not a finding\n" +
		"  [P0] src/a.go:3 — real blocking issue (actionable)\n"
	got := extractFindings(report)
	if len(got) != 2 {
		t.Fatalf("expected the deeper quoted line to stay detail — got %d findings: %#v", len(got), got)
	}
	if got[0].severity != "P2" || !strings.Contains(got[0].detail, "[P1] src/a.go:2") {
		t.Fatalf("expected the quoted line in the first finding's detail: %#v", got[0])
	}
	if got[1].severity != "P0" {
		t.Fatalf("expected the second entry to be P0, got %s", got[1].severity)
	}
}

// TestExtractVerdictItemIDMetacharacters verifies the item-id pattern is
// escaped: an id containing a regex metacharacter must not fail compilation
// or alias another id's line.
func TestExtractVerdictItemIDMetacharacters(t *testing.T) {
	spec := &gaterun.SessionSpec{Kind: gaterun.SessionKindItem, CheckKeys: []string{"auth.co(re"}}
	token, _, _, err := extractVerdict(spec, "auth.co(re", "- auth.co(re: ALIGNED — src/auth.go:1\n")
	if err != nil {
		t.Fatalf("an item id with a regex metacharacter must not break verdict parsing: %v", err)
	}
	if token != "ALIGNED" {
		t.Fatalf("expected ALIGNED, got %q", token)
	}
}

// TestHasReasonAcceptsCJK guards issue #89: a reason written entirely in CJK
// (with punctuation) is a real reason. The test must not depend on an ASCII
// token being present, or Chinese-language projects get spurious
// "carries no reason" rejections.
func TestHasReasonAcceptsCJK(t *testing.T) {
	line := "5. Acceptance coverage & correctness: PASS — 见下列四项子检查，正文/附录行为域与验收项双向对应成立，改动点均有承载，无子检查失败。"
	if !hasReason(line, "PASS") {
		t.Fatalf("an all-CJK reason must count as a reason: %q", line)
	}
}

// TestHasReasonRejectsPunctuationOnly pins the other half of the rule: a
// verdict followed by nothing but punctuation still carries no reason.
func TestHasReasonRejectsPunctuationOnly(t *testing.T) {
	for _, line := range []string{
		"5. Acceptance coverage: PASS — 。，！",
		"5. Acceptance coverage: PASS — ---",
		"5. Acceptance coverage: PASS — ",
	} {
		if hasReason(line, "PASS") {
			t.Fatalf("a verdict followed by punctuation only must not count as a reason: %q", line)
		}
	}
}

// TestValidateUnitAcceptanceBodyAcceptsCJKReasons covers the Check 5 sub-check
// call site of hasReason (#89): all-CJK sub-check reasons must validate.
func TestValidateUnitAcceptanceBodyAcceptsCJKReasons(t *testing.T) {
	report := "5a. coverage: PASS — 正文与验收项双向对应成立\n" +
		"5b. correctness: PASS — 改动点均有承载，无遗漏\n" +
		"5e. appendix: PASS — 附录行为域与正文一致\n" +
		"5h. disposition: PASS — 无子检查失败\n"
	if err := validateUnitAcceptanceBody(report); err != nil {
		t.Fatalf("all-CJK sub-check reasons must validate: %v", err)
	}
}

// TestValidateUnitAcceptanceBodyRejectsPunctuationOnlyReason keeps the
// pure-punctuation rejection intact after the #89 widening.
func TestValidateUnitAcceptanceBodyRejectsPunctuationOnlyReason(t *testing.T) {
	report := "5a. coverage: PASS — 。，\n" +
		"5b. correctness: PASS — 改动点均有承载\n" +
		"5e. appendix: PASS — 附录行为域与正文一致\n" +
		"5h. disposition: PASS — 无子检查失败\n"
	if err := validateUnitAcceptanceBody(report); err == nil {
		t.Fatal("a punctuation-only sub-check reason must be rejected")
	}
}
