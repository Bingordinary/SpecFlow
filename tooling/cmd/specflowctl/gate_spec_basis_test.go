package main

import (
	"strings"
	"testing"
)

// TestDesignReportRequiresSpecBasis pins the report contract: a design block
// must declare the spec anchors its judgment relied on, and every anchor must
// resolve. A block without the declaration can never be under-pinned silently.
func TestDesignReportRequiresSpecBasis(t *testing.T) {
	root, _, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, root, id)

	grPreparePublic(t, root, run, []string{"design:auth:contracts.js"})

	base := grDefaultQualityReport(run, *run.CoverageByKey("design:auth:contracts.js"))

	// The declaration is required.
	without := strings.Replace(base, "Spec basis: Narrative\nSpec basis: Promises\n", "", 1)
	if err := sharedSubmit(t, root, id, "design:auth:contracts.js", without); err == nil || !strings.Contains(err.Error(), "Spec basis") {
		t.Fatalf("a design block without Spec basis must be rejected, got %v", err)
	}

	// An anchor that resolves to nothing is rejected rather than ignored.
	unresolved := strings.Replace(base, "Spec basis: Narrative", "Spec basis: No Such Block", 1)
	if err := sharedSubmit(t, root, id, "design:auth:contracts.js", unresolved); err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("an unresolvable anchor must be rejected, got %v", err)
	}

	// The heading line's literal spelling is not an anchor: an anchor names the
	// heading text, so the leading '#' markers must be dropped. The rejection
	// states that spelling instead of leaving the reviewer to retry blindly.
	literal := strings.Replace(base, "Spec basis: Narrative", "Spec basis: ### Narrative", 1)
	err := sharedSubmit(t, root, id, "design:auth:contracts.js", literal)
	if err == nil || !strings.Contains(err.Error(), "does not resolve") || !strings.Contains(err.Error(), "leading '#' markers") {
		t.Fatalf("a heading line used as an anchor must be rejected with the spelling rule, got %v", err)
	}

	// A resolvable declaration is accepted and pins exactly the declared
	// anchors: the narrative block and the promise entry, not the whole spec.
	if err := sharedSubmit(t, root, id, "design:auth:contracts.js", base); err != nil {
		t.Fatalf("a declared design block must be accepted: %v", err)
	}
}
