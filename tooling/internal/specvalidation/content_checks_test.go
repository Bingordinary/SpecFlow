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
	compliant := "---\nid: auth\n---\n\n# auth\n\n## Description\n\nProse.\n"
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
	// An appendix with a narrative environment-specific path fails
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

func TestCheckReservedContentHeadings(t *testing.T) {
	compliant := "---\nid: auth\n---\n\n# auth\n\n## 1. Context\n\nText.\n\n## 2. Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Given X, When Y, Then Z.\n    pass_condition: Holds.\n"
	if got := CheckReservedContentHeadings(compliant); got.Status != Pass {
		t.Fatalf("compliant spec = %v, want PASS", got)
	}
	changelog := "---\nid: auth\n---\n\n# auth\n\n## Version Notes\n\nGate-fix round two.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Given X, When Y, Then Z.\n    pass_condition: Holds.\n"
	got := CheckReservedContentHeadings(changelog)
	if got.Status != Fail {
		t.Fatalf("changelog heading = %v, want FAIL", got)
	}
	if !strings.Contains(got.Details, "Version Notes") {
		t.Fatalf("failure must quote the heading, got: %s", got.Details)
	}
	zh := "---\nid: auth\n---\n\n# auth\n\n## 变更记录\n\nText.\n"
	if got := CheckReservedContentHeadings(zh); got.Status != Fail {
		t.Fatalf("zh changelog heading = %v, want FAIL", got)
	}
	// A numbered heading ("## 2. Changelog") is the unit's own spec style and
	// must be caught exactly like its unnumbered form.
	numbered := "---\nid: auth\n---\n\n# auth\n\n## 1. Context\n\nText.\n\n## 2. Changelog\n\nGate-fix round two.\n"
	if got := CheckReservedContentHeadings(numbered); got.Status != Fail {
		t.Fatalf("numbered changelog heading = %v, want FAIL", got)
	}
}

func TestCheckProcessTraces(t *testing.T) {
	compliant := "---\nid: auth\n---\n\n# auth\n\n## 1. Context\n\nThe unit validates credentials.\n"
	if got := CheckProcessTraces(compliant); got.Status != Pass {
		t.Fatalf("compliant spec = %v, want PASS", got)
	}
	history := "---\nid: auth\n---\n\n# auth\n\n## 1. Context\n\nThe preflight stage has been removed (门禁修复轮三).\n"
	if got := CheckProcessTraces(history); got.Status != Warn {
		t.Fatalf("process trace = %v, want WARNING", got)
	}
	fenced := "---\nid: auth\n---\n\n# auth\n\n## 1. Context\n\n```\n已移除 example\n```\n"
	if got := CheckProcessTraces(fenced); got.Status != Pass {
		t.Fatalf("fenced marker = %v, want PASS (fences excluded)", got)
	}
}

func TestCheckTestDoubles(t *testing.T) {
	compliant := "---\nid: auth\n---\n\n# auth\n\n## 2. Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Given a registered user, When they log in, Then the system returns 200.\n    pass_condition: Returns 200.\n"
	if got := CheckTestDoubles(compliant); got.Status != Pass {
		t.Fatalf("compliant spec = %v, want PASS", got)
	}
	leaked := "---\nid: auth\n---\n\n# auth\n\n## 2. Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Given a stub SessionStore, When StartRun runs, Then it persists.\n    pass_condition: Holds.\n"
	if got := CheckTestDoubles(leaked); got.Status != Warn {
		t.Fatalf("test double = %v, want WARNING", got)
	}
	zh := "---\nid: auth\n---\n\n# auth\n\n## 1. Context\n\n使用 stub 宿主驱动端口。\n"
	if got := CheckTestDoubles(zh); got.Status != Warn {
		t.Fatalf("zh test double = %v, want WARNING", got)
	}
}

func TestCheckVerificationMethodShape(t *testing.T) {
	compliant := "---\nid: auth\n---\n\n# auth\n\n## 2. Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Given X, When Y, Then Z.\n    verification_method: Go test for login behavior.\n    pass_condition: Holds.\n"
	if got := CheckVerificationMethodShape(compliant); got.Status != Pass {
		t.Fatalf("one-sentence method = %v, want PASS", got)
	}
	recipe := "---\nid: auth\n---\n\n# auth\n\n## 2. Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Given X, When Y, Then Z.\n    verification_method: Verify (1) save is called (2) load is called (3) delete is called.\n    pass_condition: Holds.\n"
	if got := CheckVerificationMethodShape(recipe); got.Status != Fail {
		t.Fatalf("step recipe = %v, want FAIL", got)
	}
	// A step recipe shown inside a fenced example is content (guide §13.7),
	// not a real field — it must PASS.
	fenced := "---\nid: auth\n---\n\n# auth\n\n## 1. Context\n\n```yaml\nacceptance_item_set:\n  - id: auth.core\n    verification_method: Verify (1) save (2) load.\n```\n\n## 2. Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.real\n    description: Given X, When Y, Then Z.\n    verification_method: Go test for login behavior.\n    pass_condition: Holds.\n"
	if got := CheckVerificationMethodShape(fenced); got.Status != Pass {
		t.Fatalf("fenced recipe = %v, want PASS (fences are content)", got)
	}
}
