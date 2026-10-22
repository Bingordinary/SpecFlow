package specvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"gopkg.in/yaml.v3"
)

// Promise is one normative contract declared by a unit.
//
// A promise owns no code and no verification activity. Acceptance items
// reference it and carry the evidence/code scope.
type Promise struct {
	ID        string
	Summary   string
	Contract  map[string]any
	DependsOn []string
	// Raw carries every top-level field so schema checks can reject
	// verification or code fields without a second parser.
	Raw map[string]any
}

var (
	promiseIDPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	promiseForbiddenTopLevel = map[string]bool{
		"implementation_surface": true,
		"affects":                true,
		"verification_method":    true,
		"pass_condition":         true,
		"verified_by":            true,
		"consumers":              true,
		"consumer":               true,
		"code":                   true,
		"files":                  true,
		"tests":                  true,
		"evidence":               true,
	}
	// promiseAllowedTopLevel is the closed set of fields the deployed promise
	// format defines. An unknown field is not part of the fingerprint and could
	// otherwise carry a second, unfingerprinted contract.
	promiseAllowedTopLevel = map[string]bool{
		"id":         true,
		"summary":    true,
		"contract":   true,
		"depends_on": true,
	}
	// promiseContractBlocks is the closed vocabulary of contract blocks. Each
	// block states one kind of externally observable fact; a contract is a
	// combination of blocks, so a new project shape composes existing blocks
	// instead of adding a new promise kind.
	promiseContractBlocks = map[string]bool{
		"operations":    true,
		"data":          true,
		"occurrences":   true,
		"state_machine": true,
		"invariants":    true,
	}
	// promiseBlockFields is the closed field set of each contract block. An
	// unknown block field is rejected: the fingerprint covers `contract`, so an
	// unvalidated key would be contract truth outside the model's shape.
	promiseBlockFields = map[string]map[string]bool{
		"operations": {
			"signature": true, "inputs": true, "outputs": true, "errors": true, "timing": true,
		},
		"data": {
			"fields": true,
		},
		"occurrences": {
			"payload": true, "delivery": true, "ordering": true,
		},
		"state_machine": {
			"states": true, "initial": true, "transitions": true,
		},
	}
)

// HasPromisesSection reports whether the spec declares the promise layer.
func HasPromisesSection(content string) bool {
	_, ok := contenthash.LocateSectionRegion(specpaths.NormalizeText(content), "Promises")
	return ok
}

// CheckUnitSpecShape enforces the fixed three-section unit spec shape.
//
// The promise model needs stable section locations for mechanical parsing and
// region pinning: Design Narrative, Promises, Acceptance Items, in that order.
func CheckUnitSpecShape(content string) CheckResult {
	content = specpaths.NormalizeText(content)
	result := CheckResult{Name: "Unit spec shape", Status: Fail}

	regions := contenthash.SectionRegions(content)
	if len(regions) == 0 {
		result.Details = "spec has no sections"
		return result
	}
	frontmatter := ""
	var headings []string
	byHeading := map[string]string{}
	for _, region := range regions {
		if region.Heading == "" {
			frontmatter = region.Text
			continue
		}
		headings = append(headings, region.Heading)
		byHeading[region.Heading] = region.Text
	}

	if !strings.HasPrefix(strings.TrimSpace(frontmatter), "---") {
		result.Details = "unit spec must start with frontmatter"
		return result
	}

	want := []string{"Design Narrative", "Promises", "Acceptance Items"}
	if len(headings) != len(want) {
		result.Details = fmt.Sprintf("expected exactly 3 ## sections (%s), found %d: %s",
			strings.Join(want, ", "), len(headings), strings.Join(headings, ", "))
		return result
	}
	for i, heading := range want {
		if headings[i] != heading {
			result.Details = fmt.Sprintf("section %d must be %q, found %q", i+1, heading, headings[i])
			return result
		}
	}

	for _, heading := range []string{"Promises", "Acceptance Items"} {
		if hasDeeperHeading(byHeading[heading]) {
			result.Details = fmt.Sprintf("%q must contain structured content only; use no ### subheadings", heading)
			return result
		}
	}

	promisesSection := byHeading["Promises"]
	if !hasTopLevelMarker(promisesSection, "promises") {
		result.Details = `"Promises" section must contain a top-level promises: list`
		return result
	}
	if _, ok := contenthash.AcceptanceItemsRegion(byHeading["Acceptance Items"]); !ok {
		result.Details = `"Acceptance Items" section must contain acceptance_item_set:`
		return result
	}

	result.Status = Pass
	result.Details = "Design Narrative, Promises, Acceptance Items are present once and in order"
	return result
}

