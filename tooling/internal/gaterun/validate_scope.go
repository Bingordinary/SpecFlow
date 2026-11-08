package gaterun

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validatekeys"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// validateObjectScope is the object-level delta decision for a unit validate
// run: the coverage groups whose object batch changed (they re-run) and the
// objects that forced them.
type validateObjectScope struct {
	SelectedGroups  map[string]bool
	RerunObjects    map[string]bool
	RuleChanged     bool
	ProviderChanged bool
	// StructureChanged reports that the object set itself changed: an object
	// was added or removed (a new or deleted item, promise, section, rule, or
	// other catalog object). A repair treats that as a structural change and
	// covers the full scope; a read-set change re-runs the affected object
	// through the ordinary selection, and a pass-baseline delta keeps its
	// ordinary object selection.
	StructureChanged bool
	Notices          []string
}

// selectValidateObjectScope compares the current per-object catalog against
// the baseline catalog and the mechanical change report. An object re-runs
// when it is not in the baseline catalog, when its read set changed, or when a read
// ref's content changed. A group re-runs when any of its objects re-runs.
func (d *Derivation) selectValidateObjectScope(run *Run, state JudgmentBaseline, report *validationcache.ChangeReport) (validateObjectScope, error) {
	scope := validateObjectScope{SelectedGroups: map[string]bool{}, RerunObjects: map[string]bool{}}
	if len(run.ValidateObjects) == 0 {
		return scope, nil
	}
	main := run.OwnSpecFiles[0]
	data, err := os.ReadFile(filepath.Join(d.root, filepath.FromSlash(main)))
	if err != nil {
		return scope, err
	}
	graph, err := validatekeys.Parse(run.TargetName, string(data))
	if err != nil {
		return scope, err
	}
	sectionRange := map[string][2]int{}
	for _, section := range graph.Sections {
		sectionRange[section.Heading] = [2]int{section.Start, section.End}
	}
	itemRange := map[string][2]int{}
	for _, item := range graph.Items {
		itemRange[item.ID] = [2]int{item.Start, item.End}
	}
	promiseRange := map[string][2]int{}
	for _, promise := range graph.Promises {
		promiseRange[promise.ID] = [2]int{promise.Start, promise.End}
	}
	evidencePath := ""
	var evidenceReportKeys []string
	for _, path := range run.OwnSpecFiles {
		clean := filepath.ToSlash(path)
		if !strings.Contains(clean, "/evidence/") {
			continue
		}
		evidencePath = path
		evidenceReportKeys = append(evidenceReportKeys, changeReportKey(path))
		if ref := logicalEvidenceRef(run, path); ref != "" {
			evidenceReportKeys = append(evidenceReportKeys, changeReportKey(ref))
		}
	}
	evidenceSectionRange := map[string][2]int{}
	if evidencePath != "" {
		data, err := os.ReadFile(filepath.Join(d.root, filepath.FromSlash(evidencePath)))
		if err != nil {
			return scope, err
		}
		base := strings.TrimSuffix(filepath.Base(evidencePath), ".md")
		sections, err := validatekeys.ParseEvidenceRecord(base, string(data), graph)
		if err != nil {
			return scope, err
		}
		for _, section := range sections {
			evidenceSectionRange[validatekeys.EvidenceSectionRef(section.Item)] = [2]int{section.Start, section.End}
		}
	}

	ownSet := map[string]bool{}
	ownSpecChanged := false
	for _, path := range run.OwnSpecFiles {
		ownSet[changeReportKey(path)] = true
		if ref := logicalEvidenceRef(run, path); ref != "" {
			ownSet[changeReportKey(ref)] = true
		}
	}
	changedLines := map[string]map[int]bool{}
	changedAll := map[string]bool{}
	changedEvidence := false
	for _, entry := range report.Entries {
		key := changeReportKey(entry.Path)
		if !ownSet[key] {
			continue
		}
		ownSpecChanged = true
		if entry.Kind != "changed" || len(entry.Changes) == 0 {
			changedAll[key] = true
		} else {
			if changedLines[key] == nil {
				changedLines[key] = map[int]bool{}
			}
			for _, change := range entry.Changes {
				start, end := change.StartLine, change.EndLine
				if start < 1 {
					start = 1
				}
				if end < start {
					end = start
				}
				for line := start; line <= end; line++ {
					changedLines[key][line] = true
				}
			}
		}
		if evidenceRecordBase(entry.Path) != "" {
			changedEvidence = true
		}
	}
	regionChangedFor := func(pathKey string, bounds [2]int) bool {
		if bounds[0] == 0 {
			return false
		}
		if changedAll[pathKey] {
			return true
		}
		for line := bounds[0]; line <= bounds[1]; line++ {
			if changedLines[pathKey][line] {
				return true
			}
		}
		return false
	}
	regionChanged := func(bounds [2]int) bool {
		return regionChangedFor(changeReportKey(main), bounds)
	}
	evidenceSectionChanged := func(ref string) bool {
		item := strings.TrimPrefix(ref, "evidence_section:")
		if item == ref {
			return true
		}
		bounds, ok := evidenceSectionRange[ref]
		if !ok {
			// A section the current catalog cannot locate has no baseline
			// evidence to carry; re-run rather than trust an unknown region.
			return true
		}
		for _, key := range evidenceReportKeys {
			if regionChangedFor(key, bounds) {
				return true
			}
		}
		return false
	}
	// Rule and provider changes are matched to the run's actual references.
	// A path substring such as "rule_" is not a dependency classification:
	// an evidence record can legitimately contain that spelling, and a rule
	// file can live outside a directory with that name.
	rulePaths := map[string]string{}
	for _, ref := range run.Refs {
		if !strings.HasPrefix(ref.Ref, "rule:") {
			continue
		}
		rulePaths[changeReportKey(ref.Ref)] = ref.Ref
		if ref.Resolved != "" {
			rulePaths[changeReportKey(ref.Resolved)] = ref.Ref
		}
	}
	changedRules := map[string]bool{}
	changedPromises := map[string]bool{}
	providerChangedAny := false
	for _, entry := range report.Entries {
		if entry.Kind == "promise" {
			if entry.Promise != "" {
				changedPromises[entry.Promise] = true
			} else {
				providerChangedAny = true
			}
			continue
		}
		if ref, ok := rulePaths[changeReportKey(entry.Path)]; ok {
			changedRules[ref] = true
		}
		if evidenceRecordBase(entry.Path) != "" {
			changedEvidence = true
		}
	}
	scope.RuleChanged = len(changedRules) > 0
	scope.ProviderChanged = len(changedPromises) > 0 || providerChangedAny
	baselineObjects := map[string]bool{}
	for _, object := range state.ValidateObjects {
		baselineObjects[object.ID] = true
	}
	baselineReads := map[string][]string{}
	for _, object := range state.ValidateObjects {
		baselineReads[object.ID] = object.Reads
	}
	graphStructureChanged := len(state.ValidateObjects) == 0 || len(state.ValidateObjects) != len(run.ValidateObjects)
	// catalogStructureChanged is the narrower structural signal a repair uses:
	// the object set itself changed (an item, promise, section, or external
	// object appeared or disappeared). A read-set change is not structural —
	// it re-runs the affected object through the ordinary selection.
	catalogStructureChanged := graphStructureChanged
	if !catalogStructureChanged {
		currentObjects := map[string]bool{}
		for _, object := range run.ValidateObjects {
			currentObjects[object.ID] = true
			if !baselineObjects[object.ID] {
				catalogStructureChanged = true
			}
		}
		for id := range baselineObjects {
			if !currentObjects[id] {
				catalogStructureChanged = true
			}
		}
	}
	anyItemOrPromiseChanged := false
	for _, object := range run.ValidateObjects {
		switch {
		case strings.HasPrefix(object.ID, "item:") || strings.HasPrefix(object.ID, "promise:") || strings.HasPrefix(object.ID, "contradiction:"):
			for _, read := range object.Reads {
				if itemOrPromiseRefChanged(read, itemRange, promiseRange, regionChanged) {
					anyItemOrPromiseChanged = true
					break
				}
			}
		}
		if anyItemOrPromiseChanged {
			break
		}
	}
	anySectionChanged := false
	for _, section := range graph.Sections {
		if section.Level >= 3 && regionChanged(sectionRange[section.Heading]) {
			anySectionChanged = true
			break
		}
	}

	refChanged := func(ref string) bool {
		switch {
		case strings.HasPrefix(ref, "unit:"):
			return ownSpecChanged
		case strings.HasPrefix(ref, "section:"):
			return regionChanged(sectionRange[strings.TrimPrefix(ref, "section:")])
		case strings.HasPrefix(ref, "item:"):
			return regionChanged(itemRange[strings.TrimPrefix(ref, "item:")])
		case strings.HasPrefix(ref, "promise:"):
			return regionChanged(promiseRange[strings.TrimPrefix(ref, "promise:")])
		case strings.HasPrefix(ref, "contradiction:"):
			return false
		case strings.HasPrefix(ref, "rule:"):
			return changedRules[ref]
		case strings.HasPrefix(ref, "provider:"):
			return providerChangedAny || changedPromises[strings.TrimPrefix(ref, "provider:")]
		case strings.HasPrefix(ref, "evidence_section:"):
			return evidenceSectionChanged(ref)
		case strings.HasPrefix(ref, "evidence:"):
			return changedEvidence
		case strings.HasPrefix(ref, "file:"):
			path := strings.TrimPrefix(ref, "file:")
			base, ok := state.FilePresence[path]
			if !ok {
				// A baseline that predates file-presence recording cannot
				// prove the declaration unchanged. Fail safe: re-run the
				// object instead of carrying a judgment over a changed path.
				return true
			}
			return fileExists(filepath.Join(d.root, filepath.FromSlash(path))) != base
		case ref == "global:graph":
			return graphStructureChanged
		case ref == "global:narrative-coherence":
			return anySectionChanged
		case ref == "global:goal-means", ref == "global:cross-behavior-interactions":
			return anyItemOrPromiseChanged || anySectionChanged
		}
		return false
	}

	for _, object := range run.ValidateObjects {
		rerun := !baselineObjects[object.ID]
		base, hasBase := baselineReads[object.ID]
		if !hasBase || !sameReadSet(base, object.Reads) {
			// A changed dependency surface invalidates the object even when
			// no current line overlaps its own region: a removed/added item,
			// a changed promise provider list, or a changed rule binding can
			// move the judgment's inputs without touching the object's text.
			rerun = true
			graphStructureChanged = true
		}
		if !rerun {
			for _, read := range object.Reads {
				if refChanged(read) {
					rerun = true
					break
				}
			}
		}
		if rerun {
			scope.RerunObjects[object.ID] = true
			if strings.HasPrefix(object.ID, "rule:") {
				scope.RuleChanged = true
			}
			if object.ID == "global:rule-interactions" {
				scope.RuleChanged = true
			}
			if strings.HasPrefix(object.ID, "promise:") && strings.HasSuffix(object.ID, ":provider") {
				scope.ProviderChanged = true
			}
			if group := validateObjectGroup(object); group != "" {
				scope.SelectedGroups[group] = true
			}
		}
	}
	// An advisory is bound to its check key, not to one object (the report
	// carries no per-object verdict table). A check with recorded P2/P3
	// advisories is therefore atomic in a delta: when one of its objects
	// re-runs, every object of that check re-runs so the owning session can
	// re-judge the advisory instead of silently dropping it
	// (framework/validation_cache.md §Advisory carry).
	advisoryChecks := map[string]bool{}
	for _, finding := range state.Findings {
		if finding.Severity != "P2" && finding.Severity != "P3" {
			continue
		}
		for _, key := range BaselineFindingKeys(finding) {
			if IsRelationshipKey(key) {
				continue
			}
			advisoryChecks[key] = true
		}
	}
	if len(advisoryChecks) > 0 {
		forcedChecks := map[string]bool{}
		for _, object := range run.ValidateObjects {
			if scope.RerunObjects[object.ID] && advisoryChecks[object.Check] {
				forcedChecks[object.Check] = true
			}
		}
		for _, object := range run.ValidateObjects {
			if !forcedChecks[object.Check] || scope.RerunObjects[object.ID] {
				continue
			}
			scope.RerunObjects[object.ID] = true
			if group := validateObjectGroup(object); group != "" {
				scope.SelectedGroups[group] = true
			}
		}
		if len(forcedChecks) > 0 {
			names := make([]string, 0, len(forcedChecks))
			for name := range forcedChecks {
				names = append(names, name)
			}
			sort.Strings(names)
			scope.Notices = append(scope.Notices, "checks with recorded P2/P3 advisories re-run fully: "+strings.Join(names, ", "))
		}
	}
	scope.StructureChanged = catalogStructureChanged
	run.RerunObjects = make([]string, 0, len(scope.RerunObjects))
	for key := range scope.RerunObjects {
		run.RerunObjects = append(run.RerunObjects, key)
	}
	sort.Strings(run.RerunObjects)
	return scope, nil
}

