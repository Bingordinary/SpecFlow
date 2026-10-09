package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

func TestGatePlanRejectsDirectoryDiscoveryErrors(t *testing.T) {
	for _, tc := range []struct {
		name, directory, expectedRef, targetKind string
	}{
		{"global rules", "docs/specs/rules/stable", "rule:g_rule_required", "unit"},
		{"candidate units for rule", "docs/specs/units/candidate", "unit:demo", "rule"},
		{"stable units for rule", "docs/specs/units/stable", "unit:peer", "rule"},
	} {
		for _, failure := range []string{"permission", "not a directory"} {
			t.Run(tc.name+"/"+failure, func(t *testing.T) {
				root := createCLITestRepo(t)
				grEnableMissionLayout(t, root)
				grWriteSpec(t, root, "demo")
				peer := strings.ReplaceAll(nextDiscoverySpec("old"), "demo", "peer")
				grWriteFile(t, root, "docs/specs/units/stable/unit_peer.md", peer)
				grWriteFile(t, root, "docs/specs/rules/stable/g_rule_required.md", publicationRuleText("g_rule_required", "global", "Reject unauthenticated requests."))
				grWriteFile(t, root, "docs/specs/rules/candidate/b_rule_target.md", publicationRuleText("b_rule_target", "bound", "Name exact owners."))
				args := []string{"--gate", "validate", "--target", "candidate"}
				if tc.targetKind == "unit" {
					args = append(args, "--unit", "demo", "--relationships", "none")
				} else {
					args = append(args, "--rule", "b_rule_target")
				}
				id := grPlan(t, root, args...)
				run := mustLoadRun(t, root, id)
				found := false
				for _, ref := range run.Refs {
					found = found || ref.Ref == tc.expectedRef
				}
				if !found {
					t.Fatalf("readable control omitted %s", tc.expectedRef)
				}
				statePath := filepath.Join(root, filepath.FromSlash(gaterun.StateRelPath(id)))
				before, err := os.ReadFile(statePath)
				if err != nil {
					t.Fatal(err)
				}
				restore := failGateInputDirectory(t, filepath.Join(root, filepath.FromSlash(tc.directory)), failure)
				var stdout, stderr bytes.Buffer
				err = runGatePlan(append([]string{"--repo-root", root}, args...), &stdout, &stderr)
				if err == nil || !strings.Contains(err.Error(), tc.directory) || stdout.Len() != 0 {
					t.Fatalf("expected directory error without successful plan, got %v; stdout=%s", err, stdout.String())
				}
				after, err := os.ReadFile(statePath)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("failed planning changed the existing run: %v", err)
				}
				entries, err := os.ReadDir(filepath.Dir(filepath.Dir(statePath)))
				if err != nil || len(entries) != 1 {
					t.Fatalf("failed planning created a partial run: entries=%v, err=%v", entries, err)
				}
				diff, err := gaterun.Compare(root, run)
				if err != nil || !strings.Contains(strings.Join(diff, "\n"), "input surface no longer resolvable:") || !strings.Contains(strings.Join(diff, "\n"), tc.directory) {
					t.Fatalf("comparison hid directory failure: diff=%v, err=%v", diff, err)
				}
				restore()
				diff, err = gaterun.Compare(root, run)
				if err != nil || len(diff) != 0 {
					t.Fatalf("restored readable snapshot differs: %v, %v", diff, err)
				}
				grPlan(t, root, args...)
			})
		}
	}
}

func failGateInputDirectory(t *testing.T, path, failure string) func() {
	t.Helper()
	var repair func() error
	if failure == "permission" {
		if runtime.GOOS == "windows" {
			t.Skip("directory permission denial via chmod is not supported on Windows")
		}
		if err := os.Chmod(path, 0000); err != nil {
			t.Fatal(err)
		}
		repair = func() error { return os.Chmod(path, 0755) }
	} else {
		backup := path + ".saved"
		if err := os.Rename(path, backup); err != nil {
			t.Fatal(err)
		}
		repair = func() error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Rename(backup, path)
		}
		if err := os.WriteFile(path, []byte("not a directory\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var restored bool
	restore := func() {
		if !restored {
			if err := repair(); err != nil {
				t.Errorf("restore directory: %v", err)
			}
			restored = true
		}
	}
	t.Cleanup(restore)
	if failure == "permission" {
		if _, err := os.ReadDir(path); !os.IsPermission(err) {
			t.Skipf("filesystem did not enforce directory permission denial: %v", err)
		}
	}
	return restore
}

func TestGatePlanAllowsAbsentDiscoveryDirectories(t *testing.T) {
	for _, kind := range []string{"unit", "rule"} {
		t.Run(kind, func(t *testing.T) {
			root := createCLITestRepo(t)
			args := []string{"--gate", "validate", "--target", "candidate"}
			if kind == "unit" {
				grWriteSpec(t, root, "demo")
				args = append(args, "--unit", "demo")
			} else {
				grWriteFile(t, root, "docs/specs/rules/candidate/b_rule_target.md", publicationRuleText("b_rule_target", "bound", "Name exact owners."))
				args = append(args, "--rule", "b_rule_target")
			}
			id := grPlan(t, root, args...)
			run := mustLoadRun(t, root, id)
			if len(run.Refs) != 1 {
				t.Fatalf("absent optional directories added inputs: %+v", run.Refs)
			}
		})
	}
}