// hasDeeperHeading reports whether a section text contains a `###` heading.
func hasDeeperHeading(section string) bool {
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "### ") {
			return true
		}
	}
	return false
}

// hasTopLevelMarker reports whether a section contains the exact marker line
// `key:` (a bare structural marker with no same-line value), mirroring the
// acceptance-item-set locator: `promises: []` is not a structural marker line.
func hasTopLevelMarker(section, key string) bool {
	inFence := false
	var fenceChar byte
	fenceLen := 0
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if strings.HasPrefix(trimmed, strings.Repeat(string(fenceChar), fenceLen)) {
				inFence = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = true
			fenceChar = trimmed[0]
			fenceLen = 0
			for fenceLen < len(trimmed) && trimmed[fenceLen] == fenceChar {
				fenceLen++
			}
			continue
		}
		if trimmed == key+":" {
			return true
		}
	}
	return false
}

// ParsePromises parses the `## Promises` section. It returns every promise it
// can read plus a list of structural issues. Schema and coverage checks run
// separately so one malformed entry does not hide the others.
func ParsePromises(content string) ([]Promise, []string) {
	content = specpaths.NormalizeText(content)
	section, ok := contenthash.LocateSectionRegion(content, "Promises")
	if !ok {
		return nil, []string{"## Promises section not found"}
	}

	body := stripSectionHeading(section.Text)
	var root map[string]any
	if err := yaml.Unmarshal([]byte(body), &root); err != nil {
		return nil, []string{fmt.Sprintf("Promises section is not valid YAML: %v", err)}
	}
	rawList, ok := root["promises"]
	if !ok {
		return nil, []string{"Promises section must contain a top-level promises: list"}
	}
	list, ok := rawList.([]any)
	if !ok {
		return nil, []string{"promises: must be a YAML list"}
	}

	var promises []Promise
	var issues []string
	for i, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("promise %d: entry must be a mapping", i+1))
			continue
		}
		normalized := normalizeYAMLMap(entry)
		p := Promise{Raw: normalized}
		p.ID, _ = normalized["id"].(string)
		p.Summary, _ = normalized["summary"].(string)
		if contract, ok := normalized["contract"].(map[string]any); ok {
			p.Contract = contract
		}
		p.DependsOn, issues = parsePromiseStringList(normalized["depends_on"], fmt.Sprintf("promise %q depends_on", p.ID), issues)
		promises = append(promises, p)
	}
	return promises, issues
}

// stripSectionHeading removes the first line of a section region.
func stripSectionHeading(section string) string {
	if i := strings.IndexByte(section, '\n'); i >= 0 {
		return section[i+1:]
	}
	return ""
}

// parsePromiseStringList accepts a scalar or a YAML list.
func parsePromiseStringList(raw any, label string, issues []string) ([]string, []string) {
	if raw == nil {
		return nil, issues
	}
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" || value == "none" {
			return nil, issues
		}
		return []string{strings.TrimSpace(value)}, issues
	case []any:
		var out []string
		for _, entry := range value {
			text, ok := entry.(string)
			if !ok || strings.TrimSpace(text) == "" {
				issues = append(issues, fmt.Sprintf("%s: entries must be non-empty strings", label))
				continue
			}
			out = append(out, strings.TrimSpace(text))
		}
		return out, issues
	default:
		issues = append(issues, fmt.Sprintf("%s: must be a string or a list of strings", label))
		return nil, issues
	}
}

