package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAcceptanceSchemaValidationAndPublication(t *testing.T) {
	for _, field := range []string{"verification_method", "pass_condition", "runnable", "complete"} {
		t.Run(field, func(t *testing.T) {
			root := createCLITestRepo(t)
			grEnableMissionLayout(t, root)
			grWriteFile(t, root, "src/demo.go", "package demo\n")
			path := grWriteSpecItems(t, root, "demo", "none", "none", []string{"demo.first", "demo.second"})
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			if field != "complete" {
				index := strings.Index(content, "  - id: demo.second")
				var kept []string
				for _, line := range strings.Split(content[index:], "\n") {
					if !strings.HasPrefix(line, "    "+field+":") {
						kept = append(kept, line)
					}
				}
				content = content[:index] + strings.Join(kept, "\n")
				grWriteFile(t, root, "docs/specs/units/candidate/unit_demo.md", content)
			}
			var stdout, stderr bytes.Buffer
			err = runValidate([]string{"candidate", "--repo-root", root, "--unit", "demo"}, &stdout, &stderr)
			if field == "complete" {
				if err != nil {
					t.Fatalf("valid shape rejected: %v\n%s", err, stdout.String())
				}
			} else if err == nil || !strings.Contains(stdout.String(), "demo.second") || !strings.Contains(stdout.String(), field) {
				t.Fatalf("malformed shape accepted or unidentified: %v\n%s", err, stdout.String())
			}
			// These are accepted report-transport fixtures, not semantic reviewer
			// verdicts. They prove publication independently checks item shape.
			ownershipCheckCandidate(t, root, "demo")
			before := publicationFiles(t, root)
			out, err := publicationPromote(t, root, "unit", "demo")
			if field == "complete" {
				if err != nil {
					t.Fatalf("valid publication rejected: %v\n%s", err, out)
				}
				stable := publicationFiles(t, root)[filepath.Join(root, "docs/specs/units/stable/unit_demo.md")]
				if stable != content {
					t.Fatal("valid publication did not preserve the complete spec")
				}
				return
			}
			if err == nil || !strings.Contains(out, "demo.second") || !strings.Contains(out, field) {
				t.Fatalf("publication accepted malformed shape or failed elsewhere: %v\n%s", err, out)
			}
			if after := publicationFiles(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("shape rejection changed publication files, caches or candidates")
			}
		})
	}
}
