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
	write("stable", "appendix/unit_auth_protocol.md", "---\nunit: auth\n---\npublished protocol\n")
	write("candidate", "appendix/unit_auth_protocol.md", "---\nunit: auth\n---\npublished protocol")
	stable := context("stable")
	if context("candidate") != stable {
		t.Fatal("layer or line endings changed identical spec context")
	}
	write("candidate", "appendix/unit_auth_protocol.md", "---\nunit: auth\n---\ndraft protocol\n")
	if context("candidate") == stable {
		t.Fatal("an appendix-only candidate change shared the stable context")
	}
	write("candidate", "appendix/unit_auth_protocol.md", "---\nunit: auth\n---\npublished protocol\n")
	write("candidate", "appendix/unit_auth_notes.md", "---\nunit: auth\nstatus: exempt\n---\nLocal notes.\n")
	if context("candidate") != stable {
		t.Fatal("inactive appendices changed the context used after promote")
	}
}
