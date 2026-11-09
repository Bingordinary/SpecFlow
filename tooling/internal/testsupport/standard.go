// Package testsupport provides fixture setup shared by tooling tests. It is
// not imported by production code.
package testsupport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validatekeys"
)

// DeployStandard gives a bare fixture repository the framework documents the
// protocol mapping names, so a protocol fingerprint resolves instead of
// failing closed on a missing install. Every section a session mission inlines
// is materialized at its declared heading level: the standard fingerprint and
// the mission read the same section set, and a fixture missing one would make
// the mapping fail closed in tests for a reason unrelated to the test.
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
	sectionKeys := map[string]judgments.ProtocolSection{}
	addSections := func(sections []judgments.ProtocolSection) {
		for _, section := range sections {
			sectionKeys[section.File+"\x00"+section.Heading] = section
		}
	}
	contract, err := validatekeys.DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range contract.Groups {
		sections, err := judgments.ProtocolSectionsFor("validate", "unit", judgments.ProtocolSessionChecks, group.Checks)
		if err != nil {
			t.Fatal(err)
		}
		addSections(sections)
	}
	sections, err := judgments.ProtocolSectionsFor("validate", "unit", judgments.ProtocolSessionCross, []string{"cross"})
	if err != nil {
		t.Fatal(err)
	}
	addSections(sections)
	sections, err = judgments.ProtocolSectionsFor("validate", "rule", judgments.ProtocolSessionChecks, []string{"1", "2", "3", "4", "5", "6"})
	if err != nil {
		t.Fatal(err)
	}
	addSections(sections)
	for _, kind := range []string{
		judgments.ProtocolSessionItem,
		judgments.ProtocolSessionUnit,
		judgments.ProtocolSessionCode,
		judgments.ProtocolSessionDesign,
		judgments.ProtocolSessionArchitecture,
		judgments.ProtocolSessionCross,
	} {
		sections, err := judgments.ProtocolSectionsFor("verify", "unit", kind, nil)
		if err != nil {
			t.Fatal(err)
		}
		addSections(sections)
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
		for _, section := range sectionKeys {
			if section.File != name {
				continue
			}
			b.WriteString(strings.Repeat("#", section.Level) + " " + section.Heading + "\n\nFixture.\n\n")
		}
		for _, section := range judgments.MechanismSectionNames(name) {
			b.WriteString("## " + section + "\n\nFixture.\n\n")
		}
		if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