func itemOrPromiseRefChanged(ref string, itemRange, promiseRange map[string][2]int, regionChanged func([2]int) bool) bool {
	switch {
	case strings.HasPrefix(ref, "item:"):
		return regionChanged(itemRange[strings.TrimPrefix(ref, "item:")])
	case strings.HasPrefix(ref, "promise:"):
		return regionChanged(promiseRange[strings.TrimPrefix(ref, "promise:")])
	case strings.HasPrefix(ref, "section:"):
		return false
	}
	return false
}

func sameReadSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left := append([]string(nil), a...)
	right := append([]string(nil), b...)
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// evidenceRecordBase reports whether a change-report path is the unit's
// evidence record, spelled physically (…/evidence/unit_<unit>.md) or
// logically (unit:<unit>:evidence). It returns "" for any other path.
func evidenceRecordBase(path string) string {
	clean := filepath.ToSlash(path)
	if strings.Contains(clean, "/evidence/") {
		return strings.TrimSuffix(filepath.Base(clean), ".md")
	}
	if strings.HasSuffix(clean, ":evidence") {
		return clean
	}
	return ""
}

// Provisional, documented cost weights for the delta-vs-full decision. They
// are deliberately simple and mechanical: a session is the execution unit and
// an object is the judgment unit. The comparison is net cost (delta work plus
// its planning/merge overhead), not a fixed percentage threshold; a delta that
// saves even a little is kept. Calibrate the constants from measured runs.
const (
	validateSessionCost   = 0.2
	validateObjectCost    = 0.05
	validateDeltaOverhead = 0.1
)

// validatePlanCost estimates the work of running the given coverage groups
// against the per-object catalog: one weight per session plus a small per
// object weight. When rerun is non-nil, only objects in that set are counted:
// a delta session judges the rerun objects of its group, not every object in
// the catalog.
func validatePlanCost(groups []CoverageKey, objects []validatekeys.Key, rerun map[string]bool) float64 {
	cost := float64(len(groups)) * validateSessionCost
	for _, group := range groups {
		for _, object := range objects {
			if rerun != nil && !rerun[object.ID] {
				continue
			}
			if validateObjectGroup(object) == group.Key {
				cost += validateObjectCost
			}
		}
	}
	return cost
}
