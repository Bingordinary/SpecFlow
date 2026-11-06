package judgments

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpecContextUsesActiveContentAcrossLayers(t *testing.T) {
	root := t.TempDir()
	write := func(layer, name, content string) {
		t.Helper()
		path := filepath.Join(root, "docs/specs/units", layer, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	context := func(layer string) string {
		t.Helper()
		value, err := SpecContext(root, "auth", layer)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	write("stable", "unit_auth.md", "approved requirement\n")
	write("candidate", "unit_auth.md", "approved requirement\r\n")
	write("stable", "evidence/unit_auth.md", "---\nunit: auth\n---\npublished protocol\n")
	write("candidate", "evidence/unit_auth.md", "---\nunit: auth\n---\npublished protocol")
	stable := context("stable")
	if context("candidate") != stable {
		t.Fatal("layer or line endings changed identical spec context")
	}
	write("candidate", "evidence/unit_auth.md", "---\nunit: auth\n---\ndraft protocol\n")
	if context("candidate") == stable {
		t.Fatal("an evidence-record-only candidate change shared the stable context")
	}
	write("candidate", "evidence/unit_auth.md", "---\nunit: auth\n---\npublished protocol\n")
	if err := os.Remove(filepath.Join(root, "docs/specs/units/stable/evidence/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	if context("candidate") == context("stable") {
		t.Fatal("a candidate-only evidence record must change the candidate context")
	}
}
