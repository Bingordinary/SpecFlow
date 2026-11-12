package judgments

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProtocolSectionFingerprintBindsOnlyTheReadSection proves the standard
// binding is per session text: editing one section changes that session's
// fingerprint and leaves a session that reads another section untouched. This
// is the property unit validate relies on to re-run one coverage group
// instead of the whole gate.
func TestProtocolSectionFingerprintBindsOnlyTheReadSection(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tooling"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tooling", "manifest.tsv"), []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "framework"), 0755); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(root, "framework", "unit_validate_checklist.md")
	writeDoc := func(check5, check2 string) {
		t.Helper()
		doc := "# Checklist\n\n## Check 5\n\n" + check5 + "\n\n## Check 2\n\n" + check2 + "\n"
		if err := os.WriteFile(docPath, []byte(doc), 0644); err != nil {
			t.Fatal(err)
		}
	}
	check5 := ProtocolSection{File: "unit_validate_checklist.md", Level: 2, Heading: "Check 5"}
	check2 := ProtocolSection{File: "unit_validate_checklist.md", Level: 2, Heading: "Check 2"}

	writeDoc("five original", "two original")
	fiveBefore, err := ProtocolSectionFingerprint(root, []ProtocolSection{check5})
	if err != nil {
		t.Fatal(err)
	}
	twoBefore, err := ProtocolSectionFingerprint(root, []ProtocolSection{check2})
	if err != nil {
		t.Fatal(err)
	}

	writeDoc("five edited", "two original")
	fiveAfter, err := ProtocolSectionFingerprint(root, []ProtocolSection{check5})
	if err != nil {
		t.Fatal(err)
	}
	twoAfter, err := ProtocolSectionFingerprint(root, []ProtocolSection{check2})
	if err != nil {
		t.Fatal(err)
	}

	if fiveBefore == fiveAfter {
		t.Fatal("the edited section must change its fingerprint")
	}
	if twoBefore != twoAfter {
		t.Fatal("an unedited section must keep its fingerprint")
	}
	if fiveBefore == twoBefore {
		t.Fatal("different sections must not collapse to the same fingerprint")
	}
}

// TestProtocolSectionFingerprintIncludesRenderedText proves the fingerprint
// hashes the same text the mission renders: source, heading, and content. A
// binding that ignored the section text would let a framework edit reuse a
// conclusion it no longer supports.
func TestProtocolSectionFingerprintIncludesRenderedText(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tooling"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tooling", "manifest.tsv"), []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "framework"), 0755); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(root, "framework", "severity_policy.md")
	if err := os.WriteFile(docPath, []byte("## 4. Severity Levels\n\nfirst body\n"), 0644); err != nil {
		t.Fatal(err)
	}
	section := ProtocolSection{File: "severity_policy.md", Level: 2, Heading: "4. Severity Levels"}
	before, err := ProtocolSectionFingerprint(root, []ProtocolSection{section})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("## 4. Severity Levels\n\nsecond body\n"), 0644); err != nil {
		t.Fatal(err)
	}
	after, err := ProtocolSectionFingerprint(root, []ProtocolSection{section})
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("the body content must be part of the fingerprint")
	}
}

// TestProtocolVerifySessionFingerprintResolvesClassSections proves the
// per-class verify fingerprint mechanism is structurally sound: every session
// class resolves to a deployed fingerprint, and every section named as a
// class-owned section exists exactly once in its document. It belongs to the
// authoritative `go test ./internal/judgments/ -run Protocol` family.
func TestProtocolVerifySessionFingerprintResolvesClassSections(t *testing.T) {
	root := sourceRepoRoot(t)
	for _, kind := range VerifySessionKinds {
		fingerprint, err := VerifySessionFingerprint(root, kind)
		if err != nil {
			t.Fatalf("session class %q: %v", kind, err)
		}
		if fingerprint == "" {
			t.Fatalf("session class %q produced an empty fingerprint", kind)
		}
	}
	for document, refs := range verifyClassSections() {
		raw, err := os.ReadFile(filepath.Join(root, "framework", filepath.FromSlash(document)))
		if err != nil {
			t.Fatalf("read class-section document %q: %v", document, err)
		}
		for _, ref := range refs {
			if _, err := ExtractSection(string(raw), ref.Level, ref.Heading); err != nil {
				t.Fatalf("%s: class section %q does not resolve: %v", document, ref.Heading, err)
			}
		}
	}
}
