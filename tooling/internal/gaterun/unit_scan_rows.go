package gaterun

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validatekeys"
)

// Row verdicts per kind, re-exported for the report contract layer.
const (
	RowKindDeclaration = judgments.RowKindDeclaration
	RowKindSurplus     = judgments.RowKindSurplus
	RowKindNonRunnable = judgments.RowKindNonRunnable
	RowKindCoverage    = judgments.RowKindCoverage

	RowMatch        = judgments.RowMatch
	RowMismatch     = judgments.RowMismatch
	RowUndetermined = judgments.RowUndetermined

	RowClean   = judgments.RowClean
	RowSurplus = judgments.RowSurplus

	RowOK     = judgments.RowOK
	RowDefect = judgments.RowDefect
)

// RowPin is one checklist row of the unit-level scan's checklist bookkeeping.
type RowPin = judgments.RowPin

// UnitScanRow is one checklist row of the unit-level scan's catalog. The row
// is the scan's delta unit: each row's own dependency pins decide whether its
// recorded verdict still holds, so a change re-runs exactly the rows that read
// it instead of the whole unit-wide conclusion
// (framework/validation_cache.md §Unit-scan row bookkeeping).
type UnitScanRow struct {
	ID     string
	Kind   string
	Anchor string
}

// unitScanRowCatalog lists the unit-level scan's checklist rows from the unit
// spec's structured content: one declaration row per promise entry and per
// evidence-record section, one surplus row per declared code file, one
// non-runnable row per `runnable: no` acceptance item, and one coverage row
// for the Step 5 Part 2 runnable-set assessment. The catalog is derived
// mechanically — the planner and the record builder share it, so a row the
// catalog declares can never be silently skipped by a report.
func UnitScanRowCatalog(root string, run *Run) ([]UnitScanRow, error) {
	content, err := readSpecContent(root, mainSpecRef(run))
	if err != nil {
		return nil, err
	}
	// The catalog is content-addressed: derive it from the same normalized
	// spelling the row pins and verification use, so CRLF and LF checkouts of
	// unchanged content produce the same row identities.
	content = specpaths.NormalizeText(content)
	graph, err := validatekeys.Parse(run.TargetName, content)
	if err != nil {
		return nil, fmt.Errorf("the unit-scan row catalog cannot be derived: %w", err)
	}
	var rows []UnitScanRow
	for _, p := range graph.Promises {
		rows = append(rows, UnitScanRow{ID: "decl:promise:" + p.ID, Kind: judgments.RowKindDeclaration, Anchor: p.ID})
	}
	record, err := specpaths.UnitEvidenceRecord(root, run.TargetName, run.Target)
	if err != nil {
		return nil, err
	}
	if record != nil {
		path := record.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sections, err := validatekeys.ParseEvidenceRecord(strings.TrimSuffix(filepath.Base(record.Path), ".md"), string(data), graph)
		if err != nil {
			return nil, fmt.Errorf("the unit-scan row catalog cannot read the evidence record: %w", err)
		}
		for _, s := range sections {
			rows = append(rows, UnitScanRow{ID: "decl:evidence:" + s.Item, Kind: judgments.RowKindDeclaration, Anchor: s.Heading})
		}
	}
	// A surplus row judges one *declared* code file. Input surfaces (an
	// evidence directory supplied through the input manifest or gate-extend)
	// stay readable background material: they are not part of the unit's
	// declared implementation surface and must never add a checklist row.
	for _, f := range surfaceFiles(run, true) {
		rows = append(rows, UnitScanRow{ID: "surplus:" + f, Kind: judgments.RowKindSurplus, Anchor: f})
	}
	for _, item := range specvalidation.ExtractAcceptanceItemIDs(content) {
		if itemRunsNo(content, item) {
			rows = append(rows, UnitScanRow{ID: "non_runnable:" + item, Kind: judgments.RowKindNonRunnable, Anchor: item})
		}
	}
	coverageID, err := unitScanCoverageRowID(content)
	if err != nil {
		return nil, err
	}
	rows = append(rows, UnitScanRow{ID: coverageID, Kind: judgments.RowKindCoverage})
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// unitScanCoverageRowID derives the Step 5 Part 2 coverage row's identity from
// the complete acceptance-item set: every item id plus its region content. The
// row is the global runnable-set assessment, so any item addition, removal or
// content change produces a new row identity. A fresh coverage row is therefore
// re-judged against the changed set instead of carrying the previous
// assessment; the row's pins cover every item region
// (framework/validation_cache.md §Unit-scan row bookkeeping).
func unitScanCoverageRowID(content string) (string, error) {
	var parts []string
	for _, item := range specvalidation.ExtractAcceptanceItemIDs(content) {
		region, ok := contenthash.LocateAcceptanceItemRegion(content, item)
		if !ok {
			return "", fmt.Errorf("acceptance item %q region cannot be located", item)
		}
		parts = append(parts, item+"\n"+region.Text)
	}
	return "coverage:" + contenthash.RegionCID(strings.Join(parts, "\n---\n")), nil
}

// itemRunsNo reports whether the acceptance item declares `runnable: no`.
var runnableNoRe = regexp.MustCompile(`(?m)^\s*runnable:\s*no\s*$`)

func itemRunsNo(content, item string) bool {
	region, ok := contenthash.LocateAcceptanceItemRegion(content, item)
	if !ok {
		return false
	}
	return runnableNoRe.MatchString(region.Text)
}

// aggregateRowVerdicts recomposes the scan's aggregate verdict from row
// verdicts: a mismatched declaration row or a surplus row is a MISMATCH, an
// undetermined row or a defective non-runnable row is a CANNOT_DETERMINE, and
// only all-clean rows are ALIGNED. It is the single home of the composition
// rule: the report contract layer's parsed rows and the record's merged row
// pins both recompose through it, so the verdict a report is validated against
// and the verdict a record carries can never follow different rules.
func aggregateRowVerdicts(verdicts []string) string {
	mismatch := false
	undetermined := false
	for _, verdict := range verdicts {
		switch verdict {
		case RowMismatch, RowSurplus:
			mismatch = true
		case RowUndetermined, RowDefect:
			undetermined = true
		}
	}
	switch {
	case mismatch:
		return "MISMATCH"
	case undetermined:
		return "CANNOT_DETERMINE"
	default:
		return "ALIGNED"
	}
}

// AggregateUnitScanVerdict recomposes the scan's aggregate verdict from parsed
// row results.
func AggregateUnitScanVerdict(rows []ScanRowResult) string {
	verdicts := make([]string, 0, len(rows))
	for _, row := range rows {
		verdicts = append(verdicts, row.Verdict)
	}
	return aggregateRowVerdicts(verdicts)
}

// unitRegionRef is one spec-side region dependency of a scan row: the file
// that carries the region and the region dependency string.
type unitRegionRef struct {
	path string
	dep  string
}

// unitScanSpecRegions lists the scan's spec-side declaration regions: one
// promise-entry region per promise and one section region per evidence-record
// section. Surplus rows pin all of them — their judgment ("this code is
// declared nowhere") is answered against the complete declaration catalog, so
// any catalog change reopens every surplus row. The section map resolves an
// evidence-record section heading to its exact region, so a declaration row
// pins the region it was judged against and never a neighboring section whose
// heading merely contains the anchor as a prefix.
func unitScanSpecRegions(root string, run *Run) ([]unitRegionRef, map[string]unitRegionRef, error) {
	content, err := readSpecContent(root, mainSpecRef(run))
	if err != nil {
		return nil, nil, err
	}
	// Region pins are content identities over normalized text, matching the
	// whole-file hash normalization and the verification path. A CRLF checkout
	// of unchanged content must not change a row's pin.
	content = specpaths.NormalizeText(content)
	graph, err := validatekeys.Parse(run.TargetName, content)
	if err != nil {
		return nil, nil, err
	}
	var refs []unitRegionRef
	for _, p := range graph.Promises {
		dep, ok := contenthash.SpecAnchorDep(content, p.ID)
		if !ok {
			return nil, nil, fmt.Errorf("promise %q region cannot be located in %s", p.ID, mainSpecRef(run))
		}
		refs = append(refs, unitRegionRef{path: mainSpecRef(run), dep: dep})
	}
	record, err := specpaths.UnitEvidenceRecord(root, run.TargetName, run.Target)
	if err != nil {
		return nil, nil, err
	}
	if record == nil {
		return refs, map[string]unitRegionRef{}, nil
	}
	path := record.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := validatekeys.ParseEvidenceRecord(strings.TrimSuffix(filepath.Base(record.Path), ".md"), string(data), graph)
	if err != nil {
		return nil, nil, err
	}
	evidenceText := specpaths.NormalizeText(string(data))
	sectionRegions := map[string]unitRegionRef{}
	for _, s := range parsed {
		dep, ok := contenthash.SpecAnchorDep(evidenceText, s.Heading)
		if !ok {
			return nil, nil, fmt.Errorf("evidence-record section %q region cannot be located in %s", s.Heading, record.Path)
		}
		ref := unitRegionRef{path: record.Path, dep: dep}
		refs = append(refs, ref)
		if _, dup := sectionRegions[s.Heading]; dup {
			return nil, nil, fmt.Errorf("duplicate evidence-record section heading %q in %s", s.Heading, record.Path)
		}
		sectionRegions[s.Heading] = ref
	}
	return refs, sectionRegions, nil
}

// buildUnitScanRows turns a scan session's row results into the record's row
// pins, merging carried rows for a row-scoped partial run. It fails closed:
// every catalog row needs a fresh verdict or a carried baseline row, and a
// session row outside the catalog is rejected. The record-layer finding rule
// is checked separately, after the row set is final (SaveJudgment).
func buildUnitScanRows(root string, run *Run, ck CoverageKey, sessionRows []ScanRowResult, specText string) ([]judgments.RowPin, string, error) {
	catalog, err := UnitScanRowCatalog(root, run)
	if err != nil {
		return nil, "", err
	}
	assigned := map[string]bool{}
	if len(run.ScanRows) > 0 {
		for _, id := range run.ScanRows {
			assigned[id] = true
		}
	} else {
		for _, cr := range catalog {
			assigned[cr.ID] = true
		}
	}
	catalogByID := map[string]UnitScanRow{}
	for _, cr := range catalog {
		catalogByID[cr.ID] = cr
	}
	reported := map[string]ScanRowResult{}
	for _, row := range sessionRows {
		if _, ok := catalogByID[row.ID]; !ok {
			return nil, "", fmt.Errorf("unit-scan report row %q is not in the row catalog", row.ID)
		}
		if _, dup := reported[row.ID]; dup {
			return nil, "", fmt.Errorf("unit-scan report declares row %q more than once", row.ID)
		}
		reported[row.ID] = row
	}
	var carried []judgments.RowPin
	if len(run.ScanRows) > 0 {
		carried, err = carriedUnitScanRows(run)
		if err != nil {
			return nil, "", err
		}
	}
	carriedByID := map[string]judgments.RowPin{}
	for _, row := range carried {
		carriedByID[row.ID] = row
	}

	layer := run.Target
	unit := run.TargetName
	var owned []string
	evidenceRef, err := specpaths.UnitEvidenceRecord(root, unit, layer)
	if err != nil {
		return nil, "", err
	}
	if evidenceRef != nil {
		owned = append(owned, evidenceRef.Path)
	}
	ownPath := func(p string) (string, bool) {
		if suffix, ok := judgments.OwnSuffix(p, unit, layer, owned); ok {
			return suffix, true
		}
		return p, false
	}
	verifySnapshot := func(p string) (string, error) {
		h, ok := run.SnapshotHash(root, p)
		if !ok || h == "" {
			return "", fmt.Errorf("judgment input %s absent from snapshot", p)
		}
		current := refreshRef(root, Ref{Ref: p})
		if current.Hash != h {
			return "", fmt.Errorf("judgment input changed: %s", p)
		}
		return h, nil
	}
	rowWholeFile := func(p string) (judgments.Dependency, error) {
		h, err := verifySnapshot(p)
		if err != nil {
			return judgments.Dependency{}, err
		}
		dep := judgments.Dependency{Path: p, Hash: h}
		dep.Path, dep.Own = ownPath(p)
		return dep, nil
	}
	rowRegion := func(p, region string) (judgments.Dependency, error) {
		if _, err := verifySnapshot(p); err != nil {
			return judgments.Dependency{}, err
		}
		d := judgments.Dependency{Path: p, Deps: []string{region}}
		d.Path, d.Own = ownPath(p)
		return d, nil
	}

	specRegions, sectionRegions, err := unitScanSpecRegions(root, run)
	if err != nil {
		return nil, "", err
	}

	var rows []judgments.RowPin
	for _, cr := range catalog {
		if sessionRow, ok := reported[cr.ID]; ok {
			if !assigned[cr.ID] {
				return nil, "", fmt.Errorf("unit-scan report judges row %q, which this run did not assign", cr.ID)
			}
			pin := judgments.RowPin{ID: cr.ID, Kind: cr.Kind, Verdict: sessionRow.Verdict}
			switch cr.Kind {
			case judgments.RowKindDeclaration:
				var region unitRegionRef
				if strings.HasPrefix(cr.ID, "decl:promise:") {
					dep, ok := contenthash.SpecAnchorDep(specText, cr.Anchor)
					if !ok {
						return nil, "", fmt.Errorf("promise %q region cannot be located in %s", cr.Anchor, mainSpecRef(run))
					}
					region = unitRegionRef{path: mainSpecRef(run), dep: dep}
				} else {
					// An evidence-section declaration row: the anchor is the
					// section heading, resolved exactly against the section
					// map — a prefix collision with a neighboring heading
					// must not pin the wrong region.
					found, ok := sectionRegions[cr.Anchor]
					if !ok {
						return nil, "", fmt.Errorf("evidence section %q region cannot be located", cr.Anchor)
					}
					region = found
				}
				d, err := rowRegion(region.path, region.dep)
				if err != nil {
					return nil, "", err
				}
				pin.Deps = append(pin.Deps, d)
				for _, f := range sessionRow.Files {
					d, err := rowWholeFile(f)
					if err != nil {
						return nil, "", err
					}
					pin.Deps = append(pin.Deps, d)
				}
			case judgments.RowKindSurplus:
				d, err := rowWholeFile(cr.Anchor)
				if err != nil {
					return nil, "", err
				}
				pin.Deps = append(pin.Deps, d)
				for _, r := range specRegions {
					d, err := rowRegion(r.path, r.dep)
					if err != nil {
						return nil, "", err
					}
					pin.Deps = append(pin.Deps, d)
				}
			case judgments.RowKindNonRunnable:
				region, ok := contenthash.LocateAcceptanceItemRegion(specText, cr.Anchor)
				if !ok {
					return nil, "", fmt.Errorf("acceptance item %q region cannot be located", cr.Anchor)
				}
				dep := "region:acceptance_item:" + cr.Anchor + ":" + contenthash.RegionCID(region.Text)
				d, err := rowRegion(mainSpecRef(run), dep)
				if err != nil {
					return nil, "", err
				}
				pin.Deps = append(pin.Deps, d)
			case judgments.RowKindCoverage:
				// The Step 5 Part 2 coverage row assesses the remaining runnable
				// set, so it pins every acceptance-item region. Its row identity
				// already changes when the item set or any item's content
				// changes; the pins keep the row dead when only the main spec
				// context changes in a way that rewrites the same set.
				for _, item := range specvalidation.ExtractAcceptanceItemIDs(specText) {
					region, ok := contenthash.LocateAcceptanceItemRegion(specText, item)
					if !ok {
						return nil, "", fmt.Errorf("acceptance item %q region cannot be located", item)
					}
					dep := "region:acceptance_item:" + item + ":" + contenthash.RegionCID(region.Text)
					d, err := rowRegion(mainSpecRef(run), dep)
					if err != nil {
						return nil, "", err
					}
					pin.Deps = append(pin.Deps, d)
				}
			}
			if err := pin.Validate(); err != nil {
				return nil, "", err
			}
			rows = append(rows, pin)
			continue
		}
		if !assigned[cr.ID] {
			base, ok := carriedByID[cr.ID]
			if !ok {
				return nil, "", fmt.Errorf("row %q has no fresh verdict and no carried baseline row", cr.ID)
			}
			rows = append(rows, base)
			continue
		}
		return nil, "", fmt.Errorf("row %q is assigned to this run but the session report does not judge it", cr.ID)
	}
	verdicts := make([]string, 0, len(rows))
	for _, row := range rows {
		verdicts = append(verdicts, row.Verdict)
	}
	return rows, aggregateRowVerdicts(verdicts), nil
}

// aggregateUnitScanRowPins recomposes the scan's aggregate verdict from the
// record's final row pins. It is the carried-record counterpart of
// AggregateUnitScanVerdict: the session path aggregates parsed row results,
// the carried re-save path reuses the record's merged pins.
func aggregateUnitScanRowPins(rows []judgments.RowPin) string {
	verdicts := make([]string, 0, len(rows))
	for _, row := range rows {
		verdicts = append(verdicts, row.Verdict)
	}
	return aggregateRowVerdicts(verdicts)
}

// checkUnitScanFindingCoverage enforces the record-layer finding rule: a
// record with mismatched or surplus rows must carry at least one finding
// bound to its unit key. The finding set is the post-synthesis terminal set,
// where the final synthesis merges findings that restate one root cause, so
// this is an existence check over the final row set, not a
// one-finding-per-row count. The one-block-per-retained-row rule belongs to
// the session report and is enforced at submission
// (framework/validation_cache.md §Unit-scan row bookkeeping).
func checkUnitScanFindingCoverage(rows []judgments.RowPin, findingCount int, key string) error {
	mismatched := 0
	for _, row := range rows {
		if row.Verdict == judgments.RowMismatch || row.Verdict == judgments.RowSurplus {
			mismatched++
		}
	}
	if mismatched > 0 && findingCount == 0 {
		return fmt.Errorf("unit-scan rows declare %d mismatched or surplus row(s) but the canonical record carries no finding — at least one finding bound to %s is required", mismatched, key)
	}
	return nil
}

// carriedUnitScanRows returns the baseline record's row pins snapshotted at
// plan time. Finalize never re-reads the baseline cache: the carried evidence
// is part of the run's immutable plan-time state.
func carriedUnitScanRows(run *Run) ([]judgments.RowPin, error) {
	if len(run.CarriedScanRows) == 0 {
		return nil, fmt.Errorf("the run records no carried unit-scan rows — plan a new run")
	}
	return run.CarriedScanRows, nil
}

// unitScanScope is the delta planner's derived re-run disposition for one
// carried unit-scan record.
type unitScanScope struct {
	// whole means the conclusion re-runs as a full report; why says why.
	whole bool
	why   string
	// scope is the row-scoped re-run assignment: a proper subset of the
	// current catalog. Empty means every row carries and the record is fully
	// usable.
	scope []string
	// notices explains the derived scope on the plan.
	notices []string
}

// unitScanRowScope derives a carried unit-scan record's delta re-run scope
// from the record itself — unconditionally, not only when the whole-record
// verification fails:
//
//  1. The record-level pins — the session-class standard fingerprint and the
//     row-independent promise pins — are not rows. Any one of them failing
//     re-runs the whole conclusion even when every row's pins hold.
//  2. Otherwise the scope is the rows whose own pins no longer hold plus the
//     current catalog rows the baseline never judged. A change to the
//     declaration-row set (a promise entry or evidence-record section added
//     or removed) reopens every surplus row as well: the surplus question is
//     answered against the complete declaration catalog, and a recorded row's
//     pin list cannot see a declaration source that did not exist when it was
//     judged.
//
// A derived scope equal to the whole catalog degrades to a full report
// (framework/validation_cache.md §Unit-scan row bookkeeping → Delta scope).
func unitScanRowScope(root string, run *Run, record *judgments.Record, ref judgments.Reference, layer string) (unitScanScope, error) {
	if err := judgments.VerifyRecordLevel(root, record, ref, layer); err != nil {
		return unitScanScope{whole: true, why: err.Error()}, nil
	}
	// Row-level reuse is only sound for a passing conclusion. A failed
	// conclusion must be re-judged whole: a carried MISMATCH/surplus row
	// carries a finding, and the gate outcome consumes findings, so row-level
	// carry would drop the failure instead of re-checking it
	// (framework/verification_scope.md §Coverage Model → Unit-scan rows).
	if record.Verdict == "MISMATCH" {
		return unitScanScope{whole: true, why: "the baseline conclusion retains a MISMATCH"}, nil
	}
	for _, row := range record.Rows {
		if row.Verdict == judgments.RowMismatch || row.Verdict == judgments.RowSurplus {
			return unitScanScope{whole: true, why: "the baseline conclusion retains a failed checklist row"}, nil
		}
	}
	dead, err := judgments.DeadUnitScanRows(root, record, layer)
	if err != nil {
		return unitScanScope{whole: true, why: err.Error()}, nil
	}
	catalog, err := UnitScanRowCatalog(root, run)
	if err != nil {
		return unitScanScope{}, err
	}
	currentRows := map[string]bool{}
	for _, cr := range catalog {
		currentRows[cr.ID] = true
	}
	baselineRows := map[string]bool{}
	removedDeclaration := false
	for _, row := range record.Rows {
		baselineRows[row.ID] = true
		if row.Kind == judgments.RowKindDeclaration && !currentRows[row.ID] {
			removedDeclaration = true
		}
	}
	rerun := map[string]bool{}
	for _, id := range dead {
		// The re-run scope is the current catalog: a baseline row that left
		// the catalog (its declaration was removed) is neither re-runnable
		// nor carriable, and an assignment outside the catalog could never
		// satisfy the scoped report contract. It simply leaves the record
		// with the shrunken catalog.
		if currentRows[id] {
			rerun[id] = true
		}
	}
	var grew []string
	grewDeclaration := false
	for _, cr := range catalog {
		if baselineRows[cr.ID] {
			continue
		}
		rerun[cr.ID] = true
		grew = append(grew, cr.ID)
		if cr.Kind == judgments.RowKindDeclaration {
			grewDeclaration = true
		}
	}
	// The row catalog scopes content changes of already-declared objects. A
	// declared code surface that grew is a whole-conclusion trigger: the
	// shared NewlyRequiredEvidence check runs before row scoping, because a
	// new implementation file can change a Step 1 declaration verdict and the
	// new surplus row alone judges only Step 5 Part A.
	scope := unitScanScope{}
	if len(grew) > 0 {
		scope.notices = append(scope.notices, "unit-scan rows the baseline never judged: "+strings.Join(grew, ", "))
	}
	if grewDeclaration || removedDeclaration {
		folded := false
		for _, cr := range catalog {
			if cr.Kind != judgments.RowKindSurplus || rerun[cr.ID] {
				continue
			}
			rerun[cr.ID] = true
			folded = true
		}
		if folded {
			scope.notices = append(scope.notices, "unit-scan surplus rows re-run: the declaration-row set changed")
		}
	}
	if len(rerun) == 0 {
		return scope, nil
	}
	if len(rerun) == len(currentRows) {
		return unitScanScope{whole: true, why: "every checklist row re-runs"}, nil
	}
	scope.scope = sortedKeySet(rerun)
	return scope, nil
}
