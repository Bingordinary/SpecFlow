package specpaths

import "testing"

func TestReadFrontmatter_InlineList(t *testing.T) {
	text := `---
id: test
layer: candidate
version: 0.1.0
unit_refs: [auth@0.1.0, billing@0.2.0]
rule_refs: none
---
`
	fm := ReadFrontmatterStringMap(text)
	if fm["id"] != "test" {
		t.Fatalf("expected id=test, got %q", fm["id"])
	}
	if fm["layer"] != "candidate" {
		t.Fatalf("expected layer=candidate, got %q", fm["layer"])
	}
	if fm["version"] != "0.1.0" {
		t.Fatalf("expected version=0.1.0, got %q", fm["version"])
	}
	if fm["unit_refs"] != "[auth@0.1.0, billing@0.2.0]" {
		t.Fatalf("expected unit_refs list, got %q", fm["unit_refs"])
	}
	if fm["rule_refs"] != "none" {
		t.Fatalf("expected rule_refs=none, got %q", fm["rule_refs"])
	}
}

func TestReadFrontmatter_BlockStyleList(t *testing.T) {
	text := `---
id: test
layer: candidate
version: 0.1.0
unit_refs:
  - auth@0.1.0
  - billing@0.2.0
rule_refs: none
---
`
	fm := ReadFrontmatterStringMap(text)
	if fm["id"] != "test" {
		t.Fatalf("expected id=test, got %q", fm["id"])
	}
	if fm["unit_refs"] != "[auth@0.1.0, billing@0.2.0]" {
		t.Fatalf("expected block-style unit_refs to be parsed as inline list, got %q", fm["unit_refs"])
	}
	if fm["rule_refs"] != "none" {
		t.Fatalf("expected rule_refs=none, got %q", fm["rule_refs"])
	}
}

func TestReadFrontmatter_BlockStyleSingleItem(t *testing.T) {
	text := `---
id: test
layer: candidate
version: 0.1.0
unit_refs:
  - auth@0.1.0
rule_refs: none
---
`
	fm := ReadFrontmatterStringMap(text)
	if fm["unit_refs"] != "[auth@0.1.0]" {
		t.Fatalf("expected single-item block-style list [auth@0.1.0], got %q", fm["unit_refs"])
	}
}

func TestReadFrontmatter_BothBlockStyle(t *testing.T) {
	text := `---
id: test
layer: candidate
version: 0.1.0
unit_refs:
  - auth
rule_refs:
  - b_rule_x
---
`
	fm := ReadFrontmatterStringMap(text)
	if fm["unit_refs"] != "[auth]" {
		t.Fatalf("expected unit_refs=[auth], got %q", fm["unit_refs"])
	}
	if fm["rule_refs"] != "[b_rule_x]" {
		t.Fatalf("expected rule_refs=[b_rule_x], got %q", fm["rule_refs"])
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