// CheckPromiseSchema validates promise fields and the no-code/no-verification
// boundary. Cross-promise resolution is owned by the dependency graph.
func CheckPromiseSchema(content, unitName string) CheckResult {
	result := CheckResult{Name: "Promise schema", Status: Fail}
	promises, issues := ParsePromises(content)
	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	if len(promises) == 0 {
		// A valid unit declares at least one promise (framework/core/promise_model.md).
		// This is the mechanical half of the promise model's minimum: with no
		// promise, promise coverage is vacuous and the model's guarantee that
		// every promise is covered by an acceptance item no longer implies the
		// unit has any acceptance item at all. The gate planner relies on that
		// implication to reject an item-less spec, so an empty promise list must
		// fail here rather than pass as "no promises declared".
		result.Details = "no promises declared: a valid unit declares at least one promise"
		return result
	}

	seen := map[string]bool{}
	for i, p := range promises {
		label := fmt.Sprintf("promise %d", i+1)
		if p.ID == "" {
			issues = append(issues, label+": missing or empty id")
			continue
		}
		label = fmt.Sprintf("promise %q", p.ID)
		if !promiseIDPattern.MatchString(p.ID) {
			issues = append(issues, label+": id must use letters, digits, '_', '-', or '.'")
		}
		if !strings.HasPrefix(p.ID, unitName+".") {
			issues = append(issues, fmt.Sprintf("%s: unit-owned promise id must start with %q", label, unitName+"."))
		}
		if seen[p.ID] {
			issues = append(issues, label+": duplicate id")
		}
		seen[p.ID] = true

		if strings.TrimSpace(p.Summary) == "" {
			issues = append(issues, label+": missing or empty summary")
		} else if strings.Contains(p.Summary, "\n") {
			issues = append(issues, label+": summary must be one line")
		}
		if len(p.Contract) == 0 {
			issues = append(issues, label+": missing or empty contract")
		} else {
			issues = append(issues, validateContractBlocks(label, p.Contract)...)
		}
		for field := range p.Raw {
			switch {
			case promiseForbiddenTopLevel[field]:
				issues = append(issues, fmt.Sprintf("%s: top-level field %q is not allowed on a promise", label, field))
			case !promiseAllowedTopLevel[field]:
				issues = append(issues, fmt.Sprintf("%s: unknown top-level field %q; allowed fields are id, summary, contract, depends_on", label, field))
			}
		}
		depSeen := map[string]bool{}
		for _, dep := range p.DependsOn {
			if dep == p.ID {
				issues = append(issues, label+": depends_on must not reference itself")
			} else if provider := PromiseProvider(dep); provider == unitName {
				issues = append(issues, fmt.Sprintf("%s: depends_on %q is owned by this unit; a promise depends on another unit's contract, not on a promise in the same unit", label, dep))
			}
			if depSeen[dep] {
				issues = append(issues, fmt.Sprintf("%s: duplicate depends_on entry %q", label, dep))
			}
			depSeen[dep] = true
		}
	}

	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	result.Status = Pass
	result.Details = fmt.Sprintf("%d promise(s) have valid schema", len(promises))
	return result
}

// CheckPromiseCoverage enforces the promise/acceptance-item relation:
// every contract item references a promise, every promise is covered by at
// least one item, and non-contract items are explicit.
func CheckPromiseCoverage(content, unitName string) CheckResult {
	result := CheckResult{Name: "Promise coverage", Status: Fail}
	promises, issues := ParsePromises(content)
	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	promiseIDs := map[string]bool{}
	for _, p := range promises {
		promiseIDs[p.ID] = true
	}

	items := parseAcceptanceItems(content)
	if len(items) == 0 {
		if len(promises) == 0 {
			result.Status = Pass
			result.Details = "no promises or acceptance items declared"
			return result
		}
		result.Details = "acceptance_item_set exists but no items found with - id:"
		return result
	}

	covered := map[string]bool{}
	for _, item := range items {
		label := fmt.Sprintf("acceptance item %q", item.id)
		promise := strings.TrimSpace(item.fields["promise"])
		nonPromise := strings.EqualFold(strings.TrimSpace(item.fields["non_promise"]), "true")
		reason := strings.TrimSpace(item.fields["non_promise_reason"])

		switch {
		case promise != "" && nonPromise:
			issues = append(issues, label+": cannot declare both promise and non_promise")
		case promise != "":
			if !promiseIDs[promise] {
				issues = append(issues, fmt.Sprintf("%s: references unknown promise %q", label, promise))
				continue
			}
			if !strings.HasPrefix(promise, unitName+".") {
				issues = append(issues, fmt.Sprintf("%s: verifies foreign promise %q; consumer dependencies belong on the consumer promise's depends_on", label, promise))
				continue
			}
			covered[promise] = true
		case nonPromise:
			if reason == "" {
				issues = append(issues, label+": non_promise item requires non_promise_reason")
			}
		default:
			issues = append(issues, label+": must declare promise or non_promise: true")
		}
	}

	for _, p := range promises {
		if !covered[p.ID] {
			issues = append(issues, fmt.Sprintf("promise %q: not covered by any acceptance item", p.ID))
		}
	}

	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	result.Status = Pass
	result.Details = fmt.Sprintf("%d promise(s) covered by %d acceptance item(s)", len(promises), len(items))
	return result
}

