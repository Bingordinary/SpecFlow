package validationcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validatekeys"
)

// validateObjectJudgments is the cache-side view of the per-object validate
// record: the catalog the run was judged against. The catalog is the coverage
// record — a check verdict covers every object the check declares — so no
// separate per-object verdict is stored.
type validateObjectJudgments struct {
	SchemaVersion   int                `json:"schema_version"`
	ValidateObjects []validatekeys.Key `json:"validate_objects"`
	FilePresence    map[string]bool    `json:"file_presence,omitempty"`
}

// checkValidateObjectClosure enforces the per-object completeness rule on a
// fresh unit validate cache: every current catalog key must be recorded with
// the same check assignment and read set. The check is key-presence and
// dependency-surface equality only; content freshness is handled by the
// whole-file check chain and, for delta, by the planner.
func checkValidateObjectClosure(repoRoot, unitName, specRel string) (CheckResult, error) {
	cachePath, err := cacheFilePath(repoRoot, "unit", unitName, "validate_result.md")
	if err != nil {
		return CheckResult{}, err
	}
	cache, err := readCache(cachePath)
	if err != nil {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: fmt.Sprintf("cannot read validate cache at %s: %v", relPath(repoRoot, cachePath), err)}, nil
	}
	return checkValidateObjectClosureForCache(repoRoot, unitName, specRel, cache)
}

// checkValidateObjectClosureForCache is the closure check over an already
// parsed cache. The rendered-cache self-check passes the in-memory candidate
// through this same function, so gate-finalize, fresh, and promote judge one
// catalog derivation.
func checkValidateObjectClosureForCache(repoRoot, unitName, specRel string, cache *cacheFile) (CheckResult, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(specRel)))
	if err != nil {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: fmt.Sprintf("cannot read spec %s: %v", specRel, err)}, nil
	}
	graph, err := validatekeys.Parse(unitName, string(data))
	if err != nil {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: "validate key catalog cannot be derived: " + err.Error()}, nil
	}
	if len(graph.UnresolvedRefs) > 0 {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: "validate key catalog has unresolved refs: " + strings.Join(graph.UnresolvedRefs, ", ")}, nil
	}
	contract, err := validatekeys.DefaultContract()
	if err != nil {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: "validate check contract cannot be loaded: " + err.Error()}, nil
	}
	evidenceSections, err := evidenceSectionsForCache(repoRoot, unitName, specRel, graph)
	if err != nil {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: "evidence keys cannot be derived: " + err.Error()}, nil
	}
	evidenceBase := ""
	if len(evidenceSections) > 0 {
		evidenceBase = evidenceSections[0].Base
	}
	if strings.TrimSpace(cache.Judgments) == "" {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: "validate cache has no per-object catalog — run the full validate"}, nil
	}
	var recorded validateObjectJudgments
	if err := json.Unmarshal([]byte(cache.Judgments), &recorded); err != nil {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: "validate cache has an unreadable judgment baseline: " + err.Error()}, nil
	}
	if recorded.SchemaVersion != RecordStateSchemaValidate {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: fmt.Sprintf("validate cache judgment schema is %d, want %d — run the full validate", recorded.SchemaVersion, RecordStateSchemaValidate)}, nil
	}
	if len(recorded.ValidateObjects) == 0 {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: "validate cache has no per-object catalog — run the full validate"}, nil
	}
	rules, err := validateCatalogRules(repoRoot, recorded.ValidateObjects)
	if err != nil {
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: "validate catalog rule inputs cannot be derived: " + err.Error()}, nil
	}
	catalog := validatekeys.BuildCatalog(graph, validatekeys.CatalogInput{
		Unit:             unitName,
		Rules:            rules,
		EvidenceBase:     evidenceBase,
		EvidenceSections: evidenceSections,
	}, contract).Keys
	recordedObjects := make(map[string]validatekeys.Key, len(recorded.ValidateObjects))
	for _, object := range recorded.ValidateObjects {
		recordedObjects[object.ID] = object
	}
	var missing []string
	for _, key := range catalog {
		if _, ok := recordedObjects[key.ID]; !ok {
			missing = append(missing, key.ID)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		shown := missing
		if len(shown) > 8 {
			shown = shown[:8]
		}
		reason := fmt.Sprintf("validate cache is missing %d per-object judgment(s): %s", len(missing), strings.Join(shown, ", "))
		if len(missing) > len(shown) {
			reason += fmt.Sprintf(", and %d more", len(missing)-len(shown))
		}
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: reason}, nil
	}
	// The judgment must have been produced against the same dependency
	// surface. A key that survived while its reads moved is a stale
	// judgment, not a reusable one: the tool cannot prove the old judgment
	// covered the new inputs.
	for _, key := range catalog {
		recorded, ok := recordedObjects[key.ID]
		if !ok {
			return CheckResult{Fresh: false, Category: CategoryStale, Reason: fmt.Sprintf("validate cache has no recorded dependency surface for object %q — run the full validate", key.ID)}, nil
		}
		if recorded.Check != key.Check {
			return CheckResult{Fresh: false, Category: CategoryStale, Reason: fmt.Sprintf("validate object %q changed its check assignment — run the full validate", key.ID)}, nil
		}
		if !sameReadSet(recorded.Reads, key.Reads) {
			return CheckResult{Fresh: false, Category: CategoryStale, Reason: fmt.Sprintf("validate object %q changed its dependency surface — run the full validate", key.ID)}, nil
		}
	}
	for path, present := range recorded.FilePresence {
		info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(path)))
		current := err == nil && !info.IsDir()
		if current != present {
			return CheckResult{Fresh: false, Category: CategoryStale, Reason: fmt.Sprintf("declared file %s changed its presence — run the full validate", path)}, nil
		}
	}
	return CheckResult{Fresh: true, Category: CategoryFresh}, nil
}

// validateCatalogRules returns the rule ids the current object catalog must
// derive. The planner builds the recorded catalog from the spec's declared
// rules, the published stable global rules that apply to every unit by default,
// and any rule file the input manifest supplied. BuildCatalog recovers the
// declared rules from the parsed graph; this function supplies the other two:
// the current stable global set (so a newly published global rule stales the
// cache), and the rule references the recorded catalog was built with. The
// recorded read sets are used instead of the cache's files list because
// gate-extend may add a read-only rule input without making it a required
// catalog object.
func validateCatalogRules(repoRoot string, recorded []validatekeys.Key) ([]string, error) {
	rules, err := specpaths.StableGlobalRuleIDs(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, key := range recorded {
		for _, read := range key.Reads {
			ruleID, ok := strings.CutPrefix(read, "rule:")
			if !ok || ruleID == "" || specpaths.ValidateTargetName("rule", ruleID) != nil {
				continue
			}
			rules = append(rules, ruleID)
		}
	}
	return rules, nil
}

// evidenceSectionsForCache parses the unit's evidence record with its Keys
// declarations, so the cache closure derives the same per-object catalog the
// planner assigned.
func evidenceSectionsForCache(repoRoot, unitName, specRel string, graph *validatekeys.Graph) ([]validatekeys.EvidenceSection, error) {
	layer := "candidate"
	if strings.Contains(filepath.ToSlash(specRel), "/stable/") {
		layer = "stable"
	}
	record, err := specpaths.UnitEvidenceRecord(repoRoot, unitName, layer)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	path := record.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, filepath.FromSlash(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(filepath.Base(record.Path), ".md")
	return validatekeys.ParseEvidenceRecord(base, string(data), graph)
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
