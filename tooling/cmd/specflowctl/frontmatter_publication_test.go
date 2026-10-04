package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUnitFrontmatterValidationAndPublication(t *testing.T) {
	for _, tc := range []struct {
		name, old, replacement, diagnostic string
	}{
		{"missing id", "id: demo\n", "", "id"},
		{"empty id", "id: demo\n", "id:\n", "id"},
		{"missing unit refs", "unit_refs: none\n", "", "unit_refs"},
		{"empty unit refs", "unit_refs: none\n", "unit_refs:\n", "unit_refs"},
		{"missing rule refs", "rule_refs: none\n", "", "rule_refs"},
		{"empty rule refs", "rule_refs: none\n", "rule_refs:\n", "rule_refs"},
		{"wrong id", "id: demo\n", "id: another\n", "does not match"},
		{"complete", "", "", ""},
		{"active", "id: demo\n", "id: demo\nstatus: active\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := createCLITestRepo(t)
			grEnableMissionLayout(t, root)
			ownershipCandidate(t, root, "demo")
			candidate := filepath.Join(root, "docs/specs/units/candidate/unit_demo.md")
			data, err := os.ReadFile(candidate)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			if tc.old != "" {
				content = strings.Replace(content, tc.old, tc.replacement, 1)
				grWriteFile(t, root, "docs/specs/units/candidate/unit_demo.md", content)
			}
			var stdout, stderr bytes.Buffer
			err = runValidate([]string{"candidate", "--repo-root", root, "--unit", "demo"}, &stdout, &stderr)
			valid := tc.diagnostic == ""
			if valid {
				if err != nil {
					t.Fatalf("valid frontmatter rejected: %v\n%s", err, stdout.String())
				}
			} else if err == nil || !strings.Contains(stdout.String(), tc.diagnostic) {
				t.Fatalf("malformed frontmatter not identified: %v\n%s", err, stdout.String())
			}
			// Accepted reports are transport fixtures, not independent reviewer
			// verdicts. Publication must still enforce its own format contract.
			ownershipCheckCandidate(t, root, "demo")
			before := publicationFiles(t, root)
			out, err := publicationPromote(t, root, "unit", "demo")
			if valid {
				if err != nil {
					t.Fatalf("valid publication rejected: %v\n%s", err, out)
				}
				stable := filepath.Join(root, "docs/specs/units/stable/unit_demo.md")
				if publicationFiles(t, root)[stable] != content {
					t.Fatal("valid publication did not preserve the complete spec")
				}
				return
			}
			if err == nil || !strings.Contains(out, tc.diagnostic) {
				t.Fatalf("publication accepted malformed header or failed elsewhere: %v\n%s", err, out)
			}
			if after := publicationFiles(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("frontmatter rejection changed files, caches, baselines or candidates")
			}
		})
	}
}
