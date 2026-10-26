package judgments

import (
	"strings"
	"testing"
)

func testProtocolFingerprint(t *testing.T) string {
	t.Helper()
	return testProtocol(t)
}

func TestSlotEncodesEachKind(t *testing.T) {
	protocol := testProtocolFingerprint(t)
	context := strings.Repeat("ab", 32)

	cases := []struct {
		name    string
		kind    string
		unit    string
		subject string
		context string
		want    string
	}{
		{"public code", Code, "", "contracts/runtime.go", "", protocol + "/code/_/contracts/runtime.go/_"},
		{"unit design", Design, "eval", "eval/api/serve.go", "", protocol + "/design/eval/eval/api/serve.go/_"},
		{"architecture", Architecture, "eval", "eval", "", protocol + "/architecture/eval/_/_"},
		{"unit scan", Unit, "eval", "eval", "", protocol + "/unit/eval/_/_"},
		{"item", Item, "eval", "eval.chat_attachments", context, protocol + "/item/eval/" + ItemSubjectKey("eval.chat_attachments") + "/" + context},
	}
	for _, tc := range cases {
		got, err := Slot(protocol, tc.kind, tc.unit, tc.subject, tc.context)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: slot = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSlotRejectsUnsafeOrIncompleteParts(t *testing.T) {
	protocol := testProtocolFingerprint(t)
	cases := []struct {
		name     string
		protocol string
		kind     string
		unit     string
		subject  string
		context  string
	}{
		{"traversal protocol", "..", Code, "", "a.go", ""},
		{"unknown kind", protocol, "verdict", "", "a.go", ""},
		{"traversal in subject", protocol, Code, "", "../outside.go", ""},
		{"absolute subject", protocol, Code, "", "/etc/passwd", ""},
		{"dot subject", protocol, Code, "", ".", ""},
		{"separator in unit", protocol, Design, "eval/other", "a.go", ""},
		{"traversal unit", protocol, Design, "..", "a.go", ""},
		{"empty subject for code", protocol, Code, "", "  ", ""},
		{"non-fingerprint context", protocol, Item, "eval", "eval.item", "not-a-context"},
	}
	for _, tc := range cases {
		if got, err := Slot(tc.protocol, tc.kind, tc.unit, tc.subject, tc.context); err == nil {
			t.Fatalf("%s: expected an error, got slot %q", tc.name, got)
		}
	}
}

// TestSlotIsDerivedFromTheRecord pins the invariant the store relies on: a
// record's slot is recomputed from its own fields, so a record cannot be filed
// under a key it does not claim.
func TestSlotIsDerivedFromTheRecord(t *testing.T) {
	protocol := testProtocolFingerprint(t)
	r := &Record{Protocol: protocol, Kind: Design, Unit: "eval", Subject: "eval/api/serve.go"}
	got, err := SlotOf(r)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Slot(protocol, Design, "eval", "eval/api/serve.go", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SlotOf = %q, want %q", got, want)
	}

	item := &Record{Protocol: protocol, Kind: Item, Unit: "eval", Subject: "eval.item", SpecContext: strings.Repeat("cd", 32)}
	got, err = SlotOf(item)
	if err != nil {
		t.Fatal(err)
	}
	want, err = Slot(protocol, Item, "eval", "eval.item", item.SpecContext)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SlotOf(item) = %q, want %q", got, want)
	}
}

// TestSlotIsStableForTheSameKey pins that two callers asking about the same key
// compute the same directory without reading the store.
func TestSlotIsStableForTheSameKey(t *testing.T) {
	protocol := testProtocolFingerprint(t)
	first, err := Slot(protocol, Code, "", "eval/api/serve.go", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Slot(protocol, Code, "", "eval/api/serve.go", "")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("slot is not stable: %q vs %q", first, second)
	}
}
