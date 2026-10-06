package specvalidation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckProsePathHygiene(t *testing.T) {
	compliant := "---\nid: auth\n---\n\n# auth\n\n## Description\n\nThe handler delegates to the token service.\n"
	if got := CheckProsePathHygiene(compliant); got.Status != Pass {
		t.Fatalf("compliant spec = %v, want PASS", got)
	}
	narrative := "---\nid: auth\n---\n\n# auth\n\n## Description\n\nSee src/auth/handler.go for details.\n"
	got := CheckProsePathHygiene(narrative)
	if got.Status != Warn {
		t.Fatalf("narrative path = %v, want WARNING", got)
	}
	if !strings.Contains(got.Details, "src/auth/handler.go") {
		t.Fatalf("warning must quote the path, got: %s", got.Details)
	}

	structured := "---\nid: auth\n---\n\n# auth\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Login.\n    implementation_surface: src/auth/handler.go\n    pass_condition: Passes.\n"
	if got := CheckProsePathHygiene(structured); got.Status != Pass {
		t.Fatalf("structured-field path = %v, want PASS (excluded), details: %s", got, got.Details)
	}

	fenced := "---\nid: auth\n---\n\n# auth\n\n## Description\n\nExample:\n\n```\ncd src/auth && go test ./...\n```\n"
	if got := CheckProsePathHygiene(fenced); got.Status != Pass {
		t.Fatalf("fenced example = %v, want PASS (fences excluded)", got)
	}

	governance := "---\nid: auth\n---\n\n# auth\n\n## Description\n\nThe gate lives in framework/concepts.md and docs/specs/meta/validation.\n"
	if got := CheckProsePathHygiene(governance); got.Status != Pass {
		t.Fatalf("governance paths = %v, want PASS (excluded)", got)
	}
}

func TestCheckEnvironmentAgnosticism(t *testing.T) {
	narrative := "---\nid: auth\n---\n\n# auth\n\n## Description\n\nRun the server on 127.0.0.1:8080.\n"
	if got := CheckEnvironmentAgnosticism(narrative); got.Status != Fail {
		t.Fatalf("narrative local address = %v, want FAIL", got)
	}
	absPath := "---\nid: auth\n---\n\n# auth\n\n## Description\n\nConfig lives at /Users/dev/config.yaml.\n"
	if got := CheckEnvironmentAgnosticism(absPath); got.Status != Fail {
		t.Fatalf("narrative absolute path = %v, want FAIL", got)
	}
	markedFence := "---\nid: auth\n---\n\n# auth\n\n## Description\n\nExample (placeholder):\n\n```\nopen http://localhost:8080\n```\n"
	if got := CheckEnvironmentAgnosticism(markedFence); got.Status != Pass {
		t.Fatalf("marked fenced example = %v, want PASS, details: %s", got, got.Details)
	}
	unmarkedFence := "---\nid: auth\n---\n\n# auth\n\n## Description\n\n```\ncurl http://localhost:8080/health\n```\n"
	if got := CheckEnvironmentAgnosticism(unmarkedFence); got.Status != Warn {
		t.Fatalf("unmarked fenced address = %v, want WARNING for the agent session to confirm", got)
	}
	secret := "---\nid: auth\n---\n\n# auth\n\n## Description\n\n```\n-----BEGIN RSA PRIVATE KEY-----\n```\n"
	if got := CheckEnvironmentAgnosticism(secret); got.Status != Fail {
		t.Fatalf("credential pattern = %v, want FAIL everywhere", got)
	}
}

func TestCheckFrontmatterRegionPurity(t *testing.T) {
	compliant := "---\nid: auth\nunit_refs: none\n---\n\n# auth\n\n## Description\n\nProse.\n"
	if got := CheckFrontmatterRegionPurity(compliant); got.Status != Pass {
		t.Fatalf("compliant = %v, want PASS", got)
	}
	stray := "---\nid: auth\n---\n\nThis paragraph sits between the frontmatter and the first heading.\n\n# auth\n\n## Description\n\nProse.\n"
	got := CheckFrontmatterRegionPurity(stray)
	if got.Status != Fail {
		t.Fatalf("stray prose = %v, want FAIL", got)
	}
	if !strings.Contains(got.Details, "stray content") {
		t.Fatalf("details must name the defect, got: %s", got.Details)
	}
}

func TestCheckContentScanCoversAppendices(t *testing.T) {
	repoRoot := newRepo(t)
	createFullCandidate(t, repoRoot, "scan_unit")
	// A non-exempt appendix with a narrative environment-specific path fails
	// the scan even when the main spec is clean.
	appendix := "docs/specs/units/candidate/appendix/unit_scan_unit_proto.md"
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/specs/units/candidate/appendix"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, appendix), []byte("---\nunit: scan_unit\n---\n\n# Protocol\n\nThe endpoint binds to /Users/dev/socket.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkEnvironmentAgnosticism(repoRoot, "scan_unit")
	if result.Status != Fail {
		t.Fatalf("appendix narrative path = %v, want FAIL", result)
	}
}
