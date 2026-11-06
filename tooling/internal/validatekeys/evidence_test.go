package validatekeys

import (
	"strings"
	"testing"
)

func evidenceGraph() *Graph {
	return &Graph{Items: []Item{{ID: "AUTH-1"}, {ID: "AUTH-2"}}}
}

func TestParseEvidenceRecordAcceptsOneKey(t *testing.T) {
	content := `## AUTH-1 — Login
Keys:
  - item:AUTH-1

observed behavior
`
	sections, err := ParseEvidenceRecord("unit_auth", content, evidenceGraph())
	if err != nil {
		t.Fatalf("ParseEvidenceRecord: %v", err)
	}
	if len(sections) != 1 || sections[0].Item != "AUTH-1" {
		t.Fatalf("sections = %+v", sections)
	}
}

func TestParseEvidenceRecordRejectsSecondKey(t *testing.T) {
	content := `## AUTH-1 and AUTH-2
Keys:
  - item:AUTH-1
  - item:AUTH-2

observed behavior
`
	_, err := ParseEvidenceRecord("unit_auth", content, evidenceGraph())
	if err == nil || !strings.Contains(err.Error(), "second item key") {
		t.Fatalf("expected a second-key error, got %v", err)
	}
}

func TestParseEvidenceRecordRejectsDuplicateKey(t *testing.T) {
	content := `## AUTH-1
Keys:
  - item:AUTH-1
  - item:AUTH-1

observed behavior
`
	_, err := ParseEvidenceRecord("unit_auth", content, evidenceGraph())
	if err == nil || !strings.Contains(err.Error(), "second item key") {
		t.Fatalf("expected a duplicate-key error, got %v", err)
	}
}

func TestParseEvidenceRecordIgnoresItemKeysInsideFences(t *testing.T) {
	content := "## AUTH-1\nKeys:\n  - item:AUTH-1\n\n```\n- item:AUTH-2\n```\n"
	sections, err := ParseEvidenceRecord("unit_auth", content, evidenceGraph())
	if err != nil {
		t.Fatalf("ParseEvidenceRecord: %v", err)
	}
	if len(sections) != 1 || sections[0].Item != "AUTH-1" {
		t.Fatalf("sections = %+v", sections)
	}
}
