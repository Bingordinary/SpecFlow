package rulesync

import (
	"testing"
)

func TestSelectedRuleRefsForObjectBareRefVersionedScoped(t *testing.T) {
	sharedFilesByRef := map[string]sharedFile{
		"b_rule_demo@0.1.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "stable", VersionRef: "b_rule_demo@0.1.0"},
		"b_rule_demo@0.2.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "candidate", VersionRef: "b_rule_demo@0.2.0"},
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)

	// objectRefs are bare names from unit frontmatter;
	// scopedRefs are versioned refs from CLI --rule-refs.
	result, err := selectedRuleRefsForObject(
		[]string{"b_rule_demo"},
		[]string{"b_rule_demo@0.2.0"},
		nil,
		sharedFilesByRef,
		sharedFilesByID,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "b_rule_demo" {
		t.Fatalf("expected [b_rule_demo], got %v", result)
	}
}

func TestSelectedRuleRefsForObjectBareRefBareScoped(t *testing.T) {
	sharedFilesByRef := map[string]sharedFile{
		"b_rule_demo@0.1.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "stable", VersionRef: "b_rule_demo@0.1.0"},
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)

	result, err := selectedRuleRefsForObject(
		[]string{"b_rule_demo"},
		[]string{"b_rule_demo"},
		nil,
		sharedFilesByRef,
		sharedFilesByID,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "b_rule_demo" {
		t.Fatalf("expected [b_rule_demo], got %v", result)
	}
}

func TestSelectedRuleRefsForObjectBareRefScopedID(t *testing.T) {
	sharedFilesByRef := map[string]sharedFile{
		"b_rule_demo@0.1.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "stable", VersionRef: "b_rule_demo@0.1.0"},
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)

	result, err := selectedRuleRefsForObject(
		[]string{"b_rule_demo"},
		nil,
		[]string{"b_rule_demo"},
		sharedFilesByRef,
		sharedFilesByID,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "b_rule_demo" {
		t.Fatalf("expected [b_rule_demo], got %v", result)
	}
}

func TestSelectedRuleRefsForObjectNoMatch(t *testing.T) {
	sharedFilesByRef := map[string]sharedFile{
		"b_rule_demo@0.1.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "stable", VersionRef: "b_rule_demo@0.1.0"},
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)

	result, err := selectedRuleRefsForObject(
		[]string{"other_rule"},
		[]string{"b_rule_demo@0.2.0"},
		nil,
		sharedFilesByRef,
		sharedFilesByID,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
}

func TestSelectedRuleRefsForObjectMultipleLayersScopedID(t *testing.T) {
	sharedFilesByRef := map[string]sharedFile{
		"b_rule_demo@0.1.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "stable", VersionRef: "b_rule_demo@0.1.0"},
		"b_rule_demo@0.2.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "candidate", VersionRef: "b_rule_demo@0.2.0"},
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)

	_, err := selectedRuleRefsForObject(
		[]string{"b_rule_demo"},
		nil,
		[]string{"b_rule_demo"},
		sharedFilesByRef,
		sharedFilesByID,
	)
	if err == nil {
		t.Fatal("expected error for multiple shared layers")
	}
}

func TestSelectedRuleRefsForObjectVersionedObjectRef(t *testing.T) {
	sharedFilesByRef := map[string]sharedFile{
		"b_rule_demo@0.1.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "stable", VersionRef: "b_rule_demo@0.1.0"},
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)

	result, err := selectedRuleRefsForObject(
		[]string{"b_rule_demo@0.1.0"},
		[]string{"b_rule_demo@0.1.0"},
		nil,
		sharedFilesByRef,
		sharedFilesByID,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "b_rule_demo@0.1.0" {
		t.Fatalf("expected [b_rule_demo@0.1.0], got %v", result)
	}
}

func TestSelectedRuleRefsForObjectExactSharedKeyMatch(t *testing.T) {
	sharedFilesByRef := map[string]sharedFile{
		"b_rule_demo@0.1.0": {RuleID: "b_rule_demo", RuleScope: "bound", Layer: "stable", VersionRef: "b_rule_demo@0.1.0"},
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)

	result, err := selectedRuleRefsForObject(
		[]string{"b_rule_demo@0.1.0"},
		nil,
		[]string{"b_rule_demo"},
		sharedFilesByRef,
		sharedFilesByID,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "b_rule_demo@0.1.0" {
		t.Fatalf("expected [b_rule_demo@0.1.0], got %v", result)
	}
}
