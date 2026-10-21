package specpaths

import "testing"

func TestReadFrontmatter_InlineList(t *testing.T) {
	text := `---
id: test
rule_refs: [b_rule_auth, b_rule_billing]
---
`
	fm := ReadFrontmatterStringMap(text)
	if fm["id"] != "test" {
		t.Fatalf("expected id=test, got %q", fm["id"])
	}
	if fm["rule_refs"] != "[b_rule_auth, b_rule_billing]" {
		t.Fatalf("expected rule_refs list, got %q", fm["rule_refs"])
	}
}

func TestReadFrontmatter_BlockStyleList(t *testing.T) {
	text := `---
id: test
rule_refs:
  - b_rule_auth
  - b_rule_billing
---
`
	fm := ReadFrontmatterStringMap(text)
	if fm["id"] != "test" {
		t.Fatalf("expected id=test, got %q", fm["id"])
	}
	if fm["rule_refs"] != "[b_rule_auth, b_rule_billing]" {
		t.Fatalf("expected block-style rule_refs to be parsed as inline list, got %q", fm["rule_refs"])
	}
}

func TestReadFrontmatter_BlockStyleSingleItem(t *testing.T) {
	text := `---
id: test
rule_refs:
  - b_rule_auth
---
`
	fm := ReadFrontmatterStringMap(text)
	if fm["rule_refs"] != "[b_rule_auth]" {
		t.Fatalf("expected single-item block-style list [b_rule_auth], got %q", fm["rule_refs"])
	}
}

func TestReadFrontmatter_BothBlockStyle(t *testing.T) {
	text := `---
id: test
rule_refs:
  - b_rule_x
rule_exceptions:
  - rule: g_rule_y
---
`
	fm := ReadFrontmatterStringMap(text)
	if fm["rule_refs"] != "[b_rule_x]" {
		t.Fatalf("expected rule_refs=[b_rule_x], got %q", fm["rule_refs"])
	}
	if fm["rule_exceptions"] != "[rule: g_rule_y]" {
		t.Fatalf("unexpected rule_exceptions parse: %q", fm["rule_exceptions"])
	}
}

func TestReadFrontmatter_NoFrontmatter(t *testing.T) {
	text := `plain text without frontmatter`
	fm := ReadFrontmatterStringMap(text)
	if len(fm) != 0 {
		t.Fatalf("expected empty result for no frontmatter, got %v", fm)
	}
}

func TestReadFrontmatter_EmptyValue(t *testing.T) {
	text := `---
key_with_no_value:
also_empty:
---
`
	fm := ReadFrontmatterStringMap(text)
	if len(fm) != 0 {
		t.Fatalf("expected empty result for frontmatter with only empty values, got %v", fm)
	}
}

func TestReadFrontmatter_ListWithoutParentKey(t *testing.T) {
	text := `---
simple_key: value
  - orphan_item
another_key: ok
---
`
	fm := ReadFrontmatterStringMap(text)
	// orphan_item lines without a parent key should be skipped.
	if fm["simple_key"] != "value" {
		t.Fatalf("expected simple_key=value, got %q", fm["simple_key"])
	}
	if fm["another_key"] != "ok" {
		t.Fatalf("expected another_key=ok, got %q", fm["another_key"])
	}
}
