package specpaths

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestPlanUnitAppendixCopiesRejectsUnresolvedDestinationOwnership(t *testing.T) {
	for _, sourceLayer := range []string{"candidate", "stable"} {
		for _, tc := range []struct {
			name, content string
			directory     bool
		}{
			{"missing_owner", "---\nstatus: active\n---\nUnknown owner.\n", false},
			{"exempt_missing_owner", "---\nstatus: exempt\n---\nUnknown owner.\n", false},
			{"inconsistent_owner", "---\nunit: other\n---\nWrong filename.\n", false},
			{"malformed_frontmatter", "---\nunit: auth\nNo closing marker.\n", false},
			{"unreadable_destination", "", true},
		} {
			t.Run(sourceLayer+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				sourceDir := filepath.Join(root, "docs/specs/units", sourceLayer, "appendix")
				if err := os.MkdirAll(sourceDir, 0755); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"unit_auth_a_first.md", "unit_auth_account_token.md"} {
					if err := os.WriteFile(filepath.Join(sourceDir, name), []byte("---\nunit: auth\n---\nOwned source.\n"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				layer := "stable"
				if sourceLayer == "stable" {
					layer = "candidate"
				}
				destination := "docs/specs/units/" + layer + "/appendix/unit_auth_account_token.md"
				path := filepath.Join(root, filepath.FromSlash(destination))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if tc.directory {
					if err := os.Mkdir(path, 0755); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
					t.Fatal(err)
				}
				copies, skipped, err := PlanUnitAppendixCopies(root, "auth", sourceLayer)
				if err == nil || !strings.Contains(err.Error(), destination) {
					t.Fatalf("must identify unresolved destination: %v", err)
				}
				if len(copies) != 0 || len(skipped) != 0 {
					t.Fatal("failed plan returned a partial copy set")
				}
				if _, err := os.Stat(filepath.Join(filepath.Dir(path), "unit_auth_a_first.md")); !os.IsNotExist(err) {
					t.Fatal("planning wrote the earlier destination")
				}
			})
		}
	}
}
