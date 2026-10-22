// Package testsupport provides fixture setup shared by tooling tests. It is
// not imported by production code.
package testsupport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

// DeployStandard writes the framework layout markers and every judging
// document the validate/verify standards name into a fixture repository, so
// judgments.Protocol can fingerprint a real standard instead of failing
// closed on a missing install.
func DeployStandard(t *testing.T, root string) {
	t.Helper()
	manifest := filepath.Join(root, "tooling", "manifest.tsv")
	if _, err := os.Stat(manifest); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(manifest), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifest, []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	documents := map[string]bool{}
	for _, gate := range []string{"validate", "verify"} {
		for _, kind := range []string{"unit", "rule"} {
			for _, name := range judgments.ProtocolDocuments(gate, kind) {
				documents[name] = true
			}
		}
	}
	for name := range documents {
		path := filepath.Join(root, "framework", filepath.FromSlash(name))
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		b.WriteString("# Fixture " + name + "\n\n## Judging text\n\nFixture.\n\n")
		for _, section := range judgments.MechanismSectionNames(name) {
			b.WriteString("## " + section + "\n\nFixture.\n\n")
		}
		if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
