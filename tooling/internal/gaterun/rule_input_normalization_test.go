package gaterun

import (
	"testing"
	"time"
)

func TestLogicalRuleRefForPath(t *testing.T) {
	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{"docs/specs/rules/stable/g_rule_logging.md", "rule:g_rule_logging", true},
		{"docs/specs/rules/candidate/b_rule_auth.md", "rule:b_rule_auth", true},
		{"docs/specs/rules/stable/g_rule_logging.txt", "", false},
		{"docs/specs/rules/other/g_rule_x.md", "", false},
		{"docs/specs/rules/g_rule_x.md", "", false},
		{"docs/specs/rules/stable/sub/g_rule_x.md", "", false},
		{"docs/specs/units/stable/unit_auth.md", "", false},
		{"src/auth.go", "", false},
		{"docs/specs/rules/stable/.md", "", false},
	}
	for _, tc := range cases {
		got, ok := LogicalRuleRefForPath(tc.path)
		if ok != tc.ok || got != tc.want {
			t.Errorf("LogicalRuleRefForPath(%q) = %q, %v; want %q, %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

func TestExtraInputPathsNormalizeRuleDirectoryExpansion(t *testing.T) {
	run := &Run{
		ExtraInputs: []string{"docs/specs/rules/stable", "src"},
		Surfaces: []Surface{
			{Path: "docs/specs/rules/stable", Source: SourceInput, Entries: []Entry{
				{Path: "docs/specs/rules/stable/g_rule_a.md"},
				{Path: "docs/specs/rules/stable/b_rule_x.md"},
			}},
			{Path: "src", Source: SourceInput, Entries: []Entry{{Path: "src/auth.go"}}},
		},
		Refs: []Ref{{Ref: "rule:g_rule_a", Source: SourceInput}},
	}
	got := extraInputPaths(run)
	want := []string{"rule:b_rule_x", "rule:g_rule_a", "src/auth.go"}
	if len(got) != len(want) {
		t.Fatalf("extraInputPaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extraInputPaths = %v, want %v", got, want)
		}
	}
}

// A rule file's physical spelling in the input manifest must plan exactly
// like its logical spelling. A physical path that reached the snapshot would
// be readable by the public code sessions while the tool-recorded whole-file
// evidence rejects it as a rule path — the logical-reference check and the
// recorded evidence contradict each other (issue #60).
func TestPlanNormalizesPhysicalRuleInputToLogicalRef(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "g_rule_logging", "src/a.go", "")
	writeRule(t, repoRoot, "stable", "g_rule_logging")
	writeFile(t, repoRoot, "src/a.go", "package a\n")

	physical := "docs/specs/rules/stable/g_rule_logging.md"
	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, []string{physical}, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.ExtraInputs) != 1 || run.ExtraInputs[0] != "rule:g_rule_logging" {
		t.Fatalf("extra inputs = %v, want the logical rule reference", run.ExtraInputs)
	}
	for _, ref := range run.Refs {
		if ref.Ref == physical {
			t.Fatalf("physical rule path reached the snapshot: %+v", run.Refs)
		}
	}
	ref, ok := refByRef(run, "rule:g_rule_logging")
	if !ok || ref.Source != SourceInput || ref.Resolved != physical {
		t.Fatalf("expected the logical input ref resolved to the rule file, got %+v", run.Refs)
	}
	sawCodeKey := false
	for _, ck := range run.Coverage {
		if ck.Kind != SessionKindCode {
			continue
		}
		sawCodeKey = true
		for _, p := range ck.ReadRefs {
			if p == physical {
				t.Fatalf("code session read refs carry the physical rule path: %v", ck.ReadRefs)
			}
		}
		if !stringInSlice(ck.ReadRefs, "rule:g_rule_logging") {
			t.Fatalf("code session read refs must carry the logical rule reference, got %v", ck.ReadRefs)
		}
	}
	if !sawCodeKey {
		t.Fatalf("fixture must produce a public code key: %+v", run.Coverage)
	}
}
