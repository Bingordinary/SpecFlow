package gaterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// TestUnitScanSpecRegionsNormalizeLineEndings pins the canonical text form of
// the scan's spec-side row pins: a CRLF checkout of unchanged content must
// produce the same region identities as an LF checkout, so the rows stay live.
func TestUnitScanSpecRegionsNormalizeLineEndings(t *testing.T) {
	root := t.TempDir()
	spec := "---\nid: auth\nrule_refs: none\n---\n\n# auth\n\n" +
		"## Design Narrative\n### Narrative\n\nProse.\n\n" +
		"## Promises\n\npromises:\n  - id: auth.core\n    summary: Core.\n    contract: {invariants: [core]}\n\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n  - id: auth.core\n    promise: auth.core\n    source: [Narrative]\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: contracts.js\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	evidence := "---\nunit: auth\n---\n\n# Evidence\n\n## Auth behavior\n\nKeys:\n  - item:auth.core\n\nObserved.\n"
	writeCRLF := func(ref, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.ReplaceAll(content, "\n", "\r\n")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeCRLF(specpaths.CandidateUnitSpecFileRef("auth"), spec)
	writeCRLF(specpaths.CandidateEvidenceRecordRef("auth"), evidence)

	run := &Run{TargetKind: TargetKindUnit, TargetName: "auth", Target: TargetCandidate}
	refs, sections, err := unitScanSpecRegions(root, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) < 2 {
		t.Fatalf("expected promise and evidence-section regions, got %+v", refs)
	}
	if _, ok := sections["Auth behavior"]; !ok {
		t.Fatalf("the evidence-section region map is missing its section: %+v", sections)
	}
	for _, ref := range refs {
		path := ref.path
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !contenthash.DepsPresent(specpaths.NormalizeText(string(raw)), []string{ref.dep}) {
			t.Fatalf("region pin %q does not survive normalization for %s", ref.dep, ref.path)
		}
		if contenthash.DepsPresent(string(raw), []string{ref.dep}) {
			t.Fatalf("probe did not exercise a CRLF checkout for %s", ref.path)
		}
	}
}
