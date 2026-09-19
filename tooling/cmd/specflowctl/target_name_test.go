package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestTargetNameValidationAcrossCommands pins the naming boundary
// (tooling/README.md §Target names): every command that accepts a unit name or
// rule id rejects a name outside the grammar before any path construction, so
// a traversal-capable name never reaches a path builder or the filesystem.
func TestTargetNameValidationAcrossCommands(t *testing.T) {
	badNames := []string{"../escape", "a/b", "bad name", "..", ".", "-lead"}

	type command struct {
		name string
		args func(root, value string) []string
		run  func(args []string, stdout, stderr io.Writer) error
	}

	commands := []command{
		{name: "validate candidate", args: func(root, v string) []string {
			return []string{"candidate", "--unit", v, "--repo-root", root}
		}, run: runValidate},
		{name: "validate rule", args: func(root, v string) []string {
			return []string{"rule", "--id", v, "--repo-root", root}
		}, run: runValidate},
		{name: "fork unit", args: func(root, v string) []string {
			return []string{"--unit", v, "--repo-root", root}
		}, run: runFork},
		{name: "fork rule", args: func(root, v string) []string {
			return []string{"--rule", v, "--repo-root", root}
		}, run: runFork},
		{name: "next", args: func(root, v string) []string {
			return []string{"--unit", v, "--repo-root", root}
		}, run: runNext},
		{name: "deps unit", args: func(root, v string) []string {
			return []string{"--unit", v, "--repo-root", root}
		}, run: runDeps},
		{name: "deps rule", args: func(root, v string) []string {
			return []string{"--rule", v, "--repo-root", root}
		}, run: runDeps},
		{name: "detect", args: func(root, v string) []string {
			return []string{"--rule", v, "--repo-root", root}
		}, run: runDetect},
		{name: "remove", args: func(root, v string) []string {
			return []string{"--rule", v, "--repo-root", root}
		}, run: runRemove},
		{name: "consumers", args: func(root, v string) []string {
			return []string{"--rule", v, "--repo-root", root}
		}, run: runConsumers},
		{name: "promote unit", args: func(root, v string) []string {
			return []string{"--unit", v, "--repo-root", root}
		}, run: runPromote},
		{name: "promote rule", args: func(root, v string) []string {
			return []string{"--rule", v, "--repo-root", root}
		}, run: runPromote},
		{name: "fresh unit", args: func(root, v string) []string {
			return []string{"--unit", v, "--repo-root", root}
		}, run: runFresh},
		{name: "fresh rule", args: func(root, v string) []string {
			return []string{"--rule", v, "--repo-root", root}
		}, run: runFresh},
		{name: "gate-status unit", args: func(root, v string) []string {
			return []string{"--unit", v, "--repo-root", root}
		}, run: runGateStatus},
		{name: "gate-status rule", args: func(root, v string) []string {
			return []string{"--rule", v, "--repo-root", root}
		}, run: runGateStatus},
	}

	for _, tc := range commands {
		for _, bad := range badNames {
			t.Run(tc.name+"/"+bad, func(t *testing.T) {
				root := t.TempDir()
				var stdout, stderr bytes.Buffer
				err := tc.run(tc.args(root, bad), &stdout, &stderr)
				if err == nil || !strings.Contains(err.Error(), "is invalid") {
					t.Fatalf("expected %q to be rejected as an invalid target name, got %v", bad, err)
				}
			})
		}
	}
}
