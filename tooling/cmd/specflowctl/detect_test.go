package main

import (
	"bytes"
	"strings"
	"testing"
)

func detectRun(t *testing.T, repoRoot string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	fullArgs := append(args, "--repo-root", repoRoot)
	err := runDetect(fullArgs, &stdout, &stderr)
	return stdout.String(), err
}

func TestDetectRuleCommand(t *testing.T) {
	repoRoot := t.TempDir()
	writeBoundRule(t, repoRoot, "stable", "b_rule_x", "")

	output, err := detectRun(t, repoRoot, "--rule", "b_rule_x")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !strings.Contains(output, "RULE DETECTION — b_rule_x") {
		t.Fatalf("expected detection header, got:\n%s", output)
	}
	if !strings.Contains(output, "Consumers: none") {
		t.Fatalf("expected no consumers, got:\n%s", output)
	}
	if !strings.Contains(output, "Removable: yes") {
		t.Fatalf("expected removable, got:\n%s", output)
	}
}

func TestDetectAllCommand(t *testing.T) {
	repoRoot := t.TempDir()
	writeBoundRule(t, repoRoot, "stable", "b_rule_gone", "")
	writeBoundRule(t, repoRoot, "candidate", "b_rule_kept", "unbound_retention: intentional\n")

	output, err := detectRun(t, repoRoot, "--all")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !strings.Contains(output, "b_rule_gone") {
		t.Fatalf("expected removable rule in output:\n%s", output)
	}
	if !strings.Contains(output, "REMOVABLE: 1 of 2") {
		t.Fatalf("expected removable count, got:\n%s", output)
	}
}

func TestDetectAllCommandEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	output, err := detectRun(t, repoRoot, "--all")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !strings.Contains(output, "No bound rules found.") {
		t.Fatalf("expected empty message, got:\n%s", output)
	}
}

func TestDetectRequiresRuleOrAll(t *testing.T) {
	repoRoot := t.TempDir()
	_, err := detectRun(t, repoRoot)
	if err == nil {
		t.Fatal("expected error when neither --rule nor --all is given")
	}
}

func TestDetectRuleNotFoundCommand(t *testing.T) {
	repoRoot := t.TempDir()
	_, err := detectRun(t, repoRoot, "--rule", "b_rule_ghost")
	if err == nil {
		t.Fatal("expected error for missing rule")
	}
}
