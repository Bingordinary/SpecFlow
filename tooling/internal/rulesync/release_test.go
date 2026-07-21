package rulesync

import (
	"testing"
)

func TestBareRuleName(t *testing.T) {
	tests := []struct {
		ref      string
		expected string
	}{
		{"b_rule_demo", "b_rule_demo"},
		{"b_rule_demo@1.0.0", "b_rule_demo"},
		{"  b_rule_demo  ", "b_rule_demo"},
	}
	for _, tt := range tests {
		got := bareRuleName(tt.ref)
		if got != tt.expected {
			t.Errorf("bareRuleName(%q) = %q, want %q", tt.ref, got, tt.expected)
		}
	}
}

func TestReplaceRuleRefBareRefUnchanged(t *testing.T) {
	refs := []string{"b_rule_demo"}
	next, changed := replaceRuleRef(refs, "b_rule_demo@0.3.0", "b_rule_demo@0.4.0")
	if changed {
		t.Fatal("bare ref should not trigger replacement")
	}
	if len(next) != 1 || next[0] != "b_rule_demo" {
		t.Fatalf("expected [b_rule_demo], got %v", next)
	}
}

func TestReplaceRuleRefVersionedRefReplaced(t *testing.T) {
	refs := []string{"b_rule_demo@0.3.0"}
	next, changed := replaceRuleRef(refs, "b_rule_demo@0.3.0", "b_rule_demo@0.4.0")
	if !changed {
		t.Fatal("versioned ref should trigger replacement")
	}
	if len(next) != 1 || next[0] != "b_rule_demo@0.4.0" {
		t.Fatalf("expected [b_rule_demo@0.4.0], got %v", next)
	}
}

func TestReplaceRuleRefNoMatch(t *testing.T) {
	refs := []string{"b_rule_other"}
	next, changed := replaceRuleRef(refs, "b_rule_demo@0.3.0", "b_rule_demo@0.4.0")
	if changed {
		t.Fatal("no match should not trigger replacement")
	}
	if len(next) != 1 || next[0] != "b_rule_other" {
		t.Fatalf("expected [b_rule_other], got %v", next)
	}
}

func TestReplaceRuleRefMultipleRefsMixed(t *testing.T) {
	refs := []string{"b_rule_other", "b_rule_demo@0.3.0", "b_rule_demo"}
	next, changed := replaceRuleRef(refs, "b_rule_demo@0.3.0", "b_rule_demo@0.4.0")
	if !changed {
		t.Fatal("should detect change from versioned ref")
	}
	if len(next) != 3 {
		t.Fatalf("expected 3 refs, got %v", next)
	}
	if next[0] != "b_rule_other" {
		t.Fatalf("expected first ref unchanged, got %q", next[0])
	}
	if next[1] != "b_rule_demo@0.4.0" {
		t.Fatalf("expected versioned ref replaced, got %q", next[1])
	}
	if next[2] != "b_rule_demo" {
		t.Fatalf("expected bare ref unchanged, got %q", next[2])
	}
}
