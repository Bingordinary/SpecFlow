package specpaths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnitAppendicesResolvesDeclaredOwnerAcrossSharedPrefixes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, fields := range map[string]string{
		"unit_auth_protocol.md":       "unit: auth",
		"unit_auth_extra_protocol.md": "unit: auth_extra",
		"unit_auth_notes.md":          "unit: auth\nstatus: exempt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("---\n"+fields+"\n---\nContent.\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for unit, names := range map[string][]string{
		"auth":       {"unit_auth_notes.md", "unit_auth_protocol.md"},
		"auth_extra": {"unit_auth_extra_protocol.md"},
	} {
		appendices, err := UnitAppendices(root, unit, "candidate")
		if err != nil {
			t.Fatal(err)
		}
		var actual []string
		for _, appendix := range appendices {
			actual = append(actual, filepath.Base(appendix.Path))
		}
		if !reflect.DeepEqual(actual, names) {
			t.Fatalf("%s owns %v, expected %v", unit, actual, names)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_auth_broken.md"), []byte("---\nunit: other\n---\nContent.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := UnitAppendices(root, "auth", "candidate"); err == nil {
		t.Fatal("active appendix with inconsistent ownership was accepted")
	}
}
