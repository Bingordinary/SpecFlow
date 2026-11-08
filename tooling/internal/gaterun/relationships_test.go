package gaterun

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// TestFullRunCarriesCarriedRelationship pins the full-run carry rule for a
// relationship recorded as `carried`: it is an unchanged passing conclusion,
// not a failed one, so an unchanged unit carries it mechanically.
func TestFullRunCarriesCarriedRelationship(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	checks := mainCheckMarkers()
	checks = append(checks, validationcache.CheckEntry{Check: RelationshipKey("coverage_scope"), Status: "carried"})
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", checks)

	statuses := map[string]string{}
	for _, check := range checks {
		status := check.Status
		if status == "" {
			status = "pass"
		}
		statuses[check.Check] = status
	}
	catalog := validateObjectBaseline(t, repoRoot, "auth")
	judgments, err := json.Marshal(JudgmentBaseline{
		SchemaVersion:   validationcache.RecordStateSchemaValidate,
		LogicalStatus:   statuses,
		Relationships:   []string{"coverage_scope"},
		SynthesisDigest: "sha256:test",
		ValidateObjects: catalog,
		FilePresence:    filePresenceForKeys(t, repoRoot, catalog),
	})
	if err != nil {
		t.Fatal(err)
	}
	pinFormat, pins, pinsSet := promiseCacheFormat(t, repoRoot, "auth", GateValidate, "pass")
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:        "validate",
		Unit:           "auth",
		Mode:           "full",
		Basis:          "repair",
		Result:         "pass",
		Target:         "candidate",
		PinFormat:      pinFormat,
		PromisePins:    pins,
		PromisePinsSet: pinsSet,
		Standard:       baselineStandard(t, repoRoot),
		Timestamp:      "2026-01-01T00:00:00Z",
		Judgments:      string(judgments),
		Entries:        []validationcache.FileEntry{entry},
	}); err != nil {
		t.Fatal(err)
	}

	run := &Run{Gate: GateValidate, TargetKind: TargetKindUnit, TargetName: "auth", Target: TargetCandidate}
	carried, notices, err := planFullRunCarry(repoRoot, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Relationships) != 0 {
		t.Fatalf("an unchanged carried relationship must carry, got re-run scope %v", run.Relationships)
	}
	if strings.Join(carried, ",") != RelationshipKey("coverage_scope") {
		t.Fatalf("expected the carried relationship candidate, got %v", carried)
	}
	if !strings.Contains(strings.Join(notices, " "), "carry mechanically") {
		t.Fatalf("expected the mechanical-carry notice, got %v", notices)
	}
}