// CheckPromiseSpec runs the promise-model checks. Provider units for external
// promise dependencies are derived from depends_on by the gate planner.
func CheckPromiseSpec(content, unitName string) []CheckResult {
	return []CheckResult{
		CheckUnitSpecShape(content),
		CheckPromiseSchema(content, unitName),
		CheckPromiseCoverage(content, unitName),
	}
}

// validateContractBlocks checks the closed contract-block vocabulary and each
// block's field shape. It validates structure only; semantic compatibility
// remains a later diff. The block vocabulary is closed, but the combination of
// blocks is open: a contract that combines existing blocks never needs a new
// promise kind.
func validateContractBlocks(label string, contract map[string]any) []string {
	var issues []string
	for block := range contract {
		if !promiseContractBlocks[block] {
			issues = append(issues, fmt.Sprintf("%s: unknown contract block %q; allowed blocks are operations, data, occurrences, state_machine, invariants", label, block))
		}
	}
	if raw, ok := contract["operations"]; ok {
		issues = append(issues, validateOperationsBlock(label, raw)...)
	}
	if raw, ok := contract["data"]; ok {
		issues = append(issues, validateDataBlock(label, raw)...)
	}
	if raw, ok := contract["occurrences"]; ok {
		issues = append(issues, validateOccurrencesBlock(label, raw)...)
	}
	if raw, ok := contract["state_machine"]; ok {
		issues = append(issues, validateStateMachineBlock(label, raw)...)
	}
	if raw, ok := contract["invariants"]; ok {
		issues = append(issues, validateInvariantsBlock(label, raw)...)
	}
	return issues
}

func validateOperationsBlock(label string, raw any) []string {
	operations, ok := raw.(map[string]any)
	if !ok || len(operations) == 0 {
		return []string{label + ": operations must be a non-empty map of operation name to operation block"}
	}
	var issues []string
	for name, rawBlock := range operations {
		if strings.TrimSpace(name) == "" {
			issues = append(issues, label+": operations has an empty operation name")
			continue
		}
		block, ok := rawBlock.(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("%s: operation %q must be a map", label, name))
			continue
		}
		for field := range block {
			if !promiseBlockFields["operations"][field] {
				issues = append(issues, fmt.Sprintf("%s: operation %q has unknown field %q", label, name, field))
			}
		}
		hasShape := false
		if value, ok := block["signature"]; ok {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				issues = append(issues, fmt.Sprintf("%s: operation %q signature must be a non-empty string", label, name))
			} else {
				hasShape = true
			}
		}
		if value, ok := block["inputs"]; ok {
			fields, ok := value.(map[string]any)
			if !ok {
				issues = append(issues, fmt.Sprintf("%s: operation %q inputs must be a field map", label, name))
			} else {
				issues = append(issues, validatePromiseFields(fmt.Sprintf("%s.operations.%s.inputs", label, name), fields)...)
				hasShape = true
			}
		}
		if value, ok := block["outputs"]; ok {
			fields, ok := value.(map[string]any)
			if !ok {
				issues = append(issues, fmt.Sprintf("%s: operation %q outputs must be a field map", label, name))
			} else {
				issues = append(issues, validatePromiseFields(fmt.Sprintf("%s.operations.%s.outputs", label, name), fields)...)
				hasShape = true
			}
		}
		if value, ok := block["errors"]; ok {
			errors, ok := value.(map[string]any)
			if !ok {
				issues = append(issues, fmt.Sprintf("%s: operation %q errors must be a map", label, name))
			} else {
				for code, meaning := range errors {
					if strings.TrimSpace(fmt.Sprint(code)) == "" {
						issues = append(issues, fmt.Sprintf("%s: operation %q has an empty error code", label, name))
					}
					if text, ok := meaning.(string); !ok || strings.TrimSpace(text) == "" {
						issues = append(issues, fmt.Sprintf("%s: operation %q error %v must be a non-empty string", label, name, code))
					}
				}
			}
		}
		if value, ok := block["timing"]; ok {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				issues = append(issues, fmt.Sprintf("%s: operation %q timing must be a non-empty string", label, name))
			}
		}
		if !hasShape {
			issues = append(issues, fmt.Sprintf("%s: operation %q requires at least one of signature, inputs, or outputs", label, name))
		}
	}
	return issues
}

