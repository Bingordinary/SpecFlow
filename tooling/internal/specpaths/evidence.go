package specpaths

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
)

// EvidenceRecord is a unit's single evidence record: observed implementation
// behavior organized as one `##` section per acceptance item, each bound to
// its item by a `Keys:` block with exactly one `item:<id>` key.
type EvidenceRecord struct {
	Path string
}

// EvidenceRecordCopy is one layer-to-layer copy of a unit's evidence record.
// Source and destination are repository-relative.
type EvidenceRecordCopy struct {
	Source      string
	Destination string
}

// CandidateEvidenceRecordRef returns the deterministic candidate-layer path of
// a unit's evidence record.
func CandidateEvidenceRecordRef(unit string) string {
	return fmt.Sprintf("%s/evidence/unit_%s.md", CandidateDir, unit)
}

// StableEvidenceRecordRef returns the deterministic stable-layer path of a
// unit's evidence record.
func StableEvidenceRecordRef(unit string) string {
	return fmt.Sprintf("%s/evidence/unit_%s.md", StableDir, unit)
}

// evidenceOwner checks the dual ownership form: the frontmatter `unit` field
// and the deterministic file name must name the same unit.
func evidenceOwner(path string, fields map[string]string) (string, error) {
	owner := fields["unit"]
	if ValidateTargetName("unit", owner) != nil || filepath.Base(path) != "unit_"+owner+".md" {
		return "", fmt.Errorf("%s: evidence record ownership is missing or inconsistent", path)
	}
	return owner, nil
}

// UnitEvidenceRecord resolves the unit's single evidence record. The path is
// deterministic — docs/specs/units/<layer>/evidence/unit_<unit>.md — so the
// file's presence is the declaration that the unit carries evidence. A record
// whose frontmatter `unit` disagrees with its file name is an ownership error,
// not an absent record. It returns nil when the unit has no evidence record.
func UnitEvidenceRecord(root, unit, layer string) (*EvidenceRecord, error) {
	if err := ValidateTargetName("unit", unit); err != nil {
		return nil, err
	}
	var ref string
	switch layer {
	case "candidate":
		ref = CandidateEvidenceRecordRef(unit)
	case "stable":
		ref = StableEvidenceRecordRef(unit)
	default:
		return nil, fmt.Errorf("invalid unit spec layer %q", layer)
	}
	path := filepath.Join(root, filepath.FromSlash(ref))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read evidence record %s: %w", ref, err)
	}
	canonical, err := repopath.Canonical(root, path)
	if err != nil {
		return nil, err
	}
	fm, _, err := ParseFrontmatterFields(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: cannot parse evidence record frontmatter: %w", canonical, err)
	}
	if _, err := evidenceOwner(canonical, fm); err != nil {
		return nil, err
	}
	return &EvidenceRecord{Path: canonical}, nil
}

// PlanEvidenceRecordCopy resolves the opposite-layer copy of the unit's
// evidence record before callers write any lifecycle artifact. An existing
// destination must declare the same owner. It returns nil when the unit has no
// evidence record in the source layer.
func PlanEvidenceRecordCopy(root, unit, sourceLayer string) (*EvidenceRecordCopy, error) {
	record, err := UnitEvidenceRecord(root, unit, sourceLayer)
	if err != nil || record == nil {
		return nil, err
	}
	destinationLayer := "stable"
	if sourceLayer == "stable" {
		destinationLayer = "candidate"
	}
	var destination string
	switch destinationLayer {
	case "candidate":
		destination = CandidateEvidenceRecordRef(unit)
	default:
		destination = StableEvidenceRecordRef(unit)
	}
	if _, err := repopath.Canonical(root, destination); err != nil {
		return nil, fmt.Errorf("%s: %w", destination, err)
	}
	destPath := filepath.Join(root, filepath.FromSlash(destination))
	if data, err := os.ReadFile(destPath); err == nil {
		fields, _, err := ParseFrontmatterFields(string(data))
		if err != nil {
			return nil, fmt.Errorf("%s: cannot determine destination evidence record ownership: %w", destination, err)
		}
		if _, err := evidenceOwner(destination, fields); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read destination evidence record %s: %w", destination, err)
	}
	return &EvidenceRecordCopy{Source: record.Path, Destination: destination}, nil
}

// ResolveEvidenceRecord resolves a unit evidence-record logical reference
// (`unit:<name>:evidence`) to the current-layer file (candidate first, stable
// fallback). It returns "" when no layer carries the record, so a promote of
// the referenced unit does not stale caches that depended on its (unchanged)
// evidence content.
func ResolveEvidenceRecord(repoRoot, unit string) string {
	for _, ref := range []string{CandidateEvidenceRecordRef(unit), StableEvidenceRecordRef(unit)} {
		p := filepath.Join(repoRoot, filepath.FromSlash(ref))
		if _, err := os.Stat(p); err == nil {
			return filepath.ToSlash(p)
		}
	}
	return ""
}