func validateDataBlock(label string, raw any) []string {
	data, ok := raw.(map[string]any)
	if !ok || len(data) == 0 {
		return []string{label + ": data must be a non-empty map of type name to data block"}
	}
	var issues []string
	for name, rawBlock := range data {
		if strings.TrimSpace(name) == "" {
			issues = append(issues, label+": data has an empty type name")
			continue
		}
		block, ok := rawBlock.(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("%s: data %q must be a map", label, name))
			continue
		}
		for field := range block {
			if !promiseBlockFields["data"][field] {
				issues = append(issues, fmt.Sprintf("%s: data %q has unknown field %q", label, name, field))
			}
		}
		fields, ok := block["fields"].(map[string]any)
		if !ok || len(fields) == 0 {
			issues = append(issues, fmt.Sprintf("%s: data %q requires a non-empty fields map", label, name))
			continue
		}
		issues = append(issues, validatePromiseFields(fmt.Sprintf("%s.data.%s.fields", label, name), fields)...)
	}
	return issues
}

func validateOccurrencesBlock(label string, raw any) []string {
	occurrences, ok := raw.(map[string]any)
	if !ok || len(occurrences) == 0 {
		return []string{label + ": occurrences must be a non-empty map of event name to occurrence block"}
	}
	var issues []string
	for name, rawBlock := range occurrences {
		if strings.TrimSpace(name) == "" {
			issues = append(issues, label+": occurrences has an empty event name")
			continue
		}
		block, ok := rawBlock.(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("%s: occurrence %q must be a map", label, name))
			continue
		}
		for field := range block {
			if !promiseBlockFields["occurrences"][field] {
				issues = append(issues, fmt.Sprintf("%s: occurrence %q has unknown field %q", label, name, field))
			}
		}
		payload, ok := block["payload"].(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("%s: occurrence %q requires a payload field map", label, name))
		} else {
			issues = append(issues, validatePromiseFields(fmt.Sprintf("%s.occurrences.%s.payload", label, name), payload)...)
		}
		for _, field := range []string{"delivery", "ordering"} {
			if value, ok := block[field]; ok {
				text, ok := value.(string)
				if !ok || strings.TrimSpace(text) == "" {
					issues = append(issues, fmt.Sprintf("%s: occurrence %q %s must be a non-empty string", label, name, field))
				}
			}
		}
	}
	return issues
}

func validateStateMachineBlock(label string, raw any) []string {
	contract, ok := raw.(map[string]any)
	if !ok {
		return []string{label + ": state_machine must be a map with states, initial, and transitions"}
	}
	var issues []string
	for field := range contract {
		if !promiseBlockFields["state_machine"][field] {
			issues = append(issues, fmt.Sprintf("%s: state_machine has unknown field %q", label, field))
		}
	}
	states, ok := stringList(contract["states"])
	if !ok || len(states) == 0 {
		issues = append(issues, label+": state_machine requires a non-empty states list")
	} else if duplicates := duplicateStrings(states); len(duplicates) > 0 {
		issues = append(issues, fmt.Sprintf("%s: state_machine states duplicate %s", label, strings.Join(duplicates, ", ")))
	}
	stateSet := map[string]bool{}
	for _, state := range states {
		stateSet[state] = true
	}
	if initial, ok := contract["initial"].(string); !ok || strings.TrimSpace(initial) == "" {
		issues = append(issues, label+": state_machine requires initial")
	} else if len(stateSet) > 0 && !stateSet[initial] {
		issues = append(issues, label+": state_machine initial state is not in states")
	}
	transitions, ok := contract["transitions"].([]any)
	if !ok || len(transitions) == 0 {
		issues = append(issues, label+": state_machine requires a non-empty transitions list")
		return issues
	}
	for i, raw := range transitions {
		entry, ok := raw.(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("%s: state_machine transition %d must be a map", label, i+1))
			continue
		}
		for _, field := range []string{"from", "event", "to"} {
			value, ok := entry[field].(string)
			if !ok || strings.TrimSpace(value) == "" {
				issues = append(issues, fmt.Sprintf("%s: state_machine transition %d requires %s", label, i+1, field))
			}
		}
		if from, ok := entry["from"].(string); ok && len(stateSet) > 0 && !stateSet[from] {
			issues = append(issues, fmt.Sprintf("%s: state_machine transition %d from state %q is not in states", label, i+1, from))
		}
		if to, ok := entry["to"].(string); ok && len(stateSet) > 0 && !stateSet[to] {
			issues = append(issues, fmt.Sprintf("%s: state_machine transition %d to state %q is not in states", label, i+1, to))
		}
	}
	return issues
}

func validateInvariantsBlock(label string, raw any) []string {
	invariants, ok := stringList(raw)
	if !ok || len(invariants) == 0 {
		return []string{label + ": invariants must be a non-empty string list"}
	}
	return nil
}

func validatePromiseFields(label string, fields map[string]any) []string {
	var issues []string
	for name, raw := range fields {
		if strings.TrimSpace(name) == "" {
			issues = append(issues, label+": empty field name")
			continue
		}
		switch value := raw.(type) {
		case string:
			if strings.TrimSpace(value) == "" {
				issues = append(issues, fmt.Sprintf("%s.%s: type must be non-empty", label, name))
			}
		case map[string]any:
			typeName, ok := value["type"].(string)
			if !ok || strings.TrimSpace(typeName) == "" {
				issues = append(issues, fmt.Sprintf("%s.%s: missing or empty type", label, name))
			}
			for _, flag := range []string{"required", "nullable"} {
				if rawFlag, ok := value[flag]; ok {
					if _, ok := rawFlag.(bool); !ok {
						issues = append(issues, fmt.Sprintf("%s.%s: %s must be a boolean", label, name, flag))
					}
				}
			}
		default:
			issues = append(issues, fmt.Sprintf("%s.%s: field spec must be a type string or a map with type", label, name))
		}
	}
	return issues
}

func stringList(raw any) ([]string, bool) {
	list, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, entry := range list {
		text, ok := entry.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, false
		}
		out = append(out, text)
	}
	return out, true
}

func duplicateStrings(values []string) []string {
	seen := map[string]bool{}
	var duplicates []string
	for _, value := range values {
		if seen[value] {
			duplicates = append(duplicates, value)
			continue
		}
		seen[value] = true
	}
	return duplicates
}

// PromiseFingerprint returns the canonical fingerprint of a promise contract.
// The fingerprint covers the contract blocks only. Identity, summary, and graph
// edges are excluded so a provider can rename a promise or update prose
// without reopening consumers.
func PromiseFingerprint(p Promise) (string, error) {
	payload := map[string]any{
		"contract": normalizeYAMLValue(p.Contract),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode promise %q: %w", p.ID, err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// normalizeYAMLMap recursively converts YAML maps with non-string keys into
// map[string]any so JSON encoding is deterministic.
func normalizeYAMLMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = normalizeYAMLValue(value)
	}
	return out
}

func normalizeYAMLValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return normalizeYAMLMap(v)
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, entry := range v {
			out[fmt.Sprint(key)] = normalizeYAMLValue(entry)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, entry := range v {
			out[i] = normalizeYAMLValue(entry)
		}
		return out
	default:
		return value
	}
}
